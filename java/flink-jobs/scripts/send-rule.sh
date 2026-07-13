#!/usr/bin/env bash
# 发送单条规则到 Kafka

set -euo pipefail

KAFKA_BROKERS="${KAFKA_BROKERS:-localhost:9092}"
TOPIC="${TOPIC:-rule.updates}"

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <rule-json-file>"
    echo "Example: $0 sample-rules/blacklist-rule.json"
    exit 1
fi

RULE_FILE="$1"

if [[ ! -f "${RULE_FILE}" ]]; then
    echo "Error: File not found: ${RULE_FILE}"
    exit 1
fi

echo "Sending rule from ${RULE_FILE} to ${TOPIC}..."

# 使用 kafka-console-producer 或 kcat
if command -v kcat &> /dev/null; then
    cat "${RULE_FILE}" | kcat -b "${KAFKA_BROKERS}" -t "${TOPIC}" -P
elif command -v kafka-console-producer.sh &> /dev/null; then
    cat "${RULE_FILE}" | kafka-console-producer.sh \
        --broker-list "${KAFKA_BROKERS}" \
        --topic "${TOPIC}"
else
    echo "Error: Neither kcat nor kafka-console-producer.sh found"
    exit 1
fi

echo "Rule sent successfully!"