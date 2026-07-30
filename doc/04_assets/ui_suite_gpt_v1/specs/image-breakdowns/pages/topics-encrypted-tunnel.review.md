# topics-encrypted-tunnel.png review

## Review Status

- Status: `business-accepted`
- Acceptance profile: business-region visual gate. Public topbar/sidebar/bottombar are not modified and are not the pass/fail focus for this review.
- Target image reviewed directly: yes
- Evidence target: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/topics?topic=tunnel&tab=tunnel&__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783397071409`
- Viewport: 1920 x 1080, device scale factor 1
- Deployed image: `traffic/web-ui:ui-topic-business-rebuild-20260707-r98`

## Business Semantics

| Area | Business meaning | Implementation rule |
|---|---|---|
| 协议分析 | Explains tunnel protocol composition: SSH/TLS/HTTPS/RDP/SOCKS share, percentage and traffic. | Dynamic ECharts pie plus data rows from API/fallback data. |
| 隧道源 TOP5 | Ranks internal source IPs that generate the highest suspicious tunnel traffic. | Dynamic ECharts horizontal bar; values are Gbps by source IP. |
| 端点国家 / ASN TOP5 | Shows where tunnels terminate externally: country/region, endpoint count, ASN and traffic; it is destination attribution, not source ranking. | Dynamic ECharts bar plus detail table driven by API/fallback data, not screenshots. |

These three areas must not be merged into one generic ranking. They answer different questions: protocol type, internal source, and external destination.

## Checks

| Check | Result | Evidence |
|---|---|---|
| Windows Chrome CDP precheck | pass | `cdp-version-r98-pre-capture.txt`, `cdp-list-r98-pre-capture.txt` |
| Windows Chrome recovery | pass | Recovered `CodexChromeCDP` on Windows host and rebuilt the 9224 SSH tunnel before final capture |
| Production route screenshot | pass | `implementation-r98-business-semantics.png` captured from real `30180` route through Windows Chrome CDP |
| Runtime gate | pass | `capture-meta-r98-business-semantics.json`: no console errors, no failed requests, no forbidden target image resources |
| Business top rule | pass | Business area shows `专题面板` plus topic tabs and edit/save actions; no breadcrumb path is used |
| Dynamic business diagrams | pass | Tunnel topology is React/SVG data nodes; protocol/source/trend/endpoint-country charts use ECharts; no business screenshot resource is used |
| Protocol/source/destination separation | pass | `capture-meta-r98-business-semantics.json` flags `protocolPanel`, `sourceTop5`, `endpointCountry` and `endpointCountryChart` all true; `echartCanvasCount=4` |
| Content match | pass | KPI values, tunnel table rows, report readiness and evidence package values match the UI image state |
| Business visual diff | pass | `pixel_mismatch_ratio=0.10923427274465922`, `max_pixel_ratio=0.12`, `channel_tolerance=90` |
| Main-thread judgment | pass | Business region accepted; full-screen diff is retained as reference only because public areas are out of scope for this review |

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/regions-overlay.png`
- Implementation screenshot: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/implementation.png`
- R98 production screenshot: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/implementation-r98-business-semantics.png`
- Business target crop: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/target-business-r98.png`
- Business implementation crop: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/implementation-business-r98.png`
- Business diff: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/diff.png`
- Business metrics: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/metrics.json`
- Capture metadata: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/capture-meta.json`
- Measurement: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/measurement.json`
- Verification: `evidence/ui-image-breakdowns/pages/topics-encrypted-tunnel/verification.json`

## Findings

- The previous topic page was structurally too generic. R97 rebuilds the business region as a dense topic workbench matching the UI image: facts row, nine KPI tiles, tunnel impact topology, encrypted tunnel analysis matrix, event/evidence table and right delivery rail.
- The three tunnel-analysis subareas now have distinct business roles: protocol composition, internal source ranking and external country/ASN destination attribution.
- R98 adds a dedicated endpoint-country ECharts bar chart next to the ASN detail table, so the endpoint module is no longer just text/table and cannot be confused with tunnel source TOP5.
- Business diagrams are deterministic React/SVG/ECharts components based on API/fallback data. No UI target PNG, full panel screenshot or business diagram screenshot is requested by the page.
- The right rail begins at the business content top and contains delivery summary, evidence package completeness, report preview and topic actions.
- The remaining pixel difference is mainly fine typography, icon glyphs and small spacing. Under the current business-only review rule and relaxed 0.12 threshold, the business region passes.

## Reproduction

1. Check Windows Chrome CDP with `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `/json/list`.
2. Open `http://10.0.5.8:30180/topics?topic=tunnel&tab=tunnel&__codex_ui_breakdown_production=1` through Windows Chrome CDP.
3. Capture at 1920 x 1080 and save `implementation-r98-business-semantics.png`.
4. Crop business content to `x=198,y=80,w=1722,h=917`, then generate `diff.png` and `metrics.json` with `channel_tolerance=90`, `max_pixel_ratio=0.12`.

## Decision

The `topics-encrypted-tunnel` business region is accepted for r98. Continue the queue with the next topic page while preserving the rule that public regions are not modified and business diagrams must be API-driven.

## topic-panel-r833 Database-backed Rebuild

- Source and implementation were compared together at 1920×1080/DPR 1 in Windows Chrome 150 through Xshell CDP `9224 -> 9222`.
- PostgreSQL supplies the page profile, nine KPI values, charts, 128 evidence events, delivery summary, evidence bundle and report preview. The API marks the response as `data_mode=simulated` and preserves ClickHouse fallback.
- Layout/fullscreen, three analysis tabs, three filters, search, compact pagination, report preview, row actions and all right-rail governance actions are functional; persistent actions call the topic backend.
- Final Windows Chrome run exercised 75 controls on this Tab, including 20 visible row actions, three filters, search and compact pagination; all backend actions returned their expected 200/201/202 status.
- Same-input evidence: `comparison-topic-panel-r833.png` and `comparison-business-topic-panel-r833.png`; implementation: `implementation-topic-panel-r833-live.png`.
- Full-frame mismatch ratio is `0.1078231096`; normalized business ROI is `0.1120169162`, both `<= 0.125`. P0/P1/P2=0. Result: pass.

## topic-panel-r839 Responsive and Business Completion

- All three topic canvases now share the API-driven `TopicTopologyGraph` ECharts graph. Solid edges are confirmed relations; dashed edges are inferred/impact relations. Pan, wheel zoom, explicit zoom and fit controls are enabled.
- Protocol family, high-risk user and fingerprint evidence cards switch to three distinct API-derived analysis states.
- 1920×1080, 1600×900 and 1366×768 Windows Chrome captures have no document overflow, left/right collision or clipped rail text. The r839 terminal cascade prevents generic responsive rules from collapsing the graph host.
- Report preview calls the report export API and renders the returned artifact in a centered Modal. Row actions use centered confirmation Modals and persist a completed `business_effect`, rather than opening a right Drawer.
- Full-frame mismatch is `0.1107653356`; normalized business ROI is `0.1166468449`, both `<= 0.125`. The final three-topic run is `158/158`; P0/P1/P2=0. Result: pass.
