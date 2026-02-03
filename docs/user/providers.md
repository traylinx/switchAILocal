# Provider Setup Guide

`switchAILocal` is designed to be flexible. Depending on your needs, you can connect to AI providers in three different ways.

## 🚀 Cheat Sheet: Which mode should I use?

| Mode                 | Best For...                                      | Setup           | Auth                    | Prefix       |
| :------------------- | :----------------------------------------------- | :-------------- | :---------------------- | :----------- |
| **1. Local Wrapper** | **Easiest.** Best for local dev & subscriptions. | Auto-discovered | Uses your existing CLI  | `geminicli:` |
| **2. API Key**       | Standard cloud usage.                            | `config.yaml`   | Static Key              | `gemini:`    |
| **3. Cloud Proxy**   | Alternative OAuth (rarely needed).               | `--login`       | OAuth (needs client ID) | `gemini:`    |

**💡 Recommendation:** Start with **Mode 1** (Local Wrapper) if you have CLI tools installed. It's zero-config and works immediately!

---

## 1. Local CLI Wrappers (Recommended - Zero Setup)

**If you already have official CLI tools installed** (like `gemini`, `claude`, or `vibe`), **switchAILocal uses them automatically. No login required!**

### How It Works

1. ✅ Install and authenticate your CLI tool normally (e.g., `gemini auth login`)
2. ✅ Start switchAILocal - it auto-discovers CLI tools in your PATH
3. ✅ Use immediately with the `cli` suffix: `geminicli:`, `claudecli:`, etc.

**Pros:** 
- Zero configuration - uses your existing CLI authentication
- No API keys needed
- Supports advanced CLI features like folder attachments
- Works immediately after CLI tool is authenticated

**Cons:** 
- Slightly slower (spawns a process per request)
- Requires CLI tool to be installed and in PATH

### Usage Example (Gemini CLI)

```bash
# First time: Authenticate your CLI tool (one-time setup)
gemini auth login

# Then use switchAILocal immediately - no --login needed!
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

### Supported CLI Tools

| CLI Tool | Install Command | Auth Command | Prefix |
|----------|----------------|--------------|--------|
| Gemini | `npm install -g @google/generative-ai-cli` | `gemini auth login` | `geminicli:` |
| Claude | `npm install -g @anthropic-ai/claude-cli` | `claude auth login` | `claudecli:` |
| Codex | `npm install -g @openai/codex-cli` | `codex auth login` | `codex:` |
| Vibe | `npm install -g @mistral/vibe-cli` | `vibe auth login` | `vibe:` |

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

## 3. Cloud Proxy Mode (Advanced OAuth - Rarely Needed)

**⚠️ Most users should skip this section!** Use this **only if:**
- ❌ You don't have CLI tools installed
- ❌ You don't have API keys
- ✅ You want switchAILocal to manage OAuth tokens directly

### When to Use Cloud Proxy

This mode is an **alternative** to CLI wrappers and API keys. It's useful for:
- Server deployments where CLI tools aren't available
- Environments where you can't install CLI tools
- Advanced OAuth-based authentication workflows

### Requirements

Before using `--login`, you need to set up OAuth credentials:

1. **Create OAuth credentials** in Google Cloud Console or Anthropic Console
2. **Set environment variables:**
   ```bash
   export GEMINI_CLIENT_ID="your-client-id"
   export GEMINI_CLIENT_SECRET="your-client-secret"
   ```

### Cloud Proxy Setup

Run the login command once to link your account:

```bash
# For Gemini (requires GEMINI_CLIENT_ID and GEMINI_CLIENT_SECRET)
./switchAILocal --login

# For Claude (requires CLAUDE_CLIENT_ID and CLAUDE_CLIENT_SECRET)
./switchAILocal --claude-login
```

**Pros:** 
- No API keys to leak
- Manages tokens automatically
- Works without CLI tools

**Cons:** 
- Requires OAuth client credentials setup
- More complex initial configuration
- Manual login step required

### Troubleshooting

If you see "Missing required parameter: client_id":
1. Verify environment variables are set: `echo $GEMINI_CLIENT_ID`
2. Consider using **Mode 1** (CLI Wrappers) instead - it's much simpler!

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

