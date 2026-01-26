package doctor

import (
	"fmt"
	"strings"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// DiagnosisBuilder provides a fluent interface for constructing Diagnosis objects.
type DiagnosisBuilder struct {
	diagnosis *types.Diagnosis
}

// NewDiagnosisBuilder creates a new DiagnosisBuilder with default values.
func NewDiagnosisBuilder() *DiagnosisBuilder {
	return &DiagnosisBuilder{
		diagnosis: &types.Diagnosis{
			FailureType:     types.FailureTypeUnknown,
			Remediation:     types.RemediationAbort,
			Confidence:      0.0,
			RemediationArgs: make(map[string]string),
		},
	}
}

// WithFailureType sets the failure type.
func (b *DiagnosisBuilder) WithFailureType(ft types.FailureType) *DiagnosisBuilder {
	b.diagnosis.FailureType = ft
	return b
}

// WithRootCause sets the root cause description.
func (b *DiagnosisBuilder) WithRootCause(cause string) *DiagnosisBuilder {
	b.diagnosis.RootCause = cause
	return b
}

// WithConfidence sets the confidence level (0.0 to 1.0).
func (b *DiagnosisBuilder) WithConfidence(conf float64) *DiagnosisBuilder {
	if conf < 0.0 {
		conf = 0.0
	}
	if conf > 1.0 {
		conf = 1.0
	}
	b.diagnosis.Confidence = conf
	return b
}

// WithRemediation sets the recommended remediation action.
func (b *DiagnosisBuilder) WithRemediation(rem types.RemediationType) *DiagnosisBuilder {
	b.diagnosis.Remediation = rem
	return b
}

// WithRemediationArg adds a remediation argument.
func (b *DiagnosisBuilder) WithRemediationArg(key, value string) *DiagnosisBuilder {
	b.diagnosis.RemediationArgs[key] = value
	return b
}

// WithRemediationArgs sets all remediation arguments.
func (b *DiagnosisBuilder) WithRemediationArgs(args map[string]string) *DiagnosisBuilder {
	for k, v := range args {
		b.diagnosis.RemediationArgs[k] = v
	}
	return b
}

// WithRawAnalysis sets the raw AI analysis output.
func (b *DiagnosisBuilder) WithRawAnalysis(analysis string) *DiagnosisBuilder {
	b.diagnosis.RawAnalysis = analysis
	return b
}

// Build returns the constructed Diagnosis.
func (b *DiagnosisBuilder) Build() *types.Diagnosis {
	return b.diagnosis
}

// DiagnosisFromPattern creates a Diagnosis from a matched FailurePattern.
func DiagnosisFromPattern(pattern *FailurePattern, matchedText string) *types.Diagnosis {
	builder := NewDiagnosisBuilder().
		WithFailureType(pattern.FailureType).
		WithRootCause(fmt.Sprintf("Pattern '%s' matched: %s", pattern.Name, pattern.Description)).
		WithConfidence(0.8). // Pattern-based diagnosis has high confidence
		WithRemediation(pattern.Remediation).
		WithRemediationArgs(pattern.RemediationArgs)

	return builder.Build()
}

// DiagnosisFromUnknown creates a Diagnosis for unknown failure patterns.
func DiagnosisFromUnknown(logContent string) *types.Diagnosis {
	// Truncate log content for root cause if too long
	truncatedLog := logContent
	if len(truncatedLog) > 500 {
		truncatedLog = truncatedLog[:500] + "..."
	}

	return NewDiagnosisBuilder().
		WithFailureType(types.FailureTypeUnknown).
		WithRootCause("No known failure pattern matched. Raw log content included for analysis.").
		WithConfidence(0.0).
		WithRemediation(types.RemediationAbort).
		WithRawAnalysis(truncatedLog).
		Build()
}

// IsRecoverable returns true if the diagnosis suggests a recoverable failure.
func IsRecoverable(d *types.Diagnosis) bool {
	switch d.Remediation {
	case types.RemediationStdinInject,
		types.RemediationRestartFlags,
		types.RemediationFallback,
		types.RemediationRetry:
		return true
	default:
		return false
	}
}

// IsHighConfidence returns true if the diagnosis confidence is above threshold.
func IsHighConfidence(d *types.Diagnosis, threshold float64) bool {
	return d.Confidence >= threshold
}

// FailureTypeDescription returns a human-readable description of a failure type.
func FailureTypeDescription(ft types.FailureType) string {
	descriptions := map[types.FailureType]string{
		types.FailureTypePermissionPrompt: "The process is waiting for user permission or confirmation",
		types.FailureTypeAuthError:        "Authentication or authorization failed",
		types.FailureTypeContextExceeded:  "The request exceeded the model's context window limit",
		types.FailureTypeRateLimit:        "The provider is rate limiting requests",
		types.FailureTypeNetworkError:     "A network connectivity issue occurred",
		types.FailureTypeProcessCrash:     "The process terminated unexpectedly",
		types.FailureTypeUnknown:          "The failure pattern was not recognized",
	}

	if desc, ok := descriptions[ft]; ok {
		return desc
	}
	return "Unknown failure type"
}

// RemediationDescription returns a human-readable description of a remediation action.
func RemediationDescription(rt types.RemediationType) string {
	descriptions := map[types.RemediationType]string{
		types.RemediationStdinInject:  "Inject a response into the process stdin",
		types.RemediationRestartFlags: "Restart the process with corrective flags",
		types.RemediationFallback:     "Route the request to an alternative provider",
		types.RemediationRetry:        "Retry the request after a brief delay",
		types.RemediationAbort:        "Abort the request and return an error",
	}

	if desc, ok := descriptions[rt]; ok {
		return desc
	}
	return "Unknown remediation action"
}

// FormatDiagnosis returns a human-readable summary of a diagnosis.
func FormatDiagnosis(d *types.Diagnosis) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Failure Type: %s\n", d.FailureType))
	sb.WriteString(fmt.Sprintf("Description: %s\n", FailureTypeDescription(d.FailureType)))
	sb.WriteString(fmt.Sprintf("Root Cause: %s\n", d.RootCause))
	sb.WriteString(fmt.Sprintf("Confidence: %.0f%%\n", d.Confidence*100))
	sb.WriteString(fmt.Sprintf("Recommended Action: %s\n", d.Remediation))
	sb.WriteString(fmt.Sprintf("Action Description: %s\n", RemediationDescription(d.Remediation)))

	if len(d.RemediationArgs) > 0 {
		sb.WriteString("Remediation Arguments:\n")
		for k, v := range d.RemediationArgs {
			sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	return sb.String()
}
