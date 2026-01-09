#!/usr/bin/env bash
# Flow Table 性能测试

set -euo pipefail

cd "$(dirname "$0")/.."

echo "🧪 Running Flow Table tests..."

cargo test --release --package probe-agent -- aggregator::flow_table --nocapture

echo "✅ Flow Table tests passed"