#!/usr/bin/env bash
set -euo pipefail

# Rotates the short-lived service JWT consumed by the Argo model-registration
# step. The signing secret never leaves the pipeline and the generated token is
# written only to the Kubernetes Secret.

traffic_namespace="${TRAFFIC_NAMESPACE:-traffic-analysis}"
argo_namespace="${ARGO_NAMESPACE:-argo}"
token_ttl_seconds="${MLOPS_TOKEN_TTL_SECONDS:-604800}"

if ! [[ "$token_ttl_seconds" =~ ^[0-9]+$ ]] || (( token_ttl_seconds < 300 )); then
  echo "MLOPS_TOKEN_TTL_SECONDS must be an integer of at least 300" >&2
  exit 2
fi

jwt_secret_b64=$(kubectl -n "$traffic_namespace" get secret traffic-credentials -o jsonpath='{.data.JWT_SECRET}')
mlops_service_token=$(JWT_SECRET_B64="$jwt_secret_b64" TOKEN_TTL_SECONDS="$token_ttl_seconds" node - <<'NODE'
const crypto = require('crypto');
const now = Math.floor(Date.now() / 1000);
const encode = (value) => Buffer.from(JSON.stringify(value)).toString('base64url');
const header = encode({ alg: 'HS256', typ: 'JWT' });
const userId = crypto.randomUUID();
const claims = encode({
  iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
  tenant_id: 'default', username: 'mlops-workflow-service', roles: ['admin'],
  permissions: ['model:read', 'model:write', 'model:create', 'model:activate'],
  token_type: 'access', iat: now, exp: now + Number(process.env.TOKEN_TTL_SECONDS),
});
const input = `${header}.${claims}`;
const key = Buffer.from(process.env.JWT_SECRET_B64, 'base64');
process.stdout.write(`${input}.${crypto.createHmac('sha256', key).update(input).digest('base64url')}`);
NODE
)

kubectl -n "$argo_namespace" create secret generic mlops-api-token \
  --from-literal=token="$mlops_service_token" \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$argo_namespace" annotate secret mlops-api-token \
  traffic.openai.com/rotated-at="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  traffic.openai.com/rotation-window="${token_ttl_seconds}s" --overwrite

echo "rotated ${argo_namespace}/mlops-api-token for ${token_ttl_seconds}s"
