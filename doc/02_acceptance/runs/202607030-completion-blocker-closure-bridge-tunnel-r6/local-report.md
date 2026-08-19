# Completion Blocker Closure Readiness

- Run ID: `202607030-completion-blocker-closure-bridge-tunnel-r6`
- Result: `pass`
- Source audit: `doc/02_acceptance/09-completion/project-completion-audit-latest.json`
- Source audit run: `202607030-project-audit-bridge-tunnel-r6` (blocked)
- Current completion blockers: 9
- Ready input packages or current evidence links: 46
- External or maintenance-window actions: 9
- Formal rerun commands: 28
- Stable package: `doc/02_acceptance/09-completion/blocker-closure/latest`

This package turns the latest project completion audit blockers into an execution board. It is review-required and does not mark the project complete.

## Blocker Ledger

- `desktop_browser_smoke`: windows_tunnel_payload_ready_but_bridge_tool_missing / next: expose the current-session Codex Desktop Chrome bridge tool for the Windows VSCode machine at 10.3.6.59, run the generated payload through Chrome extension backend, upload the bridge run summary, then rerun UI visual and project completion gates
- `ui_visual_interaction`: full_gap_capture_package_ready_but_desktop_chrome_execution_missing / next: execute the full 30 visual / 28 interaction Windows tunnel payload in Desktop Chrome extension backend, upload 58 screenshots plus one bridge run summary, finalize evidence, then rerun the UI visual interaction gate
- `production_security`: external_cni_and_waiver_required / next: install or migrate to a policy-capable CNI, review runtime waivers for privileged/hostNetwork workloads, then rerun production security preflight
- `network_policy_enforcement`: external_cni_required / next: use network-policy readiness package to migrate CNI, then run isolated default-deny and allow-list probe
- `ha_rto_rpo`: maintenance_window_required / next: execute destructive Kafka/Flink/ClickHouse/PostgreSQL/MinIO drills using HA bootstrap templates and publish formal RTO/RPO reports
- `capture_performance`: hardware_window_required / next: fill hardware and traffic profiles, run 10 x 100Gbps and 512Mpps tests, then rerun capture performance preflight
- `detection_quality`: third_party_adjudication_required / next: freeze dataset, fill labels and predictions, lock thresholds, obtain third-party attestation, then rerun detection quality preflight
- `asset_discovery_coverage`: site_inventory_required / next: review observed asset inventory bootstrap with site owner, produce authoritative SITE_ASSET_INVENTORY_JSON, then rerun coverage gate
- `trial_third_party_signoff`: signature_and_external_report_required / next: fill signoff placeholders, resolve upstream exceptions, attach pilot/third-party/economic-benefit confirmations, then rerun project completion audit

## Formal Rerun Commands

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
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

```bash
node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://127.0.0.1:5173 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
```

```bash
mcp__codex_desktop_node_repl__js < doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js
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
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

```bash
node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
```

```bash
mcp__codex_desktop_node_repl__js < doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js
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

