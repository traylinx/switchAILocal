---
name: switchailocal
description: Unified LLM proxy for AI agents. Route all model requests through http://localhost:18080/v1. Provides FREE access to Gemini CLI, Claude CLI, Codex, and Vibe via your existing subscriptions. Includes Intelligent Auto-Routing (Cortex) for autonomous model selection, a self-optimizing Lab, token conservation, and a management dashboard. Use when: (1) making LLM calls, (2) switching between CLI/Local/Cloud providers, (3) attaching local files to prompts, (4) needing intelligent routing, or (5) auto-setting up the best available models.
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

Unified LLM proxy and intelligent router for AI agents. Always use `http://localhost:18080/v1` as your base URL.

**The killer feature**: Use your paid CLI subscriptions (Gemini Pro, Claude Pro, etc.) via the API — **it's FREE** because you already pay for the subscription!

## When to Use

✅ **USE this skill when:**
- An agent needs to make an LLM call to Gemini, Claude, Ollama, OpenAI, etc.
- You need to attach entire file directories to an LLM context (via `geminicli` and `claudecli`).
- You need to start, stop, or manage the local switchAILocal server/bridge.
- You want to monitor provider health, analytics, or status.
- You want to auto-detect available models and configure optimal routing.

❌ **DON'T use this skill when:**
- The user asks about their calendar, emails, or non-LLM operations.
- Interacting with APIs that aren't OpenAI-compatible.

---

## 🎯 Quick Routing Cheatsheet (read this first)

Pick the model by **prompt budget** and **capability**. The four mains:

| Need | Model | Why |
|---|---|---|
| Chat / multimodal (image+audio) / web_search / image-gen / TTS / music | `minimax:MiniMax-M2.7` (256k) | Multimodal flagship, native web_search tool, all media endpoints |
| Coding agent work | `kimi:kimi-k2.6` (256k) | Kimi-for-Coding tier, optimized for agentic code |
| **Long context (>256k)** or **deep reasoning** | `deepseek:deepseek-v4-pro` (1M) / `deepseek-v4-flash` (1M) | Only 1M-context model in stack, 384k max output |
| Cheap chat backup | `xiaomi-tp:mimo-v2.5-pro` (200k) | Lite Token Plan, 60M credits/mo, free TTS family |
| Embeddings | `qwen3-embedding:0.6b` (local Ollama) | 1024-dim, free, private |
| Audio transcription | `whisper-large-v3` (Groq) | Cheap + fast |

**Auto-routing shortcut:** send `model: "auto"` for general chat; `model: "auto:coding"` → kimi; `model: "auto:long_ctx"` → deepseek-v4-pro. Or read `context_length` directly from `GET /v1/models` and pick.

**Capability endpoints (all OpenAI-shape):**
- `POST /v1/chat/completions` — chat, vision, web_search, tools
- `POST /v1/embeddings` — qwen3-embedding:0.6b default
- `POST /v1/images/generations` — minimax:image-01 default
- `POST /v1/audio/transcriptions` — whisper-large-v3 default
- `POST /v1/audio/speech` — minimax:speech-02-hd default
- `POST /v1/music/generations` + `/v1/music/lyrics` — minimax:music-2.6

---

## 🤖 Agent Auto-Setup (Zero-Config Bootstrap)

Follow these steps IN ORDER to bootstrap switchAILocal from scratch on a user's machine.

### Step 1 — Install & Start

**Option A (npx — recommended, no git clone needed):**
```bash
npx @traylinx/switchailocal
```

**Option B (from source):**
```bash
git clone https://github.com/traylinx/switchAILocal.git
cd switchAILocal
./ail.sh setup
source ~/.zshrc 2>/dev/null || source ~/.bashrc 2>/dev/null  # Reload PATH
```

### Step 2 — Detect Available Providers

Run these commands to discover what the user has installed:

```bash
# CLI Providers (FREE!)
which gemini  && echo "✅ Gemini CLI found"
which claude  && echo "✅ Claude CLI found"
which codex   && echo "✅ Codex CLI found"

# Local Providers
curl -s http://localhost:11434/api/tags 2>/dev/null && echo "✅ Ollama running"

# Cloud API Keys (check environment)
[ -n "$OPENAI_API_KEY" ]    && echo "✅ OpenAI key found"
[ -n "$ANTHROPIC_API_KEY" ] && echo "✅ Anthropic key found"
[ -n "$GEMINI_API_KEY" ]    && echo "✅ Google AI key found"
```

### Step 3 — Generate config.yaml

Based on detected providers, generate a minimal config:

```yaml
host: ""
port: 18080

# Enable any detected CLI providers:
# geminicli: (uses `gemini` CLI — FREE with Google AI Premium)
# claudecli: (uses `claude` CLI — FREE with Claude Pro)
# codex: (uses `codex` CLI — FREE with OpenAI Plus)

# Enable Ollama if detected:
ollama:
  enabled: true
  base-url: "http://localhost:11434"
  auto-discover: true

# Enable Intelligent Auto-Routing:
auto-routing:
  enabled: true
  weights:
    availability: 0.35
    quota: 0.25
    latency: 0.2
    success-rate: 0.2
  discovery:
    enabled: true
    probe-on-startup: true
  conservation:
    enabled: true
    simple-threshold-tokens: 500
  lab:
    enabled: true
    adaptation-interval: 24h
    max-weight-drift: 0.1
```

### Step 4 — Start & Verify

```bash
ail start
# Verify it's running:
curl -s http://localhost:18080/v1/models | head -c 200
```

You should see a JSON response listing all available models/providers.

---

## ⚠️ Critical: Model Format

**NEVER use bare model names.** Format is ALWAYS `provider:` or `provider:model`.

| ❌ Wrong             | ✅ Correct                  | Why                       |
| ------------------- | -------------------------- | ------------------------- |
| `gemini-2.5-pro`    | `geminicli:gemini-2.5-pro` | Needs provider prefix     |
| `claude-3-5-sonnet` | `claudecli:`               | `claudecli:` uses default |
| `llama3`            | `ollama:llama3`            | Needs provider prefix     |
| `auto route me`     | `auto` or `auto:coding`    | Use `auto` prefix only    |

---

## 🏗️ Provider Reference

### 1. CLI Providers (FREE — uses your subscriptions)

| Prefix          | CLI        | Subscription          |
| --------------- | ---------- | --------------------- |
| `geminicli:`    | `gemini`   | Google AI Premium/Pro |
| `claudecli:`    | `claude`   | Claude Pro/Max        |
| `codex:`        | `codex`    | OpenAI Plus           |
| `vibe:`         | `vibe`     | Mistral Le Chat       |
| `qwencli:`      | `qwen`     | (free Alibaba CLI)    |
| `opencodecli:`  | `opencode` | (open-source TUI)     |
| `picli:`        | `pi`       | (caution: loop hazard if pi alias points back at switchAILocal) |
| `kimicli:`      | `kimi`     | Kimi-for-Coding key   |

### 2. API Providers (the four "main" routes)

| Prefix      | Flagship Model            | Context | What it's for                                |
| ----------- | ------------------------- | ------- | -------------------------------------------- |
| `deepseek:` | `deepseek-v4-pro` / `-flash` | **1M**  | Long-context (>256k prompts), deep reasoning |
| `kimi:`     | `kimi-k2.6`               | 256k    | Coding agents (Kimi-for-Coding tier)         |
| `minimax:`  | `MiniMax-M2.7`            | 256k    | General chat, multimodal (text+image+audio), web search, image/speech/music gen |
| `xiaomi-tp:`| `mimo-v2.5-pro` (+ v2.5 base, TTS family, v2-omni legacy) | 200k | Xiaomi MiMo Token Plan (60M credits/mo, 0.8x off-peak 16-24 UTC, TTS free for limited time) |

### 3. Local & Cloud

| Prefix      | Source         | Cost                |
| ----------- | -------------- | ------------------- |
| `ollama:`   | Local Ollama   | FREE                |
| `auto`      | Cortex Router  | FREE (auto-selects) |
| `switchai:` | Traylinx Cloud | Per-token           |
| `groq:`     | Groq Cloud     | Per-token           |
| `zai:`      | Zhipu          | Per-token           |
| `inception:`| InceptionLabs  | Per-token           |

### 4. switchAI Cloud Aliases

| Alias              | Upstream Model       | Best For      |
| ------------------ | -------------------- | ------------- |
| `switchai-fast`    | `openai/gpt-oss-20b` | Fast tasks    |
| `switchai-chat`    | `openai/gpt-oss-20b` | Conversation  |
| `switchai-reasoner`| `deepseek-reasoner`  | Deep thinking (legacy — use `deepseek:deepseek-v4-pro` instead) |
| `switchai-embed`   | `mistral-embed`      | Embedding fallback |

---

## 🧮 Context-Window Tiers (consumer apps: read this)

Pick the provider whose `context_length` ≥ your prompt token estimate. The
gateway exposes `context_length` per model in `GET /v1/models` so callers
can route deterministically:

| Token budget | Route to                       | Why                          |
| ------------ | ------------------------------ | ---------------------------- |
| ≤ 200k       | `xiaomi-tp:mimo-v2.5-pro` (or any) | Cheapest available — uses 60M plan credits |
| ≤ 256k       | `kimi:kimi-k2.6` (coding) or `minimax:MiniMax-M2.7` (everything else) | Multimodal/coding flagships  |
| **> 256k, ≤ 1M** | `deepseek:deepseek-v4-pro` | **The only 1M-context model** in the stack |

The intent matrix already wires this: `auto:long_ctx` and `auto:long_context`
route to `deepseek:deepseek-v4-pro` automatically. Consumer apps can either
read `context_length` from `/v1/models` and pick directly, or send
`model: "auto:long_ctx"` and let the router decide.

---

## 🧠 Intelligent Auto-Routing (Cortex)

When the model is `auto` or `auto:<intent>`, the Cortex Router automatically selects the best available model using a composite scoring algorithm.

### Basic Auto-Routing
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "auto", "messages": [{"role": "user", "content": "Hello!"}]}'
```

### Intent-Based Routing
```bash
# Route to coding-optimized models
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "auto:coding", "messages": [{"role": "user", "content": "Write a Go sorting algorithm"}]}'
```

Supported intents: `coding`, `reasoning`, `creative`, `fast`, `secure`, `vision`, `audio`, `web_search`, `research`, `chat`, `cli`, `long_ctx`. Specialized slots — `image_gen`, `transcription`, `speech`, `embedding` — are used by their dedicated endpoints (see sections below), not by auto chat routing.

### How Scoring Works

Each model is scored: `FinalScore = (W_a×Availability + W_q×Quota + W_l×Latency + W_s×SuccessRate + TierBoost + PreferenceBoost) × ConservationMultiplier`

The model with the highest score wins. The Lab continuously optimizes the weights.

> For deep architecture details, see the local `docs-site/intelligent-systems/` or the online [docs-site](https://ail.traylinx.com/intelligent-systems/cortex-router).

---

## 📊 Management Dashboard & API

### Dashboard UI
```
http://localhost:18080/management
```
Provides real-time visualization of provider health, auto-routing weights, Lab experiments, and the live routing journal.

### Telemetry API

```bash
# Get current Lab status + live weights
curl http://localhost:18080/v0/management/autoroute/status

# Get recent routing decisions journal
curl http://localhost:18080/v0/management/autoroute/journal
```

> For the full Management API reference, see [references/management-api.md](references/management-api.md).

---

## 🚀 Quick API Usage

### curl (simplest)
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "geminicli:", "messages": [{"role": "user", "content": "Hello!"}]}'
```

### Python
```python
from openai import OpenAI
client = OpenAI(base_url="http://localhost:18080/v1", api_key="sk-test-123")
response = client.chat.completions.create(
    model="geminicli:", 
    messages=[{"role": "user", "content": "Hi!"}]
)
```

### Node.js
```javascript
import OpenAI from 'openai';
const client = new OpenAI({ baseURL: 'http://localhost:18080/v1', apiKey: 'sk-test-123' });
const response = await client.chat.completions.create({
  model: 'auto',
  messages: [{ role: 'user', content: 'Hello!' }],
});
```

### CLI Attachments & Flags
Pass local context and control autonomy via CLI extensions:

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

## 🎨 Image Generation

Generate images via the `/v1/images/generations` endpoint:

```bash
curl --location 'http://localhost:18080/v1/images/generations' \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:image-01",
    "prompt": "A dog wearing a space suit on Mars, photorealistic",
    "aspect_ratio": "16:9",
    "response_format": "url"
  }'
```

**Parameters:**
- `model` — Always use `minimax:image-01`
- `prompt` — Text description of the desired image
- `aspect_ratio` — `1:1`, `16:9`, `9:16`, `4:3`, `3:4`
- `response_format` — `url` (returns HTTP URL) or `base64` (returns base64 encoded image)

**Example Python:**
```python
response = client.images.generate(
    model="minimax:image-01",
    prompt="A serene Japanese garden with cherry blossoms",
    aspect_ratio="16:9",
    response_format="url"
)
image_url = response.data[0].url
```

---

## 🔊 Text-to-Speech

Generate audio from text via `/v1/audio/speech`. switchAILocal ships a MiniMax T2A adapter that translates the OpenAI-shape request to MiniMax's native `/v1/t2a_pro` API behind the scenes.

```bash
curl http://localhost:18080/v1/audio/speech \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:speech-02-hd",
    "input": "Hello from switchAILocal",
    "voice": "male-qn-qingse",
    "response_format": "mp3"
  }' --output hello.mp3
```

**Voice IDs — use MiniMax-native, NOT OpenAI voice names:**

| Voice ID | Description |
|---|---|
| `male-qn-qingse` | Male, calm |
| `female-shaonv` | Female, young |
| `audiobook_male_2` | Male, narrator |
| `presenter_male` | Male, anchor-style |
| `clever_boy` | Male, youthful |

**Formats:** `mp3` (default), `pcm`, `flac`, `wav`. Override `bitrate`, `audio_sample_rate`, `channel` at the top level if needed.

**Rate limit:** Plus plan = ~1–5 RPM (very strict). MiniMax error 1002 → HTTP 429 → `ClassRateLimit` in failover taxonomy. Plan throughput accordingly.

---

## 🎵 Music & Lyrics Generation

MiniMax-powered music suite — three capabilities under `/v1/music/*`.

### Lyrics (fast, ~2-5s)

```bash
curl http://localhost:18080/v1/music/lyrics \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{"mode":"write_full_song","prompt":"a happy pop song about sunshine"}'
```

Returns `{song_title, style_tags, lyrics}` with structure tags `[Intro]`, `[Verse]`, `[Chorus]`, `[Bridge]`, `[Outro]`, `[Inst]`.

**Modes:** `write_full_song` (default) or `edit` (extend existing `lyrics`).

### Music generation (text-to-music, ~30-90s sync or ~20s TTFB streaming)

Generate a real song from lyrics. Plan daily quota: 100 songs. Two modes:

**Sync (default):** blocks until complete, returns JSON with base64 audio.
```bash
curl http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:music-2.6",
    "lyrics": "[Verse]\nCode flows through the night\n[Chorus]\nDebugging makes it right"
  }' | jq -r .data.audio | base64 -d > song.mp3
```

Returns `{data: {audio: <base64>, format, size_bytes, duration_ms, sample_rate, channels, bitrate}, model, trace_id}`. The adapter decodes MiniMax's hex-encoded audio server-side and hands base64 to clients (same convention as OpenAI `b64_json`).

**Streaming (`stream: true`):** returns raw `audio/mpeg` bytes as MiniMax generates them — first bytes in ~20s, ~50% less wire (adapter drops the duplicated terminal frame MiniMax emits).
```bash
curl -N http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{"model":"minimax:music-2.6","stream":true,"lyrics":"[Verse]\nHello world"}' > song.mp3
```

Use streaming when UX matters (web player, agent progress signal). Use sync when you need the metadata block (duration_ms, sample_rate, bitrate) upfront — those fields are logged server-side during streaming but not exposed to the client.

### Music cover (reference-audio style transfer)

Generate a cover of an existing track in a different style. Same endpoint, different `model`.

```bash
curl http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "minimax:music-cover",
    "prompt": "upbeat jazz cover with saxophone solo",
    "audio_url": "https://example.com/reference.mp3"
  }' | jq -r .data.audio | base64 -d > cover.mp3
```

Reference audio must be 6s–6min, ≤50MB, formats: mp3/wav/flac. Use `audio_url` OR `audio_base64` (mutually exclusive).

**Typical agent workflow:** generate lyrics first (`/v1/music/lyrics`), then synthesize music from them (`/v1/music/generations`). Both calls share the same MiniMax daily quota bucket (100/day each).

---

## 🎤 Audio Transcription (ASR)

Transcribe audio to text via `/v1/audio/transcriptions` using groq-hosted whisper:

```bash
curl http://localhost:18080/v1/audio/transcriptions \
  -H "Authorization: Bearer sk-test-123" \
  -F model=whisper-large-v3 \
  -F file=@voice-note.wav
```

Returns `{text: "..."}`. Supports `whisper-large-v3` (accurate) and `whisper-large-v3-turbo` (fast).

---

## 🧠 DeepSeek-V4 (1M Context Tier)

The only 1M-context model in the stack. Route here when your prompt ≥ 256k
or when you need 384k of output budget. Two SKUs, both 1M context:

- `deepseek:deepseek-v4-pro` — flagship, thinking mode default.
- `deepseek:deepseek-v4-flash` — faster/cheaper.

### Instant mode (answer in `content`)
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek:deepseek-v4-flash",
    "messages": [{"role":"user","content":"Capital of France?"}],
    "max_tokens": 4096,
    "thinking": {"type":"disabled"}
  }'
```

### Thinking mode (deep reasoning — answer in `content`, chain of thought in `reasoning_content`)
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek:deepseek-v4-pro",
    "messages": [{"role":"user","content":"Solve 3x+5=20"}],
    "thinking": {"type":"enabled"},
    "reasoning_effort": "high",
    "max_tokens": 4096
  }'
```

> Direct upstream URL is `https://api.deepseek.com/v1` (Anthropic-compatible variant at `/anthropic`). Through the gateway, just use `deepseek:<model>` — the proxy attaches the right key.

---

## 🤖 Kimi-K2.6 (Coding Tier, 256k)

Routed via `kimi:kimi-k2.6`. Same OpenAI-compatible shape. Defaults to
thinking mode — disable for instant `content`:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" -H "Content-Type: application/json" \
  -d '{
    "model": "kimi:kimi-k2.6",
    "messages": [{"role":"user","content":"Refactor this function: ..."}],
    "max_tokens": 4096,
    "thinking": {"type":"disabled"}
  }'
```

Also supports: streaming (`stream:true`), vision via inline base64 PNG/JPG
(remote URLs not supported on this tier), custom tool calling, and the
built-in `$web_search` tool (2-step round-trip — see
`references/examples.md`).

> **Why no 401 from Kimi:** the Kimi-for-Coding endpoint gates by
> User-Agent. The gateway forwards a `User-Agent: claude-cli/...` header
> on your behalf, which the upstream allowlist accepts. Direct curl
> against `api.kimi.com/coding/v1` from a generic UA returns 403
> `access_terminated_error`. Always call through the gateway.

---

## 🌐 Web Search (Built-in Tool)

MiniMax M2.7 supports live web search as a native chat tool. Enable via the `tools` field:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "minimax:MiniMax-M2.7",
    "messages": [{"role":"user","content":"What happened today in tech news?"}],
    "max_tokens": 2000,
    "tools": [{"type": "web_search"}]
  }'
```

**Critical:** set `max_tokens >= 2000`. Web search inflates prompt context to 6k–13k tokens (search results are folded in), so low budgets produce empty responses.

**Response shape:** the model does the search internally and returns the synthesized answer in `choices[0].message.content`. There are no `tool_calls` for the client to execute — MiniMax handles everything server-side.

---

## `ail` CLI Reference

```bash
ail start      # Start the local server
ail stop       # Stop the local server
ail restart    # Restart
ail status     # Check status of server and bridge
ail logs -f    # Follow server logs in real-time
ail update     # Pull latest + rebuild
```

---

## 🌲 Decision Tree

```
What do you need?
├─ FREE + Powerful + Files
│   └─ CLI Providers (geminicli:, claudecli:)
├─ FREE + Private + Fast
│   └─ Local Ollama (ollama:llama3.2)
├─ Ultra-Fast Production
│   └─ Cloud Provider (switchai:switchai-fast)
└─ I don't know, you pick
    └─ Intelligent Routing (auto)
```

---

## 🗺️ Skill References

| File | Description |
| ---- | ----------- |
| **SKILL.md** (this file) | Core workflow and quick reference |
| [docs-site/](docs-site/) | **Comprehensive Manual**: AI agents should crawl this directory for complete CLI, API, architecture, and SDK docs. |
| [references/routing.md](references/routing.md) | Auto-routing config, intent matrix, Lab |
| [references/management-api.md](references/management-api.md) | Full Management & Telemetry API |
| [references/examples.md](references/examples.md) | Real-world agentic use cases |
| [references/multimodal.md](references/multimodal.md) | Vision and image processing |
| [references/steering.md](references/steering.md) | Conditional routing rules |
| [references/hooks.md](references/hooks.md) | Automation and event hooks |
| [references/memory.md](references/memory.md) | Analytics and history |

For full human-readable documentation: [ail.traylinx.com](https://ail.traylinx.com)

---

## 🛠️ Troubleshooting

| Problem                | Fix                                           |
| ---------------------- | --------------------------------------------- |
| Connection error       | `ail status` → `ail start` if not running     |
| Model not found        | Ensure you used the `provider:` prefix        |
| 401 Unauthorized       | Check API key in `config.yaml`                |
| 403 Access Denied      | Likely a WAF block; the proxy auto-retries    |
| `auth_unavailable`     | `ail restart`                                 |
| No models listed       | Check `ail logs -f` for provider errors       |

## Cross-Cutting Rules (ALL AGENTS MUST FOLLOW)

1. **ALWAYS use `provider:model` format** — never bare model names.
2. **Prefer CLI Providers** — they are free and support file attachments.
3. **Use `auto`** for simple tasks — let the Cortex Router pick the best model.
4. **Use `ollama:` for privacy** — local models never send data externally.
5. **Check `/v1/models`** before routing — verify the model exists.
6. **Handle errors gracefully** — 503 = provider down, use fallback chain.

---

*Route wisely. Save tokens. Use CLI.* 🚀

