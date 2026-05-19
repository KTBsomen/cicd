#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Go Service
# ═══════════════════════════════════════════
set -e

# 1. Run packages test suites
echo "🧪 Running package tests..."
go test ./... || { echo "❌ Go test suites failed! Aborting deployment."; exit 1; }

# 2. Compile static production binary
echo "🔨 Compiling binary..."
go build -ldflags="-s -w" -o app ./cmd/server/main.go || go build -ldflags="-s -w" -o app main.go

# 3. Launch application server in background (Tier 2)
echo "🚀 Starting Go binary..."

# Start the binary in the background and write its process ID (PID)
./app > server.log 2>&1 &
echo $! > "$CICD_PID_FILE"
