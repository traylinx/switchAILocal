package sculptor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFileAnalyzer(t *testing.T) {
	t.Run("creates analyzer with default base path", func(t *testing.T) {
		te := NewTokenEstimator("simple")
		fa := NewFileAnalyzer(te, "")
		if fa.basePath != "." {
			t.Errorf("expected default base path '.', got '%s'", fa.basePath)
		}
	})

	t.Run("creates analyzer with custom base path", func(t *testing.T) {
		te := NewTokenEstimator("simple")
		fa := NewFileAnalyzer(te, "/custom/path")
		if fa.basePath != "/custom/path" {
			t.Errorf("expected base path '/custom/path', got '%s'", fa.basePath)
		}
	})
}

func TestDetectFileReferences(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")

	t.Run("detects relative paths", func(t *testing.T) {
		content := "Please analyze ./src/main.go and ./README.md"
		refs := fa.DetectFileReferences(content)
		
		paths := make(map[string]bool)
		for _, ref := range refs {
			paths[ref.Path] = true
		}

		if !paths["./src/main.go"] {
			t.Error("expected to detect ./src/main.go")
		}
		if !paths["./README.md"] {
			t.Error("expected to detect ./README.md")
		}
	})

	t.Run("detects paths with common extensions", func(t *testing.T) {
		content := "Check config.yaml and package.json files"
		refs := fa.DetectFileReferences(content)

		paths := make(map[string]bool)
		for _, ref := range refs {
			paths[ref.Path] = true
		}

		if !paths["config.yaml"] {
			t.Error("expected to detect config.yaml")
		}
		if !paths["package.json"] {
			t.Error("expected to detect package.json")
		}
	})

	t.Run("deduplicates paths", func(t *testing.T) {
		content := "Check ./main.go and also ./main.go again"
		refs := fa.DetectFileReferences(content)

		count := 0
		for _, ref := range refs {
			if ref.Path == "./main.go" {
				count++
			}
		}

		if count != 1 {
			t.Errorf("expected 1 reference to ./main.go, got %d", count)
		}
	})
}

func TestDetectCLIReferences(t *testing.T) {
	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, ".")

	t.Run("detects paths in CLI args", func(t *testing.T) {
		args := []string{"--file", "./config.yaml", "-o", "./output.json"}
		refs := fa.DetectCLIReferences(args)

		paths := make(map[string]bool)
		for _, ref := range refs {
			paths[ref.Path] = true
		}

		if !paths["./config.yaml"] {
			t.Error("expected to detect ./config.yaml")
		}
		if !paths["./output.json"] {
			t.Error("expected to detect ./output.json")
		}
	})

	t.Run("ignores non-path args", func(t *testing.T) {
		args := []string{"--verbose", "true", "--count", "5"}
		refs := fa.DetectCLIReferences(args)

		if len(refs) != 0 {
			t.Errorf("expected 0 references for non-path args, got %d", len(refs))
		}
	})
}

func TestLooksLikePath(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"./main.go", true},
		{"../parent/file.ts", true},
		{"/absolute/path.py", true},
		{"src/utils/helper.js", true},
		{"config.yaml", true},
		{"README.md", true},
		{"package.json", true},
		{"true", false},
		{"5", false},
		{"--verbose", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := looksLikePath(tt.input)
			if result != tt.expected {
				t.Errorf("looksLikePath(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsTextFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"main.go", true},
		{"index.ts", true},
		{"app.py", true},
		{"config.yaml", true},
		{"data.json", true},
		{"README.md", true},
		{"Dockerfile", true},
		{"Makefile", true},
		{"image.png", false},
		{"binary.exe", false},
		{"archive.zip", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isTextFile(tt.path)
			if result != tt.expected {
				t.Errorf("isTextFile(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestAnalyzeRequest(t *testing.T) {
	// Create a temp directory with test files
	tempDir, err := os.MkdirTemp("", "sculptor_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	testFile := filepath.Join(tempDir, "test.go")
	testContent := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, tempDir)

	t.Run("analyzes request with file reference", func(t *testing.T) {
		content := "Please review test.go"
		analysis := fa.AnalyzeRequest(content, nil, "gpt-4")

		if analysis.ModelContextLimit != 8192 {
			t.Errorf("expected model limit 8192, got %d", analysis.ModelContextLimit)
		}

		if analysis.EstimatedTokens == 0 {
			t.Error("expected non-zero token estimate")
		}
	})

	t.Run("combines content and CLI args", func(t *testing.T) {
		content := "Analyze the code"
		cliArgs := []string{"test.go"}
		analysis := fa.AnalyzeRequest(content, cliArgs, "claude-3-opus")

		if analysis.ModelContextLimit != 200000 {
			t.Errorf("expected model limit 200000, got %d", analysis.ModelContextLimit)
		}
	})
}

func TestFileReference(t *testing.T) {
	// Create temp directory with test file
	tempDir, err := os.MkdirTemp("", "sculptor_ref_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "sample.txt")
	if err := os.WriteFile(testFile, []byte("hello world test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	te := NewTokenEstimator("simple")
	fa := NewFileAnalyzer(te, tempDir)

	t.Run("analyzes existing file", func(t *testing.T) {
		refs := fa.DetectFileReferences("Check sample.txt")
		
		var found *FileReference
		for i := range refs {
			if refs[i].Path == "sample.txt" {
				found = &refs[i]
				break
			}
		}

		if found == nil {
			t.Fatal("expected to find sample.txt reference")
		}

		if !found.Exists {
			t.Error("expected Exists to be true")
		}

		if found.IsDirectory {
			t.Error("expected IsDirectory to be false")
		}

		if found.FileCount != 1 {
			t.Errorf("expected FileCount 1, got %d", found.FileCount)
		}
	})

	t.Run("handles non-existent file", func(t *testing.T) {
		refs := fa.DetectFileReferences("Check nonexistent.go")
		
		var found *FileReference
		for i := range refs {
			if refs[i].Path == "nonexistent.go" {
				found = &refs[i]
				break
			}
		}

		if found == nil {
			t.Fatal("expected to find nonexistent.go reference")
		}

		if found.Exists {
			t.Error("expected Exists to be false for non-existent file")
		}
	})
}
