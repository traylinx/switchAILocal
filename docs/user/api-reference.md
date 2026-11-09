# switchAILocal API Reference: Route Index

This page lists every HTTP route switchAILocal registers (release 0.6.x). Full request and response details live in the docs site under [`docs-site/api/`](../../docs-site/api/). The **Page** column links to the page for each route.

## Basics

- **Base URL:** `http://localhost:18080` (port from `config.yaml`; the default `host: ""` binds all interfaces).
- **Client auth:** keys from top-level `api-keys:` in `config.yaml`, sent as `Authorization: Bearer <key>` (also accepted: `X-Api-Key`, `X-Goog-Api-Key`, `?key=`, `?auth_token=`). **An empty `api-keys` list means every request is accepted.** See [`api/authentication.mdx`](../../docs-site/api/authentication.mdx).
- **Management auth:** `remote-management.secret-key` or `MANAGEMENT_PASSWORD`, sent as `Authorization: Bearer <key>` or `X-Management-Key`. Client keys do not work here. Never expose `/v0/management` publicly.
- **Errors:** `{"error":{"message","type","code"}}`. The auth middleware returns `401 {"error":"Missing API key"}` or `{"error":"Invalid API key"}`.
- **Streaming:** SSE (`text/event-stream`). Upstream failures before the first chunk return a JSON error with the real status.

## Inference and data routes (client API key)

| Method | Path | Purpose | Page |
|---|---|---|---|
| GET | `/v1/models` | Public model catalog (`?modality=`); Anthropic shape for `claude-cli` user agents | [`api/models.mdx`](../../docs-site/api/models.mdx) |
| POST | `/v1/models/refresh` | Re-run model discovery (`?provider=`) | [`api/models.mdx`](../../docs-site/api/models.mdx) |
| GET | `/v1/providers` | Providers with registered models (`?filter=`) | [`api/models.mdx`](../../docs-site/api/models.mdx) |
| POST | `/v1/chat/completions` | OpenAI chat completions, SSE streaming | [`api/chat-completions.mdx`](../../docs-site/api/chat-completions.mdx) |
| POST | `/v1/completions` | Legacy text completions (wrapped as chat) | [`api/chat-completions.mdx`](../../docs-site/api/chat-completions.mdx) |
| POST | `/v1/responses` | OpenAI Responses API format | [`api/responses.mdx`](../../docs-site/api/responses.mdx) |
| POST | `/v1/messages` | Anthropic Messages API | [`api/messages.mdx`](../../docs-site/api/messages.mdx) |
| POST | `/v1/messages/count_tokens` | Anthropic token counting | [`api/messages.mdx`](../../docs-site/api/messages.mdx) |
| POST | `/v1/embeddings` | Embeddings (Ollama and OpenAI-compatible providers) | [`api/embeddings.mdx`](../../docs-site/api/embeddings.mdx) |
| POST | `/v1/images/generations` | Image generation (upstream response shape) | [`api/images.mdx`](../../docs-site/api/images.mdx) |
| POST | `/v1/images/edits` | Image edits, JSON or multipart | [`api/images.mdx`](../../docs-site/api/images.mdx) |
| POST | `/v1/audio/speech` | Text-to-speech, binary audio | [`api/audio.mdx`](../../docs-site/api/audio.mdx) |
| POST | `/v1/audio/transcriptions` | Speech-to-text, multipart | [`api/audio.mdx`](../../docs-site/api/audio.mdx) |
| POST | `/v1/audio/translations` | Audio to English text, multipart | [`api/audio.mdx`](../../docs-site/api/audio.mdx) |
| POST | `/v1/music/generations` | MiniMax text-to-music / cover; `stream:true` returns `audio/mpeg` | [`api/music.mdx`](../../docs-site/api/music.mdx) |
| POST | `/v1/music/lyrics` | MiniMax lyrics generation | [`api/music.mdx`](../../docs-site/api/music.mdx) |
| POST | `/v1/notifications/telegram/sendMessage` | Telegram relay (needs `notifications.telegram.enabled`) | [`api/notifications.mdx`](../../docs-site/api/notifications.mdx) |
| GET | `/openai/v1/models` | Same as `/v1/models` | [`api/overview.mdx`](../../docs-site/api/overview.mdx) |
| POST | `/openai/v1/chat/completions`, `/openai/v1/completions`, `/openai/v1/embeddings` | Same as `/v1` | [`api/overview.mdx`](../../docs-site/api/overview.mdx) |
| POST | `/openai/v1/images/generations`, `/openai/v1/images/edits` | Images with the response normalized to OpenAI `data[]` | [`api/images.mdx`](../../docs-site/api/images.mdx) |
| GET | `/v1beta/models` | Gemini-format model list | [`api/gemini.mdx`](../../docs-site/api/gemini.mdx) |
| GET | `/v1beta/models/{model}` | Static metadata for a few fixed IDs | [`api/gemini.mdx`](../../docs-site/api/gemini.mdx) |
| POST | `/v1beta/models/{model}:{generateContent\|streamGenerateContent\|countTokens}` | Gemini native API | [`api/gemini.mdx`](../../docs-site/api/gemini.mdx) |
| GET | `/v1/ws` | WebSocket relay; the client acts as a provider (auth when `ws-auth: true`) | [`api/websocket.mdx`](../../docs-site/api/websocket.mdx) |

## Unauthenticated routes

| Method | Path | Purpose | Page |
|---|---|---|---|
| GET | `/` | Banner JSON | [`api/system.mdx`](../../docs-site/api/system.mdx) |
| GET | `/health` | `{"status":"ok"}` | [`api/system.mdx`](../../docs-site/api/system.mdx) |
| GET | `/metrics` | Prometheus metrics (only when `observability.metrics.enabled`; path configurable) | [`api/system.mdx`](../../docs-site/api/system.mdx) |
| GET | `/management`, `/management.html` | Control panel HTML (its API calls need the management key) | [`api/system.mdx`](../../docs-site/api/system.mdx) |
| GET | `/keep-alive` | Only when an embedder enables it; local-password check | [`api/system.mdx`](../../docs-site/api/system.mdx) |
| POST | `/v1internal:{method}` | Gemini CLI bridge; `127.0.0.1` peers only | [`api/gemini.mdx`](../../docs-site/api/gemini.mdx) |
| GET | `/anthropic/callback`, `/codex/callback`, `/google/callback`, `/iflow/callback`, `/antigravity/callback` | OAuth browser redirects | [`api/management/providers.mdx`](../../docs-site/api/management/providers.mdx) |

## Amp CLI routes (client API key)

| Method | Path | Page |
|---|---|---|
| GET/POST | `/api/provider/{provider}/[v1/]{models,chat/completions,completions,responses}`, `/api/provider/{provider}/v1/messages[/count_tokens]`, `/api/provider/{provider}/v1beta/models[/*]` | [`api/amp.mdx`](../../docs-site/api/amp.mdx) |
| any | `/api/{internal,user,auth,meta,ads,telemetry,threads,otel,tab}[/*]`, `/api/provider/google/v1beta1/*` (proxied; needs `ampcode.upstream-url`) | [`api/amp.mdx`](../../docs-site/api/amp.mdx) |
| GET / any | `/threads[/*]`, `/docs[/*]`, `/settings[/*]`, `/auth[/*]` (proxied; no API key), `/threads.rss`, `/news.rss` (proxied; API key) | [`api/amp.mdx`](../../docs-site/api/amp.mdx) |

## Management routes (`/v0/management`, management key)

| Methods | Paths | Page |
|---|---|---|
| GET; POST (no key, localhost only) | `/setup-status`; `/initialize`, `/skip`, `/reset` | [`api/management/overview.mdx`](../../docs-site/api/management/overview.mdx) |
| GET; GET, PUT; GET | `/config`; `/config.yaml`; `/latest-version` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET, PUT, PATCH | `/debug`, `/logging-to-file`, `/usage-statistics-enabled`, `/request-log`, `/ws-auth`, `/request-retry`, `/max-retry-interval`, `/quota-exceeded/switch-project`, `/quota-exceeded/switch-preview-model` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET, PUT, PATCH, DELETE | `/proxy-url` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET, DELETE; GET | `/logs`; `/request-error-logs`, `/request-error-logs/:name`, `/request-log-by-id/:id` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET | `/usage`, `/metrics`, `/observability/dashboard`, `/autoroute/status`, `/autoroute/journal` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET; POST | `/memory/stats`, `/heartbeat/status`, `/steering/rules`, `/hooks/status`, `/analytics`; `/steering/reload`, `/hooks/reload` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET; POST | `/skills`, `/feedback/stats`, `/feedback/recent`, `/cache/metrics`; `/feedback`, `/cache/clear` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET | `/state-box/status` (only when a state box is configured) | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET; GET, PUT, PATCH, DELETE; GET, PUT, PATCH | `/ampcode`; `/ampcode/upstream-url`, `/ampcode/upstream-api-key`, `/ampcode/model-mappings`; `/ampcode/restrict-management-to-localhost`, `/ampcode/force-model-mappings` | [`api/management/config.mdx`](../../docs-site/api/management/config.mdx) |
| GET, PUT, PATCH, DELETE | `/api-keys`, `/gemini-api-key`, `/claude-api-key`, `/codex-api-key`, `/switchai-api-key`, `/openai-compatibility`, `/oauth-excluded-models` | [`api/management/providers.mdx`](../../docs-site/api/management/providers.mdx) |
| POST | `/providers/test`, `/providers/discover-models` | [`api/management/providers.mdx`](../../docs-site/api/management/providers.mdx) |
| GET, POST, DELETE; GET; POST | `/auth-files`; `/auth-files/models`, `/auth-files/download`; `/vertex/import` | [`api/management/providers.mdx`](../../docs-site/api/management/providers.mdx) |
| GET; POST; GET | `/anthropic-auth-url`, `/codex-auth-url`, `/gemini-cli-auth-url`, `/antigravity-auth-url`, `/qwen-auth-url`, `/iflow-auth-url`; `/iflow-auth-url`, `/oauth-callback`; `/get-auth-status` | [`api/management/providers.mdx`](../../docs-site/api/management/providers.mdx) |

## Model selection, in one paragraph

The `model` field picks the backend. The gateway tries, in order:

1. a recognized `provider:` prefix (`geminicli`, `claudecli`, `vibecli`, `codex`, `ollama`, `opencode`, `gemini`, `claude`, `switchai`, `openai`, `openai-compat`);
2. a Lua `on_request` hook;
3. a virtual model (for example `ail-compound`);
4. `auto` / `auto:<intent>` auto-routing;
5. an exact registry match on model IDs and configured aliases.

Anything else returns `400 unknown provider for model <id>`. Details: [`api/provider-prefixes.mdx`](../../docs-site/api/provider-prefixes.mdx), [`api/auto-routing.mdx`](../../docs-site/api/auto-routing.mdx). Local CLI options (`extra_body.cli`) are in [`api/cli-attachments.mdx`](../../docs-site/api/cli-attachments.mdx).
