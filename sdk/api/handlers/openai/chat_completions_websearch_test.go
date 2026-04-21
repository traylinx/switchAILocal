// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"testing"

	"github.com/tidwall/gjson"
)

const (
	allowlistModel    = "ail-compound"
	nonAllowlistModel = "ail-image"
)

// configureAllowlist sets the autoinject env pair for the duration of the test.
// t.Setenv cleans up automatically.
func configureAllowlist(t *testing.T) {
	t.Helper()
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "true")
	t.Setenv("AIL_AUTOINJECT_MODELS", allowlistModel+",minimax/ail-compound")
}

// toolTypes returns the list of tool.type values in the given request body, in order.
func toolTypes(t *testing.T, body []byte) []string {
	t.Helper()
	var out []string
	tools := gjson.GetBytes(body, "tools")
	if !tools.Exists() {
		return out
	}
	tools.ForEach(func(_, tool gjson.Result) bool {
		out = append(out, tool.Get("type").String())
		return true
	})
	return out
}

func countWebSearchEntries(t *testing.T, body []byte) int {
	t.Helper()
	n := 0
	for _, typ := range toolTypes(t, body) {
		if typ == "web_search" {
			n++
		}
	}
	return n
}

// --- Phase 3 unit tests ---------------------------------------------------

func TestAutoInject_NoTools_Allowlisted_FlagOn_AppendsWebSearch(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	types := toolTypes(t, out)
	if len(types) != 1 || types[0] != "web_search" {
		t.Fatalf("expected exactly [web_search], got %v (body=%s)", types, out)
	}
}

func TestAutoInject_CallerToolX_Allowlisted_FlagOn_AppendsWebSearchPreservesX(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"other_tool","name":"x"}]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	types := toolTypes(t, out)
	if len(types) != 2 || types[0] != "other_tool" || types[1] != "web_search" {
		t.Fatalf("expected [other_tool, web_search], got %v (body=%s)", types, out)
	}
}

func TestAutoInject_CallerAlreadyHasWebSearch_FlagOn_NoDoubleInject(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","tools":[{"type":"web_search"},{"type":"other_tool"}]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if got := countWebSearchEntries(t, out); got != 1 {
		t.Fatalf("expected exactly 1 web_search entry (dedupe by type), got %d (body=%s)", got, out)
	}
	types := toolTypes(t, out)
	if len(types) != 2 {
		t.Fatalf("expected 2 tool entries total, got %d: %v", len(types), types)
	}
}

func TestAutoInject_CallerParameterizedWebSearch_FlagOn_NoClobber(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","tools":[{"type":"web_search","max_keyword":5,"force_search":true}]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if got := countWebSearchEntries(t, out); got != 1 {
		t.Fatalf("caller's parameterized web_search must survive (dedupe), got %d entries", got)
	}
	if max := gjson.GetBytes(out, "tools.0.max_keyword").Int(); max != 5 {
		t.Fatalf("expected caller's max_keyword=5 preserved, got %d (body=%s)", max, out)
	}
	if fs := gjson.GetBytes(out, "tools.0.force_search").Bool(); !fs {
		t.Fatalf("expected caller's force_search=true preserved (body=%s)", out)
	}
}

func TestAutoInject_NonAllowlistedModel_FlagOn_NoInject(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-image","messages":[]}`)
	out := autoInjectWebSearch(body, nonAllowlistModel, false)

	if got := countWebSearchEntries(t, out); got != 0 {
		t.Fatalf("non-allowlisted model must not receive injection, got %d web_search entries", got)
	}
}

func TestAutoInject_OptOutHeader_FlagOn_NoInject(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[]}`)
	out := autoInjectWebSearch(body, allowlistModel, true) // optOut=true

	if got := countWebSearchEntries(t, out); got != 0 {
		t.Fatalf("X-Ail-Autoinject: off must suppress injection, got %d web_search entries", got)
	}
}

func TestAutoInject_FlagOff_Regression(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "false")
	t.Setenv("AIL_AUTOINJECT_MODELS", allowlistModel)
	body := []byte(`{"model":"ail-compound","messages":[]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if got := countWebSearchEntries(t, out); got != 0 {
		t.Fatalf("flag OFF must leave body untouched, got %d web_search entries", got)
	}
}

// --- max_tokens floor tests ----------------------------------------------

func TestAutoInject_MaxTokensAbsent_Injects_SetsFloor(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[]}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != 2000 {
		t.Fatalf("absent max_tokens should be set to 2000 floor, got %d (body=%s)", mt, out)
	}
}

func TestAutoInject_MaxTokensLow_Injects_BumpsToFloor(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[],"max_tokens":500}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != 2000 {
		t.Fatalf("max_tokens=500 should be bumped to 2000, got %d (body=%s)", mt, out)
	}
}

func TestAutoInject_MaxTokensHigh_Injects_Preserves(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[],"max_tokens":4000}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != 4000 {
		t.Fatalf("max_tokens=4000 must be preserved (>= floor), got %d", mt)
	}
}

func TestAutoInject_MaxTokensLow_FlagOff_Preserves(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_WEBSEARCH", "false")
	t.Setenv("AIL_AUTOINJECT_MODELS", allowlistModel)
	body := []byte(`{"model":"ail-compound","messages":[],"max_tokens":500}`)
	out := autoInjectWebSearch(body, allowlistModel, false)

	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != 500 {
		t.Fatalf("flag OFF must not touch max_tokens, got %d", mt)
	}
}

func TestAutoInject_MaxTokensLow_OptOut_Preserves(t *testing.T) {
	configureAllowlist(t)
	body := []byte(`{"model":"ail-compound","messages":[],"max_tokens":500}`)
	out := autoInjectWebSearch(body, allowlistModel, true) // optOut

	if mt := gjson.GetBytes(out, "max_tokens").Int(); mt != 500 {
		t.Fatalf("opt-out must not touch max_tokens, got %d", mt)
	}
}

// --- Allowlist helper tests ----------------------------------------------

func TestIsAutoinjectAllowlisted_EmptyEnv_ReturnsFalse(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_MODELS", "")
	if isAutoinjectAllowlisted("ail-compound") {
		t.Fatalf("empty env should disallow all models")
	}
}

func TestIsAutoinjectAllowlisted_ModelInList_ReturnsTrue(t *testing.T) {
	t.Setenv("AIL_AUTOINJECT_MODELS", " ail-compound , minimax/ail-compound ")
	if !isAutoinjectAllowlisted("ail-compound") {
		t.Fatalf("ail-compound should be allowlisted (whitespace trim)")
	}
	if !isAutoinjectAllowlisted("minimax/ail-compound") {
		t.Fatalf("minimax/ail-compound should be allowlisted")
	}
	if isAutoinjectAllowlisted("ail-image") {
		t.Fatalf("ail-image should not be allowlisted")
	}
}

// --- Build identity / debug dump helpers ---------------------------------

func TestBuildIdentity_NonEmpty(t *testing.T) {
	// No ldflags in tests → Commit defaults to "none", hostname resolves from OS.
	// We only assert non-emptiness & hyphen separator shape.
	id := buildIdentity()
	if id == "" {
		t.Fatalf("buildIdentity should never be empty")
	}
	// Minimum shape: "<something>-<something>".
	if len(id) < 3 || id[0] == '-' || id[len(id)-1] == '-' {
		t.Fatalf("buildIdentity malformed: %q", id)
	}
}
