#!/bin/bash
################################################################################
# FILE PATH: scripts/generate-certs.sh
# mTLS 证书生成脚本（生产级 - X.509 v3）
# 用途：为 Ingest Gateway 生成 CA、服务端证书、客户端证书
################################################################################

set -e

# 配置
CERT_DIR="./certs"
VALIDITY_DAYS=3650  # 10年
COUNTRY="CN"
STATE="Beijing"
LOCALITY="Beijing"
ORGANIZATION="Traffic Analysis Platform"
OU="Engineering"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== mTLS Certificate Generation (X.509 v3) ===${NC}"

# 1. 创建证书目录
mkdir -p ${CERT_DIR}/{ca,server,client}

# 2. 生成 CA 私钥和自签名证书（v3）
echo -e "${YELLOW}Step 1/6: Generating CA private key and certificate (v3)...${NC}"
openssl genrsa -out ${CERT_DIR}/ca/ca-key.pem 4096

# ✅ 修改：添加 v3 扩展
openssl req -new -x509 \
    -days ${VALIDITY_DAYS} \
    -key ${CERT_DIR}/ca/ca-key.pem \
    -out ${CERT_DIR}/ca/ca-cert.pem \
    -subj "/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORGANIZATION}/OU=${OU}/CN=Traffic Analysis Platform CA" \
    -addext "basicConstraints=critical,CA:TRUE" \
    -addext "keyUsage=critical,digitalSignature,cRLSign,keyCertSign" \
    -addext "subjectKeyIdentifier=hash"

echo -e "${GREEN}✓ CA certificate generated (v3)${NC}"

# 3. 生成服务端私钥
echo -e "${YELLOW}Step 2/6: Generating server private key...${NC}"
openssl genrsa -out ${CERT_DIR}/server/server-key.pem 4096

# 4. 生成服务端 CSR
echo -e "${YELLOW}Step 3/6: Generating server CSR...${NC}"
openssl req -new \
    -key ${CERT_DIR}/server/server-key.pem \
    -out ${CERT_DIR}/server/server.csr \
    -subj "/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORGANIZATION}/OU=${OU}/CN=ingest-gateway"

# 5. 使用 CA 签署服务端证书（添加 v3 扩展）
echo -e "${YELLOW}Step 4/6: Signing server certificate with CA (v3)...${NC}"

# ✅ 修改：完整的 v3 扩展配置
cat > ${CERT_DIR}/server/server-ext.cnf <<EOF
basicConstraints = CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
subjectAltName = @alt_names

[alt_names]
DNS.1 = ingest-gateway
DNS.2 = ingest-gateway.traffic
DNS.3 = ingest-gateway.traffic.svc
DNS.4 = ingest-gateway.traffic.svc.cluster.local
DNS.5 = localhost
IP.1 = 127.0.0.1
IP.2 = ::1
EOF

openssl x509 -req \
    -days ${VALIDITY_DAYS} \
    -in ${CERT_DIR}/server/server.csr \
    -CA ${CERT_DIR}/ca/ca-cert.pem \
    -CAkey ${CERT_DIR}/ca/ca-key.pem \
    -CAcreateserial \
    -out ${CERT_DIR}/server/server-cert.pem \
    -extfile ${CERT_DIR}/server/server-ext.cnf \
    -sha256

echo -e "${GREEN}✓ Server certificate signed (v3)${NC}"

# 6. 生成客户端私钥
echo -e "${YELLOW}Step 5/6: Generating client private key...${NC}"
openssl genrsa -out ${CERT_DIR}/client/client-key.pem 2048

# 7. 生成客户端证书（v3）
echo -e "${YELLOW}Step 6/6: Generating client certificate (v3)...${NC}"

# 示例：为探针 probe-tenant01-001 生成证书
PROBE_ID="probe-tenant01-001"
openssl req -new \
    -key ${CERT_DIR}/client/client-key.pem \
    -out ${CERT_DIR}/client/client.csr \
    -subj "/C=${COUNTRY}/ST=${STATE}/L=${LOCALITY}/O=${ORGANIZATION}/OU=Probes/CN=${PROBE_ID}"

# ✅ 修改：添加 v3 客户端扩展
cat > ${CERT_DIR}/client/client-ext.cnf <<EOF
basicConstraints = CA:FALSE
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
subjectKeyIdentifier = hash
subjectAltName = DNS:${PROBE_ID}
EOF

# 使用 CA 签署客户端证书（v3）
openssl x509 -req \
    -days ${VALIDITY_DAYS} \
    -in ${CERT_DIR}/client/client.csr \
    -CA ${CERT_DIR}/ca/ca-cert.pem \
    -CAkey ${CERT_DIR}/ca/ca-key.pem \
    -CAcreateserial \
    -out ${CERT_DIR}/client/client-cert.pem \
    -extfile ${CERT_DIR}/client/client-ext.cnf \
    -sha256

echo -e "${GREEN}✓ Client certificate signed (v3, ProbeID: ${PROBE_ID})${NC}"

# 8. 验证证书
echo -e "${YELLOW}Verifying certificates...${NC}"
openssl verify -CAfile ${CERT_DIR}/ca/ca-cert.pem ${CERT_DIR}/server/server-cert.pem
openssl verify -CAfile ${CERT_DIR}/ca/ca-cert.pem ${CERT_DIR}/client/client-cert.pem

# ✅ 新增：验证证书版本
echo -e "${YELLOW}Checking certificate versions...${NC}"
CA_VERSION=$(openssl x509 -in ${CERT_DIR}/ca/ca-cert.pem -text -noout | grep "Version:" | awk '{print $2}')
SERVER_VERSION=$(openssl x509 -in ${CERT_DIR}/server/server-cert.pem -text -noout | grep "Version:" | awk '{print $2}')
CLIENT_VERSION=$(openssl x509 -in ${CERT_DIR}/client/client-cert.pem -text -noout | grep "Version:" | awk '{print $2}')

if [ "$CA_VERSION" = "3" ]; then
    echo -e "${GREEN}✓ CA Certificate: v3${NC}"
else
    echo -e "${RED}✗ CA Certificate: v${CA_VERSION} (expected v3)${NC}"
fi

if [ "$SERVER_VERSION" = "3" ]; then
    echo -e "${GREEN}✓ Server Certificate: v3${NC}"
else
    echo -e "${RED}✗ Server Certificate: v${SERVER_VERSION} (expected v3)${NC}"
fi

if [ "$CLIENT_VERSION" = "3" ]; then
    echo -e "${GREEN}✓ Client Certificate: v3${NC}"
else
    echo -e "${RED}✗ Client Certificate: v${CLIENT_VERSION} (expected v3)${NC}"
fi

# 9. 设置权限（生产环境）
chmod 600 ${CERT_DIR}/ca/ca-key.pem
chmod 600 ${CERT_DIR}/server/server-key.pem
chmod 600 ${CERT_DIR}/client/client-key.pem
chmod 644 ${CERT_DIR}/ca/ca-cert.pem
chmod 644 ${CERT_DIR}/server/server-cert.pem
chmod 644 ${CERT_DIR}/client/client-cert.pem

# 10. 输出证书信息
echo ""
echo -e "${GREEN}=== Certificate Generation Complete ===${NC}"
echo ""
echo "CA Certificate:"
openssl x509 -in ${CERT_DIR}/ca/ca-cert.pem -noout -subject -dates
echo "  Version: $(openssl x509 -in ${CERT_DIR}/ca/ca-cert.pem -text -noout | grep 'Version:' | awk '{print $2}')"
echo ""
echo "Server Certificate:"
openssl x509 -in ${CERT_DIR}/server/server-cert.pem -noout -subject -dates
echo "  Version: $(openssl x509 -in ${CERT_DIR}/server/server-cert.pem -text -noout | grep 'Version:' | awk '{print $2}')"
openssl x509 -in ${CERT_DIR}/server/server-cert.pem -noout -ext subjectAltName
echo ""
echo "Client Certificate (ProbeID):"
openssl x509 -in ${CERT_DIR}/client/client-cert.pem -noout -subject -dates
echo "  Version: $(openssl x509 -in ${CERT_DIR}/client/client-cert.pem -text -noout | grep 'Version:' | awk '{print $2}')"
echo ""

echo -e "${GREEN}Files generated in: ${CERT_DIR}/${NC}"
echo ""
echo "Server files:"
echo "  - ${CERT_DIR}/server/server-cert.pem"
echo "  - ${CERT_DIR}/server/server-key.pem"
echo "  - ${CERT_DIR}/ca/ca-cert.pem (CA)"
echo ""
echo "Client files (for probe ${PROBE_ID}):"
echo "  - ${CERT_DIR}/client/client-cert.pem"
echo "  - ${CERT_DIR}/client/client-key.pem"
echo "  - ${CERT_DIR}/ca/ca-cert.pem (CA)"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Copy certificates to probe-agent:"
echo "   sudo cp ${CERT_DIR}/ca/ca-cert.pem /etc/probe-agent/"
echo "   sudo cp ${CERT_DIR}/client/client-cert.pem /etc/probe-agent/"
echo "   sudo cp ${CERT_DIR}/client/client-key.pem /etc/probe-agent/"
echo ""
echo "2. Update config.yaml with certificate paths"
echo "3. Set REQUIRE_MTLS=true in ingest-gateway config"
echo "4. Restart ingest-gateway"
echo "5. Test probe-agent connection"

# ✅ 新增：生成快速部署命令
echo ""
echo -e "${GREEN}Quick deployment commands:${NC}"
echo "  # For probe-agent:"
echo "  sudo mkdir -p /etc/probe-agent"
echo "  sudo cp ${CERT_DIR}/ca/ca-cert.pem /etc/probe-agent/"
echo "  sudo cp ${CERT_DIR}/client/client-cert.pem /etc/probe-agent/"
echo "  sudo cp ${CERT_DIR}/client/client-key.pem /etc/probe-agent/"
echo "  sudo chmod 600 /etc/probe-agent/client-key.pem"