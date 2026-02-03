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

## Intelligent Systems

The intelligent systems module provides advanced capabilities including usage analytics, automated health checks, steering rules, and event hooks.

### Memory System
Tracks usage, costs, and performance metrics.

```yaml
memory:
  enabled: true
  base_dir: "./data/memory"  # Where to store analytics and logs
  retention_days: 90         # How long to keep daily logs
  max_log_size_mb: 10        # Rotate logs after this size
  compression: true          # Compress old logs
```

### Heartbeat Monitor
Monitors system health and quota usage.

```yaml
heartbeat:
  enabled: true
  interval: "5m"             # Check frequency
  timeout: "10s"             # API timeout for checks
  retry_delay: "1s"          # Delay between retries
  retry_attempts: 3          # Max retries per check
  auto_discovery: true       # Automatically discover models to check
  quota_warning_threshold: 80.0
  quota_critical_threshold: 95.0
  max_concurrent_checks: 5
```

### Steering Engine
Applies conditional routing rules based on context.

```yaml
steering:
  enabled: true
  rules_file: "config/steering/rules.yaml"
  hot_reload: true           # Reload rules when file changes
  reload_interval: "30s"     # How often to check for changes
```

### Hooks System
Executes actions based on system events.

```yaml
hooks:
  enabled: true
  hooks_file: "config/hooks/hooks.yaml"
  hot_reload: true           # Reload hooks when file changes
  reload_interval: "30s"     # How often to check for changes
  max_concurrent_hooks: 10   # Max hooks running in parallel
  execution_timeout: "5s"    # Timeout for hook execution
```

---

## Intelligent Systems

`switchAILocal` includes four intelligent systems that enhance routing, monitoring, and automation capabilities. All systems are **disabled by default** and must be explicitly enabled.

### Memory System

Records routing decisions, learns from outcomes, and provides analytics about provider performance.

```yaml
memory:
  enabled: true              # Enable memory recording
  retention-days: 30         # Keep records for 30 days
  storage-path: "./.memory"  # Storage directory (default: ./.memory)
```

**Features:**
- Records every routing decision with timestamp, provider, model, and outcome
- Tracks provider quirks and failure patterns
- Provides analytics via Management API
- Automatic cleanup of old records

### Heartbeat Monitor

Background service that monitors provider health and tracks quota usage.

```yaml
heartbeat:
  enabled: true              # Enable background monitoring
  interval: 5m               # Check interval (default: 5 minutes)
  timeout: 30s               # Health check timeout
  retry-attempts: 3          # Retries before marking provider down
  quota-warning: 80          # Emit warning at 80% quota usage
  quota-critical: 95         # Emit critical alert at 95% quota usage
```

**Features:**
- Periodic health checks for all configured providers
- Quota tracking with configurable thresholds
- Automatic provider status updates
- Event emission for failures and quota warnings

### Steering Engine

Rule-based routing system that modifies requests based on conditions.

```yaml
steering:
  enabled: true              # Enable steering rules
  rules-path: "./steering"   # Directory containing rule files
  hot-reload: true           # Reload rules on file changes
```

**Features:**
- Conditional routing based on request properties
- Request modification (model, parameters, provider)
- Hot-reload without server restart
- Rule priority and fallback support

**Example Rule File** (`steering/prefer-local.yaml`):
```yaml
name: "prefer-local-models"
priority: 100
conditions:
  - field: "model"
    operator: "contains"
    value: "llama"
actions:
  - type: "set-provider"
    value: "ollama"
```

### Hooks Manager

Event-driven automation system that executes actions on system events.

```yaml
hooks:
  enabled: true              # Enable hooks
  hooks-path: "./hooks"      # Directory containing hook files
  hot-reload: true           # Reload hooks on file changes
  max-concurrent: 10         # Max concurrent hook executions
```

**Features:**
- Trigger actions on system events (failures, quota warnings, routing decisions)
- Asynchronous execution (non-blocking)
- Hot-reload without server restart
- Multiple hook types: webhook, command, script

**Example Hook File** (`hooks/alert-on-failure.yaml`):
```yaml
name: "alert-on-provider-failure"
event: "health_check_failed"
action:
  type: "webhook"
  url: "https://alerts.example.com/webhook"
  method: "POST"
  body:
    provider: "{{.Provider}}"
    timestamp: "{{.Timestamp}}"
```

**Available Events:**
- `health_check_failed` - Provider health check failed
- `quota_warning` - Quota threshold warning (80%)
- `quota_critical` - Quota threshold critical (95%)
- `routing_decision` - Request routed to provider
- `request_failed` - Request failed
- `provider_status_change` - Provider status changed

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

