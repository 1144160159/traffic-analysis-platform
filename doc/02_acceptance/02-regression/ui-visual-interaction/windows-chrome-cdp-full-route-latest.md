# Windows Chrome CDP Full Route Evidence

- Run ID: `windows-cdp-full-route-20260806014101`
- Result: `fail`
- Runtime routes passed: `25/28`
- Visual diff passed: `0/28`
- CDP URL: `http://127.0.0.1:9224`
- Browser: `Chrome/151.0.7922.71`
- Viewport: `1920x1080`
- Evidence dir: `evidence/windows-chrome-cdp-full-route-latest`
- Acceptance dir: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest`

This evidence is captured through Windows Chrome CDP. It is intentionally separate from the older Codex Desktop extension receiver gate.

## Runtime Findings

- `dashboard`: 1 error alerts
- `data-quality`: 2 error alerts
- `playbooks`: 2 bad responses; 2 console errors; 1 error alerts

## Visual Diff Gaps

- `data-quality`: mismatch=0.999985, screenshot=`evidence/windows-chrome-cdp-full-route-latest/data-quality-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/data-quality/diff-1920.png`
- `fusion`: mismatch=0.999970, screenshot=`evidence/windows-chrome-cdp-full-route-latest/fusion-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/fusion/diff-1920.png`
- `screen`: mismatch=0.999957, screenshot=`evidence/windows-chrome-cdp-full-route-latest/screen-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/screen/diff-1920.png`
- `forensics`: mismatch=0.999953, screenshot=`evidence/windows-chrome-cdp-full-route-latest/forensics-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/forensics/diff-1920.png`
- `baselines`: mismatch=0.999945, screenshot=`evidence/windows-chrome-cdp-full-route-latest/baselines-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/baselines/diff-1920.png`
- `attack-chains`: mismatch=0.999943, screenshot=`evidence/windows-chrome-cdp-full-route-latest/attack-chains-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/attack-chains/diff-1920.png`
- `mlops`: mismatch=0.999934, screenshot=`evidence/windows-chrome-cdp-full-route-latest/mlops-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/mlops/diff-1920.png`
- `encrypted-traffic`: mismatch=0.999929, screenshot=`evidence/windows-chrome-cdp-full-route-latest/encrypted-traffic-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/encrypted-traffic/diff-1920.png`
- `assets`: mismatch=0.999927, screenshot=`evidence/windows-chrome-cdp-full-route-latest/assets-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/assets/diff-1920.png`
- `campaigns`: mismatch=0.999923, screenshot=`evidence/windows-chrome-cdp-full-route-latest/campaigns-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/campaigns/diff-1920.png`
- `graph`: mismatch=0.999921, screenshot=`evidence/windows-chrome-cdp-full-route-latest/graph-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/graph/diff-1920.png`
- `probes`: mismatch=0.999921, screenshot=`evidence/windows-chrome-cdp-full-route-latest/probes-1920x1080.png`, diff=`doc/02_acceptance/02-regression/ui-visual-interaction/windows-cdp-latest/probes/diff-1920.png`
- ... 16 more visual diff gaps

## Route Evidence

| route | runtime | visual diff | mismatch | screenshot | final URL |
|---|---:|---:|---:|---|---|
| `login` | pass | fail | 0.992631 | `evidence/windows-chrome-cdp-full-route-latest/login-1920x1080.png` | `http://10.0.5.8:30180/login?windowsCdpEvidenceTs=1785980461152` |
| `screen` | pass | fail | 0.999957 | `evidence/windows-chrome-cdp-full-route-latest/screen-1920x1080.png` | `http://10.0.5.8:30180/screen?windowsCdpEvidenceTs=1785980461152` |
| `dashboard` | fail | fail | 0.999917 | `evidence/windows-chrome-cdp-full-route-latest/dashboard-1920x1080.png` | `http://10.0.5.8:30180/dashboard?windowsCdpEvidenceTs=1785980461152` |
| `alerts` | pass | fail | 0.999903 | `evidence/windows-chrome-cdp-full-route-latest/alerts-1920x1080.png` | `http://10.0.5.8:30180/alerts?windowsCdpEvidenceTs=1785980461152` |
| `alert-detail` | pass | fail | 0.999888 | `evidence/windows-chrome-cdp-full-route-latest/alert-detail-1920x1080.png` | `http://10.0.5.8:30180/alerts/alert-default-1785975666197-66142c30?windowsCdpEvidenceTs=1785980461152` |
| `campaigns` | pass | fail | 0.999923 | `evidence/windows-chrome-cdp-full-route-latest/campaigns-1920x1080.png` | `http://10.0.5.8:30180/campaigns?windowsCdpEvidenceTs=1785980461152` |
| `campaign-detail` | pass | fail | 0.999689 | `evidence/windows-chrome-cdp-full-route-latest/campaign-detail-1920x1080.png` | `http://10.0.5.8:30180/campaigns/campaign-exfil-default-1785975636176-88a7cfbc?windowsCdpEvidenceTs=1785980461152` |
| `attack-chains` | pass | fail | 0.999943 | `evidence/windows-chrome-cdp-full-route-latest/attack-chains-1920x1080.png` | `http://10.0.5.8:30180/attack-chains?windowsCdpEvidenceTs=1785980461152` |
| `encrypted-traffic` | pass | fail | 0.999929 | `evidence/windows-chrome-cdp-full-route-latest/encrypted-traffic-1920x1080.png` | `http://10.0.5.8:30180/encrypted-traffic?windowsCdpEvidenceTs=1785980461152` |
| `forensics` | pass | fail | 0.999953 | `evidence/windows-chrome-cdp-full-route-latest/forensics-1920x1080.png` | `http://10.0.5.8:30180/forensics?windowsCdpEvidenceTs=1785980461152` |
| `assets` | pass | fail | 0.999927 | `evidence/windows-chrome-cdp-full-route-latest/assets-1920x1080.png` | `http://10.0.5.8:30180/assets?windowsCdpEvidenceTs=1785980461152&tab=endpoint&assetId=491edde0-98db-8deb-74db-59f8e7fb5200` |
| `graph` | pass | fail | 0.999921 | `evidence/windows-chrome-cdp-full-route-latest/graph-1920x1080.png` | `http://10.0.5.8:30180/graph?windowsCdpEvidenceTs=1785980461152` |
| `fusion` | pass | fail | 0.999970 | `evidence/windows-chrome-cdp-full-route-latest/fusion-1920x1080.png` | `http://10.0.5.8:30180/fusion?windowsCdpEvidenceTs=1785980461152` |
| `baselines` | pass | fail | 0.999945 | `evidence/windows-chrome-cdp-full-route-latest/baselines-1920x1080.png` | `http://10.0.5.8:30180/baselines?windowsCdpEvidenceTs=1785980461152` |
| `probes` | pass | fail | 0.999921 | `evidence/windows-chrome-cdp-full-route-latest/probes-1920x1080.png` | `http://10.0.5.8:30180/probes?windowsCdpEvidenceTs=1785980461152` |
| `rules` | pass | fail | 0.999757 | `evidence/windows-chrome-cdp-full-route-latest/rules-1920x1080.png` | `http://10.0.5.8:30180/rules?windowsCdpEvidenceTs=1785980461152` |
| `deployments` | pass | fail | 0.999917 | `evidence/windows-chrome-cdp-full-route-latest/deployments-1920x1080.png` | `http://10.0.5.8:30180/deployments?windowsCdpEvidenceTs=1785980461152` |
| `models` | pass | fail | 0.999873 | `evidence/windows-chrome-cdp-full-route-latest/models-1920x1080.png` | `http://10.0.5.8:30180/models?windowsCdpEvidenceTs=1785980461152` |
| `mlops` | pass | fail | 0.999934 | `evidence/windows-chrome-cdp-full-route-latest/mlops-1920x1080.png` | `http://10.0.5.8:30180/mlops?windowsCdpEvidenceTs=1785980461152` |
| `data-quality` | fail | fail | 0.999985 | `evidence/windows-chrome-cdp-full-route-latest/data-quality-1920x1080.png` | `http://10.0.5.8:30180/data-quality?windowsCdpEvidenceTs=1785980461152` |
| `playbooks` | fail | fail | 0.999903 | `evidence/windows-chrome-cdp-full-route-latest/playbooks-1920x1080.png` | `http://10.0.5.8:30180/playbooks?windowsCdpEvidenceTs=1785980461152` |
| `whitelist` | pass | fail | 0.999911 | `evidence/windows-chrome-cdp-full-route-latest/whitelist-1920x1080.png` | `http://10.0.5.8:30180/whitelist?windowsCdpEvidenceTs=1785980461152` |
| `compliance` | pass | fail | 0.999821 | `evidence/windows-chrome-cdp-full-route-latest/compliance-1920x1080.png` | `http://10.0.5.8:30180/compliance?windowsCdpEvidenceTs=1785980461152` |
| `audit-log` | pass | fail | 0.999898 | `evidence/windows-chrome-cdp-full-route-latest/audit-log-1920x1080.png` | `http://10.0.5.8:30180/audit-log?windowsCdpEvidenceTs=1785980461152` |
| `notifications` | pass | fail | 0.999581 | `evidence/windows-chrome-cdp-full-route-latest/notifications-1920x1080.png` | `http://10.0.5.8:30180/notifications?windowsCdpEvidenceTs=1785980461152` |
| `settings` | pass | fail | 0.999921 | `evidence/windows-chrome-cdp-full-route-latest/settings-1920x1080.png` | `http://10.0.5.8:30180/settings?windowsCdpEvidenceTs=1785980461152` |
| `not-found` | pass | fail | 0.999609 | `evidence/windows-chrome-cdp-full-route-latest/not-found-1920x1080.png` | `http://10.0.5.8:30180/__codex_visual_not_found__?windowsCdpEvidenceTs=1785980461152` |
| `topics` | pass | fail | 0.999872 | `evidence/windows-chrome-cdp-full-route-latest/topics-1920x1080.png` | `http://10.0.5.8:30180/topics?windowsCdpEvidenceTs=1785980461152` |

## Reproduce

```bash
curl http://127.0.0.1:9224/json/version
curl http://127.0.0.1:9224/json/list
env -u HTTP_PROXY -u HTTPS_PROXY -u ALL_PROXY -u http_proxy -u https_proxy -u all_proxy \
  node tests/e2e/ui_windows_chrome_cdp_full_route_capture.mjs
```

