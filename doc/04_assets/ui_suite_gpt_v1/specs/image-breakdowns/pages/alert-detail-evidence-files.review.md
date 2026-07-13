# alert-detail-evidence-files.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Scope: page 18 / `alert-detail-evidence-files`
- Route: `/alerts`
- Parent menu: `alerts`
- Production URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-files&evidenceView=files`
- Image tag: `traffic/web-ui:ui-alert-detail-evidence-files-visual-20260709-r141`
- Evidence target: `evidence/ui-image-breakdowns/pages/alert-detail-evidence-files/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, `PAGES_IMPLEMENTATION_VERIFICATION_LOOP.md`, `PAGES_MENU_ORDER_QUEUE.md` |
| Queue lock | pass | order 18, `menu-state`, parent `alerts` |
| Target PNG exists | pass | `target.png` |
| Direct visual inspection | pass | panel/tabs/table/tags/footer recorded |
| Markdown breakdown | pass | current file |
| JSON breakdown | pass | `alert-detail-evidence-files.json` |
| Measurement | pass | `measurement.json` |
| Regions overlay | pass | `regions-overlay.png` |
| Windows Chrome screenshot | pass | `implementation-r141-final.png`, alias `implementation.png` |
| Runtime cleanliness | pass | `capture-meta-r141-final.json` has no console/pageerror/requestfailed/4xx/5xx/overflow |
| Forbidden resource markers | pass | none |
| Visual diff | pass | `metrics-r141.json` ratio `0.09753086419753086 <= 0.12` |
| Business diff | pass | `metrics-business-r141.json` ratio `0.09753086419753086 <= 0.12` |
| Text completeness | pass | key texts present; clipped long values have `title` |
| Dynamic business data | pass | evidence rows come from `fetchAlertDetailSnapshot` / typed fallback |
| Screenshot resource boundary | pass | no target/canonical/business screenshot used as UI resource |
| Parent menu state | pass | capture meta records active `/alerts` item while panel target hides AppShell visually |
| Data-quality 8-tab stability | pass | `tab-geometry-r141-tabs-final.json`, maxGeometryDelta `0` at 1920x1080 and 1366x768 |

## Visual Findings

- Target and implementation both render the same full-viewport evidence files panel.
- Diff hotspots are mainly text anti-aliasing, icon stroke differences and small column-width differences; the business structure, active tab, table row, file tags and footer action match the target.
- The r140 panel overflow was fixed in r141; `capture-meta.json` now reports empty root/panel/table/chart overflow arrays.
- AppShell is hidden only for this visual breakdown target. The DOM still keeps the `alerts` menu selected, satisfying detail-state parent menu selection.

## Data And Resource Review

- The file evidence row is built from typed alert detail data, not from the target image.
- Evidence tab counts are derived from `snapshot.evidenceRows`.
- Independent icons use Ant Design icon components.
- No map/topology/trend/business diagram is present in this target; no business dynamic diagram is flattened to a bitmap.

## Main-Thread Judgment

`business-pixel-accepted`.

The page passes production-route Windows Chrome evidence, runtime checks, full/business diff thresholds, data-driven rendering checks, text/tooltip checks, and the latest 8-tab geometry stability regression.
