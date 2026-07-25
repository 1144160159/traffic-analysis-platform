# Topic Governance Live Preflight

- Run: `20260725-topic-domain-r758-r757`
- Result: `pass`
- Checks: `48/48` passed, `0` blockers, `0` warnings
- Saved view: `6109809a-ffa2-4674-bbe2-e977ce4a5c2e`
- Subscription: `e95bef11-37b1-49bb-89df-9290c0f75898`
- Exports: report `7a1cecb3-d2e8-4de7-ab43-69ca92681278`, evidence package `532c502e-fb7d-44c1-864b-938e4c123ace`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
