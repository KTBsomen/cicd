#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — Python (Flask)
# ═══════════════════════════════════════════
set -e

# Query Flask local port for a successful HTTP response
echo "🔍 Checking application health..."
curl -sf http://localhost:8000/health || curl -sf http://localhost:8000/ || exit 1
