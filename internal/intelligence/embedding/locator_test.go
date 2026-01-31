// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewModelLocator tests model locator creation.
func TestNewModelLocator(t *testing.T) {
	locator := NewModelLocator()
	assert.NotNil(t, locator)
	assert.NotEmpty(t, locator.BaseDir)
}

// TestGetModelPath tests model path generation.
func TestGetModelPath(t *testing.T) {
	locator := NewModelLocator()

	tests := []struct {
		name      string
		modelName string
	}{
		{
			name:      "default model",
			modelName: DefaultModelName,
		},
		{
			name:      "custom model",
			modelName: "custom-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := locator.GetModelPath(tt.modelName)
			assert.Contains(t, path, tt.modelName)
			assert.Contains(t, path, "model.onnx")
		})
	}
}

// TestGetVocabPath tests vocabulary path generation.
func TestGetVocabPath(t *testing.T) {
	locator := NewModelLocator()

	path := locator.GetVocabPath(DefaultModelName)
	assert.Contains(t, path, DefaultModelName)
	assert.Contains(t, path, "vocab.txt")
}

// TestModelExists tests model existence check.
func TestModelExists(t *testing.T) {
	locator := NewModelLocator()

	// Non-existent model should return false
	exists := locator.ModelExists("non-existent-model-12345")
	assert.False(t, exists)

	// Default model may or may not exist depending on setup
	// Just verify the function doesn't panic
	_ = locator.ModelExists(DefaultModelName)
}

// TestEnsureModelDir tests model directory creation.
func TestEnsureModelDir(t *testing.T) {
	// Create a temporary base directory
	tmpDir, err := os.MkdirTemp("", "embedding-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	locator := &ModelLocator{BaseDir: tmpDir}

	modelName := "test-model"
	err = locator.EnsureModelDir(modelName)
	require.NoError(t, err)

	// Verify directory was created
	modelDir := filepath.Join(tmpDir, modelName)
	info, err := os.Stat(modelDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestGetSharedLibraryPath tests shared library path detection.
func TestGetSharedLibraryPath(t *testing.T) {
	locator := NewModelLocator()

	// This may or may not find a library depending on the system
	// Just verify the function doesn't panic
	path := locator.GetSharedLibraryPath()
	
	// If a path is returned, it should be a valid file
	if path != "" {
		_, err := os.Stat(path)
		assert.NoError(t, err, "Returned path should exist")
	}
}

// TestGetSharedLibraryPathEnvVar tests that environment variable is checked.
func TestGetSharedLibraryPathEnvVar(t *testing.T) {
	// Create a temporary file to simulate the library
	tmpFile, err := os.CreateTemp("", "libonnxruntime-test-*")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Set environment variable
	oldEnv := os.Getenv("ONNXRUNTIME_LIB_PATH")
	os.Setenv("ONNXRUNTIME_LIB_PATH", tmpFile.Name())
	defer os.Setenv("ONNXRUNTIME_LIB_PATH", oldEnv)

	locator := NewModelLocator()
	path := locator.GetSharedLibraryPath()

	// Should return the env var path
	assert.Equal(t, tmpFile.Name(), path)
}
