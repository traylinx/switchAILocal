package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// HeartbeatMonitorImpl implements the HeartbeatMonitor interface.
// It provides background monitoring of provider health with configurable intervals.
type HeartbeatMonitorImpl struct {
	// Configuration
	config *HeartbeatConfig

	// Provider health checkers
	checkers map[string]ProviderHealthChecker

	// Current health status for each provider
	statuses map[string]*HealthStatus

	// Event handlers
	eventHandlers []HeartbeatEventHandler

	// Statistics
	stats *HeartbeatStats

	// Synchronization
	mu sync.RWMutex

	// Background monitoring
	ctx    context.Context
	cancel context.CancelFunc
	ticker *time.Ticker
	done   chan struct{}

	// Running state
	running bool
}

// NewHeartbeatMonitor creates a new heartbeat monitor with the given configuration.
func NewHeartbeatMonitor(config *HeartbeatConfig) HeartbeatMonitor {
	if config == nil {
		config = DefaultHeartbeatConfig()
	}

	return &HeartbeatMonitorImpl{
		config:        config,
		checkers:      make(map[string]ProviderHealthChecker),
		statuses:      make(map[string]*HealthStatus),
		eventHandlers: make([]HeartbeatEventHandler, 0),
		stats: &HeartbeatStats{
			StartTime: time.Now(),
		},
		done: make(chan struct{}),
	}
}

// Start begins the heartbeat monitoring loop.
func (hm *HeartbeatMonitorImpl) Start(ctx context.Context) error {
	hm.mu.Lock()

	if !hm.config.Enabled {
		hm.mu.Unlock()
		return fmt.Errorf("heartbeat monitoring is disabled")
	}

	if hm.running {
		hm.mu.Unlock()
		return fmt.Errorf("heartbeat monitor is already running")
	}

	// Create context for background monitoring
	hm.ctx, hm.cancel = context.WithCancel(ctx)

	// Initialize statistics
	hm.stats.StartTime = time.Now()
	hm.stats.ProvidersMonitored = len(hm.checkers)

	// Start the monitoring loop
	hm.ticker = time.NewTicker(hm.config.Interval)
	hm.running = true

	// Capture config for event data while locked
	intervalStr := hm.config.Interval.String()
	providersCount := len(hm.checkers)
	autoDiscovery := hm.config.AutoDiscovery
	quotaMonitoring := hm.config.QuotaWarningThreshold > 0

	hm.mu.Unlock()

	// Emit start event (without holding lock)
	hm.emitEvent(&HeartbeatEvent{
		Type:      EventHeartbeatStarted,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"interval":         intervalStr,
			"providers_count":  providersCount,
			"auto_discovery":   autoDiscovery,
			"quota_monitoring": quotaMonitoring,
		},
	})

	// Start background monitoring goroutine
	go hm.monitoringLoop()

	// Perform initial health check
	go func() {
		if err := hm.CheckAll(hm.ctx); err != nil {
			log.Debugf("Initial health check failed: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the monitor.
func (hm *HeartbeatMonitorImpl) Stop() error {
	hm.mu.Lock()

	if !hm.running {
		hm.mu.Unlock()
		return nil // Already stopped
	}

	// Cancel context to stop background operations
	if hm.cancel != nil {
		hm.cancel()
	}

	// Stop ticker
	if hm.ticker != nil {
		hm.ticker.Stop()
	}

	hm.mu.Unlock()

	// Wait for monitoring loop to finish with timeout
	select {
	case <-hm.done:
		// Cleanup finished
	case <-time.After(5 * time.Second):
		log.Warn("Heartbeat monitor stop timed out waiting for loop")
	}

	hm.mu.Lock()
	hm.running = false

	// Capture stats for event
	totalCycles := hm.stats.TotalCycles
	totalChecks := hm.stats.TotalChecks
	successfulChecks := hm.stats.SuccessfulChecks
	failedChecks := hm.stats.FailedChecks
	uptime := time.Since(hm.stats.StartTime).String()
	hm.mu.Unlock()

	// Emit stop event (without holding lock)
	hm.emitEvent(&HeartbeatEvent{
		Type:      EventHeartbeatStopped,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"total_cycles":      totalCycles,
			"total_checks":      totalChecks,
			"successful_checks": successfulChecks,
			"failed_checks":     failedChecks,
			"uptime":            uptime,
		},
	})

	return nil
}

// monitoringLoop runs the background monitoring cycle.
func (hm *HeartbeatMonitorImpl) monitoringLoop() {
	defer close(hm.done)

	for {
		select {
		case <-hm.ctx.Done():
			return
		case <-hm.ticker.C:
			// Perform health checks
			if err := hm.CheckAll(hm.ctx); err != nil {
				log.Debugf("Scheduled health check failed: %v", err)
			}

			// Update statistics
			hm.mu.Lock()
			hm.stats.TotalCycles++
			hm.stats.LastCycleTime = time.Now()

			// Calculate average cycle time
			if hm.stats.TotalCycles > 0 {
				totalTime := time.Since(hm.stats.StartTime)
				hm.stats.AverageCycleTime = totalTime / time.Duration(hm.stats.TotalCycles)
			}

			// Update provider counts
			hm.updateProviderCounts()
			hm.mu.Unlock()
		}
	}
}

// CheckAll performs health checks on all registered providers.
func (hm *HeartbeatMonitorImpl) CheckAll(ctx context.Context) error {
	hm.mu.RLock()
	checkers := make([]ProviderHealthChecker, 0, len(hm.checkers))
	for _, checker := range hm.checkers {
		checkers = append(checkers, checker)
	}
	hm.mu.RUnlock()

	if len(checkers) == 0 {
		return nil // No providers to check
	}

	// Create semaphore to limit concurrent checks
	semaphore := make(chan struct{}, hm.config.MaxConcurrentChecks)

	// Use WaitGroup to wait for all checks to complete
	var wg sync.WaitGroup

	for _, checker := range checkers {
		wg.Add(1)

		go func(c ProviderHealthChecker) {
			// Panic recovery to prevent goroutine leaks
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("Panic in health check for %s: %v", c.GetName(), r)
				}
				wg.Done()
				<-semaphore
			}()

			// Acquire semaphore
			semaphore <- struct{}{}

			// Perform health check with timeout
			checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)
			defer cancel()

			if err := hm.checkProviderWithRetry(checkCtx, c); err != nil {
				log.Debugf("Provider health check failed: %v", err)
			}
		}(checker)
	}

	// Wait for all checks to complete
	wg.Wait()

	return nil
}

// checkProviderWithRetry performs a health check with retry logic.
func (hm *HeartbeatMonitorImpl) checkProviderWithRetry(ctx context.Context, checker ProviderHealthChecker) error {
	var lastErr error

	for attempt := 0; attempt <= hm.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(hm.config.RetryDelay):
			}
		}

		status, err := checker.Check(ctx)

		if err != nil {
			lastErr = err

			hm.mu.Lock()
			hm.stats.TotalChecks++
			hm.stats.FailedChecks++
			hm.mu.Unlock()

			// Emit health check failed event
			hm.emitEvent(&HeartbeatEvent{
				Type:      EventHealthCheckFailed,
				Provider:  checker.GetName(),
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"error":   err.Error(),
					"attempt": attempt + 1,
				},
			})

			continue // Retry
		}

		// Health check succeeded
		hm.mu.Lock()
		hm.stats.TotalChecks++
		hm.stats.SuccessfulChecks++
		hm.mu.Unlock()

		// Update status and check for changes
		hm.updateProviderStatus(status)

		return nil
	}

	// All retries failed, mark provider as unavailable
	unavailableStatus := &HealthStatus{
		Provider:     checker.GetName(),
		Status:       StatusUnavailable,
		LastCheck:    time.Now(),
		ErrorMessage: fmt.Sprintf("Health check failed after %d attempts: %v", hm.config.RetryAttempts+1, lastErr),
	}

	hm.updateProviderStatus(unavailableStatus)

	return lastErr
}

// CheckProvider performs a health check on a specific provider.
func (hm *HeartbeatMonitorImpl) CheckProvider(ctx context.Context, provider string) (*HealthStatus, error) {
	hm.mu.RLock()
	checker, exists := hm.checkers[provider]
	hm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("provider %s not registered", provider)
	}

	// Create timeout context
	checkCtx, cancel := context.WithTimeout(ctx, hm.config.Timeout)
	defer cancel()

	// Perform health check with retry
	if err := hm.checkProviderWithRetry(checkCtx, checker); err != nil {
		return nil, fmt.Errorf("health check failed for provider %s: %w", provider, err)
	}

	// Return current status
	status, _ := hm.GetStatus(provider)
	return status, nil
}

// updateProviderStatus updates the status for a provider and emits events for changes.
func (hm *HeartbeatMonitorImpl) updateProviderStatus(newStatus *HealthStatus) {
	hm.mu.Lock()

	provider := newStatus.Provider
	previousStatus := hm.statuses[provider]

	// Update status
	hm.statuses[provider] = newStatus

	var events []*HeartbeatEvent

	// Check for status changes
	if previousStatus == nil || previousStatus.Status != newStatus.Status {
		// Status changed, Prepare appropriate event
		var eventType HeartbeatEventType

		switch newStatus.Status {
		case StatusHealthy:
			eventType = EventProviderHealthy
		case StatusDegraded:
			eventType = EventProviderDegraded
		case StatusUnavailable:
			eventType = EventProviderUnavailable
		}

		events = append(events, &HeartbeatEvent{
			Type:           eventType,
			Provider:       provider,
			Timestamp:      time.Now(),
			Status:         newStatus,
			PreviousStatus: previousStatus,
		})
	}

	// Check quota thresholds
	if newStatus.QuotaUsed > 0 && newStatus.QuotaLimit > 0 {
		quotaRatio := newStatus.QuotaUsed / newStatus.QuotaLimit

		// Check critical threshold
		if quotaRatio >= hm.config.QuotaCriticalThreshold {
			events = append(events, &HeartbeatEvent{
				Type:      EventQuotaCritical,
				Provider:  provider,
				Timestamp: time.Now(),
				Status:    newStatus,
				Data: map[string]interface{}{
					"quota_used":  newStatus.QuotaUsed,
					"quota_limit": newStatus.QuotaLimit,
					"quota_ratio": quotaRatio,
					"threshold":   hm.config.QuotaCriticalThreshold,
				},
			})
		} else if quotaRatio >= hm.config.QuotaWarningThreshold {
			// Check warning threshold
			events = append(events, &HeartbeatEvent{
				Type:      EventQuotaWarning,
				Provider:  provider,
				Timestamp: time.Now(),
				Status:    newStatus,
				Data: map[string]interface{}{
					"quota_used":  newStatus.QuotaUsed,
					"quota_limit": newStatus.QuotaLimit,
					"quota_ratio": quotaRatio,
					"threshold":   hm.config.QuotaWarningThreshold,
				},
			})
		}
	}

	hm.mu.Unlock()

	// Emit events after releasing lock
	for _, event := range events {
		hm.emitEvent(event)
	}
}

// GetStatus retrieves the last known status for a provider.
func (hm *HeartbeatMonitorImpl) GetStatus(provider string) (*HealthStatus, error) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	status, exists := hm.statuses[provider]
	if !exists {
		return nil, fmt.Errorf("no status available for provider %s", provider)
	}

	// Return a copy to prevent external modification
	statusCopy := *status
	return &statusCopy, nil
}

// GetAllStatuses retrieves the last known status for all providers.
func (hm *HeartbeatMonitorImpl) GetAllStatuses() map[string]*HealthStatus {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*HealthStatus, len(hm.statuses))

	for provider, status := range hm.statuses {
		// Return copies to prevent external modification
		statusCopy := *status
		result[provider] = &statusCopy
	}

	return result
}

// RegisterChecker registers a new provider health checker.
func (hm *HeartbeatMonitorImpl) RegisterChecker(checker ProviderHealthChecker) error {
	if checker == nil {
		return fmt.Errorf("checker cannot be nil")
	}

	provider := checker.GetName()
	if provider == "" {
		return fmt.Errorf("checker must have a non-empty name")
	}

	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.checkers[provider] = checker
	hm.stats.ProvidersMonitored = len(hm.checkers)

	return nil
}

// UnregisterChecker removes a provider health checker.
func (hm *HeartbeatMonitorImpl) UnregisterChecker(provider string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	delete(hm.checkers, provider)
	delete(hm.statuses, provider)
	hm.stats.ProvidersMonitored = len(hm.checkers)

	return nil
}

// SetInterval updates the heartbeat check interval.
func (hm *HeartbeatMonitorImpl) SetInterval(interval time.Duration) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.config.Interval = interval

	// Update ticker if running
	if hm.running && hm.ticker != nil {
		hm.ticker.Stop()
		hm.ticker = time.NewTicker(interval)
	}
}

// GetInterval returns the current heartbeat check interval.
func (hm *HeartbeatMonitorImpl) GetInterval() time.Duration {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	return hm.config.Interval
}

// AddEventHandler registers an event handler for heartbeat events.
func (hm *HeartbeatMonitorImpl) AddEventHandler(handler HeartbeatEventHandler) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.eventHandlers = append(hm.eventHandlers, handler)
}

// RemoveEventHandler removes an event handler.
func (hm *HeartbeatMonitorImpl) RemoveEventHandler(handler HeartbeatEventHandler) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	for i, h := range hm.eventHandlers {
		if h == handler {
			hm.eventHandlers = append(hm.eventHandlers[:i], hm.eventHandlers[i+1:]...)
			break
		}
	}
}

// emitEvent sends an event to all registered handlers.
func (hm *HeartbeatMonitorImpl) emitEvent(event *HeartbeatEvent) {
	// Don't hold lock while calling handlers to avoid deadlocks
	hm.mu.RLock()
	handlers := make([]HeartbeatEventHandler, len(hm.eventHandlers))
	copy(handlers, hm.eventHandlers)
	hm.mu.RUnlock()

	// Call handlers asynchronously to avoid blocking
	for _, handler := range handlers {
		go func(h HeartbeatEventHandler) {
			if err := h.HandleEvent(event); err != nil {
				log.Errorf("Heartbeat event handler failed: %v", err)
			}
		}(handler)
	}
}

// updateProviderCounts updates the provider count statistics.
func (hm *HeartbeatMonitorImpl) updateProviderCounts() {
	healthy := 0
	degraded := 0
	unavailable := 0

	for _, status := range hm.statuses {
		switch status.Status {
		case StatusHealthy:
			healthy++
		case StatusDegraded:
			degraded++
		case StatusUnavailable:
			unavailable++
		}
	}

	hm.stats.HealthyProviders = healthy
	hm.stats.DegradedProviders = degraded
	hm.stats.UnavailableProviders = unavailable
}

// GetStats returns current heartbeat monitor statistics.
func (hm *HeartbeatMonitorImpl) GetStats() *HeartbeatStats {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	// Return a copy to prevent external modification
	statsCopy := *hm.stats
	return &statsCopy
}

// IsRunning returns true if the heartbeat monitor is currently running.
func (hm *HeartbeatMonitorImpl) IsRunning() bool {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	return hm.running
}

// GetConfig returns the current heartbeat configuration.
func (hm *HeartbeatMonitorImpl) GetConfig() *HeartbeatConfig {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	// Return a copy to prevent external modification
	configCopy := *hm.config
	return &configCopy
}
