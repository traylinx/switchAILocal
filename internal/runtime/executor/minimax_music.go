// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
	"unicode"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/traylinx/switchAILocal/internal/util"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// MiniMax music generation legitimately needs ~20s server-side before the
// first hex chunk arrives (verified 2026-04-18: TTFB 19.5s on a 45s song).
// The global Streaming.FirstByteTimeout default of 15s would kill every
// music request before it could start. Use these minimums instead so the
// watchdog still fires on genuinely-dead upstreams but doesn't cancel the
// happy path. Operators who set explicit higher values in config win.
const (
	minimaxMusicFirstByteMin = 60 * time.Second  // ~3× observed TTFB
	minimaxMusicStallMin     = 120 * time.Second // inter-chunk gap observed up to ~8s; generous headroom

	// Wire-tested 2026-05-11: MiniMax documents 6s as the minimum cover
	// reference length, but real music-cover generation can still fail with
	// "Cover mode requires dtw_result, beat_result, and audio_duration" for
	// short ~14s references. Do not loop short snippets: repeated audio
	// preprocesses successfully but generation still fails. Reject short WAV
	// references before burning cover quota and tell the caller to send a real
	// representative window (JULI3TA targets ~60s).
	minimaxMusicCoverReferenceMinSeconds = 30

	// MiniMax occasionally closes long-running JSON music connections with
	// a bare `unexpected EOF` after accepting the request. The generic
	// conductor sees that as status=0 and cannot retry a single-provider
	// chain, so the MiniMax adapter owns a small transport retry around the
	// actual POST. Keep this low: music requests are expensive and can take
	// minutes, but one retry is better than surfacing a raw gateway 500.
	minimaxJSONMaxAttempts = 2
)

// MiniMax exposes three subscription capabilities under a weird topology:
//
//   /v1/music_generation    — used for BOTH music-2.6 AND music-cover (model field switches mode)
//   /v1/lyrics_generation   — lyrics-only, no audio
//
// There is NO separate /v1/music_cover endpoint despite the token-plan billing
// showing music-cover as a distinct line item. The billing is tied to the
// model name, not the path.
//
// Response quirks that this adapter has to swallow so clients don't care:
//   * audio comes back as a HEX string (not base64), 1–3 MB for 20–60s clips
//   * HTTP is always 200 even on application errors — real status is in
//     base_resp.status_code (shared with TTS, reuse minimaxStatusToHTTP)
//   * extra_info carries useful metadata (duration, sample rate, bitrate,
//     channel count) that clients need but MP3 headers don't expose cleanly
//
// Exposed under the /v1/music/* namespace (see openai_handlers.go). The
// response shape mirrors OpenAI's images/generations for familiarity:
//
//   { data: { audio: "<base64>", duration_ms, sample_rate, channels, bitrate,
//             size_bytes, format: "mp3" },
//     model: "music-2.6",
//     trace_id: "...",
//     extra_info: <passthrough of upstream extra_info>  }
//
// Base64 instead of hex because OpenAI clients already understand it (images,
// audio/speech with response_format=b64_json). Clients that want raw bytes
// call hex → this → base64.StdEncoding.DecodeString.

const (
	minimaxMusicEndpoint                = "/music_generation"
	minimaxMusicCoverPreprocessEndpoint = "/music_cover_preprocess"
	minimaxLyricsEndpoint               = "/lyrics_generation"
)

// minimaxMusicResponse mirrors the upstream /v1/music_generation shape. Only
// the fields we forward are kept — upstream returns several more that are
// either redundant (trace_id duplicates) or debug noise.
type minimaxMusicResponse struct {
	Data *struct {
		Audio  string `json:"audio"`  // hex-encoded bytes
		Status int    `json:"status"` // 1=in-progress, 2=completed (always 2 for sync)
	} `json:"data"`
	TraceID   string          `json:"trace_id"`
	ExtraInfo json.RawMessage `json:"extra_info"` // passthrough — duration_ms, sample_rate, etc.
	BaseResp  struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// minimaxMusicCoverPreprocessResponse mirrors /v1/music_cover_preprocess.
// MiniMax added this two-step cover workflow after the original music adapter
// shipped. Some one-step cover calls now fail internally with
// "Cover mode requires dtw_result, beat_result, and audio_duration"; when that
// happens we preprocess the same reference audio, then retry /music_generation
// with cover_feature_id.
type minimaxMusicCoverPreprocessResponse struct {
	CoverFeatureID  string          `json:"cover_feature_id"`
	FormattedLyrics string          `json:"formatted_lyrics"`
	StructureResult string          `json:"structure_result"`
	AudioDuration   float64         `json:"audio_duration"`
	TraceID         string          `json:"trace_id"`
	DTWResult       json.RawMessage `json:"dtw_result"`
	BeatResult      json.RawMessage `json:"beat_result"`
	BaseResp        struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// minimaxLyricsResponse mirrors /v1/lyrics_generation. This endpoint returns
// pure JSON — no binary, no hex decoding needed.
type minimaxLyricsResponse struct {
	SongTitle string `json:"song_title"`
	StyleTags string `json:"style_tags"`
	Lyrics    string `json:"lyrics"`
	BaseResp  struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// executeMinimaxMusic posts to /v1/music_generation and normalises the
// response. Works for both music-2.6 (pure text-to-music) and music-cover
// (reference-audio style transfer) — the model field determines which path
// MiniMax runs internally.
//
// Sync only for now. Streaming (stream: true) is a future extension and
// requires a different execution path — the gateway's streaming layer
// already knows how to pipe SSE, but music streaming sends hex chunks that
// the client would have to concatenate + decode themselves.
func (e *OpenAICompatExecutor) executeMinimaxMusic(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (switchailocalexecutor.Response, error) {
	payload := e.normaliseMinimaxMusicRequest(req, auth)
	if err := validateMinimaxCoverReference(payload); err != nil {
		return switchailocalexecutor.Response{}, err
	}

	var body []byte
	var err error
	if shouldPreprocessMinimaxCover(payload) {
		// MiniMax's one-step cover path is now unreliable for references that
		// need feature analysis; it can fail with an internal DTW/beat error.
		// Follow the documented two-step flow up front: preprocess the reference
		// audio, then call /music_generation with cover_feature_id + lyrics.
		retryPayload, retryErr := e.prepareMinimaxCoverFeatureRetry(ctx, auth, baseURL, apiKey, payload)
		if retryErr != nil {
			return switchailocalexecutor.Response{}, retryErr
		}
		body, err = e.postMinimaxJSON(ctx, auth, baseURL, minimaxMusicEndpoint, apiKey, retryPayload)
		if err != nil {
			return switchailocalexecutor.Response{}, err
		}
		payload = retryPayload
	} else {
		body, err = e.postMinimaxJSON(ctx, auth, baseURL, minimaxMusicEndpoint, apiKey, payload)
		if err != nil {
			return switchailocalexecutor.Response{}, err
		}
	}

	var mr minimaxMusicResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: parse response: %w (body=%s)", err, truncate(string(body), 200))
	}

	if shouldRetryMinimaxCoverWithPreprocess(payload, mr.BaseResp.StatusCode, mr.BaseResp.StatusMsg) {
		retryPayload, err := e.prepareMinimaxCoverFeatureRetry(ctx, auth, baseURL, apiKey, payload)
		if err != nil {
			return switchailocalexecutor.Response{}, err
		}
		body, err = e.postMinimaxJSON(ctx, auth, baseURL, minimaxMusicEndpoint, apiKey, retryPayload)
		if err != nil {
			return switchailocalexecutor.Response{}, err
		}
		mr = minimaxMusicResponse{}
		if err := json.Unmarshal(body, &mr); err != nil {
			return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: parse cover retry response: %w (body=%s)", err, truncate(string(body), 200))
		}
		payload = retryPayload
	}

	if mr.BaseResp.StatusCode != 0 {
		httpCode, _ := minimaxStatusToHTTP(mr.BaseResp.StatusCode)
		return switchailocalexecutor.Response{}, statusErr{
			code: httpCode,
			msg:  fmt.Sprintf("minimax music: code=%d msg=%q trace=%s", mr.BaseResp.StatusCode, mr.BaseResp.StatusMsg, mr.TraceID),
		}
	}

	if mr.Data == nil || mr.Data.Audio == "" {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadGateway, msg: fmt.Sprintf("minimax music: empty audio payload (trace=%s)", mr.TraceID)}
	}

	audioValue := mr.Data.Audio
	audioFormat := "mp3"
	sizeBytes := 0
	if strings.HasPrefix(strings.ToLower(audioValue), "http://") || strings.HasPrefix(strings.ToLower(audioValue), "https://") {
		audioFormat = "url"
	} else {
		audioBytes, err := hex.DecodeString(audioValue)
		if err != nil {
			return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: hex decode audio: %w", err)
		}
		audioValue = base64.StdEncoding.EncodeToString(audioBytes)
		sizeBytes = len(audioBytes)
	}

	// Shape the response for clients. Base64 is friendlier than hex for
	// JSON transport and OpenAI clients already parse it. If the caller
	// explicitly asks MiniMax for output_format:"url", preserve that URL
	// instead of trying to hex-decode it.
	// Report the upstream model name (post-normalisation) so clients see what
	// MiniMax actually ran, not the alias they sent in.
	data := map[string]any{
		"audio":  audioValue,
		"format": audioFormat,
	}
	if sizeBytes > 0 {
		data["size_bytes"] = sizeBytes
	}
	if audioFormat == "url" {
		data["audio_url"] = audioValue
	}
	out := map[string]any{
		"data":     data,
		"model":    gjson.GetBytes(payload, "model").String(),
		"trace_id": mr.TraceID,
	}
	// Pluck standard metadata fields into top-level response fields while
	// also passing through the full upstream blob for forwards-compat.
	if len(mr.ExtraInfo) > 0 {
		var extra map[string]any
		if err := json.Unmarshal(mr.ExtraInfo, &extra); err == nil {
			if v, ok := extra["music_duration"]; ok {
				out["data"].(map[string]any)["duration_ms"] = v
			}
			if v, ok := extra["music_sample_rate"]; ok {
				out["data"].(map[string]any)["sample_rate"] = v
			}
			if v, ok := extra["music_channel"]; ok {
				out["data"].(map[string]any)["channels"] = v
			}
			if v, ok := extra["bitrate"]; ok {
				out["data"].(map[string]any)["bitrate"] = v
			}
			out["extra_info"] = extra
		}
	}

	respBody, err := json.Marshal(out)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: encode response: %w", err)
	}
	log.Infof("MINIMAX MUSIC: success trace=%s format=%s size=%d upstream_audio_len=%d", mr.TraceID, audioFormat, sizeBytes, len(mr.Data.Audio))
	return switchailocalexecutor.Response{Payload: respBody}, nil
}

func shouldPreprocessMinimaxCover(payload []byte) bool {
	model := strings.ToLower(gjson.GetBytes(payload, "model").String())
	if !strings.Contains(model, "music-cover") {
		return false
	}
	if gjson.GetBytes(payload, "cover_feature_id").Exists() {
		return false
	}
	return gjson.GetBytes(payload, "audio_base64").Exists() || gjson.GetBytes(payload, "audio_url").Exists()
}

func shouldRetryMinimaxCoverWithPreprocess(payload []byte, statusCode int, statusMsg string) bool {
	if statusCode != 2013 || !shouldPreprocessMinimaxCover(payload) {
		return false
	}
	msg := strings.ToLower(statusMsg)
	return strings.Contains(msg, "dtw_result") ||
		strings.Contains(msg, "beat_result") ||
		(strings.Contains(msg, "cover mode") && strings.Contains(msg, "requires"))
}

func validateMinimaxCoverReference(payload []byte) error {
	if !shouldPreprocessMinimaxCover(payload) {
		return nil
	}
	durationSec, ok := minimaxCoverReferenceDurationSeconds(payload)
	if !ok || durationSec >= minimaxMusicCoverReferenceMinSeconds {
		return nil
	}
	return statusErr{
		code: http.StatusBadRequest,
		msg: fmt.Sprintf(
			"minimax music-cover: reference audio is %.1fs; MiniMax cover generation fails on short references. Send at least %ds, ideally a representative ~60s sample.",
			durationSec,
			minimaxMusicCoverReferenceMinSeconds,
		),
	}
}

func minimaxCoverReferenceDurationSeconds(payload []byte) (float64, bool) {
	audio := gjson.GetBytes(payload, "audio_base64")
	if !audio.Exists() || strings.TrimSpace(audio.String()) == "" {
		return 0, false
	}
	decoded, err := base64.StdEncoding.DecodeString(audio.String())
	if err != nil {
		return 0, false
	}
	decoded, _ = repairUnknownSizeWAVHeader(decoded)
	ref, ok := parsePCM16WAVReference(decoded)
	if !ok {
		return 0, false
	}
	bytesPerSecond := float64(ref.sampleRate) * float64(ref.blockAlign)
	if bytesPerSecond <= 0 {
		return 0, false
	}
	return float64(len(ref.data)) / bytesPerSecond, true
}

func (e *OpenAICompatExecutor) prepareMinimaxCoverFeatureRetry(ctx context.Context, auth *switchailocalauth.Auth, baseURL, apiKey string, payload []byte) ([]byte, error) {
	preprocessPayload := []byte(`{"model":"music-cover"}`)
	if audioBase64 := gjson.GetBytes(payload, "audio_base64"); audioBase64.Exists() && audioBase64.String() != "" {
		preprocessPayload, _ = sjson.SetBytes(preprocessPayload, "audio_base64", audioBase64.String())
	} else if audioURL := gjson.GetBytes(payload, "audio_url"); audioURL.Exists() && audioURL.String() != "" {
		preprocessPayload, _ = sjson.SetBytes(preprocessPayload, "audio_url", audioURL.String())
	} else {
		return nil, statusErr{code: http.StatusBadRequest, msg: "minimax music-cover retry: missing audio_base64 or audio_url"}
	}

	log.Infof("MINIMAX MUSIC: one-step cover failed; retrying via music_cover_preprocess")
	body, err := e.postMinimaxJSON(ctx, auth, baseURL, minimaxMusicCoverPreprocessEndpoint, apiKey, preprocessPayload)
	if err != nil {
		return nil, err
	}

	var pr minimaxMusicCoverPreprocessResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, fmt.Errorf("minimax-music-cover-preprocess: parse response: %w (body=%s)", err, truncate(string(body), 200))
	}
	if pr.BaseResp.StatusCode != 0 {
		httpCode, _ := minimaxStatusToHTTP(pr.BaseResp.StatusCode)
		return nil, statusErr{
			code: httpCode,
			msg:  fmt.Sprintf("minimax music-cover-preprocess: code=%d msg=%q trace=%s", pr.BaseResp.StatusCode, pr.BaseResp.StatusMsg, pr.TraceID),
		}
	}
	if pr.CoverFeatureID == "" {
		return nil, statusErr{code: http.StatusBadGateway, msg: fmt.Sprintf("minimax music-cover-preprocess: empty cover_feature_id (trace=%s)", pr.TraceID)}
	}
	log.Infof(
		"MINIMAX MUSIC: cover preprocess ok trace=%s feature=%s duration=%.2f has_dtw=%t has_beat=%t lyrics_len=%d",
		pr.TraceID,
		pr.CoverFeatureID,
		pr.AudioDuration,
		len(pr.DTWResult) > 0 && string(pr.DTWResult) != "null",
		len(pr.BeatResult) > 0 && string(pr.BeatResult) != "null",
		len(strings.TrimSpace(pr.FormattedLyrics)),
	)

	retry := payload
	retry, _ = sjson.DeleteBytes(retry, "audio_base64")
	retry, _ = sjson.DeleteBytes(retry, "audio_url")
	retry, _ = sjson.SetBytes(retry, "cover_feature_id", pr.CoverFeatureID)
	if len(pr.DTWResult) > 0 && string(pr.DTWResult) != "null" {
		retry, _ = sjson.SetRawBytes(retry, "dtw_result", pr.DTWResult)
	}
	if len(pr.BeatResult) > 0 && string(pr.BeatResult) != "null" {
		retry, _ = sjson.SetRawBytes(retry, "beat_result", pr.BeatResult)
	}
	if pr.AudioDuration > 0 {
		retry, _ = sjson.SetBytes(retry, "audio_duration", pr.AudioDuration)
	}

	lyrics := strings.TrimSpace(gjson.GetBytes(retry, "lyrics").String())
	if len(lyrics) < 10 {
		retry, _ = sjson.SetBytes(retry, "lyrics", normaliseMinimaxCoverLyrics(pr.FormattedLyrics))
	}
	return retry, nil
}

func normaliseMinimaxCoverLyrics(lyrics string) string {
	clean := strings.TrimSpace(lyrics)
	if len(clean) < 10 || !hasMeaningfulLyrics(clean) {
		return "[Verse]\nLa la la la\n[Chorus]\nLa la la la"
	}
	if len(clean) <= 1000 {
		return clean
	}
	trimmed := strings.TrimSpace(clean[:1000])
	if idx := strings.LastIndex(trimmed, "\n["); idx >= 200 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	if len(trimmed) < 10 {
		return strings.TrimSpace(clean[:1000])
	}
	return trimmed
}

func hasMeaningfulLyrics(lyrics string) bool {
	withoutTags := make([]rune, 0, len(lyrics))
	inTag := false
	for _, r := range lyrics {
		switch r {
		case '[':
			inTag = true
			continue
		case ']':
			inTag = false
			continue
		}
		if inTag {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			withoutTags = append(withoutTags, r)
		}
	}
	return len(withoutTags) >= 4
}

// executeMinimaxLyrics posts to /v1/lyrics_generation. This path is the
// simplest of the three: no binary, no hex, no model field (the upstream
// ignores model and uses its own internal lyrics model). We just validate
// the mode field, forward, and return the upstream JSON nearly as-is.
func (e *OpenAICompatExecutor) executeMinimaxLyrics(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (switchailocalexecutor.Response, error) {
	payload := normaliseMinimaxLyricsRequest(req)

	body, err := e.postMinimaxJSON(ctx, auth, baseURL, minimaxLyricsEndpoint, apiKey, payload)
	if err != nil {
		return switchailocalexecutor.Response{}, err
	}

	var lr minimaxLyricsResponse
	if err := json.Unmarshal(body, &lr); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-lyrics: parse response: %w (body=%s)", err, truncate(string(body), 200))
	}

	if lr.BaseResp.StatusCode != 0 {
		httpCode, _ := minimaxStatusToHTTP(lr.BaseResp.StatusCode)
		return switchailocalexecutor.Response{}, statusErr{
			code: httpCode,
			msg:  fmt.Sprintf("minimax lyrics: code=%d msg=%q", lr.BaseResp.StatusCode, lr.BaseResp.StatusMsg),
		}
	}

	// Upstream response is already client-friendly JSON. Return as-is.
	log.Infof("MINIMAX LYRICS: success title=%q style=%q lyrics_len=%d", lr.SongTitle, lr.StyleTags, len(lr.Lyrics))
	return switchailocalexecutor.Response{Payload: body}, nil
}

// normaliseMinimaxMusicRequest injects the resolved upstream model name and
// strips fields MiniMax would reject. Called before the POST so the request
// body is what upstream actually expects regardless of what alias the client
// used ("ail-music", "minimax:music-2.6", "music-2.6", etc).
//
// Resolution order, matching executeMinimaxTTS so alias handling is
// consistent across all MiniMax-native paths:
//  1. util.ResolveOriginalModel via metadata (production path; empty in tests).
//  2. e.resolveUpstreamModel(req.Model, auth) — the config-driven alias map
//     (e.g. ail-music → music-2.6). Without this step, operator-defined
//     aliases in openai-compatibility.models reach MiniMax verbatim and
//     upstream rejects with "invalid model" (code 2013).
//  3. Strip "minimax:" prefix from the current model field (fallback for
//     clients that sent the raw name directly in the body).
//  4. Default to "music-2.6" if the field is missing entirely.
func (e *OpenAICompatExecutor) normaliseMinimaxMusicRequest(req switchailocalexecutor.Request, auth *switchailocalauth.Auth) []byte {
	payload := req.Payload
	upstream := util.ResolveOriginalModel(req.Model, req.Metadata)
	if override := e.resolveUpstreamModel(req.Model, auth); override != "" {
		upstream = override
	}
	if upstream != "" {
		payload, _ = sjson.SetBytes(payload, "model", upstream)
	}
	currentModel := gjson.GetBytes(payload, "model").String()
	if strings.HasPrefix(currentModel, "minimax:") {
		payload, _ = sjson.SetBytes(payload, "model", strings.TrimPrefix(currentModel, "minimax:"))
	}
	if !gjson.GetBytes(payload, "model").Exists() || gjson.GetBytes(payload, "model").String() == "" {
		payload, _ = sjson.SetBytes(payload, "model", "music-2.6")
	}
	// Sync path must not receive stream:true — upstream would return SSE
	// and the caller's json.Unmarshal of a single response would fail.
	// The streaming path injects stream:true itself; everyone else gets it
	// stripped here.
	payload, _ = sjson.DeleteBytes(payload, "stream")
	payload = normaliseMinimaxCoverAudioBase64(payload)
	return payload
}

func normaliseMinimaxCoverAudioBase64(payload []byte) []byte {
	model := strings.ToLower(gjson.GetBytes(payload, "model").String())
	if !strings.Contains(model, "music-cover") {
		return payload
	}
	audio := gjson.GetBytes(payload, "audio_base64")
	if !audio.Exists() || strings.TrimSpace(audio.String()) == "" {
		return payload
	}
	decoded, err := base64.StdEncoding.DecodeString(audio.String())
	if err != nil {
		return payload
	}
	normalised := decoded
	changed := false
	if repaired, ok := repairUnknownSizeWAVHeader(normalised); ok {
		normalised = repaired
		changed = true
		log.Infof("MINIMAX MUSIC: repaired non-seekable WAV header for music-cover reference bytes=%d", len(repaired))
	}
	if !changed {
		return payload
	}
	payload, _ = sjson.SetBytes(payload, "audio_base64", base64.StdEncoding.EncodeToString(normalised))
	return payload
}

func repairUnknownSizeWAVHeader(in []byte) ([]byte, bool) {
	if len(in) < 44 || string(in[0:4]) != "RIFF" || string(in[8:12]) != "WAVE" {
		return in, false
	}
	out := append([]byte(nil), in...)
	repaired := false
	riffSize := binary.LittleEndian.Uint32(out[4:8])
	actualRiffSize := uint32(len(out) - 8)
	if riffSize == 0xffffffff || riffSize == 0 || int64(riffSize) > int64(len(out)) {
		binary.LittleEndian.PutUint32(out[4:8], actualRiffSize)
		repaired = true
	}

	for off := 12; off+8 <= len(out); {
		chunkID := string(out[off : off+4])
		chunkSize := binary.LittleEndian.Uint32(out[off+4 : off+8])
		dataStart := off + 8
		if chunkID == "data" {
			actualDataSize := len(out) - dataStart
			if actualDataSize < 0 {
				return out, repaired
			}
			if chunkSize == 0xffffffff || chunkSize == 0 || int64(chunkSize) > int64(actualDataSize) {
				binary.LittleEndian.PutUint32(out[off+4:off+8], uint32(actualDataSize))
				repaired = true
			}
			return out, repaired
		}
		if chunkSize == 0xffffffff || chunkSize == 0 {
			return out, repaired
		}
		next := dataStart + int(chunkSize)
		if chunkSize%2 == 1 {
			next++
		}
		if next <= off || next > len(out) {
			return out, repaired
		}
		off = next
	}
	return out, repaired
}

type pcm16WAVReference struct {
	fmtChunk      []byte
	data          []byte
	sampleRate    uint32
	blockAlign    uint16
	bitsPerSample uint16
}

func parsePCM16WAVReference(in []byte) (pcm16WAVReference, bool) {
	if len(in) < 44 || string(in[0:4]) != "RIFF" || string(in[8:12]) != "WAVE" {
		return pcm16WAVReference{}, false
	}
	var ref pcm16WAVReference
	for off := 12; off+8 <= len(in); {
		chunkID := string(in[off : off+4])
		chunkSize := binary.LittleEndian.Uint32(in[off+4 : off+8])
		dataStart := off + 8
		if chunkSize == 0xffffffff || int64(chunkSize) > int64(len(in)-dataStart) {
			chunkSize = uint32(len(in) - dataStart)
		}
		dataEnd := dataStart + int(chunkSize)
		if dataEnd > len(in) || dataEnd < dataStart {
			return pcm16WAVReference{}, false
		}
		switch chunkID {
		case "fmt ":
			fmtChunk := in[dataStart:dataEnd]
			if len(fmtChunk) < 16 {
				return pcm16WAVReference{}, false
			}
			audioFormat := binary.LittleEndian.Uint16(fmtChunk[0:2])
			blockAlign := binary.LittleEndian.Uint16(fmtChunk[12:14])
			bitsPerSample := binary.LittleEndian.Uint16(fmtChunk[14:16])
			if audioFormat != 1 || blockAlign == 0 || bitsPerSample != 16 {
				return pcm16WAVReference{}, false
			}
			ref.fmtChunk = append([]byte(nil), fmtChunk...)
			ref.sampleRate = binary.LittleEndian.Uint32(fmtChunk[4:8])
			ref.blockAlign = blockAlign
			ref.bitsPerSample = bitsPerSample
		case "data":
			ref.data = append([]byte(nil), in[dataStart:dataEnd]...)
		}
		next := dataEnd
		if chunkSize%2 == 1 {
			next++
		}
		if next <= off || next > len(in) {
			break
		}
		off = next
	}
	if len(ref.fmtChunk) == 0 || len(ref.data) == 0 || ref.sampleRate == 0 || ref.blockAlign == 0 {
		return pcm16WAVReference{}, false
	}
	dataLen := len(ref.data) - (len(ref.data) % int(ref.blockAlign))
	if dataLen <= 0 {
		return pcm16WAVReference{}, false
	}
	ref.data = ref.data[:dataLen]
	return ref, true
}

// normaliseMinimaxLyricsRequest handles the mode-required schema. If the
// client omits `mode`, default to "write_full_song" since that's what most
// callers want (edit mode needs existing lyrics to continue from).
func normaliseMinimaxLyricsRequest(req switchailocalexecutor.Request) []byte {
	payload := req.Payload
	if payload == nil || len(bytes.TrimSpace(payload)) == 0 {
		payload = []byte("{}")
	}
	if !gjson.GetBytes(payload, "mode").Exists() || gjson.GetBytes(payload, "mode").String() == "" {
		payload, _ = sjson.SetBytes(payload, "mode", "write_full_song")
	}
	// Lyrics endpoint ignores 'model' but clients sometimes send it. Strip
	// to keep the upstream logs clean.
	payload, _ = sjson.DeleteBytes(payload, "model")
	return payload
}

// executeMinimaxMusicStream is the streaming sibling of executeMinimaxMusic.
// Wire-verified against the live API (2026-04-18): upstream returns a real
// text/event-stream with ~20s TTFB, 6–8 progressive frames of hex-encoded
// MP3 bytes (status=1), and a terminal frame (status=2) that re-duplicates
// the FULL audio. This adapter unwraps the SSE, hex-decodes each progressive
// chunk, and emits raw MP3 bytes on the returned channel so the handler can
// stream them to the client as Content-Type: audio/mpeg. The duplicated
// audio in the terminal frame is discarded; extra_info (duration, bitrate,
// sample_rate) and trace_id are logged server-side.
//
// Failover-friendly behaviour:
//   - base_resp.status_code != 0 observed BEFORE any bytes are sent → emit a
//     typed statusErr so the conductor can advance to the next provider.
//   - base_resp.status_code != 0 AFTER bytes have been sent → log + close
//     (client already has a partial, possibly-playable MP3).
//   - Upstream stall during streaming → streamStallWatchdog fires stallError
//     with the correct phase (pre-first-byte is retryable, mid-stream is not).
//
// Why raw MP3 rather than re-wrapping in our own SSE: it matches the TTS
// precedent (AudioSpeech returns audio/mpeg directly), every MP3 client
// can decode concatenated frames natively (the first frame carries the
// ID3v2 header), and it cuts client bandwidth by ~75% vs passing the hex
// through (hex is 2× overhead, plus we drop the duplicated final frame).
func (e *OpenAICompatExecutor) executeMinimaxMusicStream(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (<-chan switchailocalexecutor.StreamChunk, error) {
	payload := e.normaliseMinimaxMusicRequest(req, auth)
	payload, _ = sjson.SetBytes(payload, "stream", true)

	url := strings.TrimSuffix(baseURL, "/") + minimaxMusicEndpoint
	log.Infof("MINIMAX music stream: posting to %s with body=%s", url, truncate(string(payload), 400))

	streamCtx, streamCancel := context.WithCancel(ctx)
	httpReq, err := http.NewRequestWithContext(streamCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		streamCancel()
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if auth != nil {
		util.ApplyCustomHeadersFromAttrs(httpReq, auth.Attributes)
	}

	// Watchdog guards against an upstream that accepts the connection and
	// then hangs silently. First-byte timeout covers connect+TTFB (~20s is
	// normal for MiniMax), stall timeout covers inter-frame gaps. The
	// global SSE defaults (15s / 60s) are tuned for chat token streaming
	// and too aggressive for music — we bump to a music-appropriate floor.
	firstByte := e.cfg.Performance.Streaming.ResolveFirstByte(e.Identifier())
	if firstByte < minimaxMusicFirstByteMin {
		firstByte = minimaxMusicFirstByteMin
	}
	stallT := e.cfg.Performance.Streaming.StallTimeout
	if stallT < minimaxMusicStallMin {
		stallT = minimaxMusicStallMin
	}
	var watchdog *streamStallWatchdog
	if firstByte > 0 || stallT > 0 {
		watchdog = newStreamStallWatchdog(streamCancel, firstByte, stallT)
		watchdog.start()
	}

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		if watchdog != nil {
			watchdog.stop()
		}
		streamCancel()
		return nil, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		if watchdog != nil {
			watchdog.stop()
		}
		b, _ := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		streamCancel()
		return nil, statusErr{code: httpResp.StatusCode, msg: truncate(string(b), 300)}
	}

	out := make(chan switchailocalexecutor.StreamChunk)
	go func() {
		defer close(out)
		defer streamCancel()
		defer func() {
			if watchdog != nil {
				watchdog.stop()
			}
			if cerr := httpResp.Body.Close(); cerr != nil {
				log.Errorf("minimax-music-stream: close response body: %v", cerr)
			}
		}()

		// bufio.Reader.ReadBytes handles lines of arbitrary length — SSE
		// frames from MiniMax can exceed 2.9MB (the terminal frame carries a
		// duplicate of the full song). bufio.Scanner would hit its 1MB cap.
		reader := bufio.NewReader(httpResp.Body)
		sentFirstByte := false
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 && watchdog != nil {
				watchdog.onChunk()
			}
			if len(line) > 0 {
				// SSE frames are `data: <json>\n\n`. Strip trailing \n and
				// leading `data: ` prefix; skip heartbeats and blank lines.
				line = bytes.TrimRight(line, "\r\n")
				if len(line) > 0 && bytes.HasPrefix(line, []byte("data:")) {
					jsonPart := bytes.TrimSpace(line[len("data:"):])
					if emitErr, gotBytes := e.handleMinimaxMusicFrame(ctx, jsonPart, out, sentFirstByte); emitErr != nil {
						sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Err: emitErr})
						return
					} else if gotBytes {
						sentFirstByte = true
					}
				}
			}
			if err != nil {
				if errors.Is(err, io.EOF) {
					return
				}
				if watchdog != nil && watchdog.firedDueToStall() {
					phase := stallPhaseMidStream
					timeout := stallT
					if watchdog.preFirstChunk() {
						phase = stallPhasePreFirstByte
						if firstByte > 0 {
							timeout = firstByte
						}
					}
					sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Err: &stallError{Provider: e.Identifier(), Phase: phase, Timeout: timeout}})
					return
				}
				sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Err: err})
				return
			}
		}
	}()
	return out, nil
}

// handleMinimaxMusicFrame parses a single `data:`-stripped JSON payload
// from the MiniMax music SSE stream and either emits raw MP3 bytes on the
// out channel or returns an error for the caller to propagate.
//
// Returns (err, emittedBytes). err != nil aborts the stream; emittedBytes
// tells the caller whether we crossed the pre-first-byte boundary (used to
// decide if an upstream application error is still retryable).
func (e *OpenAICompatExecutor) handleMinimaxMusicFrame(ctx context.Context, jsonPart []byte, out chan<- switchailocalexecutor.StreamChunk, alreadySentBytes bool) (error, bool) {
	var frame minimaxMusicResponse
	if err := json.Unmarshal(jsonPart, &frame); err != nil {
		// Malformed frames are logged and skipped rather than killing the
		// stream — a single bad frame shouldn't abort a song in progress.
		log.Warnf("minimax-music-stream: skip unparseable frame: %v (snippet=%s)", err, truncate(string(jsonPart), 120))
		return nil, false
	}
	if frame.BaseResp.StatusCode != 0 {
		httpCode, _ := minimaxStatusToHTTP(frame.BaseResp.StatusCode)
		err := statusErr{
			code: httpCode,
			msg:  fmt.Sprintf("minimax music stream: code=%d msg=%q trace=%s", frame.BaseResp.StatusCode, frame.BaseResp.StatusMsg, frame.TraceID),
		}
		if alreadySentBytes {
			// Client already has bytes — no point surfacing as retryable.
			// Log the upstream error so it appears in audit trails; the
			// stream closes cleanly on the next read.
			log.Warnf("minimax-music-stream: upstream error mid-stream: %v", err)
			return err, false
		}
		return err, false
	}
	if frame.Data == nil || frame.Data.Audio == "" {
		// Heartbeat / metadata-only frame. No audio to emit.
		return nil, false
	}
	// status=2 is the terminal frame. Its audio field duplicates the full
	// song (wasteful) while extra_info carries real metadata. Drop the audio,
	// log the metadata for observability.
	if frame.Data.Status == 2 {
		if len(frame.ExtraInfo) > 0 {
			log.Infof("MINIMAX music stream: complete trace=%s extra_info=%s", frame.TraceID, truncate(string(frame.ExtraInfo), 200))
		}
		return nil, false
	}
	audioBytes, err := hex.DecodeString(frame.Data.Audio)
	if err != nil {
		log.Warnf("minimax-music-stream: hex decode failed, skipping frame: %v", err)
		return nil, false
	}
	if len(audioBytes) == 0 {
		return nil, false
	}
	// Cancellation (e.g. client disconnect) means nobody is reading; treat it as
	// "nothing emitted" and let the read loop unwind on its next error instead of
	// blocking forever on this send.
	if !sendStreamChunk(ctx, out, switchailocalexecutor.StreamChunk{Payload: audioBytes}) {
		return nil, false
	}
	return nil, true
}

// postMinimaxJSON is the shared POST helper for music + lyrics + any future
// MiniMax-native JSON endpoints. Factored out of the individual adapters so
// the auth/timeout/proxy wiring lives in one place.
func (e *OpenAICompatExecutor) postMinimaxJSON(ctx context.Context, auth *switchailocalauth.Auth, baseURL, endpointPath, apiKey string, body []byte) ([]byte, error) {
	endpoint := strings.TrimPrefix(endpointPath, "/")
	url := strings.TrimSuffix(baseURL, "/") + endpointPath
	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)

	var lastErr error
	var lastBody []byte
	for attempt := 1; attempt <= minimaxJSONMaxAttempts; attempt++ {
		log.Infof("MINIMAX %s: posting to %s attempt=%d/%d with body=%s", endpoint, url, attempt, minimaxJSONMaxAttempts, truncate(string(body), 400))

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if auth != nil {
			util.ApplyCustomHeadersFromAttrs(httpReq, auth.Attributes)
		}

		httpReq, cancel := applyProviderTimeout(ctx, e.cfg, e.Identifier(), httpReq)
		httpResp, err := httpClient.Do(httpReq)
		if err != nil {
			cancel()
			lastErr = err
			if attempt < minimaxJSONMaxAttempts && minimaxRetryableTransportError(ctx, err) {
				log.Warnf("MINIMAX %s: transient transport error attempt=%d/%d: %v; retrying", endpoint, attempt, minimaxJSONMaxAttempts, err)
				continue
			}
			return nil, minimaxTransportStatusErr(endpoint, attempt, err)
		}

		respBody, readErr := io.ReadAll(httpResp.Body)
		closeErr := httpResp.Body.Close()
		cancel()
		if closeErr != nil {
			log.Errorf("minimax-%s: close response body: %v", endpoint, closeErr)
		}
		lastBody = respBody
		if readErr != nil {
			lastErr = readErr
			if attempt < minimaxJSONMaxAttempts && minimaxRetryableTransportError(ctx, readErr) {
				log.Warnf("MINIMAX %s: transient response read error attempt=%d/%d: %v; retrying", endpoint, attempt, minimaxJSONMaxAttempts, readErr)
				continue
			}
			return respBody, minimaxTransportStatusErr(endpoint, attempt, readErr)
		}

		// Transport-level non-2xx (MiniMax usually returns 200 + base_resp error,
		// but some endpoints like lyrics return real 4xx on schema violation).
		if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
			err := statusErr{code: httpResp.StatusCode, msg: truncate(string(respBody), 300)}
			lastErr = err
			if attempt < minimaxJSONMaxAttempts && minimaxRetryableHTTPStatus(httpResp.StatusCode) {
				log.Warnf("MINIMAX %s: retryable HTTP status=%d attempt=%d/%d body=%s; retrying", endpoint, httpResp.StatusCode, attempt, minimaxJSONMaxAttempts, truncate(string(respBody), 180))
				continue
			}
			return respBody, err
		}
		return respBody, nil
	}

	if lastErr != nil {
		return lastBody, minimaxTransportStatusErr(endpoint, minimaxJSONMaxAttempts, lastErr)
	}
	return lastBody, statusErr{code: http.StatusBadGateway, msg: fmt.Sprintf("minimax %s: request failed without response", endpoint)}
}

func minimaxRetryableHTTPStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func minimaxRetryableTransportError(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe")
}

func minimaxTransportStatusErr(endpoint string, attempts int, err error) statusErr {
	return statusErr{
		code: http.StatusBadGateway,
		msg:  fmt.Sprintf("minimax %s: transient transport failed after %d attempt(s): %v", endpoint, attempts, err),
	}
}
