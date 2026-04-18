// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package failover

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
)

// statusErr is a minimal error type that implements StatusCode(), matching
// the shape used by *auth.Error and the proxy's HTTP errors.
type statusErr struct {
	code int
	msg  string
}

func (e *statusErr) Error() string  { return e.msg }
func (e *statusErr) StatusCode() int { return e.code }

// netTimeout implements net.Error with Timeout()=true.
type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func TestClassify_NilError(t *testing.T) {
	if got := Classify(context.Background(), nil, nil); got != ClassUnknown {
		t.Errorf("nil err → %s, want unknown", got)
	}
}

func TestClassify_HTTPStatus(t *testing.T) {
	tests := []struct {
		code int
		want ErrorClass
	}{
		{401, ClassAuth},
		{403, ClassAuth},
		{402, ClassOutOfCredits},
		{429, ClassRateLimit},
		{404, ClassPermanent},
		{422, ClassPermanent},
		{405, ClassPermanent},
		{415, ClassPermanent},
		{500, ClassTransient},
		{502, ClassTransient},
		{503, ClassTransient},
		{504, ClassTransient},
		{408, ClassTransient},
	}
	for _, tt := range tests {
		err := &statusErr{code: tt.code, msg: "x"}
		got := Classify(context.Background(), err, nil)
		if got != tt.want {
			t.Errorf("status=%d → %s, want %s", tt.code, got, tt.want)
		}
	}
}

func TestClassify_400Plain(t *testing.T) {
	err := &statusErr{code: 400, msg: "bad request"}
	if got := Classify(context.Background(), err, nil); got != ClassPermanent {
		t.Errorf("400 plain → %s, want permanent", got)
	}
}

func TestClassify_400ContextLengthHint(t *testing.T) {
	err := &statusErr{code: 400, msg: "context_length_exceeded: prompt is 200000 tokens"}
	if got := Classify(context.Background(), err, nil); got != ClassContextLength {
		t.Errorf("400 ctxlen → %s, want context_length", got)
	}
}

func TestClassify_400ContextLengthHint_Body(t *testing.T) {
	err := &statusErr{code: 400, msg: "Bad Request"}
	body := []byte(`{"error":{"message":"This model's maximum context length is 8192 tokens"}}`)
	if got := Classify(context.Background(), err, body); got != ClassContextLength {
		t.Errorf("400 ctxlen body → %s, want context_length", got)
	}
}

func TestClassify_400OutOfCredits_Anthropic(t *testing.T) {
	err := &statusErr{code: 400, msg: "Your credit balance is too low — billing_hard_limit_reached"}
	if got := Classify(context.Background(), err, nil); got != ClassOutOfCredits {
		t.Errorf("anthropic credits → %s, want out_of_credits", got)
	}
}

func TestClassify_NetworkErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{"deadline", context.DeadlineExceeded, ClassTransient},
		{"conn refused", syscall.ECONNREFUSED, ClassTransient},
		{"conn reset", syscall.ECONNRESET, ClassTransient},
		{"epipe", syscall.EPIPE, ClassTransient},
		{"net.Error timeout", netTimeout{}, ClassTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(context.Background(), tt.err, nil); got != tt.want {
				t.Errorf("%s → %s, want %s", tt.name, got, tt.want)
			}
		})
	}
}

// stallShim implements StallPhaser for tests.
type stallShim struct{ phase string }

func (s *stallShim) Error() string      { return "stall " + s.phase }
func (s *stallShim) StallPhase() string { return s.phase }

func TestClassify_StallPreFirstByte(t *testing.T) {
	if got := Classify(context.Background(), &stallShim{phase: "pre_first_byte"}, nil); got != ClassStallPreFirstByte {
		t.Errorf("pre_first_byte → %s, want stall_pre_first_byte", got)
	}
}

func TestClassify_StallMidStream(t *testing.T) {
	if got := Classify(context.Background(), &stallShim{phase: "mid_stream"}, nil); got != ClassStallMidStream {
		t.Errorf("mid_stream → %s, want stall_mid_stream", got)
	}
}

func TestClassify_StallUnknownPhase_DefaultsToRecoverable(t *testing.T) {
	if got := Classify(context.Background(), &stallShim{phase: "weird"}, nil); got != ClassStallPreFirstByte {
		t.Errorf("unknown phase → %s, want stall_pre_first_byte (advance)", got)
	}
}

func TestClassify_ClientDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := Classify(ctx, context.Canceled, nil); got != ClassClientDisconnect {
		t.Errorf("client disconnect → %s, want client_disconnect", got)
	}
}

func TestClassify_Unknown(t *testing.T) {
	if got := Classify(context.Background(), errors.New("mystery"), nil); got != ClassUnknown {
		t.Errorf("plain error → %s, want unknown", got)
	}
}

func TestClassify_WrappedNetError(t *testing.T) {
	var ne net.Error = netTimeout{}
	wrapped := errors.Join(errors.New("upstream call failed"), ne)
	if got := Classify(context.Background(), wrapped, nil); got != ClassTransient {
		t.Errorf("wrapped net.Error → %s, want transient", got)
	}
}

func TestErrorClass_ShouldAdvance(t *testing.T) {
	tests := []struct {
		c    ErrorClass
		want bool
	}{
		{ClassTransient, true},
		{ClassRateLimit, true},
		{ClassAuth, true},
		{ClassOutOfCredits, true},
		{ClassEmptyContent, true},
		{ClassStallPreFirstByte, true},
		{ClassUnknown, true},
		{ClassContextLength, true}, // advance — caller filters by window
		{ClassPermanent, false},
		{ClassClientDisconnect, false},
		{ClassStallMidStream, false},
	}
	for _, tt := range tests {
		if got := tt.c.ShouldAdvance(); got != tt.want {
			t.Errorf("%s.ShouldAdvance()=%v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestErrorClass_String(t *testing.T) {
	if ClassTransient.String() != "transient" {
		t.Error("transient string")
	}
	if ErrorClass(99).String() != "unknown" {
		t.Error("unknown string")
	}
}

// Ensure the timeout error path doesn't slip through unclassified after
// some refactor. This is a regression guard.
func TestClassify_RegressionNetTimeout(t *testing.T) {
	w := &wrappedNetErr{inner: netTimeout{}}
	if got := Classify(context.Background(), w, nil); got != ClassTransient {
		t.Errorf("wrapped wrappedNetErr → %s, want transient", got)
	}
}

type wrappedNetErr struct{ inner net.Error }

func (e *wrappedNetErr) Error() string  { return e.inner.Error() }
func (e *wrappedNetErr) Unwrap() error  { return e.inner }
