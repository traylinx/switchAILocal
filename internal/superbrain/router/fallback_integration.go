// Package router provides intelligent failover routing for the Superbrain system.
package router

import (
	"context"
	"fmt"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
	"github.com/traylinx/switchAILocal/internal/superbrain/metadata"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// FallbackIntegration provides a high-level interface for fallback routing
// that integrates with the Superbrain metadata and audit systems.
type FallbackIntegration struct {
	router           *FallbackRouter
	auditLogger      *audit.Logger
	metricsCollector *metrics.Metrics
}

// NewFallbackIntegration creates a new FallbackIntegration with the given configuration.
func NewFallbackIntegration(cfg *config.FallbackConfig) *FallbackIntegration {
	return &FallbackIntegration{
		router:           NewFallbackRouter(cfg),
		metricsCollector: metrics.Global(),
	}
}

// SetAuditLogger sets the audit logger for this integration.
func (fi *FallbackIntegration) SetAuditLogger(logger *audit.Logger) {
	fi.auditLogger = logger
}

// FallbackRequest contains the information needed for fallback routing.
type FallbackRequest struct {
	// RequestID uniquely identifies the request.
	RequestID string

	// FailedProvider is the provider that failed.
	FailedProvider string

	// Model is the model being used.
	Model string

	// OriginalPayload is the original request payload.
	OriginalPayload []byte

	// Requirements specifies what capabilities the request needs.
	Requirements *RequestRequirements

	// Diagnosis contains the failure diagnosis if available.
	Diagnosis *types.Diagnosis

	// OriginalError is the error from the failed provider.
	OriginalError error
}

// FallbackResponse contains the result of fallback routing.
type FallbackResponse struct {
	// Success indicates whether a fallback was found.
	Success bool

	// Decision contains the routing decision details.
	Decision *FallbackDecision

	// NegotiatedFailure contains the failure response if no fallback was available.
	NegotiatedFailure *types.NegotiatedFailureResponse

	// Error contains any error that occurred.
	Error error
}

// AttemptFallback attempts to find and route to a fallback provider.
// It records the attempt in the metadata aggregator and audit log.
func (fi *FallbackIntegration) AttemptFallback(ctx context.Context, req *FallbackRequest, aggregator *metadata.Aggregator) *FallbackResponse {
	response := &FallbackResponse{
		Success: false,
	}

	// Try to get a fallback provider
	decision, err := fi.router.GetFallbackWithAdaptation(
		ctx,
		req.FailedProvider,
		req.Requirements,
		req.OriginalPayload,
	)

	if err != nil {
		// No fallback available - create negotiated failure
		response.NegotiatedFailure = fi.createNegotiatedFailure(req, err)
		response.Error = err

		// Record the failed attempt
		if aggregator != nil {
			aggregator.RecordAction(
				"fallback_routing",
				fmt.Sprintf("No fallback available: %v", err),
				false,
				map[string]interface{}{
					"original_provider": req.FailedProvider,
					"error":             err.Error(),
				},
			)
		}

		if fi.auditLogger != nil {
			fi.auditLogger.LogFallback(
				req.RequestID,
				req.FailedProvider,
				"",
				req.Model,
				err.Error(),
				"no_fallback_available",
			)
		}

		if fi.metricsCollector != nil {
			fi.metricsCollector.RecordFallback()
		}

		return response
	}

	// Fallback found
	response.Success = true
	response.Decision = decision

	// Record the successful routing
	if aggregator != nil {
		aggregator.RecordAction(
			"fallback_routing",
			fmt.Sprintf("Routing to fallback provider: %s", decision.FallbackProvider),
			true,
			map[string]interface{}{
				"original_provider": decision.OriginalProvider,
				"fallback_provider": decision.FallbackProvider,
				"reason":            decision.Reason,
				"capability_match":  decision.CapabilityMatch,
			},
		)
		aggregator.SetFinalProvider(decision.FallbackProvider)
	}

	if fi.auditLogger != nil {
		fi.auditLogger.LogFallback(
			req.RequestID,
			decision.OriginalProvider,
			decision.FallbackProvider,
			req.Model,
			decision.Reason,
			"routed",
		)
	}

	if fi.metricsCollector != nil {
		fi.metricsCollector.RecordFallback()
	}

	return response
}

// RecordProviderResult records the result of a provider execution.
// This updates the provider statistics for future routing decisions.
func (fi *FallbackIntegration) RecordProviderResult(provider string, success bool, latency time.Duration, failureReason string) {
	fi.router.UpdateProviderStats(provider, success, latency)

	if fi.metricsCollector != nil {
		if success {
			fi.metricsCollector.RecordHealingSuccess(latency.Milliseconds())
		} else {
			fi.metricsCollector.RecordHealingFailure()
		}
	}
}

// createNegotiatedFailure creates a NegotiatedFailureResponse for when no fallback is available.
func (fi *FallbackIntegration) createNegotiatedFailure(req *FallbackRequest, fallbackErr error) *types.NegotiatedFailureResponse {
	nf := &types.NegotiatedFailureResponse{}

	// Set error information
	nf.Error.Message = "Request failed and no fallback providers are available"
	nf.Error.Type = "provider_unavailable"
	nf.Error.Code = "no_fallback"

	if req.OriginalError != nil {
		nf.Error.Message = fmt.Sprintf("Original error: %s. %s", req.OriginalError.Error(), nf.Error.Message)
	}

	// Set Superbrain information
	nf.Superbrain.AttemptedActions = []string{
		fmt.Sprintf("Attempted execution with %s", req.FailedProvider),
		"Searched for fallback providers",
	}

	if req.Diagnosis != nil {
		nf.Superbrain.DiagnosisSummary = fmt.Sprintf(
			"Failure type: %s. Root cause: %s",
			req.Diagnosis.FailureType,
			req.Diagnosis.RootCause,
		)
	} else {
		nf.Superbrain.DiagnosisSummary = "Unable to diagnose failure"
	}

	// Add suggestions based on the failure
	nf.Superbrain.Suggestions = fi.generateSuggestions(req)

	return nf
}

// generateSuggestions generates helpful suggestions based on the failure.
func (fi *FallbackIntegration) generateSuggestions(req *FallbackRequest) []string {
	suggestions := []string{}

	if req.Diagnosis != nil {
		switch req.Diagnosis.FailureType {
		case types.FailureTypeAuthError:
			suggestions = append(suggestions, "Check your API credentials and ensure they are valid")
			suggestions = append(suggestions, "Try re-authenticating with the provider")

		case types.FailureTypeRateLimit:
			suggestions = append(suggestions, "Wait a few minutes before retrying")
			suggestions = append(suggestions, "Consider using a different provider or API key")

		case types.FailureTypeContextExceeded:
			suggestions = append(suggestions, "Reduce the size of your request content")
			suggestions = append(suggestions, "Use a model with a larger context window")

		case types.FailureTypeNetworkError:
			suggestions = append(suggestions, "Check your network connection")
			suggestions = append(suggestions, "Verify the provider's service status")

		default:
			suggestions = append(suggestions, "Try the request again")
			suggestions = append(suggestions, "Check the provider's documentation for troubleshooting")
		}
	} else {
		suggestions = append(suggestions, "Try the request again")
		suggestions = append(suggestions, "Check your configuration and credentials")
	}

	// Add general suggestions
	suggestions = append(suggestions, "Configure additional fallback providers in config.yaml")

	return suggestions
}

// IsProviderAvailable checks if a provider is currently available for routing.
func (fi *FallbackIntegration) IsProviderAvailable(provider string) bool {
	return fi.router.IsProviderAvailable(provider)
}

// IsProviderConfigured checks if a provider is in the configured fallback list.
func (fi *FallbackIntegration) IsProviderConfigured(provider string) bool {
	return fi.router.IsProviderConfigured(provider)
}

// SetProviderAvailability updates the availability status for a provider.
func (fi *FallbackIntegration) SetProviderAvailability(provider string, available bool) {
	fi.router.SetProviderAvailability(provider, available)
}

// GetProviderCapabilities returns capabilities for all providers.
func (fi *FallbackIntegration) GetProviderCapabilities() []*ProviderCapability {
	return fi.router.GetProviderCapabilities()
}

// GetRouter returns the underlying FallbackRouter for advanced usage.
func (fi *FallbackIntegration) GetRouter() *FallbackRouter {
	return fi.router
}

// HandleExecutionError processes an execution error and determines if fallback should be attempted.
// Returns true if fallback should be attempted, false if the error is not recoverable.
func (fi *FallbackIntegration) HandleExecutionError(err error, diagnosis *types.Diagnosis) bool {
	if err == nil {
		return false
	}

	// If we have a diagnosis, check if it recommends fallback
	if diagnosis != nil {
		switch diagnosis.Remediation {
		case types.RemediationFallback:
			return true
		case types.RemediationAbort:
			return false
		}

		// Check failure type
		switch diagnosis.FailureType {
		case types.FailureTypeAuthError,
			types.FailureTypeRateLimit,
			types.FailureTypeNetworkError,
			types.FailureTypeProcessCrash:
			return true
		case types.FailureTypeContextExceeded:
			// Context exceeded might be handled by sculptor, not fallback
			return false
		}
	}

	// Default: attempt fallback for unknown errors
	return true
}

// CreateFallbackChain creates a chain of fallback attempts for a request.
// This is useful for trying multiple fallback providers in sequence.
type FallbackChain struct {
	integration *FallbackIntegration
	request     *FallbackRequest
	aggregator  *metadata.Aggregator
	attempts    []FallbackAttempt
	maxAttempts int
}

// FallbackAttempt records a single fallback attempt.
type FallbackAttempt struct {
	Provider string
	Success  bool
	Error    error
	Latency  time.Duration
}

// NewFallbackChain creates a new FallbackChain.
func (fi *FallbackIntegration) NewFallbackChain(req *FallbackRequest, aggregator *metadata.Aggregator, maxAttempts int) *FallbackChain {
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	return &FallbackChain{
		integration: fi,
		request:     req,
		aggregator:  aggregator,
		attempts:    make([]FallbackAttempt, 0),
		maxAttempts: maxAttempts,
	}
}

// Next returns the next fallback provider to try.
// Returns nil if no more fallbacks are available or max attempts reached.
func (fc *FallbackChain) Next(ctx context.Context) *FallbackDecision {
	if len(fc.attempts) >= fc.maxAttempts {
		return nil
	}

	// Get the last failed provider
	failedProvider := fc.request.FailedProvider
	if len(fc.attempts) > 0 {
		failedProvider = fc.attempts[len(fc.attempts)-1].Provider
	}

	// Update the request with the last failed provider
	req := *fc.request
	req.FailedProvider = failedProvider

	// Attempt fallback
	response := fc.integration.AttemptFallback(ctx, &req, fc.aggregator)
	if !response.Success {
		return nil
	}

	return response.Decision
}

// RecordAttempt records the result of a fallback attempt.
func (fc *FallbackChain) RecordAttempt(provider string, success bool, err error, latency time.Duration) {
	fc.attempts = append(fc.attempts, FallbackAttempt{
		Provider: provider,
		Success:  success,
		Error:    err,
		Latency:  latency,
	})

	fc.integration.RecordProviderResult(provider, success, latency, "")
}

// GetAttempts returns all recorded attempts.
func (fc *FallbackChain) GetAttempts() []FallbackAttempt {
	return fc.attempts
}

// GetNegotiatedFailure creates a NegotiatedFailureResponse summarizing all attempts.
func (fc *FallbackChain) GetNegotiatedFailure() *types.NegotiatedFailureResponse {
	nf := &types.NegotiatedFailureResponse{}

	nf.Error.Message = "Request failed after exhausting all fallback providers"
	nf.Error.Type = "all_providers_failed"
	nf.Error.Code = "fallback_exhausted"

	// List all attempted actions
	actions := []string{
		fmt.Sprintf("Original request to %s failed", fc.request.FailedProvider),
	}
	fallbacksTried := []string{}

	for _, attempt := range fc.attempts {
		if attempt.Success {
			actions = append(actions, fmt.Sprintf("Fallback to %s succeeded", attempt.Provider))
		} else {
			actions = append(actions, fmt.Sprintf("Fallback to %s failed: %v", attempt.Provider, attempt.Error))
			fallbacksTried = append(fallbacksTried, attempt.Provider)
		}
	}

	nf.Superbrain.AttemptedActions = actions
	nf.Superbrain.FallbacksTried = fallbacksTried

	if fc.request.Diagnosis != nil {
		nf.Superbrain.DiagnosisSummary = fmt.Sprintf(
			"Failure type: %s. Root cause: %s",
			fc.request.Diagnosis.FailureType,
			fc.request.Diagnosis.RootCause,
		)
	}

	nf.Superbrain.Suggestions = []string{
		"Check the status of all configured providers",
		"Verify your API credentials are valid",
		"Consider adding more fallback providers to your configuration",
	}

	return nf
}
