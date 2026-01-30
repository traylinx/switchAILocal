package sculptor

import (
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/registry"
)

func setupTestRegistry() {
	reg := registry.GetGlobalRegistry()
	// Register active models to simulate a real environment
	models := []*registry.ModelInfo{
		{ID: "gemini-1.5-pro", ContextLength: 1000000, Type: "gemini"},
		{ID: "gemini-2.0-flash", ContextLength: 1000000, Type: "gemini"},
		{ID: "claude-3-opus", ContextLength: 200000, Type: "claude"},
		{ID: "claude-sonnet-4", ContextLength: 200000, Type: "claude"},
		{ID: "gpt-4-turbo", ContextLength: 128000, Type: "openai"},
		{ID: "gpt-4o", ContextLength: 128000, Type: "openai"},
		{ID: "gpt-4", ContextLength: 8192, Type: "openai"},
	}
	reg.RegisterClient("test-client", "test-provider", models)
}

func TestNewContentOptimizer(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")

	t.Run("uses default priority files when none provided", func(t *testing.T) {
		co := NewContentOptimizer(te, fa, nil)
		if len(co.priorityFiles) == 0 {
			t.Error("expected default priority files to be set")
		}
	})

	t.Run("uses custom priority files when provided", func(t *testing.T) {
		custom := []string{"custom.go", "special.ts"}
		co := NewContentOptimizer(te, fa, custom)
		if len(co.priorityFiles) != 2 {
			t.Errorf("expected 2 priority files, got %d", len(co.priorityFiles))
		}
	})
}

func TestOptimize(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("includes all files when under limit", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "file1.go", Content: "package main", EstimatedTokens: 100},
			{Path: "file2.go", Content: "package util", EstimatedTokens: 100},
		}

		result := co.Optimize(files, 500, nil)

		if len(result.IncludedFiles) != 2 {
			t.Errorf("expected 2 included files, got %d", len(result.IncludedFiles))
		}
		if len(result.ExcludedFiles) != 0 {
			t.Errorf("expected 0 excluded files, got %d", len(result.ExcludedFiles))
		}
		if !result.Success {
			t.Error("expected optimization to succeed")
		}
	})

	t.Run("excludes files when over limit", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "file1.go", Content: "package main", EstimatedTokens: 300},
			{Path: "file2.go", Content: "package util", EstimatedTokens: 300},
			{Path: "file3.go", Content: "package test", EstimatedTokens: 300},
		}

		result := co.Optimize(files, 500, nil)

		if len(result.IncludedFiles) > 1 {
			t.Errorf("expected at most 1 included file, got %d", len(result.IncludedFiles))
		}
		if len(result.ExcludedFiles) < 2 {
			t.Errorf("expected at least 2 excluded files, got %d", len(result.ExcludedFiles))
		}
	})

	t.Run("prioritizes README files", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "utils.go", Content: "package util", EstimatedTokens: 200},
			{Path: "README.md", Content: "# Project", EstimatedTokens: 200},
			{Path: "helper.go", Content: "package help", EstimatedTokens: 200},
		}

		result := co.Optimize(files, 250, nil)

		if len(result.IncludedFiles) != 1 {
			t.Fatalf("expected 1 included file, got %d", len(result.IncludedFiles))
		}
		if result.IncludedFiles[0] != "README.md" {
			t.Errorf("expected README.md to be included first, got %s", result.IncludedFiles[0])
		}
	})

	t.Run("generates summaries for excluded files", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "main.go", Content: "package main\nfunc main() {}", EstimatedTokens: 100},
			{Path: "excluded.go", Content: "package excluded\n// This is excluded", EstimatedTokens: 500},
		}

		result := co.Optimize(files, 150, nil)

		if result.HighDensityMap == nil {
			t.Fatal("expected HighDensityMap to be set")
		}
		if len(result.HighDensityMap.FileSummaries) == 0 {
			t.Error("expected file summaries for excluded files")
		}
	})

	t.Run("calculates token savings", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "file1.go", Content: "content", EstimatedTokens: 100},
			{Path: "file2.go", Content: "content", EstimatedTokens: 200},
		}

		result := co.Optimize(files, 150, nil)

		if result.OriginalTokens != 300 {
			t.Errorf("expected original tokens 300, got %d", result.OriginalTokens)
		}
		if result.HighDensityMap.TokensSaved <= 0 {
			t.Error("expected positive tokens saved")
		}
	})
}

func TestFilePrioritization(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("prioritizes main entry points", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "utils.go", Content: "package util", EstimatedTokens: 100},
			{Path: "main.go", Content: "package main", EstimatedTokens: 100},
		}

		result := co.Optimize(files, 150, nil)

		if result.IncludedFiles[0] != "main.go" {
			t.Errorf("expected main.go to be prioritized, got %s first", result.IncludedFiles[0])
		}
	})

	t.Run("prioritizes config files", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "random.go", Content: "package random", EstimatedTokens: 100},
			{Path: "package.json", Content: "{}", EstimatedTokens: 100},
		}

		result := co.Optimize(files, 150, nil)

		if result.IncludedFiles[0] != "package.json" {
			t.Errorf("expected package.json to be prioritized, got %s first", result.IncludedFiles[0])
		}
	})

	t.Run("deprioritizes test files", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "main_test.go", Content: "package main", EstimatedTokens: 100},
			{Path: "main.go", Content: "package main", EstimatedTokens: 100},
		}

		result := co.Optimize(files, 150, nil)

		if result.IncludedFiles[0] != "main.go" {
			t.Errorf("expected main.go over test file, got %s first", result.IncludedFiles[0])
		}
	})

	t.Run("boosts files matching keywords", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "random.go", Content: "package random", EstimatedTokens: 100},
			{Path: "auth.go", Content: "package auth", EstimatedTokens: 100},
		}

		result := co.Optimize(files, 150, []string{"auth", "authentication"})

		if result.IncludedFiles[0] != "auth.go" {
			t.Errorf("expected auth.go to match keyword, got %s first", result.IncludedFiles[0])
		}
	})
}

func TestIsTestFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main_test.go", true},
		{"main.test.ts", true},
		{"main.spec.js", true},
		{"test_main.py", true},
		{"main_test.py", true},
		{"src/__tests__/main.ts", true},
		{"test/main.go", true},
		{"main.go", false},
		{"testing.go", false},
		{"testutils.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isTestFile(tt.path)
			if result != tt.expected {
				t.Errorf("isTestFile(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestIsGeneratedFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"vendor/pkg/file.go", true},
		{"node_modules/pkg/index.js", true},
		{"dist/bundle.js", true},
		{"build/output.js", true},
		{"package-lock.json", true},
		{"yarn.lock", true},
		{"src/main.go", false},
		{"lib/utils.ts", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isGeneratedFile(tt.path)
			if result != tt.expected {
				t.Errorf("isGeneratedFile(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGenerateSummary(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("handles empty content", func(t *testing.T) {
		summary := co.generateSummary("")
		if summary != "(empty file)" {
			t.Errorf("expected '(empty file)', got '%s'", summary)
		}
	})

	t.Run("extracts first lines", func(t *testing.T) {
		content := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}"
		summary := co.generateSummary(content)
		if !strings.Contains(summary, "package main") {
			t.Errorf("expected summary to contain 'package main', got '%s'", summary)
		}
	})

	t.Run("truncates long lines", func(t *testing.T) {
		longLine := strings.Repeat("a", 200)
		summary := co.generateSummary(longLine)
		if len(summary) > 150 {
			t.Errorf("expected summary to be truncated, got length %d", len(summary))
		}
	})
}

func TestGenerateDirectoryTree(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("generates tree from files", func(t *testing.T) {
		files := []string{
			"src/main.go",
			"src/utils.go",
			"pkg/helper.go",
		}

		tree := co.GenerateDirectoryTree(files)

		if !strings.Contains(tree, "src/") {
			t.Error("expected tree to contain 'src/'")
		}
		if !strings.Contains(tree, "pkg/") {
			t.Error("expected tree to contain 'pkg/'")
		}
		if !strings.Contains(tree, "main.go") {
			t.Error("expected tree to contain 'main.go'")
		}
	})

	t.Run("handles empty file list", func(t *testing.T) {
		tree := co.GenerateDirectoryTree(nil)
		if tree != "" {
			t.Errorf("expected empty tree, got '%s'", tree)
		}
	})
}

func TestFormatFileContent(t *testing.T) {
	result := formatFileContent("main.go", "package main")
	expected := "=== main.go ===\npackage main"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestBuildHighDensityMap(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("builds complete high density map", func(t *testing.T) {
		result := &OptimizationResult{
			IncludedFiles:  []string{"src/main.go", "README.md"},
			ExcludedFiles:  []string{"src/utils.go", "pkg/helper.go"},
			OriginalTokens: 1000,
			TotalTokens:    400,
			HighDensityMap: nil,
		}

		hdm := co.BuildHighDensityMap(result)

		if hdm.TotalFiles != 4 {
			t.Errorf("expected TotalFiles 4, got %d", hdm.TotalFiles)
		}
		if hdm.IncludedFiles != 2 {
			t.Errorf("expected IncludedFiles 2, got %d", hdm.IncludedFiles)
		}
		if hdm.ExcludedFiles != 2 {
			t.Errorf("expected ExcludedFiles 2, got %d", hdm.ExcludedFiles)
		}
		if hdm.TokensSaved != 600 {
			t.Errorf("expected TokensSaved 600, got %d", hdm.TokensSaved)
		}
		if hdm.DirectoryTree == "" {
			t.Error("expected DirectoryTree to be populated")
		}
	})
}

func TestCreateHighDensityMapFromFiles(t *testing.T) {
	t.Run("creates map from file lists", func(t *testing.T) {
		included := []string{"main.go", "config.yaml"}
		excluded := []string{"test.go", "vendor/pkg.go"}

		hdm := CreateHighDensityMapFromFiles(included, excluded, 500)

		if hdm.TotalFiles != 4 {
			t.Errorf("expected TotalFiles 4, got %d", hdm.TotalFiles)
		}
		if hdm.IncludedFiles != 2 {
			t.Errorf("expected IncludedFiles 2, got %d", hdm.IncludedFiles)
		}
		if hdm.ExcludedFiles != 2 {
			t.Errorf("expected ExcludedFiles 2, got %d", hdm.ExcludedFiles)
		}
		if hdm.TokensSaved != 500 {
			t.Errorf("expected TokensSaved 500, got %d", hdm.TokensSaved)
		}
		if hdm.FileSummaries == nil {
			t.Error("expected FileSummaries to be initialized")
		}
	})
}

func TestCheckUnreducible(t *testing.T) {
	setupTestRegistry()
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("returns nil when content fits", func(t *testing.T) {
		result := &OptimizationResult{
			TotalTokens: 5000,
		}

		err := co.CheckUnreducible(result, 10000)
		if err != nil {
			t.Errorf("expected nil error when content fits, got %v", err)
		}
	})

	t.Run("returns error when content exceeds limit", func(t *testing.T) {
		result := &OptimizationResult{
			TotalTokens: 15000,
		}

		err := co.CheckUnreducible(result, 10000)
		if err == nil {
			t.Fatal("expected error when content exceeds limit")
		}

		if err.CurrentTokens != 15000 {
			t.Errorf("expected CurrentTokens 15000, got %d", err.CurrentTokens)
		}
		if err.TargetLimit != 10000 {
			t.Errorf("expected TargetLimit 10000, got %d", err.TargetLimit)
		}
	})

	t.Run("provides model recommendations", func(t *testing.T) {
		result := &OptimizationResult{
			TotalTokens: 150000, // Needs large context model
		}

		err := co.CheckUnreducible(result, 10000)
		if err == nil {
			t.Fatal("expected error")
		}

		if len(err.RecommendedModels) == 0 {
			t.Error("expected model recommendations")
		}

		// Should recommend models with >= 150000 context
		for _, rec := range err.RecommendedModels {
			if rec.ContextLimit < 150000 {
				t.Errorf("recommended model %s has insufficient context %d", rec.Model, rec.ContextLimit)
			}
		}
	})
}

func TestPerformPreFlight(t *testing.T) {
	setupTestRegistry()
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("proceeds when content fits without optimization", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "main.go", Content: "package main", EstimatedTokens: 100},
		}

		result := co.PerformPreFlight(files, "claude-3-opus", nil)

		if !result.CanProceed {
			t.Error("expected CanProceed to be true")
		}
		if result.Error != nil {
			t.Errorf("expected no error, got %v", result.Error)
		}
		if result.Analysis.OptimizationDone {
			t.Error("expected no optimization needed")
		}
	})

	t.Run("optimizes when content exceeds limit", func(t *testing.T) {
		files := []FileWithPriority{
			{Path: "main.go", Content: "package main", EstimatedTokens: 5000},
			{Path: "utils.go", Content: "package util", EstimatedTokens: 5000},
		}

		result := co.PerformPreFlight(files, "gpt-4", nil) // 8192 limit

		if result.Analysis.OptimizationDone != true {
			t.Error("expected optimization to be performed")
		}
		if result.OptimizationResult == nil {
			t.Error("expected OptimizationResult to be set")
		}
	})

	t.Run("fails when content is unreducible", func(t *testing.T) {
		// Create multiple large files where even the smallest exceeds the limit
		files := []FileWithPriority{
			{Path: "huge1.go", Content: "huge content 1", EstimatedTokens: 50000},
			{Path: "huge2.go", Content: "huge content 2", EstimatedTokens: 50000},
		}

		result := co.PerformPreFlight(files, "gpt-4", nil) // 8192 limit

		// The optimizer will include one file (50000 tokens) which exceeds 8192
		// So optimization should fail
		if result.CanProceed {
			t.Error("expected CanProceed to be false for unreducible content")
		}
		if result.Error == nil {
			t.Error("expected error for unreducible content")
		}
		if len(result.Recommendations) == 0 {
			t.Error("expected model recommendations")
		}
	})
}

func TestUnreducibleContentError(t *testing.T) {
	err := &UnreducibleContentError{
		CurrentTokens: 50000,
		TargetLimit:   8192,
		Message:       "Content too large",
	}

	if err.Error() != "Content too large" {
		t.Errorf("expected error message 'Content too large', got '%s'", err.Error())
	}
}

func TestFindSuitableModels(t *testing.T) {
	setupTestRegistry()
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")
	co := NewContentOptimizer(te, fa, nil)

	t.Run("finds models for moderate token count", func(t *testing.T) {
		models := co.findSuitableModels(100000)
		if len(models) == 0 {
			t.Error("expected to find suitable models for 100K tokens")
		}
	})

	t.Run("returns empty for extremely large content", func(t *testing.T) {
		models := co.findSuitableModels(10000000) // 10M tokens
		if len(models) != 0 {
			t.Errorf("expected no models for 10M tokens, got %d", len(models))
		}
	})

	t.Run("limits recommendations to 3", func(t *testing.T) {
		models := co.findSuitableModels(50000)
		if len(models) > 3 {
			t.Errorf("expected at most 3 recommendations, got %d", len(models))
		}
	})
}
