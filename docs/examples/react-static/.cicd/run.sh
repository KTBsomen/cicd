#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — React Static SPA
# ═══════════════════════════════════════════
set -e

# Ensure dependencies exist before running
if ! command -v node &> /dev/null; then
  echo "❌ Error: Node.js is not installed! Aborting run."
  exit 1
fi

# 1. Run unit test suites
echo "🧪 Running frontend tests..."
npm test -- --watchAll=false || { echo "❌ React test suites failed! Aborting deployment."; exit 1; }

# 2. Compile static production build assets
echo "🔨 Compiling React build folder..."
npm run build

# 3. Serve the static folder (Tier 3)
echo "🚀 Booting background static server..."

# Use a simple utility like npx 'serve' or 'http-server' to serve compiled assets.
npx serve -s build -l 5000 > static.log 2>&1 &
echo $! > "$CICD_PID_FILE"
