# Probe Operations Governance Live Preflight

- Run: `20260630-probe-ops-governance-r2`
- Result: `pass`
- Checks: `27/27` passed, `0` blockers, `0` warnings
- Probe: `codex-probe-ops-20260630-probe-ops-governance-r2`
- Config operation: `b153b31a-18a4-4d6e-a0f4-091130a4ae67`
- Cert operation: `42747662-c2b0-43db-8837-bb192fd2537d`
- Upgrade batch: `probe-batch-20260630070326.518139054`

This gate closes the probe-management loop for config push, connectivity
test, mTLS certificate rotation by Kubernetes secret reference, batch upgrade,
viewer denial, tenant isolation, PostgreSQL persistence and audit-log evidence.
