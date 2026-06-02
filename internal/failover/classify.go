// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package failover classifies upstream-provider errors into a small,
// well-defined taxonomy that the cross-provider retry loop dispatches on.
//
// The package is intentionally narrow: it does NOT perform retry, transport,
// or scoring work. It only answers two questions for the conductor:
//
//  1. What kind of failure is this? (Class)
//  2. Should the next provider be tried?  (advance vs abort)
//
// Classification reads existing error shapes already produced by the
// executor layer — http status via the `StatusCode() int` interface,
// `*executor.stallError` via `IsStallError`, and `context.Canceled` /
// `context.DeadlineExceeded` directly — so executors do not need to be
// modified to participate.
package failover

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
)

// StallPhaser is the cross-package interface that the executor's stall
// error implements. We detect stalls via this interface rather than
// importing internal/runtime/executor directly — that would create an
// import cycle (auth → failover → executor → auth).
type StallPhaser interface {
	error
	StallPhase() string
}

// ErrorClass is the taxonomy validated by the gemini+opencode round of
// lope-negotiate (sprint doc, Decision #1).
type ErrorClass int

const (
	// ClassUnknown is the default zero value; treated like ClassTransient by
	// the conductor (advance to next provider) but logged distinctly so we
	// can spot classifier blind spots.
	ClassUnknown ErrorClass = iota

	// ClassTransient — 5xx, conn refused, ctx deadline mid-request,
	// header timeout. Retry next provider (and optionally same once).
	ClassTransient

	// ClassRateLimit — 429. Skip provider for its backoff window, advance.
	ClassRateLimit

	// ClassAuth — 401, 403. Advance; mark credential degraded.
	ClassAuth

	// ClassOutOfCredits — 402, or provider-signaled credit exhaustion.
	// Advance; mark provider unavailable for the session.
	ClassOutOfCredits

	// ClassContextLength — 400 with provider-signaled context-length-exceeded.
	// Advance only to providers with larger window; fail fast otherwise.
	ClassContextLength

	// ClassPermanent — 400 (other), 404, 422. Return to client. Do NOT advance.
	ClassPermanent

	// ClassEmptyContent — 200 OK with empty content/choices.
	// Advance immediately (sick node or safety filter).
	ClassEmptyContent

	// ClassStallPreFirstByte — upstream produced no bytes within
	// firstByteTimeout. Stream context cancelled by watchdog. Advance —
	// nothing has been flushed to the client yet.
	ClassStallPreFirstByte

	// ClassStallMidStream — stall AFTER first chunk was flushed to client.
	// UNRECOVERABLE: we cannot retract a partial SSE response. Abort.
	ClassStallMidStream

	// ClassClientDisconnect — caller cancelled the request. NOT a provider
	// failure; do not retry, do not record failure on health monitor.
	ClassClientDisconnect
)

func (c ErrorClass) String() string {
	switch c {
	case ClassTransient:
		return "transient"
	case ClassRateLimit:
		return "rate_limit"
	case ClassAuth:
		return "auth"
	case ClassOutOfCredits:
		return "out_of_credits"
	case ClassContextLength:
		return "context_length"
	case ClassPermanent:
		return "permanent"
	case ClassEmptyContent:
		return "empty_content"
	case ClassStallPreFirstByte:
		return "stall_pre_first_byte"
	case ClassStallMidStream:
		return "stall_mid_stream"
	case ClassClientDisconnect:
		return "client_disconnect"
	default:
		return "unknown"
	}
}

// ShouldAdvance reports whether the conductor should try the next provider
// for this class. ClassPermanent, ClassClientDisconnect, and
// ClassStallMidStream are terminal — everything else advances.
func (c ErrorClass) ShouldAdvance() bool {
	switch c {
	case ClassPermanent, ClassClientDisconnect, ClassStallMidStream:
		return false
	default:
		return true
	}
}

// statusCoder lets us read HTTP status from any error type that exposes one.
// The shape is shared with sdk/switchailocal/auth.statusCodeFromError.
type statusCoder interface {
	StatusCode() int
}

// Classify maps any error returned from an executor (or downstream) into
// one of the ErrorClass values. body may be nil; when present and non-empty
// it is consulted only for provider-signaled hints inside an HTTP 400
// response (out_of_credits / context_length).
//
// The function never panics on nil err — it returns ClassUnknown.
func Classify(ctx context.Context, err error, body []byte) ErrorClass {
	if err == nil {
		return ClassUnknown
	}

	// Client disconnect: the parent context was cancelled by the caller.
	// Distinguish from our own per-request DeadlineExceeded by inspecting
	// the parent ctx — if IT is cancelled, it's the client.
	//
	// Note: errors.Is(err, context.Canceled) on its own is NOT sufficient to
	// classify as client disconnect. When the stall watchdog fires it cancels
	// a child context, producing the same error — but the parent ctx is still
	// alive. In that case we fall through to the stall classification below
	// (via the StallPhaser interface) rather than treating it as user abort.
	if ctx != nil && ctx.Err() == context.Canceled {
		return ClassClientDisconnect
	}

	// Stall errors carry their own phase information via StallPhaser.
	var stall StallPhaser
	if errors.As(err, &stall) {
		switch stall.StallPhase() {
		case "mid_stream":
			return ClassStallMidStream
		default:
			// pre_first_byte and any unknown phase → recoverable.
			return ClassStallPreFirstByte
		}
	}

	// Network-level transient errors.
	if errors.Is(err, context.DeadlineExceeded) {
		return ClassTransient
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return ClassTransient
	}
	// Transport-level context cancellation where the parent ctx is still
	// alive (e.g. stall watchdog cancelled a child stream context, an
	// HTTP transport timeout in the proxy layer). Not a client disconnect
	// — classified as transient so the failover loop can advance.
	if errors.Is(err, context.Canceled) {
		return ClassTransient
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return ClassTransient
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ClassTransient
	}
	if strings.Contains(strings.ToLower(err.Error()), "unexpected eof") {
		return ClassTransient
	}

	// HTTP status-based classification.
	var sc statusCoder
	if errors.As(err, &sc) {
		switch code := sc.StatusCode(); {
		case code == 401 || code == 403:
			return ClassAuth
		case code == 402:
			return ClassOutOfCredits
		case code == 429:
			return ClassRateLimit
		case code == 400:
			if hasContextLengthHint(err, body) {
				return ClassContextLength
			}
			if hasOutOfCreditsHint(err, body) {
				return ClassOutOfCredits
			}
			return ClassPermanent
		case code == 404 || code == 422 || code == 405 || code == 415:
			return ClassPermanent
		case code >= 500 && code < 600:
			return ClassTransient
		case code == 408:
			return ClassTransient
		}
	}

	return ClassUnknown
}

// hasContextLengthHint detects provider-signaled context-length errors in
// HTTP 400 responses. We check both the error message and the response
// body (when available). Per validator consensus, this is HINT-based,
// not the final source of truth — true taxonomy classification belongs
// in per-provider classifiers (a future iteration).
func hasContextLengthHint(err error, body []byte) bool {
	needles := []string{
		"context_length_exceeded",
		"maximum context length",
		"context length",
		"too many tokens",
		"prompt is too long",
	}
	return matchAny(err.Error(), body, needles)
}

// hasOutOfCreditsHint detects provider-signaled credit exhaustion that
// some providers (notably Anthropic) return as a 400 instead of 402.
func hasOutOfCreditsHint(err error, body []byte) bool {
	needles := []string{
		"insufficient credit",
		"insufficient_balance",
		"out of credit",
		"billing_hard_limit_reached",
		"quota exceeded",
		"insufficient_quota",
	}
	return matchAny(err.Error(), body, needles)
}

func matchAny(s string, body []byte, needles []string) bool {
	lo := strings.ToLower(s)
	bo := strings.ToLower(string(body))
	for _, n := range needles {
		if strings.Contains(lo, n) || (len(bo) > 0 && strings.Contains(bo, n)) {
			return true
		}
	}
	return false
}
