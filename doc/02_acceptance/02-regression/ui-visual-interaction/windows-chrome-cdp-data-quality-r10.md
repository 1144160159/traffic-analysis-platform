# Windows Chrome CDP Full Route Evidence

- Run ID: `windows-cdp-full-route-20260703112451`
- Result: `pass`
- Runtime routes passed: `1/1`
- Visual diff passed: `0/1`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-data-quality-r10`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-data-quality-r10`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.

## Visual Diff Gaps

- `data-quality`: mismatch=0.999991, screenshot=`evidence/windows-chrome-cdp-20260703-data-quality-r10/data-quality-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-data-quality-r10/data-quality/diff-1920.png`

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `data-quality` | pass | fail | 0.999991 | `evidence/windows-chrome-cdp-20260703-data-quality-r10/data-quality-1920x1080.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783077891996` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

