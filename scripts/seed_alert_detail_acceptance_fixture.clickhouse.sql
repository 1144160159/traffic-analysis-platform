-- Idempotent alert-detail fixture. The data uses the real ClickHouse alert and
-- evidence schemas, participates in normal API reads, and expires under the
-- existing 30-day TTL.

INSERT INTO traffic.alerts_local (
  tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
  feature_set_id, src_ip, dst_ip, src_port, dst_port, protocol, protocol_name,
  alert_type, labels, severity, score, status, assignee, evidence_ids,
  arkime_session_link, feedback_label, feedback_count, first_seen, last_seen,
  count, model_version, rule_version, state_version, event_id, kafka_ts,
  flink_out_ts, created_at, updated_at
)
SELECT
  'default',
  'alert-detail-accept-r802',
  'alert-detail-accept-r802-fingerprint',
  '1:alert-detail-r802',
  'session-alert-detail-r802',
  'campaign-alert-detail-r802',
  'flow-v4',
  '172.16.5.10',
  '185.22.14.9',
  51514,
  443,
  6,
  'TCP',
  'C2_Tunnel_v3',
  ['C2通信', '横向移动', '可疑外联'],
  'critical',
  toFloat32(0.92),
  'triage',
  'sec_analyst',
  [
    'alert-detail-r802-pcap',
    'alert-detail-r802-session',
    'alert-detail-r802-session-2',
    'alert-detail-r802-log',
    'alert-detail-r802-graph',
    'alert-detail-r802-hash'
  ],
  '/sessions/session-alert-detail-r802',
  'TP',
  1,
  toUnixTimestamp64Milli(now64(3) - INTERVAL 8 MINUTE),
  toUnixTimestamp64Milli(now64(3) - INTERVAL 2 MINUTE),
  6,
  'C2-Tunnel-Detector-v3',
  'C2_Tunnel_v3',
  toUInt64(toUnixTimestamp64Milli(now64(3))),
  'event-alert-detail-r802',
  toUnixTimestamp64Milli(now64(3) - INTERVAL 115 SECOND),
  toUnixTimestamp64Milli(now64(3) - INTERVAL 114 SECOND),
  toUnixTimestamp64Milli(now64(3) - INTERVAL 8 MINUTE),
  toUnixTimestamp64Milli(now64(3))
WHERE NOT EXISTS (
  SELECT 1
  FROM traffic.alerts_latest FINAL
  WHERE tenant_id = 'default' AND alert_id = 'alert-detail-accept-r802'
);

SET mutations_sync = 1;

ALTER TABLE traffic.evidence_local
DELETE WHERE tenant_id = 'default' AND alert_id = 'alert-detail-accept-r802';

INSERT INTO traffic.evidence_local (
  tenant_id, evidence_id, alert_id, ts, type, summary, metrics_json,
  snippet_ref_json, arkime_link, confidence, event_id, ingest_ts,
  visualization_url
)
SELECT
  'default',
  tupleElement(item, 1),
  'alert-detail-accept-r802',
  toUnixTimestamp64Milli(now64(3) - toIntervalSecond(90 - number * 7)),
  tupleElement(item, 2),
  tupleElement(item, 3),
  tupleElement(item, 4),
  tupleElement(item, 5),
  tupleElement(item, 6),
  toFloat32(tupleElement(item, 7)),
  concat('event-', tupleElement(item, 1)),
  toUnixTimestamp64Milli(now64(3)),
  tupleElement(item, 8)
FROM (
  SELECT
    number,
    arrayElement([
      ('alert-detail-r802-pcap', 'PCAP', 'TLS over HTTP 可疑隧道流量切片，SHA256 已校验', '{"size":"94 B","sha256":"afa4e71d54a49563a23229b7c736b640","object_path":"minio://traffic-evidence/alerts/r802/c2-tunnel.pcap"}', '{"bucket":"traffic-evidence","object":"alerts/r802/c2-tunnel.pcap"}', '/sessions/session-alert-detail-r802', 0.99, '/forensics?evidence=alert-detail-r802-pcap'),
      ('alert-detail-r802-session', 'Session', '异常长连接，双向持续传输且 SNI 缺失', '{"size":"132 B","duration":"12m38s","object_path":"minio://traffic-evidence/alerts/r802/session-primary.json"}', '{"bucket":"traffic-evidence","object":"alerts/r802/session-primary.json","session_id":"session-alert-detail-r802"}', '/sessions/session-alert-detail-r802', 0.98, '/forensics?evidence=alert-detail-r802-session'),
      ('alert-detail-r802-session-2', 'Session', '周期心跳，每 30s 上行小包', '{"size":"137 B","duration":"08m16s","object_path":"minio://traffic-evidence/alerts/r802/session-heartbeat.json"}', '{"bucket":"traffic-evidence","object":"alerts/r802/session-heartbeat.json","session_id":"session-20260620-000124"}', '/sessions/session-20260620-000124', 0.97, '/forensics?evidence=alert-detail-r802-session-2'),
      ('alert-detail-r802-log', '日志', 'IDS 设备日志命中 C2_Tunnel_v3 与 JA3 异常', '{"size":"103 B","rule":"C2_Tunnel_v3","ja3_score":0.91,"object_path":"minio://traffic-evidence/alerts/r802/ids-alert-detail-r802.log"}', '{"bucket":"traffic-evidence","object":"alerts/r802/ids-alert-detail-r802.log","index":"traffic-logs","document":"alert-detail-r802"}', '', 0.96, '/audit?evidence=alert-detail-r802-log'),
      ('alert-detail-r802-graph', '图谱路径', '172.16.5.10 经核心区访问 185.22.14.9', '{"size":"178 B","edge_weight":0.86,"relation":"C2通信","object_path":"minio://traffic-evidence/alerts/r802/path-alert-detail-r802.json"}', '{"bucket":"traffic-evidence","object":"alerts/r802/path-alert-detail-r802.json","graph_space":"traffic_graph","path_id":"alert-detail-r802"}', '', 0.94, '/graph?path=alert-detail-r802'),
      ('alert-detail-r802-hash', '文件', '证据包 Hash 清单，SHA256 校验通过', '{"size":"65 B","sha256":"1a2b3c4d5bef79a8","object_path":"minio://traffic-evidence/alerts/r802/hash.txt"}', '{"bucket":"traffic-evidence","object":"alerts/r802/hash.txt","signed_url":"https://evidence.campus.local/signed/AL-20260620-000123"}', '', 1.0, '/forensics?evidence=alert-detail-r802-hash')
    ], number + 1) AS item
  FROM numbers(6)
);

SELECT
  'alert-detail-accept-r802' AS fixture_id,
  (SELECT count() FROM traffic.alerts_latest FINAL WHERE tenant_id = 'default' AND alert_id = fixture_id) AS alert_rows,
  (SELECT count() FROM traffic.evidence WHERE tenant_id = 'default' AND alert_id = fixture_id) AS evidence_rows;
