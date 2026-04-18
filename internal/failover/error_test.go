// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package failover

import (
	"errors"
	"strings"
	"testing"
)

func TestFailoverError_Error(t *testing.T) {
	fe := &FailoverError{
		Class:    ClassRateLimit,
		Provider: "openai",
		HTTPCode: 429,
		Wrapped:  errors.New("too many requests"),
	}
	got := fe.Error()
	if !strings.Contains(got, "openai") || !strings.Contains(got, "rate_limit") || !strings.Contains(got, "429") || !strings.Contains(got, "too many requests") {
		t.Errorf("Error() missing fields: %q", got)
	}
}

func TestFailoverError_Error_NilWrapped(t *testing.T) {
	fe := &FailoverError{Class: ClassPermanent, Provider: "x", HTTPCode: 400}
	if got := fe.Error(); !strings.Contains(got, "permanent") || !strings.Contains(got, "400") {
		t.Errorf("Error() missing fields: %q", got)
	}
}

func TestFailoverError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	fe := &FailoverError{Wrapped: inner}
	if !errors.Is(fe, inner) {
		t.Error("errors.Is should find inner via Unwrap")
	}
}

func TestFailoverError_StatusCode(t *testing.T) {
	fe := &FailoverError{HTTPCode: 503}
	type sc interface{ StatusCode() int }
	var got sc
	if !errors.As(fe, &got) {
		t.Fatal("errors.As StatusCode interface should match")
	}
	if got.StatusCode() != 503 {
		t.Errorf("StatusCode=%d, want 503", got.StatusCode())
	}
}

func TestAsFailoverError(t *testing.T) {
	if _, ok := AsFailoverError(nil); ok {
		t.Error("nil should not match")
	}
	plain := errors.New("plain")
	if _, ok := AsFailoverError(plain); ok {
		t.Error("plain should not match")
	}
	fe := &FailoverError{Class: ClassAuth, Provider: "p"}
	got, ok := AsFailoverError(fe)
	if !ok || got != fe {
		t.Error("direct match should return same pointer")
	}
	wrapped := &wrappedNetErr{inner: nil}
	_ = wrapped
	wErr := errorChain(fe)
	got2, ok := AsFailoverError(wErr)
	if !ok || got2 != fe {
		t.Error("should unwrap chain to find FailoverError")
	}
}

// errorChain wraps err in another error so AsFailoverError must unwrap.
type chainErr struct{ inner error }

func (e *chainErr) Error() string { return "chain: " + e.inner.Error() }
func (e *chainErr) Unwrap() error { return e.inner }

func errorChain(e error) error { return &chainErr{inner: e} }

// Nil receivers must not panic (defensive).
func TestFailoverError_NilSafety(t *testing.T) {
	var fe *FailoverError
	_ = fe.Error()
	_ = fe.Unwrap()
	_ = fe.StatusCode()
}
