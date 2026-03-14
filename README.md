# ai-sh

> **v0.3.0** — Convert natural language to POSIX shell commands using a local LLM. Runs entirely on your machine, no API keys needed.

```
ai show disk usage
ai find large log files
ai kill process on port 3000
```

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash
```

The installer will ask you to pick a model, then download everything to `~/.ai-sh/`.

Restart your shell or run:
```bash
export PATH="$HOME/.ai-sh/bin:$PATH"
```

**What gets installed:**
- `~/.ai-sh/bin/ai` — the CLI (v0.3.0)
- `~/.ai-sh/bin/llama-cli` — llama.cpp inference engine
- `~/.ai-sh/models/` — your chosen model (`.gguf`)

## Models

| # | Model | Size | Notes |
|---|-------|------|-------|
| 1 | TinyLlama 1.1B Q4_K_M | ~670 MB | Fastest, lowest RAM |
| 2 | Qwen2.5-Coder 1.5B Q4_K_M | ~1.0 GB | **Default** — best for shell commands |
| 3 | Qwen2.5-Coder 3B Q4_K_M | ~2.0 GB | Smarter, still fast |

To select a model during install:
```bash
curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | MODEL_CHOICE=3 bash
```

## Usage

```
ai <natural language prompt>
```

The model generates a command and shows it for confirmation:

```
ai:
df -h

↵ run   e refine   n cancel
```

- **↵** — run the command
- **e** — give feedback to refine it (re-runs inference with extra context)
- **n** — cancel

## Requirements

- Linux or macOS (amd64 or arm64)
- ~1 GB free disk space for the model
- ~512 MB RAM (model runs entirely on CPU)

## Build from source

```bash
make build    # compile to dist/ai
make install  # build and install to ~/.ai-sh/bin/ai
make release  # cross-compile for linux/darwin × amd64/arm64
```
