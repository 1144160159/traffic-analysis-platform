#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-completion-blocker-closure-readiness}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
ACCEPTANCE_DIR="${ACCEPTANCE_DIR:-doc/02_acceptance}"
COMPLETION_DIR="${COMPLETION_DIR:-$ACCEPTANCE_DIR/09-completion}"
STABLE_DIR="${STABLE_DIR:-$COMPLETION_DIR/blocker-closure}"
AUDIT_JSON="${AUDIT_JSON:-$COMPLETION_DIR/project-completion-audit-latest.json}"

REPORT="$LOG_DIR/completion-blocker-closure-readiness-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/completion-blocker-closure-readiness-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"
BOOTSTRAP_DIR="$LOG_DIR/completion-blocker-closure.bootstrap"
STABLE_BOOTSTRAP_DIR="$STABLE_DIR/latest"

mkdir -p "$LOG_DIR" "$BOOTSTRAP_DIR/inputs" "$STABLE_DIR"
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
  local path="$1" severity="$2" label="$3"
  if [[ -s "$path" ]]; then
    json_log "inputs" "$label" "info" true "ok" "$path" "$path"
    cp "$path" "$BOOTSTRAP_DIR/inputs/$(basename "$path")"
  else
    json_log "inputs" "$label" "$severity" false "missing" "$path" "$path"
  fi
}

finalize() {
  local passed total blockers warnings result blocker_count ready_count external_count command_count source_audit_run_id source_audit_result
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

  blocker_count="$(jq -r '.blocker_count // 0' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo 0)"
  ready_count="$(jq -r '.ready_input_count // 0' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo 0)"
  external_count="$(jq -r '.external_action_count // 0' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo 0)"
  command_count="$(jq -r '.formal_rerun_command_count // 0' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo 0)"
  source_audit_run_id="$(jq -r '.source_audit_run_id // "unknown"' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo unknown)"
  source_audit_result="$(jq -r '.source_audit_result // "unknown"' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json" 2>/dev/null || echo unknown)"

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg generated_at "$(date -Iseconds)" \
    --arg audit_json "$AUDIT_JSON" \
    --arg source_audit_run_id "$source_audit_run_id" \
    --arg source_audit_result "$source_audit_result" \
    --arg bootstrap_dir "$BOOTSTRAP_DIR" \
    --arg stable_bootstrap_dir "$STABLE_BOOTSTRAP_DIR" \
    --argjson passed "$passed" \
    --argjson total "$total" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --argjson blocker_count "$blocker_count" \
    --argjson ready_input_count "$ready_count" \
    --argjson external_action_count "$external_count" \
    --argjson formal_rerun_command_count "$command_count" \
    --slurpfile checks "$REPORT" \
    '{
      run_id:$run_id,
      result:$result,
      generated_at:$generated_at,
      audit_json:$audit_json,
      source_audit_run_id:$source_audit_run_id,
      source_audit_result:$source_audit_result,
      bootstrap_dir:$bootstrap_dir,
      stable_bootstrap_dir:$stable_bootstrap_dir,
      review_required:true,
      formal_gate_note:"completion blocker closure readiness only; does not close any project completion blocker",
      blocker_count:$blocker_count,
      ready_input_count:$ready_input_count,
      external_action_count:$external_action_count,
      formal_rerun_command_count:$formal_rerun_command_count,
      passed:$passed,
      total:$total,
      blockers:$blockers,
      warnings:$warnings,
      checks:$checks
    }' >"$SUMMARY"

  {
    echo "# Completion Blocker Closure Readiness"
    echo
    echo "- Run ID: \`$RUN_ID\`"
    echo "- Result: \`$result\`"
    echo "- Source audit: \`$AUDIT_JSON\`"
    echo "- Source audit run: \`$source_audit_run_id\` ($source_audit_result)"
    echo "- Current completion blockers: $blocker_count"
    echo "- Ready input packages or current evidence links: $ready_count"
    echo "- External or maintenance-window actions: $external_count"
    echo "- Formal rerun commands: $command_count"
    echo "- Stable package: \`$STABLE_BOOTSTRAP_DIR\`"
    echo
    echo "This package turns the latest project completion audit blockers into an execution board. It is review-required and does not mark the project complete."
    echo
    echo "## Blocker Ledger"
    echo
    jq -r '.closure_items[] | "- `" + .gate + "`: " + .closure_state + " / next: " + .next_action' "$BOOTSTRAP_DIR/closure-ledger.bootstrap.json"
    echo
    echo "## Formal Rerun Commands"
    echo
    sed -n '1,220p' "$BOOTSTRAP_DIR/formal-rerun-commands.md"
  } >"$LOCAL_REPORT"

  rm -rf "$STABLE_BOOTSTRAP_DIR"
  mkdir -p "$STABLE_BOOTSTRAP_DIR"
  cp -R "$BOOTSTRAP_DIR/." "$STABLE_BOOTSTRAP_DIR/"
  cp "$LOCAL_REPORT" "$STABLE_DIR/completion-blocker-closure-readiness-latest.md"
  cp "$SUMMARY" "$STABLE_DIR/completion-blocker-closure-readiness-latest.json"

  echo "completion-blocker-closure-readiness result=$result summary=$SUMMARY"
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

check_file "$AUDIT_JSON" "blocker" "latest project completion audit present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-contract-preflight-latest.json" "warn" "latest UI contract preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction-preflight-latest.json" "warn" "latest UI visual interaction dual gate present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction-gap-report-latest.json" "warn" "latest UI visual interaction gap report present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/capture-plan-latest.json" "warn" "direct APISIX UI capture plan present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/capture-session-latest.json" "warn" "direct APISIX UI capture session package present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json" "warn" "Windows Desktop bridge host preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json" "warn" "Windows Codex bridge runtime preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json" "warn" "Codex bridge tool surface preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json" "warn" "Windows Node REPL active execs preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json" "warn" "Windows Node REPL Chrome bridge smoke present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.json" "warn" "Windows Node REPL env matrix Chrome bridge smoke present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json" "warn" "Windows Node REPL scheduled Chrome bridge smoke present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json" "warn" "Windows SSH privilege preflight present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json" "warn" "Desktop Chrome bridge payload summary present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json" "warn" "Desktop Chrome bridge payload self-test present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json" "warn" "Desktop Chrome bridge MCP tool-call template present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/desktop-chrome-bridge-execution-request-latest.json" "warn" "Desktop Chrome trusted execution request present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/receiver-selftest-latest.json" "warn" "latest UI Desktop receiver self-test present"
check_file "$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/evidence-finalization-latest.json" "warn" "latest UI visual interaction evidence finalization present"
check_file "$ACCEPTANCE_DIR/05-security/network-policy-readiness/network-policy-enforcement-readiness-latest.json" "warn" "NetworkPolicy readiness package present"
check_file "$ACCEPTANCE_DIR/06-resilience/bootstrap/ha-drill-evidence-bootstrap-latest.json" "warn" "HA drill evidence bootstrap present"
check_file "$ACCEPTANCE_DIR/06-resilience/review/ha-drill-review-latest.json" "warn" "HA drill review packet present"
check_file "$ACCEPTANCE_DIR/03-performance/bootstrap/capture-performance-bootstrap-latest.json" "warn" "capture performance bootstrap present"
check_file "$ACCEPTANCE_DIR/03-performance/review/capture-performance-review-latest.json" "warn" "capture performance review packet present"
check_file "$ACCEPTANCE_DIR/04-detection-quality/bootstrap/detection-quality-bootstrap-latest.json" "warn" "detection quality bootstrap present"
check_file "$ACCEPTANCE_DIR/04-detection-quality/review/detection-quality-review-latest.json" "warn" "detection quality review packet present"
check_file "$ACCEPTANCE_DIR/02-regression/asset-discovery-site-inventory.bootstrap-latest.json" "warn" "asset inventory bootstrap present"
check_file "$ACCEPTANCE_DIR/02-regression/asset-inventory-review/asset-inventory-review-latest.json" "warn" "asset inventory review packet present"
check_file "$ACCEPTANCE_DIR/08-third-party/readiness/third-party-signoff-readiness-latest.json" "warn" "third-party signoff readiness package present"

python3 - "$RUN_ID" "$AUDIT_JSON" "$BOOTSTRAP_DIR" "$ACCEPTANCE_DIR" <<'PY'
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

run_id = sys.argv[1]
audit_path = Path(sys.argv[2])
out_dir = Path(sys.argv[3])
acceptance_dir = Path(sys.argv[4])
generated_at = datetime.now(timezone.utc).astimezone().isoformat()

audit = json.loads(audit_path.read_text(encoding="utf-8"))
blockers = audit.get("blocker_details") or []

def exists(rel):
    path = acceptance_dir / rel
    return path.is_file() and path.stat().st_size > 0

def run_id_from(rel):
    path = acceptance_dir / rel
    if not path.is_file():
        return ""
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return ""
    return str(data.get("run_id") or data.get("session_id") or "")

profiles = {
    "desktop_browser_smoke": {
        "owner": "QA / Desktop runtime",
        "closure_state": "direct_apisix_desktop_chrome_capture_required",
        "next_action": "run direct Codex Desktop Chrome extension visual capture against http://10.0.5.8:30180, with receiver and smoke redirect exposed directly on http://10.0.5.8:15174/15175; do not use Windows localhost tunnel or iab fallback",
        "external_action": True,
        "ready_inputs": [
            "02-regression/ui-contract-preflight-latest.json",
            "02-regression/ui-visual-interaction-preflight-latest.json",
            "02-regression/ui-visual-interaction-gap-report-latest.json",
            "02-regression/ui-visual-interaction/capture-plan-latest.json",
            "02-regression/ui-visual-interaction/capture-session-latest.json",
            "02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json",
            "02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-execution-request-latest.json",
            "02-regression/ui-visual-interaction/receiver-selftest-latest.json",
            "02-regression/ui-visual-interaction/evidence-finalization-latest.json",
        ],
        "formal_commands": [
            "SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs",
            "node tests/e2e/ui_codex_bridge_tool_surface_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_active_execs_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_chrome_bridge_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_env_matrix_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_scheduled_chrome_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_ssh_privilege_preflight.mjs",
            "python3 tests/e2e/ui_desktop_capture_receiver_selftest.py",
            "CODEX_CAPTURE_KEY=<redacted> DESKTOP_SMOKE_TOKEN=<redacted> tests/e2e/ui_desktop_capture_receiver.py --host 0.0.0.0 --port 15174 --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest --expected-width 1920 --expected-height 1080",
            "DESKTOP_SMOKE_TOKEN=<redacted> CODEX_SMOKE_NONCE=<redacted> tests/e2e/ui_desktop_smoke_redirect.py --host 0.0.0.0 --port 15175 --app-base-url http://10.0.5.8:30180 --default-route /dashboard --max-redirects 1",
            "node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://10.0.5.8:30180 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard",
            "tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175",
            "tests/e2e/ui_desktop_capture_session.mjs --session-id <session-id> --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175 --receiver-port 15174 --redirect-port 15175",
            "mcp__codex_desktop_node_repl.desktop_chrome_open_url url=http://10.0.5.8:15174/viewport-probe keep=true wait_ms=1500",
            "node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs",
            "node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs",
            "node tests/e2e/ui_desktop_chrome_bridge_execution_request.mjs",
            "mcp__codex_desktop_node_repl__js with JSON arguments from doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json after replacing placeholders",
            "RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json",
            "DESKTOP_CHROME_STATUS=pass CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-latest.json ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh",
        ],
        "required_evidence": [
            "capture-plan-latest.json lists all 30 visual targets and 28 interaction routes for direct http://10.0.5.8:30180 URLs",
            "ui-visual-interaction-gap-report-latest.json enumerates current visual and interaction gaps plus route-safe rerun context",
            "capture-session-latest.json binds direct receiver, direct redirect helper, visual batch, interaction batch, and formal rerun commands without replacing real evidence",
            "windows-desktop-bridge-host-preflight-latest.json result pass and selected Chrome client under C:/Users/LongShine",
            "windows-codex-bridge-runtime-preflight-latest.json result pass with Chrome/Codex/VSCode process inventory, explicit codex.exe version, MCP list, and node_repl stdio config shape without storing env values",
            "codex-bridge-tool-surface-preflight-latest.json result pass with local bridge plugin installed/enabled, proxy fallback initialized, no local 19998 backend listener, and current-session Desktop tool status recorded",
            "windows-node-repl-active-execs-preflight-latest.json result pass with SSH-visible active_execs/proxy evidence proving whether old Node REPL metadata maps to a live node_repl.exe PID or 19998 listener",
            "windows-node-repl-chrome-bridge-smoke-latest.json records direct SSH stdio node_repl minimal JS pass, no-env Chrome trust/native-pipe failure class, and full-env Chrome sandbox/firewall failure class without writing trusted env values",
            "windows-node-repl-env-matrix-smoke-latest.json records selected node_repl env subsets and proves none reached Chrome extension backend from SSH stdio without writing trusted env values",
            "windows-node-repl-scheduled-chrome-smoke-latest.json records the scheduled-task bridge launch attempt and whether /IT task creation can enter the Windows interactive context without writing trusted env values",
            "windows-ssh-privilege-preflight-latest.json result pass with Administrators group enabled, high-integrity token, net session not access-denied, firewall service running, and Codex/Chrome/node_repl processes visible",
            "desktop-chrome-bridge-payload-selftest-latest.json result pass with parseable Desktop Node REPL payload JS, direct APISIX URL templates, 30 visual targets, 28 interaction targets, and receiver uploads",
            "desktop-chrome-bridge-tool-call-latest.json result pass with 9/9 checks, tool mcp__codex_desktop_node_repl__js, timeout 900000, payload SHA256 3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a, and placeholder-only capture/smoke values",
            "desktop-chrome-bridge-execution-request-latest.json result ready_for_trusted_context with all non-secret readiness checks passing and bridge-run evidence still explicitly missing",
            "receiver-selftest-latest.json result pass proves receiver health, auth, viewport report, screenshot metadata, interaction upload, and sensitive-material rejection endpoints before Desktop capture",
            "desktop-chrome-viewport-probe-latest.json result pass before any visual screenshot upload",
            "evidence-finalization-latest.json proves all captured screenshots and interactions were finalized into strict metrics before preflight",
            "mcp__codex_desktop_node_repl__js or desktop_chrome_open_url reaches http://10.0.5.8:30180/login through Chrome extension backend",
            "protected /dashboard or /alerts business page opens without login/403",
            "UI contract preflight result pass",
            "per-route real React actual-1920.png screenshots at 1920x1080",
            "per-route diff-1920.png and metrics.json pass against UI source images",
            "per-visual-target capture-meta.json proves the uploaded Desktop Chrome screenshot was 1920x1080 before storage and was not post-capture resized",
            "per-route interaction.json proves no 4xx/5xx, requestfailed, pageerror, console error, and one route-specific business action",
        ],
    },
    "ui_visual_interaction": {
        "owner": "Frontend / QA",
        "closure_state": "direct_apisix_visual_diff_and_interaction_capture_missing",
        "next_action": "execute the full 30 visual / 28 interaction direct APISIX payload in the trusted Desktop Chrome extension backend, upload screenshots plus one bridge run summary to direct receiver, finalize evidence, then rerun the UI visual interaction gate",
        "external_action": True,
        "ready_inputs": [
            "02-regression/ui-visual-interaction-preflight-latest.json",
            "02-regression/ui-visual-interaction-gap-report-latest.json",
            "02-regression/ui-visual-interaction-matrix-latest.json",
            "02-regression/ui-visual-interaction/capture-plan-latest.json",
            "02-regression/ui-visual-interaction/capture-session-latest.json",
            "02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json",
            "02-regression/ui-visual-interaction/codex-bridge-tool-surface-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-active-execs-preflight-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-chrome-bridge-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-env-matrix-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-node-repl-scheduled-chrome-smoke-latest.json",
            "02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json",
            "02-regression/ui-visual-interaction/desktop-chrome-bridge-execution-request-latest.json",
            "02-regression/ui-visual-interaction/receiver-selftest-latest.json",
            "02-regression/ui-visual-interaction/evidence-finalization-latest.json",
        ],
        "formal_commands": [
            "SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs",
            "node tests/e2e/ui_codex_bridge_tool_surface_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_active_execs_preflight.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_chrome_bridge_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_env_matrix_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_scheduled_chrome_smoke.mjs",
            "SSHPASS=<redacted> node tests/e2e/ui_windows_ssh_privilege_preflight.mjs",
            "python3 tests/e2e/ui_desktop_capture_receiver_selftest.py",
            "CODEX_CAPTURE_KEY=<redacted> DESKTOP_SMOKE_TOKEN=<redacted> tests/e2e/ui_desktop_capture_receiver.py --host 0.0.0.0 --port 15174 --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest --expected-width 1920 --expected-height 1080",
            "DESKTOP_SMOKE_TOKEN=<redacted> CODEX_SMOKE_NONCE=<redacted> tests/e2e/ui_desktop_smoke_redirect.py --host 0.0.0.0 --port 15175 --app-base-url http://10.0.5.8:30180 --default-route /dashboard --max-redirects 1",
            "tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175",
            "tests/e2e/ui_desktop_capture_session.mjs --session-id <session-id> --receiver-url http://10.0.5.8:15174 --smoke-redirect-base-url http://10.0.5.8:15175 --receiver-port 15174 --redirect-port 15175",
            "mcp__codex_desktop_node_repl.desktop_chrome_open_url url=http://10.0.5.8:15174/viewport-probe keep=true wait_ms=1500",
            "node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs",
            "node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs",
            "node tests/e2e/ui_desktop_chrome_bridge_execution_request.mjs",
            "mcp__codex_desktop_node_repl__js with JSON arguments from doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json after replacing placeholders",
            "RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json",
            "DESKTOP_CHROME_STATUS=pass CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-latest.json ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh",
            "ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh",
        ],
        "required_evidence": [
            "capture-plan-latest.json summary records 30 visual targets and 28 interaction routes for direct http://10.0.5.8:30180 capture",
            "ui-visual-interaction-gap-report-latest.json groups current failures by cause and lists target-level metrics commands",
            "capture-session-latest.json lists the exact pending visual and interaction batches to execute through Codex Desktop Chrome extension without tunnels",
            "windows-codex-bridge-runtime-preflight-latest.json proves the Windows Codex runtime has explicit codex.exe, node_repl MCP enabled over stdio, and Chrome client files before trusted Desktop Chrome execution",
            "codex-bridge-tool-surface-preflight-latest.json records local plugin/proxy/backend/current-session tool-surface boundaries before trusted Desktop Chrome execution",
            "windows-node-repl-active-execs-preflight-latest.json records SSH-visible active_execs/proxy channel status before trusted Desktop Chrome execution",
            "windows-node-repl-chrome-bridge-smoke-latest.json proves SSH stdio node_repl can execute minimal JS while Chrome extension discovery still fails outside the trusted Desktop native-pipe context",
            "windows-node-repl-env-matrix-smoke-latest.json proves selected env-subset attempts were tested and still require the trusted current-session Desktop Chrome MCP context",
            "windows-node-repl-scheduled-chrome-smoke-latest.json proves the automated scheduled-task route was tested before falling back to the trusted current-session Desktop Chrome MCP tool requirement",
            "windows-ssh-privilege-preflight-latest.json proves the recurring Node REPL JS failure is not explained by missing admin group, low-integrity SSH token, stopped firewall service, or absent Codex/Chrome/node_repl processes",
            "desktop-chrome-bridge-payload-latest.json requires codex-desktop-chrome-extension, forbids iab, and covers 58 screenshots plus bridge result upload",
            "desktop-chrome-bridge-payload-selftest-latest.json result pass with parseable Desktop Node REPL payload JS, direct APISIX URL templates, 30 visual targets, 28 interaction targets, and receiver uploads",
            "desktop-chrome-bridge-tool-call-latest.json result pass with 9/9 checks and the exact mcp__codex_desktop_node_repl__js arguments for the trusted Desktop Node REPL context",
            "desktop-chrome-bridge-execution-request-latest.json result ready_for_trusted_context with all non-secret readiness checks passing and bridge-run evidence still explicitly missing",
            "receiver-selftest-latest.json result pass proves receiver health, auth, viewport report, screenshot metadata, interaction upload, and sensitive-material rejection endpoints before Desktop capture",
            "desktop-chrome-viewport-probe-latest.json result pass before actual-1920.png upload",
            "evidence-finalization-latest.json result pass after generating diff and metrics from captured actual screenshots",
            "30 visual targets each have 1920x1080 actual-1920.png, diff-1920.png, metrics.json with status pass, and capture-meta.json with original uploaded size 1920x1080",
            "28 route ids each have interaction.json, interaction.png, and interaction-capture-meta.json with status pass; currently all remain missing or failing until payload execution",
            "no direct use of source design PNGs as frontend page implementation",
            "Desktop Chrome backend evidence is pass",
        ],
    },
    "production_security": {
        "owner": "Security / SRE",
        "closure_state": "external_cni_and_waiver_required",
        "next_action": "install or migrate to a policy-capable CNI, review runtime waivers for privileged/hostNetwork workloads, then rerun production security preflight",
        "external_action": True,
        "ready_inputs": [
            "05-security/production-security-preflight-latest.json",
            "05-security/network-policy-readiness/network-policy-enforcement-readiness-latest.json",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false tests/e2e/live_network_policy_enforcement_preflight.sh",
            "ALLOW_BLOCKERS=false tests/e2e/live_production_security_preflight.sh",
        ],
        "required_evidence": [
            "policy-capable CNI pods > 0",
            "NetworkPolicy negative probe passes",
            "production-security-preflight-latest.json result pass",
        ],
    },
    "network_policy_enforcement": {
        "owner": "Security / Network",
        "closure_state": "external_cni_required",
        "next_action": "use network-policy readiness package to migrate CNI, then run isolated default-deny and allow-list probe",
        "external_action": True,
        "ready_inputs": [
            "05-security/network-policy-readiness/network-policy-enforcement-readiness-latest.json",
            "05-security/network-policy-enforcement-preflight-latest.json",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh",
        ],
        "required_evidence": [
            "policy-capable CNI pods > 0",
            "default-deny blocks isolated client",
            "allow-list restores isolated client",
        ],
    },
    "ha_rto_rpo": {
        "owner": "SRE / QA",
        "closure_state": "maintenance_window_required",
        "next_action": "execute destructive Kafka/Flink/ClickHouse/PostgreSQL/MinIO drills using HA bootstrap templates and publish formal RTO/RPO reports",
        "external_action": True,
        "ready_inputs": [
            "06-resilience/bootstrap/ha-drill-evidence-bootstrap-latest.json",
            "06-resilience/review/ha-drill-review-latest.json",
            "06-resilience/ha-readiness-preflight-latest.json",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false tests/chaos/live_ha_readiness_preflight.sh",
        ],
        "required_evidence": [
            "kafka-failover.md",
            "flink-failover.md",
            "clickhouse-failover.md",
            "postgres-failover.md",
            "minio-failover.md",
            "ha-rto-rpo-latest.json",
        ],
    },
    "capture_performance": {
        "owner": "Performance / Probe",
        "closure_state": "hardware_window_required",
        "next_action": "fill hardware and traffic profiles, run 10 x 100Gbps and 512Mpps tests, then rerun capture performance preflight",
        "external_action": True,
        "ready_inputs": [
            "03-performance/bootstrap/capture-performance-bootstrap-latest.json",
            "03-performance/review/capture-performance-review-latest.json",
            "03-performance/capture-performance-preflight-latest.json",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false tests/perf/100g_capture/live_capture_performance_preflight.sh",
        ],
        "required_evidence": [
            "tests/perf/100g_capture/hardware-inventory.yaml",
            "tests/perf/100g_capture/traffic-profile.yaml",
            "tests/perf/100g_capture/results/10x100g-summary.json",
            "tests/perf/100g_capture/results/512mpps-summary.json",
        ],
    },
    "detection_quality": {
        "owner": "Algorithm / Third-party QA",
        "closure_state": "third_party_adjudication_required",
        "next_action": "freeze dataset, fill labels and predictions, lock thresholds, obtain third-party attestation, then rerun detection quality preflight",
        "external_action": True,
        "ready_inputs": [
            "04-detection-quality/bootstrap/detection-quality-bootstrap-latest.json",
            "04-detection-quality/review/detection-quality-review-latest.json",
            "04-detection-quality/detection-quality-preflight-latest.json",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false tests/e2e/live_detection_quality_preflight.sh",
        ],
        "required_evidence": [
            "mlops/eval_packages/topic1_blind/dataset-manifest.yaml",
            "mlops/eval_packages/topic1_blind/threshold-lock.json",
            "mlops/eval_packages/topic1_blind/labels.csv",
            "mlops/eval_packages/topic1_blind/predictions.csv",
            "mlops/eval_packages/topic1_blind/third-party-attestation.yaml",
        ],
    },
    "asset_discovery_coverage": {
        "owner": "Implementation / Site owner",
        "closure_state": "site_inventory_required",
        "next_action": "review observed asset inventory bootstrap with site owner, produce authoritative SITE_ASSET_INVENTORY_JSON, then rerun coverage gate",
        "external_action": True,
        "ready_inputs": [
            "02-regression/asset-discovery-site-inventory.bootstrap-latest.json",
            "02-regression/asset-inventory-review/asset-inventory-review-latest.json",
            "02-regression/asset-discovery-coverage-latest.json",
        ],
        "formal_commands": [
            "SITE_ASSET_INVENTORY_JSON=/path/to/site-assets.json MIN_DISCOVERY_COVERAGE_PCT=95 ALLOW_BLOCKERS=false tests/e2e/live_asset_discovery_coverage_report.sh",
        ],
        "required_evidence": [
            "site-owner-reviewed SITE_ASSET_INVENTORY_JSON",
            "asset-discovery-coverage-latest.json result pass",
        ],
    },
    "trial_third_party_signoff": {
        "owner": "Project manager / User / Third-party",
        "closure_state": "signature_and_external_report_required",
        "next_action": "fill signoff placeholders, resolve upstream exceptions, attach pilot/third-party/economic-benefit confirmations, then rerun project completion audit",
        "external_action": True,
        "ready_inputs": [
            "08-third-party/readiness/third-party-signoff-readiness-latest.json",
            "08-third-party/user-acceptance-signoff.md",
        ],
        "formal_commands": [
            "ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh",
        ],
        "required_evidence": [
            "user-acceptance-signoff.md has no TBD placeholders",
            "user or pilot owner signature",
            "third-party report or formal exception decision",
            "economic-benefit confirmation",
        ],
    },
}

closure_items = []
for blocker in blockers:
    gate = blocker.get("gate", "")
    profile = profiles.get(gate, {
        "owner": "TBD",
        "closure_state": "unclassified",
        "next_action": "classify this blocker and define formal closure evidence",
        "external_action": True,
        "ready_inputs": [blocker.get("artifact", "")],
        "formal_commands": ["ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh"],
        "required_evidence": ["TBD"],
    })
    ready_inputs = []
    for rel in profile["ready_inputs"]:
        ready_inputs.append({
            "path": rel,
            "exists": exists(rel),
            "run_id": run_id_from(rel),
        })
    ready_input_count = sum(1 for entry in ready_inputs if entry["exists"])
    missing_input_count = len(ready_inputs) - ready_input_count
    closure_items.append({
        "gate": gate,
        "status": blocker.get("status", ""),
        "owner": profile["owner"],
        "closure_state": profile["closure_state"],
        "next_action": profile["next_action"],
        "external_action": profile["external_action"],
        "artifact": blocker.get("artifact", ""),
        "blocker_detail": blocker.get("detail", ""),
        "ready_inputs": ready_inputs,
        "ready_input_count": ready_input_count,
        "missing_input_count": missing_input_count,
        "formal_commands": profile["formal_commands"],
        "required_evidence": profile["required_evidence"],
    })

out_dir.mkdir(parents=True, exist_ok=True)
ledger = {
    "package_id": "completion_blocker_closure_readiness",
    "run_id": run_id,
    "generated_at": generated_at,
    "source_audit": str(audit_path),
    "source_audit_run_id": audit.get("run_id", ""),
    "source_audit_result": audit.get("result", ""),
    "review_required": True,
    "formal_gate_note": "This package indexes blocker closure paths only; it does not close any acceptance gate.",
    "blocker_count": len(closure_items),
    "ready_input_count": sum(1 for item in closure_items for entry in item["ready_inputs"] if entry["exists"]),
    "external_action_count": sum(1 for item in closure_items if item["external_action"]),
    "formal_rerun_command_count": sum(len(item["formal_commands"]) for item in closure_items),
    "closure_items": closure_items,
}
(out_dir / "closure-ledger.bootstrap.json").write_text(json.dumps(ledger, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

with (out_dir / "closure-board.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=[
        "gate", "owner", "closure_state", "external_action", "next_action",
        "ready_input_count", "missing_input_count", "ready_inputs", "required_evidence", "review_status", "reviewer", "target_date", "notes",
    ])
    writer.writeheader()
    for item in closure_items:
        writer.writerow({
            "gate": item["gate"],
            "owner": item["owner"],
            "closure_state": item["closure_state"],
            "external_action": str(item["external_action"]).lower(),
            "next_action": item["next_action"],
            "ready_input_count": item["ready_input_count"],
            "missing_input_count": item["missing_input_count"],
            "ready_inputs": "; ".join(f"{entry['path']} ({'ok' if entry['exists'] else 'missing'})" for entry in item["ready_inputs"]),
            "required_evidence": "; ".join(item["required_evidence"]),
            "review_status": "TBD",
            "reviewer": "TBD",
            "target_date": "TBD",
            "notes": "TBD",
        })

with (out_dir / "formal-rerun-commands.md").open("w", encoding="utf-8") as handle:
    handle.write("# Formal Rerun Commands\n\n")
    for item in closure_items:
        handle.write(f"## {item['gate']}\n\n")
        for command in item["formal_commands"]:
            handle.write("```bash\n")
            handle.write(command + "\n")
            handle.write("```\n\n")

with (out_dir / "blocker-owner-matrix.review-template.md").open("w", encoding="utf-8") as handle:
    handle.write("# Blocker Owner Matrix\n\n")
    handle.write("| Gate | Owner | Closure State | Next Action |\n")
    handle.write("|---|---|---|---|\n")
    for item in closure_items:
        handle.write(f"| {item['gate']} | {item['owner']} | {item['closure_state']} | {item['next_action']} |\n")

with (out_dir / "exception-register.review-template.csv").open("w", encoding="utf-8", newline="") as handle:
    writer = csv.DictWriter(handle, fieldnames=[
        "gate", "exception_requested", "reason", "risk", "approver", "expiration", "replacement_evidence",
    ])
    writer.writeheader()
    for item in closure_items:
        writer.writerow({
            "gate": item["gate"],
            "exception_requested": "false",
            "reason": "TBD",
            "risk": "TBD",
            "approver": "TBD",
            "expiration": "TBD",
            "replacement_evidence": "TBD",
        })

evidence_map = {
    "run_id": run_id,
    "generated_at": generated_at,
    "source_audit": str(audit_path),
    "entries": [
        {
            "gate": item["gate"],
            "ready_input_status": item["ready_inputs"],
            "formal_commands": item["formal_commands"],
        }
        for item in closure_items
    ],
}
(out_dir / "evidence-readiness-map.json").write_text(json.dumps(evidence_map, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

with (out_dir / "README.md").open("w", encoding="utf-8") as handle:
    handle.write("# Completion Blocker Closure Package\n\n")
    handle.write("This package is generated from the latest project completion audit. It is a review-required work board and does not close any acceptance gate.\n\n")
    handle.write("## Files\n\n")
    for name in [
        "closure-ledger.bootstrap.json",
        "closure-board.review-template.csv",
        "blocker-owner-matrix.review-template.md",
        "formal-rerun-commands.md",
        "evidence-readiness-map.json",
        "exception-register.review-template.csv",
    ]:
        handle.write(f"- `{name}`\n")
PY

json_log "package" "completion blocker closure ledger written" "info" true "ok" "$BOOTSTRAP_DIR" "closure-ledger.bootstrap.json"

finalize
