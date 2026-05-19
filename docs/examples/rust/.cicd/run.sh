#!/bin/bash
# ═══════════════════════════════════════════
# CICD Run Hook — Rust
# ═══════════════════════════════════════════
set -e

# Make sure cargo env is loaded
if [ -f "$HOME/.cargo/env" ]; then
  source "$HOME/.cargo/env"
fi

# 1. Run test suite before building
echo "🧪 Running unit & integration tests..."
cargo test || { echo "❌ Rust test suite failed! Aborting deployment."; exit 1; }

# 2. Build release binary
echo "🔨 Building release binary..."
cargo build --release

# 3. Start Rust application (background)
echo "🚀 Booting Rust service..."
# Assuming binary is named 'rust-service' (matching your cargo project name)
./target/release/rust-service > rust.log 2>&1 &
echo $! > "$CICD_PID_FILE"
