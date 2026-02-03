// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/memory"
)

func TestParseMemoryCommand(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expected    *MemoryOptions
		expectError bool
	}{
		{
			name: "init command",
			args: []string{"init"},
			expected: &MemoryOptions{
				Command: MemoryInit,
				Limit:   100,
				Format:  "text",
			},
			expectError: false,
		},
		{
			name: "status command",
			args: []string{"status"},
			expected: &MemoryOptions{
				Command: MemoryStatus,
				Limit:   100,
				Format:  "text",
			},
			expectError: false,
		},
		{
			name: "history command with limit",
			args: []string{"history", "--limit", "50"},
			expected: &MemoryOptions{
				Command: MemoryHistory,
				Limit:   50,
				Format:  "text",
			},
			expectError: false,
		},
		{
			name: "preferences command with API key",
			args: []string{"preferences", "--api-key", "sk-test-123"},
			expected: &MemoryOptions{
				Command: MemoryPreferences,
				Limit:   100,
				Format:  "text",
				APIKey:  "sk-test-123",
			},
			expectError: false,
		},
		{
			name: "preferences command with API key hash",
			args: []string{"preferences", "--api-key-hash", "sha256:abc123"},
			expected: &MemoryOptions{
				Command:    MemoryPreferences,
				Limit:      100,
				Format:     "text",
				APIKeyHash: "sha256:abc123",
			},
			expectError: false,
		},
		{
			name: "reset command with confirm",
			args: []string{"reset", "--confirm"},
			expected: &MemoryOptions{
				Command: MemoryReset,
				Limit:   100,
				Format:  "text",
				Confirm: true,
			},
			expectError: false,
		},
		{
			name: "export command with output",
			args: []string{"export", "--output", "backup.tar.gz"},
			expected: &MemoryOptions{
				Command: MemoryExport,
				Limit:   100,
				Format:  "text",
				Output:  "backup.tar.gz",
			},
			expectError: false,
		},
		{
			name:        "no command",
			args:        []string{},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "unknown command",
			args:        []string{"unknown"},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "limit without value",
			args:        []string{"history", "--limit"},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "invalid limit value",
			args:        []string{"history", "--limit", "invalid"},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "unknown option",
			args:        []string{"init", "--unknown"},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMemoryCommand(tt.args)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.Command != tt.expected.Command {
				t.Errorf("expected command %v, got %v", tt.expected.Command, result.Command)
			}

			if result.Limit != tt.expected.Limit {
				t.Errorf("expected limit %d, got %d", tt.expected.Limit, result.Limit)
			}

			if result.APIKey != tt.expected.APIKey {
				t.Errorf("expected API key %s, got %s", tt.expected.APIKey, result.APIKey)
			}

			if result.APIKeyHash != tt.expected.APIKeyHash {
				t.Errorf("expected API key hash %s, got %s", tt.expected.APIKeyHash, result.APIKeyHash)
			}

			if result.Confirm != tt.expected.Confirm {
				t.Errorf("expected confirm %v, got %v", tt.expected.Confirm, result.Confirm)
			}

			if result.Output != tt.expected.Output {
				t.Errorf("expected output %s, got %s", tt.expected.Output, result.Output)
			}

			if result.Format != tt.expected.Format {
				t.Errorf("expected format %s, got %s", tt.expected.Format, result.Format)
			}
		})
	}
}

func TestGetMemoryBaseDir(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.Config
		expected string
	}{
		{
			name: "with auth dir",
			config: &config.Config{
				AuthDir: "/home/user/.switchailocal/auth",
			},
			expected: "/home/user/.switchailocal/auth/memory",
		},
		{
			name:     "without auth dir",
			config:   &config.Config{},
			expected: "", // Will be user home directory + .switchailocal/memory
		},
		{
			name:     "nil config",
			config:   nil,
			expected: "", // Will be user home directory + .switchailocal/memory
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getMemoryBaseDir(tt.config)

			if tt.expected != "" {
				if result != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, result)
				}
			} else {
				// For cases where we expect user home directory + .switchailocal/memory
				home, err := os.UserHomeDir()
				if err != nil {
					// Fallback to CWD
					wd, _ := os.Getwd()
					home = wd
				}
				expected := filepath.Join(home, ".switchailocal", "memory")
				if result != expected {
					t.Errorf("expected %s, got %s", expected, result)
				}
			}
		})
	}
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected string
	}{
		{
			name:     "test key 1",
			apiKey:   "sk-test-123",
			expected: "sha256:e0dbaa0c6455768bf812d8345ec96a2677d1e3bf17dbb0020b115c80092811e6",
		},
		{
			name:     "test key 2",
			apiKey:   "sk-prod-456",
			expected: "sha256:a4765a0041c7976b145232fc80e8f75aa05b3da9ab57766315105e2bf34c32c6",
		},
		{
			name:     "empty key",
			apiKey:   "",
			expected: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashAPIKey(tt.apiKey)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestMaskAPIKeyHash(t *testing.T) {
	tests := []struct {
		name     string
		hash     string
		expected string
	}{
		{
			name:     "long hash",
			hash:     "sha256:8d969eef6ecad3c29a3a629280e686cf0c3f5d5a86aff3ca12020c923adc6c92",
			expected: "sha256:8d969eef6...",
		},
		{
			name:     "short hash",
			hash:     "short",
			expected: "short",
		},
		{
			name:     "exactly 16 chars",
			hash:     "1234567890123456",
			expected: "1234567890123456...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAPIKeyHash(tt.hash)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{
			name:     "bytes",
			bytes:    512,
			expected: "512 B",
		},
		{
			name:     "kilobytes",
			bytes:    1536, // 1.5 KB
			expected: "1.5 KB",
		},
		{
			name:     "megabytes",
			bytes:    1572864, // 1.5 MB
			expected: "1.5 MB",
		},
		{
			name:     "gigabytes",
			bytes:    1610612736, // 1.5 GB
			expected: "1.5 GB",
		},
		{
			name:     "zero bytes",
			bytes:    0,
			expected: "0 B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// Integration tests for memory commands

func TestMemoryCommandsIntegration(t *testing.T) {
	// Create temporary directory for testing
	tempDir := t.TempDir()

	// Create test config
	cfg := &config.Config{
		AuthDir: filepath.Join(tempDir, "auth"),
	}

	// Test memory init
	t.Run("memory init", func(t *testing.T) {
		// This would normally print to stdout, but we can't easily capture that in tests
		// Instead, we'll test that the memory manager can be created
		memoryConfig := &memory.MemoryConfig{
			Enabled:       true,
			BaseDir:       getMemoryBaseDir(cfg),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		}

		manager, err := memory.NewMemoryManager(memoryConfig)
		if err != nil {
			t.Errorf("failed to initialize memory manager: %v", err)
		}
		defer manager.Close()

		// Check that directory structure was created
		if _, err := os.Stat(memoryConfig.BaseDir); os.IsNotExist(err) {
			t.Errorf("memory base directory was not created")
		}
	})

	t.Run("memory status", func(t *testing.T) {
		// Initialize memory system first
		memoryConfig := &memory.MemoryConfig{
			Enabled:       true,
			BaseDir:       getMemoryBaseDir(cfg),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		}

		manager, err := memory.NewMemoryManager(memoryConfig)
		if err != nil {
			t.Errorf("failed to initialize memory manager: %v", err)
		}
		defer manager.Close()

		// Get stats to verify status command would work
		stats, err := manager.GetStats()
		if err != nil {
			t.Errorf("failed to get memory stats: %v", err)
		}

		if stats == nil {
			t.Errorf("stats should not be nil")
		}
	})

	t.Run("memory history", func(t *testing.T) {
		// Initialize memory system
		memoryConfig := &memory.MemoryConfig{
			Enabled:       true,
			BaseDir:       getMemoryBaseDir(cfg),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		}

		manager, err := memory.NewMemoryManager(memoryConfig)
		if err != nil {
			t.Errorf("failed to initialize memory manager: %v", err)
		}
		defer manager.Close()

		// Add a test routing decision
		decision := &memory.RoutingDecision{
			Timestamp:  time.Now(),
			APIKeyHash: "sha256:e0dbaa0c6455768bf812d8345ec96a2677d1e3bf17dbb0020b115c80092811e6",
			Request: memory.RequestInfo{
				Model:         "auto",
				Intent:        "coding",
				ContentHash:   "sha256:e0dbaa0c6455768bf812d8345ec96a2677d1e3bf17dbb0020b115c80092811e6",
				ContentLength: 100,
			},
			Routing: memory.RoutingInfo{
				Tier:          "semantic",
				SelectedModel: "anthropic:claude-sonnet-4",
				Confidence:    0.95,
				LatencyMs:     15,
			},
			Outcome: memory.OutcomeInfo{
				Success:        true,
				ResponseTimeMs: 2000,
				QualityScore:   0.9,
			},
		}

		err = manager.RecordRouting(decision)
		if err != nil {
			t.Errorf("failed to record routing decision: %v", err)
		}

		// Get history to verify it was recorded
		history, err := manager.GetAllHistory(10)
		if err != nil {
			t.Errorf("failed to get history: %v", err)
		}

		if len(history) == 0 {
			t.Errorf("expected at least one routing decision in history")
		}
	})

	t.Run("memory preferences", func(t *testing.T) {
		// Initialize memory system
		memoryConfig := &memory.MemoryConfig{
			Enabled:       true,
			BaseDir:       getMemoryBaseDir(cfg),
			RetentionDays: 90,
			MaxLogSizeMB:  100,
			Compression:   true,
		}

		manager, err := memory.NewMemoryManager(memoryConfig)
		if err != nil {
			t.Errorf("failed to initialize memory manager: %v", err)
		}
		defer manager.Close()

		// Get preferences for a test API key
		apiKeyHash := "sha256:e0dbaa0c6455768bf812d8345ec96a2677d1e3bf17dbb0020b115c80092811e6"
		preferences, err := manager.GetUserPreferences(apiKeyHash)
		if err != nil {
			t.Errorf("failed to get user preferences: %v", err)
		}

		if preferences == nil {
			t.Fatal("preferences should not be nil")
		}

		if preferences.APIKeyHash != apiKeyHash {
			t.Errorf("expected API key hash %s, got %s", apiKeyHash, preferences.APIKeyHash)
		}
	})
}

// Property-based tests for memory CLI commands

func TestMemoryCommandsProperties(t *testing.T) {
	// Property 1: All valid memory commands should parse successfully
	t.Run("Property: Valid commands parse successfully", func(t *testing.T) {
		validCommands := []string{"init", "status", "history", "preferences", "reset", "export"}

		for _, cmd := range validCommands {
			opts, err := ParseMemoryCommand([]string{cmd})
			if err != nil {
				t.Errorf("valid command %s should parse successfully, got error: %v", cmd, err)
			}
			if opts == nil {
				t.Errorf("valid command %s should return non-nil options", cmd)
			}
		}
	})

	// Property 2: Invalid commands should always return an error
	t.Run("Property: Invalid commands return errors", func(t *testing.T) {
		invalidCommands := []string{"invalid", "unknown", "badcmd", ""}

		for _, cmd := range invalidCommands {
			if cmd == "" {
				// Empty command is handled by empty args case
				_, err := ParseMemoryCommand([]string{})
				if err == nil {
					t.Errorf("empty command should return error")
				}
			} else {
				_, err := ParseMemoryCommand([]string{cmd})
				if err == nil {
					t.Errorf("invalid command %s should return error", cmd)
				}
			}
		}
	})

	// Property 3: API key hashing is deterministic
	t.Run("Property: API key hashing is deterministic", func(t *testing.T) {
		testKeys := []string{"sk-test-123", "sk-prod-456", "api-key-789", ""}

		for _, key := range testKeys {
			hash1 := hashAPIKey(key)
			hash2 := hashAPIKey(key)

			if hash1 != hash2 {
				t.Errorf("API key hashing should be deterministic for key %s", key)
			}

			// Hash should always start with "sha256:"
			if !strings.HasPrefix(hash1, "sha256:") {
				t.Errorf("API key hash should start with 'sha256:' for key %s", key)
			}
		}
	})

	// Property 4: Memory base directory is always valid
	t.Run("Property: Memory base directory is always valid", func(t *testing.T) {
		configs := []*config.Config{
			nil,
			{},
			{AuthDir: "/tmp/test"},
			{AuthDir: "/home/user/.switchailocal/auth"},
		}

		for i, cfg := range configs {
			baseDir := getMemoryBaseDir(cfg)

			if baseDir == "" {
				t.Errorf("memory base directory should never be empty for config %d", i)
			}

			if !filepath.IsAbs(baseDir) {
				t.Errorf("memory base directory should be absolute path for config %d, got: %s", i, baseDir)
			}
		}
	})

	// Property 5: Byte formatting is consistent
	t.Run("Property: Byte formatting is consistent", func(t *testing.T) {
		testSizes := []int64{0, 1, 512, 1024, 1536, 1048576, 1073741824}

		for _, size := range testSizes {
			formatted := formatBytes(size)

			if formatted == "" {
				t.Errorf("formatted bytes should never be empty for size %d", size)
			}

			// Should contain a number and a unit
			if !strings.Contains(formatted, " ") {
				t.Errorf("formatted bytes should contain space between number and unit for size %d", size)
			}
		}
	})
}
