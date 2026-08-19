# Codex Loop Runtime Preflight

- status: `RUNTIME_PREFLIGHT_READY`
- generated_at: `2026-06-24T02:51:45`
- profile: `sqlite_pool_plan`
- queue_backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/.loop/queue.sqlite3`
- repo_free_mb: `1718758`
- evidence_free_mb: `1718758`
- available_memory_mb: `1934593`
- dirty_items: `557`

## Findings
- none

## Guardrail
- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.
- Warnings are visible in health and release manifests, but only blockers stop execution.
