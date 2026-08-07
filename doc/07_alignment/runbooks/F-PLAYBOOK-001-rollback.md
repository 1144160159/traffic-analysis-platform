# F-PLAYBOOK-001 rollback

## Scope

This runbook covers the additive, default-off `PLAYBOOK_EXECUTION_V2_ENABLED` path, the leased HTTP provider adapter, independent execution approval, cancellation, immutable per-step receipts and independently approved compensation. It does not authorize deleting or rewriting accepted execution, approval, control, receipt, audit or outbox rows, nor provider-side effects.

## Immediate stop conditions

- a cross-tenant execution is readable or controllable;
- an unapproved, disabled or stale definition revision is leased;
- the requester approves its own execution or compensation;
- browser or API state reports success without a durable receipt for every approved action;
- provider HTTP success is treated as an effect without a stable provider receipt identity;
- a failed receipt contains an external effect instead of being classified `partial`;
- workflow revision, audit, outbox or provider receipt identity diverges;
- a lease expires while the provider effect is ambiguous;
- run-budget or cooldown admission is exceeded under concurrency;
- queue age, retries, error rate, PostgreSQL locks or provider latency exceed the approved canary budget.

## Rollback procedure

1. Stop canary expansion and set `PLAYBOOK_EXECUTION_V2_ENABLED=false`. If only the provider is unsafe, clear `PLAYBOOK_EXECUTION_PROVIDER_URL` first and roll alert-service so no new lease is claimed. Approved rows must remain `approved_awaiting_executor` or `compensation_queued` with an honest executor state.
2. Allow in-flight PostgreSQL transactions to finish. Do not manually mark a leased execution terminal and do not issue an untracked inverse provider call.
3. Reconcile the exact tenant and execution IDs using the queries below. For an ambiguous effect, query the provider by the stable `execution_id:execute` or `execution_id:compensate` idempotency identity before any retry.
4. Roll alert-service and Web UI back to the previous content-addressed candidate if disabling the feature is insufficient. Keep the additive Schema and all immutable rows.
5. Record flags, image hashes, in-flight leases, provider results, reconciliation output, residual outbox and operator approvals in an immutable run directory.

## PostgreSQL reconciliation

```sql
SELECT tenant_id,execution_id,playbook_name,playbook_version,status,
       approval_status,executor_status,workflow_revision,attempts,
       locked_by,locked_until,requested_by,approved_by,trace_id,
       created_at,updated_at,completed_at
FROM alert_playbook_executions
WHERE mode='live'
  AND status IN ('approved_awaiting_executor','running','partial',
                 'compensation_queued','compensating','failed','compensation_failed')
ORDER BY tenant_id,created_at,execution_id;

SELECT tenant_id,execution_id,phase,attempt,step_index,action_type,
       provider,provider_receipt_id,status,external_effect,payload_sha256,created_at
FROM alert_playbook_step_receipts
ORDER BY tenant_id,execution_id,phase,attempt,step_index;

SELECT e.tenant_id,e.execution_id,e.workflow_revision,
       count(DISTINCT a.approval_id) AS approvals,
       count(DISTINCT c.request_id) AS controls,
       count(DISTINCT r.receipt_id) AS step_receipts,
       count(DISTINCT o.event_id) AS outbox_events,
       count(DISTINCT l.event_id) AS audit_events
FROM alert_playbook_executions e
LEFT JOIN alert_playbook_execution_approvals a
  ON a.tenant_id=e.tenant_id AND a.execution_id=e.execution_id
LEFT JOIN alert_playbook_execution_controls c
  ON c.tenant_id=e.tenant_id AND c.execution_id=e.execution_id
LEFT JOIN alert_playbook_step_receipts r
  ON r.tenant_id=e.tenant_id AND r.execution_id=e.execution_id
LEFT JOIN alert_playbook_execution_outbox o
  ON o.tenant_id=e.tenant_id AND o.execution_id=e.execution_id
LEFT JOIN audit_logs l
  ON l.tenant_id=e.tenant_id AND l.object_id=e.execution_id
WHERE e.mode='live'
GROUP BY e.tenant_id,e.execution_id,e.workflow_revision
ORDER BY e.tenant_id,e.execution_id;
```

Recompute each step receipt SHA-256 from its stored payload and match the provider, provider receipt ID, phase and external-effect flag. Match every outbox payload's `event_id`, tenant, execution ID, event type, schema version, aggregate version, partition key and trace ID to the execution transition. A PostgreSQL terminal state without the required receipt is a stop condition; do not repair it by direct SQL.

## Ambiguous provider effect

Do not retry until the provider can return the immutable result for the same phase idempotency identity. If the provider confirms an effect but PostgreSQL has no receipt, retain the provider evidence, keep the worker disabled and escalate for an audited repair command. Never insert the missing receipt or change the workflow state manually. Compensation requires an execution receipt with `external_effect=true` and an approver different from the original requester.

## Re-enable criteria

- the triggering defect has a regression test;
- migration application and replay pass through the common SQL, Docker merged SQL, K8s ConfigMap and versioned migration entrypoints;
- tenant isolation, stable request replay, changed-payload conflict, stale revision, self-approval, self-compensation, run-budget and cooldown tests pass against sentinel-protected PostgreSQL;
- provider timeout, retry exhaustion, partial effect, durable receipt validation and compensation pass against an approved provider sandbox;
- outbox publication is acknowledged before `published=true`, and downstream reconciliation matches `event_id` plus `aggregate_version`;
- Windows Chrome shows accepted, approval, executing, terminal receipt and compensation states from the candidate bundle with mock disabled;
- independent QA, SRE and the release owner approve a new immutable candidate.
