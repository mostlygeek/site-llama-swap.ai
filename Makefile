BUILD_DIR = build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

# path to a llama-swap checkout, for `make sync`
LLAMA_SWAP ?= ../llama-swap

.PHONY: build run test test-go test-js sync clean

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/site-llama-swap .

run:
	go run . -listen localhost:8080

test: test-go test-js

test-go:
	gofmt -l . | (! grep .)
	go vet ./...
	go test ./...

test-js:
	node --test web/test/*.test.mjs

# copy llama-swap's docs, doc tools and source into internal/llamaswap
sync:
	scripts/sync-llama-swap.sh $(LLAMA_SWAP)

clean:
	rm -rf $(BUILD_DIR)
