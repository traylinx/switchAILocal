#!/bin/bash
# validate-spec.sh — Validates OpenAPI spec files
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./validate-spec.sh <spec-file> [--diff]
set -e

SPEC_FILE="$1"
PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"

if [[ -z "$SPEC_FILE" ]]; then
    # Default: look for spec in standard locations
    for candidate in "$PROJECT_ROOT/.specs/multimodal/openapi.with-code-samples.yml" \
                     "$PROJECT_ROOT/docs/openapi.yml" \
                     "$PROJECT_ROOT/openapi.yml"; do
        if [[ -f "$candidate" ]]; then
            SPEC_FILE="$candidate"
            break
        fi
    done
fi

if [[ -z "$SPEC_FILE" || ! -f "$SPEC_FILE" ]]; then
    echo '{"status":"error","message":"No OpenAPI spec file found. Provide path as argument."}'
    exit 1
fi

echo "Validating: $SPEC_FILE..." >&2

# Basic YAML validation
LINES=$(wc -l < "$SPEC_FILE" | tr -d ' ')
PATHS=$(grep -c '^\s\+/[a-z]' "$SPEC_FILE" 2>/dev/null || echo 0)
SCHEMAS=$(grep -c '^\s\+\w\+:$' "$SPEC_FILE" 2>/dev/null | head -1 || echo 0)

# Check for required OpenAPI fields  
HAS_INFO=$(grep -c '^info:' "$SPEC_FILE" || echo 0)
HAS_PATHS=$(grep -c '^paths:' "$SPEC_FILE" || echo 0)
HAS_OPENAPI=$(grep -c '^openapi:' "$SPEC_FILE" || echo 0)

ISSUES=()
[[ "$HAS_OPENAPI" == "0" ]] && ISSUES+=("Missing 'openapi:' version field")
[[ "$HAS_INFO" == "0" ]] && ISSUES+=("Missing 'info:' block")
[[ "$HAS_PATHS" == "0" ]] && ISSUES+=("Missing 'paths:' block")

ISSUES_JSON=$(printf '%s\n' "${ISSUES[@]}" | sed 's/.*/"&"/' | paste -sd, - 2>/dev/null || echo "")
[[ -z "$ISSUES_JSON" ]] && ISSUES_JSON=""

STATUS="ok"
[[ ${#ISSUES[@]} -gt 0 ]] && STATUS="warning"

echo "{\"status\":\"$STATUS\",\"file\":\"$SPEC_FILE\",\"lines\":$LINES,\"paths\":$PATHS,\"issues\":[$ISSUES_JSON]}"
echo "Validation complete." >&2
