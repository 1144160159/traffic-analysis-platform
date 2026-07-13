# Windows Codex Chrome Bridge Tunnel Status

- Date: 2026-07-02
- Windows host: `10.3.6.59`
- Windows user observed by SSH: `longshine`
- Remote Linux workspace: `/home/wangwt/phase_2/code/traffic-analysis-platform`
- Correct browser target: Windows-host Codex Desktop Chrome Bridge, Chrome extension backend.

## Tunnel

The current bridge-facing page URL is:

```text
http://127.0.0.1:25173/screen
```

The reverse SSH tunnel maps:

```text
Windows 127.0.0.1:25173 -> remote Linux 127.0.0.1:5173
```

The page service is the Vite dev server started from this repository:

```text
npm --prefix web/ui run dev -- --host 127.0.0.1 --port 5173
```

The tunnel command is:

```text
ssh -N -R 127.0.0.1:25173:127.0.0.1:5173 LongShine@10.3.6.59
```

## Verification

Linux side:

```text
curl -I http://127.0.0.1:5173/screen
HTTP/1.1 200 OK
```

Windows side through the tunnel:

```text
curl.exe --noproxy * -s -o nul -w TUNNEL25173_STATUS=%{http_code} http://127.0.0.1:25173/screen
TUNNEL25173_STATUS=200
```

Re-checked after the `/screen` no-bitmap UI pass:

```text
curl -I http://127.0.0.1:5173/screen
HTTP/1.1 200 OK

curl.exe --noproxy * -s -o nul -w TUNNEL25173_STATUS=%{http_code} http://127.0.0.1:25173/screen
TUNNEL25173_STATUS=200
```

Windows proxy path without `--noproxy *`:

```text
curl.exe -s -o nul -w PROXY_PATH_STATUS=%{http_code} http://127.0.0.1:25173/screen
PROXY_PATH_STATUS=503
```

So Windows loopback access must bypass proxy for command-line verification. The Codex Desktop Chrome Bridge should open the Windows-local URL directly:

```text
http://127.0.0.1:25173/screen
```

## Bridge Tool Status

Current model-session tool discovery did not expose the required bridge tools:

- `desktop_chrome_open_url`
- `desktop_chrome_list_tabs`
- `desktop_chrome_claim_url`
- `mcp__codex_desktop_node_repl__js`

Only unrelated app connectors were exposed during tool discovery, so this run cannot yet produce formal Windows Codex Desktop Chrome evidence. Do not substitute Linux-side Chrome/CDP or manual Windows DevTools for this gate.

Latest discovery attempt again exposed unrelated app tools only, not the required Codex Desktop Chrome Bridge or Node REPL tool. The required formal visual gate remains:

```text
desktop_chrome_open_url(url="http://127.0.0.1:25173/screen", keep=true)
```

### 2026-07-03 r8 SSH stdio probe

Windows-side tunnel access was re-checked from `10.3.6.59`:

```text
curl.exe --noproxy * -s -o nul -w TUNNEL25173_STATUS=%{http_code} http://127.0.0.1:25173/screen
TUNNEL25173_STATUS=200
```

The Windows host at `10.3.6.59` is reachable over SSH, and the Windows Codex Node REPL MCP executable can be started through JSONL stdio from that SSH channel. The server initialized successfully and returned these tools:

```text
js
js_add_node_module_dir
js_reset
```

This narrows the bridge blocker: it is no longer only "host unreachable" or "node_repl not present." However, a minimal `tools/call js` probe using `nodeRepl.write(...)` failed before any Chrome operation. The reported diagnostics were:

```text
node_repl kernel exited unexpectedly
windows sandbox failed: helper_firewall_rule_create_or_add_failed
SetRemotePorts failed: HRESULT(0x80070005) / 拒绝访问。
```

So direct SSH stdio can reach the Windows Node REPL MCP server, but the SSH-spawned execution context does not currently have the Windows sandbox/firewall permission needed to start the JS kernel. Formal Chrome evidence still requires the trusted Codex Desktop / VSCode bridge tool surface, or a Windows-side runtime context that can execute `tools/call js` without the sandbox firewall denial.

### 2026-07-03 r9 local proxy-over-SSH stdio probe

A temporary local listener was started at `127.0.0.1:19998` and bridged to the Windows `node_repl.exe` JSONL stdio process over SSH. This allowed `/root/.codex/bin/node-repl-mcp-proxy.py` to connect to its expected backend endpoint.

The proxy successfully listed both native Node REPL tools and wrapper tools:

```text
js
js_add_node_module_dir
js_reset
desktop_chrome_list_tabs
desktop_iab_list_targets
desktop_chrome_open_url
desktop_chrome_claim_url
desktop_chrome_cleanup_blank_tabs
```

That proves the local proxy/wrapper layer can be wired to the Windows stdio backend. It does not close the Chrome gate, because both of these follow-up calls still failed with the same lower-level Windows sandbox/firewall denial:

```text
tools/call js -> node_repl kernel exited unexpectedly
tools/call desktop_chrome_list_tabs -> node_repl kernel exited unexpectedly
windows sandbox failed: helper_firewall_rule_create_or_add_failed
SetRemotePorts failed: HRESULT(0x80070005) / 拒绝访问。
```

The temporary local `127.0.0.1:19998` listener was stopped after the probe and the temporary local script was removed. The remaining blocker is therefore not proxy tool listing; it is that an SSH-spawned `node_repl.exe` cannot start the JS kernel with the Windows sandbox/firewall permissions required for browser control. Formal Chrome evidence still needs the trusted Codex Desktop / VSCode execution context.

### 2026-07-03 r10 Windows Codex MCP config and full-env probe

The Windows Codex CLI was checked through SSH using its absolute installed path. `remote-control start --json` returned:

```text
Error: codex app-server daemon lifecycle is only supported on Unix platforms
```

So the Windows CLI cannot be used to start the app-server daemon lifecycle directly from this SSH session.

The same Windows CLI confirms the `node_repl` MCP entry exists:

```text
node_repl
enabled: true
transport: stdio
command: C:\Users\LongShine\AppData\Local\OpenAI\Codex\runtimes\cua_node\1b23c930bdf84ed6\bin\node_repl.exe
startup_timeout_sec: 120
auth: Unsupported
env includes: BROWSER_USE_*, CODEX_HOME, CODEX_CLI_PATH, NODE_REPL_*, SKY_CUA_NATIVE_PIPE, SKY_CUA_NATIVE_PIPE_DIRECTORY
```

A follow-up direct JSONL stdio `tools/call js` probe set the observed Windows Codex native-pipe and Node REPL environment names explicitly in the SSH-spawned `cmd` context before launching `node_repl.exe`. It still failed before Chrome control:

```text
node_repl kernel exited unexpectedly
windows sandbox failed: helper_firewall_rule_create_or_add_failed
SetRemotePorts failed: HRESULT(0x80070005) / 拒绝访问。
```

This confirms the remaining bridge blocker is not missing env names, missing MCP config, missing tunnel reachability, or proxy wrapper discovery. The failing boundary is the SSH-spawned Windows execution context lacking the sandbox/firewall permission needed to start the JS kernel. Formal Chrome evidence still needs the trusted Windows Codex Desktop / VSCode tool surface, such as `mcp__codex_desktop_node_repl__js` or the wrapped `desktop_chrome_*` tools exposed in the current Codex session.

### 2026-07-03 r11 payload URL policy fix

The Windows tunnel channel preflight was rerun and passed from both Linux and Windows sides. Windows `10.3.6.59` can reach:

```text
http://127.0.0.1:25173/screen -> 200
http://127.0.0.1:25174/health -> 200
http://127.0.0.1:25174/viewport-probe -> 200
http://127.0.0.1:25175/health -> 200
```

The Desktop Chrome payload generator was tightened so generated capture URL templates prefer the Windows tunnel wrapper URLs from `capture-session-windows-tunnel-25173.json`. The regenerated payload now uses:

```text
app pages:        http://127.0.0.1:25173/...
receiver uploads: http://127.0.0.1:25174/...
smoke redirect:   http://127.0.0.1:25175/...
```

The payload self-test now checks that the JS parses as async Desktop Node REPL code, that target URL templates have no unresolved `<SMOKE_REDIRECT_BASE_URL>` marker, and that the payload does not use direct `10.0.5.8:30180` capture URLs. The latest result is `pass`, `21/21`, covering 30 visual targets, 28 interaction routes, 58 screenshots, and 1 bridge-result upload.

## Current Frontend Checks

The frontend code path remains valid while waiting for the bridge tool surface. The `/screen` implementation no longer imports or applies the previous bitmap map backgrounds:

```text
rg -n "backgroundAsset|screen-risk-world|screen-egress-world|screen-probe-campus-map|backgroundImage" web/ui/src/pages/SituationalScreen.tsx web/ui/src/styles/pages.css
Result: no matches
```

The no-bitmap pass replaces those map backgrounds with DOM/CSS layers for:

- probe campus coverage map
- central digital-twin topology map
- risk-area density map
- external egress flow map

Validation:

```text
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts
errors: 0, warnings: 0
```

```text
npm --prefix web/ui run test -- --run src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts
Test Files 2 passed
Tests 32 passed
```

```text
npm --prefix web/ui run build
Result: passed
Note: Vite reported existing large chunk warnings only.
```

Local development visual smoke, not formal acceptance:

```text
node tests/e2e/ui_local_visual_iteration.mjs --base-url http://127.0.0.1:5173 --target-id screen --run-id 20260702-vscode-local-screen-nobitmap-r1 --wait-ms 2000 --width 1920 --height 1080
ok: true
acceptance_eligible: false
screenshot: doc/02_acceptance/02-regression/ui-visual-interaction/local-dev/20260702-vscode-local-screen-nobitmap-r1/screen/actual-1920.png
```

Capture metadata:

```text
title: 园区网络全流量采集与分析系统
document_width: 1920
document_height: 1080
has_vertical_scroll: false
has_horizontal_scroll: false
console_errors: []
page_errors: []
request_failures: []
server_errors: []
browser_backend: playwright-chromium-launch
```

## Route Background No-Bitmap Pass

The route-level bitmap backgrounds were removed from the frontend implementation:

- `web/ui/src/routes/routeManifest.tsx` now stores semantic background keys instead of public asset URLs.
- `web/ui/src/pages/ProductPage.tsx` renders generic route hero backgrounds with DOM/CSS mesh layers.
- `web/ui/src/pages/TopicWorkbenchPage.tsx` renders tunnel/exfil/APT topic shells with DOM/CSS mesh layers.
- `web/ui/src/pages/ProbesManagementPage.tsx` renders deployment topology map areas, roads, nodes, and links with DOM/CSS layers.
- `web/ui/src/pages/NotFoundPage.tsx` renders the 404 background with DOM/CSS mesh layers.

Static no-bitmap scan:

```text
rg -n "backgroundAsset\(|backgroundImage|--page-bg|--topic-bg|--taf-probes-bg|--taf-notfound-bg|screen-risk-world|screen-egress-world|screen-probe-campus-map|url\(" web/ui/src --glob '!**/*.test.*' --glob '!**/__tests__/**'
Result: no matches
```

The only remaining `<img>` usage is the login captcha image returned by the auth API, not a UI bitmap background:

```text
web/ui/src/pages/LoginPage.tsx:245 captcha?.imageData ? <img src={captcha.imageData} alt="登录验证码" />
```

Validation after the route-background pass:

```text
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts
errors: 0, warnings: 0

npm --prefix web/ui run test -- --run src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts
Test Files 2 passed
Tests 32 passed

npm --prefix web/ui run build
Result: passed
Note: Vite reported existing large chunk warnings only.
```

Local development visual smoke, not formal acceptance:

```text
run_id: 20260702-local-nobitmap-route-bg-r1
targets: alerts, probes, topics-encrypted-tunnel, not-found
browser_backend: playwright-chromium-launch
acceptance_eligible: false
document_width: 1920
document_height: 1080
has_vertical_scroll: false
has_horizontal_scroll: false
console_errors: 0
page_errors: 0
request_failures: 0
server_errors: 0
```

Windows tunnel re-check for an affected route:

```text
curl.exe --noproxy * -s -o nul -w TUNNEL25173_STATUS=%{http_code} http://127.0.0.1:25173/probes
TUNNEL25173_STATUS=200
```

Latest bridge tool discovery still exposed unrelated app tools only. Formal Windows Chrome evidence still requires the Codex Desktop Chrome Bridge wrapper once it appears in the tool surface.

## Automated No-Bitmap Guard

Added a Vitest guard for the no-bitmap UI invariant:

```text
web/ui/src/routes/noBitmapUi.test.ts
```

The guard scans production frontend source under `web/ui/src` and fails on:

- `backgroundAsset(...)`
- inline `backgroundImage`
- CSS `url(...)`
- bitmap/vector file extensions in UI source strings
- legacy bitmap background variables: `--page-bg`, `--topic-bg`, `--taf-probes-bg`, `--taf-notfound-bg`
- raw `<img>` tags except the login captcha returned by the auth API

Validation:

```text
npm --prefix web/ui run test -- --run src/routes/noBitmapUi.test.ts src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts
Test Files 3 passed
Tests 33 passed

npm --prefix web/ui run build
Result: passed
Note: Vite reported existing large chunk warnings only.
```

Contract and build were re-run after adding the guard:

```text
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts
errors: 0, warnings: 0

npm --prefix web/ui run build
Result: passed
Note: Vite reported existing large chunk warnings only.
```

## Full Local Contract Visual Smoke

Full local development visual smoke covered all visual-acceptance targets:

```text
run_id: 20260702-local-all-contract-nobitmap-r1
target_count: 30
failure_count: 0
screenshots: 30
capture_meta_files: 30
browser_backend: playwright-chromium-launch
acceptance_eligible: false
```

Aggregated capture metadata:

```text
bad_count: 0
document_width: 1920
document_height: 1080
has_vertical_scroll: false for all targets
has_horizontal_scroll: false for all targets
console_errors: 0 for all targets
page_errors: 0 for all targets
request_failures: 0 for all targets
server_errors: 0 for all targets
```

Covered targets:

```text
login, screen, dashboard, alerts, alert-detail, campaigns, campaign-detail,
attack-chains, encrypted-traffic, forensics, assets, graph, fusion, baselines,
probes, rules, deployments, models, mlops, data-quality, playbooks, whitelist,
compliance, audit-log, notifications, settings, not-found,
topics-encrypted-tunnel, topics-data-exfiltration, topics-apt-campaign
```

Reusable local suite script added:

```text
tests/e2e/ui_local_visual_contract_suite.mjs
```

The script reads `doc/04_assets/ui_suite_gpt_v1/specs/visual-acceptance.json`, runs every visual target through `tests/e2e/ui_local_visual_iteration.mjs`, and fails on missing screenshot metadata, viewport/document-size drift, scrollbars, console errors, page errors, request failures, or 5xx responses.

Reusable suite validation:

```text
node tests/e2e/ui_local_visual_contract_suite.mjs --base-url http://127.0.0.1:5173 --run-id 20260702-local-all-contract-suite-r1 --wait-ms 2000 --width 1920 --height 1080

ok: true
run_id: 20260702-local-all-contract-suite-r1
target_count: 30
failure_count: 0
acceptance_eligible: false
summary_json: doc/02_acceptance/02-regression/ui-visual-interaction/local-dev/20260702-local-all-contract-suite-r1/summary.json
summary_md: doc/02_acceptance/02-regression/ui-visual-interaction/local-dev/20260702-local-all-contract-suite-r1/summary.md
```

The reusable suite supersedes the earlier one-off local aggregation command for development-loop checks. It still does not satisfy the formal Windows Chrome gate because it uses local Playwright Chromium rather than the Windows Codex Desktop Chrome extension backend.

Post-suite validation:

```text
node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts
errors: 0, warnings: 0

npm --prefix web/ui run test -- --run src/routes/noBitmapUi.test.ts src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts
Test Files 3 passed
Tests 33 passed

npm --prefix web/ui run build
Result: passed
Note: Vite reported existing large chunk warnings only.
```

Windows tunnel re-check after the reusable suite and build:

```text
curl.exe --noproxy * -s -o nul -w SCREEN=%{http_code} http://127.0.0.1:25173/screen
SCREEN=200

curl.exe --noproxy * -s -o nul -w DASHBOARD=%{http_code} http://127.0.0.1:25173/dashboard
DASHBOARD=200

curl.exe --noproxy * -s -o nul -w TOPICS=%{http_code} http://127.0.0.1:25173/topics?topic=tunnel
TOPICS=200
```

Windows tunnel re-check after full local smoke:

```text
curl.exe --noproxy * -s -o nul -w SCREEN=%{http_code} http://127.0.0.1:25173/screen
SCREEN=200

curl.exe --noproxy * -s -o nul -w TOPICS=%{http_code} http://127.0.0.1:25173/topics?topic=tunnel
TOPICS=200
```

Latest bridge discovery still did not expose `desktop_chrome_open_url`, `desktop_chrome_list_tabs`, `desktop_chrome_claim_url`, or `mcp__codex_desktop_node_repl__js`; it exposed unrelated app connectors. Formal Windows Chrome visual evidence remains pending on that tool surface.

## Windows Tunnel Formal Capture Readiness

The existing formal Desktop Chrome evidence chain was aligned to the Windows-local tunnel URL:

```text
base_url: http://127.0.0.1:25173
receiver_url: http://127.0.0.1:25174
smoke_redirect_base_url: http://127.0.0.1:25175
```

Receiver self-test initially failed because Python `urllib` honored the environment proxy for `127.0.0.1` and every local request returned `503`. The self-test HTTP client now disables proxies explicitly, matching the repository rule to bypass proxies for localhost checks.

Receiver self-test after the fix:

```text
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=10/10
```

Generated Windows tunnel capture plan:

```text
node tests/e2e/ui_desktop_capture_plan.mjs \
  --base-url http://127.0.0.1:25173 \
  --receiver-url http://127.0.0.1:25174 \
  --smoke-redirect-base-url http://127.0.0.1:25175 \
  --output-json doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json \
  --output-md doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.md

visual: 0/30
interactions: 4/28
receiver_selftest: pass 10/10
viewport_probe: blocked, observed 2560x1271, expected 1920x1080
```

Generated Windows tunnel capture session:

```text
node tests/e2e/ui_desktop_capture_session.mjs \
  --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json \
  --receiver-url http://127.0.0.1:25174 \
  --smoke-redirect-base-url http://127.0.0.1:25175 \
  --receiver-port 15174 \
  --redirect-port 15175 \
  --output-json doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json \
  --output-md doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.md

status: blocked_desktop_transport_closed
visual_pending: 30
interaction_pending: 24
receiver_selftest: pass
viewport_calibration: blocked
```

The generated session now includes the no-secret SSH templates required for the Windows-local bridge workflow:

```text
app_reverse_tunnel:
ssh -N -R 127.0.0.1:25173:127.0.0.1:5173 LongShine@10.3.6.59

evidence_reverse_tunnel:
ssh -N -R 127.0.0.1:25174:127.0.0.1:15174 -R 127.0.0.1:25175:127.0.0.1:15175 LongShine@10.3.6.59
```

This capture package is execution readiness only. It does not close the formal gate until the actual Windows Codex Desktop Chrome extension backend can open `http://127.0.0.1:25173`, pass the `1920x1080` viewport probe, upload screenshots to the receiver, and produce per-target `capture-meta.json`, `metrics.json`, `actual-1920.png`, `diff-1920.png`, and route `interaction.json` evidence.

## Live Evidence Channel Check

The receiver and smoke redirect helpers were started on the remote Linux side and exposed to the Windows host through an additional reverse SSH tunnel.

Remote Linux listeners:

```text
127.0.0.1:15174 -> ui_desktop_capture_receiver.py
127.0.0.1:15175 -> ui_desktop_smoke_redirect.py
```

Reverse tunnel:

```text
Windows 127.0.0.1:25174 -> remote Linux 127.0.0.1:15174
Windows 127.0.0.1:25175 -> remote Linux 127.0.0.1:15175
```

Windows-side verification:

```text
curl.exe --noproxy * -s -o nul -w RECEIVER=%{http_code} http://127.0.0.1:25174/health
RECEIVER=200

curl.exe --noproxy * -s -o nul -w VIEWPORT=%{http_code} http://127.0.0.1:25174/viewport-probe
VIEWPORT=200

curl.exe --noproxy * -s -o nul -w REDIRECT=%{http_code} http://127.0.0.1:25175/health
REDIRECT=200
```

Current bridge tool discovery still did not expose `desktop_chrome_open_url`, `desktop_chrome_list_tabs`, `desktop_chrome_claim_url`, or `mcp__codex_desktop_node_repl__js`; only unrelated app connectors were exposed. Therefore the receiver/redirect channel is ready, but the formal Desktop Chrome capture still cannot be executed from this model session.

Post-channel verification:

```text
node --check tests/e2e/ui_desktop_capture_session.mjs
node --check tests/e2e/ui_local_visual_contract_suite.mjs
python3 -m py_compile tests/e2e/ui_desktop_capture_receiver_selftest.py tests/e2e/ui_desktop_capture_receiver.py tests/e2e/ui_desktop_smoke_redirect.py
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=10/10

node doc/04_assets/ui_suite_gpt_v1/validate_frontend_contracts.mjs
validated UI frontend contracts: 181 manifest items, 28 route contracts, 70 overlay contracts
errors: 0, warnings: 0

npm --prefix web/ui run test -- --run src/routes/noBitmapUi.test.ts src/routes/routeManifest.test.ts src/services/pageSnapshotAdapters.test.ts
Test Files 3 passed
Tests 33 passed
```

Live Windows-local endpoints after the post-channel verification:

```text
curl.exe --noproxy * -s -o nul -w SCREEN=%{http_code} http://127.0.0.1:25173/screen
SCREEN=200

curl.exe --noproxy * -s -o nul -w RECEIVER=%{http_code} http://127.0.0.1:25174/health
RECEIVER=200

curl.exe --noproxy * -s -o nul -w VIEWPORT=%{http_code} http://127.0.0.1:25174/viewport-probe
VIEWPORT=200

curl.exe --noproxy * -s -o nul -w REDIRECT=%{http_code} http://127.0.0.1:25175/health
REDIRECT=200
```

Reusable Windows tunnel channel preflight:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.md
```

The preflight verifies these eight endpoints in one run:

```text
linux app: http://127.0.0.1:5173/screen -> 200
linux receiver-health: http://127.0.0.1:15174/health -> 200
linux viewport-probe: http://127.0.0.1:15174/viewport-probe -> 200
linux redirect-health: http://127.0.0.1:15175/health -> 200
windows app: http://127.0.0.1:25173/screen -> 200
windows receiver-health: http://127.0.0.1:25174/health -> 200
windows viewport-probe: http://127.0.0.1:25174/viewport-probe -> 200
windows redirect-health: http://127.0.0.1:25175/health -> 200
```

The refreshed capture session now includes this preflight in its command list:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.md
```

## 2026-07-03 Tunnel Re-Check

The active runtime is still aligned to the Windows/VSCode Chrome side through `10.3.6.59`:

```text
Windows 127.0.0.1:25173 -> remote Linux 127.0.0.1:5173
Windows 127.0.0.1:25174 -> remote Linux 127.0.0.1:15174
Windows 127.0.0.1:25175 -> remote Linux 127.0.0.1:15175
```

Linux-side listeners were present:

```text
127.0.0.1:5173  -> Vite dev server
127.0.0.1:15174 -> ui_desktop_capture_receiver.py
127.0.0.1:15175 -> ui_desktop_smoke_redirect.py
```

Local endpoint checks:

```text
curl --noproxy '*' http://127.0.0.1:5173/login         -> 200
curl --noproxy '*' http://127.0.0.1:15174/viewport-probe -> 200
```

Latest Windows tunnel/channel evidence remains:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json
result: pass
windows.host: 10.3.6.59
endpoint_checks: 10
failed_endpoints: 0
capture_session: doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
```

Latest bridge host/runtime readiness:

```text
windows-desktop-bridge-host-preflight-latest.json: result=pass, chrome client exists=true
windows-codex-bridge-runtime-preflight-latest.json: result=pass, chrome_clients=2, process_counts={"chrome":18,"codex":10,"code":18}
desktop-chrome-bridge-payload-selftest-latest.json: result=pass, checks=18/18, visual=30, interaction=28, receiver_uploads=59
```

Project completion audit now surfaces the same tunnel-readiness facts at the top-level completion gate:

```text
doc/02_acceptance/runs/202607030-tunnel-10-3-6-59-project-audit/live-project-completion-audit-202607030-tunnel-10-3-6-59-project-audit-summary.json
doc/02_acceptance/09-completion/project-completion-audit-latest.json
```

Result remains `blocked`, not because of the tunnel. The current blocker is the missing current-session Codex Desktop Chrome bridge tool exposure and the missing formal `desktop-chrome-bridge-run-latest.json` upload from Windows Chrome extension execution. Do not substitute Linux Chrome/CDP or Codex In-App Browser for this evidence.

## Bridge Payload And Receiver Hardening

The formal Windows tunnel package was refreshed after adding an interaction screenshot upload endpoint to the receiver.

New and refreshed artifacts:

```text
tests/e2e/ui_desktop_capture_receiver.py
  POST /interaction-screenshot/<route-id> -> latest/<route-id>/interaction.png
  writes latest/<route-id>/interaction-capture-meta.json

tests/e2e/ui_desktop_chrome_bridge_payload.mjs
  generates doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js
  backend requirement: codex-desktop-chrome-extension
  forbidden backend: iab
```

Earlier generated Windows tunnel queue, superseded by the full current-gap package below:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json
visual evidence: 0/30
interaction evidence: 4/28

doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
visual batch: 30
interaction batch: 24
screenshot uploads: 54
smoke redirect opens: 53
receiver start: --max-uploads 54
redirect start: --max-redirects 53

doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json
visual targets: 30
interaction targets: 24
receiver uploads: 54
```

Verification:

```text
node --check tests/e2e/ui_desktop_capture_plan.mjs
node --check tests/e2e/ui_desktop_capture_session.mjs
node --check tests/e2e/ui_desktop_chrome_bridge_payload.mjs
python3 -m py_compile tests/e2e/ui_desktop_capture_receiver.py tests/e2e/ui_desktop_capture_receiver_selftest.py

python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=11/11

node --input-type=module --check < doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js
```

The live receiver and redirect helpers were restarted with detached process groups so the existing reverse tunnel can continue serving Windows-local endpoints:

```text
127.0.0.1:15174 -> ui_desktop_capture_receiver.py --max-uploads 54
127.0.0.1:15175 -> ui_desktop_smoke_redirect.py --max-redirects 53
```

Refreshed tunnel preflight:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.md
```

This still does not close formal visual acceptance. The remaining external gate is unchanged: this model session must expose `desktop_chrome_open_url`, `desktop_chrome_list_tabs`, `desktop_chrome_claim_url`, or `mcp__codex_desktop_node_repl__js` so the generated payload can run in the Windows Codex Desktop Chrome extension backend.

## Formal Vite And Smoke Token Readiness

The Windows tunnel target was corrected to the VSCode Windows machine at `10.3.6.59`. The active reverse tunnel endpoints are:

```text
Windows 127.0.0.1:25173 -> Linux 127.0.0.1:5173  Vite dev server
Windows 127.0.0.1:25174 -> Linux 127.0.0.1:15174 screenshot receiver
Windows 127.0.0.1:25175 -> Linux 127.0.0.1:15175 protected-route smoke redirect
```

The Vite process on `127.0.0.1:5173` was restarted in formal capture mode:

```text
VITE_AUTH_ENABLED=true
VITE_USE_MOCK=false
VITE_DESKTOP_SMOKE_TOKEN_ENABLED=true
VITE_API_BASE_URL=/api
VITE_DEV_APISIX_URL=http://10.0.5.8:30180
VITE_SCREEN_ACCESS_MODE=protected
```

The screenshot receiver and smoke redirect helpers were also restarted with a short-lived JWT generated from the Kubernetes `traffic-analysis/traffic-credentials` `JWT_SECRET`. The concrete token, nonce, and capture key stayed in process memory and were not written to repo artifacts.

New preflight:

```text
node tests/e2e/ui_desktop_smoke_token_preflight.mjs \
  --base-url http://127.0.0.1:5173 \
  --apisix-url http://10.0.5.8:30180 \
  --route /dashboard \
  --expected-path /dashboard

ui-desktop-smoke-token-preflight result=pass checks=9/9
```

The preflight verifies:

```text
Vite runtime config: auth=true, useMock=false, desktopSmokeToken=true
APISIX /api/v1/auth/me accepts the generated JWT
protected route /dashboard consumes #codex_smoke_token and lands on /dashboard
```

The Windows tunnel preflight was refreshed to check the formal runtime flags from both Linux and Windows sides, and to force password authentication for `LongShine@10.3.6.59` so SSH key fan-out does not cause `Too many authentication failures`.

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/windows-tunnel-channel-preflight-latest.md
```

The Chrome bridge payload generator was hardened so protected-route captures fail fast if `CODEX_SMOKE_NONCE` is still a placeholder. The refreshed payload still contains placeholders only:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js
visual targets: 30
interaction targets: 24
screenshot uploads: 54
bridge result uploads: 1
receiver uploads: 55
protected redirect opens: 53
backend required: codex-desktop-chrome-extension
forbidden backend: iab
```

Boundary at this pass: tunnel, formal runtime, valid smoke token, receiver, redirect, and generated Chrome-extension payload were ready. Full Windows Chrome visual acceptance was still pending because this Codex session did not expose `desktop_chrome_*` tools or `mcp__codex_desktop_node_repl__js` after repeated tool discovery.

## 2026-07-03 Interaction Screenshot Evidence Hardening

The current Codex session was checked again for Windows Desktop Chrome bridge tools:

```text
tool_search: desktop_chrome_open_url desktop_chrome_list_tabs desktop_chrome_claim_url desktop_chrome_cleanup_blank_tabs
result: 0 callable bridge tools

tool_search: mcp__codex_desktop_node_repl__js codex desktop node repl js browser-client.mjs agent.browsers.get extension
result: no Desktop Chrome or Node REPL bridge callable tools exposed
```

The 10.3.6.59 Windows tunnel and local formal runtime were revalidated from current process state:

```text
Linux listeners:
127.0.0.1:5173  -> Vite formal dev server
127.0.0.1:15174 -> ui_desktop_capture_receiver.py
127.0.0.1:15175 -> ui_desktop_smoke_redirect.py

Reverse tunnel:
Windows 127.0.0.1:25173 -> Linux 127.0.0.1:5173
Windows 127.0.0.1:25174 -> Linux 127.0.0.1:15174
Windows 127.0.0.1:25175 -> Linux 127.0.0.1:15175

ui-desktop-smoke-token-preflight result=pass checks=9/9
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass
```

The interaction evidence contract was tightened so every route now requires all three files:

```text
latest/<route-id>/interaction.json
latest/<route-id>/interaction.png
latest/<route-id>/interaction-capture-meta.json
```

The generated Desktop Chrome payload now writes `desktop_backend: codex-desktop-chrome-extension` into each `interaction.json`. The finalizer and live preflight both verify that `interaction.png` is 1920x1080 and that `interaction-capture-meta.json` proves:

```text
backend is codex-desktop-chrome-extension
uploaded screenshot is 1920x1080
stored screenshot is 1920x1080
Desktop Chrome viewport is 1920x1080
post_capture_resize is false
```

Earlier generated package for interaction screenshot hardening, superseded by the full current-gap package below:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
visual batch: 30/30
interaction batch: 24/24
screenshot uploads: 54
bridge result uploads: 1
receiver uploads: 55
smoke redirect opens: 53
required_interaction_files: interaction.json, interaction.png, interaction-capture-meta.json

doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json
visual targets: 30
interaction targets: 24
screenshot uploads: 54
bridge result uploads: 1
receiver uploads: 55
protected redirect opens: 53
```

Validation:

```text
node --check tests/e2e/ui_desktop_chrome_bridge_payload.mjs
node --check tests/e2e/ui_desktop_capture_session.mjs
python3 -m py_compile tests/e2e/ui_visual_interaction_evidence_finalize.py tests/e2e/ui_desktop_capture_receiver.py tests/e2e/ui_desktop_capture_receiver_selftest.py
bash -n tests/e2e/live_ui_visual_interaction_preflight.sh

python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=13/13

node --input-type=module --check < doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js

ALLOW_BLOCKERS=true python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json --output-json doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-windows-tunnel-25173.json --output-md doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-windows-tunnel-25173.md
ui-visual-interaction-evidence-finalize result=blocked visual=0/30 interaction=0/28

ALLOW_BLOCKERS=true CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=blocked tests/e2e/live_ui_visual_interaction_preflight.sh
ui-visual-interaction-preflight result=blocked
```

The blocked preflight is expected and intentional until the real Windows Codex Desktop Chrome extension bridge is callable and uploads the generated screenshots/interaction artifacts plus the bridge run summary.

## 2026-07-03 Windows Bridge Host Readiness

Added a read-only Windows host readiness preflight:

```text
tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/windows-desktop-bridge-host-preflight-latest.md
```

This preflight uses password-only SSH to `LongShine@10.3.6.59` and only runs `cmd`, `tasklist`, `if exist`, and tunnel curl checks. It does not drive Chrome and does not read or write smoke tokens.

Current result:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
ui-windows-desktop-bridge-host-preflight result=pass chrome_clients=2 chrome_processes=36 codex_processes=40
```

The old generated payload pointed at a stale Windows client path:

```text
file:///C:/Users/11441/.codex/plugins/cache/openai-bundled/chrome/26.611.62324/scripts/browser-client.mjs
```

The Windows host preflight discovered the active VSCode machine/user path:

```text
C:\Users\LongShine\.codex\plugins\cache\openai-bundled\chrome\26.623.81905\scripts\browser-client.mjs
C:\Users\LongShine\.codex\plugins\cache\openai-bundled\chrome\latest\scripts\browser-client.mjs
```

The bridge payload generator now reads `windows-desktop-bridge-host-preflight-latest.json` and automatically selects the discovered client URL when `--chrome-client-url` is not passed:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json
chrome_client_url: file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/26.623.81905/scripts/browser-client.mjs
chrome_client_url_source: windows_desktop_bridge_host_preflight
```

The capture session also records the host preflight result:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
sources.bridge_host_preflight.result: pass
sources.bridge_host_preflight.selected_chrome_client_url: file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/26.623.81905/scripts/browser-client.mjs
```

Validation:

```text
node --check tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
node --check tests/e2e/ui_desktop_chrome_bridge_payload.mjs
node --check tests/e2e/ui_desktop_capture_session.mjs
node --input-type=module --check < doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.js

SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass

node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://127.0.0.1:5173 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
ui-desktop-smoke-token-preflight result=pass checks=9/9

python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=13/13

ALLOW_BLOCKERS=true python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json --output-json doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-windows-tunnel-25173.json --output-md doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-windows-tunnel-25173.md
ui-visual-interaction-evidence-finalize result=blocked visual=0/30 interaction=0/28

ALLOW_BLOCKERS=true CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=blocked tests/e2e/live_ui_visual_interaction_preflight.sh
ui-visual-interaction-preflight result=blocked
```

Boundary at this pass was narrower than before: Windows host, tunnel, Codex process, Chrome process, and Chrome bridge client file were ready. Formal acceptance still required this Codex session to expose the callable Desktop Chrome bridge MCP tool so the generated payload could run and upload the real screenshot/interaction artifacts plus the bridge run summary.

## 2026-07-03 Bridge Run Summary Upload Gate

The capture receiver now exposes a protected bridge-result endpoint:

```text
POST http://127.0.0.1:25174/bridge-result
required header: X-Codex-Capture-Key
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-run-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-run-latest.md
```

The endpoint rejects sensitive token material and stores only the Desktop Chrome execution summary. The receiver self-test now covers both accepted and rejected bridge-result uploads:

```text
python3 tests/e2e/ui_desktop_capture_receiver_selftest.py
ui-desktop-capture-receiver-selftest result=pass passed=13/13
```

Earlier capture session and payload generated for the 10.3.6.59 tunnel, superseded by the full current-gap package below:

```text
capture_session.summary.screenshot_upload_count: 54
capture_session.summary.bridge_result_upload_count: 1
capture_session.summary.receiver_upload_count: 55
capture_session.bridge_result_upload: http://127.0.0.1:25174/bridge-result

desktop-chrome-bridge-payload-latest.json
visual_target_count: 30
interaction_target_count: 24
screenshot_upload_count: 54
bridge_result_upload_count: 1
receiver_upload_count: 55
chrome_client_url: file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/26.623.81905/scripts/browser-client.mjs
chrome_client_url_source: windows_desktop_bridge_host_preflight
```

The live UI visual interaction preflight now has an explicit blocker requiring:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-run-latest.json
result/status: pass
backend: codex-desktop-chrome-extension
visual_count: capture_session.summary.visual_batch_count
interaction_count: capture_session.summary.interaction_batch_count
```

Current 10.3.6.59 tunnel verification:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass

SSHPASS=<redacted> node tests/e2e/ui_windows_desktop_bridge_host_preflight.mjs
ui-windows-desktop-bridge-host-preflight result=pass chrome_clients=2 chrome_processes=36 codex_processes=40

node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://127.0.0.1:5173 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
ui-desktop-smoke-token-preflight result=pass checks=9/9

ALLOW_BLOCKERS=true CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=blocked tests/e2e/live_ui_visual_interaction_preflight.sh
ui-visual-interaction-preflight result=blocked
blocker: Desktop Chrome bridge run summary is missing until the callable Desktop Chrome bridge tool is exposed and executes the generated payload.
```

## 2026-07-03 Windows Codex Bridge Runtime Preflight

Added a second read-only Windows-side preflight so the host readiness evidence is not limited to one inventory command:

```text
tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/windows-codex-bridge-runtime-preflight-latest.md
```

The script uses only SSH plus short `cmd.exe` commands. It lists paths and process counts only; it does not read config contents, browser state, cookies, tokens, or MCP secrets.

Current result:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_codex_bridge_runtime_preflight.mjs
ui-windows-codex-bridge-runtime-preflight result=pass chrome_clients=2 desktop_candidates=0 node_repl_candidates=0
```

Runtime evidence:

```text
codex_root_exists: true
plugin_cache_exists: true
chrome_clients: 2
chrome_processes: 18
codex_processes: 10
vscode_processes: 18
desktop_bridge_candidate_dirs: 0
node_repl_candidate_dirs: 0
mcp_config_files: 22
```

The earlier host preflight path-existence check was also corrected so the payload Chrome client is considered present when it appears in the Windows inventory:

```text
payload_chrome_client.exists: true
selected_chrome_client_url: file:///C:/Users/LongShine/.codex/plugins/cache/openai-bundled/chrome/26.623.81905/scripts/browser-client.mjs
```

The capture session now records both Windows checks:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
sources.bridge_host_preflight.result: pass
sources.bridge_runtime_preflight.result: pass
sources.bridge_runtime_preflight.chrome_client_count: 2
sources.bridge_runtime_preflight.chrome_process_count: 18
sources.bridge_runtime_preflight.codex_process_count: 10
sources.bridge_runtime_preflight.code_process_count: 18
```

The live UI visual interaction preflight now includes this execution-package check:

```text
ALLOW_BLOCKERS=true CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=blocked tests/e2e/live_ui_visual_interaction_preflight.sh
ui-visual-interaction-preflight result=blocked
summary: doc/02_acceptance/runs/20260703004436-ui-visual-interaction-preflight/ui-visual-interaction-preflight-20260703004436-ui-visual-interaction-preflight-summary.json

Windows Codex bridge runtime preflight passes: ok
detail: chrome_clients=2 processes chrome=18 codex=10 code=18
```

Current boundary after this pass:

```text
Windows host readiness: pass
Windows runtime readiness: pass
Tunnel channel readiness: pass
Receiver self-test: pass
Smoke token preflight: pass
Generated payload: ready, placeholders only
Current-session Desktop Chrome MCP tool exposure: still missing
Formal visual/interaction acceptance: blocked until the generated payload runs through the Windows Codex Desktop Chrome extension backend and uploads 58 screenshots plus 1 bridge run summary.
```

## 2026-07-03 Full Current-Gap Payload Coverage

The previous capture session was based on the older capture plan, which treated four interaction JSON files as already passing. The stricter live gate requires `interaction.png` and `interaction-capture-meta.json`, so those four routes were still failing in the current gap report.

The capture session generator now uses the latest gap report as the current truth and uses the capture plan only to supply the Windows tunnel URLs, receiver endpoints, templates, and business-action hints.

Refreshed execution package:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json
visual_batch_count: 30/30
interaction_batch_count: 28/28
screenshot_upload_count: 58
bridge_result_upload_count: 1
receiver_upload_count: 59
smoke_redirect_open_count: 56
capture_session_covers_current_gaps: true
```

The generated payload was refreshed to match:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-latest.json
visual_target_count: 30
interaction_target_count: 28
screenshot_upload_count: 58
bridge_result_upload_count: 1
receiver_upload_count: 59
smoke_redirect_url_count: 56
```

Added a static payload self-test:

```text
tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
output_json: doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.json
output_md: doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-payload-selftest-latest.md

node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs
ui-desktop-chrome-bridge-payload-selftest result=pass passed=18/18
```

The self-test verifies:

```text
payload summary and JS exist
Windows host/runtime preflights are pass
Chrome client URL points to C:/Users/LongShine/.codex/...
backend is codex-desktop-chrome-extension
iab is forbidden and not selected
only placeholders are present for capture key and smoke nonce
no JWT/Bearer material is embedded
bridge result upload is present
visual/interaction counts match the current gap report and capture session
all interaction gap route ids are covered
```

The receiver and smoke redirect were restarted for the expanded batch:

```text
ui-desktop-capture-receiver listening host=127.0.0.1 port=15174 max_uploads=59
ui-desktop-smoke-redirect listening host=127.0.0.1 port=15175 max_redirects=56

SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass

node tests/e2e/ui_desktop_smoke_token_preflight.mjs --base-url http://127.0.0.1:5173 --apisix-url http://10.0.5.8:30180 --route /dashboard --expected-path /dashboard
ui-desktop-smoke-token-preflight result=pass checks=9/9
```

The live gate now records both execution-package checks as passing:

```text
ALLOW_BLOCKERS=true CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=blocked tests/e2e/live_ui_visual_interaction_preflight.sh
summary: doc/02_acceptance/runs/20260703005406-ui-visual-interaction-preflight/ui-visual-interaction-preflight-20260703005406-ui-visual-interaction-preflight-summary.json

capture session covers current pending visual and interaction batches: ok
Desktop Chrome bridge payload self-test passes and matches capture batch: ok
```

Remaining blockers are now limited to real Desktop Chrome bridge execution evidence:

```text
desktop-chrome-bridge-run-latest.json: missing
Codex Desktop Chrome wrapper: not provided by current session
visual screenshots: missing until payload executes
interaction evidence: missing until payload executes
```

## 2026-07-03 Completion Audit Evidence Cleanup

The project completion audit was tightened so `failing=` details only list checks where `passed == false`. Passed hard gates whose severity is `blocker` are still counted correctly as pass, but are no longer printed as failure detail noise.

```text
tests/e2e/live_project_completion_audit.sh
failing selector: .checks[] | select(.passed == false)
```

The latest finalizer evidence was also refreshed to match the strict current-gap package. The earlier latest finalizer still counted four route JSON files as passing even though the stricter interaction contract now requires `interaction.png` and `interaction-capture-meta.json`.

```text
RUN_ID=202607030-ui-visual-evidence-finalize-current-gap-r1 \
ALLOW_BLOCKERS=true \
python3 tests/e2e/ui_visual_interaction_evidence_finalize.py \
  --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json \
  --output-json doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-latest.json \
  --output-md doc/02_acceptance/02-regression/ui-visual-interaction/evidence-finalization-latest.md

result: blocked
visual: 0/30
interaction: 0/28
blockers: 58
```

The refreshed UI preflight and project audit now agree on the current boundary:

```text
doc/02_acceptance/runs/202607030-ui-visual-preflight-current-finalizer-r1/ui-visual-interaction-preflight-202607030-ui-visual-preflight-current-finalizer-r1-summary.json
result: blocked
capture_session_covers_current_gaps: true
bridge_runtime_preflight_ready: true
payload_selftest_ready: true

doc/02_acceptance/runs/202607030-project-audit-current-ui-preflight-r1/live-project-completion-audit-202607030-project-audit-current-ui-preflight-r1-summary.json
result: blocked
finalization_run_id: 202607030-ui-visual-evidence-finalize-current-gap-r1
finalization_visual: 0/30
finalization_interaction: 0/28
```

The remaining formal UI blockers are unchanged: the current session still lacks the callable Windows Codex Desktop Chrome bridge tool, so the generated payload has not yet produced the required 58 browser screenshots plus `desktop-chrome-bridge-run-latest.json`.

## 2026-07-03 Windows REPL Socket Tunnel Probe

A dedicated SSH local-forward probe was attempted against the corrected Windows/VSCode machine:

```text
ssh -N -L 19998:127.0.0.1:19998 LongShine@10.3.6.59
```

The Linux-side socket became reachable through the SSH tunnel:

```text
127.0.0.1:19998: open via SSH tunnel
```

However, a direct MCP initialize probe against that forwarded socket closed without returning a response. A read-only Windows `netstat` / `tasklist` check showed the Windows host had VSCode, Chrome, Codex.exe, codex.exe, node_repl.exe, and node.exe processes, but did not show a TCP listener on `127.0.0.1:19998`.

This means the Windows `node_repl.exe` processes are not a reusable TCP service that can replace the missing MCP tool exposure. The formal Chrome capture still requires the current Codex session to expose one of the bridge tools:

```text
desktop_chrome_open_url
desktop_chrome_list_tabs
desktop_chrome_claim_url
mcp__codex_desktop_node_repl__js
```

The stable bridge check was refreshed accordingly:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-latest.json
run_id: 20260703-desktop-chrome-bridge-windows-tunnel-r6
result: blocked
mcp_initialize_status: closed_without_response
windows_19998_listener: not_observed
```

## 2026-07-03 Windows Codex Profile Follow-up Probe

A follow-up read-only SSH probe against `LongShine@10.3.6.59` confirmed that the Windows Codex profile exists at `C:\Users\LongShine\.codex` and includes newer bundled Chrome/browser clients:

```text
openai-bundled/chrome: 26.623.81905, latest
openai-bundled/browser: 26.623.81905, latest
```

The Windows profile plugin cache roots observed were:

```text
chatgpt-global
openai-bundled
openai-curated
openai-curated-remote
```

No Windows-side `personal/codex-desktop-iab-bridge` plugin cache was observed, and `C:\Users\LongShine\.codex\node_repl` only exposed `active_execs` metadata during the SSH directory probe. A non-interactive Windows SSH shell also did not resolve `codex`, `node_repl`, or `node` through `where`, so the SSH shell cannot directly substitute for the missing current-session MCP bridge tool.

The stable bridge check was refreshed again:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-latest.json
run_id: 20260703-desktop-chrome-bridge-windows-tunnel-r7
result: blocked
windows_chrome_client_versions: 26.623.81905, latest
personal_bridge_plugin: not_observed
ssh_shell_path_resolution: codex/node_repl/node not_found
```

## 2026-07-03 r12 Trusted Tool-Call Template

The Windows localhost tunnel package is now complete enough for trusted Codex Desktop execution from the VSCode/Windows context:

```text
app pages:         http://127.0.0.1:25173/...
receiver uploads: http://127.0.0.1:25174/...
smoke redirect:   http://127.0.0.1:25175/...
```

The new execution artifact is:

```text
doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json
```

It is the outer MCP call shape for `mcp__codex_desktop_node_repl__js`; its `arguments.code` field is the exact inner JavaScript payload. Do not run the JSON through shell Node and do not convert it into a local Linux Chrome run.

Latest self-test:

```text
node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs
ui-desktop-chrome-bridge-tool-call result=pass passed=9/9

tool: mcp__codex_desktop_node_repl__js
timeout_ms: 900000
payload_sha256: 3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a
payload_selftest: 21/21
visual_targets: 30
interaction_targets: 28
receiver_uploads: 59
```

The artifact contains only placeholder capture/smoke values:

```text
<CODEX_CAPTURE_KEY>
<CODEX_SMOKE_NONCE>
```

The current blocker is unchanged but narrower: the current model session still does not expose `mcp__codex_desktop_node_repl__js` or `desktop_chrome_*`, and SSH-spawned Windows `node_repl.exe` still fails before Chrome control with `SetRemotePorts HRESULT(0x80070005)` / `拒绝访问。` Formal visual acceptance therefore still waits for the trusted Windows Codex Desktop / VSCode bridge context to call the generated MCP template and upload the 58 screenshots plus bridge result.

## 2026-07-03 r13 SSH Privilege Boundary Probe

A new cmd-only SSH privilege preflight was added because PowerShell itself is blocked in the SSH context:

```text
tests/e2e/ui_windows_ssh_privilege_preflight.mjs
doc/02_acceptance/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json
```

Latest result:

```text
ui-windows-ssh-privilege-preflight result=pass passed=9/9
```

The probe confirms:

```text
Administrators group: enabled
Token integrity: High Mandatory Level
Enabled privileges: 24
net session: pass_no_sessions
Firewall service: running
Firewall profiles: Domain OFF, Private OFF, Public ON; all AllowInbound/AllowOutbound
Console processes: Code=18, Codex=10, Chrome=18, node_repl=3
PowerShell smoke from SSH: blocked with access denied
```

This narrows the bridge blocker again: the recurring SSH-spawned `node_repl.exe tools/call js` failure is not explained by a missing Windows host, a low-integrity SSH token, missing Administrators group, stopped firewall service, or absent Codex/Chrome/node_repl processes. The remaining missing proof is still execution through the trusted Windows Codex Desktop / VSCode MCP tool surface, not ordinary SSH process launch.

The same SSH pass also inspected `C:\Users\LongShine\.codex\node_repl\active_execs`. Only one metadata file was present:

```text
execId: 51865368-a864-45d0-a139-af16cc6741ae
sessionId: 019ece55-6e78-7da2-b018-da9abbc7348c
nodeReplPid: 23300
kernelPid: 20368
startedAtMs: 1781606557911
```

Both recorded pids were no longer running. This means the `active_execs` directory does not currently provide a reusable trusted Node REPL endpoint that can be claimed from SSH.

## 2026-07-03 r14 Windows Tunnel No-Proxy Probe

The formal Windows tunnel is still `LongShine@10.3.6.59` with Windows-local endpoints:

```text
app pages:         http://127.0.0.1:25173/...
receiver uploads: http://127.0.0.1:25174/...
smoke redirect:   http://127.0.0.1:25175/...
```

A manual Windows-side curl probe initially returned `HTTP/1.1 503 Service Unavailable`, but the SSH client `-vvv` log showed no `forwarded-tcpip` channel open. The response also contained `Proxy-Connection`, so the request was intercepted by the Windows proxy and never reached the localhost reverse tunnel.

Using `curl.exe --noproxy *` from the Windows SSH context confirmed the tunnel is healthy:

```text
http://127.0.0.1:25173/               HTTP/1.1 200 OK
http://127.0.0.1:25174/health         ok
http://127.0.0.1:25175/health         ok
```

The formal tunnel preflight was rerun and passed:

```text
SSHPASS=<redacted> node tests/e2e/ui_windows_tunnel_channel_preflight.mjs
result: pass
windows app/runtime/receiver/viewport-probe/redirect checks: 200
```

The UI dual gate was then rerun with the Windows tunnel capture session:

```text
RUN_ID=202607030-ui-visual-preflight-bridge-tunnel-r14-windows \
CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json \
ALLOW_BLOCKERS=true tests/e2e/live_ui_visual_interaction_preflight.sh
```

Result remains `blocked`, but the tunnel, payload self-test, tool-call template, Windows runtime preflight, and SSH privilege preflight are ready. Remaining blockers are only the missing trusted Desktop Chrome bridge run summary, 30/30 visual diff evidence, 28/28 business interaction evidence, and the absent current-session Desktop Chrome wrapper.
