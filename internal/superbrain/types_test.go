package superbrain

import (
	"encoding/json"
	"testing"
)

// TestNegotiatedFailureResponse_Structure verifies the NegotiatedFailureResponse
// can be created and serialized correctly.
func TestNegotiatedFailureResponse_Structure(t *testing.T) {
	// Create a sample negotiated failure response
	response := NegotiatedFailureResponse{}
	response.Error.Message = "All providers failed after multiple healing attempts"
	response.Error.Type = "provider_unavailable"
	response.Error.Code = "SUPERBRAIN_EXHAUSTED"

	response.Superbrain.AttemptedActions = []string{
		"stdin_injection: Injected 'y' for permission prompt",
		"restart_with_flags: Restarted with --dangerously-skip-permissions",
		"fallback_routing: Attempted fallback to geminicli",
	}
	response.Superbrain.DiagnosisSummary = "Process hung on permission prompt; stdin injection failed; restart failed; no suitable fallback providers available"
	response.Superbrain.Suggestions = []string{
		"Check that alternative providers (gemini, ollama) are configured and available",
		"Verify network connectivity to provider APIs",
		"Review audit logs for detailed failure information",
	}
	response.Superbrain.FallbacksTried = []string{"geminicli", "gemini"}

	// Verify fields are set correctly
	if response.Error.Message == "" {
		t.Error("Error.Message should not be empty")
	}
	if response.Error.Type != "provider_unavailable" {
		t.Errorf("Expected Error.Type 'provider_unavailable', got '%s'", response.Error.Type)
	}
	if response.Error.Code != "SUPERBRAIN_EXHAUSTED" {
		t.Errorf("Expected Error.Code 'SUPERBRAIN_EXHAUSTED', got '%s'", response.Error.Code)
	}

	if len(response.Superbrain.AttemptedActions) != 3 {
		t.Errorf("Expected 3 attempted actions, got %d", len(response.Superbrain.AttemptedActions))
	}
	if response.Superbrain.DiagnosisSummary == "" {
		t.Error("DiagnosisSummary should not be empty")
	}
	if len(response.Superbrain.Suggestions) != 3 {
		t.Errorf("Expected 3 suggestions, got %d", len(response.Superbrain.Suggestions))
	}
	if len(response.Superbrain.FallbacksTried) != 2 {
		t.Errorf("Expected 2 fallbacks tried, got %d", len(response.Superbrain.FallbacksTried))
	}
}

// TestNegotiatedFailureResponse_JSONSerialization verifies the response
// can be serialized to JSON correctly.
func TestNegotiatedFailureResponse_JSONSerialization(t *testing.T) {
	response := NegotiatedFailureResponse{}
	response.Error.Message = "Request failed after healing attempts"
	response.Error.Type = "unrecoverable_failure"
	response.Error.Code = "HEALING_FAILED"

	response.Superbrain.AttemptedActions = []string{"restart_with_flags"}
	response.Superbrain.DiagnosisSummary = "Context limit exceeded"
	response.Superbrain.Suggestions = []string{"Use a model with larger context window"}

	// Serialize to JSON
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal NegotiatedFailureResponse: %v", err)
	}

	// Verify JSON structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Check top-level fields
	if _, ok := parsed["error"]; !ok {
		t.Error("JSON should contain 'error' field")
	}
	if _, ok := parsed["superbrain"]; !ok {
		t.Error("JSON should contain 'superbrain' field")
	}

	// Check error fields
	errorMap, ok := parsed["error"].(map[string]interface{})
	if !ok {
		t.Fatal("'error' should be an object")
	}
	if errorMap["message"] != "Request failed after healing attempts" {
		t.Errorf("Unexpected error message: %v", errorMap["message"])
	}
	if errorMap["type"] != "unrecoverable_failure" {
		t.Errorf("Unexpected error type: %v", errorMap["type"])
	}
	if errorMap["code"] != "HEALING_FAILED" {
		t.Errorf("Unexpected error code: %v", errorMap["code"])
	}

	// Check superbrain fields
	superbrainMap, ok := parsed["superbrain"].(map[string]interface{})
	if !ok {
		t.Fatal("'superbrain' should be an object")
	}
	if _, ok := superbrainMap["attempted_actions"]; !ok {
		t.Error("superbrain should contain 'attempted_actions'")
	}
	if _, ok := superbrainMap["diagnosis_summary"]; !ok {
		t.Error("superbrain should contain 'diagnosis_summary'")
	}
	if _, ok := superbrainMap["suggestions"]; !ok {
		t.Error("superbrain should contain 'suggestions'")
	}
}

// TestNegotiatedFailureResponse_MinimalResponse verifies the response
// works with minimal required fields.
func TestNegotiatedFailureResponse_MinimalResponse(t *testing.T) {
	response := NegotiatedFailureResponse{}
	response.Error.Message = "Provider unavailable"
	response.Error.Type = "provider_error"
	response.Error.Code = "PROVIDER_DOWN"

	response.Superbrain.AttemptedActions = []string{}
	response.Superbrain.DiagnosisSummary = "Provider is not responding"
	response.Superbrain.Suggestions = []string{"Try again later"}
	// FallbacksTried is omitted (optional field)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal minimal response: %v", err)
	}

	// Verify it can be unmarshaled
	var parsed NegotiatedFailureResponse
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal minimal response: %v", err)
	}

	if parsed.Error.Message != "Provider unavailable" {
		t.Errorf("Unexpected message: %s", parsed.Error.Message)
	}
	if len(parsed.Superbrain.FallbacksTried) != 0 {
		t.Errorf("Expected empty FallbacksTried, got %d items", len(parsed.Superbrain.FallbacksTried))
	}
}

// TestNegotiatedFailureResponse_EmptyAttemptedActions verifies the response
// handles the case where no healing actions were attempted.
func TestNegotiatedFailureResponse_EmptyAttemptedActions(t *testing.T) {
	response := NegotiatedFailureResponse{}
	response.Error.Message = "Immediate failure before healing"
	response.Error.Type = "validation_error"
	response.Error.Code = "INVALID_REQUEST"

	response.Superbrain.AttemptedActions = []string{} // No actions attempted
	response.Superbrain.DiagnosisSummary = "Request validation failed"
	response.Superbrain.Suggestions = []string{"Check request format"}

	// Verify empty slice is handled correctly
	if response.Superbrain.AttemptedActions == nil {
		t.Error("AttemptedActions should be empty slice, not nil")
	}
	if len(response.Superbrain.AttemptedActions) != 0 {
		t.Errorf("Expected 0 attempted actions, got %d", len(response.Superbrain.AttemptedActions))
	}

	// Verify JSON serialization
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	superbrainMap := parsed["superbrain"].(map[string]interface{})
	actions := superbrainMap["attempted_actions"].([]interface{})
	if len(actions) != 0 {
		t.Errorf("Expected empty actions array in JSON, got %d items", len(actions))
	}
}
