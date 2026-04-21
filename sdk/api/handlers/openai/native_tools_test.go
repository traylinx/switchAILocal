// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/traylinx/switchAILocal/internal/registry"
	sdkhandlers "github.com/traylinx/switchAILocal/sdk/api/handlers"
	"github.com/traylinx/switchAILocal/sdk/config"
)

// newTestOpenAIHandler builds the smallest OpenAIAPIHandler that
// /v1/models needs: a BaseAPIHandler with a non-nil SDKConfig so
// h.Cfg.Intelligence.Matrix lookups don't panic.
func newTestOpenAIHandler() *OpenAIAPIHandler {
	return &OpenAIAPIHandler{
		BaseAPIHandler: &sdkhandlers.BaseAPIHandler{Cfg: &config.SDKConfig{}},
	}
}

// TestOpenAIModels_PreservesNativeTools pins the end-to-end /v1/models
// shape: a model with operator-declared native_tools carries the
// field through the filter step (preserveIfPresent) all the way to
// the JSON response. This is the contract Phase 2 (OpenClaw plugin),
// Phase 3 (tytus capabilities), and Phase 4 (provider passthrough)
// all build on — if this regresses, discovery goes silent and the
// agent stack falls back to the autoinject-only path.
func TestOpenAIModels_PreservesNativeTools(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Register a client whose model carries native_tools into the
	// global registry. Using a fresh client ID so we don't collide
	// with any other test that registers into the global registry.
	reg := registry.GetGlobalRegistry()
	clientID := "test-native-tools-client"
	reg.RegisterClient(clientID, "minimax", []*registry.ModelInfo{
		{
			ID:          "ail-compound-native-test",
			Object:      "model",
			OwnedBy:     "minimax",
			Type:        "openai-compatibility",
			DisplayName: "MiniMax-M2.7",
			NativeTools: []registry.NativeTool{
				{
					Type:        "web_search",
					Description: "MiniMax native web search.",
					Params: map[string]any{
						"force_search": map[string]any{"type": "boolean", "default": false},
					},
				},
			},
		},
	})
	defer reg.UnregisterClient(clientID)

	h := newTestOpenAIHandler()

	r := gin.New()
	r.GET("/v1/models", h.OpenAIModels)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", w.Code, w.Body.String())
	}

	var body struct {
		Object string           `json:"object"`
		Data   []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v\nbody=%s", err, w.Body.String())
	}
	if body.Object != "list" {
		t.Errorf("object: got %q want list", body.Object)
	}

	// Find our test model in the response.
	var target map[string]any
	for _, m := range body.Data {
		if id, _ := m["id"].(string); id == "ail-compound-native-test" {
			target = m
			break
		}
	}
	if target == nil {
		t.Fatalf("test model not found in /v1/models response; body=%s", w.Body.String())
	}

	nt, ok := target["native_tools"]
	if !ok {
		t.Fatalf("native_tools missing from response; entry=%#v", target)
	}
	list, ok := nt.([]any)
	if !ok {
		t.Fatalf("native_tools wrong type after JSON round-trip: got %T", nt)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list))
	}
	entry, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("entry wrong type: got %T", list[0])
	}
	if entry["type"] != "web_search" {
		t.Errorf("type: got %v want web_search", entry["type"])
	}
}

// TestOpenAIModels_OmitsNativeToolsWhenEmpty pins that a model with
// no operator-declared native_tools does NOT get the key in its
// /v1/models entry. Agentic callers use key-presence as a fast check
// for "should I merge anything" — a null/empty sentinel would be a
// compatibility trap.
func TestOpenAIModels_OmitsNativeToolsWhenEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	reg := registry.GetGlobalRegistry()
	clientID := "test-native-tools-empty-client"
	reg.RegisterClient(clientID, "minimax", []*registry.ModelInfo{
		{
			ID:      "ail-image-native-test",
			Object:  "model",
			OwnedBy: "minimax",
			Type:    "openai-compatibility",
			// NativeTools intentionally unset
		},
	})
	defer reg.UnregisterClient(clientID)

	h := newTestOpenAIHandler()
	r := gin.New()
	r.GET("/v1/models", h.OpenAIModels)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}

	var body struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)

	var target map[string]any
	for _, m := range body.Data {
		if id, _ := m["id"].(string); id == "ail-image-native-test" {
			target = m
			break
		}
	}
	if target == nil {
		t.Fatalf("test model not found; body=%s", w.Body.String())
	}
	if _, ok := target["native_tools"]; ok {
		t.Errorf("native_tools should be absent; got %#v", target["native_tools"])
	}
}
