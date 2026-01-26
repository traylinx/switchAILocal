#!/bin/bash
# Checkpoint 21 Verification Script
# Verifies Integration and Security for Superbrain Intelligence

set -e

echo "=========================================="
echo "Checkpoint 21: Integration and Security"
echo "=========================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track results
TESTS_PASSED=0
TESTS_FAILED=0

# Function to run a test and track results
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    echo -e "${YELLOW}Running: ${test_name}${NC}"
    if eval "$test_command" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ PASSED${NC}"
        ((TESTS_PASSED++))
    else
        echo -e "${RED}✗ FAILED${NC}"
        ((TESTS_FAILED++))
    fi
    echo ""
}

echo "1. Running all Superbrain unit tests..."
echo "=========================================="
run_test "Superbrain Core Tests" "go test ./internal/superbrain/... -count=1"

echo "2. Running Watcher (Hot-Reload) Tests..."
echo "=========================================="
run_test "Watcher Tests" "go test ./internal/watcher/... -count=1"

echo "3. Verifying Security Fail-Safe..."
echo "=========================================="
run_test "Security Fail-Safe Tests" "go test ./internal/superbrain/security/... -v -count=1"

echo "4. Verifying Hot-Reload Functionality..."
echo "=========================================="
run_test "Superbrain Hot-Reload Tests" "go test ./internal/watcher/... -v -run TestSuperbrain -count=1"

echo "5. Verifying Metrics Exposure..."
echo "=========================================="
run_test "Metrics Tests" "go test ./internal/superbrain/metrics/... -v -count=1"

echo "6. Verifying Audit Logging..."
echo "=========================================="
run_test "Audit Logger Tests" "go test ./internal/superbrain/audit/... -v -count=1"

echo "7. Verifying Integration Components..."
echo "=========================================="
run_test "Executor Integration Tests" "go test ./internal/superbrain/... -v -run TestSuperbrainExecutor -count=1"

echo "8. Verifying Response Enrichment..."
echo "=========================================="
run_test "Metadata Tests" "go test ./internal/superbrain/metadata/... -v -count=1"

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo -e "Tests Passed: ${GREEN}${TESTS_PASSED}${NC}"
echo -e "Tests Failed: ${RED}${TESTS_FAILED}${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All checkpoint requirements verified successfully!${NC}"
    echo ""
    echo "Checkpoint 21 Status: COMPLETE"
    echo ""
    echo "Key Verifications:"
    echo "  ✓ All unit tests pass"
    echo "  ✓ Security fail-safe prevents forbidden operations"
    echo "  ✓ Hot-reload works for Superbrain config"
    echo "  ✓ Metrics are properly exposed"
    echo "  ✓ Audit logging is functional"
    echo "  ✓ Integration components work correctly"
    echo ""
    exit 0
else
    echo -e "${RED}✗ Some tests failed. Please review the output above.${NC}"
    exit 1
fi
