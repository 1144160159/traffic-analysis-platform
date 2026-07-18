-- Idempotent seeded data for the rule-management full-stack acceptance loop.
-- scenario_id: ui-rules-v1; cleanup: DELETE ... WHERE scenario_id='ui-rules-v1'.
BEGIN;

DELETE FROM rule_workbench_items
WHERE tenant_id = 'default' AND scenario_id = 'ui-rules-v1';

INSERT INTO rule_workbench_items
  (item_id, tenant_id, rule_id, category, ordinal, payload, scenario_id, occurred_at)
VALUES
  ('ui-rules-v1-pcap-01','default','*','pcap_samples',1,'{"id":"c2_tunnel_01.pcap","size":"12.4 MB","source":"探针-01","time":"06-19 22:18"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-pcap-02','default','*','pcap_samples',2,'{"id":"c2_tunnel_02.pcap","size":"8.7 MB","source":"探针-07","time":"06-19 19:41"}','ui-rules-v1','2026-06-19 19:41:00+08'),
  ('ui-rules-v1-pcap-03','default','*','pcap_samples',3,'{"id":"c2_tunnel_03.pcap","size":"15.2 MB","source":"探针-03","time":"06-19 16:02"}','ui-rules-v1','2026-06-19 16:02:00+08'),
  ('ui-rules-v1-pcap-04','default','*','pcap_samples',4,'{"id":"c2_tunnel_04.pcap","size":"6.3 MB","source":"探针-12","time":"06-19 13:37"}','ui-rules-v1','2026-06-19 13:37:00+08'),
  ('ui-rules-v1-session-01','default','*','session_samples',1,'{"id":"ses_20260619_001","tuple":"10.12.4.12:53120 → 10.20.4.19:443","protocol":"TLS / JA3命中","fields":["tls.sni","ja3_hash"]}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-session-02','default','*','session_samples',2,'{"id":"ses_20260619_002","tuple":"10.12.4.12:49102 → 10.20.4.20:3306","protocol":"MySQL 异常外联","fields":["dst_port","bytes_out"]}','ui-rules-v1','2026-06-19 19:41:00+08'),
  ('ui-rules-v1-session-03','default','*','session_samples',3,'{"id":"ses_20260619_003","tuple":"10.12.4.18:55221 → 10.20.0.53:53","protocol":"DNS 查询突增","fields":["qname","qtype"]}','ui-rules-v1','2026-06-19 16:02:00+08'),
  ('ui-rules-v1-session-04','default','*','session_samples',4,'{"id":"ses_20260619_004","tuple":"10.12.7.23:3389 → 10.12.4.21:445","protocol":"RDP/SMB 横向","fields":["proto","duration"]}','ui-rules-v1','2026-06-19 13:37:00+08'),
  ('ui-rules-v1-log-01','default','*','log_samples',1,'{"id":"log_20260619_001","source":"FW-北区-01","fields":["dst_port","action"],"reason":"C2端口命中","false_positive":false}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-log-02','default','*','log_samples',2,'{"id":"log_20260619_002","source":"用户事件","fields":["account","login_time"],"reason":"非工作时间登录","false_positive":true}','ui-rules-v1','2026-06-19 19:41:00+08'),
  ('ui-rules-v1-log-03','default','*','log_samples',3,'{"id":"log_20260619_003","source":"WAF-实验楼","fields":["uri","method"],"reason":"WebShell 字段命中","false_positive":false}','ui-rules-v1','2026-06-19 16:02:00+08'),
  ('ui-rules-v1-log-04','default','*','log_samples',4,'{"id":"log_20260619_004","source":"DNS-01","fields":["qname","qtype"],"reason":"隧道域名特征","false_positive":false}','ui-rules-v1','2026-06-19 13:37:00+08'),
  ('ui-rules-v1-validation-01','default','*','validation_results',1,'{"sample":"SMP-0001","type":"PCAP","fields":"dest_ip, dport, proto","expected":"告警（高危）","actual":"告警（高危）","difference":"—","source":"PCAP 包 32"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-validation-02','default','*','validation_results',2,'{"sample":"SMP-0002","type":"Session","fields":"http_host, uri, method","expected":"告警（中危）","actual":"忽略","difference":"误报","source":"Session 78"}','ui-rules-v1','2026-06-19 19:41:00+08'),
  ('ui-rules-v1-dependency-01','default','*','dependencies',1,'{"type":"模型","name":"Model-XGB-17","version":"v2.3.1 / 生效中","impact":"全量告警评分与命中","risk":"高","updated_at":"2026-06-18 11:23:15"}','ui-rules-v1','2026-06-18 11:23:15+08'),
  ('ui-rules-v1-dependency-02','default','*','dependencies',2,'{"type":"白名单","name":"WL-VPN-003","version":"生效中（12 条）","impact":"受影响规则命中结果","risk":"中","updated_at":"2026-06-18 16:42:31"}','ui-rules-v1','2026-06-18 16:42:31+08'),
  ('ui-rules-v1-hit-01','default','*','hit_matrix',1,'{"rule":"C2_Tunnel_v3","tp":"1,142","fp":"4","tn":"18,932","fn":"8","false_positive_rate":"0.35%"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-fp-01','default','*','false_positives',1,'{"id":"fp_20260619_001","count":"7","type":"正常流量","source":"探针-05"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-whitelist-01','default','*','whitelist_suggestions',1,'{"entity":"10.12.2.45","count":"156"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-performance-01','default','*','performance',1,'{"label":"平均延时","value":"18 ms","delta":"+2 ms","tone":"info","values":[14,16,15,17,16,18,18]}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-performance-02','default','*','performance',2,'{"label":"P95 延时","value":"46 ms","delta":"+5 ms","tone":"warn","values":[36,39,42,40,44,41,46]}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-performance-03','default','*','performance',3,'{"label":"CPU占用","value":"7.4%","delta":"-0.6%","tone":"ok","values":[8.1,8,7.8,7.6,7.9,7.5,7.4]}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-performance-04','default','*','performance',4,'{"label":"内存占用","value":"1.2 GB","delta":"+0.1 GB","tone":"info","values":[1,1.1,1.1,1.2,1.1,1.2,1.2]}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-definition','default','*','rule_definition',1,'{"conditions":[{"field":"协议 (proto)","operator":"在","value":"TLS_SSH"},{"field":"JA3 指纹 (ja3_score)","operator":"大于","value":"0.82"},{"field":"目的IP信誉 (dst_reputation)","operator":"等于","value":"high"},{"field":"出站流量 P95 (bytes_out_p95)","operator":"大于","value":"5 MB"}],"dsl":"when\n  proto in {\"TLS\", \"SSH\"}\n  and ja3_score > 0.82\n  and dst_reputation == \"high\"\n  and bytes_out_p95 > 5 MB\nthen\n  alert(\"selected-rule\")\n  level = high\n  category = \"C2\"\n  mitre = [\"TA0011\"]\nend","mitre":"TA0011 指挥与控制"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-approval-01','default','*','approvals',1,'{"name":"语法校验","status":"已通过"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-approval-02','default','*','approvals',2,'{"name":"逻辑评审","status":"已通过"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-approval-03','default','*','approvals',3,'{"name":"安全评审","status":"待评审"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-approval-04','default','*','approvals',4,'{"name":"运营评审","status":"待评审"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-approval-05','default','*','approvals',5,'{"name":"最终审核","status":"待提交"}','ui-rules-v1','2026-06-19 22:18:00+08'),
  ('ui-rules-v1-validation-03','default','*','validation_results',3,'{"sample":"SMP-0003","type":"PCAP","fields":"dns.qname, qtype","expected":"忽略","actual":"忽略","difference":"—","source":"PCAP 包 12"}','ui-rules-v1','2026-06-19 16:02:00+08'),
  ('ui-rules-v1-validation-04','default','*','validation_results',4,'{"sample":"SMP-0004","type":"Session","fields":"tls.sni, ja3.hash","expected":"告警（低危）","actual":"告警（低危）","difference":"—","source":"Session 45"}','ui-rules-v1','2026-06-19 13:37:00+08'),
  ('ui-rules-v1-validation-05','default','*','validation_results',5,'{"sample":"SMP-0005","type":"PCAP","fields":"src_ip, bytes, duration","expected":"告警（高危）","actual":"告警（中危）","difference":"级别差异","source":"PCAP 包 56"}','ui-rules-v1','2026-06-19 12:21:00+08'),
  ('ui-rules-v1-dependency-03','default','*','dependencies',3,'{"type":"部署","name":"PROD-北区","version":"生产 / 运行中","impact":"1,256 台资产","risk":"高","updated_at":"2026-06-19 09:10:02"}','ui-rules-v1','2026-06-19 09:10:02+08'),
  ('ui-rules-v1-dependency-04','default','*','dependencies',4,'{"type":"数据源","name":"detections.v1","version":"v1.8.2 / 实时","impact":"全量流量解析","risk":"低","updated_at":"2026-06-18 22:51:17"}','ui-rules-v1','2026-06-18 22:51:17+08'),
  ('ui-rules-v1-dependency-05','default','*','dependencies',5,'{"type":"字段","name":"src_ip / dst_port / tls.sni","version":"映射完整 / 生效","impact":"规则匹配与提取","risk":"低","updated_at":"2026-06-18 10:05:43"}','ui-rules-v1','2026-06-18 10:05:43+08'),
  ('ui-rules-v1-dependency-06','default','*','dependencies',6,'{"type":"告警类型","name":"C2 Beacon","version":"v1.2.0 / 生效中","impact":"告警联动与分级","risk":"中","updated_at":"2026-06-18 14:33:20"}','ui-rules-v1','2026-06-18 14:33:20+08'),
  ('ui-rules-v1-hit-02','default','*','hit_matrix',2,'{"rule":"Lateral_Move_v2","tp":"837","fp":"3","tn":"19,102","fn":"6","false_positive_rate":"0.25%"}','ui-rules-v1','2026-06-19 20:18:00+08'),
  ('ui-rules-v1-hit-03','default','*','hit_matrix',3,'{"rule":"DNS_Tunnel_v2","tp":"598","fp":"3","tn":"19,144","fn":"4","false_positive_rate":"0.33%"}','ui-rules-v1','2026-06-19 18:18:00+08'),
  ('ui-rules-v1-hit-04','default','*','hit_matrix',4,'{"rule":"Data_Exfil_v1","tp":"1,034","fp":"5","tn":"18,706","fn":"7","false_positive_rate":"0.48%"}','ui-rules-v1','2026-06-19 16:18:00+08'),
  ('ui-rules-v1-fp-02','default','*','false_positives',2,'{"id":"fp_20260619_002","count":"5","type":"软件更新","source":"探针-02"}','ui-rules-v1','2026-06-19 20:18:00+08'),
  ('ui-rules-v1-fp-03','default','*','false_positives',3,'{"id":"fp_20260618_009","count":"4","type":"备份同步","source":"探针-09"}','ui-rules-v1','2026-06-18 22:18:00+08'),
  ('ui-rules-v1-fp-04','default','*','false_positives',4,'{"id":"fp_20260618_015","count":"3","type":"远程管理","source":"探针-11"}','ui-rules-v1','2026-06-18 20:18:00+08'),
  ('ui-rules-v1-whitelist-02','default','*','whitelist_suggestions',2,'{"entity":"10.12.3.78","count":"121"}','ui-rules-v1','2026-06-19 20:18:00+08'),
  ('ui-rules-v1-whitelist-03','default','*','whitelist_suggestions',3,'{"entity":"172.16.5.23","count":"98"}','ui-rules-v1','2026-06-19 18:18:00+08'),
  ('ui-rules-v1-whitelist-04','default','*','whitelist_suggestions',4,'{"entity":"update.campus.local","count":"86"}','ui-rules-v1','2026-06-19 16:18:00+08'),
  ('ui-rules-v1-whitelist-05','default','*','whitelist_suggestions',5,'{"entity":"backup.internal.local","count":"65"}','ui-rules-v1','2026-06-19 14:18:00+08');

COMMIT;
