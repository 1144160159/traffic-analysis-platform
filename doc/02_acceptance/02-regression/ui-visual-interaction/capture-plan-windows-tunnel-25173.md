# UI Desktop Capture Plan

- Generated: `2026-07-02T16:03:06.334Z`
- Base URL: `http://127.0.0.1:25173`
- Receiver URL: `http://127.0.0.1:25174`
- Viewport probe URL: `http://127.0.0.1:25174/viewport-probe`
- Visual evidence: `0/30`
- Interaction evidence: `4/28`

This is a capture work queue, not acceptance evidence. The dual gate only passes after real Desktop Chrome screenshots, receiver capture metadata, metrics, and interaction JSON files pass.

## Auth Capture Strategy

Protected routes must be opened with a short-lived hash smoke token, for example:

```text
http://127.0.0.1:25173/dashboard#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>
```

The hash must be consumed by the app before evidence is accepted, and the final path must still be the intended route. A protected route that redirects to `/login` is a failed capture, not valid route evidence.

To avoid putting the token into the Desktop Chrome wrapper input, prefer the nonce-only redirect helper:

```bash
DESKTOP_SMOKE_TOKEN=<redacted> CODEX_SMOKE_NONCE=<redacted> tests/e2e/ui_desktop_smoke_redirect.py --host 0.0.0.0 --port <redirect-port> --app-base-url http://127.0.0.1:25173 --default-route /dashboard --max-redirects 56
```

Then open the route-specific `safe_redirect_url_pattern` with `desktop_chrome_open_url`; the helper redirects Chrome to the token hash URL exactly once.

Before capturing screenshots, open the receiver viewport probe with the Desktop Chrome wrapper and confirm it reports `1920x1080`:

```text
mcp__codex_desktop_node_repl.desktop_chrome_open_url url=http://127.0.0.1:25174/viewport-probe keep=true wait_ms=1500
```

Before starting a capture session, run the receiver endpoint self-test:

```bash
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
```

## Receiver Self-test

- Self-test: `doc/02_acceptance/02-regression/ui-visual-interaction/receiver-selftest-latest.json`
- Result: `pass`
- Checks: `11/11`
- Acceptance effect: Proves the receiver endpoint behavior only; does not replace Desktop Chrome visual or interaction evidence.

## Desktop Viewport Probe

- Probe: `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-viewport-probe-latest.json`
- Result: `blocked`
- Current screenshot size: `2560x1271`
- Acceptance effect: Current Desktop Chrome bridge can expose the Chrome extension target, open /login, and capture a real page before viewport-control experimentation. However, the only discovered viewport-control path is CDP, and the r3 CDP device metrics probe left tab control timing out. The 1:1 visual gate remains blocked until Desktop Chrome can reliably produce browser-generated 1920x1080 screenshots with receiver capture-meta.json proving uploaded/stored size and browser viewport.

## Latest Gap Report

- Gap report: `doc/02_acceptance/02-regression/ui-visual-interaction-gap-report-latest.json`
- Run ID: `20260702-ui-visual-interaction-preflight-r40-receiver-selftest`
- Visual gaps: `30/30`
- Interaction gaps: `24/28`

## Receiver

```bash
DESKTOP_SMOKE_TOKEN=<redacted> CODEX_CAPTURE_KEY=<redacted> tests/e2e/ui_desktop_capture_receiver.py --host 0.0.0.0 --port <port> --evidence-dir doc/02_acceptance/02-regression/ui-visual-interaction/latest --max-uploads 58 --expected-width 1920 --expected-height 1080
```

## Next Visual Targets

| target | route | URL | missing or failing |
|---|---|---|---|
| `login` | `login` | `http://127.0.0.1:25173/login?__taf_visual=1` | metrics status=fail; pixel mismatch ratio 0.9999099151449858 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| `screen` | `screen` | `http://127.0.0.1:25173/screen#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | metrics status=fail; pixel mismatch ratio 0.9999508069051117 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| `dashboard` | `dashboard` | `http://127.0.0.1:25173/dashboard#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | metrics status=fail; pixel mismatch ratio 0.9999188313934344 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| `alerts` | `alerts` | `http://127.0.0.1:25173/alerts#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | metrics status=fail; pixel mismatch ratio 0.9999425055703494 > 0.015; viewport 2559x1271 != 1920x1080; capture-meta status=blocked; uploaded screenshot 2559x1271 != 1920x1080; stored screenshot 2559x1271 != 1920x1080; Desktop Chrome viewport 2560x1271 != 1920x1080 |
| `alert-detail` | `alert-detail` | `http://127.0.0.1:25173/alerts/AL-20260620-000123#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `campaigns` | `campaigns` | `http://127.0.0.1:25173/campaigns#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `campaign-detail` | `campaign-detail` | `http://127.0.0.1:25173/campaigns/APT-20260619-001#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `attack-chains` | `attack-chains` | `http://127.0.0.1:25173/attack-chains#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `encrypted-traffic` | `encrypted-traffic` | `http://127.0.0.1:25173/encrypted-traffic#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `forensics` | `forensics` | `http://127.0.0.1:25173/forensics#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `assets` | `assets` | `http://127.0.0.1:25173/assets#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `graph` | `graph` | `http://127.0.0.1:25173/graph#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `fusion` | `fusion` | `http://127.0.0.1:25173/fusion#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `baselines` | `baselines` | `http://127.0.0.1:25173/baselines#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `probes` | `probes` | `http://127.0.0.1:25173/probes#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `rules` | `rules` | `http://127.0.0.1:25173/rules#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `deployments` | `deployments` | `http://127.0.0.1:25173/deployments#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `models` | `models` | `http://127.0.0.1:25173/models#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `mlops` | `mlops` | `http://127.0.0.1:25173/mlops#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `data-quality` | `data-quality` | `http://127.0.0.1:25173/data-quality#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `playbooks` | `playbooks` | `http://127.0.0.1:25173/playbooks#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `whitelist` | `whitelist` | `http://127.0.0.1:25173/whitelist#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `compliance` | `compliance` | `http://127.0.0.1:25173/compliance#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `audit-log` | `audit-log` | `http://127.0.0.1:25173/audit-log#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `notifications` | `notifications` | `http://127.0.0.1:25173/notifications#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `settings` | `settings` | `http://127.0.0.1:25173/settings#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `not-found` | `not-found` | `http://127.0.0.1:25173/__codex_visual_not_found__#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `topics-encrypted-tunnel` | `topics` | `http://127.0.0.1:25173/topics?topic=tunnel&tab=tunnel#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `topics-data-exfiltration` | `topics` | `http://127.0.0.1:25173/topics?topic=exfil&tab=exfil#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |
| `topics-apt-campaign` | `topics` | `http://127.0.0.1:25173/topics?topic=apt&tab=apt#codex_smoke_token=<DESKTOP_SMOKE_TOKEN>` | missing actual-1920.png; missing diff-1920.png; missing metrics.json; missing capture-meta.json |

## Next Interaction Routes

| route | URL | template | auth mode | required business action | missing or failing |
|---|---|---|---|---|---|
| `alert-detail` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Falerts%2FAL-20260620-000123` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/alert-detail/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 告警详情: verify live data from /api/v1/alerts/{id}, /api/v1/alerts/{id}/evidence, /api/v1/alerts/{id}/feedback; perform one route-specific read or safe UI action | missing interaction.json |
| `campaigns` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/campaigns/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 战役列表: verify live data from /api/v1/campaigns; perform one route-specific read or safe UI action | missing interaction.json |
| `campaign-detail` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns%2FAPT-20260619-001` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/campaign-detail/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 战役详情: verify live data from /api/v1/campaigns/{id}; perform one route-specific read or safe UI action | missing interaction.json |
| `attack-chains` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fattack-chains` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/attack-chains/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 攻击链分析: verify live data from /api/v1/attack-chains; perform one route-specific read or safe UI action | missing interaction.json |
| `encrypted-traffic` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fencrypted-traffic` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/encrypted-traffic/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 加密流量: verify live data from /api/v1/encrypted-traffic/stats, /api/v1/encrypted-traffic/sessions, /api/v1/encrypted-traffic/ja3, /api/v1/encrypted-traffic/tunnels, /api/v1/encrypted-traffic/exfiltration; perform one route-specific read or safe UI action | missing interaction.json |
| `forensics` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fforensics` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/forensics/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 取证分析: verify live data from /api/v1/pcap/jobs, /api/v1/pcap/stats; perform one route-specific read or safe UI action | missing interaction.json |
| `assets` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fassets` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/assets/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 资产台账: verify live data from /api/v1/assets; perform one route-specific read or safe UI action | missing interaction.json |
| `graph` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fgraph` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/graph/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 实体图谱: verify live data from /api/v1/graph/explore; perform one route-specific read or safe UI action | missing interaction.json |
| `fusion` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ffusion` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/fusion/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 数据融合: verify live data from /api/v1/fusion/stats, /api/v1/fusion/entities, /api/v1/fusion/value-report; perform one route-specific read or safe UI action | missing interaction.json |
| `baselines` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fbaselines` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/baselines/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 行为基准: verify live data from /api/v1/baselines; perform one route-specific read or safe UI action | missing interaction.json |
| `probes` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fprobes` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/probes/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 探针管理: verify live data from /api/v1/probes; perform one route-specific read or safe UI action | missing interaction.json |
| `rules` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Frules` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/rules/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 规则管理: verify live data from /api/v1/rules; perform one route-specific read or safe UI action | missing interaction.json |
| `deployments` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdeployments` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/deployments/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 部署管理: verify live data from /api/v1/deployments; perform one route-specific read or safe UI action | missing interaction.json |
| `models` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmodels` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/models/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 模型管理: verify live data from /api/v1/models; perform one route-specific read or safe UI action | missing interaction.json |
| `mlops` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmlops` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/mlops/interaction.template.json` | `authenticated-or-controlled-smoke-token` | MLOps 编排: verify live data from /api/v1/mlops/status, /api/v1/mlops/conditions; perform one route-specific read or safe UI action | missing interaction.json |
| `data-quality` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdata-quality` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/data-quality/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 数据质量: verify live data from /api/v1/data-quality; perform one route-specific read or safe UI action | missing interaction.json |
| `playbooks` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fplaybooks` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/playbooks/interaction.template.json` | `authenticated-or-controlled-smoke-token` | SOAR 剧本: verify live data from /api/v1/playbooks/catalog, /api/v1/playbooks/executions; perform one route-specific read or safe UI action | missing interaction.json |
| `whitelist` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fwhitelist` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/whitelist/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 白名单: verify live data from /api/v1/whitelist; perform one route-specific read or safe UI action | missing interaction.json |
| `compliance` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcompliance` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/compliance/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 合规审计: verify live data from /api/v1/compliance/reports, /api/v1/compliance/audit-trail; perform one route-specific read or safe UI action | missing interaction.json |
| `audit-log` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Faudit-log` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/audit-log/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 审计日志: verify live data from /api/v1/audit/logs; perform one route-specific read or safe UI action | missing interaction.json |
| `notifications` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fnotifications` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/notifications/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 通知配置: verify live data from /api/v1/notifications/settings; perform one route-specific read or safe UI action | missing interaction.json |
| `settings` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fsettings` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/settings/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 系统设置: verify live data from /api/v1/tokens/scopes, /api/v1/tokens, /api/v1/tokens/scopes/probe; perform one route-specific read or safe UI action | missing interaction.json |
| `not-found` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2F__codex_visual_not_found__` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/not-found/interaction.template.json` | `authenticated-or-controlled-smoke-token` | render not-found recovery action and return navigation affordance | missing interaction.json |
| `topics` | `http://127.0.0.1:25175/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ftopics` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/topics/interaction.template.json` | `authenticated-or-controlled-smoke-token` | 专题面板: verify live data from /api/v1/topics/tunnel, /api/v1/topics/exfil, /api/v1/topics/apt; perform one route-specific read or safe UI action | missing interaction.json |

## Metrics Commands

Run the matching command after each real screenshot upload. The upload must also create `capture-meta.json`; a cropped or resized file without receiver metadata remains blocked.

```bash
tests/e2e/ui_visual_diff_metrics.py --target-id login --route /login?__taf_visual=1 --source doc/04_assets/ui_suite_gpt_v1/screens/pages/login.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/login/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/login/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/login/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id screen --route /screen --source doc/04_assets/ui_suite_gpt_v1/screens/pages/screen.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/screen/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/screen/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/screen/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id dashboard --route /dashboard --source doc/04_assets/ui_suite_gpt_v1/screens/pages/dashboard.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/dashboard/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id alerts --route /alerts --source doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/alerts/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/alerts/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/alerts/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id alert-detail --route /alerts/AL-20260620-000123 --source doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/alert-detail/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id campaigns --route /campaigns --source doc/04_assets/ui_suite_gpt_v1/screens/pages/campaigns.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaigns/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id campaign-detail --route /campaigns/APT-20260619-001 --source doc/04_assets/ui_suite_gpt_v1/screens/pages/campaign-detail.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/campaign-detail/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id attack-chains --route /attack-chains --source doc/04_assets/ui_suite_gpt_v1/screens/pages/attack-chains.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/attack-chains/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id encrypted-traffic --route /encrypted-traffic --source doc/04_assets/ui_suite_gpt_v1/screens/pages/encrypted-traffic.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/encrypted-traffic/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id forensics --route /forensics --source doc/04_assets/ui_suite_gpt_v1/screens/pages/forensics.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/forensics/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id assets --route /assets --source doc/04_assets/ui_suite_gpt_v1/screens/pages/assets.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/assets/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id graph --route /graph --source doc/04_assets/ui_suite_gpt_v1/screens/pages/graph.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/graph/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id fusion --route /fusion --source doc/04_assets/ui_suite_gpt_v1/screens/pages/fusion.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/fusion/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id baselines --route /baselines --source doc/04_assets/ui_suite_gpt_v1/screens/pages/baselines.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/baselines/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id probes --route /probes --source doc/04_assets/ui_suite_gpt_v1/screens/pages/probes.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/probes/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id rules --route /rules --source doc/04_assets/ui_suite_gpt_v1/screens/pages/rules.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/rules/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id deployments --route /deployments --source doc/04_assets/ui_suite_gpt_v1/screens/pages/deployments.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/deployments/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id models --route /models --source doc/04_assets/ui_suite_gpt_v1/screens/pages/models.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/models/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id mlops --route /mlops --source doc/04_assets/ui_suite_gpt_v1/screens/pages/mlops.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/mlops/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id data-quality --route /data-quality --source doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/data-quality/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id playbooks --route /playbooks --source doc/04_assets/ui_suite_gpt_v1/screens/pages/playbooks.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/playbooks/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id whitelist --route /whitelist --source doc/04_assets/ui_suite_gpt_v1/screens/pages/whitelist.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/whitelist/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id compliance --route /compliance --source doc/04_assets/ui_suite_gpt_v1/screens/pages/compliance.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/compliance/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id audit-log --route /audit-log --source doc/04_assets/ui_suite_gpt_v1/screens/pages/audit-log.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/audit-log/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id notifications --route /notifications --source doc/04_assets/ui_suite_gpt_v1/screens/pages/notifications.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/notifications/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id settings --route /settings --source doc/04_assets/ui_suite_gpt_v1/screens/pages/settings.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/settings/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id not-found --route /__codex_visual_not_found__ --source doc/04_assets/ui_suite_gpt_v1/screens/pages/not-found.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/not-found/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id topics-encrypted-tunnel --route /topics?topic=tunnel&tab=tunnel --source doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-encrypted-tunnel.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-encrypted-tunnel/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-encrypted-tunnel/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-encrypted-tunnel/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id topics-data-exfiltration --route /topics?topic=exfil&tab=exfil --source doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-data-exfiltration.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-data-exfiltration/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-data-exfiltration/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-data-exfiltration/metrics.json || true
tests/e2e/ui_visual_diff_metrics.py --target-id topics-apt-campaign --route /topics?topic=apt&tab=apt --source doc/04_assets/ui_suite_gpt_v1/screens/pages/topics-apt-campaign.png --actual doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-apt-campaign/actual-1920.png --diff doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-apt-campaign/diff-1920.png --metrics doc/02_acceptance/02-regression/ui-visual-interaction/latest/topics-apt-campaign/metrics.json || true
```

