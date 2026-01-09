#!/bin/bash
# scripts/deploy/init-kafka-topics.sh

KAFKA_BOOTSTRAP="kafka-0.kafka:9092"

# Alert 相关 Topics
kafka-topics.sh --bootstrap-server $KAFKA_BOOTSTRAP --create --if-not-exists \
  --topic detections.v1 \
  --partitions 6 \
  --replication-factor 2 \
  --config retention.ms=2592000000  # 30天

kafka-topics.sh --bootstrap-server $KAFKA_BOOTSTRAP --create --if-not-exists \
  --topic alerts.v1 \
  --partitions 6 \
  --replication-factor 2 \
  --config retention.ms=2592000000

kafka-topics.sh --bootstrap-server $KAFKA_BOOTSTRAP --create --if-not-exists \
  --topic audit.logs \
  --partitions 3 \
  --replication-factor 2 \
  --config retention.ms=7776000000  # 90天