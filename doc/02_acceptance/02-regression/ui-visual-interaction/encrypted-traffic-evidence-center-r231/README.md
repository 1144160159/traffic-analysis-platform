# Encrypted Traffic Evidence Center r231

- Deployment: `traffic/web-ui:ui-encrypted-traffic-evidence-center-20260711-r231`
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Business ROI: `content-root`, `x=260, y=80, width=1650, height=917`

## Business Interaction Evidence

- The encrypted business root starts at the shared `.taf-main` content origin: expected `173.995,87.988`, actual `173.993,87.986` CSS pixels.
- The business grid is visible and full-height: `829.051` CSS pixels; the screenshot contains all main, lower, and right-rail evidence panels.
- Evidence Center, Overview, and returned Evidence Center retain the same titlebar, five-tab track, and right-aligned business-control geometry.
- `近 7 天` generated six encrypted-traffic GET requests, each with valid seven-day `start_time` and `end_time` values.
- Quick locate selected non-first `s-23a9b7d4c1e8` and reduced the evidence-session table from seven rows to one.
- `一键分析` generated `POST /api/v1/encrypted-traffic/evidence-actions` with that same target and `action=associate_analysis`; the response was `200`, with an `action_id`, `ENCRYPTED_EVIDENCE_ANALYSIS_REQUESTED`, and `recorded`.
- No bad response, request failure, console error, page error, horizontal overflow, or vertical overflow occurred.

## Visual Result

- Business ROI metric: `195275 / 1513050 = 0.12906050692310234` at channel tolerance `64`.
- Historical r231 acceptance threshold: `< 0.13`; passed at that time.
- Current global business ROI threshold: `< 0.125`; r231 is now `rework-required` because `0.12906050692310234` exceeds it.
- Historical strict target: `<= 0.015`; deferred for later visual tuning.
- Full-image diagnostic: `0.11927372685185185`; non-gating.

## Independent Review

- `Cicero` found no P0 under the historical `< 0.13` gate; that review is superseded by the current global `< 0.125` policy.
- The version-pinned review artifact is `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/agent-r231-review.md`.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `interaction-r231.json`.
