<div align="center">
  <img src="assets/switchai_logo_trans.png" alt="switchAILocal Logo" width="120"/>

  <h1>switchAILocal</h1>

  <p><em>One local endpoint. All your AI providers.</em></p>

  <p>
    <a href="https://switchailocal.traylinx.com/introduction"><strong>Documentation</strong></a> •
    <a href="#quick-start">Quick Start</a> •
    <a href="https://switchailocal.traylinx.com/installation">Installation</a> •
    <a href="https://switchailocal.traylinx.com/configuration/overview">Configuration</a> •
    <a href="https://switchailocal.traylinx.com/api/overview">API Reference</a>
  </p>
</div>

---

## What is switchAILocal?

switchAILocal is a gateway you run on your own machine. It exposes one
OpenAI-compatible API (plus Claude- and Gemini-compatible routes) and forwards
each request to the provider you choose: CLI tools you are already logged in to,
local model servers, or cloud API keys.

- **Use your existing subscriptions.** Installed and logged-in `gemini`, `claude`,
  `codex` and `vibe` CLIs are detected at startup and become providers.
- **Local models.** Ollama and LM Studio, plus an OpenCode server.
- **Cloud APIs.** Gemini, Anthropic, OpenAI, and any OpenAI-compatible service
  (Groq, OpenRouter, DeepSeek, ...) through `openai-compatibility`.
- **One client config.** Point any OpenAI SDK or tool at `http://localhost:18080/v1`.
- **Routing.** `provider:model` prefixes, model aliases, virtual models,
  round-robin across credentials, circuit breakers and failover.
- **Operations.** Web management panel, management API, Prometheus metrics,
  JSON logs and per-request NDJSON events.

## Supported providers

| Kind | Provider | Prefix | How it connects |
| ---- | -------- | ------ | --------------- |
| CLI | Google Gemini CLI | `geminicli:` | `gemini` on your `PATH` |
| CLI | Anthropic Claude Code | `claudecli:` | `claude` on your `PATH` |
| CLI | OpenAI Codex | `codex:` | `codex` on your `PATH` |
| CLI | Mistral Vibe | `vibecli:` | `vibe` on your `PATH` |
| Local | Ollama | `ollama:` | `ollama:` block in `config.yaml` |
| Local | LM Studio | `lmstudio:` | `lmstudio:` block in `config.yaml` |
| Local | OpenCode server | `opencode:` | `opencode:` block in `config.yaml` |
| Cloud | Google AI Studio | `gemini:` | `gemini-api-key` |
| Cloud | Anthropic API | `claude:` | `claude-api-key` |
| Cloud | OpenAI API | `codex:` | `codex-api-key` |
| Cloud | Traylinx switchAI | `switchai:` | `switchai-api-key` |
| Cloud | Any OpenAI-compatible API | your `prefix` | `openai-compatibility` |

Details: [Providers](https://switchailocal.traylinx.com/configuration/providers).

## Requirements

| Platform | Prebuilt binary | Docker image |
| -------- | --------------- | ------------ |
| macOS, Apple Silicon | yes (`darwin_arm64`) | yes |
| macOS, Intel | no, build from source | yes |
| Linux x86_64 / arm64 | yes (`linux_amd64`, `linux_arm64`) | yes |
| Windows | no | yes (or WSL) |

Building from source needs Go 1.26.10+ with CGO enabled (a C compiler).

## Quick start

### 1. Get the binary

**Release archive** (macOS Apple Silicon, Linux):

```bash
VERSION=0.6.2
OS=darwin ARCH=arm64          # or: OS=linux ARCH=amd64 / ARCH=arm64
curl -LO https://github.com/traylinx/switchAILocal/releases/download/v${VERSION}/switchAILocal_${VERSION}_${OS}_${ARCH}.tar.gz
tar xzf switchAILocal_${VERSION}_${OS}_${ARCH}.tar.gz
cd switchAILocal_${VERSION}_${OS}_${ARCH}
```

**Docker**: see [Docker deployment](https://switchailocal.traylinx.com/guides/docker-deployment).

**From source**:

```bash
git clone https://github.com/traylinx/switchAILocal.git
cd switchAILocal
./ail.sh setup      # builds binaries, creates config.yaml, installs the `ail` helper
```

> The `npx @traylinx/switchailocal` launcher cannot unpack the current release
> archives. Use the release archive or Docker until it is fixed. See
> [Installation](https://switchailocal.traylinx.com/installation#npx).

### 2. Create `config.yaml`

The server reads `config.yaml` from the directory you start it in and refuses
to start without one.

```bash
cat > config.yaml <<'EOF'
host: "127.0.0.1"
port: 18080
auth-dir: "~/.switchailocal"
api-keys:
  - "sk-your-key"        # clients send: Authorization: Bearer sk-your-key
EOF
chmod 600 config.yaml
```

`config.example.yaml` (in the archive and the repository) documents every
section but enables providers and keys you probably do not have; start small.

### 3. Start the server

```bash
./switchAILocal          # or: ail start   (after ./ail.sh setup)
```

Check it:

```bash
curl http://localhost:18080/health
# {"status":"ok"}
```

### 4. Make a request

If a supported CLI is installed and logged in, it already works:

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-your-key" \
  -d '{"model": "geminicli:gemini-2.5-pro",
       "messages": [{"role": "user", "content": "Hello!"}]}'
```

List what is available with `GET /v1/models`. Without a prefix, the gateway
picks any provider that serves the requested model name.

## Connect providers

**CLI tools.** Install and log in to the CLI once (`gemini`, `claude`, `codex`,
`vibe`). switchAILocal finds them on `PATH` at startup. No config needed.

**API keys.** Add a list entry per key:

```yaml
gemini-api-key:
  - api-key: "your-gemini-key"
claude-api-key:
  - api-key: "sk-ant-your-key"
codex-api-key:
  - api-key: "sk-your-openai-key"
    base-url: "https://api.openai.com/v1"   # required for codex-api-key
```

**Local models.**

```yaml
ollama:
  enabled: true
  base-url: "http://localhost:11434"
  auto-discover: true
```

**OAuth logins** (`--login`, `--claude-login`, `--codex-login`, ...) store
tokens under `auth-dir`. See [Login commands](https://switchailocal.traylinx.com/cli/login).

## Use it from an SDK

```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:18080/v1", api_key="sk-your-key")
reply = client.chat.completions.create(
    model="ollama:llama3.2",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(reply.choices[0].message.content)
```

Streaming, tools, vision, embeddings, images, audio and music endpoints are
covered in the [API reference](https://switchailocal.traylinx.com/api/overview).

## Management panel

The server serves a web panel at `http://localhost:18080/management` for
providers, model mappings and settings (bundled in the Docker image and source
checkouts; release-archive users download one file). With no
`remote-management.secret-key`, any local process can use the management API,
so set one on shared machines.
See [Management dashboard](https://switchailocal.traylinx.com/guides/management-dashboard).

## Documentation

| Topic | Link |
| ----- | ---- |
| Installation (archive, Docker, source, `ail.sh`) | [installation](https://switchailocal.traylinx.com/installation) |
| Server command, flags and environment variables | [cli/server](https://switchailocal.traylinx.com/cli/server) |
| Configuration reference | [configuration/reference](https://switchailocal.traylinx.com/configuration/reference) |
| Providers | [configuration/providers](https://switchailocal.traylinx.com/configuration/providers) |
| Security | [configuration/security](https://switchailocal.traylinx.com/configuration/security) |
| Docker | [guides/docker-deployment](https://switchailocal.traylinx.com/guides/docker-deployment) |
| Managed Linux servers (channel updates, rollback) | [guides/managed-server](https://switchailocal.traylinx.com/guides/managed-server) |
| Troubleshooting | [guides/troubleshooting](https://switchailocal.traylinx.com/guides/troubleshooting) |

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes
4. Push and open a pull request

## License

MIT. See [LICENSE](LICENSE).

---

<div align="center">
  <strong>Maintained by Sebastian Schkudlara</strong>
</div>
