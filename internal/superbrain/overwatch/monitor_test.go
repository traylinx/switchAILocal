package overwatch

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

func TestNewMonitor(t *testing.T) {
	t.Run("creates monitor with default config", func(t *testing.T) {
		m := NewMonitor(nil, nil)
		if m == nil {
			t.Fatal("expected non-nil monitor")
		}
		if m.config == nil {
			t.Error("expected non-nil config")
		}
	})

	t.Run("creates monitor with custom config", func(t *testing.T) {
		cfg := &config.OverwatchConfig{
			SilenceThresholdMs:  5000,
			LogBufferSize:       100,
			HeartbeatIntervalMs: 500,
			MaxRestartAttempts:  5,
		}
		m := NewMonitor(cfg, nil)

		if m.config.SilenceThresholdMs != 5000 {
			t.Errorf("expected SilenceThresholdMs 5000, got %d", m.config.SilenceThresholdMs)
		}
	})
}

func TestMonitorStartStopMonitoring(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 100,
		MaxRestartAttempts:  2,
	}
	m := NewMonitor(cfg, nil)
	defer m.Stop()

	t.Run("starts monitoring and returns context", func(t *testing.T) {
		ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-123")

		if ctx == nil {
			t.Fatal("expected non-nil context")
		}
		if ctx.ProcessID != 1234 {
			t.Errorf("expected ProcessID 1234, got %d", ctx.ProcessID)
		}
		if m.ActiveContextCount() != 1 {
			t.Errorf("expected 1 active context, got %d", m.ActiveContextCount())
		}
	})

	t.Run("retrieves context by request ID", func(t *testing.T) {
		ctx := m.GetContext("req-123")
		if ctx == nil {
			t.Fatal("expected to find context")
		}
		if ctx.RequestID != "req-123" {
			t.Errorf("expected RequestID req-123, got %s", ctx.RequestID)
		}
	})

	t.Run("stops monitoring", func(t *testing.T) {
		m.StopMonitoring("req-123")

		if m.ActiveContextCount() != 0 {
			t.Errorf("expected 0 active contexts, got %d", m.ActiveContextCount())
		}

		ctx := m.GetContext("req-123")
		if ctx != nil {
			t.Error("expected nil context after stop")
		}
	})
}

func TestMonitorSilenceDetection(t *testing.T) {
	var snapshotReceived bool
	var receivedSnapshot *types.DiagnosticSnapshot
	var mu sync.Mutex

	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  50, // 50ms for fast testing
		LogBufferSize:       50,
		HeartbeatIntervalMs: 20, // Check every 20ms
		MaxRestartAttempts:  2,
	}

	onSnapshot := func(ctx *OverwatchContext, snapshot *types.DiagnosticSnapshot) {
		mu.Lock()
		snapshotReceived = true
		receivedSnapshot = snapshot
		mu.Unlock()
	}

	m := NewMonitor(cfg, onSnapshot)
	defer m.Stop()

	ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-silence")

	// Record initial output
	ctx.RecordOutput("initial output")

	// Wait for silence threshold to be exceeded
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	received := snapshotReceived
	snapshot := receivedSnapshot
	mu.Unlock()

	if !received {
		t.Error("expected snapshot to be captured after silence threshold")
	}

	if snapshot != nil {
		if snapshot.ProcessState != "blocked" {
			t.Errorf("expected ProcessState blocked, got %s", snapshot.ProcessState)
		}
		if snapshot.Provider != "claudecli" {
			t.Errorf("expected Provider claudecli, got %s", snapshot.Provider)
		}
	}
}

func TestMonitorWrapReader(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 1000,
		MaxRestartAttempts:  2,
	}
	m := NewMonitor(cfg, nil)
	defer m.Stop()

	ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-reader")

	t.Run("wraps reader and records output", func(t *testing.T) {
		input := "line1\nline2\nline3\n"
		reader := strings.NewReader(input)
		mr := m.WrapReader(reader, ctx)

		// Read all data
		buf := make([]byte, 1024)
		n, _ := mr.Read(buf)

		if n != len(input) {
			t.Errorf("expected to read %d bytes, got %d", len(input), n)
		}

		// Check that output was recorded
		lines := ctx.GetLastLogLines(10)
		if len(lines) == 0 {
			t.Error("expected output to be recorded in context")
		}
	})
}

func TestMonitorWrapWriter(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 1000,
		MaxRestartAttempts:  2,
	}
	m := NewMonitor(cfg, nil)
	defer m.Stop()

	ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-writer")

	t.Run("wraps writer and records output", func(t *testing.T) {
		var buf bytes.Buffer
		mw := m.WrapWriter(&buf, ctx)

		data := []byte("test output")
		n, err := mw.Write(data)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if n != len(data) {
			t.Errorf("expected to write %d bytes, got %d", len(data), n)
		}
		if buf.String() != "test output" {
			t.Errorf("expected buffer to contain 'test output', got %s", buf.String())
		}
	})
}

func TestMonitoredReaderReadLine(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 1000,
		MaxRestartAttempts:  2,
	}
	m := NewMonitor(cfg, nil)
	defer m.Stop()

	ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-readline")

	input := "line1\nline2\nline3"
	reader := strings.NewReader(input)
	mr := m.WrapReader(reader, ctx)

	t.Run("reads lines sequentially", func(t *testing.T) {
		line1, ok1, err1 := mr.ReadLine()
		if err1 != nil || !ok1 || line1 != "line1" {
			t.Errorf("expected line1, got %s (ok=%v, err=%v)", line1, ok1, err1)
		}

		line2, ok2, err2 := mr.ReadLine()
		if err2 != nil || !ok2 || line2 != "line2" {
			t.Errorf("expected line2, got %s (ok=%v, err=%v)", line2, ok2, err2)
		}

		line3, ok3, err3 := mr.ReadLine()
		if err3 != nil || !ok3 || line3 != "line3" {
			t.Errorf("expected line3, got %s (ok=%v, err=%v)", line3, ok3, err3)
		}

		// Should return empty after all lines read
		_, ok4, _ := mr.ReadLine()
		if ok4 {
			t.Error("expected ok=false after all lines read")
		}
	})
}

func TestStreamMonitor(t *testing.T) {
	ctx := NewOverwatchContext(1234, nil, "claudecli", "claude-sonnet-4", "req-stream")

	t.Run("monitors stdout and stderr streams", func(t *testing.T) {
		sm := NewStreamMonitor(ctx)

		stdoutReader, stdoutWriter := io.Pipe()
		stderrReader, stderrWriter := io.Pipe()

		var stdoutLines, stderrLines []string
		var mu sync.Mutex

		bgCtx := context.Background()
		sm.MonitorStreams(bgCtx, stdoutReader, stderrReader,
			func(line string) {
				mu.Lock()
				stdoutLines = append(stdoutLines, line)
				mu.Unlock()
			},
			func(line string) {
				mu.Lock()
				stderrLines = append(stderrLines, line)
				mu.Unlock()
			},
		)

		// Write to stdout
		go func() {
			_, _ = stdoutWriter.Write([]byte("stdout line 1\n"))
			_, _ = stdoutWriter.Write([]byte("stdout line 2\n"))
			_ = stdoutWriter.Close()
		}()

		// Write to stderr
		go func() {
			_, _ = stderrWriter.Write([]byte("stderr line 1\n"))
			_ = stderrWriter.Close()
		}()

		sm.Wait()

		mu.Lock()
		defer mu.Unlock()

		if len(stdoutLines) != 2 {
			t.Errorf("expected 2 stdout lines, got %d", len(stdoutLines))
		}
		if len(stderrLines) != 1 {
			t.Errorf("expected 1 stderr line, got %d", len(stderrLines))
		}
	})
}

func TestMonitorStop(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  30000,
		LogBufferSize:       50,
		HeartbeatIntervalMs: 100,
		MaxRestartAttempts:  2,
	}
	m := NewMonitor(cfg, nil)

	// Start some monitoring
	m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-1")
	m.StartMonitoring(5678, "geminicli", "gemini-pro", "req-2")

	if m.ActiveContextCount() != 2 {
		t.Errorf("expected 2 active contexts, got %d", m.ActiveContextCount())
	}

	m.Stop()

	if m.ActiveContextCount() != 0 {
		t.Errorf("expected 0 active contexts after stop, got %d", m.ActiveContextCount())
	}

	// Starting monitoring after stop should return nil
	ctx := m.StartMonitoring(9999, "test", "test", "req-3")
	if ctx != nil {
		t.Error("expected nil context after monitor stopped")
	}
}

func TestMonitorHeartbeatResetsOnOutput(t *testing.T) {
	cfg := &config.OverwatchConfig{
		SilenceThresholdMs:  100, // 100ms threshold
		LogBufferSize:       50,
		HeartbeatIntervalMs: 50, // Check every 50ms
		MaxRestartAttempts:  2,
	}

	snapshotCount := 0
	var mu sync.Mutex

	onSnapshot := func(ctx *OverwatchContext, snapshot *types.DiagnosticSnapshot) {
		mu.Lock()
		snapshotCount++
		mu.Unlock()
	}

	m := NewMonitor(cfg, onSnapshot)
	defer m.Stop()

	ctx := m.StartMonitoring(1234, "claudecli", "claude-sonnet-4", "req-heartbeat")

	// Keep producing output to prevent silence detection
	for i := 0; i < 5; i++ {
		ctx.RecordOutput("output")
		time.Sleep(30 * time.Millisecond)
	}

	mu.Lock()
	count := snapshotCount
	mu.Unlock()

	if count > 0 {
		t.Errorf("expected no snapshots while output is being produced, got %d", count)
	}
}
