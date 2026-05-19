#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — PHP (Laravel)
# ═══════════════════════════════════════════
set -e

echo "🌸 Initializing environment for PHP & Laravel..."

# 1. Install PHP and extensions if missing
if ! command -v php &> /dev/null; then
  echo "📥 Installing PHP & common extensions..."
  if command -v apt-get &> /dev/null; then
    sudo apt-get update -y
    sudo apt-get install -y php php-cli php-fpm php-mbstring php-xml php-bcmath php-curl php-zip php-mysql unzip
  elif command -v yum &> /dev/null; then
    sudo yum update -y
    sudo yum install -y php php-cli php-fpm php-mbstring php-xml php-bcmath php-curl php-zip php-mysqlnd unzip
  else
    echo "⚠️ Package manager not recognized. Please install PHP manually."
    exit 1
  fi
else
  echo "✅ PHP is already installed: $(php -v | head -n 1)"
fi

# 2. Install Composer if missing
if ! command -v composer &> /dev/null; then
  echo "📥 Installing Composer..."
  curl -sS https://getcomposer.org/installer | php
  sudo mv composer.phar /usr/local/bin/composer
else
  echo "✅ Composer is already installed: $(composer --version)"
fi

# 3. Install NPM/Node if missing (needed for front-end assets)
if ! command -v npm &> /dev/null; then
  echo "📥 Installing Node.js & NPM..."
  if command -v apt-get &> /dev/null; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
    sudo apt-get install -y nodejs
  elif command -v yum &> /dev/null; then
    curl -fsSL https://rpm.nodesource.com/setup_20.x | sudo bash -
    sudo yum install -y nodejs
  fi
else
  echo "✅ NPM is already installed: $(npm -v)"
fi

# 4. Install Composer dependencies
echo "🌸 Installing Composer dependencies..."
composer install --no-interaction --prefer-dist --optimize-autoloader --no-dev

# 5. Install front-end dependencies if package.json exists
if [ -f "package.json" ]; then
  echo "📦 Installing npm packages..."
  npm ci
fi
