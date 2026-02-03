package heartbeat

import (
	"context"
	"testing"
	"time"
)

// MockHealthChecker implements ProviderHealthChecker for testing.
type MockHealthChecker struct {
	name                  string
	status                ProviderStatus
	responseTime          time.Duration
	err                   error
	checkInterval         time.Duration
	supportsQuota         bool
	supportsAutoDiscovery bool
	checkCount            int
}

func NewMockHealthChecker(name string) *MockHealthChecker {
	return &MockHealthChecker{
		name:                  name,
		status:                StatusHealthy,
		responseTime:          100 * time.Millisecond,
		checkInterval:         5 * time.Minute,
		supportsQuota:         false,
		supportsAutoDiscovery: false,
	}
}

func (m *MockHealthChecker) Check(ctx context.Context) (*HealthStatus, error) {
	m.checkCount++

	if m.err != nil {
		return nil, m.err
	}

	// Simulate response time
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(m.responseTime):
	}

	return &HealthStatus{
		Provider:     m.name,
		Status:       m.status,
		LastCheck:    time.Now(),
		ResponseTime: m.responseTime,
		ModelsCount:  5,
		QuotaUsed:    0.5,
		QuotaLimit:   1000,
	}, nil
}

func (m *MockHealthChecker) GetName() string {
	return m.name
}

func (m *MockHealthChecker) GetCheckInterval() time.Duration {
	return m.checkInterval
}

func (m *MockHealthChecker) SupportsQuotaMonitoring() bool {
	return m.supportsQuota
}

func (m *MockHealthChecker) SupportsAutoDiscovery() bool {
	return m.supportsAutoDiscovery
}

func (m *MockHealthChecker) SetStatus(status ProviderStatus) {
	m.status = status
}

func (m *MockHealthChecker) SetError(err error) {
	m.err = err
}

func (m *MockHealthChecker) GetCheckCount() int {
	return m.checkCount
}

// TestHeartbeatMonitorBasic tests basic heartbeat monitor functionality.
func TestHeartbeatMonitorBasic(t *testing.T) {
	config := &HeartbeatConfig{
		Enabled:                true,
		Interval:               100 * time.Millisecond, // Fast for testing
		Timeout:                time.Second,
		AutoDiscovery:          true,
		QuotaWarningThreshold:  0.80,
		QuotaCriticalThreshold: 0.95,
		MaxConcurrentChecks:    5,
		RetryAttempts:          1,
		RetryDelay:             50 * time.Millisecond,
	}

	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	// Test initial state
	if monitor.IsRunning() {
		t.Error("Monitor should not be running initially")
	}

	// Test registering checkers
	checker1 := NewMockHealthChecker("provider1")
	checker2 := NewMockHealthChecker("provider2")

	if err := monitor.RegisterChecker(checker1); err != nil {
		t.Fatalf("Failed to register checker1: %v", err)
	}

	if err := monitor.RegisterChecker(checker2); err != nil {
		t.Fatalf("Failed to register checker2: %v", err)
	}

	// Test getting status before any checks
	status, err := monitor.GetStatus("provider1")
	if err == nil {
		t.Error("Should return error when no status available")
	}
	if status != nil {
		t.Error("Should not have status before any checks")
	}

	// Test manual check
	ctx := context.Background()
	status, err = monitor.CheckProvider(ctx, "provider1")
	if err != nil {
		t.Fatalf("Manual check failed: %v", err)
	}

	if status.Provider != "provider1" {
		t.Errorf("Expected provider1, got %s", status.Provider)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected healthy status, got %s", status.Status)
	}

	// Test getting status after check
	status2, err := monitor.GetStatus("provider1")
	if err != nil {
		t.Errorf("Should have status after check: %v", err)
	}
	if status2 == nil {
		t.Error("Should have status after check")
	}

	// Test getting all statuses
	allStatuses := monitor.GetAllStatuses()
	if len(allStatuses) != 1 {
		t.Errorf("Expected 1 status, got %d", len(allStatuses))
	}

	// Test unregistering checker
	if err := monitor.UnregisterChecker("provider2"); err != nil {
		t.Fatalf("Failed to unregister checker: %v", err)
	}

	// Test checking non-existent provider
	_, err = monitor.CheckProvider(ctx, "nonexistent")
	if err == nil {
		t.Error("Should fail when checking non-existent provider")
	}
}

// TestHeartbeatMonitorStartStop tests starting and stopping the monitor.
func TestHeartbeatMonitorStartStop(t *testing.T) {
	config := &HeartbeatConfig{
		Enabled:                true,
		Interval:               50 * time.Millisecond, // Very fast for testing
		Timeout:                time.Second,
		AutoDiscovery:          true,
		QuotaWarningThreshold:  0.80,
		QuotaCriticalThreshold: 0.95,
		MaxConcurrentChecks:    5,
		RetryAttempts:          1,
		RetryDelay:             10 * time.Millisecond,
	}

	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	// Register a checker
	checker := NewMockHealthChecker("test-provider")
	if err := monitor.RegisterChecker(checker); err != nil {
		t.Fatalf("Failed to register checker: %v", err)
	}

	// Start monitor
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	if !monitor.IsRunning() {
		t.Error("Monitor should be running after start")
	}

	// Wait for a few cycles
	time.Sleep(200 * time.Millisecond)

	// Check that health checks were performed
	if checker.GetCheckCount() == 0 {
		t.Error("No health checks were performed")
	}

	// Check statistics
	stats := monitor.GetStats()
	if stats.TotalCycles == 0 {
		t.Error("No cycles recorded in statistics")
	}

	if stats.ProvidersMonitored != 1 {
		t.Errorf("Expected 1 provider monitored, got %d", stats.ProvidersMonitored)
	}

	// Stop monitor
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	if monitor.IsRunning() {
		t.Error("Monitor should not be running after stop")
	}

	// Test double start (should fail)
	monitor2 := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)
	if err := monitor2.RegisterChecker(checker); err != nil {
		t.Fatalf("Failed to register checker: %v", err)
	}

	if err := monitor2.Start(ctx); err != nil {
		t.Fatalf("Failed to start monitor2: %v", err)
	}

	if err := monitor2.Start(ctx); err == nil {
		t.Error("Should fail to start monitor twice")
	}

	_ = monitor2.Stop()
}

// TestHeartbeatMonitorDisabled tests behavior when monitoring is disabled.
func TestHeartbeatMonitorDisabled(t *testing.T) {
	config := &HeartbeatConfig{
		Enabled: false, // Disabled
	}

	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	ctx := context.Background()

	// Should fail to start when disabled
	if err := monitor.Start(ctx); err == nil {
		t.Error("Should fail to start when disabled")
	}
}

// TestHeartbeatMonitorRetry tests retry logic for failed health checks.
func TestHeartbeatMonitorRetry(t *testing.T) {
	config := &HeartbeatConfig{
		Enabled:                true,
		Interval:               time.Minute, // Long interval to avoid automatic checks
		Timeout:                time.Second,
		AutoDiscovery:          true,
		QuotaWarningThreshold:  0.80,
		QuotaCriticalThreshold: 0.95,
		MaxConcurrentChecks:    5,
		RetryAttempts:          2, // 2 retries
		RetryDelay:             10 * time.Millisecond,
	}

	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	// Create a checker that fails initially
	checker := NewMockHealthChecker("failing-provider")
	checker.SetError(context.DeadlineExceeded) // Simulate timeout

	if err := monitor.RegisterChecker(checker); err != nil {
		t.Fatalf("Failed to register checker: %v", err)
	}

	ctx := context.Background()

	// Manual check should retry and eventually mark as unavailable
	_, err := monitor.CheckProvider(ctx, "failing-provider")
	if err == nil {
		t.Error("Expected error from failing provider")
	}

	// Should have attempted multiple times (1 initial + 2 retries = 3 total)
	if checker.GetCheckCount() != 3 {
		t.Errorf("Expected 3 check attempts, got %d", checker.GetCheckCount())
	}

	// Status should be unavailable
	status, err := monitor.GetStatus("failing-provider")
	if err != nil {
		t.Fatalf("Should have status after failed check: %v", err)
	}
	if status == nil {
		t.Fatal("Should have status after failed check")
	}

	if status.Status != StatusUnavailable {
		t.Errorf("Expected unavailable status, got %s", status.Status)
	}

	if status.ErrorMessage == "" {
		t.Error("Should have error message for failed check")
	}
}

// TestHeartbeatMonitorInterval tests interval configuration.
func TestHeartbeatMonitorInterval(t *testing.T) {
	config := DefaultHeartbeatConfig()
	monitor := NewHeartbeatMonitor(config).(*HeartbeatMonitorImpl)

	// Test default interval
	if monitor.GetInterval() != 5*time.Minute {
		t.Errorf("Expected 5m default interval, got %v", monitor.GetInterval())
	}

	// Test setting new interval
	newInterval := 2 * time.Minute
	monitor.SetInterval(newInterval)

	if monitor.GetInterval() != newInterval {
		t.Errorf("Expected %v interval, got %v", newInterval, monitor.GetInterval())
	}
}

// TestHeartbeatMonitorConfig tests configuration validation.
func TestHeartbeatMonitorConfig(t *testing.T) {
	// Test default config
	defaultConfig := DefaultHeartbeatConfig()

	if !defaultConfig.Enabled {
		t.Error("Default config should be enabled")
	}

	if defaultConfig.Interval != 5*time.Minute {
		t.Errorf("Expected 5m default interval, got %v", defaultConfig.Interval)
	}

	if defaultConfig.Timeout != 5*time.Second {
		t.Errorf("Expected 5s default timeout, got %v", defaultConfig.Timeout)
	}

	if defaultConfig.QuotaWarningThreshold != 0.80 {
		t.Errorf("Expected 0.80 warning threshold, got %f", defaultConfig.QuotaWarningThreshold)
	}

	if defaultConfig.QuotaCriticalThreshold != 0.95 {
		t.Errorf("Expected 0.95 critical threshold, got %f", defaultConfig.QuotaCriticalThreshold)
	}

	if defaultConfig.MaxConcurrentChecks != 10 {
		t.Errorf("Expected 10 max concurrent checks, got %d", defaultConfig.MaxConcurrentChecks)
	}

	if defaultConfig.RetryAttempts != 2 {
		t.Errorf("Expected 2 retry attempts, got %d", defaultConfig.RetryAttempts)
	}

	if defaultConfig.RetryDelay != time.Second {
		t.Errorf("Expected 1s retry delay, got %v", defaultConfig.RetryDelay)
	}
}

// TestHeartbeatMonitorNilChecker tests handling of nil checker.
func TestHeartbeatMonitorNilChecker(t *testing.T) {
	monitor := NewHeartbeatMonitor(nil) // nil config should use defaults

	// Should fail to register nil checker
	if err := monitor.RegisterChecker(nil); err == nil {
		t.Error("Should fail to register nil checker")
	}
}

// TestHeartbeatMonitorEmptyName tests handling of checker with empty name.
func TestHeartbeatMonitorEmptyName(t *testing.T) {
	monitor := NewHeartbeatMonitor(nil)

	checker := NewMockHealthChecker("") // Empty name

	// Should fail to register checker with empty name
	if err := monitor.RegisterChecker(checker); err == nil {
		t.Error("Should fail to register checker with empty name")
	}
}
