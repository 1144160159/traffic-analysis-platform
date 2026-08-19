# UI Visual Interaction Dual Gate

- Run ID: `20260702-ui-visual-interaction-preflight-r11-strict-viewport-meta`
- Result: `blocked`
- Target routes: 28
- Visual targets: 30
- Target source images present: 30/30
- React page components present: 28/28
- Visual diff evidence passed: 0/30
- Business interaction evidence passed: 2/28
- Full-page design-image reference blockers: 0
- Desktop Chrome status: `blocked`
- Matrix: `doc/02_acceptance/runs/20260702-ui-visual-interaction-preflight-r11-strict-viewport-meta/ui-visual-interaction-matrix.json`

This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1.

## Blockers
- `visual-diff` every visual target has passing 1920x1080 screenshot diff evidence: 30/30 visual targets missing or failing visual diff evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics-encrypted-tunnel, topics-data-exfiltration, topics-apt-campaign
- `business-interaction` every route has passing business interaction evidence: 26/28 routes missing or failing business interaction evidence: screen, dashboard, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics
- `desktop-chrome` Codex Desktop Chrome wrapper is available for screenshot and interaction capture: Codex Desktop Chrome extension backend is available; list_tabs/open_url and raw tab.screenshot upload passed for login. Strict capture-meta now requires Desktop Chrome viewport 1920x1080; current login capture is stored 2559x1271 with desktop viewport 2560x1271 DPR 1.5, so 1:1 visual acceptance remains blocked instead of accepting cropped/resized evidence.

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
