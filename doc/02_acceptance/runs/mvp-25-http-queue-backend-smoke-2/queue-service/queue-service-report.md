# Codex Loop Queue Service

- run_id: `mvp-25-http-queue-backend-smoke-2`
- status: `QUEUE_SERVICE_SMOKE_PASSED`
- backend: `sqlite`
- queue_path: `doc/02_acceptance/runs/mvp-25-http-queue-backend-smoke-2/queue-service/queue-service-smoke.sqlite3`
- host: `127.0.0.1`
- port: `46781`

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
- `http_backend_enqueue_plan` http `backend` ok `True`
- `http_backend_claim` http `backend` ok `True`
- `http_backend_complete` http `backend` ok `True`
- `http_backend_recover` http `backend` ok `True`
- `stop` http `200` ok `True`

## Findings
- none

## Guardrail
- The service exposes queue operations only; scheduler, worker, sandbox, reviewer, and evidence gates still decide task execution and closure.
- Default binding is loopback with sqlite queue storage.
