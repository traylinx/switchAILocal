// Package types provides shared type definitions for the Superbrain system.
// This package exists to avoid import cycles between superbrain and its subpackages.
package types

import "time"

// HealingAction represents a single autonomous intervention taken by the Superbrain system.
// Each action is recorded with a timestamp and outcome for audit and transparency purposes.
type HealingAction struct {
	// Timestamp is when the action was initiated.
	Timestamp time.Time `json:"timestamp"`

	// ActionType categorizes the intervention (e.g., "stdin_injection", "restart_with_flags", "fallback_routing").
	ActionType string `json:"action_type"`

	// Description provides a human-readable explanation of what was done.
	Description string `json:"description"`

	// Success indicates whether the action achieved its intended outcome.
	Success bool `json:"success"`

	// Details contains action-specific metadata (e.g., flags applied, provider switched to).
	Details map[string]interface{} `json:"details,omitempty"`
}

// HealingMetadata aggregates all autonomous actions taken during request processing.
// This metadata is included in responses to provide transparency about what the Superbrain did.
type HealingMetadata struct {
	// RequestID uniquely identifies the request that triggered healing.
	RequestID string `json:"request_id"`

	// OriginalProvider is the provider initially selected for the request.
	OriginalProvider string `json:"original_provider"`

	// FinalProvider is the provider that ultimately fulfilled the request (may differ after fallback).
	FinalProvider string `json:"final_provider"`

	// TotalHealingTimeMs is the cumulative time spent on all healing actions.
	TotalHealingTimeMs int64 `json:"total_healing_time_ms"`

	// Actions is the chronological list of all autonomous interventions.
	Actions []HealingAction `json:"actions"`

	// ContextOptimized indicates whether the Context Sculptor reduced content to fit limits.
	ContextOptimized bool `json:"context_optimized,omitempty"`

	// HighDensityMap contains a summary of excluded content when optimization occurred.
	HighDensityMap *HighDensityMap `json:"high_density_map,omitempty"`

	// DiagnosisHistory contains all diagnoses performed during the request lifecycle.
	DiagnosisHistory []*Diagnosis `json:"diagnosis_history,omitempty"`
}


// ToOpenAIExtension formats the healing metadata for inclusion in OpenAI-compatible responses.
// This allows clients to see what autonomous actions were taken without breaking API compatibility.
func (h *HealingMetadata) ToOpenAIExtension() map[string]interface{} {
	return map[string]interface{}{
		"superbrain": map[string]interface{}{
			"healed":            len(h.Actions) > 0,
			"original_provider": h.OriginalProvider,
			"final_provider":    h.FinalProvider,
			"healing_actions":   h.formatActions(),
			"context_optimized": h.ContextOptimized,
		},
	}
}

// formatActions converts the actions slice into a simplified format for API responses.
func (h *HealingMetadata) formatActions() []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(h.Actions))
	for i, action := range h.Actions {
		formatted[i] = map[string]interface{}{
			"type":        action.ActionType,
			"description": action.Description,
			"success":     action.Success,
		}
	}
	return formatted
}

// DiagnosticSnapshot contains state captured during a potential failure.
// This snapshot is analyzed by the Internal Doctor to determine the failure type and remediation.
type DiagnosticSnapshot struct {
	// Timestamp is when the snapshot was captured.
	Timestamp time.Time `json:"timestamp"`

	// ProcessState describes the current state of the process ("running", "blocked", "terminated").
	ProcessState string `json:"process_state"`

	// LastLogLines contains the most recent output from stdout/stderr.
	LastLogLines []string `json:"last_log_lines"`

	// ElapsedTimeMs is the time elapsed since process start.
	ElapsedTimeMs int64 `json:"elapsed_time_ms"`

	// StderrContent contains the full stderr output if available.
	StderrContent string `json:"stderr_content,omitempty"`

	// Provider is the provider being executed (e.g., "claudecli", "geminicli").
	Provider string `json:"provider"`

	// Model is the model being used.
	Model string `json:"model"`
}

// FailureType categorizes the detected issue for targeted remediation.
type FailureType string

const (
	// FailureTypePermissionPrompt indicates the process is waiting for user permission.
	FailureTypePermissionPrompt FailureType = "permission_prompt"

	// FailureTypeAuthError indicates authentication or authorization failure.
	FailureTypeAuthError FailureType = "auth_error"

	// FailureTypeContextExceeded indicates the request exceeded the model's context limit.
	FailureTypeContextExceeded FailureType = "context_exceeded"

	// FailureTypeRateLimit indicates the provider is rate limiting requests.
	FailureTypeRateLimit FailureType = "rate_limit"

	// FailureTypeNetworkError indicates a network connectivity issue.
	FailureTypeNetworkError FailureType = "network_error"

	// FailureTypeProcessCrash indicates the process terminated unexpectedly.
	FailureTypeProcessCrash FailureType = "process_crash"

	// FailureTypeUnknown indicates the failure pattern was not recognized.
	FailureTypeUnknown FailureType = "unknown"
)

// RemediationType specifies the recommended action to resolve a failure.
type RemediationType string

const (
	// RemediationStdinInject indicates stdin injection should be attempted.
	RemediationStdinInject RemediationType = "stdin_inject"

	// RemediationRestartFlags indicates the process should be restarted with corrective flags.
	RemediationRestartFlags RemediationType = "restart_with_flags"

	// RemediationFallback indicates the request should be routed to an alternative provider.
	RemediationFallback RemediationType = "fallback_provider"

	// RemediationRetry indicates a simple retry without changes.
	RemediationRetry RemediationType = "simple_retry"

	// RemediationAbort indicates the failure is unrecoverable and should be reported.
	RemediationAbort RemediationType = "abort"
)

// Diagnosis represents the Internal Doctor's analysis of a failure.
type Diagnosis struct {
	// FailureType categorizes the detected issue.
	FailureType FailureType `json:"failure_type"`

	// RootCause provides a human-readable explanation of what went wrong.
	RootCause string `json:"root_cause"`

	// Confidence is the Doctor's confidence in the diagnosis (0.0 to 1.0).
	Confidence float64 `json:"confidence"`

	// Remediation specifies the recommended action.
	Remediation RemediationType `json:"remediation"`

	// RemediationArgs contains action-specific parameters (e.g., flags to apply, pattern to inject).
	RemediationArgs map[string]string `json:"remediation_args,omitempty"`

	// RawAnalysis contains the full diagnostic output from the AI model if used.
	RawAnalysis string `json:"raw_analysis,omitempty"`
}

// HighDensityMap summarizes content excluded during Context Sculptor optimization.
// This provides transparency about what was removed to fit within context limits.
type HighDensityMap struct {
	// TotalFiles is the total number of files referenced in the original request.
	TotalFiles int `json:"total_files"`

	// IncludedFiles is the number of files included after optimization.
	IncludedFiles int `json:"included_files"`

	// ExcludedFiles is the number of files excluded to fit within limits.
	ExcludedFiles int `json:"excluded_files"`

	// DirectoryTree is a text representation of the directory structure.
	DirectoryTree string `json:"directory_tree,omitempty"`

	// FileSummaries maps excluded file paths to brief summaries of their content.
	FileSummaries map[string]string `json:"file_summaries,omitempty"`

	// TokensSaved is the estimated number of tokens saved by optimization.
	TokensSaved int `json:"tokens_saved"`
}

// NegotiatedFailureResponse represents an intelligent error response that explains
// what autonomous actions were attempted and why the request ultimately failed.
// This provides transparency and actionable information instead of generic 500 errors.
type NegotiatedFailureResponse struct {
	// Error contains the standard error information.
	Error struct {
		// Message is a human-readable description of the failure.
		Message string `json:"message"`

		// Type categorizes the error (e.g., "provider_unavailable", "unrecoverable_failure").
		Type string `json:"type"`

		// Code is a machine-readable error code for programmatic handling.
		Code string `json:"code"`
	} `json:"error"`

	// Superbrain contains information about autonomous actions attempted during healing.
	Superbrain struct {
		// AttemptedActions lists all healing actions that were tried.
		AttemptedActions []string `json:"attempted_actions"`

		// DiagnosisSummary provides the Internal Doctor's analysis of the failure.
		DiagnosisSummary string `json:"diagnosis_summary"`

		// Suggestions provides actionable recommendations for the user.
		Suggestions []string `json:"suggestions"`

		// FallbacksTried lists all alternative providers that were attempted.
		FallbacksTried []string `json:"fallbacks_tried,omitempty"`
	} `json:"superbrain"`
}
