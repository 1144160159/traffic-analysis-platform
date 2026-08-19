# Codex Loop Resource Monitor

- run_id: `mvp-34-soak-service-once-c001-resource-monitor`
- status: `RESOURCE_MONITOR_DEGRADED`
- policy: `scripts/codex_loop/policies/resource-observability.yaml`
- cpu_busy_percent: `5`
- load_1_per_cpu_percent: `4`
- mem_available_mb: `1934739`
- repo_free_mb: `1718604`
- evidence_free_mb: `1718604`
- queue_counts: `{'done': 1}`
- recommended_max_workers: `1`
- allow_new_work: `True`
- allow_parallel_workers: `False`

## Findings
- `warning` `THREAD_COUNT_WARN`: thread_count is 14035; warning threshold is 8000 or higher.

## Guardrail
- Resource monitoring controls admission pressure; it does not execute tasks or close product evidence.
- DEGRADED means serial execution is recommended; BLOCKED means new automated work should not start.
