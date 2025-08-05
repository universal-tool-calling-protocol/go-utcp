#!/bin/bash
# Run tests for cursor-utcp-bridge transport

set -e

echo "========================================="
echo "Running Cursor UTCP Bridge Tests"
echo "========================================="

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to run a test suite
run_test_suite() {
    local name=$1
    local cmd=$2
    
    echo -e "\n${YELLOW}Running $name...${NC}"
    if GOWORK=off bash -c "$cmd"; then
        echo -e "${GREEN}✓ $name passed${NC}"
        return 0
    else
        echo -e "${RED}✗ $name failed${NC}"
        return 1
    fi
}

# Track failures
FAILED=0

# 1. Run unit tests
if ! run_test_suite "Unit Tests" "make test-unit"; then
    FAILED=$((FAILED + 1))
fi

# 2. Run integration tests (if available)
if ! run_test_suite "Integration Tests" "make test-integration"; then
    FAILED=$((FAILED + 1))
fi

# 3. Run race detection tests
if ! run_test_suite "Race Detection Tests" "make test-race"; then
    FAILED=$((FAILED + 1))
fi

# 4. Generate coverage report
echo -e "\n${YELLOW}Generating coverage report...${NC}"
make test-coverage

# 5. Run benchmarks (optional)
if [ "$RUN_BENCHMARKS" = "true" ]; then
    if ! run_test_suite "Benchmark Tests" "make test-bench"; then
        FAILED=$((FAILED + 1))
    fi
fi

# 6. Run E2E tests (optional)
if [ "$RUN_E2E_TESTS" = "true" ]; then
    if ! run_test_suite "End-to-End Tests" "make test-e2e"; then
        FAILED=$((FAILED + 1))
    fi
fi

# 7. Run load tests (optional)
if [ "$RUN_LOAD_TESTS" = "true" ]; then
    if ! run_test_suite "Load Tests" "make test-load"; then
        FAILED=$((FAILED + 1))
    fi
fi

# Summary
echo -e "\n========================================="
if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC}"
    echo "Coverage report: coverage.html"
else
    echo -e "${RED}$FAILED test suite(s) failed${NC}"
    exit 1
fi
echo "========================================="
