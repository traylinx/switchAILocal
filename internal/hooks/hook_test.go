package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHookSystem_Basic(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "hooks-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)

	// Register a test action
	actionCalled := make(chan bool, 1)
	manager.RegisterAction("test_action", func(hook *Hook, ctx *EventContext) error {
		actionCalled <- true
		return nil
	})

	// Create a test hook
	hookContent := `
id: "test-hook-1"
name: "Test Hook"
event: "request_received"
condition: "Data.priority > 5"
action: "test_action"
enabled: true
`
	err = os.WriteFile(filepath.Join(tmpDir, "test.yaml"), []byte(hookContent), 0644)
	require.NoError(t, err)

	// Load hooks
	err = manager.LoadHooks()
	require.NoError(t, err)
	manager.SubscribeToAllEvents()

	// Trigger event that matches
	ctx := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"priority": 10,
		},
	}
	bus.Publish(ctx)

	// Verify action called
	select {
	case <-actionCalled:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Action was not called")
	}

	// Trigger event that does NOT match
	ctx2 := &EventContext{
		Event:     EventRequestReceived,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"priority": 1,
		},
	}
	bus.Publish(ctx2)

	select {
	case <-actionCalled:
		t.Fatal("Action should not be called")
	case <-time.After(100 * time.Millisecond):
		// Success
	}
}

func TestHookSystem_BuiltInActions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hooks-builtin-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	bus := NewEventBus()
	defer bus.Shutdown()

	manager, err := NewHookManager(tmpDir, bus)
	require.NoError(t, err)
	RegisterBuiltInActions(manager)

	// Test Log Warning
	// We can't easily capture logs, but we can ensure it doesn't error
	hook := &Hook{
		Action: ActionLogWarning,
		Params: map[string]interface{}{
			"message": "Test warning",
		},
	}
	ctx := &EventContext{Event: EventRequestReceived}

	// Execute directly via manager to test handler logic
	manager.executeAction(hook, ctx)
}

func TestEventBus_Async(t *testing.T) {
	bus := NewEventBus()
	defer bus.Shutdown()

	received := make(chan bool, 1)
	bus.Subscribe(EventRequestReceived, func(ctx *EventContext) {
		received <- true
	})

	bus.PublishAsync(&EventContext{Event: EventRequestReceived})

	select {
	case <-received:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Async event not received")
	}
}
