// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
)

func TestInspectRequestJSONRestoresBody(t *testing.T) {
	body := []byte(`{"model":"model-a","messages":[{"role":"user","content":"private prompt"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	model, estimate, _, err := InspectRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if model != "model-a" || estimate < 1 {
		t.Fatalf("model=%q estimate=%d", model, estimate)
	}
	restored, _ := io.ReadAll(req.Body)
	if !bytes.Equal(restored, body) {
		t.Fatal("request body was not restored exactly")
	}
}

func TestInspectRequestReservesDeclaredOutputTokensAcrossAPIDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int64
	}{
		{"OpenAI max tokens", `{"model":"model-a","max_tokens":41}`, 41},
		{"OpenAI max completion tokens", `{"model":"model-a","max_completion_tokens":42}`, 42},
		{"Gemini max output tokens", `{"model":"model-a","generationConfig":{"maxOutputTokens":43}}`, 43},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			_, _, reserved, err := InspectRequest(req)
			if err != nil || reserved != tc.want {
				t.Fatalf("reserved=%d err=%v, want %d", reserved, err, tc.want)
			}
		})
	}
}

func TestInspectRequestGeminiPathModel(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1beta/models/model-a:generateContent", bytes.NewBufferString(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	model, _, _, err := InspectRequest(req)
	if err != nil || model != "model-a" {
		t.Fatalf("model=%q err=%v", model, err)
	}
}

func TestInspectRequestMultipartModel(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "audio-model")
	_ = writer.WriteField("prompt", "transcribe carefully")
	file, _ := writer.CreateFormFile("file", "sample.wav")
	_, _ = file.Write([]byte("binary audio"))
	_ = writer.Close()
	req := httptest.NewRequest("POST", "/v1/audio/transcriptions", bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	model, estimate, _, err := InspectRequest(req)
	if err != nil || model != "audio-model" || estimate < 1 {
		t.Fatalf("model=%q estimate=%d err=%v", model, estimate, err)
	}
}

func TestRequiresExplicitModelOnlyForInferenceRoutes(t *testing.T) {
	for _, path := range []string{
		"/v1/chat/completions", "/v1/embeddings", "/openai/v1/images/edits", "/v1beta/models/model-a:generateContent",
	} {
		if !RequiresExplicitModel(httptest.NewRequest("POST", path, nil)) {
			t.Fatalf("inference route %q did not require a model", path)
		}
	}
	for _, path := range []string{"/v1/models", "/openai/v1/models", "/api/provider/openai/v1/models"} {
		if RequiresExplicitModel(httptest.NewRequest("GET", path, nil)) {
			t.Fatalf("non-inference route %q unexpectedly required a model", path)
		}
	}
}

func TestInspectRequestRejectsConflictingModelSources(t *testing.T) {
	tests := []struct {
		name string
		url  string
		body string
	}{
		{"query versus body", "/v1/chat/completions?model=model-a", `{"model":"forbidden"}`},
		{"gemini path versus query", "/v1beta/models/forbidden:generateContent?model=model-a", `{}`},
		{"duplicate top-level model", "/v1/chat/completions", `{"model":"forbidden","model":"model-a"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tt.url, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			_, _, _, err := InspectRequest(req)
			var decision *DecisionError
			if !errors.As(err, &decision) || decision.Code != CodeModelConflict {
				t.Fatalf("error = %v, want %s", err, CodeModelConflict)
			}
		})
	}
}

func TestInspectRequestAcceptsMatchingModelSources(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions?model=model-a", strings.NewReader(`{"model":"model-a"}`))
	req.Header.Set("Content-Type", "application/json")
	model, _, _, err := InspectRequest(req)
	if err != nil || model != "model-a" {
		t.Fatalf("model=%q err=%v", model, err)
	}
}

func TestInspectRequestUsesHandlerRawBodySemanticsRegardlessOfContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{"text plain json", "text/plain", `{"model":"forbidden","messages":[]}`},
		{"json with trailing comma", "application/json", `{"model":"forbidden","messages":[],}`},
		{"json with trailing garbage", "application/json", `{"model":"forbidden"}garbage`},
		{"multipart label over json body", "multipart/form-data; boundary=unused", `{"model":"forbidden"}`},
		{"urlencoded form", "application/x-www-form-urlencoded", `model=forbidden&prompt=private`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions?model=model-a", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			_, _, _, err := InspectRequest(req)
			var decision *DecisionError
			if !errors.As(err, &decision) || decision.Code != CodeModelConflict {
				t.Fatalf("error = %v, want %s", err, CodeModelConflict)
			}
		})
	}
}

func TestInspectRequestFailsClosedWhenBodyModelCannotBeDetermined(t *testing.T) {
	for _, tt := range []struct {
		name, contentType, body string
	}{
		{"malformed json", "application/json", `{"messages":[]`},
		{"valid json without model", "application/json", `{"messages":[]}`},
		{"opaque text", "text/plain", `not a model-bearing request`},
		{"urlencoded without model", "application/x-www-form-urlencoded", `prompt=private`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions?model=model-a", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			if _, _, _, err := InspectRequest(req); err == nil {
				t.Fatal("query-only model accepted when inference body model was indeterminate")
			}
		})
	}
}

func TestInspectRequestAcceptsRawBodyModelWithMisleadingContentType(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"model-a"}trailing`))
	req.Header.Set("Content-Type", "text/plain")
	if model, _, _, err := InspectRequest(req); err == nil {
		t.Fatalf("malformed body accepted with model=%q", model)
	}
}

func TestInspectRequestRejectsDuplicateModelRegardlessOfContentTypeOrMalformedSuffix(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
	}{
		{"text plain", "text/plain", `{"model":"model-a","model":"forbidden"}`},
		{"trailing comma", "application/json", `{"model":"model-a","model":"forbidden",}`},
		{"trailing garbage", "application/json", `{"model":"model-a","model":"forbidden"}garbage`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			_, _, _, err := InspectRequest(req)
			var decision *DecisionError
			if !errors.As(err, &decision) || decision.Code != CodeModelConflict {
				t.Fatalf("error = %v, want %s", err, CodeModelConflict)
			}
		})
	}
}

func TestForbiddenModelVariantsAreRefusedAcrossGatewayRouteFamilies(t *testing.T) {
	cfg := config.CallerPoliciesConfig{
		StateFile: filepath.Join(t.TempDir(), "usage.json"),
		Callers: []config.CallerPolicyConfig{{
			ID: "caller-a", APIKey: "key-a", AllowedModels: []string{"model-a"}, MaxConcurrency: 1, DailyTokenCap: 100,
		}},
	}
	manager := New(cfg, "")
	t.Cleanup(func() { _ = manager.Close() })

	routes := []struct {
		name string
		path func(string) string
	}{
		{"openai chat", func(string) string { return "/v1/chat/completions" }},
		{"openai completions", func(string) string { return "/v1/completions" }},
		{"openai embeddings", func(string) string { return "/v1/embeddings" }},
		{"openai responses", func(string) string { return "/v1/responses" }},
		{"anthropic messages", func(string) string { return "/v1/messages" }},
		{"gemini path", func(model string) string { return "/v1beta/models/" + model + ":generateContent" }},
	}
	variants := []struct {
		name        string
		contentType string
		target      func(func(string) string) string
		body        string
	}{
		{"forbidden body", "application/json", func(path func(string) string) string { return path("forbidden") }, `{"model":"forbidden","messages":[]}`},
		{"query body conflict", "application/json", func(path func(string) string) string { return path("forbidden") + "?model=model-a" }, `{"model":"forbidden","messages":[]}`},
		{"duplicate key", "application/json", func(path func(string) string) string { return path("model-a") }, `{"model":"model-a","model":"forbidden","messages":[]}`},
		{"malformed", "application/json", func(path func(string) string) string { return path("forbidden") }, `{"model":"forbidden","messages":[],}`},
		{"misleading content type", "text/plain", func(path func(string) string) string { return path("forbidden") }, `{"model":"forbidden","messages":[]}`},
	}

	for _, route := range routes {
		for _, variant := range variants {
			t.Run(route.name+"/"+variant.name, func(t *testing.T) {
				req := httptest.NewRequest("POST", variant.target(route.path), strings.NewReader(variant.body))
				req.Header.Set("Content-Type", variant.contentType)
				model, _, _, inspectErr := InspectRequest(req)
				if inspectErr != nil {
					return
				}
				if err := manager.AuthorizeModel("caller-a", model); err == nil {
					t.Fatalf("forbidden variant resolved to %q and was allowed", model)
				}
			})
		}
	}
}

func TestInspectRequestRejectsBodyOverMemoryLimit(t *testing.T) {
	body := strings.NewReader(strings.Repeat("x", (16<<20)+1))
	req := httptest.NewRequest("POST", "/v1/chat/completions", body)
	if _, _, _, err := InspectRequest(req); err == nil || !strings.Contains(err.Error(), "request body too large") {
		t.Fatalf("oversized body error = %v", err)
	}
}

func TestRequiresExplicitModelDefaultsToDenyForUnknownRoutes(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/provider/openai/v1/chat/completions"},
		{"POST", "/future/inference"},
		{"GET", "/v1/ws"},
	} {
		if !RequiresExplicitModel(httptest.NewRequest(tc.method, tc.path, nil)) {
			t.Fatalf("%s %s unexpectedly allowed without a model", tc.method, tc.path)
		}
	}
}
