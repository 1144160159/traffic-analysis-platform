#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-ha-drill-review}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
INPUT_BOOTSTRAP_DIR="${INPUT_BOOTSTRAP_DIR:-doc/02_acceptance/06-resilience/bootstrap/latest}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/06-resilience/review}"

REPORT="$LOG_DIR/ha-drill-review-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/ha-drill-review-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
PACKET_DIR="$LOG_DIR/ha-drill-review.packet"
STABLE_PACKET_DIR="$STABLE_DIR/latest"
STABLE_JSON="$STABLE_DIR/ha-drill-review-latest.json"
STABLE_MD="$STABLE_DIR/ha-drill-review-latest.md"

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

formal_artifact_count() {
  find "$INPUT_BOOTSTRAP_DIR" -type f \
    \( -name 'kafka-failover.md' \
    -o -name 'flink-failover.md' \
    -o -name 'clickhouse-failover.md' \
    -o -name 'postgres-failover.md' \
    -o -name 'minio-failover.md' \
    -o -name 'ha-rto-rpo-latest.json' \) \
    -print | wc -l | tr -d ' '
}

finalize() {
  local passed total blockers warnings result target_count review_file_count formal_count
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

  if [[ ! -s "$PACKET_DIR/review-summary.json" ]]; then
    jq -n '{review_required:true, target_component_count:0, review_file_count:0, formal_artifact_count:0}' >"$PACKET_DIR/review-summary.json"
  fi

  target_count="$(jq -r '.target_component_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  review_file_count="$(jq -r '.review_file_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  formal_count="$(jq -r '.formal_artifact_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || formal_artifact_count 2>/dev/null || echo 0)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg input_bootstrap_dir "$INPUT_BOOTSTRAP_DIR" \
    --arg packet_dir "$PACKET_DIR" \
    --arg stable_packet_dir "$STABLE_PACKET_DIR" \
    --argjson target_component_count "$target_count" \
    --argjson review_file_count "$review_file_count" \
    --argjson formal_artifact_count "$formal_count" \
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
      input_bootstrap_dir:$input_bootstrap_dir,
      packet_dir:$packet_dir,
      stable_packet_dir:$stable_packet_dir,
      review_required:true,
      formal_gate_note:"HA drill review packet only; does not execute destructive failover and does not satisfy GATE-P0-08",
      target_component_count:$target_component_count,
      review_file_count:$review_file_count,
      formal_artifact_count:$formal_artifact_count,
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      review_summary:($review_summary[0] // {}),
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# HA Drill Review Packet"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Input bootstrap: \`$INPUT_BOOTSTRAP_DIR\`"
    echo "- Target components: $target_count"
    echo "- Review files: $review_file_count"
    echo "- Formal artifacts in packet: $formal_count"
    echo "- Stable packet: \`$STABLE_PACKET_DIR\`"
    echo
    echo "This package turns the HA RTO/RPO bootstrap templates into an operator review board for a future approved maintenance-window drill. It is not signed drill evidence and cannot close GATE-P0-08."
    echo
    echo "## Files"
    echo
    echo "- \`component-drill-review.csv\`: component-by-component drill review worklist"
    echo "- \`rto-rpo-evidence-worklist.csv\`: formal evidence and guard-marker worklist"
    echo "- \`maintenance-window-approval.template.md\`: approval review template"
    echo "- \`formal-artifact-manifest.template.json\`: formal artifact manifest template"
    echo "- \`data-consistency-review-checklist.md\`: before/after consistency checklist"
    echo "- \`operator-review-checklist.md\`: maintenance-window execution checklist"
    echo "- \`review-summary.json\`: package metadata"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_PACKET_DIR"
  mkdir -p "$STABLE_PACKET_DIR"
  cp -R "$PACKET_DIR/." "$STABLE_PACKET_DIR/"
  cp "$SUMMARY" "$STABLE_JSON"
  cp "$LOCAL_REPORT" "$STABLE_MD"

  echo "ha-drill-review result=$result summary=$SUMMARY"
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

if [[ -d "$INPUT_BOOTSTRAP_DIR" ]]; then
  json_log "input" "bootstrap directory exists" "info" true "ok" "$INPUT_BOOTSTRAP_DIR" "$INPUT_BOOTSTRAP_DIR"
else
  json_log "input" "bootstrap directory exists" "blocker" false "missing" "$INPUT_BOOTSTRAP_DIR" "$INPUT_BOOTSTRAP_DIR"
  finalize
fi

required_files=(
  "$INPUT_BOOTSTRAP_DIR/evidence-manifest.bootstrap.json"
  "$INPUT_BOOTSTRAP_DIR/operator-approval.review-template.yaml"
  "$INPUT_BOOTSTRAP_DIR/timeline.review-template.jsonl"
  "$INPUT_BOOTSTRAP_DIR/rto-rpo-table.review-template.csv"
  "$INPUT_BOOTSTRAP_DIR/ha-rto-rpo-summary.review-template.json"
  "$INPUT_BOOTSTRAP_DIR/data-consistency-report.review-template.md"
  "$INPUT_BOOTSTRAP_DIR/snapshot-index.review-template.json"
  "$INPUT_BOOTSTRAP_DIR/operator-runbook.md"
  "$INPUT_BOOTSTRAP_DIR/reports/kafka-failover.review-template.md"
  "$INPUT_BOOTSTRAP_DIR/reports/flink-failover.review-template.md"
  "$INPUT_BOOTSTRAP_DIR/reports/clickhouse-failover.review-template.md"
  "$INPUT_BOOTSTRAP_DIR/reports/postgresql-failover.review-template.md"
  "$INPUT_BOOTSTRAP_DIR/reports/minio-failover.review-template.md"
)
missing_files=()
for required_file in "${required_files[@]}"; do
  if [[ ! -s "$required_file" ]]; then
    missing_files+=("$required_file")
  fi
done
if [[ "${#missing_files[@]}" -eq 0 ]]; then
  json_log "input" "required bootstrap templates are present" "info" true "ok" "${#required_files[@]} files" "$INPUT_BOOTSTRAP_DIR"
else
  json_log "input" "required bootstrap templates are present" "blocker" false "missing" "${missing_files[*]}" "$INPUT_BOOTSTRAP_DIR"
  finalize
fi

python3 - "$INPUT_BOOTSTRAP_DIR" "$PACKET_DIR" "$RUN_ID" <<'PY'
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

bootstrap_dir = Path(sys.argv[1])
packet_dir = Path(sys.argv[2])
run_id = sys.argv[3]
generated_at = datetime.now(timezone.utc).astimezone().isoformat()
packet_dir.mkdir(parents=True, exist_ok=True)

manifest = json.loads((bootstrap_dir / "evidence-manifest.bootstrap.json").read_text(encoding="utf-8"))
components = manifest.get("components") or []
if len(components) != 5:
    raise SystemExit(f"expected 5 HA components, got {len(components)}")

latest_readiness = manifest.get("latest_readiness") or {}

def write_csv(path, rows, fieldnames):
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow({field: row.get(field, "") for field in fieldnames})

component_rows = []
evidence_rows = []
for item in components:
    component = item.get("component", "")
    formal_report = item.get("formal_report", "")
    template_name = f"reports/{component}-failover.review-template.md"
    consistency_checks = item.get("consistency_checks") or []
    component_rows.append({
        "component": component,
        "phase": item.get("phase", ""),
        "target_rto_seconds": item.get("target_rto_seconds", ""),
        "target_rpo": item.get("target_rpo", ""),
        "formal_report": formal_report,
        "source_template": template_name,
        "readiness_run_id": latest_readiness.get("run_id", ""),
        "readiness_result": latest_readiness.get("result", ""),
        "required_timeline_events": "before_snapshot; injection_start; failure_observed; recovery_observed; after_snapshot; review_signed",
        "required_consistency_checks": "; ".join(consistency_checks),
        "before_snapshot_required": "yes",
        "after_snapshot_required": "yes",
        "operator_decision": "TBD: approve | revise | defer",
        "reviewer": "TBD",
        "review_note": "TBD",
    })
    evidence_rows.append({
        "formal_artifact": formal_report,
        "source_template": template_name,
        "status": "not_created",
        "required_after_drill": "yes",
        "must_not_contain": "review-template; review_required; bootstrap_review_required; template_review_required; TBD",
        "minimum_content": "approval ticket, timeline, injection command, observed RTO/RPO, consistency verdicts, reviewer signoff",
        "destination": f"doc/02_acceptance/06-resilience/{formal_report}",
        "review_status": "TBD",
    })

evidence_rows.append({
    "formal_artifact": "ha-rto-rpo-latest.json",
    "source_template": "ha-rto-rpo-summary.review-template.json",
    "status": "not_created",
    "required_after_drill": "yes",
    "must_not_contain": "review-template; review_required; bootstrap_review_required; template_review_required; TBD",
    "minimum_content": "result=pass, observed RTO/RPO per component, data consistency verdicts, all component report links",
    "destination": "doc/02_acceptance/06-resilience/ha-rto-rpo-latest.json",
    "review_status": "TBD",
})

write_csv(
    packet_dir / "component-drill-review.csv",
    component_rows,
    [
        "component",
        "phase",
        "target_rto_seconds",
        "target_rpo",
        "formal_report",
        "source_template",
        "readiness_run_id",
        "readiness_result",
        "required_timeline_events",
        "required_consistency_checks",
        "before_snapshot_required",
        "after_snapshot_required",
        "operator_decision",
        "reviewer",
        "review_note",
    ],
)

write_csv(
    packet_dir / "rto-rpo-evidence-worklist.csv",
    evidence_rows,
    [
        "formal_artifact",
        "source_template",
        "status",
        "required_after_drill",
        "must_not_contain",
        "minimum_content",
        "destination",
        "review_status",
    ],
)

(packet_dir / "maintenance-window-approval.template.md").write_text(
    "\n".join([
        "# HA Maintenance Window Approval Review",
        "",
        f"Run ID: `{run_id}`",
        "",
        "| Item | Value |",
        "|---|---|",
        "| Ticket ID | TBD |",
        "| Approved by | TBD |",
        "| Approval time | TBD |",
        "| Window start | TBD |",
        "| Window end | TBD |",
        "| Rollback owner | TBD |",
        "| Communication channel | TBD |",
        "| Destructive scope acknowledged | TBD |",
        "",
        "This template is for review only. Formal evidence must be produced after the approved drill and copied to the root `06-resilience/` formal artifact names.",
        "",
    ]),
    encoding="utf-8",
)

formal_manifest = {
    "status": "template_review_required",
    "review_required": True,
    "run_id": run_id,
    "generated_at": generated_at,
    "formal_gate_note": "Do not use this template as GATE-P0-08 evidence. Formal artifacts must be produced after the destructive maintenance-window drill and must not contain template markers.",
    "expected_formal_artifact_count": len(evidence_rows),
    "artifacts": [
        {
            "name": row["formal_artifact"],
            "source_template": row["source_template"],
            "destination": row["destination"],
            "status": row["status"],
            "required_after_drill": row["required_after_drill"] == "yes",
            "must_not_contain": row["must_not_contain"].split("; "),
        }
        for row in evidence_rows
    ],
}
(packet_dir / "formal-artifact-manifest.template.json").write_text(json.dumps(formal_manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

checklist = ["# HA Data Consistency Review Checklist", ""]
checklist.append("| Component | Before snapshot | After snapshot | Required consistency checks | Reviewer verdict |")
checklist.append("|---|---|---|---|---|")
for item in components:
    checklist.append(
        "| {component} | TBD | TBD | {checks} | TBD |".format(
            component=item.get("component", ""),
            checks="<br>".join(item.get("consistency_checks") or []),
        )
    )
checklist.append("")
checklist.append("Every row must be backed by immutable before/after snapshots before a formal report is written.")
(packet_dir / "data-consistency-review-checklist.md").write_text("\n".join(checklist) + "\n", encoding="utf-8")

operator_checklist = ["# HA Operator Review Checklist", ""]
for label in [
    "Latest non-destructive HA readiness preflight has only the formal RTO/RPO evidence blocker.",
    "Maintenance-window approval is filled and signed.",
    "Rollback owners and communication channels are assigned.",
    "Timeline events are recorded for all five components.",
    "Before and after snapshots are immutable and referenced from each component report.",
    "Formal root reports are created only after the drill and contain no review-template or TBD markers.",
    "Final ha-rto-rpo-latest.json links all component reports and records observed RTO/RPO values.",
]:
    operator_checklist.append(f"- [ ] {label}")
operator_checklist.append("")
(packet_dir / "operator-review-checklist.md").write_text("\n".join(operator_checklist), encoding="utf-8")

review_files = [
    "component-drill-review.csv",
    "rto-rpo-evidence-worklist.csv",
    "maintenance-window-approval.template.md",
    "formal-artifact-manifest.template.json",
    "data-consistency-review-checklist.md",
    "operator-review-checklist.md",
    "review-summary.json",
]
summary = {
    "package_id": "ha_drill_review_packet",
    "run_id": run_id,
    "generated_at": generated_at,
    "input_bootstrap_dir": str(bootstrap_dir),
    "target_component_count": len(components),
    "review_file_count": len(review_files),
    "formal_artifact_count": 0,
    "review_required": True,
    "formal_gate_note": "Review packet only; does not execute destructive failover and does not satisfy GATE-P0-08.",
    "latest_readiness": latest_readiness,
    "components": component_rows,
    "formal_artifacts": evidence_rows,
    "review_files": review_files,
}
(packet_dir / "review-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

target_count="$(jq -r '.target_component_count // 0' "$PACKET_DIR/review-summary.json")"
if [[ "$target_count" -eq 5 ]]; then
  json_log "review" "HA component drill targets are complete" "info" true "ok" "components=$target_count" "review-summary.json"
else
  json_log "review" "HA component drill targets are complete" "blocker" false "invalid" "components=$target_count expected=5" "review-summary.json"
fi

review_file_count="$(jq -r '.review_file_count // 0' "$PACKET_DIR/review-summary.json")"
actual_review_file_count="$(find "$PACKET_DIR" -maxdepth 1 -type f | wc -l | tr -d ' ')"
if [[ "$review_file_count" -eq 7 && "$actual_review_file_count" -eq 7 ]]; then
  json_log "review" "HA review packet files are present" "info" true "ok" "files=$actual_review_file_count" "$PACKET_DIR"
else
  json_log "review" "HA review packet files are present" "blocker" false "invalid" "declared=$review_file_count actual=$actual_review_file_count expected=7" "$PACKET_DIR"
fi

if [[ -s "$PACKET_DIR/review-summary.json" && -s "$PACKET_DIR/component-drill-review.csv" && -s "$PACKET_DIR/rto-rpo-evidence-worklist.csv" ]]; then
  json_log "review" "HA review packet generated" "info" true "ok" "$PACKET_DIR" "$PACKET_DIR/review-summary.json"
else
  json_log "review" "HA review packet generated" "blocker" false "missing" "$PACKET_DIR" "$PACKET_DIR"
fi

formal_count="$(formal_artifact_count)"
if [[ "$formal_count" -eq 0 ]]; then
  json_log "integrity" "formal HA artifacts remain absent from review packet" "info" true "ok" "formal_artifact_count=$formal_count" "$INPUT_BOOTSTRAP_DIR"
else
  json_log "integrity" "formal HA artifacts remain absent from review packet" "blocker" false "invalid" "formal_artifact_count=$formal_count" "$INPUT_BOOTSTRAP_DIR"
fi

finalize
