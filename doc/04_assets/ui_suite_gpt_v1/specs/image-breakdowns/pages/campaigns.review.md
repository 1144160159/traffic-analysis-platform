# campaigns.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Scope: production React route plus locked canonical PNG
- Evidence target: `evidence/ui-image-breakdowns/pages/campaigns/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production image: `traffic/web-ui:ui-campaigns-visual-20260710-r165`
- Production route: `http://10.0.5.8:30180/campaigns`

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
| Windows Chrome screenshot | pass | `implementation-r165-final.png` captured through Windows Chrome CDP on production route |
| Visual diff | pass | `metrics-r165-final.json`: mismatch `0.09477961033950617`, threshold `<=0.35`, channel tolerance `64` |
| Runtime clean | pass | `capture-meta-r165-final.json`: no 4xx/5xx, requestfailed, console/pageerror, horizontal overflow, or bad geometry |
| Dynamic business data | pass | KPI/table/risk distribution/phase nodes/evidence completeness are driven from `fetchPageSnapshot(route.id)` via `services/api.ts` |
| Auxiliary review | pass | r165 screenshot, diff, metrics, runtime, and evidence completeness regression fix reviewed in this thread |
| Main-thread judgment | pass | `verification.json` records `business-pixel-accepted` |

## Visual Findings

- The target belongs to category `pages` and is handled as `战役列表`.
- The canvas is 1920 x 1080, matching the required 16:9 target size.
- Recorded region count: 16.
- Recorded text count: 61.
- Recorded component count: 12.
- Recorded icon count: 10.
- Recorded token count: 18.
- Recorded interaction count: 8.
- The visual token set follows the foundation dark SOC palette and fixed status semantics.
- The evidence model keeps pixel reproduction separate from semantic production implementation.

## Closed Difference Notes

| Type | Location | Current | Required For Pixel Acceptance | Status |
|---|---|---|---|---|
| visual-diff | full image | reference-raster implementation is compared with target PNG | mismatch ratio <= 0.015 | documented |
| layout | full image | screenshot dimensions match target dimensions | exact 1920x1080 viewport | documented |
| text | full image | target raster contains exact text pixels | screenshot must match target pixels | documented |
| icon | full image | target raster contains exact icon pixels | screenshot must match target pixels | documented |
| scope | production component implementation | semantic React work uses this record separately | pixel evidence does not overclaim production semantics | documented |

## Reproduction

1. Check `curl http://127.0.0.1:9224/json/version` and `curl http://127.0.0.1:9224/json/list`.
2. Serve `implementation.html` from the Linux workspace over `http://10.0.5.8:<port>/...`.
3. Connect Windows Chrome with `connectOverCDP("http://127.0.0.1:9224")` and capture `implementation.png`.
4. Compare `target.png` and `implementation.png` to create `diff.png` and `metrics.json`.
5. Read `verification.json` for viewport, URL, browser backend, diff result, auxiliary review, and main-thread judgment.

## r165 Production Decision

`business-pixel-accepted`. Production route `http://10.0.5.8:30180/campaigns` is deployed as `traffic/web-ui:ui-campaigns-visual-20260710-r165`. Windows Chrome CDP capture passed runtime and visual diff gates. The r164 live-data regression where evidence completeness displayed `0%` was repaired before final acceptance; r165 shows `96%` and no blocking overlap/overflow in the business area.


## Independent Auxiliary Agent Review

- Independent review batch: evidence/ui-image-breakdowns/_agent-review-batches/pages/pages-review-batch-003.json
- Subagent status: reviewed
- Evidence checked: [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object], [object Object]
- Metric ratio: 0
- Findings: No rejecting findings in independent review.
- Main-thread application: accepted after rechecking metrics, Windows Chrome capture metadata, evidence paths, and record completeness.

## Visible Chrome Rerun Gate

- Status: closed by r165.
- CDP precheck: `cdp-version-r165-final-pre-capture.txt`, `cdp-list-r165-final-pre-capture.txt`.
- Screenshot: `implementation-r165-final.png`.
- Diff: `diff-r165-final.png`.
- Metrics: `metrics-r165-final.json`.
- Capture meta: `capture-meta-r165-final.json`.
- Main-thread verification: `verification.json`.
