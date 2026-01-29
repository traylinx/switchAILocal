#!/bin/bash
# switchAILocal Management UI Deployment Script

set -e

CDIR=$(pwd)
FRONTEND_DIR="$CDIR/frontend"
STATIC_DIR="$CDIR/static"

echo "Building Management UI..."
cd "$FRONTEND_DIR"
npm run build

echo "Deploying to static folder..."
echo "Deploying to static folder... (Managed by post-build.sh)"

echo "Success! Management UI updated at static/management.html"
echo "Opening Management UI..."

# Open in default browser (macOS)
if [[ "$OSTYPE" == "darwin"* ]]; then
    open "http://localhost:18080/management"
else
    echo "Please open http://localhost:18080/management in your browser."
fi
