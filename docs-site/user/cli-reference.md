# CLI Reference

This document lists all command-line flags available when running `switchAILocal`.

## Server Mode

By default, `switchAILocal` starts in server mode and listens for OpenAI-compatible API requests:

```bash
./switchAILocal [options]
```

### General Options

| Flag | Description | Default |
|------|-------------|---------|
| `--config <path>` | Path to `config.yaml` file | `./config.yaml` |
| `--no-browser` | Don't auto-open browser for OAuth flows | `false` |

---

## Authentication & Connection Flags

> **Note:** These flags are **OPTIONAL** if you are using local CLI wrappers (e.g., `geminicli:`, `claudecli:`). switchAILocal automatically authenticates using your existing local CLI credentials in that mode.
>
> These flags are only required if you want `switchAILocal` to act as a direct **Cloud Proxy** (server-to-server).

### Google Gemini (Proxy Mode)

```bash
./switchAILocal --login [--project_id <id>] [--no-browser]
```

**Required for:** `gemini:` provider (Direct Google Cloud API connection).
**Not needed for:** `geminicli:` provider (uses local `gemini` command).

**Options**:
- `--project_id <id>`: Optional Google Cloud project ID. If omitted, you'll be prompted to select from available projects.
- `--no-browser`: Print the OAuth URL instead of auto-opening the browser.

### Anthropic Claude (Proxy Mode)

```bash
./switchAILocal --claude-login [--no-browser]
```

**Required for:** `claude:` provider (Direct Anthropic API connection).
**Not needed for:** `claudecli:` provider (uses local `claude` command).

### OpenAI Codex (Proxy Mode)

```bash
./switchAILocal --codex-login [--no-browser]
```

### Qwen (Proxy Mode)

```bash
./switchAILocal --qwen-login [--no-browser]
```

### Mistral Vibe (Local CLI)

```bash
./switchAILocal --vibe-login
```

*Optional.* Manually registers the local `vibe` CLI. switchAILocal usually discovers this automatically if `vibe` is in your PATH.

### Ollama (Local)

```bash
./switchAILocal --ollama-login
```

Connects to a local Ollama instance (default: `http://localhost:11434`) and discovers all available models. Useful if auto-discovery fails.

### iFlow

```bash
./switchAILocal --iflow-login [--no-browser]
```

Or use cookie-based authentication:

```bash
./switchAILocal --iflow-cookie
```

### Antigravity

```bash
./switchAILocal --antigravity-login [--no-browser]
```

---

## Credential Import

### Vertex AI Service Account

Import a Google Cloud service account JSON key for Vertex AI:

```bash
./switchAILocal --vertex-import /path/to/service-account.json
```

---

## Security Options

| Flag | Description |
|------|-------------|
| `--password <string>` | Set a local server password for management operations. **Note**: For better security, consider managing this via environment configuration where supported, rather than a command-line flag. |

---

## Environment Variables

For cloud deployments, `switchAILocal` supports several environment variables:

| Variable | Description |
|----------|-------------|
| `DEPLOY=cloud` | Enable cloud deploy mode (waits for configuration) |
| `PGSTORE_DSN` | PostgreSQL DSN for remote token storage |
| `GITSTORE_GIT_URL` | Git repository URL for remote token storage |
| `OBJECTSTORE_ENDPOINT` | S3-compatible endpoint for object storage |

---

## Complete Flag Reference

| Flag | Description |
|------|-------------|
| `--login` | Login to Google/Gemini via OAuth |
| `--claude-login` | Login to Anthropic Claude via OAuth |
| `--codex-login` | Login to OpenAI Codex via OAuth |
| `--qwen-login` | Login to Qwen via OAuth |
| `--vibe-login` | Discover and register local Vibe CLI |
| `--ollama-login` | Connect to local Ollama instance |
| `--iflow-login` | Login to iFlow via OAuth |
| `--iflow-cookie` | Login to iFlow using browser cookies |
| `--antigravity-login` | Login to Antigravity via OAuth |
| `--vertex-import <file>` | Import Vertex AI service account JSON |
| `--project_id <id>` | Google Cloud project ID (Gemini only) |
| `--config <path>` | Path to configuration file |
| `--no-browser` | Skip auto-opening browser for OAuth |
| `--password <string>` | Set server password (advanced/insecure) |

---

## Example: Full Setup

```bash
# 1. Login to cloud providers
./switchAILocal --login
./switchAILocal --claude-login

# 2. Connect CLI tools
./switchAILocal --vibe-login

# 3. Connect local models
./switchAILocal --ollama-login

# 4. Start the server
./switchAILocal
```

> **Note**: LM Studio is configured via `config.yaml` (see [Configuration](configuration.md)), not via a login flag.
