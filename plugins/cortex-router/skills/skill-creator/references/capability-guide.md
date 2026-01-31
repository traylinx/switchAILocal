# Capability Selection Guide

This guide helps you choose the right `required-capability` for your skill.

## Available Capabilities

| Capability | Best For | Model Characteristics |
|------------|----------|----------------------|
| `coding` | Code generation, debugging, technical tasks | High context, code-optimized |
| `reasoning` | Complex analysis, math, multi-step logic | Accuracy over speed |
| `creative` | Writing, brainstorming, content creation | General capability |
| `fast` | Quick responses, simple queries | Low latency, efficient |
| `secure` | Sensitive data, privacy-critical tasks | Local models preferred |
| `vision` | Image analysis, UI screenshots, diagrams | Vision capability required |
| `audio` | Audio transcription, speech analysis | Audio processing |
| `cli` | Command execution, system operations | CLI access enabled |
| `long_ctx` | Large documents, extensive context | Large context window |

## Decision Tree

```
Does the skill analyze images or screenshots?
├── Yes → vision
└── No
    │
    Does the skill handle sensitive/private data?
    ├── Yes → secure
    └── No
        │
        Does the skill involve code or technical tasks?
        ├── Yes → coding
        └── No
            │
            Does the skill require complex reasoning or math?
            ├── Yes → reasoning
            └── No
                │
                Does the skill involve creative writing?
                ├── Yes → creative
                └── No
                    │
                    Does the skill process large documents?
                    ├── Yes → long_ctx
                    └── No → fast (default)
```

## Examples by Domain

### Development Skills
```yaml
# Code generation, debugging, reviews
required-capability: coding

# Architecture decisions, system design
required-capability: reasoning

# UI/UX from screenshots
required-capability: vision
```

### Content Skills
```yaml
# Blog writing, marketing copy
required-capability: creative

# Document summarization
required-capability: long_ctx

# Quick edits, formatting
required-capability: fast
```

### Data Skills
```yaml
# Data analysis, statistics
required-capability: reasoning

# PII handling, compliance
required-capability: secure

# Chart/graph interpretation
required-capability: vision
```

## Capability Combinations

Some skills may benefit from multiple capabilities. In these cases:

1. **Choose the primary capability** - The one most critical for the skill's core function
2. **Document secondary needs** - Mention in the skill description that certain features may require other capabilities

Example:
```yaml
name: frontend-expert
description: Expert in React and TailwindCSS. Can analyze UI screenshots (uses vision capability) and generate code.
required-capability: coding  # Primary: code generation
```

The routing system will:
1. Match the skill based on description
2. Route to the `coding` capability slot
3. If the query includes images, the reflex tier may override to `vision`

## Performance Considerations

| Capability | Typical Latency | Cost |
|------------|-----------------|------|
| `fast` | 50-200ms | Low |
| `coding` | 200-500ms | Medium |
| `creative` | 200-500ms | Medium |
| `reasoning` | 500ms-2s | High |
| `vision` | 500ms-1s | High |
| `secure` | Varies (local) | Free |
| `long_ctx` | 500ms-2s | High |

Choose `fast` when:
- The skill doesn't require specialized capabilities
- Response time is critical
- Tasks are simple lookups or formatting
