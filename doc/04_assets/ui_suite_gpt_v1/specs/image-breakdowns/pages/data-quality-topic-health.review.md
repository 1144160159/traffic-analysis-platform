# data-quality-topic-health.png review

## Review Status

- Status: `business-pixel-accepted`
- Queue item: 9
- Type: `menu-state`
- Parent: `data-quality`
- Route: `/data-quality?tab=topic-health`
- Target image reviewed directly: yes
- Evidence target: `evidence/ui-image-breakdowns/pages/data-quality-topic-health/target.png`
- Browser evidence: Windows Chrome CDP `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/data-quality?tab=topic-health&__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783511991614#codex_smoke_token=<redacted>`
- Viewport/state: 1920 x 1080, `tab=topic-health`
- Production image: `traffic/web-ui:ui-data-quality-topic-health-visual-20260708-r114`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, verification loop, and queue docs read before edits |
| Queue lock | pass | `data-quality-topic-health`, parent `data-quality`, menu-state route `/data-quality?tab=topic-health` |
| Target PNG exists | pass | `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-topic-health.png` |
| Windows Chrome precheck | pass | `cdp-version-r114-final-pre-capture.txt`, `cdp-list-r114-final-pre-capture.txt` |
| Production screenshot | pass | `implementation-r114-final.png`, alias `implementation.png` |
| Runtime health | pass | `capture-meta-r114-final.json`: no console/page/request/HTTP errors, no forbidden resource markers, no horizontal overflow, no clipped text without full hover |
| Full diff | pass | `metrics-r114.json`, ratio `0.10362461419753087`, threshold `0.13`, tolerance `90` |
| Business diff | pass | `metrics-business-r114.json`, ratio `0.1124671801321534`, threshold `0.13`, tolerance `90` |
| Business dynamic visuals | pass | Latency trend, partition heatmap, and message distribution are React/SVG/CSS components driven by `PageSnapshot.visuals.dataQuality` typed fallback |
| Breadcrumb constraint | pass | Business area top has page name `数据质量` and tabs only; no breadcrumb/menu path |
| Menu selection | pass | Detail/tab state keeps `采集监测 / 数据质量` selected |
| Evidence completeness | pass | `target.png`, `regions-overlay.png`, `implementation.png`, `diff.png`, `metrics.json`, `measurement.json`, `capture-meta.json`, `verification.json`, JSON and review are present |

## Main-Thread Judgment

The r114 production screenshot matches the locked Topic 健康 target at the business-structure level. The page now uses the target layout: page title and tab strip, seven Topic KPI cards, Kafka Topic health detail table, latency trend, flow_original partition-skew heatmap, Consumer Group health, message-size distribution, abnormal partition queue, and right-side alert/location/repair/evidence rail.

Remaining diff hotspots are accepted under the current alpha rule: AppShell styling differences, text anti-aliasing, chart curve sampling, and heatmap color intensity. No hotspot indicates the old incorrect layout or missing business module.

## Dynamic Visual Mapping

| Visual | Data source | Fields | Implementation | Refresh |
|---|---|---|---|---|
| 消费延迟趋势 | `fetchPageSnapshot(route.id)` typed fallback | P50, P95, threshold, callout markers | React SVG polyline | 30s React Query refetch |
| Topic 分区倾斜热力图 | `PageSnapshot.visuals.dataQuality.heatmap` | partition bands, tone values, time axis, legend | React CSS grid heatmap | 30s React Query refetch |
| 消息大小吞吐分布 | `PageSnapshot.visuals.dataQuality.messageSizeDistribution/messageSizeTopicRows` | bucket percent, topic size, EPS, compression | React/CSS bar chart plus dense table | 30s React Query refetch |

## Files

- Code: `web/ui/src/pages/DataQualityPage.tsx`, `web/ui/src/services/mockData.ts`, `web/ui/src/styles/pages.css`
- Deployment: `deployments/kubernetes/applications/web-ui.yaml`
- Evidence: `evidence/ui-image-breakdowns/pages/data-quality-topic-health/`

## Decision

`business-pixel-accepted`. Do not advance beyond this page unless the queue controller accepts this r114 evidence package.

## r271 Superseding Review

- Runtime/actions: pass; canvas `7`; endpoint/audit true; error arrays empty in `../data-quality/interaction-r271-all-tabs.json`.
- Fixed geometry: all-route and direct-click deltas `0`. Visual: `metrics-business-r271.json` ratio `0.11704137994799484 < 0.125`.
- Main-thread judgment: `business-pixel-accepted-r271`; this supersedes the r114 queue restriction.
