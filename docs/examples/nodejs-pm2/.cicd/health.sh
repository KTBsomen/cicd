#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — Node.js (PM2)
# ═══════════════════════════════════════════
set -e

# Query server local port for a successful HTTP response
echo "🔍 Checking application health..."
curl -sf http://localhost:3000/health || curl -sf http://localhost:3000/ || exit 1
