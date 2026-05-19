#!/bin/sh
# ═══════════════════════════════════════════
# CICD — Production Installer
# Reliable GitHub Release Installer
# ═══════════════════════════════════════════
set -e

echo "🌸 CICD — Installing..."

REPO="KTBsomen/cicd"
BASE_URL="https://github.com/$REPO/releases/latest/download"

# -------------------------
# Detect OS
# -------------------------
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$OS" in
  linux*) OS="linux" ;;
  *)
    echo "❌ Unsupported OS: $OS"
    exit 1
    ;;
esac

# -------------------------
# Detect ARCH
# -------------------------
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  armv7*|armhf|arm) ARCH="arm" ;;
  i386|i686|386) ARCH="386" ;;
  *)
    echo "❌ Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

BINARY="cicd-${OS}-${ARCH}"
URL="${BASE_URL}/${BINARY}"

echo "💻 Platform: ${OS}/${ARCH}"
echo "📦 Downloading: ${BINARY}"

TMP="cicd.tmp"
MAX_RETRY=3
i=1

# -------------------------
# Download with retry
# -------------------------
while [ $i -le $MAX_RETRY ]; do
  echo "🔁 Attempt $i/$MAX_RETRY"

  if curl -L --fail --progress-bar -o "$TMP" "$URL"; then
    echo "✅ Download successful"
    break
  fi

  echo "⚠️ Failed, retrying..."
  i=$((i + 1))
  sleep 1
done

# -------------------------
# Final check
# -------------------------
if [ ! -s "$TMP" ]; then
  echo "❌ Installation failed (download error)"
  exit 1
fi

# -------------------------
# Install binary
# -------------------------
mv "$TMP" cicd
chmod +x cicd

echo ""
echo "🚀 CICD installed successfully!"
echo "📍 Path: $(pwd)/cicd"
echo "👉 Run: sudo ./cicd"
echo ""