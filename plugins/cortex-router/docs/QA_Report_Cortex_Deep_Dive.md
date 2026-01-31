# QA Deep Dive: Cortex Intelligent Router

This document provides a senior-level QA audit of the Cortex Intelligent Router implementation, covering internal mechanics, edge cases, security, and performance.

---

## 1. Functional Summary

The Cortex Router is a **multi-tier request orchestrator**. It determines the optimal model for any incoming request through a combination of fast heuristics (Reflex) and deep analysis (Cognitive).

### Implementation Architecture
- **Host (Go)**: Exports `switchai.classify(text)` to the Lua sandbox. Manages LLM-based classification, caching, and recursive protection.
- **Plugin (Lua)**: Implements the `on_request` hook. Controlled by `handler.lua`.
- **Registry (Go)**: Stores the `IntelligenceConfig` (Matrix, Router Model, Fallback).

---

## 2. Technical Audit: The Cognitive Flow

### Tier 1: Reflex Analysis (Lua)
| Feature             | Implementation                                     | Notes                                                                  |
| :------------------ | :------------------------------------------------- | :--------------------------------------------------------------------- |
| **PII Detection**   | Regex: `[%w%.%_%-]+@[%w%.%_%-]+%.%a%a+`            | Detects basic emails. Subject to false negatives for complex patterns. |
| **Code Detection**  | 4-way Heuristic (` ``` ` , `def`, `func`, `class`) | Very fast, but can be fooled by plain text discussions about code.     |
| **Length Trigger**  | Byte-count check (> 4,000 chars)                   | Prevents expensive classification calls for massive documents.         |
| **Endpoint Reflex** | Payload structure check for Image Gen              | Detects `/v1/images/generations` even if `model: auto` is used.        |

### Tier 2: Cognitive Routing (Go/LLM)
Cortex delegates to a specialized **Router Model**.

- **System Prompt Integrity**: The prompt is hardcoded in `sdk/api/handlers/handlers.go:L835`. It defines a strict taxonomy of intents.
- **Deterministic settings**: `temperature: 0` is strictly enforced to ensure stable routing decisions.
- **Response Format**: Expected in raw JSON.
  > [!WARNING]
  > The Lua parser `parse_classification` uses `string.match` to pull JSON values. If the LLM adds markdown wrappers (e.g., ` ```json `), the current regex `\"intent\"%s*:%s*\"(.-)\"` will still function, but complex nested JSON might fail.

---

## 3. Security & Stability Audit

### Recursive Loop Protection (CRITICAL)
- **Mechanism**: `plugin.SkipLuaContextKey` ("skip_lua").
- **Audit Result**: **PASSED**. When `BaseAPIHandler.Classify` is called, it injects the `skip_lua` key into the context. The `lua_engine.go` checks for this key before running hooks. This successfully prevents an infinite loop where the classifier triggers itself.

### Error Handling & Fallbacks
- **Router Failure**: If the classification model fails (timeout/error), Go attempts the `RouterFallback`. If that fails, it returns `nil` to Lua.
- **Lua Fallback**: In `handler.lua`, if `switchai.classify` returns an error, the script defaults to the `fast` model alias.
- **Audit Result**: **PASSED**. The system is robust against expert-model unavailability.

---

## 4. Performance Audit

### Performance Optimizations
- **Host-Side Cache**: `LuaEngine` maintains a `map[string]string` cache for classification results.
- **Audit Result**: The cache is limited to **1000 items** with a "clear all" eviction strategy. This is efficient for memory usage but might cause sudden latency spikes after a clear.

---

## 5. Identified Edge Cases & Risks

### A. Brittle JSON Parsing in Lua
The `parse_classification` function in `handler.lua` is not a full JSON parser:
```lua
local intent = string.match(json_str, '"intent"%s*:%s*"(.-)"')
```
- **Risk**: If the LLM generates JSON with different quotes, extra spaces, or escaped characters, the match might fail.
- **Recommendation**: Integrate a robust `cjson` or similar Lua JSON library if classification reliability becomes an issue.

### B. Intent Taxonomy Mismatch
The Go System Prompt supports a `factual` intent, but `handler.lua` does not have a specific matrix key for it.
- **Result**: `factual` requests fall through to the `fast` model.
- **Recommendation**: Explicitly handle all 8 intents defined in the system prompt in `handler.lua`.

---

## 6. Test Cases for Verification

1.  **Reflex Validation**: Send a 5,000-character prompt. Verify logs show `Reflex: Long input -> Long Context Model` without calling the LLM classifier.
2.  **Privacy Trigger**: Include `test@example.com` in a prompt. Verify routing to the `secure` model alias.
3.  **Recursive Test**: Set the `router-model` to `auto`. Verify the system does not crash or loop (recursion protection check).
4.  **Multimodal Switch**: Call `/v1/images/generations` with `model: auto`. Verify the `image_gen` model is selected and `operation: images_generations` is set in metadata.

---

## 7. QA Verdict

**Production Readiness: HIGH (with minor caveats)**

The implementation is architecturally sound. The separation of concerns between Go (heavy lifting/caching) and Lua (customizable logic) is excellent. The recursion protection is properly implemented.

**Key Decision Inputs for Next Steps:**
- The system prompt is the "Brain" of the router. Tuning it in `sdk/api/handlers/handlers.go` will be your primary way to improve routing accuracy.
- `handler.lua` is your configuration layer. You can add "Reflex Tier" rules here without modifying the Go core.
