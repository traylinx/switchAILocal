package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestInternalDoctor_Diagnose_PatternMatch(t *testing.T) {
	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg)

	tests := []struct {
		name         string
		snapshot     *types.DiagnosticSnapshot
		wantType     types.FailureType
		wantMatched  bool
	}{
		{
			name: "permission prompt detected",
			snapshot: &types.DiagnosticSnapshot{
				Provider:      "claudecli",
				Model:         "claude-sonnet-4",
				ProcessState:  "blocked",
				ElapsedTimeMs: 35000,
				LastLogLines:  []string{"Allow Claude to read file.txt? [y/n]"},
			},
			wantType:    types.FailureTypePermissionPrompt,
			wantMatched: true,
		},
		{
			name: "rate limit detected",
			snapshot: &types.DiagnosticSnapshot{
				Provider:      "gemini",
				Model:         "gemini-pro",
				ProcessState:  "terminated",
				ElapsedTimeMs: 1000,
				LastLogLines:  []string{"Error: HTTP 429 - Rate limit exceeded"},
			},
			wantType:    types.FailureTypeRateLimit,
			wantMatched: true,
		},
		{
			name: "auth error detected",
			snapshot: &types.DiagnosticSnapshot{
				Provider:      "claudecli",
				Model:         "claude-sonnet-4",
				ProcessState:  "terminated",
				ElapsedTimeMs: 500,
				LastLogLines:  []string{"Error: Invalid API key provided"},
			},
			wantType:    types.FailureTypeAuthError,
			wantMatched: true,
		},
		{
			name: "context exceeded detected",
			snapshot: &types.DiagnosticSnapshot{
				Provider:      "claudecli",
				Model:         "claude-sonnet-4",
				ProcessState:  "terminated",
				ElapsedTimeMs: 2000,
				LastLogLines:  []string{"Error: Context limit exceeded for this model"},
			},
			wantType:    types.FailureTypeContextExceeded,
			wantMatched: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnosis, err := doctor.Diagnose(context.Background(), tt.snapshot)
			if err != nil {
				t.Fatalf("Diagnose() error = %v", err)
			}

			if diagnosis.FailureType != tt.wantType {
				t.Errorf("Diagnose() type = %v, want %v", diagnosis.FailureType, tt.wantType)
			}

			// Pattern-based diagnosis should have high confidence
			if tt.wantMatched && diagnosis.Confidence < 0.5 {
				t.Errorf("Diagnose() confidence = %v, want >= 0.5", diagnosis.Confidence)
			}
		})
	}
}

func TestInternalDoctor_Diagnose_NilSnapshot(t *testing.T) {
	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg)

	_, err := doctor.Diagnose(context.Background(), nil)
	if err == nil {
		t.Error("Diagnose() should return error for nil snapshot")
	}
}

func TestInternalDoctor_DiagnosePatternOnly(t *testing.T) {
	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg)

	snapshot := &types.DiagnosticSnapshot{
		Provider:      "claudecli",
		Model:         "claude-sonnet-4",
		ProcessState:  "blocked",
		ElapsedTimeMs: 35000,
		LastLogLines:  []string{"Allow Claude to read file.txt? [y/n]"},
	}

	diagnosis := doctor.DiagnosePatternOnly(snapshot)

	if diagnosis.FailureType != types.FailureTypePermissionPrompt {
		t.Errorf("DiagnosePatternOnly() type = %v, want %v", diagnosis.FailureType, types.FailureTypePermissionPrompt)
	}
}

func TestInternalDoctor_DiagnosePatternOnly_NilSnapshot(t *testing.T) {
	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg)

	diagnosis := doctor.DiagnosePatternOnly(nil)

	if diagnosis.FailureType != types.FailureTypeUnknown {
		t.Errorf("DiagnosePatternOnly() type = %v, want %v", diagnosis.FailureType, types.FailureTypeUnknown)
	}
}

func TestInternalDoctor_FallbackToPatternOnAIFailure(t *testing.T) {
	// Create a mock server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "service unavailable"}`))
	}))
	defer server.Close()

	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 1000,
	}

	doctor := NewInternalDoctor(cfg, WithGatewayURL(server.URL))

	// Snapshot with no matching pattern - should fall back to unknown
	snapshot := &types.DiagnosticSnapshot{
		Provider:      "claudecli",
		Model:         "claude-sonnet-4",
		ProcessState:  "blocked",
		ElapsedTimeMs: 35000,
		LastLogLines:  []string{"Some random output that doesn't match any pattern"},
	}

	diagnosis, err := doctor.Diagnose(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Diagnose() should not return error on AI failure, got: %v", err)
	}

	// Should fall back to unknown when AI fails and no pattern matches
	if diagnosis.FailureType != types.FailureTypeUnknown {
		t.Errorf("Diagnose() type = %v, want %v", diagnosis.FailureType, types.FailureTypeUnknown)
	}
}

func TestInternalDoctor_TimeoutEnforcement(t *testing.T) {
	// Create a mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Delay longer than timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 500, // 500ms timeout
	}

	doctor := NewInternalDoctor(cfg, WithGatewayURL(server.URL))

	snapshot := &types.DiagnosticSnapshot{
		Provider:      "claudecli",
		Model:         "claude-sonnet-4",
		ProcessState:  "blocked",
		ElapsedTimeMs: 35000,
		LastLogLines:  []string{"Some output that doesn't match patterns"},
	}

	start := time.Now()
	diagnosis, err := doctor.Diagnose(context.Background(), snapshot)
	elapsed := time.Since(start)

	// Should complete within reasonable time (timeout + some buffer)
	if elapsed > 2*time.Second {
		t.Errorf("Diagnose() took %v, expected to timeout faster", elapsed)
	}

	// Should not return error, but fall back to unknown
	if err != nil {
		t.Fatalf("Diagnose() should not return error on timeout, got: %v", err)
	}

	if diagnosis.FailureType != types.FailureTypeUnknown {
		t.Errorf("Diagnose() type = %v, want %v", diagnosis.FailureType, types.FailureTypeUnknown)
	}
}

func TestInternalDoctor_DiagnoseWithAI_Success(t *testing.T) {
	// Create a mock server that returns a valid AI response
	aiResponse := aiDiagnosisResponse{
		FailureType:     "network_error",
		RootCause:       "Connection to API server failed",
		Confidence:      0.9,
		Remediation:     "simple_retry",
		RemediationArgs: map[string]string{"delay_ms": "1000"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json")
		}

		// Return mock response
		respContent, _ := json.Marshal(aiResponse)
		chatResp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": string(respContent),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(chatResp)
	}))
	defer server.Close()

	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg, WithGatewayURL(server.URL))

	// Snapshot with no matching pattern - will use AI
	snapshot := &types.DiagnosticSnapshot{
		Provider:      "claudecli",
		Model:         "claude-sonnet-4",
		ProcessState:  "terminated",
		ElapsedTimeMs: 5000,
		LastLogLines:  []string{"Connection failed to api.example.com"},
	}

	diagnosis, err := doctor.Diagnose(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}

	if diagnosis.FailureType != types.FailureTypeNetworkError {
		t.Errorf("Diagnose() type = %v, want %v", diagnosis.FailureType, types.FailureTypeNetworkError)
	}

	if diagnosis.Confidence != 0.9 {
		t.Errorf("Diagnose() confidence = %v, want 0.9", diagnosis.Confidence)
	}

	if diagnosis.Remediation != types.RemediationRetry {
		t.Errorf("Diagnose() remediation = %v, want %v", diagnosis.Remediation, types.RemediationRetry)
	}
}

func TestInternalDoctor_GetDiagnosticModel(t *testing.T) {
	tests := []struct {
		name      string
		config    *config.DoctorConfig
		wantModel string
	}{
		{
			name:      "configured model",
			config:    &config.DoctorConfig{Model: "custom-model"},
			wantModel: "custom-model",
		},
		{
			name:      "empty model uses default",
			config:    &config.DoctorConfig{Model: ""},
			wantModel: "gemini-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := NewInternalDoctor(tt.config)
			if got := doctor.GetDiagnosticModel(); got != tt.wantModel {
				t.Errorf("GetDiagnosticModel() = %v, want %v", got, tt.wantModel)
			}
		})
	}
}

func TestInternalDoctor_DiagnosisCompleteness(t *testing.T) {
	cfg := &config.DoctorConfig{
		Model:     "gemini-flash",
		TimeoutMs: 5000,
	}

	doctor := NewInternalDoctor(cfg)

	snapshot := &types.DiagnosticSnapshot{
		Provider:      "claudecli",
		Model:         "claude-sonnet-4",
		ProcessState:  "blocked",
		ElapsedTimeMs: 35000,
		LastLogLines:  []string{"Allow Claude to read file.txt? [y/n]"},
	}

	diagnosis, err := doctor.Diagnose(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Diagnose() error = %v", err)
	}

	// Verify diagnosis completeness (Property 7)
	if diagnosis.FailureType == "" {
		t.Error("Diagnosis should have non-empty failure_type")
	}

	if diagnosis.RootCause == "" {
		t.Error("Diagnosis should have non-empty root_cause")
	}

	if diagnosis.Remediation == "" {
		t.Error("Diagnosis should have non-empty remediation")
	}
}

func TestMapFailureType(t *testing.T) {
	tests := []struct {
		input string
		want  types.FailureType
	}{
		{"permission_prompt", types.FailureTypePermissionPrompt},
		{"PERMISSION_PROMPT", types.FailureTypePermissionPrompt},
		{"auth_error", types.FailureTypeAuthError},
		{"context_exceeded", types.FailureTypeContextExceeded},
		{"rate_limit", types.FailureTypeRateLimit},
		{"network_error", types.FailureTypeNetworkError},
		{"process_crash", types.FailureTypeProcessCrash},
		{"unknown", types.FailureTypeUnknown},
		{"invalid", types.FailureTypeUnknown},
		{"", types.FailureTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapFailureType(tt.input); got != tt.want {
				t.Errorf("mapFailureType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapRemediationType(t *testing.T) {
	tests := []struct {
		input string
		want  types.RemediationType
	}{
		{"stdin_inject", types.RemediationStdinInject},
		{"STDIN_INJECT", types.RemediationStdinInject},
		{"restart_with_flags", types.RemediationRestartFlags},
		{"fallback_provider", types.RemediationFallback},
		{"simple_retry", types.RemediationRetry},
		{"abort", types.RemediationAbort},
		{"invalid", types.RemediationAbort},
		{"", types.RemediationAbort},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapRemediationType(tt.input); got != tt.want {
				t.Errorf("mapRemediationType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean JSON",
			input: `{"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON with prefix",
			input: `Here is the response: {"key": "value"}`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON with suffix",
			input: `{"key": "value"} That's the answer.`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "JSON with both",
			input: `Response: {"key": "value"} Done.`,
			want:  `{"key": "value"}`,
		},
		{
			name:  "no JSON",
			input: `No JSON here`,
			want:  `No JSON here`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractJSON(tt.input); got != tt.want {
				t.Errorf("extractJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}
