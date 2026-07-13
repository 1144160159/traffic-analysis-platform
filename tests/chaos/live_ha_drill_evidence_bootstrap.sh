#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-ha-drill-evidence-bootstrap}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
RESILIENCE_DIR="${RESILIENCE_DIR:-doc/02_acceptance/06-resilience}"
STABLE_DIR="${STABLE_DIR:-$RESILIENCE_DIR/bootstrap}"
PLAN_FILE="${PLAN_FILE:-tests/chaos/ha_drill_plan.yaml}"
READINESS_FILE="${READINESS_FILE:-$RESILIENCE_DIR/ha-readiness-preflight-latest.json}"

REPORT="$LOG_DIR/ha-drill-evidence-bootstrap-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/ha-drill-evidence-bootstrap-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/ha-rto-rpo-drill.bootstrap"
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
  local phase="$1" path="$2" severity="$3" label="$4"
  if [[ -s "$path" ]]; then
    json_log "$phase" "$label" "info" true "ok" "$path" "$path"
  else
    json_log "$phase" "$label" "$severity" false "missing" "$path" "$path"
  fi
}

finalize() {
  local passed total blockers warnings result formal_count
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  total="$(jq -s 'length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed != true and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed != true and ((.severity == "warn") or (.severity == "warning")))] | length' "$REPORT")"
  formal_count="$(find "$BOOTSTRAP_DIR" -type f \
    \( -name 'kafka-failover.md' \
    -o -name 'flink-failover.md' \
    -o -name 'clickhouse-failover.md' \
    -o -name 'postgres-failover.md' \
    -o -name 'minio-failover.md' \
    -o -name 'ha-rto-rpo-latest.json' \) \
    -print | wc -l | tr -d ' ')"
  result="pass"
  if [[ "$blockers" -gt 0 || "$formal_count" -gt 0 ]]; then
    result="blocked"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg plan_file "$PLAN_FILE" \
    --arg readiness_file "$READINESS_FILE" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson formal_count "$formal_count" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      plan_file:$plan_file,
      readiness_file:$readiness_file,
      bootstrap_dir:$bootstrap_dir,
      stable_bootstrap_dir:$stable_bootstrap_dir,
      review_required:true,
      formal_gate_note:"HA drill evidence bootstrap only; does not execute destructive failover and does not satisfy GATE-P0-08",
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      formal_artifact_count:$formal_count,
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# HA Drill Evidence Bootstrap"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Bootstrap dir: \`$BOOTSTRAP_DIR\`"
    echo "- Stable bootstrap dir: \`$STABLE_BOOTSTRAP_DIR\`"
    echo "- Summary: \`$SUMMARY\`"
    echo
    echo "This package prepares the evidence structure for a future destructive RTO/RPO maintenance-window drill. It does not delete pods, scale workloads, trigger failover, restart storage, or write production traffic records."
    echo
    echo "## Boundary"
    echo
    echo "Do not move the review-template files to \`doc/02_acceptance/06-resilience/\` root or rename them to formal report names until the approved drill has been executed and reviewed."
    echo
    echo "## Failed Checks"
    echo
    jq -r '.checks[] | select(.passed|not) | "- [" + .severity + "] " + .name + ": " + .detail' "$SUMMARY"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/ha-drill-evidence-bootstrap-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/ha-drill-evidence-bootstrap-latest.json"

  echo "ha-drill-evidence-bootstrap result=$result summary=$SUMMARY"
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

check_file "contract" "$PLAN_FILE" "blocker" "HA drill plan present"
check_file "contract" "tests/chaos/README.md" "blocker" "chaos README present"
check_file "context" "$READINESS_FILE" "warn" "latest HA readiness context present"
check_file "context" "$RESILIENCE_DIR/ha-readiness-preflight-latest.md" "warn" "latest HA readiness report present"

python3 - "$BOOTSTRAP_DIR" "$RUN_ID" "$PLAN_FILE" "$READINESS_FILE" <<'PY'
import csv
import json
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path

out_dir = Path(sys.argv[1])
run_id = sys.argv[2]
plan_file = Path(sys.argv[3])
readiness_file = Path(sys.argv[4])
generated_at = datetime.now(timezone.utc).astimezone().isoformat()
out_dir.mkdir(parents=True, exist_ok=True)
(out_dir / "inputs").mkdir(exist_ok=True)
(out_dir / "reports").mkdir(exist_ok=True)
(out_dir / "snapshots").mkdir(exist_ok=True)

def read_json(path, default):
    try:
        if path.is_file() and path.stat().st_size > 0:
            return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        pass
    return default

def copy_if_present(path, target_name=None):
    if path.is_file() and path.stat().st_size > 0:
        target = out_dir / "inputs" / (target_name or path.name)
        shutil.copy2(path, target)
        return str(target.relative_to(out_dir))
    return None

readiness = read_json(readiness_file, {})
input_artifacts = {
    "ha_drill_plan": copy_if_present(plan_file),
    "ha_readiness_latest": copy_if_present(readiness_file),
}

components = [
    {
        "component": "kafka",
        "phase": "kafka-broker-loss",
        "formal_report": "kafka-failover.md",
        "target_rto_seconds": 180,
        "target_rpo": "zero_acknowledged_message_loss",
        "consistency_checks": [
            "all_partitions_have_leader",
            "isr_recovers_to_replication_factor",
            "live_topic_catalog_unchanged",
            "consumer_offsets_continue",
        ],
    },
    {
        "component": "flink",
        "phase": "flink-taskmanager-loss",
        "formal_report": "flink-failover.md",
        "target_rto_seconds": 300,
        "target_rpo": "exactly_once_checkpoint_restore",
        "consistency_checks": [
            "all_expected_jobs_return_running",
            "latest_completed_checkpoint_advances_after_recovery",
            "no_root_exceptions",
            "output_tables_do_not_duplicate_drill_records",
        ],
    },
    {
        "component": "clickhouse",
        "phase": "clickhouse-replica-loss",
        "formal_report": "clickhouse-failover.md",
        "target_rto_seconds": 300,
        "target_rpo": "no_committed_row_loss",
        "consistency_checks": [
            "system_replicas_not_readonly",
            "absolute_delay_returns_below_60s",
            "queue_size_returns_below_100",
            "drill_query_counts_match_before_after",
        ],
    },
    {
        "component": "postgresql",
        "phase": "postgresql-replica-loss",
        "formal_report": "postgres-failover.md",
        "target_rto_seconds": 300,
        "target_rpo": "no_committed_transaction_loss",
        "consistency_checks": [
            "primary_or_promoted_primary_accepts_connections",
            "replicas_resume_streaming",
            "replay_lag_returns_to_zero_or_site_threshold",
            "control_plane_crud_smoke_passes",
        ],
    },
    {
        "component": "minio",
        "phase": "minio-pod-loss",
        "formal_report": "minio-failover.md",
        "target_rto_seconds": 300,
        "target_rpo": "no_object_loss_for_completed_writes",
        "consistency_checks": [
            "health_endpoint_recovers",
            "pcap_object_head_or_verify_passes",
            "presigned_download_still_matches_sha256",
            "lifecycle_or_retention_config_unchanged",
        ],
    },
]

manifest = {
    "package_id": "ha_rto_rpo_drill_bootstrap",
    "status": "bootstrap_review_required",
    "review_required": True,
    "run_id": run_id,
    "generated_at": generated_at,
    "formal_gate_note": "This package is preparation material only. Formal GATE-P0-08 evidence requires an approved destructive drill, filled timelines, RTO/RPO values, consistency reports, and operator signoff.",
    "latest_readiness": {
        "run_id": readiness.get("run_id", ""),
        "result": readiness.get("result", ""),
        "passed": readiness.get("passed"),
        "total": readiness.get("total"),
        "blockers": readiness.get("blockers"),
        "warnings": readiness.get("warnings"),
    },
    "input_artifacts": input_artifacts,
    "components": components,
    "files": {
        "operator_approval_template": "operator-approval.review-template.yaml",
        "timeline_template": "timeline.review-template.jsonl",
        "rto_rpo_table_template": "rto-rpo-table.review-template.csv",
        "summary_template": "ha-rto-rpo-summary.review-template.json",
        "data_consistency_template": "data-consistency-report.review-template.md",
        "snapshot_index_template": "snapshot-index.review-template.json",
        "component_report_templates": [f"reports/{item['component']}-failover.review-template.md" for item in components],
        "runbook": "operator-runbook.md",
    },
}
(out_dir / "evidence-manifest.bootstrap.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

approval = """status: template_review_required
review_required: true
maintenance_window:
  ticket_id: ""
  approved_by: ""
  approved_at: ""
  start_at: ""
  end_at: ""
  rollback_owner: ""
  communication_channel: ""
scope_acknowledgement:
  destructive_drill: true
  allowed_injections: []
  forbidden_without_new_approval:
    - delete persistent volumes
    - delete persistent volume claims
    - delete Kafka topics
    - delete object buckets
    - rotate root CA or production credentials
signoff:
  operator: ""
  sre: ""
  user_representative: ""
  notes: ""
"""
(out_dir / "operator-approval.review-template.yaml").write_text(approval, encoding="utf-8")

timeline_rows = []
for item in components:
    for event in ("before_snapshot", "injection_start", "failure_observed", "recovery_observed", "after_snapshot", "review_signed"):
        timeline_rows.append({
            "ts": "",
            "component": item["component"],
            "phase": item["phase"],
            "event": event,
            "actor": "",
            "command_or_probe": "",
            "observed_state": "",
            "artifact": "",
            "review_note": "",
        })
with (out_dir / "timeline.review-template.jsonl").open("w", encoding="utf-8") as handle:
    for row in timeline_rows:
        handle.write(json.dumps(row, ensure_ascii=False) + "\n")

with (out_dir / "rto-rpo-table.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=[
        "component", "phase", "target_rto_seconds", "observed_rto_seconds", "target_rpo",
        "observed_rpo", "consistency_status", "pass_fail", "evidence", "reviewer", "review_note",
    ])
    writer.writeheader()
    for item in components:
        writer.writerow({
            "component": item["component"],
            "phase": item["phase"],
            "target_rto_seconds": item["target_rto_seconds"],
            "observed_rto_seconds": "",
            "target_rpo": item["target_rpo"],
            "observed_rpo": "",
            "consistency_status": "TBD",
            "pass_fail": "TBD",
            "evidence": item["formal_report"],
            "reviewer": "TBD",
            "review_note": "fill after destructive drill",
        })

summary = {
    "test_id": "live-ha-rto-rpo-drill",
    "status": "template_review_required",
    "review_required": True,
    "run_id": run_id,
    "started_at": "",
    "completed_at": "",
    "result": "TBD",
    "formal_gate_note": "Do not rename this file to ha-rto-rpo-latest.json until all component reports are filled and signed.",
    "components": [
        {
            "component": item["component"],
            "phase": item["phase"],
            "target_rto_seconds": item["target_rto_seconds"],
            "observed_rto_seconds": None,
            "target_rpo": item["target_rpo"],
            "observed_rpo": "",
            "consistency_checks": [
                {"name": check, "status": "TBD", "evidence": ""}
                for check in item["consistency_checks"]
            ],
            "result": "TBD",
        }
        for item in components
    ],
    "artifacts": {
        "operator_approval": "operator-approval.review-template.yaml",
        "timeline": "timeline.review-template.jsonl",
        "rto_rpo_table": "rto-rpo-table.review-template.csv",
        "data_consistency_report": "data-consistency-report.review-template.md",
    },
}
(out_dir / "ha-rto-rpo-summary.review-template.json").write_text(json.dumps(summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

data_consistency = "# Data Consistency Report Review Template\n\n"
data_consistency += f"Run ID: `{run_id}`\n\n"
data_consistency += "| Component | Before snapshot | After snapshot | Consistency verdict | Reviewer note |\n"
data_consistency += "|---|---|---|---|---|\n"
for item in components:
    data_consistency += f"| {item['component']} | TBD | TBD | TBD | TBD |\n"
data_consistency += "\nThis report must be filled from immutable before/after snapshots and drill logs. It is not valid while any `TBD` remains.\n"
(out_dir / "data-consistency-report.review-template.md").write_text(data_consistency, encoding="utf-8")

snapshot_index = {
    "status": "template_review_required",
    "review_required": True,
    "run_id": run_id,
    "snapshots": [
        {
            "component": item["component"],
            "before_snapshot_uri": "",
            "after_snapshot_uri": "",
            "sha256": "",
            "collector": "",
            "collected_at": "",
        }
        for item in components
    ],
}
(out_dir / "snapshot-index.review-template.json").write_text(json.dumps(snapshot_index, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

for item in components:
    report = f"""# {item['component'].title()} Failover Review Template

Run ID: `{run_id}`

Formal report target after the approved drill: `{item['formal_report']}`

## Target

- Phase: `{item['phase']}`
- Target RTO seconds: `{item['target_rto_seconds']}`
- Target RPO: `{item['target_rpo']}`

## Execution

| Item | Value |
|---|---|
| Approval ticket | TBD |
| Injection command | TBD |
| Injection start | TBD |
| Failure observed | TBD |
| Recovery observed | TBD |
| Observed RTO seconds | TBD |
| Observed RPO | TBD |
| Rollback action | TBD |

## Consistency Checks

| Check | Status | Evidence |
|---|---|---|
"""
    for check in item["consistency_checks"]:
        report += f"| {check} | TBD | TBD |\n"
    report += "\n## Reviewer Signoff\n\n| Role | Name | Date | Decision | Note |\n|---|---|---|---|---|\n| Operator | TBD | TBD | TBD | TBD |\n| SRE | TBD | TBD | TBD | TBD |\n| User representative | TBD | TBD | TBD | TBD |\n"
    (out_dir / "reports" / f"{item['component']}-failover.review-template.md").write_text(report, encoding="utf-8")

runbook = f"""# HA RTO/RPO Drill Bootstrap Runbook

Run ID: `{run_id}`

This directory is a preparation package for GATE-P0-08. It is intentionally not a formal HA pass.

## Operator Flow

1. Run the non-destructive preflight and confirm the only blocker is missing destructive drill evidence.
2. Fill `operator-approval.review-template.yaml` with maintenance-window approval.
3. Execute phases from `{plan_file}` in the approved window.
4. Record every event in `timeline.review-template.jsonl`.
5. Fill component reports under `reports/` and the `rto-rpo-table.review-template.csv`.
6. Fill `data-consistency-report.review-template.md` from immutable before/after snapshots.
7. Only after review, copy filled component reports to `doc/02_acceptance/06-resilience/` using the formal names listed in the plan, and write a real `ha-rto-rpo-latest.json`.

## Boundary

This bootstrap does not delete pods, scale workloads, force failover, or prove RTO/RPO. It only makes the evidence package ready for an approved drill.
"""
(out_dir / "operator-runbook.md").write_text(runbook, encoding="utf-8")
PY

if jq -e '.review_required == true and .status == "bootstrap_review_required"' "$BOOTSTRAP_DIR/evidence-manifest.bootstrap.json" >/dev/null; then
  json_log "bootstrap" "Bootstrap manifest is review-required" "info" true "ok" "evidence-manifest.bootstrap.json" "evidence-manifest.bootstrap.json"
else
  json_log "bootstrap" "Bootstrap manifest is review-required" "blocker" false "invalid" "evidence-manifest.bootstrap.json" "evidence-manifest.bootstrap.json"
fi

if [[ -s "$BOOTSTRAP_DIR/operator-approval.review-template.yaml" && -s "$BOOTSTRAP_DIR/rto-rpo-table.review-template.csv" && -s "$BOOTSTRAP_DIR/ha-rto-rpo-summary.review-template.json" ]]; then
  json_log "bootstrap" "Review-required drill templates are present" "info" true "ok" "approval/rto-rpo/summary templates" "ha-rto-rpo-drill.bootstrap"
else
  json_log "bootstrap" "Review-required drill templates are present" "blocker" false "missing" "expected drill review templates" "ha-rto-rpo-drill.bootstrap"
fi

report_count="$(find "$BOOTSTRAP_DIR/reports" -type f -name '*-failover.review-template.md' | wc -l | tr -d ' ')"
if [[ "$report_count" -eq 5 ]]; then
  json_log "bootstrap" "Component failover report templates are present" "info" true "ok" "reports=$report_count" "reports"
else
  json_log "bootstrap" "Component failover report templates are present" "blocker" false "missing" "reports=$report_count expected=5" "reports"
fi

json_log "readiness" "Formal destructive drill evidence remains missing" "warn" false "review_required" "bootstrap only; approved maintenance-window drill has not been executed" "ha-rto-rpo-drill.bootstrap"

finalize
