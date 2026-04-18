// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/traylinx/switchAILocal/internal/failover"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// httpStatusErr is a minimal status-bearing error matching the executor
// error shape used by the real provider executors.
type httpStatusErr struct {
	code int
	msg  string
}

func (e *httpStatusErr) Error() string  { return e.msg }
func (e *httpStatusErr) StatusCode() int { return e.code }

// TestExecuteProvidersOnce_AdvancesOnTransient verifies the conductor
// classifies a 503 as transient and advances to the next provider.
func TestExecuteProvidersOnce_AdvancesOnTransient(t *testing.T) {
	m := &Manager{}
	calls := []string{}
	fn := func(_ context.Context, p string) (switchailocalexecutor.Response, error) {
		calls = append(calls, p)
		if p == "openai" {
			return switchailocalexecutor.Response{}, &httpStatusErr{code: 503, msg: "service unavailable"}
		}
		return switchailocalexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
	}
	resp, err := m.executeProvidersOnce(context.Background(), []string{"openai", "gemini"}, fn)
	if err != nil {
		t.Fatalf("expected success after failover, got err: %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Errorf("expected gemini payload, got %q", resp.Payload)
	}
	if len(calls) != 2 || calls[0] != "openai" || calls[1] != "gemini" {
		t.Errorf("expected [openai, gemini], got %v", calls)
	}
}

// TestExecuteProvidersOnce_AbortsOnPermanent verifies that a 400 (permanent)
// short-circuits the chain — no further provider is tried.
func TestExecuteProvidersOnce_AbortsOnPermanent(t *testing.T) {
	m := &Manager{}
	calls := []string{}
	fn := func(_ context.Context, p string) (switchailocalexecutor.Response, error) {
		calls = append(calls, p)
		return switchailocalexecutor.Response{}, &httpStatusErr{code: 400, msg: "bad request: invalid model"}
	}
	_, err := m.executeProvidersOnce(context.Background(), []string{"openai", "gemini"}, fn)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	fe, ok := failover.AsFailoverError(err)
	if !ok {
		t.Fatalf("expected *FailoverError, got %T: %v", err, err)
	}
	if fe.Class != failover.ClassPermanent {
		t.Errorf("class=%s, want permanent", fe.Class)
	}
	if fe.Provider != "openai" {
		t.Errorf("provider=%q, want openai", fe.Provider)
	}
	if len(calls) != 1 {
		t.Errorf("expected exactly 1 call (no advance after permanent), got %d (%v)", len(calls), calls)
	}
}

// TestExecuteProvidersOnce_AbortsOnClientDisconnect verifies that a
// caller-cancelled context terminates the chain rather than treating the
// cancellation as a provider failure.
func TestExecuteProvidersOnce_AbortsOnClientDisconnect(t *testing.T) {
	m := &Manager{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate client disconnect before the call
	fn := func(c context.Context, _ string) (switchailocalexecutor.Response, error) {
		return switchailocalexecutor.Response{}, c.Err()
	}
	_, err := m.executeProvidersOnce(ctx, []string{"openai", "gemini"}, fn)
	if err == nil {
		t.Fatal("expected error from cancelled ctx")
	}
	fe, ok := failover.AsFailoverError(err)
	if !ok {
		t.Fatalf("want *FailoverError, got %T: %v", err, err)
	}
	if fe.Class != failover.ClassClientDisconnect {
		t.Errorf("class=%s, want client_disconnect", fe.Class)
	}
}

// TestExecuteProvidersOnce_ExhaustsAllProvidersAndReturnsLast verifies the
// conductor walks the entire chain when every provider returns a
// non-terminal class, and returns the LAST FailoverError so the caller can
// inspect what finally failed.
func TestExecuteProvidersOnce_ExhaustsAllProvidersAndReturnsLast(t *testing.T) {
	m := &Manager{}
	calls := []string{}
	fn := func(_ context.Context, p string) (switchailocalexecutor.Response, error) {
		calls = append(calls, p)
		switch p {
		case "openai":
			return switchailocalexecutor.Response{}, &httpStatusErr{code: 502, msg: "bad gateway"}
		case "gemini":
			return switchailocalexecutor.Response{}, &httpStatusErr{code: 429, msg: "rate limited"}
		case "claude":
			return switchailocalexecutor.Response{}, &httpStatusErr{code: 503, msg: "down"}
		}
		return switchailocalexecutor.Response{}, errors.New("unreachable")
	}
	_, err := m.executeProvidersOnce(context.Background(), []string{"openai", "gemini", "claude"}, fn)
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if len(calls) != 3 {
		t.Errorf("expected 3 attempts, got %d", len(calls))
	}
	fe, ok := failover.AsFailoverError(err)
	if !ok {
		t.Fatalf("want *FailoverError, got %T: %v", err, err)
	}
	if fe.Provider != "claude" {
		t.Errorf("last provider=%q, want claude", fe.Provider)
	}
	if fe.HTTPCode != 503 {
		t.Errorf("last http_code=%d, want 503", fe.HTTPCode)
	}
}

// TestExecuteProvidersOnce_EmptyProviders verifies the no-providers path
// returns the existing provider_not_found Error (not a FailoverError).
func TestExecuteProvidersOnce_EmptyProviders(t *testing.T) {
	m := &Manager{}
	fn := func(_ context.Context, _ string) (switchailocalexecutor.Response, error) {
		t.Fatal("fn should not be invoked")
		return switchailocalexecutor.Response{}, nil
	}
	_, err := m.executeProvidersOnce(context.Background(), nil, fn)
	if err == nil {
		t.Fatal("expected provider_not_found error")
	}
	authErr, ok := err.(*Error)
	if !ok || authErr.Code != "provider_not_found" {
		t.Errorf("want *Error{provider_not_found}, got %T %v", err, err)
	}
}
