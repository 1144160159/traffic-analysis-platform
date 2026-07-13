# Windows Chrome CDP Full Route Evidence

- Run ID: `windows-cdp-full-route-20260703105403`
- Result: `pass`
- Runtime routes passed: `2/2`
- Visual diff passed: `0/0`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1366x768`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-responsive-1366-r2`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-responsive-1366-r2`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.

## Visual Diff Gaps

- No visual diff blockers under the configured threshold.

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `dashboard` | pass | skipped | - | `evidence/windows-chrome-cdp-20260703-responsive-1366-r2/dashboard-1366x768.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1783076043106` |
| `data-quality` | pass | skipped | - | `evidence/windows-chrome-cdp-20260703-responsive-1366-r2/data-quality-1366x768.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783076043106` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

