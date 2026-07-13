# Data Quality Field Quality r234

- Deployment: `traffic/web-ui:ui-data-quality-field-quality-20260711-r234`
- Route: `/data-quality?tab=field-quality`
- Browser: Windows Chrome CDP at `http://127.0.0.1:9224`
- Viewport: `1920 x 1080`
- Business ROI: `content-root`, `x=198, y=80, width=1722, height=917`

## Business Interaction Evidence

- The initial load, switching to `近 7 天`, timed refresh, and manual refresh each called `GET /api/v1/data-quality` successfully. The range request carries `time_range`, `start_time`, and `end_time`.
- With automatic refresh enabled, one scheduled request completed with `200` after 30 seconds. After disabling it, no request occurred during the next 31.5 seconds; after re-enabling it, manual refresh remained available.
- Field quality has one ECharts anomaly-trend canvas and six ECharts KPI-trend canvases. Each chart receives `DataQualityVisuals` real API/typed fallback data rather than a bitmap.
- Switching `字段质量 -> Flink 质量 -> 字段质量` retained equal eight-slot Tabs with no horizontal or vertical overflow.
- No bad response, request failure, console error, or page error occurred.

## Visual Result

- Business ROI metric: `192312 / 1579074 = 0.12178783261582421` at channel tolerance `90`.
- Global business ROI acceptance threshold: `< 0.125`; pass.
- Full-image diagnostic: `0.11035011574074075`; non-gating.

Evidence: `evidence/ui-image-breakdowns/pages/data-quality-field-quality/implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `interaction-r234.json`.
