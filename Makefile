BUILD_DIR = build
BINARY = $(BUILD_DIR)/site-llama-swap
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: all build generate run test test-go test-js update-llama-swap clean

# `make` builds a binary you can run: ./build/site-llama-swap
all: build

# generate first: it writes llama-swap's source, from the version in go.mod,
# for the help agent's code__* tools
build: generate
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/site

generate:
	go generate ./...

run: build
	$(BINARY) -listen localhost:8080

test: test-go test-js

test-go: generate
	gofmt -l . | (! grep .)
	go vet ./...
	go test ./...

test-js:
	node --test web/test/*.test.mjs

# move to the latest commit on llama-swap's main branch, or REF=<commit>.
# The commit is looked up on GitHub because the Go module proxy can be a few
# minutes behind a new push.
REF ?= $(shell git ls-remote https://github.com/mostlygeek/llama-swap refs/heads/main | cut -f1)
update-llama-swap:
	go get -tool github.com/mostlygeek/llama-swap/docbot/gensource@$(REF)
	go mod tidy

clean:
	rm -rf $(BUILD_DIR) internal/llamasrc/archive/*.tar.gz
