# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build    # compile to dist/ai
make install  # build and install to ~/.ai-sh/bin/ai
make release  # cross-compile for linux/darwin × amd64/arm64
make fmt      # gofmt -w .
make vet      # go vet ./...
make tidy     # go mod tidy
make clean    # remove dist/
make help     # list targets
```

No tests exist yet. Once added, run a single package with `go test ./internal/llm/`.

To exercise the installer without a published GitHub release:

```bash
make build && LOCAL_BINARY=dist/ai ./install.sh
```

`install.sh` also honors `MODEL_CHOICE` (1–4, skips the interactive prompt), `AI_VERSION`, and `LLAMA_TAG`.

## Architecture

**ai-sh** turns a natural language prompt into a single POSIX `sh` command using a local llama.cpp model. Module path is `github.com/user/ai-sh`; the binary is `ai`.

Pipeline:

1. `main.go` / `cmd/root.go` — Cobra entry point. Joins all args into one prompt, resolves `llama-cli` and a model, then hands a closure `infer(prompt) (string, error)` to the runner so refinement can re-run inference without re-resolving paths.
2. `internal/llm/llama.go` — resolution + inference + output scrubbing:
   - `FindLlamaCLI` checks `~/.ai-sh/bin/llama-cli`, then `$PATH`, then `/opt/homebrew/bin`, `/usr/local/bin`.
   - `FindModel` returns the *first* `.gguf` in `~/.ai-sh/models/` (so multiple models = nondeterministic pick; the installer skips downloading if any `.gguf` is present).
   - `RunInference` invokes llama-cli with `-cnv -st` (single-turn conversation), temp 0.1, `-n 100`, `-ngl 0` (CPU only), and `SysProcAttr{Setsid: true}`. The setsid matters: without a controlling terminal, llama-cli writes its UI to stdout instead of `/dev/tty`, which is the only way we can capture the reply.
   - Because the UI is interleaved, the reply must be carved out: `cleanOutput` slices between the `> <userPrompt>` echo and the trailing `[ Prompt:` stats block, `stripBackspaces` undoes spinner `\x08` artifacts, and `stripMarkdown` pulls the first line out of a fenced block or falls back to the first non-empty line. Changing llama-cli flags or upgrading llama.cpp can break these markers — verify end-to-end after touching either.
   - The system prompt is built in `buildSystemPrompt` and includes the current working directory as context.
3. `internal/runner/exec.go` — `ConfirmAndRun` loops on a single-keypress prompt: `↵` runs via `/bin/sh -c`, `e` reads a refinement line and re-infers with `originalPrompt + " — " + feedback`, anything else cancels. `readKey` flips the terminal to raw mode with direct `ioctl` syscalls; the `TCGETS`/`TIOCGETA` constants differ per OS and live in `term_linux.go` / `term_darwin.go` — any new GOOS needs another `term_*.go`.

**Runtime dependencies** (not in go.mod — `install.sh` fetches them into `~/.ai-sh/`):
- `llama-cli` plus its co-located `*.so`/`*.dylib` files from a llama.cpp release (RUNPATH is `$ORIGIN`, so the libs must sit next to the binary)
- one `.gguf` model — TinyLlama 1.1B, Qwen2.5-Coder 1.5B (default), or Qwen2.5-Coder 3B

`VERSION` is injected via `-ldflags -X main.version=...` from `git describe`, but nothing currently surfaces it — there is no `--version` flag.
