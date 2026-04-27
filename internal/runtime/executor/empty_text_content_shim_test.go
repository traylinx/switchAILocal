// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"encoding/json"
	"testing"

	"github.com/tidwall/gjson"
)

// TestStripEmptyTextContent_DropsEmptyTextElement pins the load-bearing
// case from the 2026-04-27 outage: a content array containing an empty
// text element gets that element removed, leaving the rest intact.
func TestStripEmptyTextContent_DropsEmptyTextElement(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "hello"},
				{"type": "text", "text": ""},
				{"type": "text", "text": "world"}
			]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	c := gjson.GetBytes(out, "messages.0.content")
	if !c.IsArray() {
		t.Fatalf("content is no longer an array: %s", out)
	}
	arr := c.Array()
	if len(arr) != 2 {
		t.Fatalf("expected 2 elements, got %d: %s", len(arr), out)
	}
	if got := arr[0].Get("text").String(); got != "hello" {
		t.Errorf("first kept = %q, want %q", got, "hello")
	}
	if got := arr[1].Get("text").String(); got != "world" {
		t.Errorf("second kept = %q, want %q", got, "world")
	}
}

// TestStripEmptyTextContent_AllEmptyReplacedWithPlaceholder pins that a
// content array consisting solely of empty-text elements is replaced
// with a single non-empty placeholder element rather than left empty.
// Every upstream provider rejects an empty content array.
func TestStripEmptyTextContent_AllEmptyReplacedWithPlaceholder(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": ""},
				{"type": "text", "text": ""}
			]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	c := gjson.GetBytes(out, "messages.0.content")
	if !c.IsArray() {
		t.Fatalf("content is no longer an array: %s", out)
	}
	arr := c.Array()
	if len(arr) != 1 {
		t.Fatalf("expected 1 placeholder element, got %d: %s", len(arr), out)
	}
	if got := arr[0].Get("type").String(); got != "text" {
		t.Errorf("placeholder type = %q, want text", got)
	}
	if got := arr[0].Get("text").String(); got != emptyTextPlaceholder {
		t.Errorf("placeholder text = %q, want %q", got, emptyTextPlaceholder)
	}
}

// TestStripEmptyTextContent_PreservesNonEmptyContent pins idempotency
// on the happy path: arrays without empty-text elements pass through
// bytewise unchanged.
func TestStripEmptyTextContent_PreservesNonEmptyContent(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [
				{"type": "text", "text": "hello"},
				{"type": "image_url", "image_url": {"url": "data:image/png;base64,abc"}}
			]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	if string(out) != in {
		t.Errorf("payload was mutated despite no empty-text elements\nin:  %s\nout: %s", in, out)
	}
}

// TestStripEmptyTextContent_PreservesStringContent pins that messages
// with string-typed content (not an array) are never rewritten — the
// shim only ever touches array-shaped content.
func TestStripEmptyTextContent_PreservesStringContent(t *testing.T) {
	in := `{"messages":[{"role":"user","content":""}]}`
	out := stripEmptyTextContent([]byte(in))
	if string(out) != in {
		t.Errorf("string content was mutated; got %s want %s", out, in)
	}
}

// TestStripEmptyTextContent_PreservesNonTextEmptyContent pins that we
// only strip empty TEXT elements — an empty image_url (or any other
// non-text type with empty fields) is left for the upstream to handle.
// We don't pretend to know every provider's validation rules.
func TestStripEmptyTextContent_PreservesNonTextEmptyContent(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [
				{"type": "image_url", "image_url": {"url": ""}}
			]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	if string(out) != in {
		t.Errorf("non-text content was mutated; got %s want %s", out, in)
	}
}

// TestStripEmptyTextContent_MultiMessage pins that the shim walks every
// message independently — empty-text in message[0] does not affect
// message[1]'s content.
func TestStripEmptyTextContent_MultiMessage(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": ""}, {"type": "text", "text": "keep"}]},
			{"role": "assistant", "content": [{"type": "text", "text": "assistant reply"}]},
			{"role": "user", "content": [{"type": "text", "text": ""}]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	if got := gjson.GetBytes(out, "messages.0.content.0.text").String(); got != "keep" {
		t.Errorf("messages[0]: kept text = %q, want keep", got)
	}
	if got := len(gjson.GetBytes(out, "messages.0.content").Array()); got != 1 {
		t.Errorf("messages[0]: content len = %d, want 1", got)
	}
	if got := gjson.GetBytes(out, "messages.1.content.0.text").String(); got != "assistant reply" {
		t.Errorf("messages[1]: untouched, got %q", got)
	}
	if got := gjson.GetBytes(out, "messages.2.content.0.text").String(); got != emptyTextPlaceholder {
		t.Errorf("messages[2]: replaced with placeholder, got %q", got)
	}
}

// TestStripEmptyTextContent_NoMessagesField pins that bodies without a
// messages field (embeddings, image generation, etc.) pass through
// untouched.
func TestStripEmptyTextContent_NoMessagesField(t *testing.T) {
	in := `{"input": "embed this", "model": "ail-embed"}`
	out := stripEmptyTextContent([]byte(in))
	if string(out) != in {
		t.Errorf("non-chat body was modified; got %s want %s", out, in)
	}
}

// TestStripEmptyTextContent_EmptyBody pins that an empty payload is
// returned unchanged, matching the existing executor shim contract.
func TestStripEmptyTextContent_EmptyBody(t *testing.T) {
	out := stripEmptyTextContent([]byte{})
	if len(out) != 0 {
		t.Errorf("empty body should round-trip empty; got %q", out)
	}
}

// TestStripEmptyTextContent_Idempotent pins that running the shim twice
// produces the same output as running it once. Important for any future
// caller that ends up double-applying it via different code paths.
func TestStripEmptyTextContent_Idempotent(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": ""}, {"type": "text", "text": "ok"}]}
		]
	}`
	once := stripEmptyTextContent([]byte(in))
	twice := stripEmptyTextContent(once)
	if string(once) != string(twice) {
		t.Errorf("non-idempotent\nonce:  %s\ntwice: %s", once, twice)
	}
}

// TestStripEmptyTextContent_OutputValidJSON pins that the rewritten body
// remains valid JSON. Catches sjson edge cases where partial writes
// could otherwise leave dangling commas or brackets.
func TestStripEmptyTextContent_OutputValidJSON(t *testing.T) {
	in := `{
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": ""}, {"type": "text", "text": "a"}]},
			{"role": "user", "content": [{"type": "text", "text": ""}]}
		]
	}`
	out := stripEmptyTextContent([]byte(in))
	var v any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("output is invalid JSON: %v\nbody: %s", err, out)
	}
}
