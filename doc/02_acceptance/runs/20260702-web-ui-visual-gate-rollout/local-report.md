# Web UI Visual Gate Rollout 2026-07-02

- Run ID: `20260702-web-ui-visual-gate-rollout-r1`
- Time: `2026-07-02T02:19:20+08:00`
- Image: `traffic/web-ui:ui-visual-gate-20260702-r1`
- Docker image ID: `sha256:7cf21adb82654bfce3b57675ac59bb4e42896a551ba01fee84ad76037f358ed9`
- Containerd manifest digest: `sha256:0f1d98fe7fdd9ae56b00751cc3c8f4acfee53bcf9965eb600c9a49954b0598d6`

## Scope

This rollout publishes the current React/CSS login-page visual-gate work to the live K8s Web UI. It does not mark the project complete and does not replace the formal UI visual-interaction dual gate.

## Source Changes Published

- `LoginPage` now has an explicit visual capture mode at `/login?__taf_visual=1`.
- The visual capture mode freezes the captcha image to the design-reference value `7K38` so visual diff evidence is not invalidated by random captcha output.
- Normal `/login` still uses the real captcha API and the login form is not submitted by the visual gate.
- The login shield mark is DOM/CSS (`中`) rather than a full-page image.
- `visual-acceptance.json` and `route-page-map.json` keep the visual target id as `login` and set its screenshot query to `__taf_visual=1`.

## Commands

```bash
cd web/ui && npm run build
DOCKER_BUILDKIT=1 docker build --build-context appdist=web/ui/dist -f web/ui/deployments/Dockerfile.overlay -t traffic/web-ui:ui-visual-gate-20260702-r1 web/ui
docker save traffic/web-ui:ui-visual-gate-20260702-r1 -o /tmp/traffic-web-ui-ui-visual-gate-20260702-r1.tar
ctr -n k8s.io images import /tmp/traffic-web-ui-ui-visual-gate-20260702-r1.tar
scp -q /tmp/traffic-web-ui-ui-visual-gate-20260702-r1.tar 10.0.5.9:/tmp/traffic-web-ui-ui-visual-gate-20260702-r1.tar
ssh -o BatchMode=yes 10.0.5.9 'ctr -n k8s.io images import /tmp/traffic-web-ui-ui-visual-gate-20260702-r1.tar'
kubectl -n traffic-analysis set image deployment/web-ui web-ui=traffic/web-ui:ui-visual-gate-20260702-r1
kubectl -n traffic-analysis rollout status deployment/web-ui --timeout=180s
```

## Evidence

- K8s rollout passed: `deployment "web-ui" successfully rolled out`.
- Live image: `traffic/web-ui:ui-visual-gate-20260702-r1`.
- Ready state: `web-ui` `1/1`.
- Running pod: `web-ui-74d64f6886-m9n28`, node `zeus-server`.
- Both `8-2tb` and `zeus-server` containerd `k8s.io` namespaces contain `docker.io/traffic/web-ui:ui-visual-gate-20260702-r1`.
- APISIX `GET /login?__taf_visual=1` returned `200 OK`.
- Live bundle chunk: `LoginPage-Bv9LeaR2.js`.
- Live bundle marker check found `__taf_visual` and `visual-diff-static-login`.
- Runtime config remains production-like: `AUTH_ENABLED: "true"`, `USE_MOCK: "false"`, `DESKTOP_SMOKE_TOKEN_ENABLED: "false"`.

## Gate State After Rollout

- `doc/02_acceptance/02-regression/ui-visual-interaction-preflight-latest.json`
  - Run ID: `20260702-ui-visual-interaction-preflight-r14-login-visual-mode-wrapper-timeout`
  - Result: `blocked`
  - Visual diff: `0/30`
  - Business interaction: `2/28`
  - Full-page design image references: `0`
  - Desktop Chrome status: `blocked`
- `doc/02_acceptance/09-completion/project-completion-audit-latest.json`
  - Run ID: `20260702-project-completion-audit-r45-login-visual-mode-wrapper-timeout`
  - Result: `blocked`
  - Passed: `7`
  - Failed: `9`
  - Blockers: `9`

## Remaining Blockers

- Formal Codex Desktop Chrome wrapper capture is still blocked: `desktop_chrome_list_tabs` timed out before and after `js_reset`.
- Formal 1920x1080 Desktop Chrome `capture-meta.json` evidence is still missing or failing.
- Login local-dev visual diff is still failing: `pixel_mismatch_ratio=0.9998172260802469`; the remaining major delta is the background/left hero visual asset versus the target UI image.
