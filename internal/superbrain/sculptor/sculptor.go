// Package sculptor provides pre-flight content analysis and optimization for the Superbrain system.
// It detects when requests exceed model context limits and intelligently reduces content
// while preserving the most relevant information.
package sculptor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
	"github.com/traylinx/switchAILocal/internal/superbrain/metadata"
	"github.com/traylinx/switchAILocal/internal/superbrain/metrics"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// ContextSculptor provides pre-flight analysis and content optimization for requests.
// It integrates token estimation, file analysis, and content optimization into a
// single interface for use by the Superbrain executor layer.
type ContextSculptor struct {
	config           *config.ContextSculptorConfig
	tokenEstimator   *TokenEstimator
	fileAnalyzer     *FileAnalyzer
	contentOptimizer *ContentOptimizer
	auditLogger      *audit.Logger
	metricsCollector *metrics.Metrics
}

// NewContextSculptor creates a new ContextSculptor with the given configuration.
func NewContextSculptor(cfg *config.ContextSculptorConfig, basePath string) *ContextSculptor {
	if cfg == nil {
		cfg = &config.ContextSculptorConfig{
			Enabled:        true,
			TokenEstimator: "simple",
			PriorityFiles:  DefaultPriorityFiles,
		}
	}

	tokenEstimator := NewTokenEstimator(cfg.TokenEstimator)
	fileAnalyzer := NewFileAnalyzer(tokenEstimator, basePath)
	contentOptimizer := NewContentOptimizer(tokenEstimator, fileAnalyzer, cfg.PriorityFiles)

	return &ContextSculptor{
		config:           cfg,
		tokenEstimator:   tokenEstimator,
		fileAnalyzer:     fileAnalyzer,
		contentOptimizer: contentOptimizer,
		metricsCollector: metrics.Global(),
	}
}

// SetAuditLogger sets the audit logger for this sculptor.
func (cs *ContextSculptor) SetAuditLogger(logger *audit.Logger) {
	cs.auditLogger = logger
}

// PreFlightRequest contains the information needed for pre-flight analysis.
type PreFlightRequest struct {
	// RequestID uniquely identifies the request.
	RequestID string

	// Provider is the provider being used.
	Provider string

	// Model is the target model name.
	Model string

	// Payload is the raw request payload.
	Payload []byte

	// Content is the extracted content from the request.
	Content string

	// CLIArgs are any CLI arguments that may contain file references.
	CLIArgs []string
}

// PreFlightResponse contains the result of pre-flight analysis.
type PreFlightResponse struct {
	// CanProceed indicates whether the request can proceed.
	CanProceed bool

	// OptimizedPayload is the modified payload if optimization was performed.
	// If nil, the original payload should be used.
	OptimizedPayload []byte

	// HighDensityMap contains information about excluded content.
	HighDensityMap *types.HighDensityMap

	// Analysis contains the token analysis results.
	Analysis *ContextAnalysis

	// Error contains any error that occurred.
	Error error

	// Recommendations contains model recommendations if content is too large.
	Recommendations []ModelRecommendation
}

// PerformPreFlight performs pre-flight analysis on a request.
// It checks if the request content exceeds the model's context limit and
// attempts to optimize it if necessary.
func (cs *ContextSculptor) PerformPreFlight(ctx context.Context, req *PreFlightRequest, aggregator *metadata.Aggregator) *PreFlightResponse {
	response := &PreFlightResponse{
		CanProceed: true,
	}

	if !cs.config.Enabled {
		return response
	}

	// Analyze the request for file references and token count
	analysis := cs.fileAnalyzer.AnalyzeRequest(req.Content, req.CLIArgs, req.Model)
	response.Analysis = analysis

	// If content doesn't exceed limit, no optimization needed
	if !analysis.ExceedsLimit {
		return response
	}

	// Log that we're performing optimization
	if aggregator != nil {
		aggregator.RecordAction(
			"context_analysis",
			fmt.Sprintf("Content exceeds context limit: %d tokens > %d limit", analysis.EstimatedTokens, analysis.ModelContextLimit),
			true,
			map[string]interface{}{
				"estimated_tokens": analysis.EstimatedTokens,
				"context_limit":    analysis.ModelContextLimit,
				"file_count":       analysis.FileCount,
			},
		)
	}

	// Build file list for optimization
	var files []FileWithPriority
	for _, path := range analysis.RelevantFiles {
		fileContent := cs.fileAnalyzer.GetFileContent(path)
		if fileContent == "" {
			continue
		}
		files = append(files, FileWithPriority{
			Path:            path,
			Content:         fileContent,
			EstimatedTokens: cs.tokenEstimator.EstimateTokens(fileContent),
		})
	}

	// Extract keywords from content for prioritization
	keywords := extractKeywordsFromContent(req.Content)

	// Perform optimization
	result := cs.contentOptimizer.PerformPreFlight(files, req.Model, keywords)

	if !result.CanProceed {
		// Content is unreducible
		response.CanProceed = false
		response.Error = result.Error
		response.Recommendations = result.Recommendations

		if aggregator != nil {
			aggregator.RecordAction(
				"context_optimization",
				"Content cannot be reduced to fit context limit",
				false,
				map[string]interface{}{
					"original_tokens": analysis.EstimatedTokens,
					"target_limit":    analysis.ModelContextLimit,
				},
			)
		}

		if cs.auditLogger != nil {
			cs.auditLogger.LogContextOptimization(
				req.RequestID,
				req.Provider,
				req.Model,
				analysis.EstimatedTokens,
				0,
				"failed_unreducible",
			)
		}

		if cs.metricsCollector != nil {
			cs.metricsCollector.RecordContextOptimization()
		}

		return response
	}

	// Optimization succeeded
	if result.OptimizationResult != nil {
		response.HighDensityMap = result.OptimizationResult.HighDensityMap
		response.Analysis.EstimatedTokens = result.OptimizationResult.TotalTokens
		response.Analysis.RelevantFiles = result.OptimizationResult.IncludedFiles
		response.Analysis.ExcludedFiles = result.OptimizationResult.ExcludedFiles
		response.Analysis.OptimizationDone = true

		if aggregator != nil {
			aggregator.RecordAction(
				"context_optimization",
				fmt.Sprintf("Optimized content from %d to %d tokens", analysis.EstimatedTokens, result.OptimizationResult.TotalTokens),
				true,
				map[string]interface{}{
					"original_tokens":  analysis.EstimatedTokens,
					"optimized_tokens": result.OptimizationResult.TotalTokens,
					"files_included":   len(result.OptimizationResult.IncludedFiles),
					"files_excluded":   len(result.OptimizationResult.ExcludedFiles),
					"tokens_saved":     result.OptimizationResult.HighDensityMap.TokensSaved,
				},
			)
		}

		if cs.auditLogger != nil {
			cs.auditLogger.LogContextOptimization(
				req.RequestID,
				req.Provider,
				req.Model,
				analysis.EstimatedTokens,
				result.OptimizationResult.TotalTokens,
				"success",
			)
		}

		if cs.metricsCollector != nil {
			cs.metricsCollector.RecordContextOptimization()
		}
	}

	return response
}

// AnalyzeRequest performs token analysis on a request without optimization.
// This is useful for getting an estimate before deciding whether to proceed.
func (cs *ContextSculptor) AnalyzeRequest(content string, cliArgs []string, model string) *ContextAnalysis {
	return cs.fileAnalyzer.AnalyzeRequest(content, cliArgs, model)
}

// EstimateTokens estimates the token count for the given content.
func (cs *ContextSculptor) EstimateTokens(content string) int {
	return cs.tokenEstimator.EstimateTokens(content)
}

// GetModelContextLimit returns the context limit for a model.
func (cs *ContextSculptor) GetModelContextLimit(model string) int {
	return GetModelContextLimit(model)
}

// ProcessRequest is a convenience method that extracts content from a request
// and performs pre-flight analysis.
func (cs *ContextSculptor) ProcessRequest(ctx context.Context, req switchailocalexecutor.Request, provider, requestID string, aggregator *metadata.Aggregator) *PreFlightResponse {
	// Extract content from payload
	content := string(req.Payload)

	// Try to extract CLI args from payload
	var cliArgs []string
	var payloadMap map[string]interface{}
	if err := json.Unmarshal(req.Payload, &payloadMap); err == nil {
		if extraBody, ok := payloadMap["extra_body"].(map[string]interface{}); ok {
			if cli, ok := extraBody["cli"].(map[string]interface{}); ok {
				if attachments, ok := cli["attachments"].([]interface{}); ok {
					for _, att := range attachments {
						if attMap, ok := att.(map[string]interface{}); ok {
							if path, ok := attMap["path"].(string); ok {
								cliArgs = append(cliArgs, path)
							}
						}
					}
				}
			}
		}
	}

	// Create pre-flight request
	pfReq := &PreFlightRequest{
		RequestID: requestID,
		Provider:  provider,
		Model:     req.Model,
		Payload:   req.Payload,
		Content:   content,
		CLIArgs:   cliArgs,
	}

	return cs.PerformPreFlight(ctx, pfReq, aggregator)
}

// extractKeywordsFromContent extracts keywords from content for file prioritization.
func extractKeywordsFromContent(content string) []string {
	// Simple keyword extraction - look for common patterns
	var keywords []string

	// Look for quoted strings that might be file names or important terms
	// This is a simplified implementation
	words := strings.Fields(content)
	for _, word := range words {
		// Skip very short or very long words
		if len(word) < 3 || len(word) > 50 {
			continue
		}
		// Skip common words
		if isCommonWord(word) {
			continue
		}
		// Add words that look like identifiers or file names
		if looksLikeIdentifier(word) {
			keywords = append(keywords, word)
		}
	}

	// Limit to top 10 keywords
	if len(keywords) > 10 {
		keywords = keywords[:10]
	}

	return keywords
}

// isCommonWord checks if a word is a common English word to skip.
func isCommonWord(word string) bool {
	common := map[string]bool{
		"the": true, "and": true, "for": true, "are": true, "but": true,
		"not": true, "you": true, "all": true, "can": true, "had": true,
		"her": true, "was": true, "one": true, "our": true, "out": true,
		"has": true, "his": true, "how": true, "its": true, "may": true,
		"new": true, "now": true, "old": true, "see": true, "way": true,
		"who": true, "did": true, "get": true, "let": true, "put": true,
		"say": true, "she": true, "too": true, "use": true, "with": true,
		"this": true, "that": true, "from": true, "have": true, "what": true,
		"when": true, "where": true, "which": true, "will": true, "would": true,
	}
	return common[strings.ToLower(word)]
}

// looksLikeIdentifier checks if a word looks like a code identifier or file name.
func looksLikeIdentifier(word string) bool {
	// Contains underscore or camelCase
	if strings.Contains(word, "_") {
		return true
	}
	// Has mixed case (camelCase)
	hasLower := false
	hasUpper := false
	for _, r := range word {
		if r >= 'a' && r <= 'z' {
			hasLower = true
		}
		if r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
	}
	if hasLower && hasUpper {
		return true
	}
	// Has file extension
	if strings.Contains(word, ".") {
		parts := strings.Split(word, ".")
		if len(parts) == 2 && len(parts[1]) <= 4 {
			return true
		}
	}
	return false
}
