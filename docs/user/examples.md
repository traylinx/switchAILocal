# Usage Examples & Samples

This guide provides practical examples of how to interact with `switchAILocal` using `curl`. Our API is fully OpenAI-compatible, so you can also use any OpenAI SDK by simply changing the `base_url`.

---

## 🚀 Quick Start

### List Available Models
See all models from all connected providers (CLI tools, Cloud APIs, and Local models).
```bash
curl http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123"
```

### Check Provider Status
Get a breakdown of which providers are currently active and healthy.
```bash
curl http://localhost:18080/v1/providers \
  -H "Authorization: Bearer sk-test-123"
```

---

## ☁️ Traylinx switchAI Samples

[switchAI](https://traylinx.com/switchai) is the recommended way to use high-quality cloud models with a single key.

### Fast Model (Gemini Flash)
Use a fast, efficient model for quick responses.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai-fast",
    "messages": [{"role": "user", "content": "Tell me a joke about robots."}]
  }'
```

### Specialized Reasoning (DeepSeek)
Use high-reasoning models for complex logic.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:deepseek-reasoner",
    "messages": [{"role": "user", "content": "Solve for x: 2x + 5 = 15"}],
    "extra_body": {
      "reasoning_effort": "high"
    }
  }'
```

---

## ⚡ Groq Cloud Samples

Groq provides ultra-fast inference for Llama, Mixtral, and Gemma models.

### Llama 3.3 70B (Ultra-Fast)
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "groq:llama-3.3-70b-versatile",
    "messages": [{"role": "user", "content": "Write a fast python script for web scraping."}]
  }'
```

---

---

## 🛠️ CLI Provider Samples

If you have official CLI tools installed locally, you can use them as providers.

### Google Gemini CLI
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Hello from Gemini!"}]
  }'
```

### Mistral Vibe CLI
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "vibe:mistral-large-latest",
    "messages": [{"role": "user", "content": "Write a short poem about Mistral."}]
  }'
```

### 📎 CLI Attachments (Files & Folders)
Pass local context directly to `geminicli` or `vibe` using the `attachments` array. These are prepended to your prompt using the native CLI syntax (e.g., `@path`).
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Explain the logic in this file."}],
    "extra_body": {
      "cli": {
        "attachments": [
          {"type": "file", "path": "/absolute/path/to/script.py"},
          {"type": "folder", "path": "./internal/api"}
        ]
      }
    }
  }'
```

### 🚩 CLI Flags (Sandbox/YOLO/Auto-Approve)
Standardize CLI control flags across different providers. `switchAILocal` maps these to provider-specific arguments (e.g., `-y` for Gemini, `--dangerously-skip-permissions` for Claude).
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "vibe:",
    "messages": [{"role": "user", "content": "Update the version in package.json"}],
    "extra_body": {
      "cli": {
        "flags": {
          "auto_approve": true,
          "sandbox": true
        }
      }
    }
  }'
```

### 💾 Session Management
Resume or name specific CLI sessions.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Continue our previous discussion."}],
    "extra_body": {
      "cli": {
        "session_id": "latest"
      }
    }
  }'
```

### 🤝 Combined Attachments, Flags & Sessions
You can combine all options in a single request for complex automation tasks.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "vibe:",
    "messages": [{"role": "user", "content": "Analyze the security of this module and suggest fixes."}],
    "extra_body": {
      "cli": {
        "attachments": [
          {"type": "folder", "path": "./internal/auth"},
          {"type": "file", "path": "./main.go"}
        ],
        "flags": {
          "auto_approve": true,
          "sandbox": true
        },
        "session_id": "security-audit"
      }
    }
  }'
```

### 🔓 Claude CLI Dangerous Mode
Use the `auto_approve` flag to bypass safety confirmations in `claudecli` (maps to `--dangerously-skip-permissions`).
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "claudecli:",
    "messages": [{"role": "user", "content": "Fix the linter errors in this directory."}],
    "extra_body": {
      "cli": {
        "flags": {
          "auto_approve": true
        }
      }
    }
  }'
```

### 🧠 OpenAI Codex CLI (Sandbox Mode)
Run commands in a restricted sandbox for security when using Codex.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "codex:",
    "messages": [{"role": "user", "content": "Write a script to list s3 buckets."}],
    "extra_body": {
      "cli": {
        "flags": {
          "sandbox": true
        }
      }
    }
  }'
```

### 📁 Folder Attachments for Vibe
Attach entire directories for deep analysis.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "vibe:",
    "messages": [{"role": "user", "content": "Analyze the project structure and suggest improvements."}],
    "extra_body": {
      "cli": {
        "attachments": [{"type": "folder", "path": "."}]
      }
    }
  }'
```

### 📂 Multi-File Context for Gemini
Attach multiple files or even entire folders to provide rich context to Gemini.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [
      {"role": "user", "content": "Review the interaction between these three files and suggest optimizations for the registry logic."}
    ],
    "extra_body": {
      "cli": {
        "attachments": [
          {"type": "file", "path": "./internal/cli/discovery.go"},
          {"type": "file", "path": "./internal/runtime/executor/cli_executor.go"},
          {"type": "file", "path": "./sdk/switchailocal/service_provider_registry.go"}
        ]
      }
    }
  }'
```

### 🔍 Repository-Wide Analysis (Vibe)
Provide a whole project directory as context to Vibe for structural analysis or refactoring suggestions.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "vibe:",
    "messages": [
      {"role": "user", "content": "Identify any potential race conditions in the authentication flow across the whole project."}
    ],
    "extra_body": {
      "cli": {
        "attachments": [
          {"type": "folder", "path": "."}
        ],
        "flags": {
          "auto_approve": true
        }
      }
    }
  }'
```

### 🧊 Local LLMs (Ollama / LM Studio)
Run models locally and access them via the same API.

```bash
# Ollama
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ollama:qwen2.5-coder:32b",
    "messages": [{"role": "user", "content": "Write a Go function to reverse a string."}]
  }'

# LM Studio
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "lmstudio:luna-ai-llama2",
    "messages": [{"role": "user", "content": "Hello Luna!"}]
  }'
```

---

## 🌊 Advanced Features

### Real-time Streaming
Add `"stream": true` to see tokens as they are generated. `switchAILocal` ensures a clean SSE stream with standard `data: ` prefixes.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai-reasoner",
    "messages": [{"role": "user", "content": "Explain quantum entanglement in 3 sentences."}],
    "stream": true
  }'
```

**Expected SSE Output Format:**
```sse
data: {"id":"...","object":"chat.completion.chunk",...}

data: {"id":"...","object":"chat.completion.chunk",...}

data: [DONE]
```

### Auto-Routing (Prefix-less)
If you don't specify a provider prefix, `switchAILocal` will automatically pick the best available provider for that model name.
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai-fast",
    "messages": [{"role": "user", "content": "Who are you?"}]
  }'
```

## Multimodal Examples (Images)

`switchAILocal` supports multimodal inputs (text + images) for compatible providers like `geminicli`, `claude`, and `ollama` (if the model supports it).

### Sending an Image via cURL (OpenAI Compatible)

You can send base64-encoded images using the standard OpenAI `image_url` format.

```bash
# 1. Encode image to base64 (macOS/Linux)
IMAGE_DATA="data:image/jpeg;base64,$(base64 -i path/to/image.jpg)"

# 2. Send request
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is in this image?"},
          {
            "type": "image_url",
            "image_url": {
              "url": "'"$IMAGE_DATA"'"
            }
          }
        ]
      }
    ]
  }'
```

### Sending an Image via Python (OpenAI SDK)

```python
import base64
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:18080/v1",
    api_key="unused"
)

# Function to encode key
def encode_image(image_path):
    with open(image_path, "rb") as image_file:
        return base64.b64encode(image_file.read()).decode('utf-8')

base64_image = encode_image("path/to/image.jpg")

response = client.chat.completions.create(
    model="ollama:devstral-small-2:24b-cloud", # Or "geminicli:gemini-2.5-pro", "claude:claude-3-opus"
    messages=[
        {
            "role": "user",
            "content": [
                {"type": "text", "text": "Describe this image in detail."},
                {
                    "type": "image_url",
                    "image_url": {
                        "url": f"data:image/jpeg;base64,{base64_image}"
                    },
                },
            ],
        }
    ],
)

print(response.choices[0].message.content)
```

> **Note:** For `ollama`, ensure the underlying model (e.g., `llava`, `moondream`, `devstral-small-2`) supports vision. `switchAILocal` automatically converts the OpenAI format to the provider's native image format.

