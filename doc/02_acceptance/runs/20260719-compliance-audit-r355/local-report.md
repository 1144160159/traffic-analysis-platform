# Compliance Audit Live Preflight

- Run: `20260719-compliance-audit-r355`
- Result: `pass`
- Checks: `27/27` passed, `0` blockers, `0` warnings
- Report type: `weekly`
- Report id: `212a65cf-88ce-410c-8d04-fad960900a4c`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
