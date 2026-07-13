# Windows Chrome CDP encrypted-traffic r180

## Result

- Main-thread judgment: `business-pixel-accepted`
- Strict pixel judgment: `fail-documented`
- Production image: `traffic/web-ui:ui-encrypted-traffic-20260710-r180`
- Browser: Chrome/150.0.7871.49 via `http://127.0.0.1:9224`
- Screenshot: 1920x1080 PNG
- Business mismatch: `0.11367332175925926 <= 0.35`, tolerance 64
- Strict mismatch: `0.9999262152777778 > 0.015`, tolerance 0

## Runtime And Geometry

- Focus production route has zero console errors, page errors, failed requests, HTTP 4xx/5xx and forbidden target-image requests.
- Normal production route preserves the AppShell topbar, sidebar and bottombar.
- The page renders 5 tabs, 7 KPIs, protocol distribution, 6 JA3 rows, 34 scatter points, 6 tunnel cards, 6 tunnel rows, 6 evidence rows, egress rows and action buttons.
- Horizontal overflow is false and normal-route runtime errors are zero.

## Dynamic Implementation

- Endpoint group: `/v1/encrypted-traffic/*`
- Adapter: `fetchPageSnapshot -> adaptEncryptedTraffic -> visuals.encryptedTraffic`
- Components: `ProtocolDistribution`, `Ja3Table`, `Ja3Scatter`, `TunnelFeatureCards`, `TunnelTable`, `EvidenceTable`, `EgressProfile`, `AdviceList`
- Sparse API payloads are supplemented with typed fallback rows; no target PNG, implementation HTML or evidence image is loaded by the production route.

## Review

Auxiliary agent `019f4b06-5981-7542-8112-19d447ba1144` passed read-only review under the r180 business acceptance gate. Main-thread visual and runtime review accepted the business-tolerance gate and kept the strict pixel failure explicit.

## Evidence

- Focus actual: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/actual-1920.png`
- Normal route: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/normal-route-1920.png`
- Business metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/metrics-business.json`
- Strict metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/metrics-strict.json`
- Runtime: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/normal-route-runtime.json`
- Test log: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/npm-test.log`
- Build log: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/npm-build.log`
- Docker image inspect: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/docker-image-inspect.json`
- K8s deployment JSON: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/k8s-web-ui-deploy.json`
- APISIX status: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-r180/apisix-http-status.txt`
