# Token Lifecycle Matrix Latest

- run_id: `20260629-token-lifecycle-r2`
- result: `pass`
- checks: `23/23`
- script: `tests/e2e/live_token_lifecycle_matrix.sh`
- evidence: `doc/02_acceptance/runs/20260629-token-lifecycle/`
- live image: `traffic/auth-service:token-lifecycle-20260629-r2`

Coverage:

- API token create/read/list validation through real APISIX.
- SHA-256 `api_tokens.token_hash` and `token_prefix` persistence in PostgreSQL.
- Viewer write denial and cross-tenant token metadata denial.
- Manual regenerate: old raw token rejected and new raw token accepted.
- Revoke and short-lived expiry rejection.
- Synchronous `audit_logs` rows for create, regenerate, and revoke.

