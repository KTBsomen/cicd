#!/bin/sh
# ═══════════════════════════════════════════
# CICD — Multi-Platform Cross-Compiler Script
# ═══════════════════════════════════════════
set -e

DIST_DIR="dist"
mkdir -p "$DIST_DIR"

echo "🌸 Starting CICD multi-platform compilation..."

# Build targets: OS/ARCH
targets="
linux/amd64
linux/arm64
linux/arm
linux/386
"

# Retrieve version metadata for GitHub releases
VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')

LDFLAGS="-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildTime=${BUILD_TIME}"

for target in $targets; do
  export GOOS=$(echo "$target" | cut -d'/' -f1)
  export GOARCH=$(echo "$target" | cut -d'/' -f2)
  output_name="${DIST_DIR}/cicd-${GOOS}-${GOARCH}"
  
  if [ "$GOOS" = "windows" ]; then
    output_name="${output_name}.exe"
  fi
  
  echo "🔨 Building for ${GOOS}/${GOARCH} -> ${output_name}..."
  env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags="${LDFLAGS}" -o "$output_name" main.go
done

echo "🎉 Compilation complete! Binaries are located in: ${DIST_DIR}/"
