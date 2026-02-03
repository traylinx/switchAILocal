package heartbeat

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RecoveryAction represents a recovery action taken in response to a health issue.
type RecoveryAction struct {
	// Timestamp is when the action was taken
	Timestamp time.Time `json:"timestamp"`

	// Provider is the provider this action was taken for
	Provider string `json:"provider"`

	// ActionType describes the type of recovery action
	ActionType RecoveryActionType `json:"action_type"`

	// Description provides details about the action
	Description string `json:"description"`

	// Success indicates whether the action succeeded
	Success bool `json:"success"`

	// Error contains error details if the action failed
	Error string `json:"error,omitempty"`

	// Metadata contains action-specific data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// RecoveryActionType categorizes recovery actions.
type RecoveryActionType string

const (
	// ActionRestartConnection attempts to restart the provider connection
	ActionRestartConnection RecoveryActionType = "restart_connection"

	// ActionEnableFallback enables fallback routing to alternative providers
	ActionEnableFallback RecoveryActionType = "enable_fallback"

	// ActionDisableProvider temporarily disables the provider
	ActionDisableProvider RecoveryActionType = "disable_provider"

	// ActionEnableProvider re-enables a previously disabled provider
	ActionEnableProvider RecoveryActionType = "enable_provider"

	// ActionNotifyAdmin sends a notification to administrators
	ActionNotifyAdmin RecoveryActionType = "notify_admin"

	// ActionLogWarning logs a warning about the provider status
	ActionLogWarning RecoveryActionType = "log_warning"
)

// RecoveryManager handles automatic recovery actions for unhealthy providers.
type RecoveryManager struct {
	mu sync.RWMutex

	// config holds recovery configuration
	config *RecoveryConfig

	// actions tracks all recovery actions taken
	actions []RecoveryAction

	// disabledProviders tracks providers that have been disabled
	disabledProviders map[string]time.Time

	// fallbackEnabled tracks whether fallback routing is enabled per provider
	fallbackEnabled map[string]bool

	// recoveryAttempts tracks the number of recovery attempts per provider
	recoveryAttempts map[string]int

	// lastRecoveryTime tracks when the last recovery was attempted per provider
	lastRecoveryTime map[string]time.Time
}

// RecoveryConfig holds configuration for the recovery manager.
type RecoveryConfig struct {
	// Enabled controls whether automatic recovery is active
	Enabled bool

	// MaxRecoveryAttempts is the maximum number of recovery attempts per provider
	MaxRecoveryAttempts int

	// RecoveryBackoff is the minimum time between recovery attempts
	RecoveryBackoff time.Duration

	// AutoDisableThreshold is the number of consecutive failures before auto-disable
	AutoDisableThreshold int

	// AutoEnableDelay is how long to wait before re-enabling a disabled provider
	AutoEnableDelay time.Duration

	// EnableFallbackRouting controls whether fallback routing is enabled
	EnableFallbackRouting bool

	// NotifyAdminOnFailure controls whether to notify admins on provider failures
	NotifyAdminOnFailure bool
}

// DefaultRecoveryConfig returns the default recovery configuration.
func DefaultRecoveryConfig() *RecoveryConfig {
	return &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   3,
		RecoveryBackoff:       5 * time.Minute,
		AutoDisableThreshold:  5,
		AutoEnableDelay:       30 * time.Minute,
		EnableFallbackRouting: true,
		NotifyAdminOnFailure:  false,
	}
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(config *RecoveryConfig) *RecoveryManager {
	if config == nil {
		config = DefaultRecoveryConfig()
	}

	return &RecoveryManager{
		config:            config,
		actions:           make([]RecoveryAction, 0),
		disabledProviders: make(map[string]time.Time),
		fallbackEnabled:   make(map[string]bool),
		recoveryAttempts:  make(map[string]int),
		lastRecoveryTime:  make(map[string]time.Time),
	}
}

// HandleProviderUnavailable handles a provider becoming unavailable.
func (rm *RecoveryManager) HandleProviderUnavailable(ctx context.Context, provider string, status *HealthStatus) error {
	if !rm.config.Enabled {
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check if we should attempt recovery
	if !rm.shouldAttemptRecovery(provider) {
		return fmt.Errorf("recovery backoff in effect for provider %s", provider)
	}

	// Increment recovery attempts
	rm.recoveryAttempts[provider]++
	rm.lastRecoveryTime[provider] = time.Now()

	// Log warning
	rm.recordAction(RecoveryAction{
		Timestamp:   time.Now(),
		Provider:    provider,
		ActionType:  ActionLogWarning,
		Description: fmt.Sprintf("Provider %s is unavailable: %s", provider, status.ErrorMessage),
		Success:     true,
		Metadata: map[string]interface{}{
			"status":        string(status.Status),
			"response_time": status.ResponseTime.String(),
			"error":         status.ErrorMessage,
		},
	})

	// Enable fallback routing if configured
	if rm.config.EnableFallbackRouting {
		rm.fallbackEnabled[provider] = true
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionEnableFallback,
			Description: fmt.Sprintf("Enabled fallback routing for provider %s", provider),
			Success:     true,
		})
	}

	// Check if we should auto-disable the provider
	if rm.recoveryAttempts[provider] >= rm.config.AutoDisableThreshold {
		rm.disabledProviders[provider] = time.Now()
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionDisableProvider,
			Description: fmt.Sprintf("Auto-disabled provider %s after %d consecutive failures", provider, rm.recoveryAttempts[provider]),
			Success:     true,
			Metadata: map[string]interface{}{
				"failure_count": rm.recoveryAttempts[provider],
				"threshold":     rm.config.AutoDisableThreshold,
			},
		})
	}

	// Notify admin if configured
	if rm.config.NotifyAdminOnFailure {
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionNotifyAdmin,
			Description: fmt.Sprintf("Notified admin about provider %s failure", provider),
			Success:     true,
		})
	}

	return nil
}

// HandleProviderHealthy handles a provider becoming healthy.
func (rm *RecoveryManager) HandleProviderHealthy(ctx context.Context, provider string, status *HealthStatus) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Reset recovery attempts
	previousAttempts := rm.recoveryAttempts[provider]
	rm.recoveryAttempts[provider] = 0

	// Disable fallback routing
	if rm.fallbackEnabled[provider] {
		rm.fallbackEnabled[provider] = false
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionEnableFallback,
			Description: fmt.Sprintf("Disabled fallback routing for provider %s (now healthy)", provider),
			Success:     true,
		})
	}

	// Re-enable provider if it was disabled
	if _, disabled := rm.disabledProviders[provider]; disabled {
		delete(rm.disabledProviders, provider)
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionEnableProvider,
			Description: fmt.Sprintf("Re-enabled provider %s (recovered after %d failures)", provider, previousAttempts),
			Success:     true,
			Metadata: map[string]interface{}{
				"previous_failures": previousAttempts,
			},
		})
	}

	return nil
}

// HandleProviderDegraded handles a provider becoming degraded.
func (rm *RecoveryManager) HandleProviderDegraded(ctx context.Context, provider string, status *HealthStatus) error {
	if !rm.config.Enabled {
		return nil
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Log warning
	rm.recordAction(RecoveryAction{
		Timestamp:   time.Now(),
		Provider:    provider,
		ActionType:  ActionLogWarning,
		Description: fmt.Sprintf("Provider %s is degraded: %s", provider, status.ErrorMessage),
		Success:     true,
		Metadata: map[string]interface{}{
			"status":        string(status.Status),
			"response_time": status.ResponseTime.String(),
			"error":         status.ErrorMessage,
		},
	})

	// Enable fallback routing if configured (degraded providers should have fallback available)
	if rm.config.EnableFallbackRouting && !rm.fallbackEnabled[provider] {
		rm.fallbackEnabled[provider] = true
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionEnableFallback,
			Description: fmt.Sprintf("Enabled fallback routing for degraded provider %s", provider),
			Success:     true,
		})
	}

	return nil
}

// shouldAttemptRecovery checks if recovery should be attempted for a provider.
func (rm *RecoveryManager) shouldAttemptRecovery(provider string) bool {
	// Check if provider is disabled
	if disabledTime, disabled := rm.disabledProviders[provider]; disabled {
		// Check if enough time has passed to re-enable
		if time.Since(disabledTime) < rm.config.AutoEnableDelay {
			return false
		}
		// Auto-enable after delay
		delete(rm.disabledProviders, provider)
		rm.recordAction(RecoveryAction{
			Timestamp:   time.Now(),
			Provider:    provider,
			ActionType:  ActionEnableProvider,
			Description: fmt.Sprintf("Auto-enabled provider %s after %v delay", provider, rm.config.AutoEnableDelay),
			Success:     true,
		})
	}

	// Check recovery backoff
	if lastRecovery, exists := rm.lastRecoveryTime[provider]; exists {
		if time.Since(lastRecovery) < rm.config.RecoveryBackoff {
			return false
		}
	}

	// Check max recovery attempts
	if rm.recoveryAttempts[provider] >= rm.config.MaxRecoveryAttempts {
		return false
	}

	return true
}

// recordAction records a recovery action.
func (rm *RecoveryManager) recordAction(action RecoveryAction) {
	rm.actions = append(rm.actions, action)

	// Keep only last 1000 actions to prevent unbounded growth
	if len(rm.actions) > 1000 {
		rm.actions = rm.actions[len(rm.actions)-1000:]
	}
}

// GetActions returns all recorded recovery actions.
func (rm *RecoveryManager) GetActions() []RecoveryAction {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	actions := make([]RecoveryAction, len(rm.actions))
	copy(actions, rm.actions)
	return actions
}

// GetActionsForProvider returns recovery actions for a specific provider.
func (rm *RecoveryManager) GetActionsForProvider(provider string) []RecoveryAction {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	actions := make([]RecoveryAction, 0)
	for _, action := range rm.actions {
		if action.Provider == provider {
			actions = append(actions, action)
		}
	}
	return actions
}

// IsProviderDisabled returns whether a provider is currently disabled.
func (rm *RecoveryManager) IsProviderDisabled(provider string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	_, disabled := rm.disabledProviders[provider]
	return disabled
}

// IsFallbackEnabled returns whether fallback routing is enabled for a provider.
func (rm *RecoveryManager) IsFallbackEnabled(provider string) bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.fallbackEnabled[provider]
}

// GetRecoveryAttempts returns the number of recovery attempts for a provider.
func (rm *RecoveryManager) GetRecoveryAttempts(provider string) int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return rm.recoveryAttempts[provider]
}

// ResetRecoveryAttempts resets the recovery attempt counter for a provider.
func (rm *RecoveryManager) ResetRecoveryAttempts(provider string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.recoveryAttempts[provider] = 0
}

// GetStats returns recovery statistics.
func (rm *RecoveryManager) GetStats() *RecoveryStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := &RecoveryStats{
		TotalActions:      len(rm.actions),
		DisabledProviders: len(rm.disabledProviders),
		ActionsByType:     make(map[RecoveryActionType]int),
		ActionsByProvider: make(map[string]int),
	}

	for _, action := range rm.actions {
		stats.ActionsByType[action.ActionType]++
		stats.ActionsByProvider[action.Provider]++
		if action.Success {
			stats.SuccessfulActions++
		} else {
			stats.FailedActions++
		}
	}

	return stats
}

// RecoveryStats contains statistics about recovery actions.
type RecoveryStats struct {
	// TotalActions is the total number of recovery actions taken
	TotalActions int `json:"total_actions"`

	// SuccessfulActions is the number of successful actions
	SuccessfulActions int `json:"successful_actions"`

	// FailedActions is the number of failed actions
	FailedActions int `json:"failed_actions"`

	// DisabledProviders is the number of currently disabled providers
	DisabledProviders int `json:"disabled_providers"`

	// ActionsByType breaks down actions by type
	ActionsByType map[RecoveryActionType]int `json:"actions_by_type"`

	// ActionsByProvider breaks down actions by provider
	ActionsByProvider map[string]int `json:"actions_by_provider"`
}
