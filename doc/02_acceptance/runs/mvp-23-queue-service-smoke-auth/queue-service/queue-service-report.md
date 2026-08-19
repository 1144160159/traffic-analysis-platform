# Codex Loop Queue Service

- run_id: `mvp-23-queue-service-smoke-auth`
- status: `QUEUE_SERVICE_SMOKE_PASSED`
- backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/mvp-23-queue-service-smoke-auth/queue-service/queue-service-smoke.sqlite3`
- host: `127.0.0.1`
- port: `40633`

## Checks
- `health_no_auth` http `200` ok `True`
- `status_requires_auth` http `401` ok `True`
- `health` http `200` ok `True`
- `enqueue_plan` http `200` ok `True`
- `status_with_items` http `200` ok `True`
- `claim` http `200` ok `True`
- `complete` http `200` ok `True`
- `recover` http `200` ok `True`
- `final_status` http `200` ok `True`
- `stop` http `200` ok `True`

## Findings
- none

## Guardrail
- The service exposes queue operations only; scheduler, worker, sandbox, reviewer, and evidence gates still decide task execution and closure.
- Default binding is loopback with sqlite queue storage.
