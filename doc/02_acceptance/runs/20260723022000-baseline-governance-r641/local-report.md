# Baseline Governance Live Preflight

- Run: `20260723022000-baseline-governance-r641`
- Result: `pass`
- Checks: `65/65` passed, `0` blockers, `0` warnings
- Baseline: `asset:10.0.5.8`

This gate covers five baseline dimensions, list/detail reads, the legacy reset,
audited governance command persistence, viewer write denial, PostgreSQL action,
version and outbox rows, audit-log queryability, and cross-tenant isolation.
