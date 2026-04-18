# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
