-- Deterministic notification-governance fixture for the /notifications business and visual gate.
-- Every row is explicitly tagged as acceptance data and can be removed by its fixed id/prefix.

INSERT INTO alert_notification_settings (tenant_id, settings, updated_at)
VALUES (
  'default',
  '{"enabled":true,"min_severity":"high","rate_limit_per_min":30,"secret_ref":"traffic-analysis/notification-secret","channels":{"email":true,"webhook":true,"wechat":true,"dingtalk":true,"slack":true,"feishu":true}}'::jsonb,
  now()
)
ON CONFLICT (tenant_id) DO UPDATE SET settings=EXCLUDED.settings, updated_at=EXCLUDED.updated_at;

INSERT INTO notification_escalation_policies (policy_id, tenant_id, name, stages, enabled, created_by, created_at, updated_at)
VALUES
  ('00000000-0000-0000-0000-000000042601','default','夜间升级策略','[{"after_minutes":15,"condition":"SLA 超时","target_role":"安全值班组"},{"after_minutes":30,"condition":"未确认","target_role":"安全值班组"},{"after_minutes":30,"condition":"处置失败","target_role":"安全管理组"},{"after_minutes":30,"condition":"重复告警","target_role":"运维管理组"},{"after_minutes":1440,"condition":"验收缺口","target_role":"审计管理组"}]'::jsonb,true,'acceptance-bootstrap-r426',now()-interval '30 days',now()),
  ('00000000-0000-0000-0000-000000042602','default','安全事件升级策略','[{"after_minutes":5,"condition":"严重告警","target_role":"安全值班组"},{"after_minutes":15,"condition":"未确认","target_role":"安全管理组"}]'::jsonb,true,'acceptance-bootstrap-r426',now()-interval '20 days',now())
ON CONFLICT (tenant_id, name) DO UPDATE SET stages=EXCLUDED.stages, enabled=EXCLUDED.enabled, updated_at=now();

INSERT INTO notification_rules (rule_id, tenant_id, name, conditions, channels, enabled, created_by, created_at, updated_at)
VALUES
  ('00000000-0000-0000-0000-000000042611','default','严重告警夜间值守','{"severity":"critical","alert_type":"攻击告警","asset_scope":"核心资产","campus":"主园区","window_start":"00:00","window_end":"08:00","escalation_policy":"夜间升级策略","silence_mode":"维护窗口"}'::jsonb,'["email","wechat"]'::jsonb,true,NULL,now()-interval '12 days',now()),
  ('00000000-0000-0000-0000-000000042612','default','高危数据质量告警','{"severity":"high","alert_type":"数据质量","asset_scope":"Kafka / Flink","campus":"主园区","window_start":"08:00","window_end":"20:00","escalation_policy":"安全事件升级策略","silence_mode":"专题免打扰"}'::jsonb,'["email","slack"]'::jsonb,true,NULL,now()-interval '11 days',now()),
  ('00000000-0000-0000-0000-000000042613','default','异常登录即时通知','{"severity":"medium","alert_type":"异常登录","asset_scope":"终端设备","campus":"分园区 A","window_start":"00:00","window_end":"23:59","escalation_policy":"安全事件升级策略","silence_mode":"无"}'::jsonb,'["feishu","dingtalk"]'::jsonb,true,NULL,now()-interval '10 days',now()),
  ('00000000-0000-0000-0000-000000042614','default','外传专题告警','{"severity":"high","alert_type":"数据泄露","asset_scope":"财务系统","campus":"主园区","window_start":"00:00","window_end":"23:59","escalation_policy":"夜间升级策略","silence_mode":"低优先级静默"}'::jsonb,'["webhook","wechat"]'::jsonb,true,NULL,now()-interval '9 days',now()),
  ('00000000-0000-0000-0000-000000042615','default','取证任务失败','{"severity":"high","alert_type":"任务失败","asset_scope":"平台服务","campus":"全园区","window_start":"00:00","window_end":"23:59","escalation_policy":"安全事件升级策略","silence_mode":"无"}'::jsonb,'["feishu","email"]'::jsonb,true,NULL,now()-interval '8 days',now()),
  ('00000000-0000-0000-0000-000000042616','default','低优先级办公网通知','{"severity":"low","alert_type":"扫描告警","asset_scope":"办公终端","campus":"分园区 B","window_start":"09:00","window_end":"18:00","escalation_policy":"","silence_mode":"低优先级静默"}'::jsonb,'["email"]'::jsonb,false,NULL,now()-interval '7 days',now())
ON CONFLICT (tenant_id, name) DO UPDATE SET conditions=EXCLUDED.conditions, channels=EXCLUDED.channels, enabled=EXCLUDED.enabled, updated_at=now();

INSERT INTO notification_rules (rule_id, tenant_id, name, conditions, channels, enabled, created_by, created_at, updated_at)
SELECT
  (substr(md5('notification-rule-r426-' || series),1,8) || '-' || substr(md5('notification-rule-r426-' || series),9,4) || '-' || substr(md5('notification-rule-r426-' || series),13,4) || '-' || substr(md5('notification-rule-r426-' || series),17,4) || '-' || substr(md5('notification-rule-r426-' || series),21,12))::uuid,
  'default',
  '验收订阅策略 ' || lpad(series::text, 2, '0'),
  jsonb_build_object(
    'severity', (ARRAY['critical','high','medium','low'])[((series - 1) % 4) + 1],
    'alert_type', (ARRAY['攻击告警','数据泄露','异常登录','任务失败'])[((series - 1) % 4) + 1],
    'asset_scope', (ARRAY['核心资产','财务系统','终端设备','平台服务'])[((series - 1) % 4) + 1],
    'campus', (ARRAY['主园区','分园区 A','分园区 B','全园区'])[((series - 1) % 4) + 1],
    'window_start', CASE WHEN series % 2 = 0 THEN '08:00' ELSE '00:00' END,
    'window_end', CASE WHEN series % 2 = 0 THEN '20:00' ELSE '23:59' END,
    'escalation_policy', CASE WHEN series % 2 = 0 THEN '安全事件升级策略' ELSE '夜间升级策略' END,
    'silence_mode', CASE WHEN series % 3 = 0 THEN '低优先级静默' ELSE '无' END
  ),
  CASE WHEN series % 3 = 0 THEN '["webhook","slack"]'::jsonb ELSE '["email","wechat"]'::jsonb END,
  series % 7 <> 0,
  NULL,
  now() - make_interval(days => series),
  now()
FROM generate_series(7, 28) AS series
ON CONFLICT (tenant_id, name) DO UPDATE SET conditions=EXCLUDED.conditions, channels=EXCLUDED.channels, enabled=EXCLUDED.enabled, updated_at=now();

INSERT INTO notification_templates (template_id, tenant_id, template_type, name, version, subject, body, variable_schema, validation_status, enabled, created_by, created_at, updated_at)
VALUES
  ('00000000-0000-0000-0000-000000042621','default','告警模板','入侵告警通知模板',3,'[{{severity}}] {{title}}','告警 {{alert_id}}：{{source_ip}} -> {{dest_ip}}','{"required":["severity","title","alert_id","source_ip","dest_ip"]}'::jsonb,'passed',true,'acceptance-bootstrap-r426',now()-interval '20 days',now()),
  ('00000000-0000-0000-0000-000000042622','default','取证模板','取证任务通知模板',2,'取证任务 {{job_id}}','任务状态：{{status}}','{"required":["job_id","status"]}'::jsonb,'passed',true,'acceptance-bootstrap-r426',now()-interval '19 days',now()),
  ('00000000-0000-0000-0000-000000042623','default','数据质量模板','数据质量日报模板',2,'数据质量日报 {{date}}','合格率：{{pass_rate}}','{"required":["date","pass_rate"],"warnings":1}'::jsonb,'warning',true,'acceptance-bootstrap-r426',now()-interval '18 days',now()),
  ('00000000-0000-0000-0000-000000042624','default','合规模板','合规缺口报告模板',2,'合规缺口 {{report_id}}','待整改项：{{gap_count}}','{"required":["report_id","gap_count"]}'::jsonb,'passed',true,'acceptance-bootstrap-r426',now()-interval '17 days',now())
ON CONFLICT (tenant_id, name) DO UPDATE SET template_type=EXCLUDED.template_type, version=EXCLUDED.version, subject=EXCLUDED.subject, body=EXCLUDED.body, variable_schema=EXCLUDED.variable_schema, validation_status=EXCLUDED.validation_status, enabled=EXCLUDED.enabled, updated_at=now();

DELETE FROM notification_history WHERE tenant_id='default' AND alert_id LIKE 'notif-fixture-r426-%';
INSERT INTO notification_history (tenant_id, rule_id, alert_id, target_name, channel, alert_type, status, error_message, retry_count, trace_id, sent_at, created_at)
VALUES
  ('default','00000000-0000-0000-0000-000000042611','notif-fixture-r426-01','安全值班组','email','攻击告警','sent',NULL,0,'tr-notif-r426-01',now()-interval '5 minutes',now()-interval '5 minutes'),
  ('default','00000000-0000-0000-0000-000000042615','notif-fixture-r426-02','运维管理组','feishu','任务失败','failed','渠道限流',3,'tr-notif-r426-02',NULL,now()-interval '8 minutes'),
  ('default','00000000-0000-0000-0000-000000042612','notif-fixture-r426-03','数据平台组','webhook','数据质量','sent',NULL,0,'tr-notif-r426-03',now()-interval '12 minutes',now()-interval '12 minutes'),
  ('default','00000000-0000-0000-0000-000000042615','notif-fixture-r426-04','审计管理员','slack','验收缺口','failed','凭据失效',2,'tr-notif-r426-04',NULL,now()-interval '16 minutes'),
  ('default','00000000-0000-0000-0000-000000042613','notif-fixture-r426-05','安全值班组','wechat','系统异常','sent',NULL,0,'tr-notif-r426-05',now()-interval '20 minutes',now()-interval '20 minutes');

INSERT INTO notification_silence_rules (rule_id,tenant_id,name,scope,starts_at,ends_at,affected_targets,policy,reason,enabled,created_by,created_at,updated_at)
VALUES
  ('notif-silence-r426-01','default','核心交换机维护','主园区',now()+interval '1 day',now()+interval '1 day 4 hours','["交换机","网络设备"]'::jsonb,'夜间升级策略','计划维护',true,'acceptance-bootstrap-r426',now(),now()),
  ('notif-silence-r426-02','default','安全平台升级','主园区',now()+interval '3 days',now()+interval '3 days 4 hours','["平台服务","探针"]'::jsonb,'全部策略','平台升级',true,'acceptance-bootstrap-r426',now(),now()),
  ('notif-silence-r426-03','default','分园区维护','分园区 A',now()+interval '5 days',now()+interval '5 days 4 hours','["全部资产"]'::jsonb,'非紧急策略','园区维护',true,'acceptance-bootstrap-r426',now(),now())
ON CONFLICT (rule_id) DO UPDATE SET name=EXCLUDED.name, scope=EXCLUDED.scope, starts_at=EXCLUDED.starts_at, ends_at=EXCLUDED.ends_at, affected_targets=EXCLUDED.affected_targets, policy=EXCLUDED.policy, reason=EXCLUDED.reason, enabled=EXCLUDED.enabled, updated_at=now();
