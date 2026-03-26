package autoroute

import (
	"strings"
	"unicode"
)

// Intent constants define the standard intent categories.
const (
	IntentCoding    = "coding"
	IntentReasoning = "reasoning"
	IntentCreative  = "creative"
	IntentFast      = "fast"
	IntentVision    = "vision"
	IntentSecure    = "secure"
	IntentGeneral   = "general"
)

// IntentClassification represents a classified intent with confidence.
type IntentClassification struct {
	Intent     string  // The detected intent category
	Confidence float64 // 0.0 - 1.0
	Method     string  // "hint", "heuristic", or "cortex"
}

// ClassifyIntent detects the intent of a user message using fast heuristics.
// This is the "Reflex Tier" from the Cortex Router spec — pattern matching
// that runs in <1ms, no LLM required.
//
// Returns IntentGeneral if no strong signal is detected.
func ClassifyIntent(content string, intentHint string) IntentClassification {
	// Priority 1: Explicit hint from "auto:coding" syntax
	if intentHint != "" {
		return IntentClassification{
			Intent:     normalizeIntent(intentHint),
			Confidence: 1.0,
			Method:     "hint",
		}
	}

	// Priority 2: Fast heuristic classification
	lower := strings.ToLower(content)

	// Check for vision signals (images, screenshots)
	if hasVisionSignals(lower) {
		return IntentClassification{
			Intent:     IntentVision,
			Confidence: 0.85,
			Method:     "heuristic",
		}
	}

	// Check for security signals (PII, credentials, sensitive)
	if hasSecureSignals(lower) {
		return IntentClassification{
			Intent:     IntentSecure,
			Confidence: 0.80,
			Method:     "heuristic",
		}
	}

	// Check for coding signals (code blocks, programming terms)
	if hasCodingSignals(lower, content) {
		return IntentClassification{
			Intent:     IntentCoding,
			Confidence: 0.80,
			Method:     "heuristic",
		}
	}

	// Check for reasoning signals (math, logic, analysis)
	if hasReasoningSignals(lower) {
		return IntentClassification{
			Intent:     IntentReasoning,
			Confidence: 0.75,
			Method:     "heuristic",
		}
	}

	// Check for creative signals (stories, poems, brainstorm)
	if hasCreativeSignals(lower) {
		return IntentClassification{
			Intent:     IntentCreative,
			Confidence: 0.70,
			Method:     "heuristic",
		}
	}

	// Check for fast/simple signals (short, quick questions)
	if hasFastSignals(lower, content) {
		return IntentClassification{
			Intent:     IntentFast,
			Confidence: 0.65,
			Method:     "heuristic",
		}
	}

	// Default: general purpose
	return IntentClassification{
		Intent:     IntentGeneral,
		Confidence: 0.50,
		Method:     "heuristic",
	}
}

// normalizeIntent maps user-provided hints to standard intent categories.
func normalizeIntent(hint string) string {
	switch strings.ToLower(strings.TrimSpace(hint)) {
	case "code", "coding", "program", "programming", "dev":
		return IntentCoding
	case "reason", "reasoning", "think", "thinking", "math", "logic":
		return IntentReasoning
	case "creative", "write", "writing", "story", "poem":
		return IntentCreative
	case "fast", "quick", "simple", "chat":
		return IntentFast
	case "vision", "image", "visual", "screenshot":
		return IntentVision
	case "secure", "private", "local", "sensitive":
		return IntentSecure
	default:
		return hint // Pass through unknown intents
	}
}

// hasCodingSignals checks for programming-related content.
func hasCodingSignals(lower string, original string) bool {
	// Strong signals: code fences
	if strings.Contains(lower, "```") {
		return true
	}

	// Function/class/variable patterns
	codePatterns := []string{
		"func ", "function ", "class ", "import ", "package ",
		"def ", "return ", "const ", "let ", "var ",
		"if err != nil", "try:", "catch (", "except:",
		"console.log", "fmt.println", "print(",
		"<html", "</div>", "<?php",
	}
	for _, p := range codePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// Programming keywords in context
	codingKeywords := []string{
		"write a function", "implement", "refactor", "debug",
		"fix this code", "write code", "create a script",
		"api endpoint", "unit test", "integration test",
		"compile error", "syntax error", "runtime error",
		"pull request", "code review", "git commit",
	}
	for _, kw := range codingKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// High density of special characters (likely code)
	specialCount := 0
	for _, r := range original {
		if r == '{' || r == '}' || r == '(' || r == ')' || r == ';' || r == '=' {
			specialCount++
		}
	}
	if len(original) > 50 && float64(specialCount)/float64(len(original)) > 0.05 {
		return true
	}

	return false
}

// hasReasoningSignals checks for analytical/mathematical content.
func hasReasoningSignals(lower string) bool {
	patterns := []string{
		"explain why", "analyze", "compare and contrast",
		"what are the implications", "pros and cons",
		"step by step", "prove that", "derive",
		"calculate", "solve for", "equation",
		"evaluate", "assess", "critical thinking",
		"logical", "theorem", "hypothesis",
		"trade-off", "tradeoff", "trade off",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	// Math symbols density
	mathChars := 0
	for _, r := range lower {
		if r == '+' || r == '-' || r == '*' || r == '/' || r == '=' || r == '>' || r == '<' {
			mathChars++
		}
		if unicode.Is(unicode.S, r) { // Mathematical symbols
			mathChars++
		}
	}
	if len(lower) > 20 && float64(mathChars)/float64(len(lower)) > 0.08 {
		return true
	}

	return false
}

// hasCreativeSignals checks for creative writing content.
func hasCreativeSignals(lower string) bool {
	patterns := []string{
		"write a story", "write a poem", "once upon a time",
		"creative writing", "brainstorm", "imagine",
		"fiction", "narrative", "dialogue",
		"write me a", "compose", "draft a",
		"screenplay", "blog post", "article about",
		"haiku", "limerick", "sonnet",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// hasFastSignals checks for simple/quick question patterns.
func hasFastSignals(lower string, original string) bool {
	// Very short messages are likely simple questions
	if len(original) < 80 {
		// Quick conversational patterns
		quickPatterns := []string{
			"what is", "who is", "when was", "where is",
			"how do i", "what does", "define ",
			"translate", "convert", "how many",
			"yes or no", "true or false",
		}
		for _, p := range quickPatterns {
			if strings.Contains(lower, p) {
				return true
			}
		}
	}
	return false
}

// hasVisionSignals checks for image/visual content.
func hasVisionSignals(lower string) bool {
	patterns := []string{
		"this image", "this screenshot", "this photo",
		"look at this", "what do you see",
		"describe the image", "analyze this picture",
		"attached image", "image shows",
		"ocr", "read the text in",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// hasSecureSignals checks for security/privacy sensitive content.
func hasSecureSignals(lower string) bool {
	patterns := []string{
		"password", "api key", "secret key",
		"credit card", "social security", "ssn",
		"private data", "confidential", "classified",
		"don't share", "do not share", "keep private",
		"encrypt", "decrypt",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
