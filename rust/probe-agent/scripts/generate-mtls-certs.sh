#!/usr/bin/env bash
# rust/probe-agent/scripts/generate-mtls-certs.sh
set -euo pipefail

CERT_DIR="${CERT_DIR:-./certs}"
VALIDITY_DAYS="${VALIDITY_DAYS:-365}"

log_info() {
    echo -e "\033[0;32m[INFO]\033[0m $1"
}

mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

log_info "========================================="
log_info " Generating mTLS Certificates"
log_info "========================================="

# 1. 生成 CA
log_info "Generating CA..."
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days $VALIDITY_DAYS -key ca.key -out ca.crt \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=CA"

# 2. 生成服务端证书
log_info "Generating server certificate..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=ingest-gateway"
openssl x509 -req -days $VALIDITY_DAYS -in server.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt

# 3. 生成客户端证书
log_info "Generating client certificate..."
openssl genrsa -out client.key 2048
openssl req -new -key client.key -out client.csr \
    -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=probe-agent"
openssl x509 -req -days $VALIDITY_DAYS -in client.csr \
    -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt

# 清理临时文件
rm -f *.csr *.srl

log_info "========================================="
log_info "✓ Certificates generated successfully"
log_info "========================================="
log_info ""
log_info "Files created in $CERT_DIR:"
log_info "  ca.crt, ca.key       - Certificate Authority"
log_info "  server.crt, server.key - Server certificates"
log_info "  client.crt, client.key - Client certificates"
log_info ""
log_info "Usage:"
log_info "  Server: --tls-cert=server.crt --tls-key=server.key --tls-ca=ca.crt"
log_info "  Client: --tls-cert=client.crt --tls-key=client.key --tls-ca=ca.crt"
log_info ""