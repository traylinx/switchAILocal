# Intelligent Systems Guide

This guide covers the four intelligent systems integrated into `switchAILocal`: Memory, Heartbeat, Steering, and Hooks.

## Overview

The intelligent systems enhance `switchAILocal` with learning, monitoring, and automation capabilities:

- **Memory System** - Records routing decisions and learns from outcomes
- **Heartbeat Monitor** - Monitors provider health and tracks quotas
- **Steering Engine** - Applies conditional routing rules
- **Hooks Manager** - Automates responses to system events

All systems are **disabled by default** and operate independently. They follow a fail-safe design where failures never block request processing.

---

## Memory System

### Purpose

The Memory System records every routing decision, tracks outcomes, and provides analytics about provider performance. Over time, it learns which providers work best for specific use cases.

### Configuration

```yaml
memory:
  enabled: true              # Enable memory recording
  retention-days: 30         # Keep records for 30 days
  storage-path: "./.memory"  # Storage directory
```

### What Gets Recorded

Each routing decision includes:
- Timestamp
- Requested model
- Selected provider
- Request parameters
- Outcome (success/failure)
- Latency
- Error details (if failed)

### Viewing Memory Data

**Via Management API:**
```bash
curl http://localhost:18080/v0/management/memory/stats \
  -H "X-Management-Key: your-secret-key"
```

**Via CLI:**
```bash
switchAILocal memory stats
```

### Analytics

The memory system computes analytics including:
- Provider success rates
- Average latency per provider
- Common failure patterns
- Model availability trends

Access analytics via:
```bash
curl http://localhost:18080/v0/management/analytics \
  -H "X-Management-Key: your-secret-key"
```

### Storage Management

- Records are stored in JSON format in the configured directory
- Automatic cleanup removes records older than `retention-days`
- Cleanup runs daily at midnight
- Storage usage is reported in stats endpoint

---

## Heartbeat Monitor

### Purpose

The Heartbeat Monitor runs background health checks on all configured providers, tracks quota usage, and emits events when issues are detected.

### Configuration

```yaml
heartbeat:
  enabled: true              # Enable background monitoring
  interval: 5m               # Check interval
  timeout: 30s               # Health check timeout
  retry-attempts: 3          # Retries before marking down
  quota-warning: 80          # Warning threshold (%)
  quota-critical: 95         # Critical threshold (%)
```

### Health Checks

The monitor performs these checks:
- **API Availability** - Can the provider API be reached?
- **Authentication** - Are credentials valid?
- **Quota Status** - How much quota is remaining?
- **Response Time** - Is the provider responding quickly?

### Provider Status

Providers can be in these states:
- `healthy` - All checks passing
- `degraded` - Some checks failing but provider usable
- `unhealthy` - Provider unavailable
- `unknown` - Not yet checked

### Viewing Status

**Via Management API:**
```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "X-Management-Key: your-secret-key"
```

**Response Example:**
```json
{
  "providers": [
    {
      "name": "openai",
      "status": "healthy",
      "last_check": "2026-02-03T10:30:00Z",
      "quota_used": 75,
      "quota_remaining": 25,
      "response_time_ms": 120
    },
    {
      "name": "anthropic",
      "status": "degraded",
      "last_check": "2026-02-03T10:30:00Z",
      "quota_used": 92,
      "quota_remaining": 8,
      "response_time_ms": 450,
      "warnings": ["High quota usage"]
    }
  ]
}
```

### Events

The monitor emits these events:
- `health_check_failed` - Provider health check failed
- `provider_status_change` - Provider status changed
- `quota_warning` - Quota usage exceeded warning threshold
- `quota_critical` - Quota usage exceeded critical threshold

---

## Steering Engine

### Purpose

The Steering Engine applies conditional routing rules to modify requests before they're routed to providers. Rules can change the model, provider, or request parameters based on conditions.

### Configuration

```yaml
steering:
  enabled: true              # Enable steering rules
  rules-path: "./steering"   # Directory containing rule files
  hot-reload: true           # Reload rules on file changes
```

### Rule Files

Rules are defined in YAML files in the `rules-path` directory.

**Example: Prefer Local Models** (`steering/prefer-local.yaml`)
```yaml
name: "prefer-local-models"
description: "Route llama models to local Ollama"
priority: 100              # Higher priority = evaluated first
enabled: true

conditions:
  - field: "model"
    operator: "contains"
    value: "llama"

actions:
  - type: "set-provider"
    value: "ollama"
```

**Example: Cost Optimization** (`steering/cost-optimize.yaml`)
```yaml
name: "cost-optimization"
description: "Use cheaper models for simple tasks"
priority: 50

conditions:
  - field: "messages[0].content"
    operator: "length-less-than"
    value: 100
  - field: "model"
    operator: "equals"
    value: "gpt-4"

actions:
  - type: "set-model"
    value: "gpt-3.5-turbo"
```

### Condition Operators

- `equals` - Exact match
- `contains` - Substring match
- `starts-with` - Prefix match
- `ends-with` - Suffix match
- `matches` - Regex match
- `greater-than` - Numeric comparison
- `less-than` - Numeric comparison
- `length-greater-than` - String length
- `length-less-than` - String length

### Action Types

- `set-provider` - Force specific provider
- `set-model` - Change model name
- `set-parameter` - Modify request parameter
- `add-parameter` - Add new parameter
- `remove-parameter` - Remove parameter
- `reject` - Reject request with error

### Rule Priority

Rules are evaluated in priority order (highest first). First matching rule wins. If no rules match, default routing logic applies.

### Hot-Reload

When `hot-reload: true`:
1. File watcher monitors the rules directory
2. Changes are detected automatically
3. Rules are validated and reloaded
4. In-flight requests continue with old rules
5. New requests use new rules

**Manual Reload:**
```bash
curl -X POST http://localhost:18080/v0/management/steering/reload \
  -H "X-Management-Key: your-secret-key"
```

### Viewing Rules

```bash
curl http://localhost:18080/v0/management/steering/rules \
  -H "X-Management-Key: your-secret-key"
```

---

## Hooks Manager

### Purpose

The Hooks Manager executes automated actions in response to system events. Use hooks to send alerts, trigger workflows, or integrate with external systems.

### Configuration

```yaml
hooks:
  enabled: true              # Enable hooks
  hooks-path: "./hooks"      # Directory containing hook files
  hot-reload: true           # Reload hooks on file changes
  max-concurrent: 10         # Max concurrent executions
```

### Hook Files

Hooks are defined in YAML files in the `hooks-path` directory.

**Example: Alert on Failure** (`hooks/alert-failure.yaml`)
```yaml
name: "alert-on-provider-failure"
description: "Send webhook when provider fails"
enabled: true

event: "health_check_failed"

action:
  type: "webhook"
  url: "https://alerts.example.com/webhook"
  method: "POST"
  headers:
    Content-Type: "application/json"
  body:
    provider: "{{.Provider}}"
    timestamp: "{{.Timestamp}}"
    message: "Provider {{.Provider}} health check failed"
```

**Example: Quota Alert** (`hooks/quota-warning.yaml`)
```yaml
name: "quota-warning-alert"
description: "Alert when quota reaches 80%"
enabled: true

event: "quota_warning"

action:
  type: "webhook"
  url: "https://alerts.example.com/quota"
  method: "POST"
  body:
    provider: "{{.Provider}}"
    quota_used: "{{.QuotaUsed}}"
    quota_remaining: "{{.QuotaRemaining}}"
```

**Example: Run Command** (`hooks/log-failures.yaml`)
```yaml
name: "log-failures"
description: "Log failures to file"
enabled: true

event: "request_failed"

action:
  type: "command"
  command: "echo"
  args:
    - "{{.Timestamp}} - {{.Provider}} - {{.Error}}"
  stdout: "/var/log/switchai-failures.log"
```

### Available Events

| Event | Description | Available Fields |
|-------|-------------|------------------|
| `health_check_failed` | Provider health check failed | Provider, Timestamp, Error |
| `quota_warning` | Quota threshold warning (80%) | Provider, QuotaUsed, QuotaRemaining |
| `quota_critical` | Quota threshold critical (95%) | Provider, QuotaUsed, QuotaRemaining |
| `routing_decision` | Request routed to provider | Provider, Model, Timestamp |
| `request_failed` | Request failed | Provider, Model, Error, Timestamp |
| `provider_status_change` | Provider status changed | Provider, OldStatus, NewStatus |

### Action Types

**Webhook**
```yaml
action:
  type: "webhook"
  url: "https://example.com/webhook"
  method: "POST"
  headers:
    Authorization: "Bearer token"
  body:
    field: "{{.Variable}}"
```

**Command**
```yaml
action:
  type: "command"
  command: "/path/to/script.sh"
  args:
    - "{{.Provider}}"
    - "{{.Timestamp}}"
  env:
    PROVIDER: "{{.Provider}}"
```

**Script**
```yaml
action:
  type: "script"
  interpreter: "bash"
  script: |
    #!/bin/bash
    echo "Provider $1 failed at $2"
    # Send alert, update dashboard, etc.
  args:
    - "{{.Provider}}"
    - "{{.Timestamp}}"
```

### Template Variables

Hook actions support Go template syntax for dynamic values:

- `{{.Provider}}` - Provider name
- `{{.Model}}` - Model name
- `{{.Timestamp}}` - Event timestamp
- `{{.Error}}` - Error message
- `{{.QuotaUsed}}` - Quota used percentage
- `{{.QuotaRemaining}}` - Quota remaining percentage
- `{{.Status}}` - Provider status
- `{{.OldStatus}}` - Previous status
- `{{.NewStatus}}` - New status

### Execution

Hooks execute **asynchronously** and never block request processing:
- Events are queued
- Hooks execute in background goroutines
- Failures are logged but don't affect requests
- Concurrent execution limited by `max-concurrent`

### Hot-Reload

When `hot-reload: true`:
1. File watcher monitors the hooks directory
2. Changes are detected automatically
3. Hooks are validated and reloaded
4. In-flight executions continue
5. New events use new hooks

**Manual Reload:**
```bash
curl -X POST http://localhost:18080/v0/management/hooks/reload \
  -H "X-Management-Key: your-secret-key"
```

### Viewing Hooks

```bash
curl http://localhost:18080/v0/management/hooks/status \
  -H "X-Management-Key: your-secret-key"
```

---

## Integration Examples

### Example 1: Cost-Aware Routing with Alerts

**Goal:** Route expensive models to cheaper alternatives and alert when quota is high.

**Steering Rule** (`steering/cost-aware.yaml`):
```yaml
name: "cost-aware-routing"
priority: 100
enabled: true

conditions:
  - field: "model"
    operator: "equals"
    value: "gpt-4"

actions:
  - type: "set-model"
    value: "gpt-3.5-turbo"
```

**Hook** (`hooks/quota-alert.yaml`):
```yaml
name: "quota-alert"
enabled: true
event: "quota_warning"

action:
  type: "webhook"
  url: "https://slack.com/api/chat.postMessage"
  method: "POST"
  headers:
    Authorization: "Bearer xoxb-your-token"
  body:
    channel: "#alerts"
    text: "⚠️ {{.Provider}} quota at {{.QuotaUsed}}%"
```

### Example 2: Automatic Failover with Logging

**Goal:** Prefer local Ollama, fall back to cloud, and log all failures.

**Steering Rule** (`steering/prefer-local.yaml`):
```yaml
name: "prefer-local"
priority: 100
enabled: true

conditions:
  - field: "model"
    operator: "contains"
    value: "llama"

actions:
  - type: "set-provider"
    value: "ollama"
```

**Hook** (`hooks/log-failures.yaml`):
```yaml
name: "log-failures"
enabled: true
event: "request_failed"

action:
  type: "command"
  command: "logger"
  args:
    - "-t"
    - "switchai"
    - "{{.Provider}} failed: {{.Error}}"
```

### Example 3: Development vs Production Routing

**Goal:** Use local models in development, cloud in production.

**Steering Rule** (`steering/env-routing.yaml`):
```yaml
name: "development-routing"
priority: 100
enabled: true

conditions:
  - field: "headers.X-Environment"
    operator: "equals"
    value: "development"

actions:
  - type: "set-provider"
    value: "ollama"
```

---

## Troubleshooting

### Memory System Not Recording

**Check:**
1. Is `memory.enabled: true` in config?
2. Does the storage path exist and have write permissions?
3. Check logs for memory system errors

**Debug:**
```bash
curl http://localhost:18080/v0/management/memory/stats \
  -H "X-Management-Key: your-secret-key"
```

### Heartbeat Not Monitoring

**Check:**
1. Is `heartbeat.enabled: true` in config?
2. Are providers configured correctly?
3. Check logs for heartbeat errors

**Debug:**
```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "X-Management-Key: your-secret-key"
```

### Steering Rules Not Applied

**Check:**
1. Is `steering.enabled: true` in config?
2. Do rule files have correct YAML syntax?
3. Are rule priorities set correctly?
4. Check logs for rule evaluation errors

**Debug:**
```bash
curl http://localhost:18080/v0/management/steering/rules \
  -H "X-Management-Key: your-secret-key"
```

### Hooks Not Executing

**Check:**
1. Is `hooks.enabled: true` in config?
2. Do hook files have correct YAML syntax?
3. Are events being emitted?
4. Check logs for hook execution errors

**Debug:**
```bash
curl http://localhost:18080/v0/management/hooks/status \
  -H "X-Management-Key: your-secret-key"
```

### Performance Issues

If intelligent systems cause performance problems:

1. **Disable systems temporarily:**
   ```yaml
   memory:
     enabled: false
   heartbeat:
     enabled: false
   steering:
     enabled: false
   hooks:
     enabled: false
   ```

2. **Reduce monitoring frequency:**
   ```yaml
   heartbeat:
     interval: 15m  # Increase from 5m
   ```

3. **Limit hook concurrency:**
   ```yaml
   hooks:
     max-concurrent: 5  # Reduce from 10
   ```

4. **Check logs for errors:**
   ```bash
   tail -f logs/switchai.log | grep -i "error\|warning"
   ```

---

## Best Practices

### Memory System

- Set `retention-days` based on your analysis needs (7-30 days typical)
- Monitor storage usage regularly
- Use analytics to identify provider issues early

### Heartbeat Monitor

- Set `interval` based on provider SLAs (5-15 minutes typical)
- Configure quota thresholds to match your usage patterns
- Use events to trigger automated responses

### Steering Engine

- Start with simple rules and add complexity gradually
- Use high priorities for critical routing decisions
- Test rules thoroughly before enabling in production
- Document rule purposes in descriptions

### Hooks Manager

- Keep hook actions fast and reliable
- Use webhooks for external integrations
- Log hook failures for debugging
- Limit concurrent executions to avoid resource exhaustion

### General

- Enable systems one at a time to isolate issues
- Monitor logs during initial deployment
- Use Management API to verify system status
- Test hot-reload functionality before relying on it

---

## Performance Impact

| System | Overhead (Enabled) | Overhead (Disabled) |
|--------|-------------------|---------------------|
| Memory | < 1ms (async) | 0ms |
| Heartbeat | 0ms (background) | 0ms |
| Steering | < 2ms | 0ms |
| Hooks | < 0.5ms (async) | 0ms |
| **Total** | **< 3ms** | **0ms** |

All systems follow fail-safe design:
- Failures never block requests
- Errors are logged but don't propagate
- Systems gracefully degrade when unavailable
- Server continues functioning if systems fail

---

## Migration Guide

### Upgrading from Previous Versions

If you're upgrading from a version without intelligent systems:

1. **No action required** - Systems are disabled by default
2. **Existing configuration works** - No breaking changes
3. **Enable systems gradually** - Test each system independently
4. **Monitor performance** - Watch for any impact on latency

### Enabling Systems

**Step 1: Enable Memory**
```yaml
memory:
  enabled: true
  retention-days: 7  # Start with short retention
```

**Step 2: Enable Heartbeat**
```yaml
heartbeat:
  enabled: true
  interval: 10m  # Start with longer interval
```

**Step 3: Enable Steering (if needed)**
```yaml
steering:
  enabled: true
  rules-path: "./steering"
  hot-reload: true
```

**Step 4: Enable Hooks (if needed)**
```yaml
hooks:
  enabled: true
  hooks-path: "./hooks"
  hot-reload: true
```

### Rollback

To disable all systems:
```yaml
memory:
  enabled: false
heartbeat:
  enabled: false
steering:
  enabled: false
hooks:
  enabled: false
```

Or remove the sections entirely (defaults to disabled).

---

## Further Reading

- [Configuration Guide](configuration.md) - Full configuration reference
- [API Reference](api-reference.md) - Management API endpoints
- [Advanced Features](advanced-features.md) - Integration patterns
- [Examples](examples.md) - Practical usage examples
