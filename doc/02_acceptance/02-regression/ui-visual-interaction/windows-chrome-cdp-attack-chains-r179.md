# Windows Chrome CDP attack-chains r179

## Result

- Main-thread judgment: `business-pixel-accepted`
- Strict pixel judgment: `fail-documented`
- Production image: `traffic/web-ui:ui-attack-chains-20260710-r179`
- Browser: Chrome/150.0.7871.49 via `http://127.0.0.1:9224`
- Screenshot: 1920x1080 PNG
- Business mismatch: `0.09817563657407408 <= 0.35`, tolerance 64
- Strict mismatch: `0.9999156057098766 > 0.015`, tolerance 0

## Runtime And Geometry

- Focus production route has zero console errors, page errors, failed requests, HTTP 4xx/5xx and forbidden target-image requests.
- Normal production route preserves the AppShell topbar, sidebar and bottombar.
- The attack-chain canvas renders 6 phases.
- The right rail renders 6 evidence anchors and 6 response recommendations.
- Horizontal overflow is false and normal-route runtime errors are zero.

## Dynamic Implementation

- Endpoint: `/v1/attack-chains`
- Adapter: `fetchPageSnapshot -> adaptAttackChains`
- Components: `AttackCanvas`, `PhaseMatrix`, `PathDetail`, `EvidenceAnchorList`, `ResponseRecommendations`
- No target PNG, implementation HTML or evidence image is loaded by the production route.

## Review

Auxiliary agent `019f4aed-f7da-7cb2-a19d-adeba76c9b0f` passed read-only review under the r179 business acceptance gate. Main-thread visual and runtime review accepted the business-tolerance gate and kept the strict pixel failure explicit.

## Evidence

- Focus actual: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/actual-1920.png`
- Normal route: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/normal-route-1920.png`
- Business metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/metrics-business.json`
- Strict metrics: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/metrics-strict.json`
- Runtime: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/normal-route-runtime.json`
- Test log: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/npm-test.log`
- Build log: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/npm-build.log`
- Docker image inspect: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/docker-image-inspect.json`
- K8s deployment JSON: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/k8s-web-ui-deploy.json`
- APISIX status: `doc/02_acceptance/02-regression/ui-visual-interaction/attack-chains-r179/apisix-http-status.txt`
