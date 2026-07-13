# Topic Governance Live Preflight

- Run: `20260630-topic-governance-preflight-r2`
- Result: `pass`
- Checks: `42/42` passed, `0` blockers, `0` warnings
- Saved view: `105e481b-bd4c-4137-81c6-9dec6f6cba62`
- Subscription: `e688a990-3053-455e-9c0a-e21c1851c30a`
- Exports: report `3b62dfcb-c762-4f6e-a7ea-dd82c92e5f15`, evidence package `8329fdf4-bc6d-43bc-8aea-655ab8460c4f`

This gate closes the topic-panel governance loop:
readable tunnel/exfil/APT topic pages, saved view create/share/favorite,
topic scope update, subscription create/disable, report and evidence package
exports, viewer write denial, cross-tenant isolation, PostgreSQL persistence,
and audit-log queryability.
