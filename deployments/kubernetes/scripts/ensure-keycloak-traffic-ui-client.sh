#!/usr/bin/env bash
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
KEYCLOAK_NAMESPACE="${KEYCLOAK_NAMESPACE:-iam}"
KEYCLOAK_POD="${KEYCLOAK_POD:-keycloak-0}"
TRAFFIC_NAMESPACE="${TRAFFIC_NAMESPACE:-traffic-analysis}"
PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://10.0.5.8:30180}"
REALM="${REALM:-master}"
CLIENT_ID="${CLIENT_ID:-traffic-ui}"
TRUSTSTORE_PATH="${TRUSTSTORE_PATH:-/tmp/keycloak-truststore.p12}"
TRUSTSTORE_PASSWORD="${TRUSTSTORE_PASSWORD:-changeit}"
KCADM_CONFIG="${KCADM_CONFIG:-/tmp/kcadm-secure.config}"

admin_secret="$($KUBECTL -n "$KEYCLOAK_NAMESPACE" get secret traffic-credentials -o jsonpath='{.data.KEYCLOAK_ADMIN_PASSWORD}' | base64 -d)"
oidc_secret="$($KUBECTL -n "$TRAFFIC_NAMESPACE" get secret traffic-credentials -o jsonpath='{.data.OIDC_CLIENT_SECRET}' | base64 -d)"

$KUBECTL -n "$KEYCLOAK_NAMESPACE" exec "$KEYCLOAK_POD" -- sh -c "
  rm -f '$TRUSTSTORE_PATH' '$KCADM_CONFIG'
  keytool -importcert -noprompt -alias keycloak-local \
    -file /etc/keycloak/tls/tls.crt \
    -keystore '$TRUSTSTORE_PATH' \
    -storetype PKCS12 \
    -storepass '$TRUSTSTORE_PASSWORD' >/tmp/keycloak-truststore.out 2>/tmp/keycloak-truststore.err
"

$KUBECTL -n "$KEYCLOAK_NAMESPACE" exec "$KEYCLOAK_POD" -- \
  /opt/keycloak/bin/kcadm.sh config credentials \
    --server https://keycloak.iam.svc:8443 \
    --realm "$REALM" \
    --user admin \
    --password "$admin_secret" \
    --truststore "$TRUSTSTORE_PATH" \
    --trustpass "$TRUSTSTORE_PASSWORD" \
    --config "$KCADM_CONFIG" >/dev/null

client_uuid="$($KUBECTL -n "$KEYCLOAK_NAMESPACE" exec "$KEYCLOAK_POD" -- \
  /opt/keycloak/bin/kcadm.sh get clients \
    -r "$REALM" \
    -q "clientId=$CLIENT_ID" \
    --fields id \
    --truststore "$TRUSTSTORE_PATH" \
    --trustpass "$TRUSTSTORE_PASSWORD" \
    --config "$KCADM_CONFIG" | jq -r '.[0].id // ""')"

if [[ -z "$client_uuid" ]]; then
  client_uuid="$($KUBECTL -n "$KEYCLOAK_NAMESPACE" exec "$KEYCLOAK_POD" -- \
    /opt/keycloak/bin/kcadm.sh create clients \
      -r "$REALM" \
      --truststore "$TRUSTSTORE_PATH" \
      --trustpass "$TRUSTSTORE_PASSWORD" \
      --config "$KCADM_CONFIG" \
      -s "clientId=$CLIENT_ID" \
      -s enabled=true \
      -s protocol=openid-connect \
      -s publicClient=false \
      -s bearerOnly=false \
      -s standardFlowEnabled=true \
      -s directAccessGrantsEnabled=false \
      -s serviceAccountsEnabled=false \
      -s clientAuthenticatorType=client-secret \
      -s 'redirectUris=[\"'$PUBLIC_BASE_URL'/api/v1/auth/oidc/callback\"]' \
      -s 'webOrigins=[\"'$PUBLIC_BASE_URL'\"]' \
      -s 'attributes.\"post.logout.redirect.uris\"=\"'$PUBLIC_BASE_URL'/*\"' \
      -i)"
fi

$KUBECTL -n "$KEYCLOAK_NAMESPACE" exec "$KEYCLOAK_POD" -- \
  /opt/keycloak/bin/kcadm.sh update "clients/$client_uuid" \
    -r "$REALM" \
    --truststore "$TRUSTSTORE_PATH" \
    --trustpass "$TRUSTSTORE_PASSWORD" \
    --config "$KCADM_CONFIG" \
    -s enabled=true \
    -s protocol=openid-connect \
    -s publicClient=false \
    -s bearerOnly=false \
    -s standardFlowEnabled=true \
    -s directAccessGrantsEnabled=false \
    -s serviceAccountsEnabled=false \
    -s clientAuthenticatorType=client-secret \
    -s "secret=$oidc_secret" \
    -s 'redirectUris=["'"$PUBLIC_BASE_URL"'/api/v1/auth/oidc/callback"]' \
    -s 'webOrigins=["'"$PUBLIC_BASE_URL"'"]' \
    -s 'attributes."post.logout.redirect.uris"="'"$PUBLIC_BASE_URL"'/*"'

echo "keycloak_client_ready client_id=$CLIENT_ID realm=$REALM public_base_url=$PUBLIC_BASE_URL client_uuid=$client_uuid"
