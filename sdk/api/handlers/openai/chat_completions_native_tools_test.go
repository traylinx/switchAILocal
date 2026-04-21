// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"testing"

	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/registry"
)

// registerNativeToolsOnRegistry installs a model with the given native_tools
// onto the global registry for the duration of the test. Returns a cleanup
// func the caller must defer so we don't leak registrations across tests.
func registerNativeToolsOnRegistry(t *testing.T, modelID string, tools []registry.NativeTool) func() {
	t.Helper()
	reg := registry.GetGlobalRegistry()
	clientID := "native-tools-test-" + t.Name()
	reg.RegisterClient(clientID, "minimax", []*registry.ModelInfo{{
		ID:          modelID,
		Object:      "model",
		OwnedBy:     "minimax",
		Type:        "openai-compatibility",
		NativeTools: tools,
	}})
	return func() { reg.UnregisterClient(clientID) }
}

// TestAutoInject_Discovery_InjectsNativeTools pins that a model with
// native_tools declared in the registry receives them on auto-inject
// — caller tools preserved, web_search appended. Discovery supersedes
// the env-var allowlist (AIL_AUTOINJECT_MODELS).
func TestAutoInject_Discovery_InjectsNativeTools(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	// Intentionally leave AIL_AUTOINJECT_MODELS empty — discovery should win.
	t.Setenv("AIL_AUTOINJECT_MODELS", "")

	cleanup := registerNativeToolsOnRegistry(t, "discovery-compound", []registry.NativeTool{
		{Type: "web_search"},
	})
	defer cleanup()

	body := []byte(`{"model":"discovery-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, "discovery-compound", false)

	types := toolTypes(t, out)
	if len(types) != 1 || types[0] != "web_search" {
		t.Fatalf("expected exactly [web_search], got %v (body=%s)", types, string(out))
	}
	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != webSearchMaxTokensFloor {
		t.Errorf("max_tokens not bumped to floor: got %d want %d", mt, webSearchMaxTokensFloor)
	}
}

// TestAutoInject_Discovery_OverridesAllowlist pins that once a model has
// native_tools in the registry, the env-var allowlist becomes irrelevant
// — even if the model is NOT in AIL_AUTOINJECT_MODELS, discovery fires.
// This is the clean-up story for operators who declared native_tools in
// YAML: they can drop AIL_AUTOINJECT_MODELS entirely.
func TestAutoInject_Discovery_OverridesAllowlist(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	t.Setenv("AIL_AUTOINJECT_MODELS", "some-other-model") // deliberately doesn't include our model

	cleanup := registerNativeToolsOnRegistry(t, "declared-compound", []registry.NativeTool{
		{Type: "web_search"},
	})
	defer cleanup()

	body := []byte(`{"model":"declared-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, "declared-compound", false)

	types := toolTypes(t, out)
	if countWebSearchEntries(t, out) != 1 {
		t.Fatalf("expected 1 web_search entry (discovery fires regardless of env allowlist), got types=%v", types)
	}
}

// TestAutoInject_Discovery_NoNativeToolsFallsBackToEnv pins that a
// model WITHOUT native_tools in the registry still hits the legacy
// env-var allowlist path. This preserves the 2026-04-21 behavior
// for operators who haven't migrated their config to native_tools yet.
func TestAutoInject_Discovery_NoNativeToolsFallsBackToEnv(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	t.Setenv("AIL_AUTOINJECT_MODELS", "env-compound")

	cleanup := registerNativeToolsOnRegistry(t, "env-compound", nil) // explicit: no native_tools
	defer cleanup()

	body := []byte(`{"model":"env-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, "env-compound", false)

	if countWebSearchEntries(t, out) != 1 {
		t.Fatalf("env fallback should have injected web_search; types=%v", toolTypes(t, out))
	}
}

// TestAutoInject_Discovery_Dedupe pins that when the caller already
// declared web_search, discovery-driven autoinject does not double-append.
// This is the Phase 5 "autoinject is fallback" story — once the caller
// (e.g. a future OpenClaw version) sends its own web_search, autoinject
// becomes a no-op.
func TestAutoInject_Discovery_Dedupe(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	t.Setenv("AIL_AUTOINJECT_MODELS", "")

	cleanup := registerNativeToolsOnRegistry(t, "dedupe-compound", []registry.NativeTool{
		{Type: "web_search"},
	})
	defer cleanup()

	body := []byte(`{"model":"dedupe-compound","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search","max_keyword":5}]}`)
	out := autoInjectWebSearch(body, "dedupe-compound", false)

	if countWebSearchEntries(t, out) != 1 {
		t.Fatalf("double-inject: got %d web_search entries; types=%v", countWebSearchEntries(t, out), toolTypes(t, out))
	}
	// Caller's parameterised entry must be preserved byte-for-byte (max_keyword kept).
	if mk := gjson.GetBytes(out, "tools.0.max_keyword").Int(); mk != 5 {
		t.Errorf("caller's parameterised web_search clobbered: max_keyword=%d", mk)
	}
}

// TestAutoInject_Discovery_FiresWhenMasterFlagOff pins the Phase 5 gate
// split: discovery runs regardless of AIL_AUTOINJECT_WEBSEARCH. Operators
// who declared native_tools in config don't need the env flag — the
// config IS the operator's opt-in. Without this split, Phase 5's
// acceptance matrix row 2 ("AIL_AUTOINJECT_WEBSEARCH=false, discovery
// ON → search fires") would fail, and Phase 2's "autoinject is now
// config-driven, demoted to fallback" narrative would be aspirational
// rather than delivered.
func TestAutoInject_Discovery_FiresWhenMasterFlagOff(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "false")
	t.Setenv("AIL_AUTOINJECT_MODELS", "")

	cleanup := registerNativeToolsOnRegistry(t, "flagoff-compound", []registry.NativeTool{
		{Type: "web_search"},
	})
	defer cleanup()

	body := []byte(`{"model":"flagoff-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, "flagoff-compound", false)

	if countWebSearchEntries(t, out) != 1 {
		t.Fatalf("discovery should fire even when AIL_AUTOINJECT_WEBSEARCH=false; got types=%v", toolTypes(t, out))
	}
}

// TestAutoInject_NoDiscoveryNoEnv_NoOp pins the "search does NOT fire"
// quadrant of the Phase 5 matrix: model has no native_tools AND master
// flag is false. Nothing happens — matches the pre-2026-04-21 baseline.
// Regression-guards against an accidental "inject by default" future edit.
func TestAutoInject_NoDiscoveryNoEnv_NoOp(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "false")
	t.Setenv("AIL_AUTOINJECT_MODELS", "off-compound")

	cleanup := registerNativeToolsOnRegistry(t, "off-compound", nil)
	defer cleanup()

	body := []byte(`{"model":"off-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, "off-compound", false)

	if countWebSearchEntries(t, out) != 0 {
		t.Fatalf("no injection expected; got types=%v", toolTypes(t, out))
	}
}

// TestAutoInject_Discovery_ForceSearchThresholdStillApplies pins that
// the force_search stamping (2026-04-21 audit §4.2) continues to trigger
// on the discovery-driven path when the caller has many function tools.
// Important because MiniMax's function-tool preference heuristic is still
// real even when web_search comes from discovery.
func TestAutoInject_Discovery_ForceSearchThresholdStillApplies(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	t.Setenv("AIL_AUTOINJECT_MODELS", "")
	t.Setenv("AIL_AUTOINJECT_FORCE_THRESHOLD", "5")

	cleanup := registerNativeToolsOnRegistry(t, "force-compound", []registry.NativeTool{
		{Type: "web_search"},
	})
	defer cleanup()

	// 6 function tools → threshold tripped → injected entry should have force_search:true.
	fn := `{"type":"function","function":{"name":"x","description":"x","parameters":{}}}`
	body := []byte(`{"model":"force-compound","messages":[],"tools":[` +
		fn + `,` + fn + `,` + fn + `,` + fn + `,` + fn + `,` + fn + `]}`)
	out := autoInjectWebSearch(body, "force-compound", false)

	// The injected web_search is the last tool (index 6).
	if got := gjson.GetBytes(out, "tools.6.type").String(); got != "web_search" {
		t.Fatalf("index 6 should be web_search, got %q", got)
	}
	if force := gjson.GetBytes(out, "tools.6.force_search").Bool(); !force {
		t.Errorf("force_search not stamped for 6-function-tool caller; body=%s", string(out))
	}
}
