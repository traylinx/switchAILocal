// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package registry

import "testing"

// TestInferCapabilities pins the inference table for the model families
// we route through today. When a new family lands in the gateway, add
// a case here so /v1/models advertises the right capabilities and
// downstream clients (Vercel AI SDK / OpenCode / Cursor) auto-detect.
func TestInferCapabilities(t *testing.T) {
	cases := []struct {
		modelID   string
		wantImg   bool   // input modality includes "image"
		wantAudio bool   // input modality includes "audio"
		wantTool  bool   // tool_call true
		wantOut   string // expected output modality (text / audio / image / embedding)
	}{
		// MiniMax M2.7 — flagship multimodal chat
		{"minimax:MiniMax-M2.7", true, true, true, "text"},
		{"MiniMax-M2.7", true, true, true, "text"},
		{"minimax-m2.5", true, true, true, "text"},

		// MiniMax media adapters
		{"minimax:music-2.6", false, false, false, "audio"},
		{"minimax:music-cover", false, false, false, "audio"},
		{"minimax:speech-02-hd", false, false, false, "audio"},
		{"minimax:speech-2.8-hd", false, false, false, "audio"},
		{"minimax:image-01", false, false, false, "image"},

		// xiaomi mimo
		{"xiaomi:mimo-v2-omni", true, true, true, "text"},
		{"xiaomi:mimo-v2-pro", false, false, true, "text"},
		{"xiaomi:mimo-v2-tts", false, false, false, "audio"},

		// OpenAI family
		{"gpt-4o", true, false, true, "text"},
		{"gpt-4o-mini", true, false, true, "text"},
		{"gpt-5-something", true, false, true, "text"},
		{"o1-preview", false, false, false, "text"},
		{"o3-mini", false, false, false, "text"},

		// Claude family
		{"claude-3.5-sonnet", true, false, true, "text"},
		{"claude-3-5-haiku", true, false, true, "text"},
		{"claude-opus-4-7", true, false, true, "text"},
		{"claude-sonnet-4-6", true, false, true, "text"},

		// Gemini
		{"gemini-1.5-pro", true, true, true, "text"},
		{"gemini-2.0-flash", true, true, true, "text"},

		// Whisper / TTS / embedding
		{"whisper-large-v3", false, true, false, "text"},
		{"tts-1", false, false, false, "audio"},
		{"text-embedding-3-small", false, false, false, "embedding"},
		{"qwen3-embedding:0.6b", false, false, false, "embedding"},

		// Generic vision keyword
		{"qwen-vl-72b", true, false, true, "text"},
		{"some-vision-llm", true, false, true, "text"},

		// Plain text-tool models
		{"qwen-plus", false, false, true, "text"},
		{"glm-5", false, false, true, "text"},
		{"openai/gpt-oss-20b", false, false, true, "text"},
		{"mercury-2", false, false, true, "text"},
	}

	for _, tt := range cases {
		t.Run(tt.modelID, func(t *testing.T) {
			caps := InferCapabilities(tt.modelID)
			if caps == nil {
				t.Fatalf("got nil for %q (should always return defaults)", tt.modelID)
			}
			gotImg := containsString(caps.Modalities.Input, "image")
			gotAudio := containsString(caps.Modalities.Input, "audio")
			if gotImg != tt.wantImg {
				t.Errorf("image-input: got=%v want=%v (modalities.input=%v)", gotImg, tt.wantImg, caps.Modalities.Input)
			}
			if gotAudio != tt.wantAudio {
				t.Errorf("audio-input: got=%v want=%v (modalities.input=%v)", gotAudio, tt.wantAudio, caps.Modalities.Input)
			}
			if caps.ToolCall != tt.wantTool {
				t.Errorf("tool_call: got=%v want=%v", caps.ToolCall, tt.wantTool)
			}
			if !containsString(caps.Modalities.Output, tt.wantOut) {
				t.Errorf("output modality %q missing from %v", tt.wantOut, caps.Modalities.Output)
			}
			// Attachment must be true whenever any non-text input modality
			// is supported — that's the contract OpenCode-class clients
			// read to decide whether to forward image bytes.
			anyMedia := gotImg || gotAudio || containsString(caps.Modalities.Input, "pdf") || containsString(caps.Modalities.Input, "video")
			if caps.Attachment != anyMedia {
				t.Errorf("attachment=%v but anyMedia=%v (input=%v)", caps.Attachment, anyMedia, caps.Modalities.Input)
			}
		})
	}
}

// TestInferCapabilities_UnknownReturnsTextDefaults pins the unknown-id
// fallback: rather than nil (which would cause vision-aware clients to
// strip silently), unknown IDs return text-in/text-out + no tools. This
// is honest and predictable.
func TestInferCapabilities_UnknownReturnsTextDefaults(t *testing.T) {
	caps := InferCapabilities("totally-made-up-model-name-9000")
	if caps == nil {
		t.Fatal("got nil — unknown ID should return text defaults, not nil")
	}
	if containsString(caps.Modalities.Input, "image") {
		t.Errorf("unknown model should not claim image input")
	}
	if !containsString(caps.Modalities.Output, "text") {
		t.Errorf("unknown model should default to text output")
	}
	if caps.Attachment {
		t.Errorf("attachment should be false for text-only default")
	}
	if caps.ToolCall {
		t.Errorf("tool_call should be false for unknown text-only model (caller can override)")
	}
}

// TestInferCapabilities_EmptyID covers the truly-empty case — caller
// passed "" and we return nil so the field is omitted from /v1/models
// rather than being filled with misleading defaults for a phantom model.
func TestInferCapabilities_EmptyID(t *testing.T) {
	if caps := InferCapabilities(""); caps != nil {
		t.Errorf("expected nil for empty ID, got %+v", caps)
	}
}

// TestResolveCapabilities_ExplicitOverridesInference pins that an
// explicit Capabilities on a ModelInfo wins over inference. Operators
// who want to claim vision on a model the inference table doesn't
// recognise can do so via config.
func TestResolveCapabilities_ExplicitOverridesInference(t *testing.T) {
	m := &ModelInfo{
		ID: "totally-unknown-model",
		Capabilities: &ModelCapabilities{
			Attachment: true,
			ToolCall:   true,
			Modalities: ModelModalities{
				Input:  []string{"text", "image"},
				Output: []string{"text"},
			},
		},
	}
	caps := resolveCapabilities(m)
	if caps == nil || !caps.Attachment {
		t.Fatalf("explicit caps lost: %+v", caps)
	}
	if !containsString(caps.Modalities.Input, "image") {
		t.Errorf("explicit image input dropped")
	}
}
