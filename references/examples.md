# Real-World Examples

Working examples you can copy-paste directly. All assume the server is running on `http://localhost:18080`.

## 1. Basic Chat (FREE via CLI)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Explain the CAP theorem in 2 sentences."}]
  }'
```

## 2. Auto-Routing (Let Cortex Pick)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "auto",
    "messages": [{"role": "user", "content": "Hello! How are you?"}]
  }'
```

## 3. Intent-Based Routing (Coding)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "auto:coding",
    "messages": [{"role": "user", "content": "Write a thread-safe LRU cache in Go with generics."}]
  }'
```

## 4. Streaming Response

```bash
curl -N http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Write a poem about distributed systems"}],
    "stream": true
  }'
```

## 5. File Attachments (CLI Providers)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Review this codebase and suggest improvements"}],
    "extra_body": {
      "cli": {
        "attachments": [
          {"type": "folder", "path": "./src"},
          {"type": "file", "path": "./README.md"}
        ],
        "flags": {"auto_approve": true}
      }
    }
  }'
```

## 6. switchAI Cloud (Per-Token)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "switchai-fast",
    "messages": [{"role": "user", "content": "Summarize the latest AI news."}]
  }'
```

## 7. Python SDK

```python
from openai import OpenAI

client = OpenAI(base_url="http://localhost:18080/v1", api_key="sk-test-123")

# Auto-routing
response = client.chat.completions.create(
    model="auto",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)

# Streaming
stream = client.chat.completions.create(
    model="geminicli:",
    messages=[{"role": "user", "content": "Explain monads"}],
    stream=True
)
for chunk in stream:
    print(chunk.choices[0].delta.content or "", end="")
```

## 8. Node.js SDK

```javascript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'http://localhost:18080/v1',
  apiKey: 'sk-test-123',
});

const response = await client.chat.completions.create({
  model: 'auto:coding',
  messages: [{ role: 'user', content: 'Write a React hook for debouncing.' }],
});

console.log(response.choices[0].message.content);
```

## 9. Check Lab Telemetry

```bash
# Current weights and Lab status
curl -s http://localhost:18080/v0/management/autoroute/status | python3 -m json.tool

# Recent routing decisions
curl -s http://localhost:18080/v0/management/autoroute/journal | python3 -m json.tool
```

## 10. Multi-Provider with Xiaomi MiMo (Thinking + Web Search)

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-test-123" \
  -d '{
    "model": "xiaomi-tp:mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "Write a LinkedIn post about https://2md.traylinx.com/"}],
    "thinking": {"type": "enabled"},
    "tools": [
      {
        "type": "web_search",
        "max_keyword": 3,
        "force_search": true,
        "limit": 1,
        "user_location": {}
      }
    ],
    "tool_choice": "auto"
  }'
```
