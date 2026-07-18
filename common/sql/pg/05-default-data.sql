-- =============================================================================
-- 默认数据 — RBAC 角色权限 + 默认配置
-- =============================================================================
BEGIN;

-- 默认角色
INSERT INTO roles (role_id, tenant_id, name, permissions) VALUES
    (uuid_generate_v4(), 'default', 'admin', '{"*":"*"}'::jsonb),
    (uuid_generate_v4(), 'default', 'viewer', '{"alert":"read","flow":"read","dashboard":"read"}'::jsonb),
    (uuid_generate_v4(), 'default', 'operator', '{"alert":"*","flow":"read","pcap":"read"}'::jsonb),
    (uuid_generate_v4(), 'default', 'probe', '{"probe":"ingest","probe":"metrics"}'::jsonb)
ON CONFLICT (tenant_id, name) DO NOTHING;

-- 资产台账验收数据：五类资产各 10 条，使用稳定主键，可重复执行。
-- 默认不写入任何验收资产；仅在显式设置
--   SET traffic.enable_asset_acceptance_fixture = 'on';
-- 后执行，避免生产初始化污染 default 租户或重写验收资产历史。
DO $asset_inventory_acceptance_fixture$
BEGIN
IF COALESCE(current_setting('traffic.enable_asset_acceptance_fixture', true), 'off') NOT IN ('on', 'true', '1') THEN
  RAISE NOTICE 'asset inventory acceptance fixture is disabled';
  RETURN;
END IF;

WITH asset_types(asset_type, prefix, type_ord, department, campus, owner, os_type) AS (
  VALUES
    ('endpoint', 'END', 1, '教务处', '主园区', '终端运维组', 'Windows 11'),
    ('server', 'SRV', 2, '计算中心', '实验楼', '平台运维组', 'Ubuntu 22.04'),
    ('network-device', 'NET', 3, '网络中心', '主园区', '网络运维组', 'Network OS'),
    ('business-system', 'BIZ', 4, '信息化办公室', '数据中心', '应用保障组', 'Application'),
    ('unknown', 'UNK', 5, '', '待确认区', '', 'Unknown')
),
fixture AS (
  SELECT
    (
      substr(md5(asset_type || '-' || n::text), 1, 8) || '-' ||
      substr(md5(asset_type || '-' || n::text), 9, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 13, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 17, 4) || '-' ||
      substr(md5(asset_type || '-' || n::text), 21, 12)
    )::uuid AS asset_id,
    prefix || '-' || lpad(n::text, 4, '0') AS display_code,
    asset_type,
    CASE WHEN n = 10 THEN 'inactive' WHEN asset_type = 'unknown' THEN 'unknown' ELSE 'active' END AS status,
    format('10.12.%s.%s', type_ord, n + 10) AS ip_address,
    format('02:00:00:00:%s:%s', lpad(type_ord::text, 2, '0'), lpad(n::text, 2, '0')) AS mac_address,
    upper(replace(asset_type, '-', '_')) || '-' || lpad(n::text, 2, '0') AS hostname,
    CASE WHEN asset_type = 'network-device' THEN 'Huawei' WHEN asset_type = 'endpoint' THEN 'Lenovo' ELSE 'OpenStack' END AS vendor,
    os_type,
    department,
    campus,
    owner,
    CASE WHEN n IN (3, 7) THEN 90 WHEN n IN (2, 6) THEN 60 ELSE 30 END AS criticality,
    n
  FROM asset_types CROSS JOIN generate_series(1, 10) AS n
)
INSERT INTO assets (
  asset_id, display_code, tenant_id, asset_type, status, ip_address, mac_address,
  hostname, vendor, os_type, source, department, campus, owner, criticality,
  tags, metadata, first_seen, last_seen
)
SELECT
  asset_id, display_code, 'default', asset_type, status, ip_address, mac_address,
  hostname, vendor, os_type, 'acceptance-fixture', NULLIF(department, ''), campus,
  NULLIF(owner, ''), criticality,
  jsonb_build_object('fixture', 'asset-inventory-v3', 'environment', 'acceptance'),
  jsonb_build_object('seed_index', n, 'data_contract', 'canonical-asset-v1'),
  now() - (n + 30) * interval '1 day',
  now() - n * interval '17 minutes'
FROM fixture
ON CONFLICT (asset_id) DO UPDATE SET
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
  last_seen = EXCLUDED.last_seen;

-- 服务器详情验收数据：作为资产记录的持久化观测上下文，由 /details 子资源读取。
UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract', 'canonical-asset-detail-v1',
  'network_interfaces', jsonb_build_array(
    jsonb_build_object('name','eth0','adapter','VirtIO Network','ip_address',ip_address,'mac_address',mac_address,'vlan_id','120','mirror_mode','no','status','up','speed','10G','duplex','full','ingress_bytes',13743895347,'egress_bytes',6871947673,'packet_loss_pct',0.02,'error_count',12,'probe_id','probe-12'),
    jsonb_build_object('name','eth1','adapter','VirtIO Network','ip_address','','mac_address','','vlan_id','121','mirror_mode','no','status','up','speed','10G','duplex','full','ingress_bytes',4402341478,'egress_bytes',2362232012,'packet_loss_pct',0.01,'error_count',6,'probe_id','probe-12'),
    jsonb_build_object('name','bond0','adapter','Bond active-backup','ip_address',ip_address,'mac_address',mac_address,'vlan_id','200','mirror_mode','both','status','monitor','speed','20G','duplex','full','ingress_bytes',19542101196,'egress_bytes',10307921510,'packet_loss_pct',0.03,'error_count',21,'probe_id','probe-12'),
    jsonb_build_object('name','eth2','adapter','Mellanox CX5','ip_address','','mac_address','','vlan_id','300','mirror_mode','ingress','status','monitor','speed','40G','duplex','full','ingress_bytes',3006477107,'egress_bytes',0,'packet_loss_pct',0.00,'error_count',0,'probe_id','probe-14'),
    jsonb_build_object('name','eth3','adapter','Intel I350','ip_address','','mac_address','','vlan_id','30','mirror_mode','no','status','down','speed','1G','duplex','full','ingress_bytes',0,'egress_bytes',0,'packet_loss_pct',100.0,'error_count',68,'probe_id','probe-12'),
    jsonb_build_object('name','mgmt0','adapter','Intel I210','ip_address',ip_address,'mac_address',mac_address,'vlan_id','10','mirror_mode','no','status','up','speed','1G','duplex','full','ingress_bytes',335544320,'egress_bytes',188743680,'packet_loss_pct',0.01,'error_count',2,'probe_id','probe-12')
  ),
  'open_services', jsonb_build_array(
    jsonb_build_object('port',22,'protocol','TCP','service','SSH','version','OpenSSH 9.6p1','exposure_scope','内网+外网','access_source_count',18,'risk_level','高危','alert_count',5),
    jsonb_build_object('port',80,'protocol','TCP','service','HTTP','version','Nginx 1.20.1','exposure_scope','外网','access_source_count',9,'risk_level','中危','alert_count',3),
    jsonb_build_object('port',443,'protocol','TCP','service','HTTPS','version','Nginx 1.20.1','exposure_scope','外网','access_source_count',23,'risk_level','中危','alert_count',6),
    jsonb_build_object('port',3306,'protocol','TCP','service','MySQL','version','8.0.32','exposure_scope','内网','access_source_count',6,'risk_level','高危','alert_count',4),
    jsonb_build_object('port',6379,'protocol','TCP','service','Redis','version','6.2.6','exposure_scope','内网','access_source_count',5,'risk_level','高危','alert_count',2),
    jsonb_build_object('port',9200,'protocol','TCP','service','OpenSearch','version','2.11.0','exposure_scope','内网+外网','access_source_count',7,'risk_level','中危','alert_count',3),
    jsonb_build_object('port',9092,'protocol','TCP','service','Kafka','version','3.4.0','exposure_scope','内网','access_source_count',4,'risk_level','中危','alert_count',1)
  ),
  'ownership', jsonb_build_object(
    'campus',campus,'department',department,'owner',owner,
    'responsibilities',jsonb_build_array(
      jsonb_build_object('role','资产管理员','owner',owner,'status','已确认'),
      jsonb_build_object('role','安全复核','owner','sec_manager','status','已确认'),
      jsonb_build_object('role','业务确认','owner','应用保障组','status','已确认'),
      jsonb_build_object('role','取证审批','owner','合规团队','status','待审批')
    ),
    'business_systems',jsonb_build_array(
      jsonb_build_object('name','教学管理系统','role','核心/承载系统','owner','应用保障组','status','已确认'),
      jsonb_build_object('name','统一身份认证','role','认证依赖','owner','平台运维组','status','已确认'),
      jsonb_build_object('name','文件存储系统','role','数据存储依赖','owner','存储运维组','status','已确认')
    ),
    'asset_groups',jsonb_build_array(
      jsonb_build_object('name','计算服务器组','role','核心','owner',owner,'status','已确认'),
      jsonb_build_object('name','数据库服务器组','role','重要','owner',owner,'status','已确认')
    ),
    'data_domains',jsonb_build_array(
      jsonb_build_object('name','教学业务域','role','重要数据','owner',owner,'status','已确认'),
      jsonb_build_object('name','运维日志域','role','运维数据','owner',owner,'status','已确认'),
      jsonb_build_object('name','用户行为域','role','行为数据','owner',owner,'status','待确认')
    ),
    'pending_fields',jsonb_build_array('上级业务系统','数据资产标签')
  )
)
WHERE tenant_id = 'default'
  AND tags->>'fixture' = 'asset-inventory-v3'
  AND asset_type = 'server';

-- 分类 Tab 的持久化业务上下文：列表下方的画像、拓扑、依赖与研判模块均读取这些字段，
-- 禁止前端用视觉常量替代生产 API 数据。
UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'risk_score', CASE WHEN criticality>=80 THEN 86 WHEN criticality>=50 THEN 64 ELSE 28 END,
  'traffic_profile', jsonb_build_array(34,46,29,51,39,58,47,64,38,53,44,61,48,70,56,63),
  'traffic_outbound', jsonb_build_array(22,31,25,36,28,41,33,49,27,38,35,46,32,51,39,45),
  'traffic_east_west', jsonb_build_array(16,24,19,31,22,35,28,42,21,33,26,38,25,44,31,37),
  'traffic_time_labels', jsonb_build_array('03:00','04:30','06:00','07:30','09:00','10:30','12:00','13:30','15:00','16:30','18:00','19:30','21:00','22:30','00:00','01:30'),
  'protocol_total_throughput', '68.4 Gbps',
  'protocols', jsonb_build_array(
    jsonb_build_object('name','TCP','percent',52.6), jsonb_build_object('name','HTTP/HTTPS','percent',21.8),
    jsonb_build_object('name','SMB','percent',8.7), jsonb_build_object('name','DNS','percent',7.1),
    jsonb_build_object('name','MySQL','percent',4.0), jsonb_build_object('name','SSH','percent',2.7),
    jsonb_build_object('name','其他','percent',3.1)
  ),
  'top_peers', jsonb_build_array(
    jsonb_build_object('name','10.12.2.11','type','服务器','share',18.8),
    jsonb_build_object('name','10.12.3.11','type','网络设备','share',12.4),
    jsonb_build_object('name','10.12.4.11','type','业务系统','share',9.6),
    jsonb_build_object('name','10.12.5.11','type','数据库','share',6.3),
    jsonb_build_object('name','8.8.8.8','type','DNS','share',4.2)
  ),
  'periodic_activity', jsonb_build_array(
    22,31,18,36,28,42,34,49,27,38,35,46,29,41,
    18,26,16,32,25,38,31,45,24,35,32,43,27,39,
    20,29,17,34,27,40,33,47,26,37,34,44,28,40,
    21,30,18,35,28,41,35,48,27,39,36,45,29,42,
    19,28,16,33,26,39,32,46,25,36,33,42,27,38,
    15,22,14,27,21,32,26,38,20,30,28,36,23,34,
    14,21,13,26,20,31,25,37,19,29,27,35,22,33
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','暴露端口','value',12,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高风险','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','弱口令','value',2,'max',8,'color','#ffb020'),
    jsonb_build_object('label','异常外联','value',3,'max',20,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',23,'session',186,'dns',342,'tls',97,'alerts',9,'config',3)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='endpoint';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'risk_score', CASE WHEN criticality>=80 THEN 88 WHEN criticality>=50 THEN 67 ELSE 31 END,
  'topology_nodes', jsonb_build_array('教学管理系统','API 服务','MySQL 数据库','Redis 缓存','Kafka 消息','probe-12','取证中心'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','teaching-system','label','教学管理系统','kind','business-system','status','healthy'),
      jsonb_build_object('id','api-service','label','API 服务','kind','service','status','healthy'),
      jsonb_build_object('id','mysql-db','label','MySQL 数据库','kind','database','status','warning'),
      jsonb_build_object('id','redis-cache','label','Redis 缓存','kind','cache','status','healthy'),
      jsonb_build_object('id','kafka-bus','label','Kafka 消息','kind','message-bus','status','healthy'),
      jsonb_build_object('id','probe-12','label','probe-12','kind','probe','status','observed'),
      jsonb_build_object('id','forensics-center','label','取证中心','kind','security','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','srv-hosts-business','source','self','target','teaching-system','relationship','hosts','direction','directed','health','healthy'),
      jsonb_build_object('id','srv-serves-api','source','self','target','api-service','relationship','serves','direction','directed','health','healthy'),
      jsonb_build_object('id','srv-reads-mysql','source','self','target','mysql-db','relationship','reads_writes','direction','bidirectional','health','warning'),
      jsonb_build_object('id','srv-uses-redis','source','self','target','redis-cache','relationship','caches','direction','bidirectional','health','healthy'),
      jsonb_build_object('id','srv-publishes-kafka','source','self','target','kafka-bus','relationship','publishes','direction','directed','health','healthy'),
      jsonb_build_object('id','probe-observes-srv','source','probe-12','target','self','relationship','observes','direction','directed','health','healthy'),
      jsonb_build_object('id','forensics-collects-srv','source','self','target','forensics-center','relationship','evidence_to','direction','directed','health','healthy')
    )
  ),
  'probe_state', jsonb_build_array(
    jsonb_build_object('label',os_type,'value','在线','status','健康'),
    jsonb_build_object('label','probe-12','value','已接入','status','在线'),
    jsonb_build_object('label','接口丢包','value','0.03%','status','健康'),
    jsonb_build_object('label','服务告警','value','9','status','告警')
  ),
  'os_distribution', jsonb_build_array(
    jsonb_build_object('name','Linux','count',7,'color','#1688ff'),
    jsonb_build_object('name','Windows Server','count',2,'color','#39c978'),
    jsonb_build_object('name','Unix','count',1,'color','#ffb020'),
    jsonb_build_object('name','其他','count',0,'color','#7a8ff5')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','暴露端口','value',7,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高危服务','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','弱口令','value',1,'max',8,'color','#ffb020'),
    jsonb_build_object('label','关联告警','value',9,'max',30,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',42,'session',186,'dns',128,'tls',53,'alerts',9,'config',4)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='server';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-network-device-v1',
  'risk_score', CASE WHEN criticality>=80 THEN 84 WHEN criticality>=50 THEN 61 ELSE 26 END,
  'network_interfaces', (
    SELECT jsonb_agg(jsonb_build_object(
      'name', 'GE0/' || port_no,
      'status', CASE WHEN port_no = 3 THEN 'error' WHEN port_no = 48 THEN 'down' ELSE 'up' END,
      'speed', '10G',
      'mirror_mode', CASE WHEN port_no IN (1,2) THEN 'source' WHEN port_no = 4 THEN 'rspan' WHEN port_no IN (47,48) THEN 'remote' ELSE 'no' END,
      'probe_id', CASE WHEN port_no IN (1,4) THEN 'probe-12' WHEN port_no = 2 THEN 'probe-13' WHEN port_no IN (47,48) THEN 'probe-09' ELSE '' END
    ) ORDER BY port_no) FROM generate_series(1,48) AS ports(port_no)
  ),
  'topology_nodes', jsonb_build_array('边界路由器','汇聚交换机','接入交换机','园区防火墙','probe-12','服务器网段'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','edge-router','label','边界路由器','kind','router','status','healthy'),
      jsonb_build_object('id','aggregation-switch','label','汇聚交换机','kind','switch','status','healthy'),
      jsonb_build_object('id','access-switch','label','接入交换机','kind','switch','status','healthy'),
      jsonb_build_object('id','campus-firewall','label','园区防火墙','kind','firewall','status','warning'),
      jsonb_build_object('id','probe-12','label','probe-12','kind','probe','status','observed'),
      jsonb_build_object('id','server-segment','label','服务器网段','kind','subnet','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','router-uplink','source','edge-router','target','self','relationship','uplink','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','aggregation-link','source','aggregation-switch','target','self','relationship','trunk','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','access-link','source','self','target','access-switch','relationship','downlink','direction','bidirectional','protocol','LLDP','health','healthy'),
      jsonb_build_object('id','firewall-link','source','self','target','campus-firewall','relationship','security_path','direction','bidirectional','protocol','LLDP','health','warning'),
      jsonb_build_object('id','probe-mirror','source','self','target','probe-12','relationship','mirror_to','direction','directed','protocol','SPAN','health','healthy'),
      jsonb_build_object('id','server-segment-link','source','self','target','server-segment','relationship','serves_subnet','direction','bidirectional','protocol','VLAN','health','healthy')
    )
  ),
  'mirror_links', jsonb_build_array(
    jsonb_build_object('interface','GE0/1','direction','Out','mode','SPAN Source','target','probe-12','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/2','direction','Out','mode','SPAN Source','target','probe-13','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/4','direction','Out','mode','RSPAN VLAN 300','target','probe-13','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/48','direction','Out','mode','远程镜像','target','FW-01:mirror','bandwidth','10G','status','异常'),
    jsonb_build_object('interface','GE0/47','direction','In','mode','远程镜像','target','probe-09','bandwidth','10G','status','在线'),
    jsonb_build_object('interface','GE0/12','direction','Both','mode','ERSPAN','target','probe-08','bandwidth','10G','status','在线')
  ),
  'config_changes', jsonb_build_array(
    jsonb_build_object('time','06-20 02:12','actor','admin','change','接口配置','risk','中'),
    jsonb_build_object('time','06-19 21:35','actor','netops','change','VLAN 变更','risk','中'),
    jsonb_build_object('time','06-19 18:06','actor','admin','change','ACL 规则','risk','高'),
    jsonb_build_object('time','06-18 16:42','actor','netops','change','端口关闭','risk','高'),
    jsonb_build_object('time','06-18 09:18','actor','audit','change','镜像策略','risk','低')
  ),
  'business_impacts', jsonb_build_array(
    jsonb_build_object('name','教学管理系统','links',12,'traffic','32.6%','risk','高'),
    jsonb_build_object('name','科研数据平台','links',9,'traffic','24.1%','risk','高'),
    jsonb_build_object('name','统一认证平台','links',6,'traffic','15.8%','risk','中'),
    jsonb_build_object('name','图书馆门户','links',4,'traffic','9.6%','risk','中'),
    jsonb_build_object('name','财务结算系统','links',3,'traffic','6.1%','risk','低')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','接口总数','value',48,'max',48,'color','#1688ff'),
    jsonb_build_object('label','Up 接口','value',46,'max',48,'color','#39c978'),
    jsonb_build_object('label','Err-Disable','value',1,'max',8,'color','#ffb020'),
    jsonb_build_object('label','Down 接口','value',1,'max',8,'color','#ff4d4f')
  ),
  'evidence', jsonb_build_object('pcap',32,'session',128,'dns',86,'tls',53,'alerts',19,'config',8)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='network-device';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-business-system-v1',
  'risk_score', CASE WHEN criticality>=80 THEN 86 WHEN criticality>=50 THEN 65 ELSE 29 END,
  'business_domain','教学教务','system_level','核心','sla_target','99.5%','sla_current','99.2%',
  'topology_nodes', jsonb_build_array('统一认证平台','数据库集群','科研数据平台','图书馆门户','财务结算系统','消息队列'),
  'topology_graph', jsonb_build_object(
    'nodes', jsonb_build_array(
      jsonb_build_object('id','sso-platform','label','统一认证平台','kind','business-system','status','healthy'),
      jsonb_build_object('id','database-cluster','label','数据库集群','kind','database','status','warning'),
      jsonb_build_object('id','research-platform','label','科研数据平台','kind','business-system','status','healthy'),
      jsonb_build_object('id','library-portal','label','图书馆门户','kind','business-system','status','healthy'),
      jsonb_build_object('id','finance-system','label','财务结算系统','kind','business-system','status','healthy'),
      jsonb_build_object('id','message-queue','label','消息队列','kind','message-bus','status','healthy')
    ),
    'edges', jsonb_build_array(
      jsonb_build_object('id','biz-depends-sso','source','self','target','sso-platform','relationship','depends_on','direction','directed','health','healthy'),
      jsonb_build_object('id','biz-reads-db','source','self','target','database-cluster','relationship','reads_writes','direction','bidirectional','health','warning'),
      jsonb_build_object('id','research-calls-biz','source','research-platform','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','library-calls-biz','source','library-portal','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','finance-calls-biz','source','finance-system','target','self','relationship','calls','direction','directed','health','healthy'),
      jsonb_build_object('id','biz-publishes-queue','source','self','target','message-queue','relationship','publishes','direction','directed','health','healthy')
    )
  ),
  'risk_factors', jsonb_build_array(
    jsonb_build_object('name','漏洞暴露','percent',28), jsonb_build_object('name','异常外联','percent',24),
    jsonb_build_object('name','高危服务','percent',20), jsonb_build_object('name','证据缺口','percent',16)
  ),
  'risk_distribution', jsonb_build_array(
    jsonb_build_object('range','90-100 高风险','count',21,'percent_label','21  14.4%','color','#ff4d4f'),
    jsonb_build_object('range','70-89 较高风险','count',45,'percent_label','45  30.8%','color','#ff8a34'),
    jsonb_build_object('range','40-69 中风险','count',54,'percent_label','54  37.0%','color','#f2c94c'),
    jsonb_build_object('range','20-39 较低风险','count',18,'percent_label','18  12.3%','color','#39c978'),
    jsonb_build_object('range','0-19 低风险','count',8,'percent_label','8  5.5%','color','#1688ff')
  ),
  'key_services', jsonb_build_array(
    jsonb_build_object('name','Web Portal','endpoint','443/TCP','dependency','统一认证平台','risk','高危','health','降级'),
    jsonb_build_object('name','教务 API','endpoint','8080/TCP','dependency','数据库集群','risk','高危','health','健康'),
    jsonb_build_object('name','数据库服务','endpoint','5432/TCP','dependency','数据库集群','risk','高危','health','健康'),
    jsonb_build_object('name','消息队列','endpoint','5672/TCP','dependency','科研数据平台','risk','中危','health','健康'),
    jsonb_build_object('name','缓存服务','endpoint','6379/TCP','dependency','Redis 集群','risk','低危','health','健康')
  ),
  'dependency_health', jsonb_build_array(
    jsonb_build_object('type','服务器','total',86,'abnormal',2,'health','98.0%'),
    jsonb_build_object('type','数据库','total',12,'abnormal',1,'health','95.8%'),
    jsonb_build_object('type','存储设备','total',6,'abnormal',1,'health','97.2%'),
    jsonb_build_object('type','网络设备','total',18,'abnormal',0,'health','99.1%')
  ),
  'responsibility', jsonb_build_array(
    jsonb_build_object('department','教务处','role','主管部门','owner','张老师','sla','99.5%','status','正常'),
    jsonb_build_object('department','信息中心','role','技术支撑','owner','李老师','sla','99.0%','status','正常'),
    jsonb_build_object('department','运维团队','role','运维保障','owner','王老师','sla','99.0%','status','正常'),
    jsonb_build_object('department','安全中心','role','安全治理','owner','赵老师','sla','98.5%','status','正常')
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','风险评分','value',CASE WHEN criticality>=80 THEN 86 WHEN criticality>=50 THEN 65 ELSE 29 END,'max',100,'color','#ff4d4f'),
    jsonb_build_object('label','依赖资产','value',122,'max',220,'color','#1688ff'),
    jsonb_build_object('label','关键服务','value',5,'max',30,'color','#7a8ff5'),
    jsonb_build_object('label','高风险服务','value',3,'max',20,'color','#ff8a34')
  ),
  'evidence', jsonb_build_object('pcap',42,'session',186,'dns',128,'tls',53,'alerts',9,'config',6)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='business-system';

UPDATE assets
SET metadata = metadata || jsonb_build_object(
  'data_contract','canonical-unknown-asset-v1','risk_score',49,'suspected_type','未知识别','confidence',42,'ticket_status','待确认',
  'discovery_timeline', jsonb_build_array(
    jsonb_build_object('event','流量探针首次发现','time','06-19 21:33','status','已完成'),
    jsonb_build_object('event','ARP 绑定采集','time','06-19 21:36','status','已完成'),
    jsonb_build_object('event','DNS/TLS 指纹聚合','time','06-20 03:41','status','已完成'),
    jsonb_build_object('event','归属候选匹配','time','06-20 03:44','status','待复核')
  ),
  'discovery_activity', jsonb_build_object(
    'labels',jsonb_build_array('00:00','02:00','04:00','06:00','08:00','10:00','12:00','14:00','16:00','18:00','20:00','22:00'),
    'discovered',jsonb_build_array(4,6,5,8,12,18,15,21,16,11,8,4),
    'pending_rate',jsonb_build_array(72,69,66,63,61,58,55,52,49,47,45,42)
  ),
  'device_profile_distribution', jsonb_build_array(
    jsonb_build_object('name','Windows 终端','count',38,'color','#1688ff'),
    jsonb_build_object('name','Linux 主机','count',26,'color','#39c978'),
    jsonb_build_object('name','IoT 设备','count',22,'color','#ffb020'),
    jsonb_build_object('name','网络设备','count',18,'color','#7a8ff5'),
    jsonb_build_object('name','移动终端','count',14,'color','#27b8e6'),
    jsonb_build_object('name','其他','count',10,'color','#8a9aaa')
  ),
  'fingerprint', jsonb_build_object('mac_oui','Intel X710','dhcp_hostname',hostname,'ttl_os','128 / Windows 10/11','open_ports','135, 445, 5985, 3389','ja3','ja3_72d8b9a3c1f2','behavior','频繁访问内网文件共享'),
  'ownership_candidates', jsonb_build_array(
    jsonb_build_object('department','计算中心','owner','张老师','matched','15','confidence','72%'),
    jsonb_build_object('department','信息中心','owner','李老师','matched','12','confidence','65%'),
    jsonb_build_object('department','实验室','owner','王老师','matched','8','confidence','58%'),
    jsonb_build_object('department','图书馆','owner','赵老师','matched','6','confidence','46%'),
    jsonb_build_object('department','后勤中心','owner','陈老师','matched','4','confidence','38%')
  ),
  'exposure', jsonb_build_object('open_ports',7,'high_services',3,'weak_password',1,'related_alerts',5,'risk_score',49),
  'risk_distribution', jsonb_build_array(
    jsonb_build_object('name','高风险','count',22,'color','#ff4d4f'),
    jsonb_build_object('name','中风险','count',47,'color','#ffb020'),
    jsonb_build_object('name','低风险','count',59,'color','#39c978')
  ),
  'ticket_steps', jsonb_build_array('发现 / 已完成','归属确认 / 待处理','风险复核 / 待处理','验证 / 未开始','关闭 / 未开始'),
  'source_distribution', jsonb_build_array(
    jsonb_build_object('name','流量探针','count',62), jsonb_build_object('name','DHCP 日志','count',34),
    jsonb_build_object('name','ARP 扫描','count',18), jsonb_build_object('name','终端扫描','count',9),
    jsonb_build_object('name','其他','count',5)
  ),
  'governance_metrics', jsonb_build_array(
    jsonb_build_object('label','风险评分','value',49,'max',100,'color','#ffb020'),
    jsonb_build_object('label','暴露端口','value',7,'max',20,'color','#1688ff'),
    jsonb_build_object('label','高危服务','value',3,'max',10,'color','#ff4d4f'),
    jsonb_build_object('label','关联告警','value',5,'max',30,'color','#7a8ff5')
  ),
  'evidence', jsonb_build_object('pcap',68,'session',156,'dns',342,'tls',97,'alerts',19,'config',8)
)
WHERE tenant_id='default' AND tags->>'fixture'='asset-inventory-v3' AND asset_type='unknown';

DELETE FROM asset_events
WHERE asset_id IN (
  SELECT asset_id FROM assets
  WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3'
);

INSERT INTO asset_events (asset_id, tenant_id, event_type, old_value, new_value, created_at)
SELECT asset_id, tenant_id, 'asset.discovered', '{}'::jsonb,
       jsonb_build_object('display_code', display_code, 'asset_type', asset_type),
       first_seen
FROM assets
WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3'
UNION ALL
SELECT asset_id, tenant_id, 'asset.governance.updated',
       jsonb_build_object('owner', NULL),
       jsonb_build_object('owner', owner, 'department', department, 'campus', campus),
       last_seen
FROM assets
WHERE tenant_id = 'default' AND tags->>'fixture' = 'asset-inventory-v3';

-- 跨页取证验收必须返回真实且按资产范围过滤的任务。该记录仍受上方
-- traffic.enable_asset_acceptance_fixture 显式开关保护，不进入默认生产初始化。
INSERT INTO tasks (
  task_id, tenant_id, name, task_type, params, status, progress,
  result_file_key, result_sha256, result_packets, result_bytes, files_scanned,
  run_id, created_by, created_at, updated_at, started_at, completed_at
)
SELECT
  md5('asset-inventory-v3-pcap-' || asset_id::text)::uuid,
  tenant_id,
  '资产台账取证验收-' || display_code,
  'pcap_cut',
  jsonb_build_object(
    'asset_id', asset_id::text,
    'display_code', display_code,
    'fixture', 'asset-inventory-v3',
    'start_time', (extract(epoch FROM first_seen) * 1000)::bigint,
    'end_time', (extract(epoch FROM last_seen) * 1000)::bigint
  ),
  'completed', 100,
  'acceptance/assets/' || asset_id::text || '/capture.pcap',
  repeat('a', 64), 128, 65536, 1,
  'asset-inventory-v3', 'asset-inventory-acceptance',
  first_seen, last_seen, first_seen, last_seen
FROM assets
WHERE tenant_id = 'default'
  AND tags->>'fixture' = 'asset-inventory-v3'
  AND display_code = 'SRV-0001'
ON CONFLICT (task_id) DO UPDATE SET
  params = EXCLUDED.params,
  status = EXCLUDED.status,
  progress = EXCLUDED.progress,
  result_file_key = EXCLUDED.result_file_key,
  result_sha256 = EXCLUDED.result_sha256,
  result_packets = EXCLUDED.result_packets,
  result_bytes = EXCLUDED.result_bytes,
  files_scanned = EXCLUDED.files_scanned,
  updated_at = EXCLUDED.updated_at,
  completed_at = EXCLUDED.completed_at;

END;
$asset_inventory_acceptance_fixture$;

-- 默认 feature_set (L1 全量统计)
INSERT INTO feature_sets (feature_set_id, name, params, schema_version)
VALUES ('v1-l1-default', 'L1 Flow Statistics',
    '{"window":"flow","features":["packets","bytes","duration","pps","bps","pktlen_stats","iat_stats","tcp_flags","tos"]}'::jsonb,
    'v1')
ON CONFLICT (feature_set_id) DO NOTHING;

-- 默认模型 (XGBoost 行为检测)
INSERT INTO models (model_id, tenant_id, name, model_type, description) VALUES
    (uuid_generate_v4(), 'default', 'behavior-xgboost-v1', 'gbdt', 'XGBoost behavioral detection model'),
    (uuid_generate_v4(), 'default', 'business-rule-v1', 'rules', 'Business rule detection engine'),
    (uuid_generate_v4(), 'default', 'vpn-detector-v1', 'onnx', 'VPN traffic classifier')
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
