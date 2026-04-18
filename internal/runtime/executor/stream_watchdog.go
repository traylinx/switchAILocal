// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// streamStallWatchdog cancels a stream's context when no upstream activity
// occurs within a deadline. It begins in the "first-byte" phase using
// firstByteTimeout (covers connect + headers + initial body bytes) and
// transitions to the "stall" phase using stallTimeout once the first chunk
// is observed. Calling onChunk resets the active timer.
//
// The watchdog is designed to be safe under concurrent stop/onChunk/timer
// firing. Stop is idempotent.
//
// Use case: SSE streaming proxies where bufio.Scanner.Scan() blocks on Read
// and would otherwise hang forever if the upstream connection silently dies.
// The watchdog cancels the request context, which closes the underlying
// connection and causes Scan() to return an error.
type streamStallWatchdog struct {
	firstByteTimeout time.Duration
	stallTimeout     time.Duration
	cancel           context.CancelFunc

	mu             sync.Mutex
	timer          *time.Timer
	stopped        bool
	seenFirstChunk bool

	// fired is set true (atomically) when the timer fires and triggers cancel.
	// Distinguishes a watchdog-initiated cancellation from an external one
	// (e.g. client disconnect).
	fired atomic.Bool
}

// newStreamStallWatchdog constructs a watchdog. It does NOT arm itself; call
// start() once the request is in flight. cancel must close the request's
// context (typically the streamCtx from context.WithCancel(parent)).
//
// firstByteTimeout and stallTimeout must both be > 0; non-positive values
// disable that phase (see disabled() — caller should skip newStreamStallWatchdog
// entirely if both are 0).
func newStreamStallWatchdog(cancel context.CancelFunc, firstByteTimeout, stallTimeout time.Duration) *streamStallWatchdog {
	return &streamStallWatchdog{
		firstByteTimeout: firstByteTimeout,
		stallTimeout:     stallTimeout,
		cancel:           cancel,
	}
}

// start arms the watchdog using firstByteTimeout. Safe to call once.
func (w *streamStallWatchdog) start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.timer != nil {
		return
	}
	d := w.firstByteTimeout
	if d <= 0 {
		// First-byte detection disabled; only mid-stream stalls count. Use
		// stallTimeout as the initial deadline so a never-arriving first
		// chunk still triggers cancel.
		d = w.stallTimeout
	}
	if d <= 0 {
		return
	}
	w.timer = time.AfterFunc(d, w.onTimeout)
}

// onChunk is called for every byte-bearing chunk read from upstream. It
// transitions to the stall phase on the first call and resets the deadline
// to stallTimeout. Subsequent calls just reset the deadline.
//
// Safe to call after stop (no-op).
func (w *streamStallWatchdog) onChunk() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped || w.timer == nil {
		return
	}
	w.seenFirstChunk = true
	d := w.stallTimeout
	if d <= 0 {
		// Stall detection disabled. Stop the timer; we only cared about
		// first-byte.
		w.timer.Stop()
		w.timer = nil
		return
	}
	// time.Timer.Reset is only safe if the timer is stopped or expired.
	// Stop returns false if it has already fired or been stopped — in
	// either case our cancel has run (or is racing) and Reset is harmless.
	w.timer.Stop()
	w.timer.Reset(d)
}

// stop disarms the watchdog. Idempotent; safe to call from defer.
func (w *streamStallWatchdog) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	w.stopped = true
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// onTimeout is called by time.AfterFunc when the active deadline elapses.
// It marks fired and invokes cancel.
func (w *streamStallWatchdog) onTimeout() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	w.fired.Store(true)
	w.cancel()
}

// firedDueToStall reports whether the watchdog timer fired (and therefore
// the context cancellation should be classified as a stall, not a client
// disconnect).
func (w *streamStallWatchdog) firedDueToStall() bool {
	return w.fired.Load()
}

// preFirstChunk reports whether the watchdog fired BEFORE any chunk was
// observed (i.e. classified as stall_pre_first_byte rather than
// stall_mid_stream). Caller should consult this when deciding whether
// transparent failover is possible — only pre-first-chunk stalls are
// recoverable without breaking the SSE contract.
func (w *streamStallWatchdog) preFirstChunk() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.seenFirstChunk
}

// stallError is the error type returned through StreamChunk.Err when the
// watchdog fires. The Phase field allows downstream classification (P1
// retry layer) to distinguish recoverable pre-first-byte stalls from
// unrecoverable mid-stream stalls.
type stallError struct {
	Provider string
	Phase    stallPhase
	Timeout  time.Duration
}

type stallPhase int

const (
	stallPhasePreFirstByte stallPhase = iota
	stallPhaseMidStream
)

func (s stallPhase) String() string {
	switch s {
	case stallPhasePreFirstByte:
		return "pre_first_byte"
	case stallPhaseMidStream:
		return "mid_stream"
	default:
		return "unknown"
	}
}

func (e *stallError) Error() string {
	return fmt.Sprintf("upstream stall (%s) on provider %q after %s", e.Phase, e.Provider, e.Timeout)
}

// StallPhase returns the phase string ("pre_first_byte" or "mid_stream"),
// implementing the failover.StallPhaser contract so the conductor can
// classify stalls without importing this package directly.
func (e *stallError) StallPhase() string {
	if e == nil {
		return ""
	}
	return e.Phase.String()
}

// IsStallError reports whether err is a *stallError. Useful for the P1
// retry classifier.
func IsStallError(err error) bool {
	var s *stallError
	return errors.As(err, &s)
}
