BEGIN;

CREATE TABLE IF NOT EXISTS topic_panel_simulations (
  simulation_id TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL DEFAULT '*',
  topic         TEXT NOT NULL CHECK (topic IN ('tunnel', 'exfil', 'apt')),
  version       TEXT NOT NULL,
  enabled       BOOLEAN NOT NULL DEFAULT true,
  payload       JSONB NOT NULL,
  created_by    TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, topic, version)
);

CREATE INDEX IF NOT EXISTS idx_topic_panel_simulations_active
  ON topic_panel_simulations (tenant_id, topic, enabled, updated_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_topic_panel_simulations_one_enabled
  ON topic_panel_simulations (tenant_id, topic)
  WHERE enabled;

INSERT INTO topic_panel_simulations
  (simulation_id, tenant_id, topic, version, enabled, payload, created_by)
VALUES
  ('topic-tunnel-ui-v1', '*', 'tunnel', 'ui-suite-gpt-v1', true, $json$
  {
    "presentation": {
      "topic_id": "tunnel-20260620-01",
      "site": "主校区",
      "asset_group": "办公终端 / 服务群组",
      "ip_range": "10.12.0.0/16",
      "protocols": "SSH / TLS / HTTPS / RDP / SOCKS",
      "time_window_label": "近7天（2026-06-13 00:00:00 ~ 2026-06-20 03:45:00）",
      "rule_version": "加密隧道规则集 v2.1",
      "model_version": "加密隧道识别模型 v1.3",
      "report_title": "加密隧道专题_试点周报",
      "report_time_range": "2026-06-13 ~ 2026-06-20",
      "report_generated_at": "2026-06-20 03:40:12",
      "report_scope": "办公终端 / 服务群组",
      "report_conclusion": "发现 64 条异常隧道事件，18 项风险尚未闭环。"
    },
    "summary": {
      "protocol_count": 7,
      "active_users": 23,
      "session_count": 64,
      "encrypted_traffic_gbps": 78.3,
      "endpoint_count": 312,
      "suspicious_ratio": 18.6,
      "evidence_completeness": 62,
      "report_confidence": 62,
      "open_risk_count": 18,
      "reportable_count": 7,
      "pending_evidence_count": 3,
      "total_events": 128
    },
    "metric_deltas": {
      "protocol_count": "较昨日 +1",
      "active_users": "较昨日 +3",
      "encrypted_traffic_gbps": "较昨日 +12.6%",
      "session_count": "较昨日 +7",
      "endpoint_count": "较昨日 +11",
      "suspicious_ratio": "较昨日 +4.2%",
      "evidence_completeness": "较昨日 +8%",
      "report_confidence": "较昨日 +8%",
      "open_risk_count": "较昨日 -2"
    },
    "protocols": [
      {"protocol":"SSH","count":32,"total_bytes":26950919782},
      {"protocol":"TLS","count":28,"total_bytes":23514945946},
      {"protocol":"HTTPS","count":20,"total_bytes":16750372454},
      {"protocol":"RDP","count":10,"total_bytes":8375186227},
      {"protocol":"SOCKS","count":6,"total_bytes":5046586573},
      {"protocol":"其他","count":4,"total_bytes":3435973837}
    ],
    "users": [
      {"event_id":"TN-20260620-0001","ip":"10.12.8.45","dst_ip":"203.0.113.45(SG)","protocol":"SSH","risk":"critical","count":18,"total_bytes":19542101197,"last_seen":1781920800000,"evidence_type":"PCAP","phase":"横向移动","risk_action":"PCAP / Session / 证书 / 回溯路径 / 审计日志"},
      {"event_id":"TN-20260620-0002","ip":"10.12.6.78","dst_ip":"198.51.100.77(US)","protocol":"TLS","risk":"high","count":15,"total_bytes":15784004813,"last_seen":1781917200000,"evidence_type":"Session","phase":"横向移动","risk_action":"PCAP / Session / 证书 / 回溯路径 / 审计日志"},
      {"event_id":"TN-20260620-0003","ip":"10.12.9.33","dst_ip":"104.16.24.34(US)","protocol":"HTTPS","risk":"high","count":10,"total_bytes":10307921510,"last_seen":1781913600000,"evidence_type":"证书","phase":"数据外传","risk_action":"PCAP / Session / 证书 / 回溯路径 / 审计日志"},
      {"event_id":"TN-20260620-0004","ip":"10.12.3.67","dst_ip":"45.77.34.12(NL)","protocol":"RDP","risk":"medium","count":7,"total_bytes":7301444403,"last_seen":1781910000000,"evidence_type":"PCAP","phase":"横向移动","risk_action":"PCAP / Session / 证书 / 回溯路径 / 审计日志"},
      {"event_id":"TN-20260620-0005","ip":"10.12.2.55","dst_ip":"23.227.38.65(US)","protocol":"SOCKS","risk":"medium","count":4,"total_bytes":4617089843,"last_seen":1781906400000,"evidence_type":"回溯路径","phase":"数据外传","risk_action":"PCAP / Session / 证书 / 回溯路径 / 审计日志"}
    ],
    "destination_distribution": [
      {"label":"美国 (US)","value":2134,"asn":"AS15169","traffic_gb":28.0},
      {"label":"新加坡 (SG)","value":1421,"asn":"AS133481","traffic_gb":16.5},
      {"label":"德国 (DE)","value":1098,"asn":"AS3320","traffic_gb":9.2},
      {"label":"荷兰 (NL)","value":987,"asn":"AS6830","traffic_gb":6.4},
      {"label":"香港 (HK)","value":652,"asn":"AS4760","traffic_gb":3.6}
    ],
    "certificate_anomalies": [
      {"label":"JA3 黑名单","value":28,"percent":39.4,"sample":"771.9d...cb7d","status":"risk"},
      {"label":"自签名证书","value":18,"percent":25.4,"sample":"Self-Signed/CN=*","status":"warn"},
      {"label":"证书过期","value":12,"percent":16.9,"sample":"expired / 2024-…","status":"warn"},
      {"label":"域名不匹配","value":13,"percent":18.3,"sample":"example.com","status":"warn"}
    ],
    "tunnel_trend_unit": "GB",
    "tunnel_trend": [
      {"label":"06-13","value":21},{"label":"06-14","value":38},{"label":"06-15","value":52},{"label":"06-16","value":46},
      {"label":"06-17","value":67},{"label":"06-18","value":58},{"label":"06-19","value":74},{"label":"06-20","value":63}
    ],
    "reuse_paths": [
      {"source":"10.12.8.45","protocol":"SSH","proxy":"跳板机(SG)","destination":"203.0.113.45(US)"},
      {"source":"10.12.6.78","protocol":"TLS","proxy":"代理(US)","destination":"198.51.100.77(US)"},
      {"source":"10.12.9.33","protocol":"SOCKS","proxy":"VPN(NL)","destination":"45.77.34.12(NL)"}
    ],
    "topology_nodes": [
      {"id":"asset-office","label":"办公终端组","detail":"668 资产","x":8.2,"y":22,"tone":"asset","width":108,"height":42,"symbol":"roundRect","icon":"desktop","label_position":"inside"},
      {"id":"asset-server","label":"服务器组","detail":"284 资产","x":8.2,"y":50,"tone":"asset","width":108,"height":42,"symbol":"roundRect","icon":"server","label_position":"inside"},
      {"id":"asset-storage","label":"数据存储","detail":"76 资产","x":8.2,"y":78,"tone":"asset","width":108,"height":42,"symbol":"roundRect","icon":"storage","label_position":"inside"},
      {"id":"probe-01","label":"Probe-01","detail":"10.12.1.11","x":24.4,"y":34,"tone":"probe","width":44,"height":44,"symbol":"circle","icon":"probe","label_position":"bottom"},
      {"id":"probe-02","label":"Probe-02","detail":"10.12.1.12","x":24.4,"y":67,"tone":"probe","width":44,"height":44,"symbol":"circle","icon":"probe","label_position":"bottom"},
      {"id":"risk-845","label":"10.12.8.45","detail":"高风险隧道源","x":41.1,"y":20,"tone":"risk","width":46,"height":46,"symbol":"circle","icon":"user","label_position":"bottom"},
      {"id":"risk-678","label":"10.12.6.78","detail":"高风险隧道源","x":41.1,"y":50,"tone":"risk","width":46,"height":46,"symbol":"circle","icon":"user","label_position":"bottom"},
      {"id":"risk-933","label":"10.12.9.33","detail":"高风险隧道源","x":41.1,"y":80,"tone":"risk","width":46,"height":46,"symbol":"circle","icon":"user","label_position":"bottom"},
      {"id":"protocol-ssh","label":"SSH 隧道","detail":"203 会话","x":58.5,"y":13,"tone":"protocol","width":108,"height":38,"symbol":"roundRect","icon":"protocol","label_position":"inside"},
      {"id":"protocol-tls","label":"TLS 隧道","detail":"165.1M 会话","x":58.5,"y":31,"tone":"protocol","width":108,"height":38,"symbol":"roundRect","icon":"protocol","label_position":"inside"},
      {"id":"protocol-https","label":"HTTPS 隧道","detail":"104.3M 会话","x":58.5,"y":49,"tone":"protocol","width":108,"height":38,"symbol":"roundRect","icon":"protocol","label_position":"inside"},
      {"id":"protocol-rdp","label":"RDP 隧道","detail":"8.6M 会话","x":58.5,"y":67,"tone":"protocol","width":108,"height":38,"symbol":"roundRect","icon":"protocol","label_position":"inside"},
      {"id":"protocol-socks","label":"SOCKS 隧道","detail":"6.2M 会话","x":58.5,"y":85,"tone":"protocol","width":108,"height":38,"symbol":"roundRect","icon":"protocol","label_position":"inside"},
      {"id":"proxy-sg","label":"跳板机","detail":"SG.ASN 45102","x":75.6,"y":34,"tone":"proxy","width":46,"height":46,"symbol":"circle","icon":"gateway","label_position":"bottom"},
      {"id":"proxy-nl","label":"VPN / 中继","detail":"NL.ASN 6830","x":75.6,"y":67,"tone":"proxy","width":46,"height":46,"symbol":"circle","icon":"lock","label_position":"bottom"},
      {"id":"destination-us","label":"美国","detail":"45 端点","x":92,"y":12,"tone":"destination","width":38,"height":38,"symbol":"circle","icon":"global","label_position":"right"},
      {"id":"destination-sg","label":"新加坡","detail":"28 端点","x":92,"y":31,"tone":"destination","width":38,"height":38,"symbol":"circle","icon":"global","label_position":"right"},
      {"id":"destination-hk","label":"香港","detail":"99 端点","x":92,"y":50,"tone":"destination","width":38,"height":38,"symbol":"circle","icon":"global","label_position":"right"},
      {"id":"destination-nl","label":"荷兰","detail":"12 端点","x":92,"y":69,"tone":"destination","width":38,"height":38,"symbol":"circle","icon":"global","label_position":"right"},
      {"id":"destination-de","label":"德国","detail":"9 端点","x":92,"y":88,"tone":"destination","width":38,"height":38,"symbol":"circle","icon":"global","label_position":"right"}
    ],
    "topology_links": [
      {"source":"asset-office","target":"probe-01","value":18,"tone":"info","line_type":"dashed","label":"办公终端组 → Probe-01"},
      {"source":"asset-server","target":"probe-01","value":14,"tone":"info","line_type":"dashed","label":"服务器组 → Probe-01"},
      {"source":"asset-server","target":"probe-02","value":12,"tone":"info","line_type":"dashed","label":"服务器组 → Probe-02"},
      {"source":"asset-storage","target":"probe-02","value":9,"tone":"info","line_type":"dashed","label":"数据存储 → Probe-02"},
      {"source":"probe-01","target":"risk-845","value":20,"tone":"info","line_type":"solid","label":"Probe-01 已确认源"},
      {"source":"probe-01","target":"risk-678","value":16,"tone":"info","line_type":"solid","label":"Probe-01 已确认源"},
      {"source":"probe-02","target":"risk-678","value":15,"tone":"info","line_type":"solid","label":"Probe-02 已确认源"},
      {"source":"probe-02","target":"risk-933","value":13,"tone":"info","line_type":"solid","label":"Probe-02 已确认源"},
      {"source":"risk-845","target":"protocol-ssh","value":22,"tone":"risk","line_type":"dashed","label":"SSH 隧道识别"},
      {"source":"risk-845","target":"protocol-tls","value":18,"tone":"risk","line_type":"dashed","label":"TLS 隧道识别"},
      {"source":"risk-678","target":"protocol-tls","value":17,"tone":"risk","line_type":"dashed","label":"TLS 隧道识别"},
      {"source":"risk-678","target":"protocol-https","value":15,"tone":"risk","line_type":"dashed","label":"HTTPS 隧道识别"},
      {"source":"risk-678","target":"protocol-rdp","value":11,"tone":"risk","line_type":"dashed","label":"RDP 隧道识别"},
      {"source":"risk-933","target":"protocol-rdp","value":10,"tone":"risk","line_type":"dashed","label":"RDP 隧道识别"},
      {"source":"risk-933","target":"protocol-socks","value":8,"tone":"risk","line_type":"dashed","label":"SOCKS 隧道识别"},
      {"source":"protocol-ssh","target":"proxy-sg","value":20,"tone":"ok","line_type":"solid","label":"SSH → 跳板机"},
      {"source":"protocol-tls","target":"proxy-sg","value":18,"tone":"ok","line_type":"solid","label":"TLS → 跳板机"},
      {"source":"protocol-https","target":"proxy-sg","value":16,"tone":"ok","line_type":"solid","label":"HTTPS → 跳板机"},
      {"source":"protocol-rdp","target":"proxy-nl","value":11,"tone":"ok","line_type":"solid","label":"RDP → VPN 中继"},
      {"source":"protocol-socks","target":"proxy-nl","value":9,"tone":"ok","line_type":"solid","label":"SOCKS → VPN 中继"},
      {"source":"proxy-sg","target":"destination-us","value":18,"tone":"purple","line_type":"dashed","label":"跳板机 → 美国"},
      {"source":"proxy-sg","target":"destination-sg","value":14,"tone":"purple","line_type":"dashed","label":"跳板机 → 新加坡"},
      {"source":"proxy-sg","target":"destination-hk","value":12,"tone":"purple","line_type":"dashed","label":"跳板机 → 香港"},
      {"source":"proxy-nl","target":"destination-hk","value":11,"tone":"purple","line_type":"dashed","label":"VPN 中继 → 香港"},
      {"source":"proxy-nl","target":"destination-nl","value":9,"tone":"purple","line_type":"dashed","label":"VPN 中继 → 荷兰"},
      {"source":"proxy-nl","target":"destination-de","value":7,"tone":"purple","line_type":"dashed","label":"VPN 中继 → 德国"}
    ],
    "impact_highlights": [
      {"label":"异常隧道告警","value":"64 条","detail":"当前窗口异常隧道事件","status":"risk","target_signal":"高危用户"},
      {"label":"攻击阶段","value":"横向移动 / 数据外传","detail":"主要攻击阶段","status":"warn","target_signal":"协议族识别"},
      {"label":"关联战役 APT-CN-2026","value":"阶段：横向移动 → 外传","detail":"关联 APT 战役","status":"info","target_signal":"指纹证据"}
    ],
    "evidence_bundle": [
      {"label":"告警证据","complete":64,"total":64,"status":"ok"},
      {"label":"PCAP","complete":132,"total":156,"status":"warn"},
      {"label":"Session","complete":198,"total":204,"status":"ok"},
      {"label":"审计日志","complete":38,"total":38,"status":"ok"},
      {"label":"回溯路径","complete":18,"total":18,"status":"ok"},
      {"label":"资产快照","complete":23,"total":23,"status":"ok"}
    ]
  }$json$::jsonb, 'codex-topic-ui')
ON CONFLICT (simulation_id) DO UPDATE SET
  payload = EXCLUDED.payload, version = EXCLUDED.version, enabled = true,
  updated_at = now(), created_by = EXCLUDED.created_by;

UPDATE topic_panel_simulations
SET payload = jsonb_set(payload, '{events}', (
  SELECT jsonb_agg(jsonb_build_object(
    'event_id', 'TN-20260620-' || lpad(n::text, 4, '0'),
    'ip', CASE WHEN n <= 5
      THEN (ARRAY['10.12.8.45','10.12.6.78','10.12.9.33','10.12.3.67','10.12.2.55'])[n]
      ELSE '10.12.' || ((n * 7) % 48 + 1)::text || '.' || ((n * 13) % 220 + 10)::text END,
    'dst_ip', CASE WHEN n <= 5
      THEN (ARRAY['203.0.113.45(SG)','198.51.100.77(US)','104.16.24.34(US)','45.77.34.12(NL)','23.227.38.65(US)'])[n]
      ELSE (ARRAY['203.0.113.45(SG)','198.51.100.77(US)','104.16.24.34(US)','45.77.34.12(NL)','23.227.38.65(US)'])[((n - 1) % 5) + 1] END,
    'protocol', (ARRAY['SSH','TLS','HTTPS','RDP','SOCKS'])[((n - 1) % 5) + 1],
    'risk', (ARRAY['critical','high','high','medium','medium'])[((n - 1) % 5) + 1],
    'count', CASE WHEN n <= 5 THEN (ARRAY[18,15,10,7,4])[n] ELSE 24 - (n % 17) END,
    'total_bytes', CASE WHEN n <= 5
      THEN (ARRAY[19542101197::bigint,15784004813::bigint,10307921510::bigint,7301444403::bigint,4617089843::bigint])[n]
      ELSE 12800000000 - (n::bigint * 27000000) END,
    'last_seen', 1781920800000 - (n::bigint * 1800000),
    'evidence_type', (ARRAY['PCAP','Session','证书','PCAP','回溯路径'])[((n - 1) % 5) + 1],
    'time_window', CASE WHEN n <= 5
      THEN (ARRAY['06-19 22:14:32 ~ 22:18:47','06-19 21:05:11 ~ 21:15:09','06-18 08:32:00 ~ 08:42:15','06-18 17:26:55 ~ 17:32:20','06-17 23:10:21 ~ 23:22:47'])[n]
      ELSE to_char(timestamp '2026-06-20 03:45:00' - (n || ' hours')::interval, 'MM-DD HH24:MI:SS') END,
    'phase', (ARRAY['横向移动','持久化','数据外传','命令控制'])[((n - 1) % 4) + 1],
    'risk_action', 'PCAP / Session / 证书 / 回溯路径 / 审计日志'
  ) ORDER BY n)
  FROM generate_series(1, 128) AS n
), true), updated_at = now()
WHERE simulation_id = 'topic-tunnel-ui-v1';

UPDATE topic_panel_simulations
SET enabled = false, updated_at = now()
WHERE tenant_id = '*' AND topic = 'exfil' AND simulation_id <> 'topic-exfil-ui-v2' AND enabled;

INSERT INTO topic_panel_simulations
  (simulation_id, tenant_id, topic, version, enabled, payload, created_by)
VALUES
  ('topic-exfil-ui-v2', '*', 'exfil', 'ui-suite-gpt-v2', true, $json$
  {
    "presentation": {
      "topic_id": "exfil-20260620-01",
      "site": "主校区",
      "asset_group": "科研文件服务 / 办公终端",
      "ip_range": "10.14.0.0/16",
      "protocols": "HTTPS / S3 / WebDAV / DNS",
      "time_window_label": "近24小时",
      "rule_version": "v3.2",
      "model_version": "v2.0",
      "report_title": "数据外传专题分析报告",
      "report_generated_at": "2026-06-20 10:00",
      "report_scope": "主校区科研文件服务与办公终端",
      "report_conclusion": "识别 112 条外传路径，32 个跨境目的地需持续处置。"
    },
    "summary": {
      "alert_count": 64,
      "path_count": 112,
      "source_count": 23,
      "high_risk_sources": 23,
      "destination_count": 87,
      "sensitive_type_count": 12,
      "peak_upload_gbps": 38.6,
      "cross_border_destinations": 32,
      "evidence_completeness": 62,
      "path_confidence": 72,
      "report_confidence": 62,
      "reportable_count": 7,
      "pending_evidence_count": 3,
      "open_risk_count": 18,
      "session_count": 128,
      "upload_bytes": 482000000000,
      "total_events": 128
    },
    "top_sources": [
      {"src_ip":"10.14.12.31","session_count":31,"upload_bytes":118000000000,"total_bytes":126000000000,"dst_count":18,"last_seen":1781920800000,"risk":"critical"},
      {"src_ip":"10.14.33.77","session_count":26,"upload_bytes":94000000000,"total_bytes":101000000000,"dst_count":15,"last_seen":1781917200000,"risk":"high"},
      {"src_ip":"10.14.9.114","session_count":22,"upload_bytes":76000000000,"total_bytes":83000000000,"dst_count":13,"last_seen":1781913600000,"risk":"high"},
      {"src_ip":"10.14.45.18","session_count":18,"upload_bytes":62000000000,"total_bytes":69000000000,"dst_count":11,"last_seen":1781910000000,"risk":"medium"},
      {"src_ip":"10.14.6.205","session_count":16,"upload_bytes":51000000000,"total_bytes":57000000000,"dst_count":9,"last_seen":1781906400000,"risk":"medium"}
    ],
    "destinations": [
      {"dst_ip":"52.216.43.18","region":"美国","asn":"AS16509","session_count":29,"upload_bytes":104000000000,"total_bytes":112000000000,"src_count":8,"last_seen":1781920800000,"risk":"critical"},
      {"dst_ip":"185.199.110.153","region":"美国","asn":"AS54113","session_count":24,"upload_bytes":87000000000,"total_bytes":93000000000,"src_count":7,"last_seen":1781917200000,"risk":"high"},
      {"dst_ip":"103.21.244.16","region":"新加坡","asn":"AS13335","session_count":21,"upload_bytes":71000000000,"total_bytes":78000000000,"src_count":6,"last_seen":1781913600000,"risk":"high"},
      {"dst_ip":"46.101.77.14","region":"德国","asn":"AS14061","session_count":18,"upload_bytes":58000000000,"total_bytes":64000000000,"src_count":5,"last_seen":1781910000000,"risk":"medium"},
      {"dst_ip":"139.162.81.42","region":"日本","asn":"AS63949","session_count":15,"upload_bytes":44000000000,"total_bytes":49000000000,"src_count":4,"last_seen":1781906400000,"risk":"medium"}
    ],
    "risk_types": [
      {"type":"源代码","count":31,"severity":"critical","total_bytes":118000000000},
      {"type":"科研文档","count":27,"severity":"high","total_bytes":96000000000},
      {"type":"个人信息","count":22,"severity":"high","total_bytes":81000000000},
      {"type":"凭证密钥","count":18,"severity":"critical","total_bytes":69000000000},
      {"type":"财务数据","count":14,"severity":"medium","total_bytes":52000000000}
    ],
    "account_service_distribution": [
      {"label":"svc_backup","type":"service_account","count":21},
      {"label":"svc_share","type":"service_account","count":16},
      {"label":"user_test01","type":"user_account","count":12},
      {"label":"gitlab-ci","type":"service","count":9},
      {"label":"anonymous","type":"anonymous_account","count":7}
    ],
    "paths": [
      {"event_id":"EXF-20260620-001","src_ip":"10.14.12.31","dst_ip":"52.216.43.18","dst_region":"美国","dst_port":443,"protocol":"HTTPS","data_type":"源代码","session_count":31,"upload_bytes":118000000000,"last_seen":1781920800000,"risk":"critical"},
      {"event_id":"EXF-20260620-002","src_ip":"10.14.33.77","dst_ip":"185.199.110.153","dst_region":"美国","dst_port":443,"protocol":"WebDAV","data_type":"科研文档","session_count":26,"upload_bytes":94000000000,"last_seen":1781917200000,"risk":"high"},
      {"event_id":"EXF-20260620-003","src_ip":"10.14.9.114","dst_ip":"103.21.244.16","dst_region":"新加坡","dst_port":443,"protocol":"S3","data_type":"凭证密钥","session_count":22,"upload_bytes":76000000000,"last_seen":1781913600000,"risk":"high"},
      {"event_id":"EXF-20260620-004","src_ip":"10.14.45.18","dst_ip":"46.101.77.14","dst_region":"德国","dst_port":53,"protocol":"DNS","data_type":"个人信息","session_count":18,"upload_bytes":62000000000,"last_seen":1781910000000,"risk":"medium"},
      {"event_id":"EXF-20260620-005","src_ip":"10.14.6.205","dst_ip":"139.162.81.42","dst_region":"日本","dst_port":443,"protocol":"HTTPS","data_type":"财务数据","session_count":16,"upload_bytes":51000000000,"last_seen":1781906400000,"risk":"medium"},
      {"event_id":"EXF-20260620-006","src_ip":"10.14.22.88","dst_ip":"91.189.91.49","dst_region":"英国","dst_port":443,"protocol":"WebDAV","data_type":"设计文档","session_count":15,"upload_bytes":43000000000,"last_seen":1781902800000,"risk":"high"}
    ],
    "trend": [
      {"bucket_start":1781834400000,"destination_count":12,"large_upload_sessions":7,"long_lived_sessions":4,"non_standard_port_sessions":2,"encrypted_sessions":11},
      {"bucket_start":1781856000000,"destination_count":18,"large_upload_sessions":11,"long_lived_sessions":6,"non_standard_port_sessions":4,"encrypted_sessions":16},
      {"bucket_start":1781877600000,"destination_count":27,"large_upload_sessions":16,"long_lived_sessions":9,"non_standard_port_sessions":6,"encrypted_sessions":24},
      {"bucket_start":1781899200000,"destination_count":39,"large_upload_sessions":24,"long_lived_sessions":13,"non_standard_port_sessions":8,"encrypted_sessions":35},
      {"bucket_start":1781920800000,"destination_count":32,"large_upload_sessions":21,"long_lived_sessions":12,"non_standard_port_sessions":7,"encrypted_sessions":29}
    ],
    "evidence_bundle": [
      {"label":"告警证据","complete":64,"total":64,"status":"ok"},
      {"label":"PCAP","complete":39,"total":64,"status":"warn"},
      {"label":"Session","complete":58,"total":64,"status":"ok"},
      {"label":"审计日志","complete":27,"total":64,"status":"warn"},
      {"label":"回溯路径","complete":41,"total":64,"status":"warn"},
      {"label":"资产快照","complete":35,"total":64,"status":"warn"}
    ],
    "topology_nodes": [
      {"id":"ex-src-1","label":"10.14.48.219","detail":"科研终端 / 31 会话","x":7,"y":10,"tone":"asset","width":96,"height":56,"icon":"desktop"},
      {"id":"ex-src-2","label":"10.14.47.208","detail":"办公终端 / 30 会话","x":7,"y":30,"tone":"asset","width":96,"height":56,"icon":"desktop"},
      {"id":"ex-src-3","label":"10.14.46.197","detail":"研发终端 / 29 会话","x":7,"y":50,"tone":"asset","width":96,"height":56,"icon":"desktop"},
      {"id":"ex-src-4","label":"10.14.45.186","detail":"财务终端 / 28 会话","x":7,"y":70,"tone":"asset","width":96,"height":56,"icon":"desktop"},
      {"id":"ex-src-5","label":"10.14.44.175","detail":"文件终端 / 27 会话","x":7,"y":90,"tone":"asset","width":96,"height":56,"icon":"desktop"},
      {"id":"ex-type-1","label":"源代码","detail":"敏感数据 / 109.5 GB","x":29,"y":10,"tone":"protocol","width":102,"height":56,"icon":"storage"},
      {"id":"ex-type-2","label":"科研文档","detail":"敏感数据 / 109.2 GB","x":29,"y":30,"tone":"protocol","width":102,"height":56,"icon":"storage"},
      {"id":"ex-type-3","label":"凭证密钥","detail":"敏感数据 / 108.8 GB","x":29,"y":50,"tone":"protocol","width":102,"height":56,"icon":"lock"},
      {"id":"ex-type-4","label":"个人信息","detail":"敏感数据 / 108.4 GB","x":29,"y":70,"tone":"protocol","width":102,"height":56,"icon":"storage"},
      {"id":"ex-type-5","label":"财务数据","detail":"敏感数据 / 108.1 GB","x":29,"y":90,"tone":"protocol","width":102,"height":56,"icon":"storage"},
      {"id":"ex-proxy-1","label":"风险源代码","detail":"代理转发 / 高危","x":51,"y":10,"tone":"proxy","width":108,"height":56,"icon":"gateway"},
      {"id":"ex-proxy-2","label":"风险科研文档","detail":"WebDAV / 高危","x":51,"y":30,"tone":"proxy","width":108,"height":56,"icon":"gateway"},
      {"id":"ex-proxy-3","label":"风险凭证密钥","detail":"S3 / 高危","x":51,"y":50,"tone":"proxy","width":108,"height":56,"icon":"gateway"},
      {"id":"ex-proxy-4","label":"风险个人信息","detail":"DNS / 中危","x":51,"y":70,"tone":"proxy","width":108,"height":56,"icon":"gateway"},
      {"id":"ex-proxy-5","label":"风险财务数据","detail":"HTTPS / 中危","x":51,"y":90,"tone":"proxy","width":108,"height":56,"icon":"gateway"},
      {"id":"ex-dst-1","label":"52.216.43.18","detail":"美国 / AS16509","x":74,"y":10,"tone":"destination","width":104,"height":56,"icon":"global"},
      {"id":"ex-dst-2","label":"185.199.110.153","detail":"美国 / AS54113","x":74,"y":30,"tone":"destination","width":104,"height":56,"icon":"global"},
      {"id":"ex-dst-3","label":"103.21.244.16","detail":"新加坡 / AS13335","x":74,"y":50,"tone":"destination","width":104,"height":56,"icon":"global"},
      {"id":"ex-dst-4","label":"46.101.77.14","detail":"德国 / AS14061","x":74,"y":70,"tone":"destination","width":104,"height":56,"icon":"global"},
      {"id":"ex-dst-5","label":"139.162.81.42","detail":"日本 / AS63949","x":74,"y":90,"tone":"destination","width":104,"height":56,"icon":"global"},
      {"id":"ex-risk-1","label":"路径-01","detail":"高危 / 未闭环","x":94,"y":10,"tone":"risk","width":88,"height":56,"icon":"global"},
      {"id":"ex-risk-2","label":"路径-02","detail":"高危 / 未闭环","x":94,"y":30,"tone":"risk","width":88,"height":56,"icon":"global"},
      {"id":"ex-risk-3","label":"路径-03","detail":"高危 / 待补证","x":94,"y":50,"tone":"risk","width":88,"height":56,"icon":"global"},
      {"id":"ex-risk-4","label":"路径-04","detail":"中危 / 处置中","x":94,"y":70,"tone":"risk","width":88,"height":56,"icon":"global"},
      {"id":"ex-risk-5","label":"路径-05","detail":"中危 / 观察中","x":94,"y":90,"tone":"risk","width":88,"height":56,"icon":"global"}
    ],
    "topology_links": [
      {"source":"ex-src-1","target":"ex-type-1","value":109.5,"tone":"info","line_type":"solid","width":10,"label":"源代码上传 / 109.5 GB"},
      {"source":"ex-src-2","target":"ex-type-2","value":109.2,"tone":"info","line_type":"solid","width":9,"label":"科研文档上传 / 109.2 GB"},
      {"source":"ex-src-3","target":"ex-type-3","value":108.8,"tone":"info","line_type":"solid","width":8,"label":"凭证密钥上传 / 108.8 GB"},
      {"source":"ex-src-4","target":"ex-type-4","value":108.4,"tone":"info","line_type":"solid","width":7,"label":"个人信息上传 / 108.4 GB"},
      {"source":"ex-src-5","target":"ex-type-5","value":108.1,"tone":"info","line_type":"solid","width":6,"label":"财务数据上传 / 108.1 GB"},
      {"source":"ex-type-1","target":"ex-proxy-1","value":31,"tone":"ok","line_type":"solid","width":10,"label":"文件服务确认关系"},
      {"source":"ex-type-2","target":"ex-proxy-2","value":30,"tone":"ok","line_type":"solid","width":9,"label":"文件服务确认关系"},
      {"source":"ex-type-3","target":"ex-proxy-3","value":29,"tone":"ok","line_type":"solid","width":8,"label":"对象存储确认关系"},
      {"source":"ex-type-4","target":"ex-proxy-4","value":28,"tone":"ok","line_type":"solid","width":7,"label":"DNS 隧道确认关系"},
      {"source":"ex-type-5","target":"ex-proxy-5","value":27,"tone":"ok","line_type":"solid","width":6,"label":"HTTPS 确认关系"},
      {"source":"ex-proxy-1","target":"ex-dst-1","value":31,"tone":"warn","line_type":"solid","width":10,"label":"跨境中转 / 美国"},
      {"source":"ex-proxy-2","target":"ex-dst-2","value":30,"tone":"warn","line_type":"solid","width":9,"label":"跨境中转 / 美国"},
      {"source":"ex-proxy-3","target":"ex-dst-3","value":29,"tone":"warn","line_type":"solid","width":8,"label":"跨境中转 / 新加坡"},
      {"source":"ex-proxy-4","target":"ex-dst-4","value":28,"tone":"warn","line_type":"solid","width":7,"label":"跨境中转 / 德国"},
      {"source":"ex-proxy-5","target":"ex-dst-5","value":27,"tone":"warn","line_type":"solid","width":6,"label":"跨境中转 / 日本"},
      {"source":"ex-dst-1","target":"ex-risk-1","value":31,"tone":"purple","line_type":"dashed","width":4,"label":"高危路径推断"},
      {"source":"ex-dst-2","target":"ex-risk-2","value":30,"tone":"purple","line_type":"dashed","width":4,"label":"高危路径推断"},
      {"source":"ex-dst-3","target":"ex-risk-3","value":29,"tone":"purple","line_type":"dashed","width":4,"label":"待补证路径推断"},
      {"source":"ex-dst-4","target":"ex-risk-4","value":28,"tone":"purple","line_type":"dashed","width":3,"label":"中危路径推断"},
      {"source":"ex-dst-5","target":"ex-risk-5","value":27,"tone":"purple","line_type":"dashed","width":3,"label":"观察路径推断"}
    ]
  }$json$::jsonb, 'codex-topic-ui')
ON CONFLICT (simulation_id) DO UPDATE SET
  payload = EXCLUDED.payload, version = EXCLUDED.version, enabled = true,
  updated_at = now(), created_by = EXCLUDED.created_by;

UPDATE topic_panel_simulations
SET payload = jsonb_set(payload, '{events}', (
  SELECT jsonb_agg(jsonb_build_object(
    'event_id', 'EXF-20260620-' || lpad(n::text, 3, '0'),
    'src_ip', '10.14.' || ((n * 5) % 48 + 1)::text || '.' || ((n * 11) % 220 + 10)::text,
    'dst_ip', (ARRAY['52.216.43.18','185.199.110.153','103.21.244.16','46.101.77.14','139.162.81.42','91.189.91.49'])[((n - 1) % 6) + 1],
    'dst_region', (ARRAY['美国','美国','新加坡','德国','日本','英国'])[((n - 1) % 6) + 1],
    'dst_port', (ARRAY[443,443,443,53,443,443])[((n - 1) % 6) + 1],
    'protocol', (ARRAY['HTTPS','WebDAV','S3','DNS'])[((n - 1) % 4) + 1],
    'data_type', (ARRAY['源代码','科研文档','凭证密钥','个人信息','财务数据','设计文档'])[((n - 1) % 6) + 1],
    'session_count', 32 - (n % 19),
    'upload_bytes', 118000000000 - (n::bigint * 390000000),
    'last_seen', 1781920800000 - (n::bigint * 900000),
    'risk', (ARRAY['critical','high','high','medium'])[((n - 1) % 4) + 1]
  ) ORDER BY n)
  FROM generate_series(1, 128) AS n
), true), updated_at = now()
WHERE simulation_id = 'topic-exfil-ui-v2';

UPDATE topic_panel_simulations
SET enabled = false, updated_at = now()
WHERE tenant_id = '*' AND topic = 'apt' AND simulation_id <> 'topic-apt-ui-v2' AND enabled;

INSERT INTO topic_panel_simulations
  (simulation_id, tenant_id, topic, version, enabled, payload, created_by)
VALUES
  ('topic-apt-ui-v2', '*', 'apt', 'ui-suite-gpt-v2', true, $json$
  {
    "presentation": {
      "topic_id": "campaign-20260620-apt01",
      "site": "主校区",
      "asset_group": "办公终端 / 数据中心",
      "ip_range": "侦察 / 初始访问 / 执行 / 持久化 / 横向移动 / 外传",
      "protocols": "ATT&CK 战役关联",
      "time_window_label": "近30天",
      "rule_version": "v2.4",
      "model_version": "v1.8",
      "report_title": "APT 战役关联专题报告",
      "report_generated_at": "2026-06-20 10:00",
      "report_scope": "主校区办公终端与数据中心",
      "report_conclusion": "关联 7 个战役集，5/7 攻击阶段已覆盖，闭环率 68%。"
    },
    "summary": {
      "campaign_count": 7,
      "cluster_density": 0.72,
      "phase_coverage_done": 5,
      "phase_coverage_total": 7,
      "entity_count": 46,
      "lateral_move_links": 23,
      "persistence_signals": 18,
      "exfil_evidence_count": 32,
      "closure_rate": 68,
      "report_confidence": 62,
      "evidence_completeness": 62,
      "high_risk_count": 3,
      "alert_count": 156,
      "reportable_count": 7,
      "pending_evidence_count": 3,
      "open_risk_count": 18,
      "metric_scope": "listed_campaigns",
      "total_events": 156
    },
    "phase_distribution": {"侦察":7,"初始访问":6,"执行":6,"持久化":5,"横向移动":5},
    "campaigns": [
      {"campaign_id":"APT-CN-2026","campaign_type":"targeted","score":0.94,"status":"active","activity_status":"active","attack_phases":["初始访问","执行","持久化","横向移动","数据外传"],"entities":["10.20.11.31","研发文件服务器","域控-01"],"alerts":["ALT-001","ALT-002","ALT-003","ALT-004","ALT-005","ALT-006","ALT-007","ALT-008","ALT-009","ALT-010","ALT-011","ALT-012"],"ts_start":1779415200000,"ts_end":1781920800000},
      {"campaign_id":"TEMP.HAWK","campaign_type":"espionage","score":0.87,"status":"investigating","activity_status":"active","attack_phases":["侦察","初始访问","执行","横向移动"],"entities":["10.20.18.77","邮件网关","数据库-02"],"alerts":["ALT-021","ALT-022","ALT-023","ALT-024","ALT-025","ALT-026","ALT-027","ALT-028","ALT-029"],"ts_start":1779847200000,"ts_end":1781917200000},
      {"campaign_id":"UNKNOWN-07","campaign_type":"unknown","score":0.79,"status":"open","activity_status":"active","attack_phases":["侦察","执行","持久化"],"entities":["10.20.33.118","堡垒机-03","API网关"],"alerts":["ALT-031","ALT-032","ALT-033","ALT-034","ALT-035","ALT-036","ALT-037"],"ts_start":1780279200000,"ts_end":1781913600000},
      {"campaign_id":"SILVER-FOX","campaign_type":"financial","score":0.68,"status":"contained","activity_status":"contained","attack_phases":["初始访问","执行","持久化"],"entities":["10.20.9.206","财务终端"],"alerts":["ALT-041","ALT-042","ALT-043","ALT-044","ALT-045"],"ts_start":1780711200000,"ts_end":1781910000000},
      {"campaign_id":"CLOUD-DRIFT","campaign_type":"cloud","score":0.63,"status":"monitoring","activity_status":"monitoring","attack_phases":["侦察","执行"],"entities":["10.20.42.51","K8s-生产集群"],"alerts":["ALT-051","ALT-052","ALT-053","ALT-054"],"ts_start":1781143200000,"ts_end":1781906400000},
      {"campaign_id":"RED-LOTUS","campaign_type":"targeted","score":0.58,"status":"resolved","activity_status":"closed","attack_phases":["初始访问","执行"],"entities":["10.20.6.143","研发终端"],"alerts":["ALT-061","ALT-062","ALT-063"],"ts_start":1781402400000,"ts_end":1781902800000},
      {"campaign_id":"NIGHT-OWL","campaign_type":"unknown","score":0.51,"status":"closed","activity_status":"closed","attack_phases":["侦察"],"entities":["10.20.27.62","VPN网关"],"alerts":["ALT-071","ALT-072"],"ts_start":1781661600000,"ts_end":1781899200000}
    ],
    "iocs": [
      {"value":"185.199.108.153","type":"IP","campaign":"APT-CN-2026","hits":31,"first_seen":1779588600000,"last_seen":1781918400000},
      {"value":"update-sync.live","type":"Domain","campaign":"TEMP.HAWK","hits":24,"first_seen":1779859800000,"last_seen":1781914200000},
      {"value":"4f6d8c7a...9e12","type":"SHA256","campaign":"UNKNOWN-07","hits":18,"first_seen":1780122600000,"last_seen":1781909400000},
      {"value":"91.215.85.17","type":"IP","campaign":"APT-CN-2026","hits":16,"first_seen":1780291800000,"last_seen":1781904600000},
      {"value":"cdn-auth.work","type":"Domain","campaign":"TEMP.HAWK","hits":12,"first_seen":1780726800000,"last_seen":1781899200000}
    ],
    "response": {"closed":68,"processing":18,"open":14,"total":100},
    "evidence_bundle": [
      {"label":"Campaigns","complete":7,"total":7,"status":"ok"},
      {"label":"ATT&CK 阶段","complete":5,"total":7,"status":"warn"},
      {"label":"Entity Graph","complete":39,"total":46,"status":"warn"},
      {"label":"Evidence Bundle","complete":97,"total":156,"status":"warn"},
      {"label":"处置记录","complete":106,"total":156,"status":"warn"},
      {"label":"审计日志","complete":82,"total":156,"status":"risk"}
    ],
    "topology_nodes": [
      {"id":"campaign-0","label":"APT-CN-2026","detail":"高置信度 27 / 事件 156","x":9,"y":20,"tone":"risk","width":112,"height":68,"icon":"campaign"},
      {"id":"campaign-1","label":"TEMP.HAWK","detail":"中置信度 19 / 事件 98","x":9,"y":50,"tone":"risk","width":112,"height":68,"icon":"campaign"},
      {"id":"campaign-2","label":"UNKNOWN-07","detail":"低置信度 12 / 事件 64","x":9,"y":80,"tone":"risk","width":112,"height":68,"icon":"campaign"},
      {"id":"phase-initial","label":"初始访问","detail":"7","x":27,"y":17,"tone":"asset","width":88,"height":52,"icon":"initial"},
      {"id":"phase-execute","label":"执行","detail":"7","x":42.5,"y":17,"tone":"asset","width":88,"height":52,"icon":"execute"},
      {"id":"phase-persist","label":"持久化","detail":"6","x":58,"y":17,"tone":"proxy","width":88,"height":52,"icon":"persist"},
      {"id":"phase-evasion","label":"防御规避","detail":"6","x":27,"y":46,"tone":"proxy","width":88,"height":52,"icon":"evasion"},
      {"id":"phase-credential","label":"凭证访问","detail":"5","x":42.5,"y":46,"tone":"asset","width":88,"height":52,"icon":"credential"},
      {"id":"phase-discovery","label":"发现","detail":"6","x":58,"y":46,"tone":"asset","width":88,"height":52,"icon":"discovery"},
      {"id":"phase-lateral","label":"横向移动","detail":"23 链路","x":27,"y":75,"tone":"proxy","width":88,"height":52,"icon":"lateral"},
      {"id":"phase-c2","label":"命令控制","detail":"8","x":42.5,"y":75,"tone":"risk","width":88,"height":52,"icon":"c2"},
      {"id":"phase-exfil","label":"数据外传","detail":"32 证据","x":58,"y":75,"tone":"proxy","width":88,"height":52,"icon":"exfil"},
      {"id":"evidence-domain","label":"C2 域名","detail":"c2-apt[.]top / 命中 8","x":74.5,"y":11,"tone":"risk","width":104,"height":52,"icon":"c2"},
      {"id":"evidence-ip","label":"C2 IP","detail":"185.199.111.153 / 命中 6","x":74.5,"y":27,"tone":"risk","width":104,"height":52,"icon":"global"},
      {"id":"evidence-site","label":"外联站点","detail":"195.110.10.77 / 命中 5","x":74.5,"y":43,"tone":"risk","width":104,"height":52,"icon":"exfil"},
      {"id":"evidence-pcap","label":"PCAP","detail":"56 证据","x":74.5,"y":59,"tone":"warn","width":104,"height":52,"icon":"evidence"},
      {"id":"evidence-session","label":"Session","detail":"72 会话","x":74.5,"y":75,"tone":"warn","width":104,"height":52,"icon":"evidence"},
      {"id":"evidence-audit","label":"日志 / 审计","detail":"134 条","x":74.5,"y":91,"tone":"warn","width":104,"height":52,"icon":"audit"},
      {"id":"asset-service","label":"资产 / 服务","detail":"PowerShell / 命中 18","x":92,"y":14,"tone":"protocol","width":96,"height":54,"icon":"server"},
      {"id":"asset-evidence","label":"关键证据","detail":"翻译服务 / 命中 14","x":92,"y":38,"tone":"protocol","width":96,"height":54,"icon":"evidence"},
      {"id":"asset-account","label":"账号","detail":"CORP.LOCAL / 命中 27","x":92,"y":62,"tone":"protocol","width":96,"height":54,"icon":"user"},
      {"id":"asset-group","label":"资产 / 组","detail":"办公终端 / 命中 32","x":92,"y":86,"tone":"protocol","width":96,"height":54,"icon":"desktop"}
    ],
    "topology_links": [
      {"source":"campaign-0","target":"phase-initial","value":12,"tone":"risk","line_type":"dashed","width":2.0,"curveness":0.02,"label":"APT-CN-2026 初始访问"},
      {"source":"campaign-1","target":"phase-evasion","value":9,"tone":"purple","line_type":"dashed","width":1.8,"label":"TEMP.HAWK 防御规避"},
      {"source":"campaign-2","target":"phase-lateral","value":7,"tone":"warn","line_type":"dashed","width":1.8,"label":"UNKNOWN-07 横向移动"},
      {"source":"phase-initial","target":"phase-execute","value":7,"tone":"info","line_type":"dashed","width":2.0,"label":"阶段确认"},
      {"source":"phase-execute","target":"phase-persist","value":6,"tone":"info","line_type":"dashed","width":2.0,"label":"阶段确认"},
      {"source":"phase-evasion","target":"phase-credential","value":5,"tone":"info","line_type":"dashed","width":2.0,"label":"阶段确认"},
      {"source":"phase-credential","target":"phase-discovery","value":6,"tone":"info","line_type":"dashed","width":2.0,"label":"阶段确认"},
      {"source":"phase-lateral","target":"phase-c2","value":8,"tone":"warn","line_type":"dashed","width":2.0,"label":"横向移动到 C2"},
      {"source":"phase-c2","target":"phase-exfil","value":8,"tone":"risk","line_type":"dashed","width":2.0,"label":"C2 到数据外传"},
      {"source":"phase-persist","target":"evidence-domain","value":8,"tone":"risk","line_type":"dashed","width":1.8,"label":"C2 域名证据"},
      {"source":"phase-discovery","target":"evidence-ip","value":6,"tone":"risk","line_type":"dashed","width":1.8,"label":"C2 IP 证据"},
      {"source":"phase-c2","target":"evidence-site","value":5,"tone":"risk","line_type":"dashed","width":1.8,"label":"外联站点证据"},
      {"source":"phase-exfil","target":"evidence-pcap","value":56,"tone":"warn","line_type":"dashed","width":2.0,"label":"PCAP 证据"},
      {"source":"phase-exfil","target":"evidence-session","value":72,"tone":"warn","line_type":"dashed","width":2.0,"label":"Session 证据"},
      {"source":"phase-exfil","target":"evidence-audit","value":134,"tone":"warn","line_type":"dashed","width":2.0,"label":"日志审计证据"},
      {"source":"evidence-audit","target":"asset-group","value":32,"tone":"ok","line_type":"dashed","width":1.7,"curveness":-0.04,"label":"审计资产归因"},
      {"source":"evidence-pcap","target":"asset-account","value":27,"tone":"ok","line_type":"dashed","width":1.7,"curveness":-0.04,"label":"PCAP 账号归因"},
      {"source":"evidence-domain","target":"asset-service","value":18,"tone":"ok","line_type":"dashed","width":1.7,"curveness":0.04,"label":"域名关联服务"},
      {"source":"evidence-site","target":"asset-evidence","value":14,"tone":"ok","line_type":"dashed","width":1.7,"curveness":0.04,"label":"外联证据归因"}
    ]
  }$json$::jsonb, 'codex-topic-ui')
ON CONFLICT (simulation_id) DO UPDATE SET
  payload = EXCLUDED.payload, version = EXCLUDED.version, enabled = true,
  updated_at = now(), created_by = EXCLUDED.created_by;

UPDATE topic_panel_simulations
SET payload = jsonb_set(payload, '{events}', (
  SELECT jsonb_agg(jsonb_build_object(
    'campaign_id', (ARRAY['APT-CN-2026','TEMP.HAWK','UNKNOWN-07','SILVER-FOX','CLOUD-DRIFT','RED-LOTUS','NIGHT-OWL'])[((n - 1) % 7) + 1],
    'event_id', 'APT-EVT-20260620-' || lpad(n::text, 3, '0'),
    'campaign_type', (ARRAY['targeted','espionage','unknown','financial','cloud'])[((n - 1) % 5) + 1],
    'score', 0.95 - ((n % 8) * 0.05),
    'status', (ARRAY['active','investigating','open','contained','monitoring','resolved','closed'])[((n - 1) % 7) + 1],
    'activity_status', (ARRAY['active','active','active','contained','monitoring','closed','closed'])[((n - 1) % 7) + 1],
    'attack_phases', jsonb_build_array((ARRAY['侦察','初始访问','执行','持久化','横向移动'])[((n - 1) % 5) + 1]),
    'entities', jsonb_build_array('10.20.' || ((n * 3) % 48 + 1)::text || '.' || ((n * 17) % 220 + 10)::text),
    'alerts', jsonb_build_array('ALT-' || lpad(n::text, 3, '0')),
    'ts_start', 1779415200000 + (n::bigint * 14400000),
    'ts_end', 1781920800000 - (n::bigint * 600000)
  ) ORDER BY n)
  FROM generate_series(1, 156) AS n
), true), updated_at = now()
WHERE simulation_id = 'topic-apt-ui-v2';

COMMIT;
