# Cortex Answer Schema Steering

A minimal, tightly-scoped Lua plugin that makes the **Cortex** grounded-answer
pipeline usable through switchAILocal when the terminal model is a small/local
model (e.g. `gpt-oss:20b` via Ollama).

## Why this exists

Cortex's generic answer prompt instructs the model to emit a `classification_code`
for every statement, but the prompt does **not** enumerate the allowed values. The
Cortex gateway, however, hard-validates each code against a fixed set:

```
SUPPORTED · PARTIALLY_SUPPORTED · CONFLICTING · ABSTAINED
```

With no vocabulary in the prompt, a model guesses (`"FACT"`, `"factual"`, …), the
gateway rejects the answer, and its regeneration loop retries until Cortex's
10-second client read-lease expires — the caller sees `AUTH_REVOKED` and never gets
an answer. This is a defect on the Cortex side (the prompt should carry its own
vocabulary); this plugin is the **interim proxy-side compatibility shim** so the
pipeline works today.

## What it does

`on_request` fires only when the request body carries a Cortex marker, so ordinary
traffic is never touched:

| Trigger (substring in body) | Injected system steering |
|---|---|
| `classification_code` (answer call) | The four allowed codes, correct abstention (`{"statements": []}`, because an empty `citation_ids` list is schema-invalid), and `Reasoning: low` |
| `material statement` / `Identify contradictions` (grounding / verifier calls) | `Reasoning: low` only |

`Reasoning: low` is the harmony control token for `gpt-oss`: it drops output from
~600 reasoning tokens (>10 s) to ~300 tokens (~3.5 s), which is what keeps the full
generate + verify chain inside the client lease.

## Effect (measured)

Against a 6-doc corpus, `Cortex → switchAILocal (ail-compound) → Ollama gpt-oss:20b`:

- On-corpus questions: grounded, single-citation answers, p50 ~6.8 s.
- Off-corpus questions: correct abstention in ~2.5 s.

## Enable

```yaml
plugin:
  enabled: true
  enabled-plugins:
    - "cortex-answer-schema"
```

## Remove it when Cortex is fixed

Once the Cortex answer prompt carries its own classification vocabulary, this
plugin is redundant — drop it from `enabled-plugins`. It is deliberately inert for
any request that does not look like a Cortex answer/grounding call.
