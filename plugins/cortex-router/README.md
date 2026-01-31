# Cortex Intelligent Router

**Cortex** is the intelligent routing engine for `switchAILocal`. It intercepts "auto" model requests and dynamically routes them to the best model based on intent, complexity, and privacy requirements.

## Features

- **Reflex Tier**: Instant regex-based routing for PII security and code detection.
- **Cognitive Tier**: LLM-based intent classification (Coding vs Reasoning vs Creative).
- **Caching**: Host-side caching for sub-millisecond response times on repeated queries.

## Documentation

- **[Technical Manual](docs/CORTEX_MANUAL.md)**: Detailed explanation of the routing logic, configuration matrix, and internal mechanisms.

## Configuration

Enable this plugin in your `switchAILocal` configuration:

```yaml
enabled-plugins:
  - cortex-router
```

Configure the routing matrix in `config.yaml`:

```yaml
intelligence:
  enabled: true
  router-model: "switchai-fast"
  matrix:
    coding: "switchai-chat"
    reasoning: "switchai-reasoner"
    # ... see manual for full matrix
```
