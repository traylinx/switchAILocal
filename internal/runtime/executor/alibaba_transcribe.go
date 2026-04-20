// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/traylinx/switchAILocal/internal/util"
	switchailocalauth "github.com/traylinx/switchAILocal/sdk/switchailocal/auth"
	switchailocalexecutor "github.com/traylinx/switchAILocal/sdk/switchailocal/executor"
)

// Alibaba DashScope exposes qwen3-asr-flash via the OpenAI-compat
// /chat/completions endpoint — NOT via a whisper-shaped /audio/transcriptions.
// The canonical call looks like:
//
//	POST /compatible-mode/v1/chat/completions
//	{
//	  "model": "qwen3-asr-flash",
//	  "messages": [{"role":"user","content":[
//	    {"type":"input_audio","input_audio":{"data":"<url or data: URI>"}}
//	  ]}],
//	  "asr_options": {"enable_itn": false}
//	}
//
// and the transcription comes back in `choices[0].message.content` as plain
// text. This adapter lets any whisper-SDK client (OpenAI Python, OpenAI
// Node, the curl-based whisper CLI, et al.) keep calling the standard
// /v1/audio/transcriptions multipart endpoint — we unwrap the multipart
// here, repackage as an input_audio chat call, and reshape the reply back
// into OpenAI's whisper envelope `{"text": "..."}`.
//
// Dispatch from openai_compat_executor.go Execute → case "audio_transcriptions"
// → isAlibabaASRRequest → this function.

const alibabaTranscribeChatEndpoint = "/chat/completions"

// alibabaTranscribeMaxAudioBytes caps inlined audio at 25 MB — matches the
// OpenAI Whisper limit and keeps the base64-inflated JSON body under 34 MB,
// which is below the default gateway max-body ceiling. Callers wanting
// larger clips should chunk client-side (the transcript is verbatim so
// concatenation is safe).
const alibabaTranscribeMaxAudioBytes = 25 * 1024 * 1024

// isAlibabaASRRequest reports whether the executor/auth pair should route
// /audio/transcriptions through the Alibaba chat-completions bridge. We key
// off the executor identifier AND (belt-and-braces) the auth.Provider name,
// so a provider registered under a non-standard identifier still routes
// correctly as long as its auth is tagged Provider: "alibaba".
func isAlibabaASRRequest(executorID string, auth *switchailocalauth.Auth) bool {
	if strings.Contains(strings.ToLower(executorID), "alibaba") {
		return true
	}
	if auth != nil && strings.Contains(strings.ToLower(auth.Provider), "alibaba") {
		return true
	}
	return false
}

// executeAlibabaTranscribe handles POST /v1/audio/transcriptions when the
// resolved provider is Alibaba. The multipart body is parsed here (rather
// than relying on the dumb-proxy path) so we can strip the audio bytes,
// base64-encode them into a data URI, and repackage them as the chat
// input_audio block qwen3-asr-flash expects. The response is reshaped into
// the OpenAI whisper envelope so existing whisper clients don't need code
// changes.
func (e *OpenAICompatExecutor) executeAlibabaTranscribe(ctx context.Context, auth *switchailocalauth.Auth, req switchailocalexecutor.Request, baseURL, apiKey string) (switchailocalexecutor.Response, error) {
	audioBytes, format, language, err := extractAlibabaTranscribeMultipart(req)
	if err != nil {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadRequest, msg: fmt.Sprintf("alibaba-transcribe: %v", err)}
	}

	upstreamModel := util.ResolveOriginalModel(req.Model, req.Metadata)
	if override := e.resolveUpstreamModel(req.Model, auth); override != "" {
		upstreamModel = override
	}
	if upstreamModel == "" {
		upstreamModel = strings.TrimPrefix(req.Model, "alibaba:")
	}
	if upstreamModel == "" || strings.HasPrefix(upstreamModel, "ail-") {
		upstreamModel = "qwen3-asr-flash"
	}

	// Alibaba accepts both public URLs and data: URIs in input_audio.data.
	// Whisper multipart semantics hand us raw bytes, so we always go the
	// data-URI route. Content-Type defaults to audio/mpeg for unknown
	// extensions since qwen3-asr-flash autodetects the container.
	mimeType := mimeFromFormatHint(format)
	dataURI := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(audioBytes)

	userContent := []map[string]any{
		{"type": "input_audio", "input_audio": map[string]any{"data": dataURI}},
	}
	if language != "" {
		// qwen3-asr-flash takes language hints via asr_options, but it also
		// accepts a free-text instruction in a preceding text block. Include
		// it belt-and-braces so even if asr_options is ignored the model
		// still biases toward the right language.
		userContent = append([]map[string]any{
			{"type": "text", "text": "Transcribe this audio verbatim. Target language: " + language + "."},
		}, userContent...)
	}

	chatBody := map[string]any{
		"model":    upstreamModel,
		"stream":   false,
		"messages": []map[string]any{{"role": "user", "content": userContent}},
		"asr_options": map[string]any{
			"enable_itn": false,
		},
	}
	payload, err := json.Marshal(chatBody)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("alibaba-transcribe: marshal chat body: %w", err)
	}

	respBody, err := e.postMinimaxJSON(ctx, auth, baseURL, alibabaTranscribeChatEndpoint, apiKey, payload)
	if err != nil {
		return switchailocalexecutor.Response{}, err
	}

	text := gjson.GetBytes(respBody, "choices.0.message.content").String()
	if text == "" {
		// Rare: upstream may return a content array instead of scalar text.
		parts := gjson.GetBytes(respBody, "choices.0.message.content.#.text").Array()
		if len(parts) > 0 {
			var b strings.Builder
			for _, p := range parts {
				b.WriteString(p.String())
			}
			text = b.String()
		}
	}
	if text == "" {
		return switchailocalexecutor.Response{}, statusErr{code: http.StatusBadGateway, msg: "alibaba-transcribe: upstream returned empty transcription"}
	}

	// OpenAI whisper envelope: `{"text":"..."}` is the ONLY required field;
	// we pass through the language annotation (qwen emits `{language:"zh"}`
	// in annotations) and the upstream id as trace_id so callers can
	// correlate with upstream logs without deviating from the standard.
	lang := gjson.GetBytes(respBody, "choices.0.message.annotations.0.language").String()
	id := gjson.GetBytes(respBody, "id").String()
	out := map[string]any{
		"text":     strings.TrimSpace(text),
		"model":    upstreamModel,
		"trace_id": id,
	}
	if lang != "" {
		out["language"] = lang
	}
	payloadOut, err := json.Marshal(out)
	if err != nil {
		return switchailocalexecutor.Response{}, fmt.Errorf("alibaba-transcribe: marshal response: %w", err)
	}
	log.Infof("ALIBABA transcribe: success id=%s model=%s bytes_in=%d text_len=%d lang=%q", id, upstreamModel, len(audioBytes), len(text), lang)
	return switchailocalexecutor.Response{Payload: payloadOut}, nil
}

// extractAlibabaTranscribeMultipart reads req.Payload (the raw whisper
// multipart body) and returns the audio bytes, a format hint derived from
// the uploaded filename extension (mp3/wav/m4a/flac), and the optional
// `language` form field. req.Metadata["content_type"] must carry the
// original Content-Type header so we can parse the boundary.
func extractAlibabaTranscribeMultipart(req switchailocalexecutor.Request) (audio []byte, format, language string, err error) {
	contentType, _ := req.Metadata["content_type"].(string)
	if contentType == "" {
		return nil, "", "", fmt.Errorf("missing content-type header (expected multipart/form-data)")
	}
	mediaType, params, mtErr := mime.ParseMediaType(contentType)
	if mtErr != nil {
		return nil, "", "", fmt.Errorf("parse content-type: %w", mtErr)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return nil, "", "", fmt.Errorf("unexpected content-type %q (want multipart/form-data)", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, "", "", fmt.Errorf("multipart body missing boundary parameter")
	}
	if int64(len(req.Payload)) > alibabaTranscribeMaxAudioBytes*2 {
		// Base64 inflates by ~33%, request body by boundary overhead. Hard
		// cap at 2× to bail early on obviously-oversized uploads before we
		// do any work.
		return nil, "", "", fmt.Errorf("request body exceeds %d-byte cap (chunk audio client-side)", alibabaTranscribeMaxAudioBytes)
	}

	reader := multipart.NewReader(bytes.NewReader(req.Payload), boundary)
	for {
		part, perr := reader.NextPart()
		if perr == io.EOF {
			break
		}
		if perr != nil {
			return nil, "", "", fmt.Errorf("multipart read: %w", perr)
		}
		switch part.FormName() {
		case "file":
			buf := &bytes.Buffer{}
			if _, cerr := io.Copy(buf, io.LimitReader(part, alibabaTranscribeMaxAudioBytes+1)); cerr != nil {
				_ = part.Close()
				return nil, "", "", fmt.Errorf("read audio: %w", cerr)
			}
			if buf.Len() > alibabaTranscribeMaxAudioBytes {
				_ = part.Close()
				return nil, "", "", fmt.Errorf("audio payload exceeds %d-byte cap", alibabaTranscribeMaxAudioBytes)
			}
			audio = buf.Bytes()
			if fn := part.FileName(); fn != "" {
				ext := strings.TrimPrefix(strings.ToLower(path.Ext(fn)), ".")
				if ext != "" {
					format = ext
				}
			}
		case "language":
			buf := &bytes.Buffer{}
			_, _ = io.Copy(buf, part)
			language = strings.TrimSpace(buf.String())
		}
		_ = part.Close()
	}
	if len(audio) == 0 {
		return nil, "", "", fmt.Errorf("multipart form missing required field 'file'")
	}
	if format == "" {
		format = "mp3"
	}
	return audio, format, language, nil
}

// mimeFromFormatHint maps a bare extension (mp3/wav/m4a/flac/ogg/webm) to
// the matching MIME type for the data: URI. Unknown extensions fall back
// to audio/mpeg — qwen3-asr-flash autodetects the container so the exact
// label rarely matters, but the HTTP client chain on the path expects a
// real value.
func mimeFromFormatHint(ext string) string {
	switch strings.ToLower(ext) {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "m4a", "aac":
		return "audio/mp4"
	case "flac":
		return "audio/flac"
	case "ogg", "opus":
		return "audio/ogg"
	case "webm":
		return "audio/webm"
	default:
		return "audio/mpeg"
	}
}
