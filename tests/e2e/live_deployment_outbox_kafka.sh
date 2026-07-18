#!/usr/bin/env bash
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
KAFKA_NAMESPACE="${KAFKA_NAMESPACE:-middleware}"
KAFKA_POD="${KAFKA_POD:-kafka-0}"
LATEST_STATE_MACHINE="${LATEST_STATE_MACHINE:-doc/02_acceptance/02-regression/deployment-state-machine-latest.json}"
ACCEPTANCE_DIR="${ACCEPTANCE_DIR:-doc/02_acceptance/02-regression}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-deployment-outbox-kafka}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
SUMMARY="$LOG_DIR/deployment-outbox-kafka-summary.json"
LATEST="$ACCEPTANCE_DIR/deployment-outbox-kafka-latest.json"

mkdir -p "$LOG_DIR" "$ACCEPTANCE_DIR"

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

make_uuid() {
  tr -d '\n' </proc/sys/kernel/random/uuid
}

PG_PASSWORD="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.PG_PASSWORD}' | base64 -d)"
DEPLOYMENT_ID="$(jq -er '.deployment_id' "$LATEST_STATE_MACHINE")"
DEAD_EVENT_ID="$(make_uuid)"
FOLLOWUP_EVENT_ID="$(make_uuid)"
LEASE_EVENT_ID="$(make_uuid)"

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql" 2>/dev/null
}

cleanup() {
  psql_exec "DELETE FROM deployment_outbox WHERE event_id IN ('$DEAD_EVENT_ID','$FOLLOWUP_EVENT_ID','$LEASE_EVENT_ID');" >/dev/null || true
}
trap cleanup EXIT

psql_exec "
WITH source AS (
  SELECT deployment_id, tenant_id, topic, partition_key, payload
  FROM deployment_outbox
  WHERE deployment_id = '$DEPLOYMENT_ID'
  ORDER BY id DESC
  LIMIT 1
), probe(event_id, action, row_status, attempt_count, locked_at, locked_by, last_error) AS (
  VALUES
    ('$DEAD_EVENT_ID', 'fault_probe_dead', 'dead', 10, NULL::timestamptz, NULL::text, 'simulated poison predecessor'),
    ('$FOLLOWUP_EVENT_ID', 'fault_probe_followup', 'pending', 0, NULL::timestamptz, NULL::text, ''),
    ('$LEASE_EVENT_ID', 'fault_probe_expired_lease', 'processing', 1, now() - interval '31 seconds', 'abandoned-probe-worker', '')
)
INSERT INTO deployment_outbox (
  event_id, deployment_id, tenant_id, event_type, schema_version, topic, partition_key,
  payload, occurred_at, status, attempt_count, available_at, locked_at, locked_by,
  last_error, created_at, updated_at
)
SELECT
  probe.event_id, source.deployment_id, source.tenant_id, probe.action, 1, source.topic, source.partition_key,
  jsonb_set(
    jsonb_set(
      jsonb_set(source.payload, '{event_id}', to_jsonb(probe.event_id)),
      '{action}', to_jsonb(probe.action)
    ),
    '{occurred_at}', to_jsonb(now())
  ),
  now(), probe.row_status, probe.attempt_count, now(), probe.locked_at, probe.locked_by,
  probe.last_error, now(), now()
FROM source CROSS JOIN probe;
" >/dev/null

OUTBOX_STATUS=""
for _ in $(seq 1 25); do
  OUTBOX_STATUS="$(psql_exec "
    SELECT string_agg(event_id || '=' || status || ':' || attempt_count::text, ',' ORDER BY id)
    FROM deployment_outbox
    WHERE event_id IN ('$DEAD_EVENT_ID','$FOLLOWUP_EVENT_ID','$LEASE_EVENT_ID');
  ")"
  if [[ "$OUTBOX_STATUS" == *"$DEAD_EVENT_ID=dead:10"* && "$OUTBOX_STATUS" == *"$FOLLOWUP_EVENT_ID=published:"* && "$OUTBOX_STATUS" == *"$LEASE_EVENT_ID=published:"* ]]; then
    break
  fi
  sleep 2
done

OUTBOX_ROWS="$LOG_DIR/outbox-rows.json"
psql_exec "
  SELECT COALESCE(jsonb_agg(jsonb_build_object(
    'event_id', event_id,
    'event_type', event_type,
    'status', status,
    'attempt_count', attempt_count,
    'last_error', last_error,
    'payload_event_id', payload->>'event_id',
    'payload_action', payload->>'action',
    'schema_version', payload->>'schema_version'
  ) ORDER BY id), '[]'::jsonb)
  FROM deployment_outbox
  WHERE event_id IN ('$DEAD_EVENT_ID','$FOLLOWUP_EVENT_ID','$LEASE_EVENT_ID');
" | jq . >"$OUTBOX_ROWS"

KAFKA_RAW="$LOG_DIR/kafka-deployment-events.ndjson"
kctl -n "$KAFKA_NAMESPACE" exec "$KAFKA_POD" -- bash -lc '
  props=/tmp/deployment-outbox-probe-consumer.properties
  printf "%s\n" \
    "security.protocol=SASL_SSL" \
    "sasl.mechanism=SCRAM-SHA-512" \
    "sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username=\"${KAFKA_INTER_BROKER_USERNAME}\" password=\"${KAFKA_INTER_BROKER_PASSWORD}\";" \
    "ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12" \
    "ssl.truststore.type=PKCS12" \
    "ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}" >"$props"
  /opt/kafka/bin/kafka-console-consumer.sh \
    --bootstrap-server kafka-bootstrap.middleware.svc:9092 \
    --consumer.config "$props" \
    --topic deployment.events.v1 \
    --from-beginning \
    --timeout-ms 15000
' 2>"$LOG_DIR/kafka-consumer.stderr" >"$KAFKA_RAW" || true

KAFKA_EVENTS="$LOG_DIR/kafka-probe-events.json"
jq -s \
  --arg followup "$FOLLOWUP_EVENT_ID" \
  --arg lease "$LEASE_EVENT_ID" \
  '[.[] | select(.event_id == $followup or .event_id == $lease)]' \
  "$KAFKA_RAW" >"$KAFKA_EVENTS"

OUTBOX_OK="$(jq -e \
  --arg dead "$DEAD_EVENT_ID" \
  --arg followup "$FOLLOWUP_EVENT_ID" \
  --arg lease "$LEASE_EVENT_ID" '
  (map(select(.event_id == $dead and .status == "dead" and .attempt_count == 10)) | length) == 1 and
  (map(select(.event_id == $followup and .status == "published" and .payload_event_id == $followup and .payload_action == "fault_probe_followup" and .schema_version == "1")) | length) == 1 and
  (map(select(.event_id == $lease and .status == "published" and .attempt_count >= 2 and .payload_event_id == $lease and .payload_action == "fault_probe_expired_lease" and .schema_version == "1")) | length) == 1
  ' "$OUTBOX_ROWS" >/dev/null && printf true || printf false)"

KAFKA_OK="$(jq -e \
  --arg followup "$FOLLOWUP_EVENT_ID" \
  --arg lease "$LEASE_EVENT_ID" '
  (map(select(.event_id == $followup and .action == "fault_probe_followup" and .schema_version == 1 and .event_type == "deployment_event")) | length) == 1 and
  (map(select(.event_id == $lease and .action == "fault_probe_expired_lease" and .schema_version == 1 and .event_type == "deployment_event")) | length) == 1
  ' "$KAFKA_EVENTS" >/dev/null && printf true || printf false)"

jq -n \
  --arg run_id "$RUN_ID" \
  --arg generated_at "$(date -Iseconds)" \
  --arg deployment_id "$DEPLOYMENT_ID" \
  --arg dead_event_id "$DEAD_EVENT_ID" \
  --arg followup_event_id "$FOLLOWUP_EVENT_ID" \
  --arg lease_event_id "$LEASE_EVENT_ID" \
  --arg outbox_status "$OUTBOX_STATUS" \
  --argjson outbox_ok "$OUTBOX_OK" \
  --argjson kafka_ok "$KAFKA_OK" \
  --slurpfile outbox_rows "$OUTBOX_ROWS" \
  --slurpfile kafka_events "$KAFKA_EVENTS" '
  {
    run_id: $run_id,
    generated_at: $generated_at,
    result: (if $outbox_ok and $kafka_ok then "passed" else "failed" end),
    deployment_id: $deployment_id,
    probes: {
      dead_predecessor: $dead_event_id,
      publish_after_dead: $followup_event_id,
      expired_lease: $lease_event_id
    },
    checks: {
      dead_predecessor_does_not_block: $outbox_ok,
      expired_processing_lease_recovers: $outbox_ok,
      kafka_actual_consume_schema_v1: $kafka_ok,
      stable_event_id_end_to_end: $kafka_ok
    },
    observed_status: $outbox_status,
    outbox_rows: $outbox_rows[0],
    kafka_events: $kafka_events[0]
  }
  ' >"$SUMMARY"

cp "$SUMMARY" "$LATEST"
jq . "$SUMMARY"

if [[ "$OUTBOX_OK" != "true" || "$KAFKA_OK" != "true" ]]; then
  exit 1
fi
