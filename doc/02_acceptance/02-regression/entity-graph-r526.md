# Windows Chrome CDP Full Route Evidence

- Run ID: `entity-graph-r526`
- Result: `fail`
- Runtime routes passed: `0/1`
- Visual diff passed: `0/1`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.128`
- Viewport: `1920x1080`
- Evidence dir: `evidence/ui-image-breakdowns/pages/graph/entity-graph-r526`
- Acceptance dir: `doc/02_acceptance/02-regression/entity-graph-r526`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `graph`: 1 request failures; 1 console errors; 1 page errors

## Visual Diff Gaps

- `graph`: mismatch=0.999929, screenshot=`evidence/ui-image-breakdowns/pages/graph/entity-graph-r526/graph-1920x1080.png`, diff=`doc/02_acceptance/02-regression/entity-graph-r526/graph/diff-1920.png`

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `graph` | fail | fail | 0.999929 | `evidence/ui-image-breakdowns/pages/graph/entity-graph-r526/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1784511932585` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

