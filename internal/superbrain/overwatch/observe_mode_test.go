// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package overwatch

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// TestObserveModeLogsButDoesNotAct verifies that in "observe" mode,
// the Overwatch layer monitors and logs but takes no autonomous actions.
func TestObserveModeLogsButDoesNotAct(t *testing.T) {
	// Create a configuration for observe mode
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  100, // 100ms for fast test
		LogBufferSize:       10,
		HeartbeatIntervalMs: 50, // Check every 50ms
		MaxRestartAttempts:  2,
	}

	// Track whether snapshot handler was called (with mutex for race-free access)
	var mu sync.Mutex
	snapshotCalled := false
	var capturedSnapshot *types.DiagnosticSnapshot

	// Create monitor with snapshot handler
	monitor := NewMonitor(cfg, func(ctx *OverwatchContext, snapshot *types.DiagnosticSnapshot) {
		mu.Lock()
		defer mu.Unlock()
		snapshotCalled = true
		capturedSnapshot = snapshot
	})
	defer monitor.Stop()

	// Start monitoring a simulated process
	ctx := monitor.StartMonitoring(12345, "test-provider", "test-model", "req-observe-1")
	if ctx == nil {
		t.Fatal("StartMonitoring returned nil context")
	}

	// Simulate initial output
	ctx.RecordOutput("Starting process...")
	ctx.RecordOutput("Initializing...")

	// Wait for silence threshold to be exceeded
	time.Sleep(200 * time.Millisecond)

	// Verify snapshot was captured (monitoring is working)
	mu.Lock()
	wasCalled := snapshotCalled
	snapshot := capturedSnapshot
	mu.Unlock()

	if !wasCalled {
		t.Error("Expected snapshot handler to be called after silence threshold")
	}

	if snapshot == nil {
		t.Fatal("Expected snapshot to be captured")
	}

	// Verify snapshot contains expected data
	if snapshot.Provider != "test-provider" {
		t.Errorf("Expected provider 'test-provider', got '%s'", snapshot.Provider)
	}

	if snapshot.Model != "test-model" {
		t.Errorf("Expected model 'test-model', got '%s'", snapshot.Model)
	}

	if snapshot.ProcessState != "blocked" {
		t.Errorf("Expected process state 'blocked', got '%s'", snapshot.ProcessState)
	}

	// Verify log buffer contains the output we recorded
	logLines := ctx.LogBuffer.GetAll()
	if len(logLines) != 2 {
		t.Errorf("Expected 2 log lines, got %d", len(logLines))
	}

	// In observe mode, the system should:
	// 1. Monitor the process ✓
	// 2. Detect silence ✓
	// 3. Capture diagnostic snapshot ✓
	// 4. Call the snapshot handler ✓
	// 5. NOT take any autonomous actions (restart, stdin injection, etc.)
	//    - This is verified by the fact that we only set up monitoring
	//    - No healing actions should be recorded in the context

	healingActions := ctx.GetHealingActions()
	if len(healingActions) != 0 {
		t.Errorf("Expected no healing actions in observe mode, got %d", len(healingActions))
	}

	monitor.StopMonitoring("req-observe-1")
}

// TestObserveModeStreamMonitoring verifies that stream monitoring works
// and records output correctly without taking actions.
func TestObserveModeStreamMonitoring(t *testing.T) {
	cfg := DefaultOverwatchConfig()
	cfg.SilenceThresholdMs = 100
	cfg.HeartbeatIntervalMs = 50

	monitor := NewMonitor(cfg, func(ctx *OverwatchContext, snapshot *types.DiagnosticSnapshot) {
		// Snapshot handler - just observe
	})
	defer monitor.Stop()

	ctx := monitor.StartMonitoring(99999, "test-provider", "test-model", "req-stream-1")
	if ctx == nil {
		t.Fatal("StartMonitoring returned nil context")
	}

	// Create simulated stdout and stderr streams
	stdout := bytes.NewBufferString("stdout line 1\nstdout line 2\nstdout line 3\n")
	stderr := bytes.NewBufferString("stderr line 1\nstderr line 2\n")

	// Create stream monitor
	streamMonitor := NewStreamMonitor(ctx)

	// Track lines received
	var stdoutLines []string
	var stderrLines []string

	// Start monitoring streams
	bgCtx := context.Background()
	streamMonitor.MonitorStreams(
		bgCtx,
		stdout,
		stderr,
		func(line string) {
			stdoutLines = append(stdoutLines, line)
		},
		func(line string) {
			stderrLines = append(stderrLines, line)
		},
	)

	// Wait for streams to complete
	streamMonitor.Wait()

	// Verify all lines were captured
	if len(stdoutLines) != 3 {
		t.Errorf("Expected 3 stdout lines, got %d", len(stdoutLines))
	}

	if len(stderrLines) != 2 {
		t.Errorf("Expected 2 stderr lines, got %d", len(stderrLines))
	}

	// Verify lines were recorded in the context's log buffer
	allLines := ctx.LogBuffer.GetAll()
	if len(allLines) != 5 {
		t.Errorf("Expected 5 total lines in buffer, got %d", len(allLines))
	}

	// Verify heartbeat was updated (context should be healthy)
	if !ctx.CheckHeartbeat() {
		t.Error("Expected heartbeat to be healthy after receiving output")
	}

	monitor.StopMonitoring("req-stream-1")
}

// TestObserveModeWrappedReaderWriter verifies that wrapped readers and writers
// correctly record output for monitoring purposes.
func TestObserveModeWrappedReaderWriter(t *testing.T) {
	cfg := DefaultOverwatchConfig()
	monitor := NewMonitor(cfg, nil)
	defer monitor.Stop()

	ctx := monitor.StartMonitoring(88888, "test-provider", "test-model", "req-wrap-1")
	if ctx == nil {
		t.Fatal("StartMonitoring returned nil context")
	}

	// Test wrapped reader
	t.Run("wrapped reader records output", func(t *testing.T) {
		input := bytes.NewBufferString("test data from reader")
		wrappedReader := monitor.WrapReader(input, ctx)

		buf := make([]byte, 1024)
		n, err := wrappedReader.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read failed: %v", err)
		}

		if n == 0 {
			t.Fatal("Expected to read data")
		}

		// Verify output was recorded
		time.Sleep(10 * time.Millisecond) // Give time for recording
		allLines := ctx.LogBuffer.GetAll()
		if len(allLines) == 0 {
			t.Error("Expected output to be recorded in log buffer")
		}
	})

	// Test wrapped writer
	t.Run("wrapped writer records output", func(t *testing.T) {
		var output bytes.Buffer
		wrappedWriter := monitor.WrapWriter(&output, ctx)

		testData := []byte("test data from writer")
		n, err := wrappedWriter.Write(testData)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}

		if n != len(testData) {
			t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
		}

		// Verify data was written to underlying writer
		if output.String() != string(testData) {
			t.Errorf("Expected output '%s', got '%s'", string(testData), output.String())
		}

		// Verify output was recorded
		time.Sleep(10 * time.Millisecond) // Give time for recording
		allLines := ctx.LogBuffer.GetAll()
		if len(allLines) == 0 {
			t.Error("Expected output to be recorded in log buffer")
		}
	})

	monitor.StopMonitoring("req-wrap-1")
}
