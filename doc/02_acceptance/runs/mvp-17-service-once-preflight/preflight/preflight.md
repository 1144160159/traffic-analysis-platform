# Codex Loop Runtime Preflight

- status: `RUNTIME_PREFLIGHT_READY`
- generated_at: `2026-06-24T02:12:56`
- profile: `sqlite_conservative`
- queue_backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/.loop/queue.sqlite3`
- repo_free_mb: `1718993`
- evidence_free_mb: `1718993`
- available_memory_mb: `1934358`
- dirty_items: `557`

## Findings
- none

## Guardrail
- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.
- Warnings are visible in health and release manifests, but only blockers stop execution.
