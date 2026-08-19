# Notification Governance Live Preflight

- Run: `20260630-notification-governance-preflight-r1`
- Result: `pass`
- Checks: `26/26` passed, `0` blockers, `0` warnings
- Silence rule: `f77ee1ad-0182-425e-9004-a51e86060ede`

This gate closes the notification part of the audit-config business loop:
settings update, inline-secret rejection, notification test send, silence rule
create/disable, viewer write denial, cross-tenant isolation, PostgreSQL
persistence, and audit-log queryability.
