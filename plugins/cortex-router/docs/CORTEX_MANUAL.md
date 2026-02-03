# Cortex Intelligent Router: Technical Manual (Deep Dive)

The **Cortex Intelligent Router** is the cognitive nucleus of `switchAILocal`. It transforms the system from a passive proxy into an active decision-maker that understands user intent and optimizes model selection "in the moment."

---

## 1. Request Interception Logic

Cortex only activates when a user explicitly requests an abstract model ID. It intercepts requests where:

- `model == "auto"`
- `model == "cortex"`

If any other specific model ID is provided (e.g., `gemini:pro`), Cortex steps aside and allows the standard routing logic to proceed.

---

## 2. Multi-Tier Routing Architecture (Phase 6)

Cortex uses a cascading multi-tier approach to balance speed, cost, and intelligence.

### Tier 0: Memory & Preferences (Highest Priority)

Cortex checks for learned user preferences and cached decisions.

- **Learned Preferences**: If a user consistently prefers a specific model for an intent (e.g., "coding"), Cortex prioritizes it.
- **Semantic Cache**: Exact or highly similar prompts are routed based on previous successful results.

### Tier 1: The Reflex Tier (Pattern-Based)

Fast, rule-based routing using high-performance regex in Go.

- **PII Detection**: Routes sensitive data to local/secure models.
- **Code/Math Detection**: Recognizes programming and mathematical syntax.
- **Greeting Detection**: Routes simple interactions to fast, low-cost models.

### Tier 2: The Semantic Tier (Embedding-Based)

Uses vector embeddings to match the user's prompt against a library of known intents.

- **Mechanism**: Compares the prompt's embedding with intent centroids.
- **Confidence**: Requires high similarity (> 0.7) to trigger.

### Tier 3: The Cognitive Tier (LLM-Based Fallback)

The final fallback using LLM-based classification.

- **Router Model**: A small, fast model (e.g., Qwen 0.5B) classifies the intent and complexity.
- **Decision Matrix**: Maps the classification result to the best available expert.

---

## 3. Memory Integration

The memory system allows Cortex to "learn" from every interaction:

- **Decision Recording**: Every routing decision is logged with metadata (intent, confidence, tier).
- **Outcome Learning**: Success/failure of requests updates the provider bias and model preferences.
- **Provider Quirks**: Discovered issues (e.g., "hallucinates in system prompts") are stored as quirks and used to adjust confidence scores in future decisions.

### Production Integration

As of the intelligent systems integration, the Memory system is now fully integrated into the production server:

- **Automatic Recording**: All routing decisions are automatically recorded when `memory.enabled: true` in config
- **No CLI Required**: Memory recording happens transparently during request processing
- **Analytics API**: Access memory analytics via Management API endpoints
- **Configurable Retention**: Set retention period via `memory.retention-days` in config
- **Storage Management**: Automatic cleanup of old records

**Configuration Example:**
```yaml
memory:
  enabled: true
  retention-days: 30
  storage-path: "./.memory"
```

**Access Memory Data:**
```bash
# Via Management API
curl http://localhost:18080/v0/management/memory/stats \
  -H "X-Management-Key: your-secret-key"

# Via CLI (legacy)
switchAILocal memory stats
```

See [Intelligent Systems Guide](../../docs/user/intelligent-systems.md) for complete documentation.

---

## 4. Reliability & Health Monitoring

Cortex is aware of the health of the underlying providers:

- **Health Checks**: Periodic monitoring of provider availability and latency.
- **Fallback Routing**: If the preferred model or provider is down, Cortex automatically falls back to the next best available option in the tier.
- **Quota Awareness**: Detects and respects rate limits, shifting traffic away from exhausted quotas before failures occur.

### Production Integration

The Heartbeat Monitor is now integrated into the production server for automatic health monitoring:

- **Background Monitoring**: Continuous health checks when `heartbeat.enabled: true` in config
- **Automatic Failover**: Provider status changes trigger automatic routing adjustments
- **Quota Tracking**: Real-time quota monitoring with configurable thresholds
- **Event Emission**: Health events trigger hooks for automated responses

**Configuration Example:**
```yaml
heartbeat:
  enabled: true
  interval: 5m
  timeout: 30s
  retry-attempts: 3
  quota-warning: 80
  quota-critical: 95
```

**Monitor Provider Health:**
```bash
curl http://localhost:18080/v0/management/heartbeat/status \
  -H "X-Management-Key: your-secret-key"
```

See [Intelligent Systems Guide](../../docs/user/intelligent-systems.md) for complete documentation.

---

## 5. Implementation Reference (Go)

The core logic is implemented in `internal/intelligence/router.go`.

### Key Components
- `CortexRouter`: The main routing engine coordinating all tiers.
- `MemoryManager`: Persists history and learned preferences.
- `SemanticTier`: Handles embedding-based matching.
- `ModelRegistry`: Tracks model availability across providers.

### Integration with Intelligent Systems

Cortex now works seamlessly with the four integrated intelligent systems:

**Steering Engine Integration:**
- Steering rules are evaluated **before** Cortex routing
- Rules can override Cortex decisions by forcing specific providers/models
- Cortex respects steering constraints when making routing decisions
- Use steering for policy enforcement (cost limits, compliance, etc.)

**Hooks Manager Integration:**
- Cortex routing decisions emit `routing_decision` events
- Hooks can trigger on routing events for monitoring/alerting
- Failed routing attempts emit `request_failed` events
- Use hooks to track Cortex performance and decision patterns

**Example: Steering + Cortex**
```yaml
# steering/cortex-override.yaml
name: "force-local-for-sensitive"
priority: 200  # Higher than Cortex
enabled: true

conditions:
  - field: "messages[0].content"
    operator: "contains"
    value: "confidential"

actions:
  - type: "set-provider"
    value: "ollama"  # Force local, override Cortex
```

**Example: Hook for Cortex Monitoring**
```yaml
# hooks/cortex-analytics.yaml
name: "track-cortex-decisions"
enabled: true
event: "routing_decision"

action:
  type: "webhook"
  url: "https://analytics.example.com/cortex"
  body:
    model: "{{.Model}}"
    provider: "{{.Provider}}"
    timestamp: "{{.Timestamp}}"
```

See [Intelligent Systems Guide](../../docs/user/intelligent-systems.md) for complete integration documentation.

---

## 10. Safety: Infinite Loop Protection

Cortex ensures that classification requests (Cognitive Tier) do not themselves trigger the router, preventing recursive loops via internal flags and specialized model IDs.
