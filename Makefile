.PHONY: all run test build fmt vet tidy

GO ?= go
PKG := ./...
SERVER := ./cmd/server

all: test build

run: build
	$(GO) run $(SERVER)

test:
	$(GO) test $(PKG)

build:
	$(GO) build $(SERVER)

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

tidy:
	$(GO) mod tidy
