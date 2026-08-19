# Compliance Audit Live Preflight

- Run: `compliance-r363-20260719104509`
- Result: `pass`
- Checks: `44/44` passed, `0` blockers, `0` warnings
- Report type: `weekly`
- Report id: `ff5d76d2-546b-4b1a-8a28-419d3ce8d912`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
