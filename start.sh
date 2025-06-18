#!/bin/bash

# CICD Startup Script
# This script ensures the virtual environment is active before running the application

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_PATH="$HOME/.local/venvs/project_env"

echo "CICD Startup Script"
echo "==================="

# Check if virtual environment exists
if [ ! -d "$VENV_PATH" ]; then
    echo "Virtual environment not found. Running dependency manager..."
    cd "$SCRIPT_DIR"
    python3 dependency_manager.py
    
    if [ ! -d "$VENV_PATH" ]; then
        echo "Failed to create virtual environment. Exiting."
        exit 1
    fi
fi

# Activate virtual environment
echo "Activating virtual environment at $VENV_PATH"
source "$VENV_PATH/bin/activate"

# Verify packages are available
echo "Verifying dependencies..."
python -c "
import sys
packages = ['psutil', 'requests', 'flask', 'pymongo', 'flask_socketio']
missing = []
for pkg in packages:
    try:
        __import__(pkg.replace('-', '_'))
        print(f'✓ {pkg}')
    except ImportError:
        missing.append(pkg)
        print(f'✗ {pkg}')

if missing:
    print(f'Missing packages: {missing}')
    print('Installing missing packages...')
    import subprocess
    subprocess.check_call([sys.executable, '-m', 'pip', 'install'] + missing)
"

# Run the main application
echo "Starting CICD application..."
cd "$SCRIPT_DIR"
python main.py "$@"