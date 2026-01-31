# Cortex Router Phase 2 User Guide

This guide covers the intelligent routing features introduced in Phase 2 of the Cortex Router.

## Overview

Phase 2 transforms the Cortex Router from a static routing system into an intelligent, adaptive orchestrator that:

- **Automatically discovers** available models from all configured providers
- **Semantically matches** user queries to intents and skills
- **Dynamically assigns** optimal models to capability slots
- **Cascades** to better models when response quality is insufficient
- **Collects feedback** for continuous improvement

## Quick Start

### Enable Intelligence Services

Add to your `config.yaml`:

```yaml
intelligence:
  enabled: true  # Master switch
  
  # Core routing (v1.0)
  router-model: "ollama:qwen:0.5b"
  router-fallback: "openai:gpt-4o-mini"
  matrix:
    coding: "switchai-chat"
    reasoning: "switchai-reasoner"
    creative: "switchai-chat"
    fast: "switchai-fast"
    secure: "ollama:llama3.2"  # Local model for privacy
    vision: "switchai-chat"
```

### Enable Phase 2 Features

```yaml
intelligence:
  enabled: true
  
  # Phase 2 features (all optional)
  discovery:
    enabled: true
    refresh-interval: 3600  # Re-discover models every hour
    cache-dir: "~/.switchailocal/cache/discovery"
  
  embedding:
    enabled: true
    model: "all-MiniLM-L6-v2"
  
  semantic-tier:
    enabled: true
    confidence-threshold: 0.85
  
  skill-matching:
    enabled: true
    confidence-threshold: 0.80
  
  semantic-cache:
    enabled: true
    similarity-threshold: 0.95
    max-size: 10000
  
  confidence:
    enabled: true
  
  verification:
    enabled: true
  
  cascade:
    enabled: true
    quality-threshold: 0.70
  
  feedback:
    enabled: true
    retention-days: 90
```

### Download Embedding Model

Before using semantic features, download the embedding model:

```bash
./scripts/download-embedding-model.sh
```

## Feature Reference

### Model Discovery

Automatically discovers available models from all configured providers at startup.

**Configuration:**
```yaml
discovery:
  enabled: true
  refresh-interval: 3600  # seconds
  cache-dir: "~/.switchailocal/cache/discovery"
```

**Output:** Creates `available_models.json` in the cache directory.

**Lua API:**
```lua
local models, err = switchai.get_available_models()
if models then
    for _, model in ipairs(models) do
        print(model.id, model.provider, model.is_available)
    end
end
```

### Dynamic Matrix

Auto-assigns optimal models to capability slots based on discovered model capabilities.

**Configuration:**
```yaml
auto-assign:
  enabled: true
  prefer-local: true      # Prefer local models for 'secure' slot
  cost-optimization: true # Favor cheaper models when quality is similar
  overrides:              # Manual overrides
    secure: "ollama:llama3.2"
```

**Capability Slots:**
| Slot | Purpose | Scoring Priority |
|------|---------|------------------|
| `coding` | Code generation, debugging | Coding capability, context window |
| `reasoning` | Complex analysis, math | Reasoning capability, accuracy |
| `creative` | Writing, brainstorming | General capability, context |
| `fast` | Quick responses | Low latency, low cost |
| `secure` | Sensitive data | Local models preferred |
| `vision` | Image analysis | Vision capability required |

**Lua API:**
```lua
local matrix, err = switchai.get_dynamic_matrix()
if matrix then
    local coding = matrix.coding
    print("Primary:", coding.primary)
    print("Fallbacks:", table.concat(coding.fallbacks, ", "))
end
```

### Semantic Tier

Matches queries to intents using embedding similarity, bypassing LLM classification for high-confidence matches.

**Configuration:**
```yaml
semantic-tier:
  enabled: true
  confidence-threshold: 0.85  # Route directly if confidence >= this
```

**Supported Intents:**
- `coding` - Code-related tasks
- `reasoning` - Complex analysis
- `creative` - Creative writing
- `fast` - Quick factual questions
- `secure` - Sensitive data handling
- `vision` - Image analysis
- `long_context` - Large document processing
- `image_generation` - Creating images
- `transcription` - Audio to text
- `speech` - Text to audio
- `research` - Information gathering
- `chat` - General conversation

**Lua API:**
```lua
local result, err = switchai.semantic_match_intent(query)
if result and result.confidence >= 0.85 then
    print("Intent:", result.intent)
    print("Confidence:", result.confidence)
    print("Latency:", result.latency_ms, "ms")
end
```

### Skill Matching

Matches queries to domain-specific skills and augments prompts with skill instructions.

**Configuration:**
```yaml
skills:
  enabled: true
  directory: "plugins/cortex-router/skills"

skill-matching:
  enabled: true
  confidence-threshold: 0.80
```

**Lua API:**
```lua
local result, err = switchai.match_skill(query)
if result and result.confidence >= 0.80 then
    print("Skill:", result.skill.name)
    -- Augment prompt with skill system prompt
    req.body = switchai.json_inject(req.body, result.skill.system_prompt)
end
```

### Semantic Cache

Caches routing decisions based on semantic similarity for faster repeated queries.

**Configuration:**
```yaml
semantic-cache:
  enabled: true
  similarity-threshold: 0.95  # Cache hit if similarity >= this
  max-size: 10000             # Maximum cache entries
```

**Performance:** Cache hits return in < 1ms vs 200-500ms for LLM classification.

**Lua API:**
```lua
-- Lookup
local cached, err = switchai.cache_lookup(query)
if cached then
    return cached.decision
end

-- Store
switchai.cache_store(query, decision, {intent = "coding"})

-- Metrics
local metrics = switchai.cache_metrics()
print("Hit rate:", metrics.hit_rate)
```

### Confidence Scoring

Adds confidence scores to LLM classification results for smarter routing decisions.

**Configuration:**
```yaml
confidence:
  enabled: true

verification:
  enabled: true
  confidence-threshold-low: 0.60   # Below this: escalate to reasoning
  confidence-threshold-high: 0.90  # Above this: route immediately
```

**Routing Logic:**
- **High confidence (>0.90):** Route immediately
- **Medium confidence (0.60-0.90):** Verify with semantic tier
- **Low confidence (<0.60):** Escalate to reasoning model

**Lua API:**
```lua
local decision, err = switchai.parse_confidence(json_response)
if decision then
    print("Intent:", decision.intent)
    print("Complexity:", decision.complexity)
    print("Confidence:", decision.confidence)
end
```

### Model Cascading

Automatically retries with a more capable model when response quality is insufficient.

**Configuration:**
```yaml
cascade:
  enabled: true
  quality-threshold: 0.70  # Cascade if quality score < this
```

**Cascade Tiers:**
1. `fast` → `standard` → `reasoning`

**Quality Signals Detected:**
- Abrupt endings
- Missing sections
- Incomplete responses
- Error patterns
- Very short responses

**Lua API:**
```lua
local evaluation, err = switchai.evaluate_response(response, "standard")
if evaluation and evaluation.should_cascade then
    print("Cascade to:", evaluation.next_tier)
    print("Reason:", evaluation.reason)
    print("Quality score:", evaluation.quality_score)
end
```

### Feedback Collection

Records routing decisions and outcomes for analysis and future learning.

**Configuration:**
```yaml
feedback:
  enabled: true
  retention-days: 90
```

**Lua API:**
```lua
switchai.record_feedback({
    query = "How do I write a Go function?",
    intent = "coding",
    selected_model = "switchai-chat",
    routing_tier = "semantic",
    confidence = 0.92,
    matched_skill = "go-expert",
    cascade_occurred = false,
    latency_ms = 150,
    success = true
})
```

## Routing Flow

```
Request (model="auto")
         │
    ┌────┴────┐
    │  CACHE  │ ← Semantic cache lookup (< 1ms)
    └────┬────┘
         │ miss
    ┌────┴────┐
    │ REFLEX  │ ← Pattern matching (< 1ms)
    └────┬────┘   PII → secure, Code → coding, Image → vision
         │ no match
    ┌────┴────┐
    │SEMANTIC │ ← Embedding similarity (< 20ms)
    └────┬────┘   + Skill matching
         │ low confidence
    ┌────┴────┐
    │COGNITIVE│ ← LLM classification (200-500ms)
    └────┬────┘   + Confidence scoring
         │ medium confidence
    ┌────┴────┐
    │ VERIFY  │ ← Cross-validate with semantic (< 100ms)
    └────┬────┘
         │
    ┌────┴────┐
    │ SELECT  │ ← Dynamic matrix or static config
    └────┬────┘
         │
    ┌────┴────┐
    │ EXECUTE │
    └────┬────┘
         │
    ┌────┴────┐
    │ CASCADE │ ← Quality check, retry if needed
    └────┬────┘
         │
    ┌────┴────┐
    │FEEDBACK │ ← Record outcome
    └─────────┘
```

## Graceful Degradation

Phase 2 features degrade gracefully when dependencies are unavailable:

| Scenario | Behavior |
|----------|----------|
| `intelligence.enabled: false` | All Phase 2 disabled, v1.0 behavior |
| Embedding model missing | Semantic tier disabled, use cognitive |
| Discovery fails | Use static matrix from config |
| Skill matching fails | Route without skill augmentation |
| Verification fails | Accept original classification |
| Feedback DB fails | Log warning, continue |

## Management API

Phase 2 adds management endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /v0/management/skills` | List loaded skills |
| `GET /v0/management/feedback` | Get feedback statistics |
| `POST /v0/management/feedback` | Submit explicit feedback |

## Troubleshooting

### Semantic tier not working

1. Check embedding model is downloaded:
   ```bash
   ls ~/.switchailocal/models/all-MiniLM-L6-v2/
   ```

2. Verify embedding is enabled:
   ```yaml
   embedding:
     enabled: true
   ```

3. Check logs for initialization errors

### Skills not matching

1. Verify skills directory exists and contains SKILL.md files
2. Check skill descriptions are descriptive enough for semantic matching
3. Lower the confidence threshold if needed:
   ```yaml
   skill-matching:
     confidence-threshold: 0.70
   ```

### Cache not helping

1. Check cache is enabled and has sufficient size
2. Verify similarity threshold isn't too high:
   ```yaml
   semantic-cache:
     similarity-threshold: 0.90  # Lower for more hits
   ```

### Discovery not finding models

1. Check provider credentials are configured
2. Verify network connectivity to providers
3. Check discovery cache directory is writable

## Performance Tuning

### Optimize for Speed

```yaml
intelligence:
  semantic-tier:
    confidence-threshold: 0.80  # Lower = more semantic routing
  semantic-cache:
    enabled: true
    max-size: 50000  # Larger cache
  cascade:
    enabled: false  # Disable cascade for speed
```

### Optimize for Quality

```yaml
intelligence:
  semantic-tier:
    confidence-threshold: 0.90  # Higher = more LLM verification
  verification:
    enabled: true
  cascade:
    enabled: true
    quality-threshold: 0.80  # Higher = more cascades
```

### Optimize for Cost

```yaml
intelligence:
  auto-assign:
    cost-optimization: true
  cascade:
    enabled: true  # Start cheap, escalate if needed
```

## Available Skills

The Cortex Router includes 21 pre-built skills for domain-specific routing:

| Skill | Capability | Description |
|-------|------------|-------------|
| `api-designer` | coding | REST API design, OpenAPI specifications |
| `blog-optimizer` | creative | Blog writing and SEO optimization |
| `debugging-expert` | reasoning | Systematic debugging and root cause analysis |
| `devops-expert` | coding | CI/CD, infrastructure as code, monitoring |
| `docker-expert` | coding | Containerization, Dockerfile optimization |
| `frontend-design` | creative | Distinctive UI design, avoiding AI aesthetics |
| `frontend-expert` | coding | React, TailwindCSS, modern frontend |
| `git-expert` | cli | Git workflows, conventional commits |
| `go-expert` | coding | Go/Golang development for switchAILocal |
| `k8s-expert` | coding | Kubernetes, Helm, cloud native |
| `mcp-builder` | coding | Model Context Protocol server development |
| `python-expert` | coding | Python with async, type hints, pytest |
| `security-expert` | reasoning | Security auditing, vulnerability analysis |
| `skill-creator` | - | Creating new skills |
| `sql-expert` | reasoning | SQL queries, optimization, database design |
| `switchai-architect` | reasoning | switchAILocal architecture expertise |
| `testing-expert` | coding | Testing methodologies, TDD, Vitest |
| `typescript-expert` | coding | TypeScript type system, advanced patterns |
| `vision-expert` | vision | Image analysis, UI-to-code conversion |
| `web-artifacts-builder` | coding | React artifacts with shadcn/ui |
| `webapp-testing` | coding | Playwright web application testing |

### Creating Custom Skills

See the `skill-creator` skill for guidance on creating new skills. Each skill requires:

1. A directory under `plugins/cortex-router/skills/`
2. A `SKILL.md` file with YAML frontmatter:
   ```yaml
   ---
   name: my-skill
   description: What this skill does and when to use it
   required-capability: coding  # or reasoning, creative, vision, etc.
   ---
   ```
3. Markdown instructions in the body
