BUILD_DIR = build
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build run test test-go test-js generate update-llama-swap clean

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

# rebuild the llama-swap source archive from the version in go.mod
generate:
	go generate ./...

# move to llama-swap's latest main commit, then regenerate the source archive
# (use REF=<commit or branch> for another one)
REF ?= main
update-llama-swap:
	go get -tool github.com/mostlygeek/llama-swap/docbot/gensource@$(REF)
	go mod tidy
	go generate ./...

clean:
	rm -rf $(BUILD_DIR)
