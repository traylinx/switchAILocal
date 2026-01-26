package doctor

import (
	"regexp"
	"testing"

	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestPatternMatcher_Match_PermissionPrompts(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		logContent  string
		wantMatched bool
		wantType    types.FailureType
	}{
		{
			name:        "claude permission prompt",
			logContent:  "Allow Claude to read file.txt? [y/n]",
			wantMatched: true,
			wantType:    types.FailureTypePermissionPrompt,
		},
		{
			name:        "generic yes/no prompt",
			logContent:  "Do you want to continue? [y/n]",
			wantMatched: true,
			wantType:    types.FailureTypePermissionPrompt,
		},
		{
			name:        "press enter prompt",
			logContent:  "Press Enter to continue...",
			wantMatched: true,
			wantType:    types.FailureTypePermissionPrompt,
		},
		{
			name:        "approve prompt",
			logContent:  "Approve this action? y/n",
			wantMatched: true,
			wantType:    types.FailureTypePermissionPrompt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if result.Matched != tt.wantMatched {
				t.Errorf("Match() matched = %v, want %v", result.Matched, tt.wantMatched)
			}
			if tt.wantMatched && result.Pattern.FailureType != tt.wantType {
				t.Errorf("Match() type = %v, want %v", result.Pattern.FailureType, tt.wantType)
			}
		})
	}
}

func TestPatternMatcher_Match_AuthErrors(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		logContent  string
		wantMatched bool
		wantType    types.FailureType
	}{
		{
			name:        "invalid api key",
			logContent:  "Error: Invalid API key provided",
			wantMatched: true,
			wantType:    types.FailureTypeAuthError,
		},
		{
			name:        "expired token",
			logContent:  "Authentication failed: expired token",
			wantMatched: true,
			wantType:    types.FailureTypeAuthError,
		},
		{
			name:        "401 unauthorized",
			logContent:  "HTTP 401 Unauthorized",
			wantMatched: true,
			wantType:    types.FailureTypeAuthError,
		},
		{
			name:        "403 forbidden",
			logContent:  "Error 403: Forbidden - Access denied",
			wantMatched: true,
			wantType:    types.FailureTypeAuthError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if result.Matched != tt.wantMatched {
				t.Errorf("Match() matched = %v, want %v", result.Matched, tt.wantMatched)
			}
			if tt.wantMatched && result.Pattern.FailureType != tt.wantType {
				t.Errorf("Match() type = %v, want %v", result.Pattern.FailureType, tt.wantType)
			}
		})
	}
}

func TestPatternMatcher_Match_ContextExceeded(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		logContent  string
		wantMatched bool
		wantType    types.FailureType
	}{
		{
			name:        "context limit exceeded",
			logContent:  "Error: Context limit exceeded for this model",
			wantMatched: true,
			wantType:    types.FailureTypeContextExceeded,
		},
		{
			name:        "max tokens exceeded",
			logContent:  "Maximum tokens exceeded: 128000",
			wantMatched: true,
			wantType:    types.FailureTypeContextExceeded,
		},
		{
			name:        "input too long",
			logContent:  "Request failed: input is too long",
			wantMatched: true,
			wantType:    types.FailureTypeContextExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if result.Matched != tt.wantMatched {
				t.Errorf("Match() matched = %v, want %v", result.Matched, tt.wantMatched)
			}
			if tt.wantMatched && result.Pattern.FailureType != tt.wantType {
				t.Errorf("Match() type = %v, want %v", result.Pattern.FailureType, tt.wantType)
			}
		})
	}
}

func TestPatternMatcher_Match_RateLimits(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		logContent  string
		wantMatched bool
		wantType    types.FailureType
	}{
		{
			name:        "429 rate limit",
			logContent:  "HTTP 429: Rate limit exceeded",
			wantMatched: true,
			wantType:    types.FailureTypeRateLimit,
		},
		{
			name:        "too many requests",
			logContent:  "Error: Too many requests, please slow down",
			wantMatched: true,
			wantType:    types.FailureTypeRateLimit,
		},
		{
			name:        "quota exceeded",
			logContent:  "API quota exceeded for this billing period",
			wantMatched: true,
			wantType:    types.FailureTypeRateLimit,
		},
		{
			name:        "throttled",
			logContent:  "Request throttled, backing off",
			wantMatched: true,
			wantType:    types.FailureTypeRateLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if result.Matched != tt.wantMatched {
				t.Errorf("Match() matched = %v, want %v", result.Matched, tt.wantMatched)
			}
			if tt.wantMatched && result.Pattern.FailureType != tt.wantType {
				t.Errorf("Match() type = %v, want %v", result.Pattern.FailureType, tt.wantType)
			}
		})
	}
}

func TestPatternMatcher_Match_NoMatch(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name       string
		logContent string
	}{
		{
			name:       "normal output",
			logContent: "Processing request... Done.",
		},
		{
			name:       "success message",
			logContent: "Request completed successfully",
		},
		{
			name:       "empty content",
			logContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if result.Matched {
				t.Errorf("Match() should not match for: %s", tt.logContent)
			}
		})
	}
}

func TestPatternMatcher_Priority(t *testing.T) {
	pm := NewPatternMatcher()

	// Log content that could match multiple patterns
	// Permission prompt should take priority over auth error
	logContent := "Allow access? [y/n] - 401 Unauthorized"

	result := pm.Match(logContent)
	if !result.Matched {
		t.Fatal("Expected a match")
	}

	// Permission prompt has higher priority (100) than auth error (88)
	if result.Pattern.FailureType != types.FailureTypePermissionPrompt {
		t.Errorf("Expected permission_prompt (higher priority), got %v", result.Pattern.FailureType)
	}
}

func TestPatternMatcher_MatchAll(t *testing.T) {
	pm := NewPatternMatcher()

	// Log content that matches multiple patterns
	logContent := "Allow access? [y/n] - HTTP 429 Rate limit"

	results := pm.MatchAll(logContent)
	if len(results) < 2 {
		t.Errorf("Expected at least 2 matches, got %d", len(results))
	}

	// Verify results are sorted by priority
	for i := 1; i < len(results); i++ {
		if results[i].Pattern.Priority > results[i-1].Pattern.Priority {
			t.Error("Results should be sorted by priority (descending)")
		}
	}
}

func TestPatternMatcher_CaseInsensitive(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name       string
		logContent string
	}{
		{"lowercase", "rate limit exceeded"},
		{"uppercase", "RATE LIMIT EXCEEDED"},
		{"mixed case", "Rate Limit Exceeded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.logContent)
			if !result.Matched {
				t.Errorf("Match() should match case-insensitively: %s", tt.logContent)
			}
		})
	}
}

func TestNewPatternMatcherWithPatterns(t *testing.T) {
	customPatterns := []*FailurePattern{
		{
			Name:        "custom_low",
			Regex:       compileRegex(`custom_low`),
			FailureType: types.FailureTypeUnknown,
			Priority:    10,
		},
		{
			Name:        "custom_high",
			Regex:       compileRegex(`custom_high`),
			FailureType: types.FailureTypeUnknown,
			Priority:    50,
		},
	}

	pm := NewPatternMatcherWithPatterns(customPatterns)
	patterns := pm.GetPatterns()

	if len(patterns) != 2 {
		t.Errorf("Expected 2 patterns, got %d", len(patterns))
	}

	// Verify sorted by priority (highest first)
	if patterns[0].Priority < patterns[1].Priority {
		t.Error("Patterns should be sorted by priority (descending)")
	}
}

func TestPatternMatcher_AddPattern(t *testing.T) {
	pm := NewPatternMatcher()
	initialCount := len(pm.GetPatterns())

	newPattern := &FailurePattern{
		Name:        "custom_pattern",
		Regex:       compileRegex(`custom_error_xyz`),
		FailureType: types.FailureTypeUnknown,
		Priority:    200, // Higher than all defaults
	}

	pm.AddPattern(newPattern)

	if len(pm.GetPatterns()) != initialCount+1 {
		t.Error("Pattern should be added")
	}

	// Test that the new high-priority pattern matches first
	result := pm.Match("custom_error_xyz occurred")
	if !result.Matched || result.Pattern.Name != "custom_pattern" {
		t.Error("New high-priority pattern should match first")
	}
}

// Helper to compile regex for tests
func compileRegex(pattern string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)` + pattern)
}
