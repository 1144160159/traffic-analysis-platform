#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-kafka-sasl-ssl-rollout}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-kafka-sasl-ssl-rollout}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_DISRUPTIVE_KAFKA_ROLLOUT="${ALLOW_DISRUPTIVE_KAFKA_ROLLOUT:-false}"
ROLLBACK_ON_FAILURE="${ROLLBACK_ON_FAILURE:-true}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"
KAFKA_MANIFEST="${KAFKA_MANIFEST:-deployments/kubernetes/infrastructure/01-kafka.yaml}"
KAFKA_INIT_JOB_MANIFEST="${KAFKA_INIT_JOB_MANIFEST:-deployments/kubernetes/init-jobs/01-kafka-topics.yaml}"
PREFLIGHT_SCRIPT="${PREFLIGHT_SCRIPT:-tests/e2e/live_kafka_security_rollout_preflight.sh}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-600s}"

REPORT="$LOG_DIR/live-kafka-sasl-ssl-rollout-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-kafka-sasl-ssl-rollout-$RUN_ID-summary.json"
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

wait_pod_count() {
  local namespace="$1" selector="$2" expected="$3" timeout_seconds="$4"
  local deadline count
  deadline=$((SECONDS + timeout_seconds))
  while (( SECONDS < deadline )); do
    count="$(kctl -n "$namespace" get pods -l "$selector" --no-headers 2>/dev/null | sed '/^[[:space:]]*$/d' | wc -l | tr -d ' ')"
    if [[ "$count" == "$expected" ]]; then
      return 0
    fi
    sleep 5
  done
  return 1
}

kafka_secure_admin_config() {
  cat <<'EOF'
PROPS=/tmp/kafka-rollout-client.properties
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

secure_kafka_admin_cmd() {
  local out="$1" err="$2" admin_cmd="$3"
  set +e
  kctl -n middleware exec kafka-0 -- bash -lc "set -euo pipefail
$(kafka_secure_admin_config)
$admin_cmd --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config \"\$PROPS\"
rm -f \"\$PROPS\"" >"$out" 2>"$err"
  local rc=$?
  set -e
  return "$rc"
}

rollback_kafka() {
  json_log "rollback" "Kafka StatefulSet rollback requested" "warn" false "started" "ROLLBACK_ON_FAILURE=$ROLLBACK_ON_FAILURE" "rollback.txt"
  {
    echo "Rolling back kafka StatefulSet at $(date -Iseconds)"
    kctl -n middleware rollout undo statefulset/kafka || true
    kctl -n middleware scale statefulset/kafka --replicas=3 || true
    kctl -n middleware rollout status statefulset/kafka --timeout="$WAIT_TIMEOUT" || true
    kctl -n middleware get statefulset kafka -o json || true
    kctl -n middleware get pods -l app=kafka -o wide || true
  } >"$LOG_DIR/rollback.txt" 2>"$LOG_DIR/rollback.err"
}

finish() {
  local result="$1"
  local total passed blockers warnings pre_prereq post_blockers
  total="$(wc -l <"$REPORT" | tr -d ' ')"
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed == false and .severity == "warn")] | length' "$REPORT")"
  pre_prereq="$(jq -r '.prereq_blockers // null' "$LOG_DIR/preflight/live-kafka-security-rollout-preflight-$RUN_ID-preflight-summary.json" 2>/dev/null || echo null)"
  post_blockers="$(jq -r '.blockers // null' "$LOG_DIR/post-preflight/live-kafka-security-rollout-preflight-$RUN_ID-post-preflight-summary.json" 2>/dev/null || echo null)"

  jq -s \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg report "$REPORT" \
    --arg local_report "$LOCAL_REPORT" \
    --argjson total "$total" \
    --argjson passed "$passed" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson pre_prereq_blockers "$pre_prereq" \
    --argjson post_preflight_blockers "$post_blockers" \
    '{
      run_id:$run_id,
      result:$result,
      report:$report,
      local_report:$local_report,
      total:$total,
      passed:$passed,
      blockers:$blockers,
      warnings:$warnings,
      pre_prereq_blockers:$pre_prereq_blockers,
      post_preflight_blockers:$post_preflight_blockers,
      checks:.
    }' "$REPORT" >"$SUMMARY"

  cat >"$LOCAL_REPORT" <<EOF
# Kafka SASL_SSL Rollout

Run: \`$RUN_ID\`

Result: \`$result\`

This is the maintenance-window Kafka rollout gate. It is disruptive only when \`ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true\`.

## Summary

| Metric | Count |
|---|---:|
| Checks | $total |
| Passed | $passed |
| Blockers | $blockers |
| Warnings | $warnings |
| Preflight prerequisite blockers | $pre_prereq |
| Post-rollout preflight blockers | $post_blockers |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`preflight/\`
- \`post-preflight/\`
- \`kafka-post-rollout-topics.txt\`
- \`kafka-post-rollout-acls.txt\`
- \`kafka-post-rollout-broker-api.txt\`
EOF

  cp "$SUMMARY" "$SECURITY_DIR/kafka-sasl-ssl-rollout-latest.json"
  cp "$LOCAL_REPORT" "$SECURITY_DIR/kafka-sasl-ssl-rollout-latest.md"
  cat "$SUMMARY"
}

need_cmd git
need_cmd jq
need_cmd rg
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ ! -x "$PREFLIGHT_SCRIPT" ]]; then
  json_log "repo" "Kafka rollout preflight script is executable" "blocker" false "missing" "$PREFLIGHT_SCRIPT" "preflight-script"
  finish "blocked"
  exit 1
fi

if kctl apply --dry-run=server -f "$KAFKA_MANIFEST" >"$LOG_DIR/kafka-manifest-server-dry-run.txt" 2>"$LOG_DIR/kafka-manifest-server-dry-run.err"; then
  json_log "repo" "Kafka SASL_SSL manifest server dry-run" "info" true "ok" "$KAFKA_MANIFEST" "kafka-manifest-server-dry-run.txt"
else
  json_log "repo" "Kafka SASL_SSL manifest server dry-run" "blocker" false "failed" "$KAFKA_MANIFEST" "kafka-manifest-server-dry-run.err"
  finish "blocked"
  exit 1
fi

mkdir -p "$LOG_DIR/preflight"
ALLOW_BLOCKERS=true RUN_ID="$RUN_ID-preflight" LOG_DIR="$LOG_DIR/preflight" "$PREFLIGHT_SCRIPT" >"$LOG_DIR/preflight.stdout" 2>"$LOG_DIR/preflight.stderr" || true
pre_summary="$LOG_DIR/preflight/live-kafka-security-rollout-preflight-$RUN_ID-preflight-summary.json"
if [[ ! -s "$pre_summary" ]]; then
  json_log "preflight" "Kafka security preflight summary produced" "blocker" false "missing" "$pre_summary" "preflight.stderr"
  finish "blocked"
  exit 1
fi

pre_prereq_blockers="$(jq -r '.prereq_blockers' "$pre_summary")"
pre_rollout_blockers="$(jq -r '.rollout_blockers' "$pre_summary")"
if [[ "$pre_prereq_blockers" == "0" ]]; then
  json_log "preflight" "Kafka rollout prerequisites are clear" "info" true "ok" "rollout_blockers=$pre_rollout_blockers" "preflight"
else
  json_log "preflight" "Kafka rollout prerequisites are clear" "blocker" false "blocked" "prereq_blockers=$pre_prereq_blockers" "preflight"
  finish "blocked"
  exit 1
fi

kctl -n middleware get statefulset kafka -o json >"$LOG_DIR/kafka-statefulset-before.json"
kctl -n middleware get pods -l app=kafka -o wide >"$LOG_DIR/kafka-pods-before.txt"
kctl -n middleware get controllerrevision -l app=kafka -o json >"$LOG_DIR/kafka-controllerrevisions-before.json" 2>/dev/null || true

if [[ "$ALLOW_DISRUPTIVE_KAFKA_ROLLOUT" != "true" ]]; then
  json_log "rollout" "Disruptive Kafka rollout explicitly allowed" "blocker" false "not_allowed" "set ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true" "env"
  finish "blocked"
  exit 1
fi
json_log "rollout" "Disruptive Kafka rollout explicitly allowed" "info" true "ok" "ALLOW_DISRUPTIVE_KAFKA_ROLLOUT=true" "env"

if kctl -n middleware scale statefulset/kafka --replicas=0 >"$LOG_DIR/kafka-scale-down.txt" 2>"$LOG_DIR/kafka-scale-down.err" &&
   wait_pod_count middleware app=kafka 0 240; then
  json_log "rollout" "Kafka pods scaled down for full-protocol restart" "info" true "ok" "0 pods" "kafka-scale-down.txt"
else
  json_log "rollout" "Kafka pods scaled down for full-protocol restart" "blocker" false "failed" "pods did not reach 0" "kafka-scale-down.err"
  if [[ "$ROLLBACK_ON_FAILURE" == "true" ]]; then rollback_kafka; fi
  finish "blocked"
  exit 1
fi

rollout_failed=0
if kctl apply -f "$KAFKA_MANIFEST" >"$LOG_DIR/kafka-manifest-apply.txt" 2>"$LOG_DIR/kafka-manifest-apply.err"; then
  json_log "rollout" "Kafka SASL_SSL manifest applied" "info" true "ok" "$KAFKA_MANIFEST" "kafka-manifest-apply.txt"
else
  json_log "rollout" "Kafka SASL_SSL manifest applied" "blocker" false "failed" "$KAFKA_MANIFEST" "kafka-manifest-apply.err"
  rollout_failed=1
fi

if [[ "$rollout_failed" -eq 0 ]] &&
   kctl -n middleware rollout status statefulset/kafka --timeout="$WAIT_TIMEOUT" >"$LOG_DIR/kafka-rollout-status.txt" 2>"$LOG_DIR/kafka-rollout-status.err" &&
   kctl -n middleware wait --for=condition=Ready pod -l app=kafka --timeout="$WAIT_TIMEOUT" >"$LOG_DIR/kafka-pods-ready.txt" 2>"$LOG_DIR/kafka-pods-ready.err"; then
  json_log "rollout" "Kafka StatefulSet ready after SASL_SSL restart" "info" true "ok" "3 broker pods ready" "kafka-rollout-status.txt"
else
  json_log "rollout" "Kafka StatefulSet ready after SASL_SSL restart" "blocker" false "failed" "rollout or readiness wait failed" "kafka-rollout-status.err"
  rollout_failed=1
fi

kctl -n middleware get statefulset kafka -o json >"$LOG_DIR/kafka-statefulset-after-rollout.json" 2>/dev/null || true
kctl -n middleware get pods -l app=kafka -o wide >"$LOG_DIR/kafka-pods-after-rollout.txt" 2>/dev/null || true
kctl -n middleware describe pods -l app=kafka >"$LOG_DIR/kafka-pods-after-rollout.describe.txt" 2>/dev/null || true

if [[ "$rollout_failed" -ne 0 ]]; then
  if [[ "$ROLLBACK_ON_FAILURE" == "true" ]]; then rollback_kafka; fi
  finish "blocked"
  exit 1
fi

if kctl -n middleware delete job init-kafka-topics --ignore-not-found >"$LOG_DIR/init-kafka-topics-delete.txt" 2>"$LOG_DIR/init-kafka-topics-delete.err" &&
   kctl apply -f "$KAFKA_INIT_JOB_MANIFEST" >"$LOG_DIR/init-kafka-topics-apply.txt" 2>"$LOG_DIR/init-kafka-topics-apply.err" &&
   kctl -n middleware wait --for=condition=complete job/init-kafka-topics --timeout=300s >"$LOG_DIR/init-kafka-topics-wait.txt" 2>"$LOG_DIR/init-kafka-topics-wait.err"; then
  kctl -n middleware logs job/init-kafka-topics >"$LOG_DIR/init-kafka-topics.log" 2>"$LOG_DIR/init-kafka-topics.log.err" || true
  json_log "post" "Kafka topic and ACL init job completed on SASL_SSL" "info" true "ok" "$KAFKA_INIT_JOB_MANIFEST" "init-kafka-topics.log"
else
  kctl -n middleware logs job/init-kafka-topics >"$LOG_DIR/init-kafka-topics.log" 2>"$LOG_DIR/init-kafka-topics.log.err" || true
  json_log "post" "Kafka topic and ACL init job completed on SASL_SSL" "blocker" false "failed" "$KAFKA_INIT_JOB_MANIFEST" "init-kafka-topics-wait.err"
fi

if secure_kafka_admin_cmd "$LOG_DIR/kafka-post-rollout-broker-api.txt" "$LOG_DIR/kafka-post-rollout-broker-api.err" "/opt/kafka/bin/kafka-broker-api-versions.sh"; then
  json_log "post" "Kafka SASL_SSL broker API reachable" "info" true "ok" "broker API command succeeded" "kafka-post-rollout-broker-api.txt"
else
  json_log "post" "Kafka SASL_SSL broker API reachable" "blocker" false "failed" "broker API command failed" "kafka-post-rollout-broker-api.err"
fi

if secure_kafka_admin_cmd "$LOG_DIR/kafka-post-rollout-topics.txt" "$LOG_DIR/kafka-post-rollout-topics.err" "/opt/kafka/bin/kafka-topics.sh --list"; then
  topic_count="$(sed '/^[[:space:]]*$/d' "$LOG_DIR/kafka-post-rollout-topics.txt" | wc -l | tr -d ' ')"
  json_log "post" "Kafka topics listable over SASL_SSL" "info" true "ok" "topics=$topic_count" "kafka-post-rollout-topics.txt"
else
  json_log "post" "Kafka topics listable over SASL_SSL" "blocker" false "failed" "topic list command failed" "kafka-post-rollout-topics.err"
fi

if secure_kafka_admin_cmd "$LOG_DIR/kafka-post-rollout-acls.txt" "$LOG_DIR/kafka-post-rollout-acls.err" "/opt/kafka/bin/kafka-acls.sh --list"; then
  json_log "post" "Kafka ACLs listable over SASL_SSL" "info" true "ok" "ACL authorizer active" "kafka-post-rollout-acls.txt"
else
  json_log "post" "Kafka ACLs listable over SASL_SSL" "blocker" false "failed" "ACL list command failed" "kafka-post-rollout-acls.err"
fi

mkdir -p "$LOG_DIR/post-preflight"
ALLOW_BLOCKERS=true RUN_ID="$RUN_ID-post-preflight" LOG_DIR="$LOG_DIR/post-preflight" "$PREFLIGHT_SCRIPT" >"$LOG_DIR/post-preflight.stdout" 2>"$LOG_DIR/post-preflight.stderr" || true
post_summary="$LOG_DIR/post-preflight/live-kafka-security-rollout-preflight-$RUN_ID-post-preflight-summary.json"
if [[ -s "$post_summary" ]]; then
  post_blockers="$(jq -r '.blockers' "$post_summary")"
  if [[ "$post_blockers" == "0" ]]; then
    json_log "post" "Kafka security rollout preflight passes after rollout" "info" true "ok" "blockers=0" "post-preflight"
  else
    json_log "post" "Kafka security rollout preflight passes after rollout" "blocker" false "blocked" "blockers=$post_blockers" "post-preflight"
  fi
else
  json_log "post" "Kafka security rollout preflight passes after rollout" "blocker" false "missing" "$post_summary" "post-preflight.stderr"
fi

final_blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
if [[ "$final_blockers" -eq 0 ]]; then
  finish "pass"
else
  finish "blocked"
fi

if [[ "$final_blockers" -gt 0 ]]; then
  exit 1
fi
