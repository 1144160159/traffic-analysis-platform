# Windows Chrome CDP campaign-detail-impact-department r177

## Result

- Main-thread judgment: `business-pixel-accepted`
- Strict pixel judgment: `fail-documented`
- Production image: `traffic/web-ui:ui-campaign-impact-department-20260710-r177`
- Browser: Windows Chrome 150.0.7871.49 via `http://127.0.0.1:9224`
- Screenshot: 1920x1080 PNG; CSS viewport 2133x1200 at DPR 0.9
- Business mismatch: `0.07469184027777778 <= 0.35`, tolerance 64
- Strict mismatch: `0.9997222222222222 > 0.015`, tolerance 0

## Runtime And Geometry

- Focus production route has zero console errors, page errors, failed requests, HTTP 4xx/5xx and forbidden target-image requests.
- Normal production route preserves the AppShell topbar, sidebar and bottombar.
- The active impact tab is `部门7 个`.
- Five department data rows and five progress values are visible.
- Rows and `查看全部部门` are inside the impact panel body.
- Horizontal overflow is false and normal-route runtime errors are zero.

## Dynamic Implementation

- Endpoint: `/v1/campaigns/{campaignId}`
- Adapter: `normalizeCampaignDetailSnapshot -> buildImpactDepartment`
- Types: `CampaignDetailImpactDepartment`, `CampaignDetailDepartmentRow`, `CampaignDetailImpactRiskRow`
- Visual component: `CampaignImpactDepartmentContent`
- Risk counts drive a CSS conic-gradient; the Top 5 table and progress bars are generated from typed rows.
- Evidence mode uses a deterministic typed snapshot; the normal route exercises the live endpoint with typed fallback.

## Review

Fresh auxiliary agent `019f4ace-10da-71c3-8149-f0ba1470bf0d` passed the dynamic implementation, Windows Chrome route, runtime, normal geometry and token redaction checks. It allows business-tolerance acceptance only and requires the strict pixel failure to remain explicit. Main-thread visual and evidence review agrees.

## Evidence

- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/implementation-r177-final.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/normal-route-r177.png`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/metrics-business-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/metrics-strict-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/capture-meta-r177-final.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/normal-route-runtime-r177.json`
- `evidence/ui-image-breakdowns/pages/campaign-detail-impact-department/verification.json`
