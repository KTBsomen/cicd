#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — React Static SPA
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing React environment..."

# 1. Install Node.js/NPM if missing
if ! command -v node &> /dev/null; then
  echo "📥 Node.js is missing. Installing Node.js & NPM..."
  if command -v apt-get &> /dev/null; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
  elif command -v yum &> /dev/null; then
    curl -fsSL https://rpm.nodesource.com/setup_20.x | sudo bash -
    sudo yum install -y nodejs
  else
    echo "⚠️ Package manager not recognized. Please install Node.js manually."
    exit 1
  fi
else
  echo "✅ Node.js is already installed: $(node -v)"
fi

echo "🌸 Installing React frontend packages..."
# Run clean install
npm ci
