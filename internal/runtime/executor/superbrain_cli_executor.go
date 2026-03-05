// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain"
	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
	"github.com/traylinx/switchAILocal/internal/superbrain/doctor"
	"github.com/traylinx/switchAILocal/internal/superbrain/injector"
	"github.com/traylinx/switchAILocal/internal/superbrain/metadata"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
	"github.com/traylinx/switchAILocal/internal/superbrain/overwatch"
	"github.com/traylinx/switchAILocal/internal/superbrain/recovery"
	sdkauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// SuperbrainCLIExecutor wraps LocalCLIExecutor with Superbrain monitoring and healing.
// It provides real-time process monitoring, failure diagnosis, stdin injection,
// and process recovery capabilities.
type SuperbrainCLIExecutor struct {
	mu sync.RWMutex

	// base is the underlying CLI executor.
	base *LocalCLIExecutor

	// config holds the Superbrain configuration.
	config *config.SuperbrainConfig

	// monitor provides real-time execution monitoring.
	monitor *overwatch.Monitor

	// doctor provides AI-powered failure diagnosis.
	doctor *doctor.InternalDoctor

	// injector provides autonomous stdin injection.
	injector *injector.StdinInjector

	// restartManager handles process restart logic.
	restartManager *recovery.RestartManager

	// auditLogger logs autonomous actions.
	auditLogger *audit.Logger

	// metricsCollector tracks Superbrain metrics.
	metricsCollector *metrics.Metrics
}

// NewSuperbrainCLIExecutor creates a new Superbrain-enhanced CLI executor.
func NewSuperbrainCLIExecutor(base *LocalCLIExecutor, cfg *config.SuperbrainConfig) *SuperbrainCLIExecutor {
	se := &SuperbrainCLIExecutor{
		base:           base,
		config:         cfg,
		restartManager: recovery.NewRestartManager(),
	}

	if cfg != nil {
		// Initialize monitor with snapshot handler
		se.monitor = overwatch.NewMonitor(&cfg.Overwatch, se.onSnapshot)

		// Initialize doctor
		se.doctor = doctor.NewInternalDoctor(&cfg.Doctor)

		// Initialize injector
		var err error
		se.injector, err = injector.NewStdinInjector(injector.Config{
			Mode:              cfg.StdinInjection.Mode,
			CustomPatterns:    cfg.StdinInjection.CustomPatterns,
			ForbiddenPatterns: cfg.StdinInjection.ForbiddenPatterns,
		})
		if err != nil {
			log.Warnf("Failed to create stdin injector: %v", err)
		}
	}

	se.metricsCollector = metrics.Global()

	return se
}

// SetAuditLogger sets the audit logger for this executor.
func (se *SuperbrainCLIExecutor) SetAuditLogger(logger *audit.Logger) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.auditLogger = logger
}

// Identifier returns the provider key handled by this executor.
func (se *SuperbrainCLIExecutor) Identifier() string {
	return se.base.Identifier()
}

// Execute runs the CLI tool with Superbrain monitoring and healing.
func (se *SuperbrainCLIExecutor) Execute(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	// Check if Superbrain is enabled
	if !se.isEnabled() {
		return se.base.Execute(ctx, auth, req, opts)
	}

	// Generate request ID
	requestID := se.getOrCreateRequestID(ctx)
	modelName := extractModelName(req)

	// Create metadata aggregator
	aggregator := metadata.NewAggregator(requestID, se.base.Provider)

	// Get the current mode
	mode := se.getMode()

	switch mode {
	case "disabled":
		return se.base.Execute(ctx, auth, req, opts)

	case "observe":
		return se.executeWithObservation(ctx, auth, req, opts, requestID, modelName, aggregator)

	case "diagnose":
		return se.executeWithDiagnosis(ctx, auth, req, opts, requestID, modelName, aggregator)

	case "conservative", "autopilot":
		return se.executeWithHealing(ctx, auth, req, opts, requestID, modelName, aggregator)

	default:
		return se.base.Execute(ctx, auth, req, opts)
	}
}

// ExecuteStream runs the CLI tool with streaming and Superbrain monitoring.
func (se *SuperbrainCLIExecutor) ExecuteStream(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	// Check if Superbrain is enabled
	if !se.isEnabled() {
		return se.base.ExecuteStream(ctx, auth, req, opts)
	}

	// Generate request ID
	requestID := se.getOrCreateRequestID(ctx)
	modelName := extractModelName(req)

	// Create metadata aggregator
	aggregator := metadata.NewAggregator(requestID, se.base.Provider)

	// Get the current mode
	mode := se.getMode()

	switch mode {
	case "disabled":
		return se.base.ExecuteStream(ctx, auth, req, opts)

	case "observe", "diagnose", "conservative", "autopilot":
		return se.executeStreamWithMonitoring(ctx, auth, req, opts, requestID, modelName, aggregator)

	default:
		return se.base.ExecuteStream(ctx, auth, req, opts)
	}
}

// Refresh attempts to refresh provider credentials.
func (se *SuperbrainCLIExecutor) Refresh(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error) {
	return se.base.Refresh(ctx, auth)
}

// CountTokens returns the token count for the given request.
func (se *SuperbrainCLIExecutor) CountTokens(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	return se.base.CountTokens(ctx, auth, req, opts)
}

// isEnabled checks if Superbrain is enabled.
func (se *SuperbrainCLIExecutor) isEnabled() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return false
	}
	return se.config.Enabled && se.config.Mode != "disabled"
}

// getMode returns the current Superbrain mode.
func (se *SuperbrainCLIExecutor) getMode() string {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return "disabled"
	}
	return se.config.Mode
}

// getOrCreateRequestID extracts or generates a request ID.
func (se *SuperbrainCLIExecutor) getOrCreateRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value("request_id").(string); ok && reqID != "" {
		return reqID
	}
	return uuid.New().String()
}

// onSnapshot is called when a diagnostic snapshot is captured due to silence detection.
func (se *SuperbrainCLIExecutor) onSnapshot(owCtx *overwatch.OverwatchContext, snapshot *superbrain.DiagnosticSnapshot) {
	se.mu.RLock()
	auditLogger := se.auditLogger
	se.mu.RUnlock()

	if auditLogger != nil {
		auditLogger.LogSilenceDetection(
			owCtx.RequestID,
			owCtx.Provider,
			owCtx.Model,
			owCtx.GetSilenceDuration().Milliseconds(),
		)
	}

	if se.metricsCollector != nil {
		se.metricsCollector.RecordSilenceDetection()
	}

	log.WithFields(log.Fields{
		"request_id": owCtx.RequestID,
		"provider":   owCtx.Provider,
		"model":      owCtx.Model,
		"silence_ms": owCtx.GetSilenceDuration().Milliseconds(),
	}).Warn("Silence threshold exceeded - process may be hung")
}

// executeWithObservation executes with monitoring but no healing actions.
func (se *SuperbrainCLIExecutor) executeWithObservation(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, requestID, modelName string, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	// Start monitoring
	owCtx := se.monitor.StartMonitoring(0, se.base.Provider, modelName, requestID)
	defer se.monitor.StopMonitoring(requestID)

	// Execute the request
	resp, err := se.base.Execute(ctx, auth, req, opts)

	// Log any errors but don't take action
	if err != nil {
		log.WithFields(log.Fields{
			"request_id":     requestID,
			"provider":       se.base.Provider,
			"error":          err.Error(),
			"mode":           "observe",
			"silence_ms":     owCtx.GetSilenceDuration().Milliseconds(),
			"restart_count":  owCtx.RestartCount,
		}).Warn("Execution failed (observe mode - no action taken)")
	}

	return resp, err
}

// executeWithDiagnosis executes with diagnosis but no healing actions.
func (se *SuperbrainCLIExecutor) executeWithDiagnosis(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, requestID, modelName string, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	// Start monitoring
	owCtx := se.monitor.StartMonitoring(0, se.base.Provider, modelName, requestID)
	defer se.monitor.StopMonitoring(requestID)

	// Execute the request
	resp, err := se.base.Execute(ctx, auth, req, opts)

	// If there was an error, diagnose it
	if err != nil && se.doctor != nil {
		snapshot := owCtx.CaptureSnapshot("failed")
		snapshot.StderrContent = err.Error()

		diagnosis, diagErr := se.doctor.Diagnose(ctx, snapshot)
		if diagErr == nil && diagnosis != nil {
			aggregator.RecordDiagnosis(diagnosis)

			log.WithFields(log.Fields{
				"request_id":   requestID,
				"provider":     se.base.Provider,
				"failure_type": diagnosis.FailureType,
				"remediation":  diagnosis.Remediation,
				"confidence":   diagnosis.Confidence,
				"mode":         "diagnose",
			}).Info("Diagnosed failure (diagnose mode - no action taken)")

			se.mu.RLock()
			auditLogger := se.auditLogger
			se.mu.RUnlock()

			if auditLogger != nil {
				auditLogger.LogDiagnosis(
					requestID,
					se.base.Provider,
					modelName,
					string(diagnosis.FailureType),
					string(diagnosis.Remediation),
					diagnosis.Confidence,
				)
			}
		}
	}

	return resp, err
}

// executeWithHealing executes with full healing capabilities.
func (se *SuperbrainCLIExecutor) executeWithHealing(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, requestID, modelName string, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	startTime := time.Now()

	// Start monitoring
	owCtx := se.monitor.StartMonitoring(0, se.base.Provider, modelName, requestID)
	defer se.monitor.StopMonitoring(requestID)

	// Execute the request
	resp, err := se.base.Execute(ctx, auth, req, opts)

	// If successful, return
	if err == nil {
		return resp, nil
	}

	// Diagnose the failure
	if se.doctor == nil {
		return resp, err
	}

	snapshot := owCtx.CaptureSnapshot("failed")
	snapshot.StderrContent = err.Error()

	diagnosis, diagErr := se.doctor.Diagnose(ctx, snapshot)
	if diagErr != nil {
		return resp, err
	}

	aggregator.RecordDiagnosis(diagnosis)

	se.mu.RLock()
	auditLogger := se.auditLogger
	se.mu.RUnlock()

	if auditLogger != nil {
		auditLogger.LogDiagnosis(
			requestID,
			se.base.Provider,
			modelName,
			string(diagnosis.FailureType),
			string(diagnosis.Remediation),
			diagnosis.Confidence,
		)
	}

	// Attempt healing based on diagnosis
	switch diagnosis.Remediation {
	case superbrain.RemediationRestartFlags:
		return se.attemptRestartWithFlags(ctx, auth, req, opts, owCtx, aggregator, diagnosis, err)

	case superbrain.RemediationRetry:
		aggregator.RecordAction("simple_retry", "Retrying request", true, nil)
		retryResp, retryErr := se.base.Execute(ctx, auth, req, opts)
		if retryErr == nil {
			if se.metricsCollector != nil {
				se.metricsCollector.RecordHealingSuccess(time.Since(startTime).Milliseconds())
				se.metricsCollector.RecordHealingByType("simple_retry")
			}
			return retryResp, nil
		}
		return resp, err

	default:
		return resp, err
	}
}

// attemptRestartWithFlags attempts to restart the process with corrective flags.
func (se *SuperbrainCLIExecutor) attemptRestartWithFlags(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, owCtx *overwatch.OverwatchContext, aggregator *metadata.Aggregator, diagnosis *superbrain.Diagnosis, originalErr error) (switchailocalexecutor.Response, error) {
	startTime := time.Now()

	// Check restart limit
	maxRestarts := 2
	if se.config != nil {
		maxRestarts = se.config.Overwatch.MaxRestartAttempts
	}

	decision := se.restartManager.DecideRestartOrEscalate(diagnosis, se.base.Provider, owCtx.RestartCount, maxRestarts)

	if !decision.ShouldRestart {
		if decision.ShouldEscalate {
			aggregator.RecordAction(
				"escalate_to_fallback",
				decision.Reason,
				true,
				map[string]interface{}{
					"restart_count": owCtx.RestartCount,
					"max_restarts":  maxRestarts,
				},
			)
		}
		return switchailocalexecutor.Response{}, originalErr
	}

	// Increment restart count
	owCtx.IncrementRestartCount()

	// Record the restart attempt
	aggregator.RecordAction(
		"restart_with_flags",
		fmt.Sprintf("Restarting with corrective flags: %v", decision.Strategy.CorrectiveFlags),
		true,
		map[string]interface{}{
			"corrective_flags": decision.Strategy.CorrectiveFlags,
			"attempt":          owCtx.RestartCount,
		},
	)

	se.mu.RLock()
	auditLogger := se.auditLogger
	se.mu.RUnlock()

	if auditLogger != nil {
		auditLogger.LogRestart(
			aggregator.GetMetadata().RequestID,
			se.base.Provider,
			extractModelName(req),
			decision.Strategy.CorrectiveFlags,
			"attempted",
		)
	}

	// Note: In a real implementation, we would modify the request to include
	// the corrective flags and re-execute. For now, we just retry with the
	// base executor since the Phase 0 Quick Fix already handles the
	// --dangerously-skip-permissions flag for claudecli.
	retryResp, retryErr := se.base.Execute(ctx, auth, req, opts)
	if retryErr == nil {
		if se.metricsCollector != nil {
			se.metricsCollector.RecordHealingSuccess(time.Since(startTime).Milliseconds())
			se.metricsCollector.RecordHealingByType("restart_with_flags")
		}
		return retryResp, nil
	}

	return switchailocalexecutor.Response{}, originalErr
}

// executeStreamWithMonitoring executes streaming with Superbrain monitoring.
func (se *SuperbrainCLIExecutor) executeStreamWithMonitoring(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, requestID, modelName string, aggregator *metadata.Aggregator) (<-chan switchailocalexecutor.StreamChunk, error) {
	// Start monitoring
	owCtx := se.monitor.StartMonitoring(0, se.base.Provider, modelName, requestID)

	// Execute the streaming request
	chunks, err := se.base.ExecuteStream(ctx, auth, req, opts)
	if err != nil {
		se.monitor.StopMonitoring(requestID)
		return nil, err
	}

	// Wrap the channel to track output and stop monitoring when done
	wrappedCh := make(chan switchailocalexecutor.StreamChunk)
	go func() {
		defer close(wrappedCh)
		defer se.monitor.StopMonitoring(requestID)

		for chunk := range chunks {
			// Record output for heartbeat tracking
			if len(chunk.Payload) > 0 {
				owCtx.RecordOutput(string(chunk.Payload))
			}
			wrappedCh <- chunk
		}
	}()

	return wrappedCh, nil
}

// ExecuteWithMonitoredProcess executes a CLI command with full Superbrain monitoring.
// This method provides direct access to the process for stdin injection and monitoring.
func (se *SuperbrainCLIExecutor) ExecuteWithMonitoredProcess(ctx context.Context, binaryPath string, args []string, requestID, modelName string) (*MonitoredProcess, error) {
	if !se.isEnabled() {
		return nil, fmt.Errorf("Superbrain is not enabled")
	}

	// Create the command
	cmd := exec.CommandContext(ctx, binaryPath, args...)

	// Create pipes for stdin, stdout, stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	// Start monitoring
	owCtx := se.monitor.StartMonitoring(cmd.Process.Pid, se.base.Provider, modelName, requestID)

	// Create monitored process
	mp := &MonitoredProcess{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		owCtx:     owCtx,
		monitor:   se.monitor,
		injector:  se.injector,
		doctor:    se.doctor,
		requestID: requestID,
	}

	// Start stream monitoring
	mp.startStreamMonitoring(ctx)

	return mp, nil
}

// MonitoredProcess represents a CLI process with Superbrain monitoring.
type MonitoredProcess struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	owCtx     *overwatch.OverwatchContext
	monitor   *overwatch.Monitor
	injector  *injector.StdinInjector
	doctor    *doctor.InternalDoctor
	requestID string

	stdoutBuf bytes.Buffer
	stderrBuf bytes.Buffer
	mu        sync.Mutex
}

// startStreamMonitoring starts goroutines to monitor stdout and stderr.
func (mp *MonitoredProcess) startStreamMonitoring(ctx context.Context) {
	// Monitor stdout
	go func() {
		scanner := bufio.NewScanner(mp.stdout)
		for scanner.Scan() {
			line := scanner.Text()
			mp.mu.Lock()
			mp.stdoutBuf.WriteString(line + "\n")
			mp.mu.Unlock()
			mp.owCtx.RecordOutput(line)
		}
	}()

	// Monitor stderr
	go func() {
		scanner := bufio.NewScanner(mp.stderr)
		for scanner.Scan() {
			line := scanner.Text()
			mp.mu.Lock()
			mp.stderrBuf.WriteString(line + "\n")
			mp.mu.Unlock()
			mp.owCtx.RecordOutput(line)

			// Check for patterns that might need stdin injection
			if mp.injector != nil {
				pattern := mp.injector.MatchPattern(line)
				if pattern != nil && mp.injector.CanInject(pattern) {
					_ = mp.injector.Inject(mp.stdin, pattern.Response)
				}
			}
		}
	}()
}

// Wait waits for the process to complete.
func (mp *MonitoredProcess) Wait() error {
	err := mp.cmd.Wait()
	mp.monitor.StopMonitoring(mp.requestID)
	return err
}

// Kill terminates the process.
func (mp *MonitoredProcess) Kill() error {
	if mp.cmd.Process != nil {
		return mp.cmd.Process.Kill()
	}
	return nil
}

// GetStdout returns the captured stdout content.
func (mp *MonitoredProcess) GetStdout() string {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.stdoutBuf.String()
}

// GetStderr returns the captured stderr content.
func (mp *MonitoredProcess) GetStderr() string {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	return mp.stderrBuf.String()
}

// GetOverwatchContext returns the monitoring context.
func (mp *MonitoredProcess) GetOverwatchContext() *overwatch.OverwatchContext {
	return mp.owCtx
}

// InjectStdin writes to the process stdin.
func (mp *MonitoredProcess) InjectStdin(input string) error {
	_, err := mp.stdin.Write([]byte(input))
	return err
}

// UpdateConfig updates the Superbrain configuration at runtime.
func (se *SuperbrainCLIExecutor) UpdateConfig(cfg *config.SuperbrainConfig) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.config = cfg

	if se.injector != nil && cfg != nil {
		_ = se.injector.SetMode(cfg.StdinInjection.Mode)
	}
}

// Stop stops the Superbrain executor and cleans up resources.
func (se *SuperbrainCLIExecutor) Stop() {
	if se.monitor != nil {
		se.monitor.Stop()
	}
}
