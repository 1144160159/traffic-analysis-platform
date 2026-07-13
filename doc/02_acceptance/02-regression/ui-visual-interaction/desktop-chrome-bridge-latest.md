# Desktop Chrome Bridge Check

- Run ID: `20260703-desktop-chrome-bridge-windows-tunnel-r13`
- Checked at: `2026-07-03T02:40:29+08:00`
- Result: `blocked`
- Required backend: `codex-desktop-chrome-extension`
- Fallback allowed: `false`

## Probes

| Tool | Operation | Result | Error |
|---|---|---|---|
| `tool_search` | discover `desktop_chrome_*` or `mcp__codex_desktop_node_repl__js` in the current Codex session | `blocked` | Only unrelated app connector tools were exposed; Desktop Chrome / Desktop Node REPL tools were not callable. |
| `ssh -L 19998:127.0.0.1:19998 LongShine@10.3.6.59` | tunnel current Linux `127.0.0.1:19998` to the Windows Codex Desktop host | `blocked` | The local tunnel port opened, but an MCP initialize probe closed without a response. |
| `ssh LongShine@10.3.6.59 cmd /c netstat/tasklist` | read-only Windows process and port check | `blocked` | Windows had VSCode, Chrome, Codex.exe, codex.exe, node_repl.exe, and node.exe processes, but no TCP listener on `127.0.0.1:19998` was visible. |
| `tool_search` | rediscover Desktop Chrome bridge tools after tunnel/stdout evidence refresh | `blocked` | Search for `codex desktop node repl js` / `desktop_chrome_open_url` still exposed unrelated app connector tools only; no callable Desktop Chrome wrapper or Desktop Node REPL tool was available in this Codex session. |
| `ssh LongShine@10.3.6.59 cmd /c where codex && where node_repl` | read-only Windows non-interactive PATH check | `blocked` | The Windows SSH shell PATH did not resolve `codex`, `node_repl`, or `node` through `where`; this does not disprove running Desktop processes, but it means the SSH shell cannot directly substitute for the missing MCP tool. |
| `ssh LongShine@10.3.6.59 cmd /c dir %USERPROFILE%\.codex` | read-only Windows Codex cache inventory | `blocked` | The Windows Codex profile exists and has openai-bundled chrome/browser clients at `26.623.81905/latest`, but the plugin cache exposed no personal `codex-desktop-iab-bridge` package and the `node_repl` directory only showed `active_execs` metadata. |
| `ssh LongShine@10.3.6.59 -> codex.exe remote-control start --json` | check whether Windows Codex CLI can start the app-server daemon lifecycle directly | `blocked` | The Windows Codex CLI returned `codex app-server daemon lifecycle is only supported on Unix platforms`. |
| `ssh LongShine@10.3.6.59 -> codex.exe mcp list/get node_repl` | read-only Windows Codex MCP configuration check | `pass` | `node_repl` is enabled, uses stdio transport, points to the bundled `cua_node` `node_repl.exe`, has startup timeout `120`, auth `Unsupported`, and includes `SKY_CUA_NATIVE_PIPE*`, `NODE_REPL_*`, `BROWSER_USE_*`, `CODEX_HOME`, and `CODEX_CLI_PATH` environment names. |
| `ssh LongShine@10.3.6.59 -> node_repl.exe JSONL stdio` | initialize Windows Codex Node REPL MCP server and list available tools | `pass` | Returned 3 tools: `js`, `js_add_node_module_dir`, `js_reset`. |
| `ssh LongShine@10.3.6.59 -> node_repl.exe tools/call js` | execute minimal `nodeRepl.write` JavaScript through the Windows Codex Node REPL MCP server | `blocked` | `tools/call js` returned `isError=true` because the Node REPL kernel exited unexpectedly. Diagnostics reported Windows sandbox firewall rule creation denied: `helper_firewall_rule_create_or_add_failed` / `SetRemotePorts failed` / `HRESULT(0x80070005)` / `拒绝访问。` |
| `ssh LongShine@10.3.6.59 -> node_repl.exe tools/call js with explicit Windows Codex native-pipe env` | execute minimal `nodeRepl.write` after setting `CODEX_HOME`, `NODE_REPL_*`, `BROWSER_USE_*`, `CODEX_CLI_PATH`, `SKY_CUA_NATIVE_PIPE`, and `SKY_CUA_NATIVE_PIPE_DIRECTORY` in the SSH-spawned cmd context | `blocked` | The full-env probe still returned `isError=true` with the same Windows sandbox failure before Chrome control: `helper_firewall_rule_create_or_add_failed` / `SetRemotePorts failed` / `HRESULT(0x80070005)` / `拒绝访问。` |
| `temporary local TCP 127.0.0.1:19998 -> ssh stdio node_repl.exe -> node-repl-mcp-proxy.py` | expose Windows stdio `node_repl.exe` behind the local proxy endpoint expected by `/root/.codex/bin/node-repl-mcp-proxy.py` and list proxy tools | `pass` | Returned 8 tools: `js`, `js_add_node_module_dir`, `js_reset`, `desktop_chrome_list_tabs`, `desktop_iab_list_targets`, `desktop_chrome_open_url`, `desktop_chrome_claim_url`, `desktop_chrome_cleanup_blank_tabs`. |
| `node-repl-mcp-proxy.py via temporary local TCP bridge` | execute minimal `nodeRepl.write` JavaScript through the proxy-wrapped `js` tool | `blocked` | The proxy-wrapped `js` tool returned the same Node REPL kernel failure as direct SSH stdio: Windows sandbox firewall rule creation denied at `SetRemotePorts` with `HRESULT(0x80070005)` / `拒绝访问。` |
| `node-repl-mcp-proxy.py via temporary local TCP bridge` | execute `desktop_chrome_list_tabs` wrapper through the proxy-wrapped Desktop Chrome tool | `blocked` | The wrapped Desktop Chrome tool reached the same lower-level `js` kernel failure before Chrome control: Windows sandbox firewall rule creation denied at `SetRemotePorts` with `HRESULT(0x80070005)` / `拒绝访问。` |
| `tool_search` | rediscover Desktop Chrome bridge tools in the current Codex session after r10 evidence refresh | `blocked` | Search for Chrome / control-chrome / codex desktop node repl / `desktop_chrome_*` still exposed unrelated app connector tools only; no callable Desktop Chrome wrapper or Desktop Node REPL tool was available in this Codex session. |
| `node tests/e2e/ui_windows_tunnel_channel_preflight.mjs` | verify Linux and Windows localhost tunnel endpoints for app, receiver, viewport probe, and smoke redirect | `pass` | Windows `127.0.0.1:25173/screen`, `25174/health`, `25174/viewport-probe`, and `25175/health` returned `200`. |
| `node tests/e2e/ui_desktop_chrome_bridge_payload.mjs && node tests/e2e/ui_desktop_chrome_bridge_payload_selftest.mjs` | regenerate and self-test the trusted Windows Desktop Chrome payload package | `pass` | Payload self-test passed `21/21`; the JS parses as async Desktop Node REPL code, covers 30 visual targets and 28 interaction routes, uploads 59 artifacts, and target URLs use Windows-local `127.0.0.1:25173/25174/25175` instead of direct `10.0.5.8:30180` capture URLs. |
| `node tests/e2e/ui_desktop_chrome_bridge_tool_call.mjs` | generate and self-test the outer MCP tool-call template for the trusted Desktop Node REPL | `pass` | Tool-call template passed `9/9`, targets `mcp__codex_desktop_node_repl__js`, uses timeout `900000`, binds payload SHA256 `3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a`, covers 30 visual plus 28 interaction captures, and keeps only placeholder capture/smoke values. |
| `node tests/e2e/ui_windows_ssh_privilege_preflight.mjs` | verify Windows SSH token privilege, firewall service, PowerShell boundary, and Desktop process visibility | `pass` | SSH privilege preflight passed `9/9`: Administrators group enabled, high-integrity token, 24 enabled privileges, `net session` not access-denied, firewall service running, Codex/Chrome/node_repl visible in console session, and PowerShell remains blocked in the SSH context. |
| `ssh LongShine@10.3.6.59 cmd /c inspect active_execs metadata` | inspect Windows Codex Node REPL `active_execs` metadata for a reusable trusted execution entry | `blocked` | Only one stale `active_execs` JSON was present from 2026-06-16; its `nodeReplPid` 23300 and `kernelPid` 20368 are no longer running, so it is not a reusable trusted bridge endpoint. |

## Windows Tunnel Probe

- Windows host: `10.3.6.59`
- Windows user: `LongShine`
- Local forward attempted: `127.0.0.1:19998 -> 127.0.0.1:19998`
- Local socket status: `open_via_ssh_tunnel`
- MCP initialize status: `closed_without_response`
- Windows process evidence: VSCode, Chrome, Codex, node_repl, and node were present
- Windows `127.0.0.1:19998` listener: `not_observed`
- Direct SSH stdio MCP initialize/tools-list: `pass`
- Direct SSH stdio tools: `js`, `js_add_node_module_dir`, `js_reset`
- Direct SSH stdio `tools/call js`: `blocked` by Windows sandbox firewall rule creation denial in the SSH-spawned context
- Windows Codex CLI `remote-control start --json`: `blocked` because app-server daemon lifecycle is Unix-only
- Windows Codex MCP `node_repl`: `enabled`, stdio transport, auth `Unsupported`, with native-pipe, browser-use, and node-repl environment names present
- Direct SSH stdio full-env `tools/call js`: `blocked` by the same Windows sandbox/firewall permission denial; copying the observed native-pipe env into the SSH-spawned cmd context did not make the JS kernel start
- Local proxy over SSH stdio tool list: `pass`, 8 tools including all `desktop_chrome_*` wrappers
- Local proxy over SSH stdio `tools/call js`: `blocked` by the same Windows sandbox/firewall permission denial
- Local proxy over SSH stdio `desktop_chrome_list_tabs`: `blocked` before Chrome control by the same lower-level `js` kernel failure
- Temporary local `127.0.0.1:19998` listener: stopped after the probe
- Windows localhost tunnel channel preflight: `pass`
- Windows localhost tunnel no-proxy probe: `pass`; Windows `curl.exe --noproxy *` returns `200` for `127.0.0.1:25173/screen`, `25174/health`, `25174/viewport-probe`, and `25175/health`; proxy-mediated `503` responses are false negatives.
- Generated trusted-context payload self-test: `pass`, `21/21`
- Generated payload URL policy: all capture URL templates use Windows-local `127.0.0.1:25173/25174/25175` endpoints for the `10.3.6.59` Chrome run
- Generated outer MCP tool-call template: `pass`, `9/9`
- Tool-call target: `mcp__codex_desktop_node_repl__js`
- Tool-call timeout: `900000` ms
- Tool-call payload SHA256: `3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a`
- Tool-call artifact: `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json`
- Windows SSH privilege preflight: `pass`, `9/9`
- SSH token: Administrators group enabled, high-integrity, 24 enabled privileges
- SSH `net session` admin check: `pass_no_sessions`
- Windows firewall service: `running`
- Console process counts: Code `18`, Codex `10`, Chrome `18`, node_repl `3`
- PowerShell smoke from SSH: blocked with access denied
- SSH privilege artifact: `doc/02_acceptance/02-regression/ui-visual-interaction/windows-ssh-privilege-preflight-latest.json`
- Windows Node REPL `active_execs`: stale metadata only; `nodeReplPid` 23300 and `kernelPid` 20368 are not running
- Windows Codex profile: `C:\Users\LongShine`
- Bundled Chrome/browser clients: `26.623.81905`, `latest`
- Plugin cache roots observed: `chatgpt-global`, `openai-bundled`, `openai-curated`, `openai-curated-remote`
- Personal bridge plugin on Windows profile: `not_observed`
- Windows SSH shell PATH resolution: `codex`, `node_repl`, and `node` were not resolved by `where`

## Acceptance Effect

The full current-gap UI package is ready, but the dual gate still cannot capture Desktop Chrome screenshots or route interactions from this model session. The Windows host is reachable through `10.3.6.59`. Direct SSH stdio can start the Windows Codex Node REPL MCP server, Windows Codex MCP configuration shows `node_repl` enabled over stdio with native-pipe environment names, and a temporary local `19998` bridge can expose the proxy's `desktop_chrome_*` wrapper tools. The r10 full-env probe confirmed that explicitly setting the observed Windows Codex native-pipe / Node REPL / Browser Use environment in the SSH-spawned cmd context still fails before Chrome control because Windows sandbox firewall rule creation is denied. The r11/r14 tunnel package refresh confirms the Windows localhost tunnel endpoints are reachable when Windows proxy bypass is explicit (`curl.exe --noproxy *`), and the generated trusted-context payload parses, passes `21/21` self-test checks, uses `127.0.0.1:25173/25174/25175` URLs only, and covers all 30 visual plus 28 interaction captures. The r12 package adds an outer MCP tool-call template for `mcp__codex_desktop_node_repl__js`; it passes `9/9` checks, binds payload SHA256 `3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a`, and keeps only placeholder capture/smoke values. The r13 SSH privilege preflight passes `9/9`: the SSH token is high-integrity, has the Administrators group enabled, `net session` is not access-denied, the firewall service is running, and Codex/Chrome/node_repl processes are visible in the interactive console session; PowerShell remains blocked in the SSH context. A follow-up `active_execs` probe found only stale Windows Node REPL metadata from 2026-06-16 with non-running pids, so there is no reusable trusted Node REPL endpoint to claim from ordinary SSH. Current strict finalization remains visual `0/30` and interaction `0/28` because the trusted Desktop Chrome MCP tool is still not exposed in this session.

Next action: run the bridge from the trusted Codex Desktop / VSCode session that owns the Desktop Node REPL / native-pipe / sandbox permissions, or restore the current-session Codex Desktop Chrome Bridge MCP tools, especially `desktop_chrome_open_url`, `desktop_chrome_list_tabs`, `desktop_chrome_claim_url`, or `mcp__codex_desktop_node_repl__js`. The r13 evidence shows SSH reachability, Windows admin/high-integrity token, firewall service, process presence, tunnel endpoints, payload, and outer MCP tool-call template are ready; the remaining missing proof is execution from the trusted Desktop Chrome MCP tool surface. Then call `mcp__codex_desktop_node_repl__js` with the JSON arguments from `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json` after replacing placeholders, and rerun the UI finalizer, UI preflight, and project completion audit.
