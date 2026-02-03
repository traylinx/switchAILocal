# Intelligent Routing

Detailed guide on configuring and using intelligent routing in switchAILocal.

## 🧠 Core Concept

Intelligent routing automatically selects the most appropriate model based on the task (coding, reasoning, fast, etc.).

---

## Option 1: Cortex Plugin (Local)

Requires the `cortex-router` plugin to be enabled in your `config.yaml`.

### Configuration in `config.yaml`

```yaml
plugin:
  enabled: true
  enabled-plugins:
    - "cortex-router"

intelligence:
  enabled: true
  router-model: "ollama:qwen:0.5b" # Small model for fast intent classification
  matrix:
    coding: "geminicli:"
    reasoning: "claudecli:"
    creative: "geminicli:"
    fast: "ollama:llama3.2"
    secure: "ollama:llama3.2"
    vision: "geminicli:gemini-2.5-pro"
```

### Usage

Use `model: "auto"` or `model: "cortex"` to trigger local intelligent routing.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "auto",
    "messages": [{"role": "user", "content": "Write a Python script"}]
  }'
```

---

## Option 2: Traylinx switchAI (Cloud)

Use Traylinx's Intelligent Routing Agent (IRA) in the cloud.

### Usage

Use `model: "switchai:auto"`. Requires a valid Traylinx API key in `config.yaml`.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TRAYLINX_KEY" \
  -d '{
    "model": "switchai:auto",
    "messages": [{"role": "user", "content": "Analyze these trends"}]
  }'
```

## Routing Logic

| Task Intent   | Target Model Group | Why                          |
| ------------- | ------------------ | ---------------------------- |
| **Coding**    | Coding Optimized   | High context, code knowledge |
| **Reasoning** | Large Reasoning    | Best for complex logic       |
| **Creative**  | Creative Models    | Vibe and flow                |
| **Fast**      | Small/Local Models | Sub-second latency           |
| **Vision**    | Multimodal Models  | Image understanding          |

## Customizing the Matrix

You can update the `intelligence.matrix` section in `config.yaml` at any time. Use `POST /v0/management/steering/reload` to apply changes without restarting.
