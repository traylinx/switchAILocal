package probes

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/traylinx/switchAILocal/internal/autoroute"
)

// APIProber probes standard HTTP API endpoints (OpenAI, Anthropic, SwitchAI)
type APIProber struct {
	providerName string // e.g. "openai", "claude-api", "switchai"
	baseURL      string // e.g. "https://api.openai.com/v1"
	endpoint     string // e.g. "/models"
	apiKey       string
	authHeader   string // "Authorization", "x-api-key"
}

// NewAPIProber creates a prober for a specific HTTP API provider
func NewAPIProber(providerName, baseURL, endpoint, apiKey, authHeader string) *APIProber {
	return &APIProber{
		providerName: providerName,
		baseURL:      baseURL,
		endpoint:     endpoint,
		apiKey:       apiKey,
		authHeader:   authHeader,
	}
}

// Name returns the provider identifier
func (p *APIProber) Name() string {
	return p.providerName
}

// Probe executes a lightweight HTTP GET to the models endpoint
func (p *APIProber) Probe(ctx context.Context) autoroute.ProbeResult {
	start := time.Now()
	result := autoroute.ProbeResult{
		Provider: p.providerName,
		AuthType: autoroute.AuthTypeAPIKey,
	}

	url := strings.TrimRight(p.baseURL, "/") + "/" + strings.TrimLeft(p.endpoint, "/")

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Available = false
		result.ProbeError = err
		return result
	}

	if p.apiKey != "" {
		if p.authHeader == "Authorization" {
			req.Header.Set(p.authHeader, "Bearer "+p.apiKey)
		} else {
			req.Header.Set(p.authHeader, p.apiKey)
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)

	result.Latency = time.Since(start)

	if err != nil {
		result.Available = false
		result.ProbeError = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Available = true
		
		// Parse rate limits from headers
		result.RateLimits = extractRateLimits(p.providerName, resp.Header)
		
		// Attempt to parse models if it's a known format (e.g. OpenAI /models format)
		var jsonBody map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&jsonBody); err == nil {
			if data, ok := jsonBody["data"].([]interface{}); ok {
				for _, item := range data {
					if modelMap, ok := item.(map[string]interface{}); ok {
						if id, ok := modelMap["id"].(string); ok {
							result.Models = append(result.Models, autoroute.DiscoveredModel{ID: id})
						}
					}
				}
			}
		}
	} else {
		// e.g. 401 Unauthorized prevents availability
		result.Available = false
	}

	return result
}

// extractRateLimits parses standard HTTP quota headers
func extractRateLimits(provider string, header http.Header) *autoroute.RateLimits {
	provider = strings.ToLower(provider)
	rl := &autoroute.RateLimits{}

	if strings.Contains(provider, "claude") {
		rl.RequestsPerMinute = parseIntHeader(header.Get("anthropic-ratelimit-requests-limit"))
		rl.TokensPerMinute = parseIntHeader(header.Get("anthropic-ratelimit-tokens-limit"))
		if rl.RequestsPerMinute > 0 {
			return rl
		}
	}

	// OpenAI / Groq standard
	rpm := parseIntHeader(header.Get("x-ratelimit-limit-requests"))
	tpm := parseIntHeader(header.Get("x-ratelimit-limit-tokens"))

	if rpm > 0 || tpm > 0 {
		rl.RequestsPerMinute = rpm
		rl.TokensPerMinute = tpm
		return rl
	}

	return nil
}

func parseIntHeader(val string) int {
	if val == "" {
		return 0
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return i
}
