#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Python (Flask)
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing Python environment..."

# 1. Install Python 3 and common packages if missing
if ! command -v python3 &> /dev/null; then
  echo "📥 Python 3 is missing. Installing Python 3 & pip..."
  if command -v apt-get &> /dev/null; then
    sudo apt-get update -y
    sudo apt-get install -y python3 python3-pip python3-venv
  elif command -v yum &> /dev/null; then
    sudo yum update -y
    sudo yum install -y python3 python3-pip
  else
    echo "⚠️ Package manager not recognized. Please install Python 3 manually."
    exit 1
  fi
else
  echo "✅ Python 3 is already installed: $(python3 --version)"
fi

echo "🌸 Initializing Python virtual environment..."

# 2. Create a virtual environment inside 'venv' if it doesn't exist
if [ ! -d "venv" ]; then
  python3 -m venv venv
fi

# 3. Activate virtualenv and download packages
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
