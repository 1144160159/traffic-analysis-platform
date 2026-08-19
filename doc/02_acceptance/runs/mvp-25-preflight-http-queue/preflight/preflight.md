# Codex Loop Runtime Preflight

- status: `RUNTIME_PREFLIGHT_DEGRADED`
- generated_at: `2026-06-24T03:17:42`
- profile: `http_queue_worker_k8s`
- queue_backend: `http`
- queue_path: `http://127.0.0.1:18765`
- repo_free_mb: `1718894`
- evidence_free_mb: `1718894`
- available_memory_mb: `1934758`
- monitor_status: `RESOURCE_MONITOR_DEGRADED`
- monitor_cpu_busy_percent: `5`
- monitor_queue_claimed: `0`
- dirty_items: `557`

## Findings
- `warning` `THREAD_COUNT_WARN`: thread_count is 13881; warning threshold is 8000 or higher.

## Guardrail
- Preflight only proves runtime readiness for loop supervision; it does not close product tasks.
- Warnings are visible in health and release manifests, but only blockers stop execution.
