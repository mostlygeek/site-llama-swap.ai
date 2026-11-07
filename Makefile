BUILD_DIR = build
BINARY = $(BUILD_DIR)/site-llama-swap
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

# Builds and tests use llama-swap's latest commit on main. They run against a
# copy of go.mod in build/, so the committed go.mod is left alone. Set
# LLAMA_SWAP_REF=<commit> to build against a specific commit instead.
MODFILE = $(CURDIR)/$(BUILD_DIR)/go.mod
GO = GOFLAGS=-modfile=$(MODFILE) go

.PHONY: all build latest generate run test test-go test-js update-llama-swap clean

# `make` builds a binary you can run: ./build/site-llama-swap
all: build

# Point the build's go.mod at llama-swap main. The commit is looked up on
# GitHub because the Go module proxy can be a few minutes behind a new push.
latest:
	@mkdir -p $(BUILD_DIR)
	@cp go.mod $(MODFILE) && cp go.sum $(BUILD_DIR)/go.sum
	@ref="$${LLAMA_SWAP_REF:-$$(git ls-remote https://github.com/mostlygeek/llama-swap refs/heads/main | cut -f1)}"; \
	if [ -z "$$ref" ]; then echo "could not look up llama-swap main on GitHub" >&2; exit 1; fi; \
	echo "llama-swap: $$ref"; \
	$(GO) get github.com/mostlygeek/llama-swap@$$ref

# generate writes llama-swap's source for the help agent's code__* tools
generate: latest
	$(GO) generate ./...

build: generate
	CGO_ENABLED=0 $(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/site

run: build
	$(BINARY) -listen localhost:8080

test: test-go test-js

test-go: generate
	gofmt -l . | (! grep .)
	$(GO) vet ./...
	$(GO) test ./...

test-js:
	node --test web/test/*.test.mjs

# Move the committed go.mod to llama-swap main too. Builds do not need this;
# it keeps `go build`, `go test` and editors close to what make builds.
update-llama-swap: latest
	cp $(MODFILE) go.mod
	cp $(BUILD_DIR)/go.sum go.sum
	go mod tidy

clean:
	rm -rf $(BUILD_DIR) internal/llamasrc/archive/*.tar.gz
