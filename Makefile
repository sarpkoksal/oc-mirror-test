.PHONY: build test clean install help fmt vet lint bench profile quality all webui download download-oc download-opm download-oc-mirror download-all setup

# Variables
BINARY_NAME=oc-mirror-test
BIN_DIR=bin
CMD_DIR=cmd/oc-mirror-test
VERSION?=dev
BUILD_TIME=$(shell date +%Y-%m-%dT%H:%M:%S)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT) -s -w"
GOFLAGS=-trimpath
CGO_ENABLED?=0

# Build optimization flags
BUILD_FLAGS=-buildmode=pie -tags netgo,osusergo,static_build

# Client download variables
OCP_VERSION?=4.20
CLIENT_TOOLS?=oc,opm,oc-mirror

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
help:
	@echo 'Usage:'
	@echo '  make <target>'
	@echo ''
	@echo 'Targets:'
	@echo '  build       Build the optimized binary in $(BIN_DIR)/$(BINARY_NAME)'
	@echo '  build-fast  Build the binary with minimal optimizations (faster build)'
	@echo '  test        Run tests with race detection and coverage'
	@echo '  bench       Run benchmarks'
	@echo '  profile     Run benchmarks with CPU and memory profiling'
	@echo '  clean       Remove build artifacts'
	@echo '  install     Install dependencies'
	@echo '  fmt         Format code'
	@echo '  vet         Run go vet'
	@echo '  lint        Run golangci-lint (if installed)'
	@echo '  quality     Run all quality checks (fmt, vet, lint, test)'
	@echo '  webui       Build and run web UI server'
	@echo '  all         Run fmt, vet, test, and build'
	@echo ''
	@echo 'Client Download Targets:'
	@echo '  download          Download all client tools (oc, opm, oc-mirror)'
	@echo '  download-oc       Download oc client only'
	@echo '  download-opm      Download opm client only'
	@echo '  download-oc-mirror Download oc-mirror client only'
	@echo '  download-all      Download all tools (same as download)'
	@echo '  setup             Build binary and download required tools'
	@echo ''
	@echo 'Performance Targets:'
	@echo '  bench-cpu   Run CPU benchmarks'
	@echo '  bench-mem   Run memory benchmarks'
	@echo '  bench-all   Run all benchmarks'
	@echo '  pprof-cpu   Generate CPU profile'
	@echo '  pprof-mem   Generate memory profile'
	@echo ''
	@echo 'Variables:'
	@echo '  OCP_VERSION       OpenShift version for client downloads (default: 4.20)'
	@echo '  CLIENT_TOOLS      Comma-separated list of tools to download (default: oc,opm,oc-mirror)'

## build: Build the optimized binary
build:
	@echo "Building optimized $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=$(CGO_ENABLED) go build $(GOFLAGS) $(LDFLAGS) $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Binary built: $(BIN_DIR)/$(BINARY_NAME)"
	@ls -lh $(BIN_DIR)/$(BINARY_NAME)

## build-fast: Build with minimal optimizations (faster)
build-fast:
	@echo "Building $(BINARY_NAME) (fast mode)..."
	@mkdir -p $(BIN_DIR)
	@go build -o $(BIN_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Binary built: $(BIN_DIR)/$(BINARY_NAME)"

## test: Run tests with race detection and coverage
test:
	@echo "Running tests with race detection..."
	@go test -v -race -coverprofile=coverage.out -covermode=atomic -timeout=30m ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"
	@go tool cover -func=coverage.out | tail -1

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem -benchtime=3s ./...

## bench-cpu: Run CPU-intensive benchmarks
bench-cpu:
	@echo "Running CPU benchmarks..."
	@go test -bench=Benchmark.*CPU -benchmem -benchtime=5s ./...

## bench-mem: Run memory benchmarks
bench-mem:
	@echo "Running memory benchmarks..."
	@go test -bench=Benchmark.*Mem -benchmem -benchtime=5s ./...

## bench-all: Run all benchmarks with detailed output
bench-all:
	@echo "Running comprehensive benchmarks..."
	@go test -bench=. -benchmem -benchtime=5s -count=3 ./... | tee benchmark.txt
	@echo "Benchmark results saved to benchmark.txt"

## profile: Run benchmarks with profiling
profile: clean-profile
	@echo "Running benchmarks with profiling..."
	@go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof -benchtime=5s ./...
	@echo "Profiles generated: cpu.prof, mem.prof"
	@echo "View with: go tool pprof cpu.prof"

## pprof-cpu: Generate CPU profile
pprof-cpu:
	@echo "Generating CPU profile..."
	@go test -bench=. -cpuprofile=cpu.prof -benchtime=10s ./...
	@go tool pprof -http=:8080 cpu.prof &
	@echo "CPU profile available at http://localhost:8080"

## pprof-mem: Generate memory profile
pprof-mem:
	@echo "Generating memory profile..."
	@go test -bench=. -memprofile=mem.prof -benchtime=10s ./...
	@go tool pprof -http=:8081 mem.prof &
	@echo "Memory profile available at http://localhost:8081"

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out coverage.html benchmark.txt
	@go clean -cache -testcache
	@echo "Clean complete"

## clean-profile: Remove profiling files
clean-profile:
	@rm -f *.prof *.test

## install: Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@go mod verify
	@echo "Dependencies installed"

## fmt: Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@if command -v goimports >/dev/null 2>&1; then \
		echo "Running goimports..."; \
		goimports -w .; \
	fi
	@echo "Formatting complete"

## vet: Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete"

## lint: Run golangci-lint (if installed)
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "Running golangci-lint..."; \
		golangci-lint run --timeout=5m ./...; \
	else \
		echo "golangci-lint not installed. Skipping..."; \
		echo "Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## quality: Run all quality checks
quality: fmt vet lint test
	@echo "All quality checks passed!"

## security: Run security checks
security:
	@echo "Running security checks..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not installed. Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest"; \
	fi

## webui: Build and run web UI server
webui: build
	@echo "Starting web UI server..."
	@$(BIN_DIR)/$(BINARY_NAME) webui

## all: Run fmt, vet, test, and build
all: fmt vet test build

## release: Build release binary with all optimizations
release: clean
	@echo "Building release binary..."
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64 ./$(CMD_DIR)
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 ./$(CMD_DIR)
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 ./$(CMD_DIR)
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) $(BUILD_FLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-windows-amd64.exe ./$(CMD_DIR)
	@echo "Release binaries built in $(BIN_DIR)/"
	@ls -lh $(BIN_DIR)/

## size: Show binary size information
size: build
	@echo "Binary size information:"
	@ls -lh $(BIN_DIR)/$(BINARY_NAME)
	@echo ""
	@echo "Detailed size breakdown:"
	@go tool nm -size $(BIN_DIR)/$(BINARY_NAME) | head -20

## deps: Show dependency information
deps:
	@echo "Dependency tree:"
	@go mod graph
	@echo ""
	@echo "Outdated dependencies:"
	@go list -u -m all

## update-deps: Update all dependencies
update-deps:
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy
	@go mod verify

## download: Download all client tools (oc, opm, oc-mirror)
download: build
	@echo "Downloading OpenShift client tools..."
	@echo "OpenShift Version: $(OCP_VERSION)"
	@echo "Tools: $(CLIENT_TOOLS)"
	@$(BIN_DIR)/$(BINARY_NAME) download --version $(OCP_VERSION) --tools $(CLIENT_TOOLS) --bin-dir $(BIN_DIR)

## download-oc: Download oc client only
download-oc: build
	@echo "Downloading oc client..."
	@$(BIN_DIR)/$(BINARY_NAME) download --version $(OCP_VERSION) --tools oc --bin-dir $(BIN_DIR)

## download-opm: Download opm client only
download-opm: build
	@echo "Downloading opm client..."
	@$(BIN_DIR)/$(BINARY_NAME) download --version $(OCP_VERSION) --tools opm --bin-dir $(BIN_DIR)

## download-oc-mirror: Download oc-mirror client only
download-oc-mirror: build
	@echo "Downloading oc-mirror client..."
	@$(BIN_DIR)/$(BINARY_NAME) download --version $(OCP_VERSION) --tools oc-mirror --bin-dir $(BIN_DIR)

## download-all: Download all client tools (alias for download)
download-all: download

## setup: Build binary and download required tools
setup: build download
	@echo ""
	@echo "✅ Setup complete!"
	@echo "   Binary: $(BIN_DIR)/$(BINARY_NAME)"
	@echo "   Tools: $(BIN_DIR)/oc, $(BIN_DIR)/opm, $(BIN_DIR)/oc-mirror"
	@echo ""
	@echo "💡 Add to PATH:"
	@echo "   export PATH=\"$$(pwd)/$(BIN_DIR):\$$PATH\""
