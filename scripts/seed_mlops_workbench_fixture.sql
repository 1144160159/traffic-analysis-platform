-- Idempotent PostgreSQL seed for the production-route MLOps workbench.
-- The rows are explicitly marked as seeded operational fixtures and are read
-- through /api/v1/models/{id}/workbench. No browser-only fallback is involved.

BEGIN;

WITH selected_model AS (
  SELECT model_id, tenant_id
  FROM models
  WHERE tenant_id = 'default' AND name = 'behavior-classifier'
  ORDER BY updated_at DESC
  LIMIT 1
), task_rows AS (
  SELECT
    'mlops-task-' || lpad(n::text, 2, '0') AS item_id,
    selected_model.tenant_id,
    selected_model.model_id,
    'mlops_tasks'::text AS category,
    n AS ordinal,
    jsonb_build_object(
      'task_id', 'TR-20260719-' || lpad(n::text, 3, '0'),
      'stage', (ARRAY['训练任务','评估门禁','标注管理','注册模型','灰度发布','效果回流'])[((n - 1) % 6) + 1],
      'dataset_version', format('ds_v1.%s.%s', 6 - ((n - 1) / 12), ((n - 1) % 12) + 1),
      'algorithm', (ARRAY['xgb_v2.4','lightgbm_v1.3','manual_review','xgb_v2.4','isolation_forest','lof_v1.2'])[((n - 1) % 6) + 1],
      'feature_version', format('feat_v1.8.%s', 7 - ((n - 1) / 12)),
      'resource', (ARRAY['GPU 70% / CPU 42%','GPU 35% / CPU 24%','CPU 90% / MEM 78%','CPU 5% / MEM 8%','CPU 0% / MEM 0%','CPU 12% / MEM 18%'])[((n - 1) % 6) + 1],
      'status', CASE WHEN n % 11 = 0 THEN '失败' WHEN n % 4 = 0 THEN '排队中' WHEN n % 3 = 0 THEN '已完成' ELSE '运行中' END,
      'trace_id', 'trace-mlops-' || lpad(n::text, 3, '0'),
      'source', 'postgresql-seeded'
    ) AS payload,
    'mlops-reference-20260719-r1'::text AS scenario_id,
    now() - make_interval(mins => n * 7) AS occurred_at
  FROM selected_model CROSS JOIN generate_series(1, 36) AS series(n)
), feedback_rows AS (
  SELECT
    'mlops-feedback-' || lpad(n::text, 2, '0') AS item_id,
    selected_model.tenant_id,
    selected_model.model_id,
    'mlops_feedback_samples'::text AS category,
    n AS ordinal,
    jsonb_build_object(
      'feedback_id', 'FB-' || lpad((1000 - n * 7)::text, 6, '0'),
      'label', CASE WHEN n IN (3, 5, 10) THEN 'TP' ELSE 'FP' END,
      'reason', (ARRAY['端口扫风暴','CDN 误报','C2 连接','健康检查','NAT 变更','异常域名','P2P 通讯','系统更新','跨网段扫描','广告流量'])[n],
      'alert_id', 'AL-20260719-' || lpad((120 + n)::text, 5, '0'),
      'whitelist_suggestion', (ARRAY['建议忽略','高误报','无需','建议忽略','高置信','无需','误报忽略','无需','待复核','建议忽略'])[n],
      'quality', (ARRAY['★★★★☆','★★★☆☆','★★★★★','★★★★☆','★★★☆☆','★★★★★','★★★☆☆','★★★☆☆','★★★★☆','★★★☆☆'])[n],
      'received_at', to_char(now() - make_interval(mins => n * 6), 'MM-DD HH24:MI'),
      'source', 'postgresql-seeded'
    ) AS payload,
    'mlops-reference-20260719-r1'::text AS scenario_id,
    now() - make_interval(mins => n * 6) AS occurred_at
  FROM selected_model CROSS JOIN generate_series(1, 10) AS series(n)
), gate_rows(item_id, tenant_id, model_id, category, ordinal, payload, scenario_id, occurred_at) AS (
  SELECT 'mlops-gate-accuracy', tenant_id, model_id, 'mlops_evaluation_gates', 1, '{"label":"准确率","value":0.958,"threshold":0.920,"operator":">=","status":"通过","trend":[0.91,0.93,0.92,0.95,0.96,0.95,0.958]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-gate-recall', tenant_id, model_id, 'mlops_evaluation_gates', 2, '{"label":"召回率","value":0.932,"threshold":0.900,"operator":">=","status":"通过","trend":[0.89,0.90,0.91,0.92,0.93,0.92,0.932]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-gate-f1', tenant_id, model_id, 'mlops_evaluation_gates', 3, '{"label":"F1","value":0.944,"threshold":0.920,"operator":">=","status":"通过","trend":[0.90,0.92,0.93,0.94,0.93,0.94,0.944]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-gate-fp', tenant_id, model_id, 'mlops_evaluation_gates', 4, '{"label":"误报率","value":0.021,"threshold":0.050,"operator":"<=","status":"通过","trend":[0.048,0.041,0.036,0.031,0.028,0.024,0.021]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-gate-psi', tenant_id, model_id, 'mlops_evaluation_gates', 5, '{"label":"漂移(PSI)","value":0.18,"threshold":0.25,"operator":"<=","status":"通过","trend":[0.15,0.17,0.16,0.19,0.18,0.17,0.18]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-gate-regression', tenant_id, model_id, 'mlops_evaluation_gates', 6, '{"label":"回归集通过率","value":0.923,"threshold":0.900,"operator":">=","status":"通过","trend":[0.88,0.89,0.91,0.90,0.92,0.91,0.923]}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
), pipeline_rows AS (
  SELECT
    'mlops-stage-' || lpad(n::text, 2, '0') AS item_id,
    selected_model.tenant_id,
    selected_model.model_id,
    'mlops_pipeline_stages'::text AS category,
    n AS ordinal,
    jsonb_build_object(
      'title', (ARRAY['反馈样本池','标注管理','特征构建','训练任务','评估门禁','模型注册','灰度发布','效果回流'])[n],
      'value', (ARRAY['1,238','568','v1.8.7','6','3','2','1','542'])[n],
      'caption', (ARRAY['待处理样本','待标注样本','特征版本','运行中任务','评估中任务','待注册模型','灰度中','未反馈样本'])[n],
      'queue', (ARRAY['feedback_q','label_q','feature_q','train_q','eval_q','register_q','release_q','feedback_q'])[n],
      'duration', (ARRAY['2m','18m','6m','1h 32m','23m','6m','45m','持续回流'])[n],
      'tone', (ARRAY['ok','ok','ok','info','info','ok','warn','ok'])[n],
      'source', 'postgresql-seeded'
    ) AS payload,
    'mlops-reference-20260719-r1'::text AS scenario_id,
    now() AS occurred_at
  FROM selected_model CROSS JOIN generate_series(1, 8) AS series(n)
), release_rows(item_id, tenant_id, model_id, category, ordinal, payload, scenario_id, occurred_at) AS (
  SELECT 'mlops-release-1', tenant_id, model_id, 'mlops_releases', 1, '{"model_name":"abnorm_pkt_v3","version":"1.2.3","signature":"已签名","candidate":"是","gray_policy":"10% 30m","status":"灰度中","artifact_uri":"s3://models/abnorm_pkt_v3/1.2.3/model.json"}'::jsonb, 'mlops-reference-20260719-r1', now() FROM selected_model
  UNION ALL SELECT 'mlops-release-2', tenant_id, model_id, 'mlops_releases', 2, '{"model_name":"abnorm_pkt_v3","version":"1.2.2","signature":"已签名","candidate":"否","gray_policy":"-","status":"待发布","artifact_uri":"s3://models/abnorm_pkt_v3/1.2.2/model.json"}'::jsonb, 'mlops-reference-20260719-r1', now() - interval '1 day' FROM selected_model
  UNION ALL SELECT 'mlops-release-3', tenant_id, model_id, 'mlops_releases', 3, '{"model_name":"abnorm_pkt_v3","version":"1.2.1","signature":"已签名","candidate":"否","gray_policy":"-","status":"已回滚","artifact_uri":"s3://models/abnorm_pkt_v3/1.2.1/model.json"}'::jsonb, 'mlops-reference-20260719-r1', now() - interval '2 days' FROM selected_model
  UNION ALL SELECT 'mlops-release-4', tenant_id, model_id, 'mlops_releases', 4, '{"model_name":"abnorm_pkt_v3","version":"1.2.0","signature":"已签名","candidate":"否","gray_policy":"-","status":"已下线","artifact_uri":"s3://models/abnorm_pkt_v3/1.2.0/model.json"}'::jsonb, 'mlops-reference-20260719-r1', now() - interval '3 days' FROM selected_model
), feedback_daily AS (
  SELECT
    'mlops-feedback-day-' || lpad(n::text, 2, '0') AS item_id,
    selected_model.tenant_id,
    selected_model.model_id,
    'mlops_feedback_daily'::text AS category,
    n AS ordinal,
    jsonb_build_object(
      'day', to_char(current_date - (7 - n), 'MM-DD'),
      'false_positive_rate', (ARRAY[4.6,5.2,3.8,4.4,3.2,4.1,3.5])[n],
      'psi', (ARRAY[1.6,1.8,2.4,1.5,2.0,1.4,1.8])[n],
      'source', 'postgresql-seeded'
    ) AS payload,
    'mlops-reference-20260719-r1'::text AS scenario_id,
    now() - make_interval(days => 7 - n) AS occurred_at
  FROM selected_model CROSS JOIN generate_series(1, 7) AS series(n)
), summary_row AS (
  SELECT
    'mlops-summary-r1' AS item_id,
    tenant_id,
    model_id,
    'mlops_summary'::text AS category,
    1 AS ordinal,
    '{"tp":1256,"fp":218,"feedback_samples":542,"false_positive_rate":2.15,"psi":0.18,"alerts_total":24681,"effective_alerts":6214,"pool_samples":1238,"labeled_samples":568,"confusion":{"tn":4582,"fp":218,"fn":143,"tp":1257}}'::jsonb AS payload,
    'mlops-reference-20260719-r1'::text AS scenario_id,
    now() AS occurred_at
  FROM selected_model
), all_rows AS (
  SELECT * FROM task_rows
  UNION ALL SELECT * FROM feedback_rows
  UNION ALL SELECT * FROM gate_rows
  UNION ALL SELECT * FROM pipeline_rows
  UNION ALL SELECT * FROM release_rows
  UNION ALL SELECT * FROM feedback_daily
  UNION ALL SELECT * FROM summary_row
)
INSERT INTO model_workbench_items (
  item_id, tenant_id, model_id, category, ordinal, payload, scenario_id, occurred_at
)
SELECT item_id, tenant_id, model_id, category, ordinal, payload, scenario_id, occurred_at
FROM all_rows
ON CONFLICT (item_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  model_id = EXCLUDED.model_id,
  category = EXCLUDED.category,
  ordinal = EXCLUDED.ordinal,
  payload = EXCLUDED.payload,
  scenario_id = EXCLUDED.scenario_id,
  occurred_at = EXCLUDED.occurred_at;

COMMIT;
