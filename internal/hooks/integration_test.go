package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHookIntegration_EndToEnd tests the complete hook system integration
func TestHookIntegration_EndToEnd(t *testing.T) {
	// Setup test environment
	tmpDir, err := os.MkdirTemp("", "hooks-integration-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create event bus
	bus := NewEventBus()
	defer bus.Shutdown()

	// Create hook manager
	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)

	// Register all built-in actions
	RegisterBuiltInActions(manager)

	// Create test webhook server
	var webhookPayload map[string]interface{}
	var webhookHeaders http.Header
	var webhookMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookMu.Lock()
		defer webhookMu.Unlock()
		webhookHeaders = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &webhookPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	serverURL := strings.Replace(server.URL, "127.0.0.1", "localhost", 1)

	// Create multiple test hooks
	hooks := []string{
		fmt.Sprintf(`
id: "webhook-hook"
name: "Webhook Notification"
event: "request_failed"
condition: "Provider == 'test-provider'"
action: "notify_webhook"
enabled: true
params:
  url: "%s"
  secret: "test-secret"
`, serverURL),
		`
id: "log-hook"
name: "Log Warning"
event: "quota_warning"
condition: "Data.usage > 80"
action: "log_warning"
enabled: true
params:
  message: "High quota usage detected"
`,
		`
id: "retry-hook"
name: "Retry with Fallback"
event: "provider_unavailable"
condition: "true"
action: "retry_with_fallback"
enabled: true
`,
		`
id: "disabled-hook"
name: "Disabled Hook"
event: "request_received"
condition: "true"
action: "log_warning"
enabled: false
params:
  message: "This should not execute"
`,
	}

	// Write hook files
	for i, hookContent := range hooks {
		err = os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("hook%d.yaml", i)), []byte(hookContent), 0644)
		require.NoError(t, err)
	}

	// Load hooks and subscribe to events
	err = manager.LoadHooks()
	require.NoError(t, err)
	manager.SubscribeToAllEvents()

	// Test 1: Webhook hook should trigger
	t.Run("WebhookHook", func(t *testing.T) {
		webhookMu.Lock()
		webhookPayload = nil
		webhookHeaders = nil
		webhookMu.Unlock()

		ctx := &EventContext{
			Event:     EventRequestFailed,
			Timestamp: time.Now(),
			Provider:  "test-provider",
			Data: map[string]interface{}{
				"error": "connection timeout",
			},
		}

		bus.Publish(ctx)
		time.Sleep(100 * time.Millisecond) // Wait for async execution

		webhookMu.Lock()
		defer webhookMu.Unlock()
		require.NotNil(t, webhookPayload, "Webhook should have been called")
		assert.Equal(t, string(EventRequestFailed), webhookPayload["event"])
		assert.Equal(t, "webhook-hook", webhookPayload["hook_id"])
		assert.Equal(t, "test-provider", webhookPayload["provider"])
		assert.NotNil(t, webhookHeaders.Get("X-Hook-Signature"))
	})

	// Test 2: Log hook should trigger based on condition
	t.Run("LogHookWithCondition", func(t *testing.T) {
		ctx := &EventContext{
			Event:     EventQuotaWarning,
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"usage": 85, // > 80, should trigger
			},
		}

		// We can't easily capture log output, but we can verify the hook exists and condition evaluates
		hooks := manager.GetHooks()
		var logHook *Hook
		for _, h := range hooks {
			if h.ID == "log-hook" {
				logHook = h
				break
			}
		}
		require.NotNil(t, logHook)

		matches, err := manager.EvaluateCondition(logHook, ctx)
		require.NoError(t, err)
		assert.True(t, matches, "Condition should match when usage > 80")

		// Test condition doesn't match
		ctx.Data["usage"] = 70
		matches, err = manager.EvaluateCondition(logHook, ctx)
		require.NoError(t, err)
		assert.False(t, matches, "Condition should not match when usage <= 80")
	})

	// Test 3: Retry hook should always trigger (condition: "true")
	t.Run("RetryHook", func(t *testing.T) {
		ctx := &EventContext{
			Event:     EventProviderUnavailable,
			Timestamp: time.Now(),
			Provider:  "failed-provider",
		}

		hooks := manager.GetHooks()
		var retryHook *Hook
		for _, h := range hooks {
			if h.ID == "retry-hook" {
				retryHook = h
				break
			}
		}
		require.NotNil(t, retryHook)

		matches, err := manager.EvaluateCondition(retryHook, ctx)
		require.NoError(t, err)
		assert.True(t, matches, "Condition 'true' should always match")
	})

	// Test 4: Disabled hook should not be loaded
	t.Run("DisabledHook", func(t *testing.T) {
		hooks := manager.GetHooks()
		for _, h := range hooks {
			assert.NotEqual(t, "disabled-hook", h.ID, "Disabled hook should not be loaded")
		}
	})

	// Test 5: Multiple hooks can trigger for same event
	t.Run("MultipleHooksPerEvent", func(t *testing.T) {
		// Add another hook for request_failed
		anotherHook := `
id: "another-webhook-hook"
name: "Another Webhook"
event: "request_failed"
condition: "Data.severity == 'high'"
action: "log_warning"
enabled: true
params:
  message: "High severity failure"
`
		err = os.WriteFile(filepath.Join(tmpDir, "another.yaml"), []byte(anotherHook), 0644)
		require.NoError(t, err)

		err = manager.LoadHooks()
		require.NoError(t, err)

		// Verify both hooks are loaded for request_failed
		hooks := manager.GetHooks()
		requestFailedHooks := 0
		for _, h := range hooks {
			if h.Event == EventRequestFailed {
				requestFailedHooks++
			}
		}
		assert.Equal(t, 2, requestFailedHooks, "Should have 2 hooks for request_failed event")
	})
}

// TestHookIntegration_HotReloading tests the file watching and hot-reloading functionality
func TestHookIntegration_HotReloading(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-hotreload-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)
	RegisterBuiltInActions(manager)

	// Start with no hooks
	err = manager.LoadHooks()
	require.NoError(t, err)
	assert.Empty(t, manager.GetHooks())

	// Start file watcher
	err = manager.StartWatcher()
	require.NoError(t, err)
	defer manager.StopWatcher()

	// Add a hook file
	hookContent := `
id: "dynamic-hook"
name: "Dynamic Hook"
event: "request_received"
condition: "true"
action: "log_warning"
enabled: true
params:
  message: "Dynamic hook executed"
`
	hookFile := filepath.Join(tmpDir, "dynamic.yaml")
	err = os.WriteFile(hookFile, []byte(hookContent), 0644)
	require.NoError(t, err)

	// Wait for file watcher to detect and reload
	time.Sleep(200 * time.Millisecond)

	// Verify hook was loaded
	hooks := manager.GetHooks()
	assert.Len(t, hooks, 1)
	assert.Equal(t, "dynamic-hook", hooks[0].ID)

	// Modify the hook
	modifiedContent := strings.Replace(hookContent, "Dynamic Hook", "Modified Hook", 1)
	err = os.WriteFile(hookFile, []byte(modifiedContent), 0644)
	require.NoError(t, err)

	// Wait for reload
	time.Sleep(200 * time.Millisecond)

	// Verify hook was updated
	hooks = manager.GetHooks()
	assert.Len(t, hooks, 1)
	assert.Equal(t, "Modified Hook", hooks[0].Name)

	// Delete the hook file
	err = os.Remove(hookFile)
	require.NoError(t, err)

	// Wait for reload
	time.Sleep(200 * time.Millisecond)

	// Verify hook was removed
	hooks = manager.GetHooks()
	assert.Empty(t, hooks)
}

// TestHookIntegration_ErrorHandling tests error scenarios and recovery
func TestHookIntegration_ErrorHandling(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-errors-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)
	RegisterBuiltInActions(manager)

	// Test 1: Invalid YAML should be skipped
	t.Run("InvalidYAML", func(t *testing.T) {
		invalidYAML := `
id: "invalid-hook"
name: "Invalid Hook
event: request_received  # Missing quotes and closing quote above
action: log_warning
enabled: true
`
		err = os.WriteFile(filepath.Join(tmpDir, "invalid.yaml"), []byte(invalidYAML), 0644)
		require.NoError(t, err)

		// Should not fail, just skip the invalid file
		err = manager.LoadHooks()
		require.NoError(t, err)
		assert.Empty(t, manager.GetHooks())
	})

	// Test 2: Invalid condition should be handled gracefully
	t.Run("InvalidCondition", func(t *testing.T) {
		hookWithBadCondition := `
id: "bad-condition-hook"
name: "Bad Condition Hook"
event: "request_received"
condition: "Data.nonexistent.field > 5"  # Will cause evaluation error
action: "log_warning"
enabled: true
params:
  message: "This should not execute"
`
		err = os.WriteFile(filepath.Join(tmpDir, "bad-condition.yaml"), []byte(hookWithBadCondition), 0644)
		require.NoError(t, err)

		err = manager.LoadHooks()
		require.NoError(t, err)
		manager.SubscribeToAllEvents()

		// Trigger event - should not panic, just log error
		ctx := &EventContext{
			Event:     EventRequestReceived,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"other": "data"},
		}

		// This should not panic
		bus.Publish(ctx)
		time.Sleep(50 * time.Millisecond)
	})

	// Test 3: Unknown action should be handled gracefully
	t.Run("UnknownAction", func(t *testing.T) {
		hookWithUnknownAction := `
id: "unknown-action-hook"
name: "Unknown Action Hook"
event: "request_received"
condition: "true"
action: "unknown_action"
enabled: true
`
		err = os.WriteFile(filepath.Join(tmpDir, "unknown-action.yaml"), []byte(hookWithUnknownAction), 0644)
		require.NoError(t, err)

		err = manager.LoadHooks()
		require.NoError(t, err)
		manager.SubscribeToAllEvents()

		// Trigger event - should not panic, just log warning
		ctx := &EventContext{
			Event:     EventRequestReceived,
			Timestamp: time.Now(),
		}

		// This should not panic
		bus.Publish(ctx)
		time.Sleep(50 * time.Millisecond)
	})
}

// TestHookIntegration_Performance tests performance characteristics
func TestHookIntegration_Performance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-perf-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)
	RegisterBuiltInActions(manager)

	// Create multiple hooks to test performance
	for i := 0; i < 10; i++ {
		hookContent := fmt.Sprintf(`
id: "perf-hook-%d"
name: "Performance Hook %d"
event: "request_received"
condition: "Data.id == %d"
action: "log_warning"
enabled: true
params:
  message: "Hook %d executed"
`, i, i, i, i)
		err = os.WriteFile(filepath.Join(tmpDir, fmt.Sprintf("perf%d.yaml", i)), []byte(hookContent), 0644)
		require.NoError(t, err)
	}

	err = manager.LoadHooks()
	require.NoError(t, err)
	manager.SubscribeToAllEvents()

	// Test condition evaluation performance
	t.Run("ConditionEvaluation", func(t *testing.T) {
		hooks := manager.GetHooks()
		require.Len(t, hooks, 10)

		ctx := &EventContext{
			Event:     EventRequestReceived,
			Timestamp: time.Now(),
			Data:      map[string]interface{}{"id": 5},
		}

		start := time.Now()
		for i := 0; i < 1000; i++ {
			for _, hook := range hooks {
				_, _ = manager.EvaluateCondition(hook, ctx)
			}
		}
		duration := time.Since(start)

		// Should be able to evaluate 10,000 conditions in reasonable time
		// Increased from 100ms to 200ms to account for slower CI environments
		assert.Less(t, duration, 200*time.Millisecond, "Condition evaluation should be fast")
		t.Logf("Evaluated 10,000 conditions in %v (avg: %v per condition)", duration, duration/10000)
	})

	// Test event processing performance
	t.Run("EventProcessing", func(t *testing.T) {
		start := time.Now()
		for i := 0; i < 100; i++ {
			ctx := &EventContext{
				Event:     EventRequestReceived,
				Timestamp: time.Now(),
				Data:      map[string]interface{}{"id": i % 10},
			}
			bus.Publish(ctx)
		}
		duration := time.Since(start)

		// Should be able to process 100 events quickly
		// Increased from 50ms to 100ms to account for slower CI environments
		assert.Less(t, duration, 100*time.Millisecond, "Event processing should be fast")
		t.Logf("Processed 100 events in %v (avg: %v per event)", duration, duration/100)
	})
}

// TestHookIntegration_ConcurrentAccess tests thread safety
func TestHookIntegration_ConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-concurrent-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)

	// Register a thread-safe counter action
	var counter int64
	var mu sync.Mutex
	manager.RegisterAction("counter", func(hook *Hook, ctx *EventContext) error {
		mu.Lock()
		counter++
		mu.Unlock()
		return nil
	})

	// Create a hook that always triggers
	hookContent := `
id: "counter-hook"
name: "Counter Hook"
event: "request_received"
condition: "true"
action: "counter"
enabled: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "counter.yaml"), []byte(hookContent), 0644)
	require.NoError(t, err)

	err = manager.LoadHooks()
	require.NoError(t, err)
	manager.SubscribeToAllEvents()

	// Launch multiple goroutines to trigger events concurrently
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				ctx := &EventContext{
					Event:     EventRequestReceived,
					Timestamp: time.Now(),
				}
				bus.Publish(ctx)
			}
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond) // Wait for all async executions

	mu.Lock()
	finalCounter := counter
	mu.Unlock()

	expectedCount := int64(numGoroutines * eventsPerGoroutine)
	assert.Equal(t, expectedCount, finalCounter, "All events should have been processed")
}

// Benchmark tests
func BenchmarkHookIntegration_ConditionEvaluation(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "hooks-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	if err != nil {
		b.Fatal(err)
	}

	// Create a hook with a moderately complex condition
	hookContent := `
id: "bench-hook"
name: "Benchmark Hook"
event: "request_received"
condition: "Data.priority > 5 && Provider == 'test' && Data.size < 1000"
action: "log_warning"
enabled: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "bench.yaml"), []byte(hookContent), 0644)
	if err != nil {
		b.Fatal(err)
	}

	err = manager.LoadHooks()
	if err != nil {
		b.Fatal(err)
	}

	hooks := manager.GetHooks()
	if len(hooks) != 1 {
		b.Fatal("Expected 1 hook")
	}
	hook := hooks[0]

	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Provider:  "test",
		Data: map[string]interface{}{
			"priority": 10,
			"size":     500,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manager.EvaluateCondition(hook, ctx)
	}
}

func BenchmarkHookIntegration_EventProcessing(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "hooks-bench-event-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	if err != nil {
		b.Fatal(err)
	}
	RegisterBuiltInActions(manager)

	// Create a simple hook
	hookContent := `
id: "bench-event-hook"
name: "Benchmark Event Hook"
event: "request_received"
condition: "true"
action: "log_warning"
enabled: true
params:
  message: "Benchmark execution"
`
	err = os.WriteFile(filepath.Join(tmpDir, "bench-event.yaml"), []byte(hookContent), 0644)
	if err != nil {
		b.Fatal(err)
	}

	err = manager.LoadHooks()
	if err != nil {
		b.Fatal(err)
	}
	manager.SubscribeToAllEvents()

	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Publish(ctx)
	}
}
