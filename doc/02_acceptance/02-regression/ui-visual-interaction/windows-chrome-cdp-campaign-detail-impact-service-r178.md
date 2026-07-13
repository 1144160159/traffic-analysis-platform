# Windows Chrome CDP campaign-detail-impact-service r178

## Result

- Main-thread judgment: `business-pixel-accepted`
- Strict pixel judgment: `fail-documented`
- Production image: `traffic/web-ui:ui-campaign-impact-service-20260710-r178`
- Browser: Chrome/150.0.7871.49 via `http://127.0.0.1:9224`
- Screenshot: 1920x1080 PNG
- Business mismatch: `0.07248697916666667 <= 0.35`, tolerance 64
- Strict mismatch: `0.9998230131172839 > 0.015`, tolerance 0

## Runtime And Geometry

- Focus production route has zero console errors, page errors, failed requests, HTTP 4xx/5xx and forbidden target-image requests.
- Normal production route preserves the AppShell topbar, sidebar and bottombar.
- The active impact tab is `服务42 个`.
- Five service rows are visible: PostgreSQL, MinIO API, LDAP, NFS and Redis.
- Rows and `查看全部服务` are inside the impact panel body.
- Horizontal overflow is false and normal-route runtime errors are zero.

## Dynamic Implementation

- Endpoint: `/v1/campaigns/{campaignId}`
- Adapter: `normalizeCampaignDetailSnapshot -> buildImpactService`
- Types: `CampaignDetailImpactService`, `CampaignDetailServiceRow`, `CampaignDetailImpactRiskRow`
- Visual component: `CampaignImpactServiceContent`
- Risk counts drive a CSS conic-gradient; the Top 5 table is generated from typed rows.
- Evidence mode uses a deterministic typed snapshot; the normal route exercises the live endpoint with typed fallback.

## Review

Auxiliary agent `019f4adf-7ec1-7c61-b039-d3af900abcd4` initially returned FAIL because strict evidence status was ambiguous and build/deploy logs were not persisted. Main thread addressed both, then the same auxiliary review returned PASS. Strict-specific metadata reports `strict-pixel-fail-documented`, and test/build/Docker/rollout/APISIX evidence is stored in the r178 acceptance directory.

## Evidence

- Focus actual: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/actual-1920.png`
- Normal route: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/normal-route-1920.png`
- Business metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/metrics-business.json`
- Strict metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/metrics-strict.json`
- Runtime: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/normal-route-runtime.json`

## Build Deploy Evidence

- Test log: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/npm-test.log`
- Build log: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/npm-build.log`
- Docker image inspect: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/docker-image-inspect.json`
- K8s deployment JSON: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/k8s-web-ui-deploy.json`
- APISIX status: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/apisix-http-status.txt`
- Strict capture meta: `doc/02_acceptance/02-regression/ui-visual-interaction/campaign-detail-impact-service-r178/capture-meta-strict.json`
