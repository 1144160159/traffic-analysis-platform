#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
KUBECTL_BIN="${KUBECTL_BIN:-kubectl}"
NAMESPACE="${TOPIC_FIXTURE_NAMESPACE:-databases}"
FIXTURE_SQL="$REPO_ROOT/scripts/seed_topic_panel_simulations.postgres.sql"
JOB_MANIFEST="$REPO_ROOT/deployments/kubernetes/fixtures/topic-panel-simulations-job.yaml"

if [[ ! -f "$FIXTURE_SQL" || ! -f "$JOB_MANIFEST" ]]; then
  echo "topic fixture SQL or Job manifest is missing" >&2
  exit 1
fi

"$KUBECTL_BIN" -n "$NAMESPACE" create configmap topic-panel-simulations-sql \
  --from-file=seed_topic_panel_simulations.postgres.sql="$FIXTURE_SQL" \
  --dry-run=client -o yaml |
  "$KUBECTL_BIN" apply -f -

"$KUBECTL_BIN" -n "$NAMESPACE" delete job seed-topic-panel-simulations --ignore-not-found
"$KUBECTL_BIN" apply -f "$JOB_MANIFEST"
"$KUBECTL_BIN" -n "$NAMESPACE" wait \
  --for=condition=complete \
  job/seed-topic-panel-simulations \
  --timeout=180s
"$KUBECTL_BIN" -n "$NAMESPACE" logs job/seed-topic-panel-simulations
