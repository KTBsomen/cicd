#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Node.js (PM2)
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing Node.js environment..."

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

# 2. Install PM2 if missing
if ! command -v pm2 &> /dev/null; then
  echo "📥 PM2 is missing. Installing PM2 globally..."
  sudo npm install -g pm2
else
  echo "✅ PM2 is already installed: $(pm2 -v)"
fi

echo "🌸 Starting dependencies installation..."
# Install packages using 'npm ci' (clean install) for deterministic installs
# in production. We use --production to exclude devDependencies.
npm ci --production
