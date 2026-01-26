package audit_test

import (
	"fmt"
	"log"

	"github.com/traylinx/switchAILocal/internal/superbrain/audit"
)

// ExampleLogger demonstrates basic usage of the audit logger.
func ExampleLogger() {
	// Create a new audit logger
	logger, err := audit.NewLogger(audit.Config{
		Enabled:    true,
		LogPath:    "./logs/superbrain_audit.log",
		MaxSizeMB:  100,
		MaxBackups: 10,
		MaxAgeDays: 30,
		Compress:   true,
	})
	if err != nil {
		log.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	// Log a stdin injection action
	logger.LogStdinInjection(
		"req-12345",
		"claudecli",
		"claude-sonnet-4",
		"permission_prompt",
		"y\n",
		"success",
	)

	// Log a restart action
	logger.LogRestart(
		"req-12345",
		"claudecli",
		"claude-sonnet-4",
		[]string{"--dangerously-skip-permissions"},
		"success",
	)

	// Log a fallback routing action
	logger.LogFallback(
		"req-12345",
		"claudecli",
		"geminicli",
		"claude-opus-4",
		"max_retries_exceeded",
		"success",
	)

	fmt.Println("Audit log entries written successfully")
	// Output: Audit log entries written successfully
}

// ExampleGlobal demonstrates usage of the global audit logger.
func ExampleGlobal() {
	// Initialize the global audit logger
	err := audit.InitGlobal(audit.Config{
		Enabled:    true,
		LogPath:    "./logs/superbrain_audit.log",
		MaxSizeMB:  100,
		MaxBackups: 10,
		MaxAgeDays: 30,
		Compress:   true,
	})
	if err != nil {
		log.Fatalf("Failed to initialize global audit logger: %v", err)
	}

	// Use the global logger from anywhere in the application
	audit.Global().LogDiagnosis(
		"req-67890",
		"claudecli",
		"claude-sonnet-4",
		"permission_prompt",
		"stdin_inject",
		0.95,
	)

	fmt.Println("Global audit logger initialized and used")
	// Output: Global audit logger initialized and used
}

// ExampleLogger_disabled demonstrates that a disabled logger is a no-op.
func ExampleLogger_disabled() {
	// Create a disabled logger
	logger, err := audit.NewLogger(audit.Config{
		Enabled: false,
	})
	if err != nil {
		log.Fatalf("Failed to create audit logger: %v", err)
	}
	defer logger.Close()

	// These calls are safe but do nothing
	logger.LogStdinInjection("req-1", "test", "test-model", "pattern", "response", "success")
	logger.LogRestart("req-2", "test", "test-model", []string{"--flag"}, "success")

	fmt.Println("Disabled logger is safe to use")
	// Output: Disabled logger is safe to use
}
