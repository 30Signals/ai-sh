VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
BINARY  := ai
DIST    := dist

.PHONY: build install release fmt vet tidy clean help

## build: compile for current platform → dist/ai
build:
	@mkdir -p $(DIST)
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY) .

## install: copy binary to ~/.ai-sh/bin/ai
install: build
	@mkdir -p $(HOME)/.ai-sh/bin
	cp $(DIST)/$(BINARY) $(HOME)/.ai-sh/bin/$(BINARY)
	@echo "Installed to $(HOME)/.ai-sh/bin/$(BINARY)"

## release: cross-compile for linux/darwin × amd64/arm64
release:
	@mkdir -p $(DIST)
	GOOS=linux  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64  .
	GOOS=linux  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-arm64  .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-arm64 .
	@echo "Built binaries in $(DIST)/:"
	@ls -lh $(DIST)/

## fmt: run gofmt
fmt:
	gofmt -w .

## vet: run go vet
vet:
	go vet ./...

## tidy: run go mod tidy
tidy:
	go mod tidy

## clean: remove dist/
clean:
	rm -rf $(DIST)

## help: show this help
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
