#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Python (Flask)
# ═══════════════════════════════════════════
set -e

# Ensure dependencies exist before running
if ! command -v python3 &> /dev/null; then
  echo "❌ Error: Python 3 is not installed! Aborting run."
  exit 1
fi

# 1. Activate venv
source venv/bin/activate

# 2. Run test suite
echo "🧪 Running unit tests with pytest..."
pytest || { echo "❌ Pytest suites failed! Aborting deployment."; exit 1; }

# 3. Apply database migrations
echo "⚙️ Running database migrations..."
flask db upgrade || python manage.py db upgrade || echo "⚠️ No migration scripts detected, continuing..."

# 4. Run application via Gunicorn (Tier 2/3)
echo "🚀 Starting Flask app with Gunicorn..."

# Run Gunicorn in background, binding to port 8000 and writing PID to $CICD_PID_FILE
gunicorn --workers 3 --bind 0.0.0.0:8000 --pid "$CICD_PID_FILE" app:app
