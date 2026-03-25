# OpenCode Integration Developer Guide

This document explains how switchAILocal integrates with [OpenCode](https://github.com/sst/opencode), enabling access to its agentic coding tools via the standard OpenAI-compatible API.

## Architecture

S`switchAILocal` integrates with OpenCode by treating it as an upstream provider. This allows you to route requests to OpenCode models (`build`, `plan`, `explore`) while leveraging `switchAILocal`'s shared authentication, logging, and unified API interface.

```
Client (Cursor, Continue, etc.)
       │
       ▼
switchAILocal Gateway (/v1/chat/completions)
       │
       ▼
OpenCode Server (localhost:4096)
       │
       ▼
Upstream AI (Gemini, Claude, etc.)
```

## Key Components

| File                                                  | Purpose                                          |
| ----------------------------------------------------- | ------------------------------------------------ |
| `internal/runtime/executor/opencode_executor.go`      | Core executor, SSE streaming, session management |
| `internal/runtime/executor/opencode_session_store.go` | Thread-safe session ID mapping                   |
| `internal/config/config.go` (`OpenCodeConfig`)        | Configuration struct                             |
| `internal/discovery/parsers/opencode.go`              | Model discovery parser                           |

## Model Naming

| Model              | Description                   |
| ------------------ | ----------------------------- |
| `opencode:build`   | Full-access development agent |
| `opencode:plan`    | Read-only planning agent      |
| `opencode:explore` | Codebase exploration subagent |

## Configuration

```yaml
opencode:
  enabled: true
  base-url: "http://localhost:4096"
  default-agent: "build"
```

## Session Lifecycle

1. Client sends request with optional `extra_body.session_id`
2. Executor maps client session to OpenCode session
3. Messages are posted to `/session/:id/message`
4. SSE events are streamed back and transformed to OpenAI format
5. Sessions expire after 1 hour of inactivity (configurable)

## Testing

- Run OpenCode: `opencode serve`
- Test via curl:
  ```bash
  curl http://localhost:18080/v1/chat/completions \
    -d '{"model": "opencode:build", "messages": [{"role": "user", "content": "Hello"}]}'
  ```

## Troubleshooting

| Issue                    | Solution                                         |
| ------------------------ | ------------------------------------------------ |
| "opencode not reachable" | Ensure `opencode serve` is running               |
| Empty model list         | Check OpenCode's `/agent` endpoint               |
| Permission errors        | OpenCode may require interaction in its terminal |
