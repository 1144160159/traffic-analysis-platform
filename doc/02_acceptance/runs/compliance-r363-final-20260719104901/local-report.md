# Compliance Audit Live Preflight

- Run: `compliance-r363-final-20260719104901`
- Result: `pass`
- Checks: `49/49` passed, `0` blockers, `0` warnings
- Report type: `weekly`
- Report id: `6803bb33-466c-433d-b380-9e0c2c61fe8a`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
