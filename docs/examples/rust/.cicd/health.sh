#!/bin/bash
# ═══════════════════════════════════════════
# CICD Health Hook — Rust
# ═══════════════════════════════════════════
set -e

echo "🔍 Checking Rust service health..."
curl -sf http://localhost:8080/health || curl -sf http://localhost:8080/ || exit 1
