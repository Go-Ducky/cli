# GoDucky CLI - development Makefile.

VERSION ?= 0.1.0
BINARY  ?= goducky
GO      ?= go
CGO_ENABLED ?= 0

.PHONY: build test vet lint run clean install all

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" \
		-o $(BINARY) ./cmd/goducky

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; skipping"

run:
	$(GO) run ./cmd/goducky

install:
	$(GO) install ./cmd/goducky

clean:
	rm -rf dist $(BINARY) goducky.exe

all: vet test build
