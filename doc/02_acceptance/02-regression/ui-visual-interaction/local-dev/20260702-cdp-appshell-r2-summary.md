# AppShell Local Chrome/CDP Iteration

- Run IDs: `20260702-cdp-screen-appshell-r2`, `20260702-cdp-dashboard-appshell-r2`
- Targets: `screen`, `dashboard`
- Capture backend: local Chrome CDP development check
- Acceptance eligibility: no. Formal acceptance still requires Codex Desktop Chrome extension evidence under `latest/`.

## Code Changes

- AppShell user area now displays the semantic role from `currentUser.role` before falling back to permission roles.
- The local bypass/admin permission role is no longer exposed as visible `admin 在线`; the shell now renders `sec_analyst / 安全分析师 / 在线`.
- The sidebar user block includes the bottom-right fullscreen affordance required by the common shell reference.
- `SessionPrincipal` includes optional `role` so display semantics and permission roles remain separate.
- Added `tests/e2e/ui_visual_layer_metrics.py` to compare screenshots by contracted layer bbox.

## Validation

- `node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs`: passed, 181 manifest items, 28 route contracts, 70 overlay contracts, 0 errors, 0 warnings.
- `npm --prefix web/ui run test -- --run src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts`: passed, 32 tests.
- `npm --prefix web/ui run build`: passed, Vite large chunk warnings only.
- `PLAYWRIGHT_BASE_URL=http://10.0.5.8:30202 npm --prefix web/ui run test:e2e -- e2e/product-navigation.spec.ts --project=chromium -g "dashboard|screen"`: passed, 2 tests.

## Chrome/CDP Evidence

- `screen` screenshot: `20260702-cdp-screen-appshell-r2/screen/actual-1920.png`
- `dashboard` screenshot: `20260702-cdp-dashboard-appshell-r2/dashboard/actual-1920.png`
- Both captures report `document_width=1920`, `document_height=1080`, no vertical/horizontal scroll, and no console/page/request/server errors.

## Layer Metrics

- `screen` overall diff remains failed, `pixel_mismatch_ratio=0.9998992091049382`.
- `dashboard` overall diff remains failed, `pixel_mismatch_ratio=0.9993870563271605`.
- New layer metrics include `mean_channel_delta` and `mean_pixel_max_delta` in addition to strict mismatch ratio.
- The strict layer gate still fails for all contracted layers because the current gate counts any channel difference at tolerance 0; use `mean_channel_delta` for iteration trend and keep the strict ratio for final acceptance.

## Remaining Work

- Continue AppShell micro-alignment for topbar and sidebar geometry.
- Continue screen page topology and right rail high-fidelity replication without using page screenshots as images.
- Capture formal Desktop Chrome extension evidence once the wrapper tool is exposed.
