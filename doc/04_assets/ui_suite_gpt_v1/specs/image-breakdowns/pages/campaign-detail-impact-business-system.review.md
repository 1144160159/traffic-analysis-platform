# campaign-detail-impact-business-system.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Production image: `traffic/web-ui:ui-campaign-impact-business-system-20260710-r171`
- Production route: `/campaigns/:campaignId?impact=business-system&__codex_ui_breakdown_production=1`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/implementation-r171-final.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target and route state | pass | target is the `影响范围 / 业务系统` focus state; query is `impact=business-system` |
| Production React implementation | pass | `CampaignDetailPage.tsx`; no target raster is loaded |
| Typed dynamic data | pass | `CampaignDetailImpactBusinessSystem` from `/v1/campaigns/{campaignId}` or typed fallback |
| Dynamic ring and risk rows | pass | conic angles derive from 3/4/2; labels, counts and percentages are DOM data |
| Business system table | pass | five typed rows include services, risks and P0/P1/P2 recovery priority |
| Local gates | pass | campaign service tests, no-bitmap test and production build passed |
| Production deployment | pass | r171 Deployment is 1/1 Ready and APISIX route is 200 |
| Windows Chrome runtime | pass | no 4xx/5xx, requestfailed, console/pageerror, forbidden raster request or overflow |
| Visual diff | pass | mismatch `0.06796344521604938 <= 0.35`, tolerance `64` |
| Auxiliary review | pass | r168 and r170 were rejected; fresh r171 review passes after both structural repairs |
| Main-thread judgment | pass | `verification.json` records `business-pixel-accepted` |

## Repair History

- r168 passed the configured numeric threshold at `0.10338348765432098`, but auxiliary review rejected the extra summary frame, undersized ellipse, table whitespace, wrong columns and bordered risk labels.
- r170 aligned the outer frame, 700x303 CSS ellipse, risk box and 461px table; mismatch fell to `0.06808883101851852`.
- The second review rejected only the risk-list internal columns because counts and percentages remained too far right.
- r171 places label/count/percent text at CSS x1100/x1578/x1819, which becomes screenshot x990/x1420/x1637 at Windows DPR 0.9 and matches the target.
- The final independent review reports PASS. Remaining differences are native font antialiasing, target glow/blur and minor title offsets.

## Production Interaction Boundary

- The full-frame focus state exists only for deterministic screenshot evidence.
- Normal production campaign details render the same component inside the business page.
- Desktop business details must use a narrow side Drawer or small Modal and must not cover the whole browser business area.

## Evidence

- `target.png`, `regions-overlay.png`, `measurement.json`
- `implementation-r171-final.png`, `implementation.png`
- `diff-r171-final.png`, `diff.png`
- `metrics-r171-final.json`, `metrics.json`
- `capture-meta-r171-final.json`, `capture-meta.json`
- `cdp-version-r171-final-pre-capture.txt`, `cdp-list-r171-final-pre-capture.txt`
- `verification.json`
- `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-detail-impact-business-system-r171.json`

## Decision

The fresh auxiliary review and main-thread recheck both pass. `campaign-detail-impact-business-system` is accepted as `business-pixel-accepted`; the next queue item may start.
