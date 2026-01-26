// Package sculptor provides pre-flight content analysis and optimization for the Superbrain system.
// It estimates token counts, detects file references, and optimizes content to fit within model context limits.
package sculptor

// TokenEstimator provides methods for estimating token counts in text content.
// It supports multiple estimation strategies with different accuracy/performance tradeoffs.
type TokenEstimator struct {
	// method is the estimation method to use ("simple" or "tiktoken")
	method string
}

// NewTokenEstimator creates a new TokenEstimator with the specified method.
// Valid methods are "simple" (fast approximation) and "tiktoken" (accurate but slower).
// If an invalid method is provided, defaults to "simple".
func NewTokenEstimator(method string) *TokenEstimator {
	if method != "simple" && method != "tiktoken" {
		method = "simple"
	}
	return &TokenEstimator{method: method}
}

// EstimateTokens estimates the number of tokens in the given content.
// For "simple" method: uses words * 1.3 approximation.
// For "tiktoken" method: currently falls back to simple (tiktoken integration TODO).
func (te *TokenEstimator) EstimateTokens(content string) int {
	if te.method == "tiktoken" {
		// TODO: Integrate tiktoken library for accurate counting
		// For now, fall back to simple estimation
		return te.simpleEstimate(content)
	}
	return te.simpleEstimate(content)
}

// simpleEstimate uses a word count * 1.3 approximation for token estimation.
// This is a fast but approximate method suitable for pre-flight checks.
func (te *TokenEstimator) simpleEstimate(content string) int {
	if len(content) == 0 {
		return 0
	}

	wordCount := countWords(content)
	// Multiply by 1.3 to account for subword tokenization
	// Most tokenizers produce ~1.3 tokens per word on average
	return int(float64(wordCount) * 1.3)
}

// countWords counts the number of words in the content.
// Words are separated by whitespace characters.
func countWords(content string) int {
	if len(content) == 0 {
		return 0
	}

	wordCount := 0
	inWord := false

	for _, r := range content {
		isSpace := r == ' ' || r == '\t' || r == '\n' || r == '\r'
		if isSpace {
			inWord = false
		} else if !inWord {
			wordCount++
			inWord = true
		}
	}

	return wordCount
}

// Method returns the estimation method being used.
func (te *TokenEstimator) Method() string {
	return te.method
}


// ModelContextLimits maps model names/patterns to their context window sizes in tokens.
// This lookup table is used for pre-flight analysis to determine if content will fit.
var ModelContextLimits = map[string]int{
	// Claude models
	"claude-3-opus":          200000,
	"claude-3-sonnet":        200000,
	"claude-3-haiku":         200000,
	"claude-3.5-sonnet":      200000,
	"claude-3.5-haiku":       200000,
	"claude-sonnet-4":        200000,
	"claude-opus-4":          200000,
	"claude-opus-4.5":        200000,

	// Gemini models
	"gemini-pro":             32000,
	"gemini-1.5-pro":         1000000,
	"gemini-1.5-flash":       1000000,
	"gemini-2.0-flash":       1000000,
	"gemini-2.0-pro":         1000000,
	"gemini-flash":           1000000,

	// GPT models
	"gpt-4":                  8192,
	"gpt-4-32k":              32768,
	"gpt-4-turbo":            128000,
	"gpt-4o":                 128000,
	"gpt-4o-mini":            128000,
	"gpt-5":                  200000,

	// Default fallback
	"default":                8192,
}

// GetModelContextLimit returns the context limit for the specified model.
// If the exact model is not found, it attempts to match by prefix.
// Returns the default limit (8192) if no match is found.
func GetModelContextLimit(model string) int {
	// Try exact match first
	if limit, ok := ModelContextLimits[model]; ok {
		return limit
	}

	// Try prefix matching for versioned models
	for pattern, limit := range ModelContextLimits {
		if matchesModelPattern(model, pattern) {
			return limit
		}
	}

	// Return default
	return ModelContextLimits["default"]
}

// matchesModelPattern checks if a model name matches a pattern.
// Supports prefix matching (e.g., "claude-3-opus-20240229" matches "claude-3-opus").
func matchesModelPattern(model, pattern string) bool {
	if pattern == "default" {
		return false
	}
	if len(model) < len(pattern) {
		return false
	}
	return model[:len(pattern)] == pattern
}

// ContextAnalysis contains the results of pre-flight token analysis.
type ContextAnalysis struct {
	// EstimatedTokens is the total estimated token count for the request.
	EstimatedTokens int `json:"estimated_tokens"`

	// ModelContextLimit is the context window size for the target model.
	ModelContextLimit int `json:"model_context_limit"`

	// ExceedsLimit indicates whether the estimated tokens exceed the model's limit.
	ExceedsLimit bool `json:"exceeds_limit"`

	// FileCount is the total number of files referenced in the request.
	FileCount int `json:"file_count"`

	// RelevantFiles lists files that were included in the analysis.
	RelevantFiles []string `json:"relevant_files,omitempty"`

	// ExcludedFiles lists files that were excluded during optimization.
	ExcludedFiles []string `json:"excluded_files,omitempty"`

	// OptimizationDone indicates whether content optimization was performed.
	OptimizationDone bool `json:"optimization_done"`
}

// NewContextAnalysis creates a new ContextAnalysis for the given model.
func NewContextAnalysis(model string) *ContextAnalysis {
	return &ContextAnalysis{
		ModelContextLimit: GetModelContextLimit(model),
		RelevantFiles:     make([]string, 0),
		ExcludedFiles:     make([]string, 0),
	}
}

// UpdateExceedsLimit recalculates whether the estimated tokens exceed the limit.
func (ca *ContextAnalysis) UpdateExceedsLimit() {
	ca.ExceedsLimit = ca.EstimatedTokens > ca.ModelContextLimit
}
