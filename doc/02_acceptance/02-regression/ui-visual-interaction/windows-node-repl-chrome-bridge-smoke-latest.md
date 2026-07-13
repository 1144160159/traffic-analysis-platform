# Windows Node REPL Chrome Bridge Smoke

- Result: `blocked_chrome_bridge`
- Generated: `2026-07-02T20:21:21.508Z`
- Windows host: `10.3.6.59`
- MCP config: `pass`
- Direct JS smoke: `pass`
- Full-env JS smoke: `blocked`
- Chrome extension smoke without env: `blocked`
- Chrome no-env failure class: `native_pipe_or_trust_unavailable`
- Chrome extension smoke: `blocked`
- Chrome failure class: `sandbox_firewall_denied`

This smoke is intentionally narrow. It executes JavaScript through Windows node_repl over SSH stdio and attempts read-only Chrome extension target discovery. It does not capture UI screenshots and does not close visual acceptance.

## Blockers

- Full-env SSH-spawned node_repl minimal JS smoke failed
- No-env Chrome extension target smoke did not reach extension backend: native_pipe_or_trust_unavailable
- Full-env Chrome extension target smoke did not reach extension backend: sandbox_firewall_denied

