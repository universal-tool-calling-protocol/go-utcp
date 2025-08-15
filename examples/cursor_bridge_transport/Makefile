.PHONY: test test-unit test-integration test-e2e test-all test-coverage test-bench clean build

# Go parameters
GOCMD=GOWORK=off go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=cursor-bridge-server

# Test flags
TEST_FLAGS=-v -race
COVERAGE_FLAGS=-coverprofile=coverage.out -covermode=atomic

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) -v .

# Run all tests
test-all: test-unit test-integration test-e2e

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) $(TEST_FLAGS) -short ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) $(TEST_FLAGS) -tags=integration ./...

# Run end-to-end tests
test-e2e:
	@echo "Running end-to-end tests..."
	RUN_E2E_TESTS=true $(GOTEST) $(TEST_FLAGS) -tags=e2e ./...

# Run load tests
test-load:
	@echo "Running load tests..."
	RUN_LOAD_TESTS=true $(GOTEST) $(TEST_FLAGS) -tags=e2e -run TestE2ELoadTest ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) $(TEST_FLAGS) $(COVERAGE_FLAGS) ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmark tests
test-bench:
	@echo "Running benchmark tests..."
	$(GOTEST) -bench=. -benchmem -run=^# ./...

# Run tests with race detection
test-race:
	@echo "Running tests with race detection..."
	$(GOTEST) -race ./...

# Quick test (no race detection, parallel execution)
test-quick:
	@echo "Running quick tests..."
	$(GOTEST) -short -parallel 4 ./...

# Test a specific function/test
test-func:
	@echo "Running specific test: $(FUNC)"
	$(GOTEST) $(TEST_FLAGS) -run $(FUNC) ./...

# Clean build artifacts and test cache
clean:
	@echo "Cleaning..."
	$(GOCMD) clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	rm -rf test-results/

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

# Run linters
lint:
	@echo "Running linters..."
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Generate test report
test-report:
	@echo "Generating test report..."
	mkdir -p test-results
	$(GOTEST) $(TEST_FLAGS) -json ./... > test-results/test-output.json
	@echo "Test report saved to test-results/test-output.json"

# Run tests in CI mode
test-ci: deps
	@echo "Running tests in CI mode..."
	$(GOTEST) $(TEST_FLAGS) $(COVERAGE_FLAGS) -json ./... > test-results.json
	@echo "Tests completed"

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make test-all       - Run all tests"
	@echo "  make test-unit      - Run unit tests only"
	@echo "  make test-integration - Run integration tests"
	@echo "  make test-e2e       - Run end-to-end tests"
	@echo "  make test-load      - Run load tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-bench     - Run benchmark tests"
	@echo "  make test-race      - Run tests with race detection"
	@echo "  make test-quick     - Run quick tests (no race detection)"
	@echo "  make test-func FUNC=TestName - Run specific test"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make lint           - Run linters"
	@echo "  make fmt            - Format code"
	@echo "  make help           - Show this help"

# Default target
all: build test-all
