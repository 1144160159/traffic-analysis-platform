# Codex Loop Scheduler Queue

- run_id: `mvp-10-scheduler`
- status: `SCHEDULER_PLANNED`
- selected: `1`
- deferred: `11`

## Selected
- `CLE-P0-SCREEN-001` score `1515` attempt `2/3` paths: web/ui/src/, web/ui/e2e/, go/control-plane/internal/auth/, doc/02_acceptance/

## Deferred
- `CLE-P0-UIBACKUP-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-DLQ-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-P95-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-PCAP-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-SEC-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-AUTH-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-ROUTE-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-REVIEWER-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P0-BASELINE-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P1-FUSION-001`: workspace path overlaps CLE-P0-SCREEN-001
- `CLE-P1-PILOT-001`: workspace path overlaps CLE-P0-SCREEN-001

## Lock
- not requested

## Guardrail
- The scheduler writes queue intent only; it does not run workflow.py by itself.
- One workspace lock protects code-modifying tasks from overlapping in this MVP.
