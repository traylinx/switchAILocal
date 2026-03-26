#!/bin/bash
# lint-arch.sh — Validates architecture rules
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./lint-arch.sh
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$PROJECT_ROOT"

echo "Running architecture lint checks..." >&2

ISSUES=()
PASS=0
FAIL=0

# Rule 1: No direct imports from internal/ to plugins/
echo "  [1/4] Checking internal/ doesn't import plugins/..." >&2
if grep -r '"github.com/traylinx/switchAILocal/plugins' internal/ 2>/dev/null | grep -v '_test.go' | head -5; then
    ISSUES+=("internal/ imports plugins/ directly")
    ((FAIL++))
else
    ((PASS++))
fi

# Rule 2: SDK handlers don't import internal/plugin
echo "  [2/4] Checking sdk/ doesn't import internal/plugin..." >&2
if grep -r '"github.com/traylinx/switchAILocal/internal/plugin' sdk/ 2>/dev/null | head -5; then
    ISSUES+=("sdk/ imports internal/plugin directly")
    ((FAIL++))
else
    ((PASS++))
fi

# Rule 3: Check for TODO/FIXME counts
echo "  [3/4] Counting TODO/FIXME markers..." >&2
TODO_COUNT=$(grep -r 'TODO\|FIXME\|HACK\|XXX' --include='*.go' . 2>/dev/null | wc -l | tr -d ' ')

# Rule 4: golangci-lint
echo "  [4/4] Running golangci-lint..." >&2
LINT_ISSUES=0
if command -v golangci-lint &>/dev/null; then
    LINT_ISSUES=$(golangci-lint run ./... 2>&1 | grep -c '^' || true)
fi

# Build JSON
ISSUES_JSON=$(printf '%s\n' "${ISSUES[@]}" | sed 's/.*/"&"/' | paste -sd, - 2>/dev/null || echo "")
[[ -z "$ISSUES_JSON" ]] && ISSUES_JSON=""

echo "{\"status\":\"$([ $FAIL -eq 0 ] && echo 'ok' || echo 'warning')\",\"passed\":$PASS,\"failed\":$FAIL,\"todo_count\":$TODO_COUNT,\"lint_issues\":$LINT_ISSUES,\"issues\":[$ISSUES_JSON]}"

echo "Architecture lint complete." >&2
