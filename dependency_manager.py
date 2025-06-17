import subprocess
import importlib
import shutil
import sys
import os
import venv
import importlib.util
from pathlib import Path

# List of required dependencies
REQUIRED_MODULES = ["psutil", "requests", "flask", "pymongo", "flask_socketio"]

# Virtual environment path
VENV_PATH = Path.home() / ".local" / "venvs" / "project_env"

def is_externally_managed():
    """Check if Python environment is externally managed."""
    try:
        # Check for EXTERNALLY-MANAGED file
        python_lib_path = Path(sys.executable).parent.parent / "lib" / f"python{sys.version_info.major}.{sys.version_info.minor}"
        externally_managed_file = python_lib_path / "EXTERNALLY-MANAGED"
        return externally_managed_file.exists()
    except Exception:
        return False

def is_in_venv():
    """Check if we're already running in a virtual environment."""
    return hasattr(sys, 'real_prefix') or (hasattr(sys, 'base_prefix') and sys.base_prefix != sys.prefix)

def create_virtual_environment():
    """Create a virtual environment if it doesn't exist."""
    if not VENV_PATH.exists():
        print(f"Creating virtual environment at {VENV_PATH}")
        try:
            venv.create(VENV_PATH, with_pip=True)
            print("Virtual environment created successfully.")
        except Exception as e:
            print(f"Failed to create virtual environment: {e}")
            sys.exit(1)

def get_venv_executables():
    """Get the Python and pip executables from the virtual environment."""
    if os.name == 'nt':  # Windows
        python_exe = VENV_PATH / "Scripts" / "python.exe"
        pip_exe = VENV_PATH / "Scripts" / "pip.exe"
    else:  # Unix-like systems
        python_exe = VENV_PATH / "bin" / "python"
        pip_exe = VENV_PATH / "bin" / "pip"
    
    return str(python_exe), str(pip_exe)

def restart_in_venv():
    """Restart the current script in the virtual environment."""
    venv_python, _ = get_venv_executables()
    
    print(f"Restarting script in virtual environment...")
    print(f"Using Python: {venv_python}")
    
    # Re-execute the current script with the venv Python
    os.execv(venv_python, [venv_python] + sys.argv)

def check_module_availability(module_name):
    """Check if a module is available in the current Python environment."""
    try:
        importlib.import_module(module_name)
        return True
    except ImportError:
        return False

def install_packages_in_venv():
    """Install missing packages in the virtual environment."""
    venv_python, venv_pip = get_venv_executables()
    
    # Check which modules are missing in the venv
    missing_modules = []
    for mod in REQUIRED_MODULES:
        try:
            result = subprocess.run([venv_python, "-c", f"import {mod}"], 
                                  capture_output=True, text=True)
            if result.returncode != 0:
                missing_modules.append(mod)
        except Exception:
            missing_modules.append(mod)
    
    if missing_modules:
        print(f"Installing missing dependencies in venv: {', '.join(missing_modules)}")
        try:
            subprocess.check_call([venv_pip, "install", *missing_modules])
            print("All dependencies installed successfully in virtual environment.")
        except subprocess.CalledProcessError as e:
            print(f"Failed to install packages in venv: {e}")
            sys.exit(1)

def install_packages_system():
    """Install packages in system Python (for non-externally managed systems)."""
    missing_modules = [mod for mod in REQUIRED_MODULES if not check_module_availability(mod)]
    
    if missing_modules:
        print(f"Installing missing dependencies: {', '.join(missing_modules)}")
        try:
            # Try regular pip install first
            subprocess.check_call([sys.executable, "-m", "pip", "install", *missing_modules])
        except subprocess.CalledProcessError:
            try:
                # Fallback to --user install
                subprocess.check_call([sys.executable, "-m", "pip", "install", "--user", *missing_modules])
            except subprocess.CalledProcessError as e:
                print(f"Failed to install packages: {e}")
                sys.exit(1)
        print("All dependencies installed successfully.")

def setup_environment():
    """Main function to set up the environment and ensure dependencies."""
    
    # If we're dealing with an externally managed environment
    if is_externally_managed():
        print("Detected externally managed Python environment.")
        
        # If we're not already in a venv, we need to create one and restart
        if not is_in_venv():
            print("Not in virtual environment. Setting up...")
            create_virtual_environment()
            install_packages_in_venv()
            restart_in_venv()  # This will restart the script in the venv
        else:
            # We're already in a venv, just ensure packages are installed
            print("Already in virtual environment. Checking dependencies...")
            missing_modules = [mod for mod in REQUIRED_MODULES if not check_module_availability(mod)]
            if missing_modules:
                print(f"Installing missing modules: {', '.join(missing_modules)}")
                subprocess.check_call([sys.executable, "-m", "pip", "install", *missing_modules])
    else:
        # Traditional system - install normally
        print("Using system Python environment.")
        install_packages_system()
    
    print("Environment setup complete. All dependencies are available.")

def ensure_dependencies():
    """Public function to call from  main application."""
    # Check if all required modules are available
    missing_modules = [mod for mod in REQUIRED_MODULES if not check_module_availability(mod)]
    
    if missing_modules:
        print(f"Missing dependencies detected: {', '.join(missing_modules)}")
        setup_environment()
    else:
        print("All dependencies are available.")

# Auto-setup when imported
if __name__ == "__main__":
    setup_environment()
else:
    # When imported as a module, check dependencies
    ensure_dependencies()

