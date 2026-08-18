#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
NAMESPACE="${TOPIC_FIXTURE_NAMESPACE:-databases}"
FIXTURE_SQL="$REPO_ROOT/scripts/seed_topic_panel_simulations.postgres.sql"
JOB_MANIFEST="$REPO_ROOT/deployments/kubernetes/fixtures/topic-panel-simulations-job.yaml"

# 代理环境会致 kubectl TLS 超时,统一清理代理后调用(agent.md §3)。
kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL_BIN" "$@"
}

if [[ ! -f "$FIXTURE_SQL" || ! -f "$JOB_MANIFEST" ]]; then
  echo "topic fixture SQL or Job manifest is missing" >&2
  exit 1
fi

kctl -n "$NAMESPACE" create configmap topic-panel-simulations-sql \
  --from-file=seed_topic_panel_simulations.postgres.sql="$FIXTURE_SQL" \
  --dry-run=client -o yaml |
  kctl apply -f -

kctl -n "$NAMESPACE" delete job seed-topic-panel-simulations --ignore-not-found
kctl apply -f "$JOB_MANIFEST"
kctl -n "$NAMESPACE" wait \
  --for=condition=complete \
  job/seed-topic-panel-simulations \
  --timeout=180s
kctl -n "$NAMESPACE" logs job/seed-topic-panel-simulations
