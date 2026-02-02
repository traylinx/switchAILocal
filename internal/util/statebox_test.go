// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStateBox_DefaultPath(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	// Should default to ~/.switchailocal
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	expected := filepath.Join(home, ".switchailocal")

	if sb.RootPath() != expected {
		t.Errorf("Expected root path %s, got %s", expected, sb.RootPath())
	}

	if sb.IsReadOnly() {
		t.Error("Expected read-only to be false by default")
	}
}

func TestNewStateBox_EnvVarOverride(t *testing.T) {
	// Set custom state directory
	customDir := "/tmp/custom-state"
	os.Setenv("SWITCHAI_STATE_DIR", customDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	if sb.RootPath() != customDir {
		t.Errorf("Expected root path %s, got %s", customDir, sb.RootPath())
	}
}

func TestNewStateBox_TildeExpansion(t *testing.T) {
	// Set state directory with tilde
	os.Setenv("SWITCHAI_STATE_DIR", "~/my-state")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	// Should expand tilde to home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	expected := filepath.Join(home, "my-state")

	if sb.RootPath() != expected {
		t.Errorf("Expected root path %s, got %s", expected, sb.RootPath())
	}
}

func TestNewStateBox_ReadOnlyMode(t *testing.T) {
	os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Setenv("SWITCHAI_READONLY", "1")
	defer os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	if !sb.IsReadOnly() {
		t.Error("Expected read-only to be true when SWITCHAI_READONLY=1")
	}
}

func TestStateBox_SubdirectoryAccessors(t *testing.T) {
	os.Setenv("SWITCHAI_STATE_DIR", "/test/state")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	tests := []struct {
		name     string
		method   func() string
		expected string
	}{
		{"DiscoveryDir", sb.DiscoveryDir, "/test/state/discovery"},
		{"IntelligenceDir", sb.IntelligenceDir, "/test/state/intelligence"},
		{"CredentialsDir", sb.CredentialsDir, "/test/state/credentials"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStateBox_ResolvePath(t *testing.T) {
	os.Setenv("SWITCHAI_STATE_DIR", "/test/state")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Empty path", "", "/test/state"},
		{"Relative path", "data/file.json", "/test/state/data/file.json"},
		{"Absolute path", "/absolute/path", "/absolute/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sb.ResolvePath(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStateBox_ResolvePath_Tilde(t *testing.T) {
	os.Setenv("SWITCHAI_STATE_DIR", "/test/state")
	defer os.Unsetenv("SWITCHAI_STATE_DIR")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	result := sb.ResolvePath("~/custom/path")
	expected := filepath.Join(home, "custom/path")

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestStateBox_EnsureDir(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	testDir := filepath.Join(tempDir, "test", "nested", "dir")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	// Test creating a new directory
	err = sb.EnsureDir(testDir)
	if err != nil {
		t.Fatalf("EnsureDir() failed: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Path exists but is not a directory")
	}

	// Verify permissions are 0700
	mode := info.Mode().Perm()
	if mode != 0700 {
		t.Errorf("Expected permissions 0700, got %o", mode)
	}

	// Test calling EnsureDir on existing directory (should not error)
	err = sb.EnsureDir(testDir)
	if err != nil {
		t.Errorf("EnsureDir() on existing directory failed: %v", err)
	}
}

func TestStateBox_EnsureDir_FileExists(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "file.txt")

	// Create a file
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	// Test calling EnsureDir on a file (should error)
	err = sb.EnsureDir(testFile)
	if err == nil {
		t.Error("Expected error when calling EnsureDir on a file")
	}

	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("Expected 'not a directory' error, got: %v", err)
	}
}
