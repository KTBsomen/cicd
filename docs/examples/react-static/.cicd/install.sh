#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — React Static SPA
# ═══════════════════════════════════════════
set -e

echo "🌸 Installing React frontend packages..."

# Run clean install
npm ci
