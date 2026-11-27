// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/traylinx/switchAILocal/internal/config"
	coreusage "github.com/traylinx/switchAILocal/sdk/switchailocal/usage"
)

func testPolicies(t *testing.T, cap int64, concurrency int) config.CallerPoliciesConfig {
	t.Helper()
	return config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "caller-usage.json"),
		Callers: []config.CallerPolicyConfig{
			{ID: "caller-a", APIKey: "key-a", AllowedModels: []string{"model-a"}, MaxConcurrency: concurrency, DailyTokenCap: cap},
			{ID: "caller-b", APIKey: "key-b", AllowedModels: []string{"model-b"}, MaxConcurrency: 2, DailyTokenCap: cap},
		},
	}
}

func requireDecisionCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var decision *DecisionError
	if !errors.As(err, &decision) || decision.Code != code {
		t.Fatalf("expected decision code %q, got %v", code, err)
	}
}

func newTestManager(t *testing.T, cfg config.CallerPoliciesConfig, configPath string) *Manager {
	t.Helper()
	m := New(cfg, configPath)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func TestManagerRejectsWrongModelWithoutAffectingAnotherCaller(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	denied, err := m.Begin("caller-a", "model-b", 1)
	if err != nil {
		t.Fatalf("lease setup failed before dispatch: %v", err)
	}
	if err := denied.AuthorizeModel("model-b"); err == nil {
		t.Fatal("wrong model unexpectedly allowed")
	} else {
		requireDecisionCode(t, err, CodeModelDenied)
	}
	denied.Finish(http.StatusForbidden)

	lease, err := m.Begin("caller-b", "model-b", 1)
	if err != nil || lease == nil {
		t.Fatalf("independent caller was affected: lease=%v err=%v", lease, err)
	}
	lease.Finish(200)
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerDailyTokenCapIsCallerLocal(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 10, 1), "")
	lease, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 10}})
	lease.Finish(200)

	if _, err := m.Begin("caller-a", "model-a", 1); err == nil {
		t.Fatal("caller over daily token cap unexpectedly allowed")
	} else {
		requireDecisionCode(t, err, CodeDailyCapExceeded)
	}
	other, err := m.Begin("caller-b", "model-b", 1)
	if err != nil {
		t.Fatalf("other caller was affected by cap: %v", err)
	}
	other.Finish(200)
}

func TestManagerRefusesEstimatedInputThatWouldExceedRemainingCap(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 10, 1), "")
	first, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	first.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 9}})
	first.Finish(http.StatusOK)

	_, err = m.Begin("caller-a", "model-a", 0, 2)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
	_, err = m.Begin("caller-a", "model-a", 2)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
	exact, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatalf("estimate exactly at remaining allowance was refused: %v", err)
	}
	exact.Finish(http.StatusInternalServerError)
}

func TestManagerConcurrencyLimitFailsImmediatelyAndReleases(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	first, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Begin("caller-a", "model-a", 1); err == nil {
		t.Fatal("concurrent request unexpectedly allowed")
	} else {
		requireDecisionCode(t, err, CodeConcurrencyExceeded)
	}
	first.Finish(500)
	third, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatalf("released slot stayed blocked: %v", err)
	}
	third.Finish(500)
}

func TestManagerDailyWindowResetsAtUTCMidnight(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 5, 1), "")
	local := time.FixedZone("x", 2*60*60)
	current := time.Date(2026, 11, 27, 1, 59, 59, 0, local)
	m.now = func() time.Time { return current }
	lease, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 5}})
	lease.Finish(200)
	if _, err := m.Begin("caller-a", "model-a", 1); err == nil {
		t.Fatal("cap did not apply before UTC midnight")
	}

	current = current.Add(time.Second)
	afterReset, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatalf("cap did not reset at UTC midnight: %v", err)
	}
	afterReset.Finish(500)
}

func TestManagerPersistsOnlyCallerIDsAndCounters(t *testing.T) {
	cfg := testPolicies(t, 100, 1)
	if err := os.WriteFile(cfg.StateFile, []byte(`{"day":"2000-01-01","callers":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, cfg, "")
	lease, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(200)
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsAny(string(data), "key-a", "model-a") {
		t.Fatalf("state file contains secret or request metadata: %s", data)
	}
	info, err := os.Stat(cfg.StateFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestWriteStateFileRepairsInsecureTemporaryMode(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "caller-usage.json")
	insecureCreateTemp := func(dir, pattern string) (*os.File, error) {
		file, err := os.CreateTemp(dir, pattern)
		if err != nil {
			return nil, err
		}
		if err := file.Chmod(0o644); err != nil {
			_ = file.Close()
			return nil, err
		}
		return file, nil
	}
	if err := writeStateFileWithTemp(stateFile, []byte(`{"day":"2026-11-27","callers":{}}`), insecureCreateTemp); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file mode = %o, want 600", info.Mode().Perm())
	}
}

func TestManagerFailsClosedOnCorruptState(t *testing.T) {
	cfg := testPolicies(t, 100, 1)
	if err := os.WriteFile(cfg.StateFile, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, cfg, "")
	_, err := m.Begin("caller-a", "model-a", 1)
	requireDecisionCode(t, err, CodeStateUnavailable)
}

func TestManagerUnknownCallerFailsClosed(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	_, err := m.Begin("stale-caller", "model-a", 1)
	requireDecisionCode(t, err, CodeStateUnavailable)
}

func TestManagerFailsClosedWhileCallerMappingReloads(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	m.SetReloading(true)
	_, err := m.Begin("caller-a", "model-a", 1)
	requireDecisionCode(t, err, CodeStateUnavailable)
	requireDecisionCode(t, m.AuthorizeModel("caller-a", "model-a"), CodeStateUnavailable)
	m.SetReloading(false)
	lease, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatalf("reload gate stayed closed: %v", err)
	}
	lease.Finish(http.StatusInternalServerError)
}

func TestManagerRetriesTransientStateWriteOnNextBegin(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	original := m.writeState
	attempts := 0
	m.writeState = func(path string, data []byte) error {
		attempts++
		if attempts == 1 {
			return errors.New("transient write failure")
		}
		return original(path, data)
	}
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 1}})
	lease.Finish(500)
	if err := m.Flush(); err == nil {
		t.Fatal("first state write unexpectedly succeeded")
	}
	next, err := m.Begin("caller-a", "model-a", 0)
	if err != nil || next == nil || attempts < 2 {
		t.Fatalf("transient state error was not retried: lease=%v err=%v attempts=%d", next, err, attempts)
	}
	next.Finish(500)
}

func TestManagerPersistenceDoesNotHoldDecisionLock(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	m.flushDelay = 0
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	m.writeState = func(string, []byte) error {
		once.Do(func() { close(entered) })
		<-release
		return nil
	}
	first, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	persisted := make(chan struct{})
	go func() {
		first.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 1}})
		close(persisted)
	}()
	<-entered
	decision := make(chan error, 1)
	go func() {
		lease, beginErr := m.Begin("caller-b", "model-b", 0)
		if lease != nil {
			lease.Finish(500)
		}
		decision <- beginErr
	}()
	select {
	case err := <-decision:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("another caller's decision blocked on state-file I/O")
	}
	close(release)
	<-persisted
	first.Finish(500)
}

func TestManagerSkipsStateWriteWhenCountersDoNotChange(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	writes := 0
	m.writeState = func(string, []byte) error { writes++; return nil }
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(500)
	m.mu.Lock()
	dirty := m.dirty
	m.mu.Unlock()
	if dirty {
		t.Fatal("unchanged counters marked quota state dirty")
	}
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("unchanged counters caused %d state writes", writes)
	}
}

func TestManagerRestoresSameDayUsageAcrossRestart(t *testing.T) {
	cfg := testPolicies(t, 5, 1)
	first := newTestManager(t, cfg, "")
	lease, err := first.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 5}})
	lease.Finish(500)
	if err := first.Flush(); err != nil {
		t.Fatal(err)
	}
	restarted := newTestManager(t, cfg, "")
	_, err = restarted.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
}

func TestManagerLateUsageReplacesRequestResponseFallback(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	lease, err := m.Begin("caller-a", "model-a", 4)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot := lease.Finish(200, 16); snapshot.TokensUsed != 8 {
		t.Fatalf("fallback tokens = %d, want 8", snapshot.TokensUsed)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 12}})
	if snapshot := m.CurrentSnapshot("caller-a", "model-a"); snapshot.TokensUsed != 12 {
		t.Fatalf("late reported tokens = %d, want 12", snapshot.TokensUsed)
	}
}

func TestManagerFallbackIncludesResponseBytes(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 4, 1), "")
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(200, 16)
	_, err = m.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
}

func TestManagerUsageAccountingDoesNotWaitForDebouncedPersistence(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	m.flushDelay = 0
	entered := make(chan struct{})
	release := make(chan struct{})
	m.writeState = func(string, []byte) error {
		close(entered)
		<-release
		return nil
	}
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	go func() {
		lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 1}})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("usage observer blocked on quota-state persistence")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("background flusher did not run")
	}
	close(release)
}

func TestManagerDebouncesUsageWritesAndFlushesOnClose(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	m.flushDelay = 20 * time.Millisecond
	writes := make(chan struct{}, 4)
	m.writeState = func(string, []byte) error {
		writes <- struct{}{}
		return nil
	}
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 1}})
	}
	select {
	case <-writes:
	case <-time.After(time.Second):
		t.Fatal("debounced state write did not run")
	}
	select {
	case <-writes:
		t.Fatal("one usage burst caused multiple state writes")
	case <-time.After(60 * time.Millisecond):
	}

	second, err := m.Begin("caller-b", "model-b", 0)
	if err != nil {
		t.Fatal(err)
	}
	second.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 1}})
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writes:
	case <-time.After(time.Second):
		t.Fatal("Close did not flush pending state")
	}
}

func TestManagerRolloverFlushesResetImmediately(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 100, 1), "")
	m.flushDelay = time.Hour
	current := time.Date(2026, 11, 27, 23, 59, 59, 0, time.UTC)
	m.now = func() time.Time { return current }
	writes := make(chan diskState, 2)
	m.writeState = func(_ string, data []byte) error {
		var state diskState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		writes <- state
		return nil
	}
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 5}})
	lease.Finish(500)
	if err := m.Flush(); err != nil {
		t.Fatal(err)
	}
	<-writes

	current = current.Add(time.Second)
	resetLease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	resetLease.Finish(500)
	select {
	case state := <-writes:
		if state.Day != "2026-11-28" || state.Callers["caller-a"].Used != 0 {
			t.Fatalf("rollover state = %#v", state)
		}
	case <-time.After(time.Second):
		t.Fatal("rollover reset was left for the normal debounce interval")
	}
}

func TestManagerChargesFailedProviderRecords(t *testing.T) {
	m := newTestManager(t, testPolicies(t, 5, 1), "")
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Failed: true, Detail: coreusage.Detail{TotalTokens: 5}})
	lease.Finish(500)
	_, err = m.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
}

func TestManagerReloadPreservesActiveAndRemovedCallerUsage(t *testing.T) {
	cfg := testPolicies(t, 5, 1)
	m := newTestManager(t, cfg, "")
	active, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	changedFile := cfg
	changedFile.StateFile = filepath.Join(t.TempDir(), "moved-state.json")
	m.Apply(changedFile)
	_, err = m.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeConcurrencyExceeded)
	active.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 5}})
	active.Finish(500)

	removed := changedFile
	removed.Callers = removed.Callers[1:]
	m.Apply(removed)
	m.Apply(changedFile)
	_, err = m.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
}

func TestManagerStateFileChangeAfterMidnightDoesNotMergePriorDayUsage(t *testing.T) {
	cfg := testPolicies(t, 5, 1)
	m := newTestManager(t, cfg, "")
	m.mu.Lock()
	m.day = "2026-11-27"
	m.states["caller-a"].Used = 5
	m.mu.Unlock()
	m.now = func() time.Time { return time.Date(2026, 11, 28, 0, 0, 1, 0, time.UTC) }

	changed := cfg
	changed.StateFile = filepath.Join(t.TempDir(), "new-day-state.json")
	if err := os.WriteFile(changed.StateFile, []byte(`{"day":"2026-11-28","callers":{"caller-a":{"used":0}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m.Apply(changed)
	if got := m.CurrentSnapshot("caller-a", "model-a").TokensUsed; got != 0 {
		t.Fatalf("prior-day usage merged into new day: %d", got)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if len(candidate) > 0 && len(value) >= len(candidate) {
			for i := 0; i+len(candidate) <= len(value); i++ {
				if value[i:i+len(candidate)] == candidate {
					return true
				}
			}
		}
	}
	return false
}
