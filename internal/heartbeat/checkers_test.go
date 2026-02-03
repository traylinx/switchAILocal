package heartbeat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"
)

// --- Ollama Health Checker Tests ---

func TestOllamaHealthChecker_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("Expected /api/tags, got %s", r.URL.Path)
		}

		response := map[string]interface{}{
			"models": []interface{}{
				map[string]string{"name": "llama2"},
				map[string]string{"name": "codellama"},
				map[string]string{"name": "mistral"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	checker := NewOllamaHealthChecker(server.URL)

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if status.Provider != "ollama" {
		t.Errorf("Expected provider 'ollama', got '%s'", status.Provider)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}

	if status.ModelsCount != 3 {
		t.Errorf("Expected 3 models, got %d", status.ModelsCount)
	}

	if status.ResponseTime <= 0 {
		t.Error("Response time should be positive")
	}

	if status.LastCheck.IsZero() {
		t.Error("LastCheck should be set")
	}
}

func TestOllamaHealthChecker_ServerError(t *testing.T) {
	// Create mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := NewOllamaHealthChecker(server.URL)

	ctx := context.Background()
	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected error for server error")
	}
}

func TestOllamaHealthChecker_Timeout(t *testing.T) {
	// Create mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Second) // Longer than client timeout
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"models": []interface{}{}})
	}))
	defer server.Close()

	checker := NewOllamaHealthChecker(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestOllamaHealthChecker_InvalidJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	checker := NewOllamaHealthChecker(server.URL)

	ctx := context.Background()
	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestOllamaHealthChecker_DefaultURL(t *testing.T) {
	checker := NewOllamaHealthChecker("")

	if checker.baseURL != "http://localhost:11434" {
		t.Errorf("Expected default URL, got %s", checker.baseURL)
	}
}

func TestOllamaHealthChecker_Interface(t *testing.T) {
	checker := NewOllamaHealthChecker("")

	if checker.GetName() != "ollama" {
		t.Errorf("Expected name 'ollama', got '%s'", checker.GetName())
	}

	if checker.GetCheckInterval() != 5*time.Minute {
		t.Errorf("Expected 5m interval, got %v", checker.GetCheckInterval())
	}

	if checker.SupportsQuotaMonitoring() {
		t.Error("Ollama should not support quota monitoring")
	}

	if !checker.SupportsAutoDiscovery() {
		t.Error("Ollama should support auto-discovery")
	}
}

// --- Gemini CLI Health Checker Tests ---

func TestGeminiCLIHealthChecker_Success(t *testing.T) {
	// Skip if gemini CLI is not available
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("gemini CLI not available, skipping test")
	}

	checker := NewGeminiCLIHealthChecker("gemini")

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if status.Provider != "geminicli" {
		t.Errorf("Expected provider 'geminicli', got '%s'", status.Provider)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}

	if status.ResponseTime <= 0 {
		t.Error("Response time should be positive")
	}
}

func TestGeminiCLIHealthChecker_InvalidPath(t *testing.T) {
	checker := NewGeminiCLIHealthChecker("/nonexistent/gemini")

	ctx := context.Background()
	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected error for invalid CLI path")
	}
}

func TestGeminiCLIHealthChecker_Timeout(t *testing.T) {
	// Create a script that sleeps longer than timeout
	checker := NewGeminiCLIHealthChecker("sleep")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestGeminiCLIHealthChecker_DefaultPath(t *testing.T) {
	checker := NewGeminiCLIHealthChecker("")

	if checker.cliPath != "gemini" {
		t.Errorf("Expected default path 'gemini', got '%s'", checker.cliPath)
	}
}

func TestGeminiCLIHealthChecker_Interface(t *testing.T) {
	checker := NewGeminiCLIHealthChecker("")

	if checker.GetName() != "geminicli" {
		t.Errorf("Expected name 'geminicli', got '%s'", checker.GetName())
	}

	if checker.GetCheckInterval() != 5*time.Minute {
		t.Errorf("Expected 5m interval, got %v", checker.GetCheckInterval())
	}

	if checker.SupportsQuotaMonitoring() {
		t.Error("Gemini CLI should not support quota monitoring")
	}

	if checker.SupportsAutoDiscovery() {
		t.Error("Gemini CLI should not support auto-discovery")
	}
}

// --- Claude CLI Health Checker Tests ---

func TestClaudeCLIHealthChecker_Success(t *testing.T) {
	// Skip if claude CLI is not available
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available, skipping test")
	}

	checker := NewClaudeCLIHealthChecker("claude")

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if status.Provider != "claudecli" {
		t.Errorf("Expected provider 'claudecli', got '%s'", status.Provider)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}

	if status.ResponseTime <= 0 {
		t.Error("Response time should be positive")
	}
}

func TestClaudeCLIHealthChecker_InvalidPath(t *testing.T) {
	checker := NewClaudeCLIHealthChecker("/nonexistent/claude")

	ctx := context.Background()
	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected error for invalid CLI path")
	}
}

func TestClaudeCLIHealthChecker_Timeout(t *testing.T) {
	// Create a script that sleeps longer than timeout
	checker := NewClaudeCLIHealthChecker("sleep")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := checker.Check(ctx)

	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestClaudeCLIHealthChecker_DefaultPath(t *testing.T) {
	checker := NewClaudeCLIHealthChecker("")

	if checker.cliPath != "claude" {
		t.Errorf("Expected default path 'claude', got '%s'", checker.cliPath)
	}
}

func TestClaudeCLIHealthChecker_Interface(t *testing.T) {
	checker := NewClaudeCLIHealthChecker("")

	if checker.GetName() != "claudecli" {
		t.Errorf("Expected name 'claudecli', got '%s'", checker.GetName())
	}

	if checker.GetCheckInterval() != 5*time.Minute {
		t.Errorf("Expected 5m interval, got %v", checker.GetCheckInterval())
	}

	if checker.SupportsQuotaMonitoring() {
		t.Error("Claude CLI should not support quota monitoring")
	}

	if checker.SupportsAutoDiscovery() {
		t.Error("Claude CLI should not support auto-discovery")
	}
}

// --- API Health Checker Tests ---

func TestAPIHealthChecker_Success(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Expected 'Bearer test-key', got '%s'", auth)
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
	}))
	defer server.Close()

	checker := &APIHealthChecker{
		name:       "test-api",
		apiURL:     server.URL,
		apiKey:     "test-key",
		authHeader: "Authorization",
		authPrefix: "Bearer ",
	}

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if status.Provider != "test-api" {
		t.Errorf("Expected provider 'test-api', got '%s'", status.Provider)
	}

	if status.Status != StatusHealthy {
		t.Errorf("Expected status healthy, got %s", status.Status)
	}
}

func TestAPIHealthChecker_Unauthorized(t *testing.T) {
	// Create mock server that returns 401
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	checker := &APIHealthChecker{
		name:       "test-api",
		apiURL:     server.URL,
		apiKey:     "invalid-key",
		authHeader: "Authorization",
		authPrefix: "Bearer ",
	}

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check should not error: %v", err)
	}

	if status.Status != StatusDegraded {
		t.Errorf("Expected status degraded, got %s", status.Status)
	}

	if status.ErrorMessage != "Authentication failed" {
		t.Errorf("Expected auth error message, got '%s'", status.ErrorMessage)
	}
}

func TestAPIHealthChecker_ServerError(t *testing.T) {
	// Create mock server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := &APIHealthChecker{
		name:   "test-api",
		apiURL: server.URL,
	}

	ctx := context.Background()
	status, err := checker.Check(ctx)

	if err != nil {
		t.Fatalf("Check should not error: %v", err)
	}

	if status.Status != StatusUnavailable {
		t.Errorf("Expected status unavailable, got %s", status.Status)
	}
}

func TestAPIHealthChecker_Interface(t *testing.T) {
	checker := &APIHealthChecker{
		name:        "test-api",
		quotaHeader: "x-ratelimit",
	}

	if checker.GetName() != "test-api" {
		t.Errorf("Expected name 'test-api', got '%s'", checker.GetName())
	}

	if checker.GetCheckInterval() != 5*time.Minute {
		t.Errorf("Expected 5m interval, got %v", checker.GetCheckInterval())
	}

	if !checker.SupportsQuotaMonitoring() {
		t.Error("Should support quota monitoring when quotaHeader is set")
	}

	if checker.SupportsAutoDiscovery() {
		t.Error("Should not support auto-discovery")
	}
}

// --- Specific API Checker Tests ---

func TestNewGeminiAPIHealthChecker(t *testing.T) {
	checker := NewGeminiAPIHealthChecker("test-key")

	if checker.GetName() != "gemini" {
		t.Errorf("Expected name 'gemini', got '%s'", checker.GetName())
	}
}

func TestNewClaudeAPIHealthChecker(t *testing.T) {
	checker := NewClaudeAPIHealthChecker("test-key")

	if checker.GetName() != "claude" {
		t.Errorf("Expected name 'claude', got '%s'", checker.GetName())
	}
}

func TestNewOpenAIHealthChecker(t *testing.T) {
	checker := NewOpenAIHealthChecker("test-key")

	if checker.GetName() != "openai" {
		t.Errorf("Expected name 'openai', got '%s'", checker.GetName())
	}
}

// --- Benchmark Tests ---

func BenchmarkOllamaHealthChecker(b *testing.B) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"models": []interface{}{
				map[string]string{"name": "llama2"},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	checker := NewOllamaHealthChecker(server.URL)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := checker.Check(ctx)
		if err != nil {
			b.Fatalf("Check failed: %v", err)
		}
	}
}

func BenchmarkAPIHealthChecker(b *testing.B) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &APIHealthChecker{
		name:   "test-api",
		apiURL: server.URL,
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := checker.Check(ctx)
		if err != nil {
			b.Fatalf("Check failed: %v", err)
		}
	}
}
