# Compliance Audit Live Preflight

- Run: `compliance-r367-final-20260719110622`
- Result: `pass`
- Checks: `52/52` passed, `0` blockers, `0` warnings
- Report type: `weekly`
- Report id: `1c5f7a8b-2516-4a4c-87f4-3961aa81920a`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
