# Windows Chrome CDP Full Route Evidence

- Run ID: `windows-cdp-full-route-20260703105531`
- Result: `pass`
- Runtime routes passed: `28/28`
- Visual diff passed: `0/28`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/150.0.7871.47`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-20260703-visual-target-full-r8`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- No runtime blockers: no 4xx/5xx, requestfailed, console/pageerror, page error alerts, or page-level horizontal overflow.

## Visual Diff Gaps

- `data-quality`: mismatch=0.999990, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/data-quality-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/data-quality/diff-1920.png`
- `encrypted-traffic`: mismatch=0.999984, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/encrypted-traffic-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/encrypted-traffic/diff-1920.png`
- `attack-chains`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/attack-chains-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/attack-chains/diff-1920.png`
- `forensics`: mismatch=0.999983, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/forensics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/forensics/diff-1920.png`
- `not-found`: mismatch=0.999982, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/not-found-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/not-found/diff-1920.png`
- `mlops`: mismatch=0.999980, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/mlops-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/mlops/diff-1920.png`
- `dashboard`: mismatch=0.999979, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/dashboard-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/dashboard/diff-1920.png`
- `deployments`: mismatch=0.999977, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/deployments-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/deployments/diff-1920.png`
- `settings`: mismatch=0.999977, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/settings-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/settings/diff-1920.png`
- `baselines`: mismatch=0.999974, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/baselines-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/baselines/diff-1920.png`
- `fusion`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/fusion-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/fusion/diff-1920.png`
- `screen`: mismatch=0.999973, screenshot=`evidence/windows-chrome-cdp-20260703-visual-target-full-r8/screen-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-visual-target-latest-r8/screen/diff-1920.png`
- ... 16 more visual diff gaps

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `login` | pass | fail | 0.999301 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/login-1920x1080.png` | `http://10.0.5.8:30180/login?windowsCdpEvidenceTs=1783076131015` |
| `screen` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/screen-1920x1080.png` | `http://10.0.5.8:30180/screen?windowsCdpEvidenceTs=1783076131015` |
| `dashboard` | pass | fail | 0.999979 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/dashboard-1920x1080.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1783076131015` |
| `alerts` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/alerts-1920x1080.png` | `http://10.0.5.8:30180/alerts?windowsCdpEvidenceTs=1783076131015` |
| `alert-detail` | pass | fail | 0.999970 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/alert-detail-1920x1080.png` | `http://10.0.5.8:30180/alerts/alert-default-1782752318016-1dd589c4?windowsCdpEvidenceTs=1783076131015` |
| `campaigns` | pass | fail | 0.999972 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/campaigns-1920x1080.png` | `http://10.0.5.8:30180/campaigns?windowsCdpEvidenceTs=1783076131015` |
| `campaign-detail` | pass | fail | 0.999940 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/campaign-detail-1920x1080.png` | `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1782729598739-e1d2dc37?windowsCdpEvidenceTs=1783076131015` |
| `attack-chains` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/attack-chains-1920x1080.png` | `http://10.0.5.8:30180/attack-chains?windowsCdpEvidenceTs=1783076131015` |
| `encrypted-traffic` | pass | fail | 0.999984 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/encrypted-traffic-1920x1080.png` | `http://10.0.5.8:30180/encrypted-traffic?windowsCdpEvidenceTs=1783076131015` |
| `forensics` | pass | fail | 0.999983 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/forensics-1920x1080.png` | `http://10.0.5.8:30180/forensics?windowsCdpEvidenceTs=1783076131015` |
| `assets` | pass | fail | 0.999971 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/assets-1920x1080.png` | `http://10.0.5.8:30180/assets?windowsCdpEvidenceTs=1783076131015` |
| `graph` | pass | fail | 0.999962 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1783076131015` |
| `fusion` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/fusion-1920x1080.png` | `http://10.0.5.8:30180/fusion?windowsCdpEvidenceTs=1783076131015` |
| `baselines` | pass | fail | 0.999974 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/baselines-1920x1080.png` | `http://10.0.5.8:30180/baselines?windowsCdpEvidenceTs=1783076131015` |
| `probes` | pass | fail | 0.999966 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/probes-1920x1080.png` | `http://10.0.5.8:30180/probes?windowsCdpEvidenceTs=1783076131015` |
| `rules` | pass | fail | 0.999973 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/rules-1920x1080.png` | `http://10.0.5.8:30180/rules?windowsCdpEvidenceTs=1783076131015` |
| `deployments` | pass | fail | 0.999977 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/deployments-1920x1080.png` | `http://10.0.5.8:30180/deployments?windowsCdpEvidenceTs=1783076131015` |
| `models` | pass | fail | 0.999960 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/models-1920x1080.png` | `http://10.0.5.8:30180/models?windowsCdpEvidenceTs=1783076131015` |
| `mlops` | pass | fail | 0.999980 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/mlops-1920x1080.png` | `http://10.0.5.8:30180/mlops?windowsCdpEvidenceTs=1783076131015` |
| `data-quality` | pass | fail | 0.999990 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/data-quality-1920x1080.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1783076131015` |
| `playbooks` | pass | fail | 0.999957 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/playbooks-1920x1080.png` | `http://10.0.5.8:30180/playbooks?windowsCdpEvidenceTs=1783076131015` |
| `whitelist` | pass | fail | 0.999963 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/whitelist-1920x1080.png` | `http://10.0.5.8:30180/whitelist?windowsCdpEvidenceTs=1783076131015` |
| `compliance` | pass | fail | 0.999864 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/compliance-1920x1080.png` | `http://10.0.5.8:30180/compliance?windowsCdpEvidenceTs=1783076131015` |
| `audit-log` | pass | fail | 0.999962 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/audit-log-1920x1080.png` | `http://10.0.5.8:30180/audit-log?windowsCdpEvidenceTs=1783076131015` |
| `notifications` | pass | fail | 0.999965 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/notifications-1920x1080.png` | `http://10.0.5.8:30180/notifications?windowsCdpEvidenceTs=1783076131015` |
| `settings` | pass | fail | 0.999977 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/settings-1920x1080.png` | `http://10.0.5.8:30180/settings?windowsCdpEvidenceTs=1783076131015` |
| `not-found` | pass | fail | 0.999982 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/not-found-1920x1080.png` | `http://10.0.5.8:30180/__codex_visual_not_found__?windowsCdpEvidenceTs=1783076131015` |
| `topics` | pass | fail | 0.999957 | `evidence/windows-chrome-cdp-20260703-visual-target-full-r8/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1783076131015` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

