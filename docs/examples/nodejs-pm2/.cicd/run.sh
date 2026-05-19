#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Node.js (PM2)
# ═══════════════════════════════════════════
set -e

# Ensure dependencies exist before running
if ! command -v node &> /dev/null; then
  echo "❌ Error: Node.js is not installed! Aborting run."
  exit 1
fi

if ! command -v pm2 &> /dev/null; then
  echo "❌ Error: PM2 is not installed! Aborting run."
  exit 1
fi

echo "🧪 Running unit tests..."
npm test || { echo "❌ Unit tests failed! Aborting deployment."; exit 1; }

echo "🔨 Running build..."
npm run build --if-present

echo "🚀 Deploying with PM2..."
pm2 delete "$CICD_SERVICE_NAME" 2>/dev/null || true
pm2 start dist/index.js --name "$CICD_SERVICE_NAME"
pm2 pid "$CICD_SERVICE_NAME" > "$CICD_PID_FILE"
