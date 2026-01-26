package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// InternalDoctor provides AI-powered failure diagnosis for the Superbrain system.
// It analyzes diagnostic snapshots to identify failure patterns and recommend remediation.
type InternalDoctor struct {
	config         *config.DoctorConfig
	patternMatcher *PatternMatcher
	httpClient     *http.Client
	gatewayURL     string
}

// DoctorOption is a functional option for configuring the InternalDoctor.
type DoctorOption func(*InternalDoctor)

// WithPatternMatcher sets a custom pattern matcher.
func WithPatternMatcher(pm *PatternMatcher) DoctorOption {
	return func(d *InternalDoctor) {
		d.patternMatcher = pm
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) DoctorOption {
	return func(d *InternalDoctor) {
		d.httpClient = client
	}
}

// WithGatewayURL sets the gateway URL for AI model calls.
func WithGatewayURL(url string) DoctorOption {
	return func(d *InternalDoctor) {
		d.gatewayURL = url
	}
}

// NewInternalDoctor creates a new InternalDoctor with the given configuration.
func NewInternalDoctor(cfg *config.DoctorConfig, opts ...DoctorOption) *InternalDoctor {
	d := &InternalDoctor{
		config:         cfg,
		patternMatcher: NewPatternMatcher(),
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.TimeoutMs) * time.Millisecond,
		},
		gatewayURL: "http://localhost:4778", // Default gateway URL
	}

	for _, opt := range opts {
		opt(d)
	}

	return d
}

// Diagnose analyzes a diagnostic snapshot and returns a diagnosis.
// It first attempts pattern-based diagnosis, then falls back to AI analysis if needed.
// If AI analysis fails or times out, it returns the pattern-based diagnosis.
func (d *InternalDoctor) Diagnose(ctx context.Context, snapshot *types.DiagnosticSnapshot) (*types.Diagnosis, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("snapshot cannot be nil")
	}

	// Combine log content for analysis
	logContent := d.buildLogContent(snapshot)

	// First, try pattern-based diagnosis
	patternResult := d.patternMatcher.Match(logContent)
	if patternResult.Matched {
		diagnosis := DiagnosisFromPattern(patternResult.Pattern, patternResult.MatchedText)
		return diagnosis, nil
	}

	// If no pattern matched, try AI-powered diagnosis
	aiDiagnosis, err := d.diagnoseWithAI(ctx, snapshot, logContent)
	if err != nil {
		// Fall back to unknown pattern diagnosis
		return DiagnosisFromUnknown(logContent), nil
	}

	return aiDiagnosis, nil
}

// DiagnosePatternOnly performs pattern-based diagnosis without AI analysis.
// This is useful when AI is unavailable or for quick diagnosis.
func (d *InternalDoctor) DiagnosePatternOnly(snapshot *types.DiagnosticSnapshot) *types.Diagnosis {
	if snapshot == nil {
		return DiagnosisFromUnknown("")
	}

	logContent := d.buildLogContent(snapshot)
	patternResult := d.patternMatcher.Match(logContent)

	if patternResult.Matched {
		return DiagnosisFromPattern(patternResult.Pattern, patternResult.MatchedText)
	}

	return DiagnosisFromUnknown(logContent)
}

// GetDiagnosticModel returns the model configured for diagnosis.
func (d *InternalDoctor) GetDiagnosticModel() string {
	if d.config != nil && d.config.Model != "" {
		return d.config.Model
	}
	return "gemini-flash" // Default model
}

// buildLogContent combines snapshot data into a single string for analysis.
func (d *InternalDoctor) buildLogContent(snapshot *types.DiagnosticSnapshot) string {
	var sb strings.Builder

	// Add last log lines
	for _, line := range snapshot.LastLogLines {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Add stderr content if available
	if snapshot.StderrContent != "" {
		sb.WriteString("\n--- STDERR ---\n")
		sb.WriteString(snapshot.StderrContent)
	}

	return sb.String()
}

// diagnoseWithAI uses an AI model to analyze the failure.
func (d *InternalDoctor) diagnoseWithAI(ctx context.Context, snapshot *types.DiagnosticSnapshot, logContent string) (*types.Diagnosis, error) {
	// Create a context with timeout
	timeoutMs := d.config.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000 // Default 5 seconds
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	// Build the prompt for AI analysis
	prompt := d.buildDiagnosisPrompt(snapshot, logContent)

	// Make the API call
	response, err := d.callAIModel(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI diagnosis failed: %w", err)
	}

	// Parse the AI response into a diagnosis
	diagnosis, err := d.parseAIResponse(response, logContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	return diagnosis, nil
}

// buildDiagnosisPrompt creates the prompt for AI-powered diagnosis.
func (d *InternalDoctor) buildDiagnosisPrompt(snapshot *types.DiagnosticSnapshot, logContent string) string {
	return fmt.Sprintf(`You are a diagnostic AI analyzing a CLI process failure. Analyze the following information and provide a diagnosis.

Process Information:
- Provider: %s
- Model: %s
- Process State: %s
- Elapsed Time: %dms

Log Content:
%s

Respond with a JSON object containing:
{
  "failure_type": "one of: permission_prompt, auth_error, context_exceeded, rate_limit, network_error, process_crash, unknown",
  "root_cause": "brief description of what went wrong",
  "confidence": 0.0 to 1.0,
  "remediation": "one of: stdin_inject, restart_with_flags, fallback_provider, simple_retry, abort",
  "remediation_args": {"key": "value"} // optional arguments for the remediation
}

Only respond with the JSON object, no additional text.`,
		snapshot.Provider,
		snapshot.Model,
		snapshot.ProcessState,
		snapshot.ElapsedTimeMs,
		logContent,
	)
}

// chatRequest represents an OpenAI-compatible chat completion request.
type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatMessage represents a message in the chat completion request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse represents an OpenAI-compatible chat completion response.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// callAIModel makes a request to the AI model via the gateway.
func (d *InternalDoctor) callAIModel(ctx context.Context, prompt string) (string, error) {
	reqBody := chatRequest{
		Model: d.GetDiagnosticModel(),
		Messages: []chatMessage{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimRight(d.gatewayURL, "/"))
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// aiDiagnosisResponse represents the expected JSON response from the AI model.
type aiDiagnosisResponse struct {
	FailureType     string            `json:"failure_type"`
	RootCause       string            `json:"root_cause"`
	Confidence      float64           `json:"confidence"`
	Remediation     string            `json:"remediation"`
	RemediationArgs map[string]string `json:"remediation_args"`
}

// parseAIResponse parses the AI model's response into a Diagnosis.
func (d *InternalDoctor) parseAIResponse(response string, logContent string) (*types.Diagnosis, error) {
	// Try to extract JSON from the response (in case there's extra text)
	response = extractJSON(response)

	var aiResp aiDiagnosisResponse
	if err := json.Unmarshal([]byte(response), &aiResp); err != nil {
		return nil, fmt.Errorf("failed to parse AI response JSON: %w", err)
	}

	// Map string values to typed enums
	failureType := mapFailureType(aiResp.FailureType)
	remediation := mapRemediationType(aiResp.Remediation)

	builder := NewDiagnosisBuilder().
		WithFailureType(failureType).
		WithRootCause(aiResp.RootCause).
		WithConfidence(aiResp.Confidence).
		WithRemediation(remediation).
		WithRawAnalysis(response)

	if aiResp.RemediationArgs != nil {
		builder.WithRemediationArgs(aiResp.RemediationArgs)
	}

	return builder.Build(), nil
}

// extractJSON attempts to extract a JSON object from a string that may contain extra text.
func extractJSON(s string) string {
	// Find the first { and last }
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")

	if start >= 0 && end > start {
		return s[start : end+1]
	}

	return s
}

// mapFailureType converts a string to a FailureType enum.
func mapFailureType(s string) types.FailureType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "permission_prompt":
		return types.FailureTypePermissionPrompt
	case "auth_error":
		return types.FailureTypeAuthError
	case "context_exceeded":
		return types.FailureTypeContextExceeded
	case "rate_limit":
		return types.FailureTypeRateLimit
	case "network_error":
		return types.FailureTypeNetworkError
	case "process_crash":
		return types.FailureTypeProcessCrash
	default:
		return types.FailureTypeUnknown
	}
}

// mapRemediationType converts a string to a RemediationType enum.
func mapRemediationType(s string) types.RemediationType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "stdin_inject":
		return types.RemediationStdinInject
	case "restart_with_flags":
		return types.RemediationRestartFlags
	case "fallback_provider":
		return types.RemediationFallback
	case "simple_retry":
		return types.RemediationRetry
	default:
		return types.RemediationAbort
	}
}
