#!/bin/bash
# benchmark.sh — Runs Go benchmarks and outputs JSON results
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./benchmark.sh [package] [--compare]
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$PROJECT_ROOT"

PACKAGE="${1:-./...}"

echo "Running benchmarks for $PACKAGE..." >&2

# Run benchmarks
BENCH_OUTPUT=$(go test "$PACKAGE" -bench=. -benchmem -run='^$' -count=1 2>&1 || true)

# Parse benchmark results
BENCH_RESULTS=$(echo "$BENCH_OUTPUT" | grep '^Benchmark' | head -20)

if [[ -z "$BENCH_RESULTS" ]]; then
    echo '{"status":"ok","benchmarks":[],"message":"No benchmarks found in package"}'
    exit 0
fi

# Convert to JSON
BENCH_JSON=$(echo "$BENCH_RESULTS" | awk '{
    name=$1
    ops=$2
    ns_op=$3
    # Extract allocs if present
    allocs="0"
    bytes="0"
    for(i=4;i<=NF;i++) {
        if($i ~ /B\/op/) bytes=$(i-1)
        if($i ~ /allocs\/op/) allocs=$(i-1)
    }
    printf "{\"name\":\"%s\",\"iterations\":%s,\"ns_per_op\":%s,\"bytes_per_op\":%s,\"allocs_per_op\":%s}\n", name, ops, ns_op, bytes, allocs
}' | paste -sd, -)

TOTAL=$(echo "$BENCH_RESULTS" | wc -l | tr -d ' ')

echo "{\"status\":\"ok\",\"package\":\"$PACKAGE\",\"total_benchmarks\":$TOTAL,\"benchmarks\":[$BENCH_JSON]}"
echo "Benchmarks complete." >&2
