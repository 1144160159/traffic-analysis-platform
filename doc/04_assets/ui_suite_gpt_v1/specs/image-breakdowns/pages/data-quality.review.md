# data-quality.png review

## Review Status

- Status: `business-pixel-accepted`
- Page id: `data-quality`
- Route: `/data-quality`
- Type: `menu-route`
- Parent: `-`
- Target image: `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality.png`
- Evidence directory: `evidence/ui-image-breakdowns/pages/data-quality/`
- Production URL: `http://10.0.5.8:30180/data-quality?__codex_ui_breakdown_production=1`
- Viewport/state: `1920 x 1080`, overview tab, Windows Chrome CDP
- Production image: `traffic/web-ui:ui-data-quality-visual-20260708-r112`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guides read | pass | `agent.md`, implementation loop, queue docs |
| Queue lock | pass | order 8, `data-quality`, `/data-quality` |
| Windows Chrome CDP precheck | pass | `cdp-version-r112-final-pre-capture.txt`, `cdp-list-r112-final-pre-capture.txt` |
| Production screenshot | pass | `implementation-r112-final.png`, alias `implementation.png` |
| Runtime health | pass | `capture-meta-r112-final.json`: no console/page/request/HTTP errors, no forbidden resource markers, no overflow |
| Text completeness | pass | key text presence all true; clipped text without full hover count `0` |
| Business dynamic visuals | pass | Topic heatmap and Flink trend are React/CSS/SVG from `fetchPageSnapshot` typed fallback visuals |
| noBitmapUi | pass | `npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` |
| Build | pass | `npm --prefix web/ui run build` |
| Business visual diff | pass | `metrics-business-r112.json`, ratio `0.11281865194411408`, max `0.13`, tolerance `90` |
| Full visual diff | pass | `metrics-r112.json`, ratio `0.10591531635802469`, max `0.13`, tolerance `90` |
| Main-thread judgment | pass | `verification.json` status `business-pixel-accepted` |

## Main-Thread Review

The r112 production screenshot was rechecked against the locked target and the diff hotspot image. The business structure now matches the UI image: selected collection-monitoring/data-quality menu state, overview tabs, KPI strip, Kafka Topic table, partition-skew heatmap, Flink processing overview, field/storage/reconcile panels, anomaly rail, quick locate controls, repair advice, and evidence/report actions.

Remaining differences are alpha-level rendering differences: text metrics, sparkline stroke shape, heatmap color alpha, icon stroke, and anti-aliasing. They do not hide or replace business content, and the configured business diff gate passes.

## Dynamic Visuals

| Visual | Data source | Implementation | Refresh |
|---|---|---|---|
| Topic 分区倾斜热力图 | `PageSnapshot.visuals.dataQuality.heatmap` typed fallback via `fetchPageSnapshot(route.id)` | React/CSS grid cells | React Query `30s` |
| Flink 处理质量概览 | `PageSnapshot.visuals.dataQuality.flinkMetrics/flinkTrend` | React metric cards + inline SVG polylines | React Query `30s` |
| KPI/字段趋势 | `PageSnapshot.metrics` and `rows` | inline SVG sparklines | React Query `30s` |

No business dynamic diagram uses target screenshots or page raster assets.

## Evidence

- `target.png`
- `regions-overlay.png`
- `implementation-r112-final.png`
- `implementation.png`
- `target-business-r112.png`
- `implementation-business-r112.png`
- `diff-r112.png`
- `diff.png`
- `diff-business-r112.png`
- `metrics-r112.json`
- `metrics-business-r112.json`
- `capture-meta-r112-final.json`
- `capture-meta.json`
- `verification.json`
- `measurement.json`

## Decision

Accepted as `business-pixel-accepted` for the current page. Do not advance to `data-quality-topic-health` unless this r112 evidence remains the current production evidence.

## r271 Superseding Review

| Check | Result | Evidence |
|---|---|---|
| Windows Chrome runtime and actions | pass | `interaction-r271-all-tabs.json`: overview canvas 22; endpoint/audit Drawer true; error arrays empty |
| Tab fixed geometry | pass | all 8 routes and overview-to-field direct click; both deltas `0` |
| Business ROI | pass | `metrics-business-r271.json`: `0.12151615440441677 < 0.125` |
| API/action contract | pass | `POST /v1/data-quality/actions`, `DATA_QUALITY_ACTION_REQUESTED` |

Main-thread judgment: `business-pixel-accepted-r271`; this section supersedes the queue restriction tied to r112.
