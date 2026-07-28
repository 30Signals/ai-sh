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

Run tests with `go test ./...`; a single package with `go test ./internal/llm/`. Coverage is currently limited to reply parsing (`internal/llm/provider_test.go`) and the memory store (`internal/memory/memory_test.go`). The memory tests isolate themselves with `t.Setenv("HOME", t.TempDir())`, which works because every path goes through `config.Dir` → `os.UserHomeDir`.

To exercise the installer without a published GitHub release, against a throwaway `$HOME`:

```bash
make build
HOME=/tmp/fakehome LOCAL_BINARY=dist/ai AI_SH_BACKEND=cloud ./install.sh
```

`install.sh` also honors `AI_SH_BACKEND` (`local`/`cloud`), `MODEL_CHOICE` (1–4, implies local), `AI_VERSION`, and `LLAMA_TAG`. It reads answers through `ask`, which falls back to `/dev/tty` for the piped-into-bash case and gives up quietly when `have_tty` fails (containers have a `/dev/tty` that cannot be opened).

## Architecture

**ai-sh** turns a natural language prompt into a single POSIX `sh` command, using either a local llama.cpp model or a cloud API. Module path is `github.com/user/ai-sh`; the binary is `ai`.

Pipeline:

1. `main.go` / `cmd/root.go` — Cobra entry point. `Args` is `ArbitraryArgs` (not `MinimumNArgs(1)`) so bare flag invocations like `ai --setup` work; the arg check happens inside `RunE`. Joins all args into one prompt, builds a provider, and passes `provider.Generate` to the runner so refinement re-infers without re-resolving anything.
2. `internal/config` — `Dir` (`~/.ai-sh`) is the one place the state directory is named; config, the local runtime, and memory all hang off it. `~/.ai-sh/config.json` (mode 0600, may hold an API key). `Load` treats a missing file as `provider: local`, so pre-cloud installs keep working, then applies `AI_SH_*` env overrides. `Resolve` fills endpoint/model/key from `Presets` and the provider's conventional env key (`MISTRAL_API_KEY`, …); it is the single place that decides a backend is unusable, so error messages for a missing key or model live there.
3. `internal/memory` — `~/.ai-sh/memory.md`, a hand-editable list of notes injected into every system prompt. Deliberately explicit: nothing is stored unless the user runs `ai --remember` or edits the file, so a generated command never silently changes future ones. Parsing is tolerant (`- `, `* `, or bare lines; `#` comments skipped) and `Forget` deletes a single line rather than rewriting the file, so hand-written comments survive. `Prompt` swallows read errors — a broken memory file must never block command generation — and caps what it emits (50 notes × 200 chars, 2 KB hard ceiling) because the local models have small context windows.
4. `internal/llm/provider.go` — the `Provider` interface (`Generate`, `Describe`) plus `New(cfg)`, and everything both backends share:
   - `buildSystemPrompt` — shell-command rules, the prose escape hatch, machine context (cwd, `osName`, `userlandNote` for the GNU/BSD flag split, `$SHELL`), the user's notes from `memory.Prompt()` (re-read on every call, so refinements pick up edits), then few-shot examples. Order matters for the small local models: examples sit last, closest to the reply.
   - the prose fallback. Not every prompt is a shell task, so the system prompt asks the model to answer questions and small talk with a line starting `ANSWER:` (`AnswerPrefix`). `formatReply` recognises that marker at the start of any line — tolerating preamble and `**bold**` — and collapses the rest into one line; otherwise it falls through to `stripMarkdown` and the reply is treated as a command. `SplitAnswer` is how callers tell the two apart. Both backends return through `formatReply`, so the marker is the only contract between the prompt and `internal/runner`.
   - `stripMarkdown` — carves a bare command out of fenced or `$`-prefixed output.
5. `internal/llm/cloud.go` — one HTTP implementation for every cloud preset, since they all speak OpenAI chat-completions. Temperature 0.1 / 220 max tokens, matching local (the ceiling has to leave room for a prose answer, not just a command). Response bodies are parsed leniently so a non-JSON proxy error still yields a useful message.
6. `internal/llm/llama.go` — local resolution + inference + output scrubbing:
   - `FindLlamaCLI` checks `~/.ai-sh/bin/llama-cli`, then `$PATH`, then `/opt/homebrew/bin`, `/usr/local/bin`.
   - `FindModel` returns the *first* `.gguf` in `~/.ai-sh/models/` (so multiple models = nondeterministic pick; the installer skips downloading if any `.gguf` is present).
   - `(*local).Generate` invokes llama-cli with `-cnv -st` (single-turn conversation), temp 0.1, `-n 220`, `-ngl 0` (CPU only), and `SysProcAttr{Setsid: true}`. The setsid matters: without a controlling terminal, llama-cli writes its UI to stdout instead of `/dev/tty`, which is the only way we can capture the reply.
   - Because the UI is interleaved, the reply must be carved out: `cleanOutput` slices between the `> <userPrompt>` echo and the trailing `[ Prompt:` stats block, `stripBackspaces` undoes spinner `\x08` artifacts, and `stripMarkdown` pulls the first line out of a fenced block or falls back to the first non-empty line. Changing llama-cli flags or upgrading llama.cpp can break these markers — verify end-to-end after touching either.
7. `cmd/setup.go` — the `ai --setup` menu. `cloudOrder` exists only to give the map in `config.Presets` a stable display order; adding a preset means touching both.
8. `internal/runner/exec.go` — `Handle` is the entry point: a reply that `llm.SplitAnswer` reads as prose is printed and nothing is offered for execution, otherwise it goes to `ConfirmAndRun`. `ConfirmAndRun` loops on a single-keypress prompt: `↵` runs via `/bin/sh -c`, `e` reads a refinement line and re-infers with `originalPrompt + " — " + feedback` (a refinement can itself come back as prose, which is printed and ends the loop), anything else cancels. `readKey` flips the terminal to raw mode with direct `ioctl` syscalls; the `TCGETS`/`TIOCGETA` constants differ per OS and live in `term_linux.go` / `term_darwin.go` — any new GOOS needs another `term_*.go`.

**Runtime dependencies for the local backend only** (not in go.mod — `install.sh` fetches them into `~/.ai-sh/`; the cloud backend needs neither):
- `llama-cli` plus its co-located `*.so`/`*.dylib` files from a llama.cpp release (RUNPATH is `$ORIGIN`, so the libs must sit next to the binary)
- one `.gguf` model — TinyLlama 1.1B, Qwen2.5-Coder 1.5B (default), or Qwen2.5-Coder 3B

`VERSION` is injected via `-ldflags -X main.version=...` from `git describe`, but nothing currently surfaces it — there is no `--version` flag.
