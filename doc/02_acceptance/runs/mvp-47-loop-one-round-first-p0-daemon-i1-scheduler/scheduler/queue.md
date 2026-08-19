# Codex Loop Scheduler Queue

- run_id: `mvp-47-loop-one-round-first-p0-daemon-i1-scheduler`
- status: `LOCK_ACQUIRED`
- selected: `1`
- deferred: `0`

## Selected
- `CLE-P0-SCREEN-001` score `1515` attempt `2/3` paths: web/ui/src/, web/ui/e2e/, go/control-plane/internal/auth/, doc/02_acceptance/

## Deferred
- none

## Resource Quota
- enabled: `True`
- policy: `scripts/codex_loop/policies/resource-quotas.yaml`
- usage: `{'total_weight': 2, 'lanes': {'UI Rebuild': 1}, 'lane_groups': {'frontend': 1}, 'modes': {'local': 1}, 'data_modes': {'live_existing': 1}, 'subsystems': {'web/ui': 1, 'go/control-plane/internal/auth': 1}, 'selected': ['CLE-P0-SCREEN-001']}`
- quota deferred: none

## Lock
- acquired: `True`
- path: `doc/02_acceptance/runs/.locks/workspace.lock`

## Persistent Queue
- path: `doc/02_acceptance/runs/.loop/queue.json`
- enqueued: `0`
- updated: `0`
- skipped: `1`
- counts: `{'done': 1}`

## Guardrail
- The scheduler writes queue intent only; it does not run workflow.py by itself.
- One workspace lock protects code-modifying tasks from overlapping in this MVP.
