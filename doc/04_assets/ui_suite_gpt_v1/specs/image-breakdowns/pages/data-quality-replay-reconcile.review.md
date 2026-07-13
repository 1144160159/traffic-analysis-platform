# data-quality-replay-reconcile review

## Review Status

- Status: `business-pixel-accepted`
- Page id: `data-quality-replay-reconcile`
- Route/state: `/data-quality?tab=replay-reconcile`
- Type: `menu-state`
- Parent: `data-quality`
- Target image reviewed directly: yes
- Browser evidence: Windows Chrome CDP `http://127.0.0.1:9224`
- Production URL: `http://10.0.5.8:30180/data-quality?tab=replay-reconcile&__codex_ui_breakdown_production=1&__capture=r128-final&windowsCdpEvidenceTs=1783555335274`
- Viewport: 1920 x 1080
- Image tag: `traffic/web-ui:ui-data-quality-replay-reconcile-visual-20260709-r128`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Queue lock | pass | order 13 in `pages-menu-order-queue.json` |
| Target PNG | pass | `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-replay-reconcile.png` |
| Current menu state | pass | left menu `采集监测 / 数据质量`, Tab `重放对账` active |
| No breadcrumb path | pass | business top has title and Tab only, no menu path |
| Replay-specific KPI | pass | `对账通过率`、`待重放 DLQ`、`重放成功率`、`重复记录`、`幂等冲突`、`窗口差异率`、`验收包` |
| Business dynamic visuals | pass | reconciliation trend and replay flow are data-driven React/SVG/CSS |
| Runtime health | pass | 0 console/pageerror/requestfailed/HTTP 4xx/5xx |
| Forbidden resources | pass | no target/canonical/evidence/screen resources loaded by page |
| Overflow | pass | no business root/panel/table/chart overflow in `capture-meta-r128-final.json` |
| Text completeness | pass | key text present;省略文本有 title/aria-label |
| Diff full | pass | ratio `0.08337432484567901`, threshold `0.13`, tolerance `90` |
| Diff business | pass | ratio `0.08638923824975904`, threshold `0.13`, tolerance `90` |
| Tests | pass | `npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` |
| Build | pass | `npm --prefix web/ui run build` |
| Production rollout | pass | Deployment image confirmed as r128 |

## Visual Findings

- The old generic replay implementation was replaced by a replay-specific Tab matching the UI target.
- Target structure is matched as seven KPI cards, three upper panels, three lower panels and a four-section right rail.
- Remaining diff hotspots are expected alpha differences: line density, anti-aliasing, table micro-spacing and target raster chart curves.
- Right rail content matches replay anomalies, quick locate, repair advice and evidence/report actions.
- Business dynamic visuals are not screenshot resources.

## Evidence

- `implementation-r128-final.png`
- `implementation.png`
- `capture-meta-r128-final.json`
- `capture-meta.json`
- `diff-r128.png`
- `diff.png`
- `metrics-r128.json`
- `metrics.json`
- `target-business-r128.png`
- `implementation-business-r128.png`
- `diff-business-r128.png`
- `metrics-business-r128.json`
- `measurement.json`
- `regions-overlay.png`
- `verification.json`
- `cdp-version-r128-final-pre-capture.txt`
- `cdp-list-r128-final-pre-capture.txt`

## Auxiliary Review

- target read: pass
- evidence completeness: pass
- runtime clean: pass
- diff pass: pass
- business visuals dynamic: pass
- text complete: pass
- responsive constraints: pass for 1920 x 1080 and CSS media rules for reduced browser/Windows window widths
- no-bitmap UI gate: pass
- docs synced: pass

## Main-Thread Judgment

`business-pixel-accepted`.

The production Windows Chrome screenshot on `http://10.0.5.8:30180` matches the locked `重放对账` page within the current alpha business-pixel threshold. No blocking runtime, resource, overflow, text, menu-state or dynamic-visual issue remains for this page.

## r271 Superseding Review

- Runtime/actions: pass; canvas `7`; endpoint/audit true; error arrays empty.
- Fixed geometry: all-route and direct-click deltas `0`. Visual: `metrics-business-r271.json` ratio `0.08187963325341308 < 0.125`.
- Main-thread judgment: `business-pixel-accepted-r271`.

## Breakdown Depth Review

- Record gate: `breakdown-accepted`.
- Depth: 17 regions, 32 structured texts, 8 components, 6 icons, 12 tokens, 7 interactions.
- Reconciliation uses dynamic ECharts; replay topology remains API-driven SVG; task and difference tables require pagination.
- Target, implementation, diff, metrics and runtime evidence boundaries are explicit.
