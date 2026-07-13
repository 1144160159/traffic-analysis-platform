# Windows Chrome CDP Full Route Evidence

- Run ID: `campaign-detail-r165-baseline`
- Result: `pass`
- Runtime routes passed: `1/1`
- Visual diff passed: `1/1`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.49`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-campaign-detail/campaign-detail-r165-baseline`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-campaign-detail-r165-baseline`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.

## Visual Diff Gaps

- No visual diff blockers under the configured threshold.

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `campaign-detail` | pass | pass | 0.105456 | `evidence/windows-chrome-cdp-campaign-detail/campaign-detail-r165-baseline/campaign-detail-1920x1080.png` | `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?windowsCdpEvidenceTs=1783644331187` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

