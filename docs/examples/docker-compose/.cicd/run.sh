#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Docker Compose
# ═══════════════════════════════════════════
set -e

# Ensure dependencies exist before running
if ! command -v docker &> /dev/null; then
  echo "❌ Error: Docker is not installed! Aborting run."
  exit 1
fi

# 1. Stop and recreate containers in background
echo "🚀 Starting Docker Compose containers..."
docker compose up -d --force-recreate

# 2. Execute tests inside the running app container (Tiers 4)
# If tests return non-zero, this hooks terminates, triggering rollback!
echo "🧪 Running tests inside app container..."
docker compose exec -T app npm test || { 
  echo "❌ Tests failed inside docker! Cleaning up and aborting.";
  docker compose down;
  exit 1; 
}

# 3. Track container health via custom watch daemon (Tier 4)
# Since docker compose detaches, we run a small loop or monitor the supervisor.
# We write the PID of the tracking bash script so CICD can monitor it.
echo "⚙️ Initializing health monitor loop..."
(
  while true; do
    # Verify container statuses are 'running'
    if [ "$(docker inspect -f '{{.State.Running}}' "$(docker compose ps -q app)")" != "true" ]; then
      echo "❌ App container crashed!"
      exit 1
    fi
    sleep 30
  done
) > compose-monitor.log 2>&1 &

echo $! > "$CICD_PID_FILE"
echo "✅ Docker Compose deployment initialized."
