# UI Visual Interaction Dual Gate

- Run ID: `20260702-ui-visual-interaction-preflight-r26-interaction-strictness`
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
- Matrix: `doc/02_acceptance/runs/20260702-ui-visual-interaction-preflight-r26-interaction-strictness/ui-visual-interaction-matrix.json`

This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1.

## Blockers
- `visual-diff` every visual target has passing 1920x1080 screenshot diff evidence: 30/30 visual targets missing or failing visual diff evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics-encrypted-tunnel, topics-data-exfiltration, topics-apt-campaign
- `business-interaction` every route has passing business interaction evidence: 24/28 routes missing or failing business interaction evidence: alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics
- `desktop-chrome` Codex Desktop Chrome wrapper is available for screenshot and interaction capture: codex-desktop-node-repl MCP transport still closed on this continuation; desktop_chrome_list_tabs and js_reset cannot run, so no new Desktop Chrome screenshots/interactions can be captured.

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
