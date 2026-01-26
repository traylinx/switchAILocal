package overwatch

import (
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// OverwatchContext holds the monitoring state for a single CLI execution.
// It tracks process health, output timing, and healing actions taken during the execution lifecycle.
type OverwatchContext struct {
	mu sync.RWMutex

	// ProcessID is the OS process ID of the monitored process.
	ProcessID int

	// StartTime is when the process was spawned.
	StartTime time.Time

	// LastOutputTime is the timestamp of the most recent stdout/stderr output.
	// Used for heartbeat detection and silence threshold calculation.
	LastOutputTime time.Time

	// LogBuffer stores recent log lines for diagnostic snapshots.
	LogBuffer *RingBuffer

	// RestartCount tracks how many times this execution has been restarted.
	RestartCount int

	// DiagnosticCount tracks how many diagnostic snapshots have been captured.
	DiagnosticCount int

	// HealingActions records all autonomous interventions taken during this execution.
	HealingActions []types.HealingAction

	// Config holds the Overwatch configuration parameters.
	Config *config.OverwatchConfig

	// Provider is the provider being executed (e.g., "claudecli", "geminicli").
	Provider string

	// Model is the model being used for this execution.
	Model string

	// RequestID uniquely identifies the request that triggered this execution.
	RequestID string

	// IsSilent indicates whether the process is currently in a silent state.
	IsSilent bool

	// SilenceStartTime is when the current silence period began (if IsSilent is true).
	SilenceStartTime time.Time
}

// NewOverwatchContext creates a new monitoring context for a process execution.
// It initializes the log buffer and sets the start time.
func NewOverwatchContext(processID int, cfg *config.OverwatchConfig, provider, model, requestID string) *OverwatchContext {
	if cfg == nil {
		cfg = DefaultOverwatchConfig()
	}

	bufferSize := cfg.LogBufferSize
	if bufferSize <= 0 {
		bufferSize = 50 // Default buffer size
	}

	now := time.Now()
	return &OverwatchContext{
		ProcessID:       processID,
		StartTime:       now,
		LastOutputTime:  now,
		LogBuffer:       NewRingBuffer(bufferSize),
		RestartCount:    0,
		DiagnosticCount: 0,
		HealingActions:  make([]types.HealingAction, 0),
		Config:          cfg,
		Provider:        provider,
		Model:           model,
		RequestID:       requestID,
		IsSilent:        false,
	}
}

// DefaultOverwatchConfig returns the default Overwatch configuration.
func DefaultOverwatchConfig() *config.OverwatchConfig {
	return &config.OverwatchConfig{
		SilenceThresholdMs:  30000, // 30 seconds
		LogBufferSize:       50,
		HeartbeatIntervalMs: 1000, // 1 second
		MaxRestartAttempts:  2,
	}
}

// RecordOutput records that output was received from the process.
// This updates the LastOutputTime for heartbeat tracking and clears the silent state.
func (ctx *OverwatchContext) RecordOutput(line string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.LastOutputTime = time.Now()
	ctx.LogBuffer.Write(line)
	ctx.IsSilent = false
	ctx.SilenceStartTime = time.Time{} // Reset silence start
}

// CheckHeartbeat checks if the process is healthy based on output timing.
// Returns true if output was received within the silence threshold, false otherwise.
func (ctx *OverwatchContext) CheckHeartbeat() bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	silenceThreshold := time.Duration(ctx.Config.SilenceThresholdMs) * time.Millisecond
	silenceDuration := time.Since(ctx.LastOutputTime)

	return silenceDuration < silenceThreshold
}

// GetSilenceDuration returns how long the process has been silent.
func (ctx *OverwatchContext) GetSilenceDuration() time.Duration {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return time.Since(ctx.LastOutputTime)
}

// MarkSilent marks the process as being in a silent state.
// This is called when the silence threshold is first exceeded.
func (ctx *OverwatchContext) MarkSilent() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if !ctx.IsSilent {
		ctx.IsSilent = true
		ctx.SilenceStartTime = time.Now()
	}
}

// GetElapsedTime returns the total time elapsed since process start.
func (ctx *OverwatchContext) GetElapsedTime() time.Duration {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return time.Since(ctx.StartTime)
}

// GetElapsedTimeMs returns the total time elapsed since process start in milliseconds.
func (ctx *OverwatchContext) GetElapsedTimeMs() int64 {
	return ctx.GetElapsedTime().Milliseconds()
}

// IncrementRestartCount increments the restart counter and returns the new count.
func (ctx *OverwatchContext) IncrementRestartCount() int {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.RestartCount++
	return ctx.RestartCount
}

// CanRestart returns true if another restart attempt is allowed.
func (ctx *OverwatchContext) CanRestart() bool {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.RestartCount < ctx.Config.MaxRestartAttempts
}

// IncrementDiagnosticCount increments the diagnostic counter and returns the new count.
func (ctx *OverwatchContext) IncrementDiagnosticCount() int {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.DiagnosticCount++
	return ctx.DiagnosticCount
}

// RecordHealingAction adds a healing action to the context's history.
func (ctx *OverwatchContext) RecordHealingAction(action types.HealingAction) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.HealingActions = append(ctx.HealingActions, action)
}

// GetHealingActions returns a copy of all healing actions recorded.
func (ctx *OverwatchContext) GetHealingActions() []types.HealingAction {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	actions := make([]types.HealingAction, len(ctx.HealingActions))
	copy(actions, ctx.HealingActions)
	return actions
}

// GetLastLogLines returns the last n log lines from the buffer.
func (ctx *OverwatchContext) GetLastLogLines(n int) []string {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()
	return ctx.LogBuffer.GetLast(n)
}

// CaptureSnapshot creates a diagnostic snapshot of the current execution state.
// This is called when the silence threshold is exceeded to gather diagnostic information.
func (ctx *OverwatchContext) CaptureSnapshot(processState string) *types.DiagnosticSnapshot {
	ctx.mu.Lock()
	ctx.DiagnosticCount++
	ctx.mu.Unlock()

	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	return &types.DiagnosticSnapshot{
		Timestamp:     time.Now(),
		ProcessState:  processState,
		LastLogLines:  ctx.LogBuffer.GetAll(),
		ElapsedTimeMs: time.Since(ctx.StartTime).Milliseconds(),
		Provider:      ctx.Provider,
		Model:         ctx.Model,
	}
}

// UpdateProcessID updates the process ID (used after restart).
func (ctx *OverwatchContext) UpdateProcessID(pid int) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.ProcessID = pid
}

// ResetForRestart resets timing state for a new restart attempt.
// Preserves restart count, healing actions, and configuration.
func (ctx *OverwatchContext) ResetForRestart(newPID int) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	now := time.Now()
	ctx.ProcessID = newPID
	ctx.StartTime = now
	ctx.LastOutputTime = now
	ctx.LogBuffer.Clear()
	ctx.IsSilent = false
	ctx.SilenceStartTime = time.Time{}
}

// GetHealingMetadata creates a HealingMetadata struct from the context's recorded actions.
func (ctx *OverwatchContext) GetHealingMetadata() *types.HealingMetadata {
	ctx.mu.RLock()
	defer ctx.mu.RUnlock()

	actions := make([]types.HealingAction, len(ctx.HealingActions))
	copy(actions, ctx.HealingActions)

	return &types.HealingMetadata{
		RequestID:        ctx.RequestID,
		OriginalProvider: ctx.Provider,
		FinalProvider:    ctx.Provider, // May be updated by fallback router
		Actions:          actions,
	}
}
