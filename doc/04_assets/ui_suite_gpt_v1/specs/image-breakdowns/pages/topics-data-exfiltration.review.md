# topics-data-exfiltration.png review

## Review Status

- Status: `pixel-accepted`
- Acceptance profile: business-region visual gate; content/layout and dynamic business diagrams are blocking, while brightness/transparency remain non-blocking for alpha.
- Target image reviewed directly: yes
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`; the Xshell tunnel was restored before r103 capture after a stale SSH forward timed out.
- Production URL: `http://10.0.5.8:30180/topics?topic=exfil&tab=exfil&__codex_ui_breakdown_production=1&windowsCdpEvidenceTs=1783404852077`
- Production viewport: screenshot 1920 x 1080; DPR 1.
- Business crop viewport: 1722 x 917.
- Deployed image: `traffic/web-ui:ui-topic-business-rebuild-20260707-r103`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pages verification loop were read before edits |
| Menu-order queue | pass | `topics-data-exfiltration` follows accepted `topics-encrypted-tunnel` |
| Business-region only rule | pass | Public shell/sidebar/topbar/bottombar were not intentionally redesigned |
| Dynamic business diagram rule | pass | Sankey, line, bar, and donut charts are ECharts/React components fed by API/adapted fallback data |
| ECharts/module self-adaptive rule | pass | r103 runtime measured 5 canvases and `clippedCanvases=0` |
| Screenshot asset gate | pass | No target, overlay, screenshot, card, panel, or business diagram bitmap resource is used |
| Windows Chrome CDP precheck | pass | `cdp-version-r103-pre-capture.txt`, `cdp-list-r103-pre-capture.txt` |
| Production route screenshot | pass | `implementation-r103-business-layout.png` and alias `implementation.png` captured from real `30180` route |
| Runtime gate | pass | No console/pageerror/requestfailed/forbidden target resource; `passedRuntime=true` |
| Visual diff | pass | `pixel_mismatch_ratio=0.11803310041201362`, `max_pixel_ratio=0.13`, `channel_tolerance=90` |
| Auxiliary review | pass | Automated runtime/visual review passed; manual screenshot inspection confirmed business charts no longer overflow |
| Main-thread judgment | pass | `verification.json` records `pixel-accepted`, `accepted=true` |

## Visual Findings

- r103 keeps the business area on topic tabs only: `加密隧道专题 / 数据外传专题 / APT/战役专题`; no business-region breadcrumb is shown.
- r103 restores the data exfiltration content: topic facts, 8 KPI strip, Sankey exfiltration path board, destination ASN table, sensitive type donut, abnormal upload trend, protocol donut, account/service bar chart, evidence table, delivery/evidence/report/action right rail.
- The two donut ECharts that overflowed in r101/r102 were fixed by compact chart geometry and stricter card-internal legend overflow constraints.
- Public topbar still shows the global site selector. Business-region acceptance checks the topic fact `站点：主校区`, so the shared topbar was not altered.

## Evidence

- Target: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/target.png`
- Regions overlay: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/regions-overlay.png`
- Implementation screenshot: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/implementation-r103-business-layout.png`
- Implementation alias: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/implementation.png`
- Business target crop: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/target-business-r103.png`
- Business implementation crop: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/implementation-business-r103.png`
- Diff: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/diff.png`
- Diff archive: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/diff-business-r103.png`
- Metrics: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/metrics.json`
- Metrics archive: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/metrics-business-r103.json`
- Measurement: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/measurement.json`
- Capture metadata: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/capture-meta-r103-business-layout.json`
- Capture alias: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/capture-meta.json`
- Verification: `evidence/ui-image-breakdowns/pages/topics-data-exfiltration/verification.json`

## Reproduction

1. Ensure production `web-ui` is `traffic/web-ui:ui-topic-business-rebuild-20260707-r103`.
2. Check `curl -i --max-time 5 --noproxy '*' http://127.0.0.1:9224/json/version` and `/json/list`.
3. Open `http://10.0.5.8:30180/topics?topic=exfil&tab=exfil&__codex_ui_breakdown_production=1` in Windows Chrome through `connectOverCDP('http://127.0.0.1:9224')`.
4. Capture 1920 x 1080 screenshot and crop business region `x=198,y=80,w=1722,h=917`.
5. Run `tests/e2e/ui_visual_diff_metrics.py` with `--max-pixel-ratio 0.13 --channel-tolerance 90`.
6. Review `implementation-r103-business-layout.png`, `diff.png`, `metrics.json`, `capture-meta-r103-business-layout.json`, and `verification.json`.

## Decision

The `topics-data-exfiltration` business region is accepted for the current alpha visual gate. Continue the pages queue with `topics-apt-campaign`.
