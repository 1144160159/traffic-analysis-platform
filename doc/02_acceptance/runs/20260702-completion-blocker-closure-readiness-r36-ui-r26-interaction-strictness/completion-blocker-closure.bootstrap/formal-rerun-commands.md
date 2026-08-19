# Formal Rerun Commands

## desktop_browser_smoke

```bash
tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:<port>
```

```bash
DESKTOP_CHROME_STATUS=pass DESKTOP_CHROME_URL=http://10.0.5.8:30180/login DESKTOP_CHROME_TITLE='园区网络全流量采集与分析系统' DESKTOP_CHROME_BUSINESS_STATUS=pass tests/e2e/live_ui_contract_preflight.sh
```

```bash
DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
```

## ui_visual_interaction

```bash
tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:<port>
```

```bash
DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
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

