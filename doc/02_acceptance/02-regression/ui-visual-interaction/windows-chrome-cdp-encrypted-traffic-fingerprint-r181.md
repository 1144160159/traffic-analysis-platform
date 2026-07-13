# Windows Chrome CDP - encrypted-traffic-fingerprint r181

- Status: `business-pixel-accepted`
- Route: `/encrypted-traffic?tab=fingerprint`
- Image: `traffic/web-ui:ui-encrypted-traffic-fingerprint-20260710-r181`
- Browser: Windows Chrome 150 via `http://127.0.0.1:9224`
- Acceptance JSON: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-chrome-cdp-encrypted-traffic-fingerprint-r181.json`
- Evidence directory: `doc/02_acceptance/02-regression/ui-visual-interaction/encrypted-traffic-fingerprint-r181/`

## Gates

| Gate | Result | Evidence |
|---|---|---|
| Focused tests | pass | `encrypted-traffic-fingerprint-r181/npm-test.log` |
| Production build | pass | `encrypted-traffic-fingerprint-r181/npm-build.log` |
| Deployment | pass | `encrypted-traffic-fingerprint-r181/build-deploy-evidence.json` |
| Business visual diff | pass | `pixel_mismatch_ratio=0.11497395833333333`, threshold `0.35` |
| Strict visual diff | fail-documented | `pixel_mismatch_ratio=0.9999180169753087`, threshold `0.015` |
| Normal route runtime | pass | `encrypted-traffic-fingerprint-r181/normal-route-runtime.json` |
| Auxiliary review | pass | agent `019f4b14-af04-79b3-86f6-438cefe967e9` |

## Runtime Coverage

The normal production route landed on `tab=fingerprint`, consumed the smoke hash, and verified AppShell, 5 tabs, 7 KPI tiles, 6 JA3 rows, 34 scatter points, 4 certificate rows, 6 TLS suite items and 4 advice rows. No 4xx/5xx responses, request failures, console errors, page errors or horizontal overflow were recorded.

Main-thread judgment: `business-pixel-accepted`; strict pixel mismatch is preserved as documented non-blocking evidence.
