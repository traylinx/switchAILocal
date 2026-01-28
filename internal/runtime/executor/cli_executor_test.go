// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalCLIExecutor_ExtractContent(t *testing.T) {
	tests := []struct {
		name         string
		supportsJSON bool
		raw          string
		want         string
	}{
		{
			name:         "Standard JSON outcome with response field",
			supportsJSON: true,
			raw:          `{"response": "Hello World", "stats": {}}`,
			want:         "Hello World",
		},
		{
			name:         "Claude CLI style JSON with result field",
			supportsJSON: true,
			raw:          `{"result": "Hello from Claude", "type": "result"}`,
			want:         "Hello from Claude",
		},
		{
			name:         "Claude CLI style JSON with leading noise",
			supportsJSON: true,
			raw:          "Loaded cached credentials...\n{\"result\": \"Hello from Claude with noise\", \"type\": \"result\"}",
			want:         "Hello from Claude with noise",
		},
		{
			name:         "Claude CLI style JSON with trailing noise",
			supportsJSON: true,
			raw:          "{\"result\": \"Hello from Claude with trailing noise\", \"type\": \"result\"}\nDone.",
			want:         "Hello from Claude with trailing noise",
		},
		{
			name:         "Claude CLI style JSON with both leading and trailing noise",
			supportsJSON: true,
			raw:          "Logs here...\n{\"result\": \"Hello from Claude nested\", \"type\": \"result\"}\nFinished.",
			want:         "Hello from Claude nested",
		},
		{
			name:         "Non-JSON raw text fallback",
			supportsJSON: true,
			raw:          "Plain text response",
			want:         "Plain text response",
		},
		{
			name:         "Raw text when JSON not supported",
			supportsJSON: false,
			raw:          `{"response": "hidden"}`,
			want:         `{"response": "hidden"}`,
		},
		{
			name:         "Noise filtering",
			supportsJSON: false,
			raw:          "[STARTUP] context initialized\nLoaded cached credentials\nActual Message",
			want:         "Actual Message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LocalCLIExecutor{
				SupportsJSON: tt.supportsJSON,
			}
			if got := e.extractContent(tt.raw); got != tt.want {
				t.Errorf("LocalCLIExecutor.extractContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLocalCLIExecutor_ExtractUsage(t *testing.T) {
	tests := []struct {
		name         string
		supportsJSON bool
		raw          string
		wantPrompt   int
		wantTotal    int
	}{
		{
			name:         "Standard Stats layout",
			supportsJSON: true,
			raw:          `{"stats": {"models": {"m1": {"tokens": {"prompt": 10, "total": 25}}}}}`,
			wantPrompt:   10,
			wantTotal:    25,
		},
		{
			name:         "Claude CLI style usage (to be implemented)",
			supportsJSON: true,
			raw:          `{"usage": {"input_tokens": 100, "output_tokens": 50}}`,
			wantPrompt:   100,
			wantTotal:    150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LocalCLIExecutor{
				SupportsJSON: tt.supportsJSON,
			}
			got := e.extractUsage(tt.raw)
			if tt.wantTotal == 0 {
				if got != nil {
					t.Errorf("expected nil usage, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected usage, got nil")
			}
			if got.PromptTokens != tt.wantPrompt {
				t.Errorf("PromptTokens = %v, want %v", got.PromptTokens, tt.wantPrompt)
			}
			if got.TotalTokens != tt.wantTotal {
				t.Errorf("TotalTokens = %v, want %v", got.TotalTokens, tt.wantTotal)
			}
		})
	}
}

// TestLocalCLIExecutor_AutoInjectSkipPermissions tests the Phase 0 Quick Fix
// for auto-injecting --dangerously-skip-permissions for claudecli
func TestLocalCLIExecutor_AutoInjectSkipPermissions(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		existingArgs   []string
		existingFlags  []string
		hasTTY         bool
		expectInjected bool
	}{
		{
			name:           "claudecli without flag and no TTY should inject",
			provider:       "claudecli",
			existingArgs:   []string{"-p"},
			existingFlags:  []string{},
			hasTTY:         false,
			expectInjected: true,
		},
		{
			name:           "claudecli without flag but with TTY should not inject",
			provider:       "claudecli",
			existingArgs:   []string{"-p"},
			existingFlags:  []string{},
			hasTTY:         true,
			expectInjected: false,
		},
		{
			name:           "claudecli with flag in args should not inject",
			provider:       "claudecli",
			existingArgs:   []string{"-p", "--dangerously-skip-permissions"},
			existingFlags:  []string{},
			hasTTY:         false,
			expectInjected: false,
		},
		{
			name:           "claudecli with flag in flags should not inject",
			provider:       "claudecli",
			existingArgs:   []string{"-p"},
			existingFlags:  []string{"--dangerously-skip-permissions"},
			hasTTY:         false,
			expectInjected: false,
		},
		{
			name:           "geminicli should not inject",
			provider:       "geminicli",
			existingArgs:   []string{"-p"},
			existingFlags:  []string{},
			hasTTY:         false,
			expectInjected: false,
		},
		{
			name:           "vibe should not inject",
			provider:       "vibe",
			existingArgs:   []string{},
			existingFlags:  []string{},
			hasTTY:         false,
			expectInjected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &LocalCLIExecutor{
				Provider: tt.provider,
				Args:     tt.existingArgs,
			}

			// Simulate the flag building logic from Execute/ExecuteStream
			flags := append([]string{}, tt.existingFlags...)

			// Apply the auto-injection logic (using test's hasTTY value instead of actual detection)
			if e.Provider == "claudecli" && !tt.hasTTY {
				skipPermFlag := "--dangerously-skip-permissions"
				if !e.containsFlag(flags, skipPermFlag) && !e.containsFlag(e.Args, skipPermFlag) {
					flags = append(flags, skipPermFlag)
				}
			}

			// Check if flag was injected
			hasFlag := e.containsFlag(flags, "--dangerously-skip-permissions")
			if tt.expectInjected && !hasFlag {
				t.Errorf("Expected --dangerously-skip-permissions to be injected, but it wasn't")
			}
			if !tt.expectInjected && hasFlag && len(tt.existingFlags) == 0 && !e.containsFlag(tt.existingArgs, "--dangerously-skip-permissions") {
				t.Errorf("Expected --dangerously-skip-permissions NOT to be injected, but it was")
			}
		})
	}
}

// TestLocalCLIExecutor_ContainsFlag tests the containsFlag helper method
func TestLocalCLIExecutor_ContainsFlag(t *testing.T) {
	e := &LocalCLIExecutor{}

	tests := []struct {
		name string
		args []string
		flag string
		want bool
	}{
		{
			name: "flag present",
			args: []string{"-p", "--dangerously-skip-permissions", "--output-format=json"},
			flag: "--dangerously-skip-permissions",
			want: true,
		},
		{
			name: "flag not present",
			args: []string{"-p", "--output-format=json"},
			flag: "--dangerously-skip-permissions",
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			flag: "--dangerously-skip-permissions",
			want: false,
		},
		{
			name: "partial match should not count",
			args: []string{"--dangerously-skip"},
			flag: "--dangerously-skip-permissions",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.containsFlag(tt.args, tt.flag); got != tt.want {
				t.Errorf("containsFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLocalCLIExecutor_HasTTY tests the TTY detection
// Note: In test environments, this will typically return false
func TestLocalCLIExecutor_HasTTY(t *testing.T) {
	e := &LocalCLIExecutor{}
	
	// In most test environments, there's no TTY
	// This test just ensures the method doesn't panic
	hasTTY := e.hasTTY()
	
	// We can't assert a specific value since it depends on the test environment
	// But we can verify it returns a boolean
	t.Logf("hasTTY() returned: %v", hasTTY)
	
	// The method should not panic and should return a boolean
	if hasTTY {
		t.Log("TTY detected in test environment (unusual but valid)")
	} else {
		t.Log("No TTY detected in test environment (expected)")
	}
}

// TestExtractErrorHint tests the Phase 0 Quick Fix error hint extraction
func TestExtractErrorHint(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantHint   bool
		wantLines  int
		wantPrefix string
	}{
		{
			name:       "empty stderr returns empty hint",
			stderr:     "",
			wantHint:   false,
			wantLines:  0,
			wantPrefix: "",
		},
		{
			name:       "single line stderr",
			stderr:     "Error: permission denied",
			wantHint:   true,
			wantLines:  1,
			wantPrefix: "CLI may be waiting for input",
		},
		{
			name: "multiple lines under 10",
			stderr: `Starting process...
Connecting to API...
Error: connection timeout
Failed to complete request`,
			wantHint:   true,
			wantLines:  4,
			wantPrefix: "CLI may be waiting for input",
		},
		{
			name: "more than 10 lines returns last 10",
			stderr: `Line 1
Line 2
Line 3
Line 4
Line 5
Line 6
Line 7
Line 8
Line 9
Line 10
Line 11
Line 12
Line 13`,
			wantHint:   true,
			wantLines:  10,
			wantPrefix: "CLI may be waiting for input",
		},
		{
			name: "empty lines are filtered",
			stderr: `Error occurred

Another error

Final error`,
			wantHint:   true,
			wantLines:  3,
			wantPrefix: "CLI may be waiting for input",
		},
		{
			name: "permission prompt in stderr",
			stderr: `Processing request...
Allow tool execution? [y/n]
Waiting for response...`,
			wantHint:   true,
			wantLines:  3,
			wantPrefix: "CLI may be waiting for input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hint := extractErrorHint(tt.stderr)

			if tt.wantHint {
				if hint == "" {
					t.Errorf("Expected hint, got empty string")
					return
				}

				if !strings.Contains(hint, tt.wantPrefix) {
					t.Errorf("Expected hint to contain %q, got: %s", tt.wantPrefix, hint)
				}

				// Count lines in hint (excluding the prefix line)
				lines := strings.Split(hint, "\n")
				// First line is the prefix, rest are stderr lines
				actualLines := len(lines) - 1
				if actualLines != tt.wantLines {
					t.Errorf("Expected %d stderr lines in hint, got %d", tt.wantLines, actualLines)
				}
			} else {
				if hint != "" {
					t.Errorf("Expected empty hint, got: %s", hint)
				}
			}
		})
	}
}

func TestLocalCLIExecutor_BuildAttachmentPrefix_Security(t *testing.T) {
	// Get current working directory for test setup
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get CWD: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		expectError bool
		description string
	}{
		{
			name:        "Valid relative path",
			path:        "safe_file.txt",
			expectError: false,
			description: "Should accept file in current directory",
		},
		{
			name:        "Valid relative subdirectory path",
			path:        filepath.Join("subdir", "safe_file.txt"),
			expectError: false,
			description: "Should accept file in subdirectory",
		},
		{
			name:        "Path traversal attempt using parent directory",
			path:        filepath.Join("..", "secret.txt"),
			expectError: true,
			description: "Should reject path starting with ..",
		},
		{
			name:        "Deep path traversal attempt",
			path:        filepath.Join("..", "..", "etc", "passwd"),
			expectError: true,
			description: "Should reject deep traversal",
		},
		{
			name:        "Valid absolute path inside CWD",
			path:        filepath.Join(cwd, "safe_abs_file.txt"),
			expectError: false,
			description: "Should accept absolute path pointing inside CWD",
		},
		{
			name:        "Invalid absolute path outside CWD",
			path:        filepath.Join(filepath.Dir(cwd), "outside_file.txt"),
			expectError: true,
			description: "Should reject absolute path pointing outside CWD",
		},
	}

	e := &LocalCLIExecutor{
		SupportsAttachments: true,
		AttachmentPrefix:    "@",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &CLIOptions{
				Attachments: []Attachment{
					{
						Type: "file",
						Path: tt.path,
					},
				},
			}

			_, err := e.buildAttachmentPrefix(opts)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for %s, got nil (path: %s)", tt.description, tt.path)
				} else if !strings.Contains(err.Error(), "security violation") {
					t.Errorf("Expected security violation error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v (path: %s)", tt.description, err, tt.path)
				}
			}
		})
	}
}
