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

Use the ail-provider skill to configure my API provider
```

***Optional Fallback***: If Claude doesn't auto-detect the skill, you can paste this manual configuration prompt instead:

```text
I want to use switchAILocal as my API provider. The proxy runs at http://localhost:18080. Please set ANTHROPIC_BASE_URL=http://localhost:18080/api/provider/anthropic and ANTHROPIC_API_KEY=sk-test-123 in my environment, then use /model to select minimax:MiniMax-M2.7 (or any model from http://localhost:18080/v1/models).
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
│        DEEPSEEK (2,1M) │ KIMI (1,256k) │ MINIMAX (5,256k)│
│        XIAOMI-TP (8,200k) │ OLLAMA (17) │ GROQ (1)      │
│        ZAI (1) │ INCEPTION (1) │ SWITCHAI (4)           │
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

switchAILocal supports these providers (all discoverable by the wizard).
The table is grouped by purpose so consumer apps can pick a route based
on prompt budget and capability:

### Main API providers (the four chat routes)

| Provider | Prefix | Flagship | **Context** | Use for |
|----------|--------|----------|-------------|---------|
| **DeepSeek** | `deepseek:` | `deepseek-v4-pro` / `-flash` | **1M** | **Long context (>256k)**, deep reasoning |
| **Kimi** | `kimi:` | `kimi-k2.6` | 256k | Coding agents (Kimi-for-Coding) |
| **MiniMax** | `minimax:` | `MiniMax-M2.7` | 256k | General chat, multimodal, web search, image / speech / music |
| Xiaomi MiMo (Token Plan) | `xiaomi-tp:` | `mimo-v2.5-pro` (+ `mimo-v2.5`, `mimo-v2.5-tts`, `-voiceclone`, `-voicedesign`, `mimo-v2-omni` legacy) | 200k | Lite Monthly plan, 60M credits, 0.8x off-peak 16-24 UTC, TTS free for limited time |

`/v1/models` exposes `context_length` for these. Pick by budget.

### CLI providers (FREE — uses your subscriptions)

| Prefix | CLI binary |
|--------|------------|
| `geminicli:` | `gemini` |
| `claudecli:` | `claude` |
| `codex:` | `codex` |
| `vibe:` / `vibecli:` | `vibe` |
| `qwencli:` | `qwen` |
| `opencodecli:` | `opencode` |
| `picli:` | `pi` (⚠ loop hazard if pi alias points back at switchAILocal) |
| `kimicli:` | `kimi` |

### Specialty / cloud

| Provider | Prefix | Example Models |
|----------|--------|----------------|
| Ollama (local) | `ollama:` | `qwen3-embedding:0.6b` (default embedding), `qwen3.5:cloud` |
| Groq | `groq:` | `gpt-oss-20b`, `whisper-large-v3` (transcription) |
| Z.ai | `zai:` | `glm-5.1` |
| Inception Labs | `inception:` | `mercury-2` (diffusion LLM) |
| switchAI Cloud | `switchai:` | `switchai-fast`, `switchai-chat`, `switchai-embed`, `switchai-reasoner` (legacy) |

## Capability Endpoints (for AI agents)

Use these endpoints directly — no special client needed. All accept OpenAI-compatible shapes. The gateway internally translates to the upstream provider's native API where needed (e.g. MiniMax TTS → `/v1/t2a_pro`).

| Endpoint | What it does | Default model | Notes |
|----------|--------------|---------------|-------|
| `POST /v1/chat/completions` | Chat / reasoning / vision | `minimax:MiniMax-M2.7` (via `model: "auto"`) | Also supports built-in tools: `[{"type":"web_search"}]` for live web lookups (MiniMax native). Set `max_tokens >= 2000` when using web search — context inflates to 6k–13k tokens. |
| `POST /v1/embeddings` | Vector embeddings | `qwen3-embedding:0.6b` (local ollama) | Fallback: `switchai-embed`. All return dim=1024. |
| `POST /v1/images/generations` | Image generation | `minimax:image-01` | Prompt-to-image via MiniMax. Gateway rewrites upstream path `/v1/images/generations` → `/v1/image_generation` automatically. |
| `POST /v1/audio/transcriptions` | Audio → text (ASR) | `whisper-large-v3` (groq-hosted) | Multipart upload, `file` field. Returns `{text: "..."}`. |
| `POST /v1/audio/speech` | Text → speech (TTS) | `minimax:speech-02-hd` | **Use MiniMax voice IDs**, e.g. `male-qn-qingse`, `female-shaonv`, `audiobook_male_2`. Not OpenAI voice names. Returns binary audio (mp3/pcm/flac/wav). |
| `POST /v1/music/generations` | Text → music / style cover | `minimax:music-2.6` | Generate real songs from lyrics (text-to-music) or covers (`model: "minimax:music-cover"` + `audio_url`). Sync (default): returns `{data: {audio: <base64 MP3>, duration_ms, sample_rate, channels, bitrate}}`, 30–90s. Streaming (`stream: true`): returns raw `audio/mpeg`, TTFB ~20s. 100 songs/day. |
| `POST /v1/music/lyrics` | Song lyrics generator | (same credential) | Fast (~2s). Modes: `write_full_song` (default) or `edit`. Returns `{song_title, style_tags, lyrics}` with `[Verse]`/`[Chorus]` structure. 100/day. |

### Chat + web search (agent recipe)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:MiniMax-M2.7",
    "messages": [{"role":"user","content":"What did Apple announce today? Use web search."}],
    "max_tokens": 2000,
    "tools": [{"type": "web_search"}]
  }'
```

> **Note for agents running on operator-configured gateways (e.g. Tytus pods):** Some deployments set `AIL_AUTOINJECT_WEBSEARCH=true` so the gateway appends `{"type": "web_search"}` on the agent's behalf for an allowlist of models (typically `ail-compound`). When that flag is on, the gateway also bumps `max_tokens` to 2000 if lower. Your explicit `tools` entry is always preserved (set-union semantics), and you can suppress injection per-request with header `X-Ail-Autoinject: off`. See `docs/user/api-reference.md` → *Operator-side autoinject*.

### Text-to-speech (agent recipe)

```bash
curl http://localhost:18080/v1/audio/speech \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:speech-02-hd",
    "input": "Hello from your agent",
    "voice": "male-qn-qingse",
    "response_format": "mp3"
  }' --output out.mp3
```

**Rate-limit caveat:** MiniMax TTS on the Plus plan is ~1–5 RPM. Plan daily usage budget, not just total bytes. Gateway failover will classify 429 as `ClassRateLimit` and advance to the next provider if one is wired — otherwise the client sees 429.

### Transcription (agent recipe)

```bash
curl http://localhost:18080/v1/audio/transcriptions \
  -H "Authorization: Bearer sk-test-123" \
  -F model=whisper-large-v3 \
  -F file=@voice-note.wav
```

### Music + lyrics (agent recipe — full pipeline)

Generate lyrics then synthesize music from them. Both calls share MiniMax's 100/day quota bucket per endpoint.

```bash
# Step 1 — generate lyrics (~2s)
LYRICS=$(curl -sS http://localhost:18080/v1/music/lyrics \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{"mode":"write_full_song","prompt":"upbeat anthem about shipping code"}' \
  | jq -r .lyrics)

# Step 2 — synthesize music (~30-90s sync, or use stream: true for ~20s TTFB)
curl -sS http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d "$(jq -n --arg l "$LYRICS" '{"model":"minimax:music-2.6","lyrics":$l}')" \
  | jq -r .data.audio | base64 -d > song.mp3

# Or stream raw MP3 as it's generated (Content-Type: audio/mpeg):
curl -N -sS http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d "$(jq -n --arg l "$LYRICS" '{"model":"minimax:music-2.6","stream":true,"lyrics":$l}')" \
  > song.mp3
```

**Errors (MiniMax internal → HTTP):** 1002 rate_limit → 429 (failover advances), 2061 plan_not_support → 402 (failover advances), 2013 invalid_params → 400 (aborts). Streaming mode: pre-first-byte errors return the proper status code; errors after first byte close the stream cleanly — client keeps the partial (still-playable) MP3.

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
      "model": "minimax:MiniMax-M2.7"
    }
  }
}
```

### 3. Verify

```bash
curl -s http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model":"minimax:MiniMax-M2.7","messages":[{"role":"user","content":"Hello"}]}'
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
