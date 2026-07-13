#!/usr/bin/env bash
set -euo pipefail

LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/20260629-capture-performance-preflight}"
RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-capture-performance-preflight}"
PERF_DIR="${PERF_DIR:-doc/02_acceptance/03-performance}"
PLAN_DIR="${PLAN_DIR:-tests/perf/100g_capture}"
KUBECTL="${KUBECTL:-kubectl}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"

REPORT="$LOG_DIR/capture-performance-preflight-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/capture-performance-preflight-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/capture-performance-preflight-$RUN_ID.md"

mkdir -p "$LOG_DIR" "$PERF_DIR"
: >"$REPORT"

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
    head -c 800 "$file" | tr '\n' ' '
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

template_markers_in_file() {
  local path="$1"
  if [[ ! -s "$path" ]]; then
    return 1
  fi
  LC_ALL=C grep -Eiq \
    'review_required|review-template|bootstrap_review_required|template_review_required|formal_gate_note|do not rename|operator must replace|review copy' \
    "$path"
}

guard_formal_artifact() {
  local phase="$1" path="$2" label="$3"
  if [[ ! -s "$path" ]]; then
    return
  fi
  if template_markers_in_file "$path"; then
    json_log "$phase" "$label is not bootstrap or review template" "blocker" false "review_required" "formal performance artifact contains bootstrap/review-template markers" "$path"
  else
    json_log "$phase" "$label is not bootstrap or review template" "info" true "ok" "no bootstrap/review-template markers" "$path"
  fi
}

evaluate_result_file() {
  local file="$1" test_id="$2" min_gbps="$3" min_mpps="$4" min_duration="$5"
  local out="$LOG_DIR/$(basename "$file" .json)-evaluation.json"
  if [[ ! -s "$file" ]]; then
    json_log "results" "$test_id result summary present" "blocker" false "missing" "$file" "$file"
    return
  fi
  set +e
  python3 - "$file" "$test_id" "$min_gbps" "$min_mpps" "$min_duration" >"$out" <<'PY'
import json
import sys

path, expected_id = sys.argv[1], sys.argv[2]
min_gbps = float(sys.argv[3])
min_mpps = float(sys.argv[4])
min_duration = float(sys.argv[5])
max_loss = 0.0001
min_parse_success = 0.9999

try:
    payload = json.load(open(path, encoding="utf-8"))
except Exception as exc:
    print(json.dumps({"passed": False, "status": "invalid_json", "detail": str(exc)}))
    raise SystemExit(1)

metrics = payload.get("metrics", {})
checks = []

def num(mapping, key, default=0):
    value = mapping.get(key, default)
    if value is None or value == "":
        return float(default)
    return float(value)

def add(name, passed, detail):
    checks.append({"name": name, "passed": bool(passed), "detail": detail})

add("test_id matches", payload.get("test_id") == expected_id, payload.get("test_id", ""))
add("status completed", payload.get("status") == "completed", payload.get("status", ""))
duration = num(payload, "duration_seconds", 0)
add("duration meets target", duration >= min_duration, duration)
if min_gbps > 0:
    aggregate_gbps = num(metrics, "aggregate_gbps", 0)
    add("aggregate gbps meets target", aggregate_gbps >= min_gbps, aggregate_gbps)
if min_mpps > 0:
    aggregate_mpps = num(metrics, "aggregate_mpps", 0)
    add("aggregate mpps meets target", aggregate_mpps >= min_mpps, aggregate_mpps)
loss = num(metrics, "packet_loss_rate", num(metrics, "probe_drop_rate", 1))
add("packet loss within gate", loss <= max_loss, loss)
parse_success = num(metrics, "parse_success_rate", 0)
add("parse success within gate", parse_success >= min_parse_success, parse_success)
lag = num(metrics, "kafka_max_lag_records", 1)
add("kafka lag drained", lag <= 0, lag)
backpressure = str(metrics.get("flink_backpressure", "")).upper()
add("flink backpressure acceptable", backpressure in {"", "LOW", "OK", "NONE"}, backpressure)

failed = [check for check in checks if not check["passed"]]
print(json.dumps({"passed": not failed, "status": "ok" if not failed else "failed", "detail": failed, "checks": checks}, ensure_ascii=True))
raise SystemExit(0 if not failed else 1)
PY
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    json_log "results" "$test_id result meets acceptance gates" "info" true "ok" "$file" "$out"
  else
    json_log "results" "$test_id result meets acceptance gates" "blocker" false "failed" "$(trim_file "$out")" "$out"
  fi
}

need_cmd git
need_cmd jq
need_cmd python3
need_cmd "$KUBECTL"

git rev-parse HEAD >"$LOG_DIR/commit-sha.txt"
git branch --show-current >"$LOG_DIR/git-branch.txt"
git status --short >"$LOG_DIR/git-status.txt"

check_file "contract" "$PLAN_DIR/README.md" "blocker" "performance package README present"
check_file "contract" "$PLAN_DIR/capture-performance-plan.yaml" "blocker" "capture performance plan present"
check_file "contract" "$PLAN_DIR/result-schema.json" "blocker" "result schema present"
check_file "contract" "$PLAN_DIR/hardware-inventory.template.yaml" "blocker" "hardware inventory template present"
check_file "contract" "$PLAN_DIR/traffic-profile.template.yaml" "blocker" "traffic profile template present"
check_file "package" "$PLAN_DIR/hardware-inventory.yaml" "blocker" "hardware inventory provided"
check_file "package" "$PLAN_DIR/traffic-profile.yaml" "blocker" "traffic profile provided"
guard_formal_artifact "integrity" "$PLAN_DIR/hardware-inventory.yaml" "hardware inventory"
guard_formal_artifact "integrity" "$PLAN_DIR/traffic-profile.yaml" "traffic profile"
guard_formal_artifact "integrity" "$PLAN_DIR/results/10x100g-summary.json" "10x100g result summary"
guard_formal_artifact "integrity" "$PLAN_DIR/results/512mpps-summary.json" "512mpps result summary"
json_log "integrity" "bootstrap and review-template artifacts are blocked from formal performance pass" "info" true "ok" "formal performance artifacts are scanned for review_required/template markers before GATE-P0-03/04 can pass" "tests/perf/100g_capture/live_capture_performance_preflight.sh"

if [[ -s rust/probe-agent/tests/reports/stress_500k_report.txt ]]; then
  python3 - "$LOG_DIR/repo-stress-500k-summary.json" <<'PY'
import json
import re
from pathlib import Path

text = Path("rust/probe-agent/tests/reports/stress_500k_report.txt").read_text(encoding="utf-8")
def number(pattern):
    match = re.search(pattern, text)
    return float(match.group(1)) if match else None
payload = {
    "source": "rust/probe-agent/tests/reports/stress_500k_report.txt",
    "packets": number(r"总包数:\s+([0-9.]+)"),
    "pps": number(r"吞吐量:\s+([0-9.]+) pps"),
    "mbps": number(r"带宽:\s+([0-9.]+) Mbps"),
    "parse_success_percent": number(r"解析成功率:\s+([0-9.]+)%"),
    "acceptance_note": "small offline PCAP stress only; not a 10x100Gbps or 512Mpps acceptance result",
}
Path(__import__("sys").argv[1]).write_text(json.dumps(payload, indent=2, ensure_ascii=True), encoding="utf-8")
PY
  json_log "repo" "existing 500k stress report is only non-acceptance context" "warn" false "insufficient" "0.94Mpps/1.3Gbps class report, not GATE-P0-03/04" "repo-stress-500k-summary.json"
else
  json_log "repo" "existing 500k stress report available" "warn" false "missing" "rust/probe-agent/tests/reports/stress_500k_report.txt" ""
fi

set +e
kctl -n traffic-analysis get daemonset probe-agent -o json >"$LOG_DIR/live-probe-daemonset.json" 2>"$LOG_DIR/live-probe-daemonset.err"
DS_RC=$?
set -e
if [[ "$DS_RC" -eq 0 ]]; then
  DESIRED="$(jq '.status.desiredNumberScheduled // 0' "$LOG_DIR/live-probe-daemonset.json")"
  READY="$(jq '.status.numberReady // 0' "$LOG_DIR/live-probe-daemonset.json")"
  if [[ "$DESIRED" -gt 0 && "$READY" -eq "$DESIRED" ]]; then
    json_log "live" "probe-agent DaemonSet ready" "info" true "ok" "ready=$READY desired=$DESIRED" "live-probe-daemonset.json"
  else
    json_log "live" "probe-agent DaemonSet ready" "blocker" false "not_ready" "ready=$READY desired=$DESIRED" "live-probe-daemonset.json"
  fi
  HOST_NETWORK="$(jq -r '.spec.template.spec.hostNetwork // false' "$LOG_DIR/live-probe-daemonset.json")"
  PRIVILEGED="$(jq -r '[.spec.template.spec.containers[]? | select(.name=="probe-agent") | .securityContext.privileged // false][0] // false' "$LOG_DIR/live-probe-daemonset.json")"
  METRICS_PORT="$(jq -r '[.spec.template.spec.containers[]? | select(.name=="probe-agent") | .ports[]? | select(.name=="metrics") | .containerPort][0] // empty' "$LOG_DIR/live-probe-daemonset.json")"
  [[ "$HOST_NETWORK" == "true" ]] && json_log "live" "probe-agent hostNetwork enabled" "info" true "ok" "hostNetwork=true" "live-probe-daemonset.json" || json_log "live" "probe-agent hostNetwork enabled" "blocker" false "missing" "hostNetwork=$HOST_NETWORK" "live-probe-daemonset.json"
  [[ "$PRIVILEGED" == "true" ]] && json_log "live" "probe-agent privileged capture enabled" "info" true "ok" "privileged=true" "live-probe-daemonset.json" || json_log "live" "probe-agent privileged capture enabled" "blocker" false "missing" "privileged=$PRIVILEGED" "live-probe-daemonset.json"
  [[ "$METRICS_PORT" == "9091" ]] && json_log "live" "probe-agent metrics port declared" "info" true "ok" "9091" "live-probe-daemonset.json" || json_log "live" "probe-agent metrics port declared" "warn" false "missing" "metrics_port=$METRICS_PORT" "live-probe-daemonset.json"
else
  json_log "live" "probe-agent DaemonSet readable" "blocker" false "kubectl_failed" "$(trim_file "$LOG_DIR/live-probe-daemonset.err")" "live-probe-daemonset.err"
fi

set +e
kctl -n traffic-analysis get configmap probe-agent-config -o json >"$LOG_DIR/live-probe-configmap.json" 2>"$LOG_DIR/live-probe-configmap.err"
CM_RC=$?
set -e
if [[ "$CM_RC" -eq 0 ]]; then
  jq -r '.data["config.yaml"] // ""' "$LOG_DIR/live-probe-configmap.json" >"$LOG_DIR/live-probe-config.yaml"
  python3 - "$LOG_DIR/live-probe-config.yaml" "$LOG_DIR/live-probe-capture-profile.json" <<'PY'
import json
import re
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text(encoding="utf-8")
def scalar(name):
    match = re.search(rf"^\s*{name}:\s*\"?([^\"\n]+)\"?\s*$", text, re.M)
    return match.group(1).strip() if match else ""
def list_value(name):
    match = re.search(rf"^\s*{name}:\s*\[([^\]]*)\]", text, re.M)
    if not match:
        return []
    return [item.strip() for item in match.group(1).split(",") if item.strip()]
payload = {
    "interface": scalar("interface"),
    "mode": scalar("mode"),
    "queue_id": scalar("queue_id"),
    "frame_count": scalar("frame_count"),
    "frame_size": scalar("frame_size"),
    "cpu_cores": list_value("cpu_cores"),
    "numa_aware": scalar("numa_aware"),
}
Path(sys.argv[2]).write_text(json.dumps(payload, indent=2, ensure_ascii=True), encoding="utf-8")
PY
  MODE="$(jq -r '.mode' "$LOG_DIR/live-probe-capture-profile.json")"
  CPU_CORE_COUNT="$(jq '.cpu_cores | length' "$LOG_DIR/live-probe-capture-profile.json")"
  if [[ "$MODE" == "af_xdp" ]]; then
    json_log "live" "live probe capture mode is AF_XDP" "info" true "ok" "$MODE" "live-probe-capture-profile.json"
  else
    json_log "live" "live probe capture mode is AF_XDP" "warn" false "not_acceptance_profile" "mode=$MODE" "live-probe-capture-profile.json"
  fi
  if [[ "$CPU_CORE_COUNT" -ge 8 ]]; then
    json_log "live" "live probe CPU pinning has multi-queue capacity" "info" true "ok" "cpu_cores=$CPU_CORE_COUNT" "live-probe-capture-profile.json"
  else
    json_log "live" "live probe CPU pinning has multi-queue capacity" "warn" false "small_profile" "cpu_cores=$CPU_CORE_COUNT" "live-probe-capture-profile.json"
  fi
else
  json_log "live" "probe-agent ConfigMap readable" "blocker" false "kubectl_failed" "$(trim_file "$LOG_DIR/live-probe-configmap.err")" "live-probe-configmap.err"
fi

set +e
kctl get nodes -o json >"$LOG_DIR/live-nodes.json" 2>"$LOG_DIR/live-nodes.err"
NODES_RC=$?
set -e
if [[ "$NODES_RC" -eq 0 ]]; then
  jq '[.items[] | {name:.metadata.name, internal_ip:([.status.addresses[]? | select(.type=="InternalIP") | .address][0] // ""), kernel:.status.nodeInfo.kernelVersion, os:.status.nodeInfo.osImage, ready:([.status.conditions[]? | select(.type=="Ready") | .status][0] // "Unknown")} ]' "$LOG_DIR/live-nodes.json" >"$LOG_DIR/live-node-summary.json"
  NODE_COUNT="$(jq 'length' "$LOG_DIR/live-node-summary.json")"
  NOT_READY="$(jq '[.[] | select(.ready != "True")] | length' "$LOG_DIR/live-node-summary.json")"
  [[ "$NODE_COUNT" -ge 2 && "$NOT_READY" -eq 0 ]] && json_log "live" "cluster nodes readable and ready" "info" true "ok" "nodes=$NODE_COUNT" "live-node-summary.json" || json_log "live" "cluster nodes readable and ready" "warn" false "not_ready" "nodes=$NODE_COUNT not_ready=$NOT_READY" "live-node-summary.json"
else
  json_log "live" "cluster nodes readable" "warn" false "kubectl_failed" "$(trim_file "$LOG_DIR/live-nodes.err")" "live-nodes.err"
fi

evaluate_result_file "$PLAN_DIR/results/10x100g-summary.json" "10x100g-line-rate" 1000 0 3600
evaluate_result_file "$PLAN_DIR/results/512mpps-summary.json" "512mpps-small-packet" 0 512 1800

jq -s \
  --arg run_id "$RUN_ID" \
  --arg generated_at "$(date -Iseconds)" \
  --arg plan_dir "$PLAN_DIR" \
  '{
    run_id:$run_id,
    generated_at:$generated_at,
    plan_dir:$plan_dir,
    result:(if any(.[]; .severity=="blocker" and (.passed|not)) then "blocked" elif any(.[]; .severity=="warn" and (.passed|not)) then "warn" else "pass" end),
    total_checks:length,
    passed_checks:([.[] | select(.passed)] | length),
    blockers:([.[] | select(.severity=="blocker" and (.passed|not))] | length),
    warnings:([.[] | select(.severity=="warn" and (.passed|not))] | length),
    checks:.
  }' "$REPORT" >"$SUMMARY"

{
  echo "# Capture Performance Preflight"
  echo
  echo "- Run ID: \`$RUN_ID\`"
  echo "- Result: \`$(jq -r '.result' "$SUMMARY")\`"
  echo "- Plan dir: \`$PLAN_DIR\`"
  echo
  echo "## Summary"
  echo
  jq -r '"- Checks: \(.passed_checks)/\(.total_checks) passed, blockers=\(.blockers), warnings=\(.warnings)"' "$SUMMARY"
  echo
  echo "## Failed Checks"
  echo
  jq -r '.checks[] | select(.passed|not) | "- [" + .severity + "] " + .name + ": " + .detail' "$SUMMARY"
  echo
  echo "## Boundary"
  echo
  echo "This preflight does not execute TRex/pktgen or destructive line-rate traffic. GATE-P0-03/04 require signed hardware-window result summaries."
} >"$LOCAL_REPORT"

cp "$SUMMARY" "$PERF_DIR/capture-performance-preflight-latest.json"
cp "$LOCAL_REPORT" "$PERF_DIR/capture-performance-preflight-latest.md"
cp "$PLAN_DIR/capture-performance-plan.yaml" "$PERF_DIR/capture-performance-plan.yaml"
cp "$PLAN_DIR/result-schema.json" "$PERF_DIR/capture-performance-result-schema.json"
cp "$PLAN_DIR/hardware-inventory.template.yaml" "$PERF_DIR/hardware-inventory.template.yaml"
cp "$PLAN_DIR/traffic-profile.template.yaml" "$PERF_DIR/traffic-profile.template.yaml"
[[ -s "$LOG_DIR/repo-stress-500k-summary.json" ]] && cp "$LOG_DIR/repo-stress-500k-summary.json" "$PERF_DIR/repo-stress-500k-summary-latest.json"
[[ -s "$LOG_DIR/live-probe-capture-profile.json" ]] && cp "$LOG_DIR/live-probe-capture-profile.json" "$PERF_DIR/live-probe-capture-profile-latest.json"
[[ -s "$LOG_DIR/live-node-summary.json" ]] && cp "$LOG_DIR/live-node-summary.json" "$PERF_DIR/live-node-summary-latest.json"

RESULT="$(jq -r '.result' "$SUMMARY")"
echo "capture-performance-preflight result=$RESULT summary=$SUMMARY"

if [[ "$RESULT" == "blocked" && "$ALLOW_BLOCKERS" != "true" ]]; then
  exit 1
fi
