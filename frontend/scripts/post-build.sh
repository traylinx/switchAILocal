#!/bin/bash

# Ensure output directory exists
mkdir -p ../static

# Rename index.html to management.html in the output directory
if [ -f "../static/index.html" ]; then
    mv "../static/index.html" "../static/management.html"
    echo "✓ Successfully renamed index.html to management.html"
else
    echo "✗ Error: index.html not found in ../static/"
    exit 1
fi

# Check file size
FILE_SIZE=$(wc -c <"../static/management.html")
MAX_SIZE=2097152 # 2MB

if [ $FILE_SIZE -gt $MAX_SIZE ]; then
    echo "⚠️ Warning: management.html is larger than 2MB ($((FILE_SIZE/1024)) KB)"
else
    echo "✓ File size is within limits ($((FILE_SIZE/1024)) KB)"
fi

echo "🚀 Build complete! management.html is ready in static/ directory."
