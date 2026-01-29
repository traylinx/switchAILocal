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
