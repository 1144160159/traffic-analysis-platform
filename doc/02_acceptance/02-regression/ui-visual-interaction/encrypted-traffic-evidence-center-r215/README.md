# Encrypted Traffic Evidence Center r215 (Historical)

- Scope: strict pixel scoring is limited to the business content region.
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- ROI: historical `content-root`, `x=198, y=80, width=1722, height=917`

> Superseded by r219's corrected source business ROI `content-root (260,80,1650,917)`. See `../encrypted-traffic-evidence-center-r219/README.md` for the current gate.

## Scoring Contract

`capture_image_breakdown_production_route.mjs` resolves `pixel_diff.strict_scoring_region.region_id` from the page specification and passes it to `ui_visual_diff_metrics.py`. The comparator counts and overlays differences only inside the ROI. Topbar, sidebar, and bottombar remain in the screenshot and full-image diagnostics, but cannot affect the strict result.

## Windows Chrome Result

- Runtime: pass, with no bad response, request failure, console error, page error, or horizontal overflow.
- Strict ROI metric: `204436 / 1579074 = 0.12946575018016887` at channel tolerance `64`.
- Strict target: `<= 0.015`; result: not accepted.
- Business threshold: `<= 0.35`; result: pass.
- Full-image diagnostic: `0.12023919753086419`; non-gating.
- Comparator regression: `python3 -m unittest tests/e2e/test_ui_visual_diff_metrics.py` passed.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `production-route-report.json`.
