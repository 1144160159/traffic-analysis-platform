# Desktop Chrome Bridge Tool Call

- Result: `pass`
- Tool: `mcp__codex_desktop_node_repl__js`
- Timeout: `900000` ms
- Payload SHA256: `3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a`
- Payload self-test: `pass` (21/21)
- Visual targets: `30`
- Interaction targets: `28`
- Receiver uploads: `59`

## Usage Boundary

This file is the outer MCP call shape. Its `arguments.code` field is the inner JavaScript payload for the trusted Desktop Node REPL. Replace only the placeholder values inside the code before execution; do not convert the whole file into JavaScript and do not run it through shell Node.

## Checks

- pass: tool name targets Desktop Node REPL JS bridge (mcp__codex_desktop_node_repl__js)
- pass: payload summary requires Chrome extension backend (codex-desktop-chrome-extension)
- pass: payload forbids iab backend (iab)
- pass: payload self-test is pass (pass 21/21)
- pass: payload JS parses as async Desktop Node REPL code (parse ok)
- pass: tool-call code matches payload JS exactly (sha256=3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a)
- pass: tool-call keeps placeholders and does not embed runtime secrets (placeholders only)
- pass: payload target URLs use Windows localhost tunnel endpoints (requires 25173/25174/25175 and no direct APISIX url_template)
- pass: tool-call timeout is numeric and long enough for full capture (900000)

