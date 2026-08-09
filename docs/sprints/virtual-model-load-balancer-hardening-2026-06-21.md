# Sprint: Virtual Model Load-Balancer Hardening

**Status:** Implemented locally and prepared for release — verification gates passed 2026-06-21
**Owner:** Harvey
**Branch:** `feature/virtual-model-weighted-load-balancer`
**Started:** 2026-06-21
**Risk posture:** Surgical. No endpoint changes, no config schema break. Follow-up ProjectWannolot template patches align pod config/docs with the released virtual-pool behavior.

---

## Goal

Make `ail-compound` and other virtual model aliases balance configured backend members predictably by weight while preserving the current OpenAI-compatible API contract used by local agents, Tytus pods, tmux bots, droplets, and apps.

Users and agents keep calling:

```text
model: "ail-compound"
```

The gateway decides which eligible backend member handles the request.

---

## Current verified state

### switchAILocal branch state

- `internal/config/config.go` validates virtual model members but now treats `provider` as a lower-case opaque backend ID.
- `sdk/api/handlers/handlers.go` routes virtual model aliases before legacy provider inference.
- `internal/virtualmodels/router.go` currently uses weighted round-robin by expanding member weights into a repeated list and advancing a cursor.
- State is keyed by lower-case `<public-model>|<request-class>`.
- Attempt counts are keyed by `<public-model>|<request-class>|<member-id>`.
- State persistence is best-effort JSON under the switchAILocal state directory.
- Focused tests currently pass:

```bash
go test ./internal/config ./internal/virtualmodels ./sdk/api/handlers
go test -race -count=1 ./internal/virtualmodels
```

### ProjectWannolot / droplet compatibility findings

Read-only audit of `/Users/sebastian/Projects/makakoo/api/ProjectWannolot` found:

- Droplets already run multiple switchAILocal instances: one instance per four pods.
- Each instance has its own config file, state directory, and port.
- Host nginx exposes one load-balanced gateway on port `18090` using `least_conn` across all switchAILocal instances.
- Each pod namespace exposes `:18080` and forwards to host nginx `:18090`.
- Agent containers receive:
  - `AIL_INFERENCE_URL=http://10.42.42.1:18080/v1`
  - `AIL_API_KEY=<pod key>`
- OpenClaw/NemoClaw default runtime model is:
  - primary: `openai/ail-compound`
  - fallback: `openai/ail-fast`
- In-pod docs explicitly say agents must use AIL aliases, not upstream provider APIs.
- The pod template now has `virtual-models.ail-compound` with MiniMax-M3 and Alibaba `glm-5.2` active members.
- The pod template has a hidden `auto` virtual pool that reuses the same `ail-compound` member anchor.
- MiniMax-M3 and Alibaba `glm-5.2` are both marked safe for `chat_multiturn_tools` / tool-history replay after live probes.
- DeepSeek is not active in the pool; `ail-fast` remains only a hidden backwards-compatibility virtual alias where templates keep it.

Implication: the load-balancer change must keep capability filtering strict. Adding a second text provider must not accidentally route OpenClaw/Hermes multi-turn tool traffic to a backend without `chat_multiturn_tools` plus `tool_history_replay` or `agentic_safe`.

---

## Non-goals

- No new public endpoint.
- No change to `/v1/*`, `/openai/v1/*`, `/v1/models`, `/v1/chat/completions`, `/v1/completions`, or `/v1/responses` shapes.
- No endpoint or schema changes in ProjectWannolot. Pod config-template updates are limited to the active `ail-compound` member metadata, thinking defaults, and agent-facing docs.
- No provider-health, fallback, retry, or circuit-breaker redesign.
- No exact cross-droplet or cross-machine subscription accounting.
- No Redis/Postgres/shared distributed state.
- No token/credit-aware balancing yet.
- No adaptive weight changes based on latency, errors, quota, or cost.
- No real provider names/API keys in tests.

---

## Design decision

Use **Smooth Weighted Round Robin (SWRR)** for each virtual model request class.

State key:

```text
<public-model>|<request-class>
```

This preserves existing capability separation. Text chat, tool chat, multi-turn tool chat, vision, audio, embeddings, etc. do not share scheduler state.

Member key:

```text
member.id
```

Provider strings remain lower-case opaque runtime routing values. Model strings remain opaque runtime routing values.

### Expected deterministic sequences from zero state

Tie break: normalized eligible member order by `member.id`, then `member.model`, matching existing deterministic ordering.

Equal weights:

```text
A=1, B=1
sequence: A, B, A, B, A, B
counts over 2N requests: A=N, B=N
```

Custom weights:

```text
A=3, B=2
sequence: A, B, A, B, A, A, B, A, B, A
counts over 5N requests: A=3N, B=2N
```

The exact 60/40 guarantee applies from zero state over complete five-request cycles. After restart with persisted accumulators or after config changes, proportionality must re-converge; arbitrary five-request windows are not guaranteed to be exactly 3/2.

### Weight semantics

- Missing weight or `0` means default weight `1`, matching current config normalization.
- Negative weight remains invalid config, matching current validation.
- Disabled members remain excluded.
- All-disabled exposed pool remains invalid config.
- Capability-ineligible members remain excluded at request time.

---

## Phase 0 — Baseline and compatibility characterization

**Goal:** Lock down current behavior before scheduler changes.

### Tasks

- Characterize current routing path:
  - virtual routing before legacy provider inference
  - capability filtering
  - fallback exclusion path
  - response model rewriting
- Characterize ProjectWannolot constraints in test names/comments:
  - one public alias
  - no endpoint changes
  - strict capability filtering for agentic tool replay
  - hidden `auto` pool may reuse the same members
- Record current state format behavior:
  - existing branch JSON: `cursors`, `counts`
  - malformed JSON ignored
  - write failures do not break routing

### Acceptance gates

```bash
go test ./internal/config ./internal/virtualmodels ./sdk/api/handlers
```

No production config or endpoint changes.

---

## Phase 1 — Replace bursty expanded-list RR with SWRR

**Goal:** Replace the current expanded-list cursor with smooth weighted selection.

### Tasks

- Implement SWRR in `internal/virtualmodels/router.go`.
- Keep public constructor behavior unchanged:
  - `NewRouter()`
  - `NewRouterWithStatePath(path string)`
- Keep state key per routing bucket: `<public-model>|<request-class>` plus qualifiers when media/tool-history/context changes eligible members.
- Keep route output unchanged:
  - `Provider`
  - `NativeModel`
  - `MemberID`
  - `Requirements`
- Preserve best-effort routing if persistence fails.
- Ensure fallback `SelectExcluding` recomputes eligible members and does not mutate excluded member state unexpectedly.

### Tests

- Equal weights produce `A,B,A,B`.
- `3/2` weights produce `A,B,A,B,A` from zero state.
- `3/2` counts are exact over 5, 10, 50 requests from zero state.
- Missing/zero weight behaves as weight `1`.
- Negative weight rejected by config validation, not silently scheduled.
- Disabled member receives zero selections.
- Capability-ineligible member receives zero selections for that request class.
- `chat_text` and `chat_multiturn_tools` have independent state.
- A text-only member is never selected for a multi-turn tool-history request.
- A media member without replay safety is never selected for media + tool-history requests.
- Split image-only/audio-only members are never selected for mixed image+audio requests.

### Acceptance gates

```bash
go test ./internal/config ./internal/virtualmodels
go test -race -count=1 ./internal/virtualmodels
```

---

## Phase 2 — Versioned local state with config-hash reset

**Goal:** Make persisted local scheduler state safe across restarts and safe across config changes, without pretending it is distributed budget coordination.

### State shape

New state should be versioned. Example:

```json
{
  "schema_version": 2,
  "algorithm": "smooth-weighted-round-robin/v1",
  "pools": {
    "ail-compound|chat_text": {
      "config_hash": "sha256:...",
      "current": {
        "member-a": 0,
        "member-b": 0
      },
      "counts": {
        "member-a": 12,
        "member-b": 8
      },
      "last_selected": "member-a"
    }
  }
}
```

### Tasks

- Compute `config_hash` from canonically ordered routing inputs only:
  - public alias
  - request class
  - enabled eligible member IDs
  - normalized member order
  - effective weight
  - capability fields used by router eligibility
- Exclude secrets and unrelated provider config:
  - API keys
  - headers
  - base URLs
  - comments
  - timeouts
  - global config
- Reset a pool/class state if:
  - schema version changes
  - algorithm changes
  - config hash changes
  - member set/order/weight/capabilities change
- Handle old branch state:
  - old `{cursors, counts}` JSON must not crash startup
  - old scheduling cursor can be discarded
  - old counts may be imported only if member keys still match; otherwise discard
- Keep atomic temp-file write via existing secure write helper.
- Keep guarantee narrow:
  - one switchAILocal process per state directory is the supported production path
  - no cross-instance exact accounting
  - no cross-droplet exact accounting

### Tests

- Unchanged config preserves SWRR current weights.
- Changed weight resets class state.
- Disabled/enabled change resets class state.
- Capability change resets class state.
- Old `{cursors, counts}` state does not panic.
- Malformed JSON does not break routing.
- Read-only/unwritable state path does not break routing.

### Acceptance gates

```bash
go test ./internal/virtualmodels
go test -race -count=1 ./internal/virtualmodels
```

---

## Phase 3 — HTTP-level fake-provider integration tests

**Goal:** Prove the existing HTTP route uses the scheduler without touching public endpoints.

### Tasks

- Add fake OpenAI-compatible providers in tests only.
- Configure a test virtual model alias with lower-case opaque provider IDs.
- Exercise:
  - `/v1/chat/completions`
  - `/v1/completions` if existing handler path supports virtual aliases there
  - `/v1/responses` if existing handler path supports virtual aliases there
- Assert destination counts at the fake upstreams.
- Assert disabled member receives zero requests.
- Assert capability-ineligible member receives zero requests.
- Assert `/v1/models` still exposes the public alias but not private backend aliases when configured private.

### Tests

- Equal weights over 10 HTTP requests: exact 5/5.
- `3/2` over 10 HTTP requests: exact 6/4.
- Multi-turn tool-history request only selects member with `chat_multiturn_tools` plus replay-safe capability.
- Media + tool-history requests only select media-capable replay-safe members.
- Mixed image+audio requests fail closed unless one member supports both modalities.
- Text-only request may select text-only members.
- Direct endpoint shapes and status codes remain unchanged.

### Acceptance gates

```bash
go test ./sdk/api/handlers ./sdk/api/handlers/openai ./internal/api
```

Do not add a new endpoint for this phase.

---

## Phase 4 — ProjectWannolot compatibility proof

**Goal:** Prove the switchAILocal change does not break the droplet topology already in use by many agents/apps.

### Read-only checks

Run from `/Users/sebastian/Projects/makakoo/api/ProjectWannolot`:

```bash
python3 -m pytest \
  services/wannolot-infrastructure/tests/test_pod_template_literacy.py \
  services/wannolot-infrastructure/tests/test_switchailocal_diagnostics.py
```

If Docker/Jinja dependencies are available, also validate templates without deploying:

```bash
cd services/wannolot-infrastructure
python3 scripts/validate_pod_autonomy_contracts.py
```

### Compatibility assertions

- `AIL_INFERENCE_URL` remains `http://10.42.42.1:18080/v1` for agents.
- Pod proxy remains `pod:18080 -> host nginx:18090`.
- nginx remains `least_conn` across switchAILocal instances.
- `openai/ail-compound` remains OpenClaw primary.
- `openai/ail-fast` remains OpenClaw fallback.
- `ail-compound` route selection remains capability-filtered.
- A text-only secondary provider does not receive multi-turn tool-history traffic.
- Local state remains per switchAILocal instance. This is acceptable because every instance runs the same configured weights; global exact accounting across instances/droplets is explicitly out of scope.

### Acceptance gates

- No edits under ProjectWannolot in this sprint.
- No endpoint/schema changes in switchAILocal.
- No changes to `config.pod.yaml` unless explicitly approved in a later rollout.

---

## Phase 5 — Minimal docs/example update

**Goal:** Document only what tests prove.

### Tasks

- Update `config.example.yaml` with a small virtual-model pool example:
  - equal weights
  - `3/2` weights
  - lower-case opaque provider IDs
  - capability separation example
- Update existing user/admin docs only if they already document virtual models.
- Do not document distributed quota balancing as supported.

### Acceptance gates

```bash
go test ./internal/config
git diff --check
```

---

## Rollout plan

1. Land code behind same existing config shape.
2. Local smoke:
   - `/v1/models`
   - `/v1/chat/completions` with `ail-compound`
   - `/v1/completions` with `ail-compound`
   - `/v1/responses` with `ail-compound`
3. Verify route logs show expected member sequence for simple `chat_text`.
4. Verify multi-turn tool-history request still selects MiniMax-only in current ProjectWannolot-style config.
5. Build binary.
6. Only after approval: canary on one local switchAILocal instance.
7. Only after canary proof: discuss ProjectWannolot droplet rollout separately.

---

## Implementation result — 2026-06-21

- Replaced expanded-list weighted round-robin with smooth weighted round-robin in `internal/virtualmodels/router.go`.
- Added versioned local router state:
  - `schema_version: 2`
  - `algorithm: smooth-weighted-round-robin/v1`
  - per-pool `config_hash`
  - per-member SWRR `current` accumulators
  - per-member route `counts`
- Config hash resets pool state when routing inputs change:
  - eligible member set
  - normalized member order
  - effective weights
  - capability fields used by eligibility
- Kept old `{cursors, counts}` state and malformed JSON non-fatal.
- Kept state writes best-effort. Unwritable state path does not break routing.
- Kept fallback `SelectExcluding` from mutating primary pool state.
- Added HTTP-level fake-provider tests for:
  - `/v1/chat/completions`
  - `/v1/completions`
  - `/v1/responses`
  - `/v1/models` public alias visibility
  - 3/2 HTTP route distribution
  - tool-history traffic restricted to replay-safe member
  - Responses image requests fail closed when only text members exist
  - media + tool-history traffic restricted to replay-safe media member
  - mixed image+audio requests fail closed unless one member supports both modalities.
- Added `chat_multimodal_understanding` routing bucket for mixed image+audio requests. Members can satisfy it explicitly with `chat_multimodal_understanding`/`multimodal` or by declaring both image and audio understanding operations plus both input modalities.
- Updated `config.example.yaml` with weighted virtual-pool notes and a commented 60/40 lower-case opaque-provider example.
- ProjectWannolot remained read-only.

Targeted verification passed:

```bash
go test ./internal/config ./internal/virtualmodels ./sdk/api/handlers ./sdk/api/handlers/openai ./internal/api
go test -race -count=1 ./internal/virtualmodels
cd /Users/sebastian/Projects/makakoo/api/ProjectWannolot && python3 -m pytest \
  services/wannolot-infrastructure/tests/test_pod_template_literacy.py \
  services/wannolot-infrastructure/tests/test_switchailocal_diagnostics.py
git diff --check
```

---

## Open questions for Sebastian

1. Is exact weekly provider spend balance across **all droplets** required now, or is per-instance proportional routing enough for this sprint?
2. Should `ail-compound` on droplets add Alibaba as a text-only member later, while keeping MiniMax as the only multi-turn tool replay member?
3. Should DeepSeek stay available as `ail-fast` fallback even if removed from `ail-compound`?

Recommended defaults:

- This sprint: per-instance proportional SWRR only.
- Droplet config later: add Alibaba text member to `ail-compound`, leave MiniMax as only agentic multi-turn member, keep DeepSeek as `ail-fast`.
- Distributed quota/budget accounting: future sprint, only if subscription burn data proves per-instance proportional routing is insufficient.

---

## Final verification bundle

Run from switchAILocal:

```bash
go test ./internal/config ./internal/virtualmodels ./sdk/api/handlers
go test -race -count=1 ./internal/virtualmodels
git diff --check
```

Run read-only from ProjectWannolot:

```bash
python3 -m pytest \
  services/wannolot-infrastructure/tests/test_pod_template_literacy.py \
  services/wannolot-infrastructure/tests/test_switchailocal_diagnostics.py
```
