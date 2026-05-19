#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — React Static SPA
# ═══════════════════════════════════════════
set -e

# Query React local port for a successful HTTP response
echo "🔍 Checking application health..."
curl -sf http://localhost:5000/ || exit 1
