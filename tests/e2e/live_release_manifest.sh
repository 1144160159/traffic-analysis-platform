#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-release-manifest}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-release-manifest}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
BASELINE_DIR="${BASELINE_DIR:-doc/02_acceptance/00-baseline}"

REPORT="$LOG_DIR/live-release-manifest-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-release-manifest-$RUN_ID-summary.json"
MANIFEST="$LOG_DIR/release-manifest.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$BASELINE_DIR"
: >"$REPORT"

FAILURES=0

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
  local phase="$1" name="$2" ok="$3" status="$4" detail="${5:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --argjson ok "$ok" \
    --arg status "$status" \
    --arg detail "$detail" \
    '{ts:$ts, phase:$phase, name:$name, ok:$ok, status:$status, detail:$detail}' >>"$REPORT"
  if [[ "$ok" != "true" ]]; then
    FAILURES=$((FAILURES + 1))
  fi
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 800 "$file" | tr '\n' ' '
  fi
}

kafka_secure_admin_config() {
  cat <<'EOF'
PROPS=/tmp/release-manifest-kafka-client.properties
cat > "$PROPS" <<CLIENT_EOF
security.protocol=SASL_SSL
sasl.mechanism=SCRAM-SHA-512
sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="${KAFKA_INTER_BROKER_USERNAME}" password="${KAFKA_INTER_BROKER_PASSWORD}";
ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12
ssl.truststore.type=PKCS12
ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}
CLIENT_EOF
EOF
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

if [[ -s "$LOG_DIR/commit-sha.txt" ]]; then
  json_log "repo" "git commit captured" true "ok" "$(cat "$LOG_DIR/commit-sha.txt")"
else
  json_log "repo" "git commit captured" false "missing" "git rev-parse produced no output"
fi

mapfile -d '' HASH_FILES < <(
  find \
    deployments/kubernetes \
    common/sql \
    common/kafka \
    proto/traffic/v1 \
    mlops/workflows \
    rules \
    web/ui/src \
    tests/e2e \
    -type f \
    ! -path '*/.archive/*' \
    ! -path '*/__pycache__/*' \
    -print0 | sort -z
)
if ((${#HASH_FILES[@]} > 0)); then
  sha256sum "${HASH_FILES[@]}" >"$LOG_DIR/file-hashes.tsv"
else
  : >"$LOG_DIR/file-hashes.tsv"
fi
jq -Rn '[inputs | capture("^(?<sha256>[0-9a-f]+)  (?<path>.*)$")]' <"$LOG_DIR/file-hashes.tsv" >"$LOG_DIR/file-hashes.json"
json_log "repo" "source file hashes captured" true "ok" "$(jq 'length' "$LOG_DIR/file-hashes.json") files"

grep -Eo '"[^"]+:[0-9]+:[0-9]+:[0-9]+:[^"]+:[^"]+"' common/kafka/create-topics.sh \
  | tr -d '"' \
  | awk -F: '{printf("{\"name\":\"%s\",\"partitions\":%s,\"retention_ms\":%s,\"retention_bytes\":%s,\"key\":\"%s\",\"message_type\":\"%s\"}\n",$1,$2,$3,$4,$5,$6)}' \
  | jq -s . >"$LOG_DIR/kafka-topics-repo.json"
json_log "repo" "repo kafka topic catalog captured" true "ok" "$(jq 'length' "$LOG_DIR/kafka-topics-repo.json") topics"

set +e
kctl apply --dry-run=client \
  -f deployments/kubernetes/applications \
  -f deployments/kubernetes/configmaps \
  -f deployments/kubernetes/infrastructure \
  -f deployments/kubernetes/init-jobs \
  -f deployments/kubernetes/flink \
  >"$LOG_DIR/k8s-dry-run.txt" 2>"$LOG_DIR/k8s-dry-run.err"
DRY_RUN_RC=$?
set -e
if [[ "$DRY_RUN_RC" -eq 0 ]]; then
  json_log "k8s" "core manifests dry-run" true "ok" "$(wc -l <"$LOG_DIR/k8s-dry-run.txt" | tr -d ' ') objects"
else
  json_log "k8s" "core manifests dry-run" false "rc=$DRY_RUN_RC" "$(trim_file "$LOG_DIR/k8s-dry-run.err")"
fi

kctl get deploy,sts,ds -A -o json >"$LOG_DIR/k8s-workloads.json"
jq '[
  .items[]
  | {
      namespace: .metadata.namespace,
      kind: .kind,
      name: .metadata.name,
      desired: (.spec.replicas // .status.desiredNumberScheduled // null),
      ready: (.status.readyReplicas // .status.numberReady // 0),
      images: ([.spec.template.spec.initContainers[]?, .spec.template.spec.containers[]?] | map({name, image}))
    }
]' "$LOG_DIR/k8s-workloads.json" >"$LOG_DIR/k8s-workloads-summary.json"
json_log "k8s" "workload images captured" true "ok" "$(jq 'length' "$LOG_DIR/k8s-workloads-summary.json") workloads"
jq '[.[] | select(.desired != null and .ready < .desired)]' "$LOG_DIR/k8s-workloads-summary.json" >"$LOG_DIR/k8s-unready-workloads.json"
UNREADY_WORKLOAD_COUNT="$(jq 'length' "$LOG_DIR/k8s-unready-workloads.json")"
if [[ "$UNREADY_WORKLOAD_COUNT" -eq 0 ]]; then
  json_log "k8s" "runtime workloads ready" true "ok" "all desired replicas ready"
else
  json_log "k8s" "runtime workloads ready" false "not_ready" "$UNREADY_WORKLOAD_COUNT workloads below desired readiness"
fi

kctl get pods -A -o json >"$LOG_DIR/k8s-pods.json"
jq '[
  .items[]
  | . as $pod
  | ($pod.status.initContainerStatuses // []), ($pod.status.containerStatuses // [])
  | .[]
  | {
      namespace: $pod.metadata.namespace,
      pod: $pod.metadata.name,
      container: .name,
      image: .image,
      image_id: .imageID,
      ready: (.ready // false),
      restart_count: (.restartCount // 0)
    }
]' "$LOG_DIR/k8s-pods.json" >"$LOG_DIR/k8s-pod-images-summary.json"
json_log "k8s" "pod image ids captured" true "ok" "$(jq 'length' "$LOG_DIR/k8s-pod-images-summary.json") containers"

set +e
kctl -n middleware exec kafka-0 -- bash -lc "set -euo pipefail
$(kafka_secure_admin_config)
export KAFKA_HEAP_OPTS=\"\${KAFKA_HEAP_OPTS:--Xms128m -Xmx512m}\"
/opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config \"\$PROPS\" --list
rm -f \"\$PROPS\"" \
  >"$LOG_DIR/kafka-topics-live.txt" 2>"$LOG_DIR/kafka-topics-live.err"
KAFKA_RC=$?
set -e
if [[ "$KAFKA_RC" -eq 0 ]]; then
  sort -o "$LOG_DIR/kafka-topics-live.txt" "$LOG_DIR/kafka-topics-live.txt"
  jq -Rn '[inputs | select(length > 0)]' <"$LOG_DIR/kafka-topics-live.txt" >"$LOG_DIR/kafka-topics-live.json"
  json_log "kafka" "live topic list captured" true "ok" "$(jq 'length' "$LOG_DIR/kafka-topics-live.json") topics"
else
  jq -n '[]' >"$LOG_DIR/kafka-topics-live.json"
  json_log "kafka" "live topic list captured" false "rc=$KAFKA_RC" "$(trim_file "$LOG_DIR/kafka-topics-live.err")"
fi

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
TOKEN="$(JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" python3 - <<'PY'
import base64
import hashlib
import hmac
import json
import os
import time
import uuid

def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")

now = int(time.time())
claims = {
    "iss": "traffic-auth-service",
    "sub": str(uuid.uuid4()),
    "jti": str(uuid.uuid4()),
    "user_id": str(uuid.uuid4()),
    "tenant_id": os.environ["TENANT"],
    "username": "codex-release-manifest",
    "email": "codex-release-manifest@local",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "model:read", "rule:read", "token:read"],
    "token_type": "access",
    "session_id": "codex-release-manifest-" + os.environ["RUN_ID"],
    "iat": now,
    "exp": now + 900,
}
header = {"alg": "HS256", "typ": "JWT"}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode("utf-8")).encode("ascii"),
    b64url(json.dumps(claims, separators=(",", ":")).encode("utf-8")).encode("ascii"),
])
signature = hmac.new(os.environ["JWT_SECRET"].encode("utf-8"), signing_input, hashlib.sha256).digest()
print(signing_input.decode("ascii") + "." + b64url(signature))
PY
)"

curl_json() {
  local name="$1" path="$2" outfile="$3"
  local code err_file
  err_file="$(mktemp)"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 -o "$outfile" -w '%{http_code}' \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT" \
    "$APISIX$path" 2>"$err_file")"
  local rc=$?
  set -e
  if [[ "$rc" -eq 0 && "$code" == "200" ]]; then
    json_log "api" "$name" true "$code" "$path"
  else
    json_log "api" "$name" false "${code:-rc=$rc}" "$(trim_file "$err_file") $(trim_file "$outfile")"
  fi
  rm -f "$err_file"
}

summarize_api_items() {
  local kind="$1" infile="$2" outfile="$3"
  jq --arg kind "$kind" '
    def items:
      if (.data | type) == "array" then .data
      elif (.models | type) == "array" then .models
      elif (.rules | type) == "array" then .rules
      elif (.deployments | type) == "array" then .deployments
      elif type == "array" then .
      else []
      end;
    {
      kind: $kind,
      success: (.success // null),
      count: (items | length),
      items: (items | map({
        id: (.model_id // .rule_id // .deployment_id // .id // ""),
        name: (.name // .model_name // .rule_name // ""),
        type: (.model_type // .rule_type // .type // .category // ""),
        version: (.version // .rule_version // .model_version // .metadata.current_version // .metadata.version // .metadata.online_version // ""),
        status: (.status // .metadata.status // ""),
        updated_at: (.updated_at // .created_at // "")
      }))
    }
  ' "$infile" >"$outfile"
}

curl_json "model catalog captured" "/api/v1/models?limit=100" "$LOG_DIR/models-api.json"
summarize_api_items "models" "$LOG_DIR/models-api.json" "$LOG_DIR/models-summary.json"
curl_json "rule catalog captured" "/api/v1/rules?limit=100" "$LOG_DIR/rules-api.json"
summarize_api_items "rules" "$LOG_DIR/rules-api.json" "$LOG_DIR/rules-summary.json"
curl_json "deployment catalog captured" "/api/v1/deployments?limit=100" "$LOG_DIR/deployments-api.json"
summarize_api_items "deployments" "$LOG_DIR/deployments-api.json" "$LOG_DIR/deployments-summary.json"

: >"$LOG_DIR/evidence-runs.ndjson"
for dir in doc/02_acceptance/runs/*; do
  [[ -d "$dir" ]] || continue
  run_name="$(basename "$dir")"
  summary_count="$(find "$dir" -maxdepth 1 -type f -name '*summary.json' | wc -l | tr -d ' ')"
  has_local=false
  [[ -f "$dir/local-report.md" ]] && has_local=true
  jq -nc \
    --arg run_id "$run_name" \
    --arg path "$dir" \
    --argjson summary_count "$summary_count" \
    --argjson has_local_report "$has_local" \
    '{run_id:$run_id, path:$path, summary_count:$summary_count, has_local_report:$has_local_report}' >>"$LOG_DIR/evidence-runs.ndjson"
done
jq -s . "$LOG_DIR/evidence-runs.ndjson" >"$LOG_DIR/evidence-runs.json"
json_log "evidence" "acceptance run index captured" true "ok" "$(jq 'length' "$LOG_DIR/evidence-runs.json") runs"

GENERATED_AT="$(date -Iseconds)"
COMMIT_SHA="$(cat "$LOG_DIR/commit-sha.txt")"
BRANCH="$(cat "$LOG_DIR/git-branch.txt")"
DIRTY_COUNT="$(wc -l <"$LOG_DIR/git-status.txt" | tr -d ' ')"

jq -n \
  --arg run_id "$RUN_ID" \
  --arg generated_at "$GENERATED_AT" \
  --arg acceptance_type "regression" \
  --arg commit "$COMMIT_SHA" \
  --arg branch "$BRANCH" \
  --argjson dirty_count "$DIRTY_COUNT" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg k8s_dry_run_rc "$DRY_RUN_RC" \
  --slurpfile file_hashes "$LOG_DIR/file-hashes.json" \
  --slurpfile workloads "$LOG_DIR/k8s-workloads-summary.json" \
  --slurpfile pod_images "$LOG_DIR/k8s-pod-images-summary.json" \
  --slurpfile repo_topics "$LOG_DIR/kafka-topics-repo.json" \
  --slurpfile live_topics "$LOG_DIR/kafka-topics-live.json" \
  --slurpfile models "$LOG_DIR/models-summary.json" \
  --slurpfile rules "$LOG_DIR/rules-summary.json" \
  --slurpfile deployments "$LOG_DIR/deployments-summary.json" \
  --slurpfile evidence_runs "$LOG_DIR/evidence-runs.json" \
  '{
    run_id: $run_id,
    generated_at: $generated_at,
    acceptance_type: $acceptance_type,
    scope_note: "Release/baseline regression evidence only; not a 10x100Gbps, 512Mpps, third-party, security-production, or HA acceptance pass.",
    git: {
      commit: $commit,
      branch: $branch,
      dirty_count: $dirty_count,
      status_file: "git-status.txt",
      diff_stat_file: "git-diff-stat.txt"
    },
    live_targets: {
      apisix: $apisix,
      tenant: $tenant
    },
    kubernetes: {
      core_manifest_dry_run_rc: ($k8s_dry_run_rc | tonumber),
      workloads: $workloads[0],
      pod_images: $pod_images[0]
    },
    repo_sources: {
      file_hashes: $file_hashes[0],
      manifest_hash_count: ($file_hashes[0] | length)
    },
    kafka: {
      repo_topics: $repo_topics[0],
      live_topics: $live_topics[0],
      missing_live_topics_from_repo: (($repo_topics[0] | map(.name)) - $live_topics[0])
    },
    api_catalogs: {
      models: $models[0],
      rules: $rules[0],
      deployments: $deployments[0]
    },
    evidence_runs: $evidence_runs[0],
    required_followups: [
      "Kafka TLS/SASL/ACL and ExternalSecret production profile",
      "NetworkPolicy default-deny and business allow-list negative tests",
      "HA/RTO/RPO chaos drills",
      "10 x 100Gbps and 512Mpps performance acceptance",
      "95% detection and 5% false-positive third-party blind-test package"
    ]
  }' >"$MANIFEST"
json_log "release" "release manifest generated" true "ok" "$MANIFEST"

cp "$MANIFEST" "$BASELINE_DIR/release-manifest-$RUN_ID.json"
cp "$MANIFEST" "$BASELINE_DIR/release-manifest-latest.json"
cp "$LOG_DIR/commit-sha.txt" "$BASELINE_DIR/commit-sha.txt"
jq -r '.kubernetes.workloads[] | [.namespace, .kind, .name, (.images | map(.name + "=" + .image) | join(","))] | @tsv' "$MANIFEST" >"$BASELINE_DIR/images.txt"
jq -r '.repo_sources.file_hashes[] | [.sha256, .path] | @tsv' "$MANIFEST" >"$BASELINE_DIR/manifests.txt"
jq -r '.repo_sources.file_hashes[] | select(.path | startswith("common/sql/")) | [.sha256, .path] | @tsv' "$MANIFEST" >"$BASELINE_DIR/database-schema.txt"
jq -r '.kafka.repo_topics[] | [.name, .partitions, .retention_ms, .retention_bytes, .key, .message_type] | @tsv' "$MANIFEST" >"$BASELINE_DIR/kafka-topics.txt"

total="$(wc -l <"$REPORT" | tr -d ' ')"
failed="$(jq -s '[.[] | select(.ok == false)] | length' "$REPORT")"
passed="$((total - failed))"
jq -s \
  --arg run_id "$RUN_ID" \
  --arg report "$REPORT" \
  --arg manifest "$MANIFEST" \
  --arg baseline_dir "$BASELINE_DIR" \
  --argjson total "$total" \
  --argjson passed "$passed" \
  --argjson failed "$failed" \
  '{run_id:$run_id, report:$report, manifest:$manifest, baseline_dir:$baseline_dir, total:$total, passed:$passed, failed:$failed, checks:.}' \
  "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# Release Manifest Regression Evidence

Run: \`$RUN_ID\`

This run freezes the current repo and live Kubernetes baseline as regression evidence. It does not claim performance, detection-quality, production-security, or HA acceptance.

## Evidence

| Check | Result | Artifact |
|---|---:|---|
| Git commit/status captured | pass | \`commit-sha.txt\`, \`git-status.txt\`, \`git-diff-stat.txt\` |
| Source hashes captured | pass | \`file-hashes.json\` |
| K8s core manifests dry-run | $([[ "$DRY_RUN_RC" -eq 0 ]] && echo pass || echo fail) | \`k8s-dry-run.txt\`, \`k8s-dry-run.err\` |
| Live workload images and pod image IDs | pass | \`k8s-workloads-summary.json\`, \`k8s-pod-images-summary.json\` |
| Repo/live Kafka topic catalog | $([[ "$KAFKA_RC" -eq 0 ]] && echo pass || echo fail) | \`kafka-topics-repo.json\`, \`kafka-topics-live.json\` |
| Model/rule/deployment API catalog | see summary | \`models-summary.json\`, \`rules-summary.json\`, \`deployments-summary.json\` |
| Release manifest | pass | \`release-manifest.json\` |
| Baseline stable copy | pass | \`$BASELINE_DIR/release-manifest-latest.json\` |

## Result

Summary: \`$SUMMARY\`

Checks: $passed/$total passed, $failed failed.

## Follow-up Gates

The manifest keeps these gates open: Kafka TLS/SASL/ACL, ExternalSecret, NetworkPolicy, HA/RTO/RPO, 10 x 100Gbps / 512Mpps, and third-party detection-quality evidence.
EOF

cat "$SUMMARY"
if [[ "$failed" -ne 0 ]]; then
  exit 1
fi
