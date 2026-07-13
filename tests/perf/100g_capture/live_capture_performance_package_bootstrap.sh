#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$(date +%Y%m%d%H%M%S)-capture-performance-bootstrap}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-capture-performance-bootstrap}"
PERF_DIR="${PERF_DIR:-doc/02_acceptance/03-performance}"
PLAN_DIR="${PLAN_DIR:-tests/perf/100g_capture}"
STABLE_DIR="${STABLE_DIR:-$PERF_DIR/bootstrap}"

REPORT="$LOG_DIR/capture-performance-bootstrap-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/capture-performance-bootstrap-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/capture-performance.bootstrap"
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

trim_file() {
  local file="$1"
  if [[ -s "$file" ]]; then
    head -c 1000 "$file" | tr '\n' ' '
  fi
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
  local passed total blockers warnings result
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

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg plan_dir "$PLAN_DIR" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      plan_dir:$plan_dir,
      bootstrap_dir:$bootstrap_dir,
      stable_bootstrap_dir:$stable_bootstrap_dir,
      review_required:true,
      formal_gate_note:"review-required bootstrap only; does not satisfy GATE-P0-03 or GATE-P0-04",
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# Capture Performance Bootstrap"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Bootstrap dir: \`$BOOTSTRAP_DIR\`"
    echo "- Stable bootstrap dir: \`$STABLE_BOOTSTRAP_DIR\`"
    echo "- Summary: \`$SUMMARY\`"
    echo
    echo "This bootstrap is review-required. It organizes live readiness context, operator review templates, and result-summary templates for a future 10 x 100Gbps / 512Mpps hardware-window run. It deliberately writes \`*.bootstrap.*\` and \`*.review-template.*\` files, not the formal \`hardware-inventory.yaml\`, \`traffic-profile.yaml\`, \`results/10x100g-summary.json\`, or \`results/512mpps-summary.json\` artifacts."
    echo
    echo "## Failed Checks"
    echo
    jq -r '.checks[] | select(.passed|not) | "- [" + .severity + "] " + .name + ": " + .detail' "$SUMMARY"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/capture-performance-bootstrap-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/capture-performance-bootstrap-latest.json"

  echo "capture-performance-bootstrap result=$result summary=$SUMMARY"
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

check_file "contract" "$PLAN_DIR/README.md" "blocker" "performance package README present"
check_file "contract" "$PLAN_DIR/capture-performance-plan.yaml" "blocker" "capture performance plan present"
check_file "contract" "$PLAN_DIR/result-schema.json" "blocker" "result schema present"
check_file "contract" "$PLAN_DIR/hardware-inventory.template.yaml" "blocker" "hardware inventory template present"
check_file "contract" "$PLAN_DIR/traffic-profile.template.yaml" "blocker" "traffic profile template present"
check_file "context" "$PERF_DIR/capture-performance-preflight-latest.json" "warn" "latest capture preflight context present"
check_file "context" "$PERF_DIR/live-node-summary-latest.json" "warn" "live node summary context present"
check_file "context" "$PERF_DIR/live-probe-capture-profile-latest.json" "warn" "live probe capture profile context present"
check_file "context" "$PERF_DIR/repo-stress-500k-summary-latest.json" "warn" "repo 500k stress context present"

python3 - \
  "$BOOTSTRAP_DIR" \
  "$RUN_ID" \
  "$PLAN_DIR" \
  "$PERF_DIR" \
  "$PLAN_DIR/capture-performance-plan.yaml" \
  "$PLAN_DIR/hardware-inventory.template.yaml" \
  "$PLAN_DIR/traffic-profile.template.yaml" \
  "$PLAN_DIR/result-schema.json" \
  "$PERF_DIR/capture-performance-preflight-latest.json" \
  "$PERF_DIR/live-node-summary-latest.json" \
  "$PERF_DIR/live-probe-capture-profile-latest.json" \
  "$PERF_DIR/repo-stress-500k-summary-latest.json" <<'PY'
import json
import shutil
import sys
from datetime import datetime, timezone
from pathlib import Path

out_dir = Path(sys.argv[1])
run_id = sys.argv[2]
plan_dir = sys.argv[3]
perf_dir = sys.argv[4]
plan_path = Path(sys.argv[5])
hardware_template_path = Path(sys.argv[6])
traffic_template_path = Path(sys.argv[7])
schema_path = Path(sys.argv[8])
preflight_path = Path(sys.argv[9])
nodes_path = Path(sys.argv[10])
probe_profile_path = Path(sys.argv[11])
stress_path = Path(sys.argv[12])

generated_at = datetime.now(timezone.utc).astimezone().isoformat()
out_dir.mkdir(parents=True, exist_ok=True)
(out_dir / "inputs").mkdir(exist_ok=True)
(out_dir / "results").mkdir(exist_ok=True)

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

def scalar(value):
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (int, float)):
        return str(value)
    return json.dumps(str(value), ensure_ascii=False)

def dump_yaml(value, indent=0):
    space = " " * indent
    lines = []
    if isinstance(value, dict):
        if not value:
            return [space + "{}"]
        for key, item in value.items():
            if isinstance(item, (dict, list)):
                lines.append(f"{space}{key}:")
                lines.extend(dump_yaml(item, indent + 2))
            else:
                lines.append(f"{space}{key}: {scalar(item)}")
    elif isinstance(value, list):
        if not value:
            return [space + "[]"]
        for item in value:
            if isinstance(item, (dict, list)):
                lines.append(f"{space}-")
                lines.extend(dump_yaml(item, indent + 2))
            else:
                lines.append(f"{space}- {scalar(item)}")
    else:
        lines.append(space + scalar(value))
    return lines

def write_yaml(path, payload):
    path.write_text("\n".join(dump_yaml(payload)) + "\n", encoding="utf-8")

nodes = read_json(nodes_path, [])
probe_profile = read_json(probe_profile_path, {})
stress = read_json(stress_path, {})
preflight = read_json(preflight_path, {})

input_artifacts = {
    "capture_performance_plan": copy_if_present(plan_path),
    "hardware_inventory_template": copy_if_present(hardware_template_path),
    "traffic_profile_template": copy_if_present(traffic_template_path),
    "result_schema": copy_if_present(schema_path, "result-schema.review-copy.json"),
    "latest_preflight": copy_if_present(preflight_path),
    "live_node_summary": copy_if_present(nodes_path),
    "live_probe_capture_profile": copy_if_present(probe_profile_path),
    "repo_stress_500k_summary": copy_if_present(stress_path),
}

hardware = {
    "version": 1,
    "status": "bootstrap_review_required",
    "review_required": True,
    "formal_gate_note": "Do not rename this file to hardware-inventory.yaml until the lab operator reviews NICs, firmware, generator mapping, switch path, NUMA, and cable topology.",
    "run_id": run_id,
    "generated_at": generated_at,
    "source_context": {
        "plan_dir": plan_dir,
        "perf_dir": perf_dir,
        "input_artifacts": input_artifacts,
    },
    "observed_live_cluster": {
        "nodes": nodes,
        "current_probe_profile": probe_profile,
        "repo_stress_context": stress,
    },
    "operator_review_required": {
        "test_lab_name": "",
        "approved_window": "",
        "traffic_generator_hosts": [],
        "capture_hosts": [
            {
                "node": node.get("name", ""),
                "kernel": node.get("kernel", ""),
                "os": node.get("os", ""),
                "internal_ip": node.get("internal_ip", ""),
                "nics": [
                    {
                        "name": probe_profile.get("interface", ""),
                        "pci": "",
                        "driver": "",
                        "firmware": "",
                        "speed_gbps": 100,
                        "queues": "",
                        "numa_node": "",
                    }
                ],
            }
            for node in nodes
        ],
        "cpu_pinning": {
            "current_live_cpu_cores": probe_profile.get("cpu_cores", []),
            "acceptance_isolated_cores": [],
            "policy": "",
        },
        "switch_path": {
            "vendor": "",
            "model": "",
            "port_map": [],
        },
        "immutable_artifacts": {
            "uri": "",
            "sha256_manifest": "",
        },
    },
}
write_yaml(out_dir / "hardware-inventory.bootstrap.yaml", hardware)

traffic_profile = {
    "version": 1,
    "status": "bootstrap_review_required",
    "review_required": True,
    "formal_gate_note": "Do not rename this file to traffic-profile.yaml until generator config, packet mix, flow cardinality, duration, and thresholds are signed.",
    "run_id": run_id,
    "generated_at": generated_at,
    "profiles": {
        "ten_by_100g": {
            "generator": "trex",
            "ports": 10,
            "target_gbps_per_port": 100,
            "duration_seconds": 3600,
            "packet_size_distribution": {
                "64": 20,
                "128": 20,
                "512": 20,
                "1024": 20,
                "1518": 20,
            },
            "protocol_mix": {
                "tcp": 70,
                "udp": 30,
            },
            "flow_cardinality": "",
            "operator_review_note": "",
        },
        "small_packet_512mpps": {
            "generator": "trex",
            "packet_size_bytes": 64,
            "target_mpps": 512,
            "duration_seconds": 1800,
            "flow_cardinality": "",
            "operator_review_note": "",
        },
    },
    "thresholds": {
        "max_packet_loss_rate": 0.0001,
        "min_parse_success_rate": 0.9999,
        "kafka_max_lag_records": 0,
        "flink_allowed_backpressure": "LOW",
    },
    "evidence": {
        "raw_generator_logs_uri": "",
        "switch_telemetry_uri": "",
        "prometheus_snapshot_uri": "",
        "signed_report_uri": "",
    },
}
write_yaml(out_dir / "traffic-profile.bootstrap.yaml", traffic_profile)

def result_template(test_id, min_duration, target_gbps=None, target_mpps=None):
    metrics = {
        "aggregate_gbps": None,
        "aggregate_mpps": None,
        "packet_loss_rate": None,
        "probe_drop_rate": None,
        "parse_success_rate": None,
        "kafka_max_lag_records": None,
        "flink_backpressure": "",
        "cpu_max_percent": None,
        "numa_remote_memory_percent": None,
    }
    if target_gbps is not None:
        metrics["target_aggregate_gbps"] = target_gbps
    if target_mpps is not None:
        metrics["target_aggregate_mpps"] = target_mpps
    return {
        "test_id": test_id,
        "status": "template_review_required",
        "review_required": True,
        "formal_gate_note": "Operator must replace this review template with a real completed result summary before formal preflight can pass.",
        "started_at": "",
        "duration_seconds": None,
        "minimum_duration_seconds": min_duration,
        "traffic_generator": {
            "tool": "trex",
            "version": "",
            "ports": 10 if test_id == "10x100g-line-rate" else "",
        },
        "probe": {
            "capture_mode": probe_profile.get("mode", ""),
            "nodes": [node.get("name", "") for node in nodes],
        },
        "metrics": metrics,
        "artifacts": {
            "raw_generator_logs_uri": "",
            "switch_telemetry_uri": "",
            "prometheus_snapshot_uri": "",
            "signed_report_uri": "",
            "sha256_manifest": "",
        },
    }

(out_dir / "results" / "10x100g-summary.review-template.json").write_text(
    json.dumps(result_template("10x100g-line-rate", 3600, target_gbps=1000), indent=2, ensure_ascii=False) + "\n",
    encoding="utf-8",
)
(out_dir / "results" / "512mpps-summary.review-template.json").write_text(
    json.dumps(result_template("512mpps-small-packet", 1800, target_mpps=512), indent=2, ensure_ascii=False) + "\n",
    encoding="utf-8",
)

manifest = {
    "package_id": "capture_performance_bootstrap",
    "status": "bootstrap_review_required",
    "review_required": True,
    "run_id": run_id,
    "generated_at": generated_at,
    "formal_gate_note": "This package is preparation material only. Formal GATE-P0-03/04 evidence still requires hardware-inventory.yaml, traffic-profile.yaml, results/10x100g-summary.json, and results/512mpps-summary.json in tests/perf/100g_capture/.",
    "latest_preflight_result": preflight.get("result", ""),
    "latest_preflight_blockers": preflight.get("blockers", None),
    "files": {
        "hardware_inventory_bootstrap": "hardware-inventory.bootstrap.yaml",
        "traffic_profile_bootstrap": "traffic-profile.bootstrap.yaml",
        "ten_by_100g_result_template": "results/10x100g-summary.review-template.json",
        "small_packet_result_template": "results/512mpps-summary.review-template.json",
        "operator_runbook": "operator-runbook.md",
        "input_artifacts": input_artifacts,
    },
}
(out_dir / "evidence-manifest.bootstrap.json").write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")

runbook = f"""# Capture Performance Bootstrap Runbook

Run ID: `{run_id}`

This directory is a review-required draft for GATE-P0-03 and GATE-P0-04. It is not a formal acceptance package.

## Use This Draft

1. Review `hardware-inventory.bootstrap.yaml` with the lab operator and fill real NIC, firmware, NUMA, CPU pinning, generator, cable, and switch path details.
2. Review `traffic-profile.bootstrap.yaml` and lock generator ports, packet mix, flow cardinality, duration, and thresholds.
3. Run the approved hardware-window test on isolated 10 x 100Gbps / 512Mpps-capable equipment.
4. Replace the review templates with real completed summaries named exactly `results/10x100g-summary.json` and `results/512mpps-summary.json` under `{plan_dir}`.
5. Rerun `ALLOW_BLOCKERS=true tests/perf/100g_capture/live_capture_performance_preflight.sh` and keep the result blocked unless both formal summaries meet every gate.

## Boundary

The current live context can show that probe-agent is deployed and what the small cluster profile looks like. It cannot prove line-rate capture, 512Mpps small-packet handling, packet-loss thresholds, generator telemetry, or signed operator acceptance.
"""
(out_dir / "operator-runbook.md").write_text(runbook, encoding="utf-8")
PY

if jq -e '.review_required == true and .status == "bootstrap_review_required"' "$BOOTSTRAP_DIR/evidence-manifest.bootstrap.json" >/dev/null; then
  json_log "bootstrap" "Bootstrap manifest is review-required" "info" true "ok" "evidence-manifest.bootstrap.json" "evidence-manifest.bootstrap.json"
else
  json_log "bootstrap" "Bootstrap manifest is review-required" "blocker" false "invalid" "$(trim_file "$BOOTSTRAP_DIR/evidence-manifest.bootstrap.json")" "evidence-manifest.bootstrap.json"
fi

if [[ -s "$BOOTSTRAP_DIR/hardware-inventory.bootstrap.yaml" && -s "$BOOTSTRAP_DIR/traffic-profile.bootstrap.yaml" ]]; then
  json_log "bootstrap" "Review-required hardware and traffic drafts are present" "info" true "ok" "hardware/traffic bootstrap files" "capture-performance.bootstrap"
else
  json_log "bootstrap" "Review-required hardware and traffic drafts are present" "blocker" false "missing" "expected bootstrap yaml files" "capture-performance.bootstrap"
fi

if [[ -s "$BOOTSTRAP_DIR/results/10x100g-summary.review-template.json" && -s "$BOOTSTRAP_DIR/results/512mpps-summary.review-template.json" ]]; then
  json_log "bootstrap" "Review result templates are present" "info" true "ok" "10x100g and 512mpps review templates" "results"
else
  json_log "bootstrap" "Review result templates are present" "blocker" false "missing" "expected result review templates" "results"
fi

formal_found="$(
  find "$BOOTSTRAP_DIR" -type f \
    \( -name 'hardware-inventory.yaml' \
    -o -name 'traffic-profile.yaml' \
    -o -path '*/results/10x100g-summary.json' \
    -o -path '*/results/512mpps-summary.json' \) \
    -print
)"
if [[ -n "$formal_found" ]]; then
  json_log "safety" "Bootstrap does not create formal gate artifacts" "blocker" false "formal_artifact_found" "$formal_found" "capture-performance.bootstrap"
else
  json_log "safety" "Bootstrap does not create formal gate artifacts" "info" true "ok" "formal artifacts absent from bootstrap package" "capture-performance.bootstrap"
fi

finalize
