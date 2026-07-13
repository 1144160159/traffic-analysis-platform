#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-ha-readiness-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-ha-readiness-preflight}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
RESILIENCE_DIR="${RESILIENCE_DIR:-doc/02_acceptance/06-resilience}"
APISIX="${APISIX:-http://10.0.5.8:30180}"
MINIO_NAMESPACE="${MINIO_NAMESPACE:-minio}"
MINIO_POD="${MINIO_POD:-minio-0}"
MINIO_HEALTH_URL="${MINIO_HEALTH_URL:-http://localhost:9000/minio/health/live}"
EXPECTED_RUNNING_FLINK_JOBS="${EXPECTED_RUNNING_FLINK_JOBS:-9}"
EXPECTED_PG_REPLICAS="${EXPECTED_PG_REPLICAS:-2}"

REPORT="$LOG_DIR/live-ha-readiness-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-ha-readiness-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$RESILIENCE_DIR"
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

template_markers_in_file() {
  local path="$1"
  [[ -f "$path" ]] || return 1
  grep -Eiq 'review_required|review-template|bootstrap_review_required|template_review_required|formal_gate_note|do not rename|must be filled|TBD|not valid while any' "$path"
}

guard_formal_evidence_file() {
  local path="$1"
  local label
  label="$(basename "$path")"
  [[ -f "$path" ]] || return 0
  if template_markers_in_file "$path"; then
    json_log "integrity" "$label is not bootstrap or review template" "blocker" false "review_required" "$path still contains review-template/bootstrap markers; replace it with signed maintenance-window drill evidence" "existing-rto-rpo-evidence-files.txt"
  else
    json_log "integrity" "$label is not bootstrap or review template" "info" true "ok" "$path has no known bootstrap/template markers" "existing-rto-rpo-evidence-files.txt"
  fi
}

kafka_secure_admin_config() {
  cat <<'EOF'
PROPS=/tmp/ha-readiness-kafka-client.properties
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
need_cmd python3
need_cmd curl
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"
git diff --stat >"$LOG_DIR/git-diff-stat.txt" || true

cat >"$LOG_DIR/expected-ha-workloads.json" <<'JSON'
[
  {"namespace":"middleware","kind":"StatefulSet","name":"kafka","min_ready":3,"component":"kafka"},
  {"namespace":"middleware","kind":"StatefulSet","name":"clickhouse-keeper","min_ready":3,"component":"clickhouse"},
  {"namespace":"middleware","kind":"StatefulSet","name":"clickhouse-1","min_ready":1,"component":"clickhouse"},
  {"namespace":"middleware","kind":"StatefulSet","name":"clickhouse-2","min_ready":1,"component":"clickhouse"},
  {"namespace":"middleware","kind":"StatefulSet","name":"clickhouse-replica","min_ready":2,"component":"clickhouse"},
  {"namespace":"databases","kind":"StatefulSet","name":"postgres-primary","min_ready":1,"component":"postgres"},
  {"namespace":"databases","kind":"StatefulSet","name":"postgres-replica","min_ready":2,"component":"postgres"},
  {"namespace":"databases","kind":"StatefulSet","name":"redis-master","min_ready":1,"component":"redis"},
  {"namespace":"databases","kind":"StatefulSet","name":"redis-replica","min_ready":2,"component":"redis"},
  {"namespace":"databases","kind":"StatefulSet","name":"redis-sentinel","min_ready":3,"component":"redis"},
  {"namespace":"flink","kind":"StatefulSet","name":"flink-jobmanager","min_ready":1,"component":"flink"},
  {"namespace":"flink","kind":"StatefulSet","name":"flink-taskmanager","min_ready":2,"component":"flink"},
  {"namespace":"minio","kind":"StatefulSet","name":"minio","min_ready":4,"component":"minio"},
  {"namespace":"gateway","kind":"StatefulSet","name":"apisix","min_ready":2,"component":"gateway"},
  {"namespace":"traffic-analysis","kind":"Deployment","name":"alert-service","min_ready":1,"component":"traffic-analysis"},
  {"namespace":"traffic-analysis","kind":"Deployment","name":"ingest-gateway","min_ready":1,"component":"traffic-analysis"}
]
JSON

cat >"$LOG_DIR/expected-ha-pdbs.json" <<'JSON'
[
  {"namespace":"middleware","name":"kafka-pdb","component":"kafka"},
  {"namespace":"middleware","name":"clickhouse-keeper-pdb","component":"clickhouse"},
  {"namespace":"middleware","name":"clickhouse-pdb","component":"clickhouse"},
  {"namespace":"databases","name":"postgres-pdb","component":"postgres"},
  {"namespace":"databases","name":"redis-pdb","component":"redis"},
  {"namespace":"minio","name":"minio-pdb","component":"minio"}
]
JSON

cat >"$LOG_DIR/expected-ha-services.json" <<'JSON'
[
  {"namespace":"middleware","name":"kafka-bootstrap","component":"kafka"},
  {"namespace":"middleware","name":"clickhouse-1","component":"clickhouse"},
  {"namespace":"middleware","name":"clickhouse-2","component":"clickhouse"},
  {"namespace":"middleware","name":"clickhouse-headless","component":"clickhouse"},
  {"namespace":"databases","name":"postgres-primary","component":"postgres"},
  {"namespace":"databases","name":"postgres-replica","component":"postgres"},
  {"namespace":"databases","name":"redis-master","component":"redis"},
  {"namespace":"databases","name":"redis-sentinel","component":"redis"},
  {"namespace":"flink","name":"flink-jobmanager","component":"flink"},
  {"namespace":"minio","name":"minio","component":"minio"},
  {"namespace":"gateway","name":"apisix","component":"gateway"},
  {"namespace":"traffic-analysis","name":"alert-service","component":"traffic-analysis"},
  {"namespace":"traffic-analysis","name":"ingest-gateway","component":"traffic-analysis"}
]
JSON

kctl get deploy,sts,ds -A -o json >"$LOG_DIR/live-workloads.json"
jq -n \
  --argfile expected "$LOG_DIR/expected-ha-workloads.json" \
  --argfile live "$LOG_DIR/live-workloads.json" '
  $expected
  | map(. as $e
    | (first($live.items[]? | select(.metadata.namespace == $e.namespace and .kind == $e.kind and .metadata.name == $e.name)) // null) as $w
    | if $w == null then
        $e + {desired:null, ready:0, status:"missing"}
      else
        ($w.spec.replicas // $w.status.desiredNumberScheduled // ($e.min_ready // 1)) as $desired
        | ($w.status.readyReplicas // $w.status.numberReady // 0) as $ready
        | $e + {
            desired:$desired,
            ready:$ready,
            status:(if ($ready >= ($e.min_ready // $desired) and $ready >= $desired) then "ready" else "not_ready" end)
          }
      end
  )' >"$LOG_DIR/ha-workload-readiness.json"
WORKLOAD_BLOCKERS="$(jq '[.[] | select(.status != "ready")] | length' "$LOG_DIR/ha-workload-readiness.json")"
if [[ "$WORKLOAD_BLOCKERS" -eq 0 ]]; then
  json_log "k8s" "critical HA workloads ready" "info" true "ok" "$(jq 'length' "$LOG_DIR/ha-workload-readiness.json") workloads" "ha-workload-readiness.json"
else
  json_log "k8s" "critical HA workloads ready" "blocker" false "not_ready" "$WORKLOAD_BLOCKERS workloads missing or unready" "ha-workload-readiness.json"
fi

kctl get pdb -A -o json >"$LOG_DIR/live-pdbs.json"
jq -n \
  --argfile expected "$LOG_DIR/expected-ha-pdbs.json" \
  --argfile live "$LOG_DIR/live-pdbs.json" '
  $expected
  | map(. as $e
    | (first($live.items[]? | select(.metadata.namespace == $e.namespace and .metadata.name == $e.name)) // null) as $pdb
    | if $pdb == null then
        $e + {status:"missing", allowed_disruptions:null, current_healthy:null, desired_healthy:null}
      else
        $e + {
          status:"present",
          allowed_disruptions:($pdb.status.disruptionsAllowed // 0),
          current_healthy:($pdb.status.currentHealthy // 0),
          desired_healthy:($pdb.status.desiredHealthy // 0)
        }
      end
  )' >"$LOG_DIR/ha-pdb-readiness.json"
PDB_MISSING="$(jq '[.[] | select(.status == "missing")] | length' "$LOG_DIR/ha-pdb-readiness.json")"
PDB_ZERO_ALLOWED="$(jq '[.[] | select(.status == "present" and (.allowed_disruptions // 0) == 0)] | length' "$LOG_DIR/ha-pdb-readiness.json")"
if [[ "$PDB_MISSING" -gt 0 ]]; then
  json_log "k8s" "critical PDBs present" "blocker" false "missing" "$PDB_MISSING PDBs missing" "ha-pdb-readiness.json"
else
  json_log "k8s" "critical PDBs present" "info" true "ok" "$(jq 'length' "$LOG_DIR/ha-pdb-readiness.json") PDBs" "ha-pdb-readiness.json"
fi
if [[ "$PDB_ZERO_ALLOWED" -gt 0 ]]; then
  json_log "k8s" "controlled eviction budget available" "warn" false "zero_allowed" "$PDB_ZERO_ALLOWED PDBs currently allow 0 voluntary disruptions" "ha-pdb-readiness.json"
else
  json_log "k8s" "controlled eviction budget available" "info" true "ok" "all critical PDBs allow voluntary disruption" "ha-pdb-readiness.json"
fi

kctl get pvc -A -o json >"$LOG_DIR/live-pvcs.json"
jq '[
  .items[]
  | select(.metadata.namespace as $ns | ["middleware","databases","minio","flink"] | index($ns))
  | {
      namespace:.metadata.namespace,
      name:.metadata.name,
      status:.status.phase,
      storage_class:(.spec.storageClassName // ""),
      capacity:(.status.capacity.storage // "")
    }
]' "$LOG_DIR/live-pvcs.json" >"$LOG_DIR/ha-pvc-readiness.json"
PENDING_PVCS="$(jq '[.[] | select(.status != "Bound")] | length' "$LOG_DIR/ha-pvc-readiness.json")"
if [[ "$PENDING_PVCS" -eq 0 ]]; then
  json_log "k8s" "critical namespace PVCs bound" "info" true "ok" "$(jq 'length' "$LOG_DIR/ha-pvc-readiness.json") PVCs" "ha-pvc-readiness.json"
else
  json_log "k8s" "critical namespace PVCs bound" "warn" false "pending" "$PENDING_PVCS PVCs are not Bound" "ha-pvc-readiness.json"
fi

kctl get endpoints -A -o json >"$LOG_DIR/live-endpoints.json"
jq -n \
  --argfile expected "$LOG_DIR/expected-ha-services.json" \
  --argfile live "$LOG_DIR/live-endpoints.json" '
  $expected
  | map(. as $e
    | (first($live.items[]? | select(.metadata.namespace == $e.namespace and .metadata.name == $e.name)) // null) as $ep
    | ($ep.subsets // [] | map((.addresses // []) | length) | add // 0) as $addresses
    | $e + {
        addresses:$addresses,
        status:(if $ep == null then "missing" elif $addresses > 0 then "ready" else "empty" end)
      }
  )' >"$LOG_DIR/ha-endpoint-readiness.json"
ENDPOINT_BLOCKERS="$(jq '[.[] | select(.status != "ready")] | length' "$LOG_DIR/ha-endpoint-readiness.json")"
if [[ "$ENDPOINT_BLOCKERS" -eq 0 ]]; then
  json_log "k8s" "critical service endpoints populated" "info" true "ok" "$(jq 'length' "$LOG_DIR/ha-endpoint-readiness.json") services" "ha-endpoint-readiness.json"
else
  json_log "k8s" "critical service endpoints populated" "blocker" false "empty_or_missing" "$ENDPOINT_BLOCKERS services have no endpoints" "ha-endpoint-readiness.json"
fi

set +e
kctl -n middleware exec kafka-0 -- bash -lc "set -euo pipefail
$(kafka_secure_admin_config)
export KAFKA_HEAP_OPTS=\"\${KAFKA_HEAP_OPTS:--Xms128m -Xmx512m}\"
/opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config \"\$PROPS\" --describe
rm -f \"\$PROPS\"" \
  >"$LOG_DIR/kafka-topics-describe.txt" 2>"$LOG_DIR/kafka-topics-describe.err"
KAFKA_DESCRIBE_RC=$?
set -e
if [[ "$KAFKA_DESCRIBE_RC" -eq 0 ]]; then
  python3 - "$LOG_DIR/kafka-topics-describe.txt" "$LOG_DIR/kafka-topic-health.json" <<'PY'
import json
import re
import sys

source, target = sys.argv[1], sys.argv[2]
topics = {}
partitions = []
under_replicated = []
offline_leaders = []

def get_int(pattern, text, default=None):
    match = re.search(pattern, text)
    if not match:
        return default
    return int(match.group(1))

for line in open(source, encoding="utf-8", errors="replace"):
    line = line.strip()
    if not line:
        continue
    topic_match = re.search(r"Topic:\s+([^\s]+)", line)
    if not topic_match:
        continue
    topic = topic_match.group(1)
    if "Partition:" not in line:
        topics[topic] = {
            "topic": topic,
            "partition_count": get_int(r"PartitionCount:\s+(\d+)", line, 0),
            "replication_factor": get_int(r"ReplicationFactor:\s+(\d+)", line, 0),
        }
        continue
    partition = get_int(r"Partition:\s+(\d+)", line, -1)
    leader = get_int(r"Leader:\s+(-?\d+)", line, -1)
    replicas_match = re.search(r"Replicas:\s+([0-9,\s-]+)", line)
    isr_match = re.search(r"Isr:\s+([0-9,\s-]+)", line)
    replicas = [x.strip() for x in replicas_match.group(1).split(",") if x.strip()] if replicas_match else []
    isr = [x.strip() for x in isr_match.group(1).split(",") if x.strip()] if isr_match else []
    record = {
        "topic": topic,
        "partition": partition,
        "leader": leader,
        "replicas": replicas,
        "isr": isr,
        "replication_factor": len(replicas),
        "isr_count": len(isr),
    }
    partitions.append(record)
    if leader < 0:
        offline_leaders.append(record)
    if len(isr) < len(replicas):
        under_replicated.append(record)

payload = {
    "topics": list(topics.values()),
    "topic_count": len(topics),
    "partition_count": len(partitions),
    "under_replicated_count": len(under_replicated),
    "offline_leader_count": len(offline_leaders),
    "under_replicated": under_replicated,
    "offline_leaders": offline_leaders,
}
with open(target, "w", encoding="utf-8") as fh:
    json.dump(payload, fh, indent=2, ensure_ascii=True)
PY
  KAFKA_TOPIC_COUNT="$(jq '.topic_count' "$LOG_DIR/kafka-topic-health.json")"
  KAFKA_UNDER_REPLICATED="$(jq '.under_replicated_count' "$LOG_DIR/kafka-topic-health.json")"
  KAFKA_OFFLINE_LEADERS="$(jq '.offline_leader_count' "$LOG_DIR/kafka-topic-health.json")"
  if [[ "$KAFKA_UNDER_REPLICATED" -eq 0 && "$KAFKA_OFFLINE_LEADERS" -eq 0 ]]; then
    json_log "kafka" "Kafka leaders and ISR healthy" "info" true "ok" "$KAFKA_TOPIC_COUNT topics" "kafka-topic-health.json"
  else
    json_log "kafka" "Kafka leaders and ISR healthy" "blocker" false "degraded" "under_replicated=$KAFKA_UNDER_REPLICATED offline_leaders=$KAFKA_OFFLINE_LEADERS" "kafka-topic-health.json"
  fi
else
  jq -n '{topics:[], topic_count:0, partition_count:0, under_replicated_count:null, offline_leader_count:null}' >"$LOG_DIR/kafka-topic-health.json"
  json_log "kafka" "Kafka leaders and ISR healthy" "blocker" false "rc=$KAFKA_DESCRIBE_RC" "$(trim_file "$LOG_DIR/kafka-topics-describe.err")" "kafka-topics-describe.err"
fi

set +e
kctl -n flink exec flink-jobmanager-0 -- curl -fsS http://localhost:8081/jobs/overview \
  >"$LOG_DIR/flink-jobs-overview.json" 2>"$LOG_DIR/flink-jobs-overview.err"
FLINK_OVERVIEW_RC=$?
set -e
if [[ "$FLINK_OVERVIEW_RC" -eq 0 ]]; then
  RUNNING_FLINK_JOBS="$(jq '[.jobs[]? | select(.state == "RUNNING")] | length' "$LOG_DIR/flink-jobs-overview.json")"
  jq -r '.jobs[]? | select(.state == "RUNNING") | [.jid, .name] | @tsv' "$LOG_DIR/flink-jobs-overview.json" >"$LOG_DIR/flink-running-jobs.tsv"
  : >"$LOG_DIR/flink-running-job-health.ndjson"
  while IFS=$'\t' read -r jid job_name; do
    [[ -n "$jid" ]] || continue
    cp_file="$LOG_DIR/flink-checkpoints-$jid.json"
    ex_file="$LOG_DIR/flink-exceptions-$jid.json"
    set +e
    kctl -n flink exec flink-jobmanager-0 -- curl -fsS "http://localhost:8081/jobs/$jid/checkpoints" >"$cp_file" 2>"$cp_file.err"
    cp_rc=$?
    kctl -n flink exec flink-jobmanager-0 -- curl -fsS "http://localhost:8081/jobs/$jid/exceptions" >"$ex_file" 2>"$ex_file.err"
    ex_rc=$?
    set -e
    if [[ "$cp_rc" -ne 0 ]]; then
      jq -n --arg jid "$jid" --arg name "$job_name" --argjson checkpoint_rc "$cp_rc" --argjson exception_rc "$ex_rc" \
        '{jid:$jid, name:$name, checkpoint_rc:$checkpoint_rc, exception_rc:$exception_rc, status:"checkpoint_unreachable"}' >>"$LOG_DIR/flink-running-job-health.ndjson"
      continue
    fi
    if [[ "$ex_rc" -ne 0 ]]; then
      jq -n --arg jid "$jid" --arg name "$job_name" --argjson checkpoint_rc "$cp_rc" --argjson exception_rc "$ex_rc" \
        '{jid:$jid, name:$name, checkpoint_rc:$checkpoint_rc, exception_rc:$exception_rc, status:"exceptions_unreachable"}' >>"$LOG_DIR/flink-running-job-health.ndjson"
      continue
    fi
    jq -nc \
      --arg jid "$jid" \
      --arg name "$job_name" \
      --slurpfile cp "$cp_file" \
      --slurpfile ex "$ex_file" '
      ($cp[0].latest.completed // null) as $completed
      | ($ex[0]["root-exception"] // null) as $root_exception
      | ($ex[0]["all-exceptions"] // []) as $all_exceptions
      | {
          jid:$jid,
          name:$name,
          checkpoint_rc:0,
          exception_rc:0,
          completed_checkpoints:($cp[0].counts.completed // 0),
          failed_checkpoints:($cp[0].counts.failed // 0),
          latest_completed_id:($completed.id // null),
          latest_completed_duration_ms:($completed.end_to_end_duration // null),
          root_exception:$root_exception,
          exception_count:($all_exceptions | length),
          status:(if (($cp[0].counts.completed // 0) > 0 and $root_exception == null and (($all_exceptions | length) == 0)) then "healthy" else "degraded" end)
        }' >>"$LOG_DIR/flink-running-job-health.ndjson"
  done <"$LOG_DIR/flink-running-jobs.tsv"
  jq -s . "$LOG_DIR/flink-running-job-health.ndjson" >"$LOG_DIR/flink-running-job-health.json"
  FLINK_DEGRADED="$(jq '[.[] | select(.status != "healthy")] | length' "$LOG_DIR/flink-running-job-health.json")"
  if [[ "$RUNNING_FLINK_JOBS" -ge "$EXPECTED_RUNNING_FLINK_JOBS" && "$FLINK_DEGRADED" -eq 0 ]]; then
    json_log "flink" "Flink running jobs checkpointing without exceptions" "info" true "ok" "running=$RUNNING_FLINK_JOBS expected=$EXPECTED_RUNNING_FLINK_JOBS" "flink-running-job-health.json"
  else
    json_log "flink" "Flink running jobs checkpointing without exceptions" "blocker" false "degraded" "running=$RUNNING_FLINK_JOBS expected=$EXPECTED_RUNNING_FLINK_JOBS degraded=$FLINK_DEGRADED" "flink-running-job-health.json"
  fi
else
  jq -n '[]' >"$LOG_DIR/flink-running-job-health.json"
  RUNNING_FLINK_JOBS=0
  FLINK_DEGRADED=0
  json_log "flink" "Flink running jobs checkpointing without exceptions" "blocker" false "rc=$FLINK_OVERVIEW_RC" "$(trim_file "$LOG_DIR/flink-jobs-overview.err")" "flink-jobs-overview.err"
fi

set +e
kctl -n middleware exec clickhouse-1-0 -c clickhouse -- clickhouse-client --query \
  "SELECT database, table, is_readonly, absolute_delay, queue_size, inserts_in_queue, log_max_index - log_pointer AS log_lag FROM system.replicas WHERE database='traffic' ORDER BY table FORMAT JSON" \
  >"$LOG_DIR/clickhouse-replication.json" 2>"$LOG_DIR/clickhouse-replication.err"
CLICKHOUSE_RC=$?
set -e
if [[ "$CLICKHOUSE_RC" -eq 0 ]]; then
  CLICKHOUSE_REPLICA_ROWS="$(jq '.rows' "$LOG_DIR/clickhouse-replication.json")"
  CLICKHOUSE_REPLICA_BLOCKERS="$(jq '[.data[]? | select(((.is_readonly | tonumber) != 0) or ((.absolute_delay | tonumber) > 60) or ((.queue_size | tonumber) > 100))] | length' "$LOG_DIR/clickhouse-replication.json")"
  if [[ "$CLICKHOUSE_REPLICA_ROWS" -gt 0 && "$CLICKHOUSE_REPLICA_BLOCKERS" -eq 0 ]]; then
    json_log "clickhouse" "ClickHouse replicated tables healthy" "info" true "ok" "$CLICKHOUSE_REPLICA_ROWS replicated tables" "clickhouse-replication.json"
  else
    json_log "clickhouse" "ClickHouse replicated tables healthy" "blocker" false "degraded" "rows=$CLICKHOUSE_REPLICA_ROWS blockers=$CLICKHOUSE_REPLICA_BLOCKERS" "clickhouse-replication.json"
  fi
else
  jq -n '{rows:0,data:[]}' >"$LOG_DIR/clickhouse-replication.json"
  CLICKHOUSE_REPLICA_ROWS=0
  CLICKHOUSE_REPLICA_BLOCKERS=1
  json_log "clickhouse" "ClickHouse replicated tables healthy" "blocker" false "rc=$CLICKHOUSE_RC" "$(trim_file "$LOG_DIR/clickhouse-replication.err")" "clickhouse-replication.err"
fi

set +e
kctl -n databases exec postgres-primary-0 -- sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -U postgres -d traffic_platform -tAc "select count(*) from pg_stat_replication;"' \
  >"$LOG_DIR/postgres-replication-count.txt" 2>"$LOG_DIR/postgres-replication-count.err"
PG_COUNT_RC=$?
kctl -n databases exec postgres-primary-0 -- sh -c \
  'PGPASSWORD="$POSTGRES_PASSWORD" psql -U postgres -d traffic_platform -F "," -A -c "select application_name, client_addr, state, sync_state, coalesce(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn),0)::bigint as replay_lag_bytes from pg_stat_replication order by application_name;"' \
  >"$LOG_DIR/postgres-replication.csv" 2>"$LOG_DIR/postgres-replication.err"
PG_DETAIL_RC=$?
set -e
if [[ "$PG_COUNT_RC" -eq 0 ]]; then
  PG_REPLICA_COUNT="$(tr -d '[:space:]' <"$LOG_DIR/postgres-replication-count.txt")"
  if [[ "$PG_REPLICA_COUNT" =~ ^[0-9]+$ && "$PG_REPLICA_COUNT" -ge "$EXPECTED_PG_REPLICAS" ]]; then
    json_log "postgres" "PostgreSQL replicas streaming" "info" true "ok" "replicas=$PG_REPLICA_COUNT expected=$EXPECTED_PG_REPLICAS detail_rc=$PG_DETAIL_RC" "postgres-replication.csv"
  else
    json_log "postgres" "PostgreSQL replicas streaming" "blocker" false "degraded" "replicas=${PG_REPLICA_COUNT:-unknown} expected=$EXPECTED_PG_REPLICAS detail_rc=$PG_DETAIL_RC" "postgres-replication.csv"
  fi
else
  PG_REPLICA_COUNT=0
  json_log "postgres" "PostgreSQL replicas streaming" "blocker" false "rc=$PG_COUNT_RC" "$(trim_file "$LOG_DIR/postgres-replication-count.err")" "postgres-replication-count.err"
fi

set +e
kctl -n databases exec redis-sentinel-0 -- redis-cli -p 26379 SENTINEL get-master-addr-by-name mymaster \
  >"$LOG_DIR/redis-sentinel-master.txt" 2>"$LOG_DIR/redis-sentinel-master.err"
REDIS_SENTINEL_RC=$?
set -e
if [[ "$REDIS_SENTINEL_RC" -eq 0 && -s "$LOG_DIR/redis-sentinel-master.txt" ]]; then
  json_log "redis" "Redis Sentinel master discoverable" "info" true "ok" "$(tr '\n' ' ' <"$LOG_DIR/redis-sentinel-master.txt")" "redis-sentinel-master.txt"
else
  json_log "redis" "Redis Sentinel master discoverable" "blocker" false "rc=$REDIS_SENTINEL_RC" "$(trim_file "$LOG_DIR/redis-sentinel-master.err")" "redis-sentinel-master.err"
fi

set +e
kctl -n "$MINIO_NAMESPACE" exec "$MINIO_POD" -- curl -fsS -m 10 "$MINIO_HEALTH_URL" \
  >"$LOG_DIR/minio-health-body.txt" 2>"$LOG_DIR/minio-health.err"
MINIO_CURL_RC=$?
set -e
if [[ "$MINIO_CURL_RC" -eq 0 ]]; then
  json_log "minio" "MinIO live health endpoint reachable" "info" true "ok" "$MINIO_NAMESPACE/$MINIO_POD $MINIO_HEALTH_URL" "minio-health-body.txt"
else
  json_log "minio" "MinIO live health endpoint reachable" "blocker" false "rc=$MINIO_CURL_RC" "$(trim_file "$LOG_DIR/minio-health.err")" "minio-health.err"
fi

set +e
curl --noproxy '*' -sS -m 10 -o "$LOG_DIR/apisix-root.txt" -w '%{http_code}' "$APISIX/" >"$LOG_DIR/apisix-root-code.txt" 2>"$LOG_DIR/apisix-root.err"
APISIX_CURL_RC=$?
set -e
APISIX_HTTP_CODE="$(cat "$LOG_DIR/apisix-root-code.txt" 2>/dev/null || true)"
if [[ "$APISIX_CURL_RC" -eq 0 && "$APISIX_HTTP_CODE" =~ ^(200|301|302|404)$ ]]; then
  json_log "gateway" "APISIX business NodePort reachable" "info" true "ok" "$APISIX http=$APISIX_HTTP_CODE" "apisix-root.txt"
else
  json_log "gateway" "APISIX business NodePort reachable" "blocker" false "http=${APISIX_HTTP_CODE:-none} rc=$APISIX_CURL_RC" "$(trim_file "$LOG_DIR/apisix-root.err")" "apisix-root.err"
fi

FORMAL_HA_EVIDENCE_FILES=(
  "$RESILIENCE_DIR/kafka-failover.md"
  "$RESILIENCE_DIR/flink-failover.md"
  "$RESILIENCE_DIR/clickhouse-failover.md"
  "$RESILIENCE_DIR/postgres-failover.md"
  "$RESILIENCE_DIR/minio-failover.md"
  "$RESILIENCE_DIR/ha-rto-rpo-latest.json"
)
: >"$LOG_DIR/existing-rto-rpo-evidence-files.txt"
MISSING_RTO_RPO_EVIDENCE=()
for evidence_file in "${FORMAL_HA_EVIDENCE_FILES[@]}"; do
  if [[ -f "$evidence_file" ]]; then
    printf '%s\n' "$evidence_file" >>"$LOG_DIR/existing-rto-rpo-evidence-files.txt"
  else
    MISSING_RTO_RPO_EVIDENCE+=("$(basename "$evidence_file")")
  fi
done
RTO_RPO_EVIDENCE_COUNT="$(wc -l <"$LOG_DIR/existing-rto-rpo-evidence-files.txt" | tr -d ' ')"
REQUIRED_RTO_RPO_EVIDENCE_COUNT="${#FORMAL_HA_EVIDENCE_FILES[@]}"
if [[ "$RTO_RPO_EVIDENCE_COUNT" -eq "$REQUIRED_RTO_RPO_EVIDENCE_COUNT" ]]; then
  json_log "acceptance" "destructive RTO/RPO drill reports present" "info" true "ok" "$RTO_RPO_EVIDENCE_COUNT/$REQUIRED_RTO_RPO_EVIDENCE_COUNT formal files" "existing-rto-rpo-evidence-files.txt"
else
  json_log "acceptance" "destructive RTO/RPO drill reports present" "blocker" false "missing" "present=$RTO_RPO_EVIDENCE_COUNT required=$REQUIRED_RTO_RPO_EVIDENCE_COUNT missing=${MISSING_RTO_RPO_EVIDENCE[*]}" "existing-rto-rpo-evidence-files.txt"
fi
while IFS= read -r evidence_file; do
  guard_formal_evidence_file "$evidence_file"
done <"$LOG_DIR/existing-rto-rpo-evidence-files.txt"
json_log "integrity" "bootstrap and review-template artifacts are blocked from formal HA pass" "info" true "ok" "formal HA RTO/RPO artifacts are scanned for review-template/TBD markers before this gate can pass" "tests/chaos/live_ha_readiness_preflight.sh"

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
  --argjson total "$TOTAL" \
  --argjson passed "$PASSED" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --argjson running_flink_jobs "${RUNNING_FLINK_JOBS:-0}" \
  --argjson expected_running_flink_jobs "$EXPECTED_RUNNING_FLINK_JOBS" \
  --argjson kafka_topic_count "${KAFKA_TOPIC_COUNT:-0}" \
  --argjson clickhouse_replica_rows "${CLICKHOUSE_REPLICA_ROWS:-0}" \
  --argjson postgres_replica_count "${PG_REPLICA_COUNT:-0}" \
  --argjson rto_rpo_evidence_count "$RTO_RPO_EVIDENCE_COUNT" \
  '{
    run_id:$run_id,
    result:$result,
    report:$report,
    local_report:$local_report,
    total:$total,
    passed:$passed,
    blockers:$blockers,
    warnings:$warnings,
    running_flink_jobs:$running_flink_jobs,
    expected_running_flink_jobs:$expected_running_flink_jobs,
    kafka_topic_count:$kafka_topic_count,
    clickhouse_replica_rows:$clickhouse_replica_rows,
    postgres_replica_count:$postgres_replica_count,
    rto_rpo_evidence_count:$rto_rpo_evidence_count,
    checks:.
  }' "$REPORT" >"$SUMMARY"

cat >"$LOCAL_REPORT" <<EOF
# HA Readiness Preflight

Run: \`$RUN_ID\`

Result: \`$RESULT\`

This is a non-destructive live preflight for GATE-P0-08. It reads Kubernetes, Kafka, Flink, ClickHouse, PostgreSQL, Redis Sentinel, MinIO, and APISIX state. It does not delete pods, scale workloads, force failover, write traffic records, or rotate storage.

## Summary

| Metric | Count |
|---|---:|
| Checks | $TOTAL |
| Passed | $PASSED |
| Blockers | $BLOCKERS |
| Warnings | $WARNINGS |
| Running Flink jobs | ${RUNNING_FLINK_JOBS:-0} |
| Kafka topics inspected | ${KAFKA_TOPIC_COUNT:-0} |
| ClickHouse replicated tables | ${CLICKHOUSE_REPLICA_ROWS:-0} |
| PostgreSQL streaming replicas | ${PG_REPLICA_COUNT:-0} |
| RTO/RPO drill evidence files | $RTO_RPO_EVIDENCE_COUNT |

## Key Artifacts

- \`$SUMMARY\`
- \`$REPORT\`
- \`ha-workload-readiness.json\`
- \`ha-pdb-readiness.json\`
- \`ha-pvc-readiness.json\`
- \`ha-endpoint-readiness.json\`
- \`kafka-topic-health.json\`
- \`flink-running-job-health.json\`
- \`clickhouse-replication.json\`
- \`postgres-replication.csv\`
- \`redis-sentinel-master.txt\`
- \`minio-health-body.txt\`

## Interpretation

This preflight can prove whether the live cluster is ready for a controlled HA drill. GATE-P0-08 remains blocked until the destructive Kafka, Flink, ClickHouse, PostgreSQL, and MinIO failover drills are run in a maintenance window and produce RTO/RPO plus data-consistency reports.
EOF

cp "$SUMMARY" "$RESILIENCE_DIR/ha-readiness-preflight-latest.json"
cp "$LOCAL_REPORT" "$RESILIENCE_DIR/ha-readiness-preflight-latest.md"
cp "$LOG_DIR/kafka-topic-health.json" "$RESILIENCE_DIR/kafka-topic-health-latest.json"
cp "$LOG_DIR/flink-running-job-health.json" "$RESILIENCE_DIR/flink-running-job-health-latest.json"
cp "$LOG_DIR/clickhouse-replication.json" "$RESILIENCE_DIR/clickhouse-replication-latest.json"
cp "$LOG_DIR/ha-workload-readiness.json" "$RESILIENCE_DIR/ha-workload-readiness-latest.json"

cat "$SUMMARY"

if [[ "$BLOCKERS" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
