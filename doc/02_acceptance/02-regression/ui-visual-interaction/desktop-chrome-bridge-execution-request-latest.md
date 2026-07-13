# Desktop Chrome Bridge Execution Request

- Result: `ready_for_trusted_context`
- Generated: `2026-07-02T20:12:54.621Z`
- Tool: `mcp__codex_desktop_node_repl__js`
- Timeout: `900000` ms
- Payload SHA256: `3f0f6d0128d0e4b8807e7addd52d9371a06f070cedceb2116f3a1148743a9e4a`
- Visual targets: `30`
- Interaction targets: `28`
- Receiver uploads: `59`
- Bridge run result: `missing`

This is an execution request for the trusted Windows Codex Desktop / VSCode Chrome bridge context. It is not visual acceptance evidence by itself.

## Required Trusted Tool Call

- Call `mcp__codex_desktop_node_repl__js` with the JSON arguments from `doc/02_acceptance/02-regression/ui-visual-interaction/desktop-chrome-bridge-tool-call-latest.json`.
- Replace only `<CODEX_CAPTURE_KEY>` and `<CODEX_SMOKE_NONCE>` inside `arguments.code` at execution time.
- Do not write concrete capture keys, smoke nonces, JWTs, or bearer tokens into repo files.
- The payload must use the Chrome extension backend and must not fall back to iab.

## Readiness Checks

| check | result | detail |
|---|---|---|
| Windows localhost tunnel endpoints are reachable | pass | requires 127.0.0.1:25173/25174/25175 with Windows proxy bypass |
| Windows host has Chrome/Codex/VSCode and bridge client files | pass | chrome_clients=2 chrome_processes=36 |
| Windows Codex bridge runtime prerequisites are present | pass | chrome_clients=2 processes={"chrome":18,"codex":10,"code":18} |
| Windows SSH privilege probe narrows ordinary-SSH boundary | pass | high_integrity=true privileges=24 |
| Desktop Chrome bridge payload self-test passes | pass | checks=21/21 |
| Outer MCP tool-call template is ready | pass | tool=mcp__codex_desktop_node_repl__js checks=9/9 |
| UI preflight is aligned to the Windows tunnel capture batch | pass | ui_result=blocked visual=0/30 interaction=0/28 |

## Boundary Evidence

| check | result | detail |
|---|---|---|
| Local Codex bridge plugin/proxy tool surface is diagnosed | pass | plugin=true backend_open=false tools_list=error: [Errno 111] Connection refused session_tools=not_exposed_in_current_chat_tool_surface |
| Windows active_execs/proxy channel probe is current | pass | channel=no_active_exec_metadata_or_proxy_listener active_records=0 live_records=0 node_repl_pids=20724,10700,19720 port19998=0 |
| Windows node_repl stdio JS and Chrome trust boundary smoke is current | pass | result=blocked_chrome_bridge direct_js=pass full_env_js=blocked chrome_no_env=blocked:native_pipe_or_trust_unavailable chrome=blocked:sandbox_firewall_denied |
| Windows scheduled-task node_repl Chrome bridge smoke is diagnosed | pass | result=blocked_node_repl_js create=false run=false runner=missing blocker=scheduled task create failed: 错误:占位程序接收到错误数据。 |

## Formal Closure After Trusted Run

- `RUN_ID=<run-id> ALLOW_BLOCKERS=false python3 tests/e2e/ui_visual_interaction_evidence_finalize.py --capture-plan doc/02_acceptance/02-regression/ui-visual-interaction/capture-plan-windows-tunnel-25173.json`
- `RUN_ID=<run-id> CAPTURE_SESSION=doc/02_acceptance/02-regression/ui-visual-interaction/capture-session-windows-tunnel-25173.json DESKTOP_CHROME_STATUS=pass ALLOW_BLOCKERS=false tests/e2e/live_ui_visual_interaction_preflight.sh`
- `RUN_ID=<run-id> ALLOW_BLOCKERS=false tests/e2e/live_project_completion_audit.sh`

