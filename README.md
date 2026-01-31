<div align="center">
  <img src="assets/switchai_icon_logo.png" alt="switchAILocal Logo" width="120"/>

  <h1>switchAILocal</h1>

  <p><em>One local endpoint. All your AI providers.</em></p>

  <p>
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
| 🔑 **Use Your Subscriptions**  | Connect Gemini CLI, Claude Code, Codex, Ollama, and more—no API keys needed                                                                                          |
| 🎯 **Single Endpoint**         | Any OpenAI-compatible tool works with `http://localhost:18080`                                                                                                       |
| 📎 **CLI Attachments**         | Pass files and folders directly to CLI providers via `extra_body.cli`                                                                                                |
| 🧠 **Superbrain Intelligence** | Autonomous self-healing: monitors executions, diagnoses failures with AI, auto-responds to prompts, restarts with corrective flags, and routes to fallback providers |
| ⚖️ **Load Balancing**          | Round-robin across multiple accounts per provider                                                                                                                    |
| 🔄 **Intelligent Failover**    | Smart routing to alternatives based on capabilities and success rates                                                                                                |
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

## Quick Start

### 1. Clone & Start (The Easy Way)

We provide a unified Hub Script (`ail.sh`) to manage everything.

```bash
git clone https://github.com/traylinx/switchAILocal.git
cd switchAILocal

# Start locally (builds automatically)
./ail.sh start

# OR start with Docker (add --build to force rebuild)
./ail.sh start --docker --build
```

### 2. Connect Your Providers

#### Option A: Local CLI Wrappers (Easiest)
If you have tools like `gemini`, `claude`, or `vibe` in your PATH, **switchAILocal finds them automatically.**

- **Usage:** Use `geminicli:`, `claudecli:`, etc.
- **Auth:** Uses your existing local CLI session.

#### Option B: API Keys (Standard)
Add your AI Studio or Anthropic keys to `config.yaml`.

#### Option C: Cloud Proxy Mode (Advanced OAuth)
Run once to link your account (useful if you don't have API keys):

```bash
./switchAILocal --login        # Google Gemini
./switchAILocal --claude-login # Anthropic Claude
```

📖 See the [Provider Guide](docs/user/providers.md) for a full comparison.

### 3. Check Status

```bash
./ail.sh status
```

The server runs on `http://localhost:18080`.

The server starts on `http://localhost:18080`.

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

### List Available Models

```bash
curl http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123"
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

### For Users

| [Management Dashboard](docs/user/management-dashboard.md) | Graphical configuration & model management |

### Build from Source

```bash
# Build the main server
go build -o switchAILocal ./cmd/server

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
