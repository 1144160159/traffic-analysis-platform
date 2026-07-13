# Windows Tunnel Channel Preflight

- Result: `pass`
- Generated: `2026-07-03T00:22:20.119Z`
- Windows host: `10.3.6.59`
- Windows user: `LongShine`

## Endpoint Checks

| scope | name | status | result | detail | url |
|---|---|---:|---|---|---|
| linux | app | 200 | pass |  | http://127.0.0.1:5173/screen |
| linux | runtime-config | 200 | pass | runtime config ready for protected-route smoke capture | http://127.0.0.1:5173/src/config/runtime.ts |
| linux | receiver-health | 200 | pass |  | http://127.0.0.1:15174/health |
| linux | viewport-probe | 200 | pass |  | http://127.0.0.1:15174/viewport-probe |
| linux | redirect-health | 200 | pass |  | http://127.0.0.1:15175/health |
| windows | app | 200 | pass |  | http://127.0.0.1:25173/screen |
| windows | runtime-config | 200 | pass | runtime config ready for protected-route smoke capture | http://127.0.0.1:25173/src/config/runtime.ts |
| windows | receiver-health | 200 | pass |  | http://127.0.0.1:25174/health |
| windows | viewport-probe | 200 | pass |  | http://127.0.0.1:25174/viewport-probe |
| windows | redirect-health | 200 | pass |  | http://127.0.0.1:25175/health |

## Formal Capture Package

- Capture plan: `doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json` result=`pass`
- Capture session: `doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json` status=`blocked_desktop_transport_closed`
- Visual pending: `30`
- Interaction pending: `28`

This preflight proves that the Windows-local URLs are reachable and the formal capture package is aligned to the tunnel. It does not replace Desktop Chrome extension screenshots or interaction evidence.

