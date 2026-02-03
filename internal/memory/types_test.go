package memory

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRoutingDecision_JSONSerialization(t *testing.T) {
	decision := &RoutingDecision{
		Timestamp:  time.Date(2026, 2, 2, 9, 0, 0, 0, time.UTC),
		APIKeyHash: "sha256:abc123",
		Request: RequestInfo{
			Model:         "auto",
			Intent:        "coding",
			ContentHash:   "sha256:def456",
			ContentLength: 1234,
		},
		Routing: RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "claudecli:claude-sonnet-4",
			Confidence:    0.92,
			LatencyMs:     15,
		},
		Outcome: OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 2340,
			Error:          "",
			QualityScore:   0.88,
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("Failed to marshal RoutingDecision: %v", err)
	}

	// Deserialize from JSON
	var decoded RoutingDecision
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal RoutingDecision: %v", err)
	}

	// Verify fields
	if decoded.APIKeyHash != decision.APIKeyHash {
		t.Errorf("APIKeyHash mismatch: got %s, want %s", decoded.APIKeyHash, decision.APIKeyHash)
	}

	if decoded.Request.Intent != decision.Request.Intent {
		t.Errorf("Intent mismatch: got %s, want %s", decoded.Request.Intent, decision.Request.Intent)
	}

	if decoded.Routing.SelectedModel != decision.Routing.SelectedModel {
		t.Errorf("SelectedModel mismatch: got %s, want %s", decoded.Routing.SelectedModel, decision.Routing.SelectedModel)
	}

	if decoded.Outcome.Success != decision.Outcome.Success {
		t.Errorf("Success mismatch: got %v, want %v", decoded.Outcome.Success, decision.Outcome.Success)
	}
}

func TestUserPreferences_JSONSerialization(t *testing.T) {
	prefs := &UserPreferences{
		APIKeyHash:  "sha256:abc123",
		LastUpdated: time.Date(2026, 2, 2, 9, 0, 0, 0, time.UTC),
		ModelPreferences: map[string]string{
			"coding":    "claudecli:claude-sonnet-4",
			"reasoning": "geminicli:gemini-2.5-pro",
		},
		ProviderBias: map[string]float64{
			"ollama":    0.5,
			"claudecli": 0.3,
		},
		CustomRules: []PreferenceRule{
			{
				Condition: "intent == 'coding'",
				Model:     "claudecli:claude-sonnet-4",
				Priority:  100,
			},
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(prefs)
	if err != nil {
		t.Fatalf("Failed to marshal UserPreferences: %v", err)
	}

	// Deserialize from JSON
	var decoded UserPreferences
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal UserPreferences: %v", err)
	}

	// Verify fields
	if decoded.APIKeyHash != prefs.APIKeyHash {
		t.Errorf("APIKeyHash mismatch: got %s, want %s", decoded.APIKeyHash, prefs.APIKeyHash)
	}

	if len(decoded.ModelPreferences) != len(prefs.ModelPreferences) {
		t.Errorf("ModelPreferences length mismatch: got %d, want %d", len(decoded.ModelPreferences), len(prefs.ModelPreferences))
	}

	if decoded.ModelPreferences["coding"] != prefs.ModelPreferences["coding"] {
		t.Errorf("ModelPreferences[coding] mismatch: got %s, want %s", decoded.ModelPreferences["coding"], prefs.ModelPreferences["coding"])
	}

	if len(decoded.CustomRules) != len(prefs.CustomRules) {
		t.Errorf("CustomRules length mismatch: got %d, want %d", len(decoded.CustomRules), len(prefs.CustomRules))
	}
}

func TestQuirk_JSONSerialization(t *testing.T) {
	quirk := &Quirk{
		Provider:   "ollama",
		Issue:      "Connection timeout on first request after idle",
		Workaround: "Send warmup request on startup",
		Discovered: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		Frequency:  "3/10 startups",
		Severity:   "medium",
	}

	// Serialize to JSON
	data, err := json.Marshal(quirk)
	if err != nil {
		t.Fatalf("Failed to marshal Quirk: %v", err)
	}

	// Deserialize from JSON
	var decoded Quirk
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal Quirk: %v", err)
	}

	// Verify fields
	if decoded.Provider != quirk.Provider {
		t.Errorf("Provider mismatch: got %s, want %s", decoded.Provider, quirk.Provider)
	}

	if decoded.Issue != quirk.Issue {
		t.Errorf("Issue mismatch: got %s, want %s", decoded.Issue, quirk.Issue)
	}

	if decoded.Severity != quirk.Severity {
		t.Errorf("Severity mismatch: got %s, want %s", decoded.Severity, quirk.Severity)
	}
}

func TestProviderStats_JSONSerialization(t *testing.T) {
	stats := &ProviderStats{
		Provider:      "ollama",
		TotalRequests: 1000,
		SuccessRate:   0.95,
		AvgLatencyMs:  250.5,
		ErrorRate:     0.05,
		LastUpdated:   time.Date(2026, 2, 2, 9, 0, 0, 0, time.UTC),
	}

	// Serialize to JSON
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("Failed to marshal ProviderStats: %v", err)
	}

	// Deserialize from JSON
	var decoded ProviderStats
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal ProviderStats: %v", err)
	}

	// Verify fields
	if decoded.Provider != stats.Provider {
		t.Errorf("Provider mismatch: got %s, want %s", decoded.Provider, stats.Provider)
	}

	if decoded.TotalRequests != stats.TotalRequests {
		t.Errorf("TotalRequests mismatch: got %d, want %d", decoded.TotalRequests, stats.TotalRequests)
	}

	if decoded.SuccessRate != stats.SuccessRate {
		t.Errorf("SuccessRate mismatch: got %f, want %f", decoded.SuccessRate, stats.SuccessRate)
	}
}

func TestModelPerformance_JSONSerialization(t *testing.T) {
	perf := &ModelPerformance{
		Model:           "claudecli:claude-sonnet-4",
		TotalRequests:   500,
		SuccessRate:     0.98,
		AvgQualityScore: 0.92,
		AvgCostPerReq:   0.015,
	}

	// Serialize to JSON
	data, err := json.Marshal(perf)
	if err != nil {
		t.Fatalf("Failed to marshal ModelPerformance: %v", err)
	}

	// Deserialize from JSON
	var decoded ModelPerformance
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal ModelPerformance: %v", err)
	}

	// Verify fields
	if decoded.Model != perf.Model {
		t.Errorf("Model mismatch: got %s, want %s", decoded.Model, perf.Model)
	}

	if decoded.TotalRequests != perf.TotalRequests {
		t.Errorf("TotalRequests mismatch: got %d, want %d", decoded.TotalRequests, perf.TotalRequests)
	}

	if decoded.AvgQualityScore != perf.AvgQualityScore {
		t.Errorf("AvgQualityScore mismatch: got %f, want %f", decoded.AvgQualityScore, perf.AvgQualityScore)
	}
}

func TestDefaultMemoryConfig(t *testing.T) {
	config := DefaultMemoryConfig()

	if config == nil {
		t.Fatal("DefaultMemoryConfig() returned nil")
	}

	// Verify default values
	if config.Enabled {
		t.Error("Expected Enabled to be false by default (opt-in)")
	}

	if config.BaseDir != ".switchailocal/memory" {
		t.Errorf("BaseDir mismatch: got %s, want .switchailocal/memory", config.BaseDir)
	}

	if config.RetentionDays != 90 {
		t.Errorf("RetentionDays mismatch: got %d, want 90", config.RetentionDays)
	}

	if config.MaxLogSizeMB != 100 {
		t.Errorf("MaxLogSizeMB mismatch: got %d, want 100", config.MaxLogSizeMB)
	}

	if !config.Compression {
		t.Error("Expected Compression to be true by default")
	}
}

func TestRoutingDecision_EmptyError(t *testing.T) {
	// Test that empty error field is omitted in JSON
	decision := &RoutingDecision{
		Timestamp:  time.Now(),
		APIKeyHash: "sha256:test",
		Request: RequestInfo{
			Model:  "auto",
			Intent: "coding",
		},
		Routing: RoutingInfo{
			Tier:          "semantic",
			SelectedModel: "test-model",
			Confidence:    0.9,
		},
		Outcome: OutcomeInfo{
			Success:        true,
			ResponseTimeMs: 1000,
			Error:          "", // Empty error
			QualityScore:   0.9,
		},
	}

	data, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Verify that "error" field is omitted when empty
	dataStr := string(data)
	if contains(dataStr, `"error":""`) {
		t.Error("Empty error field should be omitted from JSON")
	}
}

func TestPreferenceRule_Validation(t *testing.T) {
	tests := []struct {
		name  string
		rule  PreferenceRule
		valid bool
	}{
		{
			name: "valid rule",
			rule: PreferenceRule{
				Condition: "intent == 'coding'",
				Model:     "claudecli:claude-sonnet-4",
				Priority:  100,
			},
			valid: true,
		},
		{
			name: "empty condition",
			rule: PreferenceRule{
				Condition: "",
				Model:     "claudecli:claude-sonnet-4",
				Priority:  100,
			},
			valid: false,
		},
		{
			name: "empty model",
			rule: PreferenceRule{
				Condition: "intent == 'coding'",
				Model:     "",
				Priority:  100,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation: check if required fields are present
			valid := tt.rule.Condition != "" && tt.rule.Model != ""

			if valid != tt.valid {
				t.Errorf("Validation mismatch: got %v, want %v", valid, tt.valid)
			}
		})
	}
}
