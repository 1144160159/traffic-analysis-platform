# campaign-detail-impact-account.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Production image: `traffic/web-ui:ui-campaign-impact-account-visual-20260710-r167`
- Production route: `/campaigns/:campaignId?impact=account&__codex_ui_breakdown_production=1`
- Browser evidence: Windows Chrome 150 through `http://127.0.0.1:9224`
- Final screenshot: `evidence/ui-image-breakdowns/pages/campaign-detail-impact-account/implementation-r167-final.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target and route state | pass | target is the `影响范围 / 账号` focus state; route query is `impact=account` |
| Production React implementation | pass | `CampaignDetailPage.tsx`; no target raster is loaded |
| Typed dynamic data | pass | `CampaignDetailImpactAccount` from `/v1/campaigns/{campaignId}` or typed fallback |
| Dynamic ring | pass | CSS conic ring angles derive from 8/14/9 typed counts |
| Account table | pass | five typed account rows and access paths rendered from snapshot data |
| Local gates | pass | campaign service tests, no-bitmap test and production build passed |
| Production deployment | pass | r167 Deployment is 1/1 Ready and APISIX route is 200 |
| Windows Chrome runtime | pass | no 4xx/5xx, requestfailed, console/pageerror, forbidden raster request or horizontal overflow |
| Visual diff | pass | mismatch `0.09737268518518519 <= 0.35`, tolerance `64` |
| Auxiliary review | pass | r166 circular ring rejected and repaired to the r167 elliptical ring |
| Main-thread judgment | pass | `verification.json` records `business-pixel-accepted` |

## Visual And Semantic Findings

- The target contains six tabs: 资产、账号、服务、部门、校区、业务系统; 账号 is active.
- The production panel contains 31 affected accounts and high/medium/low distributions of 8/14/9, matching 25.8%/45.2%/29.0%.
- Top 5 account names, types, permission-risk labels and login paths match the locked target semantics.
- r166 already passed the numeric threshold, but its ring was circular. The auxiliary review treated this as a structural mismatch and required repair.
- r167 uses a 620x245 data-driven ellipse. The mismatch ratio improved from `0.10694733796296296` to `0.09737268518518519`.
- Remaining diff is dominated by the target raster's blurred typography and minor spacing; no business module is missing or replayed as a bitmap.
- The PNG evidence is exactly 1920x1080. Windows reports a 2133x1200 CSS viewport at DPR 0.9 because of host scaling; this is preserved in capture metadata rather than hidden.

## Evidence

- `target.png`, `regions-overlay.png`, `measurement.json`
- `implementation-r167-final.png`, `implementation.png`
- `diff-r167-final.png`, `diff.png`
- `metrics-r167-final.json`, `metrics.json`
- `capture-meta-r167-final.json`, `capture-meta.json`
- `cdp-version-r167-final-pre-capture.txt`, `cdp-list-r167-final-pre-capture.txt`
- `verification.json`
- `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-detail-impact-account-r167.json`

## Reproduction

1. Confirm r167 is the live `web-ui` image and the APISIX route returns 200.
2. Check `http://127.0.0.1:9224/json/version` and `/json/list`.
3. Open the production campaign route with `impact=account` and `__codex_ui_breakdown_production=1` through Windows Chrome CDP.
4. Capture a 1920x1080 PNG and record runtime events, geometry, key texts and forbidden resources.
5. Run `tests/e2e/ui_visual_diff_metrics.py` against the locked target.
6. Review target, implementation, diff and metrics before writing the main-thread decision.

## Decision

The fresh auxiliary review and main-thread recheck both pass. `campaign-detail-impact-account` is accepted as `business-pixel-accepted`; the next queue item may start.
