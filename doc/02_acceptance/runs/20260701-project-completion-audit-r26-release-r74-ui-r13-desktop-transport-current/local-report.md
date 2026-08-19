# Project completion audit

- Run ID: `20260701-project-completion-audit-r26-release-r74-ui-r13-desktop-transport-current`
- Result: `blocked`
- Passed gates: 7
- Failed gates: 8
- Blockers: 8
- Warnings: 0
- NDJSON: `doc/02_acceptance/runs/20260701-project-completion-audit-r26-release-r74-ui-r13-desktop-transport-current/live-project-completion-audit-20260701-project-completion-audit-r26-release-r74-ui-r13-desktop-transport-current.ndjson`
- Summary: `doc/02_acceptance/runs/20260701-project-completion-audit-r26-release-r74-ui-r13-desktop-transport-current/live-project-completion-audit-20260701-project-completion-audit-r26-release-r74-ui-r13-desktop-transport-current-summary.json`

This audit is non-destructive. It reads the latest documented evidence under `doc/02_acceptance` and decides whether the full project objective can be treated as complete.

## Gate Matrix

| Gate | Result | Status | Evidence |
|---|---|---|---|
| baseline_release_manifest | pass | pass | `doc/02_acceptance/00-baseline/release-manifest-latest.json` |
| deployment_preflight | pass | pass | `doc/02_acceptance/07-deployment/deployment-preflight-latest.json` |
| business_flow_api | pass | pass | `doc/02_acceptance/02-regression/business-flow-api-preflight-latest.json` |
| ui_contract_static | pass | pass | `doc/02_acceptance/02-regression/ui-contract-preflight-latest.json` |
| desktop_browser_smoke | blocked | blocked | `doc/02_acceptance/02-regression/ui-contract-preflight-latest.json` |
| governance_and_state_flows | pass | pass | `doc/02_acceptance/02-regression/compliance-audit-preflight-latest.json, doc/02_acceptance/02-regression/notification-governance-preflight-latest.json, doc/02_acceptance/02-regression/topic-governance-preflight-latest.json, doc/02_acceptance/02-regression/whitelist-governance-preflight-latest.json, doc/02_acceptance/02-regression/baseline-governance-preflight-latest.json, doc/02_acceptance/02-regression/settings-governance-preflight-latest.json, doc/02_acceptance/02-regression/probe-ops-governance-preflight-latest.json, doc/02_acceptance/02-regression/playbook-state-machine-latest.json, doc/02_acceptance/02-regression/forensics-task-state-machine-latest.json, doc/02_acceptance/02-regression/rule-state-machine-latest.json, doc/02_acceptance/02-regression/deployment-state-machine-latest.json, doc/02_acceptance/02-regression/model-version-state-machine-latest.json, doc/02_acceptance/02-regression/token-lifecycle-matrix-latest.json, doc/02_acceptance/02-regression/threat-intel-service-latest.json, doc/02_acceptance/02-regression/fusion-threat-intel-latest.json, doc/02_acceptance/02-regression/asset-discovery-latest.json` |
| kafka_security | pass | pass | `doc/02_acceptance/05-security/kafka-sasl-ssl-rollout-latest.json, doc/02_acceptance/05-security/kafka-security-rollout-preflight-latest.json` |
| production_security | blocked | blocked | `doc/02_acceptance/05-security/production-security-preflight-latest.json` |
| network_policy_enforcement | blocked | blocked | `doc/02_acceptance/05-security/network-policy-enforcement-preflight-latest.json` |
| ha_rto_rpo | blocked | blocked | `doc/02_acceptance/06-resilience/ha-readiness-preflight-latest.json` |
| capture_performance | blocked | blocked | `doc/02_acceptance/03-performance/capture-performance-preflight-latest.json` |
| detection_quality | blocked | blocked | `doc/02_acceptance/04-detection-quality/detection-quality-preflight-latest.json` |
| fusion_value_report | pass | pass | `doc/02_acceptance/02-regression/fusion-value-report-preflight-latest.json` |
| asset_discovery_coverage | blocked | blocked | `doc/02_acceptance/02-regression/asset-discovery-coverage-latest.json` |
| trial_third_party_signoff | blocked | template_only | `doc/02_acceptance/08-third-party/user-acceptance-signoff.md` |

## Blockers

- desktop_browser_smoke: run_id=20260701-ui-contract-preflight-r13-desktop-transport-current browser_checks=2 browser_failed=2 Desktop Chrome wrapper opened login page: transport_closed 2026-07-01 recheck: codex-desktop-node-repl wrappers were exposed, but desktop_chrome_list_tabs returned Transport closed before Chrome extension backend could list tabs or open the login page; js_reset also returned Transport closed.; Desktop Chrome wrapper opened protected business page: transport_closed 2026-07-01 recheck: Desktop Chrome Bridge MCP transport closed before any protected business page navigation; no Desktop browser page was opened. (`doc/02_acceptance/02-regression/ui-contract-preflight-latest.json`)
- production_security: run_id=20260630-production-security-preflight-r49-waiver-registry result=blocked passed=20 total=21 blockers=1 warnings=0; failing=live/NetworkPolicy enforcement-capable CNI present: missing policy-capable CNI pods=0 flannel_markers=2 (`doc/02_acceptance/05-security/production-security-preflight-latest.json`)
- network_policy_enforcement: run_id=20260630-network-policy-enforcement-preflight-r1-flannel-blocked result=blocked passed=2 total=4 blockers=2 warnings=0; readiness_run_id=20260630-network-policy-enforcement-readiness-r1 readiness_result=pass readiness_policy_capable_count=0 readiness_flannel_marker_count=2; failing=live/NetworkPolicy enforcement-capable CNI present: missing policy-capable CNI pods=0 flannel_markers=2; probe/NetworkPolicy default deny and allow-list enforcement: skipped_cni_missing policy-capable CNI pods=0; negative probe would be a false pass on Flannel (`doc/02_acceptance/05-security/network-policy-enforcement-preflight-latest.json`)
- ha_rto_rpo: run_id=20260630-ha-readiness-preflight-r9-integrity-active result=blocked passed=13 total=14 blockers=1 warnings=0; failing=acceptance/destructive RTO/RPO drill reports present: missing present=0 required=6 missing=kafka-failover.md flink-failover.md clickhouse-failover.md postgres-failover.md minio-failover.md ha-rto-rpo-latest.json (`doc/02_acceptance/06-resilience/ha-readiness-preflight-latest.json`)
- capture_performance: run_id=20260630-capture-performance-preflight-r3-integrity-active result=blocked passed=11 total=18 blockers=4 warnings=3; failing=package/hardware inventory provided: missing tests/perf/100g_capture/hardware-inventory.yaml; package/traffic profile provided: missing tests/perf/100g_capture/traffic-profile.yaml; repo/existing 500k stress report is only non-acceptance context: insufficient 0.94Mpps/1.3Gbps class report, not GATE-P0-03/04; live/live probe capture mode is AF_XDP: not_acceptance_profile mode=af_packet; live/live probe CPU pinning has multi-queue capacity: small_profile cpu_cores=2; results/10x100g-line-rate result summary present: missing tests/perf/100g_capture/results/10x100g-summary.json; results/512mpps-small-packet result summary present: missing tests/perf/100g_capture/results/512mpps-summary.json (`doc/02_acceptance/03-performance/capture-performance-preflight-latest.json`)
- detection_quality: run_id=20260630-detection-quality-preflight-r4-integrity-active result=blocked passed=5 total=10 blockers=5 warnings=0; failing=package/dataset manifest present: missing missing dataset-manifest.yaml; package/threshold lock present: missing missing threshold-lock.json; package/labels present: missing missing labels.csv; package/predictions present: missing missing predictions.csv; package/third party attestation present: missing missing third-party-attestation.yaml (`doc/02_acceptance/04-detection-quality/detection-quality-preflight-latest.json`)
- asset_discovery_coverage: run_id=20260630-asset-discovery-coverage-r2-review-required-guard result=blocked passed=6 total=7 blockers=1 warnings=0; failing=coverage/Site inventory is approved for formal coverage: review_required 27/27 coverage=100% source=live_assets_api; review-required bootstrap cannot close formal coverage (`doc/02_acceptance/02-regression/asset-discovery-coverage-latest.json`)
- trial_third_party_signoff: signoff template still contains placeholders or pending signature markers; readiness_run_id=20260701-third-party-signoff-readiness-r14-release-r74-ui-r13-desktop-transport-current readiness_result=pass template_tbd_count=159 upstream_blocked_or_nonpass_inputs=5 readiness_release_run_id=20260701-release-manifest-r74-ui-r13-desktop-transport-current current_release_run_id=20260701-release-manifest-r74-ui-r13-desktop-transport-current (`doc/02_acceptance/08-third-party/user-acceptance-signoff.md`)
