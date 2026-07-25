-- =============================================================================
-- 战役影响范围资产关联
--
-- 这两条记录对应当前双节点部署的真实基础设施资产。战役告警通过
-- Community ID 关联 sessions，再以 sessions 的真实端点 IP 关联本表；
-- 账号、服务、部门、校区和业务系统影响面由资产元数据统一投影。
-- =============================================================================
BEGIN;

INSERT INTO assets (
  asset_id, display_code, tenant_id, asset_type, status, ip, ip_address, mac_address,
  hostname, vendor, os_type, source, department, campus, owner, criticality,
  tags, metadata, first_seen, last_seen, created_at, updated_at
) VALUES
(
  '10000000-0000-4000-8000-000000000058'::uuid,
  'INFRA-CP-01', 'default', 'server', 'active', '10.0.5.8', '10.0.5.8', '02:00:05:08:00:01',
  '8-2TB', 'OpenEuler', 'openEuler 22.03', 'infrastructure-inventory',
  '信息中心', '主园区-数据中心', '平台运维组', 5,
  '{"environment":"production","role":"kubernetes-control-plane","data_contract":"campaign-impact-v1"}'::jsonb,
  jsonb_build_object(
    'business_system', '全流量分析平台',
    'key_service', 'Kubernetes API / ClickHouse',
    'recovery_priority', 'P0',
    'network_path', '主园区核心链路',
    'campaign_accounts', jsonb_build_array(
      jsonb_build_object('account','sec_analyst','account_type','人员账号','permission_risk','高危','login_path','统一认证 -> 10.0.5.8'),
      jsonb_build_object('account','platform_ops','account_type','管理账号','permission_risk','高危','login_path','堡垒机 -> 10.0.5.8')
    ),
    'open_services', jsonb_build_array(
      jsonb_build_object('service','Kubernetes API','port',6443,'protocol','TCP','risk_level','高危','dependency','全流量分析平台'),
      jsonb_build_object('service','VXLAN','port',8472,'protocol','UDP','risk_level','中危','dependency','集群容器网络')
    )
  ),
  now() - interval '49 days', now(), now(), now()
),
(
  '10000000-0000-4000-8000-000000000059'::uuid,
  'INFRA-WK-01', 'default', 'server', 'active', '10.0.5.9', '10.0.5.9', '02:00:05:09:00:01',
  'zeus-server', 'OpenEuler', 'openEuler 22.03', 'infrastructure-inventory',
  '信息中心', '主园区-数据中心', '平台运维组', 4,
  '{"environment":"production","role":"kubernetes-worker","data_contract":"campaign-impact-v1"}'::jsonb,
  jsonb_build_object(
    'business_system', '全流量分析平台',
    'key_service', 'ClickHouse / Web UI',
    'recovery_priority', 'P0',
    'network_path', '主园区核心链路',
    'campaign_accounts', jsonb_build_array(
      jsonb_build_object('account','ops_admin','account_type','管理账号','permission_risk','高危','login_path','堡垒机 -> 10.0.5.9'),
      jsonb_build_object('account','svc_sync','account_type','服务账号','permission_risk','中危','login_path','10.0.5.8 -> 10.0.5.9')
    ),
    'open_services', jsonb_build_array(
      jsonb_build_object('service','ClickHouse Native','port',9000,'protocol','TCP','risk_level','高危','dependency','全流量分析平台'),
      jsonb_build_object('service','VXLAN','port',8472,'protocol','UDP','risk_level','中危','dependency','集群容器网络')
    )
  ),
  now() - interval '49 days', now(), now(), now()
)
ON CONFLICT (tenant_id, ip) WHERE ip IS NOT NULL DO UPDATE SET
  display_code = EXCLUDED.display_code,
  asset_type = EXCLUDED.asset_type,
  status = EXCLUDED.status,
  ip_address = EXCLUDED.ip_address,
  mac_address = EXCLUDED.mac_address,
  hostname = EXCLUDED.hostname,
  vendor = EXCLUDED.vendor,
  os_type = EXCLUDED.os_type,
  source = EXCLUDED.source,
  department = EXCLUDED.department,
  campus = EXCLUDED.campus,
  owner = EXCLUDED.owner,
  criticality = EXCLUDED.criticality,
  tags = EXCLUDED.tags,
  metadata = EXCLUDED.metadata,
  last_seen = EXCLUDED.last_seen,
  updated_at = EXCLUDED.updated_at;

COMMIT;
