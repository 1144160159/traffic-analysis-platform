-- Deterministic r347 whitelist governance fixture.
-- It intentionally covers every visual and business state on /whitelist.
INSERT INTO whitelist (
  id, tenant_id, type, value, reason, description, status, approval_status,
  source_alert_id, feedback_id, owner_role, scope, risk_level,
  covered_alerts, covered_assets, version, created_by, approved_by,
  approved_at, disabled_at, expires_at, created_at, updated_at
)
VALUES
  ('00000000-0000-0000-0000-000000034701','default','domain','update.campus.local','业务系统自动更新触发 DNS 异常误报','只抑制 DNS 异常告警，原始流量与证据继续保留','active','approved','AL-20260619-0187','','安全运营','全网 / 办公网','medium',128,23,4,'fixture-author','fixture-reviewer',now()-interval '25 days',NULL,now()+interval '6 days',now()-interval '30 days',now()),
  ('00000000-0000-0000-0000-000000034702','default','subnet','10.12.4.0/24','研发网段维护窗口触发端口扫描误报','限定源 IP 网段与办公网范围','active','approved','AL-20260619-0451','','安全运营','全网 / 办公网','low',42,8,3,'fixture-author','fixture-reviewer',now()-interval '20 days',NULL,now()+interval '4 days',now()-interval '28 days',now()),
  ('00000000-0000-0000-0000-000000034703','default','asset','ASSET-SRV-0421','业务巡检产生固定备份流量','科研数据平台实验楼服务器，按资产组复审','pending','pending','AL-20260619-0322','','平台团队','仅本资产 / 同组资产','medium',86,23,2,'fixture-author','',NULL,NULL,now()+interval '30 days',now()-interval '3 days',now()),
  ('00000000-0000-0000-0000-000000034704','default','account','svc_backup','夜间备份账号访问数据库','限制登录源与访问目标，周期时间窗 22:00-03:00','draft','draft','AL-20260614-0059','','平台团队','备份系统 / 夜间窗口','medium',54,12,1,'fixture-author','',NULL,NULL,now()+interval '30 days',now()-interval '1 day',now()),
  ('00000000-0000-0000-0000-000000034705','default','rule','Rule-100324 / C2_Tunnel_v3','固定业务探测命中 C2 规则','限定 src_ip 与 bytes_out_p95 条件','active','approved','FP-20260619-001','','安全运营','全网 / 办公网','high',128,1,5,'fixture-author','fixture-reviewer',now()-interval '210 days',NULL,NULL,now()-interval '241 days',now()),
  ('00000000-0000-0000-0000-000000034706','default','model','UEBA 行为分析 v1.8.0','固定备份行为高分','绑定反馈样本池和验证集，阈值 0.87','active','approved','mdl-fp-20260619-003','feedback-pool-5231','数据科学','办公网 / 备份系统','medium',64,1,4,'fixture-author','fixture-reviewer',now()-interval '30 days',NULL,now()+interval '90 days',now()-interval '35 days',now()),
  ('00000000-0000-0000-0000-000000034707','default','ip','10.23.0.18','办公终端固定探测','按单 IP 抑制端口扫描误报','active','approved','AL-20260717-0108','','安全运营','办公网','low',31,1,3,'fixture-author','fixture-reviewer',now()-interval '18 days',NULL,now()+interval '2 days',now()-interval '20 days',now()),
  ('00000000-0000-0000-0000-000000034708','default','domain','downloads.campus.local','校园软件下载镜像','到期后尚未完成责任人复核','active','approved','AL-20260612-0012','','平台团队','全网','medium',38,7,6,'fixture-author','fixture-reviewer',now()-interval '40 days',NULL,now()-interval '12 days',now()-interval '60 days',now()),
  ('00000000-0000-0000-0000-000000034709','default','account','temp_admin','临时管理员账号例外','原责任人离职，必须重新指派','active','approved','AL-20260626-0031','','','管理平台','high',22,3,4,'fixture-author','fixture-reviewer',now()-interval '35 days',NULL,now()-interval '8 days',now()-interval '50 days',now()),
  ('00000000-0000-0000-0000-000000034710','default','subnet','10.23.0.0/16','核心网段例外','长期生效条目，30 天复审','active','approved','AL-20251201-0008','','安全运营','核心网络','medium',214,48,9,'fixture-author','fixture-reviewer',now()-interval '210 days',NULL,NULL,now()-interval '214 days',now()),
  ('00000000-0000-0000-0000-000000034711','default','domain','*.cdn.campus.local','教学 CDN 固定流量','长期生效，60 天复审','active','approved','AL-20250901-0011','','平台团队','教学网','low',326,16,7,'fixture-author','fixture-reviewer',now()-interval '300 days',NULL,NULL,now()-interval '326 days',now()),
  ('00000000-0000-0000-0000-000000034712','default','asset','ASSET-SRV-0812','资产部门迁移','原平台团队已变更，等待重新分配','active','approved','WORKORDER-8821','','','数据中心','medium',45,1,3,'fixture-author','fixture-reviewer',now()-interval '15 days',NULL,now()+interval '45 days',now()-interval '18 days',now()),
  ('00000000-0000-0000-0000-000000034713','default','rule','Rule-200618 / DNS_Tunnel_v2','组织变更导致责任人缺失','高风险规则例外必须指派','active','approved','FP-20260701-001','','','全网','high',97,4,5,'fixture-author','fixture-reviewer',now()-interval '25 days',NULL,now()+interval '80 days',now()-interval '30 days',now()),
  ('00000000-0000-0000-0000-000000034714','default','model','横向移动检测 v2.4.1','模型验证集误报','等待数据科学负责人审批','pending','pending','mdl-fp-20260718-014','','数据科学','服务器区','high',73,14,2,'fixture-model-author','',NULL,NULL,now()+interval '14 days',now()-interval '2 days',now()),
  ('00000000-0000-0000-0000-000000034715','default','ip','10.88.7.19','历史白名单已停用','保留审计与历史命中证据','disabled','approved','AL-20260512-0003','','安全运营','测试环境','low',9,1,8,'fixture-author','fixture-reviewer',now()-interval '60 days',now()-interval '3 days',now()+interval '20 days',now()-interval '90 days',now())
ON CONFLICT (tenant_id, type, value) DO UPDATE SET
  reason=EXCLUDED.reason,
  description=EXCLUDED.description,
  status=EXCLUDED.status,
  approval_status=EXCLUDED.approval_status,
  source_alert_id=EXCLUDED.source_alert_id,
  feedback_id=EXCLUDED.feedback_id,
  owner_role=EXCLUDED.owner_role,
  scope=EXCLUDED.scope,
  risk_level=EXCLUDED.risk_level,
  covered_alerts=EXCLUDED.covered_alerts,
  covered_assets=EXCLUDED.covered_assets,
  version=EXCLUDED.version,
  created_by=EXCLUDED.created_by,
  approved_by=EXCLUDED.approved_by,
  approved_at=EXCLUDED.approved_at,
  disabled_at=EXCLUDED.disabled_at,
  expires_at=EXCLUDED.expires_at,
  created_at=EXCLUDED.created_at,
  updated_at=EXCLUDED.updated_at;
