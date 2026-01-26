// Package audit provides structured logging for Superbrain autonomous actions.
// All autonomous actions (stdin injection, restarts, fallbacks, etc.) are logged
// to a dedicated audit log for security review and transparency.
package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// AuditLogEntry records a single autonomous action for security review.
// Each entry is written as a JSON line to the audit log file.
type AuditLogEntry struct {
	// Timestamp is when the action was initiated.
	Timestamp time.Time `json:"timestamp"`

	// RequestID uniquely identifies the request that triggered this action.
	RequestID string `json:"request_id"`

	// ActionType categorizes the autonomous action (e.g., "stdin_injection", "restart_with_flags").
	ActionType string `json:"action_type"`

	// Provider is the provider being executed (e.g., "claudecli", "geminicli").
	Provider string `json:"provider"`

	// Model is the model being used.
	Model string `json:"model"`

	// ActionDetails contains action-specific metadata (e.g., flags applied, pattern matched).
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`

	// Outcome describes the result of the action ("success", "failed", "skipped").
	Outcome string `json:"outcome"`

	// UserIdentifier optionally identifies the user who initiated the request.
	UserIdentifier string `json:"user_identifier,omitempty"`
}

// Logger provides structured audit logging for Superbrain actions.
// It writes JSON-formatted log entries to a rotating log file.
type Logger struct {
	mu       sync.Mutex
	encoder  *json.Encoder
	file     *lumberjack.Logger
	enabled  bool
	logPath  string
	fallback *log.Logger // Fallback to standard logging if file write fails
}

// Config holds configuration for the audit logger.
type Config struct {
	// Enabled toggles audit logging.
	Enabled bool

	// LogPath is the file path for the audit log.
	LogPath string

	// MaxSizeMB is the maximum size in megabytes before rotation.
	// Default: 100 MB.
	MaxSizeMB int

	// MaxBackups is the maximum number of old log files to retain.
	// Default: 10.
	MaxBackups int

	// MaxAgeDays is the maximum number of days to retain old log files.
	// Default: 30 days.
	MaxAgeDays int

	// Compress determines whether rotated log files should be compressed.
	// Default: true.
	Compress bool
}

// NewLogger creates a new audit logger with the specified configuration.
// If audit logging is disabled, the logger will be a no-op.
func NewLogger(cfg Config) (*Logger, error) {
	if !cfg.Enabled {
		return &Logger{
			enabled:  false,
			fallback: log.New(),
		}, nil
	}

	// Set defaults
	if cfg.MaxSizeMB == 0 {
		cfg.MaxSizeMB = 100
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 10
	}
	if cfg.MaxAgeDays == 0 {
		cfg.MaxAgeDays = 30
	}

	// Ensure the log directory exists
	logDir := filepath.Dir(cfg.LogPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	// Create rotating file logger
	fileLogger := &lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
	}

	logger := &Logger{
		encoder:  json.NewEncoder(fileLogger),
		file:     fileLogger,
		enabled:  true,
		logPath:  cfg.LogPath,
		fallback: log.New(),
	}

	return logger, nil
}

// LogAction writes an audit log entry for an autonomous action.
// This method is thread-safe and can be called concurrently.
func (l *Logger) LogAction(entry AuditLogEntry) {
	if !l.enabled {
		return
	}

	// Set timestamp if not already set
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Write JSON entry to file
	if err := l.encoder.Encode(entry); err != nil {
		// If file write fails, fall back to standard logging
		l.fallback.WithFields(log.Fields{
			"error":       err.Error(),
			"request_id":  entry.RequestID,
			"action_type": entry.ActionType,
			"provider":    entry.Provider,
			"model":       entry.Model,
			"outcome":     entry.Outcome,
		}).Error("Failed to write audit log entry")
	}
}

// LogStdinInjection logs a stdin injection action.
func (l *Logger) LogStdinInjection(requestID, provider, model, pattern, response, outcome string) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "stdin_injection",
		Provider:   provider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"pattern":  pattern,
			"response": response,
		},
		Outcome: outcome,
	})
}

// LogRestart logs a process restart action.
func (l *Logger) LogRestart(requestID, provider, model string, flags []string, outcome string) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "restart_with_flags",
		Provider:   provider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"flags": flags,
		},
		Outcome: outcome,
	})
}

// LogFallback logs a fallback routing action.
func (l *Logger) LogFallback(requestID, originalProvider, fallbackProvider, model, reason, outcome string) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "fallback_routing",
		Provider:   originalProvider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"fallback_provider": fallbackProvider,
			"reason":            reason,
		},
		Outcome: outcome,
	})
}

// LogContextOptimization logs a context sculpting action.
func (l *Logger) LogContextOptimization(requestID, provider, model string, originalTokens, optimizedTokens int, outcome string) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "context_optimization",
		Provider:   provider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"original_tokens":  originalTokens,
			"optimized_tokens": optimizedTokens,
			"tokens_saved":     originalTokens - optimizedTokens,
		},
		Outcome: outcome,
	})
}

// LogDiagnosis logs a failure diagnosis action.
func (l *Logger) LogDiagnosis(requestID, provider, model, failureType, remediation string, confidence float64) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "diagnosis",
		Provider:   provider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"failure_type": failureType,
			"remediation":  remediation,
			"confidence":   confidence,
		},
		Outcome: "completed",
	})
}

// LogSilenceDetection logs when a silence threshold is exceeded.
func (l *Logger) LogSilenceDetection(requestID, provider, model string, silenceDurationMs int64) {
	l.LogAction(AuditLogEntry{
		RequestID:  requestID,
		ActionType: "silence_detection",
		Provider:   provider,
		Model:      model,
		ActionDetails: map[string]interface{}{
			"silence_duration_ms": silenceDurationMs,
		},
		Outcome: "detected",
	})
}

// Close closes the audit log file and flushes any buffered data.
func (l *Logger) Close() error {
	if !l.enabled || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Close()
}

// Rotate triggers a log file rotation.
// This is useful for testing or manual rotation triggers.
func (l *Logger) Rotate() error {
	if !l.enabled || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.file.Rotate()
}

// Global audit logger instance.
var globalLogger *Logger
var once sync.Once

// Global returns the global audit logger instance.
// It must be initialized with InitGlobal before use.
func Global() *Logger {
	once.Do(func() {
		// Create a disabled logger as default
		globalLogger, _ = NewLogger(Config{Enabled: false})
	})
	return globalLogger
}

// InitGlobal initializes the global audit logger with the specified configuration.
// This should be called once during application startup.
func InitGlobal(cfg Config) error {
	logger, err := NewLogger(cfg)
	if err != nil {
		return err
	}

	once.Do(func() {})
	globalLogger = logger
	return nil
}
