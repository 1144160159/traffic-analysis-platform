# alert-detail-evidence-session.png review

## Review Status

- Status: `business-pixel-accepted`
- Acceptance mode: `business-popup-crop-and-interaction`
- Target image reviewed directly: yes
- Production URL: `http://10.0.5.8:30180/alerts/AL-20260620-000123?__codex_ui_breakdown_production=1&__codex_page_id=alert-detail-evidence-session&evidenceView=session&__capture=r160-close-final&windowsCdpEvidenceTs=1783596176357`
- Viewport: 1920 x 1080
- Image tag: `traffic/web-ui:ui-alert-detail-evidence-session-popup-close-20260709-r160`
- Evidence target: `evidence/ui-image-breakdowns/pages/alert-detail-evidence-session/target.png`
- Browser evidence: Windows Chrome CDP `http://127.0.0.1:9224`

## Checks

| Check | Result | Evidence |
|---|---|---|
| Queue lock | pass | page 22, parent `alerts`, route `/alerts` |
| Target PNG read | pass | target copied to evidence dir and visually inspected |
| Runtime clean | pass | `capture-meta.json`: no console/pageerror/requestfailed/HTTP 4xx/5xx |
| Forbidden bitmap UI | pass | no `/screens/pages/`, `target.png`, `implementation.html`, or evidence resources requested |
| Small popup size | pass | popup card bbox `469,214,1152,648`; not full Windows/browser window |
| Top-right close X | pass | close bbox `1575,229,31,31`; `aria-label/title=关闭弹窗` |
| Close interaction | pass | `interaction.png`: popup absent, alert detail still visible |
| Menu selection | pass | `威胁分析` and `告警中心128` remain active for the detail state |
| Dynamic business diagram | pass | Session event chain is React/CSS from `sessionEvidence.timeline` typed data |
| Text completeness | pass | key texts present; no clipped text without title |
| Diff | pass | business popup crop mismatch `0.08514044281550069` <= `0.35` |
| Docs synced | pass | `.md`, `.json`, `.review.md`, `measurement.json`, `verification.json`, `text-ocr.txt`, `regions-overlay.png` updated to r160 |

## Difference Notes

- Target full canvas is now treated as content reference because the user clarified this state is a small popup.
- Accepted residual differences are font rasterization, icon glyph style, glow intensity and subpixel alignment.
- No missing table columns, no missing Session rows, no missing event-chain nodes, no bottom obstruction, and no root overflow.
- r155 “near full-screen focus panel” acceptance is superseded by r160 small-popup acceptance.

## Decision

Main-thread judgment: `business-pixel-accepted`. Page 23 may start only after this accepted r160 record is consumed.
