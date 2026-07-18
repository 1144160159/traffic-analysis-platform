-- Idempotent ClickHouse acceptance fixture for the real Argo MLOps pipeline.
-- Rows are isolated by the mlops-accept-r331 prefix and expire under the
-- existing table TTLs. They are not represented as production observations.

INSERT INTO traffic.alert_feedback (
  feedback_id, alert_id, tenant_id, user_id, label, reason_code, comment,
  add_to_whitelist, alert_type, severity, model_version, rule_version, created_at
)
SELECT
  concat('mlops-accept-r331-feedback-', leftPad(toString(number), 4, '0')),
  concat('mlops-accept-r331-alert-', leftPad(toString(number), 4, '0')),
  'default',
  'mlops-acceptance-fixture',
  if(number % 2 = 0, 'TP', 'FP'),
  'MLOPS_ACCEPTANCE_FIXTURE',
  'r331 real-pipeline acceptance fixture',
  0,
  'mlops_acceptance',
  if(number % 2 = 0, 'high', 'low'),
  'acceptance-fixture',
  'acceptance-fixture',
  now() - toIntervalSecond(number)
FROM numbers(200)
WHERE NOT EXISTS (
  SELECT 1 FROM traffic.alert_feedback
  WHERE tenant_id = 'default' AND feedback_id = 'mlops-accept-r331-feedback-0000'
);

INSERT INTO traffic.alerts (
  tenant_id, alert_id, dedup_fingerprint, community_id, session_id, campaign_id,
  feature_set_id, src_ip, dst_ip, src_port, dst_port, protocol, protocol_name,
  alert_type, labels, severity, score, status, assignee, evidence_ids,
  arkime_session_link, feedback_label, feedback_count, first_seen, last_seen,
  count, model_version, rule_version, state_version, event_id, kafka_ts,
  flink_out_ts, created_at, updated_at
)
SELECT
  'default',
  concat('mlops-accept-r331-alert-', leftPad(toString(number), 4, '0')),
  concat('mlops-accept-r331-dedup-', leftPad(toString(number), 4, '0')),
  concat('mlops-accept-r331-community-', leftPad(toString(number), 4, '0')),
  concat('mlops-accept-r331-session-', leftPad(toString(number), 4, '0')),
  '', 'v1',
  concat('10.31.', toString(intDiv(number, 250)), '.', toString(number % 250 + 1)),
  '10.32.0.1',
  toUInt32(10000 + number), 443, 6, 'TCP', 'mlops_acceptance',
  ['acceptance-fixture'],
  if(number % 2 = 0, 'high', 'low'),
  toFloat32(if(number % 2 = 0, 0.95, 0.15)),
  'closed', '', [], '', if(number % 2 = 0, 'TP', 'FP'), 1,
  toUnixTimestamp64Milli(now64(3) - toIntervalSecond(number)),
  toUnixTimestamp64Milli(now64(3) - toIntervalSecond(number)),
  1, 'acceptance-fixture', 'acceptance-fixture', 1,
  concat('mlops-accept-r331-alert-event-', leftPad(toString(number), 4, '0')),
  toUnixTimestamp64Milli(now64(3)), toUnixTimestamp64Milli(now64(3)),
  toUnixTimestamp64Milli(now64(3)), toUnixTimestamp64Milli(now64(3))
FROM numbers(200)
WHERE NOT EXISTS (
  SELECT 1 FROM traffic.alerts
  WHERE tenant_id = 'default' AND alert_id = 'mlops-accept-r331-alert-0000'
);

INSERT INTO traffic.feature_stat (
  tenant_id, run_id, feature_set_id, schema_version, event_id, object_type,
  object_id, community_id, ts, protocol, duration_ms, pps, bps,
  up_down_ratio, pktlen_mean, pktlen_std, iat_mean_ms, iat_std_ms,
  active_mean_ms, idle_mean_ms, tcp_flag_syn_cnt, tcp_flag_ack_cnt,
  tcp_init_win_bytes_fwd, tcp_init_win_bytes_bwd, extra, ingest_ts
)
SELECT
  'default', 'mlops-accept-r331', 'v1', 'acceptance-fixture-v1',
  concat('mlops-accept-r331-feature-', leftPad(toString(number), 4, '0')),
  'session',
  concat('mlops-accept-r331-object-', leftPad(toString(number), 4, '0')),
  concat('mlops-accept-r331-community-', leftPad(toString(number), 4, '0')),
  now64(3) - toIntervalSecond(number),
  6,
  toUInt32(1000 + number * 3),
  toFloat32(if(number % 2 = 0, 850 + number, 40 + number)),
  toFloat32(if(number % 2 = 0, 900000 + number * 1000, 80000 + number * 100)),
  toFloat32(if(number % 2 = 0, 4.2, 0.8)),
  toFloat32(if(number % 2 = 0, 900, 180)),
  toFloat32(20 + number % 13),
  toFloat32(if(number % 2 = 0, 2.5, 35)),
  toFloat32(1 + number % 7),
  toFloat32(if(number % 2 = 0, 700, 80)),
  toFloat32(if(number % 2 = 0, 20, 450)),
  toUInt16(if(number % 2 = 0, 12, 1)),
  toUInt16(if(number % 2 = 0, 10, 2)),
  toUInt32(if(number % 2 = 0, 65535, 8192)),
  toUInt32(if(number % 2 = 0, 32768, 4096)),
  [],
  now64(3)
FROM numbers(200)
WHERE NOT EXISTS (
  SELECT 1 FROM traffic.feature_stat
  WHERE tenant_id = 'default' AND event_id = 'mlops-accept-r331-feature-0000'
);

SELECT
  'mlops-accept-r331' AS scenario_id,
  (SELECT count() FROM traffic.alert_feedback WHERE tenant_id = 'default' AND startsWith(feedback_id, 'mlops-accept-r331-')) AS feedback_rows,
  (SELECT count() FROM traffic.alerts WHERE tenant_id = 'default' AND startsWith(alert_id, 'mlops-accept-r331-')) AS alert_rows,
  (SELECT count() FROM traffic.feature_stat WHERE tenant_id = 'default' AND startsWith(event_id, 'mlops-accept-r331-')) AS feature_rows;
