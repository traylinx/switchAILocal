// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package callerpolicy

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

func TestInspectRequestJSONRestoresBody(t *testing.T) {
	body := []byte(`{"model":"model-a","messages":[{"role":"user","content":"private prompt"}]}`)
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	model, estimate, err := InspectRequest(req)
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

func TestInspectRequestGeminiPathModel(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1beta/models/model-a:generateContent", bytes.NewBufferString(`{"contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	model, _, err := InspectRequest(req)
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
	model, estimate, err := InspectRequest(req)
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
	for _, path := range []string{"/v1/models/refresh", "/v1/notifications/telegram/sendMessage", "/health"} {
		if RequiresExplicitModel(httptest.NewRequest("POST", path, nil)) {
			t.Fatalf("non-inference route %q unexpectedly required a model", path)
		}
	}
}
