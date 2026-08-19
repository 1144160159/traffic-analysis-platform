# Evidence Report: CLE-P0-SCREEN-001

## Summary

The selected strategy is protected `/screen`: the route is no longer public and is guarded by the existing `ProtectedLayout` token check. The screen remains fullscreen because `MainLayout` renders only the outlet for `/screen` after authorization succeeds.

## Close Conditions

### /screen has exactly one public/protected/readonly strategy

`/screen has exactly one public/protected/readonly strategy`: protected.

Evidence:

- `web/ui/src/App.tsx` places `path="screen"` under `path="/" element={<ProtectedLayout />}`.
- `web/ui/src/App.tsx` keeps the single auth decision in `ProtectedLayout`: when `VITE_AUTH_ENABLED=true` and `localStorage.auth_token` is absent, it redirects to `/login`.
- `web/ui/src/components/Layout/MainLayout.tsx` only changes presentation for `/screen`; it does not create another public or readonly bypass.

### unauthorized behavior is verified

`unauthorized behavior is verified`.

Evidence:

- `web/ui/src/test/ScreenRouteAuth.test.tsx` test `启用鉴权且无 token 时重定向到登录页` stubs `VITE_AUTH_ENABLED=true`, removes `auth_token`, starts at `/screen`, expects login captcha, and asserts the screen title is absent.
- `tests/run_tests.sh web` passed with `exit_code: 0` in `local-report.md`.

### sensitive data display policy is documented

`sensitive data display policy is documented`.

Policy:

- `/screen` displays live operational dashboard, alert, traffic, probe, attack phase, and data quality signals.
- Because these signals are operationally sensitive, the chosen policy is protected access only, using the same authenticated operator token boundary as the rest of the product UI.
- No public or readonly-token mode is enabled in this change. If a future readonly-wallboard mode is needed, it must be introduced as a separate explicit token strategy with scoped claims, expiry, audit, and data minimization.

Evidence:

- `web/ui/src/test/ScreenRouteAuth.test.tsx` test `启用鉴权且有 token 时显示全屏态势大屏` verifies authorized token access and fullscreen rendering.
- `doc/02_acceptance/runs/mvp-47-screen-fix-guidance/guidance/guidance.json` reports `blockers=0` after scout re-read the route tree.

## Verification

- `npm run test -- --run src/test/ScreenRouteAuth.test.tsx`: passed, 2 tests.
- `tests/run_tests.sh web`: passed; lint had 9 existing warnings, build passed, vitest passed 151 tests.
- `python -B scripts/codex_loop/scout.py --run-id mvp-47-screen-fix-scout`: passed.
- `python -B scripts/codex_loop/guide.py --context-dir doc/02_acceptance/runs/mvp-47-screen-fix-scout/context --run-id mvp-47-screen-fix-guidance`: passed with 0 blockers.
