# topics-apt-campaign.png review

## Review Status

- Status: `business-pixel-accepted`
- Page: `topics-apt-campaign`
- Target image reviewed directly: yes
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/topics?topic=apt&tab=apt&__codex_ui_breakdown_production=1`
- Viewport/state: 1920 x 1080, DPR 1, `topic=apt`, `tab=apt`
- Business crop viewport: 1722 x 917, bbox `x=198,y=80,w=1722,h=917`
- Deployed image: `traffic/web-ui:ui-topic-apt-evidence-overlap-20260708-r150`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and UI verification docs were read before edits |
| Target PNG exists | pass | `target.png` exists in the page evidence directory |
| Breakdown artifacts | pass | `measurement.json`, `regions-overlay.png`, OCR and page spec are present |
| Windows Chrome precheck | pass | `cdp-version-r150-final-pre-capture.txt`, `cdp-list-r150-final-pre-capture.txt` |
| Production screenshot | pass | `implementation-r150-final.png` and alias `implementation.png` captured from Windows Chrome CDP on the 30180 route |
| Runtime health | pass | `capture-meta-r150-final.json` has 0 console, pageerror, requestfailed, HTTP 4xx/5xx, forbidden resource markers, panel overflow, and clipped business text without title |
| Visual diff | pass | `pixel_mismatch_ratio=0.10230046216960066`, threshold `<=0.13`, channel tolerance `90` |
| Dynamic business diagram rule | pass | APT attack chain, ATT&CK matrix, trend, IoC, response ring, evidence table and right rail derive from API/adapted typed fallback data |
| Main-thread judgment | pass | `verification.json` records `business-pixel-accepted` for r150 |

## Visual Findings

- r150 corrects the earlier generic-topic mismatch: the right delivery rail starts at the business-region top and contains delivery summary, evidence completeness, report preview and topic actions.
- The lower business row now matches the target structure: `战役关联事件与证据 / campaign-20260620-apt01` on the left, independent `处置动作状态（近30天）` response panel on the right.
- The target APT data state is represented: `APT-CN-2026`, `TEMP.HAWK`, `UNKNOWN-07`, `TA0011`, `TA0010`, target IoC values, evidence package rows, and 7 target event rows.
- Diff hotspots were manually reviewed and are concentrated on text antialiasing, SVG strokes/nodes/rings/icons, and small alpha differences. No business module is missing, overlapped, bottom-bar occluded, or structurally shifted.

## Dynamic Diagram Rule

| Rule Item | Result | Evidence |
|---|---|---|
| Business diagrams cannot be static page resources | pass | `capture-meta-r150-final.json` records no forbidden target/image resource markers |
| API data source | pass | `fetchPageSnapshot(selectedTopic)` and `adaptAptTopic` remain the APT data path |
| Typed fallback boundary | pass | r150 target state uses typed visual-breakdown fallback only under explicit `__codex_ui_breakdown_production=1` |
| Field mapping | pass | `buildAptVisualModel` and `buildAptEvidenceEventRows` map rows/metrics/evidence into campaign nodes, phases, evidence nodes, timeline, IoC rows, response counts and table rows |
| Render components | pass | `AptCanvas`, `AptAnalysisDashboard`, `AptEvidenceTable`, `AptResponsePanel`, `AptRightRail` render dynamic DOM/SVG/CSS visualizations |
| Adaptive strategy | pass | APT page uses CSS grid/minmax layout and no business-page screenshot resource |

## Evidence

| Artifact | Path |
|---|---|
| Target | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/target.png` |
| Regions overlay | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/regions-overlay.png` |
| Production implementation | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/implementation-r150-final.png` |
| Implementation alias | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/implementation.png` |
| Business target crop | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/target-business-r150.png` |
| Business implementation crop | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/implementation-business-r150.png` |
| Side-by-side review | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/target-vs-implementation-business-r150.png` |
| Diff | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/diff.png` |
| Diff archive | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/diff-business-r150.png` |
| Metrics | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/metrics.json` |
| Metrics archive | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/metrics-business-r150.json` |
| Capture meta | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/capture-meta-r150-final.json` |
| Capture alias | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/capture-meta.json` |
| CDP version precheck | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/cdp-version-r150-final-pre-capture.txt` |
| CDP list precheck | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/cdp-list-r150-final-pre-capture.txt` |
| Verification | `evidence/ui-image-breakdowns/pages/topics-apt-campaign/verification.json` |

## Reproduction

1. Check `curl -i --max-time 5 --noproxy '*' http://127.0.0.1:9224/json/version` and `/json/list`.
2. Open the production route through `connectOverCDP("http://127.0.0.1:9224")`; do not use Linux Chrome.
3. Capture `implementation-r150-final.png` on `http://10.0.5.8:30180/topics?topic=apt&tab=apt&__codex_ui_breakdown_production=1`.
4. Crop business region `x=198,y=80,w=1722,h=917`.
5. Generate `diff.png` and `metrics.json` with `--max-pixel-ratio 0.13 --channel-tolerance 90`.
6. Review `implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json`, and `verification.json`.

## Decision

Main-thread judgment is `business-pixel-accepted` for r150. Do not advance the queue based on older r145 artifacts.

## r245 Regression Review

| Check | Result | Evidence |
|---|---|---|
| Dynamic topology and chart rendering | pass | Tunnel and APT relationship graphs, APT trend, and response status render as ECharts canvas from API-contract/typed fallback data. |
| Buttons have observable outcomes | pass | `TopicActionButton` opens a confirmation Drawer with simulated task feedback for tunnel, exfiltration, and APT actions. |
| Evidence tables are usable | pass | Tunnel and APT tables expose controlled page 2; tunnel table has `overflow-y: auto`. |
| Windows Chrome interaction health | pass | `interaction-r245.json`: no bad response, console error, or page error. |
| Business ROI | pass | `metrics-business-r245.json`: `0.10059123258314683 <= 0.125`; business bbox `198,80,1722,917`. |

The r245 production route uses `traffic/web-ui:ui-topic-graphs-20260711-r245`. Screenshot review confirms that the APT relationship graph fills its intended business panel and that no public shell geometry was changed to obtain the result.

## r259 SVG Rollback Review

| Check | Result | Evidence |
|---|---|---|
| API-driven topology SVG | pass | Tunnel and APT relationship panels each render one SVG from page API/typed fallback nodes and links. |
| Non-topology ECharts | pass | Data-exfiltration relation, APT trend and response distribution remain dynamic canvases. |
| Buttons, pagination and overflow | pass | Three topic branches expose confirmation Drawers; tunnel/APT tables reach page 2 and the tunnel table uses `overflow-y: auto`. |
| Windows Chrome runtime | pass | `interaction-r259-topology-svg.json` has no bad response, console error or page error. |
| Business ROI | pass | `metrics-business-r259.json`: `0.09128387903290155 <= 0.125`. |

Main-thread judgment: `business-pixel-accepted`. The current production image is `traffic/web-ui:ui-screen-original-svg-20260711-r259`; r245's ECharts topology statement is historical and superseded for topology-only visuals.
