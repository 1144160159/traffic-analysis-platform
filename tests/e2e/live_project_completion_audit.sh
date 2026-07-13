#!/usr/bin/env bash
set -euo pipefail

RUN_ID="${RUN_ID:-$(date +%Y%m%d%H%M%S)-project-completion-audit}"
LOG_DIR="${LOG_DIR:-doc/02_acceptance/runs/$RUN_ID}"
ACCEPTANCE_DIR="${ACCEPTANCE_DIR:-doc/02_acceptance}"
COMPLETION_DIR="${COMPLETION_DIR:-$ACCEPTANCE_DIR/09-completion}"
ALLOW_BLOCKERS="${ALLOW_BLOCKERS:-false}"

REPORT="$LOG_DIR/live-project-completion-audit-$RUN_ID.ndjson"
SUMMARY="$LOG_DIR/live-project-completion-audit-$RUN_ID-summary.json"
LOCAL_REPORT="$LOG_DIR/local-report.md"

mkdir -p "$LOG_DIR" "$COMPLETION_DIR"
: >"$REPORT"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 2
  fi
}

json_log() {
  local gate="$1" phase="$2" name="$3" severity="$4" passed="$5" status="$6" detail="${7:-}" artifact="${8:-}"
  jq -nc \
    --arg ts "$(date -Iseconds)" \
    --arg gate "$gate" \
    --arg phase "$phase" \
    --arg name "$name" \
    --arg severity "$severity" \
    --argjson passed "$passed" \
    --arg status "$status" \
    --arg detail "$detail" \
    --arg artifact "$artifact" \
    '{ts:$ts, gate:$gate, phase:$phase, name:$name, severity:$severity, passed:$passed, status:$status, detail:$detail, artifact:$artifact}' >>"$REPORT"
}

jq_num() {
  local expr="$1" file="$2"
  jq -r "$expr" "$file"
}

latest_json_gate() {
  local gate="$1" name="$2" file="$3" min_passed="${4:-1}"
  if [[ ! -s "$file" ]]; then
    json_log "$gate" "evidence" "$name" "blocker" false "missing" "$file" "$file"
    return
  fi

  local result run_id passed blockers warnings total failing detail status
  result="$(jq -r '.result // "unknown"' "$file")"
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  passed="$(jq_num '(.passed // ([.checks[]? | select(.passed == true)] | length))' "$file")"
  blockers="$(jq_num '(.blockers // ([.checks[]? | select((.passed == false) and (.severity == "blocker"))] | length))' "$file")"
  warnings="$(jq_num '(.warnings // ([.checks[]? | select((.passed == false) and ((.severity == "warn") or (.severity == "warning")))] | length))' "$file")"
  total="$(jq_num '(.total // ([.checks[]?] | length))' "$file")"
  failing="$(jq -r '[.checks[]? | select(.passed == false) | ((.phase // "unknown") + "/" + (.name // "unnamed") + ": " + (.status // "unknown") + " " + (.detail // ""))] | join("; ")' "$file")"
  detail="run_id=$run_id result=$result passed=$passed total=$total blockers=$blockers warnings=$warnings"

  if [[ "$result" == "pass" && "$blockers" -eq 0 && "$passed" -ge "$min_passed" ]]; then
    status="pass"
    if [[ "$warnings" -gt 0 ]]; then
      status="pass_with_warnings"
    fi
    json_log "$gate" "evidence" "$name" "info" true "$status" "$detail" "$file"
  else
    if [[ -n "$failing" ]]; then
      detail="$detail; failing=$failing"
    fi
    json_log "$gate" "evidence" "$name" "blocker" false "$result" "$detail" "$file"
  fi
}

latest_json_all_pass() {
  local gate="$1" name="$2"
  shift 2
  local file result blockers warnings run_id failures=0 passed_count=0 detail="" artifacts=""

  for file in "$@"; do
    artifacts="${artifacts}${artifacts:+, }$file"
    if [[ ! -s "$file" ]]; then
      failures=$((failures + 1))
      detail="${detail}${detail:+; }missing $file"
      continue
    fi
    result="$(jq -r '.result // "unknown"' "$file")"
    run_id="$(jq -r '.run_id // "unknown"' "$file")"
    blockers="$(jq_num '(.blockers // ([.checks[]? | select((.passed == false) and (.severity == "blocker"))] | length))' "$file")"
    warnings="$(jq_num '(.warnings // ([.checks[]? | select((.passed == false) and ((.severity == "warn") or (.severity == "warning")))] | length))' "$file")"
    if [[ "$result" == "pass" && "$blockers" -eq 0 ]]; then
      passed_count=$((passed_count + 1))
    else
      failures=$((failures + 1))
      detail="${detail}${detail:+; }$file run_id=$run_id result=$result blockers=$blockers warnings=$warnings"
    fi
  done

  if [[ "$failures" -eq 0 ]]; then
    json_log "$gate" "evidence" "$name" "info" true "pass" "artifacts=$passed_count" "$artifacts"
  else
    json_log "$gate" "evidence" "$name" "blocker" false "blocked" "$detail" "$artifacts"
  fi
}

release_manifest_gate() {
  local file="$ACCEPTANCE_DIR/00-baseline/release-manifest-latest.json"
  if [[ ! -s "$file" ]]; then
    json_log "baseline_release_manifest" "evidence" "release manifest latest evidence exists" "blocker" false "missing" "$file" "$file"
    return
  fi

  local run_id evidence_runs workloads pod_images live_topics api_catalogs detail
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  evidence_runs="$(jq_num '(.evidence_runs // []) | length' "$file")"
  workloads="$(jq_num '(.kubernetes.workloads // []) | length' "$file")"
  pod_images="$(jq_num '(.kubernetes.pod_images // []) | length' "$file")"
  live_topics="$(jq_num '(.kafka.live_topics // []) | length' "$file")"
  api_catalogs="$(jq_num '(.api_catalogs // []) | length' "$file")"
  detail="run_id=$run_id evidence_runs=$evidence_runs workloads=$workloads pod_images=$pod_images live_topics=$live_topics api_catalogs=$api_catalogs"

  if [[ "$run_id" != "unknown" && "$evidence_runs" -gt 0 && "$workloads" -gt 0 && "$pod_images" -gt 0 && "$live_topics" -gt 0 ]]; then
    json_log "baseline_release_manifest" "evidence" "release manifest indexes current acceptance evidence" "info" true "pass" "$detail" "$file"
  else
    json_log "baseline_release_manifest" "evidence" "release manifest indexes current acceptance evidence" "blocker" false "incomplete" "$detail" "$file"
  fi
}

ui_contract_gate() {
  local file="$ACCEPTANCE_DIR/02-regression/ui-contract-preflight-latest.json"
  if [[ ! -s "$file" ]]; then
    json_log "ui_contract" "evidence" "UI contract preflight latest evidence exists" "blocker" false "missing" "$file" "$file"
    return
  fi

  local visual_file="$ACCEPTANCE_DIR/02-regression/ui-visual-interaction-preflight-latest.json"
  local run_id non_browser_passed non_browser_blockers browser_total browser_failed browser_detail visual_run_id desktop_status desktop_detail desktop_artifact interaction_passed interaction_required visual_passed visual_required node_repl_smoke node_repl_direct node_repl_no_env_failure node_repl_full_env_failure scheduled_smoke scheduled_create scheduled_blocker smoke_detail smoke_artifact
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  non_browser_passed="$(jq_num '[.checks[]? | select(.phase != "browser" and .passed == true)] | length' "$file")"
  non_browser_blockers="$(jq_num '[.checks[]? | select(.phase != "browser" and .passed == false and .severity == "blocker")] | length' "$file")"
  browser_total="$(jq_num '[.checks[]? | select(.phase == "browser")] | length' "$file")"
  browser_failed="$(jq_num '[.checks[]? | select(.phase == "browser" and .passed == false)] | length' "$file")"
  browser_detail="$(jq -r '[.checks[]? | select(.phase == "browser" and .passed == false) | ((.name // "browser") + ": " + (.status // "unknown") + " " + (.detail // ""))] | join("; ")' "$file")"

  if [[ "$non_browser_blockers" -eq 0 && "$non_browser_passed" -gt 0 ]]; then
    json_log "ui_contract_static" "evidence" "UI contracts, menu routes, API coverage and overlay constraints pass outside browser transport" "info" true "pass" "run_id=$run_id non_browser_passed=$non_browser_passed non_browser_blockers=$non_browser_blockers" "$file"
  else
    json_log "ui_contract_static" "evidence" "UI contracts, menu routes, API coverage and overlay constraints pass outside browser transport" "blocker" false "blocked" "run_id=$run_id non_browser_passed=$non_browser_passed non_browser_blockers=$non_browser_blockers" "$file"
  fi

  if [[ -s "$visual_file" ]]; then
    visual_run_id="$(jq -r '.run_id // "unknown"' "$visual_file")"
    desktop_status="$(jq -r '.desktop_chrome_status // "unknown"' "$visual_file")"
    desktop_detail="$(jq -r '.desktop_chrome_detail // ""' "$visual_file")"
    desktop_artifact="$(jq -r '.desktop_chrome_artifact // ""' "$visual_file")"
    interaction_passed="$(jq_num '(.interaction_passed_count // 0)' "$visual_file")"
    interaction_required="$(jq_num '(.interaction_required_count // 0)' "$visual_file")"
    visual_passed="$(jq_num '(.visual_diff_passed_count // 0)' "$visual_file")"
    visual_required="$(jq_num '(.visual_diff_required_count // 0)' "$visual_file")"
    node_repl_smoke="$(jq -r '.node_repl_chrome_smoke_result // "unknown"' "$visual_file")"
    node_repl_direct="$(jq -r '.node_repl_direct_js_result // "unknown"' "$visual_file")"
    node_repl_no_env_failure="$(jq -r '.node_repl_chrome_no_env_failure_class // "unknown"' "$visual_file")"
    node_repl_full_env_failure="$(jq -r '.node_repl_chrome_full_env_failure_class // "unknown"' "$visual_file")"
    scheduled_smoke="$(jq -r '.scheduled_chrome_smoke_result // "unknown"' "$visual_file")"
    scheduled_create="$(jq -r 'if has("scheduled_chrome_task_create_ok") then (.scheduled_chrome_task_create_ok | tostring) else "unknown" end' "$visual_file")"
    scheduled_blocker="$(jq -r '.scheduled_chrome_first_blocker // "none"' "$visual_file")"
    smoke_detail="ui_contract_run_id=$run_id browser_checks=$browser_total browser_failed=$browser_failed visual_run_id=$visual_run_id desktop_chrome_status=$desktop_status desktop_chrome_artifact=${desktop_artifact:-missing} business_interaction=$interaction_passed/$interaction_required visual_diff=$visual_passed/$visual_required node_repl_chrome_smoke=$node_repl_smoke direct_js=$node_repl_direct chrome_no_env_failure=$node_repl_no_env_failure chrome_full_env_failure=$node_repl_full_env_failure scheduled_chrome_smoke=$scheduled_smoke scheduled_task_create_ok=$scheduled_create scheduled_chrome_first_blocker=$scheduled_blocker"
    smoke_artifact="${desktop_artifact:-$visual_file}"
  else
    desktop_status="missing"
    interaction_passed=0
    interaction_required=0
    smoke_detail="ui_contract_run_id=$run_id browser_checks=$browser_total browser_failed=$browser_failed visual_preflight=missing"
    smoke_artifact="$file"
  fi

  if [[ "$desktop_status" =~ ^(pass|passed|ok)$ && "$interaction_passed" -gt 0 ]]; then
    json_log "desktop_browser_smoke" "evidence" "Codex Desktop Chrome wrapper validates login and business pages" "info" true "pass" "$smoke_detail" "$smoke_artifact"
  else
    if [[ -n "${desktop_detail:-}" ]]; then
      smoke_detail="$smoke_detail; current_desktop_detail=$desktop_detail"
    elif [[ -n "$browser_detail" ]]; then
      smoke_detail="$smoke_detail; legacy_browser_detail=$browser_detail"
    else
      smoke_detail="$smoke_detail; no successful Desktop Chrome evidence"
    fi
    json_log "desktop_browser_smoke" "evidence" "Codex Desktop Chrome wrapper validates login and business pages" "blocker" false "blocked" "$smoke_detail" "$smoke_artifact"
  fi
}

oidc_sso_gate() {
  local file="$ACCEPTANCE_DIR/02-regression/oidc-sso/oidc-sso-preflight-latest.json"
  local name="OIDC/SSO login entry and callback flow pass live gateway"
  if [[ ! -s "$file" ]]; then
    json_log "oidc_sso" "evidence" "$name" "blocker" false "missing" "$file" "$file"
    return
  fi

  local run_id result base_url issuer auth_endpoint token_endpoint login_asset login_chunk callback_chunk oidc_status oidc_location keycloak_status failure_count failures detail
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  result="$(jq -r '.result // "unknown"' "$file")"
  base_url="$(jq -r '.base_url // ""' "$file")"
  issuer="$(jq -r '.discovery.issuer // ""' "$file")"
  auth_endpoint="$(jq -r '.discovery.authorization_endpoint // ""' "$file")"
  token_endpoint="$(jq -r '.discovery.token_endpoint // ""' "$file")"
  login_asset="$(jq -r '.frontend.login_index_asset // ""' "$file")"
  login_chunk="$(jq -r '.frontend.login_chunk // ""' "$file")"
  callback_chunk="$(jq -r '.frontend.oidc_callback_chunk // ""' "$file")"
  oidc_status="$(jq -r '.oidc_login.status // ""' "$file")"
  oidc_location="$(jq -r '.oidc_login.location // ""' "$file")"
  keycloak_status="$(jq -r '.keycloak_authorization_page.status // ""' "$file")"
  failure_count="$(jq_num '(.failures // []) | length' "$file")"
  failures="$(jq -r '(.failures // []) | join("; ")' "$file")"
  detail="run_id=$run_id result=$result base_url=$base_url issuer=$issuer authorization_endpoint=$auth_endpoint token_endpoint=$token_endpoint login_asset=$login_asset login_chunk=$login_chunk callback_chunk=$callback_chunk oidc_status=$oidc_status keycloak_status=$keycloak_status failures=$failure_count"

  if [[ ( "$result" == "pass" || "$result" == "passed" ) && "$failure_count" -eq 0 && -n "$base_url" && "$issuer" == "$base_url/realms/master" && "$auth_endpoint" == "$base_url/realms/master/protocol/openid-connect/auth" && "$token_endpoint" == "$base_url/realms/master/protocol/openid-connect/token" && -n "$login_asset" && -n "$login_chunk" && -n "$callback_chunk" && "$oidc_status" == "302" && "$oidc_location" == "$base_url/realms/master/protocol/openid-connect/auth"* && "$oidc_location" == *"client_id=traffic-ui"* && "$keycloak_status" == "200" ]]; then
    json_log "oidc_sso" "evidence" "$name" "info" true "pass" "$detail" "$file"
  else
    if [[ -n "$failures" ]]; then
      detail="$detail; failures=$failures"
    fi
    json_log "oidc_sso" "evidence" "$name" "blocker" false "blocked" "$detail" "$file"
  fi
}

ui_visual_interaction_gate() {
  local file="$ACCEPTANCE_DIR/02-regression/ui-visual-interaction-preflight-latest.json"
  if [[ ! -s "$file" ]]; then
    json_log "ui_visual_interaction" "evidence" "UI visual diff and business interaction dual gate passes" "blocker" false "missing" "$file" "$file"
    return
  fi

  local run_id result blockers warnings visual_passed visual_required interaction_passed interaction_required desktop_status desktop_artifact design_refs components_present routes_total sources_present sources_total capture_session_path capture_session_status capture_session_covers capture_session_run_id capture_session_visual_batch capture_session_interaction_batch bridge_runtime_ready bridge_runtime_result bridge_runtime_path bridge_runtime_chrome_clients bridge_runtime_explicit_codex bridge_runtime_explicit_codex_versions bridge_runtime_mcp_list_lines bridge_runtime_node_repl_mcp_result bridge_runtime_node_repl_mcp_transport bridge_runtime_node_repl_mcp_env_keys bridge_runtime_process_counts tool_surface_ready tool_surface_result tool_surface_path tool_surface_plugin tool_surface_backend_open tool_surface_tools_list tool_surface_session_tools active_execs_ready active_execs_result active_execs_path active_execs_channel active_execs_records active_execs_live_records active_execs_stale_records active_execs_node_repl_pids active_execs_port_19998_listeners node_repl_smoke_ready node_repl_smoke_result node_repl_smoke_path node_repl_direct_js node_repl_full_env_js node_repl_chrome_no_env_result node_repl_chrome_no_env_failure node_repl_chrome_full_env_result node_repl_chrome_full_env_failure node_repl_env_matrix_ready node_repl_env_matrix_result node_repl_env_matrix_path node_repl_env_matrix_cases node_repl_env_matrix_js_pass node_repl_env_matrix_chrome_ready node_repl_env_matrix_native_pipe node_repl_env_matrix_sandbox node_repl_env_matrix_blocker scheduled_smoke_ready scheduled_smoke_result scheduled_smoke_path scheduled_create scheduled_run scheduled_runner scheduled_blocker ssh_privilege_ready ssh_privilege_result ssh_privilege_path ssh_privilege_high_integrity ssh_privilege_enabled_privileges ssh_privilege_net_session ssh_privilege_firewall ssh_privilege_process_counts ssh_privilege_powershell_blocked payload_selftest_ready payload_selftest_result payload_selftest_path payload_selftest_passed payload_selftest_total payload_selftest_visual payload_selftest_interaction payload_selftest_receiver bridge_tool_call_ready bridge_tool_call_result bridge_tool_call_path bridge_tool_call_tool bridge_tool_call_passed bridge_tool_call_total bridge_tool_call_timeout bridge_tool_call_sha bridge_tool_call_visual bridge_tool_call_interaction bridge_tool_call_receiver bridge_run_result_path bridge_run_result_exists finalization_file finalization_run_id finalization_result finalization_visual_passed finalization_visual_total finalization_interaction_passed finalization_interaction_total detail failing
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  result="$(jq -r '.result // "unknown"' "$file")"
  blockers="$(jq_num '(.blockers // ([.checks[]? | select((.passed == false) and (.severity == "blocker"))] | length))' "$file")"
  warnings="$(jq_num '(.warnings // ([.checks[]? | select((.passed == false) and ((.severity == "warn") or (.severity == "warning")))] | length))' "$file")"
  visual_passed="$(jq_num '(.visual_diff_passed_count // 0)' "$file")"
  visual_required="$(jq_num '(.visual_diff_required_count // 0)' "$file")"
  interaction_passed="$(jq_num '(.interaction_passed_count // 0)' "$file")"
  interaction_required="$(jq_num '(.interaction_required_count // 0)' "$file")"
  desktop_status="$(jq -r '.desktop_chrome_status // "unknown"' "$file")"
  desktop_artifact="$(jq -r '.desktop_chrome_artifact // ""' "$file")"
  design_refs="$(jq_num '(.full_page_design_image_reference_count // 0)' "$file")"
  components_present="$(jq_num '(.react_page_present_count // 0)' "$file")"
  routes_total="$(jq_num '(.target_route_count // 0)' "$file")"
  sources_present="$(jq_num '(.target_source_image_present_count // 0)' "$file")"
  sources_total="$(jq_num '(.visual_target_count // 0)' "$file")"
  capture_session_path="$(jq -r '.capture_session_path // ""' "$file")"
  capture_session_status="$(jq -r '.capture_session_status // "unknown"' "$file")"
  capture_session_covers="$(jq -r '.capture_session_covers_current_gaps // false' "$file")"
  capture_session_run_id="$(jq -r '.capture_session_run_id // ""' "$file")"
  capture_session_visual_batch="$(jq_num '(.capture_session_visual_batch_count // 0)' "$file")"
  capture_session_interaction_batch="$(jq_num '(.capture_session_interaction_batch_count // 0)' "$file")"
  bridge_runtime_ready="$(jq -r '.bridge_runtime_preflight_ready // false' "$file")"
  bridge_runtime_result="$(jq -r '.bridge_runtime_preflight_result // "unknown"' "$file")"
  bridge_runtime_path="$(jq -r '.bridge_runtime_preflight_path // ""' "$file")"
  bridge_runtime_chrome_clients="$(jq_num '(.bridge_runtime_chrome_client_count // 0)' "$file")"
  bridge_runtime_explicit_codex="$(jq -r '.bridge_runtime_explicit_codex_exists // "unknown"' "$file")"
  bridge_runtime_explicit_codex_versions="$(jq_num '(.bridge_runtime_explicit_codex_version_count // 0)' "$file")"
  bridge_runtime_mcp_list_lines="$(jq_num '(.bridge_runtime_mcp_list_line_count // 0)' "$file")"
  bridge_runtime_node_repl_mcp_result="$(jq -r '.bridge_runtime_node_repl_mcp_result // "unknown"' "$file")"
  bridge_runtime_node_repl_mcp_transport="$(jq -r '.bridge_runtime_node_repl_mcp_transport_type // "unknown"' "$file")"
  bridge_runtime_node_repl_mcp_env_keys="$(jq_num '(.bridge_runtime_node_repl_mcp_env_key_count // 0)' "$file")"
  bridge_runtime_process_counts="$(jq -cr '(.bridge_runtime_process_counts // {})' "$file")"
  tool_surface_ready="$(jq -r '.tool_surface_preflight_ready // false' "$file")"
  tool_surface_result="$(jq -r '.tool_surface_preflight_result // "unknown"' "$file")"
  tool_surface_path="$(jq -r '.tool_surface_preflight_path // ""' "$file")"
  tool_surface_plugin="$(jq -r '.tool_surface_plugin_installed_enabled // false' "$file")"
  tool_surface_backend_open="$(jq -r '.tool_surface_backend_open // false' "$file")"
  tool_surface_tools_list="$(jq -r '.tool_surface_tools_list_status // "unknown"' "$file")"
  tool_surface_session_tools="$(jq -r '.tool_surface_session_tool_status // "unknown"' "$file")"
  active_execs_ready="$(jq -r '.active_execs_preflight_ready // false' "$file")"
  active_execs_result="$(jq -r '.active_execs_preflight_result // "unknown"' "$file")"
  active_execs_path="$(jq -r '.active_execs_preflight_path // ""' "$file")"
  active_execs_channel="$(jq -r '.active_execs_channel_status // "unknown"' "$file")"
  active_execs_records="$(jq_num '(.active_execs_record_count // 0)' "$file")"
  active_execs_live_records="$(jq_num '(.active_execs_live_record_count // 0)' "$file")"
  active_execs_stale_records="$(jq_num '(.active_execs_stale_record_count // 0)' "$file")"
  active_execs_node_repl_pids="$(jq -cr '(.active_execs_running_node_repl_pids // [])' "$file")"
  active_execs_port_19998_listeners="$(jq_num '(.active_execs_port_19998_listener_count // 0)' "$file")"
  node_repl_smoke_ready="$(jq -r '.node_repl_chrome_smoke_ready // false' "$file")"
  node_repl_smoke_result="$(jq -r '.node_repl_chrome_smoke_result // "unknown"' "$file")"
  node_repl_smoke_path="$(jq -r '.node_repl_chrome_smoke_path // ""' "$file")"
  node_repl_direct_js="$(jq -r '.node_repl_direct_js_result // "unknown"' "$file")"
  node_repl_full_env_js="$(jq -r '.node_repl_full_env_js_result // "unknown"' "$file")"
  node_repl_chrome_no_env_result="$(jq -r '.node_repl_chrome_no_env_result // "unknown"' "$file")"
  node_repl_chrome_no_env_failure="$(jq -r '.node_repl_chrome_no_env_failure_class // "unknown"' "$file")"
  node_repl_chrome_full_env_result="$(jq -r '.node_repl_chrome_full_env_result // "unknown"' "$file")"
  node_repl_chrome_full_env_failure="$(jq -r '.node_repl_chrome_full_env_failure_class // "unknown"' "$file")"
  node_repl_env_matrix_ready="$(jq -r '.node_repl_env_matrix_smoke_ready // false' "$file")"
  node_repl_env_matrix_result="$(jq -r '.node_repl_env_matrix_smoke_result // "unknown"' "$file")"
  node_repl_env_matrix_path="$(jq -r '.node_repl_env_matrix_smoke_path // ""' "$file")"
  node_repl_env_matrix_cases="$(jq_num '(.node_repl_env_matrix_case_count // 0)' "$file")"
  node_repl_env_matrix_js_pass="$(jq_num '(.node_repl_env_matrix_js_pass_cases // 0)' "$file")"
  node_repl_env_matrix_chrome_ready="$(jq_num '(.node_repl_env_matrix_chrome_extension_ready_cases // 0)' "$file")"
  node_repl_env_matrix_native_pipe="$(jq_num '(.node_repl_env_matrix_chrome_native_pipe_or_trust_cases // 0)' "$file")"
  node_repl_env_matrix_sandbox="$(jq_num '(.node_repl_env_matrix_chrome_sandbox_firewall_cases // 0)' "$file")"
  node_repl_env_matrix_blocker="$(jq -r '.node_repl_env_matrix_first_blocker // "none"' "$file")"
  scheduled_smoke_ready="$(jq -r '.scheduled_chrome_smoke_ready // false' "$file")"
  scheduled_smoke_result="$(jq -r '.scheduled_chrome_smoke_result // "unknown"' "$file")"
  scheduled_smoke_path="$(jq -r '.scheduled_chrome_smoke_path // ""' "$file")"
  scheduled_create="$(jq -r 'if has("scheduled_chrome_task_create_ok") then (.scheduled_chrome_task_create_ok | tostring) else "unknown" end' "$file")"
  scheduled_run="$(jq -r 'if has("scheduled_chrome_task_run_ok") then (.scheduled_chrome_task_run_ok | tostring) else "unknown" end' "$file")"
  scheduled_runner="$(jq -r '.scheduled_chrome_runner_result // "unknown"' "$file")"
  scheduled_blocker="$(jq -r '.scheduled_chrome_first_blocker // "none"' "$file")"
  ssh_privilege_ready="$(jq -r '.ssh_privilege_preflight_ready // false' "$file")"
  ssh_privilege_result="$(jq -r '.ssh_privilege_preflight_result // "unknown"' "$file")"
  ssh_privilege_path="$(jq -r '.ssh_privilege_preflight_path // ""' "$file")"
  ssh_privilege_high_integrity="$(jq -r '.ssh_privilege_high_integrity // false' "$file")"
  ssh_privilege_enabled_privileges="$(jq_num '(.ssh_privilege_enabled_privilege_count // 0)' "$file")"
  ssh_privilege_net_session="$(jq -r '.ssh_privilege_net_session_admin_check // "unknown"' "$file")"
  ssh_privilege_firewall="$(jq -r '.ssh_privilege_firewall_service_running // false' "$file")"
  ssh_privilege_process_counts="$(jq -cr '(.ssh_privilege_process_counts // {})' "$file")"
  ssh_privilege_powershell_blocked="$(jq -r '.ssh_privilege_powershell_smoke_blocked // false' "$file")"
  payload_selftest_ready="$(jq -r '.payload_selftest_ready // false' "$file")"
  payload_selftest_result="$(jq -r '.payload_selftest_result // "unknown"' "$file")"
  payload_selftest_path="$(jq -r '.payload_selftest_path // ""' "$file")"
  payload_selftest_passed="$(jq_num '(.payload_selftest_passed // 0)' "$file")"
  payload_selftest_total="$(jq_num '(.payload_selftest_total // 0)' "$file")"
  payload_selftest_visual="$(jq_num '(.payload_selftest_counts.visual_targets // 0)' "$file")"
  payload_selftest_interaction="$(jq_num '(.payload_selftest_counts.interaction_targets // 0)' "$file")"
  payload_selftest_receiver="$(jq_num '(.payload_selftest_counts.receiver_uploads // 0)' "$file")"
  bridge_tool_call_ready="$(jq -r '.bridge_tool_call_ready // false' "$file")"
  bridge_tool_call_result="$(jq -r '.bridge_tool_call_result // "unknown"' "$file")"
  bridge_tool_call_path="$(jq -r '.bridge_tool_call_path // ""' "$file")"
  bridge_tool_call_tool="$(jq -r '.bridge_tool_call_tool_name // "unknown"' "$file")"
  bridge_tool_call_passed="$(jq_num '(.bridge_tool_call_passed // 0)' "$file")"
  bridge_tool_call_total="$(jq_num '(.bridge_tool_call_total // 0)' "$file")"
  bridge_tool_call_timeout="$(jq_num '(.bridge_tool_call_timeout_ms // 0)' "$file")"
  bridge_tool_call_sha="$(jq -r '.bridge_tool_call_payload_sha256 // ""' "$file")"
  bridge_tool_call_visual="$(jq_num '(.bridge_tool_call_payload.visual_target_count // 0)' "$file")"
  bridge_tool_call_interaction="$(jq_num '(.bridge_tool_call_payload.interaction_target_count // 0)' "$file")"
  bridge_tool_call_receiver="$(jq_num '(.bridge_tool_call_payload.receiver_upload_count // 0)' "$file")"
  bridge_run_result_path="$(jq -r '.bridge_run_result.path // .bridge_run_result_path // ""' "$file")"
  bridge_run_result_exists="$(jq -r '.bridge_run_result.exists // .bridge_run_result_exists // false' "$file")"
  finalization_file="$ACCEPTANCE_DIR/02-regression/ui-visual-interaction/evidence-finalization-latest.json"
  if [[ -s "$finalization_file" ]]; then
    finalization_run_id="$(jq -r '.run_id // "unknown"' "$finalization_file")"
    finalization_result="$(jq -r '.result // "unknown"' "$finalization_file")"
    finalization_visual_passed="$(jq_num '(.visual_passed_count // 0)' "$finalization_file")"
    finalization_visual_total="$(jq_num '(.visual_target_count // 0)' "$finalization_file")"
    finalization_interaction_passed="$(jq_num '(.interaction_passed_count // 0)' "$finalization_file")"
    finalization_interaction_total="$(jq_num '(.interaction_route_count // 0)' "$finalization_file")"
  else
    finalization_run_id="missing"
    finalization_result="missing"
    finalization_visual_passed=0
    finalization_visual_total=0
    finalization_interaction_passed=0
    finalization_interaction_total=0
  fi
  failing="$(jq -r '[.checks[]? | select(.passed == false) | ((.phase // "unknown") + "/" + (.name // "unnamed") + ": " + (.status // "unknown") + " " + (.detail // ""))] | join("; ")' "$file")"
  detail="run_id=$run_id result=$result blockers=$blockers warnings=$warnings desktop_chrome_status=$desktop_status desktop_chrome_artifact=${desktop_artifact:-missing} components=$components_present/$routes_total source_images=$sources_present/$sources_total design_image_reference_blockers=$design_refs visual_diff=$visual_passed/$visual_required business_interaction=$interaction_passed/$interaction_required capture_session_path=${capture_session_path:-missing} capture_session_status=$capture_session_status capture_session_covers_current_gaps=$capture_session_covers capture_session_run_id=${capture_session_run_id:-missing} capture_session_batch=$capture_session_visual_batch/$capture_session_interaction_batch bridge_runtime_preflight_ready=$bridge_runtime_ready bridge_runtime_preflight_result=$bridge_runtime_result bridge_runtime_preflight_path=${bridge_runtime_path:-missing} bridge_runtime_chrome_clients=$bridge_runtime_chrome_clients bridge_runtime_explicit_codex_exists=$bridge_runtime_explicit_codex bridge_runtime_explicit_codex_version_count=$bridge_runtime_explicit_codex_versions bridge_runtime_mcp_list_line_count=$bridge_runtime_mcp_list_lines bridge_runtime_node_repl_mcp=$bridge_runtime_node_repl_mcp_result:$bridge_runtime_node_repl_mcp_transport bridge_runtime_node_repl_mcp_env_keys=$bridge_runtime_node_repl_mcp_env_keys bridge_runtime_process_counts=$bridge_runtime_process_counts tool_surface_preflight_ready=$tool_surface_ready tool_surface_preflight_result=$tool_surface_result tool_surface_preflight_path=${tool_surface_path:-missing} tool_surface_plugin_installed_enabled=$tool_surface_plugin tool_surface_backend_open=$tool_surface_backend_open tool_surface_tools_list_status=$tool_surface_tools_list tool_surface_session_tool_status=$tool_surface_session_tools active_execs_preflight_ready=$active_execs_ready active_execs_preflight_result=$active_execs_result active_execs_preflight_path=${active_execs_path:-missing} active_execs_channel_status=$active_execs_channel active_execs_records=$active_execs_records active_execs_live_records=$active_execs_live_records active_execs_stale_records=$active_execs_stale_records active_execs_running_node_repl_pids=$active_execs_node_repl_pids active_execs_port_19998_listeners=$active_execs_port_19998_listeners node_repl_chrome_smoke_ready=$node_repl_smoke_ready node_repl_chrome_smoke_result=$node_repl_smoke_result node_repl_chrome_smoke_path=${node_repl_smoke_path:-missing} node_repl_direct_js=$node_repl_direct_js node_repl_full_env_js=$node_repl_full_env_js node_repl_chrome_no_env=$node_repl_chrome_no_env_result:$node_repl_chrome_no_env_failure node_repl_chrome_full_env=$node_repl_chrome_full_env_result:$node_repl_chrome_full_env_failure node_repl_env_matrix_ready=$node_repl_env_matrix_ready node_repl_env_matrix_result=$node_repl_env_matrix_result node_repl_env_matrix_path=${node_repl_env_matrix_path:-missing} node_repl_env_matrix_js_pass=$node_repl_env_matrix_js_pass/$node_repl_env_matrix_cases node_repl_env_matrix_chrome_ready=$node_repl_env_matrix_chrome_ready node_repl_env_matrix_native_pipe_or_trust=$node_repl_env_matrix_native_pipe node_repl_env_matrix_sandbox_firewall=$node_repl_env_matrix_sandbox node_repl_env_matrix_first_blocker=$node_repl_env_matrix_blocker scheduled_chrome_smoke_ready=$scheduled_smoke_ready scheduled_chrome_smoke_result=$scheduled_smoke_result scheduled_chrome_smoke_path=${scheduled_smoke_path:-missing} scheduled_chrome_task_create_ok=$scheduled_create scheduled_chrome_task_run_ok=$scheduled_run scheduled_chrome_runner_result=$scheduled_runner scheduled_chrome_first_blocker=$scheduled_blocker ssh_privilege_preflight_ready=$ssh_privilege_ready ssh_privilege_preflight_result=$ssh_privilege_result ssh_privilege_preflight_path=${ssh_privilege_path:-missing} ssh_privilege_high_integrity=$ssh_privilege_high_integrity ssh_privilege_enabled_privileges=$ssh_privilege_enabled_privileges ssh_privilege_net_session=$ssh_privilege_net_session ssh_privilege_firewall_service_running=$ssh_privilege_firewall ssh_privilege_process_counts=$ssh_privilege_process_counts ssh_privilege_powershell_blocked=$ssh_privilege_powershell_blocked payload_selftest_ready=$payload_selftest_ready payload_selftest_result=$payload_selftest_result payload_selftest_path=${payload_selftest_path:-missing} payload_selftest_checks=$payload_selftest_passed/$payload_selftest_total payload_selftest_batch=$payload_selftest_visual/$payload_selftest_interaction payload_selftest_receiver_uploads=$payload_selftest_receiver bridge_tool_call_ready=$bridge_tool_call_ready bridge_tool_call_result=$bridge_tool_call_result bridge_tool_call_path=${bridge_tool_call_path:-missing} bridge_tool_call_tool=$bridge_tool_call_tool bridge_tool_call_checks=$bridge_tool_call_passed/$bridge_tool_call_total bridge_tool_call_timeout_ms=$bridge_tool_call_timeout bridge_tool_call_payload_sha256=${bridge_tool_call_sha:-missing} bridge_tool_call_batch=$bridge_tool_call_visual/$bridge_tool_call_interaction bridge_tool_call_receiver_uploads=$bridge_tool_call_receiver bridge_run_result_exists=$bridge_run_result_exists bridge_run_result_path=${bridge_run_result_path:-missing} finalization_run_id=$finalization_run_id finalization_result=$finalization_result finalization_visual=$finalization_visual_passed/$finalization_visual_total finalization_interaction=$finalization_interaction_passed/$finalization_interaction_total"

  if [[ "$result" == "pass" && "$blockers" -eq 0 && "$visual_required" -gt 0 && "$visual_passed" -eq "$visual_required" && "$interaction_required" -gt 0 && "$interaction_passed" -eq "$interaction_required" && "$design_refs" -eq 0 && "$finalization_result" == "pass" ]]; then
    json_log "ui_visual_interaction" "evidence" "UI visual diff and business interaction dual gate passes" "info" true "pass" "$detail" "$file"
  else
    if [[ -n "$failing" ]]; then
      detail="$detail; failing=$failing"
    fi
    if [[ "$finalization_result" != "pass" ]]; then
      detail="$detail; finalization_blocked=$finalization_result"
    fi
    json_log "ui_visual_interaction" "evidence" "UI visual diff and business interaction dual gate passes" "blocker" false "blocked" "$detail" "$file"
  fi
}

signoff_gate() {
  local file="$ACCEPTANCE_DIR/08-third-party/user-acceptance-signoff.md"
  local readiness="$ACCEPTANCE_DIR/08-third-party/readiness/third-party-signoff-readiness-latest.json"
  local readiness_ledger="$ACCEPTANCE_DIR/08-third-party/readiness/latest/evidence-ledger.bootstrap.json"
  local release="$ACCEPTANCE_DIR/00-baseline/release-manifest-latest.json"
  local readiness_detail=""
  local placeholder_pattern='TBD|待填写|待专项验证|待第三方|待维护窗口|待用户|待签认|待外部|待现场|未提供|未签字|待签字|待确认|待恢复|待 policy-capable CNI'
  local readiness_tbd_count="unknown"
  local readiness_upstream_count="unknown"
  local current_release_run_id="unknown"
  local readiness_release_run_id="unknown"
  if [[ -s "$release" ]]; then
    current_release_run_id="$(jq -r '.run_id // "unknown"' "$release")"
  fi
  if [[ -s "$readiness" ]]; then
    readiness_tbd_count="$(jq -r '.template_tbd_count // "unknown"' "$readiness")"
    readiness_upstream_count="$(jq -r '.upstream_blocked_or_nonpass_inputs // "unknown"' "$readiness")"
    readiness_release_run_id="$(jq -r '(.evidence_input_run_ids.baseline // ((.evidence_inputs // []) | map(select(.key == "baseline")) | first | .run_id) // "unknown")' "$readiness")"
    if [[ "$readiness_release_run_id" == "unknown" && -s "$readiness_ledger" ]]; then
      readiness_release_run_id="$(jq -r '(.evidence_inputs // []) | map(select(.key == "baseline")) | first | .run_id // "unknown"' "$readiness_ledger")"
    fi
    readiness_detail="$(jq -r --arg current_release "$current_release_run_id" --arg readiness_release "$readiness_release_run_id" '" readiness_run_id=\(.run_id // "unknown") readiness_result=\(.result // "unknown") template_tbd_count=\(.template_tbd_count // "unknown") upstream_blocked_or_nonpass_inputs=\(.upstream_blocked_or_nonpass_inputs // "unknown") readiness_release_run_id=\($readiness_release) current_release_run_id=\($current_release)"' "$readiness")"
  else
    readiness_detail=" readiness_run_id=missing current_release_run_id=$current_release_run_id"
  fi
  if [[ ! -s "$file" ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "missing" "$file$readiness_detail" "$file"
    return
  fi
  if [[ ! -s "$readiness" ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "readiness_missing" "third-party readiness summary missing;$readiness_detail" "$readiness"
  elif [[ "$current_release_run_id" == "unknown" || "$readiness_release_run_id" == "unknown" ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "readiness_release_unverified" "third-party readiness release binding cannot be verified;$readiness_detail" "$readiness"
  elif [[ "$readiness_release_run_id" != "$current_release_run_id" ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "stale_release" "third-party readiness was generated against a stale release manifest;$readiness_detail" "$readiness"
  elif grep -Eq "$placeholder_pattern" "$file"; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "template_only" "signoff template still contains placeholders or pending signature markers;$readiness_detail" "$file"
  elif [[ "$readiness_tbd_count" != "unknown" && "$readiness_tbd_count" -gt 0 ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "readiness_incomplete" "third-party package still has $readiness_tbd_count placeholders;$readiness_detail" "$readiness"
  elif [[ "$readiness_upstream_count" != "unknown" && "$readiness_upstream_count" -gt 0 ]]; then
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "blocker" false "upstream_blocked" "third-party package still has $readiness_upstream_count upstream non-pass or blocked evidence inputs;$readiness_detail" "$readiness"
  else
    json_log "trial_third_party_signoff" "evidence" "user acceptance signoff is filled and signed" "info" true "pass" "no placeholder or pending signature markers found;$readiness_detail" "$file"
  fi
}

network_policy_gate() {
  local file="$ACCEPTANCE_DIR/05-security/network-policy-enforcement-preflight-latest.json"
  local readiness="$ACCEPTANCE_DIR/05-security/network-policy-readiness/network-policy-enforcement-readiness-latest.json"
  local name="NetworkPolicy default deny and allow-list enforcement passes on policy-capable CNI"
  local readiness_detail=""

  if [[ -s "$readiness" ]]; then
    readiness_detail="$(jq -r '" readiness_run_id=\(.run_id // "unknown") readiness_result=\(.result // "unknown") readiness_policy_capable_count=\(.policy_capable_count // "unknown") readiness_flannel_marker_count=\(.flannel_marker_count // "unknown")"' "$readiness")"
  else
    readiness_detail=" readiness_run_id=missing"
  fi

  if [[ ! -s "$file" ]]; then
    json_log "network_policy_enforcement" "evidence" "$name" "blocker" false "missing" "$file$readiness_detail" "$file"
    return
  fi

  local result run_id passed blockers warnings total failing detail status
  result="$(jq -r '.result // "unknown"' "$file")"
  run_id="$(jq -r '.run_id // "unknown"' "$file")"
  passed="$(jq_num '(.passed // ([.checks[]? | select(.passed == true)] | length))' "$file")"
  blockers="$(jq_num '(.blockers // ([.checks[]? | select((.passed == false) and (.severity == "blocker"))] | length))' "$file")"
  warnings="$(jq_num '(.warnings // ([.checks[]? | select((.passed == false) and ((.severity == "warn") or (.severity == "warning")))] | length))' "$file")"
  total="$(jq_num '(.total // ([.checks[]?] | length))' "$file")"
  failing="$(jq -r '[.checks[]? | select(.passed == false) | ((.phase // "unknown") + "/" + (.name // "unnamed") + ": " + (.status // "unknown") + " " + (.detail // ""))] | join("; ")' "$file")"
  detail="run_id=$run_id result=$result passed=$passed total=$total blockers=$blockers warnings=$warnings;$readiness_detail"

  if [[ "$result" == "pass" && "$blockers" -eq 0 && "$passed" -ge 1 ]]; then
    status="pass"
    if [[ "$warnings" -gt 0 ]]; then
      status="pass_with_warnings"
    fi
    json_log "network_policy_enforcement" "evidence" "$name" "info" true "$status" "$detail" "$file"
  else
    if [[ -n "$failing" ]]; then
      detail="$detail; failing=$failing"
    fi
    json_log "network_policy_enforcement" "evidence" "$name" "blocker" false "$result" "$detail" "$file"
  fi
}

finalize() {
  local passed failed blockers warnings result
  passed="$(jq -s '[.[] | select(.passed == true)] | length' "$REPORT")"
  failed="$(jq -s '[.[] | select(.passed == false)] | length' "$REPORT")"
  blockers="$(jq -s '[.[] | select(.passed == false and .severity == "blocker")] | length' "$REPORT")"
  warnings="$(jq -s '[.[] | select(.passed == false and ((.severity == "warning") or (.severity == "warn")))] | length' "$REPORT")"
  result="pass"
  if [[ "$blockers" -gt 0 ]]; then
    result="blocked"
  fi

  jq -n \
    --arg run_id "$RUN_ID" \
    --arg result "$result" \
    --arg report "$REPORT" \
    --arg generated_at "$(date -Iseconds)" \
    --argjson passed "$passed" \
    --argjson failed "$failed" \
    --argjson blockers "$blockers" \
    --argjson warnings "$warnings" \
    --slurpfile gates "$REPORT" \
    '{run_id:$run_id, result:$result, generated_at:$generated_at, passed:$passed, failed:$failed, blockers:$blockers, warnings:$warnings, report:$report, gates:$gates, blocker_details:[$gates[] | select(.passed == false and .severity == "blocker") | {gate,name,status,detail,artifact}]}' >"$SUMMARY"

  {
    cat <<EOF
# Project completion audit

- Run ID: \`$RUN_ID\`
- Result: \`$result\`
- Passed gates: $passed
- Failed gates: $failed
- Blockers: $blockers
- Warnings: $warnings
- NDJSON: \`$REPORT\`
- Summary: \`$SUMMARY\`

This audit is non-destructive. It reads the latest documented evidence under \`doc/02_acceptance\` and decides whether the full project objective can be treated as complete.

## Gate Matrix

EOF
    jq -s -r '
      "| Gate | Result | Status | Evidence |",
      "|---|---|---|---|",
      (.[] | "| \(.gate) | \(if .passed then "pass" else "blocked" end) | \(.status) | `\(.artifact)` |")
    ' "$REPORT"
    cat <<EOF

## Blockers

EOF
    jq -s -r '.[] | select(.passed == false and .severity == "blocker") | "- \(.gate): \(.detail) (`\(.artifact)`)"' "$REPORT"
  } >"$LOCAL_REPORT"

  cp "$SUMMARY" "$COMPLETION_DIR/project-completion-audit-latest.json"
  cp "$LOCAL_REPORT" "$COMPLETION_DIR/project-completion-audit-latest.md"

  if [[ "$blockers" -gt 0 && "$ALLOW_BLOCKERS" != "true" ]]; then
    exit 1
  fi
}

need_cmd jq

release_manifest_gate
latest_json_gate "deployment_preflight" "K8s deployment preflight and APISIX business entry pass" "$ACCEPTANCE_DIR/07-deployment/deployment-preflight-latest.json" 1
latest_json_gate "business_flow_api" "full business-flow API matrix passes live APISIX" "$ACCEPTANCE_DIR/02-regression/business-flow-api-preflight-latest.json" 1
oidc_sso_gate
ui_contract_gate
ui_visual_interaction_gate
latest_json_all_pass "governance_and_state_flows" "governance APIs and state machines pass" \
  "$ACCEPTANCE_DIR/02-regression/compliance-audit-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/notification-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/topic-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/whitelist-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/baseline-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/settings-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/probe-ops-governance-preflight-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/playbook-state-machine-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/forensics-task-state-machine-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/rule-state-machine-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/deployment-state-machine-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/model-version-state-machine-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/token-lifecycle-matrix-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/threat-intel-service-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/fusion-threat-intel-latest.json" \
  "$ACCEPTANCE_DIR/02-regression/asset-discovery-latest.json"
latest_json_all_pass "kafka_security" "Kafka SASL_SSL rollout and security preflight pass" \
  "$ACCEPTANCE_DIR/05-security/kafka-sasl-ssl-rollout-latest.json" \
  "$ACCEPTANCE_DIR/05-security/kafka-security-rollout-preflight-latest.json"
latest_json_gate "production_security" "production security preflight has no blockers" "$ACCEPTANCE_DIR/05-security/production-security-preflight-latest.json" 1
network_policy_gate
latest_json_gate "ha_rto_rpo" "HA readiness includes destructive RTO/RPO drill evidence" "$ACCEPTANCE_DIR/06-resilience/ha-readiness-preflight-latest.json" 1
latest_json_gate "capture_performance" "10 x 100Gbps and 512Mpps capture performance package passes" "$ACCEPTANCE_DIR/03-performance/capture-performance-preflight-latest.json" 0
latest_json_gate "detection_quality" "95 percent detection and 5 percent false-positive third-party package passes" "$ACCEPTANCE_DIR/04-detection-quality/detection-quality-preflight-latest.json" 0
latest_json_gate "fusion_value_report" "fusion value-report live rollout and API contract pass" "$ACCEPTANCE_DIR/02-regression/fusion-value-report-preflight-latest.json" 1
latest_json_gate "asset_discovery_coverage" "site asset discovery coverage is measured against expected inventory" "$ACCEPTANCE_DIR/02-regression/asset-discovery-coverage-latest.json" 1
signoff_gate

finalize
