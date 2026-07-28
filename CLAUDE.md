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

`go test ./...` covers `internal/config`, `internal/history`, and `internal/llm`; run one package with `go test ./internal/llm/`. The history tests set `HOME` to a temp dir, so they write no real transcripts. `cmd` and `internal/runner` are untested — both need a controlling terminal.

To exercise the cloud path end-to-end without an API key, point a `custom` provider at a local stub that speaks chat-completions and echoes the request bodies back; that is the only practical way to confirm history actually reaches the wire. Note that the keypress prompt needs a real tty, so drive it under `script -qec` and feed keys to `script`'s stdin — piping into `ai` itself gives it a pipe for stdin and `readKey`'s `ioctl` fails. Multiple `ai` calls only share a session when they share a parent shell, so put them all in one script rather than one `script` call each.

To exercise the installer without a published GitHub release, against a throwaway `$HOME`:

```bash
make build
HOME=/tmp/fakehome LOCAL_BINARY=dist/ai AI_SH_BACKEND=cloud ./install.sh
```

`install.sh` also honors `AI_SH_BACKEND` (`local`/`cloud`), `MODEL_CHOICE` (1–4, implies local), `AI_VERSION`, `LLAMA_TAG`, and `AI_SH_HISTORY` (inherited by `ai --setup`, where it preselects the session-history answer). It reads answers through `ask`, which falls back to `/dev/tty` for the piped-into-bash case and gives up quietly when `have_tty` fails (containers have a `/dev/tty` that cannot be opened).

## Architecture

**ai-sh** turns a natural language prompt into a single POSIX `sh` command, using either a local llama.cpp model or a cloud API. Module path is `github.com/user/ai-sh`; the binary is `ai`.

Pipeline:

1. `main.go` / `cmd/root.go` — Cobra entry point. `Args` is `ArbitraryArgs` (not `MinimumNArgs(1)`) so bare flag invocations like `ai --setup` work; the arg check happens inside `RunE`. Joins all args into one prompt, prepends any session history as `[]llm.Message`, and passes `provider.Generate` to the runner so refinement re-infers without re-resolving anything. `--new` clears the session; `--status` also reports the effective history setting.
2. `internal/config` — `~/.ai-sh/config.json` (mode 0600, may hold an API key). `Load` treats a missing file as `provider: local`, so pre-cloud installs keep working, then applies `AI_SH_*` env overrides. `Resolve` fills endpoint/model/key from `Presets` and the provider's conventional env key (`MISTRAL_API_KEY`, …); it is the single place that decides a backend is unusable, so error messages for a missing key or model live there. `HistoryEnabled` is deliberately a *clamp* rather than a `Resolve` error — it returns false for `local` however the config reads, so switching backends can never turn a saved config into a startup failure.
3. `internal/llm/provider.go` — the `Provider` interface (`Generate`, `Describe`) plus `New(cfg)`, and the pieces both backends share: `buildSystemPrompt` and `stripMarkdown`. `Generate` takes `[]Message` whose **last element must be the user's current instruction** — `currentInstruction` enforces this, because both backends split the trailing turn from its context. `buildSystemPrompt(prior)` includes the cwd and folds prior turns into text; cloud passes `nil` since it sends real roles.
4. `internal/llm/cloud.go` — one HTTP implementation for every cloud preset, since they all speak OpenAI chat-completions. Prior turns travel as real `user`/`assistant` messages. Temperature 0.1 / 100 max tokens, matching local. Response bodies are parsed leniently so a non-JSON proxy error still yields a useful message.
5. `internal/llm/llama.go` — local resolution + inference + output scrubbing:
   - `FindLlamaCLI` checks `~/.ai-sh/bin/llama-cli`, then `$PATH`, then `/opt/homebrew/bin`, `/usr/local/bin`.
   - `FindModel` returns the *first* `.gguf` in `~/.ai-sh/models/` (so multiple models = nondeterministic pick; the installer skips downloading if any `.gguf` is present).
   - `(*local).Generate` invokes llama-cli with `-cnv -st` (single-turn conversation), temp 0.1, `-n 100`, `-ngl 0` (CPU only), and `SysProcAttr{Setsid: true}`. The setsid matters: without a controlling terminal, llama-cli writes its UI to stdout instead of `/dev/tty`, which is the only way we can capture the reply.
   - Prior turns go into `-sys` as text, never into `-p`. This is forced by `cleanOutput`, which finds the reply by the `> <instruction>` echo: the current instruction has to stay the only thing in `-p` or the marker moves. Do not "fix" this by replaying turns as real conversation.
   - Because the UI is interleaved, the reply must be carved out: `cleanOutput` slices between the `> <userPrompt>` echo and the trailing `[ Prompt:` stats block, `stripBackspaces` undoes spinner `\x08` artifacts, and `stripMarkdown` pulls the first line out of a fenced block or falls back to the first non-empty line. Changing llama-cli flags or upgrading llama.cpp can break these markers — verify end-to-end after touching either.
6. `cmd/setup.go` — the `ai --setup` menu. `cloudOrder` exists only to give the map in `config.Presets` a stable display order; adding a preset means touching both. The session-history question is asked only on the cloud path, since `HistoryEnabled` clamps it off for local and a setting that visibly does nothing is worse than no setting.
7. `internal/runner/exec.go` — `ConfirmAndRun` loops on a single-keypress prompt: `↵` runs via `/bin/sh -c`, `e` reads a refinement line, anything else cancels. Refining appends the command as an assistant turn and the feedback as a user turn, so the model corrects its own output; it returns an `Outcome` (final command, ran, exit code) for the caller to record. `readKey` flips the terminal to raw mode with direct `ioctl` syscalls; the `TCGETS`/`TIOCGETA` constants differ per OS and live in `term_linux.go` / `term_darwin.go` — any new GOOS needs another `term_*.go`.
8. `internal/history` — per-terminal transcripts at `~/.ai-sh/sessions/<ppid>-<shell start>.jsonl` (dir 0700, files 0600). Off unless opted into; see the design notes on [issue #3](https://github.com/30Signals/ai-sh/issues/3). Load-bearing decisions:
   - **Everything is best-effort.** A missing, corrupt, or half-written transcript degrades to no history and never becomes a user-visible error. `read` skips lines it cannot parse.
   - **The session key includes the shell's start time**, because PIDs are reused and a new shell must not inherit a stranger's transcript. It comes from `/proc/<pid>/stat` field 22 on Linux and `ps -o lstart=` on darwin (`session_linux.go` / `session_darwin.go`, so a new GOOS needs another). When it cannot be read the key falls back to `unknown` and the 2h TTL is the only guard left — which is why the TTL is short.
   - **Redaction happens on write.** A secret that never lands in the file cannot be leaked later by a change to the read path.
   - **The budget trims newest-first** (`applyBudget`): the newest turn is the one being followed up on, so it is never the one dropped, and an oversized record is truncated rather than discarded.
   - **Records from another cwd are dropped on read**, so `cd` starts a clean slate. Field caps plus `maxRecords` trim-on-append keep the read path a plain full read — no reverse scanner, no rotation.
   - Exit codes are recorded but deliberately *not* fed into the prompt: assistant turns stay bare commands so the model does not learn to emit prose. Using them needs stderr capture in `runCommand`, which is not built.

**Runtime dependencies for the local backend only** (not in go.mod — `install.sh` fetches them into `~/.ai-sh/`; the cloud backend needs neither):
- `llama-cli` plus its co-located `*.so`/`*.dylib` files from a llama.cpp release (RUNPATH is `$ORIGIN`, so the libs must sit next to the binary)
- one `.gguf` model — TinyLlama 1.1B, Qwen2.5-Coder 1.5B (default), or Qwen2.5-Coder 3B

`VERSION` is injected via `-ldflags -X main.version=...` from `git describe`, but nothing currently surfaces it — there is no `--version` flag.
