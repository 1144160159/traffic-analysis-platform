# Topic Governance Live Preflight

- Run: `20260729-topic-panel-r853`
- Result: `blocked`
- Checks: `46/48` passed, `2` blockers, `0` warnings
- Saved view: `b7c24640-280f-4964-92a8-8b3bf3df4a14`
- Subscription: `17412321-5d89-4c8e-bfe9-04778b64d9ab`
- Exports: report `f38ca97b-79f2-489b-9c5d-3bca54be3378`, evidence package `cf1d4b76-c544-450f-8ec4-c04b723ec36f`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
