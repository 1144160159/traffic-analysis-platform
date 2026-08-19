# Codex Loop Runtime Preflight

- status: `RUNTIME_PREFLIGHT_DEGRADED`
- generated_at: `2026-06-24T06:48:10`
- profile: `conservative`
- queue_backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/.loop/queue.sqlite3`
- repo_free_mb: `1718604`
- evidence_free_mb: `1718604`
- available_memory_mb: `1934740`
- monitor_status: `RESOURCE_MONITOR_DEGRADED`
- monitor_cpu_busy_percent: `4`
- monitor_queue_claimed: `0`
- dirty_items: `557`

## Findings
- `warning` `THREAD_COUNT_WARN`: thread_count is 14030; warning threshold is 8000 or higher.

## Guardrail
- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.
- Warnings are visible in health and release manifests, but only blockers stop execution.
