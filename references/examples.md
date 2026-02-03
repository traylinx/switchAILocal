# Real-World Use Cases

Practical examples of how to leverage switchAILocal in agentic workflows.

## 1. Code Review with Deep Context

Use `geminicli:` to attach a whole directory for a security audit.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "geminicli:",
    "messages": [{"role": "user", "content": "Audit this codebase for OWASP vulnerabilities"}],
    "extra_body": {
      "cli": {
        "attachments": [{"type": "folder", "path": "./src"}]
      }
    }
  }'
```

---

## 2. Fast Private Summarization

Use local `ollama:` models for sensitive data where privacy is paramount.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "ollama:llama3.2",
    "messages": [{"role": "user", "content": "Summarize this confidential agreement: [TEXT]"}]
  }'
```

---

## 3. Autonomous Refactoring

Use `auto_approve` with CLI providers to perform multi-step refactoring tasks without manual confirmation.

```bash
curl http://localhost:18080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claudecli:",
    "messages": [{"role": "user", "content": "Split the monolith in main.go into smaller modules"}],
    "extra_body": {
      "cli": {
        "flags": {"auto_approve": true, "yolo": true}
      }
    }
  }'
```

---

## 4. Multi-Provider Validation

Query two different providers in parallel to verify a complex fact or reasoning step.

```python
import asyncio
from openai import AsyncOpenAI

client = AsyncOpenAI(base_url="http://localhost:18080/v1", api_key="sk-test-123")

async def verify_fact(question):
    tasks = [
        client.chat.completions.create(model="geminicli:", messages=[{"role": "user", "content": question}]),
        client.chat.completions.create(model="ollama:llama3.2", messages=[{"role": "user", "content": question}])
    ]
    responses = await asyncio.gather(*tasks)
    return [r.choices[0].message.content for r in responses]

# Check if both agree
```

## 5. Session-Based Workflows

Resume a previous CLI session to continue a complex task.

```json
{
  "model": "geminicli:",
  "messages": [{"role": "user", "content": "Now implement the unit tests for that function we just wrote"}],
  "extra_body": {
    "cli": {
      "session_id": "latest"
    }
  }
}
```
