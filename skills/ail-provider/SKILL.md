---
name: ail-provider
description: |
  Configure Claude Code to route all API requests through switchAILocal, a local
  AI gateway that unifies Gemini, Claude, Ollama, OpenAI, Groq, Alibaba, Xiaomi,
  and other providers behind a single OpenAI-compatible endpoint. Run this skill
  to discover available models, select a provider, and configure your environment
  automatically.
allowed-tools:
  - Bash
  - Read
  - Write
  - AskUserQuestion

---

# switchAILocal Provider Setup

Configure **switchAILocal** (`ail`) as the primary API provider for AI agents like Claude Code. This allows the user to route requests securely to multiple providers using the local `http://localhost:18080/v1` gateway.

## Decision Tree

Pick the right script based on what the user is asking. **Execute these scripts directly in your bash tool.**

| User wants to... | Script Command | Example query |
|---|---|---|
| Check if the proxy is running | `python3 <skill-path>/scripts/status.py` | "Is switchAILocal running?" |
| See all available models | `python3 <skill-path>/scripts/list-models.py` | "What models can I use?" |
| Configure Claude Code to use a model | `python3 <skill-path>/scripts/set-provider.py <model-id>` | "Set my provider to geminicli:" |

*Note: Replace `<skill-path>` with the absolute path to this skill directory.*

---

## 🚀 Workflow: Interactive Setup

If the user says "Setup my AI provider" or "Configure my models", follow this exact workflow:

### 1. Check Status
Run the health check to ensure the proxy is alive:
```bash
python3 <skill-path>/scripts/status.py
```

If it returns `OFFLINE`, instruct the user to start the server: "switchAILocal is offline. Please open a new terminal and run `ail start`, then let me know when it's running."

### 2. Discover Models
Fetch the available models grouped by provider:
```bash
python3 <skill-path>/scripts/list-models.py
```

**Present the results to the user as a clean Markdown list.** Call out the recommended models (marked with ⭐ or 🧠).

### 3. Ask for Selection
Use the `AskUserQuestion` tool to prompt the user:
> Which model ID would you like to use? (For example: `geminicli:gemini-2.5-pro`, `switchai:switchai-fast`, or `auto`)

### 4. Apply Configuration
Once the user provides a model ID, apply it immediately:
```bash
python3 <skill-path>/scripts/set-provider.py "MODEL_ID_GIVEN_BY_USER"
```

If the verification ping succeeds, confirm to the user that their environment is now fully configured to use switchAILocal.

---

## ⚠️ Critical Rules for AI Agents

1. **NEVER guess the model ID.** Always run `list-models.py` first to see what the user actually has installed, or ask them for the exact ID.
2. **Always include the prefix.** `geminicli:gemini-2.5-pro` is correct. `gemini-2.5-pro` will fail if the user is expecting CLI-based access.
3. **Shell Environment:** `set-provider.py` alters `~/.zshrc`. Remind the user they might need to restart their terminal if they intend to spawn new tabs, though Claude Code will pick up the `settings.json` changes immediately.
4. **Use `auto` for Smart Routing:** If the user isn't sure which model to use, recommend the `auto` model ID to enable Cortex Auto-Routing.
5. **On Makakoo VPS / Tytus droplets, prefer the stable `ail-*` aliases** (below) and **never hardcode upstream provider model IDs.** Calling a raw id the droplet doesn't serve (e.g. `kimi:kimi-k2.6`, `qwen3-embedding:0.6b`) returns `400 unknown provider for model` and silently degrades the feature.

---

## 🛰️ Stable Droplet AIL Aliases (Makakoo VPS / Tytus)

One stable alias per modality. The upstream model behind an alias may change; the alias is the contract. Confirm what's live with `GET /v1/models` — an alias only appears if its provider key is set.

| Alias            | Modality — endpoint                                       | Provider key needed |
| ---------------- | --------------------------------------------------------- | ------------------- |
| `ail-compound`   | chat + tools + vision — `/v1/chat/completions`            | MiniMax / Alibaba   |
| `ail-vision`     | image understanding — `/v1/chat/completions` (`image_url`)| Alibaba / MiniMax   |
| `ail-embed`      | embeddings — `/v1/embeddings`                             | local / Alibaba     |
| `ail-transcribe` | speech→text (ASR) — `/v1/audio/transcriptions` (multipart)| Alibaba (qwen3-asr) |
| `ail-speech`     | text→speech (TTS) — `/v1/audio/speech`                    | MiniMax             |
| `ail-image`      | image generation — `/v1/images/generations`              | MiniMax             |

`ail-compound` is the stable public contract. Current production pool is weighted 50/50 across `MiniMax-M3` and Alibaba `glm-5.2`, both declared with 1M context and thinking enabled for agent work. `ail-fast` may exist as a hidden compatibility alias on older clients, but do not teach new agents to depend on it.

Point env knobs at aliases, never raw ids: `EMBEDDING_MODEL=ail-embed`, `OMNI_MODEL=ail-compound`, `AIL_TRANSCRIBE_MODEL=ail-transcribe`, `AIL_VISION_MODEL=ail-vision`.
