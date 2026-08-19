# Codex Loop Runtime Preflight

- status: `RUNTIME_PREFLIGHT_DEGRADED`
- generated_at: `2026-06-24T03:11:40`
- profile: `sqlite_pool_plan`
- queue_backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/.loop/queue.sqlite3`
- repo_free_mb: `1718730`
- evidence_free_mb: `1718730`
- available_memory_mb: `1934523`
- monitor_status: `RESOURCE_MONITOR_DEGRADED`
- monitor_cpu_busy_percent: `4`
- monitor_queue_claimed: `0`
- dirty_items: `557`

## Findings
- `warning` `THREAD_COUNT_WARN`: thread_count is 13873; warning threshold is 8000 or higher.

## Guardrail
- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.
- Warnings are visible in health and release manifests, but only blockers stop execution.
