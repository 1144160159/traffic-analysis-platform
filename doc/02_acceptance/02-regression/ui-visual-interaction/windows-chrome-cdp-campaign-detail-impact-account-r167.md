# Campaign Detail Impact Account Windows Chrome Evidence

- Result: `pass`
- Page: `campaign-detail-impact-account`
- Production image: `traffic/web-ui:ui-campaign-impact-account-visual-20260710-r167`
- Production route: `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?impact=account&__codex_ui_breakdown_production=1&__codex_page_id=campaign-detail-impact-account&windowsCdpEvidenceTs=1783659030995`
- Windows Chrome: `150.0.7871.49` via `http://127.0.0.1:9224`
- Screenshot: `1920x1080` PNG
- Runtime: `pass` with no console/page errors, request failures, HTTP 4xx/5xx, forbidden raster resources or horizontal overflow
- Visual diff: `pass`, mismatch `0.09737268518518519` <= `0.35`, channel tolerance `64`
- Dynamic data: `pass`; account ring and table are driven by `CampaignDetailImpactAccount` from `/v1/campaigns/{campaignId}` or typed fallback
- Auxiliary review: repaired the r166 circular ring to the r167 elliptical ring; mismatch improved from `0.10694733796296296` to `0.09737268518518519`
- Main-thread judgment: `business-pixel-accepted`

Evidence files are under `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-campaign-detail-impact-account-r167/` and `evidence/ui-image-breakdowns/pages/campaign-detail-impact-account/`.
