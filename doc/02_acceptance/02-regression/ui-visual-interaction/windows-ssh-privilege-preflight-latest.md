# Windows SSH Privilege Preflight

- Result: `pass`
- Host: `10.3.6.59`
- User: `LongShine`
- PowerShell smoke blocked: `true`
- Administrator group enabled: `true`
- High integrity token: `true`
- Enabled privileges: `24`
- Net session admin check: `pass_no_sessions`
- Firewall service running: `true`
- Process counts: Code `18`, Codex `10`, Chrome `18`, node_repl `3`, node `0`

## Inference

SSH reaches an elevated/high-integrity LongShine token with Codex, VSCode, Chrome, node_repl, and node processes visible in the interactive console session. The recurring node_repl JS failure is therefore not explained by a missing Windows host, missing admin group, stopped firewall service, or absent Desktop processes; it remains a trusted Desktop Node REPL / native-pipe / sandbox context boundary.

## Checks

- pass: SSH cmd probe returns structured output (status=0)
- pass: SSH token is in Administrators group (Mandatory group, Enabled by default, Enabled group, Group owner)
- pass: SSH token is high integrity (S-1-16-12288 present)
- pass: net session admin check is not access-denied (pass_no_sessions)
- pass: Windows firewall service is running (MpsSvc RUNNING)
- pass: Codex Desktop processes are visible in console session (codex=10)
- pass: Chrome processes are visible in console session (chrome=18)
- pass: Windows Node REPL processes are visible in console session (node_repl=3)
- pass: PowerShell smoke is blocked in SSH context (status=1 output=�ܾ����ʡ�)

