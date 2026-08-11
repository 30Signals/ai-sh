<div align="center">

# ai-sh

**Turn plain English into a POSIX shell command — locally, with no API keys, or via any OpenAI-compatible cloud provider's free tier.**

[![Release](https://img.shields.io/github/v/release/30Signals/ai-sh?label=release&color=blue)](https://github.com/30Signals/ai-sh/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-lightgrey)](LICENSE)
[![Platforms](https://img.shields.io/badge/platform-linux%20%7C%20macos-informational)](#requirements)

</div>

```
$ ai show disk usage
$ ai find large log files
$ ai kill process on port 3000
```

Every generated command is shown for confirmation before it runs — never executed blind.

## Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash
```

That's it — the installer asks whether to run fully offline (local model) or against a cloud provider, downloads what it needs into `~/.ai-sh/`, and gets out of the way. Details below.

## Contents

- [Install](#install)
- [Local models](#local-models)
- [Cloud models](#cloud-models)
- [Usage](#usage)
- [Session history](#session-history)
- [Memory](#memory)
- [Requirements](#requirements)
- [Build from source](#build-from-source)
- [License](#license)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/30Signals/ai-sh/main/install.sh | bash
```

The installer asks whether to run the model locally or in the cloud:

- **Local** — downloads llama.cpp and a `.gguf` model (~1 GB) into `~/.ai-sh/`. Works offline.
- **Cloud** — installs only the `ai` binary (~8 MB) and runs `ai --setup` to pick a provider and paste an API key.

Restart your shell or run:
```bash
export PATH="$HOME/.ai-sh/bin:$PATH"
```

To skip the questions:
```bash
# local, Qwen 3B
curl -fsSL .../install.sh | MODEL_CHOICE=3 bash
# cloud, configure later with: ai --setup
curl -fsSL .../install.sh | AI_SH_BACKEND=cloud bash
```

## Local models

| # | Model | Size | Notes |
|---|-------|------|-------|
| 1 | TinyLlama 1.1B Q4_K_M | ~670 MB | Fastest, lowest RAM |
| 2 | Qwen2.5-Coder 1.5B Q4_K_M | ~1.0 GB | **Default** — best for shell commands |
| 3 | Qwen2.5-Coder 3B Q4_K_M | ~2.0 GB | Smarter, still fast |

## Cloud models

Any OpenAI-compatible chat-completions endpoint works. These have presets — several offer free tiers:

| Provider | Default model | API key |
|----------|---------------|---------|
| `mistral` | `devstral-medium-latest` | [console.mistral.ai](https://console.mistral.ai/api-keys) |
| `groq` | `llama-3.3-70b-versatile` | [console.groq.com](https://console.groq.com/keys) |
| `openrouter` | `mistralai/devstral-small-2505:free` | [openrouter.ai](https://openrouter.ai/keys) |
| `cerebras` | `llama-3.3-70b` | [cloud.cerebras.ai](https://cloud.cerebras.ai/platform) |
| `custom` | — | your own endpoint (`base_url`) |

```bash
ai --setup            # pick provider + model, store the key
ai --status           # show which backend is active
```

Selection is saved to `~/.ai-sh/config.json` (mode `0600`):

```json
{
  "provider": "mistral",
  "model": "devstral-medium-latest",
  "api_key": "..."
}
```

Override per call, or skip the file entirely:

```bash
ai --provider groq "tail the nginx error log"
ai --provider mistral --model devstral-small-latest "list open ports"

export AI_SH_PROVIDER=mistral   # also: AI_SH_MODEL, AI_SH_BASE_URL, AI_SH_API_KEY,
                                #       AI_SH_HISTORY, AI_SH_HISTORY_TURNS
```

If `api_key` is empty, ai-sh falls back to the provider's usual variable —
`MISTRAL_API_KEY`, `GROQ_API_KEY`, `OPENROUTER_API_KEY`, `CEREBRAS_API_KEY`.

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
- **e** — give feedback to refine it; the model sees the command it just wrote, so
  "use find instead" corrects that command rather than re-answering from scratch
- **n** — cancel

## Session history

Off by default. Once enabled, a follow-up in the same terminal can refer back to
what came before:

```
$ ai list the files here
ai:
ls -la

$ ai now only the pdfs
↳ 1 earlier turn from this terminal (ai --new to reset)
ai:
ls -la *.pdf
```

Turn it on in `ai --setup`, or set `"history": true` in `~/.ai-sh/config.json`.
It is a one-time setting, not a per-call flag — follow-up invocations stay bare.

```bash
ai --new              # forget this terminal's history
ai --status           # show whether history is on, and how many turns are live
```

Details worth knowing:

- **Cloud backends only.** Small local models tend to answer *worse* with several
  turns ahead of the instruction, and every local call re-processes the whole
  transcript on CPU. Set it with the local backend and `--status` will say
  `off (not supported by the local backend)`.
- **Scoped to the terminal and the directory.** History is keyed to the shell it
  ran in, expires after two hours, and is dropped when you `cd` elsewhere.
- **Stored on disk** at `~/.ai-sh/sessions/` (mode `0600`), holding past
  instructions and the commands they produced. Secret-shaped values
  (`API_KEY=…`, `Authorization: Bearer …`, `--password …`) are redacted before
  writing, but treat the directory as sensitive regardless. Prompts also get
  larger, so cloud calls cost a little more.

## Memory

Notes you want on every prompt — preferred tools, machine quirks, house style —
live in `~/.ai-sh/memory.md`, next to the config file, and are sent with every
inference:

```bash
ai --remember "prefer rg over grep"
ai --remember "this box uses doas, not sudo"
ai --memory              # list notes with their numbers
ai --forget 2            # drop one
ai --forget all          # drop them all
```

The file is plain markdown, so editing it by hand works just as well:

```markdown
# ai-sh memory
- prefer rg over grep
- this box uses doas, not sudo
```

One note per line, `#` lines ignored, bullets optional. Nothing is ever written
implicitly — ai-sh only stores what you tell it to. Up to 50 notes of 200
characters each, keeping the prompt small enough for the local models.

## Requirements

- Linux or macOS (amd64 or arm64)
- Local backend: ~1 GB free disk space for the model, ~512 MB RAM (CPU only)
- Cloud backend: an API key; nothing else

## Build from source

```bash
make build    # compile to dist/ai
make install  # build and install to ~/.ai-sh/bin/ai
make release  # cross-compile for linux/darwin × amd64/arm64
```

## License

[MIT](LICENSE)
