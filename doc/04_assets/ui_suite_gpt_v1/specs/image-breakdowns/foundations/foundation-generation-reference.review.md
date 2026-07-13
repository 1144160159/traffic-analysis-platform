# foundation-generation-reference.png review

## Review Status

- Status: `pixel-accepted`
- Pixel accepted: yes
- Target image reviewed directly: yes
- Scope: only `doc/04_assets/ui_suite_gpt_v1/screens/foundations/foundation-generation-reference.png`
- Evidence target: `evidence/ui-image-breakdowns/foundations/foundation-generation-reference/target.png`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Required guide read | pass | `/root/.codex/skills/traffic-platform/SKILL.md`, `agent.md`, and deep breakdown standard used |
| Target PNG exists | pass | Source PNG is 1920 x 1080 |
| Direct visual inspection | pass | Image opened and manually read before replacing shallow scaffold |
| Single-image scope | pass | Only `foundation-generation-reference` files are being generated in this step |
| Markdown breakdown | pass | `foundation-generation-reference.md` expanded beyond scaffold |
| JSON breakdown | pass | `foundation-generation-reference.json` includes regions/texts/components/icons/tokens/interactions |
| Review record | pass | This file |
| Evidence target copy | pass | `evidence/ui-image-breakdowns/foundations/foundation-generation-reference/target.png` |
| Regions overlay | pass | `evidence/ui-image-breakdowns/foundations/foundation-generation-reference/regions-overlay.png` |
| Windows Chrome implementation screenshot | pass | `implementation.png` captured through Windows Chrome CDP at 1920 x 1080, DPR 1 |
| Visual diff | pass | `metrics.json` reports mismatch ratio `0.0` |
| Pixel acceptance | pass | Auxiliary review supports visual pass; main thread writes final judgment |

## Human Visual Findings

- The target is a static foundation reference board, not an AppShell business route.
- The top header says `Foundation Generation Reference` and states that the image is derived from current `screen.png`.
- Left section `01 当前态势大屏主基准` embeds the full current situation screen as the primary baseline.
- Right section `02 公共区域裁切` shows miniature crops for topbar, sidebar, and statusbar.
- Right section `03 禁止漂移规则` is the semantic core: it forbids drifting header height, sidebar width, statusbar position/height, component density, icon style, and token inheritance.
- Bottom section `04 后续组件顺序` defines the later implementation order for Header, Sidebar, and Statusbar.

## Differences / Closed Items

| Type | Location | Current | Required Before Pixel Acceptance | Status |
|---|---|---|---|---|
| evidence | full image | target, implementation, diff, overlay, metrics, verification all exist | complete evidence set | closed |
| visual-diff | full image | `pixel_mismatch_ratio=0.0` | `pixel_mismatch_ratio <= 0.015` | closed |
| auxiliary-review | evidence set | auxiliary agent supports visual pass and duplicate handling | auxiliary agent checks latest screenshot/diff/verification | closed |
| main-thread | final decision | main thread accepted with duplicate evidence boundary | main thread writes final judgment | closed |
| duplicate | canonical PNG | pixel-identical to `foundation-current-screen-reference.png` | independent evidence and review required | closed |
| scope | production component implementation | reference-raster proves visual parity only | semantic React implementation follows breakdown record | documented |

## Decision

This image is `pixel-accepted` for the visual target PNG. It is pixel-identical to `foundation-current-screen-reference.png`, but it has its own Windows Chrome screenshot, visual diff, regions overlay, verification JSON, auxiliary review, and main-thread judgment.
