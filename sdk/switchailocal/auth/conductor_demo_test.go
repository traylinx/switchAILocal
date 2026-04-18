// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/traylinx/switchAILocal/internal/failover"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// stallDemoErr satisfies failover.StallPhaser so Classify treats it as a
// pre-first-byte stall (transient, advance-eligible) — mirroring the shape
// an executor's streamWatchdog produces when a provider freezes before any
// bytes flow.
type stallDemoErr struct {
	phase string
	msg   string
}

func (e *stallDemoErr) Error() string      { return e.msg }
func (e *stallDemoErr) StallPhase() string { return e.phase }

// TestDemo_KillProviderMidRequest_TransparentFailover is the sprint demo
// captured as a single automated test. It asserts the end-state described
// in docs/sprints/failover-recovery.md §"Demo":
//
//   - Client sends ONE request.
//   - First provider stalls before any byte (watchdog trips → transient).
//   - Second provider returns 503 (transient).
//   - Third provider returns 200. Caller gets that payload, transparently.
//   - Server log shows exactly 2 × event=failover + 1 × event=failover_recovered.
//   - No event=failover_abort.
//
// Run with -v to see the structured log lines as they would appear in prod.
func TestDemo_KillProviderMidRequest_TransparentFailover(t *testing.T) {
	// Capture every logrus record produced during the run.
	hook := logtest.NewGlobal()
	defer hook.Reset()
	logrus.SetLevel(logrus.DebugLevel)

	m := &Manager{}
	calls := []string{}
	fn := func(_ context.Context, p string) (switchailocalexecutor.Response, error) {
		calls = append(calls, p)
		switch p {
		case "ollama":
			// Simulated: `docker kill ollama` mid-handshake — watchdog trips.
			return switchailocalexecutor.Response{}, &stallDemoErr{
				phase: "pre_first_byte",
				msg:   "stream stalled before first byte (watchdog)",
			}
		case "openai":
			return switchailocalexecutor.Response{}, &httpStatusErr{
				code: 503,
				msg:  "service unavailable",
			}
		case "gemini":
			return switchailocalexecutor.Response{
				Payload: []byte(`{"choices":[{"message":{"content":"1 2 3 4 5"}}]}`),
			}, nil
		}
		t.Fatalf("unexpected provider called: %s", p)
		return switchailocalexecutor.Response{}, nil
	}

	ctx := context.WithValue(context.Background(), requestIDContextKey{}, "demo-req-001")
	resp, err := m.executeProvidersOnce(ctx, []string{"ollama", "openai", "gemini"}, fn)

	// ── Behavioral assertions ─────────────────────────────────────────────
	if err != nil {
		t.Fatalf("expected transparent recovery, got err: %v", err)
	}
	if !strings.Contains(string(resp.Payload), "1 2 3 4 5") {
		t.Errorf("expected gemini payload in response, got %q", resp.Payload)
	}
	if want := []string{"ollama", "openai", "gemini"}; !equalSlice(calls, want) {
		t.Errorf("provider chain walked as %v, want %v", calls, want)
	}

	// ── Observability assertions (P4) ─────────────────────────────────────
	var failovers, recovered, aborts int
	var classes []string
	for _, e := range hook.AllEntries() {
		if ev, ok := e.Data["event"]; ok {
			switch ev {
			case "failover":
				failovers++
				if cls, ok := e.Data["error_class"]; ok {
					classes = append(classes, cls.(string))
				}
			case "failover_recovered":
				recovered++
			case "failover_abort":
				aborts++
			}
		}
	}

	if failovers != 2 {
		t.Errorf("event=failover count = %d, want 2", failovers)
	}
	if recovered != 1 {
		t.Errorf("event=failover_recovered count = %d, want 1", recovered)
	}
	if aborts != 0 {
		t.Errorf("event=failover_abort count = %d, want 0 (no terminal class in chain)", aborts)
	}

	// The first advance came from a stall; the second from a 503.
	if len(classes) != 2 {
		t.Fatalf("expected 2 recorded error_class values, got %d: %v", len(classes), classes)
	}
	if classes[0] != failover.ClassStallPreFirstByte.String() {
		t.Errorf("first advance class = %q, want %q", classes[0], failover.ClassStallPreFirstByte.String())
	}
	if classes[1] != failover.ClassTransient.String() {
		t.Errorf("second advance class = %q, want %q", classes[1], failover.ClassTransient.String())
	}

	// Pretty-print the demo log so `go test -v` shows what prod logs look like.
	t.Logf("── Demo log output ──")
	for _, e := range hook.AllEntries() {
		if _, ok := e.Data["event"]; !ok {
			continue
		}
		t.Logf("level=%s msg=%q fields=%v", e.Level, e.Message, e.Data)
	}
}

// TestDemo_PermanentErrorShortCircuits proves the inverse: a permanent
// classification (e.g., 400 invalid model) aborts immediately without
// consuming downstream providers — a failover system that retries forever
// on bad input is worse than one that stops.
func TestDemo_PermanentErrorShortCircuits(t *testing.T) {
	hook := logtest.NewGlobal()
	defer hook.Reset()

	m := &Manager{}
	calls := []string{}
	fn := func(_ context.Context, p string) (switchailocalexecutor.Response, error) {
		calls = append(calls, p)
		return switchailocalexecutor.Response{}, &httpStatusErr{
			code: 400,
			msg:  "invalid request: unknown model",
		}
	}

	_, err := m.executeProvidersOnce(context.Background(), []string{"openai", "gemini", "claude"}, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(calls) != 1 {
		t.Errorf("permanent class should short-circuit, got %d calls: %v", len(calls), calls)
	}

	fe, ok := failover.AsFailoverError(err)
	if !ok || fe.Class != failover.ClassPermanent {
		t.Errorf("expected FailoverError{class=permanent}, got %T %v", err, err)
	}

	var aborts int
	for _, e := range hook.AllEntries() {
		if e.Data["event"] == "failover_abort" {
			aborts++
		}
	}
	if aborts != 1 {
		t.Errorf("event=failover_abort count = %d, want 1", aborts)
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
