package autoroute

import (
	"net/http"
	"testing"
)

func TestParseRateLimitHeaders_Claude(t *testing.T) {
	h := http.Header{}
	h.Set("anthropic-ratelimit-requests-limit", "50")
	h.Set("anthropic-ratelimit-requests-remaining", "10")
	h.Set("anthropic-ratelimit-tokens-limit", "100000")
	h.Set("anthropic-ratelimit-tokens-remaining", "25000")

	snap := ParseRateLimitHeaders("claude", h)
	if !snap.Detected {
		t.Fatal("Expected headers to be detected")
	}
	if snap.RequestLimit != 50 {
		t.Errorf("Expected request limit 50, got %d", snap.RequestLimit)
	}
	if snap.RequestRemaining != 10 {
		t.Errorf("Expected request remaining 10, got %d", snap.RequestRemaining)
	}
	if snap.TokenLimit != 100000 {
		t.Errorf("Expected token limit 100000, got %d", snap.TokenLimit)
	}
	if snap.TokenRemaining != 25000 {
		t.Errorf("Expected token remaining 25000, got %d", snap.TokenRemaining)
	}

	// QuotaHealth = min(10/50, 25000/100000) = min(0.2, 0.25) = 0.2
	if snap.QuotaHealth < 0.19 || snap.QuotaHealth > 0.21 {
		t.Errorf("Expected QuotaHealth ~0.2, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_OpenAI(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "500")
	h.Set("x-ratelimit-remaining-requests", "490")
	h.Set("x-ratelimit-limit-tokens", "1000000")
	h.Set("x-ratelimit-remaining-tokens", "999000")

	snap := ParseRateLimitHeaders("openai", h)
	if !snap.Detected {
		t.Fatal("Expected headers to be detected")
	}
	if snap.RequestLimit != 500 {
		t.Errorf("Expected request limit 500, got %d", snap.RequestLimit)
	}
	// QuotaHealth = min(490/500, 999000/1000000) = min(0.98, 0.999) = 0.98
	if snap.QuotaHealth < 0.97 || snap.QuotaHealth > 0.99 {
		t.Errorf("Expected QuotaHealth ~0.98, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_Groq(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "30")
	h.Set("x-ratelimit-remaining-requests", "0")
	h.Set("x-ratelimit-limit-tokens", "15000")
	h.Set("x-ratelimit-remaining-tokens", "5000")

	snap := ParseRateLimitHeaders("groq", h)
	if !snap.Detected {
		t.Fatal("Expected headers to be detected")
	}
	// QuotaHealth = min(0/30, 5000/15000) = min(0, 0.33) = 0.0
	if snap.QuotaHealth != 0.0 {
		t.Errorf("Expected QuotaHealth 0.0, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_RetryAfter(t *testing.T) {
	h := http.Header{}
	h.Set("retry-after", "30")

	snap := ParseRateLimitHeaders("gemini", h)
	if !snap.Detected {
		t.Fatal("Expected headers to be detected")
	}
	if snap.RetryAfterSec != 30 {
		t.Errorf("Expected RetryAfterSec 30, got %d", snap.RetryAfterSec)
	}
	if snap.QuotaHealth != 0.0 {
		t.Errorf("Expected QuotaHealth 0.0 when Retry-After is set, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_UnknownProvider_FallbackToOpenAI(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "100")
	h.Set("x-ratelimit-remaining-requests", "50")

	snap := ParseRateLimitHeaders("alibaba", h)
	if !snap.Detected {
		t.Fatal("Unknown providers should fallback to OpenAI header format")
	}
	if snap.RequestLimit != 100 || snap.RequestRemaining != 50 {
		t.Errorf("Fallback parsing failed: limit=%d, remaining=%d", snap.RequestLimit, snap.RequestRemaining)
	}
	// QuotaHealth = 50/100 = 0.5
	if snap.QuotaHealth < 0.49 || snap.QuotaHealth > 0.51 {
		t.Errorf("Expected QuotaHealth ~0.5, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_NoHeaders(t *testing.T) {
	snap := ParseRateLimitHeaders("openai", http.Header{})
	if snap.Detected {
		t.Error("Expected Detected=false for empty headers")
	}
	if snap.QuotaHealth != 1.0 {
		t.Errorf("Expected QuotaHealth 1.0 for no headers, got %f", snap.QuotaHealth)
	}
}

func TestParseRateLimitHeaders_NilHeaders(t *testing.T) {
	snap := ParseRateLimitHeaders("openai", nil)
	if snap.Detected {
		t.Error("Expected Detected=false for nil headers")
	}
	if snap.QuotaHealth != 1.0 {
		t.Errorf("Expected QuotaHealth 1.0 for nil headers, got %f", snap.QuotaHealth)
	}
}

func TestComputeQuotaHealth_FullQuota(t *testing.T) {
	snap := RateLimitSnapshot{
		Detected:         true,
		RequestLimit:     100,
		RequestRemaining: 100,
		TokenLimit:       1000,
		TokenRemaining:   1000,
	}
	h := computeQuotaHealth(snap)
	if h != 1.0 {
		t.Errorf("Expected QuotaHealth 1.0, got %f", h)
	}
}

func TestComputeQuotaHealth_HalfQuota(t *testing.T) {
	snap := RateLimitSnapshot{
		Detected:         true,
		RequestLimit:     100,
		RequestRemaining: 50,
		TokenLimit:       1000,
		TokenRemaining:   500,
	}
	h := computeQuotaHealth(snap)
	if h < 0.49 || h > 0.51 {
		t.Errorf("Expected QuotaHealth ~0.5, got %f", h)
	}
}

func TestComputeQuotaHealth_RetryAfter(t *testing.T) {
	snap := RateLimitSnapshot{
		Detected:      true,
		RetryAfterSec: 60,
	}
	h := computeQuotaHealth(snap)
	if h != 0.0 {
		t.Errorf("Expected QuotaHealth 0.0 with RetryAfter, got %f", h)
	}
}
