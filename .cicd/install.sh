bash
#!/bin/bash

# This script sets up a Node.js development environment.

# Set script name for logging
SCRIPT_NAME=$(basename "$0")

# Function for logging messages
log() {
  local level="$1"
  local message="$2"
  echo -e "[$SCRIPT_NAME] $level: $message"
}

# Function for checking if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
  return $?
}

# Check for necessary system dependencies
log "INFO" "Checking system dependencies..."
if ! command_exists curl; then
  log "INFO" "Installing curl..."
  sudo apt-get update && sudo apt-get install -y curl || { log "ERROR" "Failed to install curl!"; exit 1; }
fi

if ! command_exists wget; then
  log "INFO" "Installing wget..."
  sudo apt-get update && sudo apt-get install -y wget || { log "ERROR" "Failed to install wget!"; exit 1; }
fi

if ! command_exists git; then
  log "INFO" "Installing git..."
  sudo apt-get update && sudo apt-get install -y git || { log "ERROR" "Failed to install git!"; exit 1; }
fi

# Detect operating system
log "INFO" "Detecting operating system..."
if grep -q 'Ubuntu' /etc/lsb-release; then
  OS="Ubuntu"
elif grep -q 'Debian' /etc/lsb-release; then
  OS="Debian"
elif grep -q 'CentOS' /etc/redhat-release; then
  OS="CentOS"
elif grep -q 'Red Hat' /etc/redhat-release; then
  OS="Red Hat"
elif grep -q 'macOS' /System/Library/CoreServices/SystemVersion.plist; then
  OS="macOS"
else
  log "ERROR" "Unsupported operating system!"
  exit 1
fi

# Check for existing Node.js installation
log "INFO" "Checking for Node.js installation..."
if command_exists node; then
  log "INFO" "Node.js found! Checking version..."
  node -v
  log "INFO" "Using existing Node.js installation."
else
  # Install Node.js (using Node Version Manager (nvm))
  log "INFO" "Installing Node.js using nvm..."
  if [ "$OS" == "macOS" ]; then
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash
    source ~/.bashrc
  else
    curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash
    source ~/.bashrc
  fi

  log "INFO" "Installing latest stable Node.js version..."
  nvm install node
  nvm alias default node
  log "INFO" "Node.js installed successfully!"
fi

# Install essential tools and package managers
log "INFO" "Installing essential tools and package managers..."
npm install -g npm yarn pnpm

# Set up the development environment (virtual environments)
log "INFO" "Setting up development environment..."
mkdir -p ~/.config/yarn/global/node_modules
npm config set prefix "~/.config/yarn/global/node_modules"

# Install commonly used libraries and frameworks
log "INFO" "Installing commonly used libraries and frameworks..."
npm install -g express

# Add necessary binary paths to the system PATH
log "INFO" "Adding necessary paths to PATH..."
if [ "$OS" == "macOS" ]; then
  echo 'export PATH="$HOME/.nvm/versions/node/$(nvm version)/bin:$PATH"' >> ~/.zshrc
else
  echo 'export PATH="$HOME/.nvm/versions/node/$(nvm version)/bin:$PATH"' >> ~/.bashrc
fi

# Set up environment variables (optional)
log "INFO" "Setting up environment variables (optional)..."
# echo 'export MY_ENV_VAR="my value"' >> ~/.bashrc
# echo 'export MY_OTHER_ENV_VAR="another value"' >> ~/.zshrc

# Verify installations
log "INFO" "Verifying installations..."
node -v
npm -v
yarn -v
pnpm -v

log "INFO" "Node.js development environment setup completed successfully!"

# Optional installations (comment out if not needed)
# log "INFO" "Installing IDEs and additional tools (optional)..."
# # Install Visual Studio Code
# # ...
# # Install other tools
# # ...

# Instructions for completely removing Node.js and associated components:
# 1. Uninstall Node.js:
#    - If you used nvm: `nvm uninstall node`
#    - If you used a package manager: `sudo apt-get remove nodejs` (Debian/Ubuntu) or `sudo yum remove nodejs` (CentOS/RHEL)
# 2. Remove nvm (optional): `rm -rf $HOME/.nvm`
# 3. Remove global npm packages: `npm uninstall -g <package-name>` (for each package)
# 4. Remove project dependencies: `rm -rf node_modules`
# 5. Remove npm cache: `rm -rf ~/.npm`