# Codex Loop Scheduler Queue

- run_id: `mvp-50-loop-one-round-first-p0-route-daemon-i1-scheduler`
- status: `LOCK_ACQUIRED`
- selected: `1`
- deferred: `0`

## Selected
- `CLE-P0-ROUTE-001` score `1970` attempt `1/3` paths: web/ui/src/, web/ui/e2e/, doc/02_acceptance/, doc/01_design/

## Deferred
- none

## Resource Quota
- enabled: `True`
- policy: `scripts/codex_loop/policies/resource-quotas.yaml`
- usage: `{'total_weight': 2, 'lanes': {'UI Rebuild': 1}, 'lane_groups': {'frontend': 1}, 'modes': {'local': 1}, 'data_modes': {'live_existing': 1}, 'subsystems': {'web/ui': 1, 'deployments/kubernetes/configmaps': 1}, 'selected': ['CLE-P0-ROUTE-001']}`
- quota deferred: none

## Lock
- acquired: `True`
- path: `doc/02_acceptance/runs/.locks/workspace.lock`

## Persistent Queue
- path: `doc/02_acceptance/runs/.loop/queue.json`
- enqueued: `1`
- updated: `0`
- skipped: `0`
- counts: `{'done': 1, 'queued': 1}`

## Guardrail
- The scheduler writes queue intent only; it does not run workflow.py by itself.
- One workspace lock protects code-modifying tasks from overlapping in this MVP.
