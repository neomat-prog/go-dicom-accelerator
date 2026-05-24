.PHONY: all run test build fmt vet tidy

GO      ?= go
PKG     := ./...
SERVER  := ./cmd/server
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

all: test build

run:
	$(GO) run $(SERVER)

test:
	$(GO) test $(PKG)

build:
	$(GO) build $(LDFLAGS) -o server $(SERVER)

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

tidy:
	$(GO) mod tidy
