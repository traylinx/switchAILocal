// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/traylinx/switchAILocal/internal/config"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// TestIsMinimaxTTSRequest covers the three ways the adapter gets triggered:
// executor identifier contains "minimax", auth.Provider contains "minimax",
// or neither (fallthrough to dumb-proxy). Case-insensitive per the helper.
func TestIsMinimaxTTSRequest(t *testing.T) {
	cases := []struct {
		name     string
		execID   string
		auth     *switchailocalauth.Auth
		expected bool
	}{
		{"exec id minimax", "minimax", nil, true},
		{"exec id MiniMax (mixed case)", "MiniMax", nil, true},
		{"auth provider minimax", "other", &switchailocalauth.Auth{Provider: "minimax"}, true},
		{"auth provider Minimax-TP", "other", &switchailocalauth.Auth{Provider: "Minimax-TP"}, true},
		{"neither", "openai", &switchailocalauth.Auth{Provider: "openai"}, false},
		{"nil auth + non-minimax exec", "anthropic", nil, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := isMinimaxTTSRequest(tt.execID, tt.auth)
			if got != tt.expected {
				t.Errorf("isMinimaxTTSRequest(%q, %v) = %v, want %v", tt.execID, tt.auth, got, tt.expected)
			}
		})
	}
}

// TestTranslateOpenAITTSToMinimaxT2A verifies the OpenAI → MiniMax request
// shape translation. Table-driven; each case asserts both the fields that
// should appear and the ones that should not.
func TestTranslateOpenAITTSToMinimaxT2A(t *testing.T) {
	t.Run("minimal valid request", func(t *testing.T) {
		in := []byte(`{"model":"speech-02-hd","input":"hello","voice":"male-qn-qingse"}`)
		out, err := translateOpenAITTSToMinimaxT2A(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("output not valid JSON: %v (raw=%s)", err, out)
		}
		if got["model"] != "speech-02-hd" {
			t.Errorf("model=%v, want speech-02-hd", got["model"])
		}
		if got["text"] != "hello" {
			t.Errorf("text=%v, want hello (OpenAI 'input' should map to 'text')", got["text"])
		}
		if got["voice_id"] != "male-qn-qingse" {
			t.Errorf("voice_id=%v, want male-qn-qingse (OpenAI 'voice' should map to 'voice_id')", got["voice_id"])
		}
		// These OpenAI field names must NOT be present in the translated body.
		for _, forbidden := range []string{"input", "voice", "response_format"} {
			if _, exists := got[forbidden]; exists {
				t.Errorf("translated body should not contain %q (OpenAI-only field)", forbidden)
			}
		}
	})

	t.Run("full passthrough fields", func(t *testing.T) {
		in := []byte(`{"model":"speech-02-hd","input":"hi","voice":"v1","speed":1.25,"vol":0.8,"pitch":2,"audio_sample_rate":44100,"bitrate":192000,"channel":2,"response_format":"wav"}`)
		out, err := translateOpenAITTSToMinimaxT2A(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var got map[string]any
		_ = json.Unmarshal(out, &got)
		checks := map[string]any{
			"speed":             1.25,
			"vol":               0.8,
			"pitch":             float64(2),
			"audio_sample_rate": float64(44100),
			"bitrate":           float64(192000),
			"channel":           float64(2),
			"format":            "wav",
		}
		for k, want := range checks {
			if got[k] != want {
				t.Errorf("field %s = %v (%T), want %v (%T)", k, got[k], got[k], want, want)
			}
		}
	})

	t.Run("format field alias (client sent 'format' directly)", func(t *testing.T) {
		in := []byte(`{"model":"speech-02-hd","input":"hi","voice":"v1","format":"mp3"}`)
		out, _ := translateOpenAITTSToMinimaxT2A(in)
		if !strings.Contains(string(out), `"format":"mp3"`) {
			t.Errorf("expected format=mp3 in output, got %s", string(out))
		}
	})

	t.Run("missing model fails", func(t *testing.T) {
		_, err := translateOpenAITTSToMinimaxT2A([]byte(`{"input":"hi","voice":"v"}`))
		if err == nil || !strings.Contains(err.Error(), "missing model") {
			t.Errorf("expected 'missing model' error, got %v", err)
		}
	})

	t.Run("missing input fails", func(t *testing.T) {
		_, err := translateOpenAITTSToMinimaxT2A([]byte(`{"model":"speech-02-hd","voice":"v"}`))
		if err == nil || !strings.Contains(err.Error(), "missing input") {
			t.Errorf("expected 'missing input' error, got %v", err)
		}
	})

	t.Run("missing voice fails with helpful hint", func(t *testing.T) {
		_, err := translateOpenAITTSToMinimaxT2A([]byte(`{"model":"speech-02-hd","input":"hi"}`))
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "missing voice") || !strings.Contains(err.Error(), "voice_id") {
			t.Errorf("error should mention 'missing voice' and hint at MiniMax voice_id, got: %v", err)
		}
	})
}

// TestMinimaxStatusToHTTP pins the MiniMax base_resp.status_code → HTTP
// status translation so the failover taxonomy classifies each case the way
// we want: rate limit / out-of-credits → advance, invalid_params → abort.
func TestMinimaxStatusToHTTP(t *testing.T) {
	cases := []struct {
		minimaxCode int
		wantHTTP    int
		name        string
	}{
		{0, http.StatusOK, "success"},
		{1002, http.StatusTooManyRequests, "rate limit RPM → 429 (ClassRateLimit, advances)"},
		{1004, http.StatusUnauthorized, "invalid api key → 401 (ClassAuth, advances)"},
		{1008, http.StatusUnauthorized, "insufficient balance → 401 (ClassAuth, advances)"},
		{2013, http.StatusBadRequest, "invalid params → 400 (ClassPermanent, aborts)"},
		{2061, http.StatusPaymentRequired, "token plan not support → 402 (ClassOutOfCredits, advances)"},
		{9999, http.StatusBadGateway, "unknown code → 502 (ClassTransient, advances)"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := minimaxStatusToHTTP(tt.minimaxCode)
			if got != tt.wantHTTP {
				t.Errorf("minimaxStatusToHTTP(%d) = %d, want %d", tt.minimaxCode, got, tt.wantHTTP)
			}
		})
	}
}

// TestExecuteMinimaxTTS_HappyPath exercises the full round-trip using two
// stacked httptest servers: one impersonates MiniMax's /v1/t2a_pro endpoint,
// the other acts as the CDN serving the audio_file URL MiniMax hands back.
// Proves the adapter correctly translates, posts, parses, resolves, and
// returns audio bytes — without burning any real MiniMax RPM quota.
func TestExecuteMinimaxTTS_HappyPath(t *testing.T) {
	// Fake OSS/CDN that returns real MP3-shaped bytes.
	wantAudio := []byte{0xFF, 0xFB, 0x90, 0x44, 0x00, 0x00, 0x00, 0x00} // MP3 frame header
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(wantAudio)
	}))
	defer cdn.Close()

	// Fake MiniMax /v1/t2a_pro that validates the translated payload and
	// returns a pointer at the CDN above.
	var seenBody map[string]any
	minimax := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/t2a_pro") {
			t.Errorf("upstream path = %q, want suffix /t2a_pro", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"audio_file":    cdn.URL + "/signed-url.mp3",
			"subtitle_file": "",
			"trace_id":      "test-trace-123",
			"base_resp":     map[string]any{"status_code": 0, "status_msg": ""},
		})
	}))
	defer minimax.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:   "minimax:speech-02-hd",
		Payload: []byte(`{"model":"minimax:speech-02-hd","input":"hi there","voice":"male-qn-qingse","response_format":"mp3"}`),
		Metadata: map[string]any{
			"operation": "audio_speech",
		},
	}

	resp, err := exec.executeMinimaxTTS(context.Background(), nil, req, minimax.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("executeMinimaxTTS failed: %v", err)
	}
	if string(resp.Payload) != string(wantAudio) {
		t.Errorf("audio bytes mismatch: got %x, want %x", resp.Payload, wantAudio)
	}
	// Confirm the translation happened before it left the adapter.
	if seenBody["text"] != "hi there" {
		t.Errorf("upstream 'text' field = %v, want 'hi there' (OpenAI 'input' should be renamed)", seenBody["text"])
	}
	if seenBody["voice_id"] != "male-qn-qingse" {
		t.Errorf("upstream 'voice_id' = %v, want male-qn-qingse", seenBody["voice_id"])
	}
	if _, hasInput := seenBody["input"]; hasInput {
		t.Errorf("upstream body still has OpenAI 'input' field — translation didn't run")
	}
}

// TestExecuteMinimaxTTS_ErrorMapping feeds the adapter an upstream MiniMax
// error response and verifies the status code is mapped onto HTTP correctly
// so the failover taxonomy can classify it.
func TestExecuteMinimaxTTS_ErrorMapping(t *testing.T) {
	cases := []struct {
		name           string
		minimaxCode    int
		wantHTTPStatus int
	}{
		{"rate limit 1002 advances", 1002, http.StatusTooManyRequests},
		{"plan limit 2061 advances as 402", 2061, http.StatusPaymentRequired},
		{"invalid params 2013 aborts as 400", 2013, http.StatusBadRequest},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			minimax := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"audio_file": "",
					"base_resp":  map[string]any{"status_code": tt.minimaxCode, "status_msg": "simulated"},
				})
			}))
			defer minimax.Close()

			exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
			req := switchailocalexecutor.Request{
				Model:    "minimax:speech-02-hd",
				Payload:  []byte(`{"input":"hi","voice":"male-qn-qingse"}`),
				Metadata: map[string]any{"operation": "audio_speech"},
			}

			_, err := exec.executeMinimaxTTS(context.Background(), nil, req, minimax.URL+"/v1", "test-key")
			if err == nil {
				t.Fatalf("expected error from upstream code %d, got nil", tt.minimaxCode)
			}
			// Unwrap to the statusErr so failover taxonomy can read it.
			se, ok := err.(statusErr)
			if !ok {
				t.Fatalf("error is not statusErr: %T %v", err, err)
			}
			if se.code != tt.wantHTTPStatus {
				t.Errorf("HTTP code = %d, want %d (error=%s)", se.code, tt.wantHTTPStatus, se.msg)
			}
		})
	}
}

// TestTruncate is a trivial smoke test on the tiny helper — kept because the
// error-formatting branch relies on it not blowing up on edge cases.
func TestTruncate(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 5, "hello"},
		{"hello world", 5, "hello..."},
		{"", 10, ""},
	}
	for _, tt := range cases {
		if got := truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
