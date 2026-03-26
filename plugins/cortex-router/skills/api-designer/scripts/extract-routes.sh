#!/bin/bash
# extract-routes.sh — Extracts registered API routes from server.go
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./extract-routes.sh [--list]
set -e

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
SERVER_FILE="$PROJECT_ROOT/internal/api/server.go"

echo "Extracting routes from $SERVER_FILE..." >&2

if [[ ! -f "$SERVER_FILE" ]]; then
    echo '{"status":"error","message":"server.go not found"}'
    exit 1
fi

if [[ "$1" == "--list" ]]; then
    # Simple list mode
    ROUTES=$(grep -oE 'v1\.(GET|POST|PUT|DELETE|PATCH)\("([^"]+)"' "$SERVER_FILE" | \
        sed 's/v1\.\(.*\)("\(.*\)"/\1 \/v1\2/' | sort)
    
    echo "Found routes:" >&2
    echo "$ROUTES" >&2
    
    # Output as JSON
    ROUTE_JSON=$(echo "$ROUTES" | awk '{print "{\"method\":\""$1"\",\"path\":\""$2"\"}"}' | paste -sd, -)
    echo "{\"status\":\"ok\",\"routes\":[$ROUTE_JSON]}"
else
    # Full analysis
    GET_COUNT=$(grep -c 'v1\.GET(' "$SERVER_FILE" 2>/dev/null || echo 0)
    POST_COUNT=$(grep -c 'v1\.POST(' "$SERVER_FILE" 2>/dev/null || echo 0)
    
    # Extract handler function names
    HANDLERS=$(grep -oE '\.(GET|POST|PUT|DELETE|PATCH)\("[^"]*",\s*\w+\.\w+' "$SERVER_FILE" | \
        sed 's/.*,\s*//' | sort -u | head -30)
    
    HANDLER_JSON=$(echo "$HANDLERS" | sed 's/.*/"&"/' | paste -sd, - 2>/dev/null || echo "")
    
    echo "{\"status\":\"ok\",\"get_routes\":$GET_COUNT,\"post_routes\":$POST_COUNT,\"handlers\":[$HANDLER_JSON]}"
fi

echo "Done." >&2
