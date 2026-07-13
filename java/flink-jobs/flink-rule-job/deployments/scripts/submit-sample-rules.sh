#!/bin/bash
# ============================================================================
# Submit Sample Rules to Kafka
# ============================================================================

set -e

KAFKA_BROKERS=${KAFKA_BROKERS:-"localhost:9092"}
RULE_TOPIC=${RULE_TOPIC:-"rule.updates"}
SAMPLE_RULES_DIR="../src/main/resources/sample-rules"

echo "========================================="
echo "Submitting Sample Rules to Kafka"
echo "========================================="
echo "Kafka Brokers: $KAFKA_BROKERS"
echo "Topic: $RULE_TOPIC"
echo "Sample Rules Dir: $SAMPLE_RULES_DIR"
echo ""

# Check if sample rules directory exists
if [ ! -d "$SAMPLE_RULES_DIR" ]; then
    echo "✗ Sample rules directory not found: $SAMPLE_RULES_DIR"
    exit 1
fi

# Function to submit a rule
submit_rule() {
    local rule_file=$1
    local rule_name=$(basename $rule_file)
    
    echo "Submitting: $rule_name"
    
    # Validate JSON
    if ! jq . "$rule_file" > /dev/null 2>&1; then
        echo "  ✗ Invalid JSON in $rule_name"
        return 1
    fi
    
    # Submit to Kafka
    if cat "$rule_file" | kafka-console-producer.sh \
        --bootstrap-server "$KAFKA_BROKERS" \
        --topic "$RULE_TOPIC" > /dev/null 2>&1; then
        echo "  ✓ Submitted successfully"
        return 0
    else
        echo "  ✗ Failed to submit"
        return 1
    fi
}

# Submit all sample rules
SUCCESS_COUNT=0
FAILURE_COUNT=0

for rule_file in "$SAMPLE_RULES_DIR"/*.json; do
    if [ -f "$rule_file" ]; then
        if submit_rule "$rule_file"; then
            ((SUCCESS_COUNT++))
        else
            ((FAILURE_COUNT++))
        fi
    fi
done

echo ""
echo "========================================="
echo "Submission Summary"
echo "========================================="
echo "✓ Success: $SUCCESS_COUNT"
echo "✗ Failure: $FAILURE_COUNT"
echo ""

if [ $FAILURE_COUNT -eq 0 ]; then
    echo "All sample rules submitted successfully!"
    echo ""
    echo "Verify rules in Flink:"
    echo "  1. Access Flink UI: http://localhost:8081"
    echo "  2. Check metrics: active_rule_count should be $SUCCESS_COUNT"
    echo "  3. Check logs for [RULE_AUDIT] messages"
else
    echo "Some rules failed to submit. Check Kafka connectivity."
    exit 1
fi

echo "========================================="
