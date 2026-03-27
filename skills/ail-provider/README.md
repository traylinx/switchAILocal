<div align="center">
  <img src="../../assets/switchai_logo_trans.png" alt="switchAILocal Logo" width="80"/>

  <h1>ail-provider</h1>

  <p><em>Claude Code skill to route all requests through switchAILocal</em></p>
</div>

---

## What It Does

This skill turns **switchAILocal** into the API provider for **Claude Code**. Once installed:

1. 🔍 **Auto-discovers** all running providers and models on `localhost:18080`
2. 🧙 **Interactive wizard** — presents models grouped by provider, asks you to pick one
3. ⚙️ **Auto-configures** `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, and `~/.claude/settings.json`
4. ✅ **Verifies** the connection with a live test request

---

## Install

### One-Liner (Global)

```bash
cd /path/to/switchAILocal/skills/ail-provider && ./setup
```

### Options

```bash
./setup                     # Global install for Claude Code
./setup --local             # Project-local install (.claude/skills/)
./setup --host codex        # Install for OpenAI Codex CLI
./setup --host gemini       # Install for Gemini CLI
./setup --host auto         # Auto-detect all installed CLIs
```

### What `setup` Does

1. Creates a symlink: `~/.claude/skills/ail-provider → this directory`
2. Checks if switchAILocal is running on port 18080
3. Reports available model count and health status

---

## Usage

After install, open Claude Code and the skill activates automatically. Or trigger it manually:

```
Use the ail-provider skill to configure my API provider
```

### The Wizard Flow

```
┌─────────────────────────────────────────────────────────┐
│  1. Preamble runs                                       │
│     → Finds switchAILocal config                        │
│     → Checks server health                              │
│     → Fetches all available models                      │
│                                                         │
│  2. Model Selection                                     │
│     → Groups models by provider:                        │
│        GEMINICLI (5)  │ OLLAMA (15)  │ XIAOMI (4)       │
│        SWITCHAI  (4)  │ GROQ   (9)  │ ALIBABA (8)      │
│     → Asks: "Which model do you want to use?"           │
│                                                         │
│  3. Auto-Configuration                                  │
│     → Sets ANTHROPIC_BASE_URL in ~/.zshrc               │
│     → Sets ANTHROPIC_API_KEY in ~/.zshrc                │
│     → Updates ~/.claude/settings.json                   │
│                                                         │
│  4. Verification                                        │
│     → Sends test request through the proxy              │
│     → Confirms routing works                            │
└─────────────────────────────────────────────────────────┘
```

---

## Available Providers

switchAILocal supports these providers (all discoverable by the wizard):

| Provider | Prefix | Example Models |
|----------|--------|----------------|
| Gemini CLI | `geminicli:` | `gemini-2.5-pro`, `gemini-3-flash-preview` |
| Claude CLI | `claudecli:` | `claude-sonnet-4` |
| Ollama | `ollama:` | `qwen3.5:cloud`, `kimi-k2.5:cloud` |
| Groq | `groq:` | `gpt-oss-20b`, `llama-3.3-70b-versatile` |
| Xiaomi | `xiaomi:` | `mimo-v2-pro`, `mimo-v2-flash` |
| Alibaba | `alibaba:` | `qwen-plus`, `MiniMax-M2.5`, `glm-5` |
| switchAI Cloud | `switchai:` | `switchai-reasoner`, `switchai-fast` |
| OpenAI | `openai:` | `gpt-5-mini`, `gpt-5.4` |

---

## Manual Configuration

If you prefer to configure without the wizard:

### 1. Set Environment Variables

Add to `~/.zshrc` (or `~/.bashrc`):

```bash
export ANTHROPIC_BASE_URL="http://localhost:18080/api/provider/anthropic"
export ANTHROPIC_API_KEY="sk-test-123"
```

### 2. Update Claude Code Settings

Edit `~/.claude/settings.json`:

```json
{
  "providerSettings": {
    "anthropic": {
      "model": "xiaomi:mimo-v2-pro"
    }
  }
}
```

### 3. Verify

```bash
curl -s http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model":"xiaomi:mimo-v2-pro","messages":[{"role":"user","content":"Hello"}]}'
```

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Skill not found in Claude Code | Restart Claude Code to reload skills |
| Server offline | Run `ail start` from the switchAILocal directory |
| Model not available | Check `curl http://localhost:18080/v1/models -H "Authorization: Bearer sk-test-123"` |
| Permission denied on setup | Run `chmod +x setup` first |

---

## Changing Models

Switch models anytime without re-running setup:

```bash
# List all available models
curl -s http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123" | \
  python3 -c "import json,sys; [print(m['id']) for m in json.load(sys.stdin)['data']]"

# Then edit ~/.claude/settings.json and change the model field
```

Or re-run the skill wizard in Claude Code:
```
Use the ail-provider skill to change my model
```

---

<div align="center">
  <strong>Part of the <a href="https://github.com/traylinx/switchAILocal">switchAILocal</a> ecosystem</strong>
</div>
