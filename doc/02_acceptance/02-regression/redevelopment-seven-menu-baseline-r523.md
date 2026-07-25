# Windows Chrome CDP Full Route Evidence

- Run ID: `seven-menu-baseline-r523`
- Result: `fail`
- Runtime routes passed: `0/7`
- Visual diff passed: `0/7`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.128`
- Viewport: `1920x1080`
- Evidence dir: `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523`
- Acceptance dir: `doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `alerts`: 1 request failures; 1 console errors; 1 page errors
- `campaigns`: 1 request failures; 1 console errors; 1 page errors
- `attack-chains`: 1 request failures; 1 console errors; 1 page errors
- `graph`: 1 request failures; 1 console errors; 1 page errors
- `fusion`: 1 request failures; 1 console errors; 1 page errors
- `baselines`: 1 request failures; 1 console errors; 1 page errors
- `topics`: 1 request failures; 1 console errors; 1 page errors

## Visual Diff Gaps

- `fusion`: mismatch=0.999963, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/fusion-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/fusion/diff-1920.png`
- `baselines`: mismatch=0.999936, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/baselines-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/baselines/diff-1920.png`
- `graph`: mismatch=0.999930, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/graph-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/graph/diff-1920.png`
- `attack-chains`: mismatch=0.999923, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/attack-chains-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/attack-chains/diff-1920.png`
- `campaigns`: mismatch=0.999918, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/campaigns-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/campaigns/diff-1920.png`
- `topics`: mismatch=0.999904, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/topics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/topics/diff-1920.png`
- `alerts`: mismatch=0.999872, screenshot=`evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/alerts-1920x1080.png`, diff=`doc/02_acceptance/02-regression/redevelopment-seven-menu-baseline-r523/alerts/diff-1920.png`

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `alerts` | fail | fail | 0.999872 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/alerts-1920x1080.png` | `http://10.0.5.8:30180/alerts?windowsCdpEvidenceTs=1784509614534` |
| `campaigns` | fail | fail | 0.999918 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/campaigns-1920x1080.png` | `http://10.0.5.8:30180/campaigns?windowsCdpEvidenceTs=1784509614534` |
| `attack-chains` | fail | fail | 0.999923 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/attack-chains-1920x1080.png` | `http://10.0.5.8:30180/attack-chains?windowsCdpEvidenceTs=1784509614534` |
| `graph` | fail | fail | 0.999930 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1784509614534` |
| `fusion` | fail | fail | 0.999963 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/fusion-1920x1080.png` | `http://10.0.5.8:30180/fusion?windowsCdpEvidenceTs=1784509614534` |
| `baselines` | fail | fail | 0.999936 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/baselines-1920x1080.png` | `http://10.0.5.8:30180/baselines?windowsCdpEvidenceTs=1784509614534` |
| `topics` | fail | fail | 0.999904 | `evidence/ui-image-breakdowns/redevelopment-seven-menu-baseline-r523/seven-menu-baseline-r523/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1784509614534` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

