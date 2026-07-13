# UI Visual Interaction Dual Gate

- Run ID: `20260703174819-ui-visual-interaction-preflight`
- Result: `blocked`
- Target routes: 28
- Visual targets: 27
- Target source images present: 27/27
- React page components present: 28/28
- Visual diff evidence passed: 0/27
- Business interaction evidence passed: 0/28
- Full-page design-image reference blockers: 0
- Desktop smoke token config: repo=true live=true
- Desktop Chrome status: `pass`
- Capture session: `desktop_bridge_blocked` covers_current_gaps=false
- Matrix: `doc/02_acceptance/runs/20260703174819-ui-visual-interaction-preflight/ui-visual-interaction-matrix.json`

This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1.

## Blockers
- `execution-package` Desktop Chrome bridge run summary is present and matches capture batch: path=doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-run-latest.json exists=false valid=false result=missing backend=missing visual=missing/30 interaction=missing/28 reason=missing
- `visual-diff` every visual target has passing 1920x1080 screenshot diff evidence: 27/27 visual targets missing or failing visual diff evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found
- `business-interaction` every route has passing business interaction evidence: 28/28 routes missing or failing business interaction evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics

## Required Evidence Layout

```text
doc/02_acceptance/02-regression/ui-visual-interaction/latest/
  <visual-target-id>/
    actual-1920.png
    diff-1920.png
    metrics.json
    capture-meta.json
  <route-id>/
    interaction.json
    interaction.png
    interaction-capture-meta.json
  desktop-chrome-bridge-tool-call-latest.json
  desktop-chrome-bridge-run-latest.json
```

## Next Interaction Capture Queue

| route | expected final path | safe redirect URL | template | missing or failing |
|---|---|---|---|---|
| `login` | `/login` | `http://10.0.5.8:30180/login` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/login/interaction.template.json` | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| `screen` | `/screen` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fscreen` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/screen/interaction.template.json` | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| `dashboard` | `/dashboard` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdashboard` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/dashboard/interaction.template.json` | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| `alerts` | `/alerts` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Falerts` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/alerts/interaction.template.json` | desktop_backend extension != codex-desktop-chrome-extension; interaction screenshot 2559x1271 != 1920x1080; interaction-capture-meta missing |
| `alert-detail` | `/alerts/AL-20260620-000123` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Falerts%2FAL-20260620-000123` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/alert-detail/interaction.template.json` | interaction missing |
| `campaigns` | `/campaigns` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/campaigns/interaction.template.json` | interaction missing |
| `campaign-detail` | `/campaigns/APT-20260619-001` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns%2FAPT-20260619-001` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/campaign-detail/interaction.template.json` | interaction missing |
| `attack-chains` | `/attack-chains` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fattack-chains` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/attack-chains/interaction.template.json` | interaction missing |
| `encrypted-traffic` | `/encrypted-traffic` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fencrypted-traffic` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/encrypted-traffic/interaction.template.json` | interaction missing |
| `forensics` | `/forensics` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fforensics` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/forensics/interaction.template.json` | interaction missing |
| `assets` | `/assets` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fassets` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/assets/interaction.template.json` | interaction missing |
| `graph` | `/graph` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fgraph` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/graph/interaction.template.json` | interaction missing |
| `fusion` | `/fusion` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ffusion` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/fusion/interaction.template.json` | interaction missing |
| `baselines` | `/baselines` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fbaselines` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/baselines/interaction.template.json` | interaction missing |
| `probes` | `/probes` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fprobes` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/probes/interaction.template.json` | interaction missing |
| `rules` | `/rules` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Frules` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/rules/interaction.template.json` | interaction missing |
| `deployments` | `/deployments` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdeployments` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/deployments/interaction.template.json` | interaction missing |
| `models` | `/models` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmodels` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/models/interaction.template.json` | interaction missing |
| `mlops` | `/mlops` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmlops` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/mlops/interaction.template.json` | interaction missing |
| `data-quality` | `/data-quality` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdata-quality` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/data-quality/interaction.template.json` | interaction missing |
| `playbooks` | `/playbooks` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fplaybooks` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/playbooks/interaction.template.json` | interaction missing |
| `whitelist` | `/whitelist` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fwhitelist` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/whitelist/interaction.template.json` | interaction missing |
| `compliance` | `/compliance` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcompliance` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/compliance/interaction.template.json` | interaction missing |
| `audit-log` | `/audit-log` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Faudit-log` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/audit-log/interaction.template.json` | interaction missing |
| `notifications` | `/notifications` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fnotifications` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/notifications/interaction.template.json` | interaction missing |
| `settings` | `/settings` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fsettings` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/settings/interaction.template.json` | interaction missing |
| `not-found` | `/__codex_visual_not_found__` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2F__codex_visual_not_found__` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/not-found/interaction.template.json` | interaction missing |
| `topics` | `/topics` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ftopics` | `doc/02_acceptance/02-regression/ui-visual-interaction/templates/topics/interaction.template.json` | interaction missing |

Open each safe redirect URL with `mcp__codex_desktop_node_repl.desktop_chrome_open_url` after starting the nonce-only smoke redirect helper.
