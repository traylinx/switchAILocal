# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## v0.5.16 - 2026-05-12

- Fix `/v1/music/generations` routing for documented MiniMax music model names (`minimax:music-2.6`, `music-2.6`, `minimax:music-cover`, `music-cover`). These now route through the configured `ail-music` / `ail-music-cover` aliases before the adapter normalizes the upstream payload.
- Keep operator switching intact: clients may still send `ail-music`, `ail-music-cover`, or the documented MiniMax names; no JULI3TA hardcoding required.

## v0.5.15 - 2026-05-12

- Fix MiniMax music/lyrics/cover transport flaps: retry one `unexpected EOF`/connection reset inside the MiniMax JSON adapter before surfacing an error.
- Map exhausted MiniMax transport failures to HTTP 502 instead of raw status=0/500 so clients see a retryable gateway failure.
- Classify `unexpected EOF` as transient in the failover taxonomy for any future path that sees it outside the MiniMax adapter.

## [Unreleased]

### Fixed
- **MiniMax `music-cover` preprocessing fallback** — when upstream rejects a one-step cover request with `Cover mode requires dtw_result, beat_result, and audio_duration`, switchAILocal now calls `/music_cover_preprocess`, carries the returned `cover_feature_id` into `/music_generation`, and retries without raw audio. This restores JULI3TA/Tytus Restyle Song flows against current MiniMax cover-mode validation.

### Added
- **Discovery-driven autoinject (Phase 2 of 2026-04-22 native-tool-discovery sprint)** — `autoInjectWebSearch` now reads the target model's `native_tools` from the global registry (populated from `config.yaml` at startup) and injects each declared entry into the outgoing `tools[]`, superseding the legacy `AIL_AUTOINJECT_MODELS` env-var allowlist. Operators who populate `openai-compatibility.*.models[].native_tools` in their config no longer need the env-var allowlist — discovery drives the injection surface. Legacy env-var path preserved verbatim as a fallback for operators who haven't migrated their config yet, and the 2026-04-21 `force_search` threshold behavior continues to apply on the discovery path for agentic callers with crowded function-tool menus. Caller-declared tools are preserved byte-for-byte via the existing dedupe (a future OpenClaw version that calls `/v1/models` and sends its own `{type:"web_search"}` will hit dedupe and autoinject becomes a silent no-op — see dev/NATIVE_TOOLS_OPENCLAW_CONTRACT.md in wannolot-infrastructure for the pod-local-plugin fallback option).
- **Provider-native tool discovery via `/v1/models`** — each entry under `.data[]` now carries a `native_tools` array declaring the provider-native tools the upstream model supports (currently: MiniMax M2.7's `{"type":"web_search"}`). Operators declare the surface under `openai-compatibility.*.models[].native_tools` in the YAML config; the field is surfaced unmodified on the OpenAI-compatible `/v1/models` response. Agentic callers (OpenClaw's native-tools-discovery plugin, Hermes, custom SDKs) splice those entries into their caller `tools[]` at chat-completion time so MiniMax picks web_search as a first-class declared tool — no server-side autoinject hack required for the primary agentic path. Autoinject (see below) remains as a fallback for SDKs that don't consume `/v1/models`. Shape: `{type, description?, params?}` per entry — matches the OpenAI tools[] wire format so callers splice directly without translation. Empty / absent = no native tools for that model; the key is omitted entirely rather than emitted as null. See `services/wannolot-infrastructure/dev/sprints/2026-04-22-native-tool-discovery.md` for the full Phase 1-5 rollout plan.
- **`/v1/chat/completions` web_search auto-inject** (opt-in via env flag). When `AIL_AUTOINJECT_WEBSEARCH=true` and the request's model name is in `AIL_AUTOINJECT_MODELS` (comma-separated allowlist), the handler appends a `web_search` entry to the request's `tools` array before forwarding upstream. Motivation: OpenClaw agents on Tytus pods use `ail-compound` (→ `minimax:MiniMax-M2.7`) and the native web_search tool is autonomous by default, but OpenClaw's `openai` plugin never sends the `tools` field — so recent-events queries ("Who won Super Bowl LIX?") returned stale training-data answers. See `services/wannolot-infrastructure/docs/WEBSEARCH-AUDIT-2026-04-21.md` for the full audit.
- **Threshold-triggered `force_search`**: when the caller's tools array already contains ≥ `AIL_AUTOINJECT_FORCE_THRESHOLD` (default 5) entries with `type:"function"`, the injected entry is stamped with `force_search:true` to beat MiniMax's function-tool preference heuristic for agentic callers like OpenClaw. Callers below the threshold get the bare `{"type":"web_search"}` autonomous form (MiniMax decides per-query) so simple API clients don't burn search tokens for "what is 2+2". Set `AIL_AUTOINJECT_FORCE_THRESHOLD=0` to disable force_search stamping entirely; unparseable / negative values fall back to the default.
- **Set-union semantics with dedupe** — caller `tools` are preserved. A `web_search` entry is appended only if none exists. A caller's parameterized version (`{type:"web_search",max_keyword:5,force_search:true}`) is never clobbered by a bare append.
- **Per-request opt-out** via `X-Ail-Autoinject: off` header.
- **`max_tokens` safety floor** of 2000 when injection fires — MiniMax web_search inflates prompt context by 6–13k tokens; a low `max_tokens` would truncate the final answer. Only bumped when injection fires; never when the flag is off or the caller opted out.
- **`X-Ail-Build: <commit-sha>-<hostname>` response header** on every `/v1/chat/completions` response, so multi-instance LB fleets can verify each of the N instances is running the expected sha before flipping autoinject on.
- **`AIL_DEBUG_DUMP=true`** env flag — logs the raw chat-completions request body at handler entry. Off by default. Used in Phase 1 evidence capture for the 2026-04-21 sprint; kept as a permanent debug affordance.

### Scope notes
- Only the OpenAI handler (`/v1/chat/completions`) is covered. Anthropic (`/v1/messages`) and Gemini (`/v1beta/models/...`) adapters have the same structural gap and need a follow-up sprint — a proper Anthropic fix also needs tool-shape translation (`web_search_20250305` → `web_search`), not just injection.

## [0.4.1] - 2026-04-19

Capability bridge fix — matrix-aliased models now expose vision/audio in the structured fields.

### Fixed
- Models whose IDs are operator-aliased via `intelligence.matrix` (typical: `ail-compound`, `ail-image`, per-tenant aliases) showed `vision/audio: null, attachment: false` in the new structured fields even when the matrix declared `["text","vision","audio"]` in the legacy `capabilities` array. Result: AI-SDK clients still stripped images for those models because they read the new shape, not the legacy array. Now the handler bridges the matrix-derived strings into the structured fields (additive — never downgrades). Live-confirmed against `ail-compound` on prod.
- Heuristic for image vs vision: matrix `["image"]` alone for an ID containing "image" is treated as image-OUT (generation, e.g. `ail-image`), not vision-IN — prevents clients from trying to send image bytes to a generation endpoint.

### Added
- 8 bridge tests covering vision-from-matrix, image-out-vs-vision-in, additive-only, typed-modalities, and the `stringsFromAny` helper.

### Not changed
- Capability inference for ID-recognised models (MiniMax M2.x, GPT-4o, Claude 4.x, Gemini 2.x) — already correct in v0.4.0.
- Multimodal request normalizer — unchanged.

## [0.4.0] - 2026-04-19

Multimodal interop — capability discovery + request normalization across client formats.

### Added
- **Per-model capability metadata in `/v1/models`**: every model now advertises `attachment` (file uploads), `tool_call`, `reasoning`, and `modalities: {input, output}` plus flat-friendly `vision` / `audio` flags. Vercel AI SDK clients (OpenCode / Cursor / Continue.dev) auto-discover vision support and stop stripping image bytes client-side. Capability inference table in `internal/registry/model_registry.go` recognises MiniMax M2.x, Claude 3.5+/4.x, GPT-4o/5/o1/o3, Gemini 1.5+, Whisper, embedding/TTS/image-gen families. Operators can override per-model via the new `Capabilities` field on `ModelInfo`.
- **Multimodal request normalizer** (`internal/runtime/executor/multimodal_normalizer.go`): rewrites non-canonical content blocks emitted by AI-SDK-v5 clients into the canonical OpenAI shape every upstream provider accepts.
  - `{type:"image", image:"<url|data-uri>"}` → `{type:"image_url", image_url:{url:"..."}}`
  - `{type:"image", image:"<base64>", mediaType:"image/png"}` → wrapped data URI
  - `{type:"image", image:{url:"..."}}` → flat `image_url`
  - `{type:"audio", audio:{data, mediaType}}` → `{type:"input_audio", input_audio:{data, format}}` with mediaType→format mapping (wav/mp3/opus/flac/ogg/webm)
  - `{type:"file", data, mediaType}` → routed to `image_url` for image/* and `input_audio` for audio/*
  - `{type:"file", url:"..."}` → forwarded as `image_url` URL
- **Gemini fileData translator support**: `{fileData: {mimeType, fileUri}}` parts in incoming Gemini-format requests now translate to OpenAI `image_url` with the URL forwarded to upstream (was silently dropped before). Both `messages` and `contents` paths covered.
- 16 new normalizer tests + 32 capability inference cases.

### Why this matters
The first multimodal failure of v0.3.x was OpenCode dropping screenshots silently — the model said "I can't see images" because the image bytes never left the client. Root cause: OpenCode's `attachment` flag defaults to false unless the model declares vision support. With capability metadata in `/v1/models` plus the request normalizer, any client format that arrives at the gateway gets translated to what the upstream expects, and any vision-capable model auto-advertises so clients don't strip preemptively.

### Changed
- `OpenAICompatExecutor.Execute` and `ExecuteStream` invoke `NormalizeMultimodalContent` immediately after `sdktranslator.TranslateRequest`. No-op on text-only or already-canonical payloads.
- `sdk/api/handlers/openai/openai_handlers.go` `OpenAIModels` filter preserves the new capability fields (`attachment`, `tool_call`, `reasoning`, `modalities`, `vision`, `audio`) instead of stripping them. The legacy `capabilities` string-array stays for backward compat.

### Verified live (2026-04-19)
- AI-SDK-v5 image block POSTed to `/v1/chat/completions` → log shows `multimodal normalizer: rewrote non-canonical content blocks` → upstream MiniMax received canonical `{"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}`.
- `/v1/models` returns `vision:true, audio:true, attachment:true, tool_call:true` for `minimax:MiniMax-M2.7`; correct per-modality flags for music/speech/image-gen/embedding models.

## [0.3.1] - 2026-04-18

CI pipeline fix — no runtime code changes.

### Fixed
- `build.yml` now triggers on version-tag pushes (`tags: ['v*']`) directly, independent of the `release:published` event. The `release:published` event is unreliable when the release is created by another workflow (`action-gh-release` in `release.yml`), which silently suppressed the Docker build for v0.3.0 — GHCR got `:latest` but no `:0.3.0` tag, breaking pin-based rollout to droplets. Going forward, every version tag push publishes `:X.Y.Z` reliably.
- `workflow_dispatch` now accepts a `push` input, enabling `gh workflow run build.yml --ref vX.Y.Z -f push=true` to backfill a missing tag without cutting a new release.

### Not changed
- Gateway binary, behaviour, API surface, dependencies — identical to v0.3.0. This release exists only to validate the CI fix end-to-end and unblock fleet-wide rollout.

## [0.3.0] - 2026-04-18

MiniMax music generation streaming — raw MP3 bytes flow as they're rendered.

### Added
- **Music streaming via `stream: true` on `POST /v1/music/generations`** — server unwraps MiniMax's upstream SSE (hex-encoded MP3 chunks), hex-decodes each progressive frame, and writes raw bytes to the client with `Content-Type: audio/mpeg` + `Transfer-Encoding: chunked`. The terminal frame (which duplicates the full song at 2× overhead) is discarded. Source: `internal/runtime/executor/minimax_music.go:executeMinimaxMusicStream`.
- **Measured wins vs sync mode** (verified 2026-04-18 against live MiniMax, 107-second song):
  - TTFB 19.6s (vs 30–90s sync). Wire overhead above upstream TTFB is ≈0s — the adapter adds no latency of its own.
  - Client bandwidth 3.4 MB (vs 6.8 MB if we passed the hex SSE through unmodified). 50% reduction is structural: half the upstream SSE payload is a terminal-frame duplicate the adapter drops.
  - Memory flat — no buffering of the full song; chunks stream through `bufio.Reader.ReadBytes` (handles arbitrary line length, unlike the 1 MB `bufio.Scanner` cap).
  - Client cancellation works end-to-end: disconnecting mid-stream cancels the upstream context and stops generation.
- **Music-tuned stall watchdog** (`minimax_music.go:minimaxMusicFirstByteMin` = 60s, `minimaxMusicStallMin` = 120s). The global `FirstByteTimeout: 15s` default is correct for chat token streams but kills MiniMax music — TTFB is legitimately ~20s. The adapter raises the floor locally; operators who set explicit higher values in `performance.streaming.*` still win.
- **`ExecuteStreamMultimodalWithAuthManager` handler helper** (`sdk/api/handlers/handlers.go`) — streaming counterpart to the existing `ExecuteMultimodalWithAuthManager`. Injects `operation` + `content_type` into opts metadata so executors can dispatch to a non-chat streaming path without re-implementing circuit-breaker / auto-routing plumbing.
- **5 new unit tests** (`internal/runtime/executor/minimax_music_test.go`): happy path (3 progressive chunks + terminal drop), pre-first-byte upstream error → retryable `statusErr`, mid-stream error after partial audio, HTTP 4xx synchronous error, stream-field stripping for the sync path. `sseWriter` fixture helper for future MiniMax-SSE tests.

### Changed
- `normaliseMinimaxMusicRequest` now strips `stream` from the payload on the sync path. Previously a client that sent `stream: true` to sync-mode would have reached upstream and crashed the JSON parser on the SSE bytes that came back. The streaming path re-injects it explicitly. (Latent bug fix — no behaviour change for existing callers.)
- `OpenAICompatExecutor.ExecuteStream` routes `operation == "music_generation"` to `executeMinimaxMusicStream` before the chat translation pipeline. Non-MiniMax providers return 501 Not Implemented with a clear error.

### Failover behaviour (unchanged contract)
- Pre-first-byte errors (HTTP 5xx, 429, upstream `base_resp.status_code != 0` in the first frame, stall within 60s): surfaced as typed `statusErr` on the stream channel → conductor's existing bootstrap retry classifies normally and advances to the next provider if one exists.
- Post-first-byte errors: client already has a playable partial MP3. Adapter logs the upstream error, closes the stream. No retry (can't retract bytes already sent).
- Watchdog-fired cancellations produce `*stallError` with the correct phase (`pre_first_byte` retryable, `mid_stream` terminal) via the existing `StallPhaser` interface.

### Known Issues (carried from v0.2.0)
- `NPM_TOKEN` secret still scoped too narrowly for CI auto-publish. Manual `npm publish` from `npx/switchailocal/` works around it; see `docs/user/release.md` if present or the 0.2.0 release notes.
- `internal/intelligence/embedding/TestIntegration` still fails in the full `go test ./...` run due to ONNX runtime CGO global-state leakage between tests. Passes in isolation; not introduced by this release.

## [0.2.0] - 2026-04-18

Intelligent failover + MiniMax speech/music/lyrics + built-in web search.

### Added
- **Failover recovery system**
  - 10-class error taxonomy in `internal/failover/classify.go`: `transient`, `rate_limit`, `auth`, `out_of_credits`, `context_length`, `permanent`, `empty_content`, `stall_pre_first_byte`, `stall_mid_stream`, `client_disconnect`.
  - Typed `*failover.FailoverError` wrapper preserving `errors.As` chains and `StatusCode()` interface contract.
  - Conductor-level advance/abort loop in `executeProvidersOnce` with structured `event=failover`, `event=failover_recovered`, `event=failover_abort` log lines (fields: `request_id`, `attempt`, `primary_provider`, `next_provider`, `error_class`, `http_status`, `latency_ms`, `error_snippet`).
  - `StallPhaser` interface so executors can signal watchdog stalls without import cycles.
- **Exponential backoff with jitter** (`internal/autoroute/health.go`) replaces the hardcoded 5-minute cooldown. Sequence 5s → 10s → 20s → … → 300s cap with ±10% jitter, `CooldownAttempts` resets after a successful request following recovery.
- **Per-provider timeout config** (`performance.provider-timeouts.{default, per-provider}`) applied to non-streaming requests across all executors.
- **Streaming stall watchdog** (`internal/runtime/executor/stream_watchdog.go`) — pre-first-byte timeout and mid-stream stall detection via `time.AfterFunc` + context cancel.
- **MiniMax TTS adapter** — `POST /v1/audio/speech` when the upstream provider is MiniMax now transparently routes to the provider's native `/v1/t2a_pro` endpoint. Adapter translates OpenAI-shape `{model, input, voice, response_format}` → MiniMax `{model, text, voice_id, format, …}`, posts, parses `base_resp`, fetches the returned `audio_file` URL (signed aliyun OSS), and streams raw audio bytes back with the correct `Content-Type`. Source: `internal/runtime/executor/minimax_tts.go`.
- **MiniMax music adapter** — new endpoint `POST /v1/music/generations` handles both text-to-music (`model: minimax:music-2.6`) and reference-audio style transfer (`model: minimax:music-cover` + `audio_url` or `audio_base64`). Hex-encoded upstream audio is decoded server-side and returned as base64 with metadata: `{data: {audio, format, size_bytes, duration_ms, sample_rate, channels, bitrate}, model, trace_id, extra_info}`. Source: `internal/runtime/executor/minimax_music.go`.
- **MiniMax lyrics adapter** — new endpoint `POST /v1/music/lyrics` with modes `write_full_song` (default) or `edit`, returns `{song_title, style_tags, lyrics}` with structure tags (`[Verse]`, `[Chorus]`, `[Bridge]`, etc.).
- **MiniMax error-code → HTTP-status mapping** so the failover taxonomy can classify application-level errors correctly: `1002` (RPM rate limit) → 429 (`ClassRateLimit`, advances), `1004/1008` (auth/balance) → 401 (`ClassAuth`, advances), `2013` (invalid params) → 400 (`ClassPermanent`, aborts), `2061` (plan not support) → 402 (`ClassOutOfCredits`, advances). Previously these came back as HTTP 200 with application-level error bodies and the failover system couldn't see them.
- **Built-in web search documentation** — `minimax:MiniMax-M2.7` supports MiniMax-native web search via `tools: [{"type": "web_search"}]` on `/v1/chat/completions`. Required `max_tokens >= 2000` documented (search inflates context to 6k–13k tokens).
- **New `intelligence.matrix` slots**: `web_search`, `music_generation`, `music_cover`, `lyrics_generation`.
- **Reproducible failover demo** (`scripts/demo-failover.sh`) — exercises the full pipeline (classification, backoff, advance/abort, structured logs) without live providers. `--quick` runs the headline kill-provider-mid-request demo; default runs the full matrix (31 tests).
- **Sprint doc** at `docs/sprints/failover-recovery.md` capturing the design rationale and validator-in-the-loop negotiation history.

### Changed
- `intelligence.matrix` defaults updated to reachable models:
  - `vision` / `audio`: `xiaomi-tp:mimo-v2-omni` (exhausted) → `minimax:MiniMax-M2.7` (multimodal).
  - `transcription`: `whisper-1` (non-existent alias) → `whisper-large-v3` (groq-hosted).
  - `speech`: `xiaomi-tp:mimo-v2-tts` (404) → `minimax:speech-02-hd` (served by new adapter).
- Documentation updated across six surfaces: `README.md`, `SKILL.md`, `docs/user/api-reference.md`, `skills/ail-provider/README.md`, `docs-site/concepts/providers.mdx`, `docs-site/guides/auto-routing-setup.mdx`, `config.example.yaml` — all carry the new endpoint recipes, error-code mapping tables, and MiniMax voice-ID guidance.

### Fixed
- Pre-first-byte stream stalls now classify as `ClassStallPreFirstByte` (advance-eligible) instead of hanging indefinitely. Previously a frozen provider would block the request for the full upstream timeout.
- `CooldownAttempts` stale after recovery — now resets to 0 on the first success following a cooldown, so the next outage starts from the base tier instead of compounding.

### Known issues
- `internal/intelligence/embedding/TestIntegration` can fail when run as part of `go test ./...` due to ONNX runtime CGO global-state leakage between tests ("onnxruntime has already been initialized"). The package works correctly in production and the test passes when run in isolation (`go test -run TestIntegration`). Pre-existing on `main` — not introduced in this release.
- MiniMax Plus-plan quotas are tight: TTS ~1–5 RPM (9000 chars/day), music-2.6/music-cover/lyrics 100/day each. When the RPM is exceeded the adapter translates `1002` to HTTP 429 so `ClassRateLimit` fires and failover advances if a fallback chain is configured.

## [0.1.0] - 2026-04-08

First public release. One local endpoint. All your AI providers.

### Added
- **npx installer**: `npx @traylinx/switchailocal` — zero-setup, auto-downloads platform binary
- **Rolling update script** (`scripts/update-switchailocal.sh`) for zero-downtime deployments
- **5 new SSE error patterns**: Anthropic overload/rate_limit, detail+status_code format, error.code as int
- **GitHub Release** with pre-built binaries (darwin/arm64, linux/amd64)
- **GitHub Actions**: npx auto-publish workflow, matrix release workflow with CGO support

### Performance
- Switched hot-path JSON to `goccy/go-json` (~2-3x faster marshal/unmarshal)
- Replaced SHA-256 with `xxhash` for signature cache keys (~10x faster)
- Enriched SSE error detection for better retry/fallback decisions

### Fixed
- Docker healthcheck: `CMD` → `CMD-SHELL` (was passing `|| exit 1` as part of URL)
- CI workflows: Go version 1.22-1.24 → 1.25 (matching go.mod)
- GoReleaser: fixed CGO_ENABLED=0 → matrix build with CGO_ENABLED=1 (onnxruntime_go)
- golangci-lint: install from source for Go 1.25 compatibility
- Security: updated `cloudflare/circl` v1.6.1 → v1.6.3 (CVE fix)
- GHCR build: added `:latest` Docker tag on main branch pushes

### Removed
- Dead CLI subcommand files: heartbeat.go, hooks.go, learning.go, steering.go (1,290 lines)
  - Underlying packages remain intact for future re-enablement via management API

### Changed
- **Management Page V2**: Complete rewrite of the management interface
  - Modern React-based UI with responsive design
  - Provider management with 15+ supported providers
  - Model routing configuration
  - Single-file architecture (226 KB, zero external dependencies)
  - Built with React 18, Zustand, SWR, and Vite

## [1.0.0-rc1] - 2026-01-22

### Added
- Centralized secret management in `internal/secret` with environment variable support.
- MIT License headers to all Go source files.
- Gitleaks configuration and secret scanning allowlist.
- Lua Engine sandbox to prevent arbitrary code execution (Phase 1).
- Authorization middleware for Bridge Agent (Phase 1).
- Localhost-only default binding for Bridge Agent (Phase 1).
- Argument sanitization and binary whitelisting for Bridge Agent (Phase 1).
- Security documentation (`SECURITY.md`).

### Fixed
- Hardcoded OAuth client secrets in multiple providers.
- Potential Remote Code Execution (RCE) vulnerabilities in Bridge Agent.
- Potential sandbox escape vulnerabilities in Lua engine.

### Changed
- Default WebSocket authentication to be enabled by default.
- Refactored all providers to use centralized secret retrieval.
