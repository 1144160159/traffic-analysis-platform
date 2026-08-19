# Codex Loop Scheduler Queue

- run_id: `mvp-15-sqlite-service-once-daemon-i1-scheduler`
- status: `LOCK_ACQUIRED`
- selected: `1`
- deferred: `0`

## Selected
- `CLE-P0-SCREEN-001` score `1515` attempt `2/3` paths: web/ui/src/, web/ui/e2e/, go/control-plane/internal/auth/, doc/02_acceptance/

## Deferred
- none

## Lock
- acquired: `True`
- path: `doc/02_acceptance/runs/.locks/workspace.lock`

## Persistent Queue
- path: `doc/02_acceptance/runs/.loop/queue.sqlite3`
- enqueued: `1`
- updated: `0`
- skipped: `0`
- counts: `{'queued': 1}`

## Guardrail
- The scheduler writes queue intent only; it does not run workflow.py by itself.
- One workspace lock protects code-modifying tasks from overlapping in this MVP.
