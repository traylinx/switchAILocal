package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryStructure_Initialize(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)

	// Test initialization
	err := ds.Initialize()
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Verify base directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Errorf("Base directory was not created")
	}

	// Verify subdirectories exist
	subdirs := []string{
		userPreferencesDir,
		dailyLogsDir,
		analyticsDir,
	}

	for _, subdir := range subdirs {
		path := filepath.Join(baseDir, subdir)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Errorf("Subdirectory %s was not created", subdir)
		} else if !info.IsDir() {
			t.Errorf("Path %s is not a directory", subdir)
		}
	}

	// Verify files exist
	files := []string{
		routingHistoryFile,
		providerQuirksFile,
	}

	for _, file := range files {
		path := filepath.Join(baseDir, file)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			t.Errorf("File %s was not created", file)
		} else if info.IsDir() {
			t.Errorf("Path %s is a directory, not a file", file)
		}
	}
}

func TestDirectoryStructure_Initialize_Idempotent(t *testing.T) {
	// Create temporary directory for testing
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)

	// Initialize twice
	err := ds.Initialize()
	if err != nil {
		t.Fatalf("First Initialize() failed: %v", err)
	}

	err = ds.Initialize()
	if err != nil {
		t.Fatalf("Second Initialize() failed: %v", err)
	}

	// Verify structure is still valid
	err = ds.Validate()
	if err != nil {
		t.Errorf("Validate() failed after double initialization: %v", err)
	}
}

func TestDirectoryStructure_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(string) error
		wantErr bool
	}{
		{
			name: "valid structure",
			setup: func(baseDir string) error {
				ds := NewDirectoryStructure(baseDir)
				return ds.Initialize()
			},
			wantErr: false,
		},
		{
			name: "missing base directory",
			setup: func(baseDir string) error {
				// Don't create anything
				return nil
			},
			wantErr: true,
		},
		{
			name: "missing subdirectory",
			setup: func(baseDir string) error {
				ds := NewDirectoryStructure(baseDir)
				if err := ds.Initialize(); err != nil {
					return err
				}
				// Remove a subdirectory
				return os.RemoveAll(filepath.Join(baseDir, dailyLogsDir))
			},
			wantErr: true,
		},
		{
			name: "missing file",
			setup: func(baseDir string) error {
				ds := NewDirectoryStructure(baseDir)
				if err := ds.Initialize(); err != nil {
					return err
				}
				// Remove a file
				return os.Remove(filepath.Join(baseDir, routingHistoryFile))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			baseDir := filepath.Join(tmpDir, "memory")

			if err := tt.setup(baseDir); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			ds := NewDirectoryStructure(baseDir)
			err := ds.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDirectoryStructure_GetPaths(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Test path getters
	tests := []struct {
		name     string
		getPath  func() string
		expected string
	}{
		{
			name:     "routing history path",
			getPath:  ds.GetRoutingHistoryPath,
			expected: filepath.Join(baseDir, routingHistoryFile),
		},
		{
			name:     "provider quirks path",
			getPath:  ds.GetProviderQuirksPath,
			expected: filepath.Join(baseDir, providerQuirksFile),
		},
		{
			name:     "user preferences dir",
			getPath:  ds.GetUserPreferencesDir,
			expected: filepath.Join(baseDir, userPreferencesDir),
		},
		{
			name:     "daily logs dir",
			getPath:  ds.GetDailyLogsDir,
			expected: filepath.Join(baseDir, dailyLogsDir),
		},
		{
			name:     "analytics dir",
			getPath:  ds.GetAnalyticsDir,
			expected: filepath.Join(baseDir, analyticsDir),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.getPath()
			if got != tt.expected {
				t.Errorf("Path mismatch: got %s, want %s", got, tt.expected)
			}
		})
	}
}

func TestDirectoryStructure_GetUserPreferencePath(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)

	apiKeyHash := "sha256:abc123"
	expected := filepath.Join(baseDir, userPreferencesDir, "sha256:abc123.json")

	got := ds.GetUserPreferencePath(apiKeyHash)
	if got != expected {
		t.Errorf("GetUserPreferencePath() = %s, want %s", got, expected)
	}
}

func TestDirectoryStructure_GetDailyLogPath(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)

	date := "2026-02-02"
	expected := filepath.Join(baseDir, dailyLogsDir, "2026-02-02.jsonl")

	got := ds.GetDailyLogPath(date)
	if got != expected {
		t.Errorf("GetDailyLogPath() = %s, want %s", got, expected)
	}
}

func TestDirectoryStructure_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Check directory permissions
	info, err := os.Stat(baseDir)
	if err != nil {
		t.Fatalf("Failed to stat base directory: %v", err)
	}

	// On Unix systems, check that permissions are restrictive
	if info.Mode().Perm() != dirPermissions {
		t.Logf("Warning: Directory permissions are %o, expected %o", info.Mode().Perm(), dirPermissions)
	}

	// Check file permissions
	filePath := filepath.Join(baseDir, routingHistoryFile)
	info, err = os.Stat(filePath)
	if err != nil {
		t.Fatalf("Failed to stat routing history file: %v", err)
	}

	if info.Mode().Perm() != filePermissions {
		t.Logf("Warning: File permissions are %o, expected %o", info.Mode().Perm(), filePermissions)
	}
}

func TestDirectoryStructure_ProviderQuirksTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "memory")

	ds := NewDirectoryStructure(baseDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// Read provider quirks file
	filePath := ds.GetProviderQuirksPath()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read provider quirks file: %v", err)
	}

	// Verify template content exists
	contentStr := string(content)
	if len(contentStr) == 0 {
		t.Error("Provider quirks file is empty")
	}

	// Check for expected template sections
	expectedSections := []string{
		"# Provider Quirks & Workarounds",
		"## Format",
		"### Provider Name",
	}

	for _, section := range expectedSections {
		if !contains(contentStr, section) {
			t.Errorf("Provider quirks template missing section: %s", section)
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
