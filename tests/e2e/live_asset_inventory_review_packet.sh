#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-asset-inventory-review}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
INPUT_INVENTORY_JSON="${INPUT_INVENTORY_JSON:-doc/02_acceptance/02-regression/asset-discovery-site-inventory.bootstrap-latest.json}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/02-regression/asset-inventory-review}"

REPORT="$LOG_DIR/asset-inventory-review-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/asset-inventory-review-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
PACKET_DIR="$LOG_DIR/asset-inventory-review.packet"
STABLE_PACKET_DIR="$STABLE_DIR/latest"
STABLE_JSON="$STABLE_DIR/asset-inventory-review-latest.json"
STABLE_MD="$STABLE_DIR/asset-inventory-review-latest.md"

mkdir -p "$LOG_DIR" "$PACKET_DIR" "$STABLE_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
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

finalize() {
  local passed total blockers warnings result asset_count duplicate_count
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  elif [[ "$warnings" -gt 0 ]]; then
    result="warn"
  fi

  asset_count="$(jq -r '.asset_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  duplicate_count="$(jq -r '.duplicate_key_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg input_inventory "$INPUT_INVENTORY_JSON" \
    --arg packet_dir "$PACKET_DIR" \
    --arg stable_packet_dir "$STABLE_PACKET_DIR" \
    --argjson asset_count "$asset_count" \
    --argjson duplicate_key_count "$duplicate_count" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    --slurpfile review_summary "$PACKET_DIR/review-summary.json" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      input_inventory:$input_inventory,
      packet_dir:$packet_dir,
      stable_packet_dir:$stable_packet_dir,
      review_required:true,
      formal_gate_note:"site inventory review packet only; does not close asset discovery coverage",
      asset_count:$asset_count,
      duplicate_key_count:$duplicate_key_count,
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      review_summary:($review_summary[0] // {}),
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# Asset Inventory Review Packet"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Input inventory: \`$INPUT_INVENTORY_JSON\`"
    echo "- Asset rows: $asset_count"
    echo "- Duplicate key groups: $duplicate_count"
    echo "- Stable packet: \`$STABLE_PACKET_DIR\`"
    echo
    echo "This package converts the live observed asset bootstrap into review-ready files for the site owner. It is not an approved site inventory and cannot close the formal asset discovery coverage gate."
    echo
    echo "## Files"
    echo
    echo "- \`review-assets.csv\`: row-level review worklist"
    echo "- \`formal-site-inventory.template.json\`: template to fill after site-owner review"
    echo "- \`review-checklist.md\`: approval checklist and rerun command"
    echo "- \`review-summary.json\`: package metadata"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_PACKET_DIR"
  mkdir -p "$STABLE_PACKET_DIR"
  cp -R "$PACKET_DIR/." "$STABLE_PACKET_DIR/"
  cp "$SUMMARY" "$STABLE_JSON"
  cp "$LOCAL_REPORT" "$STABLE_MD"

  echo "asset inventory review packet result=$result summary=$SUMMARY"
  if [[ "$result" == "blocked" ]]; then
    exit 1
  fi
}

need_cmd git
need_cmd jq
need_cmd python3

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git status --short >"$LOG_DIR/git-status.txt"

if [[ -s "$INPUT_INVENTORY_JSON" ]]; then
  json_log "input" "input inventory exists" "info" true "ok" "$INPUT_INVENTORY_JSON" "$INPUT_INVENTORY_JSON"
else
  json_log "input" "input inventory exists" "blocker" false "missing" "$INPUT_INVENTORY_JSON" "$INPUT_INVENTORY_JSON"
  finalize
fi

python3 - "$INPUT_INVENTORY_JSON" "$PACKET_DIR" "$RUN_ID" <<'PY'
import csv
import json
import sys
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path

inventory_path = Path(sys.argv[1])
packet_dir = Path(sys.argv[2])
run_id = sys.argv[3]
packet_dir.mkdir(parents=True, exist_ok=True)

raw = json.loads(inventory_path.read_text(encoding="utf-8"))
if isinstance(raw, dict):
    assets = raw.get("assets") or []
    site = raw.get("site") or "review-required-site"
    source = raw.get("source") or ""
    source_review_required = bool(raw.get("review_required"))
else:
    assets = raw
    site = "review-required-site"
    source = ""
    source_review_required = True

if not isinstance(assets, list):
    raise SystemExit("input inventory must be an array or an object with assets array")

def norm(value: object) -> str:
    return str(value or "").strip()

def norm_mac(value: object) -> str:
    return "".join(ch for ch in str(value or "").lower() if ch in "0123456789abcdef")

rows = []
duplicate_keys = defaultdict(list)
for index, item in enumerate(assets, start=1):
    if not isinstance(item, dict):
        continue
    mac = norm(item.get("mac_address") or item.get("mac"))
    ip = norm(item.get("ip_address") or item.get("ip"))
    hostname = norm(item.get("hostname") or item.get("name"))
    asset_id = norm(item.get("asset_id") or item.get("id"))
    key_parts = [part for part in [norm_mac(mac), ip.lower(), hostname.lower()] if part]
    key = "|".join(key_parts) if key_parts else f"row-{index}"
    duplicate_keys[key].append(index)
    rows.append({
        "review_index": index,
        "asset_id": asset_id,
        "mac_address": mac,
        "ip_address": ip,
        "hostname": hostname,
        "expected_type": norm(item.get("expected_type") or item.get("asset_type") or item.get("type") or "observed-live-asset"),
        "location": norm(item.get("location")),
        "source": norm(item.get("source") or source or "live-assets-api"),
        "last_seen": norm(item.get("last_seen")),
        "current_review_status": norm(item.get("review_status") or "needs_site_owner_review"),
        "site_owner_decision": "TBD: approve | modify | exclude",
        "approved_asset_id": asset_id,
        "approved_hostname": hostname,
        "approved_location": norm(item.get("location")),
        "site_owner_comment": "TBD",
    })

duplicate_groups = [
    {"match_key": key, "review_indexes": indexes}
    for key, indexes in duplicate_keys.items()
    if len(indexes) > 1 and not key.startswith("row-")
]

with (packet_dir / "review-assets.csv").open("w", encoding="utf-8", newline="") as handle:
    fieldnames = [
        "review_index",
        "asset_id",
        "mac_address",
        "ip_address",
        "hostname",
        "expected_type",
        "location",
        "source",
        "last_seen",
        "current_review_status",
        "site_owner_decision",
        "approved_asset_id",
        "approved_hostname",
        "approved_location",
        "site_owner_comment",
    ]
    writer = csv.DictWriter(handle, fieldnames=fieldnames)
    writer.writeheader()
    writer.writerows(rows)

formal_template = {
    "site": site,
    "snapshot_at": datetime.now(timezone.utc).isoformat(),
    "source": "site-owner-review-template",
    "review_required": False,
    "approved_by": "TBD",
    "approved_at": "TBD",
    "approval_evidence": "TBD",
    "usage": "Fill site_owner_decision for each row in review-assets.csv, remove excluded rows, replace all TBD fields, and only then use this as SITE_ASSET_INVENTORY_JSON.",
    "assets": [
        {
            "asset_id": row["approved_asset_id"],
            "mac_address": row["mac_address"],
            "ip_address": row["ip_address"],
            "hostname": row["approved_hostname"],
            "expected_type": row["expected_type"],
            "location": row["approved_location"],
        }
        for row in rows
    ],
}
(packet_dir / "formal-site-inventory.template.json").write_text(
    json.dumps(formal_template, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)

summary = {
    "run_id": run_id,
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "input_inventory": str(inventory_path),
    "site": site,
    "input_source": source,
    "input_review_required": source_review_required,
    "review_required": True,
    "asset_count": len(rows),
    "duplicate_key_count": len(duplicate_groups),
    "duplicate_groups": duplicate_groups[:50],
    "outputs": {
        "review_assets_csv": str(packet_dir / "review-assets.csv"),
        "formal_template_json": str(packet_dir / "formal-site-inventory.template.json"),
        "review_checklist": str(packet_dir / "review-checklist.md"),
    },
}
(packet_dir / "review-summary.json").write_text(
    json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)

with (packet_dir / "review-checklist.md").open("w", encoding="utf-8") as handle:
    handle.write("# Site Asset Inventory Review Checklist\n\n")
    handle.write("This packet is generated from live observed assets and is review-required.\n\n")
    handle.write("## Review Steps\n\n")
    handle.write("1. Open `review-assets.csv` and set `site_owner_decision` to `approve`, `modify`, or `exclude` for every row.\n")
    handle.write("2. Fill `approved_hostname`, `approved_location`, and `site_owner_comment` for modified rows.\n")
    handle.write("3. Remove excluded rows from `formal-site-inventory.template.json`.\n")
    handle.write("4. Replace `approved_by`, `approved_at`, and `approval_evidence`; no `TBD`, `review-template`, `bootstrap`, or `needs_site_owner_review` markers may remain.\n")
    handle.write("5. Validate the formal JSON before running coverage:\n\n")
    handle.write("```bash\n")
    handle.write("node tests/e2e/site_asset_inventory_formal_check.mjs \\\n")
    handle.write("  --input /path/to/site-owner-approved-assets.json\n")
    handle.write("```\n\n")
    handle.write("6. Rerun the formal gate:\n\n")
    handle.write("```bash\n")
    handle.write("SITE_ASSET_INVENTORY_JSON=/path/to/site-owner-approved-assets.json \\\n")
    handle.write("MIN_DISCOVERY_COVERAGE_PCT=95 \\\n")
    handle.write("ALLOW_BLOCKERS=false \\\n")
    handle.write("tests/e2e/live_asset_discovery_coverage_report.sh\n")
    handle.write("```\n")

if not rows:
    raise SystemExit("input inventory has no asset rows")
PY

if jq -e '.asset_count > 0 and .review_required == true' "$PACKET_DIR/review-summary.json" >/dev/null; then
  asset_count="$(jq -r '.asset_count' "$PACKET_DIR/review-summary.json")"
  duplicate_count="$(jq -r '.duplicate_key_count' "$PACKET_DIR/review-summary.json")"
  json_log "packet" "review packet generated" "info" true "ok" "assets=$asset_count duplicate_key_groups=$duplicate_count" "$PACKET_DIR/review-summary.json"
else
  json_log "packet" "review packet generated" "blocker" false "invalid" "$PACKET_DIR/review-summary.json" "$PACKET_DIR/review-summary.json"
fi

if [[ -s "$PACKET_DIR/review-assets.csv" && -s "$PACKET_DIR/formal-site-inventory.template.json" && -s "$PACKET_DIR/review-checklist.md" ]]; then
  json_log "packet" "review worklist and templates written" "info" true "ok" "$PACKET_DIR" "$PACKET_DIR/review-assets.csv"
else
  json_log "packet" "review worklist and templates written" "blocker" false "missing_outputs" "$PACKET_DIR" "$PACKET_DIR"
fi

finalize
