// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package registry

import (
	"testing"
)

// TestConvertModelToMap_OpenAI_NativeTools pins that an openai-shaped
// /v1/models entry carries the operator-declared native_tools array
// in the exact shape agentic callers (OpenClaw, Hermes) expect to see
// — a list of {type, description, params} maps with lower_snake_case
// keys. Emptiness is suppressed (we don't want a dangling "native_tools":
// null on every non-agentic model). Spotted-check: "type" is the first
// field agents look for to splice into their own tools[] array.
func TestConvertModelToMap_OpenAI_NativeTools(t *testing.T) {
	reg := &ModelRegistry{}
	model := &ModelInfo{
		ID:      "ail-compound",
		Object:  "model",
		OwnedBy: "minimax",
		Type:    "openai-compatibility",
		NativeTools: []NativeTool{
			{
				Type:        "web_search",
				Description: "MiniMax native web search",
				Params: map[string]any{
					"force_search": map[string]any{"type": "boolean", "default": false},
					"limit":        map[string]any{"type": "integer", "default": 3},
				},
			},
		},
	}

	out := reg.convertModelToMap(model, "openai")
	if out == nil {
		t.Fatal("convertModelToMap returned nil")
	}
	raw, ok := out["native_tools"]
	if !ok {
		t.Fatalf("native_tools missing from openai-shape map: %#v", out)
	}
	list, ok := raw.([]map[string]any)
	if !ok {
		t.Fatalf("native_tools wrong type: got %T want []map[string]any", raw)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	entry := list[0]
	if entry["type"] != "web_search" {
		t.Errorf("type: got %v want web_search", entry["type"])
	}
	if entry["description"] != "MiniMax native web search" {
		t.Errorf("description: got %v", entry["description"])
	}
	params, ok := entry["params"].(map[string]any)
	if !ok {
		t.Fatalf("params missing / wrong shape: %#v", entry)
	}
	if _, hasForce := params["force_search"]; !hasForce {
		t.Errorf("params.force_search missing: %#v", params)
	}
}

// TestConvertModelToMap_OpenAI_NoNativeTools pins the omission rule:
// a model without any provider-native tools must NOT emit a
// "native_tools" key on its /v1/models entry (callers key off key
// presence to decide whether to merge). A nil / empty slice would
// marshal to "native_tools": null — we want the key gone entirely.
func TestConvertModelToMap_OpenAI_NoNativeTools(t *testing.T) {
	reg := &ModelRegistry{}
	model := &ModelInfo{
		ID:      "ail-image",
		Object:  "model",
		OwnedBy: "minimax",
		Type:    "openai-compatibility",
		// NativeTools intentionally unset
	}
	out := reg.convertModelToMap(model, "openai")
	if out == nil {
		t.Fatal("convertModelToMap returned nil")
	}
	if _, ok := out["native_tools"]; ok {
		t.Errorf("native_tools should be absent for model without tools; got %#v", out["native_tools"])
	}
}

// TestCloneModelInfo_NativeTools pins that the registry's defensive
// clone preserves NativeTools. If it didn't, a mutation through one
// registered client's model could bleed into /v1/models' response for
// every other client sharing the same upstream.
func TestCloneModelInfo_NativeTools(t *testing.T) {
	src := &ModelInfo{
		ID: "ail-compound",
		NativeTools: []NativeTool{
			{Type: "web_search", Description: "d", Params: map[string]any{"k": 1}},
		},
	}
	dst := cloneModelInfo(src)
	if dst == nil {
		t.Fatal("cloneModelInfo returned nil")
	}
	if len(dst.NativeTools) != 1 {
		t.Fatalf("native_tools not copied: %#v", dst.NativeTools)
	}
	// Mutate clone → source must stay intact.
	dst.NativeTools[0].Type = "mutated"
	dst.NativeTools[0].Params["k"] = 999
	if src.NativeTools[0].Type != "web_search" {
		t.Errorf("clone aliased source.Type: %s", src.NativeTools[0].Type)
	}
	if src.NativeTools[0].Params["k"] != 1 {
		t.Errorf("clone aliased source.Params: %#v", src.NativeTools[0].Params)
	}
}
