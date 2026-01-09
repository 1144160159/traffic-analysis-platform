#!/bin/bash
# =============================================================================
# Generate mTLS certificates for testing
# =============================================================================

set -euo pipefail

CERTS_DIR="${1:-./certs}"
DAYS=365
CA_SUBJECT="/CN=Traffic Analysis CA/O=Traffic Analysis Platform"
SERVER_SUBJECT="/CN=ingest-gateway/O=Traffic Analysis Platform"

mkdir -p "${CERTS_DIR}"
cd "${CERTS_DIR}"

echo "==> Generating CA certificate..."
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days ${DAYS} -key ca.key -out ca.crt \
    -subj "${CA_SUBJECT}"

echo "==> Generating server certificate..."
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
    -subj "${SERVER_SUBJECT}"

cat > server-ext.cnf << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = ingest-gateway
DNS.3 = ingest-gateway.traffic-analysis.svc
DNS.4 = ingest-gateway.traffic-analysis.svc.cluster.local
DNS.5 = *.ingest-gateway.traffic-analysis.svc.cluster.local
IP.1 = 127.0.0.1
IP.2 = 10.0.5.8
IP.3 = 10.0.5.9
EOF

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key \
    -CAcreateserial -out server.crt -days ${DAYS} \
    -extfile server-ext.cnf

echo "==> Generating probe client certificates..."
for i in 01 02 03; do
    PROBE_ID="probe-${i}"
    echo "    Generating certificate for ${PROBE_ID}..."
    
    openssl genrsa -out "client-${PROBE_ID}.key" 2048
    openssl req -new -key "client-${PROBE_ID}.key" -out "client-${PROBE_ID}.csr" \
        -subj "/CN=${PROBE_ID}/O=Traffic Analysis Platform"
    
    cat > "client-${PROBE_ID}-ext.cnf" << EOF
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = ${PROBE_ID}.traffic.local
EOF
    
    openssl x509 -req -in "client-${PROBE_ID}.csr" -CA ca.crt -CAkey ca.key \
        -CAcreateserial -out "client-${PROBE_ID}.crt" -days ${DAYS} \
        -extfile "client-${PROBE_ID}-ext.cnf"
    
    # Cleanup CSR and ext files
    rm -f "client-${PROBE_ID}.csr" "client-${PROBE_ID}-ext.cnf"
done

# Cleanup
rm -f server.csr server-ext.cnf

echo ""
echo "==> Certificates generated in ${CERTS_DIR}/"
echo "    CA:     ca.crt, ca.key"
echo "    Server: server.crt, server.key"
echo "    Clients: client-probe-*.crt, client-probe-*.key"
echo ""
echo "==> Verify server certificate:"
openssl verify -CAfile ca.crt server.crt
echo ""
echo "==> Verify client certificates:"
for i in 01 02 03; do
    openssl verify -CAfile ca.crt "client-probe-${i}.crt"
done

echo ""
echo "==> To create Kubernetes secret:"
echo "    kubectl create secret tls ingest-gateway-tls \\"
echo "        --cert=server.crt --key=server.key \\"
echo "        --namespace=traffic-analysis"
echo ""
echo "    kubectl create secret generic ingest-gateway-ca \\"
echo "        --from-file=ca.crt=ca.crt \\"
echo "        --namespace=traffic-analysis"