package heartbeat

import (
	"context"
	"testing"
	"time"
)

func TestRecoveryManager_HandleProviderUnavailable(t *testing.T) {
	config := &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   3,
		RecoveryBackoff:       time.Second,
		AutoDisableThreshold:  3,
		AutoEnableDelay:       5 * time.Second,
		EnableFallbackRouting: true,
		NotifyAdminOnFailure:  false,
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// First unavailable event
	err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("HandleProviderUnavailable failed: %v", err)
	}

	// Check fallback is enabled
	if !rm.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled after unavailable event")
	}

	// Check recovery attempts
	if rm.GetRecoveryAttempts("test-provider") != 1 {
		t.Errorf("Expected 1 recovery attempt, got %d", rm.GetRecoveryAttempts("test-provider"))
	}

	// Check actions were recorded
	actions := rm.GetActionsForProvider("test-provider")
	if len(actions) < 2 {
		t.Errorf("Expected at least 2 actions, got %d", len(actions))
	}

	// Provider should not be disabled yet (threshold is 3)
	if rm.IsProviderDisabled("test-provider") {
		t.Error("Provider should not be disabled after 1 failure")
	}
}

func TestRecoveryManager_AutoDisable(t *testing.T) {
	config := &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   10,
		RecoveryBackoff:       time.Millisecond, // Very short for testing
		AutoDisableThreshold:  3,
		AutoEnableDelay:       time.Hour,
		EnableFallbackRouting: true,
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// Trigger failures until auto-disable
	for i := 0; i < 3; i++ {
		time.Sleep(2 * time.Millisecond) // Wait for backoff
		err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
		if err != nil {
			t.Fatalf("HandleProviderUnavailable failed: %v", err)
		}
	}

	// Provider should now be disabled
	if !rm.IsProviderDisabled("test-provider") {
		t.Error("Provider should be disabled after reaching threshold")
	}

	// Check actions include disable action
	actions := rm.GetActionsForProvider("test-provider")
	foundDisable := false
	for _, action := range actions {
		if action.ActionType == ActionDisableProvider {
			foundDisable = true
			break
		}
	}
	if !foundDisable {
		t.Error("Expected disable action to be recorded")
	}
}

func TestRecoveryManager_HandleProviderHealthy(t *testing.T) {
	config := DefaultRecoveryConfig()
	rm := NewRecoveryManager(config)
	ctx := context.Background()

	// Simulate some failures first
	unavailableStatus := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	_ = rm.HandleProviderUnavailable(ctx, "test-provider", unavailableStatus)

	// Check fallback is enabled
	if !rm.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled after unavailable event")
	}

	// Now provider becomes healthy
	healthyStatus := &HealthStatus{
		Provider: "test-provider",
		Status:   StatusHealthy,
	}

	err := rm.HandleProviderHealthy(ctx, "test-provider", healthyStatus)
	if err != nil {
		t.Fatalf("HandleProviderHealthy failed: %v", err)
	}

	// Check fallback is disabled
	if rm.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be disabled after provider becomes healthy")
	}

	// Check recovery attempts are reset
	if rm.GetRecoveryAttempts("test-provider") != 0 {
		t.Errorf("Expected 0 recovery attempts after healthy, got %d", rm.GetRecoveryAttempts("test-provider"))
	}
}

func TestRecoveryManager_HandleProviderDegraded(t *testing.T) {
	config := DefaultRecoveryConfig()
	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusDegraded,
		ErrorMessage: "slow response",
	}

	err := rm.HandleProviderDegraded(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("HandleProviderDegraded failed: %v", err)
	}

	// Check fallback is enabled for degraded provider
	if !rm.IsFallbackEnabled("test-provider") {
		t.Error("Fallback should be enabled for degraded provider")
	}

	// Check actions were recorded
	actions := rm.GetActionsForProvider("test-provider")
	if len(actions) < 2 {
		t.Errorf("Expected at least 2 actions, got %d", len(actions))
	}
}

func TestRecoveryManager_RecoveryBackoff(t *testing.T) {
	config := &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   10,
		RecoveryBackoff:       100 * time.Millisecond,
		AutoDisableThreshold:  10,
		AutoEnableDelay:       time.Hour,
		EnableFallbackRouting: true,
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// First attempt should succeed
	err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("First HandleProviderUnavailable failed: %v", err)
	}

	// Second attempt immediately should fail due to backoff
	err = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err == nil {
		t.Error("Expected error due to recovery backoff")
	}

	// Wait for backoff period
	time.Sleep(150 * time.Millisecond)

	// Third attempt should succeed
	err = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("Third HandleProviderUnavailable failed: %v", err)
	}
}

func TestRecoveryManager_MaxRecoveryAttempts(t *testing.T) {
	config := &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   2,
		RecoveryBackoff:       time.Millisecond,
		AutoDisableThreshold:  10,
		AutoEnableDelay:       time.Hour,
		EnableFallbackRouting: true,
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// Attempt 1
	err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("Attempt 1 failed: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// Attempt 2
	err = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("Attempt 2 failed: %v", err)
	}

	time.Sleep(2 * time.Millisecond)

	// Attempt 3 should fail (max attempts reached)
	err = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err == nil {
		t.Error("Expected error due to max recovery attempts")
	}
}

func TestRecoveryManager_GetStats(t *testing.T) {
	config := DefaultRecoveryConfig()
	rm := NewRecoveryManager(config)
	ctx := context.Background()

	// Trigger some events
	unavailableStatus := &HealthStatus{
		Provider:     "provider1",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	_ = rm.HandleProviderUnavailable(ctx, "provider1", unavailableStatus)

	degradedStatus := &HealthStatus{
		Provider:     "provider2",
		Status:       StatusDegraded,
		ErrorMessage: "slow response",
	}

	_ = rm.HandleProviderDegraded(ctx, "provider2", degradedStatus)

	// Get stats
	stats := rm.GetStats()

	if stats.TotalActions == 0 {
		t.Error("Expected some actions to be recorded")
	}

	if stats.SuccessfulActions == 0 {
		t.Error("Expected some successful actions")
	}

	if len(stats.ActionsByType) == 0 {
		t.Error("Expected actions by type to be populated")
	}

	if len(stats.ActionsByProvider) != 2 {
		t.Errorf("Expected 2 providers in stats, got %d", len(stats.ActionsByProvider))
	}
}

func TestRecoveryManager_Disabled(t *testing.T) {
	config := &RecoveryConfig{
		Enabled: false, // Disabled
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// Should not trigger any actions when disabled
	err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("HandleProviderUnavailable should not error when disabled: %v", err)
	}

	// No actions should be recorded
	actions := rm.GetActions()
	if len(actions) != 0 {
		t.Errorf("Expected 0 actions when disabled, got %d", len(actions))
	}
}

func TestRecoveryManager_AutoEnable(t *testing.T) {
	config := &RecoveryConfig{
		Enabled:               true,
		MaxRecoveryAttempts:   10,
		RecoveryBackoff:       time.Millisecond,
		AutoDisableThreshold:  2,
		AutoEnableDelay:       50 * time.Millisecond,
		EnableFallbackRouting: true,
	}

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	// Trigger auto-disable
	for i := 0; i < 2; i++ {
		time.Sleep(2 * time.Millisecond)
		_ = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	}

	// Provider should be disabled
	if !rm.IsProviderDisabled("test-provider") {
		t.Error("Provider should be disabled")
	}

	// Wait for auto-enable delay
	time.Sleep(60 * time.Millisecond)

	// Try recovery again - should auto-enable
	time.Sleep(2 * time.Millisecond)
	err := rm.HandleProviderUnavailable(ctx, "test-provider", status)
	if err != nil {
		t.Fatalf("HandleProviderUnavailable failed after auto-enable: %v", err)
	}

	// Check that auto-enable action was recorded
	actions := rm.GetActionsForProvider("test-provider")
	foundAutoEnable := false
	for _, action := range actions {
		if action.ActionType == ActionEnableProvider && action.Description != "" {
			foundAutoEnable = true
			break
		}
	}
	if !foundAutoEnable {
		t.Error("Expected auto-enable action to be recorded")
	}
}

func BenchmarkRecoveryManager_HandleProviderUnavailable(b *testing.B) {
	config := DefaultRecoveryConfig()
	config.RecoveryBackoff = 0 // Disable backoff for benchmarking
	config.MaxRecoveryAttempts = 1000000

	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider:     "test-provider",
		Status:       StatusUnavailable,
		ErrorMessage: "connection refused",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rm.HandleProviderUnavailable(ctx, "test-provider", status)
	}
}

func BenchmarkRecoveryManager_HandleProviderHealthy(b *testing.B) {
	config := DefaultRecoveryConfig()
	rm := NewRecoveryManager(config)
	ctx := context.Background()

	status := &HealthStatus{
		Provider: "test-provider",
		Status:   StatusHealthy,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rm.HandleProviderHealthy(ctx, "test-provider", status)
	}
}
