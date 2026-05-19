#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Go Service
# ═══════════════════════════════════════════
set -e

echo "🌸 Fetching Go dependency modules..."

# Download all Go dependencies listed in go.mod
go mod download
