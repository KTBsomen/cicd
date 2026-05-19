#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Docker Compose
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing Docker environment..."

# 1. Install Docker & Compose if missing
if ! command -v docker &> /dev/null; then
  echo "📥 Docker is missing. Installing Docker & Docker Compose..."
  if command -v apt-get &> /dev/null; then
    sudo apt-get update -y
    sudo apt-get install -y docker.io docker-compose-v2
    sudo systemctl enable --now docker
  elif command -v yum &> /dev/null; then
    sudo yum update -y
    sudo yum install -y docker docker-compose-plugin
    sudo systemctl enable --now docker
  else
    echo "⚠️ Package manager not recognized. Please install Docker manually."
    exit 1
  fi
else
  echo "✅ Docker is already installed: $(docker --version)"
fi

echo "🌸 Compiling and building docker images..."
# Build all images configured in docker-compose.yml
docker compose build
