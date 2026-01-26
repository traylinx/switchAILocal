package injector

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
)

// TestAuditLogging tests that injection attempts are logged to the audit log.
func TestAuditLogging(t *testing.T) {
	// Create a temporary directory for the audit log
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	// Create an audit logger
	auditLogger, err := audit.NewLogger(audit.Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// Create injector with audit logger
	injector, err := NewStdinInjector(Config{
		Mode:        "autopilot",
		AuditLogger: auditLogger,
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	tests := []struct {
		name           string
		logContent     string
		requestID      string
		provider       string
		model          string
		expectedOutcome string
	}{
		{
			name:            "successful injection",
			logContent:      "Allow read file? [y/n]",
			requestID:       "req-123",
			provider:        "claudecli",
			model:           "claude-3-opus",
			expectedOutcome: "success",
		},
		{
			name:            "forbidden pattern",
			logContent:      "Delete all files? [y/n]",
			requestID:       "req-456",
			provider:        "claudecli",
			model:           "claude-3-opus",
			expectedOutcome: "blocked_forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			_, _, _ = injector.TryInjectWithContext(tt.logContent, &buf, tt.requestID, tt.provider, tt.model)

			// Read the audit log
			logContent, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("Failed to read audit log: %v", err)
			}

			logStr := string(logContent)

			// Verify the log contains the expected information
			if !strings.Contains(logStr, tt.requestID) {
				t.Errorf("Audit log does not contain request ID %s", tt.requestID)
			}
			if !strings.Contains(logStr, tt.provider) {
				t.Errorf("Audit log does not contain provider %s", tt.provider)
			}
			if !strings.Contains(logStr, tt.model) {
				t.Errorf("Audit log does not contain model %s", tt.model)
			}
			if !strings.Contains(logStr, tt.expectedOutcome) {
				t.Errorf("Audit log does not contain outcome %s", tt.expectedOutcome)
			}
			if !strings.Contains(logStr, "stdin_injection") {
				t.Error("Audit log does not contain action_type stdin_injection")
			}
		})
	}
}

// TestAuditLoggingDisabledMode tests that blocked injections are logged.
func TestAuditLoggingDisabledMode(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	auditLogger, err := audit.NewLogger(audit.Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	// Create injector in disabled mode
	injector, err := NewStdinInjector(Config{
		Mode:        "disabled",
		AuditLogger: auditLogger,
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	var buf bytes.Buffer
	_, injected, _ := injector.TryInjectWithContext("Allow read file? [y/n]", &buf, "req-789", "claudecli", "claude-3-opus")

	if injected {
		t.Error("Injection should not have occurred in disabled mode")
	}

	// Read the audit log
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	logStr := string(logContent)

	// Verify the blocked attempt was logged
	if !strings.Contains(logStr, "req-789") {
		t.Error("Audit log does not contain request ID")
	}
	if !strings.Contains(logStr, "blocked_mode") {
		t.Error("Audit log does not contain blocked_mode outcome")
	}
}

// TestAuditLoggingWithGlobalLogger tests that the global logger is used when no logger is provided.
func TestAuditLoggingWithGlobalLogger(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "global_audit.log")

	// Initialize global audit logger
	err := audit.InitGlobal(audit.Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to initialize global audit logger: %v", err)
	}
	defer audit.Global().Close()

	// Create injector without explicit audit logger (should use global)
	injector, err := NewStdinInjector(Config{
		Mode: "autopilot",
		// No AuditLogger specified
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	var buf bytes.Buffer
	_, _, _ = injector.TryInjectWithContext("Allow read file? [y/n]", &buf, "req-global", "claudecli", "claude-3-opus")

	// Read the audit log
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	logStr := string(logContent)

	// Verify the log contains the expected information
	if !strings.Contains(logStr, "req-global") {
		t.Error("Audit log does not contain request ID")
	}
	if !strings.Contains(logStr, "stdin_injection") {
		t.Error("Audit log does not contain action_type")
	}
}

// TestTryInjectBackwardCompatibility tests that TryInject (without context) still works.
func TestTryInjectBackwardCompatibility(t *testing.T) {
	injector, err := NewStdinInjector(Config{Mode: "autopilot"})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	var buf bytes.Buffer
	pattern, injected, err := injector.TryInject("Allow read file? [y/n]", &buf)

	if err != nil {
		t.Errorf("TryInject() error = %v", err)
	}
	if !injected {
		t.Error("TryInject() should have injected")
	}
	if pattern == nil {
		t.Error("TryInject() should have returned a pattern")
	}
	if buf.String() != "y\n" {
		t.Errorf("TryInject() wrote %q, want %q", buf.String(), "y\n")
	}
}

// TestAuditLoggingInjectionFailure tests that failed injections are logged.
func TestAuditLoggingInjectionFailure(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	auditLogger, err := audit.NewLogger(audit.Config{
		Enabled: true,
		LogPath: logPath,
	})
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	injector, err := NewStdinInjector(Config{
		Mode:        "autopilot",
		AuditLogger: auditLogger,
	})
	if err != nil {
		t.Fatalf("NewStdinInjector() error = %v", err)
	}

	// Try to inject with nil stdin (should fail)
	_, injected, err := injector.TryInjectWithContext("Allow read file? [y/n]", nil, "req-fail", "claudecli", "claude-3-opus")

	if err == nil {
		t.Error("TryInjectWithContext() should have returned an error for nil stdin")
	}
	if injected {
		t.Error("Injection should not have succeeded with nil stdin")
	}

	// Read the audit log
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read audit log: %v", err)
	}

	logStr := string(logContent)

	// Verify the failed attempt was logged
	if !strings.Contains(logStr, "req-fail") {
		t.Error("Audit log does not contain request ID")
	}
	if !strings.Contains(logStr, "failed") {
		t.Error("Audit log does not contain failed outcome")
	}
}
