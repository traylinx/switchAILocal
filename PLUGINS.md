# Lua Plugin System Manual

The Lua Plugin System allows you to intercept and modify requests and responses in real-time. This is the foundation of the **Cortex Router** intelligent routing engine.

## Overview

The plugin system enables:
- **Request/Response Interception**: Modify requests before they reach providers
- **Intelligent Routing**: Route requests to optimal models based on content analysis
- **Skill-Based Augmentation**: Enhance prompts with domain-specific expertise
- **Multi-Tier Routing**: Reflex → Semantic → Cognitive routing with verification

## Quick Start

### 1. Enable Plugins

Plugins are **explicitly enabled** in `config.yaml`:

```yaml
plugin:
  enabled: true
  plugin-dir: "./plugins"
  enabled-plugins:
    - "cortex-router"  # The intelligent routing plugin
```

### 2. Enable Intelligence Services (Optional)

For Phase 2 features (semantic matching, skill matching, cascading):

```yaml
intelligence:
  enabled: true
  
  # Phase 2 features
  discovery:
    enabled: true
  embedding:
    enabled: true
  semantic-tier:
    enabled: true
  skill-matching:
    enabled: true
```

See [CORTEX_ROUTER_PHASE2.md](docs/CORTEX_ROUTER_PHASE2.md) for full configuration options.

## Plugin Structure

Plugins are now **folder-based** with a standardized structure:

```
plugins/
└── my-plugin/
    ├── schema.lua      # Plugin metadata
    ├── handler.lua     # Plugin logic
    └── skills/         # Optional: Domain-specific skills
        └── my-skill/
            └── SKILL.md
```

### schema.lua (Metadata)

Defines the plugin's identity:

```lua
return {
    name = "my-plugin",              -- Must match folder name
    display_name = "My Plugin",      -- Human-readable name
    version = "1.0.0",
    description = "What this plugin does"
}
```

### handler.lua (Logic)

Implements the plugin hooks:

```lua
local Schema = require("schema")

local Plugin = {}

function Plugin:on_request(req)
    -- Modify request before it's sent
    -- req.model, req.body, req.metadata
    return req  -- or nil to skip
end

function Plugin:on_response(res)
    -- Process response after it's received
    -- res.body, res.model, res.metadata
    return res  -- or nil to skip
end

return Plugin
```

## The Cortex Router Plugin

The **cortex-router** plugin implements intelligent multi-tier routing:

### Routing Tiers

1. **Cache Tier** (<1ms): Semantic cache lookup
2. **Reflex Tier** (<1ms): Fast pattern matching (PII, code, images)
3. **Semantic Tier** (<20ms): Embedding-based intent matching
4. **Cognitive Tier** (200-500ms): LLM classification with confidence
5. **Verification**: Cross-validates results
6. **Cascade**: Quality-based model escalation

### Skills System

The Cortex Router includes 21 pre-built skills for domain-specific routing:

- `go-expert`, `python-expert`, `typescript-expert` - Language-specific expertise
- `security-expert`, `devops-expert`, `docker-expert` - Infrastructure skills
- `frontend-expert`, `vision-expert`, `testing-expert` - Development skills
- And more...

See [CORTEX_ROUTER_PHASE2.md](docs/CORTEX_ROUTER_PHASE2.md) for the complete list.

## The `switchai` Host API

Plugins access host functionality through the `switchai` bridge:

### Core Functions (Phase 1)

```lua
-- Logging
switchai.log(message)

-- LLM Classification
local json, err = switchai.classify(prompt)

-- Configuration
local router_model = switchai.config.router_model
local matrix = switchai.config.matrix

-- Cache
switchai.set_cache(key, value)
local value = switchai.get_cache(key)

-- Prompt Injection
local new_body = switchai.json_inject(req.body, system_prompt)
```

### Intelligence Functions (Phase 2)

```lua
-- Model Discovery
local models, err = switchai.get_available_models()
local available = switchai.is_model_available("openai:gpt-4")

-- Dynamic Matrix
local matrix, err = switchai.get_dynamic_matrix()

-- Embedding
local embedding, err = switchai.embed(text)
local similarity = switchai.cosine_similarity(vec_a, vec_b)

-- Semantic Matching
local result, err = switchai.semantic_match_intent(query)
-- Returns: {intent, confidence, latency_ms}

-- Skill Matching
local result, err = switchai.match_skill(query)
-- Returns: {skill: {id, name, system_prompt}, confidence}

-- Semantic Cache
local cached, err = switchai.cache_lookup(query)
switchai.cache_store(query, decision, metadata)

-- Confidence Scoring
local decision, err = switchai.parse_confidence(json_response)
-- Returns: {intent, complexity, confidence}

-- Verification
local match = switchai.verify_intent(intent1, intent2)

-- Cascade Evaluation
local eval, err = switchai.evaluate_response(response, current_tier)
-- Returns: {should_cascade, next_tier, quality_score, reason, signals}

-- Feedback
switchai.record_feedback({
    query = "...",
    intent = "coding",
    selected_model = "...",
    success = true
})
```

## Creating Custom Plugins

### 1. Create Plugin Directory

```bash
mkdir -p plugins/my-plugin
```

### 2. Create schema.lua

```lua
return {
    name = "my-plugin",
    display_name = "My Custom Plugin",
    version = "1.0.0",
    description = "Custom routing logic"
}
```

### 3. Create handler.lua

```lua
local Schema = require("schema")

local Plugin = {}

function Plugin:on_request(req)
    switchai.log("Processing request for: " .. req.model)
    
    -- Your custom logic here
    
    return req
end

return Plugin
```

### 4. Enable in config.yaml

```yaml
plugin:
  enabled: true
  enabled-plugins:
    - "my-plugin"
```

## Security & Isolation

- **Sandboxed Execution**: Plugins run in a restricted Lua environment
- **No Direct I/O**: Cannot access network or filesystem directly
- **Allowlisted Commands**: Only safe commands available via `switchai.exec()`
- **Timeout Protection**: Execution bound by request context timeout
- **No Dangerous Globals**: `dofile`, `loadfile`, `os.execute` are disabled

## Documentation

- [CORTEX_ROUTER_PHASE2.md](docs/CORTEX_ROUTER_PHASE2.md) - Full Phase 2 feature guide
- [CORTEX_ROUTER_QA_REPORT.md](docs/CORTEX_ROUTER_QA_REPORT.md) - QA findings and status
