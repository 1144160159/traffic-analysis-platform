#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-asset-discovery-coverage}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-asset-discovery-coverage}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"
SITE_ASSET_INVENTORY_JSON="${SITE_ASSET_INVENTORY_JSON:-}"
MIN_DISCOVERY_COVERAGE_PCT="${MIN_DISCOVERY_COVERAGE_PCT:-95}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"
PG_NAMESPACE="${PG_NAMESPACE:-databases}"
PG_POD="${PG_POD:-postgres-primary-0}"
PG_SECRET_NAMESPACE="${PG_SECRET_NAMESPACE:-traffic-analysis}"
PG_SECRET_NAME="${PG_SECRET_NAME:-traffic-credentials}"
PG_SECRET_KEY="${PG_SECRET_KEY:-PG_PASSWORD}"

REPORT="$LOG_DIR/live-asset-discovery-coverage-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-asset-discovery-coverage-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
STABLE_JSON="$STABLE_DIR/asset-discovery-coverage-latest.json"
STABLE_MD="$STABLE_DIR/asset-discovery-coverage-latest.md"
STABLE_TEMPLATE="$STABLE_DIR/asset-discovery-site-inventory.template.json"

mkdir -p "$LOG_DIR" "$STABLE_DIR"
: >"$REPORT"

JWT_SECRET=""
PG_PASSWORD=""

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

kctl() {
  env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy "$KUBECTL" "$@"
}

json_log() {
  local phase="$1" name="$2" severity="$3" passed="$4" status="$5" detail="${6:-}" artifact="${7:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1200 "$file" | tr '\n' ' ' | sed -E 's/Bearer [A-Za-z0-9._-]+/Bearer <redacted>/g'
  fi
}

sql_escape() {
  printf "%s" "$1" | sed "s/'/''/g"
}

psql_exec() {
  local sql="$1"
  kctl -n "$PG_NAMESPACE" exec "$PG_POD" -- env PGPASSWORD="$PG_PASSWORD" \
    psql -q -U postgres -d traffic_platform -v ON_ERROR_STOP=1 -Atc "$sql"
}

make_token() {
  JWT_SECRET="$JWT_SECRET" TENANT="$TENANT" RUN_ID="$RUN_ID" python3 - <<'PY'
import base64
import hashlib
import hmac
import json
import os
import time
import uuid

def b64url(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")

now = int(time.time())
claims = {
    "iss": "traffic-auth-service",
    "sub": str(uuid.uuid4()),
    "iat": now,
    "exp": now + 1800,
    "jti": str(uuid.uuid4()),
    "user_id": str(uuid.uuid4()),
    "tenant_id": os.environ["TENANT"],
    "username": "codex-asset-coverage",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "asset:read", "asset:discover", "audit:read"],
    "token_type": "access",
    "session_id": "codex-asset-coverage-" + os.environ["RUN_ID"],
}
header = {"alg": "HS256", "typ": "JWT"}
signing_input = b".".join([
    b64url(json.dumps(header, separators=(",", ":")).encode()).encode(),
    b64url(json.dumps(claims, separators=(",", ":")).encode()).encode(),
])
signature = hmac.new(os.environ["JWT_SECRET"].encode(), signing_input, hashlib.sha256).digest()
print(signing_input.decode() + "." + b64url(signature))
PY
}

api_get() {
  local path="$1" out="$2" token="$3" expected="${4:-200}"
  local code
  code="$(curl --noproxy '*' -sS -m 20 "$APISIX$path" \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
    -o "$out" \
    -w '%{http_code}')"
  if [[ "$code" != "$expected" ]]; then
    json_log "api" "GET $path returned HTTP $expected" "blocker" false "http_$code" "$(trim_file "$out")" "$out"
    return 1
  fi
}

need_cmd curl
need_cmd git
need_cmd jq
need_cmd node
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
PG_PASSWORD="$(kctl -n "$PG_SECRET_NAMESPACE" get secret "$PG_SECRET_NAME" -o "jsonpath={.data.$PG_SECRET_KEY}" | base64 -d)"
ADMIN_TOKEN="$(make_token)"

api_get "/api/v1/assets?tenant_id=$TENANT&limit=100" "$LOG_DIR/assets-response.json" "$ADMIN_TOKEN" || true
if jq -e '.data | type == "array"' "$LOG_DIR/assets-response.json" >/dev/null 2>&1; then
  api_asset_total="$(jq '.pagination.total // (.data | length)' "$LOG_DIR/assets-response.json")"
  json_log "api" "Asset inventory API is queryable" "info" true "ok" "total=$api_asset_total" "assets-response.json"
else
  api_asset_total=0
  json_log "api" "Asset inventory API is queryable" "blocker" false "invalid" "$(trim_file "$LOG_DIR/assets-response.json")" "assets-response.json"
fi

api_get "/api/v1/assets/discovery/runs?tenant_id=$TENANT&limit=50" "$LOG_DIR/discovery-runs-response.json" "$ADMIN_TOKEN" || true
if jq -e '.data | type == "array"' "$LOG_DIR/discovery-runs-response.json" >/dev/null 2>&1; then
  api_run_count="$(jq '.data | length' "$LOG_DIR/discovery-runs-response.json")"
  json_log "api" "Discovery runs API is queryable" "info" true "ok" "runs=$api_run_count" "discovery-runs-response.json"
else
  api_run_count=0
  json_log "api" "Discovery runs API is queryable" "blocker" false "invalid" "$(trim_file "$LOG_DIR/discovery-runs-response.json")" "discovery-runs-response.json"
fi

api_get "/api/v1/assets/discovery/neighbors?tenant_id=$TENANT&limit=100" "$LOG_DIR/discovery-neighbors-response.json" "$ADMIN_TOKEN" || true
if jq -e '.data | type == "array"' "$LOG_DIR/discovery-neighbors-response.json" >/dev/null 2>&1; then
  api_neighbor_count="$(jq '.data | length' "$LOG_DIR/discovery-neighbors-response.json")"
  json_log "api" "Discovery neighbors API is queryable" "info" true "ok" "links=$api_neighbor_count" "discovery-neighbors-response.json"
else
  api_neighbor_count=0
  json_log "api" "Discovery neighbors API is queryable" "blocker" false "invalid" "$(trim_file "$LOG_DIR/discovery-neighbors-response.json")" "discovery-neighbors-response.json"
fi

tenant_sql="$(sql_escape "$TENANT")"
psql_exec "
WITH
asset_counts AS (
  SELECT
    count(*)::int AS total_assets,
    count(*) FILTER (WHERE source LIKE 'active:%')::int AS active_discovery_assets,
    count(*) FILTER (WHERE NULLIF(hostname, '') IS NULL)::int AS unnamed_assets,
    count(*) FILTER (WHERE NULLIF(vlan_id, '') IS NOT NULL)::int AS vlan_tagged_assets,
    count(*) FILTER (WHERE last_seen >= now() - interval '24 hours')::int AS seen_24h_assets
  FROM assets
  WHERE tenant_id = '$tenant_sql'
),
run_counts AS (
  SELECT
    count(*)::int AS total_runs,
    count(*) FILTER (WHERE status = 'completed')::int AS completed_runs,
    count(*) FILTER (WHERE status = 'failed')::int AS failed_runs,
    coalesce(sum(discovered_assets), 0)::int AS run_discovered_assets,
    coalesce(sum(discovered_links), 0)::int AS run_discovered_links
  FROM asset_discovery_runs
  WHERE tenant_id = '$tenant_sql'
),
topology_counts AS (
  SELECT
    count(*)::int AS topology_links,
    count(DISTINCT NULLIF(source_asset_id, ''))::int AS source_assets_with_links,
    count(DISTINCT NULLIF(neighbor_asset_id, ''))::int AS neighbor_assets_with_links
  FROM asset_topology_links
  WHERE tenant_id = '$tenant_sql'
),
latest_run AS (
  SELECT to_jsonb(r) AS payload
  FROM (
    SELECT run_id, mode, status, target_cidr, credential_id, discovered_assets, discovered_links, error_message, started_at, completed_at
    FROM asset_discovery_runs
    WHERE tenant_id = '$tenant_sql'
    ORDER BY started_at DESC
    LIMIT 1
  ) r
),
source_breakdown AS (
  SELECT coalesce(jsonb_object_agg(source, count), '{}'::jsonb) AS payload
  FROM (
    SELECT coalesce(NULLIF(source, ''), 'unknown') AS source, count(*)::int AS count
    FROM assets
    WHERE tenant_id = '$tenant_sql'
    GROUP BY 1
  ) s
)
SELECT jsonb_build_object(
  'tenant_id', '$tenant_sql',
  'asset_counts', to_jsonb(asset_counts),
  'run_counts', to_jsonb(run_counts),
  'topology_counts', to_jsonb(topology_counts),
  'latest_run', coalesce((SELECT payload FROM latest_run), '{}'::jsonb),
  'source_breakdown', (SELECT payload FROM source_breakdown)
)::text
FROM asset_counts, run_counts, topology_counts;
" >"$LOG_DIR/postgres-coverage-summary.json"

if jq -e '.asset_counts.total_assets >= 0 and .run_counts.total_runs >= 0 and .topology_counts.topology_links >= 0' "$LOG_DIR/postgres-coverage-summary.json" >/dev/null; then
  pg_total_assets="$(jq '.asset_counts.total_assets' "$LOG_DIR/postgres-coverage-summary.json")"
  pg_active_assets="$(jq '.asset_counts.active_discovery_assets' "$LOG_DIR/postgres-coverage-summary.json")"
  pg_completed_runs="$(jq '.run_counts.completed_runs' "$LOG_DIR/postgres-coverage-summary.json")"
  pg_links="$(jq '.topology_counts.topology_links' "$LOG_DIR/postgres-coverage-summary.json")"
  json_log "postgres" "Asset discovery coverage counters are queryable" "info" true "ok" "assets=$pg_total_assets active=$pg_active_assets completed_runs=$pg_completed_runs links=$pg_links" "postgres-coverage-summary.json"
else
  json_log "postgres" "Asset discovery coverage counters are queryable" "blocker" false "invalid" "$(trim_file "$LOG_DIR/postgres-coverage-summary.json")" "postgres-coverage-summary.json"
fi

psql_exec "
SELECT coalesce(jsonb_agg(jsonb_build_object(
  'asset_id', asset_id,
  'ip_address', ip_address,
  'mac_address', mac_address,
  'hostname', hostname,
  'source', source,
  'vlan_id', vlan_id,
  'switch_port', switch_port,
  'last_seen', last_seen
) ORDER BY last_seen DESC), '[]'::jsonb)::text
FROM assets
WHERE tenant_id = '$tenant_sql';
" >"$LOG_DIR/postgres-assets.json"

cat >"$STABLE_TEMPLATE" <<'JSON'
{
  "site": "example-campus",
  "snapshot_at": "2026-06-30T00:00:00+08:00",
  "assets": [
    {
      "asset_id": "SITE-ASSET-001",
      "mac_address": "00:11:22:33:44:55",
      "ip_address": "10.12.0.31",
      "hostname": "core-switch-01",
      "expected_type": "network-device",
      "location": "main-campus-core-room"
    }
  ]
}
JSON

FORMAL_CHECK_JSON="$LOG_DIR/site-asset-inventory-formal-check.json"
FORMAL_CHECK_MD="$LOG_DIR/site-asset-inventory-formal-check.md"
if node tests/e2e/site_asset_inventory_formal_check.mjs \
  --input "$SITE_ASSET_INVENTORY_JSON" \
  --output-json "$FORMAL_CHECK_JSON" \
  --output-md "$FORMAL_CHECK_MD" >/dev/null; then
  formal_detail="$(jq -r '"asset_count=\(.asset_count) checks=\(.passed)/\(.total)"' "$FORMAL_CHECK_JSON")"
  json_log "coverage" "Formal site inventory JSON passes strict validator" "info" true "ok" "$formal_detail" "$FORMAL_CHECK_JSON"
else
  formal_status="$(jq -r '.result // "blocked"' "$FORMAL_CHECK_JSON" 2>/dev/null || echo blocked)"
  formal_detail="$(jq -r '"blockers=\(.blockers // "unknown") warnings=\(.warnings // "unknown") checks=\(.passed // 0)/\(.total // 0)"' "$FORMAL_CHECK_JSON" 2>/dev/null || echo "formal check failed")"
  json_log "coverage" "Formal site inventory JSON passes strict validator" "blocker" false "$formal_status" "$formal_detail" "$FORMAL_CHECK_JSON"
fi

python3 - "$SITE_ASSET_INVENTORY_JSON" "$LOG_DIR/postgres-assets.json" "$LOG_DIR/site-inventory-normalized.json" "$LOG_DIR/coverage-match-report.json" "$MIN_DISCOVERY_COVERAGE_PCT" <<'PY'
import json
import sys
from pathlib import Path

inventory_path = Path(sys.argv[1]) if sys.argv[1] else None
live_assets_path = Path(sys.argv[2])
normalized_path = Path(sys.argv[3])
report_path = Path(sys.argv[4])
threshold = float(sys.argv[5])

def norm_mac(value):
    return "".join(ch for ch in str(value or "").lower() if ch in "0123456789abcdef")

def norm_text(value):
    return str(value or "").strip().lower()

live_assets = json.loads(live_assets_path.read_text(encoding="utf-8") or "[]")
live_by_mac = {norm_mac(item.get("mac_address")): item for item in live_assets if norm_mac(item.get("mac_address"))}
live_by_ip = {norm_text(item.get("ip_address")): item for item in live_assets if norm_text(item.get("ip_address"))}
live_by_hostname = {norm_text(item.get("hostname")): item for item in live_assets if norm_text(item.get("hostname"))}

payload = {
    "inventory_file": str(inventory_path) if inventory_path else "",
    "inventory_present": False,
    "inventory_review_required": False,
    "inventory_source": "",
    "inventory_usage": "",
    "expected_total": None,
    "matched_total": None,
    "coverage_rate": None,
    "threshold": threshold,
    "raw_threshold_passed": False,
    "threshold_passed": False,
    "matches": [],
    "missing": [],
}

if not inventory_path or not inventory_path.exists():
    normalized_path.write_text("[]\n", encoding="utf-8")
    report_path.write_text(json.dumps(payload, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
    sys.exit(0)

raw_text = inventory_path.read_text(encoding="utf-8")
raw_text_lower = raw_text.lower()
review_marker_detected = any(marker in raw_text_lower for marker in [
    "tbd",
    "review-template",
    "needs_site_owner_review",
    "bootstrap",
])
raw = json.loads(raw_text)
review_required = bool(raw.get("review_required")) if isinstance(raw, dict) else False
inventory_source = str(raw.get("source") or "") if isinstance(raw, dict) else ""
inventory_usage = str(raw.get("usage") or "") if isinstance(raw, dict) else ""
items = raw.get("assets", raw) if isinstance(raw, dict) else raw
if not isinstance(items, list):
    raise SystemExit("site inventory JSON must be an array or an object with an assets array")

normalized = []
for index, item in enumerate(items):
    if not isinstance(item, dict):
        continue
    normalized.append({
        "index": index,
        "asset_id": item.get("asset_id") or item.get("id") or "",
        "mac_address": item.get("mac_address") or item.get("mac") or "",
        "ip_address": item.get("ip_address") or item.get("ip") or "",
        "hostname": item.get("hostname") or item.get("name") or "",
        "expected_type": item.get("expected_type") or item.get("type") or "",
        "location": item.get("location") or "",
    })

matches = []
missing = []
for item in normalized:
    live = None
    match_key = ""
    mac_key = norm_mac(item["mac_address"])
    ip_key = norm_text(item["ip_address"])
    hostname_key = norm_text(item["hostname"])
    if mac_key and mac_key in live_by_mac:
        live = live_by_mac[mac_key]
        match_key = "mac_address"
    elif ip_key and ip_key in live_by_ip:
        live = live_by_ip[ip_key]
        match_key = "ip_address"
    elif hostname_key and hostname_key in live_by_hostname:
        live = live_by_hostname[hostname_key]
        match_key = "hostname"
    if live:
        matches.append({"expected": item, "match_key": match_key, "live_asset": live})
    else:
        missing.append(item)

expected_total = len(normalized)
matched_total = len(matches)
coverage_rate = (matched_total / expected_total * 100) if expected_total else 0.0
raw_threshold_passed = coverage_rate >= threshold
payload.update({
    "inventory_present": True,
    "inventory_review_required": review_required,
    "inventory_review_marker_detected": review_marker_detected,
    "inventory_source": inventory_source,
    "inventory_usage": inventory_usage,
    "expected_total": expected_total,
    "matched_total": matched_total,
    "coverage_rate": round(coverage_rate, 4),
    "raw_threshold_passed": raw_threshold_passed,
    "threshold_passed": raw_threshold_passed and not review_required and not review_marker_detected,
    "matches": matches[:100],
    "missing": missing[:100],
})
normalized_path.write_text(json.dumps(normalized, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
report_path.write_text(json.dumps(payload, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
PY

if jq -e '.inventory_present == true' "$LOG_DIR/coverage-match-report.json" >/dev/null; then
  coverage_rate="$(jq -r '.coverage_rate' "$LOG_DIR/coverage-match-report.json")"
  matched_total="$(jq -r '.matched_total' "$LOG_DIR/coverage-match-report.json")"
  expected_total="$(jq -r '.expected_total' "$LOG_DIR/coverage-match-report.json")"
  if jq -e '.inventory_review_required == true' "$LOG_DIR/coverage-match-report.json" >/dev/null; then
    inventory_source="$(jq -r '.inventory_source // "unknown"' "$LOG_DIR/coverage-match-report.json")"
    json_log "coverage" "Site inventory is approved for formal coverage" "blocker" false "review_required" "$matched_total/$expected_total coverage=$coverage_rate% source=$inventory_source; review-required bootstrap cannot close formal coverage" "coverage-match-report.json"
  elif jq -e '.inventory_review_marker_detected == true' "$LOG_DIR/coverage-match-report.json" >/dev/null; then
    json_log "coverage" "Site inventory is approved for formal coverage" "blocker" false "review_marker" "$matched_total/$expected_total coverage=$coverage_rate%; remove TBD/review-template/bootstrap markers before formal coverage" "coverage-match-report.json"
  elif jq -e '.threshold_passed == true' "$LOG_DIR/coverage-match-report.json" >/dev/null; then
    json_log "coverage" "Site inventory discovery coverage meets threshold" "info" true "ok" "$matched_total/$expected_total coverage=$coverage_rate% threshold=$MIN_DISCOVERY_COVERAGE_PCT%" "coverage-match-report.json"
  else
    json_log "coverage" "Site inventory discovery coverage meets threshold" "blocker" false "below_threshold" "$matched_total/$expected_total coverage=$coverage_rate% threshold=$MIN_DISCOVERY_COVERAGE_PCT%" "coverage-match-report.json"
  fi
else
  json_log "coverage" "Site expected asset inventory is provided" "blocker" false "missing" "set SITE_ASSET_INVENTORY_JSON to a site inventory JSON file; template=$STABLE_TEMPLATE" "asset-discovery-site-inventory.template.json"
fi

if [[ "${pg_completed_runs:-0}" -gt 0 ]]; then
  json_log "coverage" "Completed discovery run evidence exists" "info" true "ok" "completed_runs=$pg_completed_runs" "postgres-coverage-summary.json"
else
  json_log "coverage" "Completed discovery run evidence exists" "blocker" false "missing" "completed_runs=0" "postgres-coverage-summary.json"
fi

if [[ "${pg_links:-0}" -gt 0 ]]; then
  json_log "coverage" "Topology link evidence exists" "info" true "ok" "links=$pg_links" "postgres-coverage-summary.json"
else
  json_log "coverage" "Topology link evidence exists" "warn" false "missing" "links=0" "postgres-coverage-summary.json"
fi

PASSED="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
TOTAL="$(jq -s 'length' "$REPORT")"
BLOCKERS="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
WARNINGS="$(jq -s '[.[] | select(.passed != true and .severity == "warn")] | length' "$REPORT")"
RESULT="pass"
if [[ "$BLOCKERS" -gt 0 ]]; then
  RESULT="blocked"
fi

jq -n \
  --arg run_id "$RUN_ID" \
  --arg result "$RESULT" \
  --arg apisix "$APISIX" \
  --arg tenant "$TENANT" \
  --arg expected_inventory "$SITE_ASSET_INVENTORY_JSON" \
  --arg min_coverage_pct "$MIN_DISCOVERY_COVERAGE_PCT" \
  --argjson passed "$PASSED" \
  --argjson total "$TOTAL" \
  --argjson blockers "$BLOCKERS" \
  --argjson warnings "$WARNINGS" \
  --slurpfile checks "$REPORT" \
  --slurpfile pg "$LOG_DIR/postgres-coverage-summary.json" \
  --slurpfile coverage "$LOG_DIR/coverage-match-report.json" \
  --slurpfile formal_check "$FORMAL_CHECK_JSON" \
  '{
    run_id: $run_id,
    result: $result,
    apisix: $apisix,
    tenant_id: $tenant,
    expected_inventory: $expected_inventory,
    min_coverage_pct: ($min_coverage_pct | tonumber),
    passed: $passed,
    total: $total,
    blockers: $blockers,
    warnings: $warnings,
    postgres_coverage: $pg[0],
    coverage_match_report: $coverage[0],
    checks: $checks
  } + (if ($formal_check | length) > 0 then {formal_inventory_check: $formal_check[0]} else {} end)' >"$SUMMARY"

cp "$SUMMARY" "$STABLE_JSON"

{
  echo "# SNMP/LLDP 资产发现覆盖率报告"
  echo
  echo "- Run ID：\`$RUN_ID\`"
  echo "- 结果：\`$RESULT\`"
  echo "- APISIX：\`$APISIX\`"
  echo "- 检查数：$PASSED/$TOTAL passed，blockers=$BLOCKERS，warnings=$WARNINGS"
  echo "- 期望资产清单：\`${SITE_ASSET_INVENTORY_JSON:-未提供}\`"
  echo "- 覆盖率阈值：\`${MIN_DISCOVERY_COVERAGE_PCT}%\`"
  echo
  echo "## 证据"
  echo
  echo "- Summary：\`$SUMMARY\`"
  echo "- NDJSON：\`$REPORT\`"
  echo "- PostgreSQL coverage：\`$LOG_DIR/postgres-coverage-summary.json\`"
  echo "- Coverage match report：\`$LOG_DIR/coverage-match-report.json\`"
  echo "- Formal inventory check：\`$FORMAL_CHECK_JSON\`"
  echo "- Site inventory template：\`$STABLE_TEMPLATE\`"
  echo
  echo "## 口径"
  echo
  echo "本报告只读真实 APISIX 和 PostgreSQL，统计 assets、asset_discovery_runs 和 asset_topology_links，并在提供 SITE_ASSET_INVENTORY_JSON 后按 MAC/IP/hostname 计算现场期望资产发现覆盖率。未提供现场期望清单时结果必须保持 blocked，不能声明真实园区设备发现率达标。"
  echo
  echo "带有 \`review_required=true\` 的 bootstrap 草案只能用于现场清单起草和人工复核，即使匹配率达到阈值也必须保持 blocked，不能作为正式 SITE_ASSET_INVENTORY_JSON 关闭验收。"
} >"$LOCAL_REPORT"
cp "$LOCAL_REPORT" "$STABLE_MD"

echo "asset discovery coverage result: $RESULT"
echo "summary: $SUMMARY"
if [[ "$RESULT" != "pass" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
