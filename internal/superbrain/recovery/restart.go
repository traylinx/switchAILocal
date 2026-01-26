// Package recovery provides process recovery and restart capabilities for the Superbrain system.
// It implements intelligent restart logic with corrective flags based on failure diagnosis.
package recovery

import (
	"fmt"
	"strings"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// CorrectiveFlags maps diagnosis types to corrective CLI flags for different providers.
// This enables the recovery system to restart processes with flags that address the root cause.
type CorrectiveFlags struct {
	// Provider is the CLI provider name (e.g., "claudecli", "geminicli").
	Provider string

	// Flags is a map from FailureType to the corrective flags to apply.
	Flags map[types.FailureType][]string
}

// DefaultCorrectiveFlags returns the default flag mappings for known CLI providers.
func DefaultCorrectiveFlags() map[string]*CorrectiveFlags {
	return map[string]*CorrectiveFlags{
		"claudecli": {
			Provider: "claudecli",
			Flags: map[types.FailureType][]string{
				types.FailureTypePermissionPrompt: {"--dangerously-skip-permissions"},
				types.FailureTypeAuthError:        {"--force-auth-refresh"},
			},
		},
		"geminicli": {
			Provider: "geminicli",
			Flags: map[types.FailureType][]string{
				types.FailureTypePermissionPrompt: {"--auto-approve"},
				types.FailureTypeAuthError:        {"--reauth"},
			},
		},
		"codexcli": {
			Provider: "codexcli",
			Flags: map[types.FailureType][]string{
				types.FailureTypePermissionPrompt: {"--yes"},
				types.FailureTypeAuthError:        {"--refresh-token"},
			},
		},
	}
}

// RestartStrategy defines how a process should be restarted based on diagnosis.
type RestartStrategy struct {
	// ShouldRestart indicates whether a restart should be attempted.
	ShouldRestart bool

	// CorrectiveFlags are the flags to add to the command line.
	CorrectiveFlags []string

	// Reason explains why this restart strategy was chosen.
	Reason string

	// FailureType is the diagnosed failure type that triggered this strategy.
	FailureType types.FailureType
}

// RestartManager handles process restart logic with corrective flags.
type RestartManager struct {
	// correctiveFlagsMap maps provider names to their corrective flags.
	correctiveFlagsMap map[string]*CorrectiveFlags
}

// NewRestartManager creates a new RestartManager with default corrective flags.
func NewRestartManager() *RestartManager {
	return &RestartManager{
		correctiveFlagsMap: DefaultCorrectiveFlags(),
	}
}

// NewRestartManagerWithFlags creates a RestartManager with custom corrective flags.
func NewRestartManagerWithFlags(flagsMap map[string]*CorrectiveFlags) *RestartManager {
	return &RestartManager{
		correctiveFlagsMap: flagsMap,
	}
}

// AddCorrectiveFlags adds or updates corrective flags for a provider.
func (rm *RestartManager) AddCorrectiveFlags(provider string, flags *CorrectiveFlags) {
	rm.correctiveFlagsMap[provider] = flags
}

// GetRestartStrategy determines the restart strategy based on diagnosis and provider.
// Returns a RestartStrategy indicating whether to restart and what flags to apply.
func (rm *RestartManager) GetRestartStrategy(diagnosis *types.Diagnosis, provider string) *RestartStrategy {
	strategy := &RestartStrategy{
		ShouldRestart:   false,
		CorrectiveFlags: []string{},
		FailureType:     diagnosis.FailureType,
	}

	// Check if the diagnosis recommends restart
	if diagnosis.Remediation != types.RemediationRestartFlags {
		strategy.Reason = fmt.Sprintf("Diagnosis recommends %s, not restart", diagnosis.Remediation)
		return strategy
	}

	// Check if we have corrective flags for this provider
	providerFlags, ok := rm.correctiveFlagsMap[provider]
	if !ok {
		strategy.Reason = fmt.Sprintf("No corrective flags defined for provider %s", provider)
		return strategy
	}

	// Get the corrective flags for this failure type
	flags, ok := providerFlags.Flags[diagnosis.FailureType]
	if !ok || len(flags) == 0 {
		strategy.Reason = fmt.Sprintf("No corrective flags defined for failure type %s on provider %s", 
			diagnosis.FailureType, provider)
		return strategy
	}

	// Check if diagnosis provides specific flags in RemediationArgs
	if flagsArg, ok := diagnosis.RemediationArgs["flags"]; ok && flagsArg != "" {
		// Use flags from diagnosis if provided
		strategy.CorrectiveFlags = strings.Fields(flagsArg)
		strategy.Reason = fmt.Sprintf("Using flags from diagnosis: %s", flagsArg)
	} else {
		// Use default flags for this failure type
		strategy.CorrectiveFlags = flags
		strategy.Reason = fmt.Sprintf("Using default corrective flags for %s", diagnosis.FailureType)
	}

	strategy.ShouldRestart = true
	return strategy
}

// BuildRestartCommand constructs a new command line with corrective flags applied.
// It ensures flags are not duplicated if they already exist in the original command.
func (rm *RestartManager) BuildRestartCommand(originalArgs []string, correctiveFlags []string) []string {
	// Create a set of existing flags for deduplication
	existingFlags := make(map[string]bool)
	for _, arg := range originalArgs {
		if strings.HasPrefix(arg, "-") {
			existingFlags[arg] = true
		}
	}

	// Build new args list
	newArgs := make([]string, 0, len(originalArgs)+len(correctiveFlags))
	
	// Copy original args
	newArgs = append(newArgs, originalArgs...)

	// Add corrective flags if not already present
	for _, flag := range correctiveFlags {
		if !existingFlags[flag] {
			newArgs = append(newArgs, flag)
		}
	}

	return newArgs
}

// FormatRestartAction creates a HealingAction for a restart attempt.
func FormatRestartAction(strategy *RestartStrategy, success bool, details map[string]interface{}) types.HealingAction {
	description := fmt.Sprintf("Restarted process with corrective flags: %s", 
		strings.Join(strategy.CorrectiveFlags, " "))
	
	if !success {
		description = fmt.Sprintf("Attempted restart with corrective flags: %s (failed)", 
			strings.Join(strategy.CorrectiveFlags, " "))
	}

	if details == nil {
		details = make(map[string]interface{})
	}
	details["corrective_flags"] = strategy.CorrectiveFlags
	details["failure_type"] = string(strategy.FailureType)
	details["reason"] = strategy.Reason

	return types.HealingAction{
		ActionType:  "restart_with_flags",
		Description: description,
		Success:     success,
		Details:     details,
	}
}

// ShouldAttemptRestart checks if a restart should be attempted based on diagnosis and context.
// This is a convenience function that combines diagnosis check with restart count validation.
func ShouldAttemptRestart(diagnosis *types.Diagnosis, restartCount, maxRestarts int) bool {
	// Check if we've exceeded max restart attempts
	if restartCount >= maxRestarts {
		return false
	}

	// Check if diagnosis recommends restart
	if diagnosis.Remediation != types.RemediationRestartFlags {
		return false
	}

	// Check if this is a recoverable failure type
	switch diagnosis.FailureType {
	case types.FailureTypePermissionPrompt,
		types.FailureTypeAuthError:
		return true
	default:
		return false
	}
}

// GetFlagsForProvider returns the corrective flags configuration for a provider.
func (rm *RestartManager) GetFlagsForProvider(provider string) (*CorrectiveFlags, bool) {
	flags, ok := rm.correctiveFlagsMap[provider]
	return flags, ok
}

// HasFlagsForFailureType checks if corrective flags exist for a specific failure type and provider.
func (rm *RestartManager) HasFlagsForFailureType(provider string, failureType types.FailureType) bool {
	providerFlags, ok := rm.correctiveFlagsMap[provider]
	if !ok {
		return false
	}

	flags, ok := providerFlags.Flags[failureType]
	return ok && len(flags) > 0
}

// RestartDecision encapsulates the decision about whether to restart or escalate.
type RestartDecision struct {
	// ShouldRestart indicates whether a restart should be attempted.
	ShouldRestart bool

	// ShouldEscalate indicates whether to escalate to fallback routing.
	ShouldEscalate bool

	// Reason explains the decision.
	Reason string

	// Strategy contains the restart strategy if ShouldRestart is true.
	Strategy *RestartStrategy
}

// DecideRestartOrEscalate determines whether to restart or escalate based on restart count and diagnosis.
// This enforces the restart limit and escalates to fallback when the limit is reached.
func (rm *RestartManager) DecideRestartOrEscalate(
	diagnosis *types.Diagnosis,
	provider string,
	restartCount int,
	maxRestarts int,
) *RestartDecision {
	decision := &RestartDecision{
		ShouldRestart:  false,
		ShouldEscalate: false,
	}

	// Check if restart limit has been reached
	if restartCount >= maxRestarts {
		decision.ShouldEscalate = true
		decision.Reason = fmt.Sprintf("Restart limit reached (%d/%d), escalating to fallback", 
			restartCount, maxRestarts)
		return decision
	}

	// Get restart strategy
	strategy := rm.GetRestartStrategy(diagnosis, provider)
	decision.Strategy = strategy

	if !strategy.ShouldRestart {
		// Cannot restart, should escalate if this is a failure
		decision.ShouldEscalate = true
		decision.Reason = fmt.Sprintf("Cannot restart: %s. Escalating to fallback", strategy.Reason)
		return decision
	}

	// Can restart
	decision.ShouldRestart = true
	decision.Reason = fmt.Sprintf("Restart attempt %d/%d with corrective flags", 
		restartCount+1, maxRestarts)
	return decision
}

// CheckRestartLimit verifies if another restart attempt is allowed.
// Returns true if restart count is below the maximum, false otherwise.
func CheckRestartLimit(restartCount, maxRestarts int) bool {
	return restartCount < maxRestarts
}

// FormatEscalationAction creates a HealingAction for escalation to fallback.
func FormatEscalationAction(reason string, restartCount, maxRestarts int) types.HealingAction {
	description := fmt.Sprintf("Escalating to fallback after %d restart attempts (max: %d)", 
		restartCount, maxRestarts)

	return types.HealingAction{
		ActionType:  "escalate_to_fallback",
		Description: description,
		Success:     true, // Escalation itself is successful, even if restarts failed
		Details: map[string]interface{}{
			"restart_count": restartCount,
			"max_restarts":  maxRestarts,
			"reason":        reason,
		},
	}
}

// RestartAttempt represents a single restart attempt with its outcome.
type RestartAttempt struct {
	// AttemptNumber is the sequential number of this restart attempt (1-based).
	AttemptNumber int

	// Strategy is the restart strategy that was applied.
	Strategy *RestartStrategy

	// Success indicates whether the restart succeeded.
	Success bool

	// ErrorMessage contains the error message if the restart failed.
	ErrorMessage string

	// Duration is how long the restart took.
	Duration int64 // milliseconds
}

// RestartHistory tracks all restart attempts for a single execution.
type RestartHistory struct {
	// Attempts is the list of all restart attempts.
	Attempts []RestartAttempt

	// TotalRestarts is the total number of restart attempts.
	TotalRestarts int

	// SuccessfulRestarts is the number of successful restarts.
	SuccessfulRestarts int

	// FinalOutcome indicates the final result ("success", "escalated", "failed").
	FinalOutcome string
}

// NewRestartHistory creates a new RestartHistory.
func NewRestartHistory() *RestartHistory {
	return &RestartHistory{
		Attempts:           make([]RestartAttempt, 0),
		TotalRestarts:      0,
		SuccessfulRestarts: 0,
		FinalOutcome:       "pending",
	}
}

// RecordAttempt adds a restart attempt to the history.
func (rh *RestartHistory) RecordAttempt(attempt RestartAttempt) {
	rh.Attempts = append(rh.Attempts, attempt)
	rh.TotalRestarts++
	if attempt.Success {
		rh.SuccessfulRestarts++
	}
}

// SetFinalOutcome sets the final outcome of the restart sequence.
func (rh *RestartHistory) SetFinalOutcome(outcome string) {
	rh.FinalOutcome = outcome
}

// ToHealingActions converts the restart history to a list of HealingActions.
// This is used to include all restart attempts in the HealingMetadata.
func (rh *RestartHistory) ToHealingActions() []types.HealingAction {
	actions := make([]types.HealingAction, 0, len(rh.Attempts))

	for _, attempt := range rh.Attempts {
		action := types.HealingAction{
			ActionType:  "restart_with_flags",
			Description: fmt.Sprintf("Restart attempt %d/%d with flags: %s", 
				attempt.AttemptNumber, rh.TotalRestarts, 
				strings.Join(attempt.Strategy.CorrectiveFlags, " ")),
			Success: attempt.Success,
			Details: map[string]interface{}{
				"attempt_number":   attempt.AttemptNumber,
				"corrective_flags": attempt.Strategy.CorrectiveFlags,
				"failure_type":     string(attempt.Strategy.FailureType),
				"duration_ms":      attempt.Duration,
			},
		}

		if !attempt.Success && attempt.ErrorMessage != "" {
			action.Details["error"] = attempt.ErrorMessage
		}

		actions = append(actions, action)
	}

	return actions
}

// GetSummary returns a human-readable summary of the restart history.
func (rh *RestartHistory) GetSummary() string {
	if rh.TotalRestarts == 0 {
		return "No restart attempts"
	}

	return fmt.Sprintf("%d restart attempt(s), %d successful, final outcome: %s",
		rh.TotalRestarts, rh.SuccessfulRestarts, rh.FinalOutcome)
}

// RecordSuccessfulRestart creates a HealingAction for a successful restart with corrective flags.
// This is a convenience function that includes all relevant metadata about the successful restart.
func RecordSuccessfulRestart(strategy *RestartStrategy, attemptNumber int, durationMs int64) types.HealingAction {
	description := fmt.Sprintf("Successfully restarted process (attempt %d) with corrective flags: %s", 
		attemptNumber, strings.Join(strategy.CorrectiveFlags, " "))

	return types.HealingAction{
		ActionType:  "restart_with_flags",
		Description: description,
		Success:     true,
		Details: map[string]interface{}{
			"attempt_number":   attemptNumber,
			"corrective_flags": strategy.CorrectiveFlags,
			"failure_type":     string(strategy.FailureType),
			"reason":           strategy.Reason,
			"duration_ms":      durationMs,
		},
	}
}

// RecordFailedRestart creates a HealingAction for a failed restart attempt.
func RecordFailedRestart(strategy *RestartStrategy, attemptNumber int, errorMsg string, durationMs int64) types.HealingAction {
	description := fmt.Sprintf("Restart attempt %d failed with corrective flags: %s", 
		attemptNumber, strings.Join(strategy.CorrectiveFlags, " "))

	return types.HealingAction{
		ActionType:  "restart_with_flags",
		Description: description,
		Success:     false,
		Details: map[string]interface{}{
			"attempt_number":   attemptNumber,
			"corrective_flags": strategy.CorrectiveFlags,
			"failure_type":     string(strategy.FailureType),
			"reason":           strategy.Reason,
			"error":            errorMsg,
			"duration_ms":      durationMs,
		},
	}
}
