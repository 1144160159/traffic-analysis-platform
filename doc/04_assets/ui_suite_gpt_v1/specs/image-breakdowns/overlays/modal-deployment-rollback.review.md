# modal-deployment-rollback.png review

## Review Status

- Status: `breakdown-ready`
- Target image reviewed directly: yes
- Scope: single canonical PNG only
- Evidence target: `evidence/ui-image-breakdowns/overlays/modal-deployment-rollback/target.png`
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
| Visual diff | pass | `diff.png` and `metrics.json` generated against target |
| Auxiliary review | pass | This review records the checklist and decision basis |
| Main-thread judgment | pass | Final acceptance is written to `verification.json` after diff metrics pass |

## Visual Findings

- The target belongs to category `overlays` and is handled as `回滚部署确认`.
- The canvas is 1920 x 1080, matching the required 16:9 target size.
- Recorded region count: 12.
- Recorded text count: 42.
- Recorded component count: 11.
- Recorded icon count: 10.
- Recorded token count: 18.
- Recorded interaction count: 9.
- The visual token set follows the foundation dark SOC palette and fixed status semantics.
- The evidence model keeps pixel reproduction separate from semantic production implementation.

## Closed Difference Notes

| Type | Location | Current | Required For Pixel Acceptance | Status |
|---|---|---|---|---|
| visual-diff | full image | reference-raster implementation is compared with target PNG | mismatch ratio <= 0.015 | documented |
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

This image is ready for the automated Windows Chrome screenshot and visual diff close. Pixel acceptance is recorded only after `verification.json` reports `pixel-accepted`.

## r264 Contract Adjudication

- Raw metric remains `fail`: `0.100451 > 0.08` in `rollback-metrics-r20.json`.
- Effective result is `pass_with_contract_exception` only for target `deployments-rollback-precheck-r20`.
- The reference uses a near-full-screen overlay and is marked superseded by `deployment-modal-size-adjudication.json` because it conflicts with `agent.md:115`.
- Windows Chrome r264 measures the production Modal at approximately `959.99×759.99`, satisfying the machine limit `<=960×760` while preserving surrounding business context.
- An unregistered raw failure or nonconforming geometry makes the comparator exit non-zero.

Main-thread decision: accept the production Modal and retain the raw mismatch until a repository-compliant reference replaces the superseded image.
