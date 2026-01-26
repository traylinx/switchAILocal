// Package superbrain provides intelligent orchestration and self-healing capabilities
// for the switchAILocal gateway.
package superbrain

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
	"github.com/traylinx/switchAILocal/internal/superbrain/doctor"
	"github.com/traylinx/switchAILocal/internal/superbrain/injector"
	"github.com/traylinx/switchAILocal/internal/superbrain/metadata"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
	"github.com/traylinx/switchAILocal/internal/superbrain/overwatch"
	"github.com/traylinx/switchAILocal/internal/superbrain/recovery"
	"github.com/traylinx/switchAILocal/internal/superbrain/router"
	"github.com/traylinx/switchAILocal/internal/superbrain/sculptor"
	sdkauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// ProviderExecutor defines the contract for provider executors that can be wrapped by Superbrain.
type ProviderExecutor interface {
	Identifier() string
	Execute(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error)
	ExecuteStream(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error)
	Refresh(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error)
	CountTokens(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error)
}

// SuperbrainExecutor wraps existing executors with Superbrain monitoring and healing capabilities.
// It implements the ProviderExecutor interface and adds intelligent orchestration.
type SuperbrainExecutor struct {
	mu sync.RWMutex

	// wrapped is the underlying executor being enhanced.
	wrapped ProviderExecutor

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

	// fallbackRouter handles intelligent failover.
	fallbackRouter *router.FallbackRouter

	// contextSculptor handles pre-flight content optimization.
	contextSculptor *sculptor.ContentOptimizer

	// tokenEstimator estimates token counts.
	tokenEstimator *sculptor.TokenEstimator

	// fileAnalyzer analyzes file references.
	fileAnalyzer *sculptor.FileAnalyzer

	// auditLogger logs autonomous actions.
	auditLogger *audit.Logger

	// metricsCollector tracks Superbrain metrics.
	metricsCollector *metrics.Metrics
}

// SuperbrainExecutorOption is a functional option for configuring the SuperbrainExecutor.
type SuperbrainExecutorOption func(*SuperbrainExecutor)

// WithMonitor sets a custom monitor.
func WithMonitor(m *overwatch.Monitor) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.monitor = m
	}
}

// WithDoctor sets a custom doctor.
func WithDoctor(d *doctor.InternalDoctor) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.doctor = d
	}
}

// WithInjector sets a custom injector.
func WithInjector(i *injector.StdinInjector) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.injector = i
	}
}

// WithRestartManager sets a custom restart manager.
func WithRestartManager(rm *recovery.RestartManager) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.restartManager = rm
	}
}

// WithFallbackRouter sets a custom fallback router.
func WithFallbackRouter(fr *router.FallbackRouter) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.fallbackRouter = fr
	}
}

// WithAuditLogger sets a custom audit logger.
func WithAuditLogger(al *audit.Logger) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.auditLogger = al
	}
}

// WithMetricsCollector sets a custom metrics collector.
func WithMetricsCollector(mc *metrics.Metrics) SuperbrainExecutorOption {
	return func(se *SuperbrainExecutor) {
		se.metricsCollector = mc
	}
}

// NewSuperbrainExecutor creates a new SuperbrainExecutor wrapping the given executor.
func NewSuperbrainExecutor(wrapped ProviderExecutor, cfg *config.SuperbrainConfig, opts ...SuperbrainExecutorOption) *SuperbrainExecutor {
	se := &SuperbrainExecutor{
		wrapped: wrapped,
		config:  cfg,
	}

	// Apply options first
	for _, opt := range opts {
		opt(se)
	}

	// Initialize components if not provided via options
	if se.monitor == nil && cfg != nil {
		se.monitor = overwatch.NewMonitor(&cfg.Overwatch, se.onSnapshot)
	}

	if se.doctor == nil && cfg != nil {
		se.doctor = doctor.NewInternalDoctor(&cfg.Doctor)
	}

	if se.injector == nil && cfg != nil {
		var err error
		se.injector, err = injector.NewStdinInjector(injector.Config{
			Mode:              cfg.StdinInjection.Mode,
			CustomPatterns:    cfg.StdinInjection.CustomPatterns,
			ForbiddenPatterns: cfg.StdinInjection.ForbiddenPatterns,
			AuditLogger:       se.auditLogger,
		})
		if err != nil {
			log.Warnf("Failed to create stdin injector: %v", err)
		}
	}

	if se.restartManager == nil {
		se.restartManager = recovery.NewRestartManager()
	}

	if se.fallbackRouter == nil && cfg != nil {
		se.fallbackRouter = router.NewFallbackRouter(&cfg.Fallback)
	}

	if se.tokenEstimator == nil {
		se.tokenEstimator = sculptor.NewTokenEstimator("simple")
	}

	if se.fileAnalyzer == nil {
		se.fileAnalyzer = sculptor.NewFileAnalyzer(se.tokenEstimator, ".")
	}

	if se.contextSculptor == nil && cfg != nil {
		se.contextSculptor = sculptor.NewContentOptimizer(
			se.tokenEstimator,
			se.fileAnalyzer,
			cfg.ContextSculptor.PriorityFiles,
		)
	}

	if se.metricsCollector == nil {
		se.metricsCollector = metrics.Global()
	}

	return se
}

// Identifier returns the provider key handled by this executor.
func (se *SuperbrainExecutor) Identifier() string {
	return se.wrapped.Identifier()
}

// Execute handles non-streaming execution with Superbrain monitoring and healing.
func (se *SuperbrainExecutor) Execute(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	// Check if Superbrain is enabled
	if !se.isEnabled() {
		return se.wrapped.Execute(ctx, auth, req, opts)
	}

	// Generate request ID for tracking
	requestID := se.getOrCreateRequestID(ctx)

	// Create metadata aggregator
	aggregator := metadata.NewAggregator(requestID, se.wrapped.Identifier())

	// Get the current mode
	mode := se.getMode()

	// Pre-flight analysis (Context Sculptor)
	if se.shouldPerformPreFlight() {
		optimizedReq, hdm, err := se.performPreFlight(ctx, req, aggregator)
		if err != nil {
			// Pre-flight failure - return error with recommendations
			return switchailocalexecutor.Response{}, err
		}
		if optimizedReq != nil {
			req = *optimizedReq
			if hdm != nil {
				aggregator.SetContextOptimization(hdm)
			}
		}
	}

	// Execute with monitoring based on mode
	switch mode {
	case "disabled":
		return se.wrapped.Execute(ctx, auth, req, opts)

	case "observe":
		// Monitor and log but don't take action
		return se.executeWithObservation(ctx, auth, req, opts, aggregator)

	case "diagnose":
		// Diagnose and log proposed actions but don't execute them
		return se.executeWithDiagnosis(ctx, auth, req, opts, aggregator)

	case "conservative", "autopilot":
		// Full healing capabilities
		return se.executeWithHealing(ctx, auth, req, opts, aggregator)

	default:
		// Unknown mode, fall back to wrapped executor
		return se.wrapped.Execute(ctx, auth, req, opts)
	}
}

// ExecuteStream handles streaming execution with Superbrain monitoring and healing.
func (se *SuperbrainExecutor) ExecuteStream(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (<-chan switchailocalexecutor.StreamChunk, error) {
	// Check if Superbrain is enabled
	if !se.isEnabled() {
		return se.wrapped.ExecuteStream(ctx, auth, req, opts)
	}

	// Generate request ID for tracking
	requestID := se.getOrCreateRequestID(ctx)

	// Create metadata aggregator
	aggregator := metadata.NewAggregator(requestID, se.wrapped.Identifier())

	// Get the current mode
	mode := se.getMode()

	// Pre-flight analysis (Context Sculptor)
	if se.shouldPerformPreFlight() {
		optimizedReq, hdm, err := se.performPreFlight(ctx, req, aggregator)
		if err != nil {
			return nil, err
		}
		if optimizedReq != nil {
			req = *optimizedReq
			if hdm != nil {
				aggregator.SetContextOptimization(hdm)
			}
		}
	}

	// Execute with monitoring based on mode
	switch mode {
	case "disabled":
		return se.wrapped.ExecuteStream(ctx, auth, req, opts)

	case "observe":
		return se.executeStreamWithObservation(ctx, auth, req, opts, aggregator)

	case "diagnose":
		return se.executeStreamWithDiagnosis(ctx, auth, req, opts, aggregator)

	case "conservative", "autopilot":
		return se.executeStreamWithHealing(ctx, auth, req, opts, aggregator)

	default:
		return se.wrapped.ExecuteStream(ctx, auth, req, opts)
	}
}

// Refresh attempts to refresh provider credentials.
func (se *SuperbrainExecutor) Refresh(ctx context.Context, auth *sdkauth.Auth) (*sdkauth.Auth, error) {
	return se.wrapped.Refresh(ctx, auth)
}

// CountTokens returns the token count for the given request.
func (se *SuperbrainExecutor) CountTokens(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options) (switchailocalexecutor.Response, error) {
	return se.wrapped.CountTokens(ctx, auth, req, opts)
}


// isEnabled checks if Superbrain is enabled.
func (se *SuperbrainExecutor) isEnabled() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return false
	}
	return se.config.Enabled && se.config.Mode != "disabled"
}

// getMode returns the current Superbrain mode.
func (se *SuperbrainExecutor) getMode() string {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return "disabled"
	}
	return se.config.Mode
}

// shouldPerformPreFlight checks if pre-flight analysis should be performed.
func (se *SuperbrainExecutor) shouldPerformPreFlight() bool {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return false
	}
	
	// Check if Context Sculptor is enabled via component flags
	if !se.config.ComponentFlags.SculptorEnabled {
		return false
	}
	
	return se.config.ContextSculptor.Enabled && se.contextSculptor != nil
}

// getOrCreateRequestID extracts or generates a request ID.
func (se *SuperbrainExecutor) getOrCreateRequestID(ctx context.Context) string {
	// Try to get from context
	if reqID, ok := ctx.Value("request_id").(string); ok && reqID != "" {
		return reqID
	}
	// Generate new UUID
	return uuid.New().String()
}

// onSnapshot is called when a diagnostic snapshot is captured.
func (se *SuperbrainExecutor) onSnapshot(owCtx *overwatch.OverwatchContext, snapshot *DiagnosticSnapshot) {
	if se.auditLogger != nil {
		se.auditLogger.LogSilenceDetection(
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
	}).Warn("Silence threshold exceeded")
}

// performPreFlight performs pre-flight analysis and content optimization.
func (se *SuperbrainExecutor) performPreFlight(ctx context.Context, req switchailocalexecutor.Request, aggregator *metadata.Aggregator) (*switchailocalexecutor.Request, *HighDensityMap, error) {
	if se.contextSculptor == nil || se.fileAnalyzer == nil {
		return nil, nil, nil
	}

	// Extract model name from request
	modelName := req.Model
	if modelName == "" {
		modelName = "unknown"
	}

	// Extract content from request payload
	content := string(req.Payload)

	// Analyze request for file references
	var cliArgs []string
	// Try to extract CLI args from payload if present
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(req.Payload, &payloadMap); err == nil {
		if extraBody, ok := payloadMap["extra_body"].(map[string]interface{}); ok {
			if cli, ok := extraBody["cli"].(map[string]interface{}); ok {
				if attachments, ok := cli["attachments"].([]interface{}); ok {
					for _, att := range attachments {
						if attMap, ok := att.(map[string]interface{}); ok {
							if path, ok := attMap["path"].(string); ok {
								cliArgs = append(cliArgs, path)
							}
						}
					}
				}
			}
		}
	}

	// Perform analysis
	analysis := se.fileAnalyzer.AnalyzeRequest(content, cliArgs, modelName)

	// If content doesn't exceed limit, no optimization needed
	if !analysis.ExceedsLimit {
		return nil, nil, nil
	}

	// Log that we're performing optimization
	log.WithFields(log.Fields{
		"model":           modelName,
		"estimated_tokens": analysis.EstimatedTokens,
		"context_limit":   analysis.ModelContextLimit,
		"file_count":      analysis.FileCount,
	}).Info("Content exceeds context limit, performing optimization")

	// Build file list for optimization
	var files []sculptor.FileWithPriority
	for _, path := range analysis.RelevantFiles {
		fileContent := se.fileAnalyzer.GetFileContent(path)
		if fileContent == "" {
			continue
		}
		files = append(files, sculptor.FileWithPriority{
			Path:            path,
			Content:         fileContent,
			EstimatedTokens: se.tokenEstimator.EstimateTokens(fileContent),
		})
	}

	// Extract keywords from content for prioritization
	keywords := extractKeywords(content)

	// Perform pre-flight optimization
	result := se.contextSculptor.PerformPreFlight(files, modelName, keywords)

	if !result.CanProceed {
		// Content is unreducible
		aggregator.RecordAction(
			"context_optimization",
			"Content cannot be reduced to fit context limit",
			false,
			map[string]interface{}{
				"original_tokens": analysis.EstimatedTokens,
				"target_limit":    analysis.ModelContextLimit,
			},
		)

		if se.auditLogger != nil {
			se.auditLogger.LogContextOptimization(
				aggregator.GetMetadata().RequestID,
				se.wrapped.Identifier(),
				modelName,
				analysis.EstimatedTokens,
				0,
				"failed_unreducible",
			)
		}

		// Return error with recommendations
		if unreducibleErr, ok := result.Error.(*sculptor.UnreducibleContentError); ok {
			return nil, nil, fmt.Errorf("content exceeds context limit: %s", unreducibleErr.Message)
		}
		return nil, nil, result.Error
	}

	// Optimization succeeded
	if result.OptimizationResult != nil && result.OptimizationResult.OptimizedContent != "" {
		aggregator.RecordAction(
			"context_optimization",
			fmt.Sprintf("Optimized content from %d to %d tokens", analysis.EstimatedTokens, result.OptimizationResult.TotalTokens),
			true,
			map[string]interface{}{
				"original_tokens":  analysis.EstimatedTokens,
				"optimized_tokens": result.OptimizationResult.TotalTokens,
				"files_included":   len(result.OptimizationResult.IncludedFiles),
				"files_excluded":   len(result.OptimizationResult.ExcludedFiles),
			},
		)

		if se.auditLogger != nil {
			se.auditLogger.LogContextOptimization(
				aggregator.GetMetadata().RequestID,
				se.wrapped.Identifier(),
				modelName,
				analysis.EstimatedTokens,
				result.OptimizationResult.TotalTokens,
				"success",
			)
		}

		// Create optimized request
		// Note: In a real implementation, we would modify the request payload
		// to include the optimized content. For now, we return the original request
		// with the high-density map for transparency.
		return nil, result.OptimizationResult.HighDensityMap, nil
	}

	return nil, nil, nil
}

// executeWithObservation executes with monitoring but no healing actions.
func (se *SuperbrainExecutor) executeWithObservation(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	startTime := time.Now()

	// Execute the request
	resp, err := se.wrapped.Execute(ctx, auth, req, opts)

	// Record metrics
	if se.metricsCollector != nil {
		se.metricsCollector.RecordHealingAttempt()
		if err == nil {
			se.metricsCollector.RecordHealingSuccess(time.Since(startTime).Milliseconds())
		} else {
			se.metricsCollector.RecordHealingFailure()
		}
	}

	// If there was an error, log it but don't take action
	if err != nil {
		log.WithFields(log.Fields{
			"request_id": aggregator.GetMetadata().RequestID,
			"provider":   se.wrapped.Identifier(),
			"error":      err.Error(),
			"mode":       "observe",
		}).Warn("Execution failed (observe mode - no action taken)")
	}

	// Enrich response with metadata if healing occurred
	return se.enrichResponse(resp, aggregator), err
}

// executeWithDiagnosis executes with diagnosis but no healing actions.
func (se *SuperbrainExecutor) executeWithDiagnosis(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	startTime := time.Now()

	// Execute the request
	resp, err := se.wrapped.Execute(ctx, auth, req, opts)

	// Record metrics
	if se.metricsCollector != nil {
		se.metricsCollector.RecordHealingAttempt()
		if err == nil {
			se.metricsCollector.RecordHealingSuccess(time.Since(startTime).Milliseconds())
		} else {
			se.metricsCollector.RecordHealingFailure()
		}
	}

	// If there was an error, diagnose it but don't take action
	if err != nil && se.doctor != nil {
		// Create a diagnostic snapshot from the error
		snapshot := &DiagnosticSnapshot{
			Timestamp:     time.Now(),
			ProcessState:  "failed",
			LastLogLines:  []string{err.Error()},
			ElapsedTimeMs: time.Since(startTime).Milliseconds(),
			Provider:      se.wrapped.Identifier(),
			Model:         req.Model,
		}

		diagnosis, diagErr := se.doctor.Diagnose(ctx, snapshot)
		if diagErr == nil && diagnosis != nil {
			aggregator.RecordDiagnosis(diagnosis)

			log.WithFields(log.Fields{
				"request_id":   aggregator.GetMetadata().RequestID,
				"provider":     se.wrapped.Identifier(),
				"failure_type": diagnosis.FailureType,
				"remediation":  diagnosis.Remediation,
				"mode":         "diagnose",
			}).Info("Diagnosed failure (diagnose mode - no action taken)")

			if se.auditLogger != nil {
				se.auditLogger.LogDiagnosis(
					aggregator.GetMetadata().RequestID,
					se.wrapped.Identifier(),
					req.Model,
					string(diagnosis.FailureType),
					string(diagnosis.Remediation),
					diagnosis.Confidence,
				)
			}
		}
	}

	return se.enrichResponse(resp, aggregator), err
}

// executeWithHealing executes with full healing capabilities.
func (se *SuperbrainExecutor) executeWithHealing(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (switchailocalexecutor.Response, error) {
	startTime := time.Now()

	// Execute the request
	resp, err := se.wrapped.Execute(ctx, auth, req, opts)

	// Record metrics
	if se.metricsCollector != nil {
		se.metricsCollector.RecordHealingAttempt()
		if err == nil {
			se.metricsCollector.RecordHealingSuccess(time.Since(startTime).Milliseconds())
		} else {
			se.metricsCollector.RecordHealingFailure()
		}
	}

	// If successful, return enriched response
	if err == nil {
		return se.enrichResponse(resp, aggregator), nil
	}

	// Check if Doctor component is enabled
	if !se.isComponentEnabled("doctor") {
		log.Debug("Doctor component disabled, skipping diagnosis")
		return se.enrichResponse(resp, aggregator), err
	}

	// Diagnose the failure
	if se.doctor == nil {
		return se.enrichResponse(resp, aggregator), err
	}

	snapshot := &DiagnosticSnapshot{
		Timestamp:     time.Now(),
		ProcessState:  "failed",
		LastLogLines:  []string{err.Error()},
		ElapsedTimeMs: time.Since(startTime).Milliseconds(),
		Provider:      se.wrapped.Identifier(),
		Model:         req.Model,
	}

	diagnosis, diagErr := se.doctor.Diagnose(ctx, snapshot)
	if diagErr != nil {
		return se.enrichResponse(resp, aggregator), err
	}

	aggregator.RecordDiagnosis(diagnosis)

	if se.auditLogger != nil {
		se.auditLogger.LogDiagnosis(
			aggregator.GetMetadata().RequestID,
			se.wrapped.Identifier(),
			req.Model,
			string(diagnosis.FailureType),
			string(diagnosis.Remediation),
			diagnosis.Confidence,
		)
	}

	// Attempt healing based on diagnosis
	switch diagnosis.Remediation {
	case RemediationFallback:
		// Check if Fallback component is enabled
		if !se.isComponentEnabled("fallback") {
			log.Debug("Fallback component disabled, skipping fallback routing")
			return se.enrichResponse(resp, aggregator), err
		}
		// Try fallback routing
		return se.attemptFallback(ctx, auth, req, opts, aggregator, diagnosis, err)

	case RemediationRetry:
		// Check if Recovery component is enabled
		if !se.isComponentEnabled("recovery") {
			log.Debug("Recovery component disabled, skipping retry")
			return se.enrichResponse(resp, aggregator), err
		}
		// Simple retry
		aggregator.RecordAction("simple_retry", "Retrying request", true, nil)
		retryResp, retryErr := se.wrapped.Execute(ctx, auth, req, opts)
		if retryErr == nil {
			if se.metricsCollector != nil {
				se.metricsCollector.RecordHealingByType("simple_retry")
			}
			return se.enrichResponse(retryResp, aggregator), nil
		}
		return se.enrichResponse(resp, aggregator), err

	default:
		// Cannot heal, return original error
		return se.enrichResponse(resp, aggregator), err
	}
}

// attemptFallback attempts to route the request to a fallback provider.
func (se *SuperbrainExecutor) attemptFallback(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator, diagnosis *Diagnosis, originalErr error) (switchailocalexecutor.Response, error) {
	if se.fallbackRouter == nil {
		return switchailocalexecutor.Response{}, originalErr
	}

	// Determine requirements
	requirements := &router.RequestRequirements{
		RequiresStream: opts.Stream,
		RequiresCLI:    true, // Assume CLI for now
	}

	// Get fallback decision
	decision, err := se.fallbackRouter.GetFallback(ctx, se.wrapped.Identifier(), requirements)
	if err != nil {
		aggregator.RecordAction(
			"fallback_routing",
			fmt.Sprintf("No fallback available: %v", err),
			false,
			map[string]interface{}{
				"original_provider": se.wrapped.Identifier(),
				"error":             err.Error(),
			},
		)
		return switchailocalexecutor.Response{}, originalErr
	}

	// Log fallback attempt
	aggregator.RecordAction(
		"fallback_routing",
		fmt.Sprintf("Routing to fallback provider: %s", decision.FallbackProvider),
		true,
		map[string]interface{}{
			"original_provider": decision.OriginalProvider,
			"fallback_provider": decision.FallbackProvider,
			"reason":            decision.Reason,
		},
	)

	if se.auditLogger != nil {
		se.auditLogger.LogFallback(
			aggregator.GetMetadata().RequestID,
			decision.OriginalProvider,
			decision.FallbackProvider,
			req.Model,
			decision.Reason,
			"attempted",
		)
	}

	// Update aggregator with new provider
	aggregator.SetFinalProvider(decision.FallbackProvider)

	// Note: In a real implementation, we would need to get the executor for the
	// fallback provider and execute the request. For now, we return the original error
	// since we don't have access to other executors from here.
	// The actual fallback execution would be handled at a higher level (e.g., in the Manager).

	if se.metricsCollector != nil {
		se.metricsCollector.RecordFallback()
	}

	return switchailocalexecutor.Response{}, fmt.Errorf("fallback to %s not implemented at executor level: %w", decision.FallbackProvider, originalErr)
}

// executeStreamWithObservation executes streaming with observation only.
func (se *SuperbrainExecutor) executeStreamWithObservation(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (<-chan switchailocalexecutor.StreamChunk, error) {
	return se.wrapped.ExecuteStream(ctx, auth, req, opts)
}

// executeStreamWithDiagnosis executes streaming with diagnosis only.
func (se *SuperbrainExecutor) executeStreamWithDiagnosis(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (<-chan switchailocalexecutor.StreamChunk, error) {
	return se.wrapped.ExecuteStream(ctx, auth, req, opts)
}

// executeStreamWithHealing executes streaming with full healing.
func (se *SuperbrainExecutor) executeStreamWithHealing(ctx context.Context, auth *sdkauth.Auth, req switchailocalexecutor.Request, opts switchailocalexecutor.Options, aggregator *metadata.Aggregator) (<-chan switchailocalexecutor.StreamChunk, error) {
	return se.wrapped.ExecuteStream(ctx, auth, req, opts)
}

// enrichResponse adds healing metadata to the response.
func (se *SuperbrainExecutor) enrichResponse(resp switchailocalexecutor.Response, aggregator *metadata.Aggregator) switchailocalexecutor.Response {
	if aggregator == nil || !aggregator.HasActions() {
		return resp
	}

	// Get the healing metadata
	healingMeta := aggregator.GetMetadata()

	// Try to parse the response payload as JSON and add the superbrain extension
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Payload, &payload); err == nil {
		// Add superbrain extension
		payload["superbrain"] = healingMeta.ToOpenAIExtension()["superbrain"]

		// Re-marshal the payload
		if enrichedPayload, err := json.Marshal(payload); err == nil {
			resp.Payload = enrichedPayload
		}
	}

	return resp
}

// extractKeywords extracts keywords from content for file prioritization.
func extractKeywords(content string) []string {
	// Simple keyword extraction - in a real implementation, this would be more sophisticated
	// For now, just return empty slice
	return nil
}

// UpdateConfig updates the Superbrain configuration at runtime.
func (se *SuperbrainExecutor) UpdateConfig(cfg *config.SuperbrainConfig) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.config = cfg

	// Update injector mode if it exists
	if se.injector != nil && cfg != nil {
		_ = se.injector.SetMode(cfg.StdinInjection.Mode)
	}
}

// GetConfig returns the current Superbrain configuration.
func (se *SuperbrainExecutor) GetConfig() *config.SuperbrainConfig {
	se.mu.RLock()
	defer se.mu.RUnlock()

	return se.config
}

// isComponentEnabled checks if a specific component is enabled.
func (se *SuperbrainExecutor) isComponentEnabled(component string) bool {
	se.mu.RLock()
	defer se.mu.RUnlock()

	if se.config == nil {
		return false
	}

	switch component {
	case "overwatch":
		return se.config.ComponentFlags.OverwatchEnabled
	case "doctor":
		return se.config.ComponentFlags.DoctorEnabled
	case "injector":
		return se.config.ComponentFlags.InjectorEnabled
	case "recovery":
		return se.config.ComponentFlags.RecoveryEnabled
	case "fallback":
		return se.config.ComponentFlags.FallbackEnabled
	case "sculptor":
		return se.config.ComponentFlags.SculptorEnabled
	default:
		return false
	}
}

// Stop stops the Superbrain executor and cleans up resources.
func (se *SuperbrainExecutor) Stop() {
	if se.monitor != nil {
		se.monitor.Stop()
	}
}
