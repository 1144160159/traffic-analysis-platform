# campaign-detail.png review

## Review Status

- Status: `production-interaction-pass-visual-diff-blocked`
- Target image reviewed directly: yes
- Scope: production React route plus locked canonical PNG
- Evidence target: `evidence/ui-image-breakdowns/pages/campaign-detail/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Production image: `docker.io/traffic/web-ui:campaign-detail-r777`
- Production route: `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37`

## 2026-07-26 r777 Response-Flow Visual Review

- The response flow is no longer rendered as six grid cells with absolutely positioned arrows. Six stage nodes and five Ant Design arrow connectors are sibling flex items, so circle centers, arrows, labels, and timestamps keep shared baselines.
- Visual refinement: 46px semantic-color circles, 18px icons, 12px/700 stage labels, tabular timestamps, restrained inner halo, and a dedicated 1440–1650px compact rule.
- Windows Chrome/Xshell 9224 focused measurement: six steps, five connectors, every timeline element inside the flow region, and `responseTimesClipped=false`.
- Production rollout: `docker.io/traffic/web-ui:campaign-detail-r777`; Deployment generation/observedGeneration `978/978`, revision `953`.
- Image identity: OCI manifest `sha256:92e6b2439e29dd9bca50428721071481f8b453102dd2ba9543f2bbfbb906df31`; image ID `sha256:9745430c2f6d5e7e1b39b049df09fd7c01422886a6d249193b5843b66f0b85c2`.
- Full-page acceptance and four responsive widths pass without layout collision or runtime errors. Evidence: `inline-acceptance-r777-prod.json`, `implementation-r777-prod.png`, and focused `response-flow-r777-prod.png`.

## 2026-07-26 r774 Selected-Region Review

- The latest user-selected crops supersede the older full-page PNG for three regions: the business title is the standalone 32px/900 `战役详情`; the response rail is the six-circle `发现 → 研判 → 遏制 → 根除 → 恢复 → 复盘` flow; and `证据摘要` is an independent five-row card.
- Production rollout: Deployment generation/observedGeneration `975/975`, revision `950`, Pod `web-ui-7f495d996f-47l8n` on `zeus-server`, Ready `1/1`, restart `0`.
- Image identity: OCI manifest `sha256:295410d40364809bcc4e1cf992612f368b555cdb73ab6cabdfa7ef79f2a8ae20`; image ID `sha256:0fa2f919b52b696488ef1e55f9bf2396f5d01c96e3afd676032e2f078ce4818f`.
- Windows Chrome 150/Xshell `9224 → 9222` production acceptance passed: title `32px/900`; six response steps and icons; eight real transition-backed response actions; five review rows; five evidence-digest rows; related-alert cell overlap `false`.
- High/medium/low/all alert filters, six inline impact tabs, three ECharts rings, real campaign API `200`, and 1920/1500/1366/1024 responsive geometry passed with no document horizontal overflow, module overlap, business HTTP error, console error, page error, or request failure.
- The live campaign API does not provide suspicious-file, SHA256, first-domain, or resolved-IP values. These four values intentionally render `--`; first outbound time is derived from the real campaign timestamps. The UI does not replay target-image values as production data.
- Evidence: `evidence/ui-image-breakdowns/pages/campaign-detail/inline-acceptance-r774-prod.json`, `implementation-r774-prod.png`, six `impact-*-r774-prod.png`, `comparison-r774-prod.png`, `diff-r774-prod.png`, and `metrics-r774-prod.json`.
- Selected-region contract: **pass**. Legacy full-page pixel gate: **blocked**, mismatch `0.09759837962962963 > 0.015`. It remains separate because the old canonical contains a superseded breadcrumb/title and different static business data.

## 2026-07-26 r763 Superseding Review

- Production rollout: Deployment generation `962`, revision `937`, Pod `web-ui-76fb779cd9-2sz4g` on `zeus-server`, Ready `1/1`, restart `0`.
- Image identity: OCI manifest `sha256:d4780e2381c7df1719ca7ec31c7ec54caf7f1ce479636eef8a53bb68a73667ee`; image ID `sha256:0d46714f025e8605de1ecd0554275b9d0a85033f1dea233f774d963c5b6a1760`.
- Runtime data: Windows Chrome 150 through Xshell `127.0.0.1:9224 -> 127.0.0.1:9222` received `200` from `/api/v1/campaigns/campaign-exfil-default-1782729598739-e1d2dc37`; no mock/visual-fixture parameter was used.
- Interaction result: title is only `战役详情`; three required rings are ECharts canvases; high/medium/low alert filters are functional; six impact tabs switch distinct inline content without a modal; 1920/1500/1366/1024 viewports have no module overlap or document horizontal overflow.
- Layout result: evidence completeness and evidence summary are independent stacked panels; response flow and response actions share the canonical right-rail panel; dynamic empty and unavailable states are explicit.
- Production evidence: `evidence/ui-image-breakdowns/pages/campaign-detail/inline-acceptance-r763-prod.json`, `implementation-r763-prod.png`, six `impact-*-r763-prod.png` captures, `comparison-r763-prod.png`, `diff-r763-prod.png`, and `metrics-r763-prod.json`.
- Formal visual gate: **blocked**. Pixel mismatch is `0.09181037808641976`, which exceeds the locked `0.015` threshold. The older r165 acceptance used a relaxed threshold and is historical only; it must not be treated as the current formal pixel result.
- Main-thread adjudication: production functionality, real-data wiring, deployment, responsive layout, and requested interaction fixes pass; exact full-page pixel acceptance remains open.

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
