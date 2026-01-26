package recovery

import (
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// TestNewRestartManager verifies that a new RestartManager is created with default flags.
func TestNewRestartManager(t *testing.T) {
	rm := NewRestartManager()

	if rm == nil {
		t.Fatal("NewRestartManager returned nil")
	}

	if rm.correctiveFlagsMap == nil {
		t.Fatal("correctiveFlagsMap is nil")
	}

	// Verify default providers are present
	expectedProviders := []string{"claudecli", "geminicli", "codexcli"}
	for _, provider := range expectedProviders {
		if _, ok := rm.correctiveFlagsMap[provider]; !ok {
			t.Errorf("Expected provider %s not found in default flags", provider)
		}
	}
}

// TestGetRestartStrategy_PermissionPrompt tests restart strategy for permission prompt failures.
func TestGetRestartStrategy_PermissionPrompt(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationRestartFlags,
		RootCause:   "Process waiting for permission",
	}

	strategy := rm.GetRestartStrategy(diagnosis, "claudecli")

	if !strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be true for permission prompt")
	}

	if len(strategy.CorrectiveFlags) == 0 {
		t.Error("Expected corrective flags to be set")
	}

	// Verify the correct flag is present
	expectedFlag := "--dangerously-skip-permissions"
	found := false
	for _, flag := range strategy.CorrectiveFlags {
		if flag == expectedFlag {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected flag %s not found in corrective flags: %v", expectedFlag, strategy.CorrectiveFlags)
	}

	if strategy.FailureType != types.FailureTypePermissionPrompt {
		t.Errorf("Expected FailureType %s, got %s", types.FailureTypePermissionPrompt, strategy.FailureType)
	}
}

// TestGetRestartStrategy_AuthError tests restart strategy for auth error failures.
func TestGetRestartStrategy_AuthError(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeAuthError,
		Remediation: types.RemediationRestartFlags,
		RootCause:   "Authentication failed",
	}

	strategy := rm.GetRestartStrategy(diagnosis, "geminicli")

	if !strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be true for auth error")
	}

	if len(strategy.CorrectiveFlags) == 0 {
		t.Error("Expected corrective flags to be set")
	}

	// Verify the correct flag is present
	expectedFlag := "--reauth"
	found := false
	for _, flag := range strategy.CorrectiveFlags {
		if flag == expectedFlag {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected flag %s not found in corrective flags: %v", expectedFlag, strategy.CorrectiveFlags)
	}
}

// TestGetRestartStrategy_WrongRemediation tests that restart is not attempted for non-restart remediations.
func TestGetRestartStrategy_WrongRemediation(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationStdinInject, // Not restart
		RootCause:   "Process waiting for permission",
	}

	strategy := rm.GetRestartStrategy(diagnosis, "claudecli")

	if strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be false when remediation is not restart")
	}

	if len(strategy.CorrectiveFlags) != 0 {
		t.Error("Expected no corrective flags when restart is not recommended")
	}
}

// TestGetRestartStrategy_UnknownProvider tests behavior with an unknown provider.
func TestGetRestartStrategy_UnknownProvider(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationRestartFlags,
		RootCause:   "Process waiting for permission",
	}

	strategy := rm.GetRestartStrategy(diagnosis, "unknowncli")

	if strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be false for unknown provider")
	}

	if len(strategy.CorrectiveFlags) != 0 {
		t.Error("Expected no corrective flags for unknown provider")
	}
}

// TestGetRestartStrategy_UnsupportedFailureType tests behavior with unsupported failure type.
func TestGetRestartStrategy_UnsupportedFailureType(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeNetworkError, // Not supported for restart
		Remediation: types.RemediationRestartFlags,
		RootCause:   "Network connection failed",
	}

	strategy := rm.GetRestartStrategy(diagnosis, "claudecli")

	if strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be false for unsupported failure type")
	}
}

// TestGetRestartStrategy_CustomFlags tests using custom flags from diagnosis.
func TestGetRestartStrategy_CustomFlags(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationRestartFlags,
		RootCause:   "Process waiting for permission",
		RemediationArgs: map[string]string{
			"flags": "--custom-flag --another-flag",
		},
	}

	strategy := rm.GetRestartStrategy(diagnosis, "claudecli")

	if !strategy.ShouldRestart {
		t.Error("Expected ShouldRestart to be true")
	}

	// Verify custom flags are used
	if len(strategy.CorrectiveFlags) != 2 {
		t.Errorf("Expected 2 custom flags, got %d", len(strategy.CorrectiveFlags))
	}

	expectedFlags := []string{"--custom-flag", "--another-flag"}
	for i, expected := range expectedFlags {
		if i >= len(strategy.CorrectiveFlags) || strategy.CorrectiveFlags[i] != expected {
			t.Errorf("Expected flag %s at position %d, got %v", expected, i, strategy.CorrectiveFlags)
		}
	}
}

// TestBuildRestartCommand tests command line construction with corrective flags.
func TestBuildRestartCommand(t *testing.T) {
	rm := NewRestartManager()

	tests := []struct {
		name            string
		originalArgs    []string
		correctiveFlags []string
		expectedArgs    []string
	}{
		{
			name:            "Add new flags",
			originalArgs:    []string{"claude", "chat", "--model", "claude-3"},
			correctiveFlags: []string{"--dangerously-skip-permissions"},
			expectedArgs:    []string{"claude", "chat", "--model", "claude-3", "--dangerously-skip-permissions"},
		},
		{
			name:            "Avoid duplicate flags",
			originalArgs:    []string{"claude", "chat", "--dangerously-skip-permissions"},
			correctiveFlags: []string{"--dangerously-skip-permissions"},
			expectedArgs:    []string{"claude", "chat", "--dangerously-skip-permissions"},
		},
		{
			name:            "Add multiple flags",
			originalArgs:    []string{"gemini", "chat"},
			correctiveFlags: []string{"--auto-approve", "--verbose"},
			expectedArgs:    []string{"gemini", "chat", "--auto-approve", "--verbose"},
		},
		{
			name:            "Empty corrective flags",
			originalArgs:    []string{"claude", "chat"},
			correctiveFlags: []string{},
			expectedArgs:    []string{"claude", "chat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rm.BuildRestartCommand(tt.originalArgs, tt.correctiveFlags)

			if len(result) != len(tt.expectedArgs) {
				t.Errorf("Expected %d args, got %d: %v", len(tt.expectedArgs), len(result), result)
				return
			}

			for i, expected := range tt.expectedArgs {
				if result[i] != expected {
					t.Errorf("At position %d: expected %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}

// TestShouldAttemptRestart tests the restart decision logic.
func TestShouldAttemptRestart(t *testing.T) {
	tests := []struct {
		name         string
		diagnosis    *types.Diagnosis
		restartCount int
		maxRestarts  int
		expected     bool
	}{
		{
			name: "Should restart - permission prompt, first attempt",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			restartCount: 0,
			maxRestarts:  2,
			expected:     true,
		},
		{
			name: "Should restart - auth error, first attempt",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypeAuthError,
				Remediation: types.RemediationRestartFlags,
			},
			restartCount: 0,
			maxRestarts:  2,
			expected:     true,
		},
		{
			name: "Should not restart - max attempts reached",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			restartCount: 2,
			maxRestarts:  2,
			expected:     false,
		},
		{
			name: "Should not restart - wrong remediation",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationStdinInject,
			},
			restartCount: 0,
			maxRestarts:  2,
			expected:     false,
		},
		{
			name: "Should not restart - unrecoverable failure type",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypeNetworkError,
				Remediation: types.RemediationRestartFlags,
			},
			restartCount: 0,
			maxRestarts:  2,
			expected:     false,
		},
		{
			name: "Should restart - at limit but not exceeded",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			restartCount: 1,
			maxRestarts:  2,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldAttemptRestart(tt.diagnosis, tt.restartCount, tt.maxRestarts)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestFormatRestartAction tests healing action formatting.
func TestFormatRestartAction(t *testing.T) {
	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--dangerously-skip-permissions"},
		Reason:          "Permission prompt detected",
		FailureType:     types.FailureTypePermissionPrompt,
	}

	// Test successful restart
	action := FormatRestartAction(strategy, true, nil)

	if action.ActionType != "restart_with_flags" {
		t.Errorf("Expected ActionType 'restart_with_flags', got %s", action.ActionType)
	}

	if !action.Success {
		t.Error("Expected Success to be true")
	}

	if action.Description == "" {
		t.Error("Expected non-empty Description")
	}

	if action.Details == nil {
		t.Fatal("Expected Details to be non-nil")
	}

	// Verify details contain expected fields
	if _, ok := action.Details["corrective_flags"]; !ok {
		t.Error("Expected 'corrective_flags' in Details")
	}

	if _, ok := action.Details["failure_type"]; !ok {
		t.Error("Expected 'failure_type' in Details")
	}

	if _, ok := action.Details["reason"]; !ok {
		t.Error("Expected 'reason' in Details")
	}

	// Test failed restart
	failedAction := FormatRestartAction(strategy, false, nil)
	if failedAction.Success {
		t.Error("Expected Success to be false for failed restart")
	}
}

// TestAddCorrectiveFlags tests adding custom corrective flags.
func TestAddCorrectiveFlags(t *testing.T) {
	rm := NewRestartManager()

	customFlags := &CorrectiveFlags{
		Provider: "customcli",
		Flags: map[types.FailureType][]string{
			types.FailureTypePermissionPrompt: {"--skip-all"},
		},
	}

	rm.AddCorrectiveFlags("customcli", customFlags)

	// Verify the flags were added
	flags, ok := rm.GetFlagsForProvider("customcli")
	if !ok {
		t.Fatal("Expected customcli flags to be present")
	}

	if flags.Provider != "customcli" {
		t.Errorf("Expected provider 'customcli', got %s", flags.Provider)
	}

	if len(flags.Flags) != 1 {
		t.Errorf("Expected 1 failure type mapping, got %d", len(flags.Flags))
	}
}

// TestHasFlagsForFailureType tests checking for specific failure type flags.
func TestHasFlagsForFailureType(t *testing.T) {
	rm := NewRestartManager()

	tests := []struct {
		name        string
		provider    string
		failureType types.FailureType
		expected    bool
	}{
		{
			name:        "Has flags - claudecli permission prompt",
			provider:    "claudecli",
			failureType: types.FailureTypePermissionPrompt,
			expected:    true,
		},
		{
			name:        "Has flags - geminicli auth error",
			provider:    "geminicli",
			failureType: types.FailureTypeAuthError,
			expected:    true,
		},
		{
			name:        "No flags - unknown provider",
			provider:    "unknowncli",
			failureType: types.FailureTypePermissionPrompt,
			expected:    false,
		},
		{
			name:        "No flags - unsupported failure type",
			provider:    "claudecli",
			failureType: types.FailureTypeNetworkError,
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rm.HasFlagsForFailureType(tt.provider, tt.failureType)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestNewRestartManagerWithFlags tests creating a manager with custom flags.
func TestNewRestartManagerWithFlags(t *testing.T) {
	customFlagsMap := map[string]*CorrectiveFlags{
		"testcli": {
			Provider: "testcli",
			Flags: map[types.FailureType][]string{
				types.FailureTypePermissionPrompt: {"--test-flag"},
			},
		},
	}

	rm := NewRestartManagerWithFlags(customFlagsMap)

	if rm == nil {
		t.Fatal("NewRestartManagerWithFlags returned nil")
	}

	// Verify custom flags are present
	flags, ok := rm.GetFlagsForProvider("testcli")
	if !ok {
		t.Fatal("Expected testcli flags to be present")
	}

	if flags.Provider != "testcli" {
		t.Errorf("Expected provider 'testcli', got %s", flags.Provider)
	}

	// Verify default flags are NOT present
	_, ok = rm.GetFlagsForProvider("claudecli")
	if ok {
		t.Error("Expected default claudecli flags to NOT be present when using custom flags")
	}
}

// TestDecideRestartOrEscalate tests the restart vs escalation decision logic.
func TestDecideRestartOrEscalate(t *testing.T) {
	rm := NewRestartManager()

	tests := []struct {
		name              string
		diagnosis         *types.Diagnosis
		provider          string
		restartCount      int
		maxRestarts       int
		expectRestart     bool
		expectEscalate    bool
		reasonContains    string
	}{
		{
			name: "First restart attempt - should restart",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			provider:       "claudecli",
			restartCount:   0,
			maxRestarts:    2,
			expectRestart:  true,
			expectEscalate: false,
			reasonContains: "Restart attempt 1/2",
		},
		{
			name: "Second restart attempt - should restart",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			provider:       "claudecli",
			restartCount:   1,
			maxRestarts:    2,
			expectRestart:  true,
			expectEscalate: false,
			reasonContains: "Restart attempt 2/2",
		},
		{
			name: "Restart limit reached - should escalate",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			provider:       "claudecli",
			restartCount:   2,
			maxRestarts:    2,
			expectRestart:  false,
			expectEscalate: true,
			reasonContains: "Restart limit reached",
		},
		{
			name: "Restart limit exceeded - should escalate",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			provider:       "claudecli",
			restartCount:   3,
			maxRestarts:    2,
			expectRestart:  false,
			expectEscalate: true,
			reasonContains: "Restart limit reached",
		},
		{
			name: "Cannot restart - wrong remediation - should escalate",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationStdinInject,
			},
			provider:       "claudecli",
			restartCount:   0,
			maxRestarts:    2,
			expectRestart:  false,
			expectEscalate: true,
			reasonContains: "Cannot restart",
		},
		{
			name: "Cannot restart - unknown provider - should escalate",
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				Remediation: types.RemediationRestartFlags,
			},
			provider:       "unknowncli",
			restartCount:   0,
			maxRestarts:    2,
			expectRestart:  false,
			expectEscalate: true,
			reasonContains: "Cannot restart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := rm.DecideRestartOrEscalate(tt.diagnosis, tt.provider, tt.restartCount, tt.maxRestarts)

			if decision.ShouldRestart != tt.expectRestart {
				t.Errorf("Expected ShouldRestart=%v, got %v", tt.expectRestart, decision.ShouldRestart)
			}

			if decision.ShouldEscalate != tt.expectEscalate {
				t.Errorf("Expected ShouldEscalate=%v, got %v", tt.expectEscalate, decision.ShouldEscalate)
			}

			if !strings.Contains(decision.Reason, tt.reasonContains) {
				t.Errorf("Expected reason to contain '%s', got: %s", tt.reasonContains, decision.Reason)
			}

			// If should restart, verify strategy is present
			if decision.ShouldRestart && decision.Strategy == nil {
				t.Error("Expected Strategy to be non-nil when ShouldRestart is true")
			}
		})
	}
}

// TestCheckRestartLimit tests the restart limit check function.
func TestCheckRestartLimit(t *testing.T) {
	tests := []struct {
		name         string
		restartCount int
		maxRestarts  int
		expected     bool
	}{
		{
			name:         "Below limit",
			restartCount: 0,
			maxRestarts:  2,
			expected:     true,
		},
		{
			name:         "At limit",
			restartCount: 2,
			maxRestarts:  2,
			expected:     false,
		},
		{
			name:         "Above limit",
			restartCount: 3,
			maxRestarts:  2,
			expected:     false,
		},
		{
			name:         "One below limit",
			restartCount: 1,
			maxRestarts:  2,
			expected:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckRestartLimit(tt.restartCount, tt.maxRestarts)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestFormatEscalationAction tests escalation action formatting.
func TestFormatEscalationAction(t *testing.T) {
	action := FormatEscalationAction("Restart limit reached", 2, 2)

	if action.ActionType != "escalate_to_fallback" {
		t.Errorf("Expected ActionType 'escalate_to_fallback', got %s", action.ActionType)
	}

	if !action.Success {
		t.Error("Expected Success to be true for escalation")
	}

	if action.Description == "" {
		t.Error("Expected non-empty Description")
	}

	if !strings.Contains(action.Description, "2 restart attempts") {
		t.Errorf("Expected description to mention restart count, got: %s", action.Description)
	}

	if action.Details == nil {
		t.Fatal("Expected Details to be non-nil")
	}

	// Verify details contain expected fields
	if restartCount, ok := action.Details["restart_count"].(int); !ok || restartCount != 2 {
		t.Errorf("Expected restart_count=2 in Details, got %v", action.Details["restart_count"])
	}

	if maxRestarts, ok := action.Details["max_restarts"].(int); !ok || maxRestarts != 2 {
		t.Errorf("Expected max_restarts=2 in Details, got %v", action.Details["max_restarts"])
	}

	if reason, ok := action.Details["reason"].(string); !ok || reason == "" {
		t.Errorf("Expected non-empty reason in Details, got %v", action.Details["reason"])
	}
}

// TestDecideRestartOrEscalate_StrategyPresence tests that strategy is included when restarting.
func TestDecideRestartOrEscalate_StrategyPresence(t *testing.T) {
	rm := NewRestartManager()

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationRestartFlags,
	}

	decision := rm.DecideRestartOrEscalate(diagnosis, "claudecli", 0, 2)

	if !decision.ShouldRestart {
		t.Fatal("Expected ShouldRestart to be true")
	}

	if decision.Strategy == nil {
		t.Fatal("Expected Strategy to be non-nil when restarting")
	}

	if !decision.Strategy.ShouldRestart {
		t.Error("Expected Strategy.ShouldRestart to be true")
	}

	if len(decision.Strategy.CorrectiveFlags) == 0 {
		t.Error("Expected Strategy to have corrective flags")
	}
}

// TestNewRestartHistory tests creating a new restart history.
func TestNewRestartHistory(t *testing.T) {
	history := NewRestartHistory()

	if history == nil {
		t.Fatal("NewRestartHistory returned nil")
	}

	if history.TotalRestarts != 0 {
		t.Errorf("Expected TotalRestarts=0, got %d", history.TotalRestarts)
	}

	if history.SuccessfulRestarts != 0 {
		t.Errorf("Expected SuccessfulRestarts=0, got %d", history.SuccessfulRestarts)
	}

	if history.FinalOutcome != "pending" {
		t.Errorf("Expected FinalOutcome='pending', got %s", history.FinalOutcome)
	}

	if len(history.Attempts) != 0 {
		t.Errorf("Expected empty Attempts, got %d", len(history.Attempts))
	}
}

// TestRestartHistory_RecordAttempt tests recording restart attempts.
func TestRestartHistory_RecordAttempt(t *testing.T) {
	history := NewRestartHistory()

	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--flag1"},
		FailureType:     types.FailureTypePermissionPrompt,
	}

	// Record successful attempt
	attempt1 := RestartAttempt{
		AttemptNumber: 1,
		Strategy:      strategy,
		Success:       true,
		Duration:      100,
	}
	history.RecordAttempt(attempt1)

	if history.TotalRestarts != 1 {
		t.Errorf("Expected TotalRestarts=1, got %d", history.TotalRestarts)
	}

	if history.SuccessfulRestarts != 1 {
		t.Errorf("Expected SuccessfulRestarts=1, got %d", history.SuccessfulRestarts)
	}

	// Record failed attempt
	attempt2 := RestartAttempt{
		AttemptNumber: 2,
		Strategy:      strategy,
		Success:       false,
		ErrorMessage:  "Process crashed",
		Duration:      50,
	}
	history.RecordAttempt(attempt2)

	if history.TotalRestarts != 2 {
		t.Errorf("Expected TotalRestarts=2, got %d", history.TotalRestarts)
	}

	if history.SuccessfulRestarts != 1 {
		t.Errorf("Expected SuccessfulRestarts=1 (unchanged), got %d", history.SuccessfulRestarts)
	}

	if len(history.Attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", len(history.Attempts))
	}
}

// TestRestartHistory_ToHealingActions tests converting history to healing actions.
func TestRestartHistory_ToHealingActions(t *testing.T) {
	history := NewRestartHistory()

	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--flag1", "--flag2"},
		FailureType:     types.FailureTypePermissionPrompt,
		Reason:          "Test reason",
	}

	// Add successful attempt
	history.RecordAttempt(RestartAttempt{
		AttemptNumber: 1,
		Strategy:      strategy,
		Success:       true,
		Duration:      100,
	})

	// Add failed attempt
	history.RecordAttempt(RestartAttempt{
		AttemptNumber: 2,
		Strategy:      strategy,
		Success:       false,
		ErrorMessage:  "Test error",
		Duration:      50,
	})

	actions := history.ToHealingActions()

	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}

	// Check first action (successful)
	if actions[0].ActionType != "restart_with_flags" {
		t.Errorf("Expected ActionType 'restart_with_flags', got %s", actions[0].ActionType)
	}

	if !actions[0].Success {
		t.Error("Expected first action to be successful")
	}

	if actions[0].Details == nil {
		t.Fatal("Expected Details to be non-nil")
	}

	if attemptNum, ok := actions[0].Details["attempt_number"].(int); !ok || attemptNum != 1 {
		t.Errorf("Expected attempt_number=1, got %v", actions[0].Details["attempt_number"])
	}

	// Check second action (failed)
	if actions[1].Success {
		t.Error("Expected second action to be failed")
	}

	if errorMsg, ok := actions[1].Details["error"].(string); !ok || errorMsg != "Test error" {
		t.Errorf("Expected error='Test error', got %v", actions[1].Details["error"])
	}
}

// TestRestartHistory_GetSummary tests summary generation.
func TestRestartHistory_GetSummary(t *testing.T) {
	tests := []struct {
		name             string
		setupHistory     func() *RestartHistory
		expectedContains []string
	}{
		{
			name: "No attempts",
			setupHistory: func() *RestartHistory {
				return NewRestartHistory()
			},
			expectedContains: []string{"No restart attempts"},
		},
		{
			name: "One successful attempt",
			setupHistory: func() *RestartHistory {
				h := NewRestartHistory()
				h.RecordAttempt(RestartAttempt{
					AttemptNumber: 1,
					Success:       true,
				})
				h.SetFinalOutcome("success")
				return h
			},
			expectedContains: []string{"1 restart attempt", "1 successful", "success"},
		},
		{
			name: "Multiple attempts with failures",
			setupHistory: func() *RestartHistory {
				h := NewRestartHistory()
				h.RecordAttempt(RestartAttempt{AttemptNumber: 1, Success: false})
				h.RecordAttempt(RestartAttempt{AttemptNumber: 2, Success: true})
				h.SetFinalOutcome("escalated")
				return h
			},
			expectedContains: []string{"2 restart attempt", "1 successful", "escalated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := tt.setupHistory()
			summary := history.GetSummary()

			for _, expected := range tt.expectedContains {
				if !strings.Contains(summary, expected) {
					t.Errorf("Expected summary to contain '%s', got: %s", expected, summary)
				}
			}
		})
	}
}

// TestRecordSuccessfulRestart tests recording a successful restart.
func TestRecordSuccessfulRestart(t *testing.T) {
	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--flag1"},
		FailureType:     types.FailureTypePermissionPrompt,
		Reason:          "Test reason",
	}

	action := RecordSuccessfulRestart(strategy, 1, 150)

	if action.ActionType != "restart_with_flags" {
		t.Errorf("Expected ActionType 'restart_with_flags', got %s", action.ActionType)
	}

	if !action.Success {
		t.Error("Expected Success to be true")
	}

	if action.Description == "" {
		t.Error("Expected non-empty Description")
	}

	if !strings.Contains(action.Description, "Successfully restarted") {
		t.Errorf("Expected description to mention success, got: %s", action.Description)
	}

	if action.Details == nil {
		t.Fatal("Expected Details to be non-nil")
	}

	// Verify all expected fields are present
	expectedFields := []string{"attempt_number", "corrective_flags", "failure_type", "reason", "duration_ms"}
	for _, field := range expectedFields {
		if _, ok := action.Details[field]; !ok {
			t.Errorf("Expected field '%s' in Details", field)
		}
	}

	// Verify specific values
	if attemptNum, ok := action.Details["attempt_number"].(int); !ok || attemptNum != 1 {
		t.Errorf("Expected attempt_number=1, got %v", action.Details["attempt_number"])
	}

	if duration, ok := action.Details["duration_ms"].(int64); !ok || duration != 150 {
		t.Errorf("Expected duration_ms=150, got %v", action.Details["duration_ms"])
	}
}

// TestRecordFailedRestart tests recording a failed restart.
func TestRecordFailedRestart(t *testing.T) {
	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--flag1"},
		FailureType:     types.FailureTypeAuthError,
		Reason:          "Test reason",
	}

	action := RecordFailedRestart(strategy, 2, "Connection timeout", 200)

	if action.ActionType != "restart_with_flags" {
		t.Errorf("Expected ActionType 'restart_with_flags', got %s", action.ActionType)
	}

	if action.Success {
		t.Error("Expected Success to be false")
	}

	if action.Description == "" {
		t.Error("Expected non-empty Description")
	}

	if !strings.Contains(action.Description, "failed") {
		t.Errorf("Expected description to mention failure, got: %s", action.Description)
	}

	if action.Details == nil {
		t.Fatal("Expected Details to be non-nil")
	}

	// Verify error field is present
	if errorMsg, ok := action.Details["error"].(string); !ok || errorMsg != "Connection timeout" {
		t.Errorf("Expected error='Connection timeout', got %v", action.Details["error"])
	}

	// Verify attempt number
	if attemptNum, ok := action.Details["attempt_number"].(int); !ok || attemptNum != 2 {
		t.Errorf("Expected attempt_number=2, got %v", action.Details["attempt_number"])
	}
}

// TestRestartHistory_SetFinalOutcome tests setting the final outcome.
func TestRestartHistory_SetFinalOutcome(t *testing.T) {
	history := NewRestartHistory()

	if history.FinalOutcome != "pending" {
		t.Errorf("Expected initial FinalOutcome='pending', got %s", history.FinalOutcome)
	}

	history.SetFinalOutcome("success")

	if history.FinalOutcome != "success" {
		t.Errorf("Expected FinalOutcome='success', got %s", history.FinalOutcome)
	}

	history.SetFinalOutcome("escalated")

	if history.FinalOutcome != "escalated" {
		t.Errorf("Expected FinalOutcome='escalated', got %s", history.FinalOutcome)
	}
}

// TestRestartHistory_Integration tests a complete restart sequence.
func TestRestartHistory_Integration(t *testing.T) {
	history := NewRestartHistory()

	strategy := &RestartStrategy{
		ShouldRestart:   true,
		CorrectiveFlags: []string{"--skip-permissions"},
		FailureType:     types.FailureTypePermissionPrompt,
		Reason:          "Permission prompt detected",
	}

	// First attempt fails
	history.RecordAttempt(RestartAttempt{
		AttemptNumber: 1,
		Strategy:      strategy,
		Success:       false,
		ErrorMessage:  "Still hanging",
		Duration:      100,
	})

	// Second attempt succeeds
	history.RecordAttempt(RestartAttempt{
		AttemptNumber: 2,
		Strategy:      strategy,
		Success:       true,
		Duration:      150,
	})

	history.SetFinalOutcome("success")

	// Verify counts
	if history.TotalRestarts != 2 {
		t.Errorf("Expected TotalRestarts=2, got %d", history.TotalRestarts)
	}

	if history.SuccessfulRestarts != 1 {
		t.Errorf("Expected SuccessfulRestarts=1, got %d", history.SuccessfulRestarts)
	}

	// Verify actions
	actions := history.ToHealingActions()
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}

	// First action should be failed
	if actions[0].Success {
		t.Error("Expected first action to be failed")
	}

	// Second action should be successful
	if !actions[1].Success {
		t.Error("Expected second action to be successful")
	}

	// Verify summary
	summary := history.GetSummary()
	if !strings.Contains(summary, "2 restart attempt") {
		t.Errorf("Expected summary to mention 2 attempts, got: %s", summary)
	}
	if !strings.Contains(summary, "1 successful") {
		t.Errorf("Expected summary to mention 1 successful, got: %s", summary)
	}
	if !strings.Contains(summary, "success") {
		t.Errorf("Expected summary to mention success outcome, got: %s", summary)
	}
}
