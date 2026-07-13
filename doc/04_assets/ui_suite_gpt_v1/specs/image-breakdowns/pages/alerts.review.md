# alerts.png review

## Review Status

- Status: `business-pixel-accepted`
- Target image reviewed directly: yes
- Production URL: `http://10.0.5.8:30180/alerts`
- Route: `/alerts`
- Page type: `menu-route`
- Parent: ``
- Viewport: `1920 x 1080`
- Image tag: `traffic/web-ui:ui-alerts-visual-20260709-r137`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, page loop docs, queue JSON/MD, traffic-platform skill |
| Queue lock | pass | order 16 `alerts`, route `/alerts` |
| Target PNG exists | pass | `doc/04_assets/ui_suite_gpt_v1/screens/pages/alerts.png` |
| Direct visual inspection | pass | target and `implementation-r137-final.png` inspected |
| Windows Chrome CDP precheck | pass | `cdp-version-r137-final-pre-capture.txt`, `cdp-list-r137-final-pre-capture.txt` |
| Production deployment | pass | Deployment image `traffic/web-ui:ui-alerts-visual-20260709-r137` |
| Runtime cleanliness | pass | `capture-meta-r137-final.json`: no console/page errors, no requestfailed, no 4xx/5xx |
| Layout overflow | pass | root/panel/table/chart overflow counts all `0` |
| Dynamic business visuals | pass | risk ring/table/KPI/timeline use API or typed fallback data, no screenshot substitution |
| Text completeness | pass | key text present; 41 ellipsis cells all have title text; clipped-without-title `0` |
| Full visual diff | pass | `metrics-r137.json`: ratio `0.1073755787037037`, threshold `0.12`, tolerance `48` |
| Business visual diff | pass | `metrics-business-r137.json`: ratio `0.10505398132568892`, threshold `0.12`, tolerance `48` |
| Main-thread judgment | pass | `verification.json` status `business-pixel-accepted` |

## Visual Findings

- The page matches the target business layout: left title/KPI/filter/table stack and right detail rail aligned from y=80.
- Final runtime boxes: business root `x=198 y=80 w=1712 h=917`, main column `x=198 y=80 w=1164 h=917`, right rail `x=1370 y=80 w=540 h=917`, table panel `x=198 y=468 w=1164 h=529`.
- The target text ledger is represented in production: 告警中心, 7 KPI values, 告警列表（共 425 条）, selected alert AL-20260620-000123, risk score 92/100, timeline, cluster cards and 处理与反馈.
- Diff hotspots are primarily font antialiasing, public AppShell differences and AntD control rendering; no wrong page, blank chart, module omission, bottom-bar obstruction, table overflow or right-rail displacement remains.

## Dynamic Visual Classification

| Type | Implementation | Data Source | Refresh | Screenshot Substitute |
|---|---|---|---|---|
| Risk score ring | React + CSS conic-gradient | `__riskScore` from `fetchPageSnapshot("alerts")` API/typed fallback | deterministic in visual mode; normal route refetches every 15s | no |
| KPI strip | React metric cards | `PageSnapshot.metrics` | deterministic in visual mode; normal route refetches every 15s | no |
| Alert table | AntD Table | `PageSnapshot.rows/total` | paginated table snapshot | no |
| Timeline/cluster/feedback | React controls | typed fallback/API snapshot fields | deterministic in visual mode; normal route refetches every 15s | no |

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/alerts/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/alerts/regions-overlay.png`
- Implementation: `evidence/ui-image-breakdowns/pages/alerts/implementation-r137-final.png` and alias `evidence/ui-image-breakdowns/pages/alerts/implementation.png`
- Diff: `evidence/ui-image-breakdowns/pages/alerts/diff-r137.png` and alias `evidence/ui-image-breakdowns/pages/alerts/diff.png`
- Business crop: `evidence/ui-image-breakdowns/pages/alerts/target-business-r137.png`, `evidence/ui-image-breakdowns/pages/alerts/implementation-business-r137.png`
- Business diff: `evidence/ui-image-breakdowns/pages/alerts/diff-business-r137.png` and alias `evidence/ui-image-breakdowns/pages/alerts/diff-business.png`
- Metrics: `evidence/ui-image-breakdowns/pages/alerts/metrics-r137.json`, `evidence/ui-image-breakdowns/pages/alerts/metrics-business-r137.json`
- Runtime: `evidence/ui-image-breakdowns/pages/alerts/capture-meta-r137-final.json` and alias `evidence/ui-image-breakdowns/pages/alerts/capture-meta.json`
- Verification: `evidence/ui-image-breakdowns/pages/alerts/verification.json`
- CDP precheck: `evidence/ui-image-breakdowns/pages/alerts/cdp-version-r137-final-pre-capture.txt`, `evidence/ui-image-breakdowns/pages/alerts/cdp-list-r137-final-pre-capture.txt`

## Decision

Main thread accepts `alerts` as `business-pixel-accepted`. The page can advance to queue item 17 `alert-detail` only after this evidence set is retained.

## r255 Production Semantic Review

- Release: `traffic/web-ui:ui-alert-triage-interactions-20260711-r255`.
- Dynamic chart: pass. The selected-alert risk score uses an ECharts gauge canvas from the page snapshot/typed fallback `__riskScore`; the former CSS-only conic-gradient dial is no longer the business chart.
- Business controls: pass. Controlled 10-row pagination, stable table scrolling, filters, view save, batch actions, row actions, cluster actions and feedback all have observable state or confirmation Drawer behavior.
- API reservation: pass. The Drawer maps report export, evidence access, response requests and investigation notes to `pageApiPlans["alert-detail"]`, with the selected alert ID and audit event shown before simulated submission.
- Windows Chrome runtime: pass. `interaction-r255.json` records one gauge canvas, page two, `overflow-y: scroll`, view-save, row and cluster actions, and zero HTTP/request/console/page errors.
- Business ROI: pass. `metrics-business-r255.json` scores `content-root (197,80,1713,917)` at `0.07573173518815957 <= 0.125`.
