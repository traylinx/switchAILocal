// Copyright 2026 The switchAILocal Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package executor

import (
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// NormalizeMultimodalContent rewrites non-canonical content blocks in an
// OpenAI-shaped chat-completions request so the upstream provider sees
// the canonical OpenAI multimodal shape.
//
// Why: clients built on the Vercel AI SDK v5 (OpenCode, Cursor, parts of
// Continue.dev) increasingly emit content blocks like
//
//	{"type":"image","image":"data:image/png;base64,..."}
//	{"type":"audio","audio":{"data":"...","mediaType":"audio/wav"}}
//	{"type":"file","data":"...","mediaType":"image/png"}
//
// that aren't OpenAI-canonical and are silently dropped by upstream
// providers (or by older translator paths). We translate them in-place to
//
//	{"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}
//	{"type":"input_audio","input_audio":{"data":"...","format":"wav"}}
//
// which is what every OpenAI-compat upstream we route to (MiniMax, Groq,
// xiaomi, switchai) accepts. Idempotent: blocks already in canonical
// shape pass through untouched.
//
// Pure transformation — no I/O, no provider awareness. Called by every
// executor right after sdktranslator.TranslateRequest so it sits at the
// boundary between client-format diversity and the single shape upstream
// providers expect.
func NormalizeMultimodalContent(payload []byte) []byte {
	if len(payload) == 0 {
		return payload
	}
	messages := gjson.GetBytes(payload, "messages")
	if !messages.Exists() || !messages.IsArray() {
		return payload
	}

	out := payload
	mutated := false

	messages.ForEach(func(msgIdx, msg gjson.Result) bool {
		content := msg.Get("content")
		// Only array-shaped content carries multimodal blocks. String
		// content is plain-text and needs no transformation.
		if !content.Exists() || !content.IsArray() {
			return true
		}

		content.ForEach(func(partIdx, part gjson.Result) bool {
			partType := part.Get("type").String()
			path := buildContentPath(msgIdx.Int(), partIdx.Int())

			switch partType {
			case "image":
				// AI SDK v5 emits {type:"image", image:"<url-or-data-uri>"}
				// or {type:"image", image:{url:"..."}, mediaType?:"..."}
				normalized, ok := normalizeImageBlock(part)
				if ok {
					out, _ = sjson.SetRawBytes(out, path, normalized)
					mutated = true
				}
			case "audio", "input_audio":
				// AI SDK v5: {type:"audio", audio:{data, mediaType}}
				// OpenAI canonical: {type:"input_audio", input_audio:{data, format}}
				if partType == "input_audio" {
					// Already canonical. Skip — but only if it's well-formed.
					if part.Get("input_audio.data").Exists() && part.Get("input_audio.format").Exists() {
						return true
					}
				}
				normalized, ok := normalizeAudioBlock(part)
				if ok {
					out, _ = sjson.SetRawBytes(out, path, normalized)
					mutated = true
				}
			case "file":
				// AI SDK v5: {type:"file", data:"<base64>", mediaType:"image/png"}
				// → image_url if it's an image; passthrough as image_url
				// for any other type (provider 4xx tells client what's
				// actually unsupported, better than silent drop).
				normalized, ok := normalizeFileBlock(part)
				if ok {
					out, _ = sjson.SetRawBytes(out, path, normalized)
					mutated = true
				}
			}
			return true
		})
		return true
	})

	if mutated {
		log.Debugf("multimodal normalizer: rewrote non-canonical content blocks in payload")
	}
	return out
}

// buildContentPath constructs an sjson path to a specific message+part.
func buildContentPath(msgIdx, partIdx int64) string {
	var b strings.Builder
	b.WriteString("messages.")
	b.WriteString(itoa(msgIdx))
	b.WriteString(".content.")
	b.WriteString(itoa(partIdx))
	return b.String()
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

// normalizeImageBlock converts an AI-SDK-v5 {type:"image", image:...} part
// into OpenAI's canonical {type:"image_url", image_url:{url:...}}.
// Returns the new JSON + true if it actually changed shape; false if the
// block was unrecognised (caller should leave it as-is).
//
// The `image` field can be a string (URL or data URI) OR an object with
// a `url` key — both are covered. If `mediaType` is present at the top
// level alongside raw base64, we synthesise a data URI.
func normalizeImageBlock(part gjson.Result) ([]byte, bool) {
	imageField := part.Get("image")
	if !imageField.Exists() {
		return nil, false
	}

	var url string
	switch {
	case imageField.Type == gjson.String:
		raw := imageField.String()
		if strings.HasPrefix(raw, "data:") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
			url = raw
		} else {
			// Bare base64 — caller must supply mediaType for us to wrap.
			mediaType := part.Get("mediaType").String()
			if mediaType == "" {
				mediaType = "image/png"
			}
			url = "data:" + mediaType + ";base64," + raw
		}
	case imageField.IsObject():
		if u := imageField.Get("url").String(); u != "" {
			url = u
		}
	}
	if url == "" {
		return nil, false
	}

	out := []byte(`{"type":"image_url","image_url":{"url":""}}`)
	out, _ = sjson.SetBytes(out, "image_url.url", url)
	return out, true
}

// normalizeAudioBlock converts AI-SDK-v5 {type:"audio", audio:{data,
// mediaType}} or {type:"input_audio", ...} variants into the canonical
// {type:"input_audio", input_audio:{data, format}} shape MiniMax and
// other OpenAI-compat audio-input providers expect.
//
// Format is derived from mediaType when present (audio/wav → "wav",
// audio/mpeg → "mp3", etc). If the source already has `format`, prefer it.
func normalizeAudioBlock(part gjson.Result) ([]byte, bool) {
	// Try both AI-SDK-v5 and partial-OpenAI shapes for the data payload.
	data := part.Get("audio.data").String()
	mediaType := part.Get("audio.mediaType").String()
	if data == "" {
		data = part.Get("input_audio.data").String()
	}
	if mediaType == "" {
		mediaType = part.Get("input_audio.format").String()
	}
	if data == "" {
		// Maybe `audio` is a bare base64 string + top-level mediaType.
		if s := part.Get("audio").String(); s != "" && part.Get("audio.data").Type == gjson.Null {
			data = s
			if mt := part.Get("mediaType").String(); mt != "" {
				mediaType = mt
			}
		}
	}
	if data == "" {
		return nil, false
	}

	format := audioFormatFromMediaType(mediaType)
	out := []byte(`{"type":"input_audio","input_audio":{"data":"","format":"wav"}}`)
	out, _ = sjson.SetBytes(out, "input_audio.data", data)
	out, _ = sjson.SetBytes(out, "input_audio.format", format)
	return out, true
}

// normalizeFileBlock handles AI-SDK-v5's generic {type:"file", data,
// mediaType} part. If the media is image/*, route through image_url; if
// audio/*, route through input_audio; otherwise leave alone (caller
// keeps the original block, which most providers will reject loudly).
func normalizeFileBlock(part gjson.Result) ([]byte, bool) {
	data := part.Get("data").String()
	mediaType := part.Get("mediaType").String()
	if data == "" {
		// Maybe `data` is a URL string under `url` instead.
		if u := part.Get("url").String(); u != "" && (strings.HasPrefix(u, "http") || strings.HasPrefix(u, "data:")) {
			if strings.HasPrefix(mediaType, "audio/") {
				out := []byte(`{"type":"input_audio","input_audio":{"data":"","format":"wav"}}`)
				out, _ = sjson.SetBytes(out, "input_audio.data", u)
				out, _ = sjson.SetBytes(out, "input_audio.format", audioFormatFromMediaType(mediaType))
				return out, true
			}
			out := []byte(`{"type":"image_url","image_url":{"url":""}}`)
			out, _ = sjson.SetBytes(out, "image_url.url", u)
			return out, true
		}
		return nil, false
	}

	switch {
	case strings.HasPrefix(mediaType, "image/"):
		url := "data:" + mediaType + ";base64," + data
		out := []byte(`{"type":"image_url","image_url":{"url":""}}`)
		out, _ = sjson.SetBytes(out, "image_url.url", url)
		return out, true
	case strings.HasPrefix(mediaType, "audio/"):
		out := []byte(`{"type":"input_audio","input_audio":{"data":"","format":"wav"}}`)
		out, _ = sjson.SetBytes(out, "input_audio.data", data)
		out, _ = sjson.SetBytes(out, "input_audio.format", audioFormatFromMediaType(mediaType))
		return out, true
	}
	return nil, false
}

// audioFormatFromMediaType maps an IANA media type to the short format
// string OpenAI's input_audio block expects (wav / mp3 / opus / flac).
// Falls back to "wav" since that's the most universally accepted by
// OpenAI-compat audio-input upstreams.
func audioFormatFromMediaType(mt string) string {
	switch strings.ToLower(strings.TrimSpace(mt)) {
	case "audio/wav", "audio/x-wav", "audio/wave", "wav":
		return "wav"
	case "audio/mpeg", "audio/mp3", "mp3":
		return "mp3"
	case "audio/opus", "opus":
		return "opus"
	case "audio/flac", "flac":
		return "flac"
	case "audio/ogg", "audio/vorbis", "ogg":
		return "ogg"
	case "audio/webm", "webm":
		return "webm"
	default:
		return "wav"
	}
}
