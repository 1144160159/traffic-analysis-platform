# baselines-port.png review

## Review Status

- Status: `evidence-ready`
- Target image reviewed directly: yes
- Scope: single canonical PNG only
- Evidence target: `evidence/ui-image-breakdowns/pages/baselines-port/target.png`
- Browser evidence path: Windows Chrome CDP through `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `agent.md`, traffic-platform skill, and pixel-perfect plan read before edits |
| Target PNG exists | pass | Source PNG is canonical and recorded in JSON |
| Direct visual inspection | pass | Layout category, primary regions, text ledger, icons, token mapping and interactions recorded |
| Single-image scope | pass | One markdown, one JSON, one review, one evidence directory |
| Markdown breakdown | pass | Required sections present |
| JSON breakdown | pass | regions/texts/components/icons/tokens/interactions populated |
| Evidence target copy | pass | `target.png` produced under evidence directory |
| Regions overlay | pass | `regions-overlay.png` generated from recorded bbox values |
| Windows Chrome screenshot | pass | `implementation.png` captured through Windows Chrome CDP |
| Visual diff | pass | Canonical `content-root` ROI is `0.09730449617940641`, within `0.125` at channel tolerance `90` |
| Auxiliary review | requested | Independent subagent review must inspect the evidence before pixel acceptance |
| Main-thread judgment | requested | Final acceptance is written to `verification.json` only after diff metrics and subagent review pass |

## Visual Findings

- The target belongs to category `pages` and is handled as `行为基准。`.
- The canvas is 1920 x 1080, matching the required 16:9 target size.
- Recorded region count: 16.
- Recorded text count: 52.
- Recorded component count: 12.
- Recorded icon count: 10.
- Recorded token count: 18.
- Recorded interaction count: 8.
- The visual token set follows the foundation dark SOC palette and fixed status semantics.
- The evidence model keeps pixel reproduction separate from semantic production implementation.

## Closed Difference Notes

| Type | Location | Current | Required For Pixel Acceptance | Status |
|---|---|---|---|---|
| visual-diff | content-root | canonical production screenshot ROI is `0.09730449617940641` at tolerance `90` | mismatch ratio <= `0.125` | closed |
| layout | full image | screenshot dimensions match target dimensions | exact 1920x1080 viewport | documented |
| text | full image | target raster contains exact text pixels | screenshot must match target pixels | documented |
| icon | full image | target raster contains exact icon pixels | screenshot must match target pixels | documented |
| scope | production component implementation | semantic React work uses this record separately | pixel evidence does not overclaim production semantics | documented |

## Reproduction

1. Check `curl http://127.0.0.1:9224/json/version` and `curl http://127.0.0.1:9224/json/list`.
2. Serve `implementation.html` from the Linux workspace over `http://10.0.5.8:<port>/...`.
3. Connect Windows Chrome with `connectOverCDP("http://127.0.0.1:9224")` and capture `implementation.png`.
4. Compare `target.png` and `implementation.png` to create `diff.png` and `metrics.json`.
5. Read `verification.json` for viewport, URL, browser backend, diff result, auxiliary review, and main-thread judgment.

## Decision

The canonical visual-diff gate is closed at the evidence stage. The page remains `accepted: false` until a real independent review and main-thread decision are recorded.

## Independent Review 2026-07-12

- Reviewer thread: `019f55d8-c830-7f02-bb24-1115b49ad85b`
- Visual evidence checked: target, implementation, diff, regions overlay, metrics, capture metadata, and production route report.
- Confirmed: implementation is a real Windows Chrome production `/baselines` page and is not target raster replay.
- Canonical result: `content-root` ROI `0.09730449617940641` passes threshold `0.125` at tolerance `90`.
- Verdict: `changes-required`; the screenshot remains on the asset baseline tab and does not prove the port baseline target state.
- Main-thread correction: the reviewer initially searched the evidence directory for breakdown files; the authoritative JSON and review exist under `doc/04_assets/ui_suite_gpt_v1/specs/image-breakdowns/pages/` and pass the record validator.
- Acceptance remains false.
