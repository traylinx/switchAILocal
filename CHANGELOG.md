# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
