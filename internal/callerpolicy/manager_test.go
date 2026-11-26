// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"context"
	"errors"
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

func TestManagerRejectsWrongModelWithoutAffectingAnotherCaller(t *testing.T) {
	m := New(testPolicies(t, 100, 1), "")
	if _, err := m.Begin("caller-a", "model-b", 1); err == nil {
		t.Fatal("wrong model unexpectedly allowed")
	} else {
		requireDecisionCode(t, err, CodeModelDenied)
	}

	lease, err := m.Begin("caller-b", "model-b", 1)
	if err != nil || lease == nil {
		t.Fatalf("independent caller was affected: lease=%v err=%v", lease, err)
	}
	lease.Finish(200)
}

func TestManagerDailyTokenCapIsCallerLocal(t *testing.T) {
	m := New(testPolicies(t, 10, 1), "")
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

func TestManagerConcurrencyLimitFailsImmediatelyAndReleases(t *testing.T) {
	m := New(testPolicies(t, 100, 1), "")
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
	m := New(testPolicies(t, 5, 1), "")
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
	m := New(cfg, "")
	lease, err := m.Begin("caller-a", "model-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(200)

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

func TestManagerFailsClosedOnCorruptState(t *testing.T) {
	cfg := testPolicies(t, 100, 1)
	if err := os.WriteFile(cfg.StateFile, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := New(cfg, "")
	_, err := m.Begin("caller-a", "model-a", 1)
	requireDecisionCode(t, err, CodeStateUnavailable)
}

func TestManagerUnknownCallerFailsClosed(t *testing.T) {
	m := New(testPolicies(t, 100, 1), "")
	_, err := m.Begin("stale-caller", "model-a", 1)
	requireDecisionCode(t, err, CodeStateUnavailable)
}

func TestManagerRetriesTransientStateWriteOnNextBegin(t *testing.T) {
	m := New(testPolicies(t, 100, 1), "")
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
	next, err := m.Begin("caller-a", "model-a", 0)
	if err != nil || next == nil || attempts < 2 {
		t.Fatalf("transient state error was not retried: lease=%v err=%v attempts=%d", next, err, attempts)
	}
	next.Finish(500)
}

func TestManagerPersistenceDoesNotHoldDecisionLock(t *testing.T) {
	m := New(testPolicies(t, 100, 1), "")
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
	m := New(testPolicies(t, 100, 1), "")
	writes := 0
	m.writeState = func(string, []byte) error { writes++; return nil }
	lease, err := m.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.Finish(500)
	if writes != 0 {
		t.Fatalf("unchanged counters caused %d state writes", writes)
	}
}

func TestManagerRestoresSameDayUsageAcrossRestart(t *testing.T) {
	cfg := testPolicies(t, 5, 1)
	first := New(cfg, "")
	lease, err := first.Begin("caller-a", "model-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	lease.ObserveUsage(context.Background(), coreusage.Record{Detail: coreusage.Detail{TotalTokens: 5}})
	lease.Finish(500)
	restarted := New(cfg, "")
	_, err = restarted.Begin("caller-a", "model-a", 0)
	requireDecisionCode(t, err, CodeDailyCapExceeded)
}

func TestManagerChargesFailedProviderRecords(t *testing.T) {
	m := New(testPolicies(t, 5, 1), "")
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
	m := New(cfg, "")
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
