-- Idempotent persisted action history for the alert-detail acceptance alert.
-- These rows exercise the same PostgreSQL table used by live response and
-- investigation actions.

INSERT INTO alert_response_actions (
  job_id, tenant_id, alert_id, action, target, reason, dry_run, status,
  detail, requested_by, created_at, updated_at
)
VALUES
  (
    'alert-detail-r802-investigation',
    'default',
    'alert-detail-accept-r802',
    '记录告警研判结论',
    'alert-detail-accept-r802',
    '数据库模拟：确认 C2 隧道通信与异常长连接',
    false,
    'recorded',
    '{"source":"alert-detail-fixture","verdict":"TP","labels":["C2通信","横向移动","可疑外联"]}'::jsonb,
    'sec_analyst',
    now() - interval '6 minutes',
    now() - interval '6 minutes'
  ),
  (
    'alert-detail-r802-isolate-host',
    'default',
    'alert-detail-accept-r802',
    '隔离主机',
    '172.16.5.10',
    '数据库模拟：等待审批后隔离受控终端',
    true,
    'pending_approval',
    '{"source":"alert-detail-fixture","impact_assets":1,"approval_required":true}'::jsonb,
    'sec_analyst',
    now() - interval '4 minutes',
    now() - interval '4 minutes'
  ),
  (
    'alert-detail-r802-block-ip',
    'default',
    'alert-detail-accept-r802',
    '阻断 IP',
    '185.22.14.9',
    '数据库模拟：阻断外部 C2 节点',
    true,
    'pending_approval',
    '{"source":"alert-detail-fixture","impact_assets":2,"approval_required":true}'::jsonb,
    'sec_analyst',
    now() - interval '3 minutes',
    now() - interval '3 minutes'
  )
ON CONFLICT (job_id) DO UPDATE SET
  action = EXCLUDED.action,
  target = EXCLUDED.target,
  reason = EXCLUDED.reason,
  dry_run = EXCLUDED.dry_run,
  status = EXCLUDED.status,
  detail = EXCLUDED.detail,
  requested_by = EXCLUDED.requested_by,
  updated_at = now();

SELECT
  'alert-detail-accept-r802' AS fixture_id,
  count(*) AS action_rows
FROM alert_response_actions
WHERE tenant_id = 'default' AND alert_id = 'alert-detail-accept-r802';
