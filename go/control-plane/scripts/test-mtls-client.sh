#!/bin/bash
################################################################################
# FILE PATH: scripts/test-mtls-client.sh
# mTLS 客户端测试脚本（使用 grpcurl）
################################################################################

set -e

CERT_DIR="./certs"
SERVER_ADDR="ingest-gateway:50051"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Testing mTLS Connection ===${NC}"

# 检查证书文件是否存在
if [ ! -f "${CERT_DIR}/client/client-cert.pem" ]; then
    echo -e "${RED}Error: Client certificate not found!${NC}"
    echo "Please run: bash scripts/generate-certs.sh"
    exit 1
fi

# 检查 grpcurl 是否安装
if ! command -v grpcurl &> /dev/null; then
    echo -e "${YELLOW}grpcurl not found, installing...${NC}"
    go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
fi

# 1. 测试健康检查（不需要认证）
echo -e "${YELLOW}Test 1: Health Check (without auth)${NC}"
grpcurl \
    -cacert ${CERT_DIR}/ca/ca-cert.pem \
    -cert ${CERT_DIR}/client/client-cert.pem \
    -key ${CERT_DIR}/client/client-key.pem \
    ${SERVER_ADDR} \
    grpc.health.v1.Health/Check

# 2. 测试 Heartbeat（需要 mTLS + Token）
echo -e "${YELLOW}Test 2: Heartbeat (with mTLS)${NC}"

# 构建测试请求（ProbeID 从证书 CN 中提取）
PROBE_ID=$(openssl x509 -in ${CERT_DIR}/client/client-cert.pem -noout -subject | sed 's/.*CN = //')
echo "Detected ProbeID from certificate: ${PROBE_ID}"

grpcurl \
    -cacert ${CERT_DIR}/ca/ca-cert.pem \
    -cert ${CERT_DIR}/client/client-cert.pem \
    -key ${CERT_DIR}/client/client-key.pem \
    -H "x-tenant-token: test-token-002" \
    -d '{
      "probe_id": "'"${PROBE_ID}"'",
      "tenant_id": "tenant01",
      "status": {
        "cpu_usage": 35.5,
        "memory_usage": 60.2,
        "capture_pps": 1000,
        "upload_bps": 5000000,
        "packets_captured": 1000000,
        "packets_dropped": 100
      }
    }' \
    ${SERVER_ADDR} \
    traffic.v1.IngestService/Heartbeat

# 3. 测试 UploadFlows（模拟上报流事件）
echo -e "${YELLOW}Test 3: UploadFlows (with mTLS)${NC}"
grpcurl \
    -cacert ${CERT_DIR}/ca/ca-cert.pem \
    -cert ${CERT_DIR}/client/client-cert.pem \
    -key ${CERT_DIR}/client/client-key.pem \
    -H "x-tenant-token: test-token-002" \
    -d '{
      "events": [
        {
          "header": {
            "event_id": "test-event-001",
            "tenant_id": "tenant01",
            "probe_id": "'"${PROBE_ID}"'",
            "event_ts": 1700000000000,
            "feature_set_id": "v1"
          },
          "tuple": {
            "src_ip": "192.168.1.100",
            "dst_ip": "10.0.0.1",
            "src_port": 50000,
            "dst_port": 443,
            "protocol": 6
          },
          "community_id": "1:test-community-id",
          "bytes_fwd": 1500,
          "packets_fwd": 10
        }
      ]
    }' \
    ${SERVER_ADDR} \
    traffic.v1.IngestService/UploadFlows

echo -e "${GREEN}=== All Tests Completed ===${NC}"