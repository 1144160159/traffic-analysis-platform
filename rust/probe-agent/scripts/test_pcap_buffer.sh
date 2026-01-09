#!/usr/bin/env bash
# PCAP Buffer 测试

set -euo pipefail

cd "$(dirname "$0")/.."

echo "🧪 Running PCAP Buffer tests..."

cargo test --release --package probe-agent -- archiver::buffer --nocapture

echo "✅ PCAP Buffer tests passed"