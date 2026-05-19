#!/bin/bash
# ═══════════════════════════════════════════
# CICD Installation Hook — Rust
# ═══════════════════════════════════════════
set -e

# 1. Ensure Cargo is installed
if ! command -v cargo &> /dev/null; then
  echo "🦀 Installing Rust & Cargo toolchain..."
  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
  source "$HOME/.cargo/env"
else
  echo "🦀 Cargo detected: $(cargo --version)"
fi

# 2. Pre-fetch dependencies
echo "📦 Fetching Cargo crates..."
cargo fetch
