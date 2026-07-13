# Windows Chrome CDP Full Route Evidence

- Run ID: `windows-cdp-full-route-20260703104650`
- Result: `fail`
- Runtime routes passed: `0/2`
- Visual diff passed: `0/0`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1366x768`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-responsive-1366-r1`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-responsive-1366`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `dashboard`: horizontal overflow
- `data-quality`: horizontal overflow

## Visual Diff Gaps

- No visual diff blockers under the configured threshold.

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `dashboard` | fail | skipped | - | `evidence/windows-chrome-cdp-20260703-responsive-1366-r1/dashboard-1366x768.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1783075610212` |
| `data-quality` | fail | skipped | - | `evidence/windows-chrome-cdp-20260703-responsive-1366-r1/data-quality-1366x768.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783075610212` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

