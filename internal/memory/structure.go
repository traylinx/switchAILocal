package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// Directory structure constants
	routingHistoryFile = "routing-history.jsonl"
	providerQuirksFile = "provider-quirks.md"
	userPreferencesDir = "user-preferences"
	dailyLogsDir       = "daily"
	analyticsDir       = "analytics"
	
	// File permissions
	dirPermissions  = 0700 // Owner read/write/execute only
	filePermissions = 0600 // Owner read/write only
)

// DirectoryStructure manages the memory system directory structure.
type DirectoryStructure struct {
	baseDir string
}

// NewDirectoryStructure creates a new directory structure manager.
func NewDirectoryStructure(baseDir string) *DirectoryStructure {
	return &DirectoryStructure{
		baseDir: baseDir,
	}
}

// Initialize creates the complete directory structure for the memory system.
// It creates all necessary directories and files with proper permissions.
// Returns an error if any directory or file creation fails.
func (ds *DirectoryStructure) Initialize() error {
	// Create base directory
	if err := ds.createDir(ds.baseDir); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	// Create subdirectories
	subdirs := []string{
		userPreferencesDir,
		dailyLogsDir,
		analyticsDir,
	}

	for _, subdir := range subdirs {
		path := filepath.Join(ds.baseDir, subdir)
		if err := ds.createDir(path); err != nil {
			return fmt.Errorf("failed to create %s directory: %w", subdir, err)
		}
	}

	// Create initial files if they don't exist
	if err := ds.createFileIfNotExists(filepath.Join(ds.baseDir, routingHistoryFile)); err != nil {
		return fmt.Errorf("failed to create routing history file: %w", err)
	}

	if err := ds.createProviderQuirksFile(); err != nil {
		return fmt.Errorf("failed to create provider quirks file: %w", err)
	}

	return nil
}

// createDir creates a directory with proper permissions if it doesn't exist.
func (ds *DirectoryStructure) createDir(path string) error {
	// Check if directory already exists
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		// Directory already exists, ensure correct permissions
		return os.Chmod(path, dirPermissions)
	}

	// Create directory with proper permissions
	if err := os.MkdirAll(path, dirPermissions); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	return nil
}

// createFileIfNotExists creates an empty file if it doesn't exist.
func (ds *DirectoryStructure) createFileIfNotExists(path string) error {
	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		// File exists, ensure correct permissions
		return os.Chmod(path, filePermissions)
	}

	// Create empty file with proper permissions
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, filePermissions)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer file.Close()

	return nil
}

// createProviderQuirksFile creates the provider quirks markdown file with a template.
func (ds *DirectoryStructure) createProviderQuirksFile() error {
	path := filepath.Join(ds.baseDir, providerQuirksFile)

	// Check if file already exists
	if _, err := os.Stat(path); err == nil {
		// File exists, ensure correct permissions
		return os.Chmod(path, filePermissions)
	}

	// Create file with template content
	template := `# Provider Quirks & Workarounds

This file tracks known issues with providers and their workarounds.
Quirks are automatically discovered and documented during operation.

## Format

Each quirk entry should follow this format:

### Provider Name
- **Issue**: Description of the issue
- **Workaround**: How to work around the issue
- **Discovered**: Date when the issue was first discovered
- **Frequency**: How often the issue occurs
- **Severity**: low, medium, high, or critical

---

`

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, filePermissions)
	if err != nil {
		return fmt.Errorf("failed to create provider quirks file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(template); err != nil {
		return fmt.Errorf("failed to write template to provider quirks file: %w", err)
	}

	return nil
}

// Validate checks if the directory structure is valid and complete.
func (ds *DirectoryStructure) Validate() error {
	// Check base directory exists
	if info, err := os.Stat(ds.baseDir); err != nil {
		return fmt.Errorf("base directory does not exist: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("base path is not a directory: %s", ds.baseDir)
	}

	// Check required subdirectories
	requiredDirs := []string{
		userPreferencesDir,
		dailyLogsDir,
		analyticsDir,
	}

	for _, dir := range requiredDirs {
		path := filepath.Join(ds.baseDir, dir)
		if info, err := os.Stat(path); err != nil {
			return fmt.Errorf("required directory %s does not exist: %w", dir, err)
		} else if !info.IsDir() {
			return fmt.Errorf("required path %s is not a directory", dir)
		}
	}

	// Check required files
	requiredFiles := []string{
		routingHistoryFile,
		providerQuirksFile,
	}

	for _, file := range requiredFiles {
		path := filepath.Join(ds.baseDir, file)
		if info, err := os.Stat(path); err != nil {
			return fmt.Errorf("required file %s does not exist: %w", file, err)
		} else if info.IsDir() {
			return fmt.Errorf("required path %s is a directory, not a file", file)
		}
	}

	return nil
}

// GetRoutingHistoryPath returns the full path to the routing history file.
func (ds *DirectoryStructure) GetRoutingHistoryPath() string {
	return filepath.Join(ds.baseDir, routingHistoryFile)
}

// GetProviderQuirksPath returns the full path to the provider quirks file.
func (ds *DirectoryStructure) GetProviderQuirksPath() string {
	return filepath.Join(ds.baseDir, providerQuirksFile)
}

// GetUserPreferencesDir returns the full path to the user preferences directory.
func (ds *DirectoryStructure) GetUserPreferencesDir() string {
	return filepath.Join(ds.baseDir, userPreferencesDir)
}

// GetDailyLogsDir returns the full path to the daily logs directory.
func (ds *DirectoryStructure) GetDailyLogsDir() string {
	return filepath.Join(ds.baseDir, dailyLogsDir)
}

// GetAnalyticsDir returns the full path to the analytics directory.
func (ds *DirectoryStructure) GetAnalyticsDir() string {
	return filepath.Join(ds.baseDir, analyticsDir)
}

// GetUserPreferencePath returns the full path to a specific user's preferences file.
func (ds *DirectoryStructure) GetUserPreferencePath(apiKeyHash string) string {
	return filepath.Join(ds.GetUserPreferencesDir(), fmt.Sprintf("%s.json", apiKeyHash))
}

// GetDailyLogPath returns the full path to a specific day's log file.
func (ds *DirectoryStructure) GetDailyLogPath(date string) string {
	return filepath.Join(ds.GetDailyLogsDir(), fmt.Sprintf("%s.jsonl", date))
}
