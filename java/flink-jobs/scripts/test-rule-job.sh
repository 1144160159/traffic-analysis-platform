#!/bin/bash
# test-rule-job.sh - Integration test for Rule Job

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Rule Job Integration Test${NC}"
echo -e "${GREEN}========================================${NC}"

# Configuration
KAFKA_BROKERS=${KAFKA_BROKERS:-localhost:9092}
RULE_TOPIC="rule.updates"
FEATURE_TOPIC="feature.stat.v1"
OUTPUT_TOPIC="detections.v1"

# Test data directory
SAMPLE_RULES_DIR="./flink-rule-job/src/main/resources/sample-rules"

echo -e "${YELLOW}Step 1: Checking Kafka topics...${NC}"

# Check if topics exist
for topic in $RULE_TOPIC $FEATURE_TOPIC $OUTPUT_TOPIC; do
    if kafka-topics.sh --bootstrap-server $KAFKA_BROKERS --list | grep -q "^${topic}$"; then
        echo -e "${GREEN}✓ Topic exists: $topic${NC}"
    else
        echo -e "${YELLOW}Creating topic: $topic${NC}"
        kafka-topics.sh --bootstrap-server $KAFKA_BROKERS \
            --create --topic $topic \
            --partitions 4 --replication-factor 1 || true
    fi
done

echo -e "${YELLOW}Step 2: Publishing sample rules...${NC}"

# Publish all sample rules
for rule_file in $SAMPLE_RULES_DIR/*.json; do
    if [ -f "$rule_file" ]; then
        echo -e "${YELLOW}  Publishing: $(basename $rule_file)${NC}"
        cat $rule_file | kafka-console-producer.sh \
            --bootstrap-server $KAFKA_BROKERS \
            --topic $RULE_TOPIC
        echo -e "${GREEN}  ✓ Published: $(basename $rule_file)${NC}"
    fi
done

echo -e "${YELLOW}Step 3: Waiting for rules to be processed...${NC}"
sleep 5

echo -e "${YELLOW}Step 4: Checking output topic...${NC}"

# Consume from output topic (timeout after 10 seconds)
timeout 10 kafka-console-consumer.sh \
    --bootstrap-server $KAFKA_BROKERS \
    --topic $OUTPUT_TOPIC \
    --from-beginning \
    --max-messages 10 || true

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  ✓ Integration test completed${NC}"
echo -e "${GREEN}========================================${NC}"

echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Check Flink Web UI: http://localhost:8081"
echo -e "  2. Monitor metrics: curl http://localhost:9250/metrics"
echo -e "  3. View logs: tail -f /var/log/flink/rule-job.log"