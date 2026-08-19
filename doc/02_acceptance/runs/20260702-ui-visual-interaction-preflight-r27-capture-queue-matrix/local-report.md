# UI Visual Interaction Dual Gate

- Run ID: `20260702-ui-visual-interaction-preflight-r27-capture-queue-matrix`
- Result: `blocked`
- Target routes: 28
- Visual targets: 30
- Target source images present: 30/30
- React page components present: 28/28
- Visual diff evidence passed: 0/30
- Business interaction evidence passed: 4/28
- Full-page design-image reference blockers: 0
- Desktop smoke token config: repo=true live=true
- Desktop Chrome status: `blocked`
- Matrix: `doc/02_acceptance/runs/20260702-ui-visual-interaction-preflight-r27-capture-queue-matrix/ui-visual-interaction-matrix.json`

This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1.

## Blockers
- `visual-diff` every visual target has passing 1920x1080 screenshot diff evidence: 30/30 visual targets missing or failing visual diff evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics-encrypted-tunnel, topics-data-exfiltration, topics-apt-campaign
- `business-interaction` every route has passing business interaction evidence: 24/28 routes missing or failing business interaction evidence: alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics
- `desktop-chrome` Codex Desktop Chrome wrapper is available for screenshot and interaction capture: codex-desktop-node-repl MCP transport remains closed on this continuation; desktop_chrome_list_tabs and js_reset returned Transport closed, so no new Desktop Chrome screenshots/interactions can be captured.

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
```

## Next Interaction Capture Queue

| route | expected final path | safe redirect URL | missing or failing |
|---|---|---|---|
| `alert-detail` | `/alerts/AL-20260620-000123` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Falerts%2FAL-20260620-000123` | interaction missing |
| `campaigns` | `/campaigns` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns` | interaction missing |
| `campaign-detail` | `/campaigns/APT-20260619-001` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcampaigns%2FAPT-20260619-001` | interaction missing |
| `attack-chains` | `/attack-chains` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fattack-chains` | interaction missing |
| `encrypted-traffic` | `/encrypted-traffic` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fencrypted-traffic` | interaction missing |
| `forensics` | `/forensics` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fforensics` | interaction missing |
| `assets` | `/assets` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fassets` | interaction missing |
| `graph` | `/graph` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fgraph` | interaction missing |
| `fusion` | `/fusion` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ffusion` | interaction missing |
| `baselines` | `/baselines` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fbaselines` | interaction missing |
| `probes` | `/probes` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fprobes` | interaction missing |
| `rules` | `/rules` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Frules` | interaction missing |
| `deployments` | `/deployments` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdeployments` | interaction missing |
| `models` | `/models` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmodels` | interaction missing |
| `mlops` | `/mlops` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fmlops` | interaction missing |
| `data-quality` | `/data-quality` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fdata-quality` | interaction missing |
| `playbooks` | `/playbooks` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fplaybooks` | interaction missing |
| `whitelist` | `/whitelist` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fwhitelist` | interaction missing |
| `compliance` | `/compliance` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fcompliance` | interaction missing |
| `audit-log` | `/audit-log` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Faudit-log` | interaction missing |
| `notifications` | `/notifications` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fnotifications` | interaction missing |
| `settings` | `/settings` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Fsettings` | interaction missing |
| `not-found` | `/__codex_visual_not_found__` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2F__codex_visual_not_found__` | interaction missing |
| `topics` | `/topics` | `<SMOKE_REDIRECT_BASE_URL>/start?nonce=%3CCODEX_SMOKE_NONCE%3E&route=%2Ftopics` | interaction missing |

Open each safe redirect URL with `mcp__codex_desktop_node_repl.desktop_chrome_open_url` after starting the nonce-only smoke redirect helper.
