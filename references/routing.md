# Intelligent Auto-Routing Reference

This document covers the full configuration and behavior of the Cortex Intelligent Router.

> For quick usage, see [SKILL.md](../SKILL.md). For deep architecture, see [docs-site](https://ail.traylinx.com/intelligent-systems/cortex-router).

## Configuration Block

Add this to your `config.yaml`:

```yaml
auto-routing:
  enabled: true
  max-resolution-ms: 5ms

  # Scoring weights (must sum to 1.0)
  weights:
    availability: 0.35    # Is the provider online?
    quota: 0.25           # How much quota remains?
    latency: 0.20         # How fast is the provider?
    success-rate: 0.20    # What % of requests succeed?

  # Soft-steering: boost specific models
  preferences:
    - model: "claude:claude-3-5-sonnet-20241022"
      preference: 0.9
      reason: "Best reasoning model"
    - model: "geminicli:gemini-2.5-flash"
      preference: 0.8
      reason: "Fast and free"

  # Intent-Based Filtering
  intent-matrix:
    coding:          # Coding & technical tasks
      - "claude:claude-3-5-sonnet-20241022"
      - "geminicli:gemini-2.5-pro"
    fast:            # Rapid heuristic tasks
      - "switchai:switchai-fast"
    reasoning:       # Deep logic puzzles
      - "claude:claude-3-5-sonnet-20241022"
    creative:        # Storytelling & roleplay
      - "geminicli:gemini-2.5-pro"
    vision:          # Image processing
      - "ollama:qwen3-vl:235b-instruct-cloud"
    audio:           # Audio input processing
      - "xiaomi:mimo-v2-tts"
    transcription:   # Speech-to-text
      - "whisper-1"

> **Note on `intent-matrix` vs `pipeline.intelligence.matrix`:** 
> The `intent-matrix` block evaluates an array of models and dynamically load-balances them by scoring their speed and health via the Cortex Router (e.g., when passing `model: "auto:coding"`). The older `pipeline.intelligence.matrix` block accepts only single strings and is used by Lua hooks for legacy hardcoded fallbacks.

  # Token Conservation
  conservation:
    enabled: true
    simple-threshold-tokens: 500    # Requests below this are "simple"
    premium-conservation-at: 0.20   # Penalize premium models for simple tasks

  # Provider Discovery
  discovery:
    enabled: true
    probe-on-startup: true
    probe-interval: 15m
    probe-timeout: 5s
    passive-monitoring: true
    cache-ttl: 24h

  # Autonomous Lab (Self-Optimizing Weights)
  lab:
    enabled: true
    adaptation-interval: 24h
    max-weight-drift: 0.10
```

## Scoring Formula

```
BaseScore    = W_a × Availability + W_q × Quota + W_l × Latency + W_s × SuccessRate
TierScore    = BaseScore + TierBoost(provider)
PrefScore    = TierScore + PreferenceBoost(model)
FinalScore   = PrefScore × ConservationMultiplier(tier, complexity)
```

### Tier Boosts

| Tier     | Boost  | Examples                |
|----------|--------|-------------------------|
| local    | +0.15  | `ollama:llama3`         |
| free     | +0.10  | `geminicli:`, `codex:`  |
| standard | +0.00  | `switchai:switchai-fast` |
| premium  | -0.05  | `claude:claude-3-5-sonnet` |

### Conservation

When `conservation.enabled: true`, simple requests (below `simple-threshold-tokens`) get a multiplier that penalizes premium-tier models. This ensures cheap tasks don't waste expensive models.

## Intent-Based Routing

When a request uses `model: "auto:coding"`, the router:

1. Extracts the intent hint (`coding`)
2. Filters the candidate pool to only models listed in `intent-matrix.coding`
3. Scores the filtered pool using the standard formula
4. Routes to the highest-scoring model

If no intent is provided (`model: "auto"`), the router uses heuristic classification based on the prompt content.

## Autonomous Lab

The Lab runs a shadow-weight experiment loop:

1. **Hypothesis**: Generate slightly perturbed weights (drift ≤ `max-weight-drift`)
2. **Shadow Score**: For each incoming request, score with both production and shadow weights
3. **Observe**: Accumulate RQS (Routing Quality Score) for both over the `adaptation-interval`
4. **Promote/Discard**: If shadow weights produce ≥5% higher RQS, promote them to production

This makes the routing engine self-improving over time without any manual tuning.

## RQS (Routing Quality Score)

```
RQS = W_latency × LatencyScore + W_success × SuccessScore + W_tier × TierScore
```

Default weights: `{latency: 0.3, success: 0.5, tier: 0.2}`
