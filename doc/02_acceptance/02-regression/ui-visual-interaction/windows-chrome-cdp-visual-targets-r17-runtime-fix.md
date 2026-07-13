# Windows Chrome CDP Full Route Evidence

- Run ID: `visual-targets-r17-runtime-fix`
- Result: `pass`
- Runtime routes passed: `28/28`
- Visual diff passed: `0/28`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.

## Visual Diff Gaps

- `data-quality`: mismatch=0.999990, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/data-quality-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/data-quality/diff-1920.png`
- `attack-chains`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/attack-chains-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/attack-chains/diff-1920.png`
- `forensics`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/forensics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/forensics/diff-1920.png`
- `encrypted-traffic`: mismatch=0.999982, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/encrypted-traffic-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/encrypted-traffic/diff-1920.png`
- `not-found`: mismatch=0.999980, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/not-found-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/not-found/diff-1920.png`
- `dashboard`: mismatch=0.999979, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/dashboard-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/dashboard/diff-1920.png`
- `mlops`: mismatch=0.999979, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/mlops-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/mlops/diff-1920.png`
- `deployments`: mismatch=0.999976, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/deployments-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/deployments/diff-1920.png`
- `settings`: mismatch=0.999974, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/settings-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/settings/diff-1920.png`
- `fusion`: mismatch=0.999974, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/fusion-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/fusion/diff-1920.png`
- `rules`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/rules-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/rules/diff-1920.png`
- `screen`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/screen-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r17-runtime-fix/screen/diff-1920.png`
- ... 16 more visual diff gaps

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `login` | pass | fail | 0.999301 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/login-1920x1080.png` | `http://10.0.5.8:30180/login?windowsCdpEvidenceTs=1783081324612` |
| `screen` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/screen-1920x1080.png` | `http://10.0.5.8:30180/screen?windowsCdpEvidenceTs=1783081324612` |
| `dashboard` | pass | fail | 0.999979 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/dashboard-1920x1080.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1783081324612` |
| `alerts` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/alerts-1920x1080.png` | `http://10.0.5.8:30180/alerts?windowsCdpEvidenceTs=1783081324612` |
| `alert-detail` | pass | fail | 0.999968 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/alert-detail-1920x1080.png` | `http://10.0.5.8:30180/alerts/alert-default-1782752318016-1dd589c4?windowsCdpEvidenceTs=1783081324612` |
| `campaigns` | pass | fail | 0.999971 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/campaigns-1920x1080.png` | `http://10.0.5.8:30180/campaigns?windowsCdpEvidenceTs=1783081324612` |
| `campaign-detail` | pass | fail | 0.999939 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/campaign-detail-1920x1080.png` | `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?windowsCdpEvidenceTs=1783081324612` |
| `attack-chains` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/attack-chains-1920x1080.png` | `http://10.0.5.8:30180/attack-chains?windowsCdpEvidenceTs=1783081324612` |
| `encrypted-traffic` | pass | fail | 0.999982 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/encrypted-traffic-1920x1080.png` | `http://10.0.5.8:30180/encrypted-traffic?windowsCdpEvidenceTs=1783081324612` |
| `forensics` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/forensics-1920x1080.png` | `http://10.0.5.8:30180/forensics?windowsCdpEvidenceTs=1783081324612` |
| `assets` | pass | fail | 0.999971 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/assets-1920x1080.png` | `http://10.0.5.8:30180/assets?windowsCdpEvidenceTs=1783081324612` |
| `graph` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1783081324612` |
| `fusion` | pass | fail | 0.999974 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/fusion-1920x1080.png` | `http://10.0.5.8:30180/fusion?windowsCdpEvidenceTs=1783081324612` |
| `baselines` | pass | fail | 0.999972 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/baselines-1920x1080.png` | `http://10.0.5.8:30180/baselines?windowsCdpEvidenceTs=1783081324612` |
| `probes` | pass | fail | 0.999965 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/probes-1920x1080.png` | `http://10.0.5.8:30180/probes?windowsCdpEvidenceTs=1783081324612` |
| `rules` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/rules-1920x1080.png` | `http://10.0.5.8:30180/rules?windowsCdpEvidenceTs=1783081324612` |
| `deployments` | pass | fail | 0.999976 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/deployments-1920x1080.png` | `http://10.0.5.8:30180/deployments?windowsCdpEvidenceTs=1783081324612` |
| `models` | pass | fail | 0.999960 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/models-1920x1080.png` | `http://10.0.5.8:30180/models?windowsCdpEvidenceTs=1783081324612` |
| `mlops` | pass | fail | 0.999979 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/mlops-1920x1080.png` | `http://10.0.5.8:30180/mlops?windowsCdpEvidenceTs=1783081324612` |
| `data-quality` | pass | fail | 0.999990 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/data-quality-1920x1080.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783081324612` |
| `playbooks` | pass | fail | 0.999957 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/playbooks-1920x1080.png` | `http://10.0.5.8:30180/playbooks?windowsCdpEvidenceTs=1783081324612` |
| `whitelist` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/whitelist-1920x1080.png` | `http://10.0.5.8:30180/whitelist?windowsCdpEvidenceTs=1783081324612` |
| `compliance` | pass | fail | 0.999864 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/compliance-1920x1080.png` | `http://10.0.5.8:30180/compliance?windowsCdpEvidenceTs=1783081324612` |
| `audit-log` | pass | fail | 0.999961 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/audit-log-1920x1080.png` | `http://10.0.5.8:30180/audit-log?windowsCdpEvidenceTs=1783081324612` |
| `notifications` | pass | fail | 0.999966 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/notifications-1920x1080.png` | `http://10.0.5.8:30180/notifications?windowsCdpEvidenceTs=1783081324612` |
| `settings` | pass | fail | 0.999974 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/settings-1920x1080.png` | `http://10.0.5.8:30180/settings?windowsCdpEvidenceTs=1783081324612` |
| `not-found` | pass | fail | 0.999980 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/not-found-1920x1080.png` | `http://10.0.5.8:30180/__codex_visual_not_found__?windowsCdpEvidenceTs=1783081324612` |
| `topics` | pass | fail | 0.999956 | `evidence/windows-chrome-cdp-20260703-visual-targets-r17-runtime-fix/visual-targets-r17-runtime-fix/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1783081324612` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

