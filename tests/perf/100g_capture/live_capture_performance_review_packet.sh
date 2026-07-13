#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-capture-performance-review}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
INPUT_BOOTSTRAP_DIR="${INPUT_BOOTSTRAP_DIR:-doc/02_acceptance/03-performance/bootstrap/latest}"
STABLE_DIR="${STABLE_DIR:-doc/02_acceptance/03-performance/review}"

REPORT="$LOG_DIR/capture-performance-review-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/capture-performance-review-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
PACKET_DIR="$LOG_DIR/capture-performance-review.packet"
STABLE_PACKET_DIR="$STABLE_DIR/latest"
STABLE_JSON="$STABLE_DIR/capture-performance-review-latest.json"
STABLE_MD="$STABLE_DIR/capture-performance-review-latest.md"

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
  local passed total blockers warnings result target_count review_file_count formal_artifact_count
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

  target_count="$(jq -r '.target_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  review_file_count="$(jq -r '.review_file_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"
  formal_artifact_count="$(jq -r '.formal_artifact_count // 0' "$PACKET_DIR/review-summary.json" 2>/dev/null || echo 0)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg input_bootstrap_dir "$INPUT_BOOTSTRAP_DIR" \
    --arg packet_dir "$PACKET_DIR" \
    --arg stable_packet_dir "$STABLE_PACKET_DIR" \
    --argjson target_count "$target_count" \
    --argjson review_file_count "$review_file_count" \
    --argjson formal_artifact_count "$formal_artifact_count" \
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
      formal_gate_note:"capture performance review packet only; does not close GATE-P0-03 or GATE-P0-04",
      target_count:$target_count,
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
    echo "# Capture Performance Review Packet"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Input bootstrap: \`$INPUT_BOOTSTRAP_DIR\`"
    echo "- Targets: $target_count"
    echo "- Review files: $review_file_count"
    echo "- Stable packet: \`$STABLE_PACKET_DIR\`"
    echo
    echo "This package turns the 10 x 100Gbps / 512Mpps bootstrap into an operator review board for the hardware window. It is not a signed performance result and cannot close GATE-P0-03/04."
    echo
    echo "## Files"
    echo
    echo "- \`hardware-review.csv\`: lab hardware and NIC review worklist"
    echo "- \`traffic-profile-review.csv\`: generator and traffic profile review worklist"
    echo "- \`result-summary-worklist.csv\`: required result summary checklist"
    echo "- \`formal-artifact-manifest.template.json\`: formal artifact manifest template"
    echo "- \`operator-approval.template.md\`: hardware-window approval template"
    echo "- \`review-checklist.md\`: execution and rerun checklist"
    echo "- \`review-summary.json\`: package metadata"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_PACKET_DIR"
  mkdir -p "$STABLE_PACKET_DIR"
  cp -R "$PACKET_DIR/." "$STABLE_PACKET_DIR/"
  cp "$SUMMARY" "$STABLE_JSON"
  cp "$LOCAL_REPORT" "$STABLE_MD"

  echo "capture performance review packet result=$result summary=$SUMMARY"
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

python3 - "$INPUT_BOOTSTRAP_DIR" "$PACKET_DIR" "$RUN_ID" <<'PY'
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

bootstrap_dir = Path(sys.argv[1])
packet_dir = Path(sys.argv[2])
run_id = sys.argv[3]
packet_dir.mkdir(parents=True, exist_ok=True)

required_files = [
    "hardware-inventory.bootstrap.yaml",
    "traffic-profile.bootstrap.yaml",
    "results/10x100g-summary.review-template.json",
    "results/512mpps-summary.review-template.json",
    "evidence-manifest.bootstrap.json",
    "operator-runbook.md",
]
missing_files = [name for name in required_files if not (bootstrap_dir / name).is_file()]
if missing_files:
    raise SystemExit("missing bootstrap files: " + ", ".join(missing_files))

def read_json(path, default):
    try:
        if path.is_file() and path.stat().st_size > 0:
            return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        pass
    return default

manifest = read_json(bootstrap_dir / "evidence-manifest.bootstrap.json", {})
nodes = read_json(bootstrap_dir / "inputs" / "live-node-summary-latest.json", [])
probe_profile = read_json(bootstrap_dir / "inputs" / "live-probe-capture-profile-latest.json", {})
stress_context = read_json(bootstrap_dir / "inputs" / "repo-stress-500k-summary-latest.json", {})
result_templates = {
    "10x100g-line-rate": read_json(bootstrap_dir / "results" / "10x100g-summary.review-template.json", {}),
    "512mpps-small-packet": read_json(bootstrap_dir / "results" / "512mpps-summary.review-template.json", {}),
}

def write_csv(path, rows, fieldnames):
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow({field: row.get(field, "") for field in fieldnames})

hardware_rows = []
for node in nodes or [{"name": "TBD"}]:
    hardware_rows.append({
        "node": node.get("name", "TBD"),
        "internal_ip": node.get("internal_ip", ""),
        "kernel": node.get("kernel", ""),
        "os": node.get("os", ""),
        "ready": node.get("ready", ""),
        "capture_interface_current": probe_profile.get("interface", ""),
        "capture_mode_current": probe_profile.get("mode", ""),
        "cpu_cores_current": ",".join(str(item) for item in probe_profile.get("cpu_cores", [])),
        "required_capture_mode": "af_xdp",
        "required_nic_speed_gbps": "100",
        "lab_nic_pci": "TBD",
        "lab_nic_driver": "TBD",
        "lab_nic_firmware": "TBD",
        "lab_queue_count": "TBD",
        "lab_numa_node": "TBD",
        "traffic_generator_port": "TBD",
        "switch_port": "TBD",
        "operator_decision": "TBD: approve | modify | exclude",
        "operator_comment": "TBD",
    })

write_csv(
    packet_dir / "hardware-review.csv",
    hardware_rows,
    [
        "node",
        "internal_ip",
        "kernel",
        "os",
        "ready",
        "capture_interface_current",
        "capture_mode_current",
        "cpu_cores_current",
        "required_capture_mode",
        "required_nic_speed_gbps",
        "lab_nic_pci",
        "lab_nic_driver",
        "lab_nic_firmware",
        "lab_queue_count",
        "lab_numa_node",
        "traffic_generator_port",
        "switch_port",
        "operator_decision",
        "operator_comment",
    ],
)

traffic_rows = [
    {
        "profile_id": "ten_by_100g",
        "test_id": "10x100g-line-rate",
        "target_aggregate_gbps": "1000",
        "target_aggregate_mpps": "",
        "duration_seconds": "3600",
        "packet_size_profile": "64/128/512/1024/1518 mixed",
        "protocol_mix": "tcp=70 udp=30",
        "traffic_generator": "trex",
        "ports": "10",
        "flow_cardinality": "TBD",
        "max_packet_loss_rate": "0.0001",
        "min_parse_success_rate": "0.9999",
        "kafka_max_lag_records": "0",
        "operator_decision": "TBD: approve | modify",
        "operator_comment": "TBD",
    },
    {
        "profile_id": "small_packet_512mpps",
        "test_id": "512mpps-small-packet",
        "target_aggregate_gbps": "",
        "target_aggregate_mpps": "512",
        "duration_seconds": "1800",
        "packet_size_profile": "64B or site-agreed small packet",
        "protocol_mix": "TBD",
        "traffic_generator": "trex",
        "ports": "TBD",
        "flow_cardinality": "TBD",
        "max_packet_loss_rate": "0.0001",
        "min_parse_success_rate": "0.9999",
        "kafka_max_lag_records": "0",
        "operator_decision": "TBD: approve | modify",
        "operator_comment": "TBD",
    },
]
write_csv(
    packet_dir / "traffic-profile-review.csv",
    traffic_rows,
    [
        "profile_id",
        "test_id",
        "target_aggregate_gbps",
        "target_aggregate_mpps",
        "duration_seconds",
        "packet_size_profile",
        "protocol_mix",
        "traffic_generator",
        "ports",
        "flow_cardinality",
        "max_packet_loss_rate",
        "min_parse_success_rate",
        "kafka_max_lag_records",
        "operator_decision",
        "operator_comment",
    ],
)

result_rows = []
for test_id, template in result_templates.items():
    metrics = template.get("metrics", {}) if isinstance(template, dict) else {}
    artifacts = template.get("artifacts", {}) if isinstance(template, dict) else {}
    result_rows.append({
        "test_id": test_id,
        "required_formal_path": "tests/perf/100g_capture/results/10x100g-summary.json" if test_id == "10x100g-line-rate" else "tests/perf/100g_capture/results/512mpps-summary.json",
        "status_required": "completed",
        "minimum_duration_seconds": template.get("minimum_duration_seconds", ""),
        "target_aggregate_gbps": metrics.get("target_aggregate_gbps", ""),
        "target_aggregate_mpps": metrics.get("target_aggregate_mpps", ""),
        "max_packet_loss_rate": "0.0001",
        "min_parse_success_rate": "0.9999",
        "kafka_max_lag_records": "0",
        "flink_allowed_backpressure": "LOW",
        "required_raw_logs_uri": artifacts.get("raw_generator_logs_uri", "TBD") or "TBD",
        "required_switch_telemetry_uri": artifacts.get("switch_telemetry_uri", "TBD") or "TBD",
        "required_prometheus_snapshot_uri": artifacts.get("prometheus_snapshot_uri", "TBD") or "TBD",
        "required_signed_report_uri": artifacts.get("signed_report_uri", "TBD") or "TBD",
        "required_sha256_manifest": artifacts.get("sha256_manifest", "TBD") or "TBD",
        "review_status": "TBD",
    })

write_csv(
    packet_dir / "result-summary-worklist.csv",
    result_rows,
    [
        "test_id",
        "required_formal_path",
        "status_required",
        "minimum_duration_seconds",
        "target_aggregate_gbps",
        "target_aggregate_mpps",
        "max_packet_loss_rate",
        "min_parse_success_rate",
        "kafka_max_lag_records",
        "flink_allowed_backpressure",
        "required_raw_logs_uri",
        "required_switch_telemetry_uri",
        "required_prometheus_snapshot_uri",
        "required_signed_report_uri",
        "required_sha256_manifest",
        "review_status",
    ],
)

formal_manifest_template = {
    "package_id": "capture_performance_formal_artifacts",
    "status": "review-template",
    "review_required": True,
    "generated_from_review_packet": run_id,
    "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "source_bootstrap_run_id": manifest.get("run_id", ""),
    "formal_gate_note": "This template must not be used as a formal artifact manifest until all TBD fields are replaced and the hardware-window outputs are signed.",
    "required_formal_artifacts": [
        "tests/perf/100g_capture/hardware-inventory.yaml",
        "tests/perf/100g_capture/traffic-profile.yaml",
        "tests/perf/100g_capture/results/10x100g-summary.json",
        "tests/perf/100g_capture/results/512mpps-summary.json",
    ],
    "context": {
        "current_probe_profile": probe_profile,
        "repo_stress_context": stress_context,
        "latest_preflight_result": manifest.get("latest_preflight_result", ""),
        "latest_preflight_blockers": manifest.get("latest_preflight_blockers", ""),
    },
    "operator_approval": {
        "approved_by": "TBD",
        "approved_at": "TBD",
        "test_window": "TBD",
        "lab": "TBD",
        "signed_report_uri": "TBD",
        "sha256_manifest": "TBD",
    },
}
(packet_dir / "formal-artifact-manifest.template.json").write_text(
    json.dumps(formal_manifest_template, ensure_ascii=False, indent=2) + "\n",
    encoding="utf-8",
)

(packet_dir / "operator-approval.template.md").write_text(
    "\n".join([
        "# Capture Performance Operator Approval",
        "",
        f"- Review packet: `{run_id}`",
        "- Lab/operator: TBD",
        "- Approved hardware window: TBD",
        "- Traffic generator inventory approved: TBD",
        "- Capture host and NIC inventory approved: TBD",
        "- 10 x 100Gbps result reviewed: TBD",
        "- 512Mpps result reviewed: TBD",
        "- Signed report URI: TBD",
        "- SHA256 manifest URI: TBD",
        "",
        "This template is not a signed acceptance record.",
        "",
    ]),
    encoding="utf-8",
)

(packet_dir / "review-checklist.md").write_text(
    "\n".join([
        "# Capture Performance Review Checklist",
        "",
        "1. Fill `hardware-review.csv` with real generator, NIC, firmware, queue, NUMA, switch, and cable information.",
        "2. Fill `traffic-profile-review.csv` with the approved generator packet mix, flow cardinality, duration, and thresholds.",
        "3. Create formal `tests/perf/100g_capture/hardware-inventory.yaml` and `traffic-profile.yaml` only after operator approval.",
        "4. Execute the isolated 10 x 100Gbps and 512Mpps hardware-window tests.",
        "5. Replace review templates with real `results/10x100g-summary.json` and `results/512mpps-summary.json` including raw log, switch telemetry, Prometheus snapshot, signed report, and SHA256 manifest URIs.",
        "6. Rerun `ALLOW_BLOCKERS=false tests/perf/100g_capture/live_capture_performance_preflight.sh`.",
        "",
        "Do not rename bootstrap or review-template files into formal artifact paths.",
        "",
    ]),
    encoding="utf-8",
)

summary = {
    "package_id": "capture_performance_review_packet",
    "run_id": run_id,
    "generated_at": datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z"),
    "input_bootstrap_dir": str(bootstrap_dir),
    "source_bootstrap_run_id": manifest.get("run_id", ""),
    "review_required": True,
    "formal_gate_note": "review packet only; does not close GATE-P0-03 or GATE-P0-04",
    "target_count": len(result_rows),
    "hardware_review_row_count": len(hardware_rows),
    "traffic_profile_row_count": len(traffic_rows),
    "review_file_count": 7,
    "formal_artifact_count": 0,
    "required_formal_artifacts": formal_manifest_template["required_formal_artifacts"],
    "current_context": {
        "probe_mode": probe_profile.get("mode", ""),
        "probe_cpu_core_count": len(probe_profile.get("cpu_cores", [])),
        "repo_stress_mbps": stress_context.get("mbps", None),
        "repo_stress_pps": stress_context.get("pps", None),
    },
    "review_outputs": [
        "hardware-review.csv",
        "traffic-profile-review.csv",
        "result-summary-worklist.csv",
        "formal-artifact-manifest.template.json",
        "operator-approval.template.md",
        "review-checklist.md",
        "review-summary.json",
    ],
}
(packet_dir / "review-summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
PY

if [[ -s "$PACKET_DIR/review-summary.json" ]]; then
  json_log "packet" "review packet generated" "info" true "ok" "$(jq -r '"targets=" + (.target_count|tostring) + " files=" + (.review_file_count|tostring)' "$PACKET_DIR/review-summary.json")" "review-summary.json"
else
  json_log "packet" "review packet generated" "blocker" false "missing" "review-summary.json" "review-summary.json"
fi

if jq -e '.target_count == 2' "$PACKET_DIR/review-summary.json" >/dev/null 2>&1; then
  json_log "packet" "both performance targets represented" "info" true "ok" "10x100g and 512mpps" "result-summary-worklist.csv"
else
  json_log "packet" "both performance targets represented" "blocker" false "missing" "target_count mismatch" "result-summary-worklist.csv"
fi

if [[ -s "$PACKET_DIR/hardware-review.csv" && -s "$PACKET_DIR/traffic-profile-review.csv" && -s "$PACKET_DIR/result-summary-worklist.csv" ]]; then
  json_log "packet" "review worklists present" "info" true "ok" "hardware/traffic/results" "$PACKET_DIR"
else
  json_log "packet" "review worklists present" "blocker" false "missing" "review worklist missing" "$PACKET_DIR"
fi

if [[ -s "$PACKET_DIR/formal-artifact-manifest.template.json" && -s "$PACKET_DIR/operator-approval.template.md" && -s "$PACKET_DIR/review-checklist.md" ]]; then
  json_log "packet" "manifest approval and checklist present" "info" true "ok" "manifest/approval/checklist" "$PACKET_DIR"
else
  json_log "packet" "manifest approval and checklist present" "blocker" false "missing" "manifest approval or checklist missing" "$PACKET_DIR"
fi

if jq -e '.formal_artifact_count == 0' "$PACKET_DIR/review-summary.json" >/dev/null 2>&1; then
  json_log "safety" "review packet does not create formal artifacts" "info" true "ok" "formal_artifact_count=0" "review-summary.json"
else
  json_log "safety" "review packet does not create formal artifacts" "blocker" false "formal_artifact_present" "formal_artifact_count != 0" "review-summary.json"
fi

finalize
