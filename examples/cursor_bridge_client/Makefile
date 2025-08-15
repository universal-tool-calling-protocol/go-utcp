.PHONY: test test-unit test-coverage test-bench clean build

# Go parameters
GOCMD=GOWORK=off go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=cursor-utcp

# Test flags
TEST_FLAGS=-v -race
COVERAGE_FLAGS=-coverprofile=coverage.out -covermode=atomic

# Build the binary
build:
	$(GOBUILD) -o $(BINARY_NAME) -v .

# Run unit tests
test: test-unit

test-unit:
	@echo "Running unit tests..."
	$(GOTEST) $(TEST_FLAGS) ./...

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

# Clean build artifacts and test cache
clean:
	@echo "Cleaning..."
	$(GOCMD) clean
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Help
help:
	@echo "Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make test           - Run unit tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make test-bench     - Run benchmark tests"
	@echo "  make test-race      - Run tests with race detection"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make fmt            - Format code"
	@echo "  make vet            - Run go vet"
	@echo "  make help           - Show this help"

# Default target
all: build test
