#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "[INFO] Building all Flink jobs (with tests)..."
cd "$ROOT_DIR"
mvn clean verify -DskipITs=false

echo "[INFO] Building behavior-job Docker image..."
cd "$ROOT_DIR/flink-behavior-job"
docker build -t traffic-platform/behavior-job:latest -f Dockerfile .

echo "[INFO] Done."