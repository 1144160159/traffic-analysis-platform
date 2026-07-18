BEGIN;

DELETE FROM model_workbench_items
WHERE tenant_id = 'default' AND scenario_id IN ('ui-reference-v1', 'acceptance-bootstrap-v2');

WITH fixture(category, ordinal, payload) AS (
  VALUES
    ('features', 0, '{"label":"异常登录聚合点","value":0.182}'::jsonb),
    ('features', 1, '{"label":"登录时段异常","value":0.146}'::jsonb),
    ('features', 2, '{"label":"命令执行异常","value":0.121}'::jsonb),
    ('features', 3, '{"label":"端口连接数量","value":0.093}'::jsonb),
    ('features', 4, '{"label":"敏感文件访问","value":0.071}'::jsonb),
    ('features', 5, '{"label":"数据流量大小","value":0.064}'::jsonb),
    ('rule_contributions', 0, '{"rule":"异常时段登录","score":0.31,"delta":0.31,"direction":"risk"}'::jsonb),
    ('rule_contributions', 1, '{"rule":"高频认证失败","score":0.24,"delta":0.24,"direction":"risk"}'::jsonb),
    ('rule_contributions', 2, '{"rule":"敏感命令执行","score":0.18,"delta":0.18,"direction":"risk"}'::jsonb),
    ('rule_contributions', 3, '{"rule":"可信终端基线","score":-0.09,"delta":-0.09,"direction":"protect"}'::jsonb),
    ('rule_contributions', 4, '{"rule":"历史同源行为","score":-0.06,"delta":-0.06,"direction":"protect"}'::jsonb),
    ('anomaly_causes', 0, '{"cause":"非工作时段连续认证","confidence":0.96,"evidence":"21:28-21:33 连续 7 次认证"}'::jsonb),
    ('anomaly_causes', 1, '{"cause":"登录后敏感命令链","confidence":0.91,"evidence":"whoami -> net user -> powershell"}'::jsonb),
    ('anomaly_causes', 2, '{"cause":"跨网段横向访问","confidence":0.87,"evidence":"10.12.2.45 -> 10.23.4.65"}'::jsonb),
    ('samples', 0, '{"time":"06-19 21:33","src_ip":"10.12.2.45","topic":"异常登录","score":0.96,"label":"恶意","prediction":"TP"}'::jsonb),
    ('samples', 1, '{"time":"06-19 21:28","src_ip":"10.12.3.78","topic":"提权前置","score":0.93,"label":"恶意","prediction":"TP"}'::jsonb),
    ('samples', 2, '{"time":"06-19 21:19","src_ip":"172.16.5.23","topic":"PowerShell","score":0.91,"label":"可疑","prediction":"TP"}'::jsonb),
    ('samples', 3, '{"time":"06-19 21:07","src_ip":"10.23.4.65","topic":"隧道外联","score":0.89,"label":"可疑","prediction":"FP"}'::jsonb),
    ('samples', 4, '{"time":"06-19 20:58","src_ip":"10.12.2.45","topic":"横向扫描","score":0.87,"label":"恶意","prediction":"TP"}'::jsonb),
    ('similar_samples', 0, '{"sample_id":"SMP-20260619-1842","similarity":0.94,"verdict":"恶意","summary":"异常登录后执行 PowerShell"}'::jsonb),
    ('similar_samples', 1, '{"sample_id":"SMP-20260618-0921","similarity":0.91,"verdict":"可疑","summary":"跨网段认证与主机发现"}'::jsonb),
    ('similar_samples', 2, '{"sample_id":"SMP-20260617-3381","similarity":0.88,"verdict":"恶意","summary":"凭证尝试与横向移动"}'::jsonb),
    ('similar_samples', 3, '{"sample_id":"SMP-20260616-1148","similarity":0.84,"verdict":"正常","summary":"运维窗口批量登录"}'::jsonb),
    ('datasets', 0, '{"name":"训练集","samples":8642315,"range":"2026-05-01 ~ 2026-06-18","ratio":70,"quality":99.2}'::jsonb),
    ('datasets', 1, '{"name":"验证集","samples":1852114,"range":"2026-06-10 ~ 2026-06-18","ratio":15,"quality":98.7}'::jsonb),
    ('datasets', 2, '{"name":"测试集","samples":1234876,"range":"2026-06-15 ~ 2026-06-18","ratio":10,"quality":98.4}'::jsonb),
    ('datasets', 3, '{"name":"反馈样本","samples":523645,"range":"2026-06-19 ~ 2026-07-18","ratio":5,"quality":98.1}'::jsonb),
    ('review_gates', 0, '{"name":"性能评测","status":"通过","owner":"sec_analyst","time":"06-19 22:10"}'::jsonb),
    ('review_gates', 1, '{"name":"安全评审","status":"通过","owner":"sec_manager","time":"06-19 22:35"}'::jsonb),
    ('review_gates', 2, '{"name":"合规评审","status":"待审批","owner":"compliance","time":"-"}'::jsonb),
    ('review_gates', 3, '{"name":"规范确认","status":"待审批","owner":"compliance","time":"-"}'::jsonb),
    ('metrics', 0, '{"label":"准确率","value":0.971,"delta":0.018,"tone":"ok"}'::jsonb),
    ('metrics', 1, '{"label":"召回率","value":0.925,"delta":0.011,"tone":"ok"}'::jsonb),
    ('metrics', 2, '{"label":"F1","value":0.948,"delta":0.012,"tone":"ok"}'::jsonb),
    ('metrics', 3, '{"label":"AUC","value":0.982,"delta":0.009,"tone":"ok"}'::jsonb),
    ('metrics', 4, '{"label":"误报率","value":0.82,"delta":-6.2,"tone":"risk","unit":"%"}'::jsonb),
    ('metrics', 5, '{"label":"漂移 (PSI)","value":0.12,"delta":0,"tone":"ok"}'::jsonb),
    ('metrics', 6, '{"label":"置信区间 (F1)","value":"[0.942,0.953]","delta":0,"tone":"info"}'::jsonb),
    ('distribution', 0, '{"label":"正常","value":78.3,"tone":"ok"}'::jsonb),
    ('distribution', 1, '{"label":"可疑","value":11.5,"tone":"info"}'::jsonb),
    ('distribution', 2, '{"label":"恶意","value":6.8,"tone":"risk"}'::jsonb),
    ('distribution', 3, '{"label":"未知","value":3.4,"tone":"warn"}'::jsonb)
)
,
model_context AS (
  SELECT
    m.*,
    row_number() OVER (ORDER BY m.model_id) - 1 AS variant,
    latest.model_version AS observed_version,
    latest.metrics AS observed_metrics,
    COALESCE(
      (latest.metrics->>'f1_score')::numeric,
      (latest.metrics->>'f1')::numeric
    ) AS observed_f1,
    COALESCE(latest.artifact_uri, m.metadata->>'artifact_uri', '') AS lineage_uri
  FROM models m
  LEFT JOIN LATERAL (
    SELECT v.model_version, v.metrics, v.artifact_uri
    FROM model_versions v
    WHERE v.model_id = m.model_id
    ORDER BY v.created_at DESC
    LIMIT 1
  ) latest ON true
  WHERE m.tenant_id = 'default'
),
model_payload AS (
  SELECT
    m.*,
    fixture.category,
    fixture.ordinal,
    CASE
      WHEN fixture.category = 'features' THEN jsonb_set(
        fixture.payload,
        '{value}',
        to_jsonb(round((fixture.payload->>'value')::numeric * (1 + m.variant * 0.015), 3))
      )
      WHEN fixture.category = 'rule_contributions' THEN jsonb_set(
        jsonb_set(fixture.payload, '{score}', to_jsonb(round((fixture.payload->>'score')::numeric * (1 + m.variant * 0.01), 3))),
        '{delta}', to_jsonb(round((fixture.payload->>'delta')::numeric * (1 + m.variant * 0.01), 3))
      )
      WHEN fixture.category = 'samples' THEN jsonb_set(
        fixture.payload,
        '{score}',
        to_jsonb(round(greatest(0.01, (fixture.payload->>'score')::numeric - m.variant * 0.01), 2))
      )
      WHEN fixture.category = 'datasets' THEN jsonb_set(
        fixture.payload,
        '{samples}',
        to_jsonb(((fixture.payload->>'samples')::bigint + m.variant * 1009)::bigint)
      )
      WHEN fixture.category = 'metrics' AND fixture.payload->>'label' = 'F1' AND m.observed_f1 IS NOT NULL THEN
        jsonb_set(fixture.payload, '{value}', to_jsonb(round(m.observed_f1, 3)))
      WHEN fixture.category = 'review_gates' AND fixture.ordinal = 0 THEN
        jsonb_set(fixture.payload, '{status}', to_jsonb(CASE WHEN m.observed_f1 >= 0.94 THEN '通过' ELSE '待审核' END))
      ELSE fixture.payload
    END || jsonb_build_object(
      'model_name', m.name,
      'model_type', m.model_type,
      'source', 'acceptance-bootstrap',
      'variant', m.variant,
      'observed_version', COALESCE(m.observed_version, ''),
      'observed_f1', m.observed_f1,
      'lineage_uri', m.lineage_uri
    ) AS payload
  FROM model_context m
  CROSS JOIN fixture
)
INSERT INTO model_workbench_items (
  item_id, tenant_id, model_id, category, ordinal, payload, scenario_id, occurred_at
)
SELECT
  'model-bootstrap-v2-' || m.model_id::text || '-' || m.category || '-' || m.ordinal,
  m.tenant_id,
  m.model_id,
  m.category,
  m.ordinal,
  m.payload,
  'acceptance-bootstrap-v2',
  now() - make_interval(mins => m.ordinal)
FROM model_payload m
ON CONFLICT (tenant_id, model_id, category, ordinal, scenario_id)
DO UPDATE SET payload = EXCLUDED.payload, occurred_at = EXCLUDED.occurred_at;

COMMIT;
