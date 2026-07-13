# Windows Chrome CDP campaign-detail-impact-business-system r171

## Result

- Result: pass
- Main-thread judgment: business-pixel-accepted
- Production image: traffic/web-ui:ui-campaign-impact-business-system-20260710-r171
- Browser: Windows Chrome 150.0.7871.49 via http://127.0.0.1:9224
- Screenshot: 1920x1080 PNG
- Pixel mismatch: 0.06796344521604938 <= 0.35, channel tolerance 64

## Runtime

- All 27 expected business strings are present.
- Five business-system rows and three risk rows are rendered from DOM data.
- Console errors, page errors, failed requests and HTTP 4xx/5xx: 0.
- Horizontal and vertical overflow: false.
- Forbidden target/reference raster requests: none.

## Review And Repair

- r168 passed the number gate but was rejected for the extra risk frame, undersized ellipse, table whitespace and wrong badge styling.
- r170 repaired the major structure but was rejected because risk counts and percentages remained too far right.
- r171 aligns risk label/count/percentage at screenshot x990/x1420/x1637 and received a fresh independent PASS.
- Mismatch improved from 0.10338348765432098 to 0.06796344521604938.

## Dynamic Data

- Endpoint: /v1/campaigns/{campaignId}
- Adapter: normalizeCampaignDetailSnapshot -> buildImpactBusinessSystem
- Types: CampaignDetailImpactBusinessSystem, CampaignDetailBusinessSystemRow, CampaignDetailImpactRiskRow
- Visual component: CampaignImpactBusinessSystemContent
- Normal production refresh: 30 seconds; deterministic focus capture disables refresh only for evidence.

## Overlay Policy

The full-frame focus query is screenshot evidence only. Production desktop business details must use a narrow side Drawer or small Modal and must keep the host business context visible.

## Evidence

- evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/implementation-r171-final.png
- evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/diff-r171-final.png
- evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/metrics-r171-final.json
- evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/capture-meta-r171-final.json
- evidence/ui-image-breakdowns/pages/campaign-detail-impact-business-system/verification.json
