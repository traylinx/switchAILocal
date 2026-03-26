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
