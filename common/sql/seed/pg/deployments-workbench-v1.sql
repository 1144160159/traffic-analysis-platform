-- Idempotent full-stack fixture for /deployments normal-mode acceptance.
-- scenario_id: ui-deployments-v1
BEGIN;

DELETE FROM deployment_workbench_items
WHERE tenant_id = 'default' AND scenario_id = 'ui-deployments-v1';

DELETE FROM deployment_history
WHERE deployment_id IN (
  SELECT deployment_id
  FROM deployments
  WHERE tenant_id = 'default' AND scope->>'scenario_id' = 'ui-deployments-v1'
);

DELETE FROM deployments
WHERE tenant_id = 'default' AND scope->>'scenario_id' = 'ui-deployments-v1';

WITH fixture AS (
  SELECT
    series,
    (
      substr(md5('ui-deployments-v1-' || series::text), 1, 8) || '-' ||
      substr(md5('ui-deployments-v1-' || series::text), 9, 4) || '-' ||
      substr(md5('ui-deployments-v1-' || series::text), 13, 4) || '-' ||
      substr(md5('ui-deployments-v1-' || series::text), 17, 4) || '-' ||
      substr(md5('ui-deployments-v1-' || series::text), 21, 12)
    )::uuid AS deployment_id
  FROM generate_series(1, 48) AS series
), dependencies AS (
  SELECT
    (SELECT user_id FROM users WHERE tenant_id = 'default' ORDER BY created_at LIMIT 1) AS created_by,
    (SELECT rule_version FROM rule_versions WHERE tenant_id = 'default' AND status = 'active' ORDER BY created_at DESC LIMIT 1) AS rule_version
)
INSERT INTO deployments (
  deployment_id, tenant_id, name, description, rule_version, scope, status,
  created_by, created_at, updated_at, gray_started_at, gray_expired_at,
  activated_at, rolled_back_at, rollback_reason, metadata
)
SELECT
  fixture.deployment_id,
  'default',
  CASE fixture.series
    WHEN 1 THEN '规则包-APT检测增强'
    WHEN 2 THEN '异常流量检测模型'
    WHEN 3 THEN '采集策略-办公区'
    WHEN 4 THEN 'Flink作业-流量聚合'
    WHEN 5 THEN '配置模板-告警阈值'
    WHEN 6 THEN '规则包-僵木马C2检测'
    WHEN 7 THEN '规则-UEBA行为分析'
    WHEN 8 THEN '采集策略-数据中心'
    ELSE CASE fixture.series % 5
      WHEN 0 THEN '规则包-横向移动检测'
      WHEN 1 THEN '模型-加密流量识别'
      WHEN 2 THEN '采集策略-边界区'
      WHEN 3 THEN 'Flink作业-会话聚合'
      ELSE '配置模板-证据保留'
    END || '-' || lpad(fixture.series::text, 2, '0')
  END,
  '部署管理 UI/API/DB 联调数据 ' || fixture.series,
  dependencies.rule_version,
  jsonb_build_object(
    'scenario_id', 'ui-deployments-v1',
    'version', 'v2.' || ((fixture.series % 5) + 1) || '.' || (fixture.series % 8),
    'environment', CASE WHEN fixture.series % 5 = 0 THEN 'stage' WHEN fixture.series % 7 = 0 THEN 'canary' ELSE 'prod' END,
    'owner', (ARRAY['安全运营组','算法平台组','网络平台组','Flink 平台组'])[1 + (fixture.series % 4)],
    'tenant', '租户A',
    'campus', CASE WHEN fixture.series % 3 = 0 THEN '华南园区' ELSE '华东园区' END,
    'probe_group', CASE WHEN fixture.series % 2 = 0 THEN '办公区探针组 (12)' ELSE '核心区探针组 (8)' END,
    'asset_group', CASE WHEN fixture.series % 2 = 0 THEN '核心业务资产组' ELSE '办公终端资产组' END,
    'impact', CASE WHEN fixture.series % 4 = 0 THEN '全部探针' ELSE (8 + fixture.series % 16)::text || ' 个探针' END,
    'percentage', CASE WHEN fixture.series BETWEEN 19 AND 25 THEN 20 ELSE 100 END
  ),
  CASE
    WHEN fixture.series <= 18 THEN 'planned'
    WHEN fixture.series <= 25 THEN 'gray'
    WHEN fixture.series <= 27 THEN 'failed'
    WHEN fixture.series <= 39 THEN 'active'
    WHEN fixture.series <= 44 THEN 'rolled_back'
    WHEN fixture.series <= 46 THEN 'paused'
    ELSE 'superseded'
  END,
  dependencies.created_by,
  '2026-06-20 03:45:00+08'::timestamptz - fixture.series * interval '35 minutes',
  '2026-06-20 03:45:00+08'::timestamptz - fixture.series * interval '12 minutes',
  CASE WHEN fixture.series BETWEEN 19 AND 25 THEN '2026-06-20 03:00:00+08'::timestamptz - fixture.series * interval '5 minutes' END,
  CASE WHEN fixture.series BETWEEN 19 AND 25 THEN '2026-06-21 03:00:00+08'::timestamptz - fixture.series * interval '5 minutes' END,
  CASE WHEN fixture.series BETWEEN 28 AND 39 THEN '2026-06-20 02:30:00+08'::timestamptz - fixture.series * interval '8 minutes' END,
  CASE WHEN fixture.series BETWEEN 40 AND 44 THEN '2026-06-20 02:00:00+08'::timestamptz - fixture.series * interval '8 minutes' END,
  CASE WHEN fixture.series BETWEEN 40 AND 44 THEN '误报率升高，按审批单回滚' ELSE '' END,
  jsonb_build_object(
    'scenario_id', 'ui-deployments-v1',
    'activation_latency_seconds', 42 + (fixture.series % 27),
    'avg_activation_latency_seconds', 58,
    'release_success_rate', 98.2,
    'rollback_version_count', 23,
    'risk_level', CASE WHEN fixture.series IN (5, 21, 26, 27) THEN 'high' WHEN fixture.series % 4 = 0 THEN 'medium' ELSE 'low' END
  )
FROM fixture
CROSS JOIN dependencies
WHERE dependencies.created_by IS NOT NULL AND dependencies.rule_version IS NOT NULL;

INSERT INTO deployment_history (deployment_id, action, operator_id, created_at, detail)
SELECT
  deployment_id,
  CASE status
    WHEN 'gray' THEN 'gray_started'
    WHEN 'active' THEN 'activated'
    WHEN 'rolled_back' THEN 'rolled_back'
    WHEN 'paused' THEN 'paused'
    ELSE 'created'
  END,
  coalesce(created_by::text, 'system'),
  updated_at,
  jsonb_build_object('status', status, 'scenario_id', 'ui-deployments-v1')
FROM deployments
WHERE tenant_id = 'default' AND scope->>'scenario_id' = 'ui-deployments-v1';

INSERT INTO deployment_workbench_items
  (item_id, tenant_id, deployment_id, category, ordinal, payload, scenario_id, occurred_at)
VALUES
  ('ui-deployments-v1-health-01','default','*','health',1,'{"label":"Flink Checkpoint 成功率","value":"98.8%","tone":"ok","values":[98.2,98.5,98.4,98.7,98.6,98.9,98.8]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-health-02','default','*','health',2,'{"label":"Kafka 消费延迟 (P95)","value":"320 ms","tone":"warn","values":[286,301,294,315,309,328,320]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-health-03','default','*','health',3,'{"label":"告警数量变化","value":"-18.5%","tone":"ok","values":[96,91,88,86,83,80,78]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-health-04','default','*','health',4,'{"label":"误报率变化","value":"+2.1%","tone":"risk","values":[1.1,1.2,1.3,1.5,1.6,1.9,2.1]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-health-05','default','*','health',5,'{"label":"端到端延迟 (P95)","value":"1.28 s","tone":"ok","values":[1.08,1.12,1.16,1.14,1.2,1.25,1.28]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-health-06','default','*','health',6,'{"label":"采集丢包率","value":"0.03%","tone":"ok","values":[0.02,0.03,0.02,0.02,0.03,0.03,0.03]}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-01','default','*','evidence',1,'{"label":"manifest","status":"已通过","checksum":"a1b2c3d4...e5f6"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-02','default','*','evidence',2,'{"label":"镜像","status":"已通过","checksum":"sha256:7e8d...9e0f"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-03','default','*','evidence',3,'{"label":"DDL","status":"已通过","checksum":"ddl_20260620_001"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-04','default','*','evidence',4,'{"label":"topic","status":"已通过","checksum":"topic_20260620_001"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-05','default','*','evidence',5,'{"label":"规则版本","status":"已通过","checksum":"rules_v2.3.1"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-evidence-06','default','*','evidence',6,'{"label":"模型版本","status":"已通过","checksum":"model_v1.8.0"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-change-01','default','*','change_summary',1,'{"label":"规则变更数","from":"32 条","to":"57 条","delta":"+25"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-change-02','default','*','change_summary',2,'{"label":"模型版本","from":"v1.7.3","to":"v1.8.0","delta":"升级"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-change-03','default','*','change_summary',3,'{"label":"DDL 变更","from":"2 处","to":"3 处","delta":"+1"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-change-04','default','*','change_summary',4,'{"label":"Topic 变更","from":"1 个","to":"2 个","delta":"+1"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-change-05','default','*','change_summary',5,'{"label":"风险等级","from":"低风险","to":"中风险","delta":"升高"}','ui-deployments-v1','2026-06-20 03:45:00+08'),
  ('ui-deployments-v1-rollback-01','default','*','rollback_versions',1,'{"version":"v2.2.7","released_at":"2026-06-19 16:10","scope":"租户A / 全量","owner":"安全运营组"}','ui-deployments-v1','2026-06-19 16:10:00+08'),
  ('ui-deployments-v1-rollback-02','default','*','rollback_versions',2,'{"version":"v2.2.3","released_at":"2026-06-18 11:05","scope":"租户A / 全量","owner":"安全运营组"}','ui-deployments-v1','2026-06-18 11:05:00+08'),
  ('ui-deployments-v1-rollback-03','default','*','rollback_versions',3,'{"version":"v2.1.9","released_at":"2026-06-17 09:40","scope":"租户A / 全量","owner":"安全运营组"}','ui-deployments-v1','2026-06-17 09:40:00+08');

COMMIT;
