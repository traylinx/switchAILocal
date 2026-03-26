#!/bin/bash
# scaffold-route.sh — Scaffolds a new API endpoint handler
# stdout: JSON  |  stderr: human-readable progress
# Usage: ./scaffold-route.sh <endpoint-name>
set -e

NAME="$1"
if [[ -z "$NAME" ]]; then
    echo '{"status":"error","message":"Usage: scaffold-route.sh <endpoint-name>"}' 
    exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "$0")/../../../.." && pwd)"
HANDLER_DIR="$PROJECT_ROOT/sdk/api/handlers/openai"

# Convert name to Go conventions (e.g., "audio_translations" -> "AudioTranslations")
GO_FUNC=$(echo "$NAME" | sed -E 's/(^|_)([a-z])/\U\2/g')
ROUTE_PATH=$(echo "$NAME" | tr '_' '/')

echo "Scaffolding endpoint: $NAME -> $GO_FUNC..." >&2

HANDLER_FILE="$HANDLER_DIR/openai_handlers.go"

# Check if handler already exists
if grep -q "func.*$GO_FUNC" "$HANDLER_FILE" 2>/dev/null; then
    echo "{\"status\":\"exists\",\"function\":\"$GO_FUNC\",\"message\":\"Handler already exists\"}"
    exit 0
fi

# Output scaffold info
cat << EOF
{
  "status": "scaffold",
  "function": "$GO_FUNC",
  "route": "/v1/$ROUTE_PATH",
  "handler_file": "$HANDLER_FILE",
  "template": "func (h *OpenAIAPIHandler) $GO_FUNC(c *gin.Context) {\n\trawJSON, err := c.GetRawData()\n\tif err != nil {\n\t\th.WriteErrorResponse(c, \u0026interfaces.ErrorMessage{StatusCode: http.StatusBadRequest, Error: fmt.Errorf(\"invalid request: %v\", err)})\n\t\treturn\n\t}\n\t// TODO: implement $GO_FUNC\n}",
  "route_registration": "v1.POST(\"/v1/$ROUTE_PATH\", openaiHandlers.$GO_FUNC)"
}
EOF

echo "Scaffold generated. Paste the template into $HANDLER_FILE and register the route in server.go." >&2
