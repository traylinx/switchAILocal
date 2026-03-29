package autoroute

import (
	"context"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/observability"
)

// ProviderHealthState tracks the live latency, availability, and quota health
// of a specific provider.
type ProviderHealthState struct {
	LastProbe       time.Time
	Status          string        // "healthy", "degraded", "unavailable"
	Latency         time.Duration // Exponential moving average
	SuccessRate     float64       // Sliding window approximation
	QuotaRemaining  float64       // 0.0 - 1.0
	ConsecutiveFail int
	CoolingDown     bool
	CooldownUntil   time.Time
}

// ProviderHealthMonitor runs passively and actively to maintain provider health.
type ProviderHealthMonitor struct {
	interval       time.Duration
	startupGrace   time.Duration
	cooldownCycles int
	maxProbesPerHr int

	providers map[string]*ProviderHealthState
	mu        sync.RWMutex
	stopCh    chan struct{}
	stopOnce  sync.Once

	resolver *AutoResolver // The resolver to update dynamically
}

// NewProviderHealthMonitor initializes the monitoring subsystem.
func NewProviderHealthMonitor(resolver *AutoResolver, interval time.Duration) *ProviderHealthMonitor {
	if interval == 0 {
		interval = 60 * time.Second
	}
	return &ProviderHealthMonitor{
		interval:       interval,
		startupGrace:   10 * time.Second,
		cooldownCycles: 3,
		maxProbesPerHr: 120,
		providers:      make(map[string]*ProviderHealthState),
		stopCh:         make(chan struct{}),
		resolver:       resolver,
	}
}

// Start begins the background active monitoring loop.
func (m *ProviderHealthMonitor) Start(ctx context.Context) {
	log.WithField("interval", m.interval).Info("Starting ProviderHealthMonitor")
	go m.loop(ctx)
}

// Stop halts the background monitoring. Safe to call multiple times.
func (m *ProviderHealthMonitor) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

func (m *ProviderHealthMonitor) loop(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.activeProbeCycle()
		}
	}
}

// activeProbeCycle executes lightweight active health checks on all tracked providers.
func (m *ProviderHealthMonitor) activeProbeCycle() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// In Phase E we would query via DiscoveryService or run lightweight pings.
	// For now, we evaluate cooldowns and reset state if cooled down.
	now := time.Now()
	changed := false

	for provider, state := range m.providers {
		if state.CoolingDown && now.After(state.CooldownUntil) {
			log.WithField("provider", provider).Info("Provider cooldown expired, recovering to degraded state")
			state.CoolingDown = false
			state.Status = "degraded"
			state.ConsecutiveFail = 2 // Give it a slight penalty still
			changed = true
		}
	}

	if changed {
		m.syncToResolverLocked()
	}
}

// RecordRequestOutcome is called by the handler pipeline natively on every request.
func (m *ProviderHealthMonitor) RecordRequestOutcome(provider string, latency time.Duration, success bool, httpCode int, headers http.Header) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.providers[provider]
	if !exists {
		state = &ProviderHealthState{
			Status:         "healthy",
			SuccessRate:    1.0,
			QuotaRemaining: 1.0,
		}
		m.providers[provider] = state
	}

	state.LastProbe = time.Now()

	// Update Latency EMA (alpha = 0.2 for smoothing)
	if state.Latency == 0 {
		state.Latency = latency
	} else {
		state.Latency = time.Duration(float64(state.Latency)*0.8 + float64(latency)*0.2)
	}

	// ── Phase F: Rate-Limit Header Intelligence ──
	// Parse provider-specific rate-limit headers and update quota health
	// BEFORE a hard 429 is hit. This allows the scoring engine to
	// proactively route away from providers approaching exhaustion.
	rlSnap := ParseRateLimitHeaders(provider, headers)
	if rlSnap.Detected {
		state.QuotaRemaining = rlSnap.QuotaHealth
		log.WithFields(log.Fields{
			"provider":          provider,
			"quota_health":      rlSnap.QuotaHealth,
			"req_remaining":     rlSnap.RequestRemaining,
			"req_limit":         rlSnap.RequestLimit,
			"token_remaining":   rlSnap.TokenRemaining,
			"token_limit":       rlSnap.TokenLimit,
			"retry_after_sec":   rlSnap.RetryAfterSec,
		}).Debug("Rate-limit headers parsed")
	}

	// Update Success Rate
	if success {
		state.ConsecutiveFail = 0
		state.SuccessRate = state.SuccessRate*0.9 + 1.0*0.1
		if state.Status != "healthy" && !state.CoolingDown {
			log.WithField("provider", provider).Info("Provider recovered, marking healthy")
			state.Status = "healthy"
		}
	} else {
		state.ConsecutiveFail++
		state.SuccessRate = state.SuccessRate*0.9 + 0.0*0.1

		// 429 mapping (Quota Exhaustion) — still important as a hard backstop
		if httpCode == http.StatusTooManyRequests {
			state.QuotaRemaining = 0.0
			state.ConsecutiveFail = 10 // Instantly trip breaker
			log.WithField("provider", provider).Warn("Provider hit HTTP 429 Rate Limit exhaustion")
		}

		// State transitions
		if state.ConsecutiveFail >= 10 && !state.CoolingDown {
			state.Status = "unavailable"
			state.CoolingDown = true
			state.CooldownUntil = time.Now().Add(5 * time.Minute)
			log.WithFields(log.Fields{
				"provider": provider,
				"duration": "5m",
			}).Warn("Provider circuit breaker tripped, entering cooldown")
		} else if state.ConsecutiveFail >= 3 && state.Status == "healthy" {
			state.Status = "degraded"
			log.WithField("provider", provider).Warn("Provider health degraded")
		}
	}

	m.syncToResolverLocked()
}

// syncToResolverLocked maps internal health states back into CandidateInputs
// and pushes them into the AutoResolver.
func (m *ProviderHealthMonitor) syncToResolverLocked() {
	if m.resolver == nil {
		return
	}

	// We merge the health states over the existing candidates instead of replacing them
	currentCandidates := m.resolver.GetCandidates()
	var updated []CandidateInput

	for _, c := range currentCandidates {
		newState := c
		if st, ok := m.providers[c.Provider]; ok {
			// Apply health monitor adjustments
			newState.Available = (st.Status != "unavailable" && !st.CoolingDown)
			newState.Latency = st.Latency
			newState.SuccessRate = st.SuccessRate
			newState.QuotaHealth = st.QuotaRemaining
			
			// Publish prometheus health metric
			observability.ProviderHealthScore.WithLabelValues(c.Provider, st.Status).Set(st.SuccessRate)
		}
		updated = append(updated, newState)
	}

	// Use updateCandidatesDirectly to avoid re-registering with the monitor,
	// which would deadlock since we already hold mu.Lock (C1 fix).
	m.resolver.updateCandidatesDirectly(updated)
}

// RegisterInitialCandidates bootstraps the health monitor with the boot-time discovery
func (m *ProviderHealthMonitor) RegisterInitialCandidates(candidates []CandidateInput) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, c := range candidates {
		m.providers[c.Provider] = &ProviderHealthState{
			LastProbe:      time.Now(),
			Status:         "healthy",
			Latency:        c.Latency,
			SuccessRate:    1.0,
			QuotaRemaining: 1.0,
		}
	}
}
