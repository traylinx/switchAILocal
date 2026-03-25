# Usage Examples & Samples

This guide provides practical examples of how to interact with `switchAILocal` using `curl`. Our API is fully OpenAI-compatible, so you can also use any OpenAI SDK by simply changing the `base_url`.

---

## 📋 Prerequisites and Setup

Before running any of the examples below, ensure you have completed the following setup steps:

### 1. Start the Server

Start the `switchAILocal` server using the provided script:

```bash
./ail.sh start
```

### 2. Verify Server is Running

Confirm the server is running and accessible on the default port (18080):

```bash
curl http://localhost:18080/v1/models
```

If the server is running correctly, you should see a JSON response listing available models.

### 3. Required Dependencies

- **curl**: Command-line tool for making HTTP requests (pre-installed on most systems)
- **base64**: For encoding images in multimodal examples (pre-installed on macOS/Linux)
- **Provider CLIs** (optional): Install any CLI tools you want to use as providers:
  - `geminicli` for Google Gemini
  - `vibe` for Mistral
  - `claudecli` for Claude
  - `codex` for OpenAI Codex
- **Local LLM servers** (optional): Ollama or LM Studio for running local models

### 4. Create Test Directory

Many examples reference test files. Create a dedicated test directory:

```bash
mkdir -p /tmp/switchai-test
```

**Note:** Before running examples that reference files (e.g., CLI attachments or multimodal examples), create the necessary test files in `/tmp/switchai-test/`. For example:
- `/tmp/switchai-test/test-script.py` - A sample Python script
- `/tmp/switchai-test/sample-image.jpg` - A sample image for multimodal testing
- `/tmp/switchai-test/test-document.pdf` - A sample document

### 5. Configuration

Provider configuration is managed in the `switchAILocal` configuration file. Refer to the main documentation for details on:
- Adding API keys for cloud providers (switchAI, Groq, OpenAI, etc.)
- Configuring local provider endpoints (Ollama, LM Studio)
- Setting up CLI provider paths

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
    "model": "switchai:switchai-fast",
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
          {"type": "file", "path": "/tmp/switchai-test/test-script.py"},
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
    "model": "switchai:switchai-reasoner",
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
    "model": "switchai:switchai-fast",
    "messages": [{"role": "user", "content": "Who are you?"}]
  }'
```

### Error Responses

When requests fail, `switchAILocal` returns structured error responses with appropriate HTTP status codes. Here are common error scenarios:

#### Invalid Model Name

**Request:**
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "invalid-provider:nonexistent-model",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Response (HTTP 404):**
```json
{
  "error": {
    "message": "Model 'invalid-provider:nonexistent-model' not found",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

#### Missing Required Parameters

**Request:**
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:switchai-fast"
  }'
```

**Response (HTTP 400):**
```json
{
  "error": {
    "message": "Missing required parameter: 'messages'",
    "type": "invalid_request_error",
    "code": "missing_parameter"
  }
}
```

#### Provider Not Available

**Request:**
```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "groq:llama-3.3-70b-versatile",
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

**Response (HTTP 503):**
```json
{
  "error": {
    "message": "Provider 'groq' is currently unavailable or not configured",
    "type": "service_unavailable_error",
    "code": "provider_unavailable"
  }
}
```

#### Server Connection Error

If the server is not running, you'll receive a connection error from curl:

```bash
curl: (7) Failed to connect to localhost port 18080: Connection refused
```

**Solution:** Start the server using `./ail.sh start` and verify it's running with `curl http://localhost:18080/v1/models`.

## Multimodal Examples (Images)

`switchAILocal` supports multimodal inputs (text + images) for compatible providers like `geminicli`, `claude`, and `ollama` (if the model supports it).

### Sending an Image via cURL (OpenAI Compatible)

You can send base64-encoded images using the standard OpenAI `image_url` format.

```bash
# 1. Encode image to base64 (macOS/Linux)
IMAGE_DATA="data:image/jpeg;base64,$(base64 -i /tmp/switchai-test/sample-image.jpg)"

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

base64_image = encode_image("/tmp/switchai-test/sample-image.jpg")

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

### Multimodal Error Responses

When multimodal requests fail, `switchAILocal` returns structured error responses with appropriate HTTP status codes. Here are common error scenarios:

#### File Not Found

**Request:**
```bash
# Attempting to encode a non-existent image file
IMAGE_DATA="data:image/jpeg;base64,$(base64 -i /tmp/switchai-test/nonexistent-image.jpg 2>/dev/null)"

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

**Response (HTTP 400):**
```json
{
  "error": {
    "message": "Invalid image data: unable to decode base64 image",
    "type": "invalid_request_error",
    "code": "invalid_image"
  }
}
```

**Solution:** Verify the image file exists at the specified path before encoding. Use `ls -la /tmp/switchai-test/sample-image.jpg` to confirm.

#### Unsupported Image Format

**Request:**
```bash
# Attempting to send an unsupported image format
IMAGE_DATA="data:image/bmp;base64,$(base64 -i /tmp/switchai-test/sample-image.bmp)"

curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image."},
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

**Response (HTTP 400):**
```json
{
  "error": {
    "message": "Unsupported image format: bmp. Supported formats: jpeg, jpg, png, gif, webp",
    "type": "invalid_request_error",
    "code": "unsupported_image_format"
  }
}
```

**Solution:** Convert your image to a supported format (JPEG, PNG, GIF, or WebP) before encoding and sending.

#### Model Does Not Support Vision

**Request:**
```bash
IMAGE_DATA="data:image/jpeg;base64,$(base64 -i /tmp/switchai-test/sample-image.jpg)"

curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -d '{
    "model": "ollama:llama3.2:3b",
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

**Response (HTTP 400):**
```json
{
  "error": {
    "message": "Model 'ollama:llama3.2:3b' does not support multimodal inputs",
    "type": "invalid_request_error",
    "code": "model_not_multimodal"
  }
}
```

**Solution:** Use a vision-capable model such as `ollama:llava`, `ollama:moondream`, `geminicli:gemini-2.5-pro`, or `claude:claude-3-opus`.

#### Image Too Large

**Request:**
```bash
# Attempting to send an image that exceeds size limits
IMAGE_DATA="data:image/jpeg;base64,$(base64 -i /tmp/switchai-test/large-image.jpg)"

curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer any-key" \
  -d '{
    "model": "geminicli:gemini-2.5-pro",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Analyze this image."},
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

**Response (HTTP 413):**
```json
{
  "error": {
    "message": "Image size exceeds maximum allowed size of 20MB",
    "type": "invalid_request_error",
    "code": "image_too_large"
  }
}
```

**Solution:** Resize or compress your image to reduce file size below the provider's limit. Most providers support images up to 20MB.

---

## ⚙️ Provider Setup

Before you can use different AI providers with `switchAILocal`, you need to configure them properly. This section explains how to set up and verify various providers.

### Configuration File Location

Provider configuration is managed in the `config.yaml` file located in the root directory of your `switchAILocal` installation. This file contains all provider settings, API keys, endpoints, and authentication details.

**Default Location:** `./config.yaml` (in the same directory as `ail.sh`)

### Common Providers

#### Ollama (Local LLM Server)

**Setup:**
1. Install Ollama from [ollama.ai](https://ollama.ai)
2. Start the Ollama service (usually runs on `http://localhost:11434`)
3. Pull a model: `ollama pull llama3.2`
4. Configure in `config.yaml`:

```yaml
providers:
  ollama:
    enabled: true
    endpoint: "http://localhost:11434"
    timeout: 120
```

**Verification:**
```bash
# Check if Ollama is running
curl http://localhost:11434/api/tags

# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ollama:llama3.2",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Common Models:**
- `ollama:llama3.2` - Fast general-purpose model
- `ollama:qwen2.5-coder:32b` - Code-specialized model
- `ollama:llava` - Vision-capable model
- `ollama:moondream` - Lightweight vision model

#### LM Studio (Local LLM Server)

**Setup:**
1. Download and install [LM Studio](https://lmstudio.ai)
2. Load a model in LM Studio
3. Start the local server (usually runs on `http://localhost:1234`)
4. Configure in `config.yaml`:

```yaml
providers:
  lmstudio:
    enabled: true
    endpoint: "http://localhost:1234/v1"
    timeout: 120
```

**Verification:**
```bash
# Check if LM Studio server is running
curl http://localhost:1234/v1/models

# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "lmstudio:your-loaded-model",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Note:** Replace `your-loaded-model` with the actual model name shown in LM Studio.

#### OpenAI (Cloud API)

**Setup:**
1. Get an API key from [platform.openai.com](https://platform.openai.com)
2. Configure in `config.yaml`:

```yaml
providers:
  openai:
    enabled: true
    api_key: "sk-your-openai-api-key-here"
    timeout: 60
```

**Verification:**
```bash
# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "openai:gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Common Models:**
- `openai:gpt-4` - Most capable model
- `openai:gpt-4-turbo` - Faster, more cost-effective
- `openai:gpt-3.5-turbo` - Fast and economical

#### Groq (Cloud API)

**Setup:**
1. Get an API key from [console.groq.com](https://console.groq.com)
2. Configure in `config.yaml`:

```yaml
providers:
  groq:
    enabled: true
    api_key: "gsk_your-groq-api-key-here"
    timeout: 30
```

**Verification:**
```bash
# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "groq:llama-3.3-70b-versatile",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

#### switchAI (Cloud API - Recommended)

**Setup:**
1. Get an API key from [traylinx.com/switchai](https://traylinx.com/switchai)
2. Configure in `config.yaml`:

```yaml
providers:
  switchai:
    enabled: true
    api_key: "your-switchai-api-key-here"
    timeout: 60
```

**Verification:**
```bash
# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai:switchai-fast",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Benefits:** Single API key for multiple high-quality models (Gemini, DeepSeek, Claude, etc.)

#### CLI Providers (geminicli, vibe, claudecli, codex)

**Setup:**
1. Install the CLI tool globally:
   - `npm install -g @google/generative-ai-cli` (for geminicli)
   - `npm install -g @mistralai/mistral-cli` (for vibe)
   - `npm install -g @anthropic-ai/claude-cli` (for claudecli)
   - `npm install -g openai-codex-cli` (for codex)
2. Authenticate the CLI tool (follow the tool's authentication instructions)
3. Configure in `config.yaml`:

```yaml
providers:
  geminicli:
    enabled: true
    cli_path: "/usr/local/bin/geminicli"  # Or wherever npm installed it
  
  vibe:
    enabled: true
    cli_path: "/usr/local/bin/vibe"
  
  claudecli:
    enabled: true
    cli_path: "/usr/local/bin/claude"
  
  codex:
    enabled: true
    cli_path: "/usr/local/bin/codex"
```

**Verification:**
```bash
# Test CLI provider directly
geminicli "Hello!"

# Test via switchAILocal
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Verifying Provider Configuration

After configuring providers, use these commands to verify everything is working correctly:

#### 1. Check Provider Status

```bash
curl http://localhost:18080/v1/providers \
  -H "Authorization: Bearer sk-test-123"
```

This returns a list of all configured providers with their current status (active/inactive).

#### 2. List Available Models

```bash
curl http://localhost:18080/v1/models \
  -H "Authorization: Bearer sk-test-123"
```

This returns all models from all active providers. If a provider is configured correctly, its models will appear in this list.

#### 3. Test a Simple Request

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "provider:model-name",
    "messages": [{"role": "user", "content": "Test message"}]
  }'
```

Replace `provider:model-name` with your specific provider and model (e.g., `ollama:llama3.2`, `openai:gpt-4`).

#### 4. Check Heartbeat Status (Management API)

```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "Authorization: Bearer your-secret-key"
```

This provides detailed health information for all providers, including quota status and response times.

### Configuration Tips

- **API Keys:** Never commit API keys to version control. Use environment variables or keep `config.yaml` in `.gitignore`.
- **Timeouts:** Adjust timeout values based on your network and model response times. Local models may need longer timeouts.
- **Multiple Providers:** You can enable multiple providers simultaneously. `switchAILocal` will route requests based on the model prefix.
- **Provider Priority:** If you don't specify a provider prefix, `switchAILocal` will automatically select the best available provider for that model.
- **Reload Configuration:** After changing `config.yaml`, restart the server with `./ail.sh restart` to apply changes.

### Troubleshooting Provider Setup

If a provider isn't working:

1. **Verify the provider is enabled** in `config.yaml` (`enabled: true`)
2. **Check API keys** are correct and have sufficient quota
3. **Verify endpoints** for local providers (Ollama, LM Studio) are accessible
4. **Test the provider directly** (outside of switchAILocal) to isolate issues
5. **Check server logs** for detailed error messages: `./ail.sh logs`
6. **Use the heartbeat endpoint** to see provider health status

---

## 🧠 Intelligent Systems Samples

`switchAILocal` includes four intelligent systems: Memory, Heartbeat, Steering, and Hooks.

### 🔐 Management API Authentication

The Management API (`/v0/management/*` endpoints) requires authentication to protect administrative operations.

#### Authentication Methods

You can authenticate using either of these header formats:

1. **Authorization Bearer Token** (recommended):
   ```bash
   -H "Authorization: Bearer your-secret-key"
   ```

2. **X-Management-Key Header**:
   ```bash
   -H "X-Management-Key: your-secret-key"
   ```

Both methods are equivalent and accept the same management key.

#### Configuration

The management key is configured in your `config.yaml` file under the `remote-management` section:

```yaml
remote-management:
  allow-remote: true  # Enable remote (non-localhost) access
  secret-key: "your-secret-key-here"  # Your management key (will be hashed on startup)
```

**Configuration Location:** The `config.yaml` file is typically located in the root directory of your `switchAILocal` installation.

**Alternative Configuration:** You can also set the management key using the `MANAGEMENT_PASSWORD` environment variable:

```bash
export MANAGEMENT_PASSWORD="your-secret-key"
./ail.sh start
```

#### Localhost Bypass

**Special Case:** If you are accessing the Management API from localhost (127.0.0.1) AND remote management is disabled (`allow-remote: false`) AND no secret key is configured, authentication is **not required**. This provides convenient local access during development.

To enable this localhost-only mode, set:

```yaml
remote-management:
  allow-remote: false
  secret-key: ""  # Empty or omit this line
```

**Security Note:** For production deployments or when allowing remote access (`allow-remote: true`), always configure a strong management key.

### 📜 Memory & Analytics

View performance stats for all providers and models based on historical data.

```bash
# Get overall memory and system statistics
curl http://localhost:18080/v0/management/memory/stats \
  -H "Authorization: Bearer your-secret-key"

# View performance analytics (success rates, latency)
curl http://localhost:18080/v0/management/analytics \
  -H "Authorization: Bearer your-secret-key"
```

### 💓 Heartbeat Monitoring

Check the real-time health and quota status of all configured providers.

```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "Authorization: Bearer your-secret-key"
```

### 🎛️ Steering Engine (Dynamic Routing)

Reload steering rules without restarting the server or view current active rules.

```bash
# List all active steering rules
curl http://localhost:18080/v0/management/steering/rules \
  -H "Authorization: Bearer your-secret-key"

# Force a hot-reload of steering rules from disk
curl -X POST http://localhost:18080/v0/management/steering/reload \
  -H "Authorization: Bearer your-secret-key"
```

### ⚓ Hooks Manager
Triggering actions based on system events. You can view registered hooks.

```bash
# List all registered hooks
curl http://localhost:18080/v0/management/hooks \
  -H "Authorization: Bearer your-secret-key"

# Manually trigger a test event (e.g. for testing webhooks)
curl -X POST http://localhost:18080/v0/management/hooks/test \
  -H "Authorization: Bearer your-secret-key" \
  -d '{"event": "health_check_failed", "provider": "openai"}'
```

### Management API Error Responses

When Management API requests fail, `switchAILocal` returns structured error responses with appropriate HTTP status codes. Here are common error scenarios:

#### Missing Authentication

**Request:**
```bash
curl http://localhost:18080/v0/management/memory/stats
```

**Response (HTTP 401):**
```json
{
  "error": {
    "message": "Missing or invalid management key",
    "type": "authentication_error",
    "code": "unauthorized"
  }
}
```

**Solution:** Include the `Authorization: Bearer <key>` header or `X-Management-Key: <key>` header with your configured management key.

#### Invalid Management Key

**Request:**
```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "Authorization: Bearer wrong-key"
```

**Response (HTTP 403):**
```json
{
  "error": {
    "message": "Invalid management key",
    "type": "authentication_error",
    "code": "forbidden"
  }
}
```

**Solution:** Verify your management key in the `config.yaml` file under `remote-management.secret-key` and ensure it matches the key used in the request.

#### Provider Not Found

**Request:**
```bash
curl http://localhost:18080/v0/management/providers/nonexistent-provider \
  -H "Authorization: Bearer your-secret-key"
```

**Response (HTTP 404):**
```json
{
  "error": {
    "message": "Provider 'nonexistent-provider' not found",
    "type": "invalid_request_error",
    "code": "provider_not_found"
  }
}
```

**Solution:** Use the `/v0/management/heartbeat/status` endpoint to list all available providers and verify the provider name.

#### Invalid Steering Rule

**Request:**
```bash
curl -X POST http://localhost:18080/v0/management/steering/reload \
  -H "Authorization: Bearer your-secret-key"
```

**Response (HTTP 400):**
```json
{
  "error": {
    "message": "Failed to reload steering rules: invalid rule syntax in line 15",
    "type": "invalid_request_error",
    "code": "invalid_steering_rule"
  }
}
```

**Solution:** Check your steering rules configuration file for syntax errors and ensure all rules follow the correct format.

---
## 🔧 Troubleshooting

This section covers common issues you may encounter when using `switchAILocal` and provides step-by-step solutions to resolve them.

### Common Issues

#### Connection Refused Error

**Symptom**: When running curl commands, you receive an error message:
```
curl: (7) Failed to connect to localhost port 18080: Connection refused
```

**Cause**: The `switchAILocal` server is not running or is running on a different port.

**Solution**:
1. Start the server using the startup script:
   ```bash
   ./ail.sh start
   ```

2. Verify the server is running and check which port it's using:
   ```bash
   curl http://localhost:18080/v1/models
   ```

3. If the server is running on a different port, check your `config.yaml` file for the configured port number.

4. Verify the process is running:
   ```bash
   ps aux | grep switchAILocal
   ```

#### Provider Not Found Error

**Symptom**: API requests return an error:
```json
{
  "error": {
    "message": "Provider 'provider-name' not found",
    "type": "invalid_request_error",
    "code": "provider_not_found"
  }
}
```

**Cause**: The specified provider is not configured, not enabled, or the provider name is misspelled.

**Solution**:
1. Check the list of available providers:
   ```bash
   curl http://localhost:18080/v1/providers \
     -H "Authorization: Bearer sk-test-123"
   ```

2. Verify the provider is enabled in your `config.yaml` file:
   ```yaml
   providers:
     provider-name:
       enabled: true
   ```

3. Check for typos in the provider name (e.g., `ollama` not `olama`).

4. If the provider requires an API key, ensure it's correctly configured in `config.yaml`.

5. Restart the server after making configuration changes:
   ```bash
   ./ail.sh restart
   ```

#### Model Not Available Error

**Symptom**: API requests return an error:
```json
{
  "error": {
    "message": "Model 'model-name' not found",
    "type": "invalid_request_error",
    "code": "model_not_found"
  }
}
```

**Cause**: The specified model is not available from any configured provider, or the model name format is incorrect.

**Solution**:
1. List all available models from all providers:
   ```bash
   curl http://localhost:18080/v1/models \
     -H "Authorization: Bearer sk-test-123"
   ```

2. Verify you're using the correct model name format: `provider:model-name` (e.g., `ollama:llama3.2`, `openai:gpt-4`).

3. For Ollama models, ensure the model is pulled locally:
   ```bash
   ollama list
   ollama pull model-name
   ```

4. For cloud providers (OpenAI, Groq, switchAI), verify your API key has access to the requested model.

5. Check the provider's heartbeat status to ensure it's healthy:
   ```bash
   curl http://localhost:18080/v0/management/heartbeat/status \
     -H "Authorization: Bearer your-secret-key"
   ```

#### Authentication Errors (Management API)

**Symptom**: Management API requests return:
```json
{
  "error": {
    "message": "Missing or invalid management key",
    "type": "authentication_error",
    "code": "unauthorized"
  }
}
```

**Cause**: The management key is missing, incorrect, or not configured.

**Solution**:
1. Check your `config.yaml` file for the management key:
   ```yaml
   remote-management:
     secret-key: "your-secret-key-here"
   ```

2. Include the authentication header in your requests:
   ```bash
   curl http://localhost:18080/v0/management/memory/stats \
     -H "Authorization: Bearer your-secret-key"
   ```

3. If accessing from localhost with `allow-remote: false`, authentication may not be required. Verify your configuration:
   ```yaml
   remote-management:
     allow-remote: false
   ```

4. Restart the server after changing the management key:
   ```bash
   ./ail.sh restart
   ```

#### Image/Multimodal Errors

**Symptom**: Multimodal requests fail with errors like:
```json
{
  "error": {
    "message": "Invalid image data: unable to decode base64 image",
    "type": "invalid_request_error",
    "code": "invalid_image"
  }
}
```

**Cause**: The image file doesn't exist, the base64 encoding failed, or the image format is unsupported.

**Solution**:
1. Verify the image file exists:
   ```bash
   ls -la /tmp/switchai-test/sample-image.jpg
   ```

2. Test the base64 encoding separately:
   ```bash
   base64 -i /tmp/switchai-test/sample-image.jpg | head -c 100
   ```

3. Ensure you're using a supported image format (JPEG, PNG, GIF, WebP):
   ```bash
   file /tmp/switchai-test/sample-image.jpg
   ```

4. Check the image file size (most providers have a 20MB limit):
   ```bash
   du -h /tmp/switchai-test/sample-image.jpg
   ```

5. Verify the model supports vision/multimodal inputs. Use vision-capable models like:
   - `ollama:llava`
   - `ollama:moondream`
   - `geminicli:gemini-2.5-pro`
   - `claude:claude-3-opus`

#### CLI Provider Errors

**Symptom**: CLI provider requests fail or the provider is not detected.

**Cause**: The CLI tool is not installed, not in PATH, or not properly configured.

**Solution**:
1. Verify the CLI tool is installed and accessible:
   ```bash
   which geminicli
   geminicli --version
   ```

2. Check the CLI path in your `config.yaml`:
   ```yaml
   providers:
     geminicli:
       enabled: true
       cli_path: "/usr/local/bin/geminicli"
   ```

3. Test the CLI tool directly (outside of switchAILocal):
   ```bash
   geminicli "Test message"
   ```

4. Ensure the CLI tool is authenticated (follow the tool's authentication instructions).

5. Check server logs for detailed error messages:
   ```bash
   ./ail.sh logs
   ```

#### Streaming Response Issues

**Symptom**: Streaming responses are incomplete, malformed, or not displaying in real-time.

**Cause**: Network buffering, incorrect SSE parsing, or client-side issues.

**Solution**:
1. Test streaming with curl's `--no-buffer` flag:
   ```bash
   curl --no-buffer http://localhost:18080/v1/chat/completions \
     -H "Content-Type: application/json" \
     -H "Authorization: Bearer sk-test-123" \
     -d '{
       "model": "switchai:switchai-fast",
       "messages": [{"role": "user", "content": "Count to 10"}],
       "stream": true
     }'
   ```

2. Verify the response format follows SSE standards (lines starting with `data: `).

3. Check if the provider supports streaming for the selected model.

4. Increase timeout values in `config.yaml` if streams are cutting off early:
   ```yaml
   providers:
     provider-name:
       timeout: 120
   ```

### Getting More Help

If you're still experiencing issues after trying these troubleshooting steps:

1. **Check Server Logs**: View detailed error messages and stack traces:
   ```bash
   ./ail.sh logs
   ```

2. **Verify Configuration**: Review your `config.yaml` file for syntax errors or misconfigurations.

3. **Test Providers Directly**: Try accessing providers directly (outside of switchAILocal) to isolate whether the issue is with the provider or switchAILocal.

4. **Check Provider Status**: Use the heartbeat endpoint to see real-time provider health:
   ```bash
   curl http://localhost:18080/v0/management/heartbeat/status \
     -H "Authorization: Bearer your-secret-key"
   ```

5. **Review Documentation**: Refer to the main documentation for detailed configuration guides and advanced troubleshooting.

---

**End of Examples Guide** - For more information, see the main `switchAILocal` documentation.
