# API Reference

`switchAILocal` provides an OpenAI-compatible endpoint. Point any OpenAI client to `http://localhost:18080/v1` and it works out of the box.

## Base URL

```
http://localhost:18080/v1
```

---

## Chat Completions

**Endpoint**: `POST /v1/chat/completions`

### Auto-Routing (No Provider Prefix)

switchAILocal automatically routes to an available provider:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Explicit Provider Selection

Use the `provider:model` format to force a specific provider:

```bash
# Traylinx switchAI - Auto model selection (IRA picks the best)
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:auto",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

```bash
# Traylinx switchAI - DeepSeek Reasoner
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:deepseek-reasoner",
    "messages": [{"role": "user", "content": "Explain blockchain"}]
  }'
```

```bash
# Traylinx switchAI - OpenAI compatible model
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:openai/gpt-oss-120b",
    "messages": [{"role": "user", "content": "Hello!"}],
    "max_tokens": 150
  }'
```

```bash
# Force Gemini CLI
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

```bash
# Force Claude CLI
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "claudecli:claude-sonnet-4",
    "messages": [{"role": "user", "content": "Write a hello world in LUA."}]
  }'
```

```bash
# Force Ollama (local models)
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "ollama:llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Streaming

Add `"stream": true` for real-time token streaming:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:auto",
    "messages": [{"role": "user", "content": "Tell me a story"}],
    "stream": true
  }'
```

### Provider Prefixes

| Prefix | Provider | Description |
|--------|----------|-------------|
| `switchai:` | Traylinx switchAI | Unified AI gateway (recommended) |
| `geminicli:` | Gemini CLI | Google Gemini via local CLI |
| `gemini:` | Gemini API | Google AI Studio API |
| `claudecli:` | Claude CLI | Anthropic Claude via local CLI |
| `claude:` | Claude API | Anthropic API |
| `codex:` | Codex | OpenAI Codex |
| `ollama:` | Ollama | Local Ollama models |
| `vibe:` | Vibe CLI | Mistral Vibe CLI |
| `openai-compat:` | OpenAI Compatible | OpenRouter, etc. |

---

## Models List

**Endpoint**: `GET /v1/models`

Returns all available models from all connected providers.

```bash
curl http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123"
```

---

## Providers Status

**Endpoint**: `GET /v1/providers`

Returns active providers, their status, and model count.

```bash
curl http://localhost:18080/v1/providers \
  -H "Authorization: Bearer sk-test-123"
```

**Response Example**:
```json
{
  "object": "list",
  "data": [
    {
      "id": "geminicli",
      "name": "Gemini CLI",
      "type": "cli",
      "mode": "local",
      "status": "active",
      "model_count": 5
    },
    {
      "id": "switchai",
      "name": "SwitchAI",
      "type": "api",
      "mode": "online",
      "status": "active",
      "model_count": 41
    }
  ]
}
```

---

## CLI Provider Extensions

When using CLI providers (`geminicli:`, `vibe:`, `claudecli:`, `codex:`), you can pass additional options via the `extra_body.cli` object. This enables advanced capabilities like file attachments, control flags, and session management.

### Schema

```json
{
  "model": "geminicli:",
  "messages": [...],
  "extra_body": {
    "cli": {
      "attachments": [
        {"type": "file", "path": "/path/to/file.py"},
        {"type": "folder", "path": "./src/"}
      ],
      "flags": {
        "sandbox": true,
        "auto_approve": true,
        "yolo": true
      },
      "session_id": "my-session"
    }
  }
}
```

### Attachments

Pass local files or folders as context to CLI providers:

| Type | Description | Example |
|------|-------------|---------|
| `file` | Single file | `{"type": "file", "path": "./main.go"}` |
| `folder` | Directory | `{"type": "folder", "path": "./src/"}` |

### Flags

Provider-agnostic flags mapped to CLI-specific arguments:

| Flag | Description | geminicli | vibe | claudecli | codex |
|------|-------------|-----------|------|-----------|-------|
| `sandbox` | Restricted execution | `-s` | - | - | `--sandbox` |
| `auto_approve` | Skip confirmations | `-y` | `--auto-approve` | `--dangerously-skip-permissions` | `--full-auto` |
| `yolo` | Maximum autonomy | `--yolo` | `--auto-approve` | `--dangerously-skip-permissions` | `--full-auto` |

### Session Management

Resume or name CLI sessions:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Continue our work"}],
    "extra_body": {"cli": {"session_id": "latest"}}
  }'
```

---

## Management Dashboard

**URL**: `http://localhost:18080/management.html`

Access the graphical dashboard to:

- Monitor usage statistics
- Edit `config.yaml` live
- View real-time request logs
- Manage API keys

