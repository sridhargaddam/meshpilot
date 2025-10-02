# MeshPilot Makefile

.PHONY: build clean test lint run install deps help

# Build variables
BINARY_NAME=meshpilot
BUILD_DIR=build
MAIN_PATH=main.go

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

help: ## Display this help message
	@echo "MeshPilot - Kubernetes Istio MCP Server"
	@echo ""
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

deps: ## Download and install dependencies
	$(GOMOD) download
	$(GOMOD) tidy

build: deps ## Build the binary
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)

clean: ## Clean build artifacts
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

test: ## Run tests
	$(GOTEST) -v ./...

test-coverage: ## Run tests with coverage
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	$(GOFMT) ./...

run: build ## Build and run the server
	./$(BUILD_DIR)/$(BINARY_NAME)

run-demo: build ## Run server with demo timeout (30s)
	MESHPILOT_DEMO=true ./$(BUILD_DIR)/$(BINARY_NAME)

help-tools: build ## Show help and available tools
	@echo "=== MeshPilot Help ==="
	./$(BUILD_DIR)/$(BINARY_NAME) --help
	@echo ""
	@echo "=== Available Tools ==="
	./$(BUILD_DIR)/$(BINARY_NAME) --list-tools

install: build ## Install the binary to GOPATH/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

docker-build: ## Build Docker image
	docker build -t meshpilot:$(VERSION) .

docker-run: docker-build ## Build and run Docker container
	docker run --rm -it \
		-v $(HOME)/.kube:/root/.kube:ro \
		meshpilot:$(VERSION)

# Development helpers
dev-setup: ## Set up development environment
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GOMOD) download

# KIND cluster helpers
kind-create: ## Create a KIND cluster for testing
	kind create cluster --name meshpilot-test --config examples/kind-config.yaml || true
	kubectl config use-context kind-meshpilot-test

kind-delete: ## Delete the KIND cluster
	kind delete cluster --name meshpilot-test

kind-load-image: docker-build ## Load Docker image into KIND cluster
	kind load docker-image meshpilot:$(VERSION) --name meshpilot-test

# OpenShift helpers
openshift-login: ## Login to OpenShift (requires OC_URL and OC_TOKEN)
	@if [ -z "$(OC_URL)" ] || [ -z "$(OC_TOKEN)" ]; then \
		echo "Please set OC_URL and OC_TOKEN environment variables"; \
		exit 1; \
	fi
	oc login $(OC_URL) --token=$(OC_TOKEN)

# Istio helpers
istio-install: build ## Install Istio using meshpilot
	./$(BUILD_DIR)/$(BINARY_NAME) --tool install_istio --args '{"profile": "demo"}'

istio-samples: build ## Deploy sample applications
	./$(BUILD_DIR)/$(BINARY_NAME) --tool deploy_sleep_app --args '{}'
	./$(BUILD_DIR)/$(BINARY_NAME) --tool deploy_httpbin_app --args '{}'

istio-test: build ## Test connectivity between sample apps
	./$(BUILD_DIR)/$(BINARY_NAME) --tool test_sleep_to_httpbin --args '{}'

istio-cleanup: build ## Clean up Istio and samples
	./$(BUILD_DIR)/$(BINARY_NAME) --tool undeploy_sleep_app --args '{}'
	./$(BUILD_DIR)/$(BINARY_NAME) --tool undeploy_httpbin_app --args '{}'
	./$(BUILD_DIR)/$(BINARY_NAME) --tool uninstall_istio --args '{}'

# Utility targets
check-deps: ## Check for required dependencies
	@echo "Checking dependencies..."
	@which kubectl > /dev/null || (echo "kubectl not found" && exit 1)
	@which docker > /dev/null || (echo "docker not found" && exit 1)
	@echo "All dependencies found"

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Go Version: $(shell go version)"

.DEFAULT_GOAL := help
