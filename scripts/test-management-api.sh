#!/bin/bash

# Configuration
BASE_URL="http://localhost:18080/v0/management"
AUTH_HEADER="X-Management-Key: test-key"

echo "Testing Intelligent Systems Management Endpoints..."
echo "=================================================="

# Function to test an endpoint
test_endpoint() {
    local name=$1
    local endpoint=$2
    
    echo -n "Testing $name ($endpoint)... "
    response=$(curl -s -o /dev/null -w "%{http_code}" -H "$AUTH_HEADER" "${BASE_URL}${endpoint}")
    
    if [ "$response" == "200" ]; then
        echo "✅ OK (200)"
        # Print first few chars of response for verification (optional)
        # curl -s -H "$AUTH_HEADER" "${BASE_URL}${endpoint}" | head -c 100
        # echo "..."
    else
        echo "❌ FAILED ($response)"
    fi
}

test_endpoint "Heartbeat Status" "/heartbeat/status"
test_endpoint "Memory Stats" "/memory/stats"
test_endpoint "Memory Analytics" "/analytics"
test_endpoint "Steering Rules" "/steering/rules"
test_endpoint "Hooks Status" "/hooks/status"
test_endpoint "Setup Status" "/setup-status"

echo "=================================================="
echo "Done."
