# Topic Governance Live Preflight

- Run: `20260727-topic-panel-r823`
- Result: `pass`
- Checks: `48/48` passed, `0` blockers, `0` warnings
- Saved view: `a4ae2bfa-a354-47ac-a9be-d11c44bfaa74`
- Subscription: `dea73d6b-1ea1-4774-9ee3-7c4b97cec7e8`
- Exports: report `084213ce-fd4d-4f0b-b3a1-ad9f54b4fc31`, evidence package `5ad55085-6ecf-4bba-96e0-d71ce14d3534`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
