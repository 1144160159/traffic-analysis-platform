#!/bin/bash
# submit-rule-job.sh - Submit Flink Rule Job to cluster

set -e

# Configuration
FLINK_HOME=${FLINK_HOME:-/opt/flink}
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
JAR_PATH=${JAR_PATH:-"${SCRIPT_DIR}/../flink-rule-job/target/flink-rule-job-1.0.0-SNAPSHOT.jar"}
JOB_CLASS="com.traffic.flink.rule.RuleJob"
. "${SCRIPT_DIR}/clickhouse-password.sh"

# Default parameters
PARALLELISM=${PARALLELISM:-4}
KAFKA_BROKERS=${KAFKA_BROKERS:-kafka-bootstrap.middleware.svc:9092}
CLICKHOUSE_URL=${CLICKHOUSE_URL:-localhost:8123}
CHECKPOINT_PATH=${CHECKPOINT_PATH:-s3://flink-checkpoints/checkpoints/rule-job}
OUTPUT_TOPIC=${OUTPUT_TOPIC:-detections.behavior.v1}
CLICKHOUSE_TABLE=${CLICKHOUSE_TABLE:-detections_behavior}
resolve_clickhouse_password

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Flink Rule Job Submission Script${NC}"
echo -e "${GREEN}========================================${NC}"

# Check if JAR exists
if [ ! -f "$JAR_PATH" ]; then
    echo -e "${RED}ERROR: JAR file not found: $JAR_PATH${NC}"
    echo -e "${YELLOW}Please build the project first: mvn clean package${NC}"
    exit 1
fi

# Check Flink cluster
echo -e "${YELLOW}Checking Flink cluster...${NC}"
if ! $FLINK_HOME/bin/flink list > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Cannot connect to Flink cluster${NC}"
    echo -e "${YELLOW}Please start Flink cluster first: $FLINK_HOME/bin/start-cluster.sh${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Flink cluster is running${NC}"

# Submit job
echo -e "${YELLOW}Submitting Rule Job...${NC}"
echo -e "${YELLOW}  JAR: $JAR_PATH${NC}"
echo -e "${YELLOW}  Parallelism: $PARALLELISM${NC}"
echo -e "${YELLOW}  Kafka Brokers: $KAFKA_BROKERS${NC}"
echo -e "${YELLOW}  ClickHouse URL: $CLICKHOUSE_URL${NC}"

$FLINK_HOME/bin/flink run \
    -c $JOB_CLASS \
    -p $PARALLELISM \
    -d \
    $JAR_PATH \
    --kafka.brokers $KAFKA_BROKERS \
    --kafka.feature.topic feature.stat.v1 \
    --kafka.rule.topic rule.updates \
    --kafka.output.topic $OUTPUT_TOPIC \
    --kafka.dlq.topic dlq.rule-job \
    --clickhouse.url $CLICKHOUSE_URL \
    --clickhouse.database traffic \
    --clickhouse.table $CLICKHOUSE_TABLE \
    --checkpoint.path $CHECKPOINT_PATH \
    --parallelism $PARALLELISM \
    --clickhouse.password "$CLICKHOUSE_PASSWORD"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}  ✓ Job submitted successfully!${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""
    echo -e "${YELLOW}Check job status:${NC}"
    echo -e "  $FLINK_HOME/bin/flink list"
    echo ""
    echo -e "${YELLOW}View job details:${NC}"
    echo -e "  http://localhost:8081"
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}  ✗ Job submission failed!${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
fi
