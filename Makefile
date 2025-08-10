# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Binary name
BINARY_NAME=slbot
BINARY_UNIX=$(BINARY_NAME)_unix

# Build info
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
BUILD_USER=$(shell whoami)
BUILD_HOST=$(shell hostname)

# Go build flags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.BuildUser=$(BUILD_USER) -X main.BuildHost=$(BUILD_HOST)"

# Source files
SOURCES=$(wildcard *.go) $(wildcard internal/*/*.go)

.PHONY: all build clean test deps fmt vet lint help run install uninstall

.DEFAULT_GOAL := help

## Build the binary
build: $(BINARY_NAME)

$(BINARY_NAME): $(SOURCES)
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v ./...

## Build for all platforms
all: clean build build-linux build-windows build-darwin

## Build for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_UNIX) -v ./...

## Build for Windows
build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME).exe -v ./...

## Build for macOS
build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME)_darwin -v ./...

## Run tests
test:
	$(GOTEST) -v ./...

## Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## Install dependencies
deps:
	$(GOMOD) download
	$(GOMOD) verify

## Update dependencies
deps-update:
	$(GOMOD) tidy
	$(GOGET) -u ./...

## Format Go code
fmt:
	$(GOFMT) ./...

## Run go vet
vet:
	$(GOCMD) vet ./...

## Run golint (requires golint to be installed)
lint:
	@if command -v golint >/dev/null 2>&1; then \
		golint ./...; \
	else \
		echo "golint not installed. Install with: go install golang.org/x/lint/golint@latest"; \
	fi

## Run staticcheck (requires staticcheck to be installed)
staticcheck:
	@if command -v staticcheck >/dev/null 2>&1; then \
		staticcheck ./...; \
	else \
		echo "staticcheck not installed. Install with: go install honnef.co/go/tools/cmd/staticcheck@latest"; \
	fi

## Run the application
run: build
	./$(BINARY_NAME)

## Run the application with config file
run-config: build
	./$(BINARY_NAME) bot_config.xml

## Install the binary to $GOPATH/bin
install:
	$(GOCMD) install $(LDFLAGS) ./...

## Uninstall the binary from $GOPATH/bin
uninstall:
	$(GOCMD) clean -i ./...

## Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f $(BINARY_NAME).exe
	rm -f $(BINARY_NAME)_darwin
	rm -f coverage.out
	rm -f coverage.html

## Check code quality (fmt, vet, test)
check: fmt vet test

## Full quality check including linting
check-all: fmt vet lint staticcheck test

## Create a release build
release: clean check-all
	$(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) -v -trimpath ./...

## Show build information
info:
	@echo "Binary name: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build time: $(BUILD_TIME)"
	@echo "Build user: $(BUILD_USER)"
	@echo "Build host: $(BUILD_HOST)"

## Show this help message
help:
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  %-20s %s\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)
