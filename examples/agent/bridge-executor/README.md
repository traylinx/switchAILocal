# Bridge Executor Example

This example demonstrates the **WebSocket Relay API** - the true differentiator of switchAILocal.

Unlike the "Simple Coder" example (which uses direct HTTP), this example connects via WebSocket and sends HTTP requests through the relay. This is how agentic clients like Gemini CLI and Claude Code communicate with switchAILocal.

## Architecture

```
┌─────────────────┐     WebSocket      ┌──────────────────┐     HTTP      ┌───────────────┐
│  Your Client    │ ←────────────────→ │  switchAILocal   │ ←───────────→ │  LLM Provider │
│  (executor.py)  │    ws://...        │  (WS Relay)      │               │  (OpenAI/etc) │
└─────────────────┘                    └──────────────────┘               └───────────────┘
```

## Why WebSocket?

1. **Persistent Connection**: No TCP handshake per request.
2. **Bidirectional Streaming**: Real-time token streaming.
3. **Multiplexing**: Multiple requests over one connection.
4. **Agent Protocol**: Same protocol used by Gemini CLI.

## Usage

```bash
# Install dependencies
pip install -r requirements.txt

# List available models
python executor.py models

# Send a chat message
python executor.py chat "What is the capital of France?"
```

## Message Format

The WebSocket API uses JSON messages with this structure:

```json
{
  "id": "unique-request-id",
  "type": "http_request",
  "payload": {
    "method": "POST",
    "url": "/v1/chat/completions",
    "headers": {"Content-Type": ["application/json"]},
    "body": "{...}"
  }
}
```

Response types: `http_response`, `stream_start`, `stream_chunk`, `stream_end`, `error`.

## Requirements

- switchAILocal running on `ws://localhost:8081/ws`
- Python 3.8+
- `websockets` library
