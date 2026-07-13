# Windows Chrome CDP campaign-detail-impact-campus r176

## Result

- Main-thread judgment: `business-pixel-accepted`
- Strict pixel judgment: `fail-documented`
- Production image: `traffic/web-ui:ui-campaign-impact-campus-20260710-r176`
- Browser: Windows Chrome 150.0.7871.49 via `http://127.0.0.1:9224`
- Screenshot: 1920x1080 PNG; CSS viewport 2133x1200 at DPR 0.9
- Business mismatch: `0.07146990740740741 <= 0.35`, tolerance 64
- Strict mismatch: `0.9999136766975308 > 0.015`, tolerance 0

## Runtime And Geometry

- Focus production route has zero console errors, page errors, failed requests, HTTP 4xx/5xx and forbidden target-image requests.
- Normal production route preserves the AppShell topbar, sidebar and bottombar.
- Five campus data rows plus the header, full content container and `查看全部校区` link are inside the impact panel body.
- Horizontal overflow is false and normal-route runtime errors are zero.

## Dynamic Implementation

- Endpoint: `/v1/campaigns/{campaignId}`
- Adapter: `normalizeCampaignDetailSnapshot -> buildImpactCampus`
- Types: `CampaignDetailImpactCampus`, `CampaignDetailCampusRow`, `CampaignDetailImpactRiskRow`
- Visual component: `CampaignImpactCampusContent`
- Risk counts drive a CSS conic-gradient; the Top 5 table is generated from typed rows.
- Evidence mode uses a deterministic typed snapshot; the normal route exercises the live endpoint with typed fallback.

## Review

Fresh auxiliary agent `019f4ab3-4f56-7ac1-9408-dffccb4f118f` passed the dynamic implementation, Windows Chrome route, runtime and normal geometry. It allows business-tolerance acceptance only and requires the strict pixel failure to remain explicit. Main-thread visual and evidence review agrees.

## Evidence

- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/implementation-r176-final.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/normal-route-r176.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/metrics-business-r176-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/metrics-strict-r176-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/capture-meta-r176-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/normal-route-runtime-r176.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-campus/verification.json`
