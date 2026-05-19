#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Node.js (PM2)
# ═══════════════════════════════════════════
set -e

# 1. Run automated test suites
echo "🧪 Running unit tests..."
npm test || { echo "❌ Unit tests failed! Aborting deployment."; exit 1; }

# 2. Compile/build project on success
echo "🔨 Running build..."
npm run build --if-present

# 3. Reload or start application under PM2 (Tier 3)
echo "🚀 Deploying with PM2..."
# Delete prior instance if exists, ignoring errors
pm2 delete "$CICD_SERVICE_NAME" 2>/dev/null || true

# Start application and name it with the system service name
pm2 start dist/index.js --name "$CICD_SERVICE_NAME"

# 4. Write PM2 process PID to the orchestrator tracker file
# This allows the runner to track telemetry (CPU, memory, ports)
pm2 pid "$CICD_SERVICE_NAME" > "$CICD_PID_FILE"

echo "✅ App deployed and registered under PM2."
