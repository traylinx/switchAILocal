// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package management

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/constant"
)

// TestProviderRequest defines the payload for testing a provider configuration.
type TestProviderRequest struct {
	ProviderID string                 `json:"provider_id"`
	Category   string                 `json:"category"`
	Config     map[string]interface{} `json:"config"`
}

// TestProvider attempts to verify if a provider is correctly configured and reachable.
func (h *Handler) TestProvider(c *gin.Context) {
	var req TestProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	// ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	// defer cancel()

	category := strings.ToLower(req.Category)
	switch {
	case strings.Contains(category, "cli"):
		h.testCLIProvider(c, req.ProviderID)
	case strings.Contains(category, "cloud"):
		h.testCloudProvider(c, req.ProviderID, req.Config)
	case strings.Contains(category, "local"):
		h.testLocalProvider(c, req.ProviderID, req.Config)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_category", "message": fmt.Sprintf("Unsupported category: %s", req.Category)})
	}
}

func (h *Handler) testCLIProvider(c *gin.Context, providerID string) {
	cmdName := ""
	switch providerID {
	case "claude-cli", constant.ClaudeCLI:
		cmdName = "claude"
	case "gemini-cli", constant.GeminiCLI:
		cmdName = "gemini"
	case "vibe-cli", constant.VibeCLI:
		cmdName = "vibe"
	case "codex-cli", constant.Codex:
		cmdName = "codex"
	case "opencode-cli", constant.OpenCode:
		cmdName = "opencode"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown_cli", "message": fmt.Sprintf("Unknown CLI tool: %s", providerID)})
		return
	}

	path, err := exec.LookPath(cmdName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Command '%s' not found in PATH. Please install it or check your environment.", cmdName),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": fmt.Sprintf("Command '%s' found at %s. Ready to use.", cmdName, path),
	})
}

func (h *Handler) testLocalProvider(c *gin.Context, providerID string, config map[string]interface{}) {
	baseURL, _ := config["base-url"].(string)
	if baseURL == "" {
		// Fallback to current config if not provided in request
		switch providerID {
		case "ollama":
			baseURL = h.cfg.Ollama.BaseURL
		case "lmstudio":
			baseURL = h.cfg.LMStudio.BaseURL
		case "opencode":
			baseURL = h.cfg.OpenCode.BaseURL
		}
	}

	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing_config", "message": "Base URL not provided"})
		return
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Failed to reach %s: %v", baseURL, err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": fmt.Sprintf("Successfully reached %s (Status: %d)", baseURL, resp.StatusCode),
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": fmt.Sprintf("Provider returned error status %d at %s", resp.StatusCode, baseURL),
		})
	}
}

func (h *Handler) testCloudProvider(c *gin.Context, providerID string, config map[string]interface{}) {
	apiKey, _ := config["api-key"].(string)
	baseURL, _ := config["base-url"].(string)
	proxyURL, _ := config["proxy-url"].(string)
	modelsURL, _ := config["models-url"].(string)

	if apiKey == "" && providerID != "ollama" && providerID != "lmstudio" { // specific check?
		// for cloud, key usually required
		c.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": "API Key is required",
			"tests": gin.H{
				"apiKey": gin.H{"passed": false, "message": "Missing API Key"},
			},
		})
		return
	}

	tests := gin.H{
		"apiKey": gin.H{"passed": true, "message": "Format valid"}, // Basic format check passed if we are here
	}

	// 1. Test Base URL Reachability
	client := &http.Client{Timeout: 10 * time.Second}

	// Configure Proxy
	if proxyURL != "" {
		pURL, err := url.Parse(proxyURL)
		if err != nil {
			tests["proxy"] = gin.H{"passed": false, "message": fmt.Sprintf("Invalid Proxy URL: %v", err)}
		} else {
			client.Transport = &http.Transport{
				Proxy:           http.ProxyURL(pURL),
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			tests["proxy"] = gin.H{"passed": true, "message": "Proxy configured"}
		}
	}

	if baseURL != "" {
		// Try to reach base URL (often returns 404 or welcome message, establishing connectivity)
		req, _ := http.NewRequest("GET", baseURL, nil)
		if strings.Contains(baseURL, "openai.com") {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}

		start := time.Now()
		resp, err := client.Do(req)
		latency := time.Since(start).Milliseconds()

		if err != nil {
			tests["baseUrl"] = gin.H{"passed": false, "message": fmt.Sprintf("Unreachable: %v", err)}
		} else {
			defer resp.Body.Close()
			msg := fmt.Sprintf("Reachable (Status: %d, %dms)", resp.StatusCode, latency)
			tests["baseUrl"] = gin.H{"passed": true, "message": msg, "latency": latency}

			// If status is 401, API key is invalid
			if resp.StatusCode == 401 {
				tests["apiKey"] = gin.H{"passed": false, "message": "Unauthorized (Invalid API Key)"}
			}
		}
	} else {
		tests["baseUrl"] = gin.H{"passed": false, "message": "Base URL not provided"}
	}

	// 2. Test Models URL if provided
	if modelsURL != "" {
		req, _ := http.NewRequest("GET", modelsURL, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := client.Do(req)
		if err != nil {
			tests["modelsUrl"] = gin.H{"passed": false, "message": fmt.Sprintf("Failed to fetch models: %v", err)}
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				tests["modelsUrl"] = gin.H{"passed": true, "message": "Successfully fetched models"}
			} else {
				tests["modelsUrl"] = gin.H{"passed": false, "message": fmt.Sprintf("Failed with status %d", resp.StatusCode)}
			}
		}
	}

	// Determine overall status
	failed := false
	for _, t := range tests {
		if detail, ok := t.(gin.H); ok {
			if passed, ok := detail["passed"].(bool); ok && !passed {
				failed = true
			}
		}
	}

	status := "success"
	message := "All tests passed"
	if failed {
		status = "error"
		message = "Some tests failed"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  status,
		"message": message,
		"tests":   tests,
	})
}
