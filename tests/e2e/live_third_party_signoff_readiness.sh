#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-third-party-signoff-readiness}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
THIRD_PARTY_DIR="${THIRD_PARTY_DIR:-doc/02_acceptance/08-third-party}"
STABLE_DIR="${STABLE_DIR:-$THIRD_PARTY_DIR/readiness}"

REPORT="$LOG_DIR/third-party-signoff-readiness-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/third-party-signoff-readiness-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/signoff-readiness.bootstrap"
STABLE_BOOTSTRAP_DIR="$STABLE_DIR/latest"

mkdir -p "$LOG_DIR" "$BOOTSTRAP_DIR" "$STABLE_DIR"
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

check_file() {
  local path="$1" label="$2"
  if [[ -s "$path" ]]; then
    json_log "templates" "$label" "info" true "ok" "$path" "$path"
  else
    json_log "templates" "$label" "blocker" false "missing" "$path" "$path"
  fi
}

finalize() {
  local passed total blockers warnings result tbd_count upstream_blockers
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  tbd_count="$(jq -r '.template_placeholders.total_tbd // 0' "$BOOTSTRAP_DIR/readiness-manifest.bootstrap.json" 2>/dev/null || echo 0)"
  upstream_blockers="$(jq -r '[.evidence_inputs[]? | select((.result // "") != "pass" or ((.blockers // 0) > 0))] | length' "$BOOTSTRAP_DIR/evidence-ledger.bootstrap.json" 2>/dev/null || echo 0)"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg third_party_dir "$THIRD_PARTY_DIR" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson tbd_count "$tbd_count" \
    --argjson upstream_blockers "$upstream_blockers" \
    --slurpfile checks "$REPORT" \
    --slurpfile owner_summary "$BOOTSTRAP_DIR/placeholder-owner-summary.bootstrap.json" \
    --slurpfile evidence_ledger "$BOOTSTRAP_DIR/evidence-ledger.bootstrap.json" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      third_party_dir:$third_party_dir,
      bootstrap_dir:$bootstrap_dir,
      stable_bootstrap_dir:$stable_bootstrap_dir,
      review_required:true,
      formal_gate_note:"signoff readiness bootstrap only; does not replace user signature, third-party report, or pilot acceptance",
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      template_tbd_count:$tbd_count,
      template_placeholder_owners:($owner_summary[0].by_owner // {}),
      template_placeholder_owner_summary:($owner_summary[0] // {}),
      upstream_blocked_or_nonpass_inputs:$upstream_blockers,
      evidence_inputs:($evidence_ledger[0].evidence_inputs // []),
      evidence_input_run_ids:(
        ($evidence_ledger[0].evidence_inputs // [])
        | map({key:.key, value:(.run_id // "")})
        | from_entries
      ),
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# Third-party Signoff Readiness"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Template TBD count: $tbd_count"
    echo "- Upstream non-pass or blocked evidence inputs: $upstream_blockers"
    echo "- Bootstrap dir: \`$BOOTSTRAP_DIR\`"
    echo "- Stable bootstrap dir: \`$STABLE_BOOTSTRAP_DIR\`"
    echo "- Summary: \`$SUMMARY\`"
    echo
    echo "This bootstrap organizes materials for user acceptance, pilot reporting, economic-benefit review, IPR indexing, and third-party package preparation. It is review-required and does not satisfy the formal signoff gate."
    echo
    echo "## Boundary"
    echo
    echo "A passing readiness bootstrap means the package can be reviewed and filled. It does not mean the user signed, a third party attested, 10 x 100Gbps / 512Mpps passed, 95%/5% passed, production security passed, or HA RTO/RPO passed."
    echo
    echo "## Placeholder Owners"
    echo
    jq -r '.template_placeholder_owners | to_entries | sort_by(.key)[] | "- " + .key + ": " + (.value|tostring)' "$SUMMARY"
    echo
    echo "## Evidence Inputs"
    echo
    jq -r '.evidence_inputs[]? | "- " + .key + ": " + ((.run_id // "missing")|tostring) + " / result=" + ((.result // "missing")|tostring) + " / blockers=" + ((.blockers // "unknown")|tostring)' "$SUMMARY"
    echo
    echo "## Failed Checks"
    echo
    jq -r '.checks[] | select(.passed|not) | "- [" + .severity + "] " + .name + ": " + .detail' "$SUMMARY"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/third-party-signoff-readiness-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/third-party-signoff-readiness-latest.json"

  echo "third-party-signoff-readiness result=$result summary=$SUMMARY"
  if [[ "$result" == "blocked" ]]; then
    exit 1
  fi
}

need_cmd git
need_cmd jq
need_cmd python3

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"

check_file "$THIRD_PARTY_DIR/README.md" "third-party README present"
check_file "$THIRD_PARTY_DIR/pilot-package-manifest.json" "pilot package manifest present"
check_file "$THIRD_PARTY_DIR/pilot-deployment-proof.md" "pilot deployment proof template present"
check_file "$THIRD_PARTY_DIR/demo-script.md" "demo script template present"
check_file "$THIRD_PARTY_DIR/pilot-weekly-report-template.md" "pilot weekly report template present"
check_file "$THIRD_PARTY_DIR/economic-benefit.md" "economic benefit template present"
check_file "$THIRD_PARTY_DIR/user-acceptance-signoff.md" "user acceptance signoff template present"
check_file "$THIRD_PARTY_DIR/ipr-index.md" "IPR index template present"

python3 - "$THIRD_PARTY_DIR" "$BOOTSTRAP_DIR" "$RUN_ID" <<'PY'
import csv
import json
import re
import sys
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path

third_party_dir = Path(sys.argv[1])
out_dir = Path(sys.argv[2])
run_id = sys.argv[3]
generated_at = datetime.now(timezone.utc).astimezone().isoformat()
out_dir.mkdir(parents=True, exist_ok=True)

manifest_path = third_party_dir / "pilot-package-manifest.json"
manifest = {}
if manifest_path.is_file():
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

template_paths = [
    third_party_dir / "README.md",
    third_party_dir / "pilot-deployment-proof.md",
    third_party_dir / "demo-script.md",
    third_party_dir / "pilot-weekly-report-template.md",
    third_party_dir / "economic-benefit.md",
    third_party_dir / "user-acceptance-signoff.md",
    third_party_dir / "ipr-index.md",
]

def classify_placeholder(line):
    if any(token in line for token in ["待第三方", "第三方", "盲测", "CNAS", "95%/5%", "检测质量"]):
        return "third_party_lab", "third-party quality or blind-test evidence"
    if any(token in line for token in ["待专项验证", "100Gbps", "512Mpps", "压测", "硬件窗口"]):
        return "performance_lab", "hardware performance validation"
    if any(token in line for token in ["待维护窗口", "RTO/RPO", "破坏性演练"]):
        return "maintenance_window", "scheduled HA or destructive drill"
    if any(token in line for token in ["待现场", "现场", "TAP/SPAN", "site values", "资源配额", "NTP/PTP", "数据授权", "试点站点"]):
        return "site_operations", "site deployment or operation confirmation"
    if any(token in line for token in ["待用户", "用户代表", "用户单位", "用户联系人", "试点单位", "签认", "签字", "签认日期"]):
        return "user_signoff", "user acceptance or signature"
    if any(token in line for token in ["承建单位", "项目经理", "实施负责人"]):
        return "project_team", "project delivery ownership"
    return "project_review", "review fill-in value"

placeholder_rows = []
for path in template_paths:
    if not path.is_file():
        continue
    for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), start=1):
        count = len(re.findall(r"\bTBD\b|待填写|待专项验证|待第三方|待维护窗口|待用户|待签认|待外部|待现场|未提供|未签字|待签字|待确认|待恢复|待 policy-capable CNI", line))
        if count:
            owner, owner_reason = classify_placeholder(line)
            placeholder_rows.append({
                "file": str(path),
                "line": lineno,
                "count": count,
                "owner": owner,
                "owner_reason": owner_reason,
                "text": line[:240],
            })

def read_json(path):
    try:
        if path.is_file() and path.stat().st_size > 0:
            return json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        return {"_read_error": str(exc)}
    return None

evidence_inputs = []
for key, path_text in (manifest.get("evidence_inputs") or {}).items():
    path = Path(path_text)
    payload = read_json(path)
    item = {
        "key": key,
        "path": str(path),
        "exists": path.is_file() and path.stat().st_size > 0 if path.exists() else False,
        "run_id": "",
        "result": "",
        "passed": None,
        "total": None,
        "blockers": None,
        "warnings": None,
    }
    if isinstance(payload, dict):
        checks = payload.get("checks") if isinstance(payload.get("checks"), list) else []
        derived_passed = len([check for check in checks if check.get("passed") is True]) if checks else None
        derived_total = len(checks) if checks else None
        derived_blockers = len([
            check for check in checks
            if check.get("passed") is not True and check.get("severity") == "blocker"
        ]) if checks else None
        derived_warnings = len([
            check for check in checks
            if check.get("passed") is not True and check.get("severity") in {"warn", "warning"}
        ]) if checks else None
        result = payload.get("result", "")
        if result == "passed":
            result = "pass"
        blockers = payload.get("blockers", derived_blockers)
        if key == "baseline" and not result and payload.get("run_id") and payload.get("evidence_runs"):
            result = "pass"
            blockers = 0
        if result == "pass" and blockers is None:
            blockers = 0
        item.update({
            "run_id": payload.get("run_id", ""),
            "result": result,
            "passed": payload.get("passed", payload.get("passed_checks", derived_passed)),
            "total": payload.get("total", payload.get("total_checks", derived_total)),
            "blockers": blockers,
            "warnings": payload.get("warnings", derived_warnings),
        })
    evidence_inputs.append(item)

owner_counts = Counter()
file_counts = Counter()
owner_file_counts = {}
for row in placeholder_rows:
    owner_counts[row["owner"]] += row["count"]
    file_counts[row["file"]] += row["count"]
    owner_file_counts.setdefault(row["file"], Counter())[row["owner"]] += row["count"]

placeholder_owner_summary = {
    "run_id": run_id,
    "generated_at": generated_at,
    "total_tbd": sum(row["count"] for row in placeholder_rows),
    "by_owner": dict(sorted(owner_counts.items())),
    "by_file": [
        {
            "file": file,
            "total_tbd": file_counts[file],
            "by_owner": dict(sorted(owner_file_counts[file].items())),
        }
        for file in sorted(file_counts)
    ],
}

evidence_input_run_ids = {
    item["key"]: item.get("run_id", "")
    for item in evidence_inputs
}

claim_policy = manifest.get("claim_policy", {})
readiness_manifest = {
    "package_id": "third_party_signoff_readiness_bootstrap",
    "status": "bootstrap_review_required",
    "review_required": True,
    "run_id": run_id,
    "generated_at": generated_at,
    "formal_gate_note": "Do not use this bootstrap as user acceptance, CNAS/third-party report, pilot signoff, or economic benefit confirmation.",
    "source_manifest": str(manifest_path),
    "template_placeholders": {
        "total_tbd": sum(row["count"] for row in placeholder_rows),
        "files_with_placeholders": sorted({row["file"] for row in placeholder_rows}),
        "by_owner": placeholder_owner_summary["by_owner"],
    },
    "evidence_input_count": len(evidence_inputs),
    "evidence_input_run_ids": evidence_input_run_ids,
    "evidence_nonpass_or_blocked_count": len([
        item for item in evidence_inputs
        if item.get("result") != "pass" or (item.get("blockers") or 0) > 0
    ]),
    "files": {
        "evidence_ledger": "evidence-ledger.bootstrap.json",
        "placeholder_inventory": "placeholder-inventory.bootstrap.csv",
        "placeholder_owner_summary": "placeholder-owner-summary.bootstrap.json",
        "signoff_checklist": "signoff-checklist.review-template.md",
        "exception_register": "exception-register.review-template.csv",
        "claim_boundary": "claim-boundary.review-template.md",
    },
}

(out_dir / "readiness-manifest.bootstrap.json").write_text(json.dumps(readiness_manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
(out_dir / "placeholder-owner-summary.bootstrap.json").write_text(json.dumps(placeholder_owner_summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
(out_dir / "evidence-ledger.bootstrap.json").write_text(json.dumps({
    "run_id": run_id,
    "generated_at": generated_at,
    "review_required": True,
    "evidence_inputs": evidence_inputs,
}, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

with (out_dir / "placeholder-inventory.bootstrap.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=["file", "line", "count", "owner", "owner_reason", "text"])
    writer.writeheader()
    writer.writerows(placeholder_rows)

with (out_dir / "exception-register.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=["exception_id", "source_gate", "current_status", "severity", "impact_on_trial", "owner", "closure_standard", "review_note"])
    writer.writeheader()
    for item in evidence_inputs:
        if item.get("result") != "pass" or (item.get("blockers") or 0) > 0:
            writer.writerow({
                "exception_id": f"EX-{item['key']}",
                "source_gate": item["key"],
                "current_status": f"result={item.get('result')} blockers={item.get('blockers')}",
                "severity": "TBD",
                "impact_on_trial": "TBD",
                "owner": "TBD",
                "closure_standard": item["path"],
                "review_note": "Site/user reviewer must decide whether this exception blocks pilot signoff.",
            })

checklist = f"""# Signoff Checklist Review Template

Run ID: `{run_id}`

This checklist is generated from `doc/02_acceptance/08-third-party` and current latest evidence pointers. It is a review aid only.

## Required Reviews

| Item | Required action | Evidence |
|---|---|---|
| Deployment proof | Fill site, period, topology, continuous-run proof and deployment result | `pilot-deployment-proof.md` |
| Demo walkthrough | Execute against real APISIX/API/UI evidence or archived package | `demo-script.md` |
| Weekly report | Fill pilot week metrics and cases | `pilot-weekly-report-template.md` |
| Economic benefit | Replace TBD inputs with user-confirmed numbers | `economic-benefit.md` |
| User signoff | Fill functional acceptance, exceptions and signatures | `user-acceptance-signoff.md` |
| Third-party quality | Attach signed blind-test or CNAS-equivalent report | `04-detection-quality/` |
| Performance | Attach signed hardware-window 10x100G/512Mpps summaries | `03-performance/` |
| Security and HA | Attach production-security and RTO/RPO pass evidence or signed exceptions | `05-security/`, `06-resilience/` |

## Current Evidence Inputs

| Key | Result | Blockers | Path |
|---|---:|---:|---|
"""
for item in evidence_inputs:
    checklist += f"| {item['key']} | {item.get('result') or 'missing'} | {item.get('blockers')} | `{item['path']}` |\n"
checklist += "\n## Signoff Rule\n\nDo not remove `TBD` placeholders or mark the formal completion gate complete until the user or third-party reviewer fills the final documents and signs the exceptions.\n"
(out_dir / "signoff-checklist.review-template.md").write_text(checklist, encoding="utf-8")

claim_boundary = "# Claim Boundary Review Template\n\n"
claim_boundary += "## May Claim After Review\n\n"
for claim in claim_policy.get("may_claim", []):
    claim_boundary += f"- {claim}\n"
claim_boundary += "\n## Must Not Claim Until Filled And Signed\n\n"
for claim in claim_policy.get("must_not_claim_until_filled", []):
    claim_boundary += f"- {claim}\n"
claim_boundary += "\n## Current Boundary\n\nThis bootstrap does not change any blocked formal gate. It only packages the review path.\n"
(out_dir / "claim-boundary.review-template.md").write_text(claim_boundary, encoding="utf-8")
PY

if jq -e '.review_required == true and .status == "bootstrap_review_required"' "$BOOTSTRAP_DIR/readiness-manifest.bootstrap.json" >/dev/null; then
  json_log "bootstrap" "Readiness manifest is review-required" "info" true "ok" "readiness-manifest.bootstrap.json" "readiness-manifest.bootstrap.json"
else
  json_log "bootstrap" "Readiness manifest is review-required" "blocker" false "invalid" "readiness manifest invalid" "readiness-manifest.bootstrap.json"
fi

if [[ -s "$BOOTSTRAP_DIR/signoff-checklist.review-template.md" && -s "$BOOTSTRAP_DIR/exception-register.review-template.csv" && -s "$BOOTSTRAP_DIR/claim-boundary.review-template.md" && -s "$BOOTSTRAP_DIR/placeholder-owner-summary.bootstrap.json" ]]; then
  json_log "bootstrap" "Review templates are present" "info" true "ok" "checklist/exception/claim/owner templates" "signoff-readiness.bootstrap"
else
  json_log "bootstrap" "Review templates are present" "blocker" false "missing" "expected review templates and placeholder owner summary" "signoff-readiness.bootstrap"
fi

tbd_count="$(jq -r '.template_placeholders.total_tbd' "$BOOTSTRAP_DIR/readiness-manifest.bootstrap.json")"
json_log "readiness" "Formal signoff placeholders are inventoried" "warn" false "review_required" "TBD/placeholders=$tbd_count; formal signoff remains incomplete" "placeholder-inventory.bootstrap.csv"

upstream_blockers="$(jq -r '[.evidence_inputs[] | select((.result // "") != "pass" or ((.blockers // 0) > 0))] | length' "$BOOTSTRAP_DIR/evidence-ledger.bootstrap.json")"
json_log "readiness" "Upstream non-pass evidence is inventoried" "warn" false "review_required" "nonpass_or_blocked_inputs=$upstream_blockers; exceptions require reviewer decision" "evidence-ledger.bootstrap.json"

finalize
