# alert-detail-evidence-logs review

## Review Status

- Status: `business-pixel-accepted`
- Page ID: `alert-detail-evidence-logs`
- Route: `/alerts/AL-20260620-000123?evidenceView=logs`
- Type: `menu-state`
- Parent: `alerts`
- Production URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-logs&evidenceView=logs&__capture=r146-final`
- Image tag: `traffic/web-ui:ui-alert-detail-evidence-logs-visual-20260709-r146`
- Viewport: `1920 x 1080`
- Browser evidence: Windows Chrome CDP `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, page loop, queue docs |
| Target PNG read | pass | `target.png`, direct visual inspection |
| CDP precheck | pass | `cdp-version-r146-final-pre-capture.txt`, `cdp-list-r146-final-pre-capture.txt` |
| Production screenshot | pass | `implementation-r146-final.png`, alias `implementation.png` |
| Runtime health | pass | `capture-meta-r146-final.json`: 0 console errors, 0 page errors, 0 requestfailed, 0 HTTP 4xx/5xx |
| Forbidden resources | pass | no canonical/screen/target/overlay/implementation resources requested |
| Text completeness | pass | `capture-meta-r146-final.json`, `missingTexts=[]` |
| Overflow | pass | root/page/card/table/detail have no horizontal overflow or viewport escape |
| Visual diff | pass | `metrics-r146.json`, mismatch ratio `0.08773582175925926 <= 0.12` |
| Business diff | pass | `metrics-business-r146.json`, mismatch ratio `0.08773582175925926 <= 0.12` |
| noBitmap gate | pass | `npm --prefix web/ui test -- --run src/routes/noBitmapUi.test.ts` |
| Build | pass | `npm --prefix web/ui run build` |
| Production image confirmed | pass | Deployment and Pod image are r146 |

## Visual Findings

- Target is a focused evidence-chain log panel, not a full AppShell page.
- Header and tabs match target structure: `证据链（6）`, active `日志 1`, and sibling tabs `全部 6 / PCAP 1 / Session 2 / 图谱路径 1 / 文件 1`.
- Evidence table columns, single log row, generated time, status, search/view actions, highlighted fields, source tags, and footer link are present.
- Diff hotspots are accepted alpha differences from target raster blur/ghosting versus sharper React text and icon rendering; no business structure or text mismatch remains.

## Data And Resource Review

- Data source: `AlertDetailEvidenceRow.logEvidence` from `web/ui/src/services/alertDetailApi.ts`, with typed fallback when API fields are absent.
- Component: `AlertEvidenceLogsFocusView` in `web/ui/src/pages/AlertDetailPage.tsx`.
- Styling: `.taf-alert-evidence-logs-*` in `web/ui/src/styles/pages.css`.
- Business dynamic diagram: none on this page. SVG/AntD icons are independent symbols; the table/chips/tags are data-driven React elements.
- Bitmap boundary: no target UI image, full card, full table, or business panel is loaded as an app resource.

## Main-Thread Judgment

Accepted as `business-pixel-accepted`. The r146 production screenshot matches the target business panel within threshold, runtime is clean, text is complete, and evidence artifacts are complete.
