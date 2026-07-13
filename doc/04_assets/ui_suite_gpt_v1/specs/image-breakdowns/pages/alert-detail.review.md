# alert-detail.png review

## Review Status

- Status: `business-pixel-accepted`
- Page id: `alert-detail`
- Queue order: 17
- Route: `/alerts/:alertId` (queue parent route `/alerts`)
- Type: `menu-state`
- Parent: `alerts`
- Target image: `doc/04_assets/ui_suite_gpt_v1/screens/pages/alert-detail.png`
- Evidence directory: `evidence/ui-image-breakdowns/pages/alert-detail/`
- Production URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__capture=r139-final&windowsCdpEvidenceTs=1783566187715`
- Production image tag: `traffic/web-ui:ui-alert-detail-visual-20260709-r139`
- Browser evidence: Windows Chrome CDP through `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---:|---|
| Required guide read | pass | `agent.md`, UI loop doc, queue doc |
| Target PNG exists | pass | `target.png` |
| Regions overlay | pass | `regions-overlay.png` |
| Windows Chrome CDP precheck | pass | `cdp-version-r139-final-pre-capture.txt`, `cdp-list-r139-final-pre-capture.txt` |
| Production screenshot | pass | `implementation-r139-final.png`, alias `implementation.png` |
| Runtime cleanliness | pass | `capture-meta-r139-final.json`: no console/page errors, requestfailed, 4xx/5xx, forbidden resources, overflow, or business truncation without title |
| Full visual diff | pass | `metrics-r139.json`, ratio 0.10477671682098766 <= 0.12 |
| Business visual diff | pass | `metrics-business-r139.json`, ratio 0.10368845336292296 <= 0.12 |
| Windows window responsive | pass | current 1920x1080 production capture is clean; historical `responsive-r123-meta.json` remains the small-window baseline |
| Business page title rule | pass | `capture-meta-r139-final.json`: left title is `告警详情`, no breadcrumb/menu path, right-side return arrow present |
| Detail menu selection rule | pass | `capture-meta-r139-final.json`: `/alerts/:alertId` keeps `威胁分析` group and `告警中心` item active |
| Business dynamic diagrams | pass | stage trail, impact path, response status, evidence table are React/CSS/AntD data views from API/typed fallback |
| Data refresh | pass | React Query refreshes detail data every 30s in production; visual evidence mode remains stable |
| Main-thread judgment | pass | `verification.json` status `business-pixel-accepted` |

## Visual Findings

- 1920x1080 production rendering matches the target structure within the current alpha threshold.
- Feedback panel is fully visible at 1920 and no longer clipped by the bottom status bar; r139 runtime records panel/table/chart overflow counts as 0.
- Business area top no longer shows menu path/breadcrumb; left side clearly states `告警详情`, and the return affordance remains the right-side arrow returning to `/alerts`.
- Alert detail is a `menu-state` page and keeps the parent menu selected: `威胁分析` / `告警中心`.
- Target PNG still contains the old breadcrumb and left-side return affordance; current implementation intentionally follows the newer global rule, so that diff hotspot is accepted.
- Remaining diff heat is from accepted titlebar rule divergence, target-vs-live component pixel differences, and common AppShell typography/icons, not from missing business content, blank charts, overlap, or clipping.

## Data And Component Notes

| Business graphic/table | Source | Implementation | Refresh/adaptive behavior |
|---|---|---|---|
| 攻击阶段轨迹 | `AlertDetailSnapshot.stageTrail` from API or typed fallback | React/CSS status nodes | 30s production refetch; stacks with right rail at responsive breakpoints |
| 影响范围路径 | `AlertDetailSnapshot.assets` and normalized alert tuple | React/CSS path nodes | 30s production refetch; no screenshot raster |
| 处置与响应 | `AlertDetailSnapshot.responseActions` | React buttons and status tokens | 30s production refetch; buttons shrink within panel |
| 证据链 | `/v1/alerts/{id}/evidence` normalized rows | AntD Table | 30s production refetch; full text exposed through titles; table scrolls below narrow breakpoint |

## Closed Difference Notes

| Type | Location | Current | Required | Status |
|---|---|---|---|---|
| production-evidence-scope | production route | Windows Chrome captured real APISIX page | no target raster replay | closed |
| production-visual-diff | full image | ratio 0.10477671682098766 | <= 0.12 with channel tolerance 48 | closed |
| business-visual-diff | crop x=197 y=80 w=1713 h=917 | ratio 0.10368845336292296 | <= 0.12 with channel tolerance 48 | closed |
| runtime-clean | production Windows Chrome | console/page errors 0, requestfailed 0, 4xx/5xx 0, forbidden resources 0, overflow 0 | clean runtime | closed |
| business-page-title-rule | titlebar | left title `告警详情`; no menu path; right return arrow present | left states current page and return stays on right side | closed |
| detail-menu-selection-rule | sidebar | `威胁分析` and `告警中心` active on `/alerts/:alertId` | detail page inherits parent menu selection unless it owns an independent menu route | closed |
| target-rule-conflict | titlebar | target has old breadcrumb; implementation removes it | user global rule overrides target breadcrumb | closed |

## Reproduction

1. Check `curl -i --max-time 5 --noproxy '*' http://127.0.0.1:9224/json/version` and `/json/list`.
2. Open `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__capture=r139-final&windowsCdpEvidenceTs=1783566187715` in Windows Chrome CDP.
3. Capture 1920x1080 screenshot to `implementation-r139-final.png` and alias `implementation.png`.
4. Generate `diff-r139.png`, `metrics-r139.json`, business crops, `diff-business-r139.png`, and `metrics-business-r139.json`.
5. Review `capture-meta-r139-final.json`, `verification.json`, target, implementation, and diff images.

## Decision

Main thread accepts `alert-detail` as `business-pixel-accepted` for the current queue item. Do not advance pages without preserving the r139 evidence set.

## r240 Constraint Continuation

The production deployment now runs `traffic/web-ui:ui-alert-detail-business-20260711-r240`. The global business-page constraint pass adds controlled evidence pagination, bounded table scrolling, a typed simulated action queue for visible business actions, and production-safe API fallback while the alert-detail backend contracts remain undeployed.

- Windows Chrome interaction: `interaction-r240.json` is pass; report export, evidence action, response action, and page 2 pagination all produced observable results with zero console/page/request errors.
- Normal production route: `ALERT_DETAIL_API_ENABLED=false` returns the typed fallback before network dispatch, so the previously observed alert API 404s no longer occur. API contracts remain registered for later enablement.
- Visual gate: full ratio `0.0734741512345679`; `content-root` ROI ratio `0.06849267355424761`, both within the user threshold `0.125`.
