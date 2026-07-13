# Desktop Chrome Bridge Payload Self-Test

- Result: `pass`
- Generated: `2026-07-02T18:13:15.294Z`
- Payload: `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json`
- Payload JS: `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js`
- Visual targets: `30`
- Interaction targets: `28`
- Screenshot uploads: `58`
- Bridge result uploads: `1`
- Receiver uploads: `59`

## Checks

- pass: payload summary JSON is valid (valid)
- pass: payload JS exists (present)
- pass: capture session is valid (valid)
- pass: gap report is valid (valid)
- pass: Windows host preflight is pass (pass)
- pass: Windows runtime preflight is pass (pass)
- pass: payload uses Windows LongShine Chrome client (file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/26.623.81905/scripts/browser-client.mjs)
- pass: payload JS parses as async Desktop Node REPL code (parse ok)
- pass: payload target URLs have no unresolved smoke redirect base URL marker (<SMOKE_REDIRECT_BASE_URL> absent from url_template values)
- pass: payload uses Windows localhost tunnel URLs instead of direct APISIX for capture (requires 25173/25174/25175 and rejects 10.0.5.8:30180 in payload JS)
- pass: payload requires Chrome extension backend (backend=codex-desktop-chrome-extension)
- pass: payload forbids iab (forbidden=iab)
- pass: payload contains placeholders only ({"CODEX_CAPTURE_KEY":"<CODEX_CAPTURE_KEY>","CODEX_SMOKE_NONCE":"<CODEX_SMOKE_NONCE>"})
- pass: payload JS has no JWT or Bearer material (secret scan pattern absent)
- pass: payload uploads bridge result summary (bridge_result_upload_count=1)
- pass: payload visual count matches current gap report (30/30)
- pass: payload interaction count matches current gap report (28/28)
- pass: payload counts match capture session (payload=30/28 session=30/28)
- pass: payload receiver upload count is complete (59=58+1)
- pass: payload JS target literal counts match summary (js=30/28 summary=30/28)
- pass: payload covers all interaction gap route ids (covered=28 gaps=28)

