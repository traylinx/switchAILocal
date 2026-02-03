package hooks

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gopkg.in/yaml.v3"
)

// TestProperty_HookExecution tests Property 13: Hook Execution
// Validates: Requirements FR-4.1, FR-4.2, FR-4.3
func TestProperty_HookExecution(t *testing.T) {
	properties := gopter.NewProperties(nil)

	properties.Property("matching events consistently trigger hooks", prop.ForAll(
		func(priority int, eventType string) bool {
			// Setup
			tmpDir, err := os.MkdirTemp("", "hooks-prop-*")
			if err != nil {
				return false
			}
			defer os.RemoveAll(tmpDir)

			evt := HookEvent(eventType)
			if eventType == "" {
				evt = EventRequestReceived
			}

			// Define a hook that triggers if priority > 10
			hook := Hook{
				ID:        "prop-hook",
				Name:      "Prop Hook",
				Event:     evt,
				Condition: "Data.priority > 10",
				Action:    ActionLogWarning, // Use a safe built-in action
				Enabled:   true,
			}
			data, _ := yaml.Marshal(hook)
			_ = os.WriteFile(filepath.Join(tmpDir, "hook.yaml"), data, 0644)

			bus := NewEventBus()
			defer bus.Shutdown()

			manager, _ := NewHookManager(tmpDir, bus)
			RegisterBuiltInActions(manager)
			_ = manager.LoadHooks()
			manager.SubscribeToAllEvents()

			// Use a channel to detect execution (since LogWarning just logs)
			// Implementation tricky without custom action.
			// Let's register a CUSTOM action for this test instead of built-in.
			var triggered atomic.Bool
			manager.RegisterAction("custom_action", func(h *Hook, ctx *EventContext) error {
				triggered.Store(true)
				return nil
			})

			// Update hook to use custom action
			hook.Action = "custom_action"
			data, _ = yaml.Marshal(hook)
			_ = os.WriteFile(filepath.Join(tmpDir, "hook.yaml"), data, 0644)
			_ = manager.LoadHooks() // Reload

			// Trigger event
			bus.Publish(&EventContext{
				Event: evt,
				Data: map[string]interface{}{
					"priority": priority,
				},
			})

			// Wait a bit for async (if bus was async, but Publish is sync usually in tests or if we use PublishAsync)
			// Wait, manager.SubscribeToAllEvents uses Subscribe which is sync callback.
			// But EventBus.Publish executes in goroutine if unsafe? No, Publish is sync.
			// PublishAsync is async.
			// The manager handler `m.handleEvent` calls `go m.executeAction`. So it IS async.
			time.Sleep(50 * time.Millisecond)

			shouldTrigger := priority > 10
			return triggered.Load() == shouldTrigger
		},
		gen.IntRange(0, 20),
		gen.OneConstOf("request_received", "request_failed", "quota_warning"),
	))

	properties.TestingRun(t, gopter.ConsoleReporter(false))
}
