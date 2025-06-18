#!/usr/bin/env python3
"""
Dependency Manager for CICD
Handles automatic installation and management of Python dependencies
"""

import os
import sys
import subprocess
import shutil
from pathlib import Path
import importlib.util

# Required packages for the CICD system
REQUIRED_PACKAGES = [
    'psutil',
    'requests', 
    'flask',
    'pymongo',
    'flask_socketio'
]

def check_package_installed(package_name):
    """Check if a package is already installed"""
    try:
        importlib.import_module(package_name.replace('-', '_'))
        return True
    except ImportError:
        return False

def get_missing_packages():
    """Get list of missing packages"""
    missing = []
    for package in REQUIRED_PACKAGES:
        if not check_package_installed(package):
            missing.append(package)
    return missing

def is_in_virtual_env():
    """Check if we're running in a virtual environment"""
    return hasattr(sys, 'real_prefix') or (
        hasattr(sys, 'base_prefix') and sys.base_prefix != sys.prefix
    )

def is_externally_managed():
    """Check if Python environment is externally managed (like on Ubuntu 23.04+)"""
    try:
        import sysconfig
        stdlib_path = sysconfig.get_path('stdlib')
        marker_file = Path(stdlib_path).parent / 'EXTERNALLY-MANAGED'
        return marker_file.exists()
    except:
        return False

def create_virtual_environment():
    """Create a virtual environment for the project"""
    venv_path = Path.home() / ".local" / "venvs" / "project_env"
    
    print(f"Creating virtual environment at {venv_path}")
    
    # Remove existing broken venv if it exists
    if venv_path.exists():
        print("Removing existing virtual environment...")
        shutil.rmtree(venv_path)
    
    # Create parent directories
    venv_path.parent.mkdir(parents=True, exist_ok=True)
    
    # Create virtual environment
    try:
        subprocess.check_call([
            sys.executable, "-m", "venv", str(venv_path)
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        print(f"✓ Virtual environment created successfully")
    except subprocess.CalledProcessError as e:
        print(f"✗ Failed to create virtual environment: {e}")
        return None
    except FileNotFoundError:
        print("✗ Python venv module not found. Installing python3-venv...")
        try:
            subprocess.check_call([
                "sudo", "apt", "update"
            ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            subprocess.check_call([
                "sudo", "apt", "install", "-y", "python3-venv"
            ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            print("✓ python3-venv installed, retrying venv creation...")
            subprocess.check_call([
                sys.executable, "-m", "venv", str(venv_path)
            ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        except subprocess.CalledProcessError:
            print("✗ Failed to install python3-venv or create venv")
            return None
    
    # Verify the virtual environment was created properly
    pip_path = venv_path / "bin" / "pip"
    python_path = venv_path / "bin" / "python"
    
    if not pip_path.exists() or not python_path.exists():
        print(f"✗ Virtual environment creation incomplete")
        print(f"  pip exists: {pip_path.exists()}")
        print(f"  python exists: {python_path.exists()}")
        return None
    
    return venv_path

def install_packages_in_venv():
    """Install packages in virtual environment"""
    missing_packages = get_missing_packages()
    
    if not missing_packages:
        print("✓ All required packages are already installed")
        return True
    
    print(f"Missing dependencies detected: {', '.join(missing_packages)}")
    
    # Create virtual environment
    venv_path = create_virtual_environment()
    if not venv_path:
        return False
    
    pip_path = venv_path / "bin" / "pip"
    python_path = venv_path / "bin" / "python"
    
    try:
        # Upgrade pip first
        print("Upgrading pip...")
        subprocess.check_call([
            str(python_path), "-m", "pip", "install", "--upgrade", "pip"
        ], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        
        # Install missing packages
        print(f"Installing missing dependencies in venv: {', '.join(missing_packages)}")
        subprocess.check_call([
            str(pip_path), "install"
        ] + missing_packages, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        
        print("✓ All dependencies installed successfully in virtual environment")
        
        # Add venv to Python path for current session
        site_packages = venv_path / "lib" / f"python{sys.version_info.major}.{sys.version_info.minor}" / "site-packages"
        if site_packages.exists() and str(site_packages) not in sys.path:
            sys.path.insert(0, str(site_packages))
        
        return True
        
    except subprocess.CalledProcessError as e:
        print(f"✗ Failed to install packages in virtual environment: {e}")
        return False

def install_packages_user():
    """Install packages using --user flag"""
    missing_packages = get_missing_packages()
    
    if not missing_packages:
        print("✓ All required packages are already installed")
        return True
    
    print(f"Installing missing dependencies with --user: {', '.join(missing_packages)}")
    
    try:
        subprocess.check_call([
            sys.executable, "-m", "pip", "install", "--user"
        ] + missing_packages, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        
        print("✓ All dependencies installed successfully with --user")
        return True
        
    except subprocess.CalledProcessError as e:
        print(f"✗ Failed to install packages with --user: {e}")
        return False

def install_packages_system():
    """Install packages system-wide (requires sudo)"""
    missing_packages = get_missing_packages()
    
    if not missing_packages:
        print("✓ All required packages are already installed")
        return True
    
    print(f"Installing missing dependencies system-wide: {', '.join(missing_packages)}")
    print("⚠️  This requires sudo privileges")
    
    try:
        subprocess.check_call([
            "sudo", sys.executable, "-m", "pip", "install"
        ] + missing_packages)
        
        print("✓ All dependencies installed successfully system-wide")
        return True
        
    except subprocess.CalledProcessError as e:
        print(f"✗ Failed to install packages system-wide: {e}")
        return False

def setup_environment():
    """Set up the Python environment with required dependencies"""
    print("Setting up environment...")
    
    # Check current environment status
    in_venv = is_in_virtual_env()
    externally_managed = is_externally_managed()
    
    print(f"In virtual environment: {in_venv}")
    print(f"Externally managed Python: {externally_managed}")
    
    # Strategy 1: If already in venv, install directly
    if in_venv:
        print("Already in virtual environment, installing packages...")
        missing_packages = get_missing_packages()
        if missing_packages:
            try:
                subprocess.check_call([
                    sys.executable, "-m", "pip", "install"
                ] + missing_packages)
                print("✓ Packages installed in current virtual environment")
                return True
            except subprocess.CalledProcessError:
                print("✗ Failed to install in current virtual environment")
                return False
        return True
    
    # Strategy 2: Try virtual environment approach
    if externally_managed or not in_venv:
        print("Detected externally managed Python environment.")
        print("Not in virtual environment. Setting up...")
        
        if install_packages_in_venv():
            return True
    
    # Strategy 3: Try --user installation
    print("Virtual environment setup failed, trying --user installation...")
    if install_packages_user():
        return True
    
    # Strategy 4: System-wide installation (last resort)
    print("--user installation failed, trying system-wide installation...")
    if install_packages_system():
        return True
    
    print("✗ All installation methods failed")
    return False

def ensure_dependencies():
    """Main function to ensure all dependencies are available"""
    print("CICD Dependency Manager")
    print("=" * 50)
    
    missing_packages = get_missing_packages()
    
    if not missing_packages:
        print("✓ All required dependencies are already available")
        return True
    
    print(f"Missing packages: {', '.join(missing_packages)}")
    
    success = setup_environment()
    
    if success:
        print("=" * 50)
        print("✓ Environment setup completed successfully")
        
        # Verify installation
        still_missing = get_missing_packages()
        if still_missing:
            print(f"⚠️  Some packages may still be missing: {', '.join(still_missing)}")
            print("You may need to restart your Python session or check your PYTHONPATH")
            return False
        else:
            print("✓ All dependencies verified and available")
            return True
    else:
        print("=" * 50)
        print("✗ Environment setup failed")
        print("\nManual installation options:")
        print(f"1. pip install --user {' '.join(missing_packages)}")
        print(f"2. sudo pip install {' '.join(missing_packages)}")
        print("3. Create and activate a virtual environment manually:")
        print("   python3 -m venv ~/.local/venvs/project_env")
        print("   source ~/.local/venvs/project_env/bin/activate")
        print(f"   pip install {' '.join(missing_packages)}")
        return False

if __name__ == "__main__":
    ensure_dependencies()