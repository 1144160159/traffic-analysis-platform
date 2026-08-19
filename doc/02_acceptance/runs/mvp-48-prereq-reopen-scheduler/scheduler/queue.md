# Codex Loop Scheduler Queue

- run_id: `mvp-48-prereq-reopen-scheduler`
- status: `SCHEDULER_PLANNED`
- selected: `1`
- deferred: `11`

## Selected
- `CLE-P0-ROUTE-001` score `1530` attempt `1/3` paths: web/ui/src/, web/ui/e2e/, doc/02_acceptance/, doc/01_design/

## Deferred
- `CLE-P0-UIBACKUP-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-DLQ-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-P95-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-PCAP-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-SEC-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-AUTH-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-REVIEWER-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-BASELINE-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P1-FUSION-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P1-PILOT-001`: workspace path overlaps CLE-P0-ROUTE-001
- `CLE-P0-SCREEN-001`: open prerequisites: CLE-P0-ROUTE-001

## Resource Quota
- enabled: `False`
- policy: `scripts/codex_loop/policies/resource-quotas.yaml`
- usage: `{'total_weight': 0, 'lanes': {}, 'lane_groups': {}, 'modes': {}, 'data_modes': {}, 'subsystems': {}, 'selected': []}`
- quota deferred: none

## Lock
- not requested

## Persistent Queue
- not requested

## Guardrail
- The scheduler writes queue intent only; it does not run workflow.py by itself.
- One workspace lock protects code-modifying tasks from overlapping in this MVP.
