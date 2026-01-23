# Resilient Router Configuration

This example demonstrates how to configure switchAILocal as a **Resilient Router** using a tiered failover strategy.

## The Strategy

The goal is to optimize for cost and uptime by using a "waterfall" approach:

1.  **Tier 1 (Local/Free)**: Attempt to use a local model (via Ollama) first. It's free and private.
2.  **Tier 2 await (Cheap Cloud)**: If the local model is unavailable or overloaded, fall back to a cheap, high-speed API (e.g., Gemini Flash).
3.  **Tier 3 (Premium Cloud)**: As a last resort, use the expensive frontier model.

## Configuration Highlights

*   `routing.strategy: "fill-first"`: Tells the engine to prioritize the first successfully loaded/configured provider match.
*   **Provider Ordering**: Order matters implicitly when combined with model aliasing. If multiple providers claim to support "gpt-4" (e.g., via aliasing mechanics), the router can be tuned to prefer one connection over another.

## Running this Configuration

```bash
switchAILocal -c config.yaml
```

## Testing Failover

1.  Ensure Ollama is running. Send a request. It should hit Ollama.
2.  **Stop Ollama**. Send the same request.
3.  switchAILocal will detect the upstream failure (or missing model) and route the request to the next available provider (Gemini).
