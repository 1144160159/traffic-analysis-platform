# Login capability layout smoke - 2026-07-02

Scope: focused local regression for the login page left capability strip: `加密传输`, `身份校验`, `审计留痕`.

Implementation under test:

- `web/ui/src/pages/LoginPage.tsx`
- `web/ui/src/styles/pages.css`

Evidence:

- `login/desktop-1920.png`
- `login/desktop-1440.png`
- `login/desktop-1366.png`
- `login/tablet-1024.png`
- `login/mobile-390.png`
- `login/layout-metrics.json`

Result:

- 1920/1440/1366 desktop viewports: capability item overlap area is `0`.
- 1920/1440/1366 desktop viewports: icon-to-label overlap area is `0`.
- 1920/1440/1366 desktop viewports: no horizontal overflow.
- Visual layer order is constrained to `visual=0`, `hero=2`, `panel=2`, so the background veil no longer renders above the capability labels.

This is a targeted layout smoke only. The full UI visual-interaction gate remains blocked until every target has passing 1920x1080 visual diff evidence and route-specific business interaction evidence.
