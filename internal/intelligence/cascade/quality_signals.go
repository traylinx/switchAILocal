// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package cascade provides model cascading functionality for intelligent routing.
// It detects quality signals in responses and triggers tier escalation when needed.
package cascade

import (
	"regexp"
	"strings"
)

// QualitySignal represents a detected quality issue in a response.
type QualitySignal struct {
	// Type identifies the kind of quality issue
	Type SignalType `json:"type"`
	// Severity indicates how serious the issue is (0.0-1.0)
	Severity float64 `json:"severity"`
	// Description provides human-readable details
	Description string `json:"description"`
	// Position indicates where in the response the issue was detected (if applicable)
	Position int `json:"position,omitempty"`
}

// SignalType categorizes different quality issues.
type SignalType string

const (
	// SignalAbruptEnding indicates the response ended unexpectedly
	SignalAbruptEnding SignalType = "abrupt_ending"
	// SignalMissingSections indicates expected content sections are missing
	SignalMissingSections SignalType = "missing_sections"
	// SignalIncompleteCode indicates code blocks are not properly closed
	SignalIncompleteCode SignalType = "incomplete_code"
	// SignalTruncated indicates the response appears to be cut off
	SignalTruncated SignalType = "truncated"
	// SignalRepetitive indicates excessive repetition in the response
	SignalRepetitive SignalType = "repetitive"
	// SignalIncoherent indicates the response lacks logical flow
	SignalIncoherent SignalType = "incoherent"
	// SignalRefusal indicates the model refused to answer
	SignalRefusal SignalType = "refusal"
	// SignalLowQuality indicates general low quality response
	SignalLowQuality SignalType = "low_quality"
)

// QualitySignalDetector detects quality issues in LLM responses.
type QualitySignalDetector struct {
	// Patterns for detecting various quality issues
	abruptEndingPatterns   []*regexp.Regexp
	truncationPatterns     []*regexp.Regexp
	refusalPatterns        []*regexp.Regexp
	repetitionThreshold    int
	minResponseLength      int
	codeBlockPattern       *regexp.Regexp
	incompleteCodePatterns []*regexp.Regexp
}

// NewQualitySignalDetector creates a new detector with default patterns.
func NewQualitySignalDetector() *QualitySignalDetector {
	return &QualitySignalDetector{
		abruptEndingPatterns: []*regexp.Regexp{
			regexp.MustCompile(`\.\.\.$`),                          // Ends with ...
			regexp.MustCompile(`(?i)(?:and|but|or|so|then)\s*$`),   // Ends with conjunction
			regexp.MustCompile(`(?i)(?:the|a|an|this|that)\s*$`),   // Ends with article
			regexp.MustCompile(`(?i)(?:to|for|with|from|in)\s*$`),  // Ends with preposition
			regexp.MustCompile(`(?i)(?:is|are|was|were|be)\s*$`),   // Ends with verb
			regexp.MustCompile(`(?i)(?:I|we|you|they|it)\s*$`),     // Ends with pronoun
			regexp.MustCompile(`(?i)(?:can|will|would|should)\s*$`), // Ends with modal
		},
		truncationPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\[(?:truncated|cut off|continued)\]`),
			regexp.MustCompile(`(?i)(?:output|response) (?:truncated|limit)`),
			regexp.MustCompile(`(?i)(?:maximum|max) (?:length|tokens?) (?:reached|exceeded)`),
		},
		refusalPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)^I (?:cannot|can't|am unable to|won't|will not)`),
			regexp.MustCompile(`(?i)^(?:Sorry|I'm sorry|I apologize),? (?:but )?I (?:cannot|can't)`),
			regexp.MustCompile(`(?i)^As an AI,? I (?:cannot|can't|am unable to)`),
			regexp.MustCompile(`(?i)^I'm not able to`),
		},
		repetitionThreshold: 3,
		minResponseLength:   50,
		codeBlockPattern:    regexp.MustCompile("```"),
		incompleteCodePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^\s*(?:def|func|function|class|if|for|while|switch)\s+[^{]*$`), // Unclosed block
			regexp.MustCompile(`(?m)\{\s*$`),                                                       // Opening brace at end
			regexp.MustCompile(`(?m)^\s*//\s*\.\.\.\s*$`),                                          // Comment with ...
		},
	}
}

// DetectSignals analyzes a response and returns all detected quality signals.
func (d *QualitySignalDetector) DetectSignals(response string) []QualitySignal {
	var signals []QualitySignal

	// Trim whitespace for analysis
	trimmed := strings.TrimSpace(response)

	// Check for empty or very short responses
	if len(trimmed) < d.minResponseLength {
		signals = append(signals, QualitySignal{
			Type:        SignalLowQuality,
			Severity:    0.8,
			Description: "Response is too short",
		})
	}

	// Check for refusal
	if signal := d.detectRefusal(trimmed); signal != nil {
		signals = append(signals, *signal)
	}

	// Check for abrupt ending
	if signal := d.detectAbruptEnding(trimmed); signal != nil {
		signals = append(signals, *signal)
	}

	// Check for truncation
	if signal := d.detectTruncation(trimmed); signal != nil {
		signals = append(signals, *signal)
	}

	// Check for incomplete code
	if signal := d.detectIncompleteCode(trimmed); signal != nil {
		signals = append(signals, *signal)
	}

	// Check for repetition
	if signal := d.detectRepetition(trimmed); signal != nil {
		signals = append(signals, *signal)
	}

	return signals
}

// detectRefusal checks if the response is a refusal to answer.
func (d *QualitySignalDetector) detectRefusal(response string) *QualitySignal {
	for _, pattern := range d.refusalPatterns {
		if pattern.MatchString(response) {
			return &QualitySignal{
				Type:        SignalRefusal,
				Severity:    0.9,
				Description: "Model refused to answer the request",
			}
		}
	}
	return nil
}

// detectAbruptEnding checks if the response ends abruptly.
func (d *QualitySignalDetector) detectAbruptEnding(response string) *QualitySignal {
	// Get last 100 characters for analysis
	suffix := response
	if len(response) > 100 {
		suffix = response[len(response)-100:]
	}

	for _, pattern := range d.abruptEndingPatterns {
		if pattern.MatchString(suffix) {
			return &QualitySignal{
				Type:        SignalAbruptEnding,
				Severity:    0.7,
				Description: "Response appears to end abruptly",
				Position:    len(response),
			}
		}
	}
	return nil
}

// detectTruncation checks if the response was truncated.
func (d *QualitySignalDetector) detectTruncation(response string) *QualitySignal {
	for _, pattern := range d.truncationPatterns {
		if pattern.MatchString(response) {
			return &QualitySignal{
				Type:        SignalTruncated,
				Severity:    0.85,
				Description: "Response appears to be truncated",
			}
		}
	}
	return nil
}

// detectIncompleteCode checks for unclosed code blocks or incomplete code.
func (d *QualitySignalDetector) detectIncompleteCode(response string) *QualitySignal {
	// Count code block markers
	matches := d.codeBlockPattern.FindAllStringIndex(response, -1)
	if len(matches)%2 != 0 {
		return &QualitySignal{
			Type:        SignalIncompleteCode,
			Severity:    0.8,
			Description: "Code block is not properly closed",
		}
	}

	// Check for incomplete code patterns within code blocks
	if len(matches) > 0 {
		// Get content of last code block
		lastOpenIdx := matches[len(matches)-1][1]
		if lastOpenIdx < len(response) {
			codeContent := response[lastOpenIdx:]
			for _, pattern := range d.incompleteCodePatterns {
				if pattern.MatchString(codeContent) {
					return &QualitySignal{
						Type:        SignalIncompleteCode,
						Severity:    0.6,
						Description: "Code appears to be incomplete",
					}
				}
			}
		}
	}

	return nil
}

// detectRepetition checks for excessive repetition in the response.
func (d *QualitySignalDetector) detectRepetition(response string) *QualitySignal {
	// Split into sentences
	sentences := strings.Split(response, ".")
	if len(sentences) < d.repetitionThreshold*2 {
		return nil
	}

	// Count sentence occurrences
	counts := make(map[string]int)
	for _, s := range sentences {
		normalized := strings.TrimSpace(strings.ToLower(s))
		if len(normalized) > 20 { // Only count substantial sentences
			counts[normalized]++
		}
	}

	// Check for repetition
	for _, count := range counts {
		if count >= d.repetitionThreshold {
			return &QualitySignal{
				Type:        SignalRepetitive,
				Severity:    0.6,
				Description: "Response contains excessive repetition",
			}
		}
	}

	return nil
}

// CalculateOverallQuality computes an overall quality score from signals.
// Returns a score between 0.0 (poor) and 1.0 (excellent).
func CalculateOverallQuality(signals []QualitySignal) float64 {
	if len(signals) == 0 {
		return 1.0
	}

	// Calculate weighted penalty
	totalPenalty := 0.0
	for _, signal := range signals {
		totalPenalty += signal.Severity
	}

	// Cap penalty at 1.0
	if totalPenalty > 1.0 {
		totalPenalty = 1.0
	}

	return 1.0 - totalPenalty
}

// HasCriticalSignals checks if any signals require immediate escalation.
func HasCriticalSignals(signals []QualitySignal) bool {
	for _, signal := range signals {
		if signal.Severity >= 0.8 {
			return true
		}
		// Certain signal types are always critical
		switch signal.Type {
		case SignalRefusal, SignalTruncated, SignalIncompleteCode:
			return true
		}
	}
	return false
}
