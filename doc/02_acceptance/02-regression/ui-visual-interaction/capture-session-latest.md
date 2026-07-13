# UI Desktop Capture Session

- Session ID: `20260703-ui-desktop-capture-session-direct-apisix-r2`
- Status: `desktop_bridge_blocked`
- Generated: `2026-07-03T02:03:07.982Z`
- Capture plan: `doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-latest.json`
- Gap report: `doc/02_acceptance/02-regression/ui-visual-interaction-gap-report-latest.json`
- Windows bridge host preflight: `pass`
- Windows bridge runtime preflight: `pass`
- Receiver self-test: `pass`
- Smoke token preflight: `pass`
- Visual pending: `30`
- Interaction pending: `28`
- Viewport calibration: `blocked`

This package is a Desktop Chrome execution queue. It is not acceptance evidence and cannot close the dual gate by itself.

## Commands

### direct_app_url

```bash
http://10.0.5.8:30180
```

### direct_receiver_url

```bash
http://10.0.5.8:15174
```

### direct_smoke_redirect_url

```bash
http://10.0.5.8:15175
```

### receiver_selftest

```bash
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

### smoke_token_preflight

```bash
node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://10.0.5.8:30180 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
```

### receiver_start

```bash
DESKTOP_SMOKE_TOKEN='<redacted>' CODEX_CAPTURE_KEY='<redacted>' tests/e2e/ui_desktop_capture_receiver.py --host 0.0.0.0 --port 15174 --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest --max-uploads 59 --expected-width 1920 --expected-height 1080
```

### viewport_probe_open

```bash
mcp__codex_desktop_node_repl.desktop_chrome_open_url url=http://10.0.5.8:15174/viewport-probe keep=true wait_ms=1500
```

### smoke_redirect_start

```bash
DESKTOP_SMOKE_TOKEN='<redacted>' CODEX_SMOKE_NONCE='<redacted>' tests/e2e/ui_desktop_smoke_redirect.py --host 0.0.0.0 --port 15175 --app-base-url http://10.0.5.8:30180 --default-route /dashboard --max-redirects 56
```

### bridge_result_upload

```bash
http://10.0.5.8:15174/bridge-result
```

### capture_plan_refresh

```bash
tests/e2e/ui_desktop_capture_plan.mjs --base-url http://10.0.5.8:30180 --receiver-url http://10.0.5.8:15174
```

### evidence_finalize

```bash
ALLOW_BLOCKERS=false tests/e2e/ui_visual_interaction_evidence_finalize.py
```

### ui_visual_interaction_preflight

```bash
DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh
```

### project_completion_audit

```bash
ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh
```

## Visual Batch

| Order | Target | Route | Receiver Upload | Reasons |
|---:|---|---|---|---|
| 1 | login | login | http://10.0.5.8:15174/upload/login | metrics status=fail; pixel mismatch ratio 0.9999099151449858 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| 2 | screen | screen | http://10.0.5.8:15174/upload/screen | metrics status=fail; pixel mismatch ratio 0.9999508069051117 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| 3 | dashboard | dashboard | http://10.0.5.8:15174/upload/dashboard | metrics status=fail; pixel mismatch ratio 0.9999188313934344 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| 4 | alerts | alerts | http://10.0.5.8:15174/upload/alerts | metrics status=fail; pixel mismatch ratio 0.9999425055703494 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| 5 | alert-detail | alert-detail | http://10.0.5.8:15174/upload/alert-detail | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 6 | campaigns | campaigns | http://10.0.5.8:15174/upload/campaigns | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 7 | campaign-detail | campaign-detail | http://10.0.5.8:15174/upload/campaign-detail | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 8 | attack-chains | attack-chains | http://10.0.5.8:15174/upload/attack-chains | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 9 | encrypted-traffic | encrypted-traffic | http://10.0.5.8:15174/upload/encrypted-traffic | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 10 | forensics | forensics | http://10.0.5.8:15174/upload/forensics | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 11 | assets | assets | http://10.0.5.8:15174/upload/assets | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 12 | graph | graph | http://10.0.5.8:15174/upload/graph | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 13 | fusion | fusion | http://10.0.5.8:15174/upload/fusion | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 14 | baselines | baselines | http://10.0.5.8:15174/upload/baselines | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 15 | probes | probes | http://10.0.5.8:15174/upload/probes | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 16 | rules | rules | http://10.0.5.8:15174/upload/rules | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 17 | deployments | deployments | http://10.0.5.8:15174/upload/deployments | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 18 | models | models | http://10.0.5.8:15174/upload/models | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 19 | mlops | mlops | http://10.0.5.8:15174/upload/mlops | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 20 | data-quality | data-quality | http://10.0.5.8:15174/upload/data-quality | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 21 | playbooks | playbooks | http://10.0.5.8:15174/upload/playbooks | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 22 | whitelist | whitelist | http://10.0.5.8:15174/upload/whitelist | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 23 | compliance | compliance | http://10.0.5.8:15174/upload/compliance | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 24 | audit-log | audit-log | http://10.0.5.8:15174/upload/audit-log | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 25 | notifications | notifications | http://10.0.5.8:15174/upload/notifications | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 26 | settings | settings | http://10.0.5.8:15174/upload/settings | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 27 | not-found | not-found | http://10.0.5.8:15174/upload/not-found | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 28 | topics-encrypted-tunnel | topics | http://10.0.5.8:15174/upload/topics-encrypted-tunnel | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 29 | topics-data-exfiltration | topics | http://10.0.5.8:15174/upload/topics-data-exfiltration | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |
| 30 | topics-apt-campaign | topics | http://10.0.5.8:15174/upload/topics-apt-campaign | missing actual-1920.png; missing diff-1920.png; metrics missing; capture-meta missing |

## Interaction Batch

| Order | Route ID | Expected Path | Interaction JSON Upload | Screenshot Upload | Reasons |
|---:|---|---|---|---|---|
| 1 | login | /login | http://10.0.5.8:15174/interaction/login | http://10.0.5.8:15174/interaction-screenshot/login | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| 2 | screen | /screen | http://10.0.5.8:15174/interaction/screen | http://10.0.5.8:15174/interaction-screenshot/screen | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| 3 | dashboard | /dashboard | http://10.0.5.8:15174/interaction/dashboard | http://10.0.5.8:15174/interaction-screenshot/dashboard | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| 4 | alerts | /alerts | http://10.0.5.8:15174/interaction/alerts | http://10.0.5.8:15174/interaction-screenshot/alerts | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| 5 | alert-detail | /alerts/AL-20260620-000123 | http://10.0.5.8:15174/interaction/alert-detail | http://10.0.5.8:15174/interaction-screenshot/alert-detail | interaction missing |
| 6 | campaigns | /campaigns | http://10.0.5.8:15174/interaction/campaigns | http://10.0.5.8:15174/interaction-screenshot/campaigns | interaction missing |
| 7 | campaign-detail | /campaigns/APT-20260619-001 | http://10.0.5.8:15174/interaction/campaign-detail | http://10.0.5.8:15174/interaction-screenshot/campaign-detail | interaction missing |
| 8 | attack-chains | /attack-chains | http://10.0.5.8:15174/interaction/attack-chains | http://10.0.5.8:15174/interaction-screenshot/attack-chains | interaction missing |
| 9 | encrypted-traffic | /encrypted-traffic | http://10.0.5.8:15174/interaction/encrypted-traffic | http://10.0.5.8:15174/interaction-screenshot/encrypted-traffic | interaction missing |
| 10 | forensics | /forensics | http://10.0.5.8:15174/interaction/forensics | http://10.0.5.8:15174/interaction-screenshot/forensics | interaction missing |
| 11 | assets | /assets | http://10.0.5.8:15174/interaction/assets | http://10.0.5.8:15174/interaction-screenshot/assets | interaction missing |
| 12 | graph | /graph | http://10.0.5.8:15174/interaction/graph | http://10.0.5.8:15174/interaction-screenshot/graph | interaction missing |
| 13 | fusion | /fusion | http://10.0.5.8:15174/interaction/fusion | http://10.0.5.8:15174/interaction-screenshot/fusion | interaction missing |
| 14 | baselines | /baselines | http://10.0.5.8:15174/interaction/baselines | http://10.0.5.8:15174/interaction-screenshot/baselines | interaction missing |
| 15 | probes | /probes | http://10.0.5.8:15174/interaction/probes | http://10.0.5.8:15174/interaction-screenshot/probes | interaction missing |
| 16 | rules | /rules | http://10.0.5.8:15174/interaction/rules | http://10.0.5.8:15174/interaction-screenshot/rules | interaction missing |
| 17 | deployments | /deployments | http://10.0.5.8:15174/interaction/deployments | http://10.0.5.8:15174/interaction-screenshot/deployments | interaction missing |
| 18 | models | /models | http://10.0.5.8:15174/interaction/models | http://10.0.5.8:15174/interaction-screenshot/models | interaction missing |
| 19 | mlops | /mlops | http://10.0.5.8:15174/interaction/mlops | http://10.0.5.8:15174/interaction-screenshot/mlops | interaction missing |
| 20 | data-quality | /data-quality | http://10.0.5.8:15174/interaction/data-quality | http://10.0.5.8:15174/interaction-screenshot/data-quality | interaction missing |
| 21 | playbooks | /playbooks | http://10.0.5.8:15174/interaction/playbooks | http://10.0.5.8:15174/interaction-screenshot/playbooks | interaction missing |
| 22 | whitelist | /whitelist | http://10.0.5.8:15174/interaction/whitelist | http://10.0.5.8:15174/interaction-screenshot/whitelist | interaction missing |
| 23 | compliance | /compliance | http://10.0.5.8:15174/interaction/compliance | http://10.0.5.8:15174/interaction-screenshot/compliance | interaction missing |
| 24 | audit-log | /audit-log | http://10.0.5.8:15174/interaction/audit-log | http://10.0.5.8:15174/interaction-screenshot/audit-log | interaction missing |
| 25 | notifications | /notifications | http://10.0.5.8:15174/interaction/notifications | http://10.0.5.8:15174/interaction-screenshot/notifications | interaction missing |
| 26 | settings | /settings | http://10.0.5.8:15174/interaction/settings | http://10.0.5.8:15174/interaction-screenshot/settings | interaction missing |
| 27 | not-found | /__codex_visual_not_found__ | http://10.0.5.8:15174/interaction/not-found | http://10.0.5.8:15174/interaction-screenshot/not-found | interaction missing |
| 28 | topics | /topics | http://10.0.5.8:15174/interaction/topics | http://10.0.5.8:15174/interaction-screenshot/topics | interaction missing |

## Acceptance Contract

- Backend must be `codex-desktop-chrome-extension`; `iab` is forbidden for this evidence.
- Before screenshots, open `/viewport-probe` through `mcp__codex_desktop_node_repl.desktop_chrome_open_url` and confirm it reports `1920x1080`.
- Visual evidence requires `actual-1920.png`, `diff-1920.png`, `metrics.json`, and `capture-meta.json` for every visual target.
- Interaction evidence requires `interaction.json` for every route and must prove no API/runtime failures plus a route-specific business action.
- Protected routes must consume the smoke hash, land on the requested route, avoid `/login`, and leave no token material in the final URL.

