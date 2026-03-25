---
name: switchailocal
description: Unified LLM proxy for AI agents. Route all model requests through http://localhost:18080/v1. Provides FREE access to Gemini CLI, Claude CLI, Codex, and Vibe via your existing subscriptions. Use when: (1) making LLM calls using provider prefixes, (2) switching between CLI/Local/Cloud providers, (3) needing to attach local files/folders to prompts via CLI, (4) requiring intelligent routing between models, or (5) needing to monitor provider health and analytics.
metadata:
  {
    "openclaw":
      {
        "emoji": "🔀",
        "requires": { "bins": ["ail", "go"] },
        "install":
          [
            {
              "id": "switchailocal-setup",
              "kind": "bash",
              "script": "./ail.sh setup",
              "label": "Run unified switchAILocal setup (deps, build, service)",
            }
          ]
      }
  }
---

# switchAILocal Skill

Unified LLM proxy and router for AI agents. Always use `http://localhost:18080/v1` as your base URL.

**The killer feature**: Use your paid CLI subscriptions (Gemini Pro, Claude Pro, etc.) via the API - **it's FREE** because you already pay for the subscription!

## When to Use

✅ **USE this skill when:**

- An agent needs to make an LLM call to Gemini, Claude, Ollama, OpenAI etc.
- You need to attach entire file directories to an LLM context (supported inherently via `geminicli` and `claudecli`).
- You need to start, stop, or manage the local switchAILocal server/bridge on the user's machine.
- You want to monitor provider health, analytics, or status.

## When NOT to Use

❌ **DON'T use this skill when:**

- The user asks about their calendar, emails, or non-LLM operations.
- Interacting with APIs that aren't OpenAI-compatible.

---

## Agent Setup & Operations (`ail` CLI)

The repo includes a self-contained CLI tool (`ail.sh` or the global alias `ail`) to manage the proxy.

### First-Time Setup

If this is a fresh clone, run the setup wizard:
```bash
git clone https://github.com/traylinx/switchAILocal.git
cd switchAILocal
./ail.sh setup
```
This automatically checks/installs Go, builds the binaries, creates `config.yaml`, installs the bridge service (macOS), and registers the global `ail` command.

### Daily Operations

```bash
# Start the local server (if it's not already running)
ail start

# Stop the local server
ail stop

# Check status of the core server and background bridge
ail status

# Restart functionality
ail restart

# Follow server logs
ail logs -f
```

---

## Quick API Usage

### 1. Make a request (FREE with CLI)
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### 2. Configure Python Client
```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:18080/v1", api_key="sk-test-123")
response = client.chat.completions.create(model="geminicli:", messages=[{"role": "user", "content": "Hi!"}])
```

---

## 🗺️ Skill Files

| File                                                         | Description                          |
| ------------------------------------------------------------ | ------------------------------------ |
| **SKILL.md** (this file)                                     | Core workflow and endpoint reference |
| [references/routing.md](references/routing.md)               | Intelligent routing and matrix setup |
| [references/multimodal.md](references/multimodal.md)         | Vision and image processing          |
| [references/examples.md](references/examples.md)             | Real-world agentic use cases         |
| [references/management-api.md](references/management-api.md) | Full Monitoring & Operations API     |
| [references/steering.md](references/steering.md)             | Conditional routing rules            |
| [references/hooks.md](references/hooks.md)                   | Automation and event hooks           |
| [references/memory.md](references/memory.md)                 | Analytics and history                |

---

## ⚠️ Critical: Model Format

**NEVER use bare model names.** Format is ALWAYS `provider:` or `provider:model`.

| ❌ Wrong             | ✅ Correct                  | Why                       |
| ------------------- | -------------------------- | ------------------------- |
| `gemini-2.5-pro`    | `geminicli:gemini-2.5-pro` | Needs provider prefix     |
| `claude-3-5-sonnet` | `claudecli:`               | `claudecli:` uses default |
| `llama3`            | `ollama:llama3`            | Needs provider prefix     |

---

## 🏗️ Provider Reference

### 1. CLI Providers (FREE!)
Uses your human's CLI subscriptions. Best for agents.

| Prefix       | CLI      | Subscription Required |
| ------------ | -------- | --------------------- |
| `geminicli:` | `gemini` | Google AI Premium/Pro |
| `claudecli:` | `claude` | Claude Pro/Max        |
| `codex:`     | `codex`  | OpenAI Plus           |
| `vibe:`      | `vibe`   | Mistral Le Chat       |

### 2. Local & Cloud
| Prefix      | Source         | Cost                   |
| ----------- | -------------- | ---------------------- |
| `ollama:`   | Local Ollama   | FREE                   |
| `auto`      | Local Cortex   | FREE (Requires plugin) |
| `switchai:` | Traylinx Cloud | Per-token              |
| `groq:`     | Groq Cloud     | Per-token              |

### 3. switchAI Cloud Aliases
Pre-configured aliases in `config.yaml` for the Traylinx Cloud API:

| Alias              | Upstream Model        | Best For      |
| ------------------ | --------------------- | ------------- |
| `switchai-fast`    | `openai/gpt-oss-20b`  | Fast tasks    |
| `switchai-chat`    | `openai/gpt-oss-20b`  | Conversation  |
| `switchai-reasoner`| `deepseek-reasoner`   | Deep thinking |

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "switchai-fast", "messages": [{"role": "user", "content": "Hello!"}]}'
```

---

## 🚀 Core Features

### CLI Attachments & Flags
Pass local context and control autonomy via CLI extensions.

```json
{
  "model": "geminicli:",
  "messages": [{"role": "user", "content": "Fix this code"}],
  "extra_body": {
    "cli": {
      "attachments": [{"type": "folder", "path": "./src"}],
      "flags": {"auto_approve": true, "yolo": true}
    }
  }
}
```

### Streaming
Add `"stream": true` to any request for SSE token streaming.

---

## 🌲 Decision Tree

```
What do you need?
├─ FREE + Powerful + Files
│   └─ CLI Providers (geminicli:, claudecli:)
├─ FREE + Private + Fast
│   └─ Local Ollama (ollama:llama3.2)
├─ Ultra-Fast Production
│   └─ Groq Cloud (groq:llama-3.3-70b)
└─ I don't know, you pick
    └─ Intelligent Routing (auto)
```

---

## 🛠️ Troubleshooting & Best Practices

| Problem                | Fix                                           |
| ---------------------- | --------------------------------------------- |
| Connection error       | Check if server is running on port 18080      |
| Model not found        | Ensure you used the `provider:` prefix        |
| 401 Unauthorized       | Check API key in `config.yaml`                |
| 403 Access Denied      | Likely a WAF block; the proxy auto-retries    |
| `auth_unavailable`     | Restart server: `ail restart`                 |

### Best Practices
1. **Prefer CLI Providers**: They are free and support file attachments.
2. **Check Status**: Use `GET /v1/providers` to see what is active.
3. **Use `auto`**: For simple tasks, let the router pick the best model.
4. **Local for Privacy**: Use `ollama:` for confidential data.
5. **switchai Cloud**: Use `switchai-fast` for quick per-token cloud access.

---

*Route wisely. Save tokens. Use CLI.* 🚀
