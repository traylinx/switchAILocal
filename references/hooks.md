# Hooks Reference

Execute automated actions in response to system events.

## Enable Hooks

```yaml
# config.yaml
hooks:
  enabled: true
  hooks-path: "./hooks"
  hot-reload: true
  max-concurrent: 10
```

## Hook File Format

Create YAML files in the `hooks/` directory:

```yaml
# hooks/alert-failure.yaml
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
    message: "Provider {{.Provider}} failed"
```

## Available Events

| Event                    | Description                  | Fields                              |
| ------------------------ | ---------------------------- | ----------------------------------- |
| `health_check_failed`    | Provider health check failed | Provider, Timestamp, Error          |
| `quota_warning`          | Quota at 80%                 | Provider, QuotaUsed, QuotaRemaining |
| `quota_critical`         | Quota at 95%                 | Provider, QuotaUsed, QuotaRemaining |
| `routing_decision`       | Request routed               | Provider, Model, Timestamp          |
| `request_failed`         | Request failed               | Provider, Model, Error, Timestamp   |
| `provider_status_change` | Status changed               | Provider, OldStatus, NewStatus      |

## Action Types

### Webhook

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

### Command

```yaml
action:
  type: "command"
  command: "/path/to/script.sh"
  args:
    - "{{.Provider}}"
    - "{{.Timestamp}}"
```

### Script

```yaml
action:
  type: "script"
  interpreter: "bash"
  script: |
    #!/bin/bash
    echo "Provider $1 failed at $2"
  args:
    - "{{.Provider}}"
    - "{{.Timestamp}}"
```

## Template Variables

- `{{.Provider}}` - Provider name
- `{{.Model}}` - Model name
- `{{.Timestamp}}` - Event timestamp
- `{{.Error}}` - Error message
- `{{.QuotaUsed}}` - Quota used %
- `{{.QuotaRemaining}}` - Quota remaining %
- `{{.Status}}` - Provider status

## View Hooks Status

```bash
curl http://localhost:18080/v0/management/hooks/status \
  -H "X-Management-Key: your-key"
```

## Reload Hooks

```bash
curl -X POST http://localhost:18080/v0/management/hooks/reload \
  -H "X-Management-Key: your-key"
```
