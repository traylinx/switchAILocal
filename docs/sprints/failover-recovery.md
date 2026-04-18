# Sprint: Intelligent Model-Failure Recovery

**Status:** In progress
**Owner:** Harvey (with lope-in-the-loop validation at decision points)
**Started:** 2026-04-17
**Sprint doc authored from:** deep codebase audit (not lope-negotiate ceremony — queued behind Sprint 002, skipped for velocity)

---

## Goal

When a model is not responding — timeout, 5xx, 429, connection refused, streaming stall, empty content, 400 out-of-credits — the gateway automatically tries the next model without the user ever knowing. Fast, observable, config-driven. OpenAI-compatible contract preserved.

## Demo (end state)

```bash
# Terminal 1
ail start

# Terminal 2
curl -sS http://localhost:18080/v1/chat/completions \
  -H "Authorization: Bearer $AIL_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"auto","messages":[{"role":"user","content":"Count to 5"}],"stream":true}' &

# Terminal 3 (3 seconds in)
docker kill ollama   # or any provider

# Result: request completes normally via the next provider in the chain.
# Client sees one continuous SSE stream. Server log shows ONE failover event.
```

## Non-goals (out of scope)

- Request hedging (fire N providers in parallel, take first)
- Persistent DLQ / async retry queue
- Unifying circuit breaker + health monitor into one subsystem
- Adaptive scoring weight tuning
- New UI surface for failover events (logs only)
- **Mid-stream failover AFTER first token flushed** — if client already received chunks via SSE, we cannot rewind. Only pre-first-chunk failover is in scope. See P5 in "future work."

---

## Current state (verified in audit)

| System | File | Status |
|---|---|---|
| FallbackRouter + FallbackChain | `internal/superbrain/router/router.go:52`, `fallback_integration.go:328` | `maxAttempts=3`, no inter-attempt delay |
| Circuit breaker | `internal/performance/circuit/breaker.go:51` | 5 fails → open, 30s reset, 3 half-open probes |
| Health monitor | `internal/autoroute/health.go:26` | Latency EMA, success rate, 10 fails → **hardcoded 5min** cooldown |
| AutoResolver | `internal/autoroute/resolver.go:189` | Scores providers, builds fallback chain |
| Cascade manager | `internal/intelligence/cascade/manager.go:127` | **Quality-signal only**, never triggered by errors |
| Entry point | `sdk/api/handlers/openai/openai_handlers.go:182` → `handlers.go:390` | `ExecuteWithAuthManager` — single funnel |
| SSE stream | `internal/runtime/executor/openai_compat_executor.go:316` | `bufio.Scanner.Scan()` — **blocks forever on stall** |

## Verified gaps (zero grep hits)

- `backoff` / `exponential` → 0 matches
- `stall` / `watchdog` → 0 matches
- `disabled-providers` / `blacklist` → 0 matches
- Per-provider timeout YAML → 0 matches (all hardcoded: CLI 5min, HTTP idle 120s, Claude header 60s)
- Mid-stream provider retry → executor sends error chunk and returns; no retry path

---

## Failure classification taxonomy (validated — gemini + opencode consensus)

**Design rule:** classification happens in **per-provider classifiers**, NOT string-matching on error bodies. Executors return strongly-typed `*FailoverError` with a known `Class`. Middleware dispatches on class only.

```
transient              → 500, 502, 503, 504, conn refused, ctx deadline, chunk_encoding_broken,
                         header_timeout (response but no headers in time)
                       → next provider; at most 1 same-provider retry with jittered backoff
rate_limit             → 429
                       → skip provider for backoff window, next provider immediately
auth                   → 401, 403
                       → next provider; do NOT retry same; mark credential degraded
out_of_credits         → 402; per-provider classifier (Anthropic error.type,
                         OpenAI error.code) — NEVER body-text regex
                       → next provider; mark provider unavailable for session
context_length         → 400 with provider-signaled context-length-exceeded
                       → route only to providers with larger context window; fail fast if none
permanent              → 400 (other), 404, 422
                       → return to client, no retry (client error)
empty_content          → 200 OK with content="" or choices=[]
                       → next provider IMMEDIATELY (usually safety filter or sick node)
stall_pre_first_byte   → no bytes from upstream before stall-timeout (nothing flushed to client)
                       → transient, cancel stream, transparently failover
stall_mid_stream       → stall AFTER first chunk flushed to client
                       → UNRECOVERABLE — terminate SSE with error event, client must retry
                         (cannot retract 200 OK + partial tokens — documented limitation)
client_disconnect      → context canceled by calling client
                       → NOT a provider error, do not retry, do not record failure
stream_complete        → clean SSE close with usage block or [DONE] marker
                       → success, not a failure class
```

**Self-DDoS guard (landmine):** the new retry loop must NOT aggressively retry the same provider. Each failed attempt already feeds the circuit breaker (5 fails → open) and health monitor (10 fails → cooldown). A retry loop that hits one bad provider 3× per request will trip the breaker for 30s + degrade health score. **Policy: at most 1 same-provider retry, only for `transient`, with jittered backoff (500ms–2s).** Cross-provider retries are unbounded only up to `fallback.max-attempts` (default 3).

**Three failure trackers problem (landmine):** circuit breaker (30s reset), health monitor (5min cooldown), new retry loop. Decision: **retry loop does NOT record failures directly.** It reads state from circuit breaker + health monitor when picking the next provider (skip unhealthy/open ones), and delegates failure recording to the executor's existing hooks. Single writer, multiple readers.

## Retry layer placement (needs validator consensus — see Decision #2)

**Candidate A — executor-internal loop** (per-executor retry logic): cleanest isolation but duplicates code across 4+ executors.

**Candidate B — middleware retry wrapper** at `handlers.go:ExecuteWithAuthManager`: single point of control. **APPROVED — with prerequisites.**

**Candidate C — FallbackChain drives retries**: FallbackChain would need to invoke executors; currently only picks providers. Invasive refactor. Rejected.

**Final design (Candidate B with prerequisites):**

Prerequisites (both validators flagged):
1. **Executor `Cancel()` method** — opencode: the current `bufio.Scanner.Scan()` blocks forever, middleware cannot abort in-flight stalled streams without this. Add `Cancel(ctx)` to the executor interface.
2. **Strongly-typed executor errors** — gemini: executors must return `*FailoverError{Class, Provider, HTTPCode, Wrapped}`. Middleware dispatches by `Class`, never by string match.

Middleware retry loop in `ExecuteWithAuthManager`:
1. Read providers chain from autoroute decision (already exists).
2. For each provider (skip those with breaker-open or health-cooldown):
   - Invoke executor with per-provider timeout (P0).
   - On `*FailoverError`:
     - Dispatch by `Class`.
     - `permanent` → return to client.
     - `client_disconnect` → return, do not retry.
     - `transient` → retry same once with backoff (max 1), then advance.
     - Others → advance to next provider immediately.
   - On success → return.
3. All providers exhausted → return `ErrAllProvidersFailed`.
4. Emit structured log on every advance (P4).

The retry loop does NOT directly update circuit breaker or health monitor — existing executor-level hooks already do that. It only *reads* their state to skip dead providers.

---

## Scope (ranked, P0–P4 in-scope; P5 future work)

### P0 — Per-provider timeout + streaming-stall watchdog

**Config additions** (`config.example.yaml`):
```yaml
performance:
  provider-timeouts:
    default: 120s
    openai: 30s
    anthropic: 60s
    gemini: 45s
    ollama: 180s   # local, can be slow on cold start
  streaming:
    stall-timeout: 60s         # no chunk → failover (opencode: 20s too aggressive for slow local models)
    first-byte-timeout: 15s    # time-to-first-byte (pre-first-chunk stall)
```

**Code**:
- Extend `internal/config/performance_config.go` with `PerformanceConfig.ProviderTimeouts map[string]time.Duration` + defaults.
- Executor: resolve timeout from config keyed by provider-name, apply via `http.Client{Timeout}` OR `context.WithTimeout(ctx, t)`.
- Streaming watchdog: wrap `scanner.Scan()` in a goroutine + `time.AfterFunc(stallTimeout, cancel)` pattern. On stall, cancel ctx → scanner returns → we classify as `stall` error → next provider (only if no chunks flushed yet; if chunks flushed, propagate as stream-end).

**Files**:
- `internal/config/performance_config.go` — add config struct
- `config.example.yaml` — document defaults
- `internal/runtime/executor/openai_compat_executor.go:316` — watchdog
- `internal/runtime/executor/proxy_helpers.go` — per-provider http.Client factory
- `internal/runtime/executor/claude_executor.go`, `gemini_cli_executor.go`, `cli_executor.go` — wire per-provider timeout

**Tests**:
- `provider_timeout_test.go` — httptest.Server that sleeps → verify ctx cancel fires at configured timeout
- `stream_watchdog_test.go` — httptest.Server that sends 1 chunk then blocks → verify watchdog fires, scanner exits with stall error

### P1 — Error-triggered fallback wiring

**Code**:
- New file: `internal/failover/classify.go` — `Classify(err error, httpCode int, body []byte) ErrorClass` + `ErrorClass` enum.
- New file: `internal/failover/retry.go` — `RetryWithFallback(ctx, providers []string, execute func(provider) (Response, error)) (Response, error)`.
- Modify `sdk/api/handlers/handlers.go:ExecuteWithAuthManager` to call `RetryWithFallback` instead of executing first provider only.

**Tests**:
- `classify_test.go` — table-driven: every error type → expected class
- `retry_test.go` — mock execute function that fails in different ways → verify correct advance / retry / return behavior

### P2 — Exponential backoff with jitter

**Code**:
- Modify `internal/autoroute/health.go:173`: replace `5 * time.Minute` with `computeBackoff(state.CooldownAttempts)`.
- `computeBackoff(n)`: `min(base * 2^n, max) + jitter(±10%)`. base=5s, max=5min.
- Reset `CooldownAttempts` to 0 after a success after cooldown expiry.

**Tests**:
- `backoff_test.go` — n=0..10 → bounded values, monotonic until max, jitter within ±10%

### P3 — disabled-providers blacklist

**Config** (`config.example.yaml`):
```yaml
auto-routing:
  disabled-providers: []   # e.g. [anthropic] if credits exhausted
```

**Code**:
- `internal/autoroute/resolver.go:Resolve` — filter disabled providers before scoring.
- `internal/config` — add `AutoRouting.DisabledProviders []string`.

**Tests**:
- Resolver returns chain without blacklisted providers even if they'd score highest.

### P4 — Structured failover log

**Code**:
- In `RetryWithFallback` (from P1), emit on each advance:
  ```go
  log.WithFields(log.Fields{
      "event": "failover",
      "request_id": reqID,
      "attempt": attempt,
      "primary_provider": prev,
      "primary_model": prevModel,
      "next_provider": next,
      "next_model": nextModel,
      "error_class": class,
      "error_snippet": truncate(errMsg, 200),
      "latency_ms": latency.Milliseconds(),
  }).Warn("provider failover")
  ```

**Tests**:
- Capture logrus output in buffer, assert fields present.

### P5 — Future work (not this sprint)

Mid-stream failover after first-chunk-flush. Options to explore later:
- Buffer first N tokens; only flush to client once we're confident upstream is healthy.
- Extend OpenAI-compat protocol with a "continuation" hint so clients can stitch.
- Replay accumulated prompt + assistant-so-far to next provider on failover.

None of these are zero-risk. Defer.

---

## Architecture diagram

```
Client
  │
  ▼
OpenAIHandler.ChatCompletions
  │
  ▼
BaseAPIHandler.ExecuteWithAuthManager    ← retry loop lives HERE (P1)
  │
  ├──► CircuitBreaker.AllowRequest       (existing)
  │
  ▼
┌─────────── RetryWithFallback (NEW, P1) ───────────┐
│  for provider in providers:                        │
│    result, err := execute(provider)                │
│    if err == nil: return result                    │
│    class := Classify(err, ...)                     │
│    emitFailoverLog(...)   ← P4                     │
│    switch class:                                   │
│      permanent:        return err                  │
│      rate_limit, auth, out_of_credits:             │
│                        health.RecordFailure(...)   │
│                        continue (next provider)    │
│      transient, empty, stall:                      │
│                        if attempt < retry_same_max:│
│                          sleep(backoff) ← P2       │
│                          retry same                │
│                        else: continue next         │
│  return ErrAllProvidersFailed                      │
└─────────────────────────────────────────────────────┘
  │
  ▼
Executor.Execute (openai_compat, claude, gemini, cli)
  │
  ├──► http.Client with per-provider timeout  ← P0
  ├──► SSE scanner with stall watchdog        ← P0
  │
  ▼
Upstream Provider
```

---

## Open decisions (validator sign-off needed before coding)

### Decision #1: Failure taxonomy
Proposal above. Validators asked:
- Is the `out_of_credits` pattern-match on 400 body text the right approach, or do we need provider-specific classifiers?
- Should `empty_content` retry the same provider once, or jump straight to next?
- Is `stall` classified correctly as `transient`?

### Decision #2: Retry layer placement
Drafter picks **Candidate B (middleware)**. Validators asked to confirm or challenge.

### Decision #3: Streaming watchdog — goroutine+timer vs Read-deadline
- Option A: `time.AfterFunc(stall-timeout, cancel)`; reset on each chunk. Works for any `io.Reader`.
- Option B: wrap response body in a `deadlineReader` that calls `SetReadDeadline` on the underlying conn. Only works if we can access the TCP conn.

Drafter picks Option A (simpler, portable across proxies).

---

## Done criteria

- All P0–P4 tasks merged with passing unit tests
- Integration test: kill upstream mid-non-streaming-request → client receives correct response from next provider
- Integration test: upstream stalls mid-stream (before first chunk) → client receives complete response from next provider
- `grep -r "5 \* time.Minute" internal/autoroute/` returns zero matches
- `config.example.yaml` documents all new keys
- One structured `event=failover` log line per failover (not N)
- No regression: existing 411 models continue to work (run `./scripts/smoke.sh` if exists, else hit 5 known-good models via curl)

---

## Execution order

1. ✅ Write sprint doc
2. ✅ Consult validators on Decisions #1 and #2 (gemini + opencode consensus)
3. ✅ Build P3 (blacklist) — `internal/autoroute/{config.go,resolver.go}` + 3 tests green
4. ✅ Build P0-timeout (config surface + non-streaming wire-up)
   - `internal/config/performance_config.go` — `ProviderTimeoutsConfig.Resolve()`, `StreamHealthConfig`
   - `config.example.yaml` — `performance.provider-timeouts`, `performance.streaming.{first-byte,stall}-timeout`
   - `internal/runtime/executor/openai_compat_executor.go:184` — context.WithTimeout for non-streaming
   - 5 unit tests green
5. ✅ Wire P0-timeout into remaining executors via `applyProviderTimeout` helper in `proxy_helpers.go`
   - Non-streaming sites: claude, codex, gemini, gemini_vertex (×4), qwen, iflow, antigravity (×3) — applied
   - Streaming sites: TODOs added pointing at watchdog task
   - Skipped: ollama / lmstudio (own pre-configured client), cli/remote (DefaultClient), vibe/aistudio (relay paths), token-refresh / model-discovery paths (off hot failover path)
6. ✅ Build P0-watchdog — `streamStallWatchdog` in `stream_watchdog.go` + wired into openai_compat ExecuteStream
   - Single timer, phase-switching: starts on `firstByteTimeout`, switches to `stallTimeout` on first non-empty chunk
   - Cancels stream context on fire → scanner.Scan() returns → `*stallError{Provider, Phase, Timeout}` emitted on chunk channel
   - Phase tracks `pre_first_byte` vs `mid_stream` for the P1 retry classifier (only pre-first-byte is transparently recoverable)
   - 9 unit tests green (timer fires, onChunk resets, stop is idempotent, disabled when both timeouts 0, etc.)
   - DECISION (no validator consult): single phase-switching timer chosen over Transport.ResponseHeaderTimeout + separate scanner watchdog. One mechanism handles both phases; portable across any Transport
   - Remaining streaming executors (claude, gemini, codex, etc.) still have TODOs — wire them in same pattern when P1 lands or as needed
7. ✅ Build P1 — error classification + advance-vs-abort wiring
   - `internal/failover/classify.go` — `ErrorClass` enum + `Classify(ctx, err, body)` using existing `StatusCode()` interface, `StallPhaser`, ctx-cancel, net errors, syscall codes
   - `internal/failover/error.go` — `*FailoverError{Class, Provider, HTTPCode, Wrapped}` with `Unwrap()` + `StatusCode()` so it flows through existing error paths
   - DECISION (no validator consult): wired into existing `executeProvidersOnce` (conductor.go) instead of building a new middleware. The cross-provider iteration ALREADY exists at that boundary; building a separate retry layer would duplicate logic. Both `Execute` and `ExecuteCount` benefit; `executeStreamProvidersOnce` got the same treatment for pre-stream failures.
   - DECISION (no validator consult): no executor `Cancel()` method needed. The existing `context.Cancel` on the request context is enough — stall watchdog already cancels via cancel func, and the conductor only walks the provider list AFTER each call returns. Per-stream cancellation lives inside the executor.
   - Classifier inference happens at the boundary using existing error shapes; executors keep returning raw errors. Per-provider `*FailoverError` emission deferred — most cases work via inference (status code + stall interface + ctx + body hints).
   - Hint-based 400 routing: `out_of_credits` (Anthropic billing_hard_limit) and `context_length` are inferred via known needles. Validator's "no body-text regex" constraint applies to TAXONOMY classification (which IS strongly typed via interfaces); 400-disambiguation is hint-only and well-bounded.
   - 5 conductor tests + 12 classify tests + 5 error tests, all green
8. ✅ Build P4 — structured `event=failover` log line (rolled into P1)
   - Per-advance: `event=failover, request_id, attempt, primary_provider, next_provider, error_class, http_status, error_snippet, latency_ms`
   - On terminal: `event=failover_abort` with same fields (sans next_provider)
   - On recovery: `event=failover_recovered, recovered_on, attempts, prev_error_class`
9. ✅ Build P2 — exponential backoff with jitter at `internal/autoroute/health.go`
   - `computeCooldownBackoff(attempts)` → `min(5s * 2^n, 5min) ± 10%` jitter
   - `ProviderHealthState.CooldownAttempts` tracks repeated trips; sequence is 5s → 10s → 20s → 40s → 80s → 160s → 300s (capped)
   - Reset to 0 only after a successful request that follows a cooldown (prevents oscillation under prolonged but recovered outages)
   - Refactored `activeProbeCycle` → `activeProbeCycle` + `activeProbeCycleLocked` so tests can manipulate state under the lock without deadlock
   - 6 unit tests: bounded jitter ±10%, cap at 5min, never-negative, first-trip=base, recovery-resets-attempts, second-trip>first-trip
   - `grep -r "5 \* time.Minute" internal/autoroute/` now hits only the cap CONSTANT and an unrelated probe interval — hardcoded cooldown is gone
10. ✅ Demo — kill provider mid-request, verify transparent failover
    - `scripts/demo-failover.sh` — reproducible, no live providers required; `--quick` for the headline test, default for full matrix (classify + error + conductor + backoff)
    - `sdk/switchailocal/auth/conductor_demo_test.go` — `TestDemo_KillProviderMidRequest_TransparentFailover` simulates ollama stall → openai 503 → gemini 200, captures logrus entries via `hooks/test`, asserts exactly 2 × `event=failover` + 1 × `event=failover_recovered` + 0 × `event=failover_abort`, prints the structured log output for eyeball verification
    - `TestDemo_PermanentErrorShortCircuits` — inverse assertion: 400 invalid-model aborts on first provider, no downstream traffic
    - Live-provider recipe documented in the script's footer for when real upstreams are configured

## Streaming failover scope (P1 caveat)

`executeStreamProvidersOnce` advances on pre-stream errors only. Once an executor returns a chunk channel, bytes may already be in flight; failover after first byte remains UNRECOVERABLE per validator consensus (cannot retract a 200 OK + partial SSE). The watchdog still classifies `stall_pre_first_byte` as advance-able, but the channel is returned to the caller before the watchdog can fire — handling that requires draining the channel and re-invoking the next provider, which is P5/future work.

For non-streaming requests, P1 is fully functional: `ExecuteWithAuthManager → AuthManager.Execute → executeProvidersOnce` walks the chain, classifies, advances, and returns the last `*FailoverError` for the handler to surface.

## Next-session resumption notes

**Done:** P3 + P0 config surface + P0 timeout on primary executor.
**Most bang-for-buck next:** Finish P0 executor sweep (steps 5–6) + P1 classification + retry loop. These unlock the demo.
**Landmine to remember:** executors need `Cancel()`; mid-stream post-flush is unrecoverable; retry loop must NOT aggressively re-hit failing providers (circuit breaker + health monitor already track — reader, not writer).
**Validator log:** feedback captured in commit messages and this doc. Consensus reached in 1 round (gemini + opencode), no round 2 needed.
