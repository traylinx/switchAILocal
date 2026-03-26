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

---

## Authentication

All endpoints require a valid API key in the `Authorization` header:

```
Authorization: Bearer sk-your-key-here
```

API keys are configured in `config.yaml` under `access-management.api-keys`.
