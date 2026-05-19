#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — Go Service
# ═══════════════════════════════════════════
set -e

# Query Go local port for a successful HTTP response
echo "🔍 Checking application health..."
curl -sf http://localhost:8080/health || curl -sf http://localhost:8080/ || exit 1
