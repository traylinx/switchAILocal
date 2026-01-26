// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package security provides security controls and fail-safes for the Superbrain system.
// It ensures that autonomous actions are bounded and auditable, preventing dangerous
// operations from being performed without human approval.
package security

import (
	"fmt"
	"strings"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// FailSafe provides security controls for autonomous Superbrain actions.
// It detects security-sensitive operations and enforces safety boundaries.
type FailSafe struct {
	forbiddenOperations []string
}

// NewFailSafe creates a new security fail-safe with the given forbidden operations list.
func NewFailSafe(forbiddenOperations []string) *FailSafe {
	return &FailSafe{
		forbiddenOperations: forbiddenOperations,
	}
}

// CheckDiagnosis examines a diagnosis to detect security-sensitive operations.
// Returns true if the diagnosis involves a forbidden operation, false otherwise.
func (f *FailSafe) CheckDiagnosis(diagnosis *types.Diagnosis) (isForbidden bool, reason string) {
	if diagnosis == nil {
		return false, ""
	}

	// Check if the remediation type itself is forbidden
	remediationType := string(diagnosis.Remediation)
	if f.isForbidden(remediationType) {
		return true, fmt.Sprintf("remediation type '%s' is forbidden", remediationType)
	}

	// Check if the root cause mentions forbidden operations
	if f.containsForbiddenOperation(diagnosis.RootCause) {
		return true, fmt.Sprintf("root cause contains forbidden operation: %s", diagnosis.RootCause)
	}

	// Check remediation arguments for forbidden operations
	for key, value := range diagnosis.RemediationArgs {
		valueStr := fmt.Sprintf("%v", value)
		if f.containsForbiddenOperation(valueStr) {
			return true, fmt.Sprintf("remediation argument '%s' contains forbidden operation: %s", key, valueStr)
		}
	}

	return false, ""
}

// CheckHealingAction examines a healing action to detect security-sensitive operations.
// Returns true if the action involves a forbidden operation, false otherwise.
func (f *FailSafe) CheckHealingAction(action *types.HealingAction) (isForbidden bool, reason string) {
	if action == nil {
		return false, ""
	}

	// Check action type
	if f.isForbidden(action.ActionType) {
		return true, fmt.Sprintf("action type '%s' is forbidden", action.ActionType)
	}

	// Check description for forbidden operations
	if f.containsForbiddenOperation(action.Description) {
		return true, fmt.Sprintf("action description contains forbidden operation: %s", action.Description)
	}

	// Check details map for forbidden operations
	for key, value := range action.Details {
		valueStr := fmt.Sprintf("%v", value)
		if f.containsForbiddenOperation(valueStr) {
			return true, fmt.Sprintf("action detail '%s' contains forbidden operation: %s", key, valueStr)
		}
	}

	return false, ""
}

// CreateSafeFailureResponse creates a safe failure response when a forbidden operation is detected.
// This response explains what was attempted and why it was blocked.
func (f *FailSafe) CreateSafeFailureResponse(diagnosis *types.Diagnosis, reason string) map[string]interface{} {
	response := map[string]interface{}{
		"error": map[string]interface{}{
			"message": "Autonomous remediation blocked by security fail-safe",
			"type":    "security_violation",
			"code":    "forbidden_operation",
		},
		"superbrain": map[string]interface{}{
			"security_blocked": true,
			"block_reason":     reason,
			"attempted_actions": []string{
				fmt.Sprintf("Diagnosis: %s", diagnosis.FailureType),
				fmt.Sprintf("Proposed remediation: %s", diagnosis.Remediation),
			},
			"diagnosis_summary": diagnosis.RootCause,
			"suggestions": []string{
				"This operation requires human approval",
				"Review the diagnosis and manually apply the remediation if appropriate",
				"Consider updating the forbidden_operations configuration if this operation should be allowed",
			},
		},
	}

	return response
}

// isForbidden checks if a given operation name matches any forbidden operation.
func (f *FailSafe) isForbidden(operation string) bool {
	operationLower := strings.ToLower(operation)
	for _, forbidden := range f.forbiddenOperations {
		if strings.ToLower(forbidden) == operationLower {
			return true
		}
	}
	return false
}

// containsForbiddenOperation checks if a string contains any forbidden operation keywords.
func (f *FailSafe) containsForbiddenOperation(text string) bool {
	textLower := strings.ToLower(text)
	for _, forbidden := range f.forbiddenOperations {
		forbiddenLower := strings.ToLower(forbidden)
		if strings.Contains(textLower, forbiddenLower) {
			return true
		}
	}
	return false
}
