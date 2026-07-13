# Windows Chrome CDP - encrypted-traffic-tunnel-detection r182

- Status: `business-pixel-accepted`
- Route: `/encrypted-traffic?tab=tunnel-detection`
- Image: `traffic/web-ui:ui-encrypted-traffic-tunnel-detection-20260710-r182`
- Browser: Windows Chrome 150 via `http://127.0.0.1:9224`
- Acceptance JSON: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-tunnel-detection-r182.json`
- Evidence directory: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-tunnel-detection-r182/`

## Gates

| Gate | Result | Evidence |
|---|---|---|
| Focused tests | pass | `encrypted-traffic-tunnel-detection-r182/npm-test.log` |
| Production build | pass | `encrypted-traffic-tunnel-detection-r182/npm-build.log` |
| Deployment | pass | `encrypted-traffic-tunnel-detection-r182/build-deploy-evidence.json` |
| Business visual diff | pass | `pixel_mismatch_ratio=0.11427420910493827`, threshold `0.35` |
| Strict visual diff | fail-documented | `pixel_mismatch_ratio=0.9999836033950618`, threshold `0.015` |
| Normal route runtime | pass | `encrypted-traffic-tunnel-detection-r182/normal-route-runtime.json` |
| Auxiliary review | pass | agent `019f4b28-6185-7642-a8cc-b10622b1708b` |

## Runtime Coverage

The normal production route landed on `tab=tunnel-detection`, consumed the smoke hash, and verified AppShell, 5 tabs, 7 KPI tiles, 6 tunnel cards, 6 tunnel rows, 34 scatter points, 48 heartbeat bars and 9 dense rows. No 4xx/5xx responses, request failures, console errors, page errors or horizontal overflow were recorded.

The first normal-route run exposed sparse heartbeat/rule rendering, so `pageSnapshotAdapters.ts` and `EncryptedTrafficPage.tsx` were patched before the final r182 capture. Main-thread judgment: `business-pixel-accepted`; strict pixel mismatch is preserved as documented non-blocking evidence.
