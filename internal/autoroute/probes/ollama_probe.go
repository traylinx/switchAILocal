package probes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/traylinx/switchAILocal/internal/autoroute"
)

// OllamaProber probes a local or remote Ollama instance via /api/tags
type OllamaProber struct {
	providerName string // e.g. "ollama"
	baseURL      string // e.g. "http://localhost:11434"
}

// NewOllamaProber creates a prober for Ollama
func NewOllamaProber(providerName, baseURL string) *OllamaProber {
	if baseURL == "" {
		baseURL = "http://localhost:11434" // Standard default
	}
	return &OllamaProber{
		providerName: providerName,
		baseURL:      baseURL,
	}
}

// Name returns the provider identifier
func (p *OllamaProber) Name() string {
	return p.providerName
}

// Probe executes a fast /api/tags request to see if the daemon is alive
func (p *OllamaProber) Probe(ctx context.Context) autoroute.ProbeResult {
	start := time.Now()
	result := autoroute.ProbeResult{
		Provider: p.providerName,
		AuthType: autoroute.AuthTypeLocal,
		SubscriptionInfo: &autoroute.SubscriptionInfo{
			Tier:   "local", // Free/local tier
			Source: "inferred",
		},
	}

	url := p.baseURL + "/api/tags"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		result.Available = false
		result.ProbeError = err
		return result
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)

	result.Latency = time.Since(start)

	if err != nil {
		result.Available = false
		result.ProbeError = err
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		result.Available = true
		
		// Parse local models
		var responseData struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&responseData); err == nil {
			for _, m := range responseData.Models {
				result.Models = append(result.Models, autoroute.DiscoveredModel{ID: m.Name})
			}
		}
	} else {
		result.Available = false
	}

	return result
}
