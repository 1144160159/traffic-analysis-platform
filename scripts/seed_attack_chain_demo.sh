#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixture_dir="${repo_root}/common/sql/ch/fixtures"
recommendation_schema="${repo_root}/common/sql/ch/03-attack-chain-recommendations.sql"
clickhouse_namespace="${ATTACK_CHAIN_CLICKHOUSE_NAMESPACE:-middleware}"
clickhouse_pod="${ATTACK_CHAIN_CLICKHOUSE_POD:-clickhouse-1-0}"
clickhouse_shard_pods="${ATTACK_CHAIN_CLICKHOUSE_SHARD_PODS:-clickhouse-1-0,clickhouse-2-0}"
clickhouse_all_pods="${ATTACK_CHAIN_CLICKHOUSE_ALL_PODS:-clickhouse-1-0,clickhouse-replica-0,clickhouse-2-0,clickhouse-replica-1}"

campaign_fixture="${fixture_dir}/attack-chain-demo-campaign.jsonl"
alert_fixture="${fixture_dir}/attack-chain-demo-alerts.jsonl"
evidence_fixture="${fixture_dir}/attack-chain-demo-evidence.jsonl"
recommendation_fixture="${fixture_dir}/attack-chain-demo-recommendations.jsonl"
for fixture in "${recommendation_schema}" "${campaign_fixture}" "${alert_fixture}" "${evidence_fixture}" "${recommendation_fixture}"; do
  if [[ ! -f "${fixture}" ]]; then
    echo "missing attack-chain fixture: ${fixture}" >&2
    exit 1
  fi
done

IFS=',' read -r -a all_pods <<< "${clickhouse_all_pods}"
for pod in "${all_pods[@]}"; do
  sed 's/ ON CLUSTER traffic_cluster//g' "${recommendation_schema}" | env \
    -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
    -u http_proxy -u https_proxy -u all_proxy \
    kubectl -n "${clickhouse_namespace}" exec -i "${pod}" -c clickhouse -- \
    clickhouse-client --multiquery
done

IFS=',' read -r -a shard_pods <<< "${clickhouse_shard_pods}"
for pod in "${shard_pods[@]}"; do
  for table in evidence alerts campaigns attack_chain_recommendations; do
    predicate="campaign_id='attack-chain-demo-c2-20260726'"
    if [[ "${table}" == "evidence" ]]; then
      predicate="event_id='attack-chain-demo-c2-20260726'"
    fi
    env \
      -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
      -u http_proxy -u https_proxy -u all_proxy \
      kubectl -n "${clickhouse_namespace}" exec "${pod}" -c clickhouse -- \
      clickhouse-client --query "
        ALTER TABLE traffic.${table}_local
        DELETE WHERE tenant_id='default' AND ${predicate}
        SETTINGS mutations_sync=2
      "
  done
done

for table_fixture in \
  "campaigns:${campaign_fixture}" \
  "alerts:${alert_fixture}" \
  "evidence:${evidence_fixture}" \
  "attack_chain_recommendations:${recommendation_fixture}"; do
  table="${table_fixture%%:*}"
  fixture="${table_fixture#*:}"
  env \
    -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
    -u http_proxy -u https_proxy -u all_proxy \
    kubectl -n "${clickhouse_namespace}" exec -i "${clickhouse_pod}" -c clickhouse -- \
    clickhouse-client --query "INSERT INTO traffic.${table} FORMAT JSONEachRow" < "${fixture}"
done

env \
  -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY \
  -u http_proxy -u https_proxy -u all_proxy \
  kubectl -n "${clickhouse_namespace}" exec "${clickhouse_pod}" -c clickhouse -- \
  clickhouse-client --query "
    SELECT
      campaign_id,
      length(attack_phases) AS phases,
      length(alerts) AS campaign_alert_ids,
      (
        SELECT count()
        FROM traffic.alerts
        WHERE tenant_id='default' AND campaign_id='attack-chain-demo-c2-20260726'
      ) AS alert_rows,
      (
        SELECT count()
        FROM traffic.evidence
        WHERE tenant_id='default' AND event_id='attack-chain-demo-c2-20260726'
      ) AS evidence_rows,
      (
        SELECT count()
        FROM traffic.attack_chain_recommendations
        WHERE tenant_id='default' AND campaign_id='attack-chain-demo-c2-20260726'
      ) AS recommendation_rows,
      score
    FROM traffic.campaigns
    WHERE tenant_id='default' AND campaign_id='attack-chain-demo-c2-20260726'
    ORDER BY ts_end DESC
    LIMIT 1
    FORMAT PrettyCompact
  "
