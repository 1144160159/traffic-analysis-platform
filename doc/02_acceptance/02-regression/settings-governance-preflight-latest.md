# Settings Governance Live Preflight

- Run: `20260630-settings-governance-preflight-r1`
- Result: `pass`
- Checks: `45/45` passed, `0` blockers, `0` warnings
- Settings user: `0119603f-453b-49b4-a94d-1f5f150ce2c4`
- Created token: `faf0492c-9ad5-44d0-b96b-f6729286771a`
- Regenerated token: `ae49bb57-0bf8-4430-91dc-7ff3a12f09dd`

This gate closes the system settings loop: frontend settings action contract,
display preferences save/read through auth-service, API token create/scope
update/regenerate/revoke/validate, viewer write denial, tenant isolation,
PostgreSQL persistence, and token audit-log queryability. Token-bearing API
responses are stored only in temporary files; regression artifacts are redacted.
