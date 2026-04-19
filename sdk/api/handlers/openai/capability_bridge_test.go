// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"testing"

	"github.com/traylinx/switchAILocal/internal/registry"
)

// TestUpgradeCapabilityFields_VisionFromMatrix pins the bridge between the
// matrix-derived capabilities array and the new structured fields. When
// the operator declared "vision" for a model whose ID our inference table
// doesn't know (typical for matrix-aliased names like ail-compound), the
// new structured fields should reflect the operator's truth: vision=true,
// attachment=true, modalities.input includes "image".
func TestUpgradeCapabilityFields_VisionFromMatrix(t *testing.T) {
	// Start with a model the registry inferred as text-only (because
	// the ID isn't in the inference table).
	model := map[string]any{
		"id":         "ail-compound",
		"object":     "model",
		"attachment": false,
		"vision":     nil,
		"modalities": map[string]any{
			"input":  []string{"text"},
			"output": []string{"text"},
		},
	}
	matrixCaps := []string{"text", "vision", "audio"}

	upgradeCapabilityFields(model, matrixCaps)

	if v, _ := model["vision"].(bool); !v {
		t.Errorf("vision not upgraded to true: %v", model["vision"])
	}
	if v, _ := model["audio"].(bool); !v {
		t.Errorf("audio not upgraded to true: %v", model["audio"])
	}
	if v, _ := model["attachment"].(bool); !v {
		t.Errorf("attachment not upgraded to true: %v", model["attachment"])
	}
	mods, _ := model["modalities"].(map[string]any)
	if mods == nil {
		t.Fatal("modalities lost during upgrade")
	}
	in := stringsFromAny(mods["input"])
	if !containsModality(in, "image") {
		t.Errorf("modalities.input missing image after vision upgrade: %v", in)
	}
	if !containsModality(in, "audio") {
		t.Errorf("modalities.input missing audio after audio upgrade: %v", in)
	}
}

// TestUpgradeCapabilityFields_ImageOnlyMeansOutput pins the heuristic:
// matrix says ["image"] alone for an "image" name → that's image-OUT
// (image generation), not vision-IN. Don't claim attachment+vision for
// it (clients would try to send image bytes to a generation endpoint).
func TestUpgradeCapabilityFields_ImageOnlyMeansOutput(t *testing.T) {
	model := map[string]any{
		"id":         "ail-image",
		"object":     "model",
		"attachment": false,
		"modalities": map[string]any{
			"input":  []string{"text"},
			"output": []string{"text"},
		},
	}
	upgradeCapabilityFields(model, []string{"image"})

	if att, ok := model["attachment"].(bool); ok && att {
		t.Errorf("attachment incorrectly upgraded for image-OUT model")
	}
	if v, ok := model["vision"].(bool); ok && v {
		t.Errorf("vision incorrectly upgraded for image-OUT model")
	}
	mods, _ := model["modalities"].(map[string]any)
	out := stringsFromAny(mods["output"])
	if !containsModality(out, "image") {
		t.Errorf("output should include image: %v", out)
	}
}

// TestUpgradeCapabilityFields_AdditiveOnly pins that the upgrade never
// downgrades — an inferred vision-capable model whose matrix entry only
// says "text" still keeps vision (inference is more specific).
func TestUpgradeCapabilityFields_AdditiveOnly(t *testing.T) {
	model := map[string]any{
		"id":         "minimax:MiniMax-M2.7",
		"vision":     true,
		"audio":      true,
		"attachment": true,
		"modalities": map[string]any{
			"input":  []string{"text", "image", "audio"},
			"output": []string{"text"},
		},
	}
	upgradeCapabilityFields(model, []string{"text"})

	for _, key := range []string{"vision", "audio", "attachment"} {
		if v, _ := model[key].(bool); !v {
			t.Errorf("%s downgraded by matrix=text only", key)
		}
	}
}

// TestUpgradeCapabilityFields_TypedModalities pins that the bridge
// handles the registry.ModelModalities typed struct (which is what the
// registry actually returns) AND the map shape (after JSON round-trip).
func TestUpgradeCapabilityFields_TypedModalities(t *testing.T) {
	model := map[string]any{
		"id":     "ail-compound",
		"object": "model",
		"modalities": registry.ModelModalities{
			Input:  []string{"text"},
			Output: []string{"text"},
		},
	}
	upgradeCapabilityFields(model, []string{"vision"})

	mods, ok := model["modalities"].(map[string]any)
	if !ok {
		t.Fatalf("modalities not converted to map: %T", model["modalities"])
	}
	in := stringsFromAny(mods["input"])
	if !containsModality(in, "image") {
		t.Errorf("modalities.input missing image after typed-struct upgrade: %v", in)
	}
}

// TestStringsFromAny pins the small helper since both call sites depend
// on it normalising []string AND []any (the latter happens whenever JSON
// has been decoded into interface{}).
func TestStringsFromAny(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"[]string", []string{"a", "b"}, []string{"a", "b"}},
		{"[]any with strings", []any{"a", "b"}, []string{"a", "b"}},
		{"[]any with mixed (skips non-strings)", []any{"a", 1, "b"}, []string{"a", "b"}},
		{"nil", nil, nil},
		{"unrelated type", 42, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := stringsFromAny(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %v want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
