// Package superbrain provides intelligent orchestration and self-healing capabilities
// for the switchAILocal gateway. It transforms the gateway from a passive proxy into
// an autonomous system that can detect failures, diagnose issues, and take remediation actions.
package superbrain

import (
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// Re-export types from the types subpackage for backward compatibility.
// This allows existing code to continue using superbrain.Diagnosis, etc.

// Type aliases for backward compatibility
type HealingAction = types.HealingAction
type HealingMetadata = types.HealingMetadata
type DiagnosticSnapshot = types.DiagnosticSnapshot
type FailureType = types.FailureType
type RemediationType = types.RemediationType
type Diagnosis = types.Diagnosis
type HighDensityMap = types.HighDensityMap
type NegotiatedFailureResponse = types.NegotiatedFailureResponse

// Re-export constants for backward compatibility
const (
	FailureTypePermissionPrompt = types.FailureTypePermissionPrompt
	FailureTypeAuthError        = types.FailureTypeAuthError
	FailureTypeContextExceeded  = types.FailureTypeContextExceeded
	FailureTypeRateLimit        = types.FailureTypeRateLimit
	FailureTypeNetworkError     = types.FailureTypeNetworkError
	FailureTypeProcessCrash     = types.FailureTypeProcessCrash
	FailureTypeUnknown          = types.FailureTypeUnknown

	RemediationStdinInject  = types.RemediationStdinInject
	RemediationRestartFlags = types.RemediationRestartFlags
	RemediationFallback     = types.RemediationFallback
	RemediationRetry        = types.RemediationRetry
	RemediationAbort        = types.RemediationAbort
)
