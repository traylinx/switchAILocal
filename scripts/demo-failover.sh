#!/usr/bin/env bash
# demo-failover.sh — reproducible demo for the failover-recovery sprint.
#
# Exercises the full pipeline without needing live providers or API keys:
#   1. Error classification (10 classes, 12 tests)
#   2. Error wrapper (*FailoverError, StatusCode passthrough, Unwrap chain)
#   3. Conductor cross-provider advance/abort logic
#   4. Structured observability (event=failover / _abort / _recovered)
#   5. Exponential backoff with jitter (5s → 10s → 20s → … → 300s cap)
#   6. End-to-end "kill provider mid-request" demo with log capture
#
# Usage:
#   scripts/demo-failover.sh          # full demo
#   scripts/demo-failover.sh --quick  # just the headline demo test

set -euo pipefail

cd "$(dirname "$0")/.."

quick=0
if [[ "${1:-}" == "--quick" ]]; then quick=1; fi

bold=$(tput bold 2>/dev/null || echo "")
dim=$(tput dim 2>/dev/null || echo "")
rst=$(tput sgr0 2>/dev/null || echo "")

section() { echo; echo "${bold}═══ $1 ═══${rst}"; }

section "1/3 — Headline demo: kill provider mid-request, transparent recovery"
go test ./sdk/switchailocal/auth/ -run TestDemo -v -count=1

if [[ $quick -eq 1 ]]; then
  echo
  echo "${bold}Quick demo complete.${rst} Run without --quick for the full matrix."
  exit 0
fi

section "2/3 — Full failover unit coverage (classify + error + conductor)"
go test ./internal/failover/... ./sdk/switchailocal/auth/ \
  -run 'Classify|FailoverError|AsFailoverError|ExecuteProvidersOnce' \
  -v -count=1

section "3/3 — Exponential backoff with jitter (5s → 300s cap)"
go test ./internal/autoroute/ \
  -run 'ComputeCooldownBackoff|RecordRequestOutcome_First|RecordRequestOutcome_Second|RecordRequestOutcome_Success' \
  -v -count=1

echo
echo "${bold}✓ Demo complete.${rst}"
echo "${dim}Key log events to look for above:${rst}"
echo "  event=failover              — one per advance, with request_id + error_class + next_provider"
echo "  event=failover_recovered    — the request that finally succeeded"
echo "  event=failover_abort        — terminal classes short-circuit (permanent / client_disconnect / mid_stream)"
echo
echo "${dim}To run against live providers once configured:${rst}"
echo "  ail start"
echo "  curl -sS http://localhost:18080/v1/chat/completions \\"
echo "    -H \"Authorization: Bearer \$AIL_API_KEY\" \\"
echo "    -d '{\"model\":\"auto\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}'"
echo "  # then grep the ail log for 'event=failover'"
