# probes.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Scope: business-area-focused production visual gate with AppShell proportion check
- Evidence target: `evidence/ui-image-breakdowns/pages/probes/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/probes?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783506839076#codex_smoke_token=<redacted>`
- Viewport/state: 1920 x 1080, probe management default data state, visible Windows Chrome tab
- Current round: `r160`, image `traffic/web-ui:ui-probes-visual-20260708-r160`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Queue lock | pass | page-id `probes`, route `/probes`, type `menu-route`, parent `-` |
| Required guide read | pass | `agent.md`, traffic-platform skill, and page verification loop read before edits |
| Target PNG exists | pass | `evidence/ui-image-breakdowns/pages/probes/target.png` |
| Windows Chrome precheck | pass | `cdp-version-r160-pre-capture.txt`, `cdp-list-r160-pre-capture.txt` |
| Windows Chrome screenshot | pass | `implementation-r160-final.png` captured through CDP and aliased to `implementation.png` |
| Production image | pass | Deployment image `traffic/web-ui:ui-probes-visual-20260708-r160` confirmed after rollout |
| Runtime health | pass | `capture-meta-r160-final.json`: no console/pageerror/requestfailed/HTTP 4xx/5xx; no forbidden resource markers |
| Layout/overflow | pass | business content x=198,y=80,w=1712,h=917; sidebar 190px; no panel overflow or horizontal overflow |
| Text completeness | pass | key texts present; clipped cells have title/Tooltip/full text |
| Dynamic visuals | pass | topology and trends are React/CSS/SVG from API/typed fallback data; no business screenshot replay |
| Business visual diff | pass | ratio `0.06454352360940653`, threshold `<= 0.13`, tolerance `90` |
| Main-thread judgment | pass | `verification.json` updated to `business-pixel-accepted` |

## Dynamic Visual Classification

| Visual | Data Source | Mapping | Refresh | Implementation |
|---|---|---|---|---|
| Deployment topology | `fetchPageSnapshot("probes")` via `services/api.ts` + typed fallback | probe id, location, status, link bandwidth/tone | 15s React Query | ECharts graph canvas |
| Throughput/loss trend | PageSnapshot-derived typed fallback series, reserved for real API trend fields | probeDc/probeBuild/probeOffice/batchBandwidth, PPS/drop/parse/backpressure | follows page 15s refresh | Two ECharts line canvases and KPI cells |
| Status matrix/detail rail | PageSnapshot rows | table columns, selected row, status tags, batch config, heartbeat rows | 15s page refresh and row selection | CSS grid, StatusTag, AntD icons/tooltips |

## Main-Thread Findings

- r160 repairs the visible mismatch called out after r159: the visual-breakdown AppShell now uses a 190px sidebar and a 430px brand/topbar area, so the product title is no longer truncated and the shell proportions match the target more closely.
- The probes business region stays anchored at x=198 and does not overlap the bottom status bar.
- Diff hotspots remain in topology line geometry, microtext antialiasing, icon glyph strokes and brightness; these are accepted under the current alpha business-page tolerance.
- No business diagram is implemented as a screenshot resource; forbidden target/evidence resource requests are absent.

## Reproduction

1. Save CDP prechecks from `http://127.0.0.1:9224/json/version` and `/json/list` as the r160 pre-capture files.
2. Connect with `chromium.connectOverCDP("http://127.0.0.1:9224")`.
3. Open `http://10.0.5.8:30180/probes?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783506839076#codex_smoke_token=<redacted>` at 1920 x 1080 DPR 1 and save `implementation-r160-final.png`.
4. Crop x=198, y=80, w=1722, h=917 from target and implementation.
5. Run `tests/e2e/ui_visual_diff_metrics.py` with `--channel-tolerance 90 --max-pixel-ratio 0.13`.
6. Recheck `implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `verification.json` before accepting.

## Decision

Accepted for the current page gate as `business-pixel-accepted`. The latest production screenshot matches the locked probe-management target closely enough in shell proportions, menu selected state, KPI strip, topology, status matrix, trend chart, detail rail, batch actions and heartbeat log; remaining differences are documented alpha-tolerance rendering differences.

## r246 Regression Review

| Check | Result | Evidence |
|---|---|---|
| Dynamic charts and topology | pass | Three ECharts canvases are rendered for deployment topology, throughput, and batch-send bandwidth. |
| Buttons have observable outcomes | pass | Topology mode, range selection, full-screen matrix, batch actions, row actions, and heartbeat rows change state or open a confirmation Drawer. |
| Matrix is usable | pass | Controlled page 2 is available and `.taf-probes-status-matrix` uses `overflow-y: auto`. |
| Windows Chrome runtime | pass | `interaction-r246.json` has no bad response, console error, page error, or request failure. |
| Business ROI | pass | `metrics-business-r246.json`: `0.06313953620919602 <= 0.125`; bbox `198,80,1722,917`. |

The r246 production image is `traffic/web-ui:ui-probes-echarts-20260711-r246`. Screenshot review confirms the ECharts topology and two trend panels stay inside the existing probes business grid without changing public shell geometry.

## r259 SVG Rollback Review

| Check | Result | Evidence |
|---|---|---|
| API-driven topology SVG | pass | `interaction-r259-topology-svg.json` reports one SVG and eight rendered nodes; the data path remains `fetchPageSnapshot("probes")`. |
| Non-topology charts | pass | Two throughput and batch-bandwidth ECharts canvases remain dynamic. |
| Buttons, pagination and overflow | pass | 2D/range controls, matrix page 2, `overflow-y: auto`, batch/row Drawers and full-screen matrix all produce observable state. |
| Windows Chrome runtime | pass | No bad response, request failure, console error or page error. |
| Business ROI | pass | `metrics-business-r259.json`: `0.06007381541333719 <= 0.125`. |

Main-thread judgment: `business-pixel-accepted`. The current production image is `traffic/web-ui:ui-screen-original-svg-20260711-r259`; the r246 ECharts topology record is historical and no longer describes the deployed topology implementation.
