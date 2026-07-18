#!/usr/bin/env bash

resolve_clickhouse_password() {
  local kubectl_bin="${KUBECTL:-kubectl}"
  local secret_name="${CLICKHOUSE_SECRET_NAME:-traffic-credentials}"
  local secret_namespace="${CLICKHOUSE_SECRET_NAMESPACE:-middleware}"

  if [ -z "${CLICKHOUSE_PASSWORD:-}" ] && command -v "$kubectl_bin" >/dev/null 2>&1; then
    CLICKHOUSE_PASSWORD="$(
      "$kubectl_bin" get secret "$secret_name" -n "$secret_namespace" \
        -o jsonpath='{.data.CLICKHOUSE_PASSWORD}' 2>/dev/null | base64 -d 2>/dev/null || true
    )"
  fi

  if [ -z "${CLICKHOUSE_PASSWORD:-}" ]; then
    echo "ERROR: CLICKHOUSE_PASSWORD is required. Set it or make ${secret_namespace}/${secret_name} available." >&2
    exit 1
  fi

  export CLICKHOUSE_PASSWORD
}
