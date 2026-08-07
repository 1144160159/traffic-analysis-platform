# PostgreSQL remediation migrations

All alignment schema changes use immutable, lexically ordered files named
`YYYYMMDDHHMM_<description>.sql`. Services must not add production DDL at
startup. Each migration must document expand, backfill, verification, cutover
and rollback behavior in its header.

The directory now contains the additive W1+ remediation migrations. Their
presence in the repository or generated init ConfigMap does not prove that a
shared environment has applied them. Runtime evidence must read
`alignment_schema_migrations`, verify the expected tables and constraints, and
bind the result to the immutable candidate before any dependent feature is
enabled.

Routine service startup must not execute these files. Apply them in lexical
order through an approved migration window, stop on the first error, retain
the expand objects on rollback, and keep every dependent runtime flag disabled
until the migration ledger and schema verification pass.
