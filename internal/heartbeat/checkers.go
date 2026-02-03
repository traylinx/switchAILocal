package heartbeat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// --- Ollama Health Checker ---

type OllamaHealthChecker struct {
	baseURL string
	client  *http.Client
}

func NewOllamaHealthChecker(baseURL string) *OllamaHealthChecker {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaHealthChecker{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *OllamaHealthChecker) GetName() string {
	return "ollama"
}

func (c *OllamaHealthChecker) GetCheckInterval() time.Duration {
	return 5 * time.Minute
}

func (c *OllamaHealthChecker) SupportsQuotaMonitoring() bool {
	return false
}

func (c *OllamaHealthChecker) SupportsAutoDiscovery() bool {
	return true
}

func (c *OllamaHealthChecker) Check(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()

	// Check /api/tags to verify connectivity and get models
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse models to get count
	var result struct {
		Models []interface{} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &HealthStatus{
		Provider:     c.GetName(),
		Status:       StatusHealthy,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
		ModelsCount:  len(result.Models),
	}, nil
}

// --- Gemini CLI Health Checker ---

type GeminiCLIHealthChecker struct {
	cliPath string
}

func NewGeminiCLIHealthChecker(cliPath string) *GeminiCLIHealthChecker {
	if cliPath == "" {
		cliPath = "gemini"
	}
	return &GeminiCLIHealthChecker{
		cliPath: cliPath,
	}
}

func (c *GeminiCLIHealthChecker) GetName() string {
	return "geminicli"
}

func (c *GeminiCLIHealthChecker) GetCheckInterval() time.Duration {
	return 5 * time.Minute
}

func (c *GeminiCLIHealthChecker) SupportsQuotaMonitoring() bool {
	return false
}

func (c *GeminiCLIHealthChecker) SupportsAutoDiscovery() bool {
	return false
}

func (c *GeminiCLIHealthChecker) Check(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()

	// Run `gemini --version` to check if CLI is responsive
	cmd := exec.CommandContext(ctx, c.cliPath, "--version")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute gemini cli: %w", err)
	}

	return &HealthStatus{
		Provider:     c.GetName(),
		Status:       StatusHealthy,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}, nil
}

// --- Claude CLI Health Checker ---

type ClaudeCLIHealthChecker struct {
	cliPath string
}

func NewClaudeCLIHealthChecker(cliPath string) *ClaudeCLIHealthChecker {
	if cliPath == "" {
		cliPath = "claude"
	}
	return &ClaudeCLIHealthChecker{
		cliPath: cliPath,
	}
}

func (c *ClaudeCLIHealthChecker) GetName() string {
	return "claudecli"
}

func (c *ClaudeCLIHealthChecker) GetCheckInterval() time.Duration {
	return 5 * time.Minute
}

func (c *ClaudeCLIHealthChecker) SupportsQuotaMonitoring() bool {
	return false
}

func (c *ClaudeCLIHealthChecker) SupportsAutoDiscovery() bool {
	return false
}

func (c *ClaudeCLIHealthChecker) Check(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()

	// Run `claude --version` to check if CLI is responsive
	cmd := exec.CommandContext(ctx, c.cliPath, "--version")
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute claude cli: %w", err)
	}

	return &HealthStatus{
		Provider:     c.GetName(),
		Status:       StatusHealthy,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
	}, nil
}

// --- Generic API Health Checker (Base for API providers) ---

type APIHealthChecker struct {
	name        string
	apiURL      string
	apiKey      string
	authHeader  string // "Authorization" or "x-goog-api-key", etc.
	authPrefix  string // "Bearer " or "" or "key "
	quotaHeader string // Header prefix to check for quota
}

func (c *APIHealthChecker) GetName() string {
	return c.name
}

func (c *APIHealthChecker) GetCheckInterval() time.Duration {
	return 5 * time.Minute
}

func (c *APIHealthChecker) SupportsQuotaMonitoring() bool {
	return c.quotaHeader != ""
}

func (c *APIHealthChecker) SupportsAutoDiscovery() bool {
	return false
}

func (c *APIHealthChecker) Check(ctx context.Context) (*HealthStatus, error) {
	start := time.Now()

	// Create a minimal request to check connectivity and auth
	// NOTE: This is a simplified check. In production, might want to list models or do a cheap generation.
	// For now, we'll try to list models if the API supports standard list endpoints,
	// or validatethe API key via a dedicated endpoint if available.
	// Assuming OpenAI-compatible or similar structure for now for simplicity in this base implementation,
	// but can be overridden.

	// For this implementation, we will perform a very simple request.
	// Ideally we want to hit an endpoint that returns 200 OK and headers.

	req, err := http.NewRequestWithContext(ctx, "GET", c.apiURL, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set(c.authHeader, c.authPrefix+c.apiKey)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// We might get 401/403 if key is bad, or 200 if good.
	// 404 might also happen if URL is base.
	// We consider 200, 404 (connected but bad path), 400 (bad request) as "Alive but maybe misconfigured"
	// but 401/403 is "Healthy connection, but auth failed" -> which strictly speaking means the provider is reachable.
	// However, for "Health", we want it to be usable.

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &HealthStatus{
			Provider:     c.GetName(),
			Status:       StatusDegraded,
			LastCheck:    time.Now(),
			ResponseTime: time.Since(start),
			ErrorMessage: "Authentication failed",
		}, nil
	}

	if resp.StatusCode >= 500 {
		return &HealthStatus{
			Provider:     c.GetName(),
			Status:       StatusUnavailable,
			LastCheck:    time.Now(),
			ResponseTime: time.Since(start),
			ErrorMessage: fmt.Sprintf("Server error: %d", resp.StatusCode),
		}, nil
	}

	status := &HealthStatus{
		Provider:     c.GetName(),
		Status:       StatusHealthy,
		LastCheck:    time.Now(),
		ResponseTime: time.Since(start),
		Metadata:     make(map[string]interface{}),
	}

	// Check Quota/RateLimit headers if applicable
	if c.SupportsQuotaMonitoring() {
		// Example: x-ratelimit-remaining / x-ratelimit-limit
		// This needs to be tailored per provider in real implementation
		// For now simple generic logic can be placed here or in specific implementations
		c.extractQuota(resp, status)
	}

	return status, nil
}

func (c *APIHealthChecker) extractQuota(resp *http.Response, status *HealthStatus) {
	// Generic logic placeholder.
	// Real implementation needs to look at specific headers like:
	// OpenAI: x-ratelimit-remaining-requests, x-ratelimit-limit-requests
	// Anthropic: anthropic-ratelimit-requests-remaining
	// Gemini: x-goog-quota...

	// Pass headers to metadata for specialized parsers
	for k, v := range resp.Header {
		if strings.HasPrefix(strings.ToLower(k), strings.ToLower(c.quotaHeader)) {
			status.Metadata[k] = v[0]
		}
	}
}

// --- Specific API Checkers (Wrappers around APIHealthChecker or custom) ---

func NewGeminiAPIHealthChecker(apiKey string) ProviderHealthChecker {
	// URL for listing models is a good health check
	return &APIHealthChecker{
		name:        "gemini",
		apiURL:      "https://generativelanguage.googleapis.com/v1beta/models?key=" + apiKey,
		apiKey:      "",               // key is in URL for Gemini usually, or header
		authHeader:  "x-goog-api-key", // redundancy
		authPrefix:  "",
		quotaHeader: "x-goog-quota",
	}
}

func NewClaudeAPIHealthChecker(apiKey string) ProviderHealthChecker {
	// Anthropic doesn't have a simple GET models endpoint that is public/easy without body?
	// Actually v1/models is not standard.
	// For now we might just check strict connectivity or a "hey" message.
	// We'll trust the base URL connectivity for now.
	return &APIHealthChecker{
		name:        "claude",
		apiURL:      "https://api.anthropic.com/v1/models", // Hypothetical or check version
		apiKey:      apiKey,
		authHeader:  "x-api-key",
		authPrefix:  "",
		quotaHeader: "anthropic-ratelimit",
	}
}

func NewOpenAIHealthChecker(apiKey string) ProviderHealthChecker {
	return &APIHealthChecker{
		name:        "openai",
		apiURL:      "https://api.openai.com/v1/models",
		apiKey:      apiKey,
		authHeader:  "Authorization",
		authPrefix:  "Bearer ",
		quotaHeader: "x-ratelimit",
	}
}

// --- SwitchAI API Health Checker ---

func NewSwitchAIHealthChecker(apiKey, baseURL string) ProviderHealthChecker {
	if baseURL == "" {
		baseURL = "https://switchai.traylinx.com/v1"
	}
	return &APIHealthChecker{
		name:        "switchai",
		apiURL:      baseURL + "/models",
		apiKey:      apiKey,
		authHeader:  "Authorization",
		authPrefix:  "Bearer ",
		quotaHeader: "x-ratelimit",
	}
}

// --- Groq API Health Checker ---

func NewGroqHealthChecker(apiKey, baseURL string) ProviderHealthChecker {
	if baseURL == "" {
		baseURL = "https://api.groq.com/openai/v1"
	}
	return &APIHealthChecker{
		name:        "groq",
		apiURL:      baseURL + "/models",
		apiKey:      apiKey,
		authHeader:  "Authorization",
		authPrefix:  "Bearer ",
		quotaHeader: "x-ratelimit",
	}
}

// --- Generic OpenAI-Compatible Health Checker ---

func NewOpenAICompatibilityHealthChecker(name, baseURL, apiKey string) ProviderHealthChecker {
	return &APIHealthChecker{
		name:        name,
		apiURL:      baseURL + "/models",
		apiKey:      apiKey,
		authHeader:  "Authorization",
		authPrefix:  "Bearer ",
		quotaHeader: "x-ratelimit",
	}
}
