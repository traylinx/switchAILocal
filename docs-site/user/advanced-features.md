# Advanced Features

This guide covers advanced configuration and integration options for `switchAILocal`.

## Payload Overrides & Defaults

`switchAILocal` allows you to modify JSON payloads using a rule-based system. This is useful for injecting parameters (like `temperature` or `max_tokens`) across all requests for specific models without changing your client code.

### Configuration

Add the `payload` block to your `config.yaml`:

```yaml
payload:
  # Defaults ONLY fill missing fields
  default:
    - models:
        - name: "gpt-*" # Wildcard matching
          protocol: "openai"
      params:
        "temperature": 0.7
        "max_tokens": 2000

  # Overrides ALWAYS overwrite existing values
  override:
    - models:
        - name: "gemini-*-pro"
      params:
        "safetySettings": [] # Force disable safety
```

### Syntax
- **Model Patterns**: Supports simple wildcard `*` (e.g., `*-sonnet`, `gemini-*`).
- **Paths**: Uses [gjson/sjson](https://github.com/tidwall/sjson) syntax for targeting nested fields (e.g., `request.parameters.temperature`).

---

## Amp CLI Integration

Integrate with the Amp CLI control plane to leverage OAuth-based ChatGPT and Anthropic subscriptions.

```yaml
ampcode:
  upstream-url: "https://amp.example.com"
  upstream-api-key: "..."
  restrict-management-to-localhost: true
  model-mappings:
    - from: "claude-opus-4.5"
      to: "claude-3-opus-20240229"
```

---

## HTTPS / TLS

Enable HTTPS by providing certificate and key files.

```yaml
tls:
  enable: true
  cert: "/path/to/fullchain.pem"
  key: "/path/to/privkey.pem"
```

---

## High Availability & Failover

`switchAILocal` includes built-in mechanisms to ensure reliability and high availability for your LLM requests.

### Multi-Provider Failover

The system automatically handles provider failures by transparently failing over to other available providers that support the same model.

**How it works:**
1. If you request a model (e.g., `llama3.2`) that is served by multiple providers (e.g., `ollama` and `vibe`).
2. `switchAILocal` attempts to call the first available provider.
3. If that provider fails (network error, 500 Internal Server Error, etc.), the request is **immediately retried** with the next available provider.

**No configuration required:** This behavior is automatic as long as multiple providers serve the same model name.

### Rate Limits & Smart Retries

When a provider returns a `429 Too Many Requests` status:
1. **Cooldown**: The specific credential/provider is placed on a temporary "cooldown" list.
2. **Retry-After**: The system respects the `Retry-After` header provided by the upstream API.
3. **Queueing**: Requests are queued and retried once the cooldown period expires, or routed to an alternative provider if available.

### Configuration

You can tune the retry behavior in `config.yaml`:

```yaml
quota-exceeded:
  switch-project: true       # Try next available key/project in the list
  switch-preview-model: true # Attempt to use a preview version if the main model is rate-limited
  disable-cooling: false     # Set to true to disable cooldowns (not recommended)
  max-retry-interval: 60     # Max seconds to wait for a retry
```

---

## Usage Statistics

Enable in-memory aggregation of request metadata (latency, token counts, error rates). These can be viewed in real-time on the Management Dashboard (`/management.html`).

```yaml
usage-statistics-enabled: true
```

---

## Intelligent Systems Integration

The four intelligent systems (Memory, Heartbeat, Steering, Hooks) work together to provide enhanced routing, monitoring, and automation:

### Request Flow with Intelligent Systems

1. **Request Arrives** → HTTP handler receives request
2. **Steering Applied** → Steering engine evaluates rules and may modify request (model, provider, parameters)
3. **Routing Decision** → Server selects provider based on availability and rules
4. **Memory Recording** → Routing decision is recorded with timestamp and context
5. **Request Forwarded** → Request sent to selected provider
6. **Response Returned** → Response sent back to client
7. **Outcome Updated** → Memory system updates decision with success/failure outcome
8. **Events Emitted** → Routing events trigger any matching hooks

### Background Operations

- **Heartbeat Monitor** runs periodic health checks on all providers
- **Memory Cleanup** removes old records based on retention policy
- **Hook Execution** processes events asynchronously without blocking requests
- **Analytics Computation** calculates provider performance metrics

### Hot-Reload Workflow

When steering rules or hooks are modified:

1. File watcher detects changes
2. System validates new configuration
3. If valid, new rules/hooks are loaded atomically
4. Old configuration is replaced
5. In-flight requests continue with old configuration
6. New requests use new configuration
7. Event is emitted for monitoring

### Performance Impact

- **Disabled**: Zero overhead (systems not initialized)
- **Enabled**: < 3ms total overhead per request
  - Steering evaluation: < 2ms
  - Memory recording: < 1ms (async)
  - Event emission: < 0.5ms (async)

### Error Handling

All intelligent systems follow fail-safe design:
- Failures never block request processing
- Errors are logged but don't propagate to clients
- Systems gracefully degrade when unavailable
- Server continues functioning if systems fail to initialize

---

## CLI Provider Capabilities

When using CLI tool providers (`geminicli`, `vibe`, `claudecli`, `codex`), `switchAILocal` exposes advanced capabilities through the `extra_body.cli` extension.

### Supported Capabilities by Provider

| Capability | geminicli | vibe | claudecli | codex |
|------------|-----------|------|-----------|-------|
| Attachments | ✅ `@path` | ✅ `@path` | ❌ | ❌ |
| Sandbox Mode | ✅ `-s` | ❌ | ❌ | ✅ `--sandbox` |
| Auto-Approve | ✅ `-y` | ✅ `--auto-approve` | ✅ `--dangerously-skip-permissions` | ✅ `--full-auto` |
| YOLO Mode | ✅ `--yolo` | ✅ `--auto-approve` | ✅ `--dangerously-skip-permissions` | ✅ `--full-auto` |
| Sessions | ✅ `--resume=` | ✅ `--continue` | ✅ `--resume` | ❌ |
| JSON Output | ✅ | ❌ | ✅ | ❌ |
| Streaming | ✅ | ❌ | ✅ | ❌ |

### Attachment Types

| Type | Description | Path Format |
|------|-------------|-------------|
| `file` | Single file | Absolute or relative path |
| `folder` | Directory (recursive) | Path ending with `/` optional |

### How It Works

1. You send a request with `extra_body.cli` options
2. `switchAILocal` extracts the options and maps them to CLI-specific flags
3. Attachments are prepended to the prompt using the native `@path` syntax
4. The CLI tool is invoked with the constructed command
5. Output is parsed and returned in OpenAI-compatible format

See [examples.md](examples.md) for practical usage examples.

