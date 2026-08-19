# Compliance Audit Live Preflight

- Run: `compliance-r365-final-20260719110103`
- Result: `blocked`
- Checks: `51/52` passed, `1` blockers, `0` warnings
- Report type: `weekly`
- Report id: `698ac282-7fb3-4ed3-97df-fa5ca3b99c1d`

This gate closes the compliance/audit business loop for the audit-config menu:
admin report generation, report query, audit trail query, audit-log page API
query, PostgreSQL persistence, tenant isolation, and viewer write denial.
