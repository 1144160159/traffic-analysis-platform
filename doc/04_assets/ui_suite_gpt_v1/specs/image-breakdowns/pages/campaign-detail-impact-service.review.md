# campaign-detail-impact-service.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail-documented`
- Production image: `traffic/web-ui:ui-campaign-impact-service-20260710-r178`
- Production route: `/campaigns/:campaignId?impact=service`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final focus screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/implementation-r178-final.png`
- Normal route screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-service/normal-route-r178.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target and route state | pass | `impact=service`; focus state contains service summary and Top 5 |
| Production React implementation | pass | `CampaignDetailPage.tsx`; no target raster is loaded |
| Typed dynamic data | pass | `CampaignDetailImpactService` from `/v1/campaigns/{campaignId}` with typed fallback |
| Dynamic risk ring and table | pass | risk counts drive CSS angles; five rows render from `snapshot.impactService` |
| Local gates | pass | 3 test files, 7 tests and production build passed |
| Production deployment | pass | r178 Deployment is `1/1 Ready`; APISIX `/` is 200 |
| Windows Chrome runtime | pass | no 4xx/5xx, failed request, console/page error, forbidden raster request or horizontal overflow |
| Normal business geometry | pass | service tab, five rows and downlink are inside the panel body; risk summary/table overlap count is 0 |
| Business visual gate | pass | `0.07248697916666667 <= 0.35`, channel tolerance `64` |
| Strict visual gate | fail | `0.9998230131172839 > 0.015`, channel tolerance `0` |
| Auxiliary review | pass | agent `019f4adf-7ec1-7c61-b039-d3af900abcd4` returned PASS after strict status and build/deploy evidence were added |
| Main-thread judgment | pass | business-pixel-accepted with strict failure documented |

## Notes

The service page is now a real production React state, not a replay of `target.png` or `implementation.html`.

The strict-pixel gate remains explicit because the generated target and production implementation differ at exact pixel level. The strict-specific capture metadata now reports `strict-pixel-fail-documented` while preserving runtime capture pass. The business gate is accepted because semantic content, geometry, runtime, deployment and interaction checks pass.

## Breakdown Depth Review

- Record gate: `breakdown-accepted`.
- Depth: 16 regions, 52 structured texts, 12 components, 10 icons, 18 tokens, 8 interactions.
- Target was read directly; service names, ports, risks and dependency labels match the image ledger.
- Mapping, evidence, differences and conclusion are explicit; strict-pixel failure remains documented.
