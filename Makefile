GO ?= .tools/go-root/bin/go
BINARY ?= bin/dex

.PHONY: build test fmt vet scripts-check qa

build:
	$(GO) build -o $(BINARY) ./cmd/dex

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

scripts-check:
	bash -n scripts/install.sh
	bash -n scripts/build-release.sh

fmt:
	$(GO)fmt -w cmd internal

qa: fmt test vet scripts-check build
