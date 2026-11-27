// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package callerpolicy enforces per-caller model, concurrency, and daily-token
// limits without exposing inbound credentials to logs or durable quota state.
package callerpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/traylinx/switchAILocal/internal/config"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

// ErrorCode identifies a stable caller-policy refusal reason.
type ErrorCode string

const (
	CodeModelDenied         ErrorCode = "caller_model_not_allowed"
	CodeModelRequired       ErrorCode = "caller_model_required"
	CodeModelConflict       ErrorCode = "caller_model_conflict"
	CodeRouteDenied         ErrorCode = "caller_route_not_allowed"
	CodeConcurrencyExceeded ErrorCode = "caller_concurrency_exceeded"
	CodeDailyCapExceeded    ErrorCode = "caller_daily_token_cap_exceeded"
	CodeStateUnavailable    ErrorCode = "caller_quota_state_unavailable"
)

// DecisionError is safe to return to an API caller. It never contains an API
// key, prompt, or persisted quota-state content.
type DecisionError struct {
	Code ErrorCode
}

func (e *DecisionError) Error() string { return string(e.Code) }

// StatusCode lets dispatch layers preserve caller-policy HTTP semantics.
func (e *DecisionError) StatusCode() int {
	if e == nil {
		return http.StatusServiceUnavailable
	}
	switch e.Code {
	case CodeModelDenied, CodeModelConflict, CodeRouteDenied:
		return http.StatusForbidden
	case CodeModelRequired:
		return http.StatusBadRequest
	case CodeConcurrencyExceeded, CodeDailyCapExceeded:
		return http.StatusTooManyRequests
	default:
		return http.StatusServiceUnavailable
	}
}

type policy struct {
	allowed        map[string]struct{}
	maxConcurrency int
	dailyTokenCap  int64
}

type callerState struct {
	Used   int64 `json:"used"`
	Active int   `json:"-"`
}

type diskState struct {
	Day     string                `json:"day"`
	Callers map[string]diskCaller `json:"callers"`
}

type diskCaller struct {
	Used int64 `json:"used"`
}

// Snapshot contains non-secret counters suitable for structured logging.
type Snapshot struct {
	CallerID      string
	Model         string
	Active        int
	TokensUsed    int64
	DailyCap      int64
	UsageReported bool
}

// Manager owns live caller counters and durable daily usage state.
type Manager struct {
	mu        sync.Mutex
	persistMu sync.Mutex

	policies          map[string]policy
	states            map[string]*callerState
	day               string
	stateFile         string
	configDir         string
	stateErr          error
	stateErrRetryable bool
	now               func() time.Time
	writeState        func(string, []byte) error
	flushDelay        time.Duration
	flushTimer        *time.Timer
	dirty             bool
	closed            bool
	reloading         bool
}

// New constructs a policy manager. Relative state-file paths resolve from the
// configuration file directory, not the process working directory.
func New(cfg config.CallerPoliciesConfig, configFilePath string) *Manager {
	configDir := filepath.Dir(configFilePath)
	if configFilePath == "" || configDir == "." {
		configDir = ""
	}
	m := &Manager{
		policies:   make(map[string]policy),
		states:     make(map[string]*callerState),
		configDir:  configDir,
		now:        time.Now,
		writeState: writeStateFile,
		flushDelay: time.Second,
	}
	m.Apply(cfg)
	return m
}

// Enabled reports whether any named caller policy is configured.
func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.policies) > 0
}

// SetReloading temporarily fails named-caller decisions closed while caller
// identities and policies are replaced as one logical configuration update.
func (m *Manager) SetReloading(reloading bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.reloading = reloading
	m.mu.Unlock()
}

// Apply atomically replaces policy definitions while preserving usage for
// caller IDs that remain configured. A changed state-file is loaded before the
// new policies become active.
func (m *Manager) Apply(cfg config.CallerPoliciesConfig) {
	if m == nil {
		return
	}
	policies := make(map[string]policy, len(cfg.Callers))
	for _, caller := range cfg.Callers {
		allowed := make(map[string]struct{}, len(caller.AllowedModels))
		for _, model := range caller.AllowedModels {
			allowed[model] = struct{}{}
		}
		policies[caller.ID] = policy{
			allowed:        allowed,
			maxConcurrency: caller.MaxConcurrency,
			dailyTokenCap:  caller.DailyTokenCap,
		}
	}

	stateFile := cfg.StateFile
	if stateFile != "" && !filepath.IsAbs(stateFile) && m.configDir != "" {
		stateFile = filepath.Join(m.configDir, stateFile)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	fileChanged := stateFile != m.stateFile
	previousStates := m.states
	previousDay := m.day
	m.policies = policies
	m.stateFile = stateFile
	if fileChanged {
		m.stateErr = nil
		m.stateErrRetryable = false
		m.states = make(map[string]*callerState)
		m.day = ""
		if len(policies) > 0 {
			m.stateErr = m.loadLocked()
		}
		for id, previous := range previousStates {
			if current := m.states[id]; current != nil {
				current.Active = previous.Active
				if previousDay == m.day && previous.Used > current.Used {
					current.Used = previous.Used
				}
			} else {
				used := int64(0)
				if previousDay == m.day {
					used = previous.Used
				}
				m.states[id] = &callerState{Used: used, Active: previous.Active}
			}
		}
	}
	for id := range policies {
		if m.states[id] == nil {
			m.states[id] = &callerState{}
		}
	}
}

// Begin checks model, quota, and concurrency guards and starts one request.
// Concurrency overflow fails immediately; it is never queued.
func (m *Manager) Begin(callerID, model string, estimatedInputTokens int64, reservedOutputTokens ...int64) (*Lease, error) {
	if m == nil || callerID == "" {
		return nil, nil
	}
	m.mu.Lock()
	if m.reloading {
		m.mu.Unlock()
		return nil, &DecisionError{Code: CodeStateUnavailable}
	}
	if m.stateErr != nil {
		if !m.stateErrRetryable {
			m.mu.Unlock()
			return nil, &DecisionError{Code: CodeStateUnavailable}
		}
		m.mu.Unlock()
		if err := m.persist(); err != nil {
			return nil, &DecisionError{Code: CodeStateUnavailable}
		}
		m.mu.Lock()
		if m.reloading {
			m.mu.Unlock()
			return nil, &DecisionError{Code: CodeStateUnavailable}
		}
	}
	defer m.mu.Unlock()
	p, ok := m.policies[callerID]
	if !ok {
		return nil, &DecisionError{Code: CodeStateUnavailable}
	}
	if err := m.rolloverLocked(m.now().UTC()); err != nil {
		m.stateErr = err
		m.stateErrRetryable = true
		return nil, &DecisionError{Code: CodeStateUnavailable}
	}
	state := m.states[callerID]
	if state == nil {
		state = &callerState{}
		m.states[callerID] = state
	}
	if estimatedInputTokens < 0 {
		estimatedInputTokens = 0
	}
	reservedOutput := int64(0)
	if len(reservedOutputTokens) > 0 && reservedOutputTokens[0] > 0 {
		reservedOutput = reservedOutputTokens[0]
	}
	remaining := p.dailyTokenCap - state.Used
	if state.Used >= p.dailyTokenCap || estimatedInputTokens > remaining || reservedOutput > remaining-estimatedInputTokens {
		return nil, &DecisionError{Code: CodeDailyCapExceeded}
	}
	if state.Active >= p.maxConcurrency {
		return nil, &DecisionError{Code: CodeConcurrencyExceeded}
	}
	state.Active++
	return &Lease{
		manager:         m,
		callerID:        callerID,
		model:           model,
		estimatedTokens: estimatedInputTokens,
	}, nil
}

// Lease is a request-scoped concurrency slot and synchronous usage observer.
type Lease struct {
	manager         *Manager
	callerID        string
	model           string
	estimatedTokens int64

	mu              sync.Mutex
	finished        bool
	usageReported   bool
	reportedTokens  int64
	fallbackCharged int64
}

// ObserveUsage charges successful provider-reported usage synchronously.
func (l *Lease) ObserveUsage(_ context.Context, record coreusage.Record) {
	if l == nil || l.manager == nil {
		return
	}
	tokens := record.Detail.TotalTokens
	if tokens <= 0 {
		tokens = record.Detail.InputTokens + record.Detail.OutputTokens + record.Detail.ReasoningTokens
	}
	if tokens <= 0 {
		return
	}
	l.mu.Lock()
	charge := tokens
	if l.finished && l.fallbackCharged > 0 {
		// Finish may have conservatively charged request/response bytes before a
		// streaming executor published its final usage. Replace that estimate
		// rather than double charging it when the late record arrives.
		if charge > l.fallbackCharged {
			charge -= l.fallbackCharged
		} else {
			charge = 0
		}
		l.fallbackCharged = 0
	}
	l.usageReported = true
	l.reportedTokens += tokens
	l.mu.Unlock()
	l.manager.addTokens(l.callerID, charge)
}

// Finish releases concurrency and charges a conservative request/response-size
// estimate when a successful handler emitted no provider usage record. The
// optional response byte count comes from the HTTP response writer.
func (l *Lease) Finish(status int, responseBytes ...int64) Snapshot {
	if l == nil || l.manager == nil {
		return Snapshot{}
	}
	l.mu.Lock()
	if l.finished {
		l.mu.Unlock()
		return l.manager.snapshot(l.callerID, l.model, l.usageReported)
	}
	l.finished = true
	usageReported := l.usageReported
	fallback := int64(0)
	if status >= 200 && status < 400 && l.reportedTokens == 0 {
		fallback = l.estimatedTokens
		if len(responseBytes) > 0 && responseBytes[0] > 0 {
			fallback += estimateBytes(int(responseBytes[0]))
		}
		l.fallbackCharged = fallback
	}
	l.mu.Unlock()

	return l.manager.finish(l.callerID, l.model, fallback, usageReported)
}

func (m *Manager) addTokens(callerID string, tokens int64) {
	if tokens <= 0 {
		return
	}
	m.mu.Lock()
	state := m.states[callerID]
	if state == nil {
		m.mu.Unlock()
		return
	}
	state.Used += tokens
	m.markDirtyLocked(false)
	m.mu.Unlock()
}

func (m *Manager) finish(callerID, model string, fallback int64, usageReported bool) Snapshot {
	m.mu.Lock()
	state := m.states[callerID]
	if state == nil {
		m.mu.Unlock()
		return Snapshot{CallerID: callerID, Model: model, UsageReported: usageReported}
	}
	if state.Active > 0 {
		state.Active--
	}
	if fallback > 0 {
		state.Used += fallback
		m.markDirtyLocked(false)
	}
	p := m.policies[callerID]
	result := Snapshot{
		CallerID:      callerID,
		Model:         model,
		Active:        state.Active,
		TokensUsed:    state.Used,
		DailyCap:      p.dailyTokenCap,
		UsageReported: usageReported,
	}
	m.mu.Unlock()
	return result
}

func (m *Manager) snapshot(callerID, model string, usageReported bool) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.states[callerID]
	p := m.policies[callerID]
	result := Snapshot{CallerID: callerID, Model: model, DailyCap: p.dailyTokenCap, UsageReported: usageReported}
	if state != nil {
		result.Active = state.Active
		result.TokensUsed = state.Used
	}
	return result
}

// CurrentSnapshot returns non-secret live counters for a caller.
func (m *Manager) CurrentSnapshot(callerID, model string) Snapshot {
	if m == nil {
		return Snapshot{CallerID: callerID, Model: model}
	}
	return m.snapshot(callerID, model, false)
}

func (m *Manager) rolloverLocked(now time.Time) error {
	day := now.Format("2006-01-02")
	if m.day == day {
		return nil
	}
	m.day = day
	for _, state := range m.states {
		state.Used = 0
	}
	m.markDirtyLocked(true)
	return nil
}

func (m *Manager) loadLocked() error {
	if m.stateFile == "" {
		return errors.New("caller policy state file is empty")
	}
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			m.day = m.now().UTC().Format("2006-01-02")
			return nil
		}
		return fmt.Errorf("read caller quota state: %w", err)
	}
	var persisted diskState
	if err := json.Unmarshal(data, &persisted); err != nil {
		return fmt.Errorf("decode caller quota state: %w", err)
	}
	today := m.now().UTC().Format("2006-01-02")
	m.day = persisted.Day
	if m.day != today {
		m.day = today
		return nil
	}
	for id, saved := range persisted.Callers {
		if saved.Used < 0 {
			return errors.New("caller quota state contains a negative counter")
		}
		m.states[id] = &callerState{Used: saved.Used}
	}
	return nil
}

// persist snapshots counters under the manager lock, then performs filesystem
// I/O outside it. A separate mutex preserves write ordering without letting a
// slow disk stall another caller's Begin decision.
func (m *Manager) persist() error {
	m.persistMu.Lock()
	defer m.persistMu.Unlock()
	m.mu.Lock()
	m.dirty = false
	if len(m.policies) == 0 {
		m.stateErr = nil
		m.stateErrRetryable = false
		m.mu.Unlock()
		return nil
	}
	stateFile := m.stateFile
	persisted := diskState{Day: m.day, Callers: make(map[string]diskCaller, len(m.states))}
	for id, state := range m.states {
		persisted.Callers[id] = diskCaller{Used: state.Used}
	}
	data, err := json.Marshal(persisted)
	m.mu.Unlock()
	if err == nil {
		if stateFile == "" {
			err = errors.New("caller policy state file is empty")
		} else {
			err = m.writeState(stateFile, data)
		}
	}
	m.mu.Lock()
	m.stateErr = err
	m.stateErrRetryable = err != nil
	m.mu.Unlock()
	return err
}

func (m *Manager) markDirtyLocked(immediate bool) {
	if m.closed {
		return
	}
	m.dirty = true
	delay := m.flushDelay
	if immediate {
		delay = 0
	}
	if m.flushTimer == nil {
		m.flushTimer = time.AfterFunc(delay, m.flushDirty)
		return
	}
	if immediate {
		m.flushTimer.Reset(0)
	}
}

func (m *Manager) flushDirty() {
	m.mu.Lock()
	m.flushTimer = nil
	if !m.dirty || m.closed {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	_ = m.persist()
}

// Flush durably writes pending counters. Normal usage accounting is debounced
// so provider executor goroutines never wait for filesystem I/O.
func (m *Manager) Flush() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.flushTimer != nil {
		m.flushTimer.Stop()
		m.flushTimer = nil
	}
	dirty := m.dirty
	m.mu.Unlock()
	if !dirty {
		return nil
	}
	return m.persist()
}

// Close flushes pending counters and prevents future debounce timers. Late
// usage remains chargeable in memory until the manager is discarded.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	if err := m.Flush(); err != nil {
		return err
	}
	m.mu.Lock()
	m.closed = true
	if m.flushTimer != nil {
		m.flushTimer.Stop()
		m.flushTimer = nil
	}
	m.mu.Unlock()
	return nil
}

// AuthorizeModel rechecks the concrete model selected after hooks and routing.
func (m *Manager) AuthorizeModel(callerID, model string) error {
	if callerID == "" {
		return nil
	}
	if m == nil {
		return &DecisionError{Code: CodeStateUnavailable}
	}
	if strings.TrimSpace(model) == "" {
		return &DecisionError{Code: CodeModelRequired}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reloading {
		return &DecisionError{Code: CodeStateUnavailable}
	}
	p, ok := m.policies[callerID]
	if !ok {
		return &DecisionError{Code: CodeStateUnavailable}
	}
	if _, allowed := p.allowed[model]; !allowed {
		return &DecisionError{Code: CodeModelDenied}
	}
	return nil
}

// AuthorizeModel implements usage.ModelAuthorizer for executor-dispatch checks.
func (l *Lease) AuthorizeModel(model string) error {
	if l == nil || l.manager == nil {
		return &DecisionError{Code: CodeStateUnavailable}
	}
	err := l.manager.AuthorizeModel(l.callerID, model)
	if err != nil {
		log.WithFields(log.Fields{
			"caller_id": l.callerID,
			"model":     "<denied>",
			"status":    err.Error(),
		}).Warn("caller policy refused resolved model at dispatch")
	}
	return err
}

func writeStateFile(stateFile string, data []byte) error {
	return writeStateFileWithTemp(stateFile, data, os.CreateTemp)
}

func writeStateFileWithTemp(stateFile string, data []byte, createTemp func(string, string) (*os.File, error)) error {
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o700); err != nil {
		return fmt.Errorf("create caller quota state directory: %w", err)
	}
	tmp, err := createTemp(filepath.Dir(stateFile), ".caller-usage-*")
	if err != nil {
		return fmt.Errorf("create caller quota state temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("protect caller quota state temporary file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write caller quota state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync caller quota state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close caller quota state: %w", err)
	}
	if err := os.Rename(tmpName, stateFile); err != nil {
		return fmt.Errorf("replace caller quota state: %w", err)
	}
	return nil
}
