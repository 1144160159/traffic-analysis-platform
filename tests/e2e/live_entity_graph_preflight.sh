#!/usr/bin/env bash
set -euo pipefail

unset HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy
export NO_PROXY="10.0.5.8,127.0.0.1,localhost"

BASE_URL="${UI_BASE_URL:-http://10.0.5.8:30180}"
KUBECTL="${KUBECTL:-kubectl}"
KUBECTL_REMOTE_HOST="${KUBECTL_REMOTE_HOST:-}"
KUBECTL_REMOTE_CONFIG="${KUBECTL_REMOTE_CONFIG:-/tmp/codex-nebula-kubeconfig}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TMP_DIR"' EXIT

kctl() {
  if [[ -n "$KUBECTL_REMOTE_HOST" ]]; then
    ssh "$KUBECTL_REMOTE_HOST" kubectl --kubeconfig="$KUBECTL_REMOTE_CONFIG" "$@"
  else
    "$KUBECTL" "$@"
  fi
}

base64url() {
  base64 | tr -d '\n=' | tr '+/' '-_'
}

JWT_SECRET="$(kctl -n traffic-analysis get secret traffic-credentials -o jsonpath='{.data.JWT_SECRET}' | base64 -d)"

make_token() {
  local tenant="$1"
  local permissions="${2:-[\"*\",\"admin:*\",\"graph:read\"]}"
  local now exp header payload input signature
  now="$(date +%s)"
  exp="$((now + 1800))"
  header="$(printf '%s' '{"alg":"HS256","typ":"JWT"}' | base64url)"
  payload="$(printf '{"iss":"traffic-auth-service","sub":"%s","jti":"%s","user_id":"%s","tenant_id":"%s","username":"entity-graph-preflight","roles":["analyst"],"permissions":%s,"token_type":"access","session_id":"entity-graph-preflight-%s","iat":%s,"exp":%s}' "$(cat /proc/sys/kernel/random/uuid)" "$(cat /proc/sys/kernel/random/uuid)" "$(cat /proc/sys/kernel/random/uuid)" "$tenant" "$permissions" "$(cat /proc/sys/kernel/random/uuid)" "$now" "$exp" | base64url)"
  input="${header}.${payload}"
  signature="$(printf '%s' "$input" | openssl dgst -sha256 -hmac "$JWT_SECRET" -binary | base64url)"
  printf '%s.%s' "$input" "$signature"
}

unauthorized_status="$(curl -sS -o "$TMP_DIR/unauthorized.json" -w '%{http_code}' "$BASE_URL/api/v1/graph/workbench")"
test "$unauthorized_status" = "401"

default_token="$(make_token default)"
curl -fsS -H "Authorization: Bearer $default_token" \
  "$BASE_URL/api/v1/graph/workbench?center_id=host%3A10.20.4.18&depth=2" > "$TMP_DIR/default.json"

jq -e '.data.meta.source == "nebula_graph"' "$TMP_DIR/default.json" >/dev/null
jq -e '.data.meta.node_count == 11 and .data.meta.edge_count == 13' "$TMP_DIR/default.json" >/dev/null
jq -e '.data.meta.node_limit > 0 and .data.meta.query_duration_ms >= 0 and .data.meta.cache_hit_rate == "N/A" and .data.meta.cache_applicable == false and .data.meta.data_origin == "nebula_graph_persisted_projection" and .data.meta.time_range == "24h" and (.data.meta.slow_query | type) == "boolean"' "$TMP_DIR/default.json" >/dev/null
jq -e '([.data.graph.nodes[].entity_type] | unique | sort) == ["account","alert","domain","evidence","host","ip","service"]' "$TMP_DIR/default.json" >/dev/null
jq -e '([.data.graph.edges[].evidence_id | select(length > 0)] | unique | length) >= 4' "$TMP_DIR/default.json" >/dev/null

curl -fsS -H "Authorization: Bearer $default_token" \
  "$BASE_URL/api/v1/graph/workbench?center_id=host%3A10.20.4.18&entity_type=account&depth=2&site=main&time_range=all" > "$TMP_DIR/account-filter.json"
jq -e '.data.meta.node_count == 2 and .data.meta.edge_count == 1 and ([.data.graph.nodes[].entity_type] | sort) == ["account","host"]' "$TMP_DIR/account-filter.json" >/dev/null

for spec in \
  'shortest|ip%3A185.234.15.23|host%3A10.20.4.20|2|通信|service|HTTPS' \
  'attack|ip%3A185.234.15.23|alert%3AALERT-20260620-1287|2|关联告警|attack_stage|初始访问' \
  'communication|domain%3Aerp.corp.edu.cn|host%3A10.20.4.20|2|DNS解析|frequency|221' \
  'account|account%3Abiz_admin|host%3A10.20.4.18|1|登录|identity_label|高权限'; do
  IFS='|' read -r mode source target expected_length relation attribute expected_attribute <<<"$spec"
  curl -fsS -H "Authorization: Bearer $default_token" \
    "$BASE_URL/api/v1/graph/workbench/path?mode=$mode&source_id=$source&target_id=$target&anchor_id=host%3A10.20.4.18&max_depth=6&site=main&entity_type=all&time_range=24h" > "$TMP_DIR/path-$mode.json"
  jq -e --arg mode "$mode" --argjson expected_length "$expected_length" --arg relation "$relation" --arg attribute "$attribute" --arg expected "$expected_attribute" \
    '.data.path.mode == $mode and .data.path.length == $expected_length and any(.data.path.edges[]; .relation_type == $relation) and any(.data.path.edges[]; (.attributes[$attribute] | tostring) == $expected)' \
    "$TMP_DIR/path-$mode.json" >/dev/null
done

curl -fsS -H "Authorization: Bearer $default_token" \
  "$BASE_URL/api/v1/graph/workbench/path?mode=account&source_id=account%3Abiz_admin&target_id=host%3A10.20.4.18&anchor_id=host%3A10.20.4.18&max_depth=6&site=main&entity_type=account&time_range=24h" > "$TMP_DIR/path-account-filtered.json"
jq -e '.data.path.length == 1 and .data.path.node_ids == ["account:biz_admin", "host:10.20.4.18"] and .data.path.edges[0].relation_type == "登录"' "$TMP_DIR/path-account-filtered.json" >/dev/null

curl -fsS -H "Authorization: Bearer $default_token" \
  "$BASE_URL/api/v1/graph/workbench/path?mode=attack&source_id=alert%3AALERT-20260620-1287&target_id=ip%3A185.234.15.23&anchor_id=host%3A10.20.4.18&max_depth=6&site=main&entity_type=all&time_range=24h" > "$TMP_DIR/reverse-attack.json"
jq -e '.data.path.length == 0 and (.data.path.edges | length) == 0' "$TMP_DIR/reverse-attack.json" >/dev/null

low_scope_token="$(make_token default '["alert:read"]')"
low_scope_status="$(curl -sS -o "$TMP_DIR/low-scope.json" -w '%{http_code}' -H "Authorization: Bearer $low_scope_token" "$BASE_URL/api/v1/graph/workbench")"
test "$low_scope_status" = "403"

isolated_token="$(make_token entity-graph-empty-tenant)"
curl -fsS -H "Authorization: Bearer $isolated_token" \
  "$BASE_URL/api/v1/graph/workbench?center_id=host%3A10.20.4.18&tenant_id=default" > "$TMP_DIR/isolated.json"
jq -e '.data.meta.node_count == 0 and .data.meta.edge_count == 0' "$TMP_DIR/isolated.json" >/dev/null
curl -fsS -H "Authorization: Bearer $isolated_token" \
  "$BASE_URL/api/v1/graph/workbench/path?mode=shortest&source_id=ip%3A185.234.15.23&target_id=host%3A10.20.4.20&anchor_id=host%3A10.20.4.18&max_depth=6&site=main&entity_type=all&time_range=24h" > "$TMP_DIR/isolated-path.json"
jq -e '.data.path.length == 0 and (.data.path.edges | length) == 0' "$TMP_DIR/isolated-path.json" >/dev/null

collision_token="$(make_token entity-graph-collision-tenant)"
curl -fsS -H "Authorization: Bearer $collision_token" \
  "$BASE_URL/api/v1/graph/workbench?center_id=host%3A10.20.4.18&depth=3&site=all&time_range=all" > "$TMP_DIR/collision-tenant.json"
jq -e '.data.meta.node_count == 1 and .data.meta.edge_count == 0 and .data.graph.nodes[0].entity_id == "host:10.20.4.18" and .data.graph.nodes[0].label == "租户隔离验证主机"' "$TMP_DIR/collision-tenant.json" >/dev/null

nebula_services_json="$(kctl -n middleware get svc nebula-meta nebula-storage-headless -o json)"
jq -e 'all(.items[]; .spec.clusterIP == "None" and .spec.publishNotReadyAddresses == true)' <<<"$nebula_services_json" >/dev/null

nebula_pods_json="$(kctl -n middleware get pods -l app=nebula -o json)"
for component in meta graph storage; do
  jq -e --arg component "$component" \
    '[.items[] | select(.metadata.labels.component == $component)] | length == 3' \
    <<<"$nebula_pods_json" >/dev/null
  jq -e --arg component "$component" \
    'all(.items[] | select(.metadata.labels.component == $component); any(.status.conditions[]?; .type == "Ready" and .status == "True") and ([.status.containerStatuses[]?.restartCount] | add // 0) == 0)' \
    <<<"$nebula_pods_json" >/dev/null
done

jq -n \
  --arg generated_at "$(date --iso-8601=seconds)" \
  --arg base_url "$BASE_URL" \
  --arg unauthorized_status "$unauthorized_status" \
  --arg low_scope_status "$low_scope_status" \
  --argjson graph "$(jq '.data | {meta, graph: {center_id: .graph.center_id, node_types: ([.graph.nodes[].entity_type] | unique | sort), evidence_ids: ([.graph.edges[].evidence_id | select(length > 0)] | unique | sort)}}' "$TMP_DIR/default.json")" \
  --argjson isolated "$(jq '.data.meta' "$TMP_DIR/isolated.json")" \
  --argjson collision_tenant "$(jq '.data | {meta, nodes:.graph.nodes, edges:.graph.edges}' "$TMP_DIR/collision-tenant.json")" \
  --argjson account_filter "$(jq '.data | {meta, node_ids: [.graph.nodes[].entity_id], relation_ids: [.graph.edges[].relation_id]}' "$TMP_DIR/account-filter.json")" \
  --argjson paths "$(jq -s 'map(.data.path | {mode, source_id, target_id, length, relations: [.edges[].relation_type], attributes: [.edges[].attributes], evidence_ids})' "$TMP_DIR/path-shortest.json" "$TMP_DIR/path-attack.json" "$TMP_DIR/path-communication.json" "$TMP_DIR/path-account.json")" \
  --argjson filtered_account_path "$(jq '.data.path' "$TMP_DIR/path-account-filtered.json")" \
  --argjson nebula_cluster "$(jq '[.items[] | {name:.metadata.name, component:.metadata.labels.component, ready:any(.status.conditions[]?; .type == "Ready" and .status == "True"), restarts:([.status.containerStatuses[]?.restartCount] | add // 0)}] | sort_by(.component,.name)' <<<"$nebula_pods_json")" \
  --argjson reverse_attack "$(jq '.data.path' "$TMP_DIR/reverse-attack.json")" \
  --argjson isolated_path "$(jq '.data.path' "$TMP_DIR/isolated-path.json")" \
  --argjson headless_services "$(jq '[.items[] | {name:.metadata.name, clusterIP:.spec.clusterIP, publishNotReadyAddresses:.spec.publishNotReadyAddresses}]' <<<"$nebula_services_json")" \
  '{result:"pass", generated_at:$generated_at, base_url:$base_url, checks:{unauthorized_status:$unauthorized_status, low_scope_status:$low_scope_status, authenticated_graph:$graph, account_filter:$account_filter, filtered_account_path:$filtered_account_path, path_modes:$paths, reverse_attack_is_rejected:$reverse_attack, cross_tenant_empty_query_isolated:$isolated, cross_tenant_path_isolated:$isolated_path, same_entity_id_tenant_collision_isolated:$collision_tenant, headless_services:$headless_services, nebula_cluster:$nebula_cluster}}' \
  | tee doc/02_acceptance/02-regression/entity-graph-live-preflight-r580.json
