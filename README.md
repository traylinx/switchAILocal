<div align="center">
  <img src="assets/switchai_logo_trans.png" alt="switchAILocal Logo" width="120"/>

  <h1>switchAILocal</h1>

  <p><em>One local endpoint. All your AI providers.</em></p>

  <p>
    <a href="https://switchailocal.traylinx.com/introduction"><strong>Official Documentation</strong></a> •
    <a href="#quick-start">Quick Start</a> •
    <a href="docs/user/installation.md">Installation</a> •
    <a href="docs/user/providers.md">Setup Providers</a> •
    <a href="docs/user/api-reference.md">API Reference</a>
  </p>
</div>

---

## What is switchAILocal?

**switchAILocal** is a unified API gateway that lets you use **all your AI providers** through a single OpenAI-compatible endpoint running on your machine.

### Key Benefits

| Feature                       | Description                                                                                                                                                          |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 🎨 **Modern Web UI**           | Single-file React dashboard to configure providers, manage model routing, and adjust settings (226 KB, zero dependencies)                                            |
| 🔑 **Use Your Subscriptions**  | Connect Gemini CLI, Claude Code, Codex, Ollama, and more—no API keys needed                                                                                          |
| 🎯 **Single Endpoint**         | Any OpenAI-compatible tool works with `http://localhost:18080`                                                                                                       |
| 📎 **CLI Attachments**         | Pass files and folders directly to CLI providers via `extra_body.cli`                                                                                                |
| 🧠 **Superbrain Intelligence** | Autonomous self-healing: monitors executions, diagnoses failures with AI, auto-responds to prompts, restarts with corrective flags, and routes to fallback providers |
| ⚖️ **Load Balancing**          | Round-robin across multiple accounts per provider                                                                                                                    |
| 🔄 **Intelligent Failover**    | Smart routing to alternatives based on capabilities and success rates                                                                                                |
| 📊 **Observability Ready**     | Enterprise-grade JSON structured logging, raw NDJSON event streams, and a native `/metrics` endpoint for Prometheus & Grafana integration                            |
| 🔒 **Local-First**             | Everything runs on your machine, your data never leaves                                                                                                              |

---

## Supported Providers

### CLI Tools (Use Your Paid Subscriptions)

| Provider         | CLI Tool   | Prefix       | Status  |
| ---------------- | ---------- | ------------ | ------- |
| Google Gemini    | `gemini`   | `geminicli:` | ✅ Ready |
| Anthropic Claude | `claude`   | `claudecli:` | ✅ Ready |
| OpenAI Codex     | `codex`    | `codex:`     | ✅ Ready |
| Mistral Vibe     | `vibe`     | `vibe:`      | ✅ Ready |
| OpenCode         | `opencode` | `opencode:`  | ✅ Ready |

### Local Models

| Provider  | Prefix      | Status  |
| --------- | ----------- | ------- |
| Ollama    | `ollama:`   | ✅ Ready |
| LM Studio | `lmstudio:` | ✅ Ready |

### Cloud APIs

| Provider              | Prefix           | Status  |
| --------------------- | ---------------- | ------- |
| **Traylinx switchAI** | `switchai:`      | ✅ Ready |
| Google AI Studio      | `gemini:`        | ✅ Ready |
| Anthropic API         | `claude:`        | ✅ Ready |
| OpenAI API            | `openai:`        | ✅ Ready |
| OpenRouter            | `openai-compat:` | ✅ Ready |

---

## Prerequisites

| Requirement | Minimum  | Notes                                        |
| ----------- | -------- | -------------------------------------------- |
| **Node.js** | 18+      | For `npx` install (recommended)              |
| **Go**      | 1.25+    | Only needed for building from source         |
| **Docker**  | Optional | Only needed for `ail start --docker`         |
| **macOS**   | Ventura+ | Linux support is experimental                |

---

## Quick Start

### Option 1: npx (Recommended)

```bash
npx @traylinx/switchailocal
```

That's it. Downloads the right binary for your platform, caches it, and runs.

### Option 2: Docker

```bash
docker run -p 18080:18080 ghcr.io/traylinx/switchailocal:latest
```

### Option 3: Build from Source

```bash
git clone https://github.com/traylinx/switchAILocal.git
cd switchAILocal

# First-time setup (builds binaries, installs bridge service, registers CLI)
./ail.sh setup

# Then start the server (from anywhere, after setup)
ail start

# OR start with Docker (add --build to force rebuild)
ail start --docker --build
```

### 2. Connect Your Providers

Choose the authentication method that works best for you:

#### Option A: Local CLI Wrappers (Recommended - Zero Setup)

**If you already have `gemini`, `claude`, or `vibe` CLI tools installed and authenticated**, switchAILocal uses them automatically. **No additional login required!**

```bash
# Just use the CLI prefix - it works immediately
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "geminicli:gemini-2.5-pro", "messages": [...]}'
```

- ✅ **Zero configuration** - Uses your existing CLI authentication
- ✅ **Works immediately** - No `--login` needed
- ✅ **Supports:** `geminicli:`, `claudecli:`, `codex:`, `vibe:`, `opencode:`

#### Option B: API Keys (Standard)

Add your AI Studio or Anthropic API keys to `config.yaml`:

```yaml
gemini:
  api-key: "your-gemini-api-key"
claude:
  api-key: "your-claude-api-key"
```

Then use without the `cli` suffix: `gemini:`, `claude:`

#### Option C: OAuth Cloud Proxy (Advanced - Alternative to CLI)

**Only needed if:**
- ❌ You don't have the CLI tools installed
- ❌ You don't have API keys
- ✅ You want switchAILocal to manage OAuth tokens directly

```bash
# Optional OAuth login (alternative to CLI wrappers)
./switchAILocal --login        # Google Gemini OAuth
./switchAILocal --claude-login # Anthropic Claude OAuth
```

⚠️ **Note:** This requires `GEMINI_CLIENT_ID` and `GEMINI_CLIENT_SECRET` environment variables. Most users should use **Option A** (CLI wrappers) instead.

📖 See the [Provider Guide](docs/user/providers.md) for detailed setup instructions.

### 3. Check Status

```bash
./ail.sh status
```

The server runs on `http://localhost:18080`.

---

## Usage Examples

### Basic Request (Auto-Routing)

When you omit the provider prefix, switchAILocal automatically routes to an available provider:

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

Use the `provider:model` format to route to a specific provider:

```bash
# Force Gemini CLI
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Force Ollama
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "ollama:llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Force Claude CLI
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "claudecli:claude-sonnet-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# Force LM Studio
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "lmstudio:mistral-7b",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```


### Custom Providers & Legacy Client Support

If you use external generic APIs (e.g., DashScope, Groq) via `openai-compatibility` but your HTTP client strictly hardcodes a legacy model string (e.g., `"model": "alibaba:qwen-plus"` with a colon), you can seamlessly intercept this payload by assigning an explicit `alias`:

```yaml
# config.yaml
openai-compatibility:
  - name: "alibaba"
    prefix: "alibaba"
    base-url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
    models-url: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/models"
    api-key-entries:
      - api-key: "sk-your-key-here"
    models:
      - name: "qwen-plus"                # Actual DashScope upstream model
        alias: "alibaba:qwen-plus"       # Captures legacy strictly formatted client requests
```

Now, your locked-in client can directly query switchAILocal without code changes:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-key" \
  -d '{
    "model": "alibaba:qwen-plus",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### List Available Models

```bash
curl http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123"

# Filter by modality (text, image, audio, embedding, vision)
curl http://localhost:18080/v1/models?modality=image \
  -H "Authorization: Bearer sk-test-123"
```

### Multimodal API

switchAILocal supports the full OpenAI multimodal API surface:

#### Embeddings
```bash
curl http://localhost:18080/v1/embeddings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "switchai-embed", "input": "Hello world"}'
```

#### Image Generation
```bash
curl http://localhost:18080/v1/images/generations \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "dall-e-3", "prompt": "A sunset over mountains", "size": "1024x1024"}'
```

#### Image Editing
```bash
curl http://localhost:18080/v1/images/edits \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{"model": "dall-e-3", "prompt": "Add a rainbow", "images": [{"image_url": "https://example.com/photo.png"}]}'
```

#### Text-to-Speech

Works natively via the MiniMax T2A adapter — gateway translates OpenAI shape or MiniMax-native shape to MiniMax `/v1/t2a_v2` behind the scenes. Use MiniMax voice IDs (not OpenAI voice names):

```bash
curl http://localhost:18080/v1/audio/speech \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "ail-speech",
    "input": "Hello from switchAILocal",
    "voice": "English_expressive_narrator",
    "response_format": "mp3"
  }' --output speech.mp3
```

MiniMax-native T2A controls are also accepted through the same endpoint:

```bash
curl http://localhost:18080/v1/audio/speech \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "ail-speech",
    "text": "Omg(sighs), hello from switchAILocal",
    "stream": false,
    "voice_setting": {"voice_id": "English_expressive_narrator", "speed": 1, "vol": 1, "pitch": 0},
    "audio_setting": {"sample_rate": 32000, "bitrate": 128000, "format": "mp3", "channel": 1},
    "language_boost": "auto",
    "output_format": "hex"
  }' --output speech.mp3
```

Common voice IDs: `English_expressive_narrator`, `male-qn-qingse`, `female-shaonv`, `audiobook_male_2`, `presenter_male`, `clever_boy`. Formats: `mp3`, `flac`, `wav`.

#### Audio Transcription
```bash
curl http://localhost:18080/v1/audio/transcriptions \
  -H "Authorization: Bearer sk-test-123" \
  -F file="@audio.mp3" \
  -F model="whisper-large-v3"
```

#### Music Generation

Generate songs (text-to-music) or covers via MiniMax. 30–90s sync, 100/day on Plus plan. Pass `stream: true` to stream raw MP3 bytes as they arrive (~20s TTFB instead of ~60s wall-clock).

```bash
# Sync: returns JSON with base64-encoded MP3
curl http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ail-music",
    "lyrics": "[Verse]\nHello world\n[Chorus]\nLet us sing"
  }' | jq -r .data.audio | base64 -d > song.mp3

# Stream: returns audio/mpeg directly, playable as it downloads
curl -N http://localhost:18080/v1/music/generations \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ail-music",
    "stream": true,
    "lyrics": "[Verse]\nHello world\n[Chorus]\nLet us sing"
  }' > song.mp3
```

Sync returns `{data: {audio: <base64>, duration_ms, sample_rate, channels, bitrate}, model, trace_id}`. Streaming returns `Content-Type: audio/mpeg` with progressive MP3 frames. For cover mode use `"model": "ail-music-cover"` with `"audio_url"` or `"audio_base64"` + `"prompt"`.

#### Lyrics Generation

```bash
curl http://localhost:18080/v1/music/lyrics \
  -H "Authorization: Bearer sk-test-123" \
  -H "Content-Type: application/json" \
  -d '{"mode":"write_full_song","prompt":"happy pop about coding"}'
```

Returns `{song_title, style_tags, lyrics}` with `[Verse]`/`[Chorus]` structure tags.

#### Audio Translation (to English)
```bash
curl http://localhost:18080/v1/audio/translations \
  -H "Authorization: Bearer sk-test-123" \
  -F file="@german_audio.mp3" \
  -F model="whisper-1"
```

### Advanced Features

switchAILocal passes through all standard and provider-specific parameters, so you can use tools, thinking mode, vision, and more — just like talking directly to the upstream provider.

#### Tool Calling (Web Search)

Built-in tools like web search are forwarded as-is to the upstream. **MiniMax M2.7** is the canonical web-search provider — the model runs the search internally and returns the synthesized answer in `choices[0].message.content`.

> **Critical:** set `max_tokens >= 2000` when using web_search. Search results are folded into the prompt and inflate context to 6k–13k tokens — lower budgets produce truncated or empty responses.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "ail-compound",
    "messages": [
      {"role": "user", "content": "What are the latest AI news today?"}
    ],
    "max_tokens": 2000,
    "tools": [
      {
        "type": "web_search",
        "max_keyword": 3,
        "force_search": true,
        "limit": 3
      }
    ],
    "tool_choice": "auto"
  }'
```

#### Function Calling (OpenAI Standard)

Standard OpenAI function calling works with any compatible provider:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [
      {"role": "user", "content": "What is the weather in Berlin?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string", "description": "City name"},
              "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'
```

#### Thinking / Reasoning Mode

Enable extended thinking for complex reasoning tasks:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "claudecli:claude-sonnet-4",
    "messages": [
      {"role": "user", "content": "Prove that the square root of 2 is irrational"}
    ],
    "thinking": {
      "type": "enabled",
      "budget_tokens": 10000
    }
  }'
```

#### Vision (Image Analysis)

Send images for analysis using the standard multimodal content format:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What do you see in this image?"},
          {"type": "image_url", "image_url": {"url": "https://example.com/photo.jpg"}}
        ]
      }
    ]
  }'
```

#### CLI Attachments (Files & Folders)

Pass files and folders directly to CLI providers via `extra_body.cli`:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [
      {"role": "user", "content": "Review this code for security issues"}
    ],
    "extra_body": {
      "cli": {
        "files": ["/path/to/main.go", "/path/to/server.go"],
        "directories": ["/path/to/internal/"]
      }
    }
  }'
```

#### Streaming

All chat endpoints support SSE streaming:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -N \
  -d '{
    "model": "gemini-2.5-pro",
    "messages": [{"role": "user", "content": "Tell me a story"}],
    "stream": true
  }'
```

---

## SDK Integration

### Python (OpenAI SDK)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:18080/v1",
    api_key="sk-test-123",  # Must match a key in config.yaml
)

# Recommended: Auto-routing (switchAILocal picks the best available provider)
completion = client.chat.completions.create(
    model="gemini-2.5-pro",  # No prefix = auto-route to any logged-in provider
    messages=[
        {"role": "user", "content": "What is the meaning of life?"}
    ]
)
print(completion.choices[0].message.content)

# Streaming example
stream = client.chat.completions.create(
    model="gemini-2.5-pro",
    messages=[{"role": "user", "content": "Tell me a story"}],
    stream=True,
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)

# Optional: Explicit provider selection (use prefix only when needed)
completion = client.chat.completions.create(
    model="ollama:llama3.2",  # Force Ollama provider
    messages=[{"role": "user", "content": "Hello!"}]
)
```

### JavaScript/Node.js (OpenAI SDK)

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:18080/v1',
  apiKey: 'sk-test-123', // Must match a key in config.yaml
});

async function main() {
  // Auto-routing
  const completion = await client.chat.completions.create({
    model: 'gemini-2.5-pro',
    messages: [
      { role: 'user', content: 'What is the meaning of life?' }
    ],
  });

  console.log(completion.choices[0].message.content);

  // Explicit provider selection
  const ollamaResponse = await client.chat.completions.create({
    model: 'ollama:llama3.2',  // Force Ollama
    messages: [
      { role: 'user', content: 'Hello!' }
    ],
  });
}

main();
```

### Streaming Example (Python)

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:18080/v1",
    api_key="sk-test-123",
)

stream = client.chat.completions.create(
    model="geminicli:gemini-2.5-pro",
    messages=[{"role": "user", "content": "Tell me a story"}],
    stream=True,
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

---

## Configuration

All settings are in `config.yaml`. Copy the example to get started:

```bash
cp config.example.yaml config.yaml
```

Key configuration options:

```yaml
# Server port (default: 18080)
port: 18080

# Enable Ollama integration
ollama:
  enabled: true
  base-url: "http://localhost:11434"

# Enable LM Studio
lmstudio:
  enabled: true
  base-url: "http://localhost:1234/v1"

# Enable LUA plugins for request/response modification
plugin:
  enabled: true
  plugin-dir: "./plugins"
```

📖 See [Configuration Guide](docs/user/configuration.md) for all options.

---

## Cortex Router: Intelligent Model Selection

The **Cortex Router** plugin provides intelligent, multi-tier routing that automatically selects the optimal model based on request content.

### Quick Start

Enable intelligent routing in `config.yaml`:

```yaml
plugin:
  enabled: true
  enabled-plugins:
    - "cortex-router"

intelligence:
  enabled: true
  router-model: "ollama:qwen:0.5b"  # Fast classification model
  matrix:
    coding: "switchai-chat"
    reasoning: "switchai-reasoner"
    fast: "switchai-fast"
    secure: "ollama:llama3.2"  # Local model for sensitive data
```

### How It Works

When you use `model="auto"` or `model="cortex"`, the router analyzes your request through multiple tiers:

1. **Reflex Tier** (<1ms): Pattern matching for obvious cases (code blocks → coding model, PII → secure model)
2. **Semantic Tier** (<20ms): Embedding-based intent matching (requires Phase 2)
3. **Cognitive Tier** (200-500ms): LLM-based classification with confidence scoring

```python
# Automatic intelligent routing
completion = client.chat.completions.create(
    model="auto",  # Let Cortex Router decide
    messages=[{"role": "user", "content": "Write a Python function to sort a list"}]
)
# → Routes to coding model automatically
```

### Phase 2 Features (Optional)

Enable advanced features for even smarter routing:

```yaml
intelligence:
  enabled: true
  
  # Semantic matching (faster than LLM classification)
  embedding:
    enabled: true
  semantic-tier:
    enabled: true
  
  # Skill-based prompt augmentation
  skill-matching:
    enabled: true
  
  # Quality-based model cascading
  cascade:
    enabled: true
```

**21 Pre-built Skills** including:
- Language experts (Go, Python, TypeScript)
- Infrastructure (Docker, Kubernetes, DevOps)
- Security, Testing, Debugging
- Frontend, Vision, and more

📖 See [Cortex Router Phase 2 Guide](docs/CORTEX_ROUTER_PHASE2.md) for full documentation.

---

## Documentation

📚 **[Read the Official switchAILocal Documentation](https://switchailocal.traylinx.com/introduction)**

### For Users

| Guide                                                     | Description                                                |
| --------------------------------------------------------- | ---------------------------------------------------------- |
| [Installation](docs/user/installation.md)                 | Getting started guide                                      |
| [Performance Guide](docs/user/performance_guide.md)       | Rate limiting, load shedding, and profiling                |
| [Configuration](docs/user/configuration.md)               | All configuration options                                  |
| [Providers](docs/user/providers.md)                       | Setting up AI providers                                    |
| [API Reference](docs/user/api-reference.md)               | REST API documentation                                     |
| [Intelligent Systems](docs/user/intelligent-systems.md)   | Memory, Heartbeat, Steering, and Hooks                     |
| [Advanced Features](docs/user/advanced-features.md)       | Payload overrides, failover, and more                      |
| [State Box](docs/user/state-box.md)                       | Secure state management & configuration                    |
| [Management Dashboard](docs/user/management-dashboard.md) | Modern web UI for provider setup, model routing & settings |

### Build from Source

```bash
# Recommended: use the setup script
./ail.sh setup

# OR build manually
go build -o switchAILocal ./cmd/server
go build -o bridge-agent ./cmd/bridge-agent

# Build the Management UI (optional)
./ail_ui.sh
```

### For Developers

| Guide                                          | Description                         |
| ---------------------------------------------- | ----------------------------------- |
| [SDK Usage](docs/developer/sdk-usage.md)       | Embed switchAILocal in your Go apps |
| [LUA Plugins](docs/developer/lua-plugins.md)   | Custom request/response hooks       |
| [SDK Advanced](docs/developer/sdk-advanced.md) | Create custom providers             |

---

## Contributing

Contributions are welcome!

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes
4. Push and open a Pull Request

---

## License

MIT License - see [LICENSE](LICENSE) for details.

---

<div align="center">
  <strong>Maintained by Sebastian Schkudlara</strong>
</div>
