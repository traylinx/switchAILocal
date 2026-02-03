// Package heartbeat provides proactive background monitoring for provider health.
// It implements continuous health checks, quota monitoring, and auto-discovery
// to prevent failures before they impact users.
package heartbeat

import (
	"context"
	"time"
)

// ProviderStatus represents the health status of a provider.
type ProviderStatus string

const (
	// StatusHealthy indicates the provider is fully operational
	StatusHealthy ProviderStatus = "healthy"

	// StatusDegraded indicates the provider is operational but with issues
	StatusDegraded ProviderStatus = "degraded"

	// StatusUnavailable indicates the provider is not accessible
	StatusUnavailable ProviderStatus = "unavailable"
)

// HealthStatus contains comprehensive health information for a provider.
type HealthStatus struct {
	// Provider is the name of the provider being monitored
	Provider string `json:"provider"`

	// Status is the current health status
	Status ProviderStatus `json:"status"`

	// LastCheck is when this status was last updated
	LastCheck time.Time `json:"last_check"`

	// ResponseTime is the time taken for the health check
	ResponseTime time.Duration `json:"response_time"`

	// ModelsCount is the number of available models (if applicable)
	ModelsCount int `json:"models_count"`

	// QuotaUsed is the current quota usage (0.0 to 1.0)
	QuotaUsed float64 `json:"quota_used"`

	// QuotaLimit is the quota limit (requests, tokens, etc.)
	QuotaLimit float64 `json:"quota_limit"`

	// ErrorMessage contains error details if status is not healthy
	ErrorMessage string `json:"error_message,omitempty"`

	// Metadata contains provider-specific health information
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ProviderHealthChecker defines the interface for provider-specific health checkers.
type ProviderHealthChecker interface {
	// Check performs a health check and returns the current status
	Check(ctx context.Context) (*HealthStatus, error)

	// GetName returns the provider name this checker handles
	GetName() string

	// GetCheckInterval returns the preferred check interval for this provider
	GetCheckInterval() time.Duration

	// SupportsQuotaMonitoring returns true if this provider supports quota monitoring
	SupportsQuotaMonitoring() bool

	// SupportsAutoDiscovery returns true if this provider supports model auto-discovery
	SupportsAutoDiscovery() bool
}

// HeartbeatMonitor defines the interface for the heartbeat monitoring service.
type HeartbeatMonitor interface {
	// Start begins the heartbeat monitoring loop
	Start(ctx context.Context) error

	// Stop gracefully shuts down the monitor
	Stop() error

	// CheckAll performs health checks on all registered providers
	CheckAll(ctx context.Context) error

	// CheckProvider performs a health check on a specific provider
	CheckProvider(ctx context.Context, provider string) (*HealthStatus, error)

	// GetStatus retrieves the last known status for a provider
	GetStatus(provider string) (*HealthStatus, error)

	// GetAllStatuses retrieves the last known status for all providers
	GetAllStatuses() map[string]*HealthStatus

	// RegisterChecker registers a new provider health checker
	RegisterChecker(checker ProviderHealthChecker) error

	// UnregisterChecker removes a provider health checker
	UnregisterChecker(provider string) error

	// SetInterval updates the heartbeat check interval
	SetInterval(interval time.Duration)

	// GetInterval returns the current heartbeat check interval
	GetInterval() time.Duration

	// AddEventHandler registers an event handler for heartbeat events
	AddEventHandler(handler HeartbeatEventHandler)

	// RemoveEventHandler removes an event handler
	RemoveEventHandler(handler HeartbeatEventHandler)
}

// HeartbeatConfig contains configuration for the heartbeat monitor.
type HeartbeatConfig struct {
	// Enabled controls whether heartbeat monitoring is active
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Interval is the time between heartbeat cycles
	Interval time.Duration `yaml:"interval" json:"interval"`

	// Timeout is the maximum time to wait for a single provider check
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// AutoDiscovery enables automatic model discovery
	AutoDiscovery bool `yaml:"auto-discovery" json:"auto-discovery"`

	// QuotaWarningThreshold triggers warnings when quota usage exceeds this (0.0-1.0)
	QuotaWarningThreshold float64 `yaml:"quota-warning-threshold" json:"quota-warning-threshold"`

	// QuotaCriticalThreshold triggers critical alerts when quota usage exceeds this (0.0-1.0)
	QuotaCriticalThreshold float64 `yaml:"quota-critical-threshold" json:"quota-critical-threshold"`

	// MaxConcurrentChecks limits the number of simultaneous health checks
	MaxConcurrentChecks int `yaml:"max-concurrent-checks" json:"max-concurrent-checks"`

	// RetryAttempts is the number of retries for failed health checks
	RetryAttempts int `yaml:"retry-attempts" json:"retry-attempts"`

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration `yaml:"retry-delay" json:"retry-delay"`
}

// DefaultHeartbeatConfig returns the default heartbeat configuration.
func DefaultHeartbeatConfig() *HeartbeatConfig {
	return &HeartbeatConfig{
		Enabled:                true,
		Interval:               5 * time.Minute,
		Timeout:                5 * time.Second,
		AutoDiscovery:          true,
		QuotaWarningThreshold:  0.80, // 80%
		QuotaCriticalThreshold: 0.95, // 95%
		MaxConcurrentChecks:    10,
		RetryAttempts:          2,
		RetryDelay:             time.Second,
	}
}

// HeartbeatEvent represents events that can be triggered by the heartbeat monitor.
type HeartbeatEvent struct {
	// Type is the event type
	Type HeartbeatEventType `json:"type"`

	// Provider is the provider that triggered the event
	Provider string `json:"provider"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Status is the current health status
	Status *HealthStatus `json:"status,omitempty"`

	// PreviousStatus is the previous health status (for status change events)
	PreviousStatus *HealthStatus `json:"previous_status,omitempty"`

	// Data contains event-specific data
	Data map[string]interface{} `json:"data,omitempty"`
}

// HeartbeatEventType represents the type of heartbeat event.
type HeartbeatEventType string

const (
	// EventProviderHealthy indicates a provider became healthy
	EventProviderHealthy HeartbeatEventType = "provider_healthy"

	// EventProviderDegraded indicates a provider became degraded
	EventProviderDegraded HeartbeatEventType = "provider_degraded"

	// EventProviderUnavailable indicates a provider became unavailable
	EventProviderUnavailable HeartbeatEventType = "provider_unavailable"

	// EventQuotaWarning indicates quota usage exceeded warning threshold
	EventQuotaWarning HeartbeatEventType = "quota_warning"

	// EventQuotaCritical indicates quota usage exceeded critical threshold
	EventQuotaCritical HeartbeatEventType = "quota_critical"

	// EventModelDiscovered indicates new models were discovered
	EventModelDiscovered HeartbeatEventType = "model_discovered"

	// EventHealthCheckFailed indicates a health check failed
	EventHealthCheckFailed HeartbeatEventType = "health_check_failed"

	// EventHeartbeatStarted indicates the heartbeat monitor started
	EventHeartbeatStarted HeartbeatEventType = "heartbeat_started"

	// EventHeartbeatStopped indicates the heartbeat monitor stopped
	EventHeartbeatStopped HeartbeatEventType = "heartbeat_stopped"
)

// HeartbeatEventHandler defines the interface for handling heartbeat events.
type HeartbeatEventHandler interface {
	// HandleEvent processes a heartbeat event
	HandleEvent(event *HeartbeatEvent) error
}

// HeartbeatStats contains statistics about the heartbeat monitor.
type HeartbeatStats struct {
	// StartTime is when the monitor was started
	StartTime time.Time `json:"start_time"`

	// LastCycleTime is when the last full cycle completed
	LastCycleTime time.Time `json:"last_cycle_time"`

	// TotalCycles is the number of completed heartbeat cycles
	TotalCycles int64 `json:"total_cycles"`

	// TotalChecks is the total number of provider checks performed
	TotalChecks int64 `json:"total_checks"`

	// SuccessfulChecks is the number of successful checks
	SuccessfulChecks int64 `json:"successful_checks"`

	// FailedChecks is the number of failed checks
	FailedChecks int64 `json:"failed_checks"`

	// AverageCycleTime is the average time per heartbeat cycle
	AverageCycleTime time.Duration `json:"average_cycle_time"`

	// ProvidersMonitored is the number of providers being monitored
	ProvidersMonitored int `json:"providers_monitored"`

	// HealthyProviders is the number of currently healthy providers
	HealthyProviders int `json:"healthy_providers"`

	// DegradedProviders is the number of currently degraded providers
	DegradedProviders int `json:"degraded_providers"`

	// UnavailableProviders is the number of currently unavailable providers
	UnavailableProviders int `json:"unavailable_providers"`
}
