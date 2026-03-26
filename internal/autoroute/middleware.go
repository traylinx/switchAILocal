package autoroute

import (
	"context"
	"strings"

	log "github.com/sirupsen/logrus"
)

// AutoRoutingResult holds the outcome of an auto-routing resolution,
// consumable by the handler pipeline.
type AutoRoutingResult struct {
	// ResolvedModel is the model chosen by the auto-router (e.g., "geminicli:gemini-3.1-pro")
	ResolvedModel string

	// Providers is the ordered list of providers to try (winner first, then fallbacks)
	Providers []string

	// Intent is the classified or hinted intent (e.g., "coding")
	Intent string

	// Complexity is the estimated prompt complexity (0.0-1.0)
	Complexity float64

	// Decision is the full routing rationale, retained for Lab telemetry
	Decision *RoutingDecision

	// WasAutoRouted indicates this request was handled by auto-routing
	WasAutoRouted bool
}

// IsAutoModel returns true if the model string signals auto-routing
// (i.e., "auto", "", or "auto:*").
func IsAutoModel(model string) bool {
	if model == "" || model == "auto" {
		return true
	}
	return strings.HasPrefix(model, "auto:")
}

// ResolveAutoRequest runs the auto-routing pipeline for a given request.
// It takes the model name (which may contain an intent hint like "auto:coding"),
// the user content for complexity estimation, and the resolver instance.
//
// If the resolver is nil or routing is disabled, it returns nil (caller should
// fall back to legacy logic).
func ResolveAutoRequest(ctx context.Context, modelName string, content string, resolver *AutoResolver) *AutoRoutingResult {
	if resolver == nil {
		return nil
	}

	// Parse intent hint from model name
	_, intentHint := ParseAutoModelHint(modelName)

	// Build the routing request
	// AvailableModels are populated by discovery/health monitor dynamically
	req := &RoutingRequest{
		Content:         content,
		IntentHint:      intentHint,
		AvailableModels: resolver.GetCandidates(),
	}

	decision, err := resolver.Resolve(ctx, req)
	if err != nil {
		// Auto-routing failed (disabled, no candidates, timeout) — log and fall back
		if err != ErrAutoRoutingDisabled {
			log.WithField("component", "autoroute").
				WithError(err).
				Debug("auto-routing resolution failed, falling back to legacy")
		}
		return nil
	}

	// Build the provider list (winner + fallbacks)
	providers := []string{ExtractProvider(decision.SelectedModel)}
	for _, fb := range decision.FallbackChain {
		providers = append(providers, fb.Provider)
	}

	return &AutoRoutingResult{
		ResolvedModel: decision.SelectedModel,
		Providers:     providers,
		Intent:        decision.Intent,
		Complexity:    decision.EstimatedComplexity,
		Decision:      decision,
		WasAutoRouted: true,
	}
}

// ExtractContentFromRawJSON returns a text sample from a raw JSON request body
// for complexity estimation. This is intentionally coarse — it does NOT parse
// the "content" field from JSON. Instead it returns the raw string (or its tail
// for large payloads). EstimateComplexity then uses len/4 as a token proxy.
//
// NOTE: This over-estimates token count for JSON-heavy payloads (system prompts,
// tool definitions, etc.) but is acceptable because EstimateComplexity uses
// wide buckets (0.1/0.3/0.5/0.7/0.9), making the coarseness tolerable.
func ExtractContentFromRawJSON(rawJSON []byte) string {
	s := string(rawJSON)
	if len(s) > 8000 {
		// For very large payloads, sample the tail (more likely to contain the main prompt)
		s = s[len(s)-8000:]
	}
	return s
}
