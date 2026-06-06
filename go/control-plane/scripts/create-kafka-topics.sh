#!/usr/bin/env bash
set -euo pipefail

KAFKA_CONTAINER=${KAFKA_CONTAINER:-ingest-kafka}
BOOTSTRAP_SERVER=${BOOTSTRAP_SERVER:-localhost:9092}

TOPICS=(
  "flow.events.v1:3:1"
  "session.events.v1:3:1"
  "pcap.index.v1:1:1"
  "dlq.ingest-gateway:1:1"
)

log() {
  echo "[create-topics] $1"
}

if ! docker ps --format '{{.Names}}' | grep -q "^${KAFKA_CONTAINER}$"; then
  log "Kafka container '${KAFKA_CONTAINER}' not found. Set KAFKA_CONTAINER or start Kafka."
  exit 1
fi

log "Using Kafka container: ${KAFKA_CONTAINER}"
log "Bootstrap server: ${BOOTSTRAP_SERVER}"

for entry in "${TOPICS[@]}"; do
  IFS=":" read -r topic partitions replication <<< "${entry}"
  log "Creating topic: ${topic} (partitions=${partitions}, replication=${replication})"
  docker exec -it "${KAFKA_CONTAINER}" \
    kafka-topics --bootstrap-server "${BOOTSTRAP_SERVER}" \
    --create --if-not-exists \
    --topic "${topic}" \
    --partitions "${partitions}" \
    --replication-factor "${replication}"
  echo ""
done

log "All topics created (if not exists)."

docker exec -it "${KAFKA_CONTAINER}" \
  kafka-topics --bootstrap-server "${BOOTSTRAP_SERVER}" --list
