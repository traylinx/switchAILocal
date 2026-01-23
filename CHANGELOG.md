# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
