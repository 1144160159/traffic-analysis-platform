# Completion Blocker Closure Readiness

- Run ID: `20260701-completion-blocker-closure-readiness-r21-detection-quality-review-r1`
- Result: `pass`
- Source audit: `doc/02_acceptance/09-completion/project-completion-audit-latest.json`
- Current completion blockers: 8
- Ready input packages or current evidence links: 17
- External or maintenance-window actions: 8
- Formal rerun commands: 9
- Stable package: `doc/02_acceptance/09-completion/blocker-closure/latest`

This package turns the latest project completion audit blockers into an execution board. It is review-required and does not mark the project complete.

## Blocker Ledger

- `desktop_browser_smoke`: runtime_blocked / next: restore Codex Desktop Chrome bridge transport and rerun UI contract preflight with Desktop Chrome login and protected business-page smoke
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
DESKTOP_CHROME_STATUS=pass DESKTOP_CHROME_URL=http://10.0.5.8:30180/login DESKTOP_CHROME_TITLE='园区网络全流量采集与分析系统' DESKTOP_CHROME_BUSINESS_STATUS=pass tests/e2e/live_ui_contract_preflight.sh
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

