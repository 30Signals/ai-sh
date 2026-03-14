#!/usr/bin/env bash
set -euo pipefail

REPO="30Signals/ai-sh"
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

# --- Download ai binary ---
if [ -n "${LOCAL_BINARY:-}" ]; then
  # Local test mode: use a pre-built binary instead of downloading from GitHub
  echo "Using local binary: $LOCAL_BINARY"
  cp "$LOCAL_BINARY" "$BIN_DIR/ai"
  chmod +x "$BIN_DIR/ai"
  echo "  -> $BIN_DIR/ai"
else
  AI_VERSION="${AI_VERSION:-latest}"
  if [ "$AI_VERSION" = "latest" ]; then
    AI_URL="https://github.com/$REPO/releases/latest/download/ai-${OS_NAME}-${ARCH_NAME}"
  else
    AI_URL="https://github.com/$REPO/releases/download/${AI_VERSION}/ai-${OS_NAME}-${ARCH_NAME}"
  fi

  echo "Downloading ai binary..."
  curl -fsSL "$AI_URL" -o "$BIN_DIR/ai"
  chmod +x "$BIN_DIR/ai"
  echo "  -> $BIN_DIR/ai"
fi

# --- Download llama-cli ---
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

# --- Download model ---
GGUF_COUNT="$(find "$MODELS_DIR" -name "*.gguf" 2>/dev/null | wc -l)"
if [ "$GGUF_COUNT" -gt 0 ]; then
  echo "Model already present in $MODELS_DIR, skipping."
else
  echo ""
  echo "Choose a model to download:"
  echo "  1) Tiny      - TinyLlama 1.1B Q4_K_M      (~670MB)  fastest, lowest RAM"
  echo "  2) Qwen      - Qwen2.5-Coder 1.5B Q4_K_M  (~1.0GB)  good for shell commands"
  echo "  3) DeepSeek  - DeepSeek-Coder 1.3B Q4_K_M (~800MB)  recommended (default)"
  echo "  4) Skip      - I'll place a model manually"
  echo ""

  if [ -t 0 ]; then
    read -r -p "Enter choice [1-4] (default: 3): " MODEL_CHOICE
  else
    # Non-interactive (piped install): respect env var or default to deepseek
    MODEL_CHOICE="${MODEL_CHOICE:-3}"
    echo "Non-interactive mode, using choice: $MODEL_CHOICE"
  fi

  MODEL_CHOICE="${MODEL_CHOICE:-3}"

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
      MODEL_NAME="DeepSeek-Coder 1.3B"
      MODEL_FILE="deepseek-coder-1.3b-instruct.Q4_K_M.gguf"
      MODEL_URL="https://huggingface.co/TheBloke/deepseek-coder-1.3b-instruct-GGUF/resolve/main/deepseek-coder-1.3b-instruct.Q4_K_M.gguf"
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
