# Encrypted Traffic Evidence Center r214

> Historical r214 decision. r215 replaces the strict full-image scoring interpretation with the canonical business ROI `content-root (198,80,1722,917)`; see `../encrypted-traffic-evidence-center-r215/README.md` for the current gate.

- Deployment: `traffic/web-ui:ui-encrypted-traffic-evidence-center-20260710-r214`
- Route: `/encrypted-traffic?tab=evidence-center`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- UI tests: 50 passed
- Production build: passed
- Runtime: no bad response, request failure, console error, page error, or horizontal overflow.

## Business Acceptance

- Shared topbar, sidebar, and bottombar match the dashboard and are not route-specialized.
- Five primary Tabs retain fixed geometry.
- The business titlebar keeps time range, refresh, one-click analysis, fingerprint detail, and certificate detail aligned to the right edge.
- Evidence data uses one outer KPI band, a three-panel primary work row, visible bottom details, four ECharts canvases, and PCAP near-field request actions.
- Time-range change, one-click analysis, fingerprint detail Drawer, and PCAP download confirmation were exercised in Windows Chrome.

## Visual Decision

Two production captures recorded pixel mismatch ratios `0.12023148148148148` and `0.12023967978395061` at tolerance `64`. This passes the business threshold `0.35`, but does not meet the strict target `0.015`; strict pixel acceptance is therefore withheld. Public-shell differences are documented as non-actionable under the user-required shared-shell constraint.

Evidence: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/normal-route-r214-business-rework-runtime.json` and `verification.json`.
