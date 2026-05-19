#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Python (Flask)
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing Python virtual environment..."

# 1. Create a virtual environment inside 'venv' if it doesn't exist
if [ ! -d "venv" ]; then
  python3 -m venv venv
fi

# 2. Activate virtualenv and download packages
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
