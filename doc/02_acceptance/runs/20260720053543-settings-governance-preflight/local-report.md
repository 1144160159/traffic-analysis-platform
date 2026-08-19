# Settings Governance Live Preflight

- Run: `20260720-settings-governance-r486`
- Result: `blocked`
- Checks: `65/66` passed, `1` blockers, `0` warnings
- Settings user: `0119603f-453b-49b4-a94d-1f5f150ce2c4`
- Created token: `5e8b09ed-1aac-42bc-bc89-3b13d873a59e`
- Regenerated token: `332bd704-211a-47e0-b845-0a6b9a917756`

This gate closes the system settings loop: frontend settings action contract,
display preferences save/read through auth-service, API token create/scope
update/regenerate/revoke/validate, viewer write denial, tenant isolation,
PostgreSQL persistence, and token audit-log queryability. Token-bearing API
responses are stored only in temporary files; regression artifacts are redacted.
