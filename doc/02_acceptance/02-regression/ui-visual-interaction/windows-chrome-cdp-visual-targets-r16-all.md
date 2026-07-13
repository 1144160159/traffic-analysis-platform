# Windows Chrome CDP Full Route Evidence

- Run ID: `visual-targets-r16-all`
- Result: `fail`
- Runtime routes passed: `26/28`
- Visual diff passed: `0/28`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `baselines`: 1 bad responses; 1 console errors; 1 error alerts
- `topics`: 1 bad responses; 1 console errors

## Visual Diff Gaps

- `data-quality`: mismatch=0.999990, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/data-quality-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/data-quality/diff-1920.png`
- `attack-chains`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/attack-chains-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/attack-chains/diff-1920.png`
- `encrypted-traffic`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/encrypted-traffic-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/encrypted-traffic/diff-1920.png`
- `forensics`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/forensics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/forensics/diff-1920.png`
- `not-found`: mismatch=0.999980, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/not-found-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/not-found/diff-1920.png`
- `dashboard`: mismatch=0.999979, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/dashboard-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/dashboard/diff-1920.png`
- `mlops`: mismatch=0.999979, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/mlops-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/mlops/diff-1920.png`
- `fusion`: mismatch=0.999976, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/fusion-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/fusion/diff-1920.png`
- `deployments`: mismatch=0.999976, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/deployments-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/deployments/diff-1920.png`
- `settings`: mismatch=0.999974, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/settings-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/settings/diff-1920.png`
- `rules`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/rules-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/rules/diff-1920.png`
- `screen`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/screen-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-targets-r16-all/screen/diff-1920.png`
- ... 16 more visual diff gaps

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `login` | pass | fail | 0.999301 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/login-1920x1080.png` | `http://10.0.5.8:30180/login?windowsCdpEvidenceTs=1783080729835` |
| `screen` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/screen-1920x1080.png` | `http://10.0.5.8:30180/screen?windowsCdpEvidenceTs=1783080729835` |
| `dashboard` | pass | fail | 0.999979 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/dashboard-1920x1080.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1783080729835` |
| `alerts` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/alerts-1920x1080.png` | `http://10.0.5.8:30180/alerts?windowsCdpEvidenceTs=1783080729835` |
| `alert-detail` | pass | fail | 0.999968 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/alert-detail-1920x1080.png` | `http://10.0.5.8:30180/alerts/alert-default-1782752318016-1dd589c4?windowsCdpEvidenceTs=1783080729835` |
| `campaigns` | pass | fail | 0.999971 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/campaigns-1920x1080.png` | `http://10.0.5.8:30180/campaigns?windowsCdpEvidenceTs=1783080729835` |
| `campaign-detail` | pass | fail | 0.999939 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/campaign-detail-1920x1080.png` | `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?windowsCdpEvidenceTs=1783080729835` |
| `attack-chains` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/attack-chains-1920x1080.png` | `http://10.0.5.8:30180/attack-chains?windowsCdpEvidenceTs=1783080729835` |
| `encrypted-traffic` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/encrypted-traffic-1920x1080.png` | `http://10.0.5.8:30180/encrypted-traffic?windowsCdpEvidenceTs=1783080729835` |
| `forensics` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/forensics-1920x1080.png` | `http://10.0.5.8:30180/forensics?windowsCdpEvidenceTs=1783080729835` |
| `assets` | pass | fail | 0.999971 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/assets-1920x1080.png` | `http://10.0.5.8:30180/assets?windowsCdpEvidenceTs=1783080729835` |
| `graph` | pass | fail | 0.999964 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1783080729835` |
| `fusion` | pass | fail | 0.999976 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/fusion-1920x1080.png` | `http://10.0.5.8:30180/fusion?windowsCdpEvidenceTs=1783080729835` |
| `baselines` | fail | fail | 0.999970 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/baselines-1920x1080.png` | `http://10.0.5.8:30180/baselines?windowsCdpEvidenceTs=1783080729835` |
| `probes` | pass | fail | 0.999965 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/probes-1920x1080.png` | `http://10.0.5.8:30180/probes?windowsCdpEvidenceTs=1783080729835` |
| `rules` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/rules-1920x1080.png` | `http://10.0.5.8:30180/rules?windowsCdpEvidenceTs=1783080729835` |
| `deployments` | pass | fail | 0.999976 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/deployments-1920x1080.png` | `http://10.0.5.8:30180/deployments?windowsCdpEvidenceTs=1783080729835` |
| `models` | pass | fail | 0.999960 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/models-1920x1080.png` | `http://10.0.5.8:30180/models?windowsCdpEvidenceTs=1783080729835` |
| `mlops` | pass | fail | 0.999979 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/mlops-1920x1080.png` | `http://10.0.5.8:30180/mlops?windowsCdpEvidenceTs=1783080729835` |
| `data-quality` | pass | fail | 0.999990 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/data-quality-1920x1080.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783080729835` |
| `playbooks` | pass | fail | 0.999957 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/playbooks-1920x1080.png` | `http://10.0.5.8:30180/playbooks?windowsCdpEvidenceTs=1783080729835` |
| `whitelist` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/whitelist-1920x1080.png` | `http://10.0.5.8:30180/whitelist?windowsCdpEvidenceTs=1783080729835` |
| `compliance` | pass | fail | 0.999864 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/compliance-1920x1080.png` | `http://10.0.5.8:30180/compliance?windowsCdpEvidenceTs=1783080729835` |
| `audit-log` | pass | fail | 0.999961 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/audit-log-1920x1080.png` | `http://10.0.5.8:30180/audit-log?windowsCdpEvidenceTs=1783080729835` |
| `notifications` | pass | fail | 0.999968 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/notifications-1920x1080.png` | `http://10.0.5.8:30180/notifications?windowsCdpEvidenceTs=1783080729835` |
| `settings` | pass | fail | 0.999974 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/settings-1920x1080.png` | `http://10.0.5.8:30180/settings?windowsCdpEvidenceTs=1783080729835` |
| `not-found` | pass | fail | 0.999980 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/not-found-1920x1080.png` | `http://10.0.5.8:30180/__codex_visual_not_found__?windowsCdpEvidenceTs=1783080729835` |
| `topics` | fail | fail | 0.999956 | `evidence/windows-chrome-cdp-20260703-visual-targets-r16-all/visual-targets-r16-all/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1783080729835` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

