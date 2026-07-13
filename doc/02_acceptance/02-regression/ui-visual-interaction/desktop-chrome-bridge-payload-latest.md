# Desktop Chrome Bridge Payload

- Generated: `2026-07-02T18:12:57.511Z`
- Capture session: `doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json`
- Payload JS: `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js`
- Backend required: `codex-desktop-chrome-extension`
- Forbidden backend: `iab`
- Expected viewport: `1920x1080`
- Visual targets: `30`
- Interaction targets: `28`
- Screenshot uploads: `58`
- Bridge result uploads: `1`
- Receiver uploads: `59`

The JS payload is an execution aid for `mcp__codex_desktop_node_repl__js`. It intentionally contains placeholders for `CODEX_CAPTURE_KEY` and `CODEX_SMOKE_NONCE`; concrete secrets must stay in process memory and out of repo files.

After the payload runs, execute the generated metrics commands from the capture session and then run the visual interaction finalizer.
