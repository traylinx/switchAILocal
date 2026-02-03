package heartbeat

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// QuotaStatusType represents the status of a quota.
type QuotaStatusType string

const (
	QuotaOK       QuotaStatusType = "ok"
	QuotaWarning  QuotaStatusType = "warning"  // > 80%
	QuotaCritical QuotaStatusType = "critical" // > 95%
	QuotaExceeded QuotaStatusType = "exceeded" // 100%
)

// QuotaInfo contains extracted quota information.
type QuotaInfo struct {
	Used      float64
	Limit     float64
	ResetTime time.Time
	Found     bool
}

// ExtractQuotaFromHeaders attempts to extract quota information from response headers
// based on the provider type.
func ExtractQuotaFromHeaders(provider string, headers http.Header) QuotaInfo {
	provider = strings.ToLower(provider)

	if strings.Contains(provider, "openai") {
		return extractOpenAIQuota(headers)
	} else if strings.Contains(provider, "anthropic") || strings.Contains(provider, "claude") {
		return extractClaudeQuota(headers)
	}

	return QuotaInfo{Found: false}
}

func extractOpenAIQuota(headers http.Header) QuotaInfo {
	info := QuotaInfo{}

	// OpenAI uses x-ratelimit-remaining-* and x-ratelimit-limit-*
	// We'll prioritize requests over tokens for simple health check

	limitStr := headers.Get("x-ratelimit-limit-requests")
	remainingStr := headers.Get("x-ratelimit-remaining-requests")
	resetStr := headers.Get("x-ratelimit-reset-requests")

	if limitStr != "" && remainingStr != "" {
		limit, err1 := strconv.ParseFloat(limitStr, 64)
		remaining, err2 := strconv.ParseFloat(remainingStr, 64)

		if err1 == nil && err2 == nil && limit > 0 {
			info.Limit = limit
			info.Used = limit - remaining
			info.Found = true
		}
	}

	if resetStr != "" {
		// OpenAI reset is often in seconds (e.g. "20ms", "6s", "1m")
		// Log but don't parse yet for MVP
		log.Debugf("OpenAI quota reset header found: %s", resetStr)
	}

	return info
}

func extractClaudeQuota(headers http.Header) QuotaInfo {
	info := QuotaInfo{}

	// Anthropic headers: anthropic-ratelimit-requests-limit, anthropic-ratelimit-requests-remaining

	limitStr := headers.Get("anthropic-ratelimit-requests-limit")
	remainingStr := headers.Get("anthropic-ratelimit-requests-remaining")
	resetStr := headers.Get("anthropic-ratelimit-requests-reset") // Unix timestamp?

	if limitStr != "" && remainingStr != "" {
		limit, err1 := strconv.ParseFloat(limitStr, 64)
		remaining, err2 := strconv.ParseFloat(remainingStr, 64)

		if err1 == nil && err2 == nil && limit > 0 {
			info.Limit = limit
			info.Used = limit - remaining
			info.Found = true
		}
	}

	if resetStr != "" {
		// Attempt to parse validation
		if validReset, err := time.Parse(time.RFC3339, resetStr); err == nil {
			info.ResetTime = validReset
		}
	}

	return info
}

// CalculateQuotaStatus determines the status based on usage and limit.
func CalculateQuotaStatus(used, limit float64, warningThreshold, criticalThreshold float64) QuotaStatusType {
	if limit <= 0 {
		return QuotaOK
	}

	ratio := used / limit

	if ratio >= 1.0 {
		return QuotaExceeded
	}
	if ratio >= criticalThreshold {
		return QuotaCritical
	}
	if ratio >= warningThreshold {
		return QuotaWarning
	}

	return QuotaOK
}
