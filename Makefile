GO       ?= go
BUF      := $(GO) tool -modfile=tools/go.mod buf
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X github.com/Soo-de/suiro/internal/version.Version=$(VERSION)
PROGRAMS := server git-collector renderer cloud-collector suiro

.DEFAULT_GOAL := help

.PHONY: help
help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-16s %s\n", $$1, $$2}'

.PHONY: build
build: ## Build every program into bin/
	@mkdir -p bin
	@for p in $(PROGRAMS); do \
		CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/$$p ./cmd/$$p || exit 1; \
	done

.PHONY: test
test: ## Run all tests
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run all tests with the race detector, as CI does
	$(GO) test -race ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (v2, installed separately)
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go code
	gofmt -w cmd internal

.PHONY: generate
generate: ## Generate gRPC code from proto/ into internal/gen/ (git-ignored)
	$(BUF) generate

.PHONY: proto-lint
proto-lint: ## Lint the .proto contracts
	$(BUF) lint

.PHONY: proto-breaking
proto-breaking: ## Check the .proto contracts against main for breaking changes
	$(BUF) breaking --against '.git#branch=main'

.PHONY: tidy
tidy: ## Tidy both modules
	$(GO) mod tidy
	cd tools && $(GO) mod tidy

.PHONY: clean
clean: ## Remove build and generated output
	rm -rf bin internal/gen
