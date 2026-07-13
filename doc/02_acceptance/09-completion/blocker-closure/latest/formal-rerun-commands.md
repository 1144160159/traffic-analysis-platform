# Formal Rerun Commands

## desktop_browser_smoke

```bash
ssh -N -R 127.0.0.1:25173:127.0.0.1:5173 LongShine@10.3.6.59
```

```bash
ssh -N -R 127.0.0.1:25174:127.0.0.1:15174 -R 127.0.0.1:25175:127.0.0.1:15175 LongShine@10.3.6.59
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs
```

```bash
node tests/e2e/ui_codex_bridge_tool_surface_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_active_execs_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_chrome_bridge_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_env_matrix_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_scheduled_chrome_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_ssh_privilege_preflight.mjs
```

```bash
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

```bash
node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://127.0.0.1:5173 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_execution_request.mjs
```

```bash
mcp__codex_desktop_node_repl__js with JSON arguments from doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json after replacing placeholders
```

```bash
RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json
```

```bash
DESKTOP_CHROME_STATUS=pass CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
```

## ui_visual_interaction

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs
```

```bash
node tests/e2e/ui_codex_bridge_tool_surface_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_active_execs_preflight.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_chrome_bridge_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_env_matrix_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_node_repl_scheduled_chrome_smoke.mjs
```

```bash
SSHPASS=<redacted> node tests/e2e/ui_windows_ssh_privilege_preflight.mjs
```

```bash
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_execution_request.mjs
```

```bash
mcp__codex_desktop_node_repl__js with JSON arguments from doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json after replacing placeholders
```

```bash
RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json
```

```bash
DESKTOP_CHROME_STATUS=pass CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
```

```bash
ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh
```

## production_security

```bash
ALLOW_BLOCKERS=false tests/e2e/live_network_policy_enforcement_preflight.sh
```

```bash
ALLOW_BLOCKERS=false tests/e2e/live_production_security_preflight.sh
```

## network_policy_enforcement

```bash
ALLOW_BLOCKERS=false RUN_ENFORCEMENT_PROBE=auto tests/e2e/live_network_policy_enforcement_preflight.sh
```

## ha_rto_rpo

```bash
ALLOW_BLOCKERS=false tests/chaos/live_ha_readiness_preflight.sh
```

## capture_performance

```bash
ALLOW_BLOCKERS=false tests/perf/100g_capture/live_capture_performance_preflight.sh
```

## detection_quality

```bash
ALLOW_BLOCKERS=false tests/e2e/live_detection_quality_preflight.sh
```

## asset_discovery_coverage

```bash
SITE_ASSET_INVENTORY_JSON=/path/to/site-assets.json MIN_DISCOVERY_COVERAGE_PCT=95 ALLOW_BLOCKERS=false tests/e2e/live_asset_discovery_coverage_report.sh
```

## trial_third_party_signoff

```bash
ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh
```

