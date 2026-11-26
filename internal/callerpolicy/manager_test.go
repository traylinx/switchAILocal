// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	current := time.Date(2026, 11, 26, 23, 59, 59, 0, time.UTC)
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
