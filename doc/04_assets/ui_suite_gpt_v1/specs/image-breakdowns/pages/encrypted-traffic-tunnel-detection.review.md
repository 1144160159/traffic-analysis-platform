# encrypted-traffic-tunnel-detection.png review

## Review Status

- Status: `business-pixel-accepted`
- Strict pixel status: `fail-documented`
- Production image: `traffic/web-ui:ui-encrypted-traffic-tunnel-detection-20260710-r182`
- Production route: `/encrypted-traffic?tab=tunnel-detection`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final screenshot: `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/implementation-r182-final.png`
- Normal route screenshot: `evidence/ui-image-breakdowns/pages/encrypted-traffic-tunnel-detection/normal-route-r182.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Production React implementation | pass | `EncryptedTrafficPage.tsx`; no target raster is loaded |
| Route state | pass | `tab=tunnel-detection` activates `TunnelDetectionContent` |
| API/snapshot data path | pass | `fetchPageSnapshot(route.id) -> adaptEncryptedTraffic -> visuals.encryptedTraffic` |
| Tunnel visuals | pass | tunnel cards, tunnel rows, scatter, heartbeat, rules and evidence preview consume typed visuals |
| Focused local gates | pass | 3 focused tests passed |
| Production build | pass | `npm-build.log`, Vite large chunk warning only |
| Production deployment | pass | r182 Deployment is `1/1`; APISIX `200` |
| Business visual gate | pass | `0.11427420910493827 <= 0.35`, channel tolerance `64` |
| Strict visual gate | fail-documented | `0.9999836033950618 > 0.015`, channel tolerance `0` |
| Normal route runtime | pass | active tab, counts, no runtime errors, no horizontal overflow |
| Rework validation | pass | heartbeat 48 bars, dense rows 9, DoH/低熵 business text present |
| Sensitive data hygiene | pass | persisted URLs use `<redacted>` token material |
| Auxiliary review | pass | agent `019f4b28-6185-7642-a8cc-b10622b1708b` confirmed r182 |
| Main-thread judgment | pass | business-pixel-accepted with strict failure documented |

## Notes

The first normal-route check exposed sparse-data rendering gaps. The adapter and page fallback were patched, rebuilt, redeployed and recaptured. Current evidence supports business-pixel acceptance; strict pixel mismatch remains documented because the target PNG and production DOM differ at exact pixel level.

## Breakdown Record Review

- Direct inspection confirms a `213px` navigation, `58px` toolbar and no bottom status bar.
- Regions capture KPI, tunnel list, entropy scatter, heartbeat, features, rules, evidence and right rail.
- The ledger covers tunnel metrics, table actions, annotations, features and report commands.
- Scatter, heartbeat and risk donut require dynamic ECharts, tooltip and resize behavior.
- Tunnel and rule tables expose pagination; list selection drives evidence preview.
- Alert, evidence, watchlist, remediation and export controls return visible feedback.

## Record Decision

The record meets the deep-breakdown bar. Business ROI remains distinct from strict-pixel failure.
