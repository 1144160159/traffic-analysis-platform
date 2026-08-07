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
CA_VALIDITY_DAYS="${CA_VALIDITY_DAYS:-3650}"
LEAF_VALIDITY_DAYS="${LEAF_VALIDITY_DAYS:-90}"
case "$CA_VALIDITY_DAYS:$LEAF_VALIDITY_DAYS" in
  *[!0-9:]*|:*|*:)
    echo "CA_VALIDITY_DAYS and LEAF_VALIDITY_DAYS must be positive integers" >&2
    exit 1
    ;;
esac
if [ "$CA_VALIDITY_DAYS" -lt 365 ] || [ "$LEAF_VALIDITY_DAYS" -lt 1 ] || [ "$LEAF_VALIDITY_DAYS" -gt 90 ]; then
  echo "CA validity must be >=365 days and leaf validity must be between 1 and 90 days" >&2
  exit 1
fi
mkdir -p "$OUT_DIR"
cd "$OUT_DIR"

echo "Generating mTLS certificates in $OUT_DIR..."

# 1. CA 根证书
openssl genrsa -out ca-key.pem 4096 2>/dev/null
openssl req -new -x509 -days "$CA_VALIDITY_DAYS" -key ca-key.pem -out ca-cert.pem \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=Traffic Root CA" \
  -addext "basicConstraints=critical,CA:TRUE,pathlen:0" \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -addext "subjectKeyIdentifier=hash" 2>/dev/null
echo "  ✅ CA certificate"

# 2. 服务端证书 (Ingest Gateway)
openssl genrsa -out server-key.pem 2048 2>/dev/null
cat > server-ext.cnf << EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
[req_distinguished_name]
[v3_req]
basicConstraints = critical,CA:FALSE
subjectAltName = DNS:ingest-gateway,DNS:ingest-gateway.traffic-analysis.svc,DNS:ingest-gateway.traffic-analysis.svc.cluster.local
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
EOF
openssl req -new -key server-key.pem -out server.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=ingest-gateway" 2>/dev/null
openssl x509 -req -days "$LEAF_VALIDITY_DAYS" -in server.csr -CA ca-cert.pem -CAkey ca-key.pem \
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
basicConstraints = critical,CA:FALSE
subjectAltName = DNS:probe-agent
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = clientAuth
subjectKeyIdentifier = hash
authorityKeyIdentifier = keyid,issuer
EOF
openssl req -new -key client-key.pem -out client.csr \
  -subj "/C=CN/ST=Beijing/L=Beijing/O=TrafficAnalysis/CN=probe-agent" 2>/dev/null
openssl x509 -req -days "$LEAF_VALIDITY_DAYS" -in client.csr -CA ca-cert.pem -CAkey ca-key.pem \
  -CAcreateserial -out client-cert.pem -extfile client-ext.cnf -extensions v3_req 2>/dev/null
rm -f client.csr client-ext.cnf
echo "  ✅ Client certificate (probe-agent)"

openssl verify -CAfile ca-cert.pem server-cert.pem client-cert.pem >/dev/null
openssl x509 -checkend 86400 -noout -in server-cert.pem >/dev/null
openssl x509 -checkend 86400 -noout -in client-cert.pem >/dev/null
chmod 600 ca-key.pem server-key.pem client-key.pem
chmod 644 ca-cert.pem server-cert.pem client-cert.pem

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
