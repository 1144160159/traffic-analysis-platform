#!/usr/bin/env bash
set -euo pipefail

# Logical-idempotent repair for the alerts_latest ReplacingMergeTree projection.
# Re-running the same window may add physical versions, but FINAL keeps one row
# per (tenant_id, alert_id); the reconciliation gate below prevents silent gaps.
window_hours="${1:-25}"
tenant_id="${2:-default}"
if ! [[ "$window_hours" =~ ^[0-9]+$ ]] || (( window_hours < 1 || window_hours > 720 )); then
  echo "window_hours must be an integer between 1 and 720" >&2
  exit 2
fi

kubectl_cmd=(env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy kubectl)
pod="${CLICKHOUSE_POD:-clickhouse-1-0}"
namespace="${CLICKHOUSE_NAMESPACE:-middleware}"
cutoff_expression="toUnixTimestamp64Milli(now64()) - ${window_hours} * 3600000"

"${kubectl_cmd[@]}" -n "$namespace" exec "$pod" -c clickhouse -- clickhouse-client --query "
  INSERT INTO traffic.alerts_latest
  SELECT *
  FROM traffic.alerts
  WHERE tenant_id = '${tenant_id}' AND last_seen >= ${cutoff_expression}
"

reconciliation=$("${kubectl_cmd[@]}" -n "$namespace" exec "$pod" -c clickhouse -- clickhouse-client --query "
  SELECT
    (SELECT uniqExact(alert_id) FROM traffic.alerts WHERE tenant_id='${tenant_id}' AND last_seen >= ${cutoff_expression}) AS raw_unique,
    (SELECT count() FROM traffic.alerts_latest FINAL WHERE tenant_id='${tenant_id}' AND last_seen >= ${cutoff_expression}) AS latest_rows,
    (SELECT uniqExact(alert_id) FROM traffic.alerts_latest FINAL WHERE tenant_id='${tenant_id}' AND last_seen >= ${cutoff_expression}) AS latest_unique
  FORMAT TSV
")

read -r raw_unique latest_rows latest_unique <<<"$reconciliation"
drift=$(( raw_unique > latest_unique ? raw_unique - latest_unique : latest_unique - raw_unique ))
if (( latest_rows != latest_unique || drift > 20 )); then
  echo "alerts_latest reconciliation failed: raw_unique=$raw_unique latest_rows=$latest_rows latest_unique=$latest_unique drift=$drift" >&2
  exit 1
fi
echo "alerts_latest reconciliation passed: raw_unique=$raw_unique latest_rows=$latest_rows latest_unique=$latest_unique drift=$drift"
