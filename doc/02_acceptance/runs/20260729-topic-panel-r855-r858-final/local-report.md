# Topic Governance Live Preflight

- Run: `20260729-topic-panel-r855-r858-final`
- Result: `pass`
- Checks: `52/52` passed, `0` blockers, `0` warnings
- Saved view: `0d99b5a3-7bd3-474d-ba7e-08e2e8f3ea96`
- Subscription: `48a378d6-e4e7-4712-be90-a2779cb6882c`
- Exports: report `b190cc03-33ff-4e1a-bf0e-07633ed3a683`, evidence package `2cdc6a05-13f0-48ec-91be-7b662c8157d5`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
