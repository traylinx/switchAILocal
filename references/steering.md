# Steering Rules Reference

Apply conditional routing rules to modify requests before they reach providers.

## Enable Steering

```yaml
# config.yaml
steering:
  enabled: true
  rules-path: "./steering"
  hot-reload: true
```

## Rule File Format

Create YAML files in the `steering/` directory:

```yaml
# steering/prefer-local.yaml
name: "prefer-local-models"
description: "Route llama models to local Ollama"
priority: 100  # Higher = evaluated first
enabled: true

conditions:
  - field: "model"
    operator: "contains"
    value: "llama"

actions:
  - type: "set-provider"
    value: "ollama"
```

## Condition Operators

| Operator              | Description        |
| --------------------- | ------------------ |
| `equals`              | Exact match        |
| `contains`            | Substring match    |
| `starts-with`         | Prefix match       |
| `ends-with`           | Suffix match       |
| `matches`             | Regex match        |
| `greater-than`        | Numeric comparison |
| `less-than`           | Numeric comparison |
| `length-greater-than` | String length      |
| `length-less-than`    | String length      |

## Action Types

| Action             | Description               |
| ------------------ | ------------------------- |
| `set-provider`     | Force specific provider   |
| `set-model`        | Change model name         |
| `set-parameter`    | Modify request parameter  |
| `add-parameter`    | Add new parameter         |
| `remove-parameter` | Remove parameter          |
| `reject`           | Reject request with error |

## Example: Cost Optimization

```yaml
# steering/cost-optimize.yaml
name: "cost-optimization"
description: "Use cheaper models for short messages"
priority: 50

conditions:
  - field: "messages[0].content"
    operator: "length-less-than"
    value: 100

actions:
  - type: "set-model"
    value: "gpt-3.5-turbo"
```

## View Active Rules

```bash
curl http://localhost:18080/v0/management/steering/rules \
  -H "X-Management-Key: your-key"
```

## Reload Rules

```bash
curl -X POST http://localhost:18080/v0/management/steering/reload \
  -H "X-Management-Key: your-key"
```
