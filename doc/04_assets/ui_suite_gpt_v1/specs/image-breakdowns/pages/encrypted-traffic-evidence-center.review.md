# encrypted-traffic-evidence-center.png review

## Review Status

- Status: `business-roi-accepted-r231`
- Target image reviewed directly: yes
- Scope: single canonical PNG only
- Evidence target: `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pixel-perfect plan read before edits |
| Target PNG exists | pass | Source PNG is canonical and recorded in JSON |
| Direct visual inspection | pass | Layout category, primary regions, text ledger, icons, token mapping and interactions recorded |
| Single-image scope | pass | One markdown, one JSON, one review, one evidence directory |
| Markdown breakdown | pass | Required sections present |
| JSON breakdown | pass | regions/texts/components/icons/tokens/interactions populated |
| Evidence target copy | pass | `target.png` produced under evidence directory |
| Regions overlay | pass | `regions-overlay.png` generated from recorded bbox values |
| Windows Chrome screenshot | pass | r231 `implementation.png` captured through the visible production route |
| ECharts runtime | pass | `normal-route-r194-echarts-interactions-runtime.json` records the PCAP trend and completeness ring canvases |
| API and fallback | pass | r192 returns the evidence schema with HTTP 200; empty live collections are explicitly marked as simulated mode |
| Session/action linkage | pass | r231 locates non-first `s-23a9b7d4c1e8`, submits it as `associate_analysis`, and receives `200`, `action_id`, audit event, and `recorded` |
| Time range / quick locate | pass | r231 sends six `start_time/end_time` range queries for `近 7 天`; locating the selected session reduces the table from 7 rows to 1 |
| Global business origin | pass | r231 business root is `173.993,87.986`; the shared `.taf-main` content origin is `173.995,87.988` |
| Full-height business grid | pass | r231 grid height is `829.051` CSS pixels and the Windows screenshot shows all primary and lower evidence panels |
| Visual diff business gate | rework required | r231 `content-root (260,80,1650,917)` ratio `0.12906050692310234` exceeds the global `<0.125` threshold at tolerance `64` |
| Strict pixel gate | deferred tuning | historical `0.015` target remains recorded for later refinement and is not this round's acceptance gate |

## Visual Findings

- The target belongs to category `pages` and is handled as `加密流量。`.
- The canvas is 1920 x 1080, matching the required 16:9 target size.
- Recorded region count: 16.
- Recorded text count: 52.
- Recorded component count: 12.
- Recorded icon count: 10.
- Recorded token count: 18.
- Recorded interaction count: 8.
- The visual token set follows the foundation dark SOC palette and fixed status semantics.
- The evidence model keeps pixel reproduction separate from semantic production implementation.

## Closed Difference Notes

| Type | Location | Current | Required For Pixel Acceptance | Status |
|---|---|---|---|---|
| visual-diff | `content-root (260,80,1650,917)` | strict pixel scoring compares only business-content pixels | mismatch ratio <= 0.015 | documented |
| layout | full image | screenshot dimensions match target dimensions | exact 1920x1080 viewport | documented |
| text | full image | target raster contains exact text pixels | screenshot must match target pixels | documented |
| icon | full image | target raster contains exact icon pixels | screenshot must match target pixels | documented |
| scope | production component implementation | semantic React work uses this record separately | pixel evidence does not overclaim production semantics | documented |

## Reproduction

1. Check `curl http://127.0.0.1:9224/json/version` and `curl http://127.0.0.1:9224/json/list`.
2. Serve `implementation.html` from the Linux workspace over `http://10.0.5.8:<port>/...`.
3. Connect Windows Chrome with `connectOverCDP("http://127.0.0.1:9224")` and capture `implementation.png`.
4. Compare `target.png` and `implementation.png` with `--scoring-region 260,80,1650,917 --scoring-region-id content-root` to create `diff.png` and `metrics.json`; outside-ROI pixels remain diagnostic-only.
5. Read `verification.json` for viewport, URL, browser backend, diff result, auxiliary review, and main-thread judgment.

## Decision

The API, ECharts, selection and audited-action rework is complete. r231 verifies the non-first selected-session business interaction loop in Windows Chrome and keeps the encrypted business root aligned with the shared application content area. Its historical `<0.13` acceptance is superseded: `0.12906050692310234` is above the current global `<0.125` business ROI gate, so business-only visual rework is required while the public AppShell remains unchanged.

## r231 Independent Review

- Independent reviewer: `Cicero`.
- P0: none. The final production evidence is internally consistent and the user-approved business ROI gate passes.
- P1 closed: the final reviewer artifact is now version-pinned to r231 at `evidence/ui-image-breakdowns/pages/encrypted-traffic-evidence-center/agent-r231-review.md`.
- P2 deferred: remaining target differences are business-panel density/detail and the narrow ROI margin; no public-shell rework is warranted.
- Main-thread decision: r231 is rework-required under the global `<0.125` business ROI gate.

## r222 Independent Review

- Independent reviewer: `Kant`.
- P1 closed: a quick-location result now becomes the page-level action target. The Windows Chrome interaction locates non-first `s-23a9b7d4c1e8` and confirms the same target in `POST /api/v1/encrypted-traffic/evidence-actions`.
- P2 documented: secondary-read failures remain visibly labeled simulated fallback data; this does not affect the independently evidenced range, filtering, action, and audit-response path.
- Public topbar, sidebar, and bottombar remain shared structures and are outside `content-root` scoring.
- Main-thread decision: business interaction gate passes; strict pixel acceptance remains withheld at `0.13246951521760683` versus `0.015` in the business ROI.

## r214 Independent Review

- Independent reviewer: `Dalton`.
- Review scope: current target, implementation, diff, production source, shared-shell constraint, and business interactions.
- P1 findings closed: evidence KPI duplication, non-functional time range and one-click analysis controls, and missing PCAP near-field entry points.
- P1 finding closed: the three primary evidence panels and lower details now fit in the business viewport without touching the shared bottom bar.
- Public topbar, sidebar, and bottombar are excluded from strict scoring and diff overlay by the canonical `content-root` ROI; the user requires these regions to remain the common product shell.
- Main-thread decision: business route and interaction gates pass; r219 strict pixel acceptance remains withheld at `0.13239945804831302` versus `0.015` in the business ROI.


## Independent Auxiliary Agent Review

- Independent reviewer: `Lagrange`, r194 follow-up.
- Business findings: the optional `feature_fp` enrichment no longer causes an API 500, the “握手” completeness label matches the frontend KPI, invalid `data_mode` is rejected, and the selected Session now drives detail and action targets.
- Runtime finding: the first-frame undefined read is closed; the current production capture reports no bad response, console error, page error, request failure, or horizontal overflow.
- Evidence checked: `r194-evidence-api-runtime.json`, `normal-route-r194-echarts-interactions-runtime.json`, `capture-meta.json`, `metrics.json`, and the r194 deployment images.
- Visual finding: the business gate is pass at `0.11718605324074075 <= 0.35`, but strict pixel acceptance remains rejected because the recorded target is `<= 0.015`.
- Main-thread application: production behavior and interaction are accepted; `pixel-accepted` is deliberately not recorded.

## Visible Chrome Rerun Gate

- r194 was captured through the current interactive Windows Chrome CDP path at 1920 x 1080.
- The fresh independent review is complete for the production implementation and interaction loop.
- Further work for this page is visual alignment inside `content-root` to the strict pixel target, not API, interaction, or public-shell repair.
