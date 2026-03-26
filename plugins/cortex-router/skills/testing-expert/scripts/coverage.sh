#!/bin/bash
# coverage.sh — Analyzes Go test coverage
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./coverage.sh [package] [--gaps] [--list]
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
cd "$PROJECT_ROOT"

PACKAGE="${1:-./...}"

if [[ "$1" == "--list" ]]; then
    echo "Listing test files..." >&2
    TEST_FILES=$(find . -name '*_test.go' -not -path './vendor/*' | sort)
    COUNT=$(echo "$TEST_FILES" | wc -l | tr -d ' ')
    FILES_JSON=$(echo "$TEST_FILES" | sed 's|^\./||; s/.*/"&"/' | paste -sd, -)
    echo "{\"status\":\"ok\",\"test_files_count\":$COUNT,\"files\":[$FILES_JSON]}"
    exit 0
fi

if [[ "$1" == "--gaps" ]]; then
    echo "Analyzing coverage gaps..." >&2
    COVER_FILE="/tmp/switchai_coverage_$$.out"
    go test ./... -coverprofile="$COVER_FILE" -count=1 2>&1 | tail -5 >&2
    
    if [[ -f "$COVER_FILE" ]]; then
        # Find packages with <50% coverage
        LOW_COV=$(go tool cover -func="$COVER_FILE" 2>/dev/null | \
            grep -v 'total:' | \
            awk -F'\t+' '{gsub(/%/,"",$NF); if ($NF+0 < 50) print $1":"$2":"$NF}' | head -20)
        
        TOTAL=$(go tool cover -func="$COVER_FILE" | grep 'total:' | awk '{print $NF}' | tr -d '%')
        
        GAPS_JSON=""
        if [[ -n "$LOW_COV" ]]; then
            GAPS_JSON=$(echo "$LOW_COV" | awk -F: '{print "{\"file\":\""$1"\",\"func\":\""$2"\",\"coverage\":"$3"}"}' | paste -sd, -)
        fi
        
        rm -f "$COVER_FILE"
        echo "{\"status\":\"ok\",\"total_coverage\":$TOTAL,\"low_coverage_functions\":[$GAPS_JSON]}"
    else
        echo '{"status":"error","message":"Coverage profile generation failed"}'
    fi
    exit 0
fi

# Default: run coverage for specified package
echo "Running coverage for $PACKAGE..." >&2
COVER_FILE="/tmp/switchai_coverage_$$.out"
go test "$PACKAGE" -coverprofile="$COVER_FILE" -count=1 2>&1 | tail -5 >&2

if [[ -f "$COVER_FILE" ]]; then
    TOTAL=$(go tool cover -func="$COVER_FILE" | grep 'total:' | awk '{print $NF}' | tr -d '%')
    PKG_COUNT=$(go tool cover -func="$COVER_FILE" | grep -v 'total:' | awk '{print $1}' | sort -u | wc -l | tr -d ' ')
    FUNC_COUNT=$(go tool cover -func="$COVER_FILE" | grep -v 'total:' | wc -l | tr -d ' ')
    rm -f "$COVER_FILE"
    echo "{\"status\":\"ok\",\"package\":\"$PACKAGE\",\"total_coverage\":$TOTAL,\"packages_tested\":$PKG_COUNT,\"functions_tested\":$FUNC_COUNT}"
else
    echo '{"status":"error","message":"No test coverage generated"}'
fi

echo "Done." >&2
