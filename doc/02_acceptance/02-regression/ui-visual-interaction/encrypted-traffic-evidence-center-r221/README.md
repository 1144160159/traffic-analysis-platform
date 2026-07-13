# Encrypted Traffic Evidence Center r221 (Superseded)

This intermediate interaction evidence is superseded by r222, which verifies a non-first quick-location result as the audited one-click analysis target.

- Deployment: `traffic/web-ui:ui-encrypted-traffic-evidence-center-20260711-r221`
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Strict ROI: `content-root`, `x=260, y=80, width=1650, height=917`

## Business Interaction Evidence

- `近 7 天` generated six encrypted-traffic GET requests, each with valid seven-day `start_time` and `end_time` values.
- `一键分析` generated `POST /api/v1/encrypted-traffic/evidence-actions` with `action=associate_analysis`; the UI showed success after the response.
- Quick locate reduced the evidence-session table from seven rows to the selected one matching row.
- No bad response, request failure, console error, page error, or horizontal overflow was observed.

## Visual Result

- Business ROI metric: `200433 / 1513050 = 0.13246951521760683` at channel tolerance `64`.
- Business threshold: `<= 0.35`; pass.
- Strict target: `<= 0.015`; not accepted.
- Full-image diagnostic: `0.11733362268518518`; non-gating.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, `production-route-report.json`, and `interaction-r220.json`.
