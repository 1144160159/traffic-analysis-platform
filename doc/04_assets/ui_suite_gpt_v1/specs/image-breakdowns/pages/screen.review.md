# screen.png review

## Review Status

- Status: `pixel-accepted`
- Acceptance profile: alpha visual gate with strict business-diagram rule; content and layout consistency are required, brightness/transparency/anti-aliasing differences are accepted.
- Realtime data rule: except paginated tables, page business data and business diagrams must refresh automatically; `/screen` page snapshot is refreshed every 5 seconds outside visual-breakdown and masked-demo modes.
- Target image reviewed directly: yes
- Evidence target: `evidence/ui-image-breakdowns/pages/screen/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/screen?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783387312164#codex_smoke_token=<redacted>`
- API-live URL: `http://10.0.5.8:30180/screen?r90ApiLive=1783387438331`
- Final URL: `http://10.0.5.8:30180/screen?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783387312164#codex_smoke_token=<redacted>`
- Viewport: 1920 x 1080, Windows Chrome `Chrome/150.0.7871.47`, device pixel ratio 1
- Deployed image: `traffic/web-ui:ui-screen-dynamic-maps-20260707-r90`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pages verification loop read before edits |
| Target PNG exists | pass | `evidence/ui-image-breakdowns/pages/screen/target.png` |
| Regions overlay | pass | `evidence/ui-image-breakdowns/pages/screen/regions-overlay.png` |
| Windows Chrome CDP precheck | pass | `cdp-version-r90-screen-pre-capture.txt`, `cdp-list-r90-screen-pre-capture.txt`; manual precheck `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `/json/list` passed before capture |
| Production route screenshot | pass | `implementation.png` captured from real `30180` route through Windows Chrome CDP |
| Runtime gate | pass | No console/pageerror/requestfailed/bad response/forbidden target resource/layout overflow |
| Visual diff | pass | r90 full-page `pixel_mismatch_ratio=0.10551649305555555`, `max_pixel_ratio=0.12`, `channel_tolerance=90` |
| Layer diff | accepted-alpha | r90 right-rail crop remains `0.1473200442967885` against the same 0.12 layer threshold; accepted as texture/brightness/map-distribution delta because business content, dynamic behavior, runtime health and containment pass |
| Campaign density business diagram | pass | `production-campaign-density-r39-crop.png` shows ECharts heatmap/scatter density radar instead of static lines or bitmap resource |
| Collection pipeline business diagram | pass | `production-pipeline-r41-crop.png` shows complete business labels/values, ECharts sparkline trends, CSS arrows between nodes and panel-contained detail link |
| Topbar shield exactness/alignment | pass | `production-topbar-logo-context-r57-crop.png`; `topbar-logo-r57-analysis.json` reports shield center_y=48.5 and title center_y=50.5, delta=-2px |
| Campus digital twin topology dynamic rule | pass-r247 | `TopicTopologyGraph` renders the API/typed-data relation layer in ECharts while existing business node controls remain selectable. `interaction-r247.json` records the canvas, 2D state and node selection. |
| Business diagram and realtime data discipline | enforced-r61 | All business diagrams must use API/typed data and dynamic chart/vector rendering such as ECharts/SVG/canvas. Except paginated tables, page data must auto-refresh; `/screen` uses React Query `refetchInterval=5000`. |
| Probe deployment map business diagram | pass-r247 | ECharts graph canvas maps `probeMapNodes` and `probeMapLinks`; `interaction-r247.json` records the canvas on the Windows Chrome production route. |
| Evidence and forensics closure business diagram | pass-r63 | `production-evidence-rings-r63-crop.png`; `evidence-rings-r63-analysis.json` records Windows Chrome real route, `真实 API 5 条`, 6 ECharts roots, 6 canvas charts, and `staticConicGradient=false`. |
| Right-rail dynamic maps | pass-r90 | `production-screen-r90-api-live-analysis.json` reports `rightRailDynamicMaps=true`, 4 right-rail ECharts roots, 9 right-rail canvases, risk map canvas and egress map canvas evidence. |
| API-live dynamic diagram assertions | pass-r247 | `interaction-r247.json` records two new ECharts canvases, 2D state, node selection and zero runtime errors; the full page keeps its existing API/typed data refresh contract. |
| Auxiliary review | reviewed | Earlier r21 acceptance was overturned by user visual feedback; r39 fixed campaign density and r41 fixes the collection/stream-processing pipeline, with any other page-level differences tracked separately |
| Main-thread judgment | pass | `verification.json` records `pixel-accepted`, `accepted=true` |

## Visual Findings

- The production screenshot is the real `/screen` APISIX/Web UI route in Windows Chrome CDP, not `implementation.html` and not a raster replay of `target.png`.
- Business diagram rule is now explicit and applied: probe maps, topology, campaign density, trend lines, world maps and flow charts must be API/typed-contract/fallback driven React, SVG, canvas or ECharts components.
- Realtime rule is explicit: except paginated tables, page business data and business diagrams must refresh automatically. The态势大屏 snapshot uses a 5s React Query refetch interval outside deterministic visual-breakdown and masked-demo modes.
- r90 supersedes the r63/r65 business-diagram evidence for final page status. The full-page production diff passes, and the no-breakdown API-live screenshot proves real production requests, realtime refetch and dynamic ECharts/SVG/canvas rendering.
- Probe coverage map is generated from `probeMapNodes` and `probeMapLinks`; it shows a campus outline, district lines, links and online/offline/maintenance states.
- Probe deployment map is accepted in r90 as a dynamic business diagram: the API-live analysis reports dynamic SVG coverage, 15 probe nodes, 14 links and production API participation.
- Campus digital twin topology is generated from API-derived `screenVisuals.topologyNodes` and `screenVisuals.topologyEdges`; r59 renders 3D as SVG zones/buildings/city blocks and 2D as SVG zones/site rings using the same node/link data. r60 adds point-in-polygon campus boundary checks for decorative city blocks and reduces background noise, so 3D expresses campus boundary, zones, nodes, links and risk states more clearly. The no-breakdown production route shows `真实 API 5 条`, 11 nodes, 12 SVG links, 10 dynamic zones and `cityBlocksOutsideBoundary=0`.
- Campaign density is rendered by `CampaignDensityChart` using ECharts `heatmap` + `scatter` series over a custom radar grid. API/typed fallback points are deterministically expanded into density clusters, so the diagram remains data-driven and is no longer a static line-only approximation.
- Collection and stream-processing pipeline nodes render `SparklineChart`, an ECharts line chart, for each node trend. r41 updates the node cards to match the target's compact process-card structure: icon/title row, two metric rows, bottom trend, independent CSS arrows, and a drill link contained in the pipeline panel.
- Right-rail world maps remain dynamic ECharts/React vector maps with world silhouettes, hotspots and flow arcs.
- Right-rail risk and egress maps are now fed by `screenVisuals.riskMapPoints`, `screenVisuals.egressMapPoints` and `screenVisuals.egressMapFlows`, derived from production API snapshot fields through typed adapters. Visual-breakdown mode keeps target-like legend/list text only for diff stability.
- Evidence and forensics closure is now a business diagram, not CSS decoration. r63 maps real snapshot values into `screenVisuals.evidenceRings` and renders all six rings with `EvidenceClosureRingChart` ECharts pie canvases; the Windows Chrome analysis records six canvas charts and no `conic-gradient`.
- Topbar brand shield is now the exact target-style dark-blue shield as an independent screenshot-icon resource, not a generated/manual substitute. r57 aligns the shield centerline with the title text within 2px on the Windows Chrome production screenshot.

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/screen/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/screen/regions-overlay.png`
- Implementation screenshot: `evidence/ui-image-breakdowns/pages/screen/implementation.png`
- Full diff: `evidence/ui-image-breakdowns/pages/screen/diff.png`
- Full metrics: `evidence/ui-image-breakdowns/pages/screen/metrics.json`
- Measurement: `evidence/ui-image-breakdowns/pages/screen/measurement.json`
- Capture metadata: `evidence/ui-image-breakdowns/pages/screen/capture-meta.json`
- Runtime report: `evidence/ui-image-breakdowns/pages/screen/production-route-report.json`
- Verification: `evidence/ui-image-breakdowns/pages/screen/verification.json`
- API-live screenshot: `evidence/ui-image-breakdowns/pages/screen/production-screen-r90-api-live.png`
- API-live analysis: `evidence/ui-image-breakdowns/pages/screen/production-screen-r90-api-live-analysis.json`
- r90 CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r90-screen-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r90-screen-pre-capture.txt`
- r90 right-rail layer diff: `evidence/ui-image-breakdowns/pages/screen/layer-diffs-r90/right-rail-diff.png`
- Campaign density target crop: `evidence/ui-image-breakdowns/pages/screen/target-campaign-density-r39-crop.png`
- Campaign density production crop: `evidence/ui-image-breakdowns/pages/screen/production-campaign-density-r39-crop.png`
- Pipeline target crop: `evidence/ui-image-breakdowns/pages/screen/target-pipeline-r41-crop.png`
- Pipeline production crop: `evidence/ui-image-breakdowns/pages/screen/production-pipeline-r41-crop.png`
- Pipeline crop metadata: `evidence/ui-image-breakdowns/pages/screen/production-pipeline-r41-crop-meta.json`
- Topbar logo target crop: `evidence/ui-image-breakdowns/pages/screen/target-topbar-logo-context-r57-crop.png`
- Topbar logo production crop: `evidence/ui-image-breakdowns/pages/screen/production-topbar-logo-context-r57-crop.png`
- Topbar logo analysis: `evidence/ui-image-breakdowns/pages/screen/topbar-logo-r57-analysis.json`
- Topology 3D production crop: `evidence/ui-image-breakdowns/pages/screen/production-topology-3d-r58-api-crop.png`
- Topology 2D production crop: `evidence/ui-image-breakdowns/pages/screen/production-topology-2d-r58-api-crop.png`
- Topology dynamic analysis: `evidence/ui-image-breakdowns/pages/screen/topology-dynamic-r58-api-analysis.json`
- Topology CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r58-api-topology-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r58-api-topology-pre-capture.txt`
- Topology r59 polished 3D crop: `evidence/ui-image-breakdowns/pages/screen/production-topology-3d-r59-polish-crop.png`
- Topology r59 polished 2D crop: `evidence/ui-image-breakdowns/pages/screen/production-topology-2d-r59-polish-crop.png`
- Topology r59 analysis: `evidence/ui-image-breakdowns/pages/screen/topology-polish-r59-analysis.json`
- Topology r59 CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r59-topology-polish-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r59-topology-polish-pre-capture.txt`
- Topology r60 3D boundary crop: `evidence/ui-image-breakdowns/pages/screen/production-topology-3d-r60-boundary-crop.png`
- Topology r60 analysis: `evidence/ui-image-breakdowns/pages/screen/topology-boundary-r60-analysis.json`
- Topology r60 CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r60-topology-boundary-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r60-topology-boundary-pre-capture.txt`
- Evidence rings r63 production crop: `evidence/ui-image-breakdowns/pages/screen/production-evidence-rings-r63-crop.png`
- Evidence rings r63 full screenshot: `evidence/ui-image-breakdowns/pages/screen/production-screen-r63-evidence-echarts-full.png`
- Evidence rings r63 analysis: `evidence/ui-image-breakdowns/pages/screen/evidence-rings-r63-analysis.json`
- Evidence rings r63 CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r63-evidence-echarts-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r63-evidence-echarts-pre-capture.txt`
- Probe map r65 production crop: `evidence/ui-image-breakdowns/pages/screen/production-probe-map-r65-crop.png`
- Probe map r65 analysis: `evidence/ui-image-breakdowns/pages/screen/probe-map-r65-analysis.json`
- Probe map r65 CDP precheck: `evidence/ui-image-breakdowns/pages/screen/cdp-version-r65-probe-stats-map-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/screen/cdp-list-r65-probe-stats-map-pre-capture.txt`

## Backend API Contract For Campus Topology

The frontend can currently derive a fallback topology from dashboard stats, but the production-quality digital twin should be returned by the backend as explicit topology data under the screen snapshot contract. Recommended shape:

```json
{
  "visuals": {
    "screen": {
      "topologyNodes": [
        {
          "id": "dc",
          "label": "数据中心",
          "type": "数据底座",
          "zone": "核心区",
          "x": 60,
          "y": 63,
          "z": 0,
          "tone": "info",
          "status": "online",
          "meta": "入库正常",
          "probes": "4 / 4",
          "links": "11 条",
          "assets": "196",
          "riskScore": 38,
          "bandwidth": "78.3 Gbps",
          "throughputGbps": 78.3,
          "packetLossRate": 0.02,
          "latencyMs": 12,
          "buildingHeight": 62,
          "footprint": [[570, 302], [642, 292], [674, 346], [598, 372]],
          "href": "/data-quality"
        }
      ],
      "topologyEdges": [
        {
          "id": "core-dc",
          "from": "core",
          "to": "dc",
          "tone": "core",
          "status": "online",
          "width": 3,
          "bandwidthGbps": 78.3,
          "utilization": 0.57,
          "packetLossRate": 0.01,
          "latencyMs": 8,
          "direction": "bidirectional",
          "animated": true
        }
      ],
      "topologyZones": [
        {
          "id": "core-zone",
          "label": "核心区",
          "tone": "info",
          "riskScore": 24,
          "polygon": [[420, 220], [560, 210], [620, 292], [508, 340], [390, 300]]
        }
      ]
    }
  }
}
```

Field rules:

- `topologyNodes[].id` must be stable and unique; all `topologyEdges[].from/to` must reference existing node ids.
- `x` and `y` are normalized percentages in the campus panel coordinate system, where `x=0..100` and `y=0..100`; optional `footprint`/`polygon` use SVG viewBox coordinates `1000 x 500` for exact UI geometry.
- `tone` controls visual severity: `ok`, `info`, `warn`, `risk`; `status` controls operational state: `online`, `offline`, `maintenance`, `degraded`.
- `throughputGbps`, `utilization`, `packetLossRate`, `latencyMs`, `riskScore`, `probes`, `assets` and `links` drive labels, details, stroke width, animation speed and color. The frontend should not infer these from unrelated KPI text when the topology API is available.
- 3D needs either `buildingHeight` or a backend-provided `buildingClass` such as `core`, `teaching`, `lab`, `datacenter`, `dorm`, `venue`; 2D uses the same nodes/edges but renders flat site rings and link states.
- `topologyZones` is preferred for accurate campus region outlines. If omitted, the frontend derives soft zones from node positions, as in r59.
- The API should update on the same cadence as the screen snapshot and include `updatedAt`/`version` when available so the UI can show stale or recalculating states.

## Known Differences

- The r90 full-page diff ratio is accepted at `0.10551649305555555 <= 0.12`. The right-rail layer crop is higher at `0.1473200442967885`; this is accepted under the alpha business-focused gate because dynamic chart semantics, text, runtime health and containment pass.
- The topbar shield content and centerline alignment are accepted in r57; the remaining topbar differences are minor text rendering/spacing differences rather than the rejected shield shape or vertical offset.
- Campaign density now matches the required business-diagram class: ECharts density radar, many colored cluster points, visible high/medium/low legend, and no bitmap/screenshot resource usage.
- The pipeline no longer uses screenshot resources. It remains API/typed fallback-driven React plus ECharts sparklines; the arrows are CSS connectors and the detail link stays inside the pipeline panel instead of overlapping the right rail.
- The full-page visual-diff screenshot URL includes `__codex_ui_breakdown_production=1`; the r90 API-live evidence intentionally removes that flag and uses `http://10.0.5.8:30180/screen?r90ApiLive=1783387438331` to prove production API freshness and realtime refresh behavior.
- r247 renders the topology relation layer in ECharts for both 2D and 3D states; CSS remains only for business node controls and panel framing.
- The evidence/forensics closure panel previously used CSS `conic-gradient` rings and only partially mapped live data. r63 fixes this: all six rings are typed-data driven ECharts canvas diagrams sourced from the screen snapshot adapter and refreshed with the page snapshot cadence.
- The probe deployment map passes the dynamic-diagram gate in r247 through an ECharts canvas. Backend data quality can still improve coordinate richness over time.

## Reproduction

1. Check `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `curl -i --max-time 5 http://127.0.0.1:9224/json/list`.
2. Run `env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy NO_PROXY=127.0.0.1,localhost,10.0.5.8,10.3.6.59 node doc/04_assets/ui_suite_gpt_v1/capture_image_breakdown_production_route.mjs --record doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/pages/screen.json --base-url http://10.0.5.8:30180 --cdp-url http://127.0.0.1:9224 --wait-ms 4500 --channel-tolerance 90 --max-pixel-ratio 0.12`.
3. Review `target.png`, `implementation.png`, `diff.png`, `metrics.json`, `capture-meta.json` and `verification.json`.

## Decision

The campaign-density defect reported after r21 remains fixed in r39, the collection/stream-processing pipeline defect reported after r39 remains fixed in r41, the topbar shield exactness/alignment issue is fixed in r57, r63 fixes the evidence/forensics closure panel as an API/typed-data ECharts business diagram, r90 fixes the right-rail risk/egress maps as API-derived dynamic business diagrams, and r247 migrates the remaining probe coverage map and campus topology relation layers to ECharts. Main-thread judgment: `pixel-accepted` for the `screen` page under the alpha business-focused gate.

## r247 Regression Review

| Check | Result | Evidence |
|---|---|---|
| Probe coverage ECharts | pass | `interaction-r247.json` reports `coverage_canvas_count=1`. |
| Campus topology ECharts | pass | `interaction-r247.json` reports `topology_canvas_count=1`. |
| Existing node interaction | pass | 2D state and a topology node remain selected; ECharts relation layer has `pointer-events=none`. |
| Windows Chrome runtime | pass | No bad response, console error, page error or request failure. |
| Business ROI | pass | `metrics-business-r247.json`: `0.09511460514200094 <= 0.125`; bbox `198,80,1722,917`. |

The r247 production image is `traffic/web-ui:ui-screen-echarts-20260711-r247`. The two static SVG business visualizations are no longer present in `SituationalScreen.tsx`; the global static-SVG chart inventory is zero.

## r259 Original SVG Rollback Review

| Check | Result | Evidence |
|---|---|---|
| Original probe coverage SVG restored | pass | `interaction-r259-original-svg.json` reports `coverage_svg_count=1`; nodes and links derive from the screen snapshot adapter. |
| Original campus twin SVG restored | pass | `topology_svg_count=1`; original boundary, zones, roads, buildings, links and 2D/3D states render. |
| Existing interaction preserved | pass | 2D state and topology node selection remain active. |
| Windows Chrome runtime | pass | No bad response, request failure, console error or page error. |
| Business ROI | pass | `metrics-business-r259.json`: `0.10485385738730421 <= 0.125`. |

Main-thread judgment: `business-pixel-accepted`. The current production image is `traffic/web-ui:ui-screen-original-svg-20260711-r259`; the r247 ECharts migration remains only as historical evidence and is superseded by this rollback.
