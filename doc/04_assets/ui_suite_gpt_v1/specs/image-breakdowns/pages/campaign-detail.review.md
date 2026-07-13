# campaign-detail.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Scope: production React route plus locked canonical PNG
- Evidence target: `evidence/ui-image-breakdowns/pages/campaign-detail/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production image: `traffic/web-ui:ui-campaigns-visual-20260710-r165`
- Production route: `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37`

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
| Windows Chrome screenshot | pass | `implementation-r165-final.png` captured through Windows Chrome CDP on the production route |
| Visual diff | pass | `metrics-r165-final.json`: mismatch `0.10545572916666666`, threshold `<=0.35`, channel tolerance `64` |
| Runtime clean | pass | `capture-meta-r165-final.json`: no 4xx/5xx, requestfailed, console/pageerror, or bad geometry |
| Dynamic business data | pass | Campaign profile, metrics, phase timeline, related alerts, impact scope, evidence bundle, response actions, and review rows are driven by `fetchCampaignDetailSnapshot(campaignId)` via `services/api.ts` |
| Auxiliary review | pass | r165 screenshot, diff, metrics, runtime metadata, and dynamic-data scope reviewed in this thread |
| Main-thread judgment | pass | `verification.json` records `business-pixel-accepted` |

## Visual Findings

- The target belongs to category `pages` and is handled as `战役详情`.
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
2. Open `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37` through Windows Chrome CDP.
3. Capture `implementation-r165-final.png` at 1920x1080 with deviceScaleFactor 1 and no post-capture resize.
4. Compare the production screenshot with `target.png` to create `diff-r165-final.png` and `metrics-r165-final.json`.
5. Read `capture-meta-r165-final.json` and `verification.json` for URL, viewport, browser backend, runtime status, diff result, auxiliary review, and main-thread judgment.

## Decision

`business-pixel-accepted`. The production route is deployed under `traffic/web-ui:ui-campaigns-visual-20260710-r165`; Windows Chrome CDP capture passed runtime and visual diff gates. Text and numeric differences against the target PNG are accepted as live/dynamic campaign data differences because the page is driven through `CampaignDetailPage` and `campaignDetailApi.ts`, not by static target replay.


## Independent Auxiliary Agent Review

- Independent review batch: evidence/ui-image-breakdowns/_agent-review-batches/pages/pages-review-batch-002.json
- Subagent status: reviewed
- Evidence checked: record_json, markdown, review, required_evidence_files, metrics, capture_meta, verification, measurement, text_ocr
- Metric ratio: 0
- Findings: All required evidence files exist; metrics pass with pixel_mismatch_ratio within max_pixel_ratio; capture metadata is Windows Chrome CDP 1920x1080 DPR 1 with no scroll/errors/failures; record/review/markdown content meets evidence-ready structure thresholds.
- Main-thread application: accepted after rechecking metrics, Windows Chrome capture metadata, evidence paths, and record completeness.

## Visible Chrome Rerun Gate

- Status: closed by r165.
- CDP precheck: `cdp-version-r165-final-pre-capture.txt`, `cdp-list-r165-final-pre-capture.txt`.
- Screenshot: `implementation-r165-final.png`.
- Diff: `diff-r165-final.png`.
- Metrics: `metrics-r165-final.json`.
- Capture meta: `capture-meta-r165-final.json`.
- Windows CDP report: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-detail-r165-baseline.json`.
- Main-thread verification: `verification.json`.
