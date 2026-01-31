// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package embedding

import (
	"os"
	"path/filepath"
	"runtime"
)

// ModelLocator helps find model files and ONNX runtime libraries.
type ModelLocator struct {
	// BaseDir is the base directory for model storage
	BaseDir string
}

// NewModelLocator creates a new model locator with default paths.
func NewModelLocator() *ModelLocator {
	homeDir, _ := os.UserHomeDir()
	return &ModelLocator{
		BaseDir: filepath.Join(homeDir, ".switchailocal", "models"),
	}
}

// GetModelPath returns the path to the ONNX model file.
//
// Parameters:
//   - modelName: Name of the model (e.g., "all-MiniLM-L6-v2")
//
// Returns:
//   - string: Full path to the model file
func (l *ModelLocator) GetModelPath(modelName string) string {
	return filepath.Join(l.BaseDir, modelName, "model.onnx")
}

// GetVocabPath returns the path to the vocabulary file.
//
// Parameters:
//   - modelName: Name of the model
//
// Returns:
//   - string: Full path to the vocabulary file
func (l *ModelLocator) GetVocabPath(modelName string) string {
	return filepath.Join(l.BaseDir, modelName, "vocab.txt")
}

// GetSharedLibraryPath returns the path to the ONNX runtime shared library.
// It checks common installation locations based on the operating system.
//
// Returns:
//   - string: Path to the shared library, or empty string if not found
func (l *ModelLocator) GetSharedLibraryPath() string {
	var paths []string

	switch runtime.GOOS {
	case "darwin":
		// macOS paths
		paths = []string{
			"/usr/local/lib/libonnxruntime.dylib",
			"/opt/homebrew/lib/libonnxruntime.dylib",
			filepath.Join(l.BaseDir, "..", "lib", "libonnxruntime.dylib"),
		}
	case "linux":
		// Linux paths
		paths = []string{
			"/usr/local/lib/libonnxruntime.so",
			"/usr/lib/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
			filepath.Join(l.BaseDir, "..", "lib", "libonnxruntime.so"),
		}
	case "windows":
		// Windows paths
		paths = []string{
			"C:\\Program Files\\onnxruntime\\lib\\onnxruntime.dll",
			filepath.Join(l.BaseDir, "..", "lib", "onnxruntime.dll"),
		}
	}

	// Check environment variable first
	if envPath := os.Getenv("ONNXRUNTIME_LIB_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// Check common paths
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// ModelExists checks if the model files exist.
//
// Parameters:
//   - modelName: Name of the model
//
// Returns:
//   - bool: true if model files exist
func (l *ModelLocator) ModelExists(modelName string) bool {
	modelPath := l.GetModelPath(modelName)
	_, err := os.Stat(modelPath)
	return err == nil
}

// EnsureModelDir creates the model directory if it doesn't exist.
//
// Parameters:
//   - modelName: Name of the model
//
// Returns:
//   - error: Any error encountered
func (l *ModelLocator) EnsureModelDir(modelName string) error {
	modelDir := filepath.Join(l.BaseDir, modelName)
	return os.MkdirAll(modelDir, 0755)
}
