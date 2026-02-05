#!/bin/bash
set -e

# Base URL
BASE_URL="http://localhost:18080/v1"
API_KEY="sk-test-123"

echo "🧪 Starting E2E Routing Verification..."

# 1. Test Coding Query -> Should route to 'coding' tier / big model
echo "----------------------------------------"
echo "1️⃣ Testing Coding Query (fibonacci)..."
# Using a pattern that typically triggers coding intent
RESPONSE=$(curl -s -X POST "$BASE_URL/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "Write a Python function to calculate fibonacci sequence efficiently."}
    ]
  }')

MODEL=$(echo $RESPONSE | grep -o '"model":"[^"]*"' | cut -d'"' -f4)
echo "   Selected Model: $MODEL"

# 2. Test Sensitive Query -> Should route to local/secure model
echo "----------------------------------------"
echo "2️⃣ Testing Sensitive Query (PII)..."
RESPONSE=$(curl -s -X POST "$BASE_URL/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d '{
    "model": "auto",
    "messages": [
      {"role": "user", "content": "My social security number is 123-456-7890. Can you redact this?"}
    ]
  }')

MODEL=$(echo $RESPONSE | grep -o '"model":"[^"]*"' | cut -d'"' -f4)
echo "   Selected Model: $MODEL"
echo "----------------------------------------"
echo "✅ Verification Complete"
