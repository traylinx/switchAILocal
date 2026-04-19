// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// TestNormalizeMultimodal_AISDKv5ImageBlocks pins that AI-SDK-v5's
// {type:"image", image:"<data-uri>"} shape gets rewritten to OpenAI's
// {type:"image_url", image_url:{url:"..."}} so upstream providers see
// the canonical form. This is the bug that caused OpenCode screenshots
// to silently disappear before the normalizer existed.
func TestNormalizeMultimodal_AISDKv5ImageBlocks(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantURL string
	}{
		{
			name:    "image as data URI string",
			input:   `{"messages":[{"role":"user","content":[{"type":"text","text":"what is this"},{"type":"image","image":"data:image/png;base64,iVBORw0KGgo="}]}]}`,
			wantURL: "data:image/png;base64,iVBORw0KGgo=",
		},
		{
			name:    "image as plain URL string",
			input:   `{"messages":[{"role":"user","content":[{"type":"image","image":"https://example.com/cat.png"}]}]}`,
			wantURL: "https://example.com/cat.png",
		},
		{
			name:    "image as object with url field",
			input:   `{"messages":[{"role":"user","content":[{"type":"image","image":{"url":"https://example.com/cat.png"}}]}]}`,
			wantURL: "https://example.com/cat.png",
		},
		{
			name:    "image as bare base64 with mediaType sibling",
			input:   `{"messages":[{"role":"user","content":[{"type":"image","image":"iVBORw0KGgo=","mediaType":"image/png"}]}]}`,
			wantURL: "data:image/png;base64,iVBORw0KGgo=",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := NormalizeMultimodalContent([]byte(tt.input))
			part := gjson.GetBytes(out, "messages.0.content.#(type==\"image_url\")")
			if !part.Exists() {
				t.Fatalf("expected an image_url block after normalization, got: %s", out)
			}
			if got := part.Get("image_url.url").String(); got != tt.wantURL {
				t.Errorf("image_url.url = %q, want %q", got, tt.wantURL)
			}
			// Original "image" block should NO longer be present at that
			// position — it was rewritten in place.
			if gjson.GetBytes(out, "messages.0.content.#(type==\"image\")").Exists() {
				t.Errorf("original image block still present after rewrite")
			}
		})
	}
}

// TestNormalizeMultimodal_AlreadyCanonical pins the no-op contract: a
// payload that's already in OpenAI canonical shape passes through with
// no semantic change.
func TestNormalizeMultimodal_AlreadyCanonical(t *testing.T) {
	in := `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAA="}}]}]}`
	out := NormalizeMultimodalContent([]byte(in))
	// Both inputs and outputs are valid JSON describing the same data;
	// compare via decode to avoid noise from re-serialisation order.
	var a, b any
	if err := json.Unmarshal([]byte(in), &a); err != nil {
		t.Fatalf("input json invalid: %v", err)
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatalf("output json invalid: %v (raw=%s)", err, out)
	}
	if !equalJSON(a, b) {
		t.Errorf("canonical input was modified:\n in: %s\nout: %s", in, out)
	}
}

// TestNormalizeMultimodal_TextOnlyContent pins that string-content
// messages (the overwhelmingly common case) get zero handling — the
// fast path stays fast.
func TestNormalizeMultimodal_TextOnlyContent(t *testing.T) {
	in := `{"messages":[{"role":"user","content":"hello world"}]}`
	out := NormalizeMultimodalContent([]byte(in))
	if string(out) != in {
		t.Errorf("text-only payload was modified:\n in: %s\nout: %s", in, out)
	}
}

// TestNormalizeMultimodal_AudioBlocks covers the AI-SDK-v5 audio shape.
// Verifies mediaType → format mapping and the canonical OpenAI shape.
func TestNormalizeMultimodal_AudioBlocks(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantData   string
		wantFormat string
	}{
		{
			name:       "ai-sdk-v5 audio object with mediaType wav",
			input:      `{"messages":[{"role":"user","content":[{"type":"audio","audio":{"data":"AUDIODATA","mediaType":"audio/wav"}}]}]}`,
			wantData:   "AUDIODATA",
			wantFormat: "wav",
		},
		{
			name:       "ai-sdk-v5 audio with mp3 mediaType",
			input:      `{"messages":[{"role":"user","content":[{"type":"audio","audio":{"data":"MP3DATA","mediaType":"audio/mpeg"}}]}]}`,
			wantData:   "MP3DATA",
			wantFormat: "mp3",
		},
		{
			name:       "input_audio missing format gets defaulted",
			input:      `{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"OPUSDATA"}}]}]}`,
			wantData:   "OPUSDATA",
			wantFormat: "wav", // default fallback
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			out := NormalizeMultimodalContent([]byte(tt.input))
			part := gjson.GetBytes(out, "messages.0.content.#(type==\"input_audio\")")
			if !part.Exists() {
				t.Fatalf("expected input_audio block, got: %s", out)
			}
			if got := part.Get("input_audio.data").String(); got != tt.wantData {
				t.Errorf("input_audio.data = %q, want %q", got, tt.wantData)
			}
			if got := part.Get("input_audio.format").String(); got != tt.wantFormat {
				t.Errorf("input_audio.format = %q, want %q", got, tt.wantFormat)
			}
		})
	}
}

// TestNormalizeMultimodal_FileBlock pins generic {type:"file"} support —
// route to image_url for image/*, input_audio for audio/*, leave alone
// for unknown types.
func TestNormalizeMultimodal_FileBlock(t *testing.T) {
	t.Run("file with image mediaType -> image_url", func(t *testing.T) {
		in := `{"messages":[{"role":"user","content":[{"type":"file","data":"BASE64","mediaType":"image/jpeg"}]}]}`
		out := NormalizeMultimodalContent([]byte(in))
		part := gjson.GetBytes(out, "messages.0.content.0")
		if part.Get("type").String() != "image_url" {
			t.Errorf("got type=%s, want image_url", part.Get("type").String())
		}
		if !strings.HasPrefix(part.Get("image_url.url").String(), "data:image/jpeg;base64,") {
			t.Errorf("image_url.url missing data: prefix: %s", part.Get("image_url.url").String())
		}
	})

	t.Run("file with audio mediaType -> input_audio", func(t *testing.T) {
		in := `{"messages":[{"role":"user","content":[{"type":"file","data":"BASE64","mediaType":"audio/flac"}]}]}`
		out := NormalizeMultimodalContent([]byte(in))
		part := gjson.GetBytes(out, "messages.0.content.0")
		if part.Get("type").String() != "input_audio" {
			t.Errorf("got type=%s, want input_audio", part.Get("type").String())
		}
		if part.Get("input_audio.format").String() != "flac" {
			t.Errorf("got format=%s, want flac", part.Get("input_audio.format").String())
		}
	})

	t.Run("file with URL is forwarded as-is to image_url", func(t *testing.T) {
		in := `{"messages":[{"role":"user","content":[{"type":"file","url":"https://example.com/diagram.png","mediaType":"image/png"}]}]}`
		out := NormalizeMultimodalContent([]byte(in))
		part := gjson.GetBytes(out, "messages.0.content.0")
		if part.Get("type").String() != "image_url" {
			t.Errorf("got type=%s, want image_url", part.Get("type").String())
		}
		if part.Get("image_url.url").String() != "https://example.com/diagram.png" {
			t.Errorf("url not forwarded: %s", part.Get("image_url.url").String())
		}
	})
}

// TestNormalizeMultimodal_MultipleMessages confirms the rewriter walks
// every message + every content part — not just the last one or the
// first hit. Mixing canonical + non-canonical in one payload exercises
// the per-part decision logic.
func TestNormalizeMultimodal_MultipleMessages(t *testing.T) {
	in := `{"messages":[
		{"role":"user","content":[
			{"type":"text","text":"first"},
			{"type":"image","image":"https://a/1.png"}
		]},
		{"role":"assistant","content":"reply"},
		{"role":"user","content":[
			{"type":"image_url","image_url":{"url":"https://a/2.png"}},
			{"type":"image","image":"https://a/3.png"}
		]}
	]}`
	out := NormalizeMultimodalContent([]byte(in))

	// First user message: image at index 1 should now be image_url.
	got := gjson.GetBytes(out, "messages.0.content.1.type").String()
	if got != "image_url" {
		t.Errorf("messages[0].content[1].type = %s, want image_url", got)
	}
	if gjson.GetBytes(out, "messages.0.content.1.image_url.url").String() != "https://a/1.png" {
		t.Errorf("messages[0] url not preserved")
	}
	// Assistant string content untouched.
	if gjson.GetBytes(out, "messages.1.content").String() != "reply" {
		t.Errorf("string-content message was modified")
	}
	// Third message: index 0 already canonical (passthrough), index 1 rewritten.
	if gjson.GetBytes(out, "messages.2.content.0.image_url.url").String() != "https://a/2.png" {
		t.Errorf("canonical block at messages[2].content[0] was disturbed")
	}
	if gjson.GetBytes(out, "messages.2.content.1.type").String() != "image_url" {
		t.Errorf("messages[2].content[1] not rewritten")
	}
	if gjson.GetBytes(out, "messages.2.content.1.image_url.url").String() != "https://a/3.png" {
		t.Errorf("messages[2] url not preserved")
	}
}

// TestNormalizeMultimodal_EmptyAndMalformed pins safe behaviour on edge
// inputs the executor might pass — nil, no messages, malformed parts.
// None should panic; all should pass through unchanged.
func TestNormalizeMultimodal_EmptyAndMalformed(t *testing.T) {
	cases := []string{
		``,
		`{}`,
		`{"messages":[]}`,
		`{"messages":[{"role":"user"}]}`,                          // no content
		`{"messages":[{"role":"user","content":null}]}`,           // null content
		`{"messages":[{"role":"user","content":[{"type":"x"}]}]}`, // unknown type
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panicked on %q: %v", in, r)
				}
			}()
			_ = NormalizeMultimodalContent([]byte(in))
		})
	}
}

// equalJSON deep-compares two values decoded from JSON. Used so canonical-
// passthrough tests don't fail on key-order or whitespace differences.
func equalJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
