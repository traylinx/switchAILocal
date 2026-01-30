# Configuration Guide

`switchAILocal` is configured via a `config.yaml` file. This file controls server behavior, security, routing strategies, and provider credentials.

## Server Settings

| Parameter                  | Description                                                       | Default         |
| -------------------------- | ----------------------------------------------------------------- | --------------- |
| `host`                     | Interface to bind to (`""` for all, `127.0.0.1` for local only).  | `""`            |
| `port`                     | Network port for the API server.                                  | `18080`         |
| `debug`                    | Enable verbose debug logging.                                     | `false`         |
| `logging-to-file`          | Write logs to rotating files in `logs/` directory.                | `false`         |
| `logs-max-total-size-mb`   | Max total size of log files before rotation.                      | `0` (unlimited) |
| `usage-statistics-enabled` | Enable in-memory usage aggregation for the dashboard.             | `false`         |
| `disable-cooling`          | Disable quota cooldown scheduling.                                | `false`         |
| `request-retry`            | Number of retries for failed requests.                            | `0`             |
| `max-retry-interval`       | Max wait time (seconds) before retrying a cooled-down credential. | `0`             |

---

## Remote Management

The Management Control Panel (found at `/management`) allows for GUI-based administration.

```yaml
remote-management:
  secret-key: "your-strong-password" # Plaintext or bcrypt
  allow-remote: false              # Allow access from non-localhost IPs
  disable-control-panel: false     # Skip serving the web UI
  panel-github-repository: "..."   # Override asset source
```

---

## Routing & Quota

Control how credentials are selected and what happens when limits are hit.

### Routing Strategy
```yaml
routing:
  strategy: "round-robin" # Options: "round-robin", "fill-first"
```

### Quota Exceeded Behavior
```yaml
quota-exceeded:
  switch-project: true       # Switch to another project/key on 429
  switch-preview-model: true # Switch to a preview model if available
```

## Cloud APIs

### Traylinx switchAI (Recommended)

Dedicated configuration for the [Traylinx switchAI](https://traylinx.com/switchai) gateway.

```yaml
switchai-api-key:
  - api-key: "sk-lf-..."
    base-url: "https://switchai.traylinx.com/v1"
```

**Note**: You don't need to list models - any switchAI model works automatically!

---

### Explicit Model Configuration

**IMPORTANT**: Starting from v1.5, `switchAILocal` does **not** have hardcoded default models for API providers (Gemini, Claude, OpenAI).
If you configure an API key but do not list any models, that provider will have **zero** models available.

You must explicitly list the models you wish to use:

```yaml
gemini-api-key:
  - api-key: "AIza..."
    models:
      - name: "gemini-1.5-pro"  # Upstream model name
        alias: "pro"            # Client-facing alias
```

---

## Intelligence & Capabilities

The `intelligence` section controls the "Cortex" routing engine and defines which models are used for specific non-chat capabilities.

### Capability Matrix
Use the `matrix` to map tasks to models when no specific model is requested by the client.

```yaml
intelligence:
  enabled: true
  matrix:
    image_gen: "gemini:imagen-3.0"   # Default for /v1/images/generations
    transcription: "openai:whisper-1" # Default for /v1/audio/transcriptions
    speech: "openai:tts-1"           # Default for /v1/audio/speech
```

---

## Model Aliases

Create shortcuts for long model names. This works for any provider.

### Example: Without aliases
```bash
curl -d '{"model": "switchai:deepseek-reasoner", ...}'
curl -d '{"model": "switchai:meta-llama/llama-4-maverick-17b-128e-instruct", ...}'
```

### Example: With aliases
```yaml
switchai-api-key:
  - api-key: "sk-lf-..."
    models:
      - name: "deepseek-reasoner"
        alias: "reasoner"
      - name: "meta-llama/llama-4-maverick-17b-128e-instruct"
        alias: "llama4"
```

Now you can use:

```bash
curl -d '{"model": "switchai:reasoner", ...}'  # → deepseek-reasoner
curl -d '{"model": "switchai:llama4", ...}'    # → llama-4-maverick
```

---

### OpenAI & Compatible

```yaml
openai-compatibility:
  - name: "my-provider"
    base-url: "https://api.example.com/v1"
    api-key-entries:
      - api-key: "sk-..."
        proxy-url: "http://my-proxy:8080" # Optional per-key proxy
    models:
      - name: "upstream-model-name"
        alias: "client-facing-alias"
```

### Google Gemini

```yaml
gemini-api-key:

  - api-key: "AIza..."
    prefix: "prod"         # Optional model namespace (prod/gemini-...)
    proxy-url: "..."       # Optional per-key proxy
    excluded-models: ["*"] # Optional models to skip
```

### Anthropic Claude

```yaml
claude-api-key:
  - api-key: "sk-ant-..."
    base-url: "..."        # Optional custom endpoint
    models:
      - name: "claude-3-opus"
        alias: "opus"
```

---

## Local Systems

### Ollama

```yaml
ollama:
  enabled: true
  base-url: "http://localhost:11434"
  auto-discover: true # Fetch model list from Ollama automatically
```

### LM Studio

```yaml
lmstudio:
  enabled: true
  base-url: "http://localhost:1234/v1"
  auto-discover: true # Fetch model list from LM Studio automatically
```

### LUA Plugins

```yaml
plugin:
  enabled: true
  plugin-dir: "./plugins"
```

