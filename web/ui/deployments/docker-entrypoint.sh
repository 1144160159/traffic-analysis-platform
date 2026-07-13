#!/bin/sh
set -eu

: "${API_BASE_URL:=/api}"
: "${API_BACKEND_HOST:=apisix.gateway.svc}"
: "${API_BACKEND_PORT:=9080}"
: "${WS_URL:=/ws/events}"
: "${WS_BACKEND_HOST:=apisix.gateway.svc}"
: "${WS_BACKEND_PORT:=9080}"
: "${ARKIME_BASE_URL:=}"
: "${AUTH_ENABLED:=true}"
: "${USE_MOCK:=false}"
: "${ENABLE_REALTIME:=false}"
: "${SCREEN_ACCESS_MODE:=protected}"
: "${DESKTOP_SMOKE_TOKEN_ENABLED:=false}"

export API_BASE_URL API_BACKEND_HOST API_BACKEND_PORT
export WS_URL WS_BACKEND_HOST WS_BACKEND_PORT ARKIME_BASE_URL
export AUTH_ENABLED USE_MOCK ENABLE_REALTIME SCREEN_ACCESS_MODE DESKTOP_SMOKE_TOKEN_ENABLED

envsubst '${API_BACKEND_HOST} ${API_BACKEND_PORT} ${WS_BACKEND_HOST} ${WS_BACKEND_PORT}' \
  < /etc/nginx/nginx.conf.template \
  > /etc/nginx/nginx.conf

envsubst '${API_BASE_URL} ${WS_URL} ${ARKIME_BASE_URL} ${AUTH_ENABLED} ${USE_MOCK} ${ENABLE_REALTIME} ${SCREEN_ACCESS_MODE} ${DESKTOP_SMOKE_TOKEN_ENABLED}' \
  < /usr/share/nginx/html/config.js.template \
  > /usr/share/nginx/html/config.js

exec "$@"
