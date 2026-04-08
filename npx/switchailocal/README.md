# switchAILocal

One local endpoint. All your AI providers.

## Quick Start

```bash
npx @traylinx/switchailocal
```

That's it. This downloads and runs the switchAILocal binary for your platform.

## What is switchAILocal?

A unified AI API gateway that runs locally, providing a single OpenAI-compatible endpoint (`http://localhost:18080`) to access all your AI providers seamlessly.

**Supported providers:** OpenAI, Claude, Gemini, Ollama, LM Studio, Groq, OpenRouter, and many more.

## Options

```bash
# Run a specific version
npx @traylinx/switchailocal --version=v1.0.0

# Pass flags to switchAILocal
npx @traylinx/switchailocal --port 8080
```

## Configuration

On first run, a default config is created at `~/.switchailocal/config.yaml`. Edit it to add your API keys.

## Links

- [GitHub](https://github.com/traylinx/switchAILocal)
- [Documentation](https://github.com/traylinx/switchAILocal#readme)
- [Docker](https://github.com/traylinx/switchAILocal/pkgs/container/switchailocal)
