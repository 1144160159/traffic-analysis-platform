#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# K8s-backed pipeline smoke. This intentionally delegates to the live smoke
# harness so the pipeline test uses real APISIX, JWT, PostgreSQL, ClickHouse,
# Flink, and Web UI instead of localhost or mocks.
export ROUNDS="${ROUNDS:-3}"
export GRAPH_CHECK_EVERY="${GRAPH_CHECK_EVERY:-0}"
export LOG_DIR="${LOG_DIR:-.artifacts/e2e}"

exec "${SCRIPT_DIR}/live_100_round_smoke.sh"
