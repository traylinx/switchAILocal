# Provider Setup Guide

`switchAILocal` is designed to be flexible. Depending on your needs, you can connect to AI providers in three different ways.

## 🚀 Cheat Sheet: Which mode should I use?

| Mode                 | Best For...                                      | Setup           | Auth          | Prefix       |
| :------------------- | :----------------------------------------------- | :-------------- | :------------ | :----------- |
| **1. Local Wrapper** | **Easiest.** Best for local dev & subscriptions. | Auto-discovered | Uses your CLI | `geminicli:` |
| **2. API Key**       | Standard cloud usage.                            | `config.yaml`   | Static Key    | `gemini:`    |
| **3. Cloud Proxy**   | Server-to-server w/o keys.                       | `--login`       | OAuth         | `gemini:`    |

---

## 1. Local CLI Wrappers (Easiest)
If you have official CLI tools installed (like `gemini`, `claude`, or `vibe`), **you don't need to do anything!** `switchAILocal` finds them on your PATH and lets you use them immediately.

**Pros:** No API keys needed, supports advanced CLI features like folder attachments.
**Cons:** Slightly slower (spawns a process).

### Usage Example (Gemini CLI)

Use the `cli:` suffix to target your local binary:

```bash
curl http://localhost:18080/v1/chat/completions \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Refactor this"}],
    "extra_body": {
      "cli": {
        "attachments": [{"type": "folder", "path": "./src"}]
      }
    }
  }'
```

---

## 2. API Keys in `config.yaml` (Standard)
The best way to use Cloud APIs if you already have an API key. Just add your keys to the `config.yaml` file.

**Pros:** Fastest performance, no local tool dependencies.
**Cons:** Requires manual key management.

### API Key Configuration

1. Open `config.yaml`.
2. Find your provider section and add your key.
3. **Crucial**: You must explicitly list each model you wish to use.

```yaml

# Google AI Studio
gemini-api-key:
  - api-key: "AIza..."
    models:
      - name: "gemini-1.5-pro"
        alias: "pro"

# Anthropic
claude-api-key:
  - api-key: "sk-ant-..."
    models:
      - name: "claude-3-5-sonnet-20241022"
        alias: "sonnet"
```

---

## 3. Cloud Proxy Mode (Advanced OAuth)
Use this if you want `switchAILocal` to act as a standalone server using your personal Google/Anthropic account credentials (OAuth) instead of a static API key.

**Pros:** No API keys to leak, manages tokens automatically.
**Cons:** Requires manual login step.

### Cloud Proxy Setup

Run the login command once to link your account:

```bash
# For Gemini
./switchAILocal --login

# For Claude
./switchAILocal --claude-login
```

---

## 4. Local Self-Hosted Models

### Ollama

1. Install [Ollama](https://ollama.com).
2. Ensure it is running. `switchAILocal` will auto-discover it on `localhost:11434`.
3. Use the `ollama:` prefix:

```bash
curl http://localhost:18080/v1/chat/completions \
  -d '{"model": "ollama:llama3.2", "messages": [{"role": "user", "content": "Hi!"}]}'
```

### LM Studio

1. Start the Local Server in [LM Studio](https://lmstudio.ai) (default port 1234).
2. Enable in `config.yaml`.
3. Use the `lmstudio:` prefix.

### OpenCode

`switchAILocal` integrates deeply with [OpenCode](https://github.com/microsoft/opencode) to provide powerful agentic tools and session persistence.

1. Install and start OpenCode: `opencode serve`.
2. Enable in `config.yaml` (default port 4096).
3. Use the `opencode:` prefix:

```bash
curl http://localhost:18080/v1/chat/completions \
  -d '{
    "model": "opencode:build",
    "messages": [{"role": "user", "content": "Refactor this file"}]
  }'
```

**Available Agents:**

- `opencode:build`: Standard development agent with full tool access.
- `opencode:plan`: Read-only agent for planning and exploration.
- `opencode:explore`: Specialized subagent for codebase mapping.

---

## 💎 Traylinx switchAI (Proprietary Integration)

The most powerful mode. Use `model: auto` and let our Intelligent Routing Agent (IRA) pick the best model for your task across all available providers.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:auto",
    "messages": [{"role": "user", "content": "Fix this bug"}]
  }'
```

