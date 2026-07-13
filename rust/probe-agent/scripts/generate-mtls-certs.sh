#!/bin/bash
# =============================================================================
# mTLS 证书生成 — Probe Agent ↔ Ingest Gateway 双向 TLS 认证
#
# 生成:
#   ca-cert.pem  / ca-key.pem          (CA 根证书)
#   server-cert.pem / server-key.pem    (Ingest Gateway 服务端)
#   client-cert.pem / client-key.pem    (Probe Agent 客户端)
#
# 用法:
#   ./scripts/generate-mtls-certs.sh
#   kubectl create secret generic probe-agent-certs \
#     --from-file=ca-cert.pem --from-file=client-cert.pem --from-file=client-key.pem \
#     -n traffic-analysis
#   kubectl create secret generic ingest-gateway-certs \
#     --from-file=ca-cert.pem --from-file=server-cert.pem --from-file=server-key.pem \
#     -n traffic-analysis
# =============================================================================
set -euo pipefail

OUT_DIR="${1:-./certs}"
mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo "Generating mTLS certificates in $OUT_DIR..."

# 1. CA 根证书
openssl genrsa -out ca-key.pem 4096 2>/dev/null
openssl req -new -x509 -days 3650 -key ca-key.pem -out ca-cert.pem \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=Traffic Root CA" 2>/dev/null
echo "  ✅ CA certificate"

# 2. 服务端证书 (Ingest Gateway)
openssl genrsa -out server-key.pem 2048 2>/dev/null
cat > server-ext.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
[req_distinguished_name]
[v3_req]
subjectAltName = DNS:ingest-gateway,DNS:ingest-gateway.traffic-analysis.svc,DNS:ingest-gateway.traffic-analysis.svc.cluster.local,IP:10.0.5.210
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
EOF
openssl req -new -key server-key.pem -out server.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=ingest-gateway" 2>/dev/null
openssl x509 -req -days 365 -in server.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out server-cert.pem -extfile server-ext.cnf -extensions v3_req 2>/dev/null
rm -f server.csr server-ext.cnf
echo "  ✅ Server certificate (ingest-gateway)"

# 3. 客户端证书 (Probe Agent)
openssl genrsa -out client-key.pem 2048 2>/dev/null
cat > client-ext.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
[req_distinguished_name]
[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
EOF
openssl req -new -key client-key.pem -out client.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=probe-agent" 2>/dev/null
openssl x509 -req -days 365 -in client.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-cert.pem -extfile client-ext.cnf -extensions v3_req 2>/dev/null
rm -f client.csr client-ext.cnf
echo "  ✅ Client certificate (probe-agent)"

# Summary
echo ""
echo "=== Certificates Generated ==="
ls -la *.pem 2>/dev/null
echo ""
echo "Probe Agent config.yaml:"
echo "  tls_ca_cert: /etc/probe-agent/certs/ca-cert.pem"
echo "  tls_client_cert: /etc/probe-agent/certs/client-cert.pem"
echo "  tls_client_key: /etc/probe-agent/certs/client-key.pem"
echo ""
echo "K8s Secret creation:"
echo "  kubectl create secret generic probe-agent-certs \\"
echo "    --from-file=ca-cert.pem --from-file=client-cert.pem --from-file=client-key.pem \\"
echo "    -n traffic-analysis"
echo "  kubectl create secret generic ingest-gateway-certs \\"
echo "    --from-file=ca-cert.pem --from-file=server-cert.pem --from-file=server-key.pem \\"
echo "    -n traffic-analysis"
