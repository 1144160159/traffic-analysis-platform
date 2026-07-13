# Data Quality Field Quality r232

- Deployment: `traffic/web-ui:ui-data-quality-field-quality-20260711-r232`
- Route: `/data-quality?tab=field-quality`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Business ROI: `content-root`, `x=198, y=80, width=1722, height=917`

## Business Interaction Evidence

- The production route requested `GET /api/v1/data-quality?limit=8&page_size=8` on initial load and again after manual refresh; both responses were `200`.
- `DataQualityFieldTrendChart` rendered one ECharts canvas from `DataQualityVisuals.fieldTrend`; no bitmap UI resource is used for the chart.
- The automatic-refresh control changed `aria-pressed` from `true` to `false` and back to `true`, which enables or pauses the 30-second React Query refresh interval.
- Switching `字段质量 -> Flink 质量 -> 字段质量` retained the same titlebar and Tab-track geometry. All eight Tab slots were equal width, with no horizontal or vertical overflow.
- No bad response, request failure, console error, or page error occurred.

## Visual Result

- Business ROI metric: `192465 / 1579074 = 0.12188472484506742` at channel tolerance `90`.
- Global business ROI acceptance threshold: `< 0.125`; pass.
- Full-image diagnostic: `0.11041618441358024`; non-gating.

Evidence: `evidence/ui-image-breakdowns/pages/data-quality-field-quality/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `interaction-r232.json`.
