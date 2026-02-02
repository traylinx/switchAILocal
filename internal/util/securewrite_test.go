// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSecureWrite_SuccessfulWrite(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := []byte("test content")
	err = SecureWrite(sb, testFile, testData, nil)
	if err != nil {
		t.Fatalf("SecureWrite() failed: %v", err)
	}

	// Verify file exists and has correct content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != string(testData) {
		t.Errorf("Expected content %s, got %s", testData, content)
	}

	// Verify no temp files remain
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	for _, entry := range entries {
		if entry.Name() != "test.txt" {
			t.Errorf("Unexpected file in directory: %s", entry.Name())
		}
	}
}

func TestSecureWrite_ReadOnlyMode(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Setenv("SWITCHAI_READONLY", "1")
	defer os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := []byte("test content")
	err = SecureWrite(sb, testFile, testData, nil)

	if err != ErrReadOnlyMode {
		t.Errorf("Expected ErrReadOnlyMode, got %v", err)
	}

	// Verify file was not created
	_, err = os.Stat(testFile)
	if err == nil {
		t.Error("File should not exist in read-only mode")
	}
}

func TestSecureWrite_BackupCreation(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	// Write initial content
	initialData := []byte("initial content")
	err = SecureWrite(sb, testFile, initialData, nil)
	if err != nil {
		t.Fatalf("First SecureWrite() failed: %v", err)
	}

	// Write new content with backup enabled
	newData := []byte("new content")
	opts := &SecureWriteOptions{CreateBackup: true}
	err = SecureWrite(sb, testFile, newData, opts)
	if err != nil {
		t.Fatalf("Second SecureWrite() failed: %v", err)
	}

	// Verify backup file exists with original content
	backupFile := testFile + ".bak"
	backupContent, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("Failed to read backup file: %v", err)
	}

	if string(backupContent) != string(initialData) {
		t.Errorf("Expected backup content %s, got %s", initialData, backupContent)
	}

	// Verify main file has new content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read main file: %v", err)
	}

	if string(content) != string(newData) {
		t.Errorf("Expected file content %s, got %s", newData, content)
	}
}

func TestSecureWrite_Permissions(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := []byte("test content")
	opts := &SecureWriteOptions{Permissions: 0600}
	err = SecureWrite(sb, testFile, testData, opts)
	if err != nil {
		t.Fatalf("SecureWrite() failed: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Expected permissions 0600, got %o", mode)
	}
}

func TestSecureWriteJSON_SuccessfulWrite(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := map[string]interface{}{
		"key":   "value",
		"count": 42,
	}

	err = SecureWriteJSON(sb, testFile, testData, nil)
	if err != nil {
		t.Fatalf("SecureWriteJSON() failed: %v", err)
	}

	// Verify file exists and contains valid JSON
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var result map[string]interface{}
	err = json.Unmarshal(content, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if result["key"] != "value" || result["count"] != float64(42) {
		t.Errorf("JSON content mismatch: %v", result)
	}
}

func TestSecureWriteJSON_ReadOnlyMode(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.json")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Setenv("SWITCHAI_READONLY", "1")
	defer os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := map[string]interface{}{
		"key": "value",
	}

	err = SecureWriteJSON(sb, testFile, testData, nil)

	if err != ErrReadOnlyMode {
		t.Errorf("Expected ErrReadOnlyMode, got %v", err)
	}

	// Verify file was not created
	_, err = os.Stat(testFile)
	if err == nil {
		t.Error("File should not exist in read-only mode")
	}
}

func TestSecureWrite_CreateParentDirectories(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "nested", "deep", "dir", "test.txt")

	os.Setenv("SWITCHAI_STATE_DIR", tempDir)
	defer os.Unsetenv("SWITCHAI_STATE_DIR")
	os.Unsetenv("SWITCHAI_READONLY")

	sb, err := NewStateBox()
	if err != nil {
		t.Fatalf("NewStateBox() failed: %v", err)
	}

	testData := []byte("test content")
	err = SecureWrite(sb, testFile, testData, nil)
	if err != nil {
		t.Fatalf("SecureWrite() failed: %v", err)
	}

	// Verify file exists
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != string(testData) {
		t.Errorf("Expected content %s, got %s", testData, content)
	}
}

func TestSecureWrite_NilStateBox(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	testData := []byte("test content")
	err := SecureWrite(nil, testFile, testData, nil)
	if err != nil {
		t.Fatalf("SecureWrite() with nil StateBox failed: %v", err)
	}

	// Verify file exists
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != string(testData) {
		t.Errorf("Expected content %s, got %s", testData, content)
	}
}

