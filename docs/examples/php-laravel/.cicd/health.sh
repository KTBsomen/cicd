#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — PHP (Laravel)
# ═══════════════════════════════════════════
set -e

echo "🔍 Checking Laravel service health..."
curl -sf http://localhost:8000/ || exit 1
