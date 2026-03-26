package autoroute

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitHeaderConfig maps provider-specific HTTP header names to
// rate-limit semantics. Each provider uses slightly different header
// names, so we normalise them into a common struct.
type RateLimitHeaderConfig struct {
	RequestLimit     string
	RequestRemaining string
	RequestReset     string
	TokenLimit       string
	TokenRemaining   string
	TokenReset       string
	RetryAfter       string
}

// RateLimitSnapshot is the normalised quota state extracted from
// a single HTTP response.
type RateLimitSnapshot struct {
	Provider string

	// Request-level limits
	RequestLimit     int
	RequestRemaining int
	RequestReset     time.Time

	// Token-level limits
	TokenLimit     int
	TokenRemaining int
	TokenReset     time.Time

	// Retry-After (seconds or absolute time)
	RetryAfterSec int

	// Derived health metric (0.0 = exhausted, 1.0 = full)
	QuotaHealth float64

	// Whether any rate-limit headers were detected at all
	Detected bool
}

// rateLimitHeaderMap contains the per-provider header mappings as defined
// in the Phase 2 spec (09_AUTO_DISCOVERY_AND_MONITORING §5.1).
var rateLimitHeaderMap = map[string]RateLimitHeaderConfig{
	"claude": {
		RequestLimit:     "anthropic-ratelimit-requests-limit",
		RequestRemaining: "anthropic-ratelimit-requests-remaining",
		RequestReset:     "anthropic-ratelimit-requests-reset",
		TokenLimit:       "anthropic-ratelimit-tokens-limit",
		TokenRemaining:   "anthropic-ratelimit-tokens-remaining",
		TokenReset:       "anthropic-ratelimit-tokens-reset",
	},
	"claudecli": {
		RequestLimit:     "anthropic-ratelimit-requests-limit",
		RequestRemaining: "anthropic-ratelimit-requests-remaining",
		RequestReset:     "anthropic-ratelimit-requests-reset",
		TokenLimit:       "anthropic-ratelimit-tokens-limit",
		TokenRemaining:   "anthropic-ratelimit-tokens-remaining",
		TokenReset:       "anthropic-ratelimit-tokens-reset",
	},
	"openai": {
		RequestLimit:     "x-ratelimit-limit-requests",
		RequestRemaining: "x-ratelimit-remaining-requests",
		TokenLimit:       "x-ratelimit-limit-tokens",
		TokenRemaining:   "x-ratelimit-remaining-tokens",
		RetryAfter:       "retry-after",
	},
	"codex": {
		RequestLimit:     "x-ratelimit-limit-requests",
		RequestRemaining: "x-ratelimit-remaining-requests",
		TokenLimit:       "x-ratelimit-limit-tokens",
		TokenRemaining:   "x-ratelimit-remaining-tokens",
		RetryAfter:       "retry-after",
	},
	"groq": {
		RequestLimit:     "x-ratelimit-limit-requests",
		RequestRemaining: "x-ratelimit-remaining-requests",
		TokenLimit:       "x-ratelimit-limit-tokens",
		TokenRemaining:   "x-ratelimit-remaining-tokens",
	},
	"gemini": {
		RetryAfter: "retry-after",
	},
	"geminicli": {
		RetryAfter: "retry-after",
	},
}

// ParseRateLimitHeaders extracts rate-limit quota data from an HTTP
// response according to the provider's known header scheme.
// If no relevant headers are found, Detected will be false.
func ParseRateLimitHeaders(provider string, headers http.Header) RateLimitSnapshot {
	snap := RateLimitSnapshot{
		Provider:    provider,
		QuotaHealth: 1.0,
	}

	if headers == nil {
		return snap
	}

	providerKey := strings.ToLower(provider)

	// Try provider-specific mapping first
	cfg, known := rateLimitHeaderMap[providerKey]
	if !known {
		// Fallback: try OpenAI-style headers (most OpenAI-compat providers use this)
		cfg = rateLimitHeaderMap["openai"]
	}

	// Parse request-level limits
	if v := headerInt(headers, cfg.RequestLimit); v > 0 {
		snap.RequestLimit = v
		snap.Detected = true
	}
	if v := headerInt(headers, cfg.RequestRemaining); v >= 0 && cfg.RequestRemaining != "" {
		snap.RequestRemaining = v
		snap.Detected = true
	}
	if v := headerTime(headers, cfg.RequestReset); !v.IsZero() {
		snap.RequestReset = v
		snap.Detected = true
	}

	// Parse token-level limits
	if v := headerInt(headers, cfg.TokenLimit); v > 0 {
		snap.TokenLimit = v
		snap.Detected = true
	}
	if v := headerInt(headers, cfg.TokenRemaining); v >= 0 && cfg.TokenRemaining != "" {
		snap.TokenRemaining = v
		snap.Detected = true
	}
	if v := headerTime(headers, cfg.TokenReset); !v.IsZero() {
		snap.TokenReset = v
		snap.Detected = true
	}

	// Parse Retry-After
	if cfg.RetryAfter != "" {
		if v := headerInt(headers, cfg.RetryAfter); v > 0 {
			snap.RetryAfterSec = v
			snap.Detected = true
		}
	}

	// Compute QuotaHealth
	snap.QuotaHealth = computeQuotaHealth(snap)

	return snap
}

// computeQuotaHealth calculates a 0.0-1.0 quota health score.
// If both request and token limits are available, we use the minimum
// (most constrained) of the two ratios. If neither is available,
// the score stays at 1.0 (unknown = optimistic).
func computeQuotaHealth(s RateLimitSnapshot) float64 {
	if !s.Detected {
		return 1.0
	}

	// Explicit exhaustion signal
	if s.RetryAfterSec > 0 {
		return 0.0
	}

	health := 1.0
	computed := false

	if s.RequestLimit > 0 {
		ratio := float64(s.RequestRemaining) / float64(s.RequestLimit)
		if ratio < health {
			health = ratio
		}
		computed = true
	}

	if s.TokenLimit > 0 {
		ratio := float64(s.TokenRemaining) / float64(s.TokenLimit)
		if ratio < health {
			health = ratio
		}
		computed = true
	}

	if !computed {
		return 1.0
	}

	// Clamp
	if health < 0 {
		health = 0
	}
	if health > 1 {
		health = 1
	}
	return health
}

// headerInt reads a header value as an integer, returning -1 if missing or unparseable.
func headerInt(h http.Header, key string) int {
	if key == "" {
		return -1
	}
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

// headerTime reads a header value as either an RFC3339 timestamp or
// an integer (seconds from now), returning zero time if unparseable.
func headerTime(h http.Header, key string) time.Time {
	if key == "" {
		return time.Time{}
	}
	v := strings.TrimSpace(h.Get(key))
	if v == "" {
		return time.Time{}
	}

	// Try RFC3339 first (Anthropic uses this format)
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}

	// Try integer seconds
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Now().Add(time.Duration(secs) * time.Second)
	}

	return time.Time{}
}
