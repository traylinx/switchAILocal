// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditPermissions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Set up test environment
	t.Setenv("SWITCHAI_STATE_DIR", tempDir)
	t.Setenv("SWITCHAI_READONLY", "0")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}

	// Create test directory structure
	discoveryDir := sb.DiscoveryDir()
	if err := os.MkdirAll(discoveryDir, 0755); err != nil {
		t.Fatalf("Failed to create discovery directory: %v", err)
	}

	// Create test files with incorrect permissions
	registryPath := filepath.Join(discoveryDir, "registry.json")
	if err := os.WriteFile(registryPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create registry file: %v", err)
	}

	dbPath := filepath.Join(sb.IntelligenceDir(), "feedback.db")
	if err := os.MkdirAll(sb.IntelligenceDir(), 0755); err != nil {
		t.Fatalf("Failed to create intelligence directory: %v", err)
	}
	if err := os.WriteFile(dbPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Run audit
	results, err := AuditPermissions(sb)
	if err != nil {
		t.Fatalf("AuditPermissions failed: %v", err)
	}

	// Verify results
	if len(results) == 0 {
		t.Fatal("Expected audit results, got none")
	}

	// Check that directories and sensitive files are in results
	foundDir := false
	foundJSON := false
	foundDB := false

	for _, result := range results {
		if result.Error != nil {
			t.Errorf("Unexpected error in audit result for %s: %v", result.Path, result.Error)
		}

		info, err := os.Stat(result.Path)
		if err != nil {
			continue
		}

		if info.IsDir() {
			foundDir = true
			if result.RequiredMode != 0700 {
				t.Errorf("Directory %s should require mode 0700, got %04o", result.Path, result.RequiredMode)
			}
		} else if filepath.Ext(result.Path) == ".json" {
			foundJSON = true
			if result.RequiredMode != 0600 {
				t.Errorf("JSON file %s should require mode 0600, got %04o", result.Path, result.RequiredMode)
			}
		} else if filepath.Ext(result.Path) == ".db" {
			foundDB = true
			if result.RequiredMode != 0600 {
				t.Errorf("DB file %s should require mode 0600, got %04o", result.Path, result.RequiredMode)
			}
		}
	}

	if !foundDir {
		t.Error("Expected to find directory in audit results")
	}
	if !foundJSON {
		t.Error("Expected to find .json file in audit results")
	}
	if !foundDB {
		t.Error("Expected to find .db file in audit results")
	}
}

func TestHardenPermissions_DirectoryCorrection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Set up test environment
	t.Setenv("SWITCHAI_STATE_DIR", tempDir)
	t.Setenv("SWITCHAI_READONLY", "0")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}

	// Create test directory with incorrect permissions
	discoveryDir := sb.DiscoveryDir()
	if err := os.MkdirAll(discoveryDir, 0755); err != nil {
		t.Fatalf("Failed to create discovery directory: %v", err)
	}

	// Verify initial permissions are incorrect
	info, err := os.Stat(discoveryDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}
	if info.Mode().Perm() == 0700 {
		// Change to incorrect permissions for test
		if err := os.Chmod(discoveryDir, 0755); err != nil {
			t.Fatalf("Failed to set incorrect permissions: %v", err)
		}
	}

	// Run hardening
	if err := HardenPermissions(sb); err != nil {
		t.Fatalf("HardenPermissions failed: %v", err)
	}

	// Verify permissions were corrected
	info, err = os.Stat(discoveryDir)
	if err != nil {
		t.Fatalf("Failed to stat directory after hardening: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("Expected directory permissions 0700, got %04o", info.Mode().Perm())
	}
}

func TestHardenPermissions_JSONFileCorrection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Set up test environment
	t.Setenv("SWITCHAI_STATE_DIR", tempDir)
	t.Setenv("SWITCHAI_READONLY", "0")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}

	// Create test JSON file with incorrect permissions
	discoveryDir := sb.DiscoveryDir()
	if err := os.MkdirAll(discoveryDir, 0700); err != nil {
		t.Fatalf("Failed to create discovery directory: %v", err)
	}

	registryPath := filepath.Join(discoveryDir, "registry.json")
	if err := os.WriteFile(registryPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create registry file: %v", err)
	}

	// Run hardening
	if err := HardenPermissions(sb); err != nil {
		t.Fatalf("HardenPermissions failed: %v", err)
	}

	// Verify permissions were corrected
	info, err := os.Stat(registryPath)
	if err != nil {
		t.Fatalf("Failed to stat file after hardening: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected .json file permissions 0600, got %04o", info.Mode().Perm())
	}
}

func TestHardenPermissions_DBFileCorrection(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Set up test environment
	t.Setenv("SWITCHAI_STATE_DIR", tempDir)
	t.Setenv("SWITCHAI_READONLY", "0")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}

	// Create test DB file with incorrect permissions
	intelligenceDir := sb.IntelligenceDir()
	if err := os.MkdirAll(intelligenceDir, 0700); err != nil {
		t.Fatalf("Failed to create intelligence directory: %v", err)
	}

	dbPath := filepath.Join(intelligenceDir, "feedback.db")
	if err := os.WriteFile(dbPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create database file: %v", err)
	}

	// Run hardening
	if err := HardenPermissions(sb); err != nil {
		t.Fatalf("HardenPermissions failed: %v", err)
	}

	// Verify permissions were corrected
	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Failed to stat file after hardening: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("Expected .db file permissions 0600, got %04o", info.Mode().Perm())
	}
}

func TestHardenPermissions_NonExistentRoot(t *testing.T) {
	// Create a StateBox with a non-existent root
	tempDir := t.TempDir()
	nonExistentPath := filepath.Join(tempDir, "does-not-exist")

	t.Setenv("SWITCHAI_STATE_DIR", nonExistentPath)
	t.Setenv("SWITCHAI_READONLY", "0")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("Failed to create StateBox: %v", err)
	}

	// Run hardening - should not error, just log warning
	if err := HardenPermissions(sb); err != nil {
		t.Fatalf("HardenPermissions should not error on non-existent root: %v", err)
	}
}

func TestHardenPermissions_NilStateBox(t *testing.T) {
	// Test with nil StateBox
	err := HardenPermissions(nil)
	if err == nil {
		t.Fatal("Expected error when StateBox is nil")
	}
	if err.Error() != "StateBox cannot be nil" {
		t.Errorf("Expected 'StateBox cannot be nil' error, got: %v", err)
	}
}

func TestAuditPermissions_NilStateBox(t *testing.T) {
	// Test with nil StateBox
	_, err := AuditPermissions(nil)
	if err == nil {
		t.Fatal("Expected error when StateBox is nil")
	}
	if err.Error() != "StateBox cannot be nil" {
		t.Errorf("Expected 'StateBox cannot be nil' error, got: %v", err)
	}
}

func TestIsSensitiveFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"registry.json", true},
		{"feedback.db", true},
		{"config.JSON", true}, // Case insensitive
		{"data.DB", true},     // Case insensitive
		{"readme.txt", false},
		{"script.sh", false},
		{"noextension", false},
		{"/path/to/file.json", true},
		{"/path/to/file.db", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := isSensitiveFile(tt.path)
			if result != tt.expected {
				t.Errorf("isSensitiveFile(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}
