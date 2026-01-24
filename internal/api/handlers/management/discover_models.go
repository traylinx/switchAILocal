package management

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type DiscoverModelsRequest struct {
	ModelsURL string            `json:"modelsUrl"`
	APIKey    string            `json:"apiKey"`
	BaseURL   string            `json:"baseUrl"`
	ProxyURL  string            `json:"proxyUrl"`
	Headers   map[string]string `json:"headers"`
}

type ModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
}

func (h *Handler) DiscoverModels(c *gin.Context) {
	var req DiscoverModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": err.Error()})
		return
	}

	if req.ModelsURL == "" {
		// Try to infer models URL from BaseURL if possible
		if req.BaseURL != "" {
			trimmed := strings.TrimSuffix(req.BaseURL, "/")
			if strings.Contains(req.BaseURL, "ollama") || strings.Contains(req.BaseURL, "localhost:11434") {
				req.ModelsURL = trimmed + "/api/tags"
			} else {
				// Default to OpenAI standard
				req.ModelsURL = trimmed + "/models"
			}
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing_url", "message": "Models URL or Base URL is required"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	// Configure Proxy
	if req.ProxyURL != "" {
		proxyURL, err := url.Parse(req.ProxyURL)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_proxy", "message": "Invalid Proxy URL"})
			return
		}
		client.Transport = &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // Allow insecure for testing/local proxies? Maybe strictly verifying is better but for devs...
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "GET", req.ModelsURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "request_creation_failed", "message": err.Error()})
		return
	}

	// Set Headers
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream_error", "message": fmt.Sprintf("Failed to contact provider: %v", err)})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "read_error", "message": "Failed to read response body"})
		return
	}

	if resp.StatusCode >= 400 {
		c.JSON(resp.StatusCode, gin.H{
			"error":   "provider_error",
			"message": fmt.Sprintf("Provider returned error %d: %s", resp.StatusCode, string(body)),
		})
		return
	}

	// Parse Response
	// Try OpenAI format first: { data: [...] }
	type OpenAIModel struct {
		ID string `json:"id"`
	}
	type OpenAIResponse struct {
		Data []OpenAIModel `json:"data"`
	}

	// Try Ollama format: { models: [...] }
	type OllamaModel struct {
		Name string `json:"name"`
		// ... other fields
	}
	type OllamaResponse struct {
		Models []OllamaModel `json:"models"`
	}

	var models []ModelInfo

	// Attempt parse as OpenAI
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(body, &openaiResp); err == nil && len(openaiResp.Data) > 0 {
		for _, m := range openaiResp.Data {
			models = append(models, ModelInfo{ID: m.ID})
		}
	} else {
		// Attempt parse as Ollama
		var ollamaResp OllamaResponse
		if err := json.Unmarshal(body, &ollamaResp); err == nil && len(ollamaResp.Models) > 0 {
			for _, m := range ollamaResp.Models {
				models = append(models, ModelInfo{ID: m.Name})
			}
		} else {
			// Fallback: maybe array?
			log.Warnf("Failed to parse models response from %s. Body snippet: %s", req.ModelsURL, string(body))
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "parse_error",
				"message": "Could not parse models response. Ensure provider follows OpenAI or Ollama format.",
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"models":  models,
	})
}
