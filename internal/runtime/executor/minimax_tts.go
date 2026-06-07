// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"
	"context"
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

// MiniMax T2A endpoint and shape are completely different from OpenAI's
// /v1/audio/speech. The native MiniMax TTS path is /v1/t2a_v2 and returns
// hex-encoded audio by default, or a short-lived URL when output_format=url.
//
// This file converts an OpenAI-shaped /v1/audio/speech request into a MiniMax
// /v1/t2a_v2 call, decodes/fetches the returned audio, and passes the bytes
// back to the caller. The handler at the public boundary
// (sdk/api/handlers/openai/openai_handlers.go:AudioSpeech) does not need to
// change — it still talks OpenAI shape; this adapter sits behind it.
//
// The translator also accepts MiniMax-native fields (text, voice_setting,
// audio_setting, pronunciation_dict, language_boost, voice_modify,
// output_format) so clients can use the richer MiniMax T2A controls through
// the same SwitchAI Local endpoint without losing existing OpenAI clients.
//
// Trigger conditions (see openai_compat_executor.go Execute):
//   - operation == "audio_speech"
//   - provider identifier or auth.Provider contains "minimax"

const minimaxTTSEndpoint = "/t2a_v2"

// minimaxTTSResponse mirrors the shapes returned by /v1/t2a_v2 plus the
// legacy audio_file field kept for old upstream compatibility. The JSON has
// other fields too but only these matter for converting back to bytes.
type minimaxTTSResponse struct {
	AudioFile    string `json:"audio_file"`
	SubtitleFile string `json:"subtitle_file"`
	Data         struct {
		Audio  string `json:"audio"`
		Status int    `json:"status"`
	} `json:"data"`
	TraceID  string `json:"trace_id"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// isMinimaxTTSRequest reports whether the executor + auth combination should
// route audio_speech through the MiniMax T2A path instead of the dumb-proxy.
// Mirrors the detection used elsewhere in this file for image_generation.
func isMinimaxTTSRequest(executorID string, auth *switchailocalauth.Auth) bool {
	if strings.Contains(strings.ToLower(executorID), "minimax") {
		return true
	}
	if auth != nil && strings.Contains(strings.ToLower(auth.Provider), "minimax") {
		return true
	}
	return false
}

// translateOpenAITTSToMinimaxT2A rewrites an OpenAI /v1/audio/speech request
// body into the MiniMax /v1/t2a_v2 shape. It preserves the upstream model
// name (already injected by the caller) and maps the well-known fields. Any
// unknown fields are dropped — MiniMax rejects unrecognised keys with 2013.
//
// Field mapping:
//
//	OpenAI                       MiniMax t2a_v2
//	model                        model
//	input              ────►     text
//	voice              ────►     voice_setting.voice_id
//	speed                        voice_setting.speed
//	response_format    ────►     audio_setting.format (mp3 / flac / wav)
//
// MiniMax-native fields are preserved when supplied:
// text, stream=false, voice_setting, audio_setting, pronunciation_dict,
// language_boost, voice_modify, subtitle_enable, subtitle_type, output_format.
func translateOpenAITTSToMinimaxT2A(body []byte) ([]byte, error) {
	model := gjson.GetBytes(body, "model").String()
	input := gjson.GetBytes(body, "text").String()
	if input == "" {
		input = gjson.GetBytes(body, "input").String()
	}
	voice := gjson.GetBytes(body, "voice").String()
	voiceSetting := gjson.GetBytes(body, "voice_setting")
	if voice == "" && voiceSetting.IsObject() {
		voice = voiceSetting.Get("voice_id").String()
	}
	if model == "" {
		return nil, fmt.Errorf("minimax-tts: missing model")
	}
	if input == "" {
		return nil, fmt.Errorf("minimax-tts: missing input/text")
	}
	if voice == "" {
		return nil, fmt.Errorf("minimax-tts: missing voice or voice_setting.voice_id (use a MiniMax voice_id, e.g. English_expressive_narrator)")
	}

	out := map[string]any{
		"model":         model,
		"text":          input,
		"stream":        false,
		"output_format": "hex",
	}

	if stream := gjson.GetBytes(body, "stream"); stream.Exists() {
		if stream.Bool() {
			return nil, fmt.Errorf("minimax-tts: stream=true is not supported on /v1/audio/speech yet; use stream=false")
		}
		out["stream"] = false
	}

	if voiceSetting.IsObject() {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(voiceSetting.Raw), &parsed); err == nil {
			out["voice_setting"] = parsed
		}
	} else {
		setting := map[string]any{"voice_id": voice}
		if speed := gjson.GetBytes(body, "speed"); speed.Exists() {
			setting["speed"] = speed.Float()
		}
		if vol := gjson.GetBytes(body, "vol"); vol.Exists() {
			setting["vol"] = vol.Float()
		}
		if pitch := gjson.GetBytes(body, "pitch"); pitch.Exists() {
			setting["pitch"] = pitch.Int()
		}
		out["voice_setting"] = setting
	}

	audioSetting := gjson.GetBytes(body, "audio_setting")
	if audioSetting.IsObject() {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(audioSetting.Raw), &parsed); err == nil {
			out["audio_setting"] = parsed
		}
	} else {
		setting := map[string]any{}
		if rf := gjson.GetBytes(body, "response_format").String(); rf != "" {
			setting["format"] = rf
		} else if f := gjson.GetBytes(body, "format").String(); f != "" {
			setting["format"] = f
		}
		if sr := gjson.GetBytes(body, "audio_sample_rate"); sr.Exists() {
			setting["sample_rate"] = sr.Int()
		} else if sr := gjson.GetBytes(body, "sample_rate"); sr.Exists() {
			setting["sample_rate"] = sr.Int()
		}
		if br := gjson.GetBytes(body, "bitrate"); br.Exists() {
			setting["bitrate"] = br.Int()
		}
		if ch := gjson.GetBytes(body, "channel"); ch.Exists() {
			setting["channel"] = ch.Int()
		}
		if len(setting) > 0 {
			out["audio_setting"] = setting
		}
	}

	preserveJSONField(body, out, "pronunciation_dict")
	preserveJSONField(body, out, "timbre_weights")
	preserveJSONField(body, out, "voice_modify")
	preserveJSONField(body, out, "stream_options")
	preserveJSONScalar(body, out, "language_boost")
	preserveJSONScalar(body, out, "subtitle_enable")
	preserveJSONScalar(body, out, "subtitle_type")
	if outputFormat := gjson.GetBytes(body, "output_format").String(); outputFormat != "" {
		out["output_format"] = outputFormat
	}

	return json.Marshal(out)
}

func preserveJSONField(body []byte, out map[string]any, field string) {
	value := gjson.GetBytes(body, field)
	if !value.Exists() {
		return
	}
	var parsed any
	if err := json.Unmarshal([]byte(value.Raw), &parsed); err == nil {
		out[field] = parsed
	}
}

func preserveJSONScalar(body []byte, out map[string]any, field string) {
	value := gjson.GetBytes(body, field)
	if !value.Exists() {
		return
	}
	switch value.Type {
	case gjson.String:
		out[field] = value.String()
	case gjson.True, gjson.False:
		out[field] = value.Bool()
	case gjson.Number:
		out[field] = value.Value()
	}
}

func decodeMinimaxT2AAudio(resp minimaxTTSResponse) ([]byte, string, error) {
	if resp.Data.Audio == "" {
		return nil, "", nil
	}
	audio := strings.TrimSpace(resp.Data.Audio)
	if strings.HasPrefix(strings.ToLower(audio), "http://") || strings.HasPrefix(strings.ToLower(audio), "https://") {
		return nil, audio, nil
	}
	decoded, err := hex.DecodeString(audio)
	if err != nil {
		return nil, "", fmt.Errorf("minimax t2a_v2: decode hex audio: %w", err)
	}
	return decoded, "", nil
}

// minimaxStatusToHTTP maps MiniMax base_resp.status_code values onto HTTP
// status codes that the failover taxonomy understands. The mapping is
// intentionally narrow — only codes we've actually observed are listed.
// Unknown codes fall through as 502, treated as transient by Classify.
//
// Why this matters: MiniMax always returns HTTP 200 even on application
// errors (rate limits, plan limits). Without this translation the failover
// system would never advance, since it sees a 2xx and assumes success.
func minimaxStatusToHTTP(code int) (httpCode int, retryable bool) {
	switch code {
	case 0:
		return http.StatusOK, false
	case 1002:
		// "rate limit exceeded(RPM)" — transient, advance / cool down.
		return http.StatusTooManyRequests, true
	case 1004, 1008:
		// 1004 = invalid api key, 1008 = insufficient balance — terminal,
		// advance to next provider.
		return http.StatusUnauthorized, true
	case 2013:
		// "invalid params" — caller sent something we can't fix; permanent.
		return http.StatusBadRequest, false
	case 2061:
		// "your current token plan not support model X" — out of credits
		// for THIS model, advance.
		return http.StatusPaymentRequired, true
	default:
		// Unknown — surface as upstream error, let failover classify as
		// transient and try next provider.
		return http.StatusBadGateway, true
	}
}

// executeMinimaxTTS runs the full MiniMax TTS path: translate request → POST
// to /v1/t2a_v2 → handle MiniMax error codes → decode/fetch audio →
// return bytes. Returns the same shape the dumb-proxy would have, so the
// public AudioSpeech handler doesn't need to know it ran.
func (e *OpenAICompatExecutor) executeMinimaxTTS(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (switchailocalexecutor.Response, error) {
	// Inject upstream model (alias → real name) before translating, otherwise
	// the request body still carries "minimax:speech-2.8-hd" instead of
	// "speech-2.8-hd" and MiniMax rejects with 2013 invalid_params.
	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)
	if override := e.resolveUpstreamModel(req.Model, auth); override != "" {
		upstreamModel = override
	}
	if upstreamModel == "" {
		upstreamModel = req.Model
	}

	payload := req.Payload
	if upstreamModel != "" {
		payload, _ = sjson.SetBytes(payload, "model", upstreamModel)
	}

	t2aBody, err := translateOpenAITTSToMinimaxT2A(payload)
	if err != nil {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadRequest, msg: err.Error()}
	}

	url := strings.TrimSuffix(baseURL, "/") + minimaxTTSEndpoint
	log.Infof("MINIMAX TTS: posting to %s with body=%s", url, string(t2aBody))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(t2aBody))
	if err != nil {
		return switchailocalexecutor.Response{}, err
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
		return switchailocalexecutor.Response{}, err
	}
	defer func() {
		if cerr := httpResp.Body.Close(); cerr != nil {
			log.Errorf("minimax-tts: close response body: %v", cerr)
		}
	}()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return switchailocalexecutor.Response{}, err
	}

	// Transport-level non-2xx (rare for MiniMax — usually returns 200 + error
	// in base_resp) — surface as-is.
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return switchailocalexecutor.Response{}, statusErr{code: httpResp.StatusCode, msg: string(respBody)}
	}

	var t2aResp minimaxTTSResponse
	if err := json.Unmarshal(respBody, &t2aResp); err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("minimax-tts: parse response: %w (body=%s)", err, truncate(string(respBody), 200))
	}

	if t2aResp.BaseResp.StatusCode != 0 {
		httpCode, _ := minimaxStatusToHTTP(t2aResp.BaseResp.StatusCode)
		return switchailocalexecutor.Response{}, statusErr{
			code: httpCode,
			msg:  fmt.Sprintf("minimax t2a_v2: code=%d msg=%q trace=%s", t2aResp.BaseResp.StatusCode, t2aResp.BaseResp.StatusMsg, t2aResp.TraceID),
		}
	}

	audioBytes, audioURL, err := decodeMinimaxT2AAudio(t2aResp)
	if err != nil {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadGateway, msg: fmt.Sprintf("%v (trace=%s)", err, t2aResp.TraceID)}
	}

	if len(audioBytes) == 0 && audioURL == "" {
		audioURL = t2aResp.AudioFile
	}
	if len(audioBytes) == 0 && audioURL == "" {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadGateway, msg: fmt.Sprintf("minimax t2a_v2: empty audio (trace=%s)", t2aResp.TraceID)}
	}

	if len(audioBytes) == 0 {
		// Fetch the audio bytes from the URL MiniMax handed back. Reuse the same
		// proxy-aware client so SOCKS / HTTP proxy settings carry through to OSS.
		audioBytes, err = fetchMinimaxAudioFile(ctx, httpClient, audioURL)
		if err != nil {
			return switchailocalexecutor.Response{}, fmt.Errorf("minimax-tts: fetch audio url %q: %w", audioURL, err)
		}
	}

	log.Infof("MINIMAX TTS: success trace=%s audio_url=%s bytes=%d", t2aResp.TraceID, audioURL, len(audioBytes))
	return switchailocalexecutor.Response{Payload: audioBytes}, nil
}

// fetchMinimaxAudioFile resolves an audio URL the upstream returned to the
// actual binary bytes the client is waiting for. MiniMax URL outputs are
// short-lived signed URLs.
func fetchMinimaxAudioFile(ctx context.Context, client *http.Client, audioURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Errorf("minimax-tts: close audio_file body: %v", cerr)
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("audio_file fetch returned %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// truncate is a tiny helper for log/error formatting.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
