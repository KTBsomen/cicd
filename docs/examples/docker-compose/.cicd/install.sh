#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Docker Compose
# ═══════════════════════════════════════════
set -e

echo "🌸 Compiling and building docker images..."

# Build all images configured in docker-compose.yml
docker compose build
