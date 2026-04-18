// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStreamStallWatchdog_FiresOnFirstByteTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 20*time.Millisecond, 200*time.Millisecond)
	w.start()
	defer w.stop()

	select {
	case <-ctx.Done():
		// Expected: timer fired.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected ctx to be cancelled by first-byte timeout")
	}

	if !w.firedDueToStall() {
		t.Error("expected firedDueToStall=true after first-byte timeout")
	}
	if !w.preFirstChunk() {
		t.Error("expected preFirstChunk=true (no chunk was observed)")
	}
}

func TestStreamStallWatchdog_FiresOnStallTimeoutAfterFirstChunk(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 200*time.Millisecond, 30*time.Millisecond)
	w.start()
	defer w.stop()

	// Observe a chunk; deadline switches to stallTimeout (30ms).
	w.onChunk()

	select {
	case <-ctx.Done():
		// Expected: stall timer fired.
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected ctx to be cancelled by stall timeout after first chunk")
	}

	if !w.firedDueToStall() {
		t.Error("expected firedDueToStall=true")
	}
	if w.preFirstChunk() {
		t.Error("expected preFirstChunk=false (a chunk was observed)")
	}
}

func TestStreamStallWatchdog_OnChunkResetsDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 30*time.Millisecond, 30*time.Millisecond)
	w.start()
	defer w.stop()

	// Pulse a chunk every 10ms for 80ms; ctx must NOT be cancelled because
	// each onChunk resets the 30ms stall deadline.
	deadline := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			t.Fatal("watchdog fired despite continuous onChunk pulses")
		case <-time.After(10 * time.Millisecond):
			w.onChunk()
		}
	}
	w.stop()
}

func TestStreamStallWatchdog_StopPreventsLateFire(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 20*time.Millisecond, 20*time.Millisecond)
	w.start()
	w.stop()

	select {
	case <-ctx.Done():
		t.Fatal("watchdog fired after stop()")
	case <-time.After(60 * time.Millisecond):
		// Expected: ctx remains live.
	}

	if w.firedDueToStall() {
		t.Error("firedDueToStall should be false after stop")
	}
}

func TestStreamStallWatchdog_StopIsIdempotent(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 100*time.Millisecond, 100*time.Millisecond)
	w.start()
	w.stop()
	w.stop() // must not panic
	w.onChunk()
	w.stop()
}

func TestStreamStallWatchdog_DisabledWhenBothTimeoutsZero(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 0, 0)
	w.start()
	defer w.stop()

	select {
	case <-ctx.Done():
		t.Fatal("ctx cancelled despite both timeouts being zero")
	case <-time.After(50 * time.Millisecond):
		// Expected: nothing armed.
	}
}

func TestStreamStallWatchdog_FirstByteOnly(t *testing.T) {
	// firstByteTimeout > 0, stallTimeout = 0: cancellation only protects the
	// pre-first-chunk window. Once a chunk arrives, the timer is disarmed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newStreamStallWatchdog(cancel, 30*time.Millisecond, 0)
	w.start()
	defer w.stop()

	w.onChunk() // disarm

	select {
	case <-ctx.Done():
		t.Fatal("ctx cancelled despite stallTimeout=0 after onChunk")
	case <-time.After(80 * time.Millisecond):
		// Expected: no fire.
	}
}

func TestIsStallError(t *testing.T) {
	if IsStallError(nil) {
		t.Error("nil should not be a stall error")
	}
	if IsStallError(errors.New("plain")) {
		t.Error("plain error should not be a stall error")
	}
	se := &stallError{Provider: "openai", Phase: stallPhasePreFirstByte, Timeout: 15 * time.Second}
	if !IsStallError(se) {
		t.Error("stallError should be detected")
	}
	wrapped := &wrapErr{inner: se}
	if !IsStallError(wrapped) {
		t.Error("wrapped stallError should be detected via errors.As")
	}
}

func TestStallError_Message(t *testing.T) {
	se := &stallError{Provider: "anthropic", Phase: stallPhaseMidStream, Timeout: 60 * time.Second}
	got := se.Error()
	want := `upstream stall (mid_stream) on provider "anthropic" after 1m0s`
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// wrapErr is a minimal error wrapper used to verify errors.As behavior.
type wrapErr struct{ inner error }

func (w *wrapErr) Error() string { return w.inner.Error() }
func (w *wrapErr) Unwrap() error { return w.inner }
