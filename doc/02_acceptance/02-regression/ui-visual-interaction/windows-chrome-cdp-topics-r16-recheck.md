# Windows Chrome CDP Full Route Evidence

- Run ID: `topics-r16-recheck`
- Result: `fail`
- Runtime routes passed: `0/1`
- Visual diff passed: `0/1`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-topics-r16-recheck/topics-r16-recheck`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-topics-r16-recheck`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `topics`: 1 bad responses; 1 console errors

## Visual Diff Gaps

- `topics`: mismatch=0.999956, screenshot=`evidence/windows-chrome-cdp-20260703-topics-r16-recheck/topics-r16-recheck/topics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-topics-r16-recheck/topics/diff-1920.png`

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `topics` | fail | fail | 0.999956 | `evidence/windows-chrome-cdp-20260703-topics-r16-recheck/topics-r16-recheck/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1783081178599` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

