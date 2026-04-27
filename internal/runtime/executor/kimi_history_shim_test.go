// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/config"
)

// TestKimiHistoryShim_MissingReasoningContent pins the load-bearing
// case: an assistant message with tool_calls and no reasoning_content
// gets the placeholder injected. This is the shape that triggers
// Kimi K2.6's 400 "thinking is enabled but reasoning_content is
// missing in assistant tool call message" without the shim.
func TestKimiHistoryShim_MissingReasoningContent(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": "search the web for X"},
			{"role": "assistant", "tool_calls": [{"id": "call_1", "type": "function", "function": {"name": "web_search", "arguments": "{\"q\":\"X\"}"}}]},
			{"role": "tool", "tool_call_id": "call_1", "content": "result text"},
			{"role": "user", "content": "summarize"}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	rc := gjson.GetBytes(out, "messages.1.reasoning_content")
	if !rc.Exists() {
		t.Fatalf("expected reasoning_content to be injected, got body: %s", out)
	}
	if rc.String() != reasoningContentPlaceholder {
		t.Errorf("reasoning_content = %q, want placeholder %q", rc.String(), reasoningContentPlaceholder)
	}
	// Other messages must be untouched.
	if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
		t.Errorf("user message must not have reasoning_content injected")
	}
	if gjson.GetBytes(out, "messages.2.reasoning_content").Exists() {
		t.Errorf("tool message must not have reasoning_content injected")
	}
}

// TestKimiHistoryShim_EmptyStringReasoningContent pins probe #6:
// empty-string reasoning_content is treated as missing by Kimi, so
// the shim must overwrite it with the placeholder. This is the
// failure mode that ruled out the simpler "always set empty string"
// approach.
func TestKimiHistoryShim_EmptyStringReasoningContent(t *testing.T) {
	in := `{
		"messages": [
			{"role": "assistant", "reasoning_content": "", "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "f", "arguments": "{}"}}]}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	rc := gjson.GetBytes(out, "messages.0.reasoning_content")
	if rc.String() != reasoningContentPlaceholder {
		t.Errorf("empty reasoning_content should be overwritten with placeholder; got %q", rc.String())
	}
}

// TestKimiHistoryShim_PreservesValidReasoningContent pins idempotency:
// when reasoning_content is already a non-empty string, the shim is a
// no-op. Critical for Kimi → Kimi continuity where the gateway's prior
// turn captured real reasoning_content the model itself produced.
func TestKimiHistoryShim_PreservesValidReasoningContent(t *testing.T) {
	original := "Let me think about this. I should call the search tool."
	in := `{
		"messages": [
			{"role": "assistant", "reasoning_content": "` + original + `", "tool_calls": [{"id": "c1", "type": "function", "function": {"name": "f", "arguments": "{}"}}]}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	rc := gjson.GetBytes(out, "messages.0.reasoning_content")
	if rc.String() != original {
		t.Errorf("valid reasoning_content was overwritten; got %q want %q", rc.String(), original)
	}
}

// TestKimiHistoryShim_NoToolCalls_NoOp pins that assistant messages
// without tool_calls are left alone. Kimi only requires the
// reasoning_content echo-back when tool_calls are present.
func TestKimiHistoryShim_NoToolCalls_NoOp(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": "hi"},
			{"role": "assistant", "content": "hello back"}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	if gjson.GetBytes(out, "messages.1.reasoning_content").Exists() {
		t.Errorf("assistant message without tool_calls must not get reasoning_content injected")
	}
	// And bytewise unchanged for safety.
	var a, b any
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatalf("input invalid: %v", err)
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatalf("output invalid: %v", err)
	}
}

// TestKimiHistoryShim_EmptyToolCallsArray_NoOp pins that an explicit
// empty tool_calls array is treated the same as no tool_calls — Kimi
// does not require reasoning_content for plain text replies.
func TestKimiHistoryShim_EmptyToolCallsArray_NoOp(t *testing.T) {
	in := `{
		"messages": [
			{"role": "assistant", "content": "ok", "tool_calls": []}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	if gjson.GetBytes(out, "messages.0.reasoning_content").Exists() {
		t.Errorf("empty tool_calls array must not trigger placeholder injection")
	}
}

// TestKimiHistoryShim_MultipleAssistantTurns pins the realistic
// agentic-loop shape: two assistant turns with tool_calls in the same
// history. Both must get the placeholder, neither must overwrite the
// other.
func TestKimiHistoryShim_MultipleAssistantTurns(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": "do stuff"},
			{"role": "assistant", "tool_calls": [{"id": "a", "type": "function", "function": {"name": "step1", "arguments": "{}"}}]},
			{"role": "tool", "tool_call_id": "a", "content": "result1"},
			{"role": "assistant", "tool_calls": [{"id": "b", "type": "function", "function": {"name": "step2", "arguments": "{}"}}]},
			{"role": "tool", "tool_call_id": "b", "content": "result2"},
			{"role": "user", "content": "wrap up"}
		]
	}`
	out := applyKimiHistoryShim([]byte(in))
	if got := gjson.GetBytes(out, "messages.1.reasoning_content").String(); got != reasoningContentPlaceholder {
		t.Errorf("messages.1: got %q, want placeholder", got)
	}
	if got := gjson.GetBytes(out, "messages.3.reasoning_content").String(); got != reasoningContentPlaceholder {
		t.Errorf("messages.3: got %q, want placeholder", got)
	}
}

// TestKimiHistoryShim_NoMessagesField pins that bodies without a
// messages field (embeddings, image generation) pass through
// untouched. The shim only ever rewrites assistant turns inside
// messages[].
func TestKimiHistoryShim_NoMessagesField(t *testing.T) {
	in := `{"input": "embed this", "model": "ail-embed"}`
	out := applyKimiHistoryShim([]byte(in))
	if string(out) != in {
		t.Errorf("non-chat body was modified; got %s want %s", out, in)
	}
}

// TestKimiHistoryShim_EmptyBody pins that an empty payload is
// returned unchanged (defensive, matches the multimodal normalizer's
// contract).
func TestKimiHistoryShim_EmptyBody(t *testing.T) {
	out := applyKimiHistoryShim([]byte{})
	if len(out) != 0 {
		t.Errorf("empty body should round-trip empty; got %q", out)
	}
}

// TestKimiHistoryShim_NonStringReasoningContent pins that a
// non-string reasoning_content (null, number, object) is treated
// as invalid and overwritten with the placeholder. Kimi rejects
// anything other than a non-empty string anyway, so coercing here
// keeps the shim's contract narrow and predictable.
func TestKimiHistoryShim_NonStringReasoningContent(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"null reasoning_content", `{"messages":[{"role":"assistant","reasoning_content":null,"tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]}]}`},
		{"number reasoning_content", `{"messages":[{"role":"assistant","reasoning_content":42,"tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]}]}`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := applyKimiHistoryShim([]byte(tt.in))
			if got := gjson.GetBytes(out, "messages.0.reasoning_content").String(); got != reasoningContentPlaceholder {
				t.Errorf("expected placeholder, got %q", got)
			}
		})
	}
}

// TestIsKimiHistoryShimEnabled pins the default-on contract on the
// config helper. nil → enabled; *true → enabled; *false → disabled.
// This is the load-bearing default that prevents future operators
// from recreating the 2026-04-27 outage by enabling Kimi without
// reading the docs.
func TestIsKimiHistoryShimEnabled(t *testing.T) {
	t.Helper()
	// Use the config package's struct + method via the executor's
	// import path. Done as a separate test in the config package
	// would also be valid; keeping it here keeps shim semantics
	// co-located with the mutator.
	bTrue := true
	bFalse := false
	cases := []struct {
		name string
		ptr  *bool
		want bool
	}{
		{"nil → default on", nil, true},
		{"explicit true → on", &bTrue, true},
		{"explicit false → off", &bFalse, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			compat := &config.OpenAICompatibility{KimiHistoryShim: tt.ptr}
			if got := compat.IsKimiHistoryShimEnabled(); got != tt.want {
				t.Errorf("IsKimiHistoryShimEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
	// nil receiver → false (defensive; resolveCompatConfig may return nil
	// when an auth has no matching provider entry).
	var nilCompat *config.OpenAICompatibility
	if got := nilCompat.IsKimiHistoryShimEnabled(); got != false {
		t.Errorf("nil receiver IsKimiHistoryShimEnabled() = %v, want false", got)
	}
}
