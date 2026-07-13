# VSCode Local Chrome Screen Topology R1

> Superseded note: this evidence was captured from the remote Linux-side Chrome/CDP path and does **not** satisfy the corrected requirement to use the VSCode Windows host Chrome through the Codex Desktop Chrome Bridge. Keep it only as a local development artifact, not as Windows/bridge validation evidence.

- Date: 2026-07-02
- Target: `/screen`
- Base URL: `http://127.0.0.1:5173`
- Chrome/CDP: `http://127.0.0.1:9222`
- Evidence: `doc/02_acceptance/02-regression/ui-visual-interaction/local-dev/20260702-vscode-local-chrome-screen-topology-r1/screen/`
- Acceptance scope: remote Linux local-development only. This run used local Vite and Chrome/CDP on the remote Linux side, so it is not eligible for the Windows VSCode host Chrome requirement.

## Windows Tunnel Endpoint

- Corrected browser target: VSCode Windows host `10.3.6.59`, via Codex Desktop Chrome Bridge / Chrome extension backend.
- SSH reverse tunnel established from remote Linux to Windows:
  - Windows URL for bridge/browser: `http://127.0.0.1:25173/screen`
  - Remote bind on Windows: `127.0.0.1:25173`
  - Forward target on Linux: `127.0.0.1:5173`
- Verification from Windows host:
  - `curl.exe --noproxy * http://127.0.0.1:25173/screen`
  - Result: `HTTP/1.1 200 OK`
- Note: Windows command-line HTTP checks must bypass proxy for loopback; without `--noproxy *`, the local request was intercepted by proxy and returned `503`.
- Bridge status in this chat: `desktop_chrome_open_url`, `desktop_chrome_list_tabs`, and `mcp__codex_desktop_node_repl__js` are not exposed to the current model session yet. Once exposed, the bridge should open `http://127.0.0.1:25173/screen`.

## Code Iteration

- Added native DOM/CSS topology district patches for teaching, office, core, experiment, dormitory, and high-risk boundary zones.
- Added native CSS building beacons for topology nodes and a core-zone icon shell.
- Kept route data, links, API usage, and React Query behavior unchanged.

## Validation

- `node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs`
  - Pass: `181` manifest items, `28` route contracts, `70` overlay contracts, `0` errors, `0` warnings.
- `npm --prefix web/ui run test -- --run src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts`
  - Pass: `32` tests.
- `npm --prefix web/ui run build`
  - Pass. Vite reported existing large chunk warnings only.
- `node tests/e2e/ui_local_visual_iteration.mjs --target-id screen --base-url http://127.0.0.1:5173 --wait-ms 3500 --cdp-url http://127.0.0.1:9222 --run-id 20260702-vscode-local-chrome-screen-topology-r1`
  - Pass capture: `actual-1920.png`, viewport `1920x1080`, document/body `1920x1080`, no vertical or horizontal scroll.
- Direct local Chrome websocket resource check for `http://127.0.0.1:5173/screen`
  - Pass: no `4xx/5xx` responses and no request failures.
- Direct local Chrome request-host check for `http://127.0.0.1:5173/screen`
  - Pass: unique request hosts were only `http://127.0.0.1:5173`; `10.0.5.8` request count was `0`.
- `PLAYWRIGHT_BASE_URL=http://127.0.0.1:5173 npm --prefix web/ui run test:e2e -- e2e/product-navigation.spec.ts --project=chromium -g "dashboard|screen"`
  - Pass: `2/2`.

## Visual Metrics

- Strict full-page diff: fail.
  - `pixel_mismatch_ratio`: `0.9999107831790124`
  - Size: source `1920x1080`, actual `1920x1080`
- Layer diff: fail.
  - `global-app-shell.mean_channel_delta`: `13.11341851146433`
  - `page-workspace.mean_channel_delta`: `19.43450069471095`
  - `closed-loop-rail.mean_channel_delta`: `17.857990033222592`

## Current Reading

The VSCode local Chrome test path is usable and the page is stable at 1920x1080. The remaining visual blocker is not runtime correctness; it is fidelity to the target image, especially the central 3D campus topology and pixel-level AppShell details.
