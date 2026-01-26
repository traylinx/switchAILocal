// Package injector provides autonomous stdin injection capabilities for the Superbrain system.
// It can automatically respond to interactive CLI prompts to prevent processes from hanging
// while waiting for user input.
package injector

import (
	"regexp"
)

// StdinPattern defines a recognizable prompt pattern and its automatic response.
// Patterns are matched against process output to detect when stdin injection is needed.
type StdinPattern struct {
	// Name is a unique identifier for this pattern.
	Name string

	// Regex is the compiled regular expression pattern to match in process output.
	Regex *regexp.Regexp

	// Response is the text to inject into stdin when the pattern is matched.
	Response string

	// IsSafe indicates whether this pattern is safe for automatic injection in autopilot mode.
	// Safe patterns are those that don't perform destructive operations.
	IsSafe bool

	// Description provides human-readable context about what this pattern matches.
	Description string
}

// DefaultStdinPatterns returns the built-in patterns for common CLI prompts.
// These patterns cover the most common interactive prompts from Claude, Gemini, and other tools.
func DefaultStdinPatterns() []*StdinPattern {
	return []*StdinPattern{
		{
			Name:        "claude_file_permission",
			Regex:       regexp.MustCompile(`(?i)allow.*read.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Claude Code file read permission prompt",
		},
		{
			Name:        "claude_tool_permission",
			Regex:       regexp.MustCompile(`(?i)allow.*tool.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Claude Code tool execution permission prompt",
		},
		{
			Name:        "claude_write_permission",
			Regex:       regexp.MustCompile(`(?i)allow.*write.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Claude Code file write permission prompt",
		},
		{
			Name:        "claude_edit_permission",
			Regex:       regexp.MustCompile(`(?i)allow.*edit.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Claude Code file edit permission prompt",
		},
		{
			Name:        "gemini_file_access",
			Regex:       regexp.MustCompile(`(?i)grant.*file.*access.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Gemini CLI file access permission prompt",
		},
		{
			Name:        "gemini_tool_execution",
			Regex:       regexp.MustCompile(`(?i)execute.*tool.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Gemini CLI tool execution permission prompt",
		},
		{
			Name:        "generic_continue",
			Regex:       regexp.MustCompile(`(?i)press.*enter.*continue`),
			Response:    "\n",
			IsSafe:      true,
			Description: "Generic continue prompt",
		},
		{
			Name:        "generic_yes_no",
			Regex:       regexp.MustCompile(`(?i)continue.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Generic yes/no continue prompt",
		},
		{
			Name:        "codex_permission",
			Regex:       regexp.MustCompile(`(?i)allow.*codex.*\?\s*\[?y/n\]?`),
			Response:    "y\n",
			IsSafe:      true,
			Description: "Codex CLI permission prompt",
		},
	}
}

// DefaultForbiddenPatterns returns patterns that should never trigger automatic responses.
// These patterns indicate potentially dangerous operations that require human approval.
func DefaultForbiddenPatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		regexp.MustCompile(`(?i)delete`),
		regexp.MustCompile(`(?i)remove`),
		regexp.MustCompile(`(?i)rm\s`),
		regexp.MustCompile(`(?i)sudo`),
		regexp.MustCompile(`(?i)format`),
		regexp.MustCompile(`(?i)destroy`),
		regexp.MustCompile(`(?i)drop\s+(table|database)`),
		regexp.MustCompile(`(?i)truncate`),
		regexp.MustCompile(`(?i)overwrite`),
	}
}

// MatchPattern searches for a matching stdin pattern in the given log content.
// It returns the first matching pattern, or nil if no patterns match.
// Forbidden patterns are checked first and will prevent any match.
func MatchPattern(logContent string, patterns []*StdinPattern, forbiddenPatterns []*regexp.Regexp) *StdinPattern {
	// First check if any forbidden patterns match
	for _, forbidden := range forbiddenPatterns {
		if forbidden.MatchString(logContent) {
			return nil
		}
	}

	// Then check for matching stdin patterns
	for _, pattern := range patterns {
		if pattern.Regex.MatchString(logContent) {
			return pattern
		}
	}

	return nil
}

// IsForbidden checks if the log content contains any forbidden patterns.
// Returns true if a forbidden pattern is detected.
func IsForbidden(logContent string, forbiddenPatterns []*regexp.Regexp) bool {
	for _, forbidden := range forbiddenPatterns {
		if forbidden.MatchString(logContent) {
			return true
		}
	}
	return false
}
