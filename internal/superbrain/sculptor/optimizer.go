package sculptor

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/traylinx/switchAILocal/internal/registry"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// ContentOptimizer reduces content to fit within model context limits.
// It prioritizes important files and generates summaries for excluded content.
type ContentOptimizer struct {
	estimator     *TokenEstimator
	analyzer      *FileAnalyzer
	priorityFiles []string
}

// DefaultPriorityFiles are files that should be prioritized during optimization.
var DefaultPriorityFiles = []string{
	"README.md",
	"README",
	"readme.md",
	"main.go",
	"main.ts",
	"main.py",
	"main.rs",
	"main.java",
	"index.ts",
	"index.js",
	"index.tsx",
	"index.jsx",
	"app.ts",
	"app.js",
	"app.py",
	"package.json",
	"go.mod",
	"Cargo.toml",
	"pyproject.toml",
	"requirements.txt",
	"pom.xml",
	"build.gradle",
	"Makefile",
	"Dockerfile",
	"docker-compose.yml",
	"docker-compose.yaml",
	".env.example",
	"config.yaml",
	"config.yml",
	"config.json",
}

// NewContentOptimizer creates a new ContentOptimizer.
func NewContentOptimizer(estimator *TokenEstimator, analyzer *FileAnalyzer, priorityFiles []string) *ContentOptimizer {
	if len(priorityFiles) == 0 {
		priorityFiles = DefaultPriorityFiles
	}
	return &ContentOptimizer{
		estimator:     estimator,
		analyzer:      analyzer,
		priorityFiles: priorityFiles,
	}
}

// FileWithPriority represents a file with its priority score for optimization.
type FileWithPriority struct {
	Path            string
	Content         string
	EstimatedTokens int
	Priority        int // Higher = more important
	IsIncluded      bool
}

// OptimizationResult contains the result of content optimization.
type OptimizationResult struct {
	// OptimizedContent is the content after optimization.
	OptimizedContent string

	// IncludedFiles lists files that were included.
	IncludedFiles []string

	// ExcludedFiles lists files that were excluded.
	ExcludedFiles []string

	// HighDensityMap summarizes excluded content.
	HighDensityMap *types.HighDensityMap

	// TotalTokens is the estimated token count after optimization.
	TotalTokens int

	// OriginalTokens is the estimated token count before optimization.
	OriginalTokens int

	// Success indicates whether optimization achieved the target limit.
	Success bool
}

// Optimize reduces content to fit within the specified token limit.
// It prioritizes important files and generates summaries for excluded content.
func (co *ContentOptimizer) Optimize(files []FileWithPriority, targetLimit int, queryKeywords []string) *OptimizationResult {
	result := &OptimizationResult{
		IncludedFiles: make([]string, 0),
		ExcludedFiles: make([]string, 0),
		HighDensityMap: &types.HighDensityMap{
			FileSummaries: make(map[string]string),
		},
	}

	// Calculate original tokens
	for _, f := range files {
		result.OriginalTokens += f.EstimatedTokens
	}
	result.HighDensityMap.TotalFiles = len(files)

	// Score and sort files by priority
	scoredFiles := co.scoreFiles(files, queryKeywords)
	sort.Slice(scoredFiles, func(i, j int) bool {
		return scoredFiles[i].Priority > scoredFiles[j].Priority
	})

	// Include files until we hit the limit
	currentTokens := 0
	var contentParts []string

	for i := range scoredFiles {
		if currentTokens+scoredFiles[i].EstimatedTokens <= targetLimit {
			scoredFiles[i].IsIncluded = true
			currentTokens += scoredFiles[i].EstimatedTokens
			result.IncludedFiles = append(result.IncludedFiles, scoredFiles[i].Path)
			contentParts = append(contentParts, formatFileContent(scoredFiles[i].Path, scoredFiles[i].Content))
		} else {
			result.ExcludedFiles = append(result.ExcludedFiles, scoredFiles[i].Path)
			// Generate summary for excluded file
			result.HighDensityMap.FileSummaries[scoredFiles[i].Path] = co.generateSummary(scoredFiles[i].Content)
		}
	}

	result.OptimizedContent = strings.Join(contentParts, "\n\n")
	result.TotalTokens = currentTokens
	// Success is true only if we included at least one file AND fit within the limit
	// If we couldn't include any files, that's a failure
	result.Success = len(result.IncludedFiles) > 0 && currentTokens <= targetLimit
	result.HighDensityMap.IncludedFiles = len(result.IncludedFiles)
	result.HighDensityMap.ExcludedFiles = len(result.ExcludedFiles)
	result.HighDensityMap.TokensSaved = result.OriginalTokens - result.TotalTokens

	return result
}

// scoreFiles assigns priority scores to files based on various criteria.
func (co *ContentOptimizer) scoreFiles(files []FileWithPriority, queryKeywords []string) []FileWithPriority {
	scored := make([]FileWithPriority, len(files))
	copy(scored, files)

	for i := range scored {
		score := 0

		// Priority file bonus (highest priority)
		if co.isPriorityFile(scored[i].Path) {
			score += 1000
		}

		// README files get extra priority
		baseName := strings.ToLower(filepath.Base(scored[i].Path))
		if strings.HasPrefix(baseName, "readme") {
			score += 500
		}

		// Main entry points
		if isEntryPoint(scored[i].Path) {
			score += 400
		}

		// Config files
		if isConfigFile(scored[i].Path) {
			score += 300
		}

		// Keyword matching bonus
		keywordScore := co.calculateKeywordScore(scored[i].Path, scored[i].Content, queryKeywords)
		score += keywordScore

		// Penalize very large files (they consume too much context)
		if scored[i].EstimatedTokens > 5000 {
			score -= 100
		}
		if scored[i].EstimatedTokens > 10000 {
			score -= 200
		}

		// Penalize test files (usually less important for understanding)
		if isTestFile(scored[i].Path) {
			score -= 50
		}

		// Penalize generated/vendor files
		if isGeneratedFile(scored[i].Path) {
			score -= 500
		}

		scored[i].Priority = score
	}

	return scored
}

// isPriorityFile checks if a file matches the priority file list.
func (co *ContentOptimizer) isPriorityFile(path string) bool {
	baseName := filepath.Base(path)
	for _, pf := range co.priorityFiles {
		if baseName == pf || strings.EqualFold(baseName, pf) {
			return true
		}
	}
	return false
}

// calculateKeywordScore calculates a score based on keyword matches.
func (co *ContentOptimizer) calculateKeywordScore(path, content string, keywords []string) int {
	if len(keywords) == 0 {
		return 0
	}

	score := 0
	lowerPath := strings.ToLower(path)
	lowerContent := strings.ToLower(content)

	for _, kw := range keywords {
		lowerKw := strings.ToLower(kw)
		// Path match is worth more
		if strings.Contains(lowerPath, lowerKw) {
			score += 200
		}
		// Content match
		if strings.Contains(lowerContent, lowerKw) {
			score += 50
		}
	}

	return score
}

// isEntryPoint checks if a file is likely a main entry point.
func isEntryPoint(path string) bool {
	baseName := strings.ToLower(filepath.Base(path))
	entryPoints := []string{
		"main.go", "main.ts", "main.js", "main.py", "main.rs", "main.java",
		"index.ts", "index.js", "index.tsx", "index.jsx",
		"app.ts", "app.js", "app.py", "app.tsx", "app.jsx",
		"server.go", "server.ts", "server.js", "server.py",
		"cmd.go", "cli.go", "run.py",
	}
	for _, ep := range entryPoints {
		if baseName == ep {
			return true
		}
	}
	return false
}

// isConfigFile checks if a file is a configuration file.
func isConfigFile(path string) bool {
	baseName := strings.ToLower(filepath.Base(path))
	configFiles := []string{
		"package.json", "go.mod", "go.sum", "cargo.toml", "cargo.lock",
		"pyproject.toml", "requirements.txt", "setup.py", "setup.cfg",
		"pom.xml", "build.gradle", "build.gradle.kts",
		"tsconfig.json", "jsconfig.json", "webpack.config.js",
		"vite.config.ts", "vite.config.js",
		"makefile", "dockerfile", "docker-compose.yml", "docker-compose.yaml",
		".env.example", "config.yaml", "config.yml", "config.json",
		".eslintrc", ".eslintrc.js", ".eslintrc.json",
		".prettierrc", ".prettierrc.js", ".prettierrc.json",
	}
	for _, cf := range configFiles {
		if baseName == cf {
			return true
		}
	}
	return false
}

// isTestFile checks if a file is a test file.
func isTestFile(path string) bool {
	baseName := strings.ToLower(filepath.Base(path))
	// Go test files
	if strings.HasSuffix(baseName, "_test.go") {
		return true
	}
	// JS/TS test files
	if strings.Contains(baseName, ".test.") || strings.Contains(baseName, ".spec.") {
		return true
	}
	// Python test files
	if strings.HasPrefix(baseName, "test_") || strings.HasSuffix(baseName, "_test.py") {
		return true
	}
	// Test directories - normalize path for matching
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "/test/") || strings.Contains(lowerPath, "/tests/") ||
		strings.Contains(lowerPath, "/__tests__/") ||
		strings.HasPrefix(lowerPath, "test/") || strings.HasPrefix(lowerPath, "tests/") ||
		strings.HasPrefix(lowerPath, "__tests__/") {
		return true
	}
	return false
}

// isGeneratedFile checks if a file is likely generated or vendored.
func isGeneratedFile(path string) bool {
	lowerPath := strings.ToLower(path)
	// Vendor directories - check both with and without leading slash
	if strings.Contains(lowerPath, "/vendor/") || strings.Contains(lowerPath, "/node_modules/") ||
		strings.HasPrefix(lowerPath, "vendor/") || strings.HasPrefix(lowerPath, "node_modules/") {
		return true
	}
	// Generated directories - check both with and without leading slash
	if strings.Contains(lowerPath, "/dist/") || strings.Contains(lowerPath, "/build/") ||
		strings.Contains(lowerPath, "/target/") || strings.Contains(lowerPath, "/.next/") ||
		strings.HasPrefix(lowerPath, "dist/") || strings.HasPrefix(lowerPath, "build/") ||
		strings.HasPrefix(lowerPath, "target/") || strings.HasPrefix(lowerPath, ".next/") {
		return true
	}
	// Lock files
	baseName := filepath.Base(lowerPath)
	if baseName == "package-lock.json" || baseName == "yarn.lock" ||
		baseName == "pnpm-lock.yaml" || baseName == "go.sum" {
		return true
	}
	return false
}

// generateSummary creates a brief summary of file content.
func (co *ContentOptimizer) generateSummary(content string) string {
	if len(content) == 0 {
		return "(empty file)"
	}

	// Extract first meaningful lines
	lines := strings.Split(content, "\n")
	var summaryLines []string
	lineCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comments at the start
		if trimmed == "" {
			continue
		}
		// Include up to 3 meaningful lines
		if lineCount < 3 {
			if len(trimmed) > 100 {
				trimmed = trimmed[:100] + "..."
			}
			summaryLines = append(summaryLines, trimmed)
			lineCount++
		} else {
			break
		}
	}

	if len(summaryLines) == 0 {
		return "(no meaningful content)"
	}

	summary := strings.Join(summaryLines, " | ")
	wordCount := countWords(content)
	return summary + " [" + formatWordCount(wordCount) + "]"
}

// formatWordCount formats a word count for display.
func formatWordCount(count int) string {
	if count < 1000 {
		return strings.TrimRight(strings.TrimRight(
			strings.Replace(string(rune('0'+count/100))+string(rune('0'+(count%100)/10))+string(rune('0'+count%10)), "0", "", -1),
			"0"), "") + " words"
	}
	return string(rune('0'+count/1000)) + "k words"
}

// formatFileContent formats a file's content with a header.
func formatFileContent(path, content string) string {
	return "=== " + path + " ===\n" + content
}

// OptimizeFromPaths loads files from paths and optimizes them.
func (co *ContentOptimizer) OptimizeFromPaths(paths []string, targetLimit int, queryKeywords []string) *OptimizationResult {
	var files []FileWithPriority

	for _, path := range paths {
		content := co.analyzer.GetFileContent(path)
		if content == "" {
			continue
		}

		files = append(files, FileWithPriority{
			Path:            path,
			Content:         content,
			EstimatedTokens: co.estimator.EstimateTokens(content),
		})
	}

	return co.Optimize(files, targetLimit, queryKeywords)
}

// GenerateDirectoryTree creates a text representation of the directory structure.
func (co *ContentOptimizer) GenerateDirectoryTree(files []string) string {
	if len(files) == 0 {
		return ""
	}

	// Build tree structure
	tree := make(map[string][]string)
	for _, f := range files {
		dir := filepath.Dir(f)
		if dir == "." {
			dir = "/"
		}
		tree[dir] = append(tree[dir], filepath.Base(f))
	}

	// Sort directories
	var dirs []string
	for d := range tree {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	// Build output
	var lines []string
	for _, dir := range dirs {
		lines = append(lines, dir+"/")
		sort.Strings(tree[dir])
		for _, f := range tree[dir] {
			lines = append(lines, "  "+f)
		}
	}

	return strings.Join(lines, "\n")
}

// BuildHighDensityMap creates a comprehensive HighDensityMap from optimization results.
// This provides transparency about what content was excluded and why.
func (co *ContentOptimizer) BuildHighDensityMap(result *OptimizationResult) *types.HighDensityMap {
	if result.HighDensityMap == nil {
		result.HighDensityMap = &types.HighDensityMap{
			FileSummaries: make(map[string]string),
		}
	}

	hdm := result.HighDensityMap
	hdm.TotalFiles = len(result.IncludedFiles) + len(result.ExcludedFiles)
	hdm.IncludedFiles = len(result.IncludedFiles)
	hdm.ExcludedFiles = len(result.ExcludedFiles)
	hdm.TokensSaved = result.OriginalTokens - result.TotalTokens

	// Generate directory tree for all files
	allFiles := append(result.IncludedFiles, result.ExcludedFiles...)
	hdm.DirectoryTree = co.GenerateDirectoryTree(allFiles)

	return hdm
}

// CreateHighDensityMapFromFiles creates a HighDensityMap directly from file lists.
// Useful when you have pre-computed file lists without running full optimization.
func CreateHighDensityMapFromFiles(includedFiles, excludedFiles []string, tokensSaved int) *types.HighDensityMap {
	hdm := &types.HighDensityMap{
		TotalFiles:    len(includedFiles) + len(excludedFiles),
		IncludedFiles: len(includedFiles),
		ExcludedFiles: len(excludedFiles),
		TokensSaved:   tokensSaved,
		FileSummaries: make(map[string]string),
	}

	// Generate simple directory tree
	allFiles := append(includedFiles, excludedFiles...)
	tree := make(map[string][]string)
	for _, f := range allFiles {
		dir := filepath.Dir(f)
		if dir == "." {
			dir = "/"
		}
		tree[dir] = append(tree[dir], filepath.Base(f))
	}

	var dirs []string
	for d := range tree {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var lines []string
	for _, dir := range dirs {
		lines = append(lines, dir+"/")
		sort.Strings(tree[dir])
		for _, f := range tree[dir] {
			lines = append(lines, "  "+f)
		}
	}

	hdm.DirectoryTree = strings.Join(lines, "\n")
	return hdm
}

// UnreducibleContentError represents a failure to reduce content below the context limit.
type UnreducibleContentError struct {
	// CurrentTokens is the token count after maximum optimization.
	CurrentTokens int

	// TargetLimit is the context limit we were trying to meet.
	TargetLimit int

	// RecommendedModels lists models that could handle the content.
	RecommendedModels []ModelRecommendation

	// Message provides a human-readable explanation.
	Message string
}

func (e *UnreducibleContentError) Error() string {
	return e.Message
}

// ModelRecommendation suggests an alternative model that could handle the content.
type ModelRecommendation struct {
	// Model is the model name.
	Model string `json:"model"`

	// ContextLimit is the model's context window size.
	ContextLimit int `json:"context_limit"`

	// Provider is the provider offering this model.
	Provider string `json:"provider,omitempty"`

	// Reason explains why this model is recommended.
	Reason string `json:"reason"`
}

// CheckUnreducible checks if content cannot be reduced to fit the target limit.
// Returns an UnreducibleContentError with recommendations if content is unreducible.
func (co *ContentOptimizer) CheckUnreducible(result *OptimizationResult, targetLimit int) *UnreducibleContentError {
	if result.TotalTokens <= targetLimit {
		return nil // Content fits, no error
	}

	// Find models that could handle this content
	recommendations := co.findSuitableModels(result.TotalTokens)

	return &UnreducibleContentError{
		CurrentTokens:     result.TotalTokens,
		TargetLimit:       targetLimit,
		RecommendedModels: recommendations,
		Message:           formatUnreducibleMessage(result.TotalTokens, targetLimit, recommendations),
	}
}

// findSuitableModels finds models that can handle the given token count.
func (co *ContentOptimizer) findSuitableModels(tokenCount int) []ModelRecommendation {
	var suitable []ModelRecommendation

	// Dynamically query the global registry for models that fit
	regModels := registry.GetGlobalRegistry().GetModelsWithMinContext(tokenCount)
	for _, m := range regModels {
		suitable = append(suitable, ModelRecommendation{
			Model:        m.ID,
			ContextLimit: m.ContextLength,
			Provider:     m.Type,
			Reason:       fmt.Sprintf("%s token context window", formatTokenCount(m.ContextLength)),
		})
	}

	// Sort by context limit (smallest sufficient first)
	sort.Slice(suitable, func(i, j int) bool {
		return suitable[i].ContextLimit < suitable[j].ContextLimit
	})

	// Return top 3 recommendations
	if len(suitable) > 3 {
		suitable = suitable[:3]
	}

	return suitable
}

// formatUnreducibleMessage creates a human-readable error message.
func formatUnreducibleMessage(currentTokens, targetLimit int, recommendations []ModelRecommendation) string {
	msg := "Content cannot be reduced to fit within the context limit. "
	msg += "Estimated tokens: " + formatTokenCount(currentTokens) + ", "
	msg += "Target limit: " + formatTokenCount(targetLimit) + ". "

	if len(recommendations) > 0 {
		msg += "Consider using a model with a larger context window: "
		for i, rec := range recommendations {
			if i > 0 {
				msg += ", "
			}
			msg += rec.Model + " (" + rec.Reason + ")"
		}
		msg += "."
	} else {
		msg += "No suitable models found for this content size."
	}

	return msg
}

// formatTokenCount formats a token count for display.
func formatTokenCount(count int) string {
	if count >= 1000000 {
		return string(rune('0'+count/1000000)) + "M"
	}
	if count >= 1000 {
		return string(rune('0'+count/1000)) + "K"
	}
	return string(rune('0'+count/100)) + string(rune('0'+(count%100)/10)) + string(rune('0'+count%10))
}

// PreFlightResult contains the complete result of pre-flight analysis.
type PreFlightResult struct {
	// Analysis contains the token analysis results.
	Analysis *ContextAnalysis

	// OptimizationResult contains optimization results if optimization was performed.
	OptimizationResult *OptimizationResult

	// Error contains any error that occurred (e.g., UnreducibleContentError).
	Error error

	// CanProceed indicates whether the request can proceed.
	CanProceed bool

	// Recommendations contains model recommendations if content is too large.
	Recommendations []ModelRecommendation
}

// PerformPreFlight performs complete pre-flight analysis and optimization.
// Returns a PreFlightResult indicating whether the request can proceed.
func (co *ContentOptimizer) PerformPreFlight(files []FileWithPriority, targetModel string, queryKeywords []string) *PreFlightResult {
	result := &PreFlightResult{
		Analysis: NewContextAnalysis(targetModel),
	}

	// Calculate total tokens
	totalTokens := 0
	for _, f := range files {
		totalTokens += f.EstimatedTokens
	}
	result.Analysis.EstimatedTokens = totalTokens
	result.Analysis.FileCount = len(files)
	result.Analysis.UpdateExceedsLimit()

	// If under limit, no optimization needed
	if !result.Analysis.ExceedsLimit {
		result.CanProceed = true
		return result
	}

	// Attempt optimization
	optResult := co.Optimize(files, result.Analysis.ModelContextLimit, queryKeywords)
	result.OptimizationResult = optResult
	result.Analysis.OptimizationDone = true

	// Check if optimization succeeded
	if optResult.Success {
		result.CanProceed = true
		result.Analysis.EstimatedTokens = optResult.TotalTokens
		result.Analysis.RelevantFiles = optResult.IncludedFiles
		result.Analysis.ExcludedFiles = optResult.ExcludedFiles
		result.Analysis.UpdateExceedsLimit()
		return result
	}

	// Optimization failed - content is unreducible
	// Use original tokens for recommendations since optimized result may be 0
	tokensForRecommendation := optResult.OriginalTokens
	if tokensForRecommendation == 0 {
		tokensForRecommendation = result.Analysis.EstimatedTokens
	}

	unreducibleErr := &UnreducibleContentError{
		CurrentTokens:     tokensForRecommendation,
		TargetLimit:       result.Analysis.ModelContextLimit,
		RecommendedModels: co.findSuitableModels(tokensForRecommendation),
		Message:           formatUnreducibleMessage(tokensForRecommendation, result.Analysis.ModelContextLimit, co.findSuitableModels(tokensForRecommendation)),
	}
	result.Error = unreducibleErr
	result.CanProceed = false
	result.Recommendations = unreducibleErr.RecommendedModels

	return result
}
