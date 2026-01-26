package overwatch

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestNewOverwatchContext(t *testing.T) {
	t.Run("creates context with default config", func(t *testing.T) {
		ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")

		if ctx.ProcessID != 1234 {
			t.Errorf("expected ProcessID 1234, got %d", ctx.ProcessID)
		}
		if ctx.Provider != "claudecli" {
			t.Errorf("expected Provider claudecli, got %s", ctx.Provider)
		}
		if ctx.Model != "claude-sonnet-4" {
			t.Errorf("expected Model claude-sonnet-4, got %s", ctx.Model)
		}
		if ctx.RequestID != "req-123" {
			t.Errorf("expected RequestID req-123, got %s", ctx.RequestID)
		}
		if ctx.RestartCount != 0 {
			t.Errorf("expected RestartCount 0, got %d", ctx.RestartCount)
		}
		if ctx.LogBuffer == nil {
			t.Error("expected non-nil LogBuffer")
		}
		if ctx.Config == nil {
			t.Error("expected non-nil Config")
		}
	})

	t.Run("creates context with custom config", func(t *testing.T) {
		cfg := &config.OverwatchConfig{
			SilenceThresholdMs:  5000,
			LogBufferSize:       100,
			HeartbeatIntervalMs: 500,
			MaxRestartAttempts:  5,
		}
		ctx := NewOverwatchContext(5678, cfg, "geminicli", "gemini-pro", "req-456")

		if ctx.Config.SilenceThresholdMs != 5000 {
			t.Errorf("expected SilenceThresholdMs 5000, got %d", ctx.Config.SilenceThresholdMs)
		}
		if ctx.LogBuffer.Cap() != 100 {
			t.Errorf("expected LogBuffer capacity 100, got %d", ctx.LogBuffer.Cap())
		}
	})
}

func TestOverwatchContextRecordOutput(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")

	t.Run("records output and updates LastOutputTime", func(t *testing.T) {
		initialTime := ctx.LastOutputTime
		time.Sleep(10 * time.Millisecond)

		ctx.RecordOutput("test line 1")

		if !ctx.LastOutputTime.After(initialTime) {
			t.Error("expected LastOutputTime to be updated")
		}

		lines := ctx.GetLastLogLines(10)
		if len(lines) != 1 || lines[0] != "test line 1" {
			t.Errorf("expected [test line 1], got %v", lines)
		}
	})

	t.Run("clears silent state on output", func(t *testing.T) {
		ctx.MarkSilent()
		if !ctx.IsSilent {
			t.Error("expected IsSilent to be true after MarkSilent")
		}

		ctx.RecordOutput("new output")

		if ctx.IsSilent {
			t.Error("expected IsSilent to be false after RecordOutput")
		}
	})
}

func TestOverwatchContextHeartbeat(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  100, // 100ms for fast testing
		LogBufferSize:       50,
		HeartbeatIntervalMs: 50,
		MaxRestartAttempts:  2,
	}
	ctx := NewOverwatchContext(1234, cfg, "claudecli", "claude-sonnet-4", "req-123")

	t.Run("heartbeat is healthy immediately after creation", func(t *testing.T) {
		if !ctx.CheckHeartbeat() {
			t.Error("expected healthy heartbeat immediately after creation")
		}
	})

	t.Run("heartbeat becomes unhealthy after silence threshold", func(t *testing.T) {
		time.Sleep(150 * time.Millisecond) // Wait longer than threshold

		if ctx.CheckHeartbeat() {
			t.Error("expected unhealthy heartbeat after silence threshold")
		}
	})

	t.Run("heartbeat becomes healthy after output", func(t *testing.T) {
		ctx.RecordOutput("new output")

		if !ctx.CheckHeartbeat() {
			t.Error("expected healthy heartbeat after output")
		}
	})
}

func TestOverwatchContextSilenceDuration(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")

	t.Run("silence duration increases over time", func(t *testing.T) {
		time.Sleep(50 * time.Millisecond)
		duration := ctx.GetSilenceDuration()

		if duration < 50*time.Millisecond {
			t.Errorf("expected silence duration >= 50ms, got %v", duration)
		}
	})

	t.Run("silence duration resets on output", func(t *testing.T) {
		time.Sleep(50 * time.Millisecond)
		ctx.RecordOutput("output")
		duration := ctx.GetSilenceDuration()

		if duration > 10*time.Millisecond {
			t.Errorf("expected silence duration < 10ms after output, got %v", duration)
		}
	})
}

func TestOverwatchContextRestartCount(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 1000,
		MaxRestartAttempts:  2,
	}
	ctx := NewOverwatchContext(1234, cfg, "claudecli", "claude-sonnet-4", "req-123")

	t.Run("can restart when under limit", func(t *testing.T) {
		if !ctx.CanRestart() {
			t.Error("expected CanRestart to be true initially")
		}
	})

	t.Run("increments restart count", func(t *testing.T) {
		count := ctx.IncrementRestartCount()
		if count != 1 {
			t.Errorf("expected restart count 1, got %d", count)
		}

		if !ctx.CanRestart() {
			t.Error("expected CanRestart to be true after 1 restart")
		}
	})

	t.Run("cannot restart when at limit", func(t *testing.T) {
		ctx.IncrementRestartCount() // Now at 2

		if ctx.CanRestart() {
			t.Error("expected CanRestart to be false at limit")
		}
	})
}

func TestOverwatchContextHealingActions(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")

	t.Run("records healing actions", func(t *testing.T) {
		action := types.HealingAction{
			Timestamp:   time.Now(),
			ActionType:  "stdin_injection",
			Description: "Injected 'y' for permission prompt",
			Success:     true,
		}
		ctx.RecordHealingAction(action)

		actions := ctx.GetHealingActions()
		if len(actions) != 1 {
			t.Errorf("expected 1 action, got %d", len(actions))
		}
		if actions[0].ActionType != "stdin_injection" {
			t.Errorf("expected action type stdin_injection, got %s", actions[0].ActionType)
		}
	})

	t.Run("returns copy of actions", func(t *testing.T) {
		actions := ctx.GetHealingActions()
		actions[0].ActionType = "modified"

		originalActions := ctx.GetHealingActions()
		if originalActions[0].ActionType == "modified" {
			t.Error("expected GetHealingActions to return a copy")
		}
	})
}

func TestOverwatchContextCaptureSnapshot(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")
	ctx.RecordOutput("line 1")
	ctx.RecordOutput("line 2")
	ctx.RecordOutput("line 3")

	t.Run("captures snapshot with all fields", func(t *testing.T) {
		snapshot := ctx.CaptureSnapshot("blocked")

		if snapshot.ProcessState != "blocked" {
			t.Errorf("expected ProcessState blocked, got %s", snapshot.ProcessState)
		}
		if snapshot.Provider != "claudecli" {
			t.Errorf("expected Provider claudecli, got %s", snapshot.Provider)
		}
		if snapshot.Model != "claude-sonnet-4" {
			t.Errorf("expected Model claude-sonnet-4, got %s", snapshot.Model)
		}
		if len(snapshot.LastLogLines) != 3 {
			t.Errorf("expected 3 log lines, got %d", len(snapshot.LastLogLines))
		}
		if snapshot.ElapsedTimeMs < 0 {
			t.Errorf("expected non-negative ElapsedTimeMs, got %d", snapshot.ElapsedTimeMs)
		}
		if snapshot.Timestamp.IsZero() {
			t.Error("expected non-zero Timestamp")
		}
	})

	t.Run("increments diagnostic count", func(t *testing.T) {
		initialCount := ctx.DiagnosticCount
		ctx.CaptureSnapshot("running")

		if ctx.DiagnosticCount != initialCount+1 {
			t.Errorf("expected DiagnosticCount to increment")
		}
	})
}

func TestOverwatchContextResetForRestart(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")
	ctx.RecordOutput("old line")
	ctx.IncrementRestartCount()
	ctx.RecordHealingAction(types.HealingAction{ActionType: "test"})
	ctx.MarkSilent()

	ctx.ResetForRestart(5678)

	t.Run("updates process ID", func(t *testing.T) {
		if ctx.ProcessID != 5678 {
			t.Errorf("expected ProcessID 5678, got %d", ctx.ProcessID)
		}
	})

	t.Run("clears log buffer", func(t *testing.T) {
		if ctx.LogBuffer.Len() != 0 {
			t.Errorf("expected empty log buffer, got %d lines", ctx.LogBuffer.Len())
		}
	})

	t.Run("clears silent state", func(t *testing.T) {
		if ctx.IsSilent {
			t.Error("expected IsSilent to be false after reset")
		}
	})

	t.Run("preserves restart count", func(t *testing.T) {
		if ctx.RestartCount != 1 {
			t.Errorf("expected RestartCount 1, got %d", ctx.RestartCount)
		}
	})

	t.Run("preserves healing actions", func(t *testing.T) {
		if len(ctx.HealingActions) != 1 {
			t.Errorf("expected 1 healing action, got %d", len(ctx.HealingActions))
		}
	})
}

func TestOverwatchContextGetHealingMetadata(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-123")
	ctx.RecordHealingAction(types.HealingAction{
		ActionType:  "stdin_injection",
		Description: "Test action",
		Success:     true,
	})

	metadata := ctx.GetHealingMetadata()

	if metadata.RequestID != "req-123" {
		t.Errorf("expected RequestID req-123, got %s", metadata.RequestID)
	}
	if metadata.OriginalProvider != "claudecli" {
		t.Errorf("expected OriginalProvider claudecli, got %s", metadata.OriginalProvider)
	}
	if len(metadata.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(metadata.Actions))
	}
}
