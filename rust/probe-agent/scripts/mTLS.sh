#!/usr/bin/env bash
# 生成 mTLS 证书（用于测试）

set -euo pipefail

CERT_DIR="./certs"
mkdir -p "${CERT_DIR}"

echo "🔐 Generating mTLS certificates for testing..."

# 生成 CA
openssl genrsa -out "${CERT_DIR}/ca.key" 4096
openssl req -new -x509 -key "${CERT_DIR}/ca.key" -out "${CERT_DIR}/ca.crt" -days 365 \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=NTA/CN=NTA-CA"

# 生成服务端证书
openssl genrsa -out "${CERT_DIR}/server.key" 2048
openssl req -new -key "${CERT_DIR}/server.key" -out "${CERT_DIR}/server.csr" \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=NTA/CN=ingest-gateway"
openssl x509 -req -in "${CERT_DIR}/server.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial -out "${CERT_DIR}/server.crt" -days 365

# 生成客户端证书
openssl genrsa -out "${CERT_DIR}/client.key" 2048
openssl req -new -key "${CERT_DIR}/client.key" -out "${CERT_DIR}/client.csr" \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=NTA/CN=probe-agent"
openssl x509 -req -in "${CERT_DIR}/client.csr" -CA "${CERT_DIR}/ca.crt" -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial -out "${CERT_DIR}/client.crt" -days 365

echo "✅ Certificates generated in ${CERT_DIR}/"
ls -lh "${CERT_DIR}/"