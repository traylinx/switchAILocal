// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/config"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// TestNormaliseMinimaxMusicRequest covers the defaulting behaviour so a
// client that sends no model at all still gets routed to music-2.6, and an
// alias ("minimax:music-2.6") is rewritten to the upstream-accepted name.
func TestNormaliseMinimaxMusicRequest(t *testing.T) {
	t.Run("empty body gets music-2.6 default", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"lyrics":"la la la"}`)}
		out := normaliseMinimaxMusicRequest(req)
		if gjson.GetBytes(out, "model").String() != "music-2.6" {
			t.Errorf("model=%q, want music-2.6 (default)", gjson.GetBytes(out, "model").String())
		}
		if gjson.GetBytes(out, "lyrics").String() != "la la la" {
			t.Errorf("lyrics not preserved: %s", out)
		}
	})

	t.Run("existing model preserved", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"model":"music-cover","prompt":"jazz cover"}`)}
		out := normaliseMinimaxMusicRequest(req)
		if gjson.GetBytes(out, "model").String() != "music-cover" {
			t.Errorf("model was overwritten: %s", gjson.GetBytes(out, "model").String())
		}
	})
}

// TestNormaliseMinimaxLyricsRequest pins the mode-defaulting and
// model-stripping behaviour. Upstream rejects requests without `mode`, so
// the adapter has to fill one in; and upstream ignores `model` but logs
// noisily if we send one, so we strip it.
func TestNormaliseMinimaxLyricsRequest(t *testing.T) {
	t.Run("missing mode defaults to write_full_song", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"prompt":"sunset"}`)}
		out := normaliseMinimaxLyricsRequest(req)
		if gjson.GetBytes(out, "mode").String() != "write_full_song" {
			t.Errorf("mode=%q, want write_full_song", gjson.GetBytes(out, "mode").String())
		}
	})

	t.Run("existing mode preserved", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"mode":"edit","lyrics":"[Verse]\nhello world"}`)}
		out := normaliseMinimaxLyricsRequest(req)
		if gjson.GetBytes(out, "mode").String() != "edit" {
			t.Errorf("mode was overwritten: %s", gjson.GetBytes(out, "mode").String())
		}
	})

	t.Run("model stripped (upstream ignores it)", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"model":"minimax:music-2.6","prompt":"rock"}`)}
		out := normaliseMinimaxLyricsRequest(req)
		if gjson.GetBytes(out, "model").Exists() {
			t.Errorf("model field should have been stripped: %s", out)
		}
	})

	t.Run("empty body tolerated", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: nil}
		out := normaliseMinimaxLyricsRequest(req)
		if gjson.GetBytes(out, "mode").String() != "write_full_song" {
			t.Errorf("mode not defaulted on nil body: %s", out)
		}
	})
}

// TestExecuteMinimaxMusic_HappyPath is the end-to-end integration proof:
// hex-encoded audio in the upstream response → base64 audio in the client
// response, with metadata (duration_ms, sample_rate, channels, bitrate)
// lifted from extra_info. No real MiniMax quota burned.
func TestExecuteMinimaxMusic_HappyPath(t *testing.T) {
	wantAudio := []byte{0x49, 0x44, 0x33, 0x04, 0x00, 0xAA, 0xBB, 0xCC} // "ID3\x04..." MP3 with ID3v2 tag
	hexAudio := hex.EncodeToString(wantAudio)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/music_generation") {
			t.Errorf("path = %q, want suffix /music_generation", r.URL.Path)
		}
		// Verify the model defaulting ran.
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "music-2.6" {
			t.Errorf("upstream saw model=%v, want music-2.6", reqBody["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"audio":  hexAudio,
				"status": 2,
			},
			"trace_id": "music-trace-123",
			"extra_info": map[string]any{
				"music_duration":    17606,
				"music_sample_rate": 44100,
				"music_channel":     2,
				"bitrate":           256000,
				"music_size":        len(wantAudio),
			},
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"lyrics":"la la la la la la la la la la la la"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}

	resp, err := exec.executeMinimaxMusic(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("executeMinimaxMusic failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(resp.Payload, &got); err != nil {
		t.Fatalf("response not valid JSON: %v (raw=%s)", err, resp.Payload)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data field missing or wrong shape: %v", got)
	}
	// Hex from upstream → base64 to client.
	audioB64, _ := data["audio"].(string)
	decoded, err := base64.StdEncoding.DecodeString(audioB64)
	if err != nil {
		t.Fatalf("audio not valid base64: %v", err)
	}
	if string(decoded) != string(wantAudio) {
		t.Errorf("audio mismatch: got %x, want %x", decoded, wantAudio)
	}
	if data["format"] != "mp3" {
		t.Errorf("format=%v, want mp3", data["format"])
	}
	// Duration, sample rate, channels lifted into data.
	if int(data["duration_ms"].(float64)) != 17606 {
		t.Errorf("duration_ms=%v, want 17606", data["duration_ms"])
	}
	if int(data["sample_rate"].(float64)) != 44100 {
		t.Errorf("sample_rate=%v, want 44100", data["sample_rate"])
	}
	if int(data["channels"].(float64)) != 2 {
		t.Errorf("channels=%v, want 2", data["channels"])
	}
	if got["model"] != "music-2.6" {
		t.Errorf("model in response = %v, want music-2.6", got["model"])
	}
	if got["trace_id"] != "music-trace-123" {
		t.Errorf("trace_id = %v, want music-trace-123", got["trace_id"])
	}
}

// TestExecuteMinimaxMusic_ErrorCodes verifies MiniMax application-level
// error codes get translated onto HTTP status so the failover taxonomy can
// classify them correctly (reuses the TTS mapping table).
func TestExecuteMinimaxMusic_ErrorCodes(t *testing.T) {
	cases := []struct {
		name           string
		minimaxCode    int
		wantHTTPStatus int
	}{
		{"rate_limit 1002", 1002, http.StatusTooManyRequests},
		{"plan_limit 2061", 2061, http.StatusPaymentRequired},
		{"invalid_params 2013", 2013, http.StatusBadRequest},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"base_resp": map[string]any{"status_code": tt.minimaxCode, "status_msg": "simulated"},
				})
			}))
			defer upstream.Close()

			exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
			req := switchailocalexecutor.Request{
				Model:    "minimax:music-2.6",
				Payload:  []byte(`{"lyrics":"la"}`),
				Metadata: map[string]any{"operation": "music_generation"},
			}
			_, err := exec.executeMinimaxMusic(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
			if err == nil {
				t.Fatalf("expected error for code %d, got nil", tt.minimaxCode)
			}
			se, ok := err.(statusErr)
			if !ok {
				t.Fatalf("error is not statusErr: %T %v", err, err)
			}
			if se.code != tt.wantHTTPStatus {
				t.Errorf("HTTP code=%d, want %d (msg=%s)", se.code, tt.wantHTTPStatus, se.msg)
			}
		})
	}
}

// TestExecuteMinimaxLyrics_HappyPath verifies the lyrics adapter
// pass-through: body goes up, body comes back. The mode-defaulting ran
// because the client didn't send one.
func TestExecuteMinimaxLyrics_HappyPath(t *testing.T) {
	var seenBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/lyrics_generation") {
			t.Errorf("path = %q, want suffix /lyrics_generation", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&seenBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"song_title": "Test Song",
			"style_tags": "pop, upbeat",
			"lyrics":     "[Verse]\nhello world\n[Chorus]\noh yeah",
			"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"prompt":"a cheerful pop song"}`),
		Metadata: map[string]any{"operation": "lyrics_generation"},
	}
	resp, err := exec.executeMinimaxLyrics(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("executeMinimaxLyrics failed: %v", err)
	}
	// Mode defaulting ran.
	if seenBody["mode"] != "write_full_song" {
		t.Errorf("upstream saw mode=%v, want write_full_song (default)", seenBody["mode"])
	}
	// Model was stripped.
	if _, hasModel := seenBody["model"]; hasModel {
		t.Errorf("model field should have been stripped by adapter, upstream saw: %v", seenBody["model"])
	}
	// Response body returned as-is.
	var got map[string]any
	_ = json.Unmarshal(resp.Payload, &got)
	if got["song_title"] != "Test Song" || got["style_tags"] != "pop, upbeat" {
		t.Errorf("response lost fields: %v", got)
	}
	if !strings.Contains(got["lyrics"].(string), "[Chorus]") {
		t.Errorf("lyrics body not forwarded: %v", got["lyrics"])
	}
}

// TestExecuteMinimaxLyrics_ErrorMapping — same code table, different
// endpoint. Pins the same HTTP-status translation is applied.
func TestExecuteMinimaxLyrics_ErrorMapping(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"base_resp": map[string]any{"status_code": 2061, "status_msg": "plan no support"},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"prompt":"test"}`),
		Metadata: map[string]any{"operation": "lyrics_generation"},
	}
	_, err := exec.executeMinimaxLyrics(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	se, ok := err.(statusErr)
	if !ok || se.code != http.StatusPaymentRequired {
		t.Errorf("wanted 402 statusErr, got %T %v", err, err)
	}
}
