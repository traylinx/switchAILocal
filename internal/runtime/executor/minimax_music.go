// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/traylinx/switchAILocal/internal/util"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
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
	minimaxMusicEndpoint  = "/music_generation"
	minimaxLyricsEndpoint = "/lyrics_generation"
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
	payload := normaliseMinimaxMusicRequest(req)

	body, err := e.postMinimaxJSON(ctx, auth, baseURL, minimaxMusicEndpoint, apiKey, payload)
	if err != nil {
		return switchailocalexecutor.Response{}, err
	}

	var mr minimaxMusicResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: parse response: %w (body=%s)", err, truncate(string(body), 200))
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

	audioBytes, err := hex.DecodeString(mr.Data.Audio)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-music: hex decode audio: %w", err)
	}

	// Shape the response for clients. Base64 is friendlier than hex for
	// JSON transport and OpenAI clients already parse it.
	// Report the upstream model name (post-normalisation) so clients see what
	// MiniMax actually ran, not the alias they sent in.
	out := map[string]any{
		"data": map[string]any{
			"audio":      base64.StdEncoding.EncodeToString(audioBytes),
			"format":     "mp3",
			"size_bytes": len(audioBytes),
		},
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
	log.Infof("MINIMAX MUSIC: success trace=%s bytes=%d (hex_len=%d)", mr.TraceID, len(audioBytes), len(mr.Data.Audio))
	return switchailocalexecutor.Response{Payload: respBody}, nil
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
// used ("minimax:music-2.6", "music-2.6", etc).
//
// Resolution order, matching how resolveUpstreamModel resolves aliases
// elsewhere in this package:
//  1. util.ResolveOriginalModel via metadata (production path; empty in tests).
//  2. Strip "minimax:" prefix from the current model field (fallback for
//     clients that sent the alias directly in the body).
//  3. Default to "music-2.6" if the field is missing entirely.
func normaliseMinimaxMusicRequest(req switchailocalexecutor.Request) []byte {
	payload := req.Payload
	if upstream := util.ResolveOriginalModel(req.Model, req.Metadata); upstream != "" {
		payload, _ = sjson.SetBytes(payload, "model", upstream)
	}
	currentModel := gjson.GetBytes(payload, "model").String()
	if strings.HasPrefix(currentModel, "minimax:") {
		payload, _ = sjson.SetBytes(payload, "model", strings.TrimPrefix(currentModel, "minimax:"))
	}
	if !gjson.GetBytes(payload, "model").Exists() || gjson.GetBytes(payload, "model").String() == "" {
		payload, _ = sjson.SetBytes(payload, "model", "music-2.6")
	}
	return payload
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

// postMinimaxJSON is the shared POST helper for music + lyrics + any future
// MiniMax-native JSON endpoints. Factored out of the individual adapters so
// the auth/timeout/proxy wiring lives in one place.
func (e *OpenAICompatExecutor) postMinimaxJSON(ctx context.Context, auth *switchailocalauth.Auth, baseURL, endpointPath, apiKey string, body []byte) ([]byte, error) {
	url := strings.TrimSuffix(baseURL, "/") + endpointPath
	log.Infof("MINIMAX %s: posting to %s with body=%s", strings.TrimPrefix(endpointPath, "/"), url, truncate(string(body), 400))

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
	defer cancel()

	httpClient := newProxyAwareHTTPClient(ctx, e.cfg, auth, 0)
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := httpResp.Body.Close(); cerr != nil {
			log.Errorf("minimax-%s: close response body: %v", strings.TrimPrefix(endpointPath, "/"), cerr)
		}
	}()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	// Transport-level non-2xx (MiniMax usually returns 200 + base_resp error,
	// but some endpoints like lyrics return real 4xx on schema violation).
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return respBody, statusErr{code: httpResp.StatusCode, msg: truncate(string(respBody), 300)}
	}
	return respBody, nil
}
