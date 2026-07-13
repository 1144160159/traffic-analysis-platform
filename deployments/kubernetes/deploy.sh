#!/bin/bash
# =============================================================================
# Traffic Analysis Platform — 一键 K8s 部署脚本
#
# 用法:
#   ./deploy.sh              # 完整部署（基础设施 + 初始化 + 应用）
#   ./deploy.sh secrets      # 仅同步 Secret/TLS 引用（不滚动工作负载）
#   ./deploy.sh infra        # 仅部署基础设施
#   ./deploy.sh init         # 仅执行初始化 Jobs
#   ./deploy.sh apps         # 仅部署应用服务
#   ./deploy.sh services     # 仅收敛 Service 暴露面（APISIX 业务端口对外）
#   ./deploy.sh status       # 查看部署状态
#   ./deploy.sh clean        # 删除所有资源
#
# 前置条件:
#   1. K8s 集群已安装（v1.29+）
#   2. kubectl 已配置可访问集群
#   3. StorageClass "local-hdd" 已创建（或修改 PVC 为其他 SC）
#   4. MetalLB 已配置（IP 池 10.0.5.200-10.0.5.250）
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KUBECTL="${KUBECTL:-kubectl}"

# ---- 颜色 ----
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'
info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*"; }

apply() {
  local dir="$1"
  local desc="$2"
  info "Deploying $desc..."
  for f in "$dir"/*.yaml; do
    if [ -f "$f" ]; then
      $KUBECTL apply -f "$f"
    fi
  done
}

reconcile_service_exposure_profile() {
  local root
  if ! command -v python3 >/dev/null 2>&1; then
    error "python3 is required to reconcile Service exposure profile"
    return 1
  fi
  if [ ! -f "$SCRIPT_DIR/scripts/reconcile_service_exposure.py" ]; then
    error "Service exposure reconciler not found: $SCRIPT_DIR/scripts/reconcile_service_exposure.py"
    return 1
  fi

  info "Reconciling Service exposure profile..."
  local args=()
  for root in "$@"; do
    if [ -e "$root" ]; then
      args+=(--root "$root")
    fi
  done
  if [ "${#args[@]}" -eq 0 ]; then
    warn "No Service manifest roots found, skipping exposure reconciliation"
    return 0
  fi
  python3 "$SCRIPT_DIR/scripts/reconcile_service_exposure.py" "${args[@]}"
}

ensure_namespaces() {
  for namespace in middleware databases traffic-analysis flink gateway minio observability argo argo-events iam nacos streampark registry; do
    $KUBECTL create namespace "$namespace" --dry-run=client -o yaml | $KUBECTL apply -f - >/dev/null
  done
}

secret_value() {
  local namespace="$1"
  local key="$2"
  $KUBECTL get secret traffic-credentials -n "$namespace" \
    -o "jsonpath={.data.${key}}" 2>/dev/null | base64 -d 2>/dev/null || true
}

ensure_application_credentials() {
  local pg_password pg_dsn pg_replication_password minio_access_key minio_secret_key
  local ch_password jwt_secret redis_password apisix_admin_key oidc_client_secret
  local grafana_admin_password keycloak_admin_password opensearch_admin_password
  local kafka_client_username kafka_client_password kafka_client_jaas_config
  local kafka_inter_broker_username kafka_inter_broker_password
  local kafka_tls_keystore_password kafka_tls_truststore_password

  pg_password="$(secret_value databases PG_PASSWORD)"
  pg_password="${pg_password:-$(secret_value traffic-analysis PG_PASSWORD)}"
  pg_password="${pg_password:-$(secret_value middleware PG_PASSWORD)}"
  pg_password="${pg_password:-$(openssl rand -base64 32)}"
  pg_dsn="$(secret_value traffic-analysis POSTGRES_DSN)"
  pg_dsn="${pg_dsn:-host=postgres-primary.databases.svc port=5432 user=postgres password=$pg_password dbname=traffic_platform sslmode=disable}"

  pg_replication_password="$(secret_value databases PG_REPLICATION_PASSWORD)"
  pg_replication_password="${pg_replication_password:-$(secret_value middleware PG_REPLICATION_PASSWORD)}"
  pg_replication_password="${pg_replication_password:-$(openssl rand -base64 32)}"

  minio_access_key="$(secret_value minio MINIO_ACCESS_KEY)"
  minio_access_key="${minio_access_key:-$(secret_value traffic-analysis MINIO_ACCESS_KEY)}"
  minio_access_key="${minio_access_key:-$(secret_value middleware MINIO_ACCESS_KEY)}"
  minio_access_key="${minio_access_key:-$(openssl rand -hex 16)}"

  minio_secret_key="$(secret_value minio MINIO_SECRET_KEY)"
  minio_secret_key="${minio_secret_key:-$(secret_value traffic-analysis MINIO_SECRET_KEY)}"
  minio_secret_key="${minio_secret_key:-$(secret_value middleware MINIO_SECRET_KEY)}"
  minio_secret_key="${minio_secret_key:-$(openssl rand -base64 32)}"

  ch_password="$(secret_value middleware CLICKHOUSE_PASSWORD)"
  ch_password="${ch_password:-$(openssl rand -base64 32)}"
  redis_password="$(secret_value middleware REDIS_PASSWORD)"
  redis_password="${redis_password:-$(openssl rand -base64 32)}"
  jwt_secret="$(secret_value traffic-analysis JWT_SECRET)"
  jwt_secret="${jwt_secret:-$(secret_value middleware JWT_SECRET)}"
  jwt_secret="${jwt_secret:-$(openssl rand -base64 64)}"
  apisix_admin_key="$(secret_value gateway APISIX_ADMIN_KEY)"
  apisix_admin_key="${apisix_admin_key:-$(secret_value middleware APISIX_ADMIN_KEY)}"
  apisix_admin_key="${apisix_admin_key:-$(openssl rand -hex 16)}"
  opensearch_admin_password="$(secret_value middleware OPENSEARCH_ADMIN_PASSWORD)"
  opensearch_admin_password="${opensearch_admin_password:-$(secret_value traffic-analysis OPENSEARCH_ADMIN_PASSWORD)}"
  opensearch_admin_password="${opensearch_admin_password:-$(secret_value flink OPENSEARCH_ADMIN_PASSWORD)}"
  opensearch_admin_password="${opensearch_admin_password:-$(openssl rand -base64 32)}"
  kafka_client_username="$(secret_value middleware KAFKA_CLIENT_USERNAME)"
  kafka_client_username="${kafka_client_username:-$(secret_value traffic-analysis KAFKA_CLIENT_USERNAME)}"
  kafka_client_username="${kafka_client_username:-traffic-app}"
  kafka_client_password="$(secret_value middleware KAFKA_CLIENT_PASSWORD)"
  kafka_client_password="${kafka_client_password:-$(secret_value traffic-analysis KAFKA_CLIENT_PASSWORD)}"
  kafka_client_password="${kafka_client_password:-$(openssl rand -base64 32)}"
  kafka_client_jaas_config="$(secret_value middleware KAFKA_CLIENT_JAAS_CONFIG)"
  kafka_client_jaas_config="${kafka_client_jaas_config:-org.apache.kafka.common.security.scram.ScramLoginModule required username=\"$kafka_client_username\" password=\"$kafka_client_password\";}"
  kafka_inter_broker_username="$(secret_value middleware KAFKA_INTER_BROKER_USERNAME)"
  kafka_inter_broker_username="${kafka_inter_broker_username:-traffic-broker}"
  kafka_inter_broker_password="$(secret_value middleware KAFKA_INTER_BROKER_PASSWORD)"
  kafka_inter_broker_password="${kafka_inter_broker_password:-$(openssl rand -base64 32)}"
  kafka_tls_keystore_password="$(secret_value middleware KAFKA_TLS_KEYSTORE_PASSWORD)"
  kafka_tls_keystore_password="${kafka_tls_keystore_password:-$(openssl rand -base64 32)}"
  kafka_tls_truststore_password="$(secret_value middleware KAFKA_TLS_TRUSTSTORE_PASSWORD)"
  kafka_tls_truststore_password="${kafka_tls_truststore_password:-$(secret_value traffic-analysis KAFKA_TLS_TRUSTSTORE_PASSWORD)}"
  kafka_tls_truststore_password="${kafka_tls_truststore_password:-$(openssl rand -base64 32)}"
  oidc_client_secret="$(secret_value traffic-analysis OIDC_CLIENT_SECRET)"
  oidc_client_secret="${oidc_client_secret:-$(secret_value middleware OIDC_CLIENT_SECRET)}"
  oidc_client_secret="${oidc_client_secret:-$(openssl rand -base64 48)}"
  grafana_admin_password="$(secret_value observability GRAFANA_ADMIN_PASSWORD)"
  grafana_admin_password="${grafana_admin_password:-$(secret_value middleware GRAFANA_ADMIN_PASSWORD)}"
  grafana_admin_password="${grafana_admin_password:-$(openssl rand -base64 32)}"
  keycloak_admin_password="$(secret_value iam KEYCLOAK_ADMIN_PASSWORD)"
  keycloak_admin_password="${keycloak_admin_password:-$(secret_value middleware KEYCLOAK_ADMIN_PASSWORD)}"
  keycloak_admin_password="${keycloak_admin_password:-$(openssl rand -base64 32)}"

  info "Syncing application credentials with running infrastructure..."
  for target_namespace in middleware traffic-analysis gateway flink databases minio observability iam; do
    if ! $KUBECTL get namespace "$target_namespace" >/dev/null 2>&1; then
      warn "Namespace $target_namespace not found; skipping traffic-credentials sync"
      continue
    fi

    $KUBECTL create secret generic traffic-credentials \
      --from-literal=PG_PASSWORD="$pg_password" \
      --from-literal=POSTGRES_DSN="$pg_dsn" \
      --from-literal=PG_REPLICATION_PASSWORD="$pg_replication_password" \
      --from-literal=CLICKHOUSE_PASSWORD="$ch_password" \
      --from-literal=REDIS_PASSWORD="$redis_password" \
      --from-literal=MINIO_ACCESS_KEY="$minio_access_key" \
      --from-literal=MINIO_SECRET_KEY="$minio_secret_key" \
      --from-literal=JWT_SECRET="$jwt_secret" \
      --from-literal=OIDC_CLIENT_SECRET="$oidc_client_secret" \
      --from-literal=APISIX_ADMIN_KEY="$apisix_admin_key" \
      --from-literal=OPENSEARCH_ADMIN_PASSWORD="$opensearch_admin_password" \
      --from-literal=KAFKA_CLIENT_USERNAME="$kafka_client_username" \
      --from-literal=KAFKA_CLIENT_PASSWORD="$kafka_client_password" \
      --from-literal=KAFKA_CLIENT_JAAS_CONFIG="$kafka_client_jaas_config" \
      --from-literal=KAFKA_INTER_BROKER_USERNAME="$kafka_inter_broker_username" \
      --from-literal=KAFKA_INTER_BROKER_PASSWORD="$kafka_inter_broker_password" \
      --from-literal=KAFKA_TLS_KEYSTORE_PASSWORD="$kafka_tls_keystore_password" \
      --from-literal=KAFKA_TLS_TRUSTSTORE_PASSWORD="$kafka_tls_truststore_password" \
      --from-literal=GRAFANA_ADMIN_PASSWORD="$grafana_admin_password" \
      --from-literal=KEYCLOAK_ADMIN_PASSWORD="$keycloak_admin_password" \
      -n "$target_namespace" --dry-run=client -o yaml | $KUBECTL apply -f -
  done
}

ensure_kafka_tls_secrets() {
  if $KUBECTL get secret kafka-broker-tls -n middleware >/dev/null 2>&1 &&
     $KUBECTL get secret kafka-client-tls -n middleware >/dev/null 2>&1 &&
     $KUBECTL get secret kafka-client-tls -n traffic-analysis >/dev/null 2>&1 &&
     $KUBECTL get secret kafka-client-tls -n flink >/dev/null 2>&1; then
    local existing_truststore_password existing_tmp_dir
    existing_truststore_password="$(secret_value middleware KAFKA_TLS_TRUSTSTORE_PASSWORD)"
    existing_tmp_dir="$(mktemp -d)"
    if [ -n "$existing_truststore_password" ] && command -v keytool >/dev/null 2>&1; then
      $KUBECTL get secret kafka-broker-tls -n middleware \
        -o jsonpath='{.data.kafka\.truststore\.p12}' | base64 -d >"$existing_tmp_dir/kafka.truststore.p12" 2>/dev/null || true
      if keytool -list -storetype PKCS12 \
          -keystore "$existing_tmp_dir/kafka.truststore.p12" \
          -storepass "$existing_truststore_password" 2>/dev/null | grep -q 'trustedCertEntry'; then
        rm -rf "$existing_tmp_dir"
        info "Kafka TLS secrets already exist."
        return
      fi
    fi
    rm -rf "$existing_tmp_dir"
    warn "Kafka TLS secrets already exist but truststore validation failed; regenerating."
  fi

  if ! command -v keytool >/dev/null 2>&1; then
    error "keytool is required to generate Java-compatible Kafka truststores"
    return 1
  fi

  local keystore_password truststore_password tmp_dir
  keystore_password="$(secret_value middleware KAFKA_TLS_KEYSTORE_PASSWORD)"
  truststore_password="$(secret_value middleware KAFKA_TLS_TRUSTSTORE_PASSWORD)"
  if [ -z "$keystore_password" ] || [ -z "$truststore_password" ]; then
    warn "Kafka TLS passwords are missing; syncing traffic-credentials first"
    ensure_application_credentials
    keystore_password="$(secret_value middleware KAFKA_TLS_KEYSTORE_PASSWORD)"
    truststore_password="$(secret_value middleware KAFKA_TLS_TRUSTSTORE_PASSWORD)"
  fi

  tmp_dir="$(mktemp -d)"
  info "Generating Kafka TLS keystore/truststore secrets..."
  cat >"$tmp_dir/san.cnf" <<'EOF'
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = kafka-bootstrap.middleware.svc
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = kafka-bootstrap.middleware.svc
DNS.2 = kafka-bootstrap.middleware.svc.cluster.local
DNS.3 = kafka-headless.middleware.svc
DNS.4 = kafka-headless.middleware.svc.cluster.local
DNS.5 = kafka-0.kafka-headless.middleware.svc.cluster.local
DNS.6 = kafka-1.kafka-headless.middleware.svc.cluster.local
DNS.7 = kafka-2.kafka-headless.middleware.svc.cluster.local
EOF

  openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout "$tmp_dir/ca.key" -out "$tmp_dir/ca.crt" \
    -subj "/CN=traffic-kafka-ca" -days 3650 >/dev/null 2>&1
  openssl req -newkey rsa:4096 -nodes \
    -keyout "$tmp_dir/kafka.key" -out "$tmp_dir/kafka.csr" \
    -config "$tmp_dir/san.cnf" >/dev/null 2>&1
  openssl x509 -req -in "$tmp_dir/kafka.csr" \
    -CA "$tmp_dir/ca.crt" -CAkey "$tmp_dir/ca.key" -CAcreateserial \
    -out "$tmp_dir/kafka.crt" -days 825 -sha256 \
    -extensions v3_req -extfile "$tmp_dir/san.cnf" >/dev/null 2>&1
  openssl pkcs12 -export \
    -in "$tmp_dir/kafka.crt" -inkey "$tmp_dir/kafka.key" -certfile "$tmp_dir/ca.crt" \
    -out "$tmp_dir/kafka.keystore.p12" -name kafka \
    -passout "pass:$keystore_password" >/dev/null 2>&1
  keytool -importcert -noprompt -trustcacerts \
    -alias traffic-kafka-ca \
    -file "$tmp_dir/ca.crt" \
    -keystore "$tmp_dir/kafka.truststore.p12" \
    -storetype PKCS12 \
    -storepass "$truststore_password" >/dev/null 2>&1

  $KUBECTL create secret generic kafka-broker-tls \
    --from-file="$tmp_dir/kafka.keystore.p12" \
    --from-file="$tmp_dir/kafka.truststore.p12" \
    --from-file="$tmp_dir/ca.crt" \
    -n middleware --dry-run=client -o yaml | $KUBECTL apply -f -

  for namespace in middleware traffic-analysis flink; do
    $KUBECTL create secret generic kafka-client-tls \
      --from-file="$tmp_dir/kafka.truststore.p12" \
      --from-file="$tmp_dir/ca.crt" \
      -n "$namespace" --dry-run=client -o yaml | $KUBECTL apply -f -
  done

  if $KUBECTL get namespace external-secrets-source >/dev/null 2>&1; then
    $KUBECTL create secret generic traffic-platform-prod-kafka-broker-tls \
      --from-file="$tmp_dir/kafka.keystore.p12" \
      --from-file="$tmp_dir/kafka.truststore.p12" \
      --from-file="$tmp_dir/ca.crt" \
      -n external-secrets-source --dry-run=client -o yaml | $KUBECTL apply -f -
    $KUBECTL create secret generic traffic-platform-prod-kafka-client-tls \
      --from-file="$tmp_dir/kafka.truststore.p12" \
      --from-file="$tmp_dir/ca.crt" \
      -n external-secrets-source --dry-run=client -o yaml | $KUBECTL apply -f -
  fi

  rm -rf "$tmp_dir"
}

ensure_keycloak_tls_secret() {
  if $KUBECTL get secret keycloak-tls -n iam >/dev/null 2>&1; then
    info "Keycloak TLS secret already exists."
    return
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  info "Generating Keycloak TLS secret..."
  cat >"$tmp_dir/san.cnf" <<'EOF'
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = keycloak.iam.svc
[v3_req]
subjectAltName = @alt_names
[alt_names]
DNS.1 = keycloak
DNS.2 = keycloak.iam
DNS.3 = keycloak.iam.svc
DNS.4 = keycloak.iam.svc.cluster.local
EOF

  openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout "$tmp_dir/tls.key" -out "$tmp_dir/tls.crt" \
    -subj "/CN=keycloak.iam.svc" -days 825 \
    -extensions v3_req -config "$tmp_dir/san.cnf" >/dev/null 2>&1

  $KUBECTL create secret tls keycloak-tls \
    --cert="$tmp_dir/tls.crt" \
    --key="$tmp_dir/tls.key" \
    -n iam --dry-run=client -o yaml | $KUBECTL apply -f -

  rm -rf "$tmp_dir"
}

ensure_secret_readiness() {
  info "========================================="
  info "Secret/TLS readiness"
  info "========================================="
  ensure_namespaces
  ensure_application_credentials
  ensure_kafka_tls_secrets
  ensure_keycloak_tls_secret
  ensure_probe_mtls_certs
}

seed_kafka_scram_users() {
  if ! $KUBECTL get pod kafka-0 -n middleware >/dev/null 2>&1; then
    info "Kafka is not running yet; SCRAM users will be seeded during storage format."
    return
  fi

  local client_username client_password broker_username broker_password
  client_username="$(secret_value middleware KAFKA_CLIENT_USERNAME)"
  client_password="$(secret_value middleware KAFKA_CLIENT_PASSWORD)"
  broker_username="$(secret_value middleware KAFKA_INTER_BROKER_USERNAME)"
  broker_password="$(secret_value middleware KAFKA_INTER_BROKER_PASSWORD)"
  if [ -z "$client_username" ] || [ -z "$client_password" ] || [ -z "$broker_username" ] || [ -z "$broker_password" ]; then
    warn "Kafka SCRAM credentials are incomplete; skipping live SCRAM seed"
    return
  fi

  info "Seeding Kafka SCRAM users on existing plaintext cluster before SASL_SSL rollout..."
  $KUBECTL exec -n middleware kafka-0 -- /opt/kafka/bin/kafka-configs.sh \
    --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
    --alter --add-config "SCRAM-SHA-512=[password=$broker_password]" \
    --entity-type users --entity-name "$broker_username" >/dev/null 2>&1 || \
    warn "Could not seed inter-broker SCRAM user; cluster may already require SASL_SSL"
  $KUBECTL exec -n middleware kafka-0 -- /opt/kafka/bin/kafka-configs.sh \
    --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
    --alter --add-config "SCRAM-SHA-512=[password=$client_password]" \
    --entity-type users --entity-name "$client_username" >/dev/null 2>&1 || \
    warn "Could not seed client SCRAM user; cluster may already require SASL_SSL"
}

ensure_probe_mtls_certs() {
  if $KUBECTL get secret probe-agent-certs -n traffic-analysis >/dev/null 2>&1 &&
     $KUBECTL get secret ingest-gateway-certs -n traffic-analysis >/dev/null 2>&1; then
    info "Probe/Ingest mTLS secrets already exist."
    return
  fi

  local cert_script="${SCRIPT_DIR}/../../rust/probe-agent/scripts/generate-mtls-certs.sh"
  if [ ! -x "$cert_script" ]; then
    warn "mTLS cert script not found or not executable: $cert_script"
    return
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  info "Generating Probe ↔ Ingest mTLS secrets..."
  bash "$cert_script" "$tmp_dir" >/dev/null

  $KUBECTL create secret generic probe-agent-certs \
    --from-file="$tmp_dir/ca-cert.pem" \
    --from-file="$tmp_dir/client-cert.pem" \
    --from-file="$tmp_dir/client-key.pem" \
    -n traffic-analysis --dry-run=client -o yaml | $KUBECTL apply -f -

  $KUBECTL create secret generic ingest-gateway-certs \
    --from-file="$tmp_dir/ca-cert.pem" \
    --from-file="$tmp_dir/server-cert.pem" \
    --from-file="$tmp_dir/server-key.pem" \
    -n traffic-analysis --dry-run=client -o yaml | $KUBECTL apply -f -

  rm -rf "$tmp_dir"
}

seed_probe_token() {
  local token="${PROBE_AUTH_TOKEN:-probe-token-default-001}"
  local tenant="${PROBE_TENANT_ID:-default}"
  local probe_cn="${PROBE_CERT_CN:-probe-agent}"

  if ! $KUBECTL get pod redis-master-0 -n databases >/dev/null 2>&1; then
    warn "redis-master-0 not found; skipping probe token seed"
    return
  fi

  local token_hash expires payload
  token_hash="$(printf "%s" "$token" | sha256sum | awk '{print $1}')"
  expires="$(($(date +%s) + 31536000))"
  payload="$(printf '{"tenant_id":"%s","probe_id":"%s","scopes":["ingest:write","pcap:write"],"expires_at":%s}' "$tenant" "$probe_cn" "$expires")"

  info "Seeding probe token cache for CN=$probe_cn tenant=$tenant..."
  local master_addr master_ip master_port
  master_addr="$($KUBECTL exec -n databases redis-sentinel-0 -- \
    redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster 2>/dev/null || true)"
  master_ip="$(printf "%s\n" "$master_addr" | sed -n '1p' | tr -d '\r')"
  master_port="$(printf "%s\n" "$master_addr" | sed -n '2p' | tr -d '\r')"

  if [ -n "$master_ip" ] && [ -n "$master_port" ]; then
    $KUBECTL exec -n databases redis-master-0 -- \
      redis-cli -h "$master_ip" -p "$master_port" \
      SET "session:${probe_cn}:${token_hash}" "$payload" EX 31536000 >/dev/null || \
      warn "Could not seed probe token cache via Redis Sentinel master"
  else
    warn "Could not resolve Redis Sentinel master; falling back to redis-master service"
    $KUBECTL exec -n databases redis-master-0 -- \
      redis-cli SET "session:${probe_cn}:${token_hash}" "$payload" EX 31536000 >/dev/null || \
      warn "Could not seed probe token cache"
  fi
}

deploy_infra() {
  info "========================================="
  info "Phase 1/4: Namespaces"
  info "========================================="
  ensure_namespaces
  ensure_application_credentials
  ensure_kafka_tls_secrets
  ensure_keycloak_tls_secret
  seed_kafka_scram_users

  info "========================================="
  info "Phase 2/4: Middleware (Kafka, ClickHouse, OpenSearch, MinIO)"
  info "========================================="
  apply "$SCRIPT_DIR/infrastructure" "Infrastructure StatefulSets"
  reconcile_service_exposure_profile "$SCRIPT_DIR/infrastructure"

  info "Waiting for middleware StatefulSets (this may take 2-5 minutes)..."
  $KUBECTL wait --for=condition=ready pod -l app=kafka -n middleware --timeout=300s 2>/dev/null || warn "Kafka not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=clickhouse,shard=1 -n middleware --timeout=300s 2>/dev/null || warn "ClickHouse shard-1 not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=clickhouse,shard=2 -n middleware --timeout=300s 2>/dev/null || warn "ClickHouse shard-2 not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=opensearch -n middleware --timeout=300s 2>/dev/null || warn "OpenSearch not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=minio -n middleware --timeout=300s 2>/dev/null || warn "MinIO not all ready yet"

  info "========================================="
  info "Phase 3/4: Databases (PostgreSQL, Redis)"
  info "========================================="
  $KUBECTL wait --for=condition=ready pod -l app=postgres -n databases --timeout=300s 2>/dev/null || warn "PostgreSQL not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=redis -n databases --timeout=120s 2>/dev/null || warn "Redis not all ready yet"
  $KUBECTL wait --for=condition=ready pod -l app=redis-sentinel -n databases --timeout=60s 2>/dev/null || warn "Redis Sentinel not all ready yet"

  info "========================================="
  info "Phase 4/4: Supporting Services"
  info "========================================="
  apply "$SCRIPT_DIR/infrastructure" "Flink + Gateway Services + NebulaGraph"
  reconcile_service_exposure_profile "$SCRIPT_DIR/infrastructure"

  # 标记 NebulaGraph 主节点 (Node-9)
  info "Labeling Node-9 as NebulaGraph primary..."
  $KUBECTL label node zeus-server nebula-primary=true --overwrite 2>/dev/null || warn "Could not label zeus-server (may already be labeled)"

  info "Waiting for NebulaGraph Meta..."
  $KUBECTL wait --for=condition=ready pod -l app=nebula,component=meta -n middleware --timeout=300s 2>/dev/null || warn "NebulaGraph Meta not all ready yet"
  info "Waiting for NebulaGraph Storage..."
  $KUBECTL wait --for=condition=ready pod -l app=nebula,component=storage -n middleware --timeout=300s 2>/dev/null || warn "NebulaGraph Storage not all ready yet"
  info "Waiting for NebulaGraph Graph..."
  $KUBECTL wait --for=condition=ready pod -l app=nebula,component=graph -n middleware --timeout=120s 2>/dev/null || warn "NebulaGraph Graph not all ready yet"

  apply "$SCRIPT_DIR/observability" "Monitoring (VictoriaMetrics, Grafana, Loki)"
}

deploy_init() {
  info "========================================="
  info "Initialization Jobs"
  info "========================================="

  info "Creating Kafka topics..."
  apply "$SCRIPT_DIR/init-jobs" "Kafka Topics Job"
  $KUBECTL wait --for=condition=complete job/init-kafka-topics -n middleware --timeout=120s 2>/dev/null || warn "Kafka topic init may still be running"

  info "Initializing PostgreSQL schema..."
  apply "$SCRIPT_DIR/init-jobs" "PostgreSQL Schema Job"
  $KUBECTL wait --for=condition=complete job/init-postgres-schema -n databases --timeout=120s 2>/dev/null || warn "PG schema init may still be running"

  info "Initializing ClickHouse schema..."
  apply "$SCRIPT_DIR/init-jobs" "ClickHouse Schema Job"
  $KUBECTL wait --for=condition=complete job/init-clickhouse-schema -n middleware --timeout=120s 2>/dev/null || warn "CH schema init may still be running"

  info "Initializing OpenSearch templates..."
  apply "$SCRIPT_DIR/init-jobs" "OpenSearch Templates Job"
  $KUBECTL wait --for=condition=complete job/init-opensearch-templates -n middleware --timeout=120s 2>/dev/null || warn "OS template init may still be running"

  info "Applying MinIO lifecycle policies..."
  $KUBECTL wait --for=condition=complete job/init-minio-lifecycle -n minio --timeout=120s 2>/dev/null || warn "MinIO lifecycle init may still be running"

  info "Initializing NebulaGraph schema..."
  apply "$SCRIPT_DIR/init-jobs" "NebulaGraph Schema Job"
  $KUBECTL wait --for=condition=complete job/init-nebula-schema -n middleware --timeout=180s 2>/dev/null || warn "NebulaGraph schema init may still be running"
}

deploy_apps() {
  info "========================================="
  info "Deploying Application Services"
  info "========================================="
  ensure_application_credentials
  ensure_probe_mtls_certs
  seed_probe_token
  apply "$SCRIPT_DIR/applications" "Go Control-Plane Services"
  reconcile_service_exposure_profile "$SCRIPT_DIR/applications"

  info "Waiting for application deployments..."
  $KUBECTL wait --for=condition=available deployment/ingest-gateway -n traffic-analysis --timeout=120s 2>/dev/null || warn "ingest-gateway not ready yet"
  $KUBECTL wait --for=condition=available deployment/auth-service -n traffic-analysis --timeout=120s 2>/dev/null || true
  $KUBECTL wait --for=condition=available deployment/alert-service -n traffic-analysis --timeout=120s 2>/dev/null || true
  $KUBECTL wait --for=condition=available deployment/asset-service -n traffic-analysis --timeout=120s 2>/dev/null || true
  $KUBECTL wait --for=condition=available deployment/rule-manager -n traffic-analysis --timeout=120s 2>/dev/null || true
  $KUBECTL wait --for=condition=available deployment/graph-service -n traffic-analysis --timeout=120s 2>/dev/null || true
  $KUBECTL wait --for=condition=available deployment/forensics-service -n traffic-analysis --timeout=120s 2>/dev/null || true

  # ---- Web UI ----
  info "Deploying Web UI..."
  $KUBECTL apply -f "$SCRIPT_DIR/applications/web-ui.yaml" 2>/dev/null || warn "Web UI YAML not found, skipping"
  reconcile_service_exposure_profile "$SCRIPT_DIR/applications"
}

show_status() {
  info "========================================="
  info "Deployment Status"
  info "========================================="
  echo ""
  for ns in middleware databases traffic-analysis flink gateway iam minio observability argo nacos streampark registry; do
    echo -e "${YELLOW}--- Namespace: $ns ---${NC}"
    $KUBECTL get pods -n "$ns" 2>/dev/null || echo "  (not found)"
    echo ""
  done
  echo -e "${YELLOW}--- Services ---${NC}"
  $KUBECTL get svc --all-namespaces 2>/dev/null | grep -E "NAMESPACE|gateway|middleware|databases|traffic-analysis|flink|iam|minio|argo|nacos|streampark|registry" || true
}

deploy_flink_jobs() {
  info "========================================="
  info "Submitting Flink Jobs"
  info "========================================="
  local FLINK_SCRIPTS="${SCRIPT_DIR}/../../java/flink-jobs/scripts"
  if [ ! -d "$FLINK_SCRIPTS" ]; then
    warn "Flink scripts not found at $FLINK_SCRIPTS, skipping"
    return
  fi
  # 确保 JAR 已构建
  cd "${SCRIPT_DIR}/../../java/flink-jobs"
  mvn -pl flink-session-job,flink-feature-job,flink-rule-job,flink-pcap-index-job,flink-cep-job,flink-behavior-job,flink-alert-generator-job -am package -DskipTests -q 2>/dev/null || warn "Some Flink JARs failed to build"
  cd "$SCRIPT_DIR"
  # 等待 Flink JobManager 就绪
  $KUBECTL wait --for=condition=ready pod -l app=flink,component=jobmanager -n flink --timeout=120s 2>/dev/null || warn "Flink JobManager not ready"
  # 按数据流顺序提交
  for job in submit-session-job submit-feature-job submit-pcap-index-job submit-rule-job submit-cep-job submit-behavior-job submit-alert-generator-job; do
    if [ -f "$FLINK_SCRIPTS/$job" ] && [ -s "$FLINK_SCRIPTS/$job" ]; then
      info "  Submitting $job..."
      bash "$FLINK_SCRIPTS/$job" 2>/dev/null || warn "  $job submission failed (may be OK if Flink not running locally)"
    fi
  done
  info "Flink jobs submitted."
}

clean() {
  warn "This will delete ALL traffic-analysis-platform resources!"
  read -p "Are you sure? (yes/no): " confirm
  if [ "$confirm" != "yes" ]; then
    info "Aborted."
    exit 0
  fi
  for ns in traffic-analysis flink observability middleware databases; do
    $KUBECTL delete namespace "$ns" --timeout=120s 2>/dev/null || true
  done
  info "All resources deleted."
}

# ---- Main ----
case "${1:-}" in
  secrets)
    ensure_secret_readiness
    ;;
  infra)
    deploy_infra
    ;;
  init)
    deploy_init
    ;;
  apps)
    deploy_apps
    ;;
  services)
    reconcile_service_exposure_profile "$SCRIPT_DIR/infrastructure" "$SCRIPT_DIR/applications"
    ;;
  flink)
    deploy_flink_jobs
    ;;
  status)
    show_status
    ;;
  clean)
    clean
    ;;
  *)
    info "========================================="
    info "Traffic Analysis Platform — Full Deployment"
    info "========================================="
    echo ""
	    info "External endpoint (NodePort via 10.0.5.8):"
	    info "  APISIX business gateway: 10.0.5.8:30180"
	    info "Infrastructure and management services are ClusterIP-only; use kubectl port-forward or an approved bastion path for operations."
    echo ""
    deploy_infra
    deploy_init
    deploy_apps
    deploy_flink_jobs
    echo ""
    show_status
    ;;
esac
