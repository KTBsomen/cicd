#!/bin/sh
# ═══════════════════════════════════════════
# CICD — Universal Shell Installer
# ═══════════════════════════════════════════
set -e

# Symmetrical ASCII header
echo "   🌸 CICD — Universal Setup"
echo "   ───────────────────────────────────"

# 1. Detect OS
OS_NAME=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS_NAME" in
  linux*)   OS="linux" ;;
  *)
    echo "❌ Unsupported operating system: $OS_NAME (This daemon requires Linux / systemd support)"
    exit 1
    ;;
esac

# 2. Detect Architecture
ARCH_NAME=$(uname -m)
case "$ARCH_NAME" in
  x86_64|amd64)        ARCH="amd64" ;;
  aarch64|arm64)       ARCH="arm64" ;;
  armv7*|armhf|arm)    ARCH="arm" ;;
  i386|i686|386)       ARCH="386" ;;
  *)
    echo "❌ Unsupported CPU architecture: $ARCH_NAME"
    exit 1
    ;;
esac

BINARY_NAME="cicd-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/KTBsomen/cicd/releases/latest/download/${BINARY_NAME}"

echo "💻 Detected Platform: ${OS} (${ARCH})"
echo "📥 Downloading binary from: ${DOWNLOAD_URL}"

# 3. Download using curl or wget
if command -v curl >/dev/null 2>&1; then
  curl -L -s "$DOWNLOAD_URL" -o cicd
elif command -v wget >/dev/null 2>&1; then
  wget -q "$DOWNLOAD_URL" -O cicd
else
  echo "❌ Error: Please install 'curl' or 'wget' to download the binary."
  exit 1
fi

# 4. Make executable
chmod +x cicd
echo "🚀 Success! The executable binary 'cicd' is ready."
echo "CICD installed at $(pwd)/cicd"
echo "👉 Start the core daemon: sudo ./cicd"
echo "   ───────────────────────────────────"
