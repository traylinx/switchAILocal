# Sprint: Strict OpenAI Image Compatibility Namespace

**Status:** Deployed locally on 2026-06-07  
**Owner:** Harvey  
**Started:** 2026-06-07  
**Risk posture:** Additive only — preserve existing `/v1/*` behavior

---

## Goal

Expose a strict OpenAI-compatible image generation/editing flow for clients like Open Design without breaking existing SwitchAI local apps that already consume the current `/v1/images/generations` response shape.

## Problem

SwitchAI local image generation works, but some upstream image providers return a gateway-native shape:

```json
{ "data": { "image_urls": ["https://..."] } }
```

Strict OpenAI Images API clients expect:

```json
{ "data": [{ "url": "https://..." }] }
```

Changing `/v1/images/generations` globally could break existing SwitchAI clients that already parse `data.image_urls[]`.

## Decision

Add a new strict OpenAI namespace and leave legacy endpoints untouched.

```text
Legacy / current behavior:     /v1/images/generations
Strict OpenAI-compatible path: /openai/v1/images/generations
Strict OpenAI-compatible edit: /openai/v1/images/edits
```

The new namespace also exposes common OpenAI-compatible routes for base-URL compatibility:

```text
/openai/v1/models
/openai/v1/chat/completions
/openai/v1/completions
/openai/v1/embeddings
```

## Implementation

Files changed:

```text
internal/api/server.go
sdk/api/handlers/openai/openai_handlers.go
sdk/api/handlers/openai/image_response_normalizer_test.go
docs/user/api-reference.md
SKILL.md
```

Behavior:

- `/v1/images/generations` remains unchanged.
- `/v1/images/edits` remains unchanged.
- `/openai/v1/images/generations` runs the same image backend, then normalizes known gateway response variants into OpenAI shape.
- Already-standard OpenAI responses are returned byte-for-byte unchanged.
- Unknown response shapes are returned unchanged instead of erroring.

Normalizer supports:

```text
data.image_urls[]     -> data[].url
data.image_url        -> data[].url
data.url              -> data[].url
data.base64           -> data[].b64_json
data.b64_json         -> data[].b64_json
data.revised_prompt   -> data[].revised_prompt
data.images[] strings -> data[].url
data.images[] objects -> data[].url or data[].b64_json
```

## Verification

Passed:

```bash
go test ./sdk/api/handlers/openai ./internal/runtime/executor ./internal/api
go test ./...
go build -o /tmp/switchAILocal-openai-image-compat ./cmd/server
```

Recheck on 2026-06-07:

```bash
git diff --check
go test ./sdk/api/handlers/openai ./internal/api
go test ./...
go build -o /tmp/switchAILocal-openai-image-compat ./cmd/server
```

Extra production-safety tests added:

- `/openai/v1/models` route is registered and authenticated through the same API-key path.
- `/openai/v1/images/generations` route is registered.
- `/openai/v1/images/edits` route is registered.
- Normalizer combines `url` + `b64_json` on one image instead of emitting duplicate image entries.
- Normalizer preserves `revised_prompt` on top-level and nested image responses.

Multi-model sprint review was run through Lope on the patch artifact with focus:

```text
production safety and backward compatibility for additive OpenAI image endpoint
```

Findings addressed:

- Duplicate-entry risk for responses carrying both `url` and `b64_json` fixed.
- `revised_prompt` preservation added.
- Route registration tests added.

Findings classified as acceptable:

- Re-marshalling only happens in the new `/openai/v1/*` strict namespace and only for known non-standard image payloads.
- Unknown payload shapes are returned unchanged.
- Legacy `/v1/images/*` handlers still call the same backend operation names and still skip normalization.

Deployment caveat:

- Sebastian approved deploying the unrelated uncommitted model metadata / multipart model rewriting changes too, if valid. They passed focused tests, full tests, `go vet`, and the deployed binary build.

## Deployment Note

Deployed via LaunchAgent:

```text
LaunchAgent: com.makakoo.switchailocal.local
Binary: /Users/sebastian/Projects/makakoo/agents/switchAILocal/switchAILocal
Config: /Users/sebastian/Projects/makakoo/agents/switchAILocal/config.yaml
PID after restart: 5669
Listening: :18080
Backup binary: switchAILocal.bak.pre-openai-image-compat-20260607-172813
```

Post-deploy smoke:

```text
/v1/models              -> 200
/openai/v1/models       -> 200
/openai/v1/images/generations without auth -> 401, route registered
/openai/v1/images/edits without auth       -> 401, route registered
/v1/images/generations without auth        -> 401, legacy route still registered
/v1/images/edits without auth              -> 401, legacy route still registered
```

Real image smoke:

```text
POST /openai/v1/images/generations
model=ail-image
status=200
response data type=list
data length=1
first item keys=[url]
```

Note:

- Restart emitted an old shutdown panic in `server.err.log` from `DailyLogsManager.startRotationRoutine` (`close of closed channel`). The new process stayed running, listened on `:18080`, and produced no fresh errors after smoke tests.

## Open Design Configuration After Deploy

Use this media-provider base URL for strict image compatibility:

```text
http://127.0.0.1:18080/openai/v1
```

Use this for normal BYOK chat, which already works:

```text
http://127.0.0.1:18080/v1
```
