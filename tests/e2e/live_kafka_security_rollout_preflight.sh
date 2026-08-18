#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-kafka-security-rollout-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-kafka-security-rollout-preflight}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
SEED_SCRAM="${SEED_SCRAM:-false}"
SECURITY_DIR="${SECURITY_DIR:-doc/02_acceptance/05-security}"

REPORT="$LOG_DIR/live-kafka-security-rollout-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-kafka-security-rollout-preflight-$RUN_ID-summary.json"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 800 "$file" | tr '\n' ' '
  fi
}

kafka_secure_admin_config() {
  cat <<'EOF'
PROPS=/tmp/kafka-security-preflight-client.properties
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

describe_scram_users_secure() {
  kctl -n middleware exec kafka-0 -- bash -lc "set -euo pipefail
$(kafka_secure_admin_config)
/opt/kafka/bin/kafka-configs.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config \"\$PROPS\" --describe --entity-type users
rm -f \"\$PROPS\""
}

list_acls_secure() {
  kctl -n middleware exec kafka-0 -- bash -lc "set -euo pipefail
$(kafka_secure_admin_config)
/opt/kafka/bin/kafka-acls.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config \"\$PROPS\" --list
rm -f \"\$PROPS\""
}

secret_value() {
  local namespace="$1" name="$2" key="$3"
  kctl -n "$namespace" get secret "$name" -o json 2>/dev/null |
    jq -r --arg key "$key" '.data[$key] // empty' |
    base64 -d 2>/dev/null || true
}

need_cmd git
need_cmd jq
need_cmd rg
need_cmd openssl
need_cmd keytool
need_cmd base64
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if rg -q 'listeners=SASL_SSL://|inter\.broker\.listener\.name=SASL_SSL|authorizer\.class\.name=org\.apache\.kafka\.metadata\.authorizer\.StandardAuthorizer|allow\.everyone\.if\.no\.acl\.found=false' deployments/kubernetes/infrastructure/01-kafka.yaml; then
  json_log "repo" "repo Kafka StatefulSet uses SASL_SSL authorizer profile" "info" true "ok" "SASL_SSL/SCRAM/StandardAuthorizer markers present" "01-kafka.yaml"
else
  json_log "repo" "repo Kafka StatefulSet uses SASL_SSL authorizer profile" "blocker" false "missing" "SASL_SSL/SCRAM/StandardAuthorizer markers missing" "01-kafka.yaml"
fi

if rg -q 'ssl\.keystore\.type=PKCS12|ssl\.truststore\.type=PKCS12|listener\.name\.controller\.ssl\.truststore\.type=PKCS12' deployments/kubernetes/infrastructure/01-kafka.yaml &&
   rg -q 'publishNotReadyAddresses: true' deployments/kubernetes/infrastructure/01-kafka.yaml &&
   rg -q 'listener\.name\.controller\.ssl\.client\.auth=required|super\.users=.*User:CN=kafka-bootstrap\.middleware\.svc' deployments/kubernetes/infrastructure/01-kafka.yaml; then
  json_log "repo" "repo Kafka TLS store type and headless DNS are rollout-safe" "info" true "ok" "PKCS12 store type, controller mTLS, and publishNotReadyAddresses present" "01-kafka.yaml"
else
  json_log "repo" "repo Kafka TLS store type and headless DNS are rollout-safe" "blocker" false "missing" "PKCS12 store type, controller mTLS, or publishNotReadyAddresses missing" "01-kafka.yaml"
fi

if rg -q 'kafka-acl-plan-v1|KAFKA_ACL_MIGRATION_PHASE' deployments/kubernetes/init-jobs/01-kafka-topics.yaml &&
   ! rg -q -- '--add[^[:cntrl:]]*--operation All' deployments/kubernetes/init-jobs/01-kafka-topics.yaml &&
   python3 scripts/alignment/generate_kafka_acl_plan.py --check-generated >/dev/null; then
  json_log "repo" "repo Kafka init job uses generated least-privilege ACLs" "info" true "ok" "literal per-service plan present; no wildcard All grant" "kafka-acl-catalog.v1.json"
else
  json_log "repo" "repo Kafka init job uses generated least-privilege ACLs" "blocker" false "missing" "generated ACL plan missing, stale, or broad grant remains" "01-kafka-topics.yaml"
fi

if rg -q 'KAFKA_SECURITY_PROTOCOL.*SASL_SSL|kafka.security.protocol:.*SASL_SSL|security.protocol=SASL_SSL' deployments/kubernetes/applications deployments/kubernetes/infrastructure deployments/kubernetes/flink deployments/kubernetes/init-jobs; then
  json_log "repo" "repo Kafka clients are configured for SASL_SSL" "info" true "ok" "Go/Flink/init-job client markers present" "deployments/kubernetes"
else
  json_log "repo" "repo Kafka clients are configured for SASL_SSL" "blocker" false "missing" "client SASL_SSL markers missing" "deployments/kubernetes"
fi

if ! rg -q 'key: KAFKA_CLIENT_USERNAME|key: KAFKA_CLIENT_PASSWORD' deployments/kubernetes/applications/go-services.yaml &&
   [[ "$(rg -o 'name: kafka-[a-z0-9-]+-credentials, key: username' deployments/kubernetes/applications/go-services.yaml | sort -u | wc -l)" -eq 8 ]] &&
   [[ "$(rg -o 'name: kafka-[a-z0-9-]+-credentials, key: password' deployments/kubernetes/applications/go-services.yaml | sort -u | wc -l)" -eq 8 ]]; then
  json_log "repo" "Go services use distinct Kafka credential Secrets" "info" true "ok" "8 username and 8 password bindings are unique" "go-services.yaml"
else
  json_log "repo" "Go services use distinct Kafka credential Secrets" "blocker" false "shared_or_missing" "shared client reference remains or service Secret bindings are incomplete" "go-services.yaml"
fi

if python3 scripts/alignment/render_audit_materializer_expand.py \
     --image "docker.io/traffic/audit-materializer@sha256:$(printf '1%.0s' {1..64})" \
     >"$LOG_DIR/audit-materializer-expand-rendered.yaml" &&
   rg -q 'name: kafka-audit-materializer-credentials, key: username' "$LOG_DIR/audit-materializer-expand-rendered.yaml" &&
   rg -q 'name: kafka-audit-materializer-credentials, key: password' "$LOG_DIR/audit-materializer-expand-rendered.yaml" &&
   ! rg -q '\$\{' "$LOG_DIR/audit-materializer-expand-rendered.yaml"; then
  json_log "repo" "audit materializer has an immutable-image expand manifest" "info" true "ok" "dedicated identity and digest-only renderer present" "audit-materializer-expand-rendered.yaml"
else
  json_log "repo" "audit materializer has an immutable-image expand manifest" "blocker" false "missing" "renderer, digest pin, or dedicated Secret binding is incomplete" "audit-materializer-expand.template.yaml"
fi

declare -a REQUIRED_SECRET_SPECS=(
  "middleware traffic-credentials KAFKA_CLIENT_USERNAME"
  "middleware traffic-credentials KAFKA_CLIENT_PASSWORD"
  "middleware traffic-credentials KAFKA_CLIENT_JAAS_CONFIG"
  "middleware traffic-credentials KAFKA_INTER_BROKER_USERNAME"
  "middleware traffic-credentials KAFKA_INTER_BROKER_PASSWORD"
  "middleware traffic-credentials KAFKA_TLS_KEYSTORE_PASSWORD"
  "middleware traffic-credentials KAFKA_TLS_TRUSTSTORE_PASSWORD"
  "middleware kafka-broker-tls kafka.keystore.p12"
  "middleware kafka-broker-tls kafka.truststore.p12"
  "middleware kafka-broker-tls ca.crt"
  "middleware kafka-client-tls kafka.truststore.p12"
  "middleware kafka-client-tls ca.crt"
  "middleware kafka-principal-credentials KAFKA_ALERT_SERVICE_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_AUDIT_MATERIALIZER_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_ASSET_SERVICE_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_AUTH_SERVICE_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_FORENSICS_SERVICE_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_GRAPH_SERVICE_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_INGEST_GATEWAY_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_RULE_MANAGER_PASSWORD"
  "middleware kafka-principal-credentials KAFKA_THREAT_INTEL_SERVICE_PASSWORD"
  "traffic-analysis kafka-client-tls kafka.truststore.p12"
  "traffic-analysis kafka-client-tls ca.crt"
  "traffic-analysis kafka-ingest-gateway-credentials username"
  "traffic-analysis kafka-ingest-gateway-credentials password"
  "traffic-analysis kafka-auth-service-credentials username"
  "traffic-analysis kafka-auth-service-credentials password"
  "traffic-analysis kafka-alert-service-credentials username"
  "traffic-analysis kafka-alert-service-credentials password"
  "traffic-analysis kafka-audit-materializer-credentials username"
  "traffic-analysis kafka-audit-materializer-credentials password"
  "traffic-analysis kafka-asset-service-credentials username"
  "traffic-analysis kafka-asset-service-credentials password"
  "traffic-analysis kafka-rule-manager-credentials username"
  "traffic-analysis kafka-rule-manager-credentials password"
  "traffic-analysis kafka-graph-service-credentials username"
  "traffic-analysis kafka-graph-service-credentials password"
  "traffic-analysis kafka-threat-intel-service-credentials username"
  "traffic-analysis kafka-threat-intel-service-credentials password"
  "traffic-analysis kafka-forensics-service-credentials username"
  "traffic-analysis kafka-forensics-service-credentials password"
  "flink kafka-client-tls kafka.truststore.p12"
  "flink kafka-client-tls ca.crt"
)

jq -n '[]' >"$LOG_DIR/kafka-security-secret-readiness.json"
SECRET_MISSING_COUNT=0
for spec in "${REQUIRED_SECRET_SPECS[@]}"; do
  read -r namespace name key <<<"$spec"
  set +e
  kctl -n "$namespace" get secret "$name" -o json >"$LOG_DIR/secret.tmp" 2>/dev/null
  rc=$?
  set -e
  present=false
  if [[ "$rc" -eq 0 ]] && jq -e --arg key "$key" '.data[$key] != null and .data[$key] != ""' "$LOG_DIR/secret.tmp" >/dev/null; then
    present=true
  else
    SECRET_MISSING_COUNT=$((SECRET_MISSING_COUNT + 1))
  fi
  jq \
    --arg namespace "$namespace" \
    --arg name "$name" \
    --arg key "$key" \
    --argjson present "$present" \
    '. + [{namespace:$namespace, name:$name, key:$key, present:$present}]' \
    "$LOG_DIR/kafka-security-secret-readiness.json" >"$LOG_DIR/kafka-security-secret-readiness.tmp"
  mv "$LOG_DIR/kafka-security-secret-readiness.tmp" "$LOG_DIR/kafka-security-secret-readiness.json"
done
rm -f "$LOG_DIR/secret.tmp"
if [[ "$SECRET_MISSING_COUNT" -eq 0 ]]; then
  json_log "live" "Kafka Secret/TLS material exists in target namespaces" "info" true "ok" "all ${#REQUIRED_SECRET_SPECS[@]} required keys present" "kafka-security-secret-readiness.json"
else
  json_log "live" "Kafka Secret/TLS material exists in target namespaces" "blocker" false "missing" "$SECRET_MISSING_COUNT required keys missing" "kafka-security-secret-readiness.json"
fi

TLS_VALIDATION_BLOCKERS=0
tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
  # 清理可能残留于 pod 内的 SCRAM 属性文件,避免凭证落盘于 pod。
  kctl -n middleware exec kafka-0 -- rm -f /tmp/seed-broker-scram.properties /tmp/seed-client-scram.properties >/dev/null 2>&1 || true
}
trap cleanup EXIT

kctl -n middleware get secret kafka-broker-tls -o jsonpath='{.data.kafka\.keystore\.p12}' 2>/dev/null | base64 -d >"$tmp_dir/kafka.keystore.p12" 2>/dev/null || true
kctl -n middleware get secret kafka-broker-tls -o jsonpath='{.data.kafka\.truststore\.p12}' 2>/dev/null | base64 -d >"$tmp_dir/kafka.truststore.p12" 2>/dev/null || true
kctl -n middleware get secret kafka-broker-tls -o jsonpath='{.data.ca\.crt}' 2>/dev/null | base64 -d >"$tmp_dir/ca.crt" 2>/dev/null || true
keystore_password="$(secret_value middleware traffic-credentials KAFKA_TLS_KEYSTORE_PASSWORD)"
truststore_password="$(secret_value middleware traffic-credentials KAFKA_TLS_TRUSTSTORE_PASSWORD)"

set +e
openssl pkcs12 -in "$tmp_dir/kafka.keystore.p12" -passin "pass:$keystore_password" -nokeys -info -out "$LOG_DIR/kafka-keystore-cert.pem" >"$LOG_DIR/kafka-keystore-validate.txt" 2>"$LOG_DIR/kafka-keystore-validate.err"
keystore_rc=$?
openssl pkcs12 -in "$tmp_dir/kafka.truststore.p12" -passin "pass:$truststore_password" -nokeys -info -out "$LOG_DIR/kafka-truststore-cert.pem" >"$LOG_DIR/kafka-truststore-validate.txt" 2>"$LOG_DIR/kafka-truststore-validate.err"
truststore_rc=$?
openssl x509 -in "$tmp_dir/ca.crt" -noout -subject -issuer -dates >"$LOG_DIR/kafka-ca-certificate.txt" 2>"$LOG_DIR/kafka-ca-certificate.err"
ca_rc=$?
keytool -list -storetype PKCS12 -keystore "$tmp_dir/kafka.truststore.p12" -storepass "$truststore_password" >"$LOG_DIR/kafka-truststore-keytool-list.txt" 2>"$LOG_DIR/kafka-truststore-keytool-list.err"
truststore_keytool_rc=$?
set -e

if [[ "$keystore_rc" -ne 0 ]]; then
  TLS_VALIDATION_BLOCKERS=$((TLS_VALIDATION_BLOCKERS + 1))
fi
if [[ "$truststore_rc" -ne 0 ]]; then
  TLS_VALIDATION_BLOCKERS=$((TLS_VALIDATION_BLOCKERS + 1))
fi
if [[ "$ca_rc" -ne 0 ]]; then
  TLS_VALIDATION_BLOCKERS=$((TLS_VALIDATION_BLOCKERS + 1))
fi
if [[ "$truststore_keytool_rc" -ne 0 ]] || ! rg -q 'trustedCertEntry' "$LOG_DIR/kafka-truststore-keytool-list.txt"; then
  TLS_VALIDATION_BLOCKERS=$((TLS_VALIDATION_BLOCKERS + 1))
fi
jq -n \
  --argjson keystore_rc "$keystore_rc" \
  --argjson truststore_rc "$truststore_rc" \
  --argjson ca_rc "$ca_rc" \
  --argjson truststore_keytool_rc "$truststore_keytool_rc" \
  --argjson truststore_has_trusted_cert "$(rg -q 'trustedCertEntry' "$LOG_DIR/kafka-truststore-keytool-list.txt" && echo true || echo false)" \
  --argjson blockers "$TLS_VALIDATION_BLOCKERS" \
  '{
    keystore_pkcs12_rc:$keystore_rc,
    truststore_pkcs12_rc:$truststore_rc,
    ca_certificate_rc:$ca_rc,
    truststore_keytool_rc:$truststore_keytool_rc,
    truststore_has_trusted_cert:$truststore_has_trusted_cert,
    tls_validation_blockers:$blockers
  }' >"$LOG_DIR/kafka-tls-material-validation.json"
if [[ "$TLS_VALIDATION_BLOCKERS" -eq 0 ]]; then
  json_log "live" "Kafka TLS material is parseable with live passwords" "info" true "ok" "keystore/truststore/CA parsed" "kafka-tls-material-validation.json"
else
  json_log "live" "Kafka TLS material is parseable with live passwords" "blocker" false "invalid" "tls validation blockers=$TLS_VALIDATION_BLOCKERS" "kafka-tls-material-validation.json"
fi

client_username="$(secret_value middleware traffic-credentials KAFKA_CLIENT_USERNAME)"
client_password="$(secret_value middleware traffic-credentials KAFKA_CLIENT_PASSWORD)"
broker_username="$(secret_value middleware traffic-credentials KAFKA_INTER_BROKER_USERNAME)"
broker_password="$(secret_value middleware traffic-credentials KAFKA_INTER_BROKER_PASSWORD)"
SCRAM_SEED_RC=0
if [[ "$SEED_SCRAM" == "true" ]]; then
  set +e
  # SCRAM 凭证经 stdin 写入 pod 内属性文件(不进命令行/审计日志),随后用 --add-config-file 执行。
  kctl -n middleware exec -i kafka-0 -- tee /tmp/seed-broker-scram.properties >/dev/null 2>&1 <<EOF
SCRAM-SHA-512=[password=$broker_password]
EOF
  broker_write_rc=$?
  if [[ "$broker_write_rc" -eq 0 ]]; then
    kctl -n middleware exec kafka-0 -- /opt/kafka/bin/kafka-configs.sh \
      --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
      --alter --add-config-file /tmp/seed-broker-scram.properties \
      --entity-type users --entity-name "$broker_username" >"$LOG_DIR/kafka-seed-broker-user.txt" 2>"$LOG_DIR/kafka-seed-broker-user.err"
    broker_seed_rc=$?
    kctl -n middleware exec kafka-0 -- rm -f /tmp/seed-broker-scram.properties >/dev/null 2>&1 || true
  else
    broker_seed_rc=1
    echo "failed to write broker scram properties into pod" >"$LOG_DIR/kafka-seed-broker-user.err"
  fi
  kctl -n middleware exec -i kafka-0 -- tee /tmp/seed-client-scram.properties >/dev/null 2>&1 <<EOF
SCRAM-SHA-512=[password=$client_password]
EOF
  client_write_rc=$?
  if [[ "$client_write_rc" -eq 0 ]]; then
    kctl -n middleware exec kafka-0 -- /opt/kafka/bin/kafka-configs.sh \
      --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
      --alter --add-config-file /tmp/seed-client-scram.properties \
      --entity-type users --entity-name "$client_username" >"$LOG_DIR/kafka-seed-client-user.txt" 2>"$LOG_DIR/kafka-seed-client-user.err"
    client_seed_rc=$?
    kctl -n middleware exec kafka-0 -- rm -f /tmp/seed-client-scram.properties >/dev/null 2>&1 || true
  else
    client_seed_rc=1
    echo "failed to write client scram properties into pod" >"$LOG_DIR/kafka-seed-client-user.err"
  fi
  set -e
  if [[ "$broker_seed_rc" -ne 0 || "$client_seed_rc" -ne 0 ]]; then
    SCRAM_SEED_RC=1
  fi
  jq -n \
    --argjson broker_seed_rc "$broker_seed_rc" \
    --argjson client_seed_rc "$client_seed_rc" \
    '{broker_seed_rc:$broker_seed_rc, client_seed_rc:$client_seed_rc}' >"$LOG_DIR/kafka-scram-seed-summary.json"
else
  jq -n '{seed_skipped:true}' >"$LOG_DIR/kafka-scram-seed-summary.json"
fi
if [[ "$SEED_SCRAM" == "true" && "$SCRAM_SEED_RC" -eq 0 ]]; then
  json_log "live" "Kafka SCRAM users seeded on plaintext cluster" "info" true "ok" "broker and app users altered" "kafka-scram-seed-summary.json"
elif [[ "$SEED_SCRAM" == "true" ]]; then
  json_log "live" "Kafka SCRAM users seeded on plaintext cluster" "blocker" false "failed" "seed rc=$SCRAM_SEED_RC" "kafka-scram-seed-summary.json"
else
  json_log "live" "Kafka SCRAM users seeded on plaintext cluster" "warn" false "skipped" "set SEED_SCRAM=true to seed before rollout" "kafka-scram-seed-summary.json"
fi

SCRAM_DESCRIBE_MODE="secure"
set +e
describe_scram_users_secure >"$LOG_DIR/kafka-scram-users-raw.txt" 2>"$LOG_DIR/kafka-scram-users.err"
SCRAM_DESCRIBE_RC=$?
if [[ "$SCRAM_DESCRIBE_RC" -ne 0 ]]; then
  SCRAM_DESCRIBE_MODE="plaintext"
  kctl -n middleware exec kafka-0 -- /opt/kafka/bin/kafka-configs.sh \
    --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
    --describe --entity-type users >"$LOG_DIR/kafka-scram-users-raw.txt" 2>"$LOG_DIR/kafka-scram-users.err"
  SCRAM_DESCRIBE_RC=$?
fi
set -e
sed -E 's/(salt|stored_key|server_key)=[^, ]+/\1=<redacted>/g' "$LOG_DIR/kafka-scram-users-raw.txt" >"$LOG_DIR/kafka-scram-users.txt" || true
rm -f "$LOG_DIR/kafka-scram-users-raw.txt"
SCRAM_CLIENT_PRESENT=0
SCRAM_BROKER_PRESENT=0
if [[ "$SCRAM_DESCRIBE_RC" -eq 0 ]]; then
  # 用户名按固定字符串匹配,禁止作为正则注入(agent.md §2 凭证/输入校验)。
  if rg -F -q "SCRAM credential configs for user-principal '${client_username}'" "$LOG_DIR/kafka-scram-users.txt"; then
    SCRAM_CLIENT_PRESENT=1
  fi
  if rg -F -q "SCRAM credential configs for user-principal '${broker_username}'" "$LOG_DIR/kafka-scram-users.txt"; then
    SCRAM_BROKER_PRESENT=1
  fi
fi
python3 - \
  contracts/events/kafka-acl-catalog.v1.json \
  "$LOG_DIR/kafka-scram-users.txt" \
  "$SCRAM_DESCRIBE_RC" \
  >"$LOG_DIR/kafka-workload-scram-readiness.json" <<'PY'
import json
import sys

catalog = json.load(open(sys.argv[1], encoding="utf-8"))
observed = open(sys.argv[2], encoding="utf-8", errors="replace").read()
describe_ok = int(sys.argv[3]) == 0
expected = sorted(
    item["principal"].removeprefix("User:")
    for item in catalog["principals"]
    if item.get("rollout_state") == "expand" and isinstance(item.get("credential"), dict)
)
present = [item for item in expected if describe_ok and item in observed]
missing = sorted(set(expected) - set(present))
print(json.dumps({
    "expected": expected,
    "present": present,
    "missing": missing,
    "expected_count": len(expected),
    "present_count": len(present),
    "ready": describe_ok and not missing,
}, ensure_ascii=False, indent=2))
PY
SCRAM_WORKLOAD_EXPECTED="$(jq -r '.expected_count' "$LOG_DIR/kafka-workload-scram-readiness.json")"
SCRAM_WORKLOAD_PRESENT="$(jq -r '.present_count' "$LOG_DIR/kafka-workload-scram-readiness.json")"
SCRAM_WORKLOAD_READY="$(jq -r '.ready' "$LOG_DIR/kafka-workload-scram-readiness.json")"
jq -n \
  --argjson describe_rc "$SCRAM_DESCRIBE_RC" \
  --arg describe_mode "$SCRAM_DESCRIBE_MODE" \
  --argjson client_present "$SCRAM_CLIENT_PRESENT" \
  --argjson broker_present "$SCRAM_BROKER_PRESENT" \
  --argjson workload_expected "$SCRAM_WORKLOAD_EXPECTED" \
  --argjson workload_present "$SCRAM_WORKLOAD_PRESENT" \
  --argjson workload_ready "$SCRAM_WORKLOAD_READY" \
  '{
    describe_rc:$describe_rc,
    describe_mode:$describe_mode,
    client_user_present:($client_present == 1),
    broker_user_present:($broker_present == 1),
    workload_users_expected:$workload_expected,
    workload_users_present:$workload_present,
    workload_users_ready:$workload_ready,
    scram_prereq_ready:($describe_rc == 0 and $broker_present == 1 and $workload_ready)
  }' >"$LOG_DIR/kafka-scram-readiness.json"
if [[ "$SCRAM_DESCRIBE_RC" -eq 0 && "$SCRAM_BROKER_PRESENT" -eq 1 && "$SCRAM_WORKLOAD_READY" == "true" ]]; then
  json_log "live" "Kafka SCRAM users exist before SASL_SSL rollout" "info" true "ok" "broker and all $SCRAM_WORKLOAD_EXPECTED workload users present" "kafka-scram-readiness.json"
else
  json_log "live" "Kafka SCRAM users exist before SASL_SSL rollout" "blocker" false "missing" "describe_rc=$SCRAM_DESCRIBE_RC broker=$SCRAM_BROKER_PRESENT workload=$SCRAM_WORKLOAD_PRESENT/$SCRAM_WORKLOAD_EXPECTED" "kafka-workload-scram-readiness.json"
fi

ACL_LIST_MODE="secure"
set +e
list_acls_secure >"$LOG_DIR/kafka-acls-live.txt" 2>"$LOG_DIR/kafka-acls-live.err"
ACL_LIST_RC=$?
if [[ "$ACL_LIST_RC" -ne 0 ]]; then
  ACL_LIST_MODE="plaintext"
  kctl -n middleware exec kafka-0 -- /opt/kafka/bin/kafka-acls.sh \
    --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
    --list >"$LOG_DIR/kafka-acls-live.txt" 2>"$LOG_DIR/kafka-acls-live.err"
  ACL_LIST_RC=$?
fi
set -e
cat "$LOG_DIR/kafka-acls-live.txt" "$LOG_DIR/kafka-acls-live.err" >"$LOG_DIR/kafka-acls-live.combined" 2>/dev/null || true
ACL_AUTHORIZER_DISABLED=0
if rg -q 'No Authorizer is configured|SecurityDisabledException' "$LOG_DIR/kafka-acls-live.combined"; then
  ACL_AUTHORIZER_DISABLED=1
fi
jq -n \
  --argjson acl_list_rc "$ACL_LIST_RC" \
  --arg acl_list_mode "$ACL_LIST_MODE" \
  --argjson authorizer_disabled "$ACL_AUTHORIZER_DISABLED" \
  '{acl_list_rc:$acl_list_rc, acl_list_mode:$acl_list_mode, authorizer_disabled:($authorizer_disabled == 1)}' >"$LOG_DIR/kafka-acl-live-summary.json"
if [[ "$ACL_LIST_RC" -eq 0 ]]; then
  json_log "live" "live Kafka ACL authorizer is enabled" "info" true "ok" "ACLs listable" "kafka-acl-live-summary.json"
elif [[ "$ACL_AUTHORIZER_DISABLED" -eq 1 ]]; then
  json_log "live" "live Kafka ACL authorizer is enabled" "blocker" false "disabled" "No Authorizer is configured on live broker" "kafka-acl-live-summary.json"
else
  json_log "live" "live Kafka ACL authorizer is enabled" "blocker" false "failed" "$(trim_file "$LOG_DIR/kafka-acls-live.err")" "kafka-acl-live-summary.json"
fi

kctl -n middleware get sts kafka -o json >"$LOG_DIR/live-kafka-statefulset.json"
jq -r '[.spec.template.spec.containers[]?.args[]?] | join("\n")' "$LOG_DIR/live-kafka-statefulset.json" >"$LOG_DIR/live-kafka-args.txt"
LIVE_KAFKA_PLAINTEXT_COUNT=0
LIVE_KAFKA_SASL_SSL_COUNT=0
if rg -q 'PLAINTEXT|inter\.broker\.listener\.name=PLAINTEXT' "$LOG_DIR/live-kafka-args.txt"; then
  LIVE_KAFKA_PLAINTEXT_COUNT=1
fi
if rg -q 'SASL_SSL|inter\.broker\.listener\.name=SASL_SSL' "$LOG_DIR/live-kafka-args.txt"; then
  LIVE_KAFKA_SASL_SSL_COUNT=1
fi
jq -n \
  --argjson live_plaintext "$LIVE_KAFKA_PLAINTEXT_COUNT" \
  --argjson live_sasl_ssl "$LIVE_KAFKA_SASL_SSL_COUNT" \
  '{live_plaintext_markers:$live_plaintext, live_sasl_ssl_markers:$live_sasl_ssl}' >"$LOG_DIR/live-kafka-listener-summary.json"
if [[ "$LIVE_KAFKA_PLAINTEXT_COUNT" -eq 0 && "$LIVE_KAFKA_SASL_SSL_COUNT" -gt 0 ]]; then
  json_log "live" "live Kafka listener has rolled to SASL_SSL" "info" true "ok" "SASL_SSL markers present and plaintext absent" "live-kafka-listener-summary.json"
else
  json_log "live" "live Kafka listener has rolled to SASL_SSL" "blocker" false "not_rolled" "plaintext=$LIVE_KAFKA_PLAINTEXT_COUNT sasl_ssl=$LIVE_KAFKA_SASL_SSL_COUNT" "live-kafka-listener-summary.json"
fi

TOTAL="$(wc -l <"$REPORT" | tr -d ' ')"
BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.passed == false and .severity == "warn")] | length' "$REPORT")"
PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
PREREQ_BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker" and (.name | test("live Kafka listener has rolled|live Kafka ACL authorizer is enabled") | not))] | length' "$REPORT")"
ROLLOUT_BLOCKERS="$(jq -s '[.[] | select(.passed == false and .severity == "blocker" and (.name | test("live Kafka listener has rolled|live Kafka ACL authorizer is enabled")))] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
fi

jq -s \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg report "$REPORT" \
  --arg local_report "$LOCAL_REPORT" \
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson prereq_blockers "$PREREQ_BLOCKERS" \
  --argjson rollout_blockers "$ROLLOUT_BLOCKERS" \
  --argjson secret_missing_count "$SECRET_MISSING_COUNT" \
  --argjson tls_validation_blockers "$TLS_VALIDATION_BLOCKERS" \
  --argjson scram_describe_rc "$SCRAM_DESCRIBE_RC" \
  --arg scram_describe_mode "$SCRAM_DESCRIBE_MODE" \
  --argjson scram_client_present "$SCRAM_CLIENT_PRESENT" \
  --argjson scram_broker_present "$SCRAM_BROKER_PRESENT" \
  --argjson scram_workload_expected "$SCRAM_WORKLOAD_EXPECTED" \
  --argjson scram_workload_present "$SCRAM_WORKLOAD_PRESENT" \
  --argjson scram_workload_ready "$SCRAM_WORKLOAD_READY" \
  --argjson acl_list_rc "$ACL_LIST_RC" \
  --arg acl_list_mode "$ACL_LIST_MODE" \
  --argjson acl_authorizer_disabled "$ACL_AUTHORIZER_DISABLED" \
  --argjson live_kafka_plaintext_count "$LIVE_KAFKA_PLAINTEXT_COUNT" \
  --argjson live_kafka_sasl_ssl_count "$LIVE_KAFKA_SASL_SSL_COUNT" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    prereq_blockers:$prereq_blockers,
    rollout_blockers:$rollout_blockers,
    secret_missing_count:$secret_missing_count,
    tls_validation_blockers:$tls_validation_blockers,
    scram_describe_rc:$scram_describe_rc,
    scram_describe_mode:$scram_describe_mode,
    scram_client_present:($scram_client_present == 1),
    scram_broker_present:($scram_broker_present == 1),
    scram_workload_expected:$scram_workload_expected,
    scram_workload_present:$scram_workload_present,
    scram_workload_ready:$scram_workload_ready,
    acl_list_rc:$acl_list_rc,
    acl_list_mode:$acl_list_mode,
    acl_authorizer_disabled:($acl_authorizer_disabled == 1),
    live_kafka_plaintext_count:$live_kafka_plaintext_count,
    live_kafka_sasl_ssl_count:$live_kafka_sasl_ssl_count,
    checks:.
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# Kafka Security Rollout Preflight

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This preflight is non-rolling. It checks whether the live plaintext Kafka cluster has the prerequisites needed for a later SASL_SSL/SCRAM/TLS/ACL maintenance-window rollout. Set \`SEED_SCRAM=true\` to seed SCRAM users without rolling Kafka.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| Prerequisite blockers | $PREREQ_BLOCKERS |
| Rollout blockers | $ROLLOUT_BLOCKERS |
| Missing Secret/TLS keys | $SECRET_MISSING_COUNT |
| TLS validation blockers | $TLS_VALIDATION_BLOCKERS |
| SCRAM client user present | $SCRAM_CLIENT_PRESENT |
| SCRAM broker user present | $SCRAM_BROKER_PRESENT |
| SCRAM workload users present | $SCRAM_WORKLOAD_PRESENT / $SCRAM_WORKLOAD_EXPECTED |
| ACL authorizer disabled | $ACL_AUTHORIZER_DISABLED |
| Live Kafka plaintext markers | $LIVE_KAFKA_PLAINTEXT_COUNT |
| Live Kafka SASL_SSL markers | $LIVE_KAFKA_SASL_SSL_COUNT |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`kafka-security-secret-readiness.json\`
- \`kafka-tls-material-validation.json\`
- \`kafka-scram-readiness.json\`
- \`kafka-workload-scram-readiness.json\`
- \`kafka-acl-live-summary.json\`
- \`live-kafka-listener-summary.json\`

## Interpretation

The repo Kafka profile, per-workload client manifests, Secret/TLS material, broker plus workload SCRAM users, live ACL authorizer state, and live listener state are checked separately. The legacy shared client is observed only for expand compatibility and does not satisfy workload-identity readiness. A later Kafka rollout remains blocked while workload users are missing, live listener markers are plaintext, or ACL authorizer is disabled. Prerequisite blockers should be zero before rolling the StatefulSet.
EOF

cp "$SUMMARY" "$SECURITY_DIR/kafka-security-rollout-preflight-latest.json"
cp "$LOCAL_REPORT" "$SECURITY_DIR/kafka-security-rollout-preflight-latest.md"
cp "$LOG_DIR/kafka-security-secret-readiness.json" "$SECURITY_DIR/kafka-security-secret-readiness-latest.json"
cp "$LOG_DIR/kafka-tls-material-validation.json" "$SECURITY_DIR/kafka-tls-material-validation-latest.json"
cp "$LOG_DIR/kafka-scram-readiness.json" "$SECURITY_DIR/kafka-scram-readiness-latest.json"
cp "$LOG_DIR/kafka-workload-scram-readiness.json" "$SECURITY_DIR/kafka-workload-scram-readiness-latest.json"
cp "$LOG_DIR/kafka-acl-live-summary.json" "$SECURITY_DIR/kafka-acl-live-summary-latest.json"
cp "$LOG_DIR/live-kafka-listener-summary.json" "$SECURITY_DIR/live-kafka-listener-summary-latest.json"

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
