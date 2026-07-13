# Baseline Governance Live Preflight

- Run: `20260630-baseline-governance-preflight-r2`
- Result: `pass`
- Checks: `16/16` passed, `0` blockers, `0` warnings
- Baseline: `ip:10.0.5.8`

This gate closes the behavior-baseline reset loop: baseline list/detail read,
frontend action contract, admin reset, viewer write denial, PostgreSQL
`behavior_baseline_resets` persistence, audit-log queryability, and
cross-tenant audit isolation.
