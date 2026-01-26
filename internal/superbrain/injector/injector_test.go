package injector

import (
	"bytes"
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
)

// TestNewStdinInjector tests the creation of a new stdin injector.
func TestNewStdinInjector(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid disabled mode",
			cfg: Config{
				Mode: "disabled",
			},
			wantErr: false,
		},
		{
			name: "valid conservative mode",
			cfg: Config{
				Mode: "conservative",
			},
			wantErr: false,
		},
		{
			name: "valid autopilot mode",
			cfg: Config{
				Mode: "autopilot",
			},
			wantErr: false,
		},
		{
			name: "invalid mode",
			cfg: Config{
				Mode: "invalid",
			},
			wantErr: true,
		},
		{
			name: "with custom patterns",
			cfg: Config{
				Mode: "autopilot",
				CustomPatterns: []config.StdinPattern{
					{
						Name:        "custom_test",
						Regex:       `test\?`,
						Response:    "yes\n",
						IsSafe:      true,
						Description: "Test pattern",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid custom pattern regex",
			cfg: Config{
				Mode: "autopilot",
				CustomPatterns: []config.StdinPattern{
					{
						Name:   "invalid",
						Regex:  `[invalid(`,
						IsSafe: true,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "with forbidden patterns",
			cfg: Config{
				Mode:              "autopilot",
				ForbiddenPatterns: []string{`dangerous`, `risky`},
			},
			wantErr: false,
		},
		{
			name: "invalid forbidden pattern regex",
			cfg: Config{
				Mode:              "autopilot",
				ForbiddenPatterns: []string{`[invalid(`},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injector, err := NewStdinInjector(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStdinInjector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && injector == nil {
				t.Error("NewStdinInjector() returned nil injector without error")
			}
		})
	}
}

// TestCanInject tests the mode-based injection behavior.
func TestCanInject(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		pattern *StdinPattern
		want    bool
	}{
		{
			name: "disabled mode - safe pattern",
			mode: "disabled",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: true,
			},
			want: false,
		},
		{
			name: "disabled mode - unsafe pattern",
			mode: "disabled",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: false,
			},
			want: false,
		},
		{
			name: "conservative mode - safe pattern",
			mode: "conservative",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: true,
			},
			want: true,
		},
		{
			name: "conservative mode - unsafe pattern",
			mode: "conservative",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: false,
			},
			want: false,
		},
		{
			name: "autopilot mode - safe pattern",
			mode: "autopilot",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: true,
			},
			want: true,
		},
		{
			name: "autopilot mode - unsafe pattern",
			mode: "autopilot",
			pattern: &StdinPattern{
				Name:   "test",
				IsSafe: false,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injector, err := NewStdinInjector(Config{Mode: tt.mode})
			if err != nil {
				t.Fatalf("NewStdinInjector() error = %v", err)
			}

			got := injector.CanInject(tt.pattern)
			if got != tt.want {
				t.Errorf("CanInject() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMatchPattern tests pattern matching in log content.
func TestMatchPattern(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "autopilot"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	tests := []struct {
		name       string
		logContent string
		wantMatch  bool
		wantName   string
	}{
		{
			name:       "claude file permission",
			logContent: "Allow read file.txt? [y/n]",
			wantMatch:  true,
			wantName:   "claude_file_permission",
		},
		{
			name:       "claude tool permission",
			logContent: "Allow tool execution? [y/n]",
			wantMatch:  true,
			wantName:   "claude_tool_permission",
		},
		{
			name:       "generic continue",
			logContent: "Press enter to continue",
			wantMatch:  true,
			wantName:   "generic_continue",
		},
		{
			name:       "no match",
			logContent: "This is just regular output",
			wantMatch:  false,
		},
		{
			name:       "case insensitive match",
			logContent: "ALLOW READ FILE? [Y/N]",
			wantMatch:  true,
			wantName:   "claude_file_permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern := injector.MatchPattern(tt.logContent)
			if tt.wantMatch {
				if pattern == nil {
					t.Error("MatchPattern() returned nil, expected a match")
					return
				}
				if pattern.Name != tt.wantName {
					t.Errorf("MatchPattern() matched pattern %s, want %s", pattern.Name, tt.wantName)
				}
			} else {
				if pattern != nil {
					t.Errorf("MatchPattern() returned pattern %s, expected no match", pattern.Name)
				}
			}
		})
	}
}

// TestIsForbidden tests forbidden pattern detection.
func TestIsForbidden(t *testing.T) {
	injector, err := NewStdinInjector(Config{
		Mode:              "autopilot",
		ForbiddenPatterns: []string{`custom_forbidden`},
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	tests := []struct {
		name       string
		logContent string
		want       bool
	}{
		{
			name:       "contains delete",
			logContent: "Do you want to delete this file?",
			want:       true,
		},
		{
			name:       "contains remove",
			logContent: "Remove all files?",
			want:       true,
		},
		{
			name:       "contains sudo",
			logContent: "Run with sudo?",
			want:       true,
		},
		{
			name:       "contains custom forbidden",
			logContent: "This is custom_forbidden operation",
			want:       true,
		},
		{
			name:       "safe content",
			logContent: "Allow read file?",
			want:       false,
		},
		{
			name:       "case insensitive forbidden",
			logContent: "DELETE this?",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injector.IsForbidden(tt.logContent)
			if got != tt.want {
				t.Errorf("IsForbidden() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInject tests writing to stdin.
func TestInject(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "autopilot"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "simple response",
			response: "y\n",
			wantErr:  false,
		},
		{
			name:     "newline only",
			response: "\n",
			wantErr:  false,
		},
		{
			name:     "multi-line response",
			response: "yes\nconfirm\n",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := injector.Inject(&buf, tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("Inject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				got := buf.String()
				if got != tt.response {
					t.Errorf("Inject() wrote %q, want %q", got, tt.response)
				}
			}
		})
	}

	// Test nil stdin
	t.Run("nil stdin", func(t *testing.T) {
		err := injector.Inject(nil, "y\n")
		if err == nil {
			t.Error("Inject() with nil stdin should return error")
		}
	})
}

// TestTryInject tests the complete injection flow.
func TestTryInject(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		logContent     string
		wantPattern    bool
		wantInjected   bool
		wantErr        bool
		expectedOutput string
	}{
		{
			name:           "autopilot mode - safe pattern match",
			mode:           "autopilot",
			logContent:     "Allow read file? [y/n]",
			wantPattern:    true,
			wantInjected:   true,
			wantErr:        false,
			expectedOutput: "y\n",
		},
		{
			name:         "disabled mode - pattern match but no injection",
			mode:         "disabled",
			logContent:   "Allow read file? [y/n]",
			wantPattern:  true, // Pattern is matched, but not injected
			wantInjected: false,
			wantErr:      false,
		},
		{
			name:         "forbidden pattern",
			mode:         "autopilot",
			logContent:   "Delete all files? [y/n]",
			wantPattern:  false,
			wantInjected: false,
			wantErr:      true,
		},
		{
			name:         "no pattern match",
			mode:         "autopilot",
			logContent:   "Just regular output",
			wantPattern:  false,
			wantInjected: false,
			wantErr:      false,
		},
		{
			name:           "conservative mode - safe pattern",
			mode:           "conservative",
			logContent:     "Allow tool execution? [y/n]",
			wantPattern:    true,
			wantInjected:   true,
			wantErr:        false,
			expectedOutput: "y\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injector, err := NewStdinInjector(Config{Mode: tt.mode})
			if err != nil {
				t.Fatalf("NewStdinInjector() error = %v", err)
			}

			var buf bytes.Buffer
			pattern, injected, err := injector.TryInject(tt.logContent, &buf)

			if (err != nil) != tt.wantErr {
				t.Errorf("TryInject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if (pattern != nil) != tt.wantPattern {
				t.Errorf("TryInject() pattern = %v, wantPattern %v", pattern != nil, tt.wantPattern)
			}

			if injected != tt.wantInjected {
				t.Errorf("TryInject() injected = %v, wantInjected %v", injected, tt.wantInjected)
			}

			if tt.wantInjected {
				got := buf.String()
				if got != tt.expectedOutput {
					t.Errorf("TryInject() wrote %q, want %q", got, tt.expectedOutput)
				}
			}
		})
	}
}

// TestSetMode tests runtime mode changes.
func TestSetMode(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "disabled"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	tests := []struct {
		name    string
		newMode string
		wantErr bool
	}{
		{
			name:    "change to conservative",
			newMode: "conservative",
			wantErr: false,
		},
		{
			name:    "change to autopilot",
			newMode: "autopilot",
			wantErr: false,
		},
		{
			name:    "change to disabled",
			newMode: "disabled",
			wantErr: false,
		},
		{
			name:    "invalid mode",
			newMode: "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := injector.SetMode(tt.newMode)
			if (err != nil) != tt.wantErr {
				t.Errorf("SetMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				got := injector.GetMode()
				if got != tt.newMode {
					t.Errorf("GetMode() = %v, want %v", got, tt.newMode)
				}
			}
		})
	}
}

// TestGetPatterns tests retrieving configured patterns.
func TestGetPatterns(t *testing.T) {
	injector, err := NewStdinInjector(Config{
		Mode: "autopilot",
		CustomPatterns: []config.StdinPattern{
			{
				Name:   "custom",
				Regex:  `custom\?`,
				IsSafe: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	patterns := injector.GetPatterns()
	if len(patterns) == 0 {
		t.Error("GetPatterns() returned empty slice")
	}

	// Should include default patterns + custom pattern
	defaultCount := len(DefaultStdinPatterns())
	expectedCount := defaultCount + 1
	if len(patterns) != expectedCount {
		t.Errorf("GetPatterns() returned %d patterns, want %d", len(patterns), expectedCount)
	}

	// Verify custom pattern is included
	found := false
	for _, p := range patterns {
		if p.Name == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetPatterns() did not include custom pattern")
	}
}

// TestForbiddenPatternBlocksMatch tests that forbidden patterns prevent matching.
func TestForbiddenPatternBlocksMatch(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "autopilot"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	// This log contains both a valid pattern AND a forbidden pattern
	logContent := "Allow read file? [y/n] This will delete everything."

	pattern := injector.MatchPattern(logContent)
	if pattern != nil {
		t.Errorf("MatchPattern() returned pattern %s, expected nil due to forbidden pattern", pattern.Name)
	}
}

// TestConcurrentAccess tests thread-safety of the injector.
func TestConcurrentAccess(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "autopilot"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	// Run multiple goroutines accessing the injector concurrently
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// Read operations
			_ = injector.GetMode()
			_ = injector.GetPatterns()
			_ = injector.MatchPattern("Allow read file? [y/n]")
			_ = injector.IsForbidden("delete")

			// Write operation
			var buf bytes.Buffer
			_, _, _ = injector.TryInject("Allow read file? [y/n]", &buf)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestCustomPatternOverridesDefault tests that custom patterns work alongside defaults.
func TestCustomPatternOverridesDefault(t *testing.T) {
	injector, err := NewStdinInjector(Config{
		Mode: "autopilot",
		CustomPatterns: []config.StdinPattern{
			{
				Name:        "my_custom",
				Regex:       `proceed\?`,
				Response:    "yes please\n",
				IsSafe:      true,
				Description: "Custom proceed prompt",
			},
		},
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	// Test custom pattern
	pattern := injector.MatchPattern("Do you want to proceed?")
	if pattern == nil {
		t.Fatal("MatchPattern() returned nil for custom pattern")
	}
	if pattern.Name != "my_custom" {
		t.Errorf("MatchPattern() matched %s, want my_custom", pattern.Name)
	}
	if pattern.Response != "yes please\n" {
		t.Errorf("Pattern response = %q, want %q", pattern.Response, "yes please\n")
	}

	// Test default pattern still works
	pattern = injector.MatchPattern("Allow read file? [y/n]")
	if pattern == nil {
		t.Fatal("MatchPattern() returned nil for default pattern")
	}
	if !strings.Contains(pattern.Name, "claude") {
		t.Errorf("MatchPattern() matched %s, expected a claude pattern", pattern.Name)
	}
}
