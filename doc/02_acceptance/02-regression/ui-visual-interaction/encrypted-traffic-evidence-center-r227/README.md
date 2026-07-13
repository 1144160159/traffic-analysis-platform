# Encrypted Traffic Evidence Center r227

- Deployment: `traffic/web-ui:ui-encrypted-traffic-evidence-center-20260711-r227`
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Business ROI: `content-root`, `x=260, y=80, width=1650, height=917`

## Business Interaction Evidence

- The encrypted business root starts at the shared `.taf-main` content origin: expected `173.995,87.988`, actual `173.993,87.986` CSS pixels.
- Evidence Center, Overview, and returned Evidence Center retain the same titlebar, five-tab track, and right-aligned business-control geometry.
- `近 7 天` generated six encrypted-traffic GET requests, each with valid seven-day `start_time` and `end_time` values.
- Quick locate selected non-first `s-23a9b7d4c1e8` and reduced the evidence-session table from seven rows to one.
- `一键分析` generated `POST /api/v1/encrypted-traffic/evidence-actions` with that same target and `action=associate_analysis`; the response was `200`, with an `action_id`, `ENCRYPTED_EVIDENCE_ANALYSIS_REQUESTED`, and `recorded`.
- No bad response, request failure, console error, page error, horizontal overflow, or vertical overflow occurred.

## Visual Result

- Business ROI metric: `143442 / 1513050 = 0.09480321205512045` at channel tolerance `64`.
- User-approved acceptance threshold: `< 0.13`; pass.
- Historical strict target: `<= 0.015`; deferred for later visual tuning.
- Full-image diagnostic: `0.09058883101851851`; non-gating.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `interaction-r227.json`.
