#!/usr/bin/env bash
set -euo pipefail

REPO="30Signals/ai-sh"

# The oldest release that satisfies everything this script asks the binary to
# do — currently `ai --setup`, which the cloud path shells out to. We install
# this by default instead of "latest" so a script on main can never pair itself
# with an older published binary that lacks a feature it depends on. Bump it in
# the same commit that makes install.sh require a newer binary feature.
# Override with AI_VERSION=latest (or any tag) to install something else.
KNOWN_GOOD_VERSION="v0.5.0"

INSTALL_DIR="$HOME/.ai-sh"
BIN_DIR="$INSTALL_DIR/bin"
MODELS_DIR="$INSTALL_DIR/models"

# Detect OS and arch
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  OS_NAME="linux" ;;
  Darwin) OS_NAME="darwin" ;;
  *)
    echo "Error: Unsupported OS: $OS"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64)  ARCH_NAME="amd64" ;;
  aarch64|arm64) ARCH_NAME="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

echo "Detected: $OS_NAME/$ARCH_NAME"

# Create directories
mkdir -p "$BIN_DIR" "$MODELS_DIR"

# have_tty reports whether /dev/tty can actually be opened. It exists but is
# unusable in containers and CI, so -e alone is not enough.
have_tty() {
  [ -e /dev/tty ] && (exec </dev/tty) 2>/dev/null
}

# ask <prompt> — reads one line from the terminal, even when this script is
# piped into bash. Echoes an empty string when no terminal is available.
ask() {
  local answer=""
  if [ -t 0 ]; then
    read -r -p "$1" answer || true
  elif have_tty; then
    read -r -p "$1" answer </dev/tty || true
  fi
  echo "$answer"
}

# --- Download ai binary ---
if [ -n "${LOCAL_BINARY:-}" ]; then
  # Local test mode: use a pre-built binary instead of downloading from GitHub
  echo "Using local binary: $LOCAL_BINARY"
  cp "$LOCAL_BINARY" "$BIN_DIR/ai"
  chmod +x "$BIN_DIR/ai"
  echo "  -> $BIN_DIR/ai"
  AI_SOURCE="local build from $LOCAL_BINARY"
else
  AI_VERSION="${AI_VERSION:-$KNOWN_GOOD_VERSION}"
  if [ "$AI_VERSION" = "latest" ]; then
    AI_URL="https://github.com/$REPO/releases/latest/download/ai-${OS_NAME}-${ARCH_NAME}"
  else
    AI_URL="https://github.com/$REPO/releases/download/${AI_VERSION}/ai-${OS_NAME}-${ARCH_NAME}"
  fi

  echo "Downloading ai binary ($AI_VERSION)..."
  if curl -fsSL "$AI_URL" -o "$BIN_DIR/ai" 2>/dev/null; then
    chmod +x "$BIN_DIR/ai"
    echo "  -> $BIN_DIR/ai"
    AI_SOURCE="release $AI_VERSION"
  else
    echo "No release found — building from source..."
    if ! command -v go &>/dev/null; then
      echo "Error: Go is required to build from source. Install from https://go.dev/dl/"
      exit 1
    fi
    BUILD_TMPDIR="$(mktemp -d)"
    trap 'rm -rf "$BUILD_TMPDIR"' EXIT
    git clone --depth 1 "https://github.com/$REPO.git" "$BUILD_TMPDIR/ai-sh"
    cd "$BUILD_TMPDIR/ai-sh"
    go build -ldflags "-s -w" -o "$BIN_DIR/ai" .
    chmod +x "$BIN_DIR/ai"
    cd -
    echo "  -> $BIN_DIR/ai (built from source)"
    AI_SOURCE="source build of $REPO@main"
  fi
fi

# --- Choose backend ---
# Cloud skips the llama-cli and model downloads entirely (~1GB+ saved).
BACKEND="${AI_SH_BACKEND:-}"
if [ -z "$BACKEND" ]; then
  if [ -n "${MODEL_CHOICE:-}" ]; then
    # An explicit model choice means the caller wants the local runtime.
    BACKEND="local"
  else
    echo ""
    echo "Where should ai-sh run the model?"
    echo "  1) Local - llama.cpp on this machine, offline, no API key (~1GB download)"
    echo "  2) Cloud - Mistral, Groq, OpenRouter, Cerebras, or any OpenAI-compatible API"
    echo ""
    BACKEND_CHOICE="$(ask 'Enter choice [1-2] (default: 1): ')"
    case "${BACKEND_CHOICE:-1}" in
      2|cloud|Cloud) BACKEND="cloud" ;;
      *)             BACKEND="local" ;;
    esac
  fi
fi

install_llama_cli() {
  LLAMA_TAG="${LLAMA_TAG:-}"
  if [ -z "$LLAMA_TAG" ]; then
    echo "Fetching latest llama.cpp release..."
    LLAMA_TAG="$(curl -sL "https://api.github.com/repos/ggerganov/llama.cpp/releases?per_page=1" \
      | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['tag_name'])")"
    echo "  -> $LLAMA_TAG"
  fi

  if [ "$OS_NAME" = "darwin" ]; then
    if [ "$ARCH_NAME" = "arm64" ]; then
      LLAMA_FILE="llama-${LLAMA_TAG}-bin-macos-arm64.tar.gz"
    else
      LLAMA_FILE="llama-${LLAMA_TAG}-bin-macos-x64.tar.gz"
    fi
  else
    if [ "$ARCH_NAME" = "arm64" ]; then
      LLAMA_FILE="llama-${LLAMA_TAG}-bin-ubuntu-arm64.tar.gz"
    else
      LLAMA_FILE="llama-${LLAMA_TAG}-bin-ubuntu-x64.tar.gz"
    fi
  fi

  LLAMA_URL="https://github.com/ggerganov/llama.cpp/releases/download/${LLAMA_TAG}/${LLAMA_FILE}"

  echo "Downloading llama-cli..."
  LLAMA_TMPDIR="$(mktemp -d)"
  trap 'rm -rf "$LLAMA_TMPDIR"' EXIT

  curl -fL --progress-bar "$LLAMA_URL" -o "$LLAMA_TMPDIR/llama.tar.gz"
  mkdir -p "$LLAMA_TMPDIR/llama"
  tar -xzf "$LLAMA_TMPDIR/llama.tar.gz" -C "$LLAMA_TMPDIR/llama" 2>/dev/null || true

  # Find llama-cli in extracted files
  LLAMA_BIN="$(find "$LLAMA_TMPDIR/llama" -name "llama-cli" -type f | head -1)"
  if [ -z "$LLAMA_BIN" ]; then
    echo "Error: llama-cli not found in downloaded archive"
    exit 1
  fi

  # Copy llama-cli and all companion .so/.dylib files (RUNPATH=$ORIGIN requires co-location)
  LLAMA_DIR="$(dirname "$LLAMA_BIN")"
  cp "$LLAMA_DIR"/llama-cli "$BIN_DIR/llama-cli"
  chmod +x "$BIN_DIR/llama-cli"
  find "$LLAMA_DIR" -name "*.so*" -o -name "*.dylib" | while read -r lib; do
    cp "$lib" "$BIN_DIR/"
  done
  echo "  -> $BIN_DIR/llama-cli (+ shared libs)"
}

install_model() {
  GGUF_COUNT="$(find "$MODELS_DIR" -name "*.gguf" 2>/dev/null | wc -l)"
  if [ "$GGUF_COUNT" -gt 0 ]; then
    echo "Model already present in $MODELS_DIR, skipping."
    return 0
  fi

  echo ""
  echo "Choose a model to download:"
  echo "  1) Tiny      - TinyLlama 1.1B Q4_K_M      (~670MB)  fastest, lowest RAM"
  echo "  2) Qwen 1.5B - Qwen2.5-Coder 1.5B Q4_K_M  (~1.0GB)  recommended (default)"
  echo "  3) Qwen 3B   - Qwen2.5-Coder 3B Q4_K_M   (~2.0GB)  smarter, still fast"
  echo "  4) Skip      - I'll place a model manually"
  echo ""

  if [ -z "${MODEL_CHOICE:-}" ]; then
    MODEL_CHOICE="$(ask 'Enter choice [1-4] (default: 2): ')"
  fi

  MODEL_CHOICE="${MODEL_CHOICE:-2}"

  case "$MODEL_CHOICE" in
    1)
      MODEL_NAME="TinyLlama 1.1B"
      MODEL_FILE="tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf"
      MODEL_URL="https://huggingface.co/TheBloke/TinyLlama-1.1B-Chat-v1.0-GGUF/resolve/main/tinyllama-1.1b-chat-v1.0.Q4_K_M.gguf"
      ;;
    2)
      MODEL_NAME="Qwen2.5-Coder 1.5B"
      MODEL_FILE="qwen2.5-coder-1.5b-instruct-q4_k_m.gguf"
      MODEL_URL="https://huggingface.co/Qwen/Qwen2.5-Coder-1.5B-Instruct-GGUF/resolve/main/qwen2.5-coder-1.5b-instruct-q4_k_m.gguf"
      ;;
    3)
      MODEL_NAME="Qwen2.5-Coder 3B"
      MODEL_FILE="qwen2.5-coder-3b-instruct-q4_k_m.gguf"
      MODEL_URL="https://huggingface.co/Qwen/Qwen2.5-Coder-3B-Instruct-GGUF/resolve/main/qwen2.5-coder-3b-instruct-q4_k_m.gguf"
      ;;
    4|[sS]kip)
      echo "Skipping model download."
      echo "Place any .gguf model in $MODELS_DIR when ready."
      MODEL_URL=""
      MODEL_FILE=""
      ;;
    *)
      echo "Invalid choice, skipping model download."
      MODEL_URL=""
      MODEL_FILE=""
      ;;
  esac

  if [ -n "${MODEL_URL:-}" ]; then
    echo "Downloading $MODEL_NAME..."
    curl -fL --progress-bar "$MODEL_URL" -o "$MODELS_DIR/$MODEL_FILE"
    echo "  -> $MODELS_DIR/$MODEL_FILE"
  fi
}

# configure_cloud hands off to `ai --setup`, which owns the provider list and
# writes ~/.ai-sh/config.json.
#
# AI_SH_HISTORY is inherited rather than passed as a flag: `ai` reads it as a
# config default, so setting it here just preselects the answer to the session
# history question. Nothing about this call depends on a new binary feature, so
# an older published `ai` degrades to simply not asking.
configure_cloud() {
  # The cloud path is only reachable on binaries that know about --setup. If a
  # pinned-down AI_VERSION or a stale build predates it, say so plainly rather
  # than letting `ai --setup` fail with an unknown-flag error.
  # Captured rather than piped into grep: under `set -o pipefail`, grep -q
  # closing the pipe early would surface as a failed pipeline.
  local help_text=""
  help_text="$("$BIN_DIR/ai" --help 2>&1 || true)"
  if ! printf '%s' "$help_text" | grep -q -- "--setup"; then
    echo ""
    echo "Error: the installed ai binary does not support '--setup', which the"
    echo "cloud backend needs to save a provider and API key."
    echo ""
    echo "  installed: $BIN_DIR/ai ($AI_SOURCE)"
    echo "  required:  $KNOWN_GOOD_VERSION or newer"
    echo ""
    echo "Re-run without pinning an older version, or use the local backend:"
    echo "  AI_SH_BACKEND=local"
    exit 1
  fi

  echo ""
  if [ -t 0 ]; then
    "$BIN_DIR/ai" --setup
  elif have_tty; then
    "$BIN_DIR/ai" --setup </dev/tty
  else
    echo "No terminal available for cloud setup. Finish it later with:"
    echo "  ai --setup"
  fi
}

if [ "$BACKEND" = "cloud" ]; then
  configure_cloud
else
  install_llama_cli
  install_model
fi

# --- Add to PATH ---
add_to_path() {
  local rc_file="$1"
  local line='export PATH="$HOME/.ai-sh/bin:$PATH"'

  if [ -f "$rc_file" ] && grep -q "\.ai-sh/bin" "$rc_file"; then
    return 0
  fi

  if [ -f "$rc_file" ]; then
    echo "" >> "$rc_file"
    echo "# ai-sh" >> "$rc_file"
    echo "$line" >> "$rc_file"
    echo "  -> Added PATH to $rc_file"
  fi
}

SHELL_NAME="$(basename "${SHELL:-bash}")"
case "$SHELL_NAME" in
  zsh)
    add_to_path "$HOME/.zshrc"
    ;;
  fish)
    FISH_CONFIG="$HOME/.config/fish/config.fish"
    mkdir -p "$(dirname "$FISH_CONFIG")"
    if ! grep -q "\.ai-sh/bin" "$FISH_CONFIG" 2>/dev/null; then
      echo "" >> "$FISH_CONFIG"
      echo "# ai-sh" >> "$FISH_CONFIG"
      echo 'fish_add_path "$HOME/.ai-sh/bin"' >> "$FISH_CONFIG"
      echo "  -> Added PATH to $FISH_CONFIG"
    fi
    ;;
  *)
    add_to_path "$HOME/.bashrc"
    add_to_path "$HOME/.bash_profile"
    ;;
esac

echo ""
echo "Installation complete!"
echo ""
echo "Restart your shell or run:"
echo "  export PATH=\"\$HOME/.ai-sh/bin:\$PATH\""
echo ""
echo "Usage:"
echo "  ai install numpy"
echo "  ai \"kill process on port 3000\""
echo ""
echo "Switch backends anytime with:  ai --setup"
