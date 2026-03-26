#!/bin/bash
# analyze-deps.sh — Analyzes Go package dependencies
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./analyze-deps.sh [--cycles]
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$PROJECT_ROOT"

echo "Analyzing Go dependencies..." >&2

if [[ "$1" == "--cycles" ]]; then
    echo "Checking for circular imports..." >&2
    CYCLES=$(go vet ./... 2>&1 | grep "import cycle" || true)
    if [[ -z "$CYCLES" ]]; then
        echo '{"status":"ok","cycles":[],"message":"No circular imports detected"}' 
    else
        echo "{\"status\":\"warning\",\"cycles\":[\"$CYCLES\"],\"message\":\"Circular imports found\"}"
    fi
else
    echo "Mapping package dependency tree..." >&2
    PACKAGES=$(go list ./... 2>/dev/null | wc -l | tr -d ' ')
    DEPS=$(go list -m all 2>/dev/null | wc -l | tr -d ' ')
    
    # Get top-level internal packages
    INTERNAL=$(go list ./internal/... 2>/dev/null | sed 's|.*/internal/||' | head -20)
    SDK=$(go list ./sdk/... 2>/dev/null | sed 's|.*/sdk/||' | head -20)
    
    echo "{\"status\":\"ok\",\"total_packages\":$PACKAGES,\"total_dependencies\":$DEPS,\"internal_packages\":[$(echo "$INTERNAL" | sed 's/.*/"&"/' | paste -sd, -)],\"sdk_packages\":[$(echo "$SDK" | sed 's/.*/"&"/' | paste -sd, -)]}"
fi

echo "Done." >&2
