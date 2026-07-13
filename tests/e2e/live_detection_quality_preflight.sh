#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-detection-quality-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-detection-quality-preflight}"
PACKAGE_DIR="${PACKAGE_DIR:-mlops/eval_packages/topic1_blind}"
DETECTION_QUALITY_DIR="${DETECTION_QUALITY_DIR:-doc/02_acceptance/04-detection-quality}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"

SUMMARY="$LOG_DIR/blind-evaluation-summary.json"
REPORT="$LOG_DIR/blind-evaluation-report.md"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

need_cmd git
need_cmd jq
need_cmd python3

mkdir -p "$LOG_DIR" "$DETECTION_QUALITY_DIR"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"

python3 mlops/scripts/evaluate_blind_package.py \
  --package-dir "$PACKAGE_DIR" \
  --output-dir "$LOG_DIR" \
  --run-id "$RUN_ID"

cp "$SUMMARY" "$DETECTION_QUALITY_DIR/detection-quality-preflight-latest.json"
cp "$REPORT" "$DETECTION_QUALITY_DIR/detection-quality-preflight-latest.md"
cp "$LOG_DIR/package-file-inventory.json" "$DETECTION_QUALITY_DIR/package-file-inventory-latest.json"
cp "$LOG_DIR/confusion-matrix.csv" "$DETECTION_QUALITY_DIR/confusion-matrix-latest.csv"
cp "$LOG_DIR/stratum-metrics.csv" "$DETECTION_QUALITY_DIR/stratum-metrics-latest.csv"
cp "$PACKAGE_DIR/dataset-manifest.template.yaml" "$DETECTION_QUALITY_DIR/dataset-manifest.template.yaml"
cp "$PACKAGE_DIR/label-schema.yaml" "$DETECTION_QUALITY_DIR/label-schema.yaml"
cp "$PACKAGE_DIR/metric-definition.md" "$DETECTION_QUALITY_DIR/metric-definition.md"

RESULT="$(jq -r '.result' "$SUMMARY")"
echo "detection-quality-preflight result=$RESULT summary=$SUMMARY"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
