# ai-sh

Convert natural language to bash commands using a local LLM.

```
ai show disk usage
ai find large log files
ai kill process on port 3000
```

Shows the generated command in a confirmation box before running anything.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash
```

This installs:
- `~/.ai-sh/bin/ai` — the CLI binary
- `~/.ai-sh/bin/llama-cli` — llama.cpp inference engine
- `~/.ai-sh/models/` — your chosen model (`.gguf`)

## Models

| # | Model | Size | Notes |
|---|-------|------|-------|
| 1 | TinyLlama 1.1B Q4_K_M | ~670 MB | Fastest, lowest RAM |
| 2 | Qwen2.5-Coder 1.5B Q4_K_M | ~1.0 GB | Good for shell commands |
| 3 | DeepSeek-Coder 1.3B Q4_K_M | ~800 MB | **Default** — best balance |

## Usage

```
ai <natural language prompt>
```

The generated command is shown for confirmation before execution:

```
+----------------------+
|  df -h               |
+----------------------+

Run? [y/N]:
```

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
