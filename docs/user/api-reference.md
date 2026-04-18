# switchAILocal API Reference

## Base URL
```
http://localhost:18080/v1
```

All requests require an `Authorization: Bearer <api-key>` header matching a key in `config.yaml`.

---

## Endpoints

### Chat Completions

```
POST /v1/chat/completions
```

Standard OpenAI chat completions. Supports streaming (`"stream": true`).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Model name (e.g., `gemini-2.5-pro`, `ollama:llama3.2`) |
| `messages` | array | Yes | Array of message objects |
| `stream` | boolean | No | Enable SSE streaming |
| `tools` | array | No | Tool definitions. `[{"type":"web_search"}]` is supported natively by `minimax:MiniMax-M2.7` — see Web Search below. |

#### Web Search (MiniMax native tool)

`minimax:MiniMax-M2.7` supports a built-in `web_search` tool that performs live web lookups and folds the results into the response. Enable it via the `tools` field:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:MiniMax-M2.7",
    "messages": [{"role": "user", "content": "Who won the 2026 Super Bowl?"}],
    "max_tokens": 2000,
    "tools": [{"type": "web_search"}]
  }'
```

**Important:** web search inflates the prompt context with search results (often 6k–13k prompt tokens per query). Set `max_tokens >= 2000` to leave headroom for reasoning + final answer, otherwise the response will be truncated or empty. The response returns the final answer in `choices[0].message.content` — there are no separate `tool_calls` for the client to handle; the model calls search internally.

### Completions

```
POST /v1/completions
```

Legacy completions endpoint.

### Models

```
GET /v1/models
```

List all available models. Supports modality filtering.

| Query Param | Values | Description |
|-------------|--------|-------------|
| `modality` | `text`, `image`, `audio`, `embedding`, `vision` | Filter models by capability |

Response includes a `capabilities` array per model (e.g., `["text"]`, `["image"]`, `["audio"]`).

### Embeddings

```
POST /v1/embeddings
```

Generate vector embeddings for input text.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Embedding model (e.g., `switchai-embed`) |
| `input` | string/array | Yes | Text to embed |

### Image Generation

```
POST /v1/images/generations
```

Generate images from text prompts. Supports `application/json` and `multipart/form-data`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Image model (e.g., `dall-e-3`) |
| `prompt` | string | Yes | Image description |
| `size` | string | No | Image size (e.g., `1024x1024`) |
| `n` | integer | No | Number of images |

> [!NOTE]
> **Provider Quirks**: For the `minimax` provider, switchAILocal automatically rewrites the upstream path from `/v1/images/generations` to their native `/v1/image_generation` to maintain OpenAI compatibility.

### Image Editing

```
POST /v1/images/edits
```

Edit existing images. Supports `application/json` (image URLs) and `multipart/form-data` (binary upload).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Image model |
| `prompt` | string | Yes | Edit instructions |
| `image` | file/url | Yes | Source image |
| `mask` | file/url | No | Edit mask |

### Text-to-Speech

```
POST /v1/audio/speech
```

Convert text to speech audio.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | TTS model (e.g., `tts-1`) |
| `input` | string | Yes | Text to speak |
| `voice` | string | Yes | Voice (e.g., `alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`) |
| `response_format` | string | No | `mp3` (default), `opus`, `aac`, `flac`, `wav`, `pcm` |

Response is binary audio with appropriate `Content-Type`.

#### MiniMax TTS — working example

MiniMax `speech-02-hd` is the default `speech` model in `intelligence.matrix`. The gateway runs an adapter (`internal/runtime/executor/minimax_tts.go`) that translates the OpenAI-shape request into MiniMax's native `/v1/t2a_pro` API and resolves the returned audio URL to raw bytes before handing them back.

```bash
curl http://localhost:18080/v1/audio/speech \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:speech-02-hd",
    "input": "Hello from switchAILocal",
    "voice": "male-qn-qingse",
    "response_format": "mp3"
  }' \
  --output hello.mp3
```

**Voice IDs are MiniMax-native** — OpenAI voice names (`alloy`, `echo`, `fable`, `onyx`, `nova`, `shimmer`) are not aliased. Sample MiniMax voices:

| Voice ID | Description |
|---|---|
| `male-qn-qingse` | Male, calm (default for testing) |
| `female-shaonv` | Female, young |
| `audiobook_male_2` | Male, narrator-style |
| `presenter_male` | Male, anchor-style |
| `clever_boy` | Male, youthful |

**Supported formats:** `mp3` (default), `pcm`, `flac`, `wav`. Bitrate and sample rate can be overridden via top-level `bitrate`, `audio_sample_rate`, `channel`.

**Plan limits:** MiniMax Plus token plan allows ~9000 characters/day and imposes a very tight RPM (1–5 requests/min). When the RPM is exceeded, MiniMax returns internal code `1002` which the adapter maps to HTTP 429 (rate_limit) — the failover taxonomy then classifies this as `ClassRateLimit` and advances to the next provider if a fallback chain is configured. Internal code `2061` ("plan not support") maps to HTTP 402 (out_of_credits).

**Known error codes (internal MiniMax → HTTP):**

| MiniMax code | HTTP | Meaning | Failover class |
|---|---|---|---|
| 0 | 200 | success | — |
| 1002 | 429 | RPM rate limit | ClassRateLimit (advances) |
| 1004 / 1008 | 401 | auth / balance | ClassAuth (advances) |
| 2013 | 400 | invalid params | ClassPermanent (aborts) |
| 2061 | 402 | plan doesn't support this model | ClassOutOfCredits (advances) |

### Music Generation

```
POST /v1/music/generations
```

Generate music from lyrics using MiniMax `music-2.6` (text-to-music) or `music-cover` (reference-audio style transfer). Synthesis runs synchronously by default and typically takes 30–90 seconds for a 1–2 minute song; the sync response hex-decodes the upstream audio and returns base64-encoded MP3 bytes with lifted metadata. Pass `stream: true` to get raw MP3 bytes streamed progressively (see **Streaming mode** below).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | No | `minimax:music-2.6` (default via `intelligence.matrix.music_generation`) or `minimax:music-cover` |
| `stream` | bool | No | `true` streams raw `audio/mpeg` bytes as they arrive (~20s TTFB vs ~60s sync); default `false` returns JSON with base64 audio |
| `lyrics` | string | Yes for `music-2.6` | Lyrics with optional structure tags: `[Intro]`, `[Verse]`, `[Chorus]`, `[Bridge]`, `[Outro]`, `[Inst]` |
| `prompt` | string | Yes for `music-cover` (10–300 chars) | Style description for the cover |
| `audio_url` OR `audio_base64` | string | Yes for `music-cover` | Reference audio: 6s–6min, ≤50MB, mp3/wav/flac. Mutually exclusive. |

**Response shape:**
```json
{
  "model": "music-2.6",
  "trace_id": "063294b934ecf975cf75a296dc3cc3f4",
  "data": {
    "audio": "<base64-encoded MP3 bytes>",
    "format": "mp3",
    "size_bytes": 3150729,
    "duration_ms": 98298,
    "sample_rate": 44100,
    "channels": 2,
    "bitrate": 256000
  },
  "extra_info": { /* raw upstream metadata */ }
}
```

**Text-to-music example:**
```bash
curl http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:music-2.6",
    "lyrics": "[Verse]\nCode flows through the night\n[Chorus]\nDebugging makes it right"
  }' | jq -r .data.audio | base64 -d > song.mp3
```

**Style-cover example (reference audio):**
```bash
curl http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:music-cover",
    "prompt": "upbeat jazz cover with saxophone solo",
    "audio_url": "https://example.com/reference.mp3"
  }' | jq -r .data.audio | base64 -d > cover.mp3
```

**Plan limits:** MiniMax Plus plan = 100 songs/day each for `music-2.6` and `music-cover`. Max song length 6 minutes. Errors follow the same mapping as TTS — `2013` (invalid_params) aborts, `1002` (rate_limit) and `2061` (plan_not_support) are advance-eligible.

**Streaming mode (`stream: true`):**

Instead of JSON with a base64 audio blob, the server returns `Content-Type: audio/mpeg` with raw MP3 bytes streamed as they arrive from upstream. Measured wins vs sync mode (verified 2026-04-18 against live MiniMax):

- **TTFB ≈ 20s** vs 30–90s sync — client can start playback before MiniMax finishes generating
- **~50% less wire bandwidth** — upstream's SSE stream includes a terminal frame that duplicates the full song as hex; the adapter discards it and only forwards progressive chunks
- **Natively playable** — the first chunk carries the ID3v2 header, so concatenated bytes form a valid MP3 that any audio element can decode while downloading

```bash
curl -N http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:music-2.6",
    "stream": true,
    "lyrics": "[Verse]\nCode flows through the night\n[Chorus]\nDebugging makes it right"
  }' > song.mp3
```

```html
<!-- Browser: play while downloading -->
<audio src="/v1/music/generations" controls></audio>
<!-- send the request via fetch() with POST body then feed response.body to MediaSource -->
```

Pre-first-byte errors (HTTP 5xx, 429, `2061` plan-not-support, upstream stall within 60s) return a JSON error with the appropriate status code. Errors that arrive AFTER bytes have been flushed close the stream cleanly — the client keeps whatever MP3 frames it received, which are still playable.

### Lyrics Generation

```
POST /v1/music/lyrics
```

Generate structured song lyrics. Typical latency 2–5 seconds. Pure JSON in / JSON out — no audio.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `mode` | string | No | `write_full_song` (default) or `edit` (modify/continue existing `lyrics`) |
| `prompt` | string | No | Song theme / style description (max 2000 chars). Empty = random. |
| `lyrics` | string | Only in `edit` mode | Existing lyrics to extend (max 3500 chars) |
| `title` | string | No | Desired song title |

**Response shape:**
```json
{
  "song_title": "Midnight Code Delight",
  "style_tags": "pop, happy, coding, late night, electronic",
  "lyrics": "[Intro]\n\n[Verse]\nScreen glow bright...",
  "base_resp": { "status_code": 0, "status_msg": "success" }
}
```

**Example:**
```bash
curl http://localhost:18080/v1/music/lyrics \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "mode": "write_full_song",
    "prompt": "a happy pop song about sunshine"
  }'
```

**Common workflow:** call `/v1/music/lyrics` first to generate structured lyrics, then feed the result into `/v1/music/generations` with `model: "minimax:music-2.6"`.

**Plan limits:** 100 lyrics/day on the Plus plan.

### Audio Transcription

```
POST /v1/audio/transcriptions
```

Transcribe audio to text. Requires `multipart/form-data`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Transcription model (e.g., `whisper-1`) |
| `file` | file | Yes | Audio file (mp3, mp4, mpeg, mpga, m4a, wav, webm) |
| `language` | string | No | ISO-639-1 language code |
| `response_format` | string | No | `json`, `text`, `srt`, `verbose_json`, `vtt` |

### Audio Translation

```
POST /v1/audio/translations
```

Translate audio to English text. Requires `multipart/form-data`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `model` | string | Yes | Translation model (e.g., `whisper-1`) |
| `file` | file | Yes | Audio file in any language |
| `response_format` | string | No | `json`, `text`, `srt`, `verbose_json`, `vtt` |

### Providers

```
GET /v1/providers
```

List all configured AI providers.

| Query Param | Values | Description |
|-------------|--------|-------------|
| `filter` | `active`, `inactive`, `all` | Filter by provider status |

### Model Refresh

```
POST /v1/models/refresh
```

Trigger re-discovery of available models from all providers.

| Query Param | Description |
|-------------|-------------|
| `provider` | Refresh specific provider only |

### Metrics & Observability

```
GET /metrics
```

Prometheus-compatible metrics endpoint.

Exposes standard Go process metrics along with `switchAILocal`-specific telemetry:
- `switchailocal_requests_total`: Total requests by model, provider, status, and routing type.
- `switchailocal_request_duration_milliseconds`: Request latency histogram.
- `switchailocal_llm_tokens_total`: Input and output token consumption.
- `switchailocal_routing_quality_score`: Routing Quality Score (RQS) tracking.
- `switchailocal_fallbacks_total`: Failover/fallback tracking by model.

*Note: This endpoint is served on the root level (`/metrics`), not under `/v1/`.*

### Management Dashboard

```
GET /v0/management/observability/dashboard
```

Returns real-time proxy health and Go runtime statistics (heap, GC, goroutines). Requires your `management-secret-key`.

📖 **See the [Performance & Production Guide](performance_guide.md)** for details on tuning rate limits, load shedding, and observability.

---

## Advanced Features

All advanced parameters are passed through transparently to the upstream provider. switchAILocal doesn't modify or filter these fields.

### Tool Calling

#### Provider-Specific Tools (Web Search)

```json
{
  "model": "xiaomi:mimo-v2-flash",
  "messages": [{"role": "user", "content": "Latest AI news?"}],
  "tools": [{"type": "web_search", "max_keyword": 3, "force_search": true, "limit": 3}],
  "tool_choice": "auto"
}
```

#### OpenAI Function Calling

```json
{
  "model": "gemini-2.5-pro",
  "messages": [{"role": "user", "content": "Weather in Berlin?"}],
  "tools": [{
    "type": "function",
    "function": {
      "name": "get_weather",
      "description": "Get weather for a location",
      "parameters": {
        "type": "object",
        "properties": {
          "location": {"type": "string"},
          "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
        },
        "required": ["location"]
      }
    }
  }],
  "tool_choice": "auto"
}
```

### Thinking / Reasoning Mode

```json
{
  "model": "claudecli:claude-sonnet-4",
  "messages": [{"role": "user", "content": "Prove sqrt(2) is irrational"}],
  "thinking": {"type": "enabled", "budget_tokens": 10000}
}
```

### Vision (Multimodal Content)

```json
{
  "model": "geminicli:gemini-2.5-pro",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "text", "text": "What do you see?"},
      {"type": "image_url", "image_url": {"url": "https://example.com/photo.jpg"}}
    ]
  }]
}
```

### CLI Attachments

Pass local files and folders to CLI providers:

```json
{
  "model": "geminicli:gemini-2.5-pro",
  "messages": [{"role": "user", "content": "Review this code"}],
  "extra_body": {
    "cli": {
      "files": ["/path/to/main.go"],
      "directories": ["/path/to/internal/"]
    }
  }
}
```

### Streaming

Set `"stream": true` to receive Server-Sent Events:

```json
{
  "model": "gemini-2.5-pro",
  "messages": [{"role": "user", "content": "Tell me a story"}],
  "stream": true
}
```

---

## Authentication

All endpoints require a valid API key in the `Authorization` header:

```
Authorization: Bearer sk-your-key-here
```

API keys are configured in `config.yaml` under `access-management.api-keys`.
