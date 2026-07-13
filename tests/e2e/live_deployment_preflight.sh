#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-deployment-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-deployment-preflight}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
DEPLOYMENT_DIR="${DEPLOYMENT_DIR:-doc/02_acceptance/07-deployment}"
SITE_VALUES="${SITE_VALUES:-deployments/kubernetes/site-values.template.yaml}"
IMAGE_LOCK="${IMAGE_LOCK:-deployments/kubernetes/image-digests.lock.json}"

REPORT="$LOG_DIR/live-deployment-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-deployment-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
PACKAGE_MANIFEST="$LOG_DIR/release-package-manifest.json"

mkdir -p "$LOG_DIR" "$DEPLOYMENT_DIR"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 800 "$file" | tr '\n' ' '
  fi
}

need_cmd git
need_cmd jq
need_cmd sha256sum
need_cmd python3
need_cmd curl
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

if [[ -s "$SITE_VALUES" ]]; then
  json_log "repo" "site values template present" "info" true "ok" "$SITE_VALUES" "site-values.template.yaml"
  cp "$SITE_VALUES" "$LOG_DIR/site-values.template.yaml"
else
  json_log "repo" "site values template present" "blocker" false "missing" "$SITE_VALUES" "site-values.template.yaml"
fi

set +e
kctl apply --dry-run=client \
  -f deployments/kubernetes/applications \
  -f deployments/kubernetes/configmaps \
  -f deployments/kubernetes/infrastructure \
  -f deployments/kubernetes/init-jobs \
  -f deployments/kubernetes/flink \
  >"$LOG_DIR/k8s-core-dry-run.txt" 2>"$LOG_DIR/k8s-core-dry-run.err"
DRY_RUN_RC=$?
set -e
if [[ "$DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "core K8s manifests client dry-run" "info" true "ok" "$(wc -l <"$LOG_DIR/k8s-core-dry-run.txt" | tr -d ' ') objects" "k8s-core-dry-run.txt"
else
  json_log "repo" "core K8s manifests client dry-run" "blocker" false "rc=$DRY_RUN_RC" "$(trim_file "$LOG_DIR/k8s-core-dry-run.err")" "k8s-core-dry-run.err"
fi

mapfile -d '' PACKAGE_FILES < <(
  find \
    deployments/kubernetes \
    common/kafka \
    common/sql \
    proto/traffic/v1 \
    rules \
    mlops/workflows \
    tests/e2e \
    tests/chaos \
    web/ui/src/routes \
    -type f \
    ! -path '*/.archive/*' \
    ! -path '*/__pycache__/*' \
    -print0 | sort -z
)
if ((${#PACKAGE_FILES[@]} > 0)); then
  sha256sum "${PACKAGE_FILES[@]}" >"$LOG_DIR/release-package-file-hashes.tsv"
else
  : >"$LOG_DIR/release-package-file-hashes.tsv"
fi
jq -Rn '[inputs | capture("^(?<sha256>[0-9a-f]+)  (?<path>.*)$")]' <"$LOG_DIR/release-package-file-hashes.tsv" >"$LOG_DIR/release-package-file-hashes.json"

python3 - "$LOG_DIR/release-package-file-hashes.json" "$PACKAGE_MANIFEST" <<'PY'
import json
import sys
from collections import Counter

source, target = sys.argv[1], sys.argv[2]
items = json.load(open(source, encoding="utf-8"))
required_dirs = [
    "deployments/kubernetes",
    "common/kafka",
    "common/sql",
    "proto/traffic/v1",
    "rules",
    "mlops/workflows",
    "tests/e2e",
    "tests/chaos",
    "web/ui/src/routes",
]
counts = Counter()
for item in items:
    path = item["path"]
    for directory in required_dirs:
        if path == directory or path.startswith(directory + "/"):
            counts[directory] += 1
missing = [directory for directory in required_dirs if counts[directory] == 0]
payload = {
    "required_directories": required_dirs,
    "file_count": len(items),
    "directory_file_counts": dict(counts),
    "missing_required_directories": missing,
    "files": items,
}
json.dump(payload, open(target, "w", encoding="utf-8"), indent=2, ensure_ascii=True)
PY
PACKAGE_FILE_COUNT="$(jq '.file_count' "$PACKAGE_MANIFEST")"
PACKAGE_MISSING_COUNT="$(jq '.missing_required_directories | length' "$PACKAGE_MANIFEST")"
if [[ "$PACKAGE_FILE_COUNT" -gt 0 && "$PACKAGE_MISSING_COUNT" -eq 0 ]]; then
  json_log "repo" "release package manifest complete" "info" true "ok" "$PACKAGE_FILE_COUNT files" "release-package-manifest.json"
else
  json_log "repo" "release package manifest complete" "blocker" false "missing" "files=$PACKAGE_FILE_COUNT missing_dirs=$PACKAGE_MISSING_COUNT" "release-package-manifest.json"
fi

set +e
python3 tests/e2e/image_digest_lock.py validate \
  --root deployments/kubernetes \
  --lock "$IMAGE_LOCK" \
  --out-inventory "$LOG_DIR/repo-image-lock-inventory.json" \
  --out-missing "$LOG_DIR/repo-image-lock-missing.txt" \
  --out-missing-files "$LOG_DIR/repo-image-lock-missing-files.txt" \
  --out-mutable "$LOG_DIR/repo-latest-or-mutable-image-lines.txt" \
  >"$LOG_DIR/repo-image-lock-summary.json" 2>"$LOG_DIR/repo-image-lock.err"
IMAGE_LOCK_RC=$?
set -e
if [[ -s "$LOG_DIR/repo-image-lock-summary.json" ]]; then
  REPO_IMAGE_LOCK_MISSING_COUNT="$(jq '.missing_lock_lines' "$LOG_DIR/repo-image-lock-summary.json")"
  REPO_MUTABLE_IMAGE_COUNT="$(jq '.mutable_tag_lines' "$LOG_DIR/repo-image-lock-summary.json")"
else
  REPO_IMAGE_LOCK_MISSING_COUNT=1
  REPO_MUTABLE_IMAGE_COUNT=0
fi
REPO_UNPINNED_IMAGE_COUNT="$REPO_IMAGE_LOCK_MISSING_COUNT"
if [[ "$IMAGE_LOCK_RC" -le 1 && "$REPO_IMAGE_LOCK_MISSING_COUNT" -eq 0 ]]; then
  json_log "repo" "repo K8s images have digest evidence lock" "info" true "ok" "0 missing image lock lines" "repo-image-lock-summary.json"
else
  json_log "repo" "repo K8s images have digest evidence lock" "blocker" false "missing" "$REPO_IMAGE_LOCK_MISSING_COUNT missing image lock lines" "repo-image-lock-missing.txt"
fi
if [[ "$REPO_MUTABLE_IMAGE_COUNT" -gt 0 ]]; then
  json_log "repo" "repo K8s mutable/latest image refs covered by lock" "warn" false "found" "$REPO_MUTABLE_IMAGE_COUNT mutable tag lines" "repo-latest-or-mutable-image-lines.txt"
fi

set +e
python3 tests/e2e/k8s_service_exposure.py validate \
  --root deployments/kubernetes \
  --out-inventory "$LOG_DIR/repo-service-exposure-inventory.json" \
  --out-blockers "$LOG_DIR/repo-service-exposure-blockers.json" \
  --out-blocker-lines "$LOG_DIR/repo-service-exposure-blockers.txt" \
  >"$LOG_DIR/repo-service-exposure-summary.json" 2>"$LOG_DIR/repo-service-exposure.err"
SERVICE_EXPOSURE_RC=$?
set -e
if [[ -s "$LOG_DIR/repo-service-exposure-summary.json" ]]; then
  REPO_SERVICE_EXPOSURE_BLOCKERS="$(jq '.blocked_external_service_ports' "$LOG_DIR/repo-service-exposure-summary.json")"
  REPO_SERVICE_EXTERNAL_PORTS="$(jq '.external_service_ports' "$LOG_DIR/repo-service-exposure-summary.json")"
else
  REPO_SERVICE_EXPOSURE_BLOCKERS=1
  REPO_SERVICE_EXTERNAL_PORTS=0
fi
if [[ "$SERVICE_EXPOSURE_RC" -le 1 && "$REPO_SERVICE_EXPOSURE_BLOCKERS" -eq 0 ]]; then
  json_log "repo" "repo Service exposure limited to APISIX business port" "info" true "ok" "$REPO_SERVICE_EXTERNAL_PORTS external service ports" "repo-service-exposure-summary.json"
else
  json_log "repo" "repo Service exposure limited to APISIX business port" "blocker" false "found" "$REPO_SERVICE_EXPOSURE_BLOCKERS non-business external service ports" "repo-service-exposure-blockers.txt"
fi

set +e
kctl apply --dry-run=client -o json \
  -f deployments/kubernetes/applications \
  -f deployments/kubernetes/configmaps \
  -f deployments/kubernetes/infrastructure \
  -f deployments/kubernetes/init-jobs \
  -f deployments/kubernetes/flink \
  >"$LOG_DIR/k8s-core-apply-convergence.json" 2>"$LOG_DIR/k8s-core-apply-convergence.err"
APPLY_CONVERGENCE_RC=$?
set -e
APPLY_CONVERGENCE_SERVICE_BLOCKERS=1
APPLY_CONVERGENCE_EXTERNAL_PORTS=0
if [[ "$APPLY_CONVERGENCE_RC" -eq 0 ]]; then
  set +e
  python3 tests/e2e/k8s_service_exposure.py validate-kubectl-json \
    --input "$LOG_DIR/k8s-core-apply-convergence.json" \
    --out-inventory "$LOG_DIR/apply-convergence-service-exposure-inventory.json" \
    --out-blockers "$LOG_DIR/apply-convergence-service-exposure-blockers.json" \
    --out-blocker-lines "$LOG_DIR/apply-convergence-service-exposure-blockers.txt" \
    >"$LOG_DIR/apply-convergence-service-exposure-summary.json" 2>"$LOG_DIR/apply-convergence-service-exposure.err"
  APPLY_CONVERGENCE_SERVICE_RC=$?
  set -e
  if [[ -s "$LOG_DIR/apply-convergence-service-exposure-summary.json" ]]; then
    APPLY_CONVERGENCE_SERVICE_BLOCKERS="$(jq '.blocked_external_service_ports' "$LOG_DIR/apply-convergence-service-exposure-summary.json")"
    APPLY_CONVERGENCE_EXTERNAL_PORTS="$(jq '.external_service_ports' "$LOG_DIR/apply-convergence-service-exposure-summary.json")"
  fi
  if [[ "$APPLY_CONVERGENCE_SERVICE_RC" -le 1 && "$APPLY_CONVERGENCE_SERVICE_BLOCKERS" -eq 0 ]]; then
    json_log "repo" "kubectl apply convergence keeps APISIX-only Service exposure" "info" true "ok" "$APPLY_CONVERGENCE_EXTERNAL_PORTS external service ports" "apply-convergence-service-exposure-summary.json"
  else
    json_log "repo" "kubectl apply convergence keeps APISIX-only Service exposure" "blocker" false "found" "$APPLY_CONVERGENCE_SERVICE_BLOCKERS preserved non-business external service ports" "apply-convergence-service-exposure-blockers.txt"
  fi
else
  json_log "repo" "kubectl apply convergence keeps APISIX-only Service exposure" "blocker" false "rc=$APPLY_CONVERGENCE_RC" "$(trim_file "$LOG_DIR/k8s-core-apply-convergence.err")" "k8s-core-apply-convergence.err"
fi
if [[ ! -s "$LOG_DIR/apply-convergence-service-exposure-summary.json" ]]; then
  jq -n \
    --arg input "$LOG_DIR/k8s-core-apply-convergence.json" \
    --argjson blocked "$APPLY_CONVERGENCE_SERVICE_BLOCKERS" \
    --argjson external "$APPLY_CONVERGENCE_EXTERNAL_PORTS" \
    '{input:$input, service_ports:0, external_service_ports:$external, blocked_external_service_ports:$blocked, allowed_external_service_ports:0}' \
    >"$LOG_DIR/apply-convergence-service-exposure-summary.json"
fi
if [[ ! -s "$LOG_DIR/apply-convergence-service-exposure-inventory.json" ]]; then
  jq -n '[]' >"$LOG_DIR/apply-convergence-service-exposure-inventory.json"
fi
if [[ ! -s "$LOG_DIR/apply-convergence-service-exposure-blockers.json" ]]; then
  jq -n '[]' >"$LOG_DIR/apply-convergence-service-exposure-blockers.json"
fi

kctl get nodes -o json >"$LOG_DIR/live-nodes.json"
kctl get namespace -o json >"$LOG_DIR/live-namespaces.json"
kctl get storageclass -o json >"$LOG_DIR/live-storageclasses.json"
kctl get pv -o json >"$LOG_DIR/live-pvs.json"
kctl get pvc -A -o json >"$LOG_DIR/live-pvcs.json"
kctl get secret -A -o json >"$LOG_DIR/live-secrets.json"
kctl get svc -A -o json >"$LOG_DIR/live-services.json"
kctl get deploy,sts,ds -A -o json >"$LOG_DIR/live-workloads.json"
kctl get pods -A -o json >"$LOG_DIR/live-pods.json"

jq '[
  .items[]
  | {
      name:.metadata.name,
      ready: ([.status.conditions[]? | select(.type=="Ready") | .status][0] // "Unknown"),
      kubernetes_version:.status.nodeInfo.kubeletVersion,
      os_image:.status.nodeInfo.osImage,
      container_runtime:.status.nodeInfo.containerRuntimeVersion,
      internal_ip: ([.status.addresses[]? | select(.type=="InternalIP") | .address][0] // "")
    }
]' "$LOG_DIR/live-nodes.json" >"$LOG_DIR/live-node-summary.json"
NODE_COUNT="$(jq 'length' "$LOG_DIR/live-node-summary.json")"
NOT_READY_NODES="$(jq '[.[] | select(.ready != "True")] | length' "$LOG_DIR/live-node-summary.json")"
if [[ "$NODE_COUNT" -ge 2 && "$NOT_READY_NODES" -eq 0 ]]; then
  json_log "live" "cluster node baseline ready" "info" true "ok" "$NODE_COUNT nodes" "live-node-summary.json"
else
  json_log "live" "cluster node baseline ready" "blocker" false "not_ready" "nodes=$NODE_COUNT not_ready=$NOT_READY_NODES" "live-node-summary.json"
fi

cat >"$LOG_DIR/expected-runtime-namespaces.txt" <<'EOF'
traffic-analysis
gateway
middleware
databases
flink
minio
observability
argo
argo-events
iam
nacos
streampark
EOF
jq -r '.items[].metadata.name' "$LOG_DIR/live-namespaces.json" | sort >"$LOG_DIR/live-namespaces.txt"
comm -23 <(sort "$LOG_DIR/expected-runtime-namespaces.txt") "$LOG_DIR/live-namespaces.txt" >"$LOG_DIR/missing-runtime-namespaces.txt"
MISSING_NAMESPACE_COUNT="$(wc -l <"$LOG_DIR/missing-runtime-namespaces.txt" | tr -d ' ')"
if [[ "$MISSING_NAMESPACE_COUNT" -eq 0 ]]; then
  json_log "live" "expected runtime namespaces present" "info" true "ok" "$(wc -l <"$LOG_DIR/expected-runtime-namespaces.txt" | tr -d ' ') namespaces" "live-namespaces.txt"
else
  json_log "live" "expected runtime namespaces present" "blocker" false "missing" "$MISSING_NAMESPACE_COUNT namespaces missing" "missing-runtime-namespaces.txt"
fi

if jq -e '.items[] | select(.metadata.name == "local-hdd")' "$LOG_DIR/live-storageclasses.json" >/dev/null; then
  json_log "live" "required StorageClass local-hdd present" "info" true "ok" "local-hdd" "live-storageclasses.json"
else
  json_log "live" "required StorageClass local-hdd present" "blocker" false "missing" "local-hdd" "live-storageclasses.json"
fi

jq '[
  .items[]
  | {
      namespace:.metadata.namespace,
      name:.metadata.name,
      status:.status.phase,
      storage_class:(.spec.storageClassName // ""),
      volume:(.spec.volumeName // ""),
      capacity:(.status.capacity.storage // "")
    }
]' "$LOG_DIR/live-pvcs.json" >"$LOG_DIR/live-pvc-summary.json"
PENDING_PVC_COUNT="$(jq '[.[] | select(.status != "Bound")] | length' "$LOG_DIR/live-pvc-summary.json")"
if [[ "$PENDING_PVC_COUNT" -eq 0 ]]; then
  json_log "live" "all PVCs bound" "info" true "ok" "$(jq 'length' "$LOG_DIR/live-pvc-summary.json") PVCs" "live-pvc-summary.json"
else
  json_log "live" "all PVCs bound" "warn" false "pending" "$PENDING_PVC_COUNT PVCs not Bound" "live-pvc-summary.json"
fi

jq '[
  .items[]
  | select(.status.phase != "Bound" and .status.phase != "Available")
  | {
      name:.metadata.name,
      status:.status.phase,
      claim:(.spec.claimRef.namespace + "/" + .spec.claimRef.name // "")
    }
]' "$LOG_DIR/live-pvs.json" >"$LOG_DIR/live-pv-drift.json"
PV_DRIFT_COUNT="$(jq 'length' "$LOG_DIR/live-pv-drift.json")"
if [[ "$PV_DRIFT_COUNT" -eq 0 ]]; then
  json_log "live" "PV states are bound or available" "info" true "ok" "0 drift PVs" "live-pv-drift.json"
else
  json_log "live" "PV states are bound or available" "warn" false "drift" "$PV_DRIFT_COUNT PVs in non-bound/available state" "live-pv-drift.json"
fi

jq '[
  .items[]
  | {
      namespace:.metadata.namespace,
      name:.metadata.name,
      keys:(.data // {} | keys | sort)
    }
]' "$LOG_DIR/live-secrets.json" >"$LOG_DIR/live-secret-inventory.json"
cat >"$LOG_DIR/expected-secret-refs.json" <<'JSON'
[
  {"namespace":"middleware","name":"traffic-credentials","keys":["CLICKHOUSE_PASSWORD","REDIS_PASSWORD","JWT_SECRET","APISIX_ADMIN_KEY","OPENSEARCH_ADMIN_PASSWORD","KAFKA_CLIENT_USERNAME","KAFKA_CLIENT_PASSWORD","KAFKA_CLIENT_JAAS_CONFIG","KAFKA_INTER_BROKER_USERNAME","KAFKA_INTER_BROKER_PASSWORD","KAFKA_TLS_KEYSTORE_PASSWORD","KAFKA_TLS_TRUSTSTORE_PASSWORD"]},
  {"namespace":"middleware","name":"kafka-broker-tls","keys":["kafka.keystore.p12","kafka.truststore.p12","ca.crt"]},
  {"namespace":"middleware","name":"kafka-client-tls","keys":["kafka.truststore.p12","ca.crt"]},
  {"namespace":"traffic-analysis","name":"traffic-credentials","keys":["PG_PASSWORD","CLICKHOUSE_PASSWORD","REDIS_PASSWORD","MINIO_ACCESS_KEY","MINIO_SECRET_KEY","JWT_SECRET","OIDC_CLIENT_SECRET","OPENSEARCH_ADMIN_PASSWORD","KAFKA_CLIENT_USERNAME","KAFKA_CLIENT_PASSWORD","KAFKA_TLS_TRUSTSTORE_PASSWORD"]},
  {"namespace":"traffic-analysis","name":"kafka-client-tls","keys":["kafka.truststore.p12","ca.crt"]},
  {"namespace":"databases","name":"traffic-credentials","keys":["PG_PASSWORD","PG_REPLICATION_PASSWORD","REDIS_PASSWORD"]},
  {"namespace":"gateway","name":"traffic-credentials","keys":["APISIX_ADMIN_KEY"]},
  {"namespace":"flink","name":"traffic-credentials","keys":["CLICKHOUSE_PASSWORD","MINIO_ACCESS_KEY","MINIO_SECRET_KEY","OPENSEARCH_ADMIN_PASSWORD","KAFKA_CLIENT_USERNAME","KAFKA_CLIENT_PASSWORD","KAFKA_TLS_TRUSTSTORE_PASSWORD"]},
  {"namespace":"flink","name":"kafka-client-tls","keys":["kafka.truststore.p12","ca.crt"]},
  {"namespace":"minio","name":"traffic-credentials","keys":["MINIO_ACCESS_KEY","MINIO_SECRET_KEY"]},
  {"namespace":"observability","name":"traffic-credentials","keys":["GRAFANA_ADMIN_PASSWORD"]},
  {"namespace":"iam","name":"traffic-credentials","keys":["KEYCLOAK_ADMIN_PASSWORD"]},
  {"namespace":"iam","name":"keycloak-tls","keys":["tls.crt","tls.key"]},
  {"namespace":"traffic-analysis","name":"probe-agent-certs","keys":["ca-cert.pem","client-cert.pem","client-key.pem"]},
  {"namespace":"traffic-analysis","name":"ingest-gateway-certs","keys":["ca-cert.pem","server-cert.pem","server-key.pem"]}
]
JSON
jq -n \
  --argfile expected "$LOG_DIR/expected-secret-refs.json" \
  --argfile live "$LOG_DIR/live-secret-inventory.json" '
  $expected
  | map(. as $e
    | (first($live[]? | select(.namespace == $e.namespace and .name == $e.name)) // null) as $s
    | $e + {
        present:($s != null),
        missing_keys:(if $s == null then $e.keys else ($e.keys - ($s.keys // [])) end),
        status:(if $s == null then "missing" elif (($e.keys - ($s.keys // [])) | length) > 0 then "missing_keys" else "ok" end)
      }
  )' >"$LOG_DIR/secret-reference-readiness.json"
SECRET_BLOCKERS="$(jq '[.[] | select(.status != "ok")] | length' "$LOG_DIR/secret-reference-readiness.json")"
if [[ "$SECRET_BLOCKERS" -eq 0 ]]; then
  json_log "live" "required secret references present" "info" true "ok" "$(jq 'length' "$LOG_DIR/secret-reference-readiness.json") secrets" "secret-reference-readiness.json"
else
  json_log "live" "required secret references present" "blocker" false "missing" "$SECRET_BLOCKERS secret refs missing or incomplete" "secret-reference-readiness.json"
fi

jq '[
  .items[]
  | . as $svc
  | .spec.ports[]
  | select($svc.spec.type == "NodePort" or $svc.spec.type == "LoadBalancer")
  | {
      namespace:$svc.metadata.namespace,
      service:$svc.metadata.name,
      type:$svc.spec.type,
      port_name:(.name // ""),
      port:.port,
      node_port:(.nodePort // null),
      target_port:(.targetPort // null),
      public_business_entry:(
        $svc.metadata.namespace == "gateway" and
        $svc.metadata.name == "apisix" and
        (.name // "") == "http" and
        .port == 9080 and
        (.nodePort // 0) == 30180
      )
    }
]' "$LOG_DIR/live-services.json" >"$LOG_DIR/live-external-service-ports.json"
jq '[.[] | select(.public_business_entry | not)]' "$LOG_DIR/live-external-service-ports.json" >"$LOG_DIR/live-non-business-external-ports.json"
NON_BUSINESS_EXTERNAL_PORTS="$(jq 'length' "$LOG_DIR/live-non-business-external-ports.json")"
if [[ "$NON_BUSINESS_EXTERNAL_PORTS" -eq 0 ]]; then
  json_log "live" "external exposure limited to APISIX business port" "info" true "ok" "only 30180" "live-external-service-ports.json"
else
  json_log "live" "external exposure limited to APISIX business port" "warn" false "found" "$NON_BUSINESS_EXTERNAL_PORTS non-business external ports" "live-non-business-external-ports.json"
fi

jq '[
  .items[]
  | {
      namespace:.metadata.namespace,
      kind:.kind,
      name:.metadata.name,
      desired:(.spec.replicas // .status.desiredNumberScheduled // null),
      ready:(.status.readyReplicas // .status.numberReady // 0)
    }
]' "$LOG_DIR/live-workloads.json" >"$LOG_DIR/live-workload-readiness.json"
jq '[.[] | select(.namespace as $ns | ["traffic-analysis","gateway","middleware","databases","flink","minio"] | index($ns)) | select((.desired // 0) > (.ready // 0))]' "$LOG_DIR/live-workload-readiness.json" >"$LOG_DIR/live-unready-runtime-workloads.json"
UNREADY_RUNTIME_WORKLOADS="$(jq 'length' "$LOG_DIR/live-unready-runtime-workloads.json")"
if [[ "$UNREADY_RUNTIME_WORKLOADS" -eq 0 ]]; then
  json_log "live" "runtime workloads ready" "info" true "ok" "$(jq 'length' "$LOG_DIR/live-workload-readiness.json") workloads captured" "live-workload-readiness.json"
else
  json_log "live" "runtime workloads ready" "blocker" false "not_ready" "$UNREADY_RUNTIME_WORKLOADS runtime workloads not ready" "live-unready-runtime-workloads.json"
fi

jq '[
  .items[]
  | . as $w
  | ([($w.spec.template.spec.initContainers // [])[], ($w.spec.template.spec.containers // [])[]])
  | .[]
  | {
      namespace:$w.metadata.namespace,
      kind:$w.kind,
      workload:$w.metadata.name,
      container:.name,
      image:.image
    }
]' "$LOG_DIR/live-workloads.json" >"$LOG_DIR/live-workload-images.json"
jq '[.[] | select(((.image // "") | contains("@sha256:") | not) or ((.image // "") | endswith(":latest")))]' "$LOG_DIR/live-workload-images.json" >"$LOG_DIR/live-unpinned-or-latest-images.json"
LIVE_UNPINNED_IMAGES="$(jq 'length' "$LOG_DIR/live-unpinned-or-latest-images.json")"
if [[ "$LIVE_UNPINNED_IMAGES" -eq 0 ]]; then
  json_log "live" "live workload spec images digest-pinned and avoid latest" "info" true "ok" "0 containers" "live-unpinned-or-latest-images.json"
else
  json_log "live" "live workload spec images digest-pinned and avoid latest" "blocker" false "found" "$LIVE_UNPINNED_IMAGES containers" "live-unpinned-or-latest-images.json"
fi

set +e
APISIX_CODE="$(curl --noproxy '*' -sS -m 10 -o "$LOG_DIR/apisix-root.txt" -w '%{http_code}' "$APISIX/" 2>"$LOG_DIR/apisix-root.err")"
APISIX_RC=$?
set -e
if [[ "$APISIX_RC" -eq 0 && "$APISIX_CODE" =~ ^(200|301|302|404)$ ]]; then
  json_log "live" "APISIX business entry reachable" "info" true "ok" "$APISIX http=$APISIX_CODE" "apisix-root.txt"
else
  json_log "live" "APISIX business entry reachable" "blocker" false "http=${APISIX_CODE:-none} rc=$APISIX_RC" "$(trim_file "$LOG_DIR/apisix-root.err")" "apisix-root.err"
fi

python3 - "$LOG_DIR/live-namespaces.txt" "$LOG_DIR/live-services.json" "$LOG_DIR/live-storageclasses.json" "$LOG_DIR/live-pvc-summary.json" "$LOG_DIR/live-node-summary.json" "$LOG_DIR/live-site-values-observed.json" <<'PY'
import json
import sys

namespaces_file, services_file, storageclasses_file, pvc_file, nodes_file, out_file = sys.argv[1:7]
namespaces = [line.strip() for line in open(namespaces_file, encoding="utf-8") if line.strip()]
services = json.load(open(services_file, encoding="utf-8"))["items"]
storageclasses = [item["metadata"]["name"] for item in json.load(open(storageclasses_file, encoding="utf-8"))["items"]]
pvcs = json.load(open(pvc_file, encoding="utf-8"))
nodes = json.load(open(nodes_file, encoding="utf-8"))
node_ports = []
for svc in services:
    if svc.get("spec", {}).get("type") not in {"NodePort", "LoadBalancer"}:
        continue
    for port in svc.get("spec", {}).get("ports", []):
        node_ports.append({
            "namespace": svc["metadata"]["namespace"],
            "service": svc["metadata"]["name"],
            "port_name": port.get("name", ""),
            "port": port.get("port"),
            "node_port": port.get("nodePort"),
            "type": svc.get("spec", {}).get("type"),
        })
payload = {
    "namespaces": namespaces,
    "storageclasses": storageclasses,
    "nodes": nodes,
    "node_ports": sorted(node_ports, key=lambda x: (x["namespace"], x["service"], x.get("node_port") or 0)),
    "pending_pvcs": [pvc for pvc in pvcs if pvc["status"] != "Bound"],
}
json.dump(payload, open(out_file, "w", encoding="utf-8"), indent=2, ensure_ascii=True)
PY

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
  --arg package_manifest "$PACKAGE_MANIFEST" \
  --arg site_values "$SITE_VALUES" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson package_file_count "$PACKAGE_FILE_COUNT" \
  --argjson repo_unpinned_image_count "$REPO_UNPINNED_IMAGE_COUNT" \
  --argjson repo_image_lock_missing_count "$REPO_IMAGE_LOCK_MISSING_COUNT" \
  --argjson repo_mutable_image_count "$REPO_MUTABLE_IMAGE_COUNT" \
  --argjson repo_service_exposure_blockers "$REPO_SERVICE_EXPOSURE_BLOCKERS" \
  --argjson repo_service_external_ports "$REPO_SERVICE_EXTERNAL_PORTS" \
  --argjson apply_convergence_service_blockers "$APPLY_CONVERGENCE_SERVICE_BLOCKERS" \
  --argjson apply_convergence_external_ports "$APPLY_CONVERGENCE_EXTERNAL_PORTS" \
  --argjson live_unpinned_image_count "$LIVE_UNPINNED_IMAGES" \
  --argjson non_business_external_ports "$NON_BUSINESS_EXTERNAL_PORTS" \
  --argjson pending_pvc_count "$PENDING_PVC_COUNT" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    package_manifest:$package_manifest,
    site_values:$site_values,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    package_file_count:$package_file_count,
    repo_unpinned_image_count:$repo_unpinned_image_count,
    repo_image_lock_missing_count:$repo_image_lock_missing_count,
    repo_mutable_image_count:$repo_mutable_image_count,
    repo_service_exposure_blockers:$repo_service_exposure_blockers,
    repo_service_external_ports:$repo_service_external_ports,
    apply_convergence_service_blockers:$apply_convergence_service_blockers,
    apply_convergence_external_ports:$apply_convergence_external_ports,
    live_unpinned_image_count:$live_unpinned_image_count,
    non_business_external_ports:$non_business_external_ports,
    pending_pvc_count:$pending_pvc_count,
    checks:.
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# Deployment Preflight

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This is a non-destructive preflight for GATE-P0-09. It does not apply manifests, mutate live resources, or read secret values.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| Release package files | $PACKAGE_FILE_COUNT |
| Repo image lock missing lines | $REPO_IMAGE_LOCK_MISSING_COUNT |
| Repo mutable/latest image lines with lock evidence | $REPO_MUTABLE_IMAGE_COUNT |
| Repo non-business external service ports | $REPO_SERVICE_EXPOSURE_BLOCKERS |
| Apply-convergence non-business external service ports | $APPLY_CONVERGENCE_SERVICE_BLOCKERS |
| Live unpinned/latest workload spec image containers | $LIVE_UNPINNED_IMAGES |
| Non-business external ports | $NON_BUSINESS_EXTERNAL_PORTS |
| Pending PVCs | $PENDING_PVC_COUNT |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`release-package-manifest.json\`
- \`repo-image-lock-summary.json\`
- \`repo-image-lock-inventory.json\`
- \`repo-service-exposure-summary.json\`
- \`repo-service-exposure-blockers.json\`
- \`apply-convergence-service-exposure-summary.json\`
- \`apply-convergence-service-exposure-blockers.json\`
- \`live-site-values-observed.json\`
- \`k8s-core-dry-run.txt\`
- \`secret-reference-readiness.json\`
- \`live-workload-readiness.json\`
- \`live-workload-images.json\`
- \`live-unpinned-or-latest-images.json\`
- \`live-non-business-external-ports.json\`

## Interpretation

The site-values template, release package manifest, image digest evidence lock, APISIX-only repo Service exposure profile, live APISIX-only Service exposure, and kubectl apply convergence behavior now have evidence artifacts. The script captures enough live state to reproduce or reject a site deployment. GATE-P0-09 remains blocked until live workload image specs are rolled to pullable digest references, production security blockers are cleared, and the release package is promoted from evidence package to signed/released artifact.
EOF

cp "$SUMMARY" "$DEPLOYMENT_DIR/deployment-preflight-latest.json"
cp "$LOCAL_REPORT" "$DEPLOYMENT_DIR/deployment-preflight-latest.md"
cp "$PACKAGE_MANIFEST" "$DEPLOYMENT_DIR/release-package-manifest-latest.json"
cp "$LOG_DIR/live-site-values-observed.json" "$DEPLOYMENT_DIR/site-values-observed-latest.json"
cp "$LOG_DIR/secret-reference-readiness.json" "$DEPLOYMENT_DIR/secret-reference-readiness-latest.json"
cp "$LOG_DIR/live-non-business-external-ports.json" "$DEPLOYMENT_DIR/non-business-external-ports-latest.json"
cp "$LOG_DIR/live-unpinned-or-latest-images.json" "$DEPLOYMENT_DIR/unpinned-or-latest-images-latest.json"
cp "$LOG_DIR/repo-image-lock-summary.json" "$DEPLOYMENT_DIR/repo-image-lock-summary-latest.json"
cp "$LOG_DIR/repo-image-lock-inventory.json" "$DEPLOYMENT_DIR/repo-image-lock-inventory-latest.json"
cp "$LOG_DIR/repo-latest-or-mutable-image-lines.txt" "$DEPLOYMENT_DIR/repo-latest-or-mutable-image-lines-latest.txt"
cp "$LOG_DIR/repo-service-exposure-summary.json" "$DEPLOYMENT_DIR/repo-service-exposure-summary-latest.json"
cp "$LOG_DIR/repo-service-exposure-inventory.json" "$DEPLOYMENT_DIR/repo-service-exposure-inventory-latest.json"
cp "$LOG_DIR/repo-service-exposure-blockers.json" "$DEPLOYMENT_DIR/repo-service-exposure-blockers-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-summary.json" "$DEPLOYMENT_DIR/apply-convergence-service-exposure-summary-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-inventory.json" "$DEPLOYMENT_DIR/apply-convergence-service-exposure-inventory-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-blockers.json" "$DEPLOYMENT_DIR/apply-convergence-service-exposure-blockers-latest.json"

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
