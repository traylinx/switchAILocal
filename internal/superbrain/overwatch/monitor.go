package overwatch

import (
	"bufio"
	"context"
	"io"
	"sync"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	"github.com/traylinx/switchAILocal/internal/superbrain/types"
)

// SnapshotHandler is called when a diagnostic snapshot is captured due to silence detection.
type SnapshotHandler func(ctx *OverwatchContext, snapshot *types.DiagnosticSnapshot)

// Monitor provides real-time monitoring of CLI process execution.
// It wraps stdout/stderr streams, detects silence, and emits diagnostic snapshots.
type Monitor struct {
	mu sync.RWMutex

	// config holds the monitoring configuration.
	config *config.OverwatchConfig

	// onSnapshot is called when a diagnostic snapshot is captured.
	onSnapshot SnapshotHandler

	// activeContexts tracks all currently monitored executions.
	activeContexts map[string]*OverwatchContext

	// stopCh signals the monitor to stop all monitoring.
	stopCh chan struct{}

	// stopped indicates whether the monitor has been stopped.
	stopped bool
}

// NewMonitor creates a new Monitor with the given configuration.
func NewMonitor(cfg *config.OverwatchConfig, onSnapshot SnapshotHandler) *Monitor {
	if cfg == nil {
		cfg = DefaultOverwatchConfig()
	}
	if onSnapshot == nil {
		onSnapshot = func(*OverwatchContext, *types.DiagnosticSnapshot) {}
	}

	return &Monitor{
		config:         cfg,
		onSnapshot:     onSnapshot,
		activeContexts: make(map[string]*OverwatchContext),
		stopCh:         make(chan struct{}),
		stopped:        false,
	}
}

// StartMonitoring begins monitoring a process execution.
// It returns an OverwatchContext that tracks the execution state.
func (m *Monitor) StartMonitoring(processID int, provider, model, requestID string) *OverwatchContext {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	ctx := NewOverwatchContext(processID, m.config, provider, model, requestID)
	m.activeContexts[requestID] = ctx

	// Start the heartbeat checker for this context
	go m.runHeartbeatChecker(ctx)

	return ctx
}

// StopMonitoring stops monitoring for a specific request.
func (m *Monitor) StopMonitoring(requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.activeContexts, requestID)
}

// Stop stops all monitoring and cleans up resources.
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return
	}

	m.stopped = true
	close(m.stopCh)
	m.activeContexts = make(map[string]*OverwatchContext)
}

// GetContext returns the monitoring context for a request ID.
func (m *Monitor) GetContext(requestID string) *OverwatchContext {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeContexts[requestID]
}

// ActiveContextCount returns the number of currently monitored executions.
func (m *Monitor) ActiveContextCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeContexts)
}

// runHeartbeatChecker periodically checks for silence and emits snapshots.
func (m *Monitor) runHeartbeatChecker(ctx *OverwatchContext) {
	heartbeatInterval := time.Duration(m.config.HeartbeatIntervalMs) * time.Millisecond
	if heartbeatInterval <= 0 {
		heartbeatInterval = time.Second
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.mu.RLock()
			_, exists := m.activeContexts[ctx.RequestID]
			m.mu.RUnlock()

			if !exists {
				return // Context was removed, stop checking
			}

			if !ctx.CheckHeartbeat() {
				// Silence threshold exceeded
				if !ctx.IsSilent {
					ctx.MarkSilent()
					snapshot := ctx.CaptureSnapshot("blocked")
					m.onSnapshot(ctx, snapshot)
				}
			}
		}
	}
}

// MonitoredReader wraps an io.Reader to track output and update the monitoring context.
type MonitoredReader struct {
	reader  io.Reader
	ctx     *OverwatchContext
	scanner *bufio.Scanner
	mu      sync.Mutex
}

// WrapReader creates a MonitoredReader that tracks output from the given reader.
func (m *Monitor) WrapReader(reader io.Reader, ctx *OverwatchContext) *MonitoredReader {
	return &MonitoredReader{
		reader:  reader,
		ctx:     ctx,
		scanner: bufio.NewScanner(reader),
	}
}

// Read implements io.Reader, recording output to the monitoring context.
func (mr *MonitoredReader) Read(p []byte) (n int, err error) {
	n, err = mr.reader.Read(p)
	if n > 0 && mr.ctx != nil {
		// Record that we received output (for heartbeat tracking)
		mr.ctx.RecordOutput(string(p[:n]))
	}
	return n, err
}

// ReadLine reads a single line from the reader and records it.
// Returns the line, whether more data is available, and any error.
func (mr *MonitoredReader) ReadLine() (string, bool, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.scanner.Scan() {
		line := mr.scanner.Text()
		if mr.ctx != nil {
			mr.ctx.RecordOutput(line)
		}
		return line, true, nil
	}

	err := mr.scanner.Err()
	return "", false, err
}

// MonitoredWriter wraps an io.Writer to track output and update the monitoring context.
type MonitoredWriter struct {
	writer io.Writer
	ctx    *OverwatchContext
}

// WrapWriter creates a MonitoredWriter that tracks output written to the given writer.
func (m *Monitor) WrapWriter(writer io.Writer, ctx *OverwatchContext) *MonitoredWriter {
	return &MonitoredWriter{
		writer: writer,
		ctx:    ctx,
	}
}

// Write implements io.Writer, recording output to the monitoring context.
func (mw *MonitoredWriter) Write(p []byte) (n int, err error) {
	n, err = mw.writer.Write(p)
	if n > 0 && mw.ctx != nil {
		// Record that we produced output (for heartbeat tracking)
		mw.ctx.RecordOutput(string(p[:n]))
	}
	return n, err
}

// StreamMonitor provides line-by-line monitoring of stdout and stderr streams.
type StreamMonitor struct {
	ctx        *OverwatchContext
	stdoutDone chan struct{}
	stderrDone chan struct{}
	cancel     context.CancelFunc
}

// NewStreamMonitor creates a StreamMonitor for the given context.
func NewStreamMonitor(ctx *OverwatchContext) *StreamMonitor {
	return &StreamMonitor{
		ctx:        ctx,
		stdoutDone: make(chan struct{}),
		stderrDone: make(chan struct{}),
	}
}

// MonitorStreams starts monitoring stdout and stderr streams concurrently.
// It reads lines from each stream and records them in the context.
// The provided callbacks are called for each line read.
func (sm *StreamMonitor) MonitorStreams(
	bgCtx context.Context,
	stdout, stderr io.Reader,
	onStdout, onStderr func(line string),
) {
	ctx, cancel := context.WithCancel(bgCtx)
	sm.cancel = cancel

	// Monitor stdout
	go func() {
		defer close(sm.stdoutDone)
		sm.monitorStream(ctx, stdout, onStdout)
	}()

	// Monitor stderr
	go func() {
		defer close(sm.stderrDone)
		sm.monitorStream(ctx, stderr, onStderr)
	}()
}

// monitorStream reads lines from a stream and records them.
func (sm *StreamMonitor) monitorStream(ctx context.Context, reader io.Reader, onLine func(line string)) {
	if reader == nil {
		return
	}

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			line := scanner.Text()
			sm.ctx.RecordOutput(line)
			if onLine != nil {
				onLine(line)
			}
		}
	}
}

// Wait blocks until both stdout and stderr monitoring complete.
func (sm *StreamMonitor) Wait() {
	<-sm.stdoutDone
	<-sm.stderrDone
}

// Stop cancels stream monitoring.
func (sm *StreamMonitor) Stop() {
	if sm.cancel != nil {
		sm.cancel()
	}
}

// Overwatch is the main interface for the Overwatch Layer.
// It provides methods for starting monitoring, checking health, and capturing snapshots.
type Overwatch interface {
	// StartMonitoring begins watching an execution.
	StartMonitoring(processID int, provider, model, requestID string) *OverwatchContext

	// StopMonitoring stops watching an execution.
	StopMonitoring(requestID string)

	// GetContext returns the monitoring context for a request.
	GetContext(requestID string) *OverwatchContext

	// WrapReader creates a monitored reader.
	WrapReader(reader io.Reader, ctx *OverwatchContext) *MonitoredReader

	// WrapWriter creates a monitored writer.
	WrapWriter(writer io.Writer, ctx *OverwatchContext) *MonitoredWriter

	// Stop stops all monitoring.
	Stop()
}

// Ensure Monitor implements Overwatch interface.
var _ Overwatch = (*Monitor)(nil)
