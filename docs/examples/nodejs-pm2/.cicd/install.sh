#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Node.js (PM2)
# ═══════════════════════════════════════════
set -e

echo "🌸 Starting dependencies installation..."

# Install packages using 'npm ci' (clean install) for deterministic installs
# in production. We use --production to exclude devDependencies.
npm ci --production
