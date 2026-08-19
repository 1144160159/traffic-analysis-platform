# Third-view Review: CLE-P0-SCREEN-001

- decision: `pass`
- reviewer: `codex-static-review`
- scope: `/screen auth boundary and fullscreen layout preservation`

## Findings

- none

## Review Notes

- `/screen` now lives inside the same `ProtectedLayout` auth guard used by authenticated product routes, so when `VITE_AUTH_ENABLED=true` and `auth_token` is absent the route redirects to `/login`.
- `MainLayout` detects `/screen` after auth has passed and renders only the outlet, preserving the fullscreen visual contract without exposing the route outside the protected shell.
- `ScreenRouteAuth.test.tsx` verifies both negative and positive paths: unauthorized `/screen` shows login captcha and does not render the screen title; authorized `/screen` renders the screen title and omits the normal layout brand chrome.
- The screen continues to use existing dashboard and alert APIs through the shared API client, so authorization headers still flow from the existing `auth_token` path.

## Residual Risk

- This is local regression evidence. Live browser verification through APISIX should be repeated before production release if the deployed frontend image is rebuilt.
