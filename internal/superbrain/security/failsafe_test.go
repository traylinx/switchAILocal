// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package security

import (
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestNewFailSafe(t *testing.T) {
	forbiddenOps := []string{"file_delete", "system_command"}
	fs := NewFailSafe(forbiddenOps)

	if fs == nil {
		t.Fatal("NewFailSafe returned nil")
	}

	if len(fs.forbiddenOperations) != 2 {
		t.Errorf("Expected 2 forbidden operations, got %d", len(fs.forbiddenOperations))
	}
}

func TestCheckDiagnosis_ForbiddenRemediationType(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "system_command"})

	diagnosis := &types.Diagnosis{
		FailureType:     types.FailureTypePermissionPrompt,
		RootCause:       "Process needs file access",
		Remediation:     types.RemediationType("file_delete"),
		RemediationArgs: map[string]string{},
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if !isForbidden {
		t.Error("Expected diagnosis to be flagged as forbidden")
	}

	if reason == "" {
		t.Error("Expected non-empty reason")
	}

	if !contains(reason, "file_delete") {
		t.Errorf("Expected reason to mention 'file_delete', got: %s", reason)
	}
}

func TestCheckDiagnosis_ForbiddenInRootCause(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "system_command"})

	diagnosis := &types.Diagnosis{
		FailureType:     types.FailureTypeProcessCrash,
		RootCause:       "Process attempted to execute system_command which failed",
		Remediation:     types.RemediationRestartFlags,
		RemediationArgs: map[string]string{},
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if !isForbidden {
		t.Error("Expected diagnosis to be flagged as forbidden")
	}

	if !contains(reason, "system_command") {
		t.Errorf("Expected reason to mention 'system_command', got: %s", reason)
	}
}

func TestCheckDiagnosis_ForbiddenInRemediationArgs(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "sudo"})

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeAuthError,
		RootCause:   "Authentication failed",
		Remediation: types.RemediationRestartFlags,
		RemediationArgs: map[string]string{
			"command": "sudo authenticate",
			"flags":   "--force",
		},
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if !isForbidden {
		t.Error("Expected diagnosis to be flagged as forbidden")
	}

	if !contains(reason, "sudo") {
		t.Errorf("Expected reason to mention 'sudo', got: %s", reason)
	}
}

func TestCheckDiagnosis_SafeDiagnosis(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "system_command"})

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypePermissionPrompt,
		RootCause:   "Process waiting for user input",
		Remediation: types.RemediationStdinInject,
		RemediationArgs: map[string]string{
			"response": "y\n",
		},
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if isForbidden {
		t.Errorf("Expected diagnosis to be safe, but was flagged as forbidden: %s", reason)
	}

	if reason != "" {
		t.Errorf("Expected empty reason for safe diagnosis, got: %s", reason)
	}
}

func TestCheckDiagnosis_NilDiagnosis(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete"})

	isForbidden, reason := fs.CheckDiagnosis(nil)

	if isForbidden {
		t.Error("Expected nil diagnosis to be safe")
	}

	if reason != "" {
		t.Errorf("Expected empty reason for nil diagnosis, got: %s", reason)
	}
}

func TestCheckDiagnosis_CaseInsensitive(t *testing.T) {
	fs := NewFailSafe([]string{"FILE_DELETE", "System_Command"})

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeProcessCrash,
		RootCause:   "Process tried to file_delete something",
		Remediation: types.RemediationAbort,
		RemediationArgs: map[string]string{
			"command": "system_command execution",
		},
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if !isForbidden {
		t.Error("Expected case-insensitive matching to flag diagnosis as forbidden")
	}

	if reason == "" {
		t.Error("Expected non-empty reason")
	}
}

func TestCheckHealingAction_ForbiddenActionType(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "restart_with_sudo"})

	action := &types.HealingAction{
		Timestamp:   time.Now(),
		ActionType:  "restart_with_sudo",
		Description: "Restarting process with elevated privileges",
		Success:     false,
		Details:     map[string]interface{}{},
	}

	isForbidden, reason := fs.CheckHealingAction(action)

	if !isForbidden {
		t.Error("Expected action to be flagged as forbidden")
	}

	if !contains(reason, "restart_with_sudo") {
		t.Errorf("Expected reason to mention 'restart_with_sudo', got: %s", reason)
	}
}

func TestCheckHealingAction_ForbiddenInDescription(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "rm -rf"})

	action := &types.HealingAction{
		Timestamp:   time.Now(),
		ActionType:  "cleanup",
		Description: "Running rm -rf to clean temporary files",
		Success:     false,
		Details:     map[string]interface{}{},
	}

	isForbidden, reason := fs.CheckHealingAction(action)

	if !isForbidden {
		t.Error("Expected action to be flagged as forbidden")
	}

	if !contains(reason, "rm -rf") {
		t.Errorf("Expected reason to mention 'rm -rf', got: %s", reason)
	}
}

func TestCheckHealingAction_ForbiddenInDetails(t *testing.T) {
	fs := NewFailSafe([]string{"delete", "remove"})

	action := &types.HealingAction{
		Timestamp:   time.Now(),
		ActionType:  "file_operation",
		Description: "Performing file operation",
		Success:     false,
		Details: map[string]interface{}{
			"operation": "delete",
			"target":    "/tmp/file.txt",
		},
	}

	isForbidden, reason := fs.CheckHealingAction(action)

	if !isForbidden {
		t.Error("Expected action to be flagged as forbidden")
	}

	if !contains(reason, "delete") {
		t.Errorf("Expected reason to mention 'delete', got: %s", reason)
	}
}

func TestCheckHealingAction_SafeAction(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete", "system_command"})

	action := &types.HealingAction{
		Timestamp:   time.Now(),
		ActionType:  "stdin_injection",
		Description: "Injecting 'y' to approve file read",
		Success:     true,
		Details: map[string]interface{}{
			"pattern":  "file_read_permission",
			"response": "y\n",
		},
	}

	isForbidden, reason := fs.CheckHealingAction(action)

	if isForbidden {
		t.Errorf("Expected action to be safe, but was flagged as forbidden: %s", reason)
	}

	if reason != "" {
		t.Errorf("Expected empty reason for safe action, got: %s", reason)
	}
}

func TestCheckHealingAction_NilAction(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete"})

	isForbidden, reason := fs.CheckHealingAction(nil)

	if isForbidden {
		t.Error("Expected nil action to be safe")
	}

	if reason != "" {
		t.Errorf("Expected empty reason for nil action, got: %s", reason)
	}
}

func TestCreateSafeFailureResponse(t *testing.T) {
	fs := NewFailSafe([]string{"file_delete"})

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeProcessCrash,
		RootCause:   "Process attempted file_delete operation",
		Remediation: types.RemediationType("file_delete"),
		RemediationArgs: map[string]string{
			"target": "/important/file.txt",
		},
	}

	reason := "remediation type 'file_delete' is forbidden"
	response := fs.CreateSafeFailureResponse(diagnosis, reason)

	// Check error structure
	errorMap, ok := response["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'error' field in response")
	}

	if errorMap["message"] == "" {
		t.Error("Expected non-empty error message")
	}

	if errorMap["type"] != "security_violation" {
		t.Errorf("Expected error type 'security_violation', got: %v", errorMap["type"])
	}

	if errorMap["code"] != "forbidden_operation" {
		t.Errorf("Expected error code 'forbidden_operation', got: %v", errorMap["code"])
	}

	// Check superbrain structure
	superbrainMap, ok := response["superbrain"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'superbrain' field in response")
	}

	if superbrainMap["security_blocked"] != true {
		t.Error("Expected security_blocked to be true")
	}

	if superbrainMap["block_reason"] != reason {
		t.Errorf("Expected block_reason to be '%s', got: %v", reason, superbrainMap["block_reason"])
	}

	// Check attempted_actions
	attemptedActions, ok := superbrainMap["attempted_actions"].([]string)
	if !ok {
		t.Fatal("Expected attempted_actions to be []string")
	}

	if len(attemptedActions) == 0 {
		t.Error("Expected non-empty attempted_actions")
	}

	// Check suggestions
	suggestions, ok := superbrainMap["suggestions"].([]string)
	if !ok {
		t.Fatal("Expected suggestions to be []string")
	}

	if len(suggestions) == 0 {
		t.Error("Expected non-empty suggestions")
	}
}

func TestCreateSafeFailureResponse_WithVariousForbiddenOperations(t *testing.T) {
	testCases := []struct {
		name              string
		forbiddenOps      []string
		diagnosis         *types.Diagnosis
		expectedInReason  string
	}{
		{
			name:         "file_delete operation",
			forbiddenOps: []string{"file_delete"},
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypeProcessCrash,
				RootCause:   "Attempted to delete critical file",
				Remediation: types.RemediationType("file_delete"),
			},
			expectedInReason: "file_delete",
		},
		{
			name:         "system_command operation",
			forbiddenOps: []string{"system_command"},
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypeAuthError,
				RootCause:   "Needs system_command execution",
				Remediation: types.RemediationAbort,
			},
			expectedInReason: "system_command",
		},
		{
			name:         "sudo operation",
			forbiddenOps: []string{"sudo"},
			diagnosis: &types.Diagnosis{
				FailureType: types.FailureTypePermissionPrompt,
				RootCause:   "Requires sudo privileges",
				Remediation: types.RemediationRestartFlags,
			},
			expectedInReason: "sudo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs := NewFailSafe(tc.forbiddenOps)
			isForbidden, reason := fs.CheckDiagnosis(tc.diagnosis)

			if !isForbidden {
				t.Errorf("Expected diagnosis to be forbidden for %s", tc.name)
			}

			response := fs.CreateSafeFailureResponse(tc.diagnosis, reason)

			superbrainMap := response["superbrain"].(map[string]interface{})
			blockReason := superbrainMap["block_reason"].(string)

			if !contains(blockReason, tc.expectedInReason) {
				t.Errorf("Expected block_reason to contain '%s', got: %s", tc.expectedInReason, blockReason)
			}
		})
	}
}

func TestEmptyForbiddenOperations(t *testing.T) {
	fs := NewFailSafe([]string{})

	diagnosis := &types.Diagnosis{
		FailureType: types.FailureTypeProcessCrash,
		RootCause:   "Process attempted file_delete operation",
		Remediation: types.RemediationType("file_delete"),
	}

	isForbidden, reason := fs.CheckDiagnosis(diagnosis)

	if isForbidden {
		t.Errorf("Expected diagnosis to be safe with empty forbidden list, but was flagged: %s", reason)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
