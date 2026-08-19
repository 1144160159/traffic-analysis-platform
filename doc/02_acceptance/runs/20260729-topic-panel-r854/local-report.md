# Topic Governance Live Preflight

- Run: `20260729-topic-panel-r854`
- Result: `pass`
- Checks: `48/48` passed, `0` blockers, `0` warnings
- Saved view: `6d94f39b-0579-4e6f-97ef-4137f874a41f`
- Subscription: `4507012c-9280-4173-adb3-01c729b5beb7`
- Exports: report `6bc85f22-55a6-4893-9e4e-a2c8bbd5e251`, evidence package `e20cb13c-98f2-4098-8cb1-ed7f5e71885e`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
