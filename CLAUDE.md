# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build    # compile to dist/ai
make install  # build and install to ~/.ai-sh/bin/ai
make release  # cross-compile for linux/darwin × amd64/arm64
make fmt      # gofmt
make vet      # go vet
make tidy     # go mod tidy
make clean    # remove dist/
```

No tests are currently defined. Run a single package with `go test ./internal/llm/` etc.

## Architecture

**ai-sh** converts natural language prompts into bash commands using a local LLM (llama.cpp). The flow:

1. `cmd/root.go` — Cobra CLI entry point; validates that `llama-cli` binary and a `.gguf` model exist, then orchestrates the pipeline
2. `internal/llm/llama.go` — Finds `llama-cli` (checks `~/.ai-sh/bin/`, then PATH) and a model file (first `.gguf` in `~/.ai-sh/models/`), runs inference with temperature 0.1 / max 100 tokens using `-cnv -st` (single-turn conversation mode) with `Setsid: true` to detach from the controlling terminal so llama-cli's UI output is captured rather than written to `/dev/tty`. Parses the model reply out of the conversation output and strips markdown fences.
3. `internal/runner/exec.go` — Shows the generated command in an ASCII box, prompts `[y/N]`, and executes via `/bin/sh -c` if confirmed

**Runtime dependencies** (not in go.mod — installed by `install.sh` to `~/.ai-sh/`):
- `llama-cli` binary from llama.cpp releases
- A `.gguf` model file (TinyLlama 1.1B, Qwen2.5-Coder 1.5B, or DeepSeek-Coder 1.3B)

The binary name is `ai`. Version is injected at build time via `-ldflags "-X main.version=..."` in the Makefile.
