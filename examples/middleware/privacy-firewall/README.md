# Privacy Firewall Example

This example demonstrates how to use the **Lua Plugin System** to implement a "Privacy Firewall" middleware. The middleware intercepts every request and automatically redacts sensitive information (PII) before it leaves your local machine.

## How it works

1.  **Interceptor**: The `pii_redactor.lua` script defines an `on_request` hook.
2.  **Engine**: switchAILocal loads this script at startup.
3.  **Sanitization**: Before any request is sent to an upstream provider (OpenAI, Gemini, etc.), the prompt is scanned for email addresses and phone numbers.
4.  **Redaction**: Detected PII is replaced with `[EMAIL REDACTED]` or `[PHONE REDACTED]`.

## Running the Example

1.  Start the server with the example configuration:
    ```bash
    switchAILocal -c config.yaml
    ```
    *(Or run using `go run`: `go run ../../../cmd/bridge-agent/main.go -c config.yaml`)*

2.  Send a test request with PII:
    ```bash
    curl http://localhost:8082/v1/chat/completions \
      -H "Content-Type: application/json" \
      -d '{
        "model": "gpt-4",
        "messages": [
          {"role": "user", "content": "My email is sebastian@example.com and phone is 555-0199. Please remember this."}
        ]
      }'
    ```

3.  **Observe Server Logs**:
    You will see:
    ```
    [Privacy Firewall] 🚨 PII Detected and Redacted!
    [Privacy Firewall] Request inspected and sanitized.
    ```

4.  **Result**: The LLM receives the sanitized prompt, ensuring your private data never leaves the privacy of your local environment.
