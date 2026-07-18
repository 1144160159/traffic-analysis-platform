#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-external-secret-operator-canary}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-external-secret-operator-canary}"
KUBECTL="${KUBECTL:-kubectl}"
HELM="${HELM:-helm}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
INSTALL_ESO="${INSTALL_ESO:-true}"
ESO_CHART_VERSION="${ESO_CHART_VERSION:-2.7.0}"
ESO_CHART_REF="${ESO_CHART_REF:-external-secrets/external-secrets}"
ESO_APP_VERSION="${ESO_APP_VERSION:-v2.7.0}"
ESO_IMAGE_REPOSITORY="${ESO_IMAGE_REPOSITORY:-ghcr.io/external-secrets/external-secrets}"
ESO_IMAGE_DIGEST="${ESO_IMAGE_DIGEST:-sha256:04b0d005dc52fbfd92b1673011ae296772cde67fe57e8771b580a7e290e26b31}"
ESO_NAMESPACE="${ESO_NAMESPACE:-external-secrets}"
ESO_RELEASE="${ESO_RELEASE:-external-secrets}"
CANARY_NAMESPACE="${CANARY_NAMESPACE:-external-secrets-canary}"
CANARY_FILE="${CANARY_FILE:-deployments/kubernetes/security/external-secrets-canary.yaml}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"

REPORT="$LOG_DIR/live-external-secret-operator-canary-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-external-secret-operator-canary-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
POST_RENDERER="$LOG_DIR/eso-digest-post-renderer.sh"

mkdir -p "$LOG_DIR" "$SECURITY_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

hctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$HELM" "$@"
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

b64_hash() {
  if [[ -n "$1" ]]; then
    printf '%s' "$1" | sha256sum | awk '{print $1}'
  else
    printf ''
  fi
}

need_cmd jq
need_cmd sha256sum
need_cmd "$KUBECTL"
need_cmd "$HELM"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ ! -s "$CANARY_FILE" ]]; then
  json_log "repo" "ExternalSecret canary manifest present" "blocker" false "missing" "$CANARY_FILE" "$CANARY_FILE"
else
  json_log "repo" "ExternalSecret canary manifest present" "info" true "ok" "$CANARY_FILE" "$CANARY_FILE"
fi

cat >"$POST_RENDERER" <<EOF
#!/usr/bin/env bash
set -euo pipefail
sed 's#${ESO_IMAGE_REPOSITORY}:${ESO_APP_VERSION}#${ESO_IMAGE_REPOSITORY}@${ESO_IMAGE_DIGEST}#g'
EOF
chmod +x "$POST_RENDERER"

set +e
hctl repo add external-secrets https://charts.external-secrets.io >"$LOG_DIR/helm-repo-add.out" 2>"$LOG_DIR/helm-repo-add.err"
HELM_REPO_ADD_RC=$?
hctl repo update external-secrets >"$LOG_DIR/helm-repo-update.out" 2>"$LOG_DIR/helm-repo-update.err"
HELM_REPO_UPDATE_RC=$?
set -e
if [[ "$HELM_REPO_ADD_RC" -eq 0 || "$HELM_REPO_ADD_RC" -eq 1 ]]; then
  json_log "helm" "External Secrets Helm repository configured" "info" true "ok" "repo-add rc=$HELM_REPO_ADD_RC update rc=$HELM_REPO_UPDATE_RC" "helm-repo-update.out"
else
  json_log "helm" "External Secrets Helm repository configured" "blocker" false "rc=$HELM_REPO_ADD_RC" "$(head -c 500 "$LOG_DIR/helm-repo-add.err" | tr '\n' ' ')" "helm-repo-add.err"
fi

HELM_INSTALL_RC=0
if [[ "$INSTALL_ESO" == "true" ]]; then
  set +e
  hctl upgrade --install "$ESO_RELEASE" "$ESO_CHART_REF" \
    --namespace "$ESO_NAMESPACE" \
    --create-namespace \
    --version "$ESO_CHART_VERSION" \
    --wait \
    --timeout 5m \
    --post-renderer "$POST_RENDERER" \
    >"$LOG_DIR/helm-upgrade-install.out" 2>"$LOG_DIR/helm-upgrade-install.err"
  HELM_INSTALL_RC=$?
  set -e
  if [[ "$HELM_INSTALL_RC" -eq 0 ]]; then
    json_log "helm" "External Secrets Operator Helm install/upgrade" "info" true "ok" "chart=$ESO_CHART_VERSION image=${ESO_IMAGE_REPOSITORY}@${ESO_IMAGE_DIGEST}" "helm-upgrade-install.out"
  else
    json_log "helm" "External Secrets Operator Helm install/upgrade" "blocker" false "rc=$HELM_INSTALL_RC" "$(head -c 800 "$LOG_DIR/helm-upgrade-install.err" | tr '\n' ' ')" "helm-upgrade-install.err"
  fi
else
  json_log "helm" "External Secrets Operator Helm install/upgrade" "info" true "skipped" "INSTALL_ESO=false" ""
fi

set +e
hctl -n "$ESO_NAMESPACE" status "$ESO_RELEASE" >"$LOG_DIR/helm-status.out" 2>"$LOG_DIR/helm-status.err"
HELM_STATUS_RC=$?
kctl -n "$ESO_NAMESPACE" rollout status deploy/external-secrets --timeout=180s >"$LOG_DIR/rollout-external-secrets.out" 2>"$LOG_DIR/rollout-external-secrets.err"
ROLLOUT_CONTROLLER_RC=$?
kctl -n "$ESO_NAMESPACE" rollout status deploy/external-secrets-webhook --timeout=180s >"$LOG_DIR/rollout-webhook.out" 2>"$LOG_DIR/rollout-webhook.err"
ROLLOUT_WEBHOOK_RC=$?
kctl -n "$ESO_NAMESPACE" rollout status deploy/external-secrets-cert-controller --timeout=180s >"$LOG_DIR/rollout-cert-controller.out" 2>"$LOG_DIR/rollout-cert-controller.err"
ROLLOUT_CERT_RC=$?
kctl get crd externalsecrets.external-secrets.io secretstores.external-secrets.io clustersecretstores.external-secrets.io -o json >"$LOG_DIR/eso-crds.json" 2>"$LOG_DIR/eso-crds.err"
ESO_CRDS_RC=$?
kctl -n "$ESO_NAMESPACE" get deploy external-secrets external-secrets-webhook external-secrets-cert-controller -o json >"$LOG_DIR/eso-deployments.json" 2>"$LOG_DIR/eso-deployments.err"
ESO_DEPLOYMENTS_RC=$?
set -e

if [[ "$HELM_STATUS_RC" -eq 0 ]]; then
  json_log "live" "External Secrets Helm release status readable" "info" true "ok" "$ESO_NAMESPACE/$ESO_RELEASE" "helm-status.out"
else
  json_log "live" "External Secrets Helm release status readable" "blocker" false "rc=$HELM_STATUS_RC" "$(head -c 500 "$LOG_DIR/helm-status.err" | tr '\n' ' ')" "helm-status.err"
fi

READY_DEPLOYMENTS=0
PINNED_IMAGE_COUNT=0
UNPINNED_IMAGE_COUNT=3
if [[ "$ESO_DEPLOYMENTS_RC" -eq 0 ]]; then
  jq '[
    .items[]
    | {
        name: .metadata.name,
        namespace: .metadata.namespace,
        ready: ((.status.readyReplicas // 0) >= (.spec.replicas // 1)),
        images: [(.spec.template.spec.containers // [])[] | .image]
      }
  ]' "$LOG_DIR/eso-deployments.json" >"$LOG_DIR/eso-deployment-summary.json"
  READY_DEPLOYMENTS="$(jq '[.[] | select(.ready == true)] | length' "$LOG_DIR/eso-deployment-summary.json")"
  PINNED_IMAGE_COUNT="$(jq '[.[] | .images[] | select(contains("@sha256:"))] | length' "$LOG_DIR/eso-deployment-summary.json")"
  UNPINNED_IMAGE_COUNT="$(jq '[.[] | .images[] | select(contains("@sha256:") | not)] | length' "$LOG_DIR/eso-deployment-summary.json")"
fi
if [[ "$ROLLOUT_CONTROLLER_RC" -eq 0 && "$ROLLOUT_WEBHOOK_RC" -eq 0 && "$ROLLOUT_CERT_RC" -eq 0 && "$READY_DEPLOYMENTS" -eq 3 ]]; then
  json_log "live" "External Secrets Operator deployments are ready" "info" true "ok" "ready_deployments=$READY_DEPLOYMENTS" "eso-deployment-summary.json"
else
  json_log "live" "External Secrets Operator deployments are ready" "blocker" false "not_ready" "rollout_rcs=$ROLLOUT_CONTROLLER_RC/$ROLLOUT_WEBHOOK_RC/$ROLLOUT_CERT_RC ready_deployments=$READY_DEPLOYMENTS" "eso-deployment-summary.json"
fi
if [[ "$UNPINNED_IMAGE_COUNT" -eq 0 && "$PINNED_IMAGE_COUNT" -ge 3 ]]; then
  json_log "live" "External Secrets Operator images are digest-pinned" "info" true "ok" "$PINNED_IMAGE_COUNT pinned images" "eso-deployment-summary.json"
else
  json_log "live" "External Secrets Operator images are digest-pinned" "blocker" false "unpinned" "$UNPINNED_IMAGE_COUNT unpinned images" "eso-deployment-summary.json"
fi
if [[ "$ESO_CRDS_RC" -eq 0 ]]; then
  json_log "live" "External Secrets CRDs established" "info" true "ok" "externalsecrets/secretstores/clustersecretstores" "eso-crds.json"
else
  json_log "live" "External Secrets CRDs established" "blocker" false "rc=$ESO_CRDS_RC" "$(head -c 500 "$LOG_DIR/eso-crds.err" | tr '\n' ' ')" "eso-crds.err"
fi

set +e
kctl apply --dry-run=server -f "$CANARY_FILE" >"$LOG_DIR/canary-server-dry-run.out" 2>"$LOG_DIR/canary-server-dry-run.err"
CANARY_DRY_RUN_RC=$?
set -e
if [[ "$CANARY_DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "ExternalSecret canary manifest server dry-run" "info" true "ok" "$CANARY_FILE" "canary-server-dry-run.out"
else
  json_log "repo" "ExternalSecret canary manifest server dry-run" "blocker" false "rc=$CANARY_DRY_RUN_RC" "$(head -c 800 "$LOG_DIR/canary-server-dry-run.err" | tr '\n' ' ')" "canary-server-dry-run.err"
fi

set +e
kctl apply -f "$CANARY_FILE" >"$LOG_DIR/canary-apply.out" 2>"$LOG_DIR/canary-apply.err"
CANARY_APPLY_RC=$?
set -e
if [[ "$CANARY_APPLY_RC" -eq 0 ]]; then
  json_log "live" "ExternalSecret canary support resources applied" "info" true "ok" "$CANARY_FILE" "canary-apply.out"
else
  json_log "live" "ExternalSecret canary support resources applied" "blocker" false "rc=$CANARY_APPLY_RC" "$(head -c 800 "$LOG_DIR/canary-apply.err" | tr '\n' ' ')" "canary-apply.err"
fi

CANARY_TOKEN="traffic-platform-eso-canary-$RUN_ID"
set +e
kctl -n "$CANARY_NAMESPACE" create secret generic eso-canary-source \
  --from-literal=canary-token="$CANARY_TOKEN" \
  --dry-run=client -o yaml >"$LOG_DIR/eso-canary-source-secret.rendered.yaml" 2>"$LOG_DIR/eso-canary-source-secret.render.err"
SECRET_RENDER_RC=$?
if [[ "$SECRET_RENDER_RC" -eq 0 ]]; then
  kctl apply -f "$LOG_DIR/eso-canary-source-secret.rendered.yaml" >"$LOG_DIR/eso-canary-source-secret.apply.out" 2>"$LOG_DIR/eso-canary-source-secret.apply.err"
  SOURCE_SECRET_APPLY_RC=$?
else
  SOURCE_SECRET_APPLY_RC=1
fi
set -e
rm -f "$LOG_DIR/eso-canary-source-secret.rendered.yaml"
if [[ "$SECRET_RENDER_RC" -eq 0 && "$SOURCE_SECRET_APPLY_RC" -eq 0 ]]; then
  json_log "live" "ExternalSecret canary source secret applied" "info" true "ok" "namespace=$CANARY_NAMESPACE secret=eso-canary-source" "eso-canary-source-secret.apply.out"
else
  json_log "live" "ExternalSecret canary source secret applied" "blocker" false "rc=$SECRET_RENDER_RC/$SOURCE_SECRET_APPLY_RC" "namespace=$CANARY_NAMESPACE secret=eso-canary-source" "eso-canary-source-secret.apply.err"
fi

SECRETSTORE_READY=false
EXTERNALSECRET_READY=false
TARGET_SECRET_EXISTS=false
CANARY_DATA_MATCH=false
SOURCE_B64=""
TARGET_B64=""
for _ in $(seq 1 90); do
  set +e
  kctl -n "$CANARY_NAMESPACE" get secretstore traffic-platform-canary-store -o json >"$LOG_DIR/canary-secretstore.json" 2>"$LOG_DIR/canary-secretstore.err"
  SECRETSTORE_RC=$?
  kctl -n "$CANARY_NAMESPACE" get externalsecret traffic-platform-eso-canary -o json >"$LOG_DIR/canary-externalsecret.json" 2>"$LOG_DIR/canary-externalsecret.err"
  EXTERNALSECRET_RC=$?
  SOURCE_B64="$(kctl -n "$CANARY_NAMESPACE" get secret eso-canary-source -o jsonpath='{.data.canary-token}' 2>/dev/null)"
  SOURCE_SECRET_RC=$?
  TARGET_B64="$(kctl -n "$CANARY_NAMESPACE" get secret eso-canary-target -o jsonpath='{.data.canary-token}' 2>/dev/null)"
  TARGET_SECRET_RC=$?
  set -e

  if [[ "$SECRETSTORE_RC" -eq 0 ]]; then
    SECRETSTORE_READY="$(jq -r '([.status.conditions[]? | select(.type == "Ready" and .status == "True")] | length) > 0' "$LOG_DIR/canary-secretstore.json")"
  fi
  if [[ "$EXTERNALSECRET_RC" -eq 0 ]]; then
    EXTERNALSECRET_READY="$(jq -r '([.status.conditions[]? | select(((.type == "Ready") or (.type == "Synced")) and .status == "True")] | length) > 0' "$LOG_DIR/canary-externalsecret.json")"
  fi
  if [[ "$TARGET_SECRET_RC" -eq 0 && -n "$TARGET_B64" ]]; then
    TARGET_SECRET_EXISTS=true
  fi
  if [[ "$SOURCE_SECRET_RC" -eq 0 && "$TARGET_SECRET_RC" -eq 0 && -n "$SOURCE_B64" && "$SOURCE_B64" == "$TARGET_B64" ]]; then
    CANARY_DATA_MATCH=true
  fi
  if [[ "$SECRETSTORE_READY" == "true" && "$EXTERNALSECRET_READY" == "true" && "$TARGET_SECRET_EXISTS" == "true" && "$CANARY_DATA_MATCH" == "true" ]]; then
    break
  fi
  sleep 2
done

SOURCE_HASH="$(b64_hash "$SOURCE_B64")"
TARGET_HASH="$(b64_hash "$TARGET_B64")"
jq -n \
  --arg run_id "$RUN_ID" \
  --arg namespace "$CANARY_NAMESPACE" \
  --arg source_secret "eso-canary-source" \
  --arg target_secret "eso-canary-target" \
  --arg source_b64_sha256 "$SOURCE_HASH" \
  --arg target_b64_sha256 "$TARGET_HASH" \
  --argjson secretstore_ready "$SECRETSTORE_READY" \
  --argjson externalsecret_ready "$EXTERNALSECRET_READY" \
  --argjson target_secret_exists "$TARGET_SECRET_EXISTS" \
  --argjson canary_data_match "$CANARY_DATA_MATCH" \
  '{
    run_id:$run_id,
    namespace:$namespace,
    source_secret:$source_secret,
    target_secret:$target_secret,
    source_b64_sha256:$source_b64_sha256,
    target_b64_sha256:$target_b64_sha256,
    secretstore_ready:$secretstore_ready,
    externalsecret_ready:$externalsecret_ready,
    target_secret_exists:$target_secret_exists,
    canary_data_match:$canary_data_match,
    canary_reconciled:($secretstore_ready and $externalsecret_ready and $target_secret_exists and $canary_data_match)
  }' >"$LOG_DIR/canary-reconciliation-summary.json"

if [[ "$SECRETSTORE_READY" == "true" && "$EXTERNALSECRET_READY" == "true" && "$TARGET_SECRET_EXISTS" == "true" && "$CANARY_DATA_MATCH" == "true" ]]; then
  json_log "live" "ExternalSecret canary reconciles source to target Secret" "info" true "ok" "source_hash=$SOURCE_HASH target_hash=$TARGET_HASH" "canary-reconciliation-summary.json"
else
  json_log "live" "ExternalSecret canary reconciles source to target Secret" "blocker" false "not_reconciled" "secretstore=$SECRETSTORE_READY externalsecret=$EXTERNALSECRET_READY target=$TARGET_SECRET_EXISTS data_match=$CANARY_DATA_MATCH" "canary-reconciliation-summary.json"
fi

kctl -n "$ESO_NAMESPACE" get pods,deploy -o json >"$LOG_DIR/eso-runtime.json" 2>"$LOG_DIR/eso-runtime.err" || true
kctl -n "$CANARY_NAMESPACE" get secretstore,externalsecret,secret -o json >"$LOG_DIR/canary-runtime.json" 2>"$LOG_DIR/canary-runtime.err" || true

TOTAL="$(wc -l <"$REPORT" | tr -d ' ')"
BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.passed == false and .severity == "warn")] | length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  --arg eso_chart_version "$ESO_CHART_VERSION" \
  --arg eso_chart_ref "$ESO_CHART_REF" \
  --arg eso_image "${ESO_IMAGE_REPOSITORY}@${ESO_IMAGE_DIGEST}" \
  --arg canary_file "$CANARY_FILE" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson helm_install_rc "$HELM_INSTALL_RC" \
  --argjson ready_deployments "$READY_DEPLOYMENTS" \
  --argjson pinned_image_count "$PINNED_IMAGE_COUNT" \
  --argjson unpinned_image_count "$UNPINNED_IMAGE_COUNT" \
  --argjson canary_secretstore_ready "$SECRETSTORE_READY" \
  --argjson canary_externalsecret_ready "$EXTERNALSECRET_READY" \
  --argjson canary_target_secret_exists "$TARGET_SECRET_EXISTS" \
  --argjson canary_data_match "$CANARY_DATA_MATCH" \
  --slurpfile canary "$LOG_DIR/canary-reconciliation-summary.json" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    eso_chart_version:$eso_chart_version,
    eso_chart_ref:$eso_chart_ref,
    eso_image:$eso_image,
    canary_file:$canary_file,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    helm_install_rc:$helm_install_rc,
    ready_deployments:$ready_deployments,
    pinned_image_count:$pinned_image_count,
    unpinned_image_count:$unpinned_image_count,
    canary_secretstore_ready:$canary_secretstore_ready,
    canary_externalsecret_ready:$canary_externalsecret_ready,
    canary_target_secret_exists:$canary_target_secret_exists,
    canary_data_match:$canary_data_match,
    canary_reconciliation:($canary[0] // {}),
    checks:.
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# ExternalSecret Operator Canary

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This live canary installs or verifies External Secrets Operator and proves one
operator-backed Secret reconciliation without touching production
\`traffic-credentials\` or Kafka TLS secrets.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| ESO ready deployments | $READY_DEPLOYMENTS |
| ESO digest-pinned images | $PINNED_IMAGE_COUNT |
| ESO unpinned images | $UNPINNED_IMAGE_COUNT |
| Canary SecretStore ready | $SECRETSTORE_READY |
| Canary ExternalSecret ready | $EXTERNALSECRET_READY |
| Canary target Secret exists | $TARGET_SECRET_EXISTS |
| Canary source/target data match | $CANARY_DATA_MATCH |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`eso-deployment-summary.json\`
- \`eso-crds.json\`
- \`canary-reconciliation-summary.json\`
- \`canary-secretstore.json\`
- \`canary-externalsecret.json\`
EOF

cp "$SUMMARY" "$SECURITY_DIR/external-secret-operator-canary-latest.json"
cp "$LOCAL_REPORT" "$SECURITY_DIR/external-secret-operator-canary-latest.md"
cp "$LOG_DIR/eso-deployment-summary.json" "$SECURITY_DIR/external-secret-operator-deployments-latest.json" 2>/dev/null || true
cp "$LOG_DIR/eso-crds.json" "$SECURITY_DIR/external-secret-operator-crds-latest.json" 2>/dev/null || true
cp "$LOG_DIR/canary-reconciliation-summary.json" "$SECURITY_DIR/external-secret-operator-canary-reconciliation-latest.json"
cp "$LOG_DIR/canary-secretstore.json" "$SECURITY_DIR/external-secret-operator-canary-secretstore-latest.json" 2>/dev/null || true
cp "$LOG_DIR/canary-externalsecret.json" "$SECURITY_DIR/external-secret-operator-canary-externalsecret-latest.json" 2>/dev/null || true

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
