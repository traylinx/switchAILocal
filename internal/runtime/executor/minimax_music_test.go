// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/config"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// TestNormaliseMinimaxMusicRequest covers the defaulting behaviour so a
// client that sends no model at all still gets routed to music-2.6, and an
// alias ("minimax:music-2.6") is rewritten to the upstream-accepted name.
func TestNormaliseMinimaxMusicRequest(t *testing.T) {
	t.Run("empty body gets music-2.6 default", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"lyrics":"la la la"}`)}
		e := &OpenAICompatExecutor{cfg: &config.Config{}}
		out := e.normaliseMinimaxMusicRequest(req, nil)
		if gjson.GetBytes(out, "model").String() != "music-2.6" {
			t.Errorf("model=%q, want music-2.6 (default)", gjson.GetBytes(out, "model").String())
		}
		if gjson.GetBytes(out, "lyrics").String() != "la la la" {
			t.Errorf("lyrics not preserved: %s", out)
		}
	})

	t.Run("existing model preserved", func(t *testing.T) {
		req := switchailocalexecutor.Request{Payload: []byte(`{"model":"music-cover","prompt":"jazz cover"}`)}
		e := &OpenAICompatExecutor{cfg: &config.Config{}}
		out := e.normaliseMinimaxMusicRequest(req, nil)
		if gjson.GetBytes(out, "model").String() != "music-cover" {
			t.Errorf("model was overwritten: %s", gjson.GetBytes(out, "model").String())
		}
	})

	// Regression for v0.5.4: operator-defined aliases in the openai-compat
	// config (e.g. ail-music → music-2.6) must be resolved BEFORE the
	// request is posted to MiniMax. Prior versions forwarded the alias
	// verbatim and MiniMax rejected with "invalid model" (code 2013).
	t.Run("alias resolves via auth model map", func(t *testing.T) {
		cfg := &config.Config{
			OpenAICompatibility: []config.OpenAICompatibility{{
				Name: "minimax",
				Models: []config.OpenAICompatibilityModel{
					{Name: "music-2.6", Alias: "ail-music"},
					{Name: "music-cover", Alias: "ail-music-cover"},
				},
			}},
		}
		e := &OpenAICompatExecutor{provider: "minimax", cfg: cfg}
		auth := &switchailocalauth.Auth{Provider: "minimax"}
		req := switchailocalexecutor.Request{
			Model:   "ail-music",
			Payload: []byte(`{"model":"ail-music","lyrics":"test"}`),
		}
		out := e.normaliseMinimaxMusicRequest(req, auth)
		got := gjson.GetBytes(out, "model").String()
		if got != "music-2.6" {
			t.Errorf("model=%q, want music-2.6 (alias must be resolved before upstream post)", got)
		}
	})
}

func TestNormaliseMinimaxMusicRequest_RepairsNonSeekableCoverWav(t *testing.T) {
	wav := []byte{
		'R', 'I', 'F', 'F', 0xff, 0xff, 0xff, 0xff, 'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ', 16, 0, 0, 0, 1, 0, 1, 0,
		0x80, 0xbb, 0, 0, 0, 0x77, 1, 0, 2, 0, 16, 0,
		'd', 'a', 't', 'a', 0xff, 0xff, 0xff, 0xff,
		1, 2, 3, 4,
	}
	req := switchailocalexecutor.Request{Payload: []byte(`{"model":"music-cover","audio_base64":"` + base64.StdEncoding.EncodeToString(wav) + `"}`)}
	e := &OpenAICompatExecutor{cfg: &config.Config{}}
	out := e.normaliseMinimaxMusicRequest(req, nil)
	repaired, err := base64.StdEncoding.DecodeString(gjson.GetBytes(out, "audio_base64").String())
	if err != nil {
		t.Fatalf("audio_base64 not valid base64: %v", err)
	}
	if got := repaired[4:8]; string(got) == "\xff\xff\xff\xff" {
		t.Fatalf("RIFF size was not repaired")
	}
	if got := repaired[40:44]; string(got) == "\xff\xff\xff\xff" {
		t.Fatalf("data size was not repaired")
	}
	if got, want := binary.LittleEndian.Uint32(repaired[4:8]), uint32(len(repaired)-8); got != want {
		t.Fatalf("unexpected RIFF size: got=%d want=%d", got, want)
	}
	if got, want := binary.LittleEndian.Uint32(repaired[40:44]), uint32(len(repaired)-44); got != want {
		t.Fatalf("unexpected data size: got=%d want=%d", got, want)
	}
}

func TestNormaliseMinimaxMusicRequest_DoesNotLoopShortCoverWav(t *testing.T) {
	sampleRate := uint32(24_000)
	blockAlign := uint16(2)
	data := make([]byte, int(sampleRate)*int(blockAlign)*14)
	for i := 0; i < len(data); i += 2 {
		data[i] = byte(i / 2)
		data[i+1] = byte((i / 2) >> 8)
	}
	wav := encodeTestPCM16WAV(sampleRate, blockAlign, data)
	req := switchailocalexecutor.Request{Payload: []byte(`{"model":"music-cover","audio_base64":"` + base64.StdEncoding.EncodeToString(wav) + `"}`)}
	e := &OpenAICompatExecutor{cfg: &config.Config{}}
	out := e.normaliseMinimaxMusicRequest(req, nil)
	got, err := base64.StdEncoding.DecodeString(gjson.GetBytes(out, "audio_base64").String())
	if err != nil {
		t.Fatalf("audio_base64 not valid base64: %v", err)
	}
	if len(got) != len(wav) {
		t.Fatalf("short reference was mutated: got %d bytes want %d", len(got), len(wav))
	}
	sec, ok := minimaxCoverReferenceDurationSeconds(out)
	if !ok {
		t.Fatalf("duration should be parseable")
	}
	if sec < 13.9 || sec > 14.1 {
		t.Fatalf("duration=%.2fs, want about 14s", sec)
	}
}

func TestExecuteMinimaxMusic_RejectsShortCoverWavBeforeUpstream(t *testing.T) {
	sampleRate := uint32(24_000)
	blockAlign := uint16(2)
	data := make([]byte, int(sampleRate)*int(blockAlign)*14)
	wav := encodeTestPCM16WAV(sampleRate, blockAlign, data)
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("upstream should not be called for too-short cover reference")
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-cover",
		Payload:  []byte(`{"model":"minimax:music-cover","prompt":"lofi cover style","audio_base64":"` + base64.StdEncoding.EncodeToString(wav) + `"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}
	_, err := exec.executeMinimaxMusic(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err == nil {
		t.Fatalf("expected short reference error")
	}
	se, ok := err.(statusErr)
	if !ok {
		t.Fatalf("err type=%T, want statusErr", err)
	}
	if se.StatusCode() != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", se.StatusCode())
	}
	if !strings.Contains(se.Error(), "reference audio is 14.0s") {
		t.Fatalf("unexpected error: %v", se)
	}
	if called {
		t.Fatalf("upstream was called")
	}
}

func encodeTestPCM16WAV(sampleRate uint32, blockAlign uint16, data []byte) []byte {
	out := make([]byte, 0, 44+len(data))
	out = append(out, 'R', 'I', 'F', 'F')
	out = binary.LittleEndian.AppendUint32(out, uint32(36+len(data)))
	out = append(out, 'W', 'A', 'V', 'E')
	out = append(out, 'f', 'm', 't', ' ')
	out = binary.LittleEndian.AppendUint32(out, 16)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint16(out, 1)
	out = binary.LittleEndian.AppendUint32(out, sampleRate)
	out = binary.LittleEndian.AppendUint32(out, sampleRate*uint32(blockAlign))
	out = binary.LittleEndian.AppendUint16(out, blockAlign)
	out = binary.LittleEndian.AppendUint16(out, 16)
	out = append(out, 'd', 'a', 't', 'a')
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	out = append(out, data...)
	return out
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

func TestExecuteMinimaxMusic_CoverUsesPreprocessBeforeGeneration(t *testing.T) {
	wantAudio := []byte{0x49, 0x44, 0x33, 0x04, 0x11, 0x22}
	hexAudio := hex.EncodeToString(wantAudio)
	var musicCalls int
	var sawPreprocess bool
	var paths []string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.HasSuffix(r.URL.Path, "/music_cover_preprocess"):
			sawPreprocess = true
			if reqBody["model"] != "music-cover" {
				t.Errorf("preprocess model=%v, want music-cover", reqBody["model"])
			}
			if reqBody["audio_base64"] != "UklGRg==" {
				t.Errorf("preprocess audio_base64=%v, want UklGRg==", reqBody["audio_base64"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"cover_feature_id": "feature_123",
				"formatted_lyrics": "[Verse]\nextracted words\n[Chorus]\nextracted hook",
				"structure_result": "{}",
				"audio_duration":   60,
				"trace_id":         "preprocess-trace",
				"base_resp":        map[string]any{"status_code": 0, "status_msg": ""},
			})
		case strings.HasSuffix(r.URL.Path, "/music_generation"):
			musicCalls++
			if reqBody["audio_base64"] != nil || reqBody["audio_url"] != nil {
				t.Errorf("generation should use cover_feature_id instead of raw audio: %v", reqBody)
			}
			if reqBody["cover_feature_id"] != "feature_123" {
				t.Errorf("cover_feature_id=%v, want feature_123", reqBody["cover_feature_id"])
			}
			if reqBody["lyrics"] != "[Verse]\nextracted words\n[Chorus]\nextracted hook" {
				t.Errorf("lyrics=%q, want extracted preprocess lyrics", reqBody["lyrics"])
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"audio":  hexAudio,
					"status": 2,
				},
				"trace_id": "cover-retry-trace",
				"extra_info": map[string]any{
					"music_duration":    14000,
					"music_sample_rate": 44100,
					"music_channel":     2,
					"bitrate":           256000,
				},
				"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-cover",
		Payload:  []byte(`{"model":"minimax:music-cover","prompt":"lofi cover style","audio_base64":"UklGRg=="}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}

	resp, err := exec.executeMinimaxMusic(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("executeMinimaxMusic failed: %v", err)
	}
	if !sawPreprocess {
		t.Fatalf("expected /music_cover_preprocess")
	}
	if musicCalls != 1 {
		t.Fatalf("music calls=%d, want 1", musicCalls)
	}
	if len(paths) != 2 || !strings.HasSuffix(paths[0], "/music_cover_preprocess") || !strings.HasSuffix(paths[1], "/music_generation") {
		t.Fatalf("unexpected call order: %v", paths)
	}
	audioB64 := gjson.GetBytes(resp.Payload, "data.audio").String()
	decoded, err := base64.StdEncoding.DecodeString(audioB64)
	if err != nil {
		t.Fatalf("audio not valid base64: %v", err)
	}
	if string(decoded) != string(wantAudio) {
		t.Errorf("audio mismatch: got %x, want %x", decoded, wantAudio)
	}
}

func TestNormaliseMinimaxCoverLyrics_BlankTagsFallback(t *testing.T) {
	got := normaliseMinimaxCoverLyrics("[Verse]\n\n[Inst]")
	want := "[Verse]\nLa la la la\n[Chorus]\nLa la la la"
	if got != want {
		t.Fatalf("normaliseMinimaxCoverLyrics()=%q, want %q", got, want)
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

// sseWriter is a tiny helper for emitting event-stream frames in tests. It
// matches the wire format observed from MiniMax: `data: <json>\n\n`.
func sseWriter(t *testing.T, w http.ResponseWriter, payload map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	if _, err := w.Write([]byte("data: ")); err != nil {
		t.Errorf("sse write prefix: %v", err)
	}
	if _, err := w.Write(body); err != nil {
		t.Errorf("sse write body: %v", err)
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		t.Errorf("sse write sep: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

// TestExecuteMinimaxMusicStream_HappyPath verifies the full SSE → raw MP3
// pipeline: three progressive hex-encoded chunks land as raw bytes on the
// output channel, the terminal frame (status=2, duplicated audio) is
// discarded, and the channel closes cleanly with no error.
func TestExecuteMinimaxMusicStream_HappyPath(t *testing.T) {
	chunk1 := []byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x01, 0x02, 0x03} // ID3v2 header
	chunk2 := []byte{0xFF, 0xFB, 0xD0, 0x64, 0x10, 0x11, 0x12, 0x13} // MP3 frame sync
	chunk3 := []byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the handler injected stream:true before posting upstream.
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["stream"] != true {
			t.Errorf("upstream saw stream=%v, want true", reqBody["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": hex.EncodeToString(chunk1), "status": 1},
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
		})
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": hex.EncodeToString(chunk2), "status": 1},
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
		})
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": hex.EncodeToString(chunk3), "status": 1},
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
		})
		// Terminal frame: status=2, duplicated audio (full concat), extra_info.
		// Adapter MUST drop the audio and keep the metadata for logging only.
		sseWriter(t, w, map[string]any{
			"data":       map[string]any{"audio": hex.EncodeToString(append(append(append([]byte{}, chunk1...), chunk2...), chunk3...)), "status": 2},
			"trace_id":   "stream-trace-abc",
			"extra_info": map[string]any{"music_duration": 45000, "music_sample_rate": 44100, "music_channel": 2, "bitrate": 256000},
			"base_resp":  map[string]any{"status_code": 0, "status_msg": "success"},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"lyrics":"la la la"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}
	stream, err := exec.executeMinimaxMusicStream(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("executeMinimaxMusicStream returned err: %v", err)
	}

	var got [][]byte
	for chunk := range stream {
		if chunk.Err != nil {
			t.Fatalf("unexpected mid-stream error: %v", chunk.Err)
		}
		got = append(got, chunk.Payload)
	}
	if len(got) != 3 {
		t.Fatalf("got %d chunks, want 3 (terminal frame audio should be dropped); sizes=%v", len(got), chunkSizes(got))
	}
	if string(got[0]) != string(chunk1) || string(got[1]) != string(chunk2) || string(got[2]) != string(chunk3) {
		t.Errorf("chunk bytes did not match: got[0]=%x got[1]=%x got[2]=%x", got[0], got[1], got[2])
	}
}

func chunkSizes(chunks [][]byte) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}

// TestExecuteMinimaxMusicStream_PreFirstByteError verifies that an upstream
// application error in the FIRST SSE frame surfaces as a retryable
// statusErr (not a raw bytes emission). This is the "failover still works"
// contract — conductor needs to see the right HTTP code to advance.
func TestExecuteMinimaxMusicStream_PreFirstByteError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": "", "status": 1},
			"base_resp": map[string]any{"status_code": 2061, "status_msg": "plan not support"},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"lyrics":"test"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}
	stream, err := exec.executeMinimaxMusicStream(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("expected channel (pre-body error), got immediate err: %v", err)
	}
	var sawErr error
	var audioBytes int
	for chunk := range stream {
		if chunk.Err != nil {
			sawErr = chunk.Err
		}
		audioBytes += len(chunk.Payload)
	}
	if audioBytes != 0 {
		t.Errorf("got %d audio bytes before error — expected zero for pre-first-byte failure", audioBytes)
	}
	if sawErr == nil {
		t.Fatal("expected error on channel, got nil")
	}
	se, ok := sawErr.(statusErr)
	if !ok {
		t.Fatalf("error is %T, want statusErr (needed for failover classify): %v", sawErr, sawErr)
	}
	if se.code != http.StatusPaymentRequired {
		t.Errorf("HTTP code=%d, want 402 (maps from MiniMax 2061)", se.code)
	}
}

// TestExecuteMinimaxMusicStream_MidStreamError pins the "already committed"
// behaviour: client got audio bytes before upstream died, so the error goes
// on the channel but we don't retry — client has a partial, still-playable
// MP3 to work with.
func TestExecuteMinimaxMusicStream_MidStreamError(t *testing.T) {
	chunk1 := []byte{0x49, 0x44, 0x33, 0x04, 0x00}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": hex.EncodeToString(chunk1), "status": 1},
			"base_resp": map[string]any{"status_code": 0, "status_msg": ""},
		})
		sseWriter(t, w, map[string]any{
			"data":      map[string]any{"audio": "", "status": 1},
			"base_resp": map[string]any{"status_code": 1002, "status_msg": "rpm exceeded"},
		})
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"lyrics":"x"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}
	stream, err := exec.executeMinimaxMusicStream(context.Background(), nil, req, upstream.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("unexpected setup err: %v", err)
	}
	var sawErr error
	var audioBytes int
	for chunk := range stream {
		if chunk.Err != nil {
			sawErr = chunk.Err
		}
		audioBytes += len(chunk.Payload)
	}
	if audioBytes != len(chunk1) {
		t.Errorf("got %d audio bytes, want %d (one progressive frame before the error)", audioBytes, len(chunk1))
	}
	if sawErr == nil {
		t.Fatal("expected error on channel after partial audio, got nil")
	}
}

// TestExecuteMinimaxMusicStream_HTTPError pins that a transport-level 4xx/5xx
// surfaces synchronously (before a channel is returned) so the conductor's
// pre-stream failover path picks it up.
func TestExecuteMinimaxMusicStream_HTTPError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer upstream.Close()

	exec := &OpenAICompatExecutor{provider: "minimax", cfg: &config.Config{}}
	req := switchailocalexecutor.Request{
		Model:    "minimax:music-2.6",
		Payload:  []byte(`{"lyrics":"x"}`),
		Metadata: map[string]any{"operation": "music_generation"},
	}
	_, err := exec.executeMinimaxMusicStream(context.Background(), nil, req, upstream.URL+"/v1", "bad-key")
	if err == nil {
		t.Fatal("expected synchronous error for HTTP 401, got nil")
	}
	se, ok := err.(statusErr)
	if !ok {
		t.Fatalf("error is %T, want statusErr: %v", err, err)
	}
	if se.code != http.StatusUnauthorized {
		t.Errorf("HTTP code=%d, want 401", se.code)
	}
}

// TestNormaliseMinimaxMusicRequest_StripsStream pins the safety fence: if a
// client sends stream:true to the SYNC path (before the stream-aware
// handler was wired), we strip it so the sync JSON parser doesn't choke on
// SSE bytes. The streaming path re-injects it explicitly.
func TestNormaliseMinimaxMusicRequest_StripsStream(t *testing.T) {
	req := switchailocalexecutor.Request{Payload: []byte(`{"model":"music-2.6","stream":true,"lyrics":"x"}`)}
	e := &OpenAICompatExecutor{cfg: &config.Config{}}
	out := e.normaliseMinimaxMusicRequest(req, nil)
	if gjson.GetBytes(out, "stream").Exists() {
		t.Errorf("stream should be stripped for sync path: %s", out)
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
