# login.png review

## Review Status

- Status: `pixel-accepted` under active relaxed alpha gate
- Target image reviewed directly: yes
- Scope: single canonical PNG, production route `/login`
- Evidence target: `evidence/ui-image-breakdowns/pages/login/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`
- Latest deployed image: `traffic/web-ui:ui-dashboard-window-adaptive-20260706-r86`
- Production URL: `http://10.0.5.8:30180/login?__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783384900198`
- Viewport: `1920x1080`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pages verification loop read before edits |
| Windows Chrome CDP preflight | pass | `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `/json/list`; Chrome `150.0.7871.47`, Windows user agent |
| Production route screenshot | pass | `implementation.png` captured from 30180 production route through Windows Chrome CDP |
| Runtime | pass | `capture-meta.json` reports `runtime_status=pass`; no console/pageerror/requestfailed/bad response/forbidden target resource request |
| Bitmap/resource guard | pass | `npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` passed before production capture |
| Build | pass | `npm --prefix web/ui run build` passed on current worktree |
| Deployment | pass | production Deployment is `traffic/web-ui:ui-dashboard-window-adaptive-20260706-r86`, ready `1/1` |
| Active visual diff | pass | `metrics.json` uses `channel_tolerance=12`, `max_pixel_ratio=0.1`, actual `pixel_mismatch_ratio=0.08856385030864197` |
| Strict reference diff | documented-not-active | `metrics-strict-r12.json` keeps zero-tolerance reference `pixel_mismatch_ratio=0.5320765817901234` > `0.015` |
| Region metrics | documented | top hotspots remain `hero_title`, `hero_shield`, `capability_buttons`, `campus_band`, `panel_tabs` for future tightening |
| Main-thread judgment | pass | `verification.json` accepted with no missing evidence or outstanding differences under the active relaxed alpha gate; strict reference artifacts are retained |

## Policy Notes

- Existing component/style icons do not need screenshot extraction when crop evidence shows they already match the target.
- For this login page, capability screenshot PNG rendering became visibly blurry at Windows Chrome DPR=1, so r12 uses existing code-rendered SVG icons from `@ant-design/icons` instead of screenshot bitmap variants. Exact upstream SVG URLs are recorded on the DOM via `data-icon-source-url`.
- Current relaxed resource gate allows explicitly scoped page background and bottom/footer panel resources; business page UI screenshots remain forbidden as page resources.
- Full `target.png`, `regions-overlay.png`, `implementation.png`, canonical screen replays, full forms/cards/business panels, and public replay assets remain forbidden.

## Production Findings

| Type | Location | Current | Required For Acceptance | Status |
|---|---|---|---|---|
| evidence-scope | `/login` | Windows Chrome CDP production evidence at 30180, viewport `1920x1080` | final acceptance must use 30180 production route evidence from Windows Chrome CDP | documented |
| active-relaxed-pixel-gate | full image | channel tolerance 12 mismatch `0.08856385030864197` | `<=0.1` | accepted |
| strict-reference-gate | full image | zero-tolerance mismatch ratio `0.5320765817901234` | `<=0.015` | documented-not-active |
| hero-title | left hero | `0.5405465587044535`, rms `66.177`; font/background remain the main strict-mode hotspot | strict typography/background tightening only | follow-up |
| hero-shield | left hero | current mismatch `0.2147948637644848`, rms `7.823`; ordinary login keeps the visual shield resource | accepted-alpha |
| capability-buttons | left hero | code-rendered SVG icons stay crisp; current region mismatch `0.2568268733850129`, rms `34.429` | icon glyph silhouettes can still be tightened if strict mode is reactivated | accepted-alpha |
| campus-band | background | current mismatch `0.16521447028423772`, rms `6.813` | background density is non-business decoration unless it affects readability | accepted-alpha |
| right-panel | login panel | current mismatch `0.11218772694262891`, rms `21.707` | production capture shows form layout intact at `1920x1080` | accepted-alpha |

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/login/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/login/regions-overlay.png`
- Implementation screenshot: `evidence/ui-image-breakdowns/pages/login/implementation.png`
- Full diff: `evidence/ui-image-breakdowns/pages/login/diff.png`
- Full metrics: `evidence/ui-image-breakdowns/pages/login/metrics.json`
- Strict metrics: `evidence/ui-image-breakdowns/pages/login/metrics-strict-r12.json`
- Region metrics: `evidence/ui-image-breakdowns/pages/login/measurement.json`
- Capture metadata: `evidence/ui-image-breakdowns/pages/login/capture-meta.json`
- Runtime report: `evidence/ui-image-breakdowns/pages/login/production-route-report.json`
- Verification: `evidence/ui-image-breakdowns/pages/login/verification.json`
- Capability crop comparison: `evidence/ui-image-breakdowns/pages/login/icon-crop-probes/capability-buttons-target-vs-r12.png`

## Reproduction

1. Run `curl -i --max-time 5 http://127.0.0.1:9224/json/version` and `curl -i --max-time 5 http://127.0.0.1:9224/json/list`.
2. Capture production route with `env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy node doc/04_assets/ui_suite_gpt_v1/capture_image_breakdown_production_route.mjs --record doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/pages/login.json --base-url http://10.0.5.8:30180 --cdp-url http://127.0.0.1:9224 --wait-ms 2600 --channel-tolerance 12 --max-pixel-ratio 0.1`.
3. Refresh region metrics with `python3 tests/e2e/ui_login_region_metrics.py --source evidence/ui-image-breakdowns/pages/login/target.png --actual evidence/ui-image-breakdowns/pages/login/implementation.png --output evidence/ui-image-breakdowns/pages/login/measurement.json --tolerance 12`.
4. Review `verification.json`, `target.png`, `implementation.png`, `diff.png`, `metrics.json`, `measurement.json`, and `capture-meta.json`.

## Decision

This image is accepted under the active relaxed alpha gate requested by the user: Windows Chrome production route, runtime clean, `channel_tolerance=12`, `max_pixel_ratio=0.1`, actual `0.08856385030864197`. The zero-tolerance strict reference remains documented-only and is retained for future tightening; login is not a business page, so background brightness and decorative strict-mode hotspots do not block the current queue gate.
