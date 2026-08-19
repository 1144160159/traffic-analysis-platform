# Whitelist Governance Live Preflight

- Run: `20260630-whitelist-governance-preflight-r1`
- Result: `blocked`
- Checks: `25/26` passed, `1` blockers, `0` warnings
- Whitelist entry: `0ab9346b-edf0-4046-adb0-270be30def34`
- Match value: `codex-20260630-whitelist-governance-preflight-r1.example.test`

This gate closes the whitelist governance business loop:
draft creation, approval submission, activation, expiry extension, disable,
match-check behavior, viewer write denial, cross-tenant isolation, PostgreSQL
persistence, and audit-log queryability.
