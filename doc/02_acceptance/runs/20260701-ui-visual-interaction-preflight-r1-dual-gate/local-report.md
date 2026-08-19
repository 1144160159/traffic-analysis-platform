# UI Visual Interaction Dual Gate

- Run ID: `20260701-ui-visual-interaction-preflight-r1-dual-gate`
- Result: `blocked`
- Target routes: 28
- Target source images present: 27/28
- React page components present: 28/28
- Visual diff evidence passed: 0/28
- Business interaction evidence passed: 0/28
- Full-page design-image reference blockers: 0
- Desktop Chrome status: `transport_closed`
- Matrix: `doc/02_acceptance/runs/20260701-ui-visual-interaction-preflight-r1-dual-gate/ui-visual-interaction-matrix.json`

This gate is intentionally stricter than the UI contract gate. Passing the contract proves route/API/page structure; it does not prove that the real frontend visually matches the generated UI references 1:1.

## Blockers
- `targets` all visual source images exist at required size: topics:missing-sourceImage
- `visual-diff` every route has passing 1920x1080 screenshot diff evidence: 28/28 routes missing or failing visual diff evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics
- `business-interaction` every route has passing business interaction evidence: 28/28 routes missing or failing business interaction evidence: login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail, attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines, probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist, compliance, audit-log, notifications, settings, not-found, topics
- `desktop-chrome` Codex Desktop Chrome wrapper is available for screenshot and interaction capture: Codex Desktop Chrome bridge wrapper transport remains closed; per-page screenshot diff and business interaction capture cannot be completed in this run.

## Required Evidence Layout

```text
doc/02_acceptance/02-regression/ui-visual-interaction/latest/
  <route-id>/
    actual-1920.png
    diff-1920.png
    metrics.json
    interaction.json
```
