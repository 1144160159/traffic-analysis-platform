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
CRL_VALIDITY_DAYS="${CRL_VALIDITY_DAYS:-7}"
PKI_GENERATION_ID="${PKI_GENERATION_ID:-probe-ingest-$(date -u +%Y%m%dT%H%M%SZ)}"
case "$CA_VALIDITY_DAYS:$LEAF_VALIDITY_DAYS" in
  *[!0-9:]*|:*|*:)
    echo "CA_VALIDITY_DAYS and LEAF_VALIDITY_DAYS must be positive integers" >&2
    exit 1
    ;;
esac
case "$CRL_VALIDITY_DAYS" in
  *[!0-9]*|'')
    echo "CRL_VALIDITY_DAYS must be a positive integer" >&2
    exit 1
    ;;
esac
case "$PKI_GENERATION_ID" in
  *[!A-Za-z0-9._-]*|'')
    echo "PKI_GENERATION_ID must use only letters, digits, dot, underscore, and hyphen" >&2
    exit 1
    ;;
esac
if [ "$CA_VALIDITY_DAYS" -lt 365 ] || [ "$LEAF_VALIDITY_DAYS" -lt 1 ] || [ "$LEAF_VALIDITY_DAYS" -gt 90 ] || [ "$CRL_VALIDITY_DAYS" -lt 1 ]; then
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

# 4. 当前 CA 的空 CRL。吊销时由批准的 issuer 使用同一格式发布新 CRL；
# ingest-gateway 会拒绝无 CRL、过期 CRL 或非受信 CA 签名的 CRL。
mkdir -p ca-newcerts
: > ca-index.txt
echo 1000 > ca-serial
echo 1000 > ca-crlnumber
cat > ca-crl.cnf << EOF
[ca]
default_ca = CA_default
[CA_default]
dir = .
database = \$dir/ca-index.txt
new_certs_dir = \$dir/ca-newcerts
certificate = \$dir/ca-cert.pem
private_key = \$dir/ca-key.pem
serial = \$dir/ca-serial
crlnumber = \$dir/ca-crlnumber
default_md = sha256
default_days = $LEAF_VALIDITY_DAYS
default_crl_days = $CRL_VALIDITY_DAYS
policy = policy_any
[policy_any]
commonName = supplied
EOF
openssl ca -gencrl -batch -config ca-crl.cnf -out client-crl.pem 2>/dev/null
echo "  ✅ Client certificate revocation list"

# 5. 原子投影 manifest。Kubernetes Secret 更新期间只要任一文件来自另一代，
# 摘要即不匹配，服务会继续使用上一套已验证材料。
cert_sha="$(sha256sum server-cert.pem | awk '{print $1}')"
key_sha="$(sha256sum server-key.pem | awk '{print $1}')"
trust_sha="$(sha256sum ca-cert.pem | awk '{print $1}')"
crl_sha="$(sha256sum client-crl.pem | awk '{print $1}')"
cat > generation.json << EOF
{"schema_version":1,"generation":"$PKI_GENERATION_ID","certificate_sha256":"$cert_sha","private_key_sha256":"$key_sha","trust_bundle_sha256":"$trust_sha","revocation_sha256":"$crl_sha"}
EOF
echo "  ✅ Atomic generation manifest ($PKI_GENERATION_ID)"

openssl verify -CAfile ca-cert.pem server-cert.pem client-cert.pem >/dev/null
openssl x509 -checkend 86400 -noout -in server-cert.pem >/dev/null
openssl x509 -checkend 86400 -noout -in client-cert.pem >/dev/null
openssl crl -noout -in client-crl.pem >/dev/null
chmod 600 ca-key.pem server-key.pem client-key.pem
chmod 644 ca-cert.pem server-cert.pem client-cert.pem client-crl.pem generation.json

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
echo "    --from-file=client-crl.pem --from-file=generation.json \\"
echo "    -n traffic-analysis"
