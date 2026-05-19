#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — PHP (Laravel)
# ═══════════════════════════════════════════
set -e

# 1. Run database migrations
echo "⚙️ Running database migrations..."
php artisan migrate --force || echo "⚠️ Migration failed or not configured, continuing..."

# 2. Optimize Laravel cache
echo "⚡ Optimizing application configuration and routes..."
php artisan config:cache
php artisan route:cache
php artisan view:cache

# 3. Build front-end assets
if [ -f "package.json" ]; then
  echo "🔨 Compiling front-end assets..."
  npm run build
fi

# 4. Start local development server (background)
echo "🚀 Starting Laravel server..."
php artisan serve --host 0.0.0.0 --port 8000 > laravel.log 2>&1 &
echo $! > "$CICD_PID_FILE"
