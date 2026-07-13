#!/usr/bin/env bash
set -euo pipefail

APISIX="${APISIX:-http://10.0.5.8:30180}"
TENANT="${TENANT:-default}"
SITE_NAME="${SITE_NAME:-observed-live-campus}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-asset-inventory-bootstrap}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-asset-inventory-bootstrap}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/02-regression}"
KUBECTL="${KUBECTL:-kubectl}"
JWT_SECRET_NAMESPACE="${JWT_SECRET_NAMESPACE:-traffic-analysis}"
JWT_SECRET_NAME="${JWT_SECRET_NAME:-traffic-credentials}"
JWT_SECRET_KEY="${JWT_SECRET_KEY:-JWT_SECRET}"

REPORT="$LOG_DIR/live-asset-inventory-bootstrap-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-asset-inventory-bootstrap-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_JSON="$LOG_DIR/site-asset-inventory.bootstrap.json"
STABLE_BOOTSTRAP_JSON="$STABLE_DIR/asset-discovery-site-inventory.bootstrap-latest.json"
STABLE_BOOTSTRAP_MD="$STABLE_DIR/asset-discovery-site-inventory.bootstrap-latest.md"

mkdir -p "$LOG_DIR" "$STABLE_DIR"
: >"$REPORT"

JWT_SECRET=""

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
    "username": "codex-asset-inventory-bootstrap",
    "roles": ["admin"],
    "permissions": ["*", "admin:*", "asset:read"],
    "token_type": "access",
    "session_id": "codex-asset-inventory-bootstrap-" + os.environ["RUN_ID"],
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
  local path="$1" out="$2" token="$3"
  local code rc err_file
  err_file="$out.err"
  set +e
  code="$(curl --noproxy '*' -sS -m 20 "$APISIX$path" \
    -H "Authorization: Bearer $token" \
    -H "X-Tenant-ID: $TENANT" \
    -o "$out" \
    -w '%{http_code}' 2>"$err_file")"
  rc=$?
  set -e
  if [[ "$rc" -ne 0 ]]; then
    json_log "api" "GET $path" "blocker" false "curl_rc_$rc" "$(trim_file "$err_file")" "$(basename "$err_file")"
    return 1
  fi
  if [[ "$code" != 2* ]]; then
    json_log "api" "GET $path" "blocker" false "http_$code" "$(trim_file "$out")" "$(basename "$out")"
    return 1
  fi
  json_log "api" "GET $path" "info" true "$code" "$path" "$(basename "$out")"
}

finalize() {
  local passed total blockers warnings result asset_count
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi
  asset_count=0
  if [[ -s "$BOOTSTRAP_JSON" ]]; then
    asset_count="$(jq '.assets | length' "$BOOTSTRAP_JSON")"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg apisix "$APISIX" \
    --arg tenant "$TENANT" \
    --arg site "$SITE_NAME" \
    --arg bootstrap_json "$BOOTSTRAP_JSON" \
    --arg generated_at "$(date -Iseconds)" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson asset_count "$asset_count" \
    --slurpfile checks "$REPORT" \
    '{run_id:$run_id, result:$result, generated_at:$generated_at, apisix:$apisix, tenant_id:$tenant, site:$site, bootstrap_json:$bootstrap_json, asset_count:$asset_count, passed:$passed, total:$total, blockers:$blockers, warnings:$warnings, checks:$checks}' >"$SUMMARY"

  {
    echo "# Asset inventory bootstrap"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Site: \`$SITE_NAME\`"
    echo "- Asset draft count: $asset_count"
    echo "- Bootstrap JSON: \`$BOOTSTRAP_JSON\`"
    echo "- Summary: \`$SUMMARY\`"
    echo
    echo "This bootstrap is generated from live observed assets and is review-required. It is not a signed site inventory and must not be used to claim discovery coverage without site owner review."
  } >"$LOCAL_REPORT"

  if [[ -s "$BOOTSTRAP_JSON" ]]; then
    cp "$BOOTSTRAP_JSON" "$STABLE_BOOTSTRAP_JSON"
  fi
  cp "$LOCAL_REPORT" "$STABLE_BOOTSTRAP_MD"

  echo "asset inventory bootstrap result: $result"
  echo "summary: $SUMMARY"
  if [[ "$result" != "pass" ]]; then
    exit 1
  fi
}

need_cmd curl
need_cmd git
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

JWT_SECRET="$(kctl -n "$JWT_SECRET_NAMESPACE" get secret "$JWT_SECRET_NAME" -o "jsonpath={.data.$JWT_SECRET_KEY}" | base64 -d)"
ADMIN_TOKEN="$(make_token)"

if api_get "/api/v1/assets?tenant_id=$TENANT&limit=1000" "$LOG_DIR/assets-response.json" "$ADMIN_TOKEN"; then
  if jq -e '.data | type == "array"' "$LOG_DIR/assets-response.json" >/dev/null 2>&1; then
    live_count="$(jq '.data | length' "$LOG_DIR/assets-response.json")"
    json_log "inventory" "Live asset response can seed a review draft" "info" true "ok" "assets=$live_count" "assets-response.json"
  else
    json_log "inventory" "Live asset response can seed a review draft" "blocker" false "invalid_shape" "$(trim_file "$LOG_DIR/assets-response.json")" "assets-response.json"
  fi
fi

python3 - "$LOG_DIR/assets-response.json" "$BOOTSTRAP_JSON" "$SITE_NAME" <<'PY'
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

source_path = Path(sys.argv[1])
out_path = Path(sys.argv[2])
site = sys.argv[3]

payload = json.loads(source_path.read_text(encoding="utf-8"))
items = payload.get("data") or []
assets = []
seen = set()
for item in items:
    if not isinstance(item, dict):
        continue
    mac = item.get("mac_address") or item.get("mac") or ""
    ip = item.get("ip_address") or item.get("ip") or ""
    hostname = item.get("hostname") or item.get("name") or ""
    asset_id = item.get("asset_id") or item.get("id") or ""
    dedupe = (str(asset_id), str(mac).lower(), str(ip), str(hostname).lower())
    if dedupe in seen:
        continue
    seen.add(dedupe)
    assets.append({
        "asset_id": asset_id,
        "mac_address": mac,
        "ip_address": ip,
        "hostname": hostname,
        "expected_type": item.get("asset_type") or item.get("type") or item.get("category") or "observed-live-asset",
        "location": item.get("location") or "",
        "source": item.get("source") or "live-assets-api",
        "last_seen": item.get("last_seen") or "",
        "review_status": "needs_site_owner_review",
    })

bootstrap = {
    "site": site,
    "snapshot_at": datetime.now(timezone.utc).isoformat(),
    "source": "live_assets_api",
    "review_required": True,
    "usage": "Review and correct this draft before passing it as SITE_ASSET_INVENTORY_JSON to live_asset_discovery_coverage_report.sh.",
    "assets": assets,
}
out_path.write_text(json.dumps(bootstrap, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
PY

if jq -e '.review_required == true and (.assets | type == "array") and (.assets | length > 0)' "$BOOTSTRAP_JSON" >/dev/null; then
  json_log "inventory" "Review-required bootstrap inventory was generated" "info" true "ok" "assets=$(jq '.assets | length' "$BOOTSTRAP_JSON")" "$(basename "$BOOTSTRAP_JSON")"
else
  json_log "inventory" "Review-required bootstrap inventory was generated" "blocker" false "empty_or_invalid" "$(trim_file "$BOOTSTRAP_JSON")" "$(basename "$BOOTSTRAP_JSON")"
fi

finalize
