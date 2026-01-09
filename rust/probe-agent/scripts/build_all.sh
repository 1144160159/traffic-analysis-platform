#!/usr/bin/env bash
# 构建所有架构

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/.."

echo "🔨 Building Probe Agent for all architectures..."

# x86_64
echo "Building for x86_64..."
cargo build --release --target x86_64-unknown-linux-gnu

# aarch64 (if available)
if rustup target list | grep -q "aarch64-unknown-linux-gnu (installed)"; then
    echo "Building for aarch64..."
    cargo build --release --target aarch64-unknown-linux-gnu
else
    echo "⚠️  aarch64 target not installed, skipping"
fi

echo "✅ Build completed"
ls -lh target/*/release/probe-agent