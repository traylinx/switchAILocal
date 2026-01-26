package sculptor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileAnalyzer detects and analyzes file references in request content.
// It scans for file/folder paths and estimates their token contribution.
type FileAnalyzer struct {
	estimator *TokenEstimator
	basePath  string
}

// NewFileAnalyzer creates a new FileAnalyzer with the given token estimator.
// basePath is the root directory for resolving relative file paths.
func NewFileAnalyzer(estimator *TokenEstimator, basePath string) *FileAnalyzer {
	if basePath == "" {
		basePath = "."
	}
	return &FileAnalyzer{
		estimator: estimator,
		basePath:  basePath,
	}
}

// FileReference represents a detected file or folder reference.
type FileReference struct {
	// Path is the detected file or folder path.
	Path string `json:"path"`

	// IsDirectory indicates whether the path is a directory.
	IsDirectory bool `json:"is_directory"`

	// Exists indicates whether the path exists on the filesystem.
	Exists bool `json:"exists"`

	// EstimatedTokens is the estimated token count for this file/folder.
	EstimatedTokens int `json:"estimated_tokens"`

	// FileCount is the number of files (1 for files, N for directories).
	FileCount int `json:"file_count"`
}

// filePathPatterns are regex patterns for detecting file references in content.
var filePathPatterns = []*regexp.Regexp{
	// Unix-style paths: /path/to/file, ./relative/path, ../parent/path
	regexp.MustCompile(`(?:^|\s|["'\(])([./][a-zA-Z0-9_\-./]+\.[a-zA-Z0-9]+)(?:["'\)]|\s|$)`),
	// Paths with common extensions
	regexp.MustCompile(`(?:^|\s|["'\(])([a-zA-Z0-9_\-./]+\.(?:go|ts|js|py|rs|java|c|cpp|h|hpp|md|txt|json|yaml|yml|toml|xml|html|css|scss|sql))(?:["'\)]|\s|$)`),
	// Directory paths ending with /
	regexp.MustCompile(`(?:^|\s|["'\(])([a-zA-Z0-9_\-./]+/)(?:["'\)]|\s|$)`),
}

// cliPathPatterns detect paths in CLI-style arguments.
var cliPathPatterns = []*regexp.Regexp{
	// --flag=/path/to/file or --flag /path/to/file
	regexp.MustCompile(`--[a-zA-Z\-]+=?["']?([./][a-zA-Z0-9_\-./]+)["']?`),
	// -f /path/to/file
	regexp.MustCompile(`-[a-zA-Z]\s+["']?([./][a-zA-Z0-9_\-./]+)["']?`),
}

// DetectFileReferences scans content for file and folder references.
// It returns a list of detected references with their metadata.
func (fa *FileAnalyzer) DetectFileReferences(content string) []FileReference {
	refs := make(map[string]bool) // Use map to deduplicate
	var results []FileReference

	// Apply all patterns
	allPatterns := append(filePathPatterns, cliPathPatterns...)
	for _, pattern := range allPatterns {
		matches := pattern.FindAllStringSubmatch(content, -1)
		for _, match := range matches {
			if len(match) > 1 {
				path := strings.TrimSpace(match[1])
				if path != "" && !refs[path] {
					refs[path] = true
				}
			}
		}
	}

	// Analyze each unique path
	for path := range refs {
		ref := fa.analyzeReference(path)
		results = append(results, ref)
	}

	return results
}

// DetectCLIReferences extracts file references from CLI arguments.
// This is specifically for extra_body.cli style arguments.
func (fa *FileAnalyzer) DetectCLIReferences(cliArgs []string) []FileReference {
	refs := make(map[string]bool)
	var results []FileReference

	for _, arg := range cliArgs {
		// Check if arg looks like a path
		if looksLikePath(arg) {
			if !refs[arg] {
				refs[arg] = true
			}
		}
	}

	for path := range refs {
		ref := fa.analyzeReference(path)
		results = append(results, ref)
	}

	return results
}

// looksLikePath checks if a string appears to be a file path.
func looksLikePath(s string) bool {
	// Starts with ./ or ../ or /
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/") {
		return true
	}
	// Contains path separator and has extension
	if strings.Contains(s, "/") && strings.Contains(s, ".") {
		return true
	}
	// Common file extensions
	extensions := []string{".go", ".ts", ".js", ".py", ".rs", ".java", ".md", ".txt", ".json", ".yaml", ".yml"}
	for _, ext := range extensions {
		if strings.HasSuffix(s, ext) {
			return true
		}
	}
	return false
}

// analyzeReference analyzes a single file reference.
func (fa *FileAnalyzer) analyzeReference(path string) FileReference {
	ref := FileReference{
		Path:      path,
		FileCount: 0,
	}

	// Resolve the full path
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(fa.basePath, path)
	}

	// Check if path exists
	info, err := os.Stat(fullPath)
	if err != nil {
		ref.Exists = false
		return ref
	}

	ref.Exists = true
	ref.IsDirectory = info.IsDir()

	if ref.IsDirectory {
		// Scan directory for files
		tokens, count := fa.scanDirectory(fullPath)
		ref.EstimatedTokens = tokens
		ref.FileCount = count
	} else {
		// Read and estimate single file
		content, err := os.ReadFile(fullPath)
		if err == nil {
			ref.EstimatedTokens = fa.estimator.EstimateTokens(string(content))
			ref.FileCount = 1
		}
	}

	return ref
}


// scanDirectory recursively scans a directory and estimates total tokens.
// Returns the total estimated tokens and file count.
func (fa *FileAnalyzer) scanDirectory(dirPath string) (int, int) {
	totalTokens := 0
	fileCount := 0

	// Skip common non-content directories
	skipDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
		".venv":        true,
		"__pycache__":  true,
		".idea":        true,
		".vscode":      true,
		"dist":         true,
		"build":        true,
		"target":       true,
	}

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip hidden and excluded directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip binary and non-text files
		if !isTextFile(path) {
			return nil
		}

		// Read and estimate file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		totalTokens += fa.estimator.EstimateTokens(string(content))
		fileCount++

		return nil
	})

	if err != nil {
		return 0, 0
	}

	return totalTokens, fileCount
}

// isTextFile checks if a file is likely a text file based on extension.
func isTextFile(path string) bool {
	textExtensions := map[string]bool{
		".go":     true,
		".ts":     true,
		".tsx":    true,
		".js":     true,
		".jsx":    true,
		".py":     true,
		".rs":     true,
		".java":   true,
		".c":      true,
		".cpp":    true,
		".h":      true,
		".hpp":    true,
		".cs":     true,
		".rb":     true,
		".php":    true,
		".swift":  true,
		".kt":     true,
		".scala":  true,
		".md":     true,
		".txt":    true,
		".json":   true,
		".yaml":   true,
		".yml":    true,
		".toml":   true,
		".xml":    true,
		".html":   true,
		".css":    true,
		".scss":   true,
		".less":   true,
		".sql":    true,
		".sh":     true,
		".bash":   true,
		".zsh":    true,
		".ps1":    true,
		".bat":    true,
		".cmd":    true,
		".env":    true,
		".gitignore": true,
		".dockerignore": true,
		".editorconfig": true,
		"Makefile": true,
		"Dockerfile": true,
	}

	ext := filepath.Ext(path)
	if textExtensions[ext] {
		return true
	}

	// Check for extensionless files that are commonly text
	base := filepath.Base(path)
	return textExtensions[base]
}

// AnalyzeRequest performs complete file reference analysis on request content.
// It combines content analysis and CLI argument analysis.
func (fa *FileAnalyzer) AnalyzeRequest(content string, cliArgs []string, targetModel string) *ContextAnalysis {
	analysis := NewContextAnalysis(targetModel)

	// Detect references from content
	contentRefs := fa.DetectFileReferences(content)

	// Detect references from CLI args
	cliRefs := fa.DetectCLIReferences(cliArgs)

	// Combine and deduplicate
	allRefs := make(map[string]FileReference)
	for _, ref := range contentRefs {
		allRefs[ref.Path] = ref
	}
	for _, ref := range cliRefs {
		if _, exists := allRefs[ref.Path]; !exists {
			allRefs[ref.Path] = ref
		}
	}

	// Calculate totals
	totalTokens := 0
	totalFiles := 0

	for _, ref := range allRefs {
		if ref.Exists {
			totalTokens += ref.EstimatedTokens
			totalFiles += ref.FileCount
			analysis.RelevantFiles = append(analysis.RelevantFiles, ref.Path)
		}
	}

	// Add base content tokens
	contentTokens := fa.estimator.EstimateTokens(content)
	totalTokens += contentTokens

	analysis.EstimatedTokens = totalTokens
	analysis.FileCount = totalFiles
	analysis.UpdateExceedsLimit()

	return analysis
}

// GetFileContent reads and returns the content of a file.
// Returns empty string if file cannot be read.
func (fa *FileAnalyzer) GetFileContent(path string) string {
	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(fa.basePath, path)
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	return string(content)
}

// ListDirectoryFiles returns all text files in a directory.
func (fa *FileAnalyzer) ListDirectoryFiles(dirPath string) []string {
	var files []string

	fullPath := dirPath
	if !filepath.IsAbs(dirPath) {
		fullPath = filepath.Join(fa.basePath, dirPath)
	}

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true,
		".venv": true, "__pycache__": true, ".idea": true,
		".vscode": true, "dist": true, "build": true, "target": true,
	}

	_ = filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}
		if isTextFile(path) {
			// Return relative path
			relPath, _ := filepath.Rel(fa.basePath, path)
			files = append(files, relPath)
		}
		return nil
	})

	return files
}
