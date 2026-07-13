#!/bin/bash
# 一键提交所有 Flink Job（按数据流顺序）
set -euo pipefail
cd "$(dirname "$0")"

echo "=== Submitting All Flink Jobs ==="
JOBS=(submit-session-job.sh submit-feature-job.sh submit-pcap-index-job.sh submit-rule-job.sh submit-cep-job.sh submit-behavior-job.sh submit-alert-generator-job.sh submit-user-behavior-job.sh submit-log-job.sh)
for job in "${JOBS[@]}"; do
  if [ -f "$job" ] && [ -s "$job" ]; then
    echo "--- $job ---"
    case "$job" in
      submit-feature-job.sh|submit-pcap-index-job.sh|submit-cep-job.sh)
        bash "$job" --detached || echo "⚠️  $job failed"
        ;;
      *)
        bash "$job" || echo "⚠️  $job failed"
        ;;
    esac
  fi
done
echo "=== All jobs submitted. Check Flink UI. ==="
