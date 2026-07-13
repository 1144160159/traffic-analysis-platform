# campaign-detail-impact-campus.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail`
- Production image: `traffic/web-ui:ui-campaign-impact-campus-20260710-r176`
- Production route: `/campaigns/:campaignId?impact=campus`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final focus screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/implementation-r176-final.png`
- Normal route screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/normal-route-r176.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target and route state | pass | `impact=campus`; focus state contains campus summary and Top 5 |
| Production React implementation | pass | `CampaignDetailPage.tsx`; no target raster is loaded |
| Typed dynamic data | pass | `CampaignDetailImpactCampus` from `/v1/campaigns/{campaignId}` with typed fallback |
| Dynamic risk ring and table | pass | risk counts drive CSS angles; five rows render from `snapshot.impactCampus` |
| Local gates | pass | 3 test files, 5 tests and production build passed |
| Production deployment | pass | r176 Deployment is `1/1 Ready`; APISIX `/campaigns` is 200 |
| Windows Chrome runtime | pass | no 4xx/5xx, failed request, console/page error, forbidden raster request or scroll |
| Normal business geometry | pass | header plus five rows, full content and downlink are inside panel body |
| Business visual gate | pass | `0.07146990740740741 <= 0.35`, channel tolerance `64` |
| Strict visual gate | fail | `0.9999136766975308 > 0.015`, channel tolerance `0` |
| Auxiliary review | pass | fresh r176 review allows business acceptance only |
| Main-thread judgment | pass | accepted only as `business-pixel-accepted`; strict failure remains explicit |

## Repair History

- r174 focus evidence was runtime-clean, but independent review rejected the normal route because the impact panel clipped the Top 5 content.
- r175 changed the detail grid from a fixed 278px impact row to a `2.1fr / 1fr` allocation. All table rows became visible, but content still exceeded the panel by about 11.7 CSS px and the downlink by about 1px.
- r176 removed the normal impact content minimum height and compacted table-block spacing. The content bottom is `848.82` and the panel-body bottom is `849.06`; all six rows and the downlink are inside the body.
- The capture tool now redacts `codex_smoke_token` in persisted CDP target URLs. A recursive evidence scan found no remaining JWT.

## Evidence Interpretation

- The focus screenshot is a real APISIX/Web UI React route using a deterministic typed snapshot for repeatable visual evidence.
- The normal route uses the live `/v1/campaigns/{campaignId}` path with typed fallback when campus fields are absent. The focus screenshot alone is not evidence of a live campus API payload.
- The PNG is 1920x1080. Windows reports a 2133x1200 CSS viewport at DPR 0.9; this scaling is recorded in capture metadata.
- `business-pixel-accepted` is not `pixel-perfect`. The strict `0.015 / 0` result remains failed and unresolved.

## Auxiliary Review

- Agent: `019f4ab3-4f56-7ac1-9408-dffccb4f118f`
- Dynamic React/CSS/typed implementation: pass
- Windows Chrome CDP, APISIX and runtime: pass
- Normal route AppShell and geometry: pass
- Business tolerance gate: pass
- Strict pixel gate: fail, correctly documented
- Verdict: allow `business-pixel-accepted` only; do not claim strict pixel acceptance

## Evidence

- `target.png`, `regions-overlay.png`, `measurement.json`
- `implementation-r176-final.png`, `implementation.png`
- `diff-business-r176-final.png`, `metrics-business-r176-final.json`
- `diff-strict-r176-final.png`, `metrics-strict-r176-final.json`
- `capture-meta-r176-final.json`, `production-route-report-r176-final.json`
- `normal-route-r176.png`, `normal-route-runtime-r176.json`
- `cdp-version-r176-final.json`, `cdp-list-r176-final.json`
- `verification.json`

## Decision

The fresh auxiliary review and main-thread recheck both pass for the business-tolerance gate. `campaign-detail-impact-campus` is accepted as `business-pixel-accepted`; strict pixel matching remains failed and must not be represented otherwise.
