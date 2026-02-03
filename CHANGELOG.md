# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Management Page V2**: Complete rewrite of the management interface
  - Modern React-based UI with responsive design
  - Provider management with 15+ supported providers (CLI tools, local models, cloud APIs)
  - Model routing configuration (create mappings without editing YAML)
  - System settings panel (debug mode, proxy URL, system info)
  - Single-file architecture (226 KB, zero external dependencies)
  - State-box integration with read-only mode support
  - Authentication with management key and URL parameter support (`?key=...`)
  - Built with React 18, Zustand, SWR, and Vite

### Changed
- Management dashboard now uses modern React stack instead of legacy implementation
- Build process simplified with `vite-plugin-singlefile` for single HTML output
- Provider configuration moved to dedicated modal dialogs
- Model aliasing renamed to "Model Routing" with improved UI

### Technical
- Frontend: React 18 + Vite + vite-plugin-singlefile
- State Management: Zustand (lightweight, minimal boilerplate)
- Data Fetching: SWR (caching, revalidation, optimistic updates)
- Styling: Vanilla CSS with CSS variables (no external frameworks)
- Icons: lucide-react (inline SVGs)
- Build Output: Single 226 KB HTML file with all assets inlined
- Testing: Jest + React Testing Library configured

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
