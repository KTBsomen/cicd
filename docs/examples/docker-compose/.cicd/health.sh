#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — Docker Compose
# ═══════════════════════════════════════════
set -e

# Query container host port mapping for a successful HTTP response
echo "🔍 Checking application health..."
curl -sf http://localhost:8080/health || curl -sf http://localhost:8080/ || exit 1
