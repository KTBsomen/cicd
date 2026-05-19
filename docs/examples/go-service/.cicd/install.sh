#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Go Service
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing Go environment..."

# 1. Install Go if missing
if ! command -v go &> /dev/null; then
  echo "📥 Go is missing. Installing Go compiler..."
  if command -v apt-get &> /dev/null; then
    sudo apt-get update -y
    sudo apt-get install -y golang
  elif command -v yum &> /dev/null; then
    sudo yum update -y
    sudo yum install -y golang
  else
    echo "⚠️ Package manager not recognized. Please install Go manually."
    exit 1
  fi
else
  echo "✅ Go compiler is already installed: $(go version)"
fi

echo "🌸 Fetching Go dependency modules..."
# Download all Go dependencies listed in go.mod
go mod download
