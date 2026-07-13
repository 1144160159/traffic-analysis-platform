# dashboard.png review

## Review Status

- Status: `pixel-accepted`
- Acceptance profile: alpha visual gate; content and layout consistency are required, brightness/transparency/anti-aliasing differences are accepted.
- Latest business/adaptive review: `pass` on 2026-07-07 r88.
- Latest production image: `traffic/web-ui:ui-dashboard-stage-fit-20260707-r88`
- Latest production URL: `http://10.0.5.8:30180/dashboard?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783386069441`
- Latest API-live URL: `http://10.0.5.8:30180/dashboard?r88ApiLive=1783386123344`
- Latest Windows Chrome coverage: standard production diff at `1920x1080` DPR 1, plus API-live production route at `1920x1080` DPR 1; r86 five-window adaptive evidence remains as historical narrow-window coverage.
- Latest evidence screenshots: `evidence/ui-image-breakdowns/pages/dashboard/implementation.png`, `evidence/ui-image-breakdowns/pages/dashboard/production-dashboard-r88-api-live.png`
- Latest analysis: `evidence/ui-image-breakdowns/pages/dashboard/production-dashboard-r88-api-live-analysis.json`
- Target image reviewed directly: yes
- Evidence target: `evidence/ui-image-breakdowns/pages/dashboard/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/dashboard?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783386069441`
- Final URL: `http://10.0.5.8:30180/dashboard?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783386069441`
- Viewport: 1920 x 1080, device scale factor 1
- Deployed image: `traffic/web-ui:ui-dashboard-stage-fit-20260707-r88`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pages verification loop read before edits |
| Menu-order queue | pass | `dashboard` is queue item 2 after `login` |
| Target PNG exists | pass | `evidence/ui-image-breakdowns/pages/dashboard/target.png` |
| Regions overlay | pass | `evidence/ui-image-breakdowns/pages/dashboard/regions-overlay.png` |
| Windows Chrome CDP precheck | pass | `cdp-version-r8-production-restart-pre-capture.txt`, `cdp-list-r8-production-restart-pre-capture.txt` |
| Production route screenshot | pass | `implementation.png` captured from real `30180` route through Windows Chrome CDP |
| Runtime gate | pass | No console/pageerror/requestfailed/bad response/forbidden target resource/overflow blocker |
| Visual diff | pass | `pixel_mismatch_ratio=0.09903018904320987`, `max_pixel_ratio=0.12`, `channel_tolerance=88` |
| Main-thread judgment | pass | `verification.json` records `pixel-accepted`, `accepted=true` |
| r84 Windows Chrome CDP precheck | pass | `cdp-version-r84-dashboard-optimise-pre-capture.txt`, `cdp-list-r84-dashboard-optimise-pre-capture.txt` |
| r84 production screenshot | pass | `production-dashboard-r84-optimise-full.png` captured from real `30180` route through Windows Chrome CDP |
| r84 runtime gate | pass | No console/pageerror/requestfailed/bad response |
| r84 business dynamic gate | pass | 20 ECharts canvas elements and `/api/v1/dashboard/*` requests observed |
| r84 adaptive layout gate | pass | No horizontal overflow, no panel child overflow, stage charts and quality rings stay inside cards |
| r84 hover/ellipsis gate | pass | Deficit list, health gate, KPI/queue compact labels expose full text with `title`/tooltip behavior |
| r86 Windows Chrome CDP precheck | pass | `cdp-version-r86-dashboard-adaptive-pre-capture.txt`, `cdp-list-r86-dashboard-adaptive-pre-capture.txt` |
| r86 browser-window adaptive screenshots | pass | Five real Windows Chrome window sizes captured from `30180` production route |
| r86 no horizontal overflow | pass | `rootOverflowX=0`, `mainOverflowX=0`, measured business overflow count `0` in all five windows |
| r86 no business panel overlap | pass | Panel overlap count `0` in all five windows |
| r86 business dynamic gate | pass | 20 dashboard ECharts canvas elements and `/api/v1/dashboard/*` responses observed |
| r86 hover/ellipsis gate | pass | Truncated business text exposes full text through `title` or accessible label |
| r88 Windows Chrome CDP precheck | pass | `cdp-version-r88-dashboard-stage-fit-pre-capture.txt`, `cdp-list-r88-dashboard-stage-fit-pre-capture.txt`, `cdp-version-r88-dashboard-api-live-pre-capture.txt`, `cdp-list-r88-dashboard-api-live-pre-capture.txt` |
| r88 production screenshot | pass | `implementation.png` captured from real `30180` route through Windows Chrome CDP |
| r88 visual diff | pass | `pixel_mismatch_ratio=0.10437210648148149`, `max_pixel_ratio=0.12`, `channel_tolerance=90` |
| r88 layer measurement | pass | `measurement.json` reports `failed_layer_count=0`, `layer_count=5` |
| r88 API-live dynamic gate | pass | `production-dashboard-r88-api-live-analysis.json` records 7 `/api/v1/*` requests, all 200, and 20 ECharts canvas elements |
| r88 stage work-basket fit | pass | API-live metrics: `mainVerticalOverflow=0`, `stagePanelAboveBottomBar=true`, `stageCardsInsidePanel=true`, `stageChartsInsideCards=true`, `stageSmallVisible=true`, `stageCardsAboveBottomBar=true` |

## Visual Findings

- The production screenshot is the real APISIX/Web UI route, not `implementation.html` or target-raster replay.
- r84 fixed the 1536px Windows Chrome viewport overflow: the priority queue table no longer bleeds into the health gate panel.
- r86 closes the later browser-window adaptation gap: the dashboard now reflows from a two-column business grid to a single-column business stack across narrower Windows Chrome windows instead of relying on a fixed table width or horizontal scrolling.
- r86 specifically fixes the `1600x900` window regression where the queue table head/row exceeded the 2-column panel by about 16-17px.
- r88 extends the height fix from visual breakdown mode to the real dashboard route: at `1920x1080`, the API-live page has no main vertical overflow and the fixed bottom bar no longer covers the stage work-basket.
- r88 confirms the stage work-basket is still ECharts/API-driven rather than bitmap-based: the API-live capture observes `/api/v1/dashboard/stats`, `/api/v1/dashboard/alerts/trend`, `/api/v1/dashboard/attack-phases` and 20 canvas elements.
- r84 fixes the deficit rail regression: `真实 API` is no longer displayed as a yesterday delta; compact actions remain inside the right rail and expose full action text on hover.
- r84 keeps the dashboard business graphics data-driven: KPI sparklines, stage work-basket bars, quality rings, the progress ring and Top Talkers render through ECharts/API-backed data rather than bitmap UI resources.
- Dashboard content matches the alpha target: top KPI strip, priority queue, health gate matrix, right deficit/action rail, stage basket with note, evidence quality rings, Top Talkers, sidebar, topbar and bottombar are present and aligned at 1920 x 1080.
- The overview sidebar includes `专题面板`; this closes the earlier hidden-submenu regression for the overview group.
- Visual breakdown mode now bypasses `/api/v1/auth/me`, so dashboard runtime evidence has no auth 401/bad-response pollution.
- Remaining differences are accepted under the user-approved alpha rule: brightness, transparency, glow intensity, font anti-aliasing and minor icon rendering do not block this page.

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/dashboard/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/dashboard/regions-overlay.png`
- Implementation screenshot: `evidence/ui-image-breakdowns/pages/dashboard/implementation.png`
- Window adaptive screenshots: `evidence/ui-image-breakdowns/pages/dashboard/production-dashboard-r86-window-adaptive-1920x1080.png`, `production-dashboard-r86-window-adaptive-1600x900.png`, `production-dashboard-r86-window-adaptive-1366x768.png`, `production-dashboard-r86-window-adaptive-1200x760.png`, `production-dashboard-r86-window-adaptive-1024x768.png`
- API-live screenshot: `evidence/ui-image-breakdowns/pages/dashboard/production-dashboard-r88-api-live.png`
- API-live analysis: `evidence/ui-image-breakdowns/pages/dashboard/production-dashboard-r88-api-live-analysis.json`
- Full diff: `evidence/ui-image-breakdowns/pages/dashboard/diff.png`
- Full metrics: `evidence/ui-image-breakdowns/pages/dashboard/metrics.json`
- Measurement: `evidence/ui-image-breakdowns/pages/dashboard/measurement.json`
- Capture metadata: `evidence/ui-image-breakdowns/pages/dashboard/capture-meta.json`
- Runtime report: `evidence/ui-image-breakdowns/pages/dashboard/production-route-report.json`
- Verification: `evidence/ui-image-breakdowns/pages/dashboard/verification.json`

## Reproduction

1. Check `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `curl -i --max-time 5 http://127.0.0.1:9224/json/list`.
2. For standard visual diff, connect with Playwright `connectOverCDP('http://127.0.0.1:9224')` and open `http://10.0.5.8:30180/dashboard?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783386069441`.
3. For API-live dynamic verification, open `http://10.0.5.8:30180/dashboard?r88ApiLive=1783386123344` with a temporary smoke token, then verify `/api/v1/dashboard/*` requests, ECharts canvases and stage work-basket layout in `production-dashboard-r88-api-live-analysis.json`.
4. Review `target.png`, `implementation.png`, `diff.png`, `metrics.json`, `measurement.json`, `capture-meta.json` and `verification.json`.

## Decision

The dashboard page is accepted for the current alpha visual gate, API-live dynamic-chart gate and stage work-basket self-adaptive gate. Continue the pages queue with `screen` only after no new dashboard-specific blocker is raised.

## r260 Interaction Regression Review

| Check | Result | Evidence |
|---|---|---|
| Dynamic business charts | pass | `interaction-r260.json` reports 20 ECharts canvases from dashboard API/typed fallback data. |
| Buttons have observable outcomes | pass | The deficit actions open a confirmation Drawer with endpoint, audit event and success feedback; static inventory reports zero passive controls for this page. |
| API reservation | pass | `pageApiPlans.dashboard.actions` defines task endpoints, scopes, audit events, default bodies and guardrails. |
| Queue pagination | pass | Page 2 is active with eight visible rows. |
| Windows Chrome runtime | pass | No bad response, request failure, console error or page error. |
| Business ROI | pass | `metrics-business-r260.json`: `0.11076111695841993 <= 0.125`. |

Main-thread judgment: `business-pixel-accepted`. The current production image is `traffic/web-ui:ui-dashboard-interactions-20260711-r260`; independent auxiliary review is appended after the active review batch returns.
