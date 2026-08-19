# Topic Governance Live Preflight

- Run: `20260725-topic-domain-r754`
- Result: `pass`
- Checks: `48/48` passed, `0` blockers, `0` warnings
- Saved view: `af005cc3-b8ad-4d5a-9429-2bd949f12367`
- Subscription: `1081c626-fc2d-4ec5-80f7-03fdbac7fd5f`
- Exports: report `fe779f71-7032-48f4-b6fc-2e7e0d62332f`, evidence package `2587e2ad-cc56-4a16-9d10-25b210f68ee3`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
