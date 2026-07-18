#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-external-secret-production-reconciliation}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-external-secret-production-reconciliation}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"
STORE_FILE="${STORE_FILE:-deployments/kubernetes/security/external-secrets-prod-store.yaml}"
TEMPLATE_FILE="${TEMPLATE_FILE:-deployments/kubernetes/security/external-secrets-template.yaml}"
SOURCE_NAMESPACE="${SOURCE_NAMESPACE:-external-secrets-source}"

REPORT="$LOG_DIR/live-external-secret-production-reconciliation-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-external-secret-production-reconciliation-$RUN_ID-summary.json"
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

copy_secret_to_source() {
  local src_ns="$1" src_name="$2" dst_name="$3" out="$4" err="$5"
  set +e
  kctl -n "$src_ns" get secret "$src_name" -o json \
    | jq --arg ns "$SOURCE_NAMESPACE" --arg name "$dst_name" '{
        apiVersion:"v1",
        kind:"Secret",
        metadata:{
          name:$name,
          namespace:$ns,
          labels:{
            "app.kubernetes.io/name":$name,
            "traffic-platform.io/security-profile":"external-secrets-production"
          }
        },
        type:(.type // "Opaque"),
        data:(.data // {})
      }' \
    | kctl apply -f - >"$out" 2>"$err"
  local rc=$?
  set -e
  return "$rc"
}

need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ -s "$STORE_FILE" && -s "$TEMPLATE_FILE" ]]; then
  json_log "repo" "production ClusterSecretStore and ExternalSecret manifests present" "info" true "ok" "$STORE_FILE $TEMPLATE_FILE" "$STORE_FILE"
else
  json_log "repo" "production ClusterSecretStore and ExternalSecret manifests present" "blocker" false "missing" "$STORE_FILE or $TEMPLATE_FILE missing" ""
fi

set +e
kctl apply --dry-run=client -f "$STORE_FILE" >"$LOG_DIR/prod-store-client-dry-run.out" 2>"$LOG_DIR/prod-store-client-dry-run.err"
STORE_DRY_RUN_RC=$?
kctl apply --dry-run=server -f "$TEMPLATE_FILE" >"$LOG_DIR/prod-externalsecret-server-dry-run.out" 2>"$LOG_DIR/prod-externalsecret-server-dry-run.err"
TEMPLATE_DRY_RUN_RC=$?
set -e
if [[ "$STORE_DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "production ClusterSecretStore manifest client dry-run" "info" true "ok" "$STORE_FILE" "prod-store-client-dry-run.out"
else
  json_log "repo" "production ClusterSecretStore manifest client dry-run" "blocker" false "rc=$STORE_DRY_RUN_RC" "$(head -c 800 "$LOG_DIR/prod-store-client-dry-run.err" | tr '\n' ' ')" "prod-store-client-dry-run.err"
fi
if [[ "$TEMPLATE_DRY_RUN_RC" -eq 0 ]]; then
  json_log "repo" "production ExternalSecret manifest server dry-run" "info" true "ok" "$TEMPLATE_FILE" "prod-externalsecret-server-dry-run.out"
else
  json_log "repo" "production ExternalSecret manifest server dry-run" "blocker" false "rc=$TEMPLATE_DRY_RUN_RC" "$(head -c 800 "$LOG_DIR/prod-externalsecret-server-dry-run.err" | tr '\n' ' ')" "prod-externalsecret-server-dry-run.err"
fi

python3 - "$TEMPLATE_FILE" >"$LOG_DIR/preflight-secret-key-readiness.json" <<'PY'
import json
import subprocess
import sys

import yaml

template = sys.argv[1]

def kubectl_json(args):
    out = subprocess.check_output(
        ["env","-u","HTTP_PROXY","-u","HTTPS_PROXY","-u","ALL_PROXY","-u","http_proxy","-u","https_proxy","-u","all_proxy","kubectl"] + args,
        text=True,
    )
    return json.loads(out)

with open(template, encoding="utf-8") as fh:
    docs = [d for d in yaml.safe_load_all(fh) if isinstance(d, dict) and d.get("kind") == "ExternalSecret"]
secrets = kubectl_json(["get", "secret", "-A", "-o", "json"])["items"]
by_ns_name = {(s["metadata"]["namespace"], s["metadata"]["name"]): s for s in secrets}
source_by_remote_key = {
    "traffic-platform-prod-credentials": by_ns_name.get(("databases", "traffic-credentials")),
    "traffic-platform-prod-kafka-broker-tls": by_ns_name.get(("middleware", "kafka-broker-tls")),
    "traffic-platform-prod-kafka-client-tls": by_ns_name.get(("middleware", "kafka-client-tls")),
    "traffic-platform-prod-keycloak-tls": by_ns_name.get(("iam", "keycloak-tls")),
}
items = []
for doc in docs:
    ns = doc["metadata"]["namespace"]
    name = doc["metadata"]["name"]
    target = (doc.get("spec") or {}).get("target", {}).get("name") or name
    live = by_ns_name.get((ns, target))
    for entry in (doc.get("spec") or {}).get("data", []):
        remote = entry.get("remoteRef") or {}
        source_key = remote.get("key")
        prop = remote.get("property") or entry.get("secretKey")
        source = source_by_remote_key.get(source_key)
        source_present = bool((source or {}).get("data", {}).get(prop))
        target_present = bool((live or {}).get("data", {}).get(entry["secretKey"]))
        match = source_present and target_present and (source["data"][prop] == live["data"][entry["secretKey"]])
        items.append({
            "namespace": ns,
            "externalsecret": name,
            "target": target,
            "target_key": entry["secretKey"],
            "remote_key": source_key,
            "remote_property": prop,
            "source_present": source_present,
            "target_present": target_present,
            "source_target_match": match,
        })
summary = {
    "expected_externalsecrets": len(docs),
    "expected_keys": len(items),
    "source_missing_count": len([i for i in items if not i["source_present"]]),
    "target_missing_count": len([i for i in items if not i["target_present"]]),
    "mismatch_count": len([i for i in items if not i["source_target_match"]]),
    "items": items,
}
json.dump(summary, sys.stdout, ensure_ascii=False, indent=2)
sys.stdout.write("\n")
PY

EXPECTED_EXTERNAL_SECRET_COUNT="$(jq '.expected_externalsecrets' "$LOG_DIR/preflight-secret-key-readiness.json")"
EXPECTED_KEY_COUNT="$(jq '.expected_keys' "$LOG_DIR/preflight-secret-key-readiness.json")"
SOURCE_MISSING_COUNT="$(jq '.source_missing_count' "$LOG_DIR/preflight-secret-key-readiness.json")"
TARGET_MISSING_COUNT="$(jq '.target_missing_count' "$LOG_DIR/preflight-secret-key-readiness.json")"
MISMATCH_COUNT="$(jq '.mismatch_count' "$LOG_DIR/preflight-secret-key-readiness.json")"
if [[ "$SOURCE_MISSING_COUNT" -eq 0 && "$TARGET_MISSING_COUNT" -eq 0 && "$MISMATCH_COUNT" -eq 0 ]]; then
  json_log "live" "current live Secret keys match planned ExternalSecret sources" "info" true "ok" "$EXPECTED_KEY_COUNT keys across $EXPECTED_EXTERNAL_SECRET_COUNT ExternalSecrets" "preflight-secret-key-readiness.json"
else
  json_log "live" "current live Secret keys match planned ExternalSecret sources" "blocker" false "mismatch" "source_missing=$SOURCE_MISSING_COUNT target_missing=$TARGET_MISSING_COUNT mismatch=$MISMATCH_COUNT" "preflight-secret-key-readiness.json"
fi

set +e
kctl apply -f "$STORE_FILE" >"$LOG_DIR/prod-store-apply.out" 2>"$LOG_DIR/prod-store-apply.err"
STORE_APPLY_RC=$?
set -e
if [[ "$STORE_APPLY_RC" -eq 0 ]]; then
  json_log "live" "production ClusterSecretStore and RBAC resources applied" "info" true "ok" "$STORE_FILE" "prod-store-apply.out"
else
  json_log "live" "production ClusterSecretStore and RBAC resources applied" "blocker" false "rc=$STORE_APPLY_RC" "$(head -c 800 "$LOG_DIR/prod-store-apply.err" | tr '\n' ' ')" "prod-store-apply.err"
fi

copy_secret_to_source databases traffic-credentials traffic-platform-prod-credentials "$LOG_DIR/source-credentials.apply.out" "$LOG_DIR/source-credentials.apply.err"
SOURCE_CREDENTIALS_RC=$?
copy_secret_to_source middleware kafka-broker-tls traffic-platform-prod-kafka-broker-tls "$LOG_DIR/source-kafka-broker-tls.apply.out" "$LOG_DIR/source-kafka-broker-tls.apply.err"
SOURCE_KAFKA_BROKER_RC=$?
copy_secret_to_source middleware kafka-client-tls traffic-platform-prod-kafka-client-tls "$LOG_DIR/source-kafka-client-tls.apply.out" "$LOG_DIR/source-kafka-client-tls.apply.err"
SOURCE_KAFKA_CLIENT_RC=$?
copy_secret_to_source iam keycloak-tls traffic-platform-prod-keycloak-tls "$LOG_DIR/source-keycloak-tls.apply.out" "$LOG_DIR/source-keycloak-tls.apply.err"
SOURCE_KEYCLOAK_RC=$?
SOURCE_SECRET_BLOCKERS=0
for rc in "$SOURCE_CREDENTIALS_RC" "$SOURCE_KAFKA_BROKER_RC" "$SOURCE_KAFKA_CLIENT_RC" "$SOURCE_KEYCLOAK_RC"; do
  if [[ "$rc" -ne 0 ]]; then
    SOURCE_SECRET_BLOCKERS=$((SOURCE_SECRET_BLOCKERS + 1))
  fi
done
if [[ "$SOURCE_SECRET_BLOCKERS" -eq 0 ]]; then
  json_log "live" "production source Secrets seeded without writing secret values to artifacts" "info" true "ok" "4 source Secrets in $SOURCE_NAMESPACE" "source-credentials.apply.out"
else
  json_log "live" "production source Secrets seeded without writing secret values to artifacts" "blocker" false "rcs=$SOURCE_CREDENTIALS_RC/$SOURCE_KAFKA_BROKER_RC/$SOURCE_KAFKA_CLIENT_RC/$SOURCE_KEYCLOAK_RC" "one or more source Secret copies failed" "source-credentials.apply.err"
fi

set +e
kctl apply -f "$TEMPLATE_FILE" >"$LOG_DIR/prod-externalsecret-apply.out" 2>"$LOG_DIR/prod-externalsecret-apply.err"
TEMPLATE_APPLY_RC=$?
set -e
if [[ "$TEMPLATE_APPLY_RC" -eq 0 ]]; then
  json_log "live" "production ExternalSecret resources applied" "info" true "ok" "$TEMPLATE_FILE" "prod-externalsecret-apply.out"
else
  json_log "live" "production ExternalSecret resources applied" "blocker" false "rc=$TEMPLATE_APPLY_RC" "$(head -c 800 "$LOG_DIR/prod-externalsecret-apply.err" | tr '\n' ' ')" "prod-externalsecret-apply.err"
fi

SECRETSTORE_READY_COUNT=0
EXTERNAL_SECRET_READY_COUNT=0
for _ in $(seq 1 90); do
  set +e
  kctl get clustersecretstore traffic-platform-secret-store -o json >"$LOG_DIR/live-secretstores.json" 2>"$LOG_DIR/live-secretstores.err"
  SECRETSTORE_GET_RC=$?
  kctl get externalsecrets.external-secrets.io -A -o json >"$LOG_DIR/live-externalsecrets.json" 2>"$LOG_DIR/live-externalsecrets.err"
  EXTERNALSECRET_GET_RC=$?
  set -e
  if [[ "$SECRETSTORE_GET_RC" -eq 0 ]]; then
    jq '[{
      name:.metadata.name,
      ready:(([.status.conditions[]? | select(.type == "Ready" and .status == "True")] | length) > 0),
      conditions:[.status.conditions[]? | {type:(.type // ""), status:(.status // ""), reason:(.reason // ""), message:(.message // "")}]
    }]' "$LOG_DIR/live-secretstores.json" >"$LOG_DIR/live-production-secretstores.json"
    SECRETSTORE_READY_COUNT="$(jq '[.[] | select(.ready == true)] | length' "$LOG_DIR/live-production-secretstores.json")"
  fi
  if [[ "$EXTERNALSECRET_GET_RC" -eq 0 ]]; then
    python3 - "$TEMPLATE_FILE" "$LOG_DIR/live-externalsecrets.json" >"$LOG_DIR/live-production-externalsecret-readiness.json" <<'PY'
import json
import sys
import yaml

template, live_path = sys.argv[1], sys.argv[2]
with open(template, encoding="utf-8") as fh:
    expected = [
        d for d in yaml.safe_load_all(fh)
        if isinstance(d, dict) and d.get("kind") == "ExternalSecret"
    ]
with open(live_path, encoding="utf-8") as fh:
    live_items = json.load(fh).get("items", [])
live = {(i["metadata"]["namespace"], i["metadata"]["name"]): i for i in live_items}
rows = []
for item in expected:
    ns = item["metadata"]["namespace"]
    name = item["metadata"]["name"]
    match = live.get((ns, name))
    conditions = [
        {
            "type": c.get("type", ""),
            "status": c.get("status", ""),
            "reason": c.get("reason", ""),
            "message": c.get("message", ""),
        }
        for c in ((match or {}).get("status") or {}).get("conditions", [])
    ]
    ready = any(c["type"] in ("Ready", "Synced") and c["status"] == "True" for c in conditions)
    rows.append({
        "namespace": ns,
        "name": name,
        "target_name": (item.get("spec") or {}).get("target", {}).get("name") or name,
        "live": match is not None,
        "ready": ready,
        "conditions": conditions,
    })
json.dump(rows, sys.stdout, ensure_ascii=False, indent=2)
sys.stdout.write("\n")
PY
    EXTERNAL_SECRET_READY_COUNT="$(jq '[.[] | select(.ready == true)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json")"
  fi
  if [[ "$SECRETSTORE_READY_COUNT" -ge 1 && "$EXTERNAL_SECRET_READY_COUNT" -ge "$EXPECTED_EXTERNAL_SECRET_COUNT" ]]; then
    break
  fi
  sleep 2
done

SECRETSTORE_COUNT="$(jq 'length' "$LOG_DIR/live-production-secretstores.json" 2>/dev/null || echo 0)"
EXTERNAL_SECRET_LIVE_COUNT="$(jq '[.[] | select(.live == true)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json" 2>/dev/null || echo 0)"
EXTERNAL_SECRET_MISSING_COUNT="$(jq '[.[] | select(.live == false)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json" 2>/dev/null || echo "$EXPECTED_EXTERNAL_SECRET_COUNT")"
EXTERNAL_SECRET_NOT_READY_COUNT="$(jq '[.[] | select(.live == true and .ready == false)] | length' "$LOG_DIR/live-production-externalsecret-readiness.json" 2>/dev/null || echo "$EXPECTED_EXTERNAL_SECRET_COUNT")"

if [[ "$SECRETSTORE_READY_COUNT" -ge 1 ]]; then
  json_log "live" "production ClusterSecretStore is Ready" "info" true "ok" "$SECRETSTORE_READY_COUNT/$SECRETSTORE_COUNT Ready" "live-production-secretstores.json"
else
  json_log "live" "production ClusterSecretStore is Ready" "blocker" false "not_ready" "$SECRETSTORE_READY_COUNT/$SECRETSTORE_COUNT Ready" "live-production-secretstores.json"
fi
if [[ "$EXTERNAL_SECRET_READY_COUNT" -eq "$EXPECTED_EXTERNAL_SECRET_COUNT" ]]; then
  json_log "live" "production ExternalSecrets are live and Ready" "info" true "ok" "$EXTERNAL_SECRET_READY_COUNT/$EXPECTED_EXTERNAL_SECRET_COUNT Ready" "live-production-externalsecret-readiness.json"
else
  json_log "live" "production ExternalSecrets are live and Ready" "blocker" false "not_ready" "live=$EXTERNAL_SECRET_LIVE_COUNT ready=$EXTERNAL_SECRET_READY_COUNT missing=$EXTERNAL_SECRET_MISSING_COUNT not_ready=$EXTERNAL_SECRET_NOT_READY_COUNT expected=$EXPECTED_EXTERNAL_SECRET_COUNT" "live-production-externalsecret-readiness.json"
fi

python3 - "$TEMPLATE_FILE" "$SOURCE_NAMESPACE" >"$LOG_DIR/post-reconcile-secret-key-readiness.json" <<'PY'
import json
import subprocess
import sys

import yaml

template, source_namespace = sys.argv[1], sys.argv[2]

def kubectl_json(args):
    out = subprocess.check_output(
        ["env","-u","HTTP_PROXY","-u","HTTPS_PROXY","-u","ALL_PROXY","-u","http_proxy","-u","https_proxy","-u","all_proxy","kubectl"] + args,
        text=True,
    )
    return json.loads(out)

with open(template, encoding="utf-8") as fh:
    docs = [d for d in yaml.safe_load_all(fh) if isinstance(d, dict) and d.get("kind") == "ExternalSecret"]
secrets = kubectl_json(["get", "secret", "-A", "-o", "json"])["items"]
by_ns_name = {(s["metadata"]["namespace"], s["metadata"]["name"]): s for s in secrets}
items = []
for doc in docs:
    ns = doc["metadata"]["namespace"]
    name = doc["metadata"]["name"]
    target = (doc.get("spec") or {}).get("target", {}).get("name") or name
    live = by_ns_name.get((ns, target))
    for entry in (doc.get("spec") or {}).get("data", []):
        remote = entry.get("remoteRef") or {}
        source_key = remote.get("key")
        prop = remote.get("property") or entry.get("secretKey")
        source = by_ns_name.get((source_namespace, source_key))
        source_present = bool((source or {}).get("data", {}).get(prop))
        target_present = bool((live or {}).get("data", {}).get(entry["secretKey"]))
        match = source_present and target_present and (source["data"][prop] == live["data"][entry["secretKey"]])
        items.append({
            "namespace": ns,
            "externalsecret": name,
            "target": target,
            "target_key": entry["secretKey"],
            "remote_key": source_key,
            "remote_property": prop,
            "source_present": source_present,
            "target_present": target_present,
            "source_target_match": match,
        })
summary = {
    "expected_externalsecrets": len(docs),
    "expected_keys": len(items),
    "source_missing_count": len([i for i in items if not i["source_present"]]),
    "target_missing_count": len([i for i in items if not i["target_present"]]),
    "mismatch_count": len([i for i in items if not i["source_target_match"]]),
    "items": items,
}
json.dump(summary, sys.stdout, ensure_ascii=False, indent=2)
sys.stdout.write("\n")
PY
POST_SOURCE_MISSING_COUNT="$(jq '.source_missing_count' "$LOG_DIR/post-reconcile-secret-key-readiness.json")"
POST_TARGET_MISSING_COUNT="$(jq '.target_missing_count' "$LOG_DIR/post-reconcile-secret-key-readiness.json")"
POST_MISMATCH_COUNT="$(jq '.mismatch_count' "$LOG_DIR/post-reconcile-secret-key-readiness.json")"
if [[ "$POST_SOURCE_MISSING_COUNT" -eq 0 && "$POST_TARGET_MISSING_COUNT" -eq 0 && "$POST_MISMATCH_COUNT" -eq 0 ]]; then
  json_log "live" "post-reconcile production Secret data matches source keys" "info" true "ok" "$EXPECTED_KEY_COUNT keys match without exposing values" "post-reconcile-secret-key-readiness.json"
else
  json_log "live" "post-reconcile production Secret data matches source keys" "blocker" false "mismatch" "source_missing=$POST_SOURCE_MISSING_COUNT target_missing=$POST_TARGET_MISSING_COUNT mismatch=$POST_MISMATCH_COUNT" "post-reconcile-secret-key-readiness.json"
fi

kctl get secret -n "$SOURCE_NAMESPACE" -o json >"$LOG_DIR/source-secret-inventory.raw.json"
jq '[.items[] | {namespace:.metadata.namespace,name:.metadata.name,type:.type,keys:(.data | keys | sort)}]' "$LOG_DIR/source-secret-inventory.raw.json" >"$LOG_DIR/source-secret-inventory.json"
rm -f "$LOG_DIR/source-secret-inventory.raw.json"

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
  --arg store_file "$STORE_FILE" \
  --arg template_file "$TEMPLATE_FILE" \
  --arg source_namespace "$SOURCE_NAMESPACE" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson expected_externalsecrets "$EXPECTED_EXTERNAL_SECRET_COUNT" \
  --argjson expected_keys "$EXPECTED_KEY_COUNT" \
  --argjson source_secret_blockers "$SOURCE_SECRET_BLOCKERS" \
  --argjson secretstore_count "$SECRETSTORE_COUNT" \
  --argjson secretstore_ready_count "$SECRETSTORE_READY_COUNT" \
  --argjson externalsecret_live_count "$EXTERNAL_SECRET_LIVE_COUNT" \
  --argjson externalsecret_ready_count "$EXTERNAL_SECRET_READY_COUNT" \
  --argjson externalsecret_missing_count "$EXTERNAL_SECRET_MISSING_COUNT" \
  --argjson externalsecret_not_ready_count "$EXTERNAL_SECRET_NOT_READY_COUNT" \
  --argjson post_source_missing_count "$POST_SOURCE_MISSING_COUNT" \
  --argjson post_target_missing_count "$POST_TARGET_MISSING_COUNT" \
  --argjson post_mismatch_count "$POST_MISMATCH_COUNT" \
  --slurpfile source_inventory "$LOG_DIR/source-secret-inventory.json" \
  --slurpfile preflight "$LOG_DIR/preflight-secret-key-readiness.json" \
  --slurpfile post_reconcile "$LOG_DIR/post-reconcile-secret-key-readiness.json" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    store_file:$store_file,
    template_file:$template_file,
    source_namespace:$source_namespace,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    expected_externalsecrets:$expected_externalsecrets,
    expected_keys:$expected_keys,
    source_secret_blockers:$source_secret_blockers,
    secretstore_count:$secretstore_count,
    secretstore_ready_count:$secretstore_ready_count,
    externalsecret_live_count:$externalsecret_live_count,
    externalsecret_ready_count:$externalsecret_ready_count,
    externalsecret_missing_count:$externalsecret_missing_count,
    externalsecret_not_ready_count:$externalsecret_not_ready_count,
    post_source_missing_count:$post_source_missing_count,
    post_target_missing_count:$post_target_missing_count,
    post_mismatch_count:$post_mismatch_count,
    source_secret_inventory:($source_inventory[0] // []),
    preflight_secret_key_readiness:($preflight[0] // {}),
    post_reconcile_secret_key_readiness:($post_reconcile[0] // {}),
    checks:.
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# ExternalSecret Production Reconciliation

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This live run seeds a Kubernetes-provider source namespace from existing live
Secrets, applies the production ClusterSecretStore and ExternalSecrets, waits for
operator-backed reconciliation, and verifies key-level source/target equality
without writing secret values to artifacts.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| Expected ExternalSecrets | $EXPECTED_EXTERNAL_SECRET_COUNT |
| Expected keys | $EXPECTED_KEY_COUNT |
| Source Secret copy blockers | $SOURCE_SECRET_BLOCKERS |
| Production ClusterSecretStore ready | $SECRETSTORE_READY_COUNT / $SECRETSTORE_COUNT |
| Production ExternalSecrets live | $EXTERNAL_SECRET_LIVE_COUNT / $EXPECTED_EXTERNAL_SECRET_COUNT |
| Production ExternalSecrets ready | $EXTERNAL_SECRET_READY_COUNT / $EXPECTED_EXTERNAL_SECRET_COUNT |
| Production ExternalSecrets missing | $EXTERNAL_SECRET_MISSING_COUNT |
| Production ExternalSecrets not ready | $EXTERNAL_SECRET_NOT_READY_COUNT |
| Post-reconcile source missing keys | $POST_SOURCE_MISSING_COUNT |
| Post-reconcile target missing keys | $POST_TARGET_MISSING_COUNT |
| Post-reconcile mismatched keys | $POST_MISMATCH_COUNT |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`preflight-secret-key-readiness.json\`
- \`live-production-secretstores.json\`
- \`live-production-externalsecret-readiness.json\`
- \`post-reconcile-secret-key-readiness.json\`
- \`source-secret-inventory.json\`
EOF

cp "$SUMMARY" "$SECURITY_DIR/external-secret-production-reconciliation-latest.json"
cp "$LOCAL_REPORT" "$SECURITY_DIR/external-secret-production-reconciliation-latest.md"
cp "$LOG_DIR/live-production-secretstores.json" "$SECURITY_DIR/live-production-secretstores-latest.json"
cp "$LOG_DIR/live-production-externalsecret-readiness.json" "$SECURITY_DIR/live-production-externalsecret-readiness-latest.json"
cp "$LOG_DIR/preflight-secret-key-readiness.json" "$SECURITY_DIR/external-secret-production-preflight-key-readiness-latest.json"
cp "$LOG_DIR/post-reconcile-secret-key-readiness.json" "$SECURITY_DIR/external-secret-production-post-reconcile-key-readiness-latest.json"
cp "$LOG_DIR/source-secret-inventory.json" "$SECURITY_DIR/external-secret-source-secret-inventory-latest.json"

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
