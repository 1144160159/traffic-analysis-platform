# data-quality-flink-quality.png review

## Review Status

- Status: `business-pixel-accepted`
- Page id: `data-quality-flink-quality`
- Route/state: `/data-quality?tab=flink-quality`
- Type: `menu-state`, parent `data-quality`
- Target image: `doc/04_assets/ui_suite_gpt_v1/screens/pages/data-quality-flink-quality.png`
- Evidence dir: `evidence/ui-image-breakdowns/pages/data-quality-flink-quality/`
- Production URL: `http://10.0.5.8:30180/data-quality?tab=flink-quality&__codex_ui_breakdown_production=1&__capture=r117-final`
- Viewport: 1920 x 1080, Windows Chrome CDP `http://127.0.0.1:9224`
- Production image: `traffic/web-ui:ui-data-quality-flink-quality-visual-20260708-r117`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Target image read | pass | `target.png`, `regions-overlay.png` |
| Runtime clean | pass | `capture-meta-r117-final.json`: 0 console/page/request/HTTP errors, 0 overflow, 0 clipped-without-title |
| Full diff | pass | ratio `0.109515335648`, threshold `<= 0.13` |
| Business diff | pass | ratio `0.121231810542`, threshold `<= 0.13` |
| Dynamic business visuals | pass | Checkpoint/Watermark SVG, Backpressure heatmap, late-window bars, Sink sparklines use `PageSnapshot.visuals.dataQuality` typed fallback/API data |
| Text completeness | pass | key texts present; truncated business text has `title`/accessible label |
| Forbidden bitmap UI | pass | no target/implementation/whole-panel screenshots loaded as business UI |
| Docs synced | pass | JSON, review, measurement, verification updated for r117 |

## Main-Thread Judgment

Accepted as `business-pixel-accepted`. The r117 production screenshot matches the locked Flink 质量 target state closely enough for the current alpha visual gate: the business layout has the target tab row, filter row, seven KPI cards, Flink job table, Checkpoint/Watermark trend, Backpressure heatmap, late-window panel, failure Top 10 table, Sink quality cards, and Flink-specific right rail.

Remaining diff heat is concentrated in global live AppShell values, dense text anti-aliasing, and chart stroke rendering. These are acceptable because runtime is clean and both full/business metrics pass.

## Evidence

- `implementation-r117-final.png` and alias `implementation.png`
- `capture-meta-r117-final.json` and alias `capture-meta.json`
- `diff-r117.png` and alias `diff.png`
- `metrics-r117.json` and alias `metrics.json`
- `diff-business-r117.png`, `metrics-business-r117.json`
- `cdp-version-r117-final-pre-capture.txt`, `cdp-list-r117-final-pre-capture.txt`
- `verification.json`

## r271 Superseding Review

- Runtime/actions: pass; canvas `11`; endpoint/audit true; error arrays empty in `../data-quality/interaction-r271-all-tabs.json`.
- Fixed geometry: all-route and direct-click deltas `0`. Visual: `metrics-business-r271.json` ratio `0.12448878266629683 < 0.125`.
- Main-thread judgment: `business-pixel-accepted-r271`.

## Breakdown Depth Review

- Record gate: `breakdown-accepted`.
- Depth: 15 regions, 32 structured texts, 11 components, 6 icons, 12 tokens, 5 interactions.
- Checkpoint/Watermark and Backpressure are specified as dynamic ECharts; the job table is paginated and actions are testable.
- Target evidence exists; the near-threshold r271 business ratio remains explicitly recorded for later precision tuning.
