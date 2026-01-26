// Package doctor provides AI-powered failure diagnosis for the Superbrain system.
// It analyzes diagnostic snapshots to identify failure patterns and recommend remediation actions.
package doctor

import (
	"regexp"
	"strings"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// FailurePattern defines a recognizable failure pattern with its associated metadata.
type FailurePattern struct {
	// Name is a unique identifier for this pattern.
	Name string

	// Regex is the compiled regular expression to match against log content.
	Regex *regexp.Regexp

	// FailureType is the categorization of this failure.
	FailureType types.FailureType

	// Remediation is the recommended action for this failure type.
	Remediation types.RemediationType

	// Priority determines matching order when multiple patterns match (higher = checked first).
	Priority int

	// Description provides human-readable context about what this pattern detects.
	Description string

	// RemediationArgs contains default arguments for the remediation action.
	RemediationArgs map[string]string
}

// DefaultPatterns contains the built-in failure patterns for common CLI issues.
// Patterns are ordered by priority (highest first) for deterministic matching.
var DefaultPatterns = []*FailurePattern{
	// Permission prompt patterns (highest priority - most actionable)
	{
		Name:        "claude_permission_prompt",
		Regex:       regexp.MustCompile(`(?i)(allow|approve|permit).*\?\s*\[?y/n\]?`),
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationStdinInject,
		Priority:    100,
		Description: "Claude Code permission prompt waiting for user input",
		RemediationArgs: map[string]string{
			"response": "y\n",
		},
	},
	{
		Name:        "claude_dangerous_skip_prompt",
		Regex:       regexp.MustCompile(`(?i)--dangerously-skip-permissions`),
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationRestartFlags,
		Priority:    99,
		Description: "Claude CLI needs --dangerously-skip-permissions flag",
		RemediationArgs: map[string]string{
			"flag": "--dangerously-skip-permissions",
		},
	},
	{
		Name:        "generic_yes_no_prompt",
		Regex:       regexp.MustCompile(`(?i)(continue|proceed|confirm)\?\s*\[?y/n\]?`),
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationStdinInject,
		Priority:    95,
		Description: "Generic yes/no confirmation prompt",
		RemediationArgs: map[string]string{
			"response": "y\n",
		},
	},
	{
		Name:        "press_enter_prompt",
		Regex:       regexp.MustCompile(`(?i)press\s+(enter|return)\s+to\s+continue`),
		FailureType: types.FailureTypePermissionPrompt,
		Remediation: types.RemediationStdinInject,
		Priority:    94,
		Description: "Press enter to continue prompt",
		RemediationArgs: map[string]string{
			"response": "\n",
		},
	},

	// Authentication error patterns
	{
		Name:        "auth_invalid_api_key",
		Regex:       regexp.MustCompile(`(?i)(invalid|incorrect|wrong)\s*(api[_\s]?key|token|credential)`),
		FailureType: types.FailureTypeAuthError,
		Remediation: types.RemediationFallback,
		Priority:    90,
		Description: "Invalid API key or authentication token",
	},
	{
		Name:        "auth_expired_token",
		Regex:       regexp.MustCompile(`(?i)(expired|revoked)\s*(token|credential|session)`),
		FailureType: types.FailureTypeAuthError,
		Remediation: types.RemediationFallback,
		Priority:    89,
		Description: "Expired or revoked authentication token",
	},
	{
		Name:        "auth_unauthorized",
		Regex:       regexp.MustCompile(`(?i)(401|unauthorized|authentication\s+failed|not\s+authenticated)`),
		FailureType: types.FailureTypeAuthError,
		Remediation: types.RemediationFallback,
		Priority:    88,
		Description: "Unauthorized access or authentication failure",
	},
	{
		Name:        "auth_forbidden",
		Regex:       regexp.MustCompile(`(?i)(403|forbidden|access\s+denied|permission\s+denied)`),
		FailureType: types.FailureTypeAuthError,
		Remediation: types.RemediationFallback,
		Priority:    87,
		Description: "Forbidden access or permission denied",
	},

	// Context exceeded patterns
	{
		Name:        "context_limit_exceeded",
		Regex:       regexp.MustCompile(`(?i)(context|token)\s*(limit|length|window)\s*(exceeded|too\s+(long|large))`),
		FailureType: types.FailureTypeContextExceeded,
		Remediation: types.RemediationFallback,
		Priority:    85,
		Description: "Request exceeded model context limit",
	},
	{
		Name:        "max_tokens_exceeded",
		Regex:       regexp.MustCompile(`(?i)max(imum)?\s*tokens?\s*(exceeded|reached|limit)`),
		FailureType: types.FailureTypeContextExceeded,
		Remediation: types.RemediationFallback,
		Priority:    84,
		Description: "Maximum token limit exceeded",
	},
	{
		Name:        "input_too_long",
		Regex:       regexp.MustCompile(`(?i)(input|request|message)\s*(is\s+)?(too\s+long|exceeds?\s+limit)`),
		FailureType: types.FailureTypeContextExceeded,
		Remediation: types.RemediationFallback,
		Priority:    83,
		Description: "Input content too long for model",
	},

	// Rate limit patterns
	{
		Name:        "rate_limit_429",
		Regex:       regexp.MustCompile(`(?i)(429|rate\s*limit|too\s+many\s+requests)`),
		FailureType: types.FailureTypeRateLimit,
		Remediation: types.RemediationRetry,
		Priority:    80,
		Description: "Rate limit exceeded (HTTP 429)",
	},
	{
		Name:        "quota_exceeded",
		Regex:       regexp.MustCompile(`(?i)(quota|usage)\s*(exceeded|limit|exhausted)`),
		FailureType: types.FailureTypeRateLimit,
		Remediation: types.RemediationFallback,
		Priority:    79,
		Description: "API quota or usage limit exceeded",
	},
	{
		Name:        "throttled",
		Regex:       regexp.MustCompile(`(?i)(throttl(ed|ing)|slow\s*down|back\s*off)`),
		FailureType: types.FailureTypeRateLimit,
		Remediation: types.RemediationRetry,
		Priority:    78,
		Description: "Request throttled by provider",
	},

	// Network error patterns
	{
		Name:        "network_timeout",
		Regex:       regexp.MustCompile(`(?i)(connection|request|network)\s*(timed?\s*out|timeout)`),
		FailureType: types.FailureTypeNetworkError,
		Remediation: types.RemediationRetry,
		Priority:    70,
		Description: "Network connection timeout",
	},
	{
		Name:        "network_refused",
		Regex:       regexp.MustCompile(`(?i)connection\s*(refused|reset|closed)`),
		FailureType: types.FailureTypeNetworkError,
		Remediation: types.RemediationRetry,
		Priority:    69,
		Description: "Network connection refused or reset",
	},
	{
		Name:        "dns_error",
		Regex:       regexp.MustCompile(`(?i)(dns|name\s+resolution)\s*(error|failed|lookup)`),
		FailureType: types.FailureTypeNetworkError,
		Remediation: types.RemediationRetry,
		Priority:    68,
		Description: "DNS resolution failure",
	},
	{
		Name:        "ssl_error",
		Regex:       regexp.MustCompile(`(?i)(ssl|tls|certificate)\s*(error|failed|invalid)`),
		FailureType: types.FailureTypeNetworkError,
		Remediation: types.RemediationAbort,
		Priority:    67,
		Description: "SSL/TLS certificate error",
	},

	// Process crash patterns
	{
		Name:        "segfault",
		Regex:       regexp.MustCompile(`(?i)(segmentation\s+fault|sigsegv|signal\s+11)`),
		FailureType: types.FailureTypeProcessCrash,
		Remediation: types.RemediationFallback,
		Priority:    60,
		Description: "Process crashed with segmentation fault",
	},
	{
		Name:        "panic",
		Regex:       regexp.MustCompile(`(?i)(panic|fatal\s+error|unhandled\s+exception)`),
		FailureType: types.FailureTypeProcessCrash,
		Remediation: types.RemediationFallback,
		Priority:    59,
		Description: "Process panic or fatal error",
	},
	{
		Name:        "killed",
		Regex:       regexp.MustCompile(`(?i)(killed|sigkill|signal\s+9|oom)`),
		FailureType: types.FailureTypeProcessCrash,
		Remediation: types.RemediationFallback,
		Priority:    58,
		Description: "Process killed (possibly OOM)",
	},
}

// PatternMatcher provides pattern-based failure detection.
type PatternMatcher struct {
	patterns []*FailurePattern
}

// NewPatternMatcher creates a new PatternMatcher with the default patterns.
func NewPatternMatcher() *PatternMatcher {
	return NewPatternMatcherWithPatterns(DefaultPatterns)
}

// NewPatternMatcherWithPatterns creates a PatternMatcher with custom patterns.
// Patterns are sorted by priority (highest first) for deterministic matching.
func NewPatternMatcherWithPatterns(patterns []*FailurePattern) *PatternMatcher {
	// Sort patterns by priority (highest first)
	sorted := make([]*FailurePattern, len(patterns))
	copy(sorted, patterns)
	sortPatternsByPriority(sorted)

	return &PatternMatcher{
		patterns: sorted,
	}
}

// sortPatternsByPriority sorts patterns in descending order by priority.
func sortPatternsByPriority(patterns []*FailurePattern) {
	// Simple insertion sort (patterns list is small)
	for i := 1; i < len(patterns); i++ {
		key := patterns[i]
		j := i - 1
		for j >= 0 && patterns[j].Priority < key.Priority {
			patterns[j+1] = patterns[j]
			j--
		}
		patterns[j+1] = key
	}
}

// MatchResult contains the result of pattern matching.
type MatchResult struct {
	// Matched indicates whether any pattern was matched.
	Matched bool

	// Pattern is the matched pattern (nil if no match).
	Pattern *FailurePattern

	// MatchedText is the text that matched the pattern.
	MatchedText string
}

// Match analyzes log content and returns the highest-priority matching pattern.
// If no pattern matches, returns a result with Matched=false.
func (pm *PatternMatcher) Match(logContent string) *MatchResult {
	// Normalize log content for matching
	normalizedContent := strings.ToLower(logContent)

	for _, pattern := range pm.patterns {
		if match := pattern.Regex.FindString(normalizedContent); match != "" {
			return &MatchResult{
				Matched:     true,
				Pattern:     pattern,
				MatchedText: match,
			}
		}
	}

	return &MatchResult{
		Matched: false,
	}
}

// MatchAll returns all patterns that match the log content, sorted by priority.
func (pm *PatternMatcher) MatchAll(logContent string) []*MatchResult {
	normalizedContent := strings.ToLower(logContent)
	var results []*MatchResult

	for _, pattern := range pm.patterns {
		if match := pattern.Regex.FindString(normalizedContent); match != "" {
			results = append(results, &MatchResult{
				Matched:     true,
				Pattern:     pattern,
				MatchedText: match,
			})
		}
	}

	return results
}

// GetPatterns returns all registered patterns.
func (pm *PatternMatcher) GetPatterns() []*FailurePattern {
	return pm.patterns
}

// AddPattern adds a custom pattern to the matcher.
// The pattern list is re-sorted after addition.
func (pm *PatternMatcher) AddPattern(pattern *FailurePattern) {
	pm.patterns = append(pm.patterns, pattern)
	sortPatternsByPriority(pm.patterns)
}
