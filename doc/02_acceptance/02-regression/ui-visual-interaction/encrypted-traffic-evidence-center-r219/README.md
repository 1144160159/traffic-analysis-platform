# Encrypted Traffic Evidence Center r219

- Deployment: `traffic/web-ui:ui-encrypted-traffic-evidence-center-20260711-r219`
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Strict ROI: `content-root`, `x=260, y=80, width=1650, height=917`

## Business Scope

The encrypted evidence workbench alone was changed: its width, seven-card KPI strip, three-column main area, compact right evidence rail, typed simulated evidence density, ECharts PCAP/entropy/completeness charts, and evidence actions. The shared topbar, sidebar, and bottombar remain unmodified and are excluded from strict scoring.

## Windows Chrome Result

- Runtime: pass with no bad response, request failure, console error, page error, or horizontal overflow.
- Business ROI metric: `200327 / 1513050 = 0.13239945804831302` at channel tolerance `64`.
- Business threshold: `<= 0.35`; pass.
- Strict target: `<= 0.015`; not accepted.
- Full-image diagnostic: `0.11728636188271604`; non-gating.
- UI checks: 50 passed. Production build: passed.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `production-route-report.json`.
