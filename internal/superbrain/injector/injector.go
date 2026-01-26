// Package injector provides autonomous stdin injection capabilities for the Superbrain system.
package injector

import (
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
)

// StdinInjector provides autonomous stdin injection for interactive CLI prompts.
// It can automatically respond to permission prompts and other interactive questions
// to prevent processes from hanging while waiting for user input.
type StdinInjector struct {
	mu                sync.RWMutex
	mode              string
	patterns          []*StdinPattern
	forbiddenPatterns []*regexp.Regexp
	customPatterns    []*StdinPattern
	auditLogger       *audit.Logger
}

// Config holds configuration for the StdinInjector.
type Config struct {
	// Mode controls stdin injection behavior.
	// Valid values: "disabled", "conservative", "autopilot"
	Mode string

	// CustomPatterns defines additional prompt patterns to recognize.
	CustomPatterns []config.StdinPattern

	// ForbiddenPatterns lists regex patterns that should never trigger automatic responses.
	ForbiddenPatterns []string

	// AuditLogger is the logger for recording injection attempts.
	// If nil, the global audit logger will be used.
	AuditLogger *audit.Logger
}

// NewStdinInjector creates a new stdin injector with the specified configuration.
func NewStdinInjector(cfg Config) (*StdinInjector, error) {
	// Validate mode
	if cfg.Mode != "disabled" && cfg.Mode != "conservative" && cfg.Mode != "autopilot" {
		return nil, fmt.Errorf("invalid stdin injection mode: %s (must be disabled, conservative, or autopilot)", cfg.Mode)
	}

	// Start with default patterns
	patterns := DefaultStdinPatterns()

	// Parse custom patterns from config
	var customPatterns []*StdinPattern
	for _, cp := range cfg.CustomPatterns {
		regex, err := regexp.Compile(cp.Regex)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in custom pattern %s: %w", cp.Name, err)
		}

		customPatterns = append(customPatterns, &StdinPattern{
			Name:        cp.Name,
			Regex:       regex,
			Response:    cp.Response,
			IsSafe:      cp.IsSafe,
			Description: cp.Description,
		})
	}

	// Combine default and custom patterns
	allPatterns := append(patterns, customPatterns...)

	// Parse forbidden patterns
	forbiddenPatterns := DefaultForbiddenPatterns()
	for _, fp := range cfg.ForbiddenPatterns {
		regex, err := regexp.Compile(fp)
		if err != nil {
			return nil, fmt.Errorf("invalid forbidden pattern regex: %w", err)
		}
		forbiddenPatterns = append(forbiddenPatterns, regex)
	}

	return &StdinInjector{
		mode:              cfg.Mode,
		patterns:          allPatterns,
		forbiddenPatterns: forbiddenPatterns,
		customPatterns:    customPatterns,
		auditLogger:       cfg.AuditLogger,
	}, nil
}

// CanInject checks if injection is allowed for the given pattern and current mode.
// Returns true if injection should proceed, false otherwise.
func (s *StdinInjector) CanInject(pattern *StdinPattern) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Never inject in disabled mode
	if s.mode == "disabled" {
		return false
	}

	// In conservative mode, only inject safe patterns
	if s.mode == "conservative" {
		return pattern.IsSafe
	}

	// In autopilot mode, inject all safe patterns
	if s.mode == "autopilot" {
		return pattern.IsSafe
	}

	return false
}

// Inject writes the response to the process stdin.
// Returns an error if the write fails.
func (s *StdinInjector) Inject(stdin io.Writer, response string) error {
	if stdin == nil {
		return fmt.Errorf("stdin writer is nil")
	}

	_, err := stdin.Write([]byte(response))
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}

	return nil
}

// MatchPattern searches for a matching stdin pattern in the given log content.
// It returns the first matching pattern, or nil if no patterns match.
// Forbidden patterns are checked first and will prevent any match.
func (s *StdinInjector) MatchPattern(logContent string) *StdinPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return MatchPattern(logContent, s.patterns, s.forbiddenPatterns)
}

// IsForbidden checks if the log content contains any forbidden patterns.
// Returns true if a forbidden pattern is detected.
func (s *StdinInjector) IsForbidden(logContent string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return IsForbidden(logContent, s.forbiddenPatterns)
}

// GetPatterns returns all configured stdin patterns (default + custom).
func (s *StdinInjector) GetPatterns() []*StdinPattern {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	patterns := make([]*StdinPattern, len(s.patterns))
	copy(patterns, s.patterns)
	return patterns
}

// GetMode returns the current injection mode.
func (s *StdinInjector) GetMode() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.mode
}

// SetMode updates the injection mode at runtime.
// Valid values: "disabled", "conservative", "autopilot"
func (s *StdinInjector) SetMode(mode string) error {
	if mode != "disabled" && mode != "conservative" && mode != "autopilot" {
		return fmt.Errorf("invalid stdin injection mode: %s", mode)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mode = mode
	return nil
}

// TryInject attempts to match a pattern in the log content and inject the response if allowed.
// Returns the matched pattern, whether injection was performed, and any error.
// All injection attempts are logged to the audit log.
func (s *StdinInjector) TryInject(logContent string, stdin io.Writer) (*StdinPattern, bool, error) {
	return s.TryInjectWithContext(logContent, stdin, "", "", "")
}

// TryInjectWithContext attempts injection with additional context for audit logging.
// requestID, provider, and model are used for audit trail purposes.
func (s *StdinInjector) TryInjectWithContext(logContent string, stdin io.Writer, requestID, provider, model string) (*StdinPattern, bool, error) {
	// Check for forbidden patterns first
	if s.IsForbidden(logContent) {
		s.logInjectionAttempt(requestID, provider, model, "", "", "blocked_forbidden")
		return nil, false, fmt.Errorf("forbidden pattern detected in log content")
	}

	// Try to match a pattern
	pattern := s.MatchPattern(logContent)
	if pattern == nil {
		return nil, false, nil // No pattern matched
	}

	// Check if injection is allowed for this pattern
	if !s.CanInject(pattern) {
		s.logInjectionAttempt(requestID, provider, model, pattern.Name, pattern.Response, "blocked_mode")
		return pattern, false, nil // Pattern matched but injection not allowed
	}

	// Perform the injection
	err := s.Inject(stdin, pattern.Response)
	if err != nil {
		s.logInjectionAttempt(requestID, provider, model, pattern.Name, pattern.Response, "failed")
		return pattern, false, err
	}

	s.logInjectionAttempt(requestID, provider, model, pattern.Name, pattern.Response, "success")
	return pattern, true, nil
}

// logInjectionAttempt logs an injection attempt to the audit log.
func (s *StdinInjector) logInjectionAttempt(requestID, provider, model, pattern, response, outcome string) {
	logger := s.auditLogger
	if logger == nil {
		logger = audit.Global()
	}

	logger.LogStdinInjection(requestID, provider, model, pattern, response, outcome)
}
