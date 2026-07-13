#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-production-security-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-production-security-preflight}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"
NETWORK_POLICY_FILE="${NETWORK_POLICY_FILE:-deployments/kubernetes/security/00-network-policies.yaml}"
EXTERNAL_SECRET_TEMPLATE_FILE="${EXTERNAL_SECRET_TEMPLATE_FILE:-deployments/kubernetes/security/external-secrets-template.yaml}"
IMAGE_LOCK="${IMAGE_LOCK:-deployments/kubernetes/image-digests.lock.json}"
SECURITY_WAIVER_FILE="${SECURITY_WAIVER_FILE:-$SECURITY_DIR/production-security-waivers.yaml}"

REPORT="$LOG_DIR/live-production-security-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-production-security-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

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

file_count() {
  local file="$1"
  if [[ -s "$file" ]]; then
    wc -l <"$file" | tr -d ' '
  else
    echo 0
  fi
}

waiver_count() {
  local category="$1"
  jq --arg category "$category" '[.waivers[$category][]?] | length' "$LOG_DIR/production-security-waivers.json"
}

check_path_waivers() {
  local category="$1" input_file="$2" output_file="$3"
  python3 - "$LOG_DIR/production-security-waivers.json" "$category" "$input_file" "$output_file" <<'PY'
import json
import sys
from pathlib import Path

waiver_json, category, input_file, output_file = sys.argv[1:]
registry = json.loads(Path(waiver_json).read_text(encoding="utf-8"))
waived = {str(item.get("path", "")) for item in registry.get("waivers", {}).get(category, [])}
findings = [line.strip() for line in Path(input_file).read_text(encoding="utf-8").splitlines() if line.strip()]
unwaived = [path for path in findings if path not in waived]
Path(output_file).write_text("\n".join(unwaived) + ("\n" if unwaived else ""), encoding="utf-8")
print(len(unwaived))
PY
}

check_workload_waivers() {
  local category="$1" input_file="$2" output_file="$3"
  python3 - "$LOG_DIR/production-security-waivers.json" "$category" "$input_file" "$output_file" <<'PY'
import json
import sys
from pathlib import Path

waiver_json, category, input_file, output_file = sys.argv[1:]
registry = json.loads(Path(waiver_json).read_text(encoding="utf-8"))
findings = json.loads(Path(input_file).read_text(encoding="utf-8"))
waivers = registry.get("waivers", {}).get(category, [])

def key(item):
    values = [
        item.get("namespace", ""),
        item.get("kind", ""),
        item.get("workload", ""),
    ]
    if "container" in item:
        values.append(item.get("container", ""))
    return tuple(str(value) for value in values)

waived = {key(item) for item in waivers}
unwaived = [item for item in findings if key(item) not in waived]
Path(output_file).write_text(json.dumps(unwaived, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(len(unwaived))
PY
}

need_cmd jq
need_cmd python3
need_cmd rg
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ -s "$SECURITY_WAIVER_FILE" ]]; then
  python3 - "$SECURITY_WAIVER_FILE" >"$LOG_DIR/production-security-waivers.json" <<'PY'
import json
import sys
from pathlib import Path

import yaml

path = Path(sys.argv[1])
data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
if not isinstance(data, dict):
    raise SystemExit("waiver registry must be a YAML mapping")
waivers = data.get("waivers") or {}
if not isinstance(waivers, dict):
    raise SystemExit("waivers must be a mapping")
for category, items in waivers.items():
    if not isinstance(items, list):
        raise SystemExit(f"waiver category {category} must be a list")
    for idx, item in enumerate(items):
        if not isinstance(item, dict):
            raise SystemExit(f"waiver {category}[{idx}] must be a mapping")
        for field in ("owner", "reason", "review_status"):
            if not item.get(field):
                raise SystemExit(f"waiver {category}[{idx}] missing {field}")
print(json.dumps(data, ensure_ascii=False, indent=2))
PY
  json_log "repo" "production security waiver registry present" "info" true "ok" "$SECURITY_WAIVER_FILE" "production-security-waivers.json"
else
  jq -n '{waivers:{}}' >"$LOG_DIR/production-security-waivers.json"
  json_log "repo" "production security waiver registry present" "warn" false "missing" "$SECURITY_WAIVER_FILE" "production-security-waivers.json"
fi

set +e
kctl apply --dry-run=client -f "$NETWORK_POLICY_FILE" >"$LOG_DIR/network-policy-dry-run.txt" 2>"$LOG_DIR/network-policy-dry-run.err"
NETWORK_POLICY_DRY_RUN_RC=$?
set -e
if [[ "$NETWORK_POLICY_DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "NetworkPolicy profile client dry-run" "info" true "ok" "$NETWORK_POLICY_FILE" "network-policy-dry-run.txt"
else
  json_log "repo" "NetworkPolicy profile client dry-run" "blocker" false "rc=$NETWORK_POLICY_DRY_RUN_RC" "$NETWORK_POLICY_FILE" "network-policy-dry-run.err"
fi

rg -l --glob '!**/.archive/**' 'kind:[[:space:]]*NetworkPolicy' deployments/kubernetes >"$LOG_DIR/repo-networkpolicy-files.txt" || true
REPO_NP_COUNT="$(file_count "$LOG_DIR/repo-networkpolicy-files.txt")"
if [[ "$REPO_NP_COUNT" -gt 0 ]]; then
  json_log "repo" "repo NetworkPolicy manifests present" "info" true "ok" "$REPO_NP_COUNT files" "repo-networkpolicy-files.txt"
else
  json_log "repo" "repo NetworkPolicy manifests present" "blocker" false "missing" "no NetworkPolicy manifests in deployments/kubernetes" "repo-networkpolicy-files.txt"
fi

rg -l --glob '!**/.archive/**' 'kind:[[:space:]]*(ExternalSecret|SealedSecret)' deployments/kubernetes >"$LOG_DIR/repo-external-secret-files.txt" || true
EXTERNAL_SECRET_COUNT="$(file_count "$LOG_DIR/repo-external-secret-files.txt")"
if [[ "$EXTERNAL_SECRET_COUNT" -gt 0 ]]; then
  json_log "repo" "ExternalSecret or SealedSecret profile present" "info" true "ok" "$EXTERNAL_SECRET_COUNT files" "repo-external-secret-files.txt"
else
  json_log "repo" "ExternalSecret or SealedSecret profile present" "blocker" false "missing" "no ExternalSecret/SealedSecret manifest" "repo-external-secret-files.txt"
fi

rg -l --glob '!**/.archive/**' 'PLAINTEXT://|inter\.broker\.listener\.name=PLAINTEXT|advertised\.listeners=PLAINTEXT' deployments/kubernetes common/kafka common/config >"$LOG_DIR/repo-kafka-plaintext-files.txt" || true
KAFKA_PLAINTEXT_COUNT="$(file_count "$LOG_DIR/repo-kafka-plaintext-files.txt")"
if [[ "$KAFKA_PLAINTEXT_COUNT" -eq 0 ]]; then
  json_log "repo" "Kafka plaintext listeners absent from production manifests" "info" true "ok" "0 files" "repo-kafka-plaintext-files.txt"
else
  json_log "repo" "Kafka plaintext listeners absent from production manifests" "blocker" false "found" "$KAFKA_PLAINTEXT_COUNT files" "repo-kafka-plaintext-files.txt"
fi

rg -l --glob '!**/.archive/**' 'PLAINTEXT://|inter\.broker\.listener\.name=PLAINTEXT|advertised\.listeners=PLAINTEXT' java go rust >"$LOG_DIR/repo-local-kafka-plaintext-files.txt" || true
LOCAL_KAFKA_PLAINTEXT_COUNT="$(file_count "$LOG_DIR/repo-local-kafka-plaintext-files.txt")"
LOCAL_KAFKA_PLAINTEXT_UNWAIVED_COUNT="$(check_path_waivers "local_kafka_plaintext" "$LOG_DIR/repo-local-kafka-plaintext-files.txt" "$LOG_DIR/repo-local-kafka-plaintext-unwaived.txt")"
if [[ "$LOCAL_KAFKA_PLAINTEXT_COUNT" -eq 0 ]]; then
  json_log "repo" "local/dev Kafka plaintext examples absent" "info" true "ok" "0 files" "repo-local-kafka-plaintext-files.txt"
elif [[ "$LOCAL_KAFKA_PLAINTEXT_UNWAIVED_COUNT" -eq 0 ]]; then
  json_log "repo" "local/dev Kafka plaintext examples absent or explicitly waived" "info" true "explicitly_waived" "$LOCAL_KAFKA_PLAINTEXT_COUNT local/dev files covered by $SECURITY_WAIVER_FILE" "repo-local-kafka-plaintext-files.txt"
else
  json_log "repo" "local/dev Kafka plaintext examples absent or explicitly waived" "warn" false "found" "$LOCAL_KAFKA_PLAINTEXT_UNWAIVED_COUNT/$LOCAL_KAFKA_PLAINTEXT_COUNT local/dev files require non-production waiver" "repo-local-kafka-plaintext-unwaived.txt"
fi

set +e
python3 tests/e2e/image_digest_lock.py validate \
  --root deployments/kubernetes \
  --lock "$IMAGE_LOCK" \
  --out-inventory "$LOG_DIR/repo-image-lock-inventory.json" \
  --out-missing "$LOG_DIR/repo-image-lock-missing.txt" \
  --out-missing-files "$LOG_DIR/repo-unpinned-or-latest-image-files.txt" \
  --out-mutable "$LOG_DIR/repo-latest-or-mutable-image-lines.txt" \
  >"$LOG_DIR/repo-image-lock-summary.json" 2>"$LOG_DIR/repo-image-lock.err"
IMAGE_LOCK_RC=$?
set -e
if [[ -s "$LOG_DIR/repo-image-lock-summary.json" ]]; then
  REPO_IMAGE_LOCK_MISSING_FILE_COUNT="$(jq '.missing_lock_files' "$LOG_DIR/repo-image-lock-summary.json")"
  REPO_IMAGE_LOCK_MISSING_LINE_COUNT="$(jq '.missing_lock_lines' "$LOG_DIR/repo-image-lock-summary.json")"
  REPO_MUTABLE_IMAGE_COUNT="$(jq '.mutable_tag_lines' "$LOG_DIR/repo-image-lock-summary.json")"
else
  REPO_IMAGE_LOCK_MISSING_FILE_COUNT=1
  REPO_IMAGE_LOCK_MISSING_LINE_COUNT=1
  REPO_MUTABLE_IMAGE_COUNT=0
fi
UNPINNED_IMAGE_COUNT="$REPO_IMAGE_LOCK_MISSING_FILE_COUNT"
if [[ "$IMAGE_LOCK_RC" -le 1 && "$REPO_IMAGE_LOCK_MISSING_FILE_COUNT" -eq 0 ]]; then
  json_log "repo" "K8s manifest images have digest evidence lock" "info" true "ok" "0 missing image lock files" "repo-image-lock-summary.json"
else
  json_log "repo" "K8s manifest images have digest evidence lock" "blocker" false "missing" "$REPO_IMAGE_LOCK_MISSING_FILE_COUNT files / $REPO_IMAGE_LOCK_MISSING_LINE_COUNT lines missing lock evidence" "repo-unpinned-or-latest-image-files.txt"
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
  json_log "repo" "kubectl apply convergence keeps APISIX-only Service exposure" "blocker" false "rc=$APPLY_CONVERGENCE_RC" "$(head -c 800 "$LOG_DIR/k8s-core-apply-convergence.err" | tr '\n' ' ')" "k8s-core-apply-convergence.err"
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

rg -l --glob '!**/.archive/**' 'pgadmin123|minioadmin123|admin123|replicator123|traffic-ui-secret-20260617|DISABLE_SECURITY_PLUGIN|KC_HTTP_ENABLED|POSTGRES_PASSWORD[[:space:]]*:[[:space:]]*(postgres|traffic123|postgres123)|s3_access_key:[[:space:]]*"(admin|minioadmin)"|s3_secret_key:[[:space:]]*"(minioadmin|minioadmin123)"' deployments/kubernetes common go java rust >"$LOG_DIR/repo-default-credential-pattern-files.txt" || true
DEFAULT_CRED_COUNT="$(file_count "$LOG_DIR/repo-default-credential-pattern-files.txt")"
if [[ "$DEFAULT_CRED_COUNT" -eq 0 ]]; then
  json_log "repo" "default credential and disabled-security patterns absent" "info" true "ok" "0 files" "repo-default-credential-pattern-files.txt"
else
  json_log "repo" "default credential and disabled-security patterns absent" "blocker" false "found" "$DEFAULT_CRED_COUNT files" "repo-default-credential-pattern-files.txt"
fi

rg -l --glob '!**/.archive/**' '^kind:[[:space:]]*Secret$' deployments/kubernetes >"$LOG_DIR/repo-static-secret-files.txt" || true
STATIC_SECRET_COUNT="$(file_count "$LOG_DIR/repo-static-secret-files.txt")"
STATIC_SECRET_UNWAIVED_COUNT="$(check_path_waivers "raw_secret_manifests" "$LOG_DIR/repo-static-secret-files.txt" "$LOG_DIR/repo-static-secret-unwaived.txt")"
if [[ "$STATIC_SECRET_COUNT" -eq 0 ]]; then
  json_log "repo" "raw Secret manifests absent from production path" "info" true "ok" "0 files" "repo-static-secret-files.txt"
elif [[ "$STATIC_SECRET_UNWAIVED_COUNT" -eq 0 ]]; then
  json_log "repo" "raw Secret manifests absent from production path or explicitly waived" "info" true "explicitly_waived" "$STATIC_SECRET_COUNT template files covered by $SECURITY_WAIVER_FILE" "repo-static-secret-files.txt"
else
  json_log "repo" "raw Secret manifests absent from production path or explicitly waived" "warn" false "found" "$STATIC_SECRET_UNWAIVED_COUNT/$STATIC_SECRET_COUNT files require waiver" "repo-static-secret-unwaived.txt"
fi

kctl get pods -A -o json >"$LOG_DIR/live-cni-pods.json"
jq '[
  .items[]
  | . as $pod
  | ([($pod.spec.containers // [])[] | .image] | join(" ")) as $images
  | (($pod.metadata.namespace // "") + " " + ($pod.metadata.name // "") + " " + $images) as $haystack
  | select($haystack | test("flannel|calico|cilium|antrea|kube-router|ovn|ovnkube|weave|canal"; "i"))
  | {
      namespace: $pod.metadata.namespace,
      pod: $pod.metadata.name,
      node: ($pod.spec.nodeName // ""),
      phase: ($pod.status.phase // ""),
      images: [($pod.spec.containers // [])[] | .image],
      policy_capable: ($haystack | test("calico|cilium|antrea|kube-router|ovn|ovnkube|weave|canal"; "i")),
      flannel_only_marker: ($haystack | test("flannel"; "i"))
    }
]' "$LOG_DIR/live-cni-pods.json" >"$LOG_DIR/live-cni-policy-capability.json"
CNI_POLICY_CAPABLE_COUNT="$(jq '[.[] | select(.policy_capable == true)] | length' "$LOG_DIR/live-cni-policy-capability.json")"
FLANNEL_MARKER_COUNT="$(jq '[.[] | select(.flannel_only_marker == true)] | length' "$LOG_DIR/live-cni-policy-capability.json")"
jq -n \
  --argjson policy_capable_count "$CNI_POLICY_CAPABLE_COUNT" \
  --argjson flannel_marker_count "$FLANNEL_MARKER_COUNT" \
  --argfile inventory "$LOG_DIR/live-cni-policy-capability.json" \
  '{
    policy_capable_count:$policy_capable_count,
    flannel_marker_count:$flannel_marker_count,
    networkpolicy_enforcement_ready:($policy_capable_count > 0),
    inventory:$inventory
  }' >"$LOG_DIR/live-cni-policy-capability-summary.json"
if [[ "$CNI_POLICY_CAPABLE_COUNT" -gt 0 ]]; then
  json_log "live" "NetworkPolicy enforcement-capable CNI present" "info" true "ok" "$CNI_POLICY_CAPABLE_COUNT policy-capable CNI pods" "live-cni-policy-capability-summary.json"
else
  json_log "live" "NetworkPolicy enforcement-capable CNI present" "blocker" false "missing" "policy-capable CNI pods=0 flannel_markers=$FLANNEL_MARKER_COUNT" "live-cni-policy-capability-summary.json"
fi

kctl get networkpolicy -A -o json >"$LOG_DIR/live-networkpolicies.json"
LIVE_NP_COUNT="$(jq '.items | length' "$LOG_DIR/live-networkpolicies.json")"
if [[ "$LIVE_NP_COUNT" -gt 0 ]]; then
  json_log "live" "live NetworkPolicy objects present" "info" true "ok" "$LIVE_NP_COUNT policies" "live-networkpolicies.json"
else
  json_log "live" "live NetworkPolicy objects present" "blocker" false "missing" "0 policies" "live-networkpolicies.json"
fi

set +e
kctl get crd -o json >"$LOG_DIR/live-crds.json" 2>"$LOG_DIR/live-crds.err"
CRD_LIST_RC=$?
set -e
if [[ "$CRD_LIST_RC" -ne 0 ]]; then
  jq -n --rawfile err "$LOG_DIR/live-crds.err" '{items:[], error:$err}' >"$LOG_DIR/live-crds.json"
fi
jq '[
  .items[]?
  | select(.metadata.name == "externalsecrets.external-secrets.io" or .metadata.name == "sealedsecrets.bitnami.com")
  | {
      name: .metadata.name,
      created_at: (.metadata.creationTimestamp // ""),
      stored_versions: (.status.storedVersions // []),
      established: (([.status.conditions[]? | select(.type == "Established" and .status == "True")] | length) > 0)
    }
]' "$LOG_DIR/live-crds.json" >"$LOG_DIR/live-secret-operator-crds.json"
EXTERNAL_SECRET_CRD_COUNT="$(jq '[.[] | select(.name == "externalsecrets.external-secrets.io")] | length' "$LOG_DIR/live-secret-operator-crds.json")"
SEALED_SECRET_CRD_COUNT="$(jq '[.[] | select(.name == "sealedsecrets.bitnami.com")] | length' "$LOG_DIR/live-secret-operator-crds.json")"
SECRET_OPERATOR_CRD_COUNT="$(jq 'length' "$LOG_DIR/live-secret-operator-crds.json")"

jq '[
  .items[]?
  | . as $pod
  | ([($pod.spec.containers // [])[] | .image] | join(" ")) as $images
  | ([($pod.metadata.labels // {}) | to_entries[]? | "\(.key)=\(.value)"] | join(" ")) as $labels
  | (($pod.metadata.namespace // "") + " " + ($pod.metadata.name // "") + " " + $labels + " " + $images) as $haystack
  | select($haystack | test("external-secrets|external-secrets.io|sealed-secrets|sealedsecrets|bitnami/sealed"; "i"))
  | {
      namespace: $pod.metadata.namespace,
      pod: $pod.metadata.name,
      phase: ($pod.status.phase // ""),
      container_count: (($pod.spec.containers // []) | length),
      ready_containers: ([($pod.status.containerStatuses // [])[] | select(.ready == true)] | length),
      images: [($pod.spec.containers // [])[] | .image],
      labels: ($pod.metadata.labels // {})
    }
]' "$LOG_DIR/live-cni-pods.json" >"$LOG_DIR/live-secret-operator-pods.json"
SECRET_OPERATOR_POD_COUNT="$(jq 'length' "$LOG_DIR/live-secret-operator-pods.json")"
SECRET_OPERATOR_READY_POD_COUNT="$(jq '[.[] | select(.phase == "Running" and .container_count > 0 and .ready_containers == .container_count)] | length' "$LOG_DIR/live-secret-operator-pods.json")"

: >"$LOG_DIR/live-externalsecrets.err"
EXTERNAL_SECRET_GET_RC=1
if [[ "$EXTERNAL_SECRET_CRD_COUNT" -gt 0 ]]; then
  set +e
  kctl get externalsecrets.external-secrets.io -A -o json >"$LOG_DIR/live-externalsecrets.json" 2>"$LOG_DIR/live-externalsecrets.err"
  EXTERNAL_SECRET_GET_RC=$?
  set -e
else
  echo "externalsecrets.external-secrets.io CRD missing" >"$LOG_DIR/live-externalsecrets.err"
fi
if [[ "$EXTERNAL_SECRET_GET_RC" -ne 0 ]]; then
  jq -n --rawfile err "$LOG_DIR/live-externalsecrets.err" '{apiVersion:"external-secrets.io/v1beta1", kind:"ExternalSecretList", items:[], error:$err}' >"$LOG_DIR/live-externalsecrets.json"
fi
jq '[
  .items[]?
  | {
      namespace: (.metadata.namespace // ""),
      name: (.metadata.name // ""),
      secret_store_ref: (.spec.secretStoreRef // null),
      target_name: (.spec.target.name // null),
      refresh_interval: (.spec.refreshInterval // null),
      conditions: [(.status.conditions // [])[] | {type:(.type // ""), status:(.status // ""), reason:(.reason // ""), message:(.message // "")}],
      ready: (([.status.conditions[]? | select((((.type // "") == "Ready") or ((.type // "") == "Synced")) and ((.status // "") == "True"))] | length) > 0)
    }
]' "$LOG_DIR/live-externalsecrets.json" >"$LOG_DIR/live-externalsecrets-inventory.json"
LIVE_EXTERNAL_SECRET_COUNT="$(jq 'length' "$LOG_DIR/live-externalsecrets-inventory.json")"
LIVE_EXTERNAL_SECRET_READY_COUNT="$(jq '[.[] | select(.ready == true)] | length' "$LOG_DIR/live-externalsecrets-inventory.json")"

: >"$LOG_DIR/live-sealedsecrets.err"
SEALED_SECRET_GET_RC=1
if [[ "$SEALED_SECRET_CRD_COUNT" -gt 0 ]]; then
  set +e
  kctl get sealedsecrets.bitnami.com -A -o json >"$LOG_DIR/live-sealedsecrets.json" 2>"$LOG_DIR/live-sealedsecrets.err"
  SEALED_SECRET_GET_RC=$?
  set -e
else
  echo "sealedsecrets.bitnami.com CRD missing" >"$LOG_DIR/live-sealedsecrets.err"
fi
if [[ "$SEALED_SECRET_GET_RC" -ne 0 ]]; then
  jq -n --rawfile err "$LOG_DIR/live-sealedsecrets.err" '{apiVersion:"bitnami.com/v1alpha1", kind:"SealedSecretList", items:[], error:$err}' >"$LOG_DIR/live-sealedsecrets.json"
fi
jq '[
  .items[]?
  | {
      namespace: (.metadata.namespace // ""),
      name: (.metadata.name // ""),
      target_name: (.spec.template.metadata.name // .metadata.name // ""),
      conditions: [(.status.conditions // [])[] | {type:(.type // ""), status:(.status // ""), reason:(.reason // ""), message:(.message // "")}],
      synced: (([.status.conditions[]? | select((((.type // "") == "Ready") or ((.type // "") == "Synced")) and ((.status // "") == "True"))] | length) > 0)
    }
]' "$LOG_DIR/live-sealedsecrets.json" >"$LOG_DIR/live-sealedsecrets-inventory.json"
LIVE_SEALED_SECRET_COUNT="$(jq 'length' "$LOG_DIR/live-sealedsecrets-inventory.json")"
LIVE_SEALED_SECRET_SYNCED_COUNT="$(jq '[.[] | select(.synced == true)] | length' "$LOG_DIR/live-sealedsecrets-inventory.json")"

python3 - "$EXTERNAL_SECRET_TEMPLATE_FILE" >"$LOG_DIR/expected-production-externalsecrets.json" <<'PY'
import json
import sys

import yaml

path = sys.argv[1]
items = []
with open(path, "r", encoding="utf-8") as fh:
    for doc in yaml.safe_load_all(fh):
        if not isinstance(doc, dict):
            continue
        if doc.get("kind") != "ExternalSecret":
            continue
        metadata = doc.get("metadata") or {}
        spec = doc.get("spec") or {}
        target = spec.get("target") or {}
        items.append(
            {
                "apiVersion": doc.get("apiVersion", ""),
                "namespace": metadata.get("namespace", ""),
                "name": metadata.get("name", ""),
                "target_name": target.get("name") or metadata.get("name", ""),
                "secret_store_ref": spec.get("secretStoreRef") or {},
            }
        )
json.dump(items, sys.stdout, ensure_ascii=False, indent=2)
sys.stdout.write("\n")
PY
PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT="$(jq 'length' "$LOG_DIR/expected-production-externalsecrets.json")"
jq -n \
  --argfile expected "$LOG_DIR/expected-production-externalsecrets.json" \
  --argfile live "$LOG_DIR/live-externalsecrets-inventory.json" \
  '[
    $expected[]
    | . as $e
    | (first($live[] | select(.namespace == $e.namespace and .name == $e.name)) // null) as $match
    | {
        namespace:$e.namespace,
        name:$e.name,
        target_name:$e.target_name,
        apiVersion:$e.apiVersion,
        live:($match != null),
        ready:(($match.ready // false) == true),
        condition_summary:($match.conditions // []),
        secret_store_ref:($match.secret_store_ref // $e.secret_store_ref)
      }
  ]' >"$LOG_DIR/live-production-externalsecret-readiness.json"
PRODUCTION_EXTERNAL_SECRET_READY_COUNT="$(jq '[.[] | select(.ready == true)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json")"
PRODUCTION_EXTERNAL_SECRET_MISSING_COUNT="$(jq '[.[] | select(.live == false)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json")"
PRODUCTION_EXTERNAL_SECRET_NOT_READY_COUNT="$(jq '[.[] | select(.live == true and .ready == false)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json")"
PRODUCTION_EXTERNAL_SECRET_BLOCKERS=0
if [[ "$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT" -eq 0 || "$PRODUCTION_EXTERNAL_SECRET_READY_COUNT" -ne "$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT" ]]; then
  PRODUCTION_EXTERNAL_SECRET_BLOCKERS=1
fi
if [[ "$PRODUCTION_EXTERNAL_SECRET_BLOCKERS" -eq 0 ]]; then
  json_log "live" "production ExternalSecret templates are live and Ready" "info" true "ok" "$PRODUCTION_EXTERNAL_SECRET_READY_COUNT/$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT expected objects ready" "live-production-externalsecret-readiness.json"
else
  json_log "live" "production ExternalSecret templates are live and Ready" "blocker" false "not_ready" "expected=$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT ready=$PRODUCTION_EXTERNAL_SECRET_READY_COUNT missing=$PRODUCTION_EXTERNAL_SECRET_MISSING_COUNT not_ready=$PRODUCTION_EXTERNAL_SECRET_NOT_READY_COUNT" "live-production-externalsecret-readiness.json"
fi

SECRET_RECONCILIATION_OBJECT_COUNT=$((LIVE_EXTERNAL_SECRET_COUNT + LIVE_SEALED_SECRET_COUNT))
SECRET_RECONCILIATION_READY_COUNT=$((LIVE_EXTERNAL_SECRET_READY_COUNT + LIVE_SEALED_SECRET_SYNCED_COUNT))
SECRET_RECONCILIATION_BLOCKERS=0
if [[ "$SECRET_OPERATOR_CRD_COUNT" -eq 0 || "$SECRET_OPERATOR_READY_POD_COUNT" -eq 0 || "$SECRET_RECONCILIATION_OBJECT_COUNT" -eq 0 || "$PRODUCTION_EXTERNAL_SECRET_BLOCKERS" -ne 0 ]]; then
  SECRET_RECONCILIATION_BLOCKERS=1
elif [[ "$LIVE_EXTERNAL_SECRET_COUNT" -ne "$LIVE_EXTERNAL_SECRET_READY_COUNT" || "$LIVE_SEALED_SECRET_COUNT" -ne "$LIVE_SEALED_SECRET_SYNCED_COUNT" ]]; then
  SECRET_RECONCILIATION_BLOCKERS=1
fi
jq -n \
  --argjson crd_list_rc "$CRD_LIST_RC" \
  --argjson external_secret_crd_count "$EXTERNAL_SECRET_CRD_COUNT" \
  --argjson sealed_secret_crd_count "$SEALED_SECRET_CRD_COUNT" \
  --argjson secret_operator_crd_count "$SECRET_OPERATOR_CRD_COUNT" \
  --argjson secret_operator_pod_count "$SECRET_OPERATOR_POD_COUNT" \
  --argjson secret_operator_ready_pod_count "$SECRET_OPERATOR_READY_POD_COUNT" \
  --argjson external_secret_get_rc "$EXTERNAL_SECRET_GET_RC" \
  --argjson sealed_secret_get_rc "$SEALED_SECRET_GET_RC" \
  --argjson live_external_secret_count "$LIVE_EXTERNAL_SECRET_COUNT" \
  --argjson live_external_secret_ready_count "$LIVE_EXTERNAL_SECRET_READY_COUNT" \
  --argjson live_sealed_secret_count "$LIVE_SEALED_SECRET_COUNT" \
  --argjson live_sealed_secret_synced_count "$LIVE_SEALED_SECRET_SYNCED_COUNT" \
  --argjson production_external_secret_expected_count "$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT" \
  --argjson production_external_secret_ready_count "$PRODUCTION_EXTERNAL_SECRET_READY_COUNT" \
  --argjson production_external_secret_missing_count "$PRODUCTION_EXTERNAL_SECRET_MISSING_COUNT" \
  --argjson production_external_secret_not_ready_count "$PRODUCTION_EXTERNAL_SECRET_NOT_READY_COUNT" \
  --argjson production_external_secret_blockers "$PRODUCTION_EXTERNAL_SECRET_BLOCKERS" \
  --argjson secret_reconciliation_object_count "$SECRET_RECONCILIATION_OBJECT_COUNT" \
  --argjson secret_reconciliation_ready_count "$SECRET_RECONCILIATION_READY_COUNT" \
  --argjson secret_reconciliation_blockers "$SECRET_RECONCILIATION_BLOCKERS" \
  --argfile crds "$LOG_DIR/live-secret-operator-crds.json" \
  --argfile operator_pods "$LOG_DIR/live-secret-operator-pods.json" \
  --argfile externalsecrets "$LOG_DIR/live-externalsecrets-inventory.json" \
  --argfile sealedsecrets "$LOG_DIR/live-sealedsecrets-inventory.json" \
  --argfile production_externalsecrets "$LOG_DIR/live-production-externalsecret-readiness.json" \
  '{
    crd_list_rc:$crd_list_rc,
    external_secret_crd_count:$external_secret_crd_count,
    sealed_secret_crd_count:$sealed_secret_crd_count,
    secret_operator_crd_count:$secret_operator_crd_count,
    secret_operator_pod_count:$secret_operator_pod_count,
    secret_operator_ready_pod_count:$secret_operator_ready_pod_count,
    external_secret_get_rc:$external_secret_get_rc,
    sealed_secret_get_rc:$sealed_secret_get_rc,
    live_external_secret_count:$live_external_secret_count,
    live_external_secret_ready_count:$live_external_secret_ready_count,
    live_sealed_secret_count:$live_sealed_secret_count,
    live_sealed_secret_synced_count:$live_sealed_secret_synced_count,
    production_external_secret_expected_count:$production_external_secret_expected_count,
    production_external_secret_ready_count:$production_external_secret_ready_count,
    production_external_secret_missing_count:$production_external_secret_missing_count,
    production_external_secret_not_ready_count:$production_external_secret_not_ready_count,
    production_external_secret_blockers:$production_external_secret_blockers,
    secret_reconciliation_object_count:$secret_reconciliation_object_count,
    secret_reconciliation_ready_count:$secret_reconciliation_ready_count,
    secret_reconciliation_blockers:$secret_reconciliation_blockers,
    operator_backed_reconciliation_ready:($secret_reconciliation_blockers == 0),
    crds:$crds,
    operator_pods:$operator_pods,
    externalsecrets:$externalsecrets,
    sealedsecrets:$sealedsecrets,
    production_externalsecrets:$production_externalsecrets
  }' >"$LOG_DIR/live-external-secret-reconciliation-summary.json"
if [[ "$SECRET_RECONCILIATION_BLOCKERS" -eq 0 ]]; then
  json_log "live" "live ExternalSecret/SealedSecret operator reconciles secrets" "info" true "ok" "crds=$SECRET_OPERATOR_CRD_COUNT ready_pods=$SECRET_OPERATOR_READY_POD_COUNT externalsecrets=$LIVE_EXTERNAL_SECRET_COUNT/$LIVE_EXTERNAL_SECRET_READY_COUNT sealedsecrets=$LIVE_SEALED_SECRET_COUNT/$LIVE_SEALED_SECRET_SYNCED_COUNT" "live-external-secret-reconciliation-summary.json"
else
  json_log "live" "live ExternalSecret/SealedSecret operator reconciles secrets" "blocker" false "not_reconciled" "crds=$SECRET_OPERATOR_CRD_COUNT ready_pods=$SECRET_OPERATOR_READY_POD_COUNT externalsecrets=$LIVE_EXTERNAL_SECRET_COUNT/$LIVE_EXTERNAL_SECRET_READY_COUNT sealedsecrets=$LIVE_SEALED_SECRET_COUNT/$LIVE_SEALED_SECRET_SYNCED_COUNT" "live-external-secret-reconciliation-summary.json"
fi

kctl get svc -A -o json >"$LOG_DIR/live-services.json"
jq '[
  .items[]
  | select(.spec.type == "NodePort" or .spec.type == "LoadBalancer")
  | . as $svc
  | .spec.ports[]
  | {
      namespace: $svc.metadata.namespace,
      service: $svc.metadata.name,
      type: $svc.spec.type,
      port_name: (.name // ""),
      port: .port,
      node_port: (.nodePort // null),
      target_port: (.targetPort // null),
      allowed_public_business_port: (
        $svc.metadata.namespace == "gateway"
        and $svc.metadata.name == "apisix"
        and (.name // "") == "http"
        and .port == 9080
        and (.nodePort // 0) == 30180
      )
    }
]' "$LOG_DIR/live-services.json" >"$LOG_DIR/live-external-service-ports.json"
jq '[.[] | select(.allowed_public_business_port | not)]' "$LOG_DIR/live-external-service-ports.json" >"$LOG_DIR/live-external-service-blockers.json"
EXTERNAL_PORT_COUNT="$(jq 'length' "$LOG_DIR/live-external-service-ports.json")"
EXTERNAL_BLOCKER_COUNT="$(jq 'length' "$LOG_DIR/live-external-service-blockers.json")"
if [[ "$EXTERNAL_BLOCKER_COUNT" -eq 0 ]]; then
  json_log "live" "only APISIX business NodePort is externally exposed" "info" true "ok" "$EXTERNAL_PORT_COUNT external ports" "live-external-service-ports.json"
else
  json_log "live" "only APISIX business NodePort is externally exposed" "blocker" false "found" "$EXTERNAL_BLOCKER_COUNT non-business external ports" "live-external-service-blockers.json"
fi

kctl get deploy,sts,ds -A -o json >"$LOG_DIR/live-workloads.json"
jq '[
  .items[]
  | . as $w
  | ([($w.spec.template.spec.initContainers // [])[], ($w.spec.template.spec.containers // [])[]])
  | .[]
  | select(((.image // "") | contains("@sha256:") | not) or ((.image // "") | endswith(":latest")))
  | {
      namespace: $w.metadata.namespace,
      kind: $w.kind,
      workload: $w.metadata.name,
      container: .name,
      image: .image
    }
]' "$LOG_DIR/live-workloads.json" >"$LOG_DIR/live-unpinned-or-latest-images.json"
LIVE_UNPINNED_IMAGE_COUNT="$(jq 'length' "$LOG_DIR/live-unpinned-or-latest-images.json")"
if [[ "$LIVE_UNPINNED_IMAGE_COUNT" -eq 0 ]]; then
  json_log "live" "live workload images are digest-pinned and avoid latest" "info" true "ok" "0 containers" "live-unpinned-or-latest-images.json"
else
  json_log "live" "live workload images are digest-pinned and avoid latest" "blocker" false "found" "$LIVE_UNPINNED_IMAGE_COUNT containers" "live-unpinned-or-latest-images.json"
fi

jq '[
  .items[]
  | . as $w
  | ([($w.spec.template.spec.initContainers // [])[], ($w.spec.template.spec.containers // [])[]])
  | .[]
  | select((.securityContext.privileged // false) == true)
  | {
      namespace: $w.metadata.namespace,
      kind: $w.kind,
      workload: $w.metadata.name,
      container: .name
    }
]' "$LOG_DIR/live-workloads.json" >"$LOG_DIR/live-privileged-containers.json"
PRIVILEGED_COUNT="$(jq 'length' "$LOG_DIR/live-privileged-containers.json")"
PRIVILEGED_UNWAIVED_COUNT="$(check_workload_waivers "privileged_containers" "$LOG_DIR/live-privileged-containers.json" "$LOG_DIR/live-privileged-containers-unwaived.json")"
if [[ "$PRIVILEGED_COUNT" -eq 0 ]]; then
  json_log "live" "privileged containers absent or explicitly waived" "info" true "ok" "0 containers" "live-privileged-containers.json"
elif [[ "$PRIVILEGED_UNWAIVED_COUNT" -eq 0 ]]; then
  json_log "live" "privileged containers absent or explicitly waived" "info" true "explicitly_waived" "$PRIVILEGED_COUNT containers covered by $SECURITY_WAIVER_FILE" "live-privileged-containers.json"
else
  json_log "live" "privileged containers absent or explicitly waived" "warn" false "found" "$PRIVILEGED_UNWAIVED_COUNT/$PRIVILEGED_COUNT containers require waiver" "live-privileged-containers-unwaived.json"
fi

jq '[
  .items[]
  | . as $w
  | select(($w.spec.template.spec.hostNetwork // false) == true or ($w.spec.template.spec.hostPID // false) == true)
  | {
      namespace: $w.metadata.namespace,
      kind: $w.kind,
      workload: $w.metadata.name,
      host_network: ($w.spec.template.spec.hostNetwork // false),
      host_pid: ($w.spec.template.spec.hostPID // false)
    }
]' "$LOG_DIR/live-workloads.json" >"$LOG_DIR/live-host-namespace-workloads.json"
HOST_NS_COUNT="$(jq 'length' "$LOG_DIR/live-host-namespace-workloads.json")"
HOST_NS_UNWAIVED_COUNT="$(check_workload_waivers "host_namespace_workloads" "$LOG_DIR/live-host-namespace-workloads.json" "$LOG_DIR/live-host-namespace-workloads-unwaived.json")"
if [[ "$HOST_NS_COUNT" -eq 0 ]]; then
  json_log "live" "hostNetwork or hostPID workloads absent or explicitly waived" "info" true "ok" "0 workloads" "live-host-namespace-workloads.json"
elif [[ "$HOST_NS_UNWAIVED_COUNT" -eq 0 ]]; then
  json_log "live" "hostNetwork or hostPID workloads absent or explicitly waived" "info" true "explicitly_waived" "$HOST_NS_COUNT workloads covered by $SECURITY_WAIVER_FILE" "live-host-namespace-workloads.json"
else
  json_log "live" "hostNetwork or hostPID workloads absent or explicitly waived" "warn" false "found" "$HOST_NS_UNWAIVED_COUNT/$HOST_NS_COUNT workloads require waiver" "live-host-namespace-workloads-unwaived.json"
fi

set +e
kctl -n middleware get sts kafka -o json >"$LOG_DIR/live-kafka-statefulset.json" 2>"$LOG_DIR/live-kafka-statefulset.err"
KAFKA_STS_RC=$?
set -e
if [[ "$KAFKA_STS_RC" -eq 0 ]]; then
  jq -r '[.spec.template.spec.containers[]?.args[]?] | join("\n")' "$LOG_DIR/live-kafka-statefulset.json" >"$LOG_DIR/live-kafka-args.txt"
  if rg -q 'PLAINTEXT|inter\.broker\.listener\.name=PLAINTEXT' "$LOG_DIR/live-kafka-args.txt"; then
    json_log "live" "live Kafka TLS/SASL listener profile enabled" "blocker" false "plaintext" "Kafka StatefulSet args contain plaintext listener markers" "live-kafka-args.txt"
  else
    json_log "live" "live Kafka TLS/SASL listener profile enabled" "info" true "ok" "no plaintext markers" "live-kafka-args.txt"
  fi
else
  json_log "live" "live Kafka TLS/SASL listener profile enabled" "blocker" false "missing" "cannot read Kafka StatefulSet" "live-kafka-statefulset.err"
fi

KEYCLOAK_PROFILE_BLOCKERS=0
KEYCLOAK_SVC_HTTP_CODE="000"
set +e
kctl -n iam get sts keycloak -o json >"$LOG_DIR/live-keycloak-statefulset.json" 2>"$LOG_DIR/live-keycloak-statefulset.err"
KEYCLOAK_STS_RC=$?
kctl -n iam get svc keycloak -o json >"$LOG_DIR/live-keycloak-service.json" 2>"$LOG_DIR/live-keycloak-service.err"
KEYCLOAK_SVC_RC=$?
kctl -n iam get endpoints keycloak -o json >"$LOG_DIR/live-keycloak-endpoints.json" 2>"$LOG_DIR/live-keycloak-endpoints.err"
KEYCLOAK_ENDPOINTS_RC=$?
set -e
if [[ "$KEYCLOAK_STS_RC" -eq 0 && "$KEYCLOAK_SVC_RC" -eq 0 && "$KEYCLOAK_ENDPOINTS_RC" -eq 0 ]]; then
  jq '{
    image: .spec.template.spec.containers[0].image,
    args: (.spec.template.spec.containers[0].args // []),
    env: [(.spec.template.spec.containers[0].env // [])[] | {name, has_value:(has("value")), has_valueFrom:(has("valueFrom")), value:(.value // null), secret:(.valueFrom.secretKeyRef.name // null), key:(.valueFrom.secretKeyRef.key // null)}]
  }' "$LOG_DIR/live-keycloak-statefulset.json" >"$LOG_DIR/live-keycloak-profile.json"
  KEYCLOAK_START_DEV_COUNT="$(jq '[.args[]? | select(. == "start-dev")] | length' "$LOG_DIR/live-keycloak-profile.json")"
  KEYCLOAK_HTTP_ENABLED_COUNT="$(jq '[.env[] | select(.name == "KC_HTTP_ENABLED" and (.value // "") == "true")] | length' "$LOG_DIR/live-keycloak-profile.json")"
  KEYCLOAK_DIRECT_PASSWORD_COUNT="$(jq '[.env[] | select(.name == "KEYCLOAK_ADMIN_PASSWORD" and .has_value == true)] | length' "$LOG_DIR/live-keycloak-profile.json")"
  KEYCLOAK_PASSWORD_SECRET_COUNT="$(jq '[.env[] | select(.name == "KEYCLOAK_ADMIN_PASSWORD" and .secret == "traffic-credentials" and .key == "KEYCLOAK_ADMIN_PASSWORD")] | length' "$LOG_DIR/live-keycloak-profile.json")"
  KEYCLOAK_SERVICE_HTTPS_COUNT="$(jq '[.spec.ports[]? | select(.port == 8443 and (.targetPort == 8443 or .targetPort == "https"))] | length' "$LOG_DIR/live-keycloak-service.json")"
  KEYCLOAK_READY_ENDPOINT_COUNT="$(jq '[.subsets[]?.addresses[]?] | length' "$LOG_DIR/live-keycloak-endpoints.json")"
  KEYCLOAK_SERVICE_IP="$(jq -r '.spec.clusterIP // ""' "$LOG_DIR/live-keycloak-service.json")"
  if [[ -n "$KEYCLOAK_SERVICE_IP" && "$KEYCLOAK_SERVICE_IP" != "None" ]]; then
    set +e
    KEYCLOAK_SVC_HTTP_CODE="$(curl --noproxy '*' -ksS -o "$LOG_DIR/live-keycloak-https-root.txt" -w '%{http_code}' "https://$KEYCLOAK_SERVICE_IP:8443/" 2>"$LOG_DIR/live-keycloak-https-root.err")"
    KEYCLOAK_CURL_RC=$?
    set -e
    if [[ "$KEYCLOAK_CURL_RC" -ne 0 ]]; then
      KEYCLOAK_SVC_HTTP_CODE="000"
    fi
  fi
  jq -n \
    --argjson start_dev "$KEYCLOAK_START_DEV_COUNT" \
    --argjson http_enabled "$KEYCLOAK_HTTP_ENABLED_COUNT" \
    --argjson direct_password "$KEYCLOAK_DIRECT_PASSWORD_COUNT" \
    --argjson password_secret "$KEYCLOAK_PASSWORD_SECRET_COUNT" \
    --argjson service_https "$KEYCLOAK_SERVICE_HTTPS_COUNT" \
    --argjson ready_endpoints "$KEYCLOAK_READY_ENDPOINT_COUNT" \
    --arg http_code "$KEYCLOAK_SVC_HTTP_CODE" \
    '{
      start_dev_args:$start_dev,
      http_enabled_env:$http_enabled,
      direct_admin_password_env:$direct_password,
      admin_password_secret_refs:$password_secret,
      https_service_ports:$service_https,
      ready_endpoint_count:$ready_endpoints,
      https_root_http_code:$http_code,
      keycloak_live_profile_ready:(
        $start_dev == 0
        and $http_enabled == 0
        and $direct_password == 0
        and $password_secret > 0
        and $service_https > 0
        and $ready_endpoints > 0
        and ($http_code | test("^(200|301|302|303|401|403)$"))
      )
    }' >"$LOG_DIR/live-keycloak-profile-summary.json"
  KEYCLOAK_PROFILE_READY="$(jq '.keycloak_live_profile_ready' "$LOG_DIR/live-keycloak-profile-summary.json")"
  if [[ "$KEYCLOAK_PROFILE_READY" == "true" ]]; then
    json_log "live" "live Keycloak TLS/SecretRef profile enabled" "info" true "ok" "https=$KEYCLOAK_SVC_HTTP_CODE endpoints=$KEYCLOAK_READY_ENDPOINT_COUNT" "live-keycloak-profile-summary.json"
  else
    KEYCLOAK_PROFILE_BLOCKERS=1
    json_log "live" "live Keycloak TLS/SecretRef profile enabled" "blocker" false "drift" "start-dev=$KEYCLOAK_START_DEV_COUNT http-env=$KEYCLOAK_HTTP_ENABLED_COUNT direct-password=$KEYCLOAK_DIRECT_PASSWORD_COUNT https=$KEYCLOAK_SVC_HTTP_CODE endpoints=$KEYCLOAK_READY_ENDPOINT_COUNT" "live-keycloak-profile-summary.json"
  fi
else
  KEYCLOAK_PROFILE_BLOCKERS=1
  jq -n \
    --argjson sts_rc "$KEYCLOAK_STS_RC" \
    --argjson svc_rc "$KEYCLOAK_SVC_RC" \
    --argjson endpoints_rc "$KEYCLOAK_ENDPOINTS_RC" \
    '{keycloak_live_profile_ready:false, statefulset_rc:$sts_rc, service_rc:$svc_rc, endpoints_rc:$endpoints_rc}' \
    >"$LOG_DIR/live-keycloak-profile-summary.json"
  json_log "live" "live Keycloak TLS/SecretRef profile enabled" "blocker" false "missing" "cannot read Keycloak StatefulSet/Service/Endpoints" "live-keycloak-profile-summary.json"
fi

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
  --arg network_policy_file "$NETWORK_POLICY_FILE" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson external_port_count "$EXTERNAL_PORT_COUNT" \
  --argjson external_blocker_count "$EXTERNAL_BLOCKER_COUNT" \
  --argjson live_network_policy_count "$LIVE_NP_COUNT" \
  --argjson live_unpinned_image_count "$LIVE_UNPINNED_IMAGE_COUNT" \
  --argjson repo_kafka_plaintext_count "$KAFKA_PLAINTEXT_COUNT" \
  --argjson repo_image_lock_missing_files "$REPO_IMAGE_LOCK_MISSING_FILE_COUNT" \
  --argjson repo_image_lock_missing_lines "$REPO_IMAGE_LOCK_MISSING_LINE_COUNT" \
  --argjson repo_mutable_image_count "$REPO_MUTABLE_IMAGE_COUNT" \
  --argjson repo_service_exposure_blockers "$REPO_SERVICE_EXPOSURE_BLOCKERS" \
  --argjson repo_service_external_ports "$REPO_SERVICE_EXTERNAL_PORTS" \
  --argjson apply_convergence_service_blockers "$APPLY_CONVERGENCE_SERVICE_BLOCKERS" \
  --argjson apply_convergence_external_ports "$APPLY_CONVERGENCE_EXTERNAL_PORTS" \
  --argjson repo_default_credential_count "$DEFAULT_CRED_COUNT" \
  --argjson repo_static_secret_count "$STATIC_SECRET_COUNT" \
  --argjson local_kafka_plaintext_unwaived_count "$LOCAL_KAFKA_PLAINTEXT_UNWAIVED_COUNT" \
  --argjson static_secret_unwaived_count "$STATIC_SECRET_UNWAIVED_COUNT" \
  --argjson privileged_unwaived_count "$PRIVILEGED_UNWAIVED_COUNT" \
  --argjson host_namespace_unwaived_count "$HOST_NS_UNWAIVED_COUNT" \
  --argjson cni_policy_capable_count "$CNI_POLICY_CAPABLE_COUNT" \
  --argjson secret_operator_crd_count "$SECRET_OPERATOR_CRD_COUNT" \
  --argjson secret_operator_pod_count "$SECRET_OPERATOR_POD_COUNT" \
  --argjson secret_operator_ready_pod_count "$SECRET_OPERATOR_READY_POD_COUNT" \
  --argjson live_external_secret_count "$LIVE_EXTERNAL_SECRET_COUNT" \
  --argjson live_external_secret_ready_count "$LIVE_EXTERNAL_SECRET_READY_COUNT" \
  --argjson live_sealed_secret_count "$LIVE_SEALED_SECRET_COUNT" \
  --argjson live_sealed_secret_synced_count "$LIVE_SEALED_SECRET_SYNCED_COUNT" \
  --argjson production_external_secret_expected_count "$PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT" \
  --argjson production_external_secret_ready_count "$PRODUCTION_EXTERNAL_SECRET_READY_COUNT" \
  --argjson production_external_secret_missing_count "$PRODUCTION_EXTERNAL_SECRET_MISSING_COUNT" \
  --argjson production_external_secret_not_ready_count "$PRODUCTION_EXTERNAL_SECRET_NOT_READY_COUNT" \
  --argjson production_external_secret_blockers "$PRODUCTION_EXTERNAL_SECRET_BLOCKERS" \
  --argjson secret_reconciliation_blockers "$SECRET_RECONCILIATION_BLOCKERS" \
  --argjson keycloak_profile_blockers "$KEYCLOAK_PROFILE_BLOCKERS" \
  '{
    run_id: $run_id,
    result: $result,
    report: $report,
    local_report: $local_report,
    network_policy_file: $network_policy_file,
    total: $total,
    passed: $passed,
    blockers: $blockers,
    warnings: $warnings,
    external_port_count: $external_port_count,
    external_blocker_count: $external_blocker_count,
    live_network_policy_count: $live_network_policy_count,
    live_unpinned_image_count: $live_unpinned_image_count,
    repo_kafka_plaintext_count: $repo_kafka_plaintext_count,
    repo_image_lock_missing_files: $repo_image_lock_missing_files,
    repo_image_lock_missing_lines: $repo_image_lock_missing_lines,
    repo_mutable_image_count: $repo_mutable_image_count,
    repo_service_exposure_blockers: $repo_service_exposure_blockers,
    repo_service_external_ports: $repo_service_external_ports,
    apply_convergence_service_blockers: $apply_convergence_service_blockers,
    apply_convergence_external_ports: $apply_convergence_external_ports,
    repo_default_credential_count: $repo_default_credential_count,
    repo_static_secret_count: $repo_static_secret_count,
    local_kafka_plaintext_unwaived_count: $local_kafka_plaintext_unwaived_count,
    static_secret_unwaived_count: $static_secret_unwaived_count,
    privileged_unwaived_count: $privileged_unwaived_count,
    host_namespace_unwaived_count: $host_namespace_unwaived_count,
    cni_policy_capable_count: $cni_policy_capable_count,
    secret_operator_crd_count: $secret_operator_crd_count,
    secret_operator_pod_count: $secret_operator_pod_count,
    secret_operator_ready_pod_count: $secret_operator_ready_pod_count,
    live_external_secret_count: $live_external_secret_count,
    live_external_secret_ready_count: $live_external_secret_ready_count,
    live_sealed_secret_count: $live_sealed_secret_count,
    live_sealed_secret_synced_count: $live_sealed_secret_synced_count,
    production_external_secret_expected_count: $production_external_secret_expected_count,
    production_external_secret_ready_count: $production_external_secret_ready_count,
    production_external_secret_missing_count: $production_external_secret_missing_count,
    production_external_secret_not_ready_count: $production_external_secret_not_ready_count,
    production_external_secret_blockers: $production_external_secret_blockers,
    secret_reconciliation_blockers: $secret_reconciliation_blockers,
    keycloak_profile_blockers: $keycloak_profile_blockers,
    checks: .
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# Production Security Preflight

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This is a non-destructive live/repo preflight for GATE-P0-07 and GATE-P0-10. It does not apply NetworkPolicy, rotate secrets, or switch Kafka listeners.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| External service ports | $EXTERNAL_PORT_COUNT |
| Non-business external ports | $EXTERNAL_BLOCKER_COUNT |
| Live NetworkPolicies | $LIVE_NP_COUNT |
| Live unpinned/latest images | $LIVE_UNPINNED_IMAGE_COUNT |
| Repo Kafka plaintext files | $KAFKA_PLAINTEXT_COUNT |
| Repo image lock missing files | $REPO_IMAGE_LOCK_MISSING_FILE_COUNT |
| Repo mutable/latest image lines with lock evidence | $REPO_MUTABLE_IMAGE_COUNT |
| Repo non-business external service ports | $REPO_SERVICE_EXPOSURE_BLOCKERS |
| Apply-convergence non-business external service ports | $APPLY_CONVERGENCE_SERVICE_BLOCKERS |
| Repo default credential pattern files | $DEFAULT_CRED_COUNT |
| Repo raw Secret manifest files | $STATIC_SECRET_COUNT |
| Local/dev Kafka plaintext unwaived files | $LOCAL_KAFKA_PLAINTEXT_UNWAIVED_COUNT |
| Repo raw Secret unwaived files | $STATIC_SECRET_UNWAIVED_COUNT |
| Privileged unwaived containers | $PRIVILEGED_UNWAIVED_COUNT |
| Host namespace unwaived workloads | $HOST_NS_UNWAIVED_COUNT |
| NetworkPolicy-capable CNI pods | $CNI_POLICY_CAPABLE_COUNT |
| Secret operator CRDs | $SECRET_OPERATOR_CRD_COUNT |
| Secret operator ready pods | $SECRET_OPERATOR_READY_POD_COUNT |
| ExternalSecrets ready | $LIVE_EXTERNAL_SECRET_READY_COUNT / $LIVE_EXTERNAL_SECRET_COUNT |
| SealedSecrets synced | $LIVE_SEALED_SECRET_SYNCED_COUNT / $LIVE_SEALED_SECRET_COUNT |
| Production ExternalSecrets ready | $PRODUCTION_EXTERNAL_SECRET_READY_COUNT / $PRODUCTION_EXTERNAL_SECRET_EXPECTED_COUNT |
| Production ExternalSecrets missing | $PRODUCTION_EXTERNAL_SECRET_MISSING_COUNT |
| Production ExternalSecrets not ready | $PRODUCTION_EXTERNAL_SECRET_NOT_READY_COUNT |
| Secret reconciliation blockers | $SECRET_RECONCILIATION_BLOCKERS |
| Keycloak live profile blockers | $KEYCLOAK_PROFILE_BLOCKERS |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`network-policy-dry-run.txt\`
- \`live-external-service-blockers.json\`
- \`live-networkpolicies.json\`
- \`live-cni-policy-capability-summary.json\`
- \`repo-default-credential-pattern-files.txt\`
- \`repo-kafka-plaintext-files.txt\`
- \`repo-unpinned-or-latest-image-files.txt\`
- \`repo-image-lock-summary.json\`
- \`repo-latest-or-mutable-image-lines.txt\`
- \`repo-service-exposure-summary.json\`
- \`repo-service-exposure-blockers.json\`
- \`apply-convergence-service-exposure-summary.json\`
- \`apply-convergence-service-exposure-blockers.json\`
- \`repo-external-secret-files.txt\`
- \`live-external-secret-reconciliation-summary.json\`
- \`live-secret-operator-crds.json\`
- \`live-secret-operator-pods.json\`
- \`live-externalsecrets-inventory.json\`
- \`live-sealedsecrets-inventory.json\`
- \`expected-production-externalsecrets.json\`
- \`live-production-externalsecret-readiness.json\`
- \`live-keycloak-profile-summary.json\`
- \`production-security-waivers.json\`
- \`repo-local-kafka-plaintext-unwaived.txt\`
- \`repo-static-secret-unwaived.txt\`
- \`live-privileged-containers-unwaived.json\`
- \`live-host-namespace-workloads-unwaived.json\`

## Interpretation

The starter NetworkPolicy profile can be client-side dry-run checked, production manifests no longer contain Kafka plaintext listener definitions, repo image references are covered by an explicit evidence lock, repo Service exposure is limited to the APISIX business port, and kubectl apply convergence behavior is explicitly checked. The live cluster remains blocked for production security when the live Kafka SASL_SSL/TLS listener, NetworkPolicy-capable CNI, operator-backed ExternalSecret/SealedSecret reconciliation, or the production ExternalSecret templates report blockers. Keycloak TLS/SecretRef readiness and digest-pinned live workload images are measured by their own counters in this report instead of being assumed.
EOF

cp "$SUMMARY" "$SECURITY_DIR/production-security-preflight-latest.json"
cp "$LOCAL_REPORT" "$SECURITY_DIR/production-security-preflight-latest.md"
cp "$LOG_DIR/live-external-service-blockers.json" "$SECURITY_DIR/external-service-blockers-latest.json"
cp "$LOG_DIR/live-networkpolicies.json" "$SECURITY_DIR/network-policy-live-latest.json"
cp "$LOG_DIR/live-cni-policy-capability-summary.json" "$SECURITY_DIR/live-cni-policy-capability-summary-latest.json"
cp "$LOG_DIR/live-cni-policy-capability.json" "$SECURITY_DIR/live-cni-policy-capability-latest.json"
cp "$LOG_DIR/repo-default-credential-pattern-files.txt" "$SECURITY_DIR/repo-default-credential-pattern-files-latest.txt"
cp "$LOG_DIR/repo-kafka-plaintext-files.txt" "$SECURITY_DIR/repo-kafka-plaintext-files-latest.txt"
cp "$LOG_DIR/repo-unpinned-or-latest-image-files.txt" "$SECURITY_DIR/repo-unpinned-or-latest-image-files-latest.txt"
cp "$LOG_DIR/repo-image-lock-summary.json" "$SECURITY_DIR/repo-image-lock-summary-latest.json"
cp "$LOG_DIR/repo-image-lock-inventory.json" "$SECURITY_DIR/repo-image-lock-inventory-latest.json"
cp "$LOG_DIR/repo-latest-or-mutable-image-lines.txt" "$SECURITY_DIR/repo-latest-or-mutable-image-lines-latest.txt"
cp "$LOG_DIR/repo-service-exposure-summary.json" "$SECURITY_DIR/repo-service-exposure-summary-latest.json"
cp "$LOG_DIR/repo-service-exposure-inventory.json" "$SECURITY_DIR/repo-service-exposure-inventory-latest.json"
cp "$LOG_DIR/repo-service-exposure-blockers.json" "$SECURITY_DIR/repo-service-exposure-blockers-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-summary.json" "$SECURITY_DIR/apply-convergence-service-exposure-summary-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-inventory.json" "$SECURITY_DIR/apply-convergence-service-exposure-inventory-latest.json"
cp "$LOG_DIR/apply-convergence-service-exposure-blockers.json" "$SECURITY_DIR/apply-convergence-service-exposure-blockers-latest.json"
cp "$LOG_DIR/live-external-secret-reconciliation-summary.json" "$SECURITY_DIR/live-external-secret-reconciliation-summary-latest.json"
cp "$LOG_DIR/live-secret-operator-crds.json" "$SECURITY_DIR/live-secret-operator-crds-latest.json"
cp "$LOG_DIR/live-secret-operator-pods.json" "$SECURITY_DIR/live-secret-operator-pods-latest.json"
cp "$LOG_DIR/live-externalsecrets-inventory.json" "$SECURITY_DIR/live-externalsecrets-inventory-latest.json"
cp "$LOG_DIR/live-sealedsecrets-inventory.json" "$SECURITY_DIR/live-sealedsecrets-inventory-latest.json"
cp "$LOG_DIR/expected-production-externalsecrets.json" "$SECURITY_DIR/expected-production-externalsecrets-latest.json"
cp "$LOG_DIR/live-production-externalsecret-readiness.json" "$SECURITY_DIR/live-production-externalsecret-readiness-latest.json"
cp "$LOG_DIR/live-keycloak-profile-summary.json" "$SECURITY_DIR/live-keycloak-profile-summary-latest.json"
cp "$LOG_DIR/live-keycloak-profile.json" "$SECURITY_DIR/live-keycloak-profile-latest.json" 2>/dev/null || true
cp "$LOG_DIR/production-security-waivers.json" "$SECURITY_DIR/production-security-waivers-latest.json"
cp "$LOG_DIR/repo-local-kafka-plaintext-unwaived.txt" "$SECURITY_DIR/repo-local-kafka-plaintext-unwaived-latest.txt"
cp "$LOG_DIR/repo-static-secret-unwaived.txt" "$SECURITY_DIR/repo-static-secret-unwaived-latest.txt"
cp "$LOG_DIR/live-privileged-containers-unwaived.json" "$SECURITY_DIR/live-privileged-containers-unwaived-latest.json"
cp "$LOG_DIR/live-host-namespace-workloads-unwaived.json" "$SECURITY_DIR/live-host-namespace-workloads-unwaived-latest.json"

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
