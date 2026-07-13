# attack-chains.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail-documented`
- Production image: `traffic/web-ui:ui-attack-chains-20260710-r179`
- Production route: `/attack-chains`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final screenshot: `evidence/ui-image-breakdowns/pages/attack-chains/implementation-r179-final.png`
- Normal route screenshot: `evidence/ui-image-breakdowns/pages/attack-chains/normal-route-r179.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Production React implementation | pass | `AttackChainAnalysisPage.tsx`; no target raster is loaded |
| API/snapshot data path | pass | `fetchPageSnapshot(route.id) -> adaptAttackChains` |
| Attack canvas | pass | 6 phase cards render in the main work area |
| Evidence anchors | pass | 6 evidence rows render in the right rail |
| Response recommendations | pass | 6 recommendation rows render in the right rail |
| Normal route runtime | pass | no 4xx/5xx, failed request, console/page error or horizontal overflow |
| Focused local gates | pass | 3 focused tests passed; full adapter file has 2 unrelated existing failures |
| Production deployment | pass | r179 Deployment is `1/1`; APISIX `200` |
| Business visual gate | pass | `0.09817563657407408 <= 0.35`, channel tolerance `64` |
| Strict visual gate | fail-documented | `0.9999156057098766 > 0.015`, channel tolerance `0` |
| Auxiliary review | pass | agent `019f4aed-f7da-7cb2-a19d-adeba76c9b0f` confirmed r179 under the business acceptance gate |
| Main-thread judgment | pass | business-pixel-accepted with strict failure documented |

## Notes

The page is accepted on the business-pixel gate. The strict pixel gate remains explicit because the target PNG and production DOM differ at exact pixel level, while semantic content, runtime behavior, deployment, focused tests and auxiliary review pass.

## Record Completeness Review

- Required Markdown sections map to facts visible in the canonical target.
- Region ledger separates public AppShell geometry from the attack-chain business area.
- Text ledger names stages, entities, alerts, evidence anchors, actions, and rail headings.
- Component ledger distinguishes the API-driven SVG topology from pageable tables.
- Icon ledger records semantic symbols rather than treating the target raster as an asset.
- Token ledger preserves success, warning, danger, information, border, and text semantics.
- Interaction ledger covers filters, selection, zoom, evidence actions, confirmation, and pagination.
- Evidence target exists; breakdown acceptance alone does not claim all-system completion.
