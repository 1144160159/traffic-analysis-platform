-- Canonical probe-management UI fixture.
-- Intended for acceptance/dev environments. Re-running it only replaces rows
-- carrying the same fixture marker and never deletes operator-created probes.
BEGIN;

DELETE FROM probes
WHERE tenant_id = 'default'
  AND hardware_info->>'fixture' = 'probes-ui-v1';

WITH fixture AS (
  SELECT
    i,
    CASE i
      WHEN 1 THEN 'PROBE-DC-01'
      WHEN 2 THEN 'PROBE-DC-02'
      WHEN 3 THEN 'PROBE-BUILD-01'
      WHEN 4 THEN 'PROBE-BUILD-02'
      WHEN 5 THEN 'PROBE-OFFICE-01'
      WHEN 6 THEN 'PROBE-SPORT-01'
      WHEN 7 THEN 'PROBE-DORM-01'
      WHEN 8 THEN 'PROBE-LIB-01'
      WHEN 9 THEN 'PROBE-LAB-01'
      WHEN 10 THEN 'PROBE-BRANCH-01'
      ELSE 'PROBE-CAMPUS-' || lpad((i - 10)::text, 2, '0')
    END AS probe_id,
    CASE WHEN i <= 10 THEN
      (ARRAY['数据中心机房 A','数据中心机房 B','教学楼 A','教学楼 B','办公区','体育馆','宿舍区','图书馆','实验楼','汇聚区 B'])[i]
    END AS named_location,
    CASE (i - 1) % 4
      WHEN 0 THEN '混合 (L2+L3)'
      WHEN 1 THEN 'L2 镜像'
      WHEN 2 THEN 'L3 路由'
      ELSE '流量聚合'
    END AS capture_mode
  FROM generate_series(1, 25) AS i
)
INSERT INTO probes (
  probe_id, tenant_id, name, status, hardware_info, software_version,
  last_heartbeat, created_at, updated_at
)
SELECT
  probe_id,
  'default',
  probe_id,
  CASE WHEN i = 25 THEN 'inactive' WHEN i >= 22 THEN 'warning' ELSE 'active' END,
  jsonb_build_object(
    'fixture', 'probes-ui-v1',
    'hostname', lower(probe_id),
    'ip_address', format('10.20.%s.%s', 10 + ((i - 1) / 10), 20 + i),
    'location', COALESCE(named_location, '主园区节点 ' || lpad((i - 10)::text, 2, '0')),
    'health_score', CASE WHEN i = 25 THEN 0 WHEN i >= 22 THEN 72 ELSE 96 END,
    'cpu_usage', round((19.6 + i)::numeric, 1),
    'memory_usage', round((28.3 + i)::numeric, 1),
    'disk_usage', round((37.5 + (i % 9) * 2.3)::numeric, 1),
    'drop_rate', CASE WHEN i = 25 THEN 1.0 WHEN i >= 22 THEN round((0.0082 + (i - 22) * 0.0015)::numeric, 4) ELSE round((0.0001 + (i % 4) * 0.0001)::numeric, 4) END,
    'parse_rate', CASE WHEN i = 25 THEN 0.0 WHEN i >= 22 THEN round((97.6 - (i - 22) * 0.31)::numeric, 2) ELSE round((99.48 - (i % 5) * 0.07)::numeric, 2) END,
    'bandwidth_mbps', CASE WHEN i = 25 THEN 0 ELSE 3600 + i * 620 END,
    'capture_mode', capture_mode,
    'interfaces', CASE WHEN i = 25 THEN '[]'::jsonb ELSE jsonb_build_array('eth2', 'eth3') END,
    'uptime_seconds', CASE WHEN i = 25 THEN 0 ELSE 432000 + i * 21600 END,
    'archive_path', 's3://pcap-archive/' || lower(probe_id) || '/',
    'mtls_enabled', i <> 25,
    'topology_x', CASE WHEN i <= 8 THEN (ARRAY[32,51,14,30,51,78,91,78])[i] ELSE 10 + ((i - 1) % 8) * 11 END,
    'topology_y', CASE WHEN i <= 8 THEN (ARRAY[25,78,48,69,49,26,49,73])[i] ELSE 18 + ((i - 1) % 4) * 20 END,
    'topology_z', CASE WHEN i <= 8 THEN (ARRAY[4,3,2,2,4,2,3,3])[i] ELSE 1 END,
    'topology_zone', CASE WHEN i <= 2 THEN '核心机房' WHEN i <= 5 THEN '教学办公区' WHEN i <= 6 THEN '生活区' ELSE '公共服务区' END,
    'topology_role', CASE WHEN i <= 2 THEN '汇聚探针' WHEN i <= 5 THEN '接入探针' ELSE '边缘探针' END,
    'topology_links', CASE i
      WHEN 1 THEN jsonb_build_array('PROBE-DC-02','PROBE-BUILD-01')
      WHEN 2 THEN jsonb_build_array('PROBE-BUILD-02','PROBE-OFFICE-01','PROBE-DORM-01')
      WHEN 3 THEN jsonb_build_array('PROBE-OFFICE-01')
      WHEN 4 THEN jsonb_build_array('PROBE-DC-02')
      WHEN 5 THEN jsonb_build_array('PROBE-SPORT-01','PROBE-LIB-01')
      WHEN 6 THEN jsonb_build_array('PROBE-DORM-01')
      WHEN 7 THEN jsonb_build_array('PROBE-LIB-01')
      WHEN 22 THEN jsonb_build_array('PROBE-DC-01')
      WHEN 23 THEN jsonb_build_array('PROBE-DC-02')
      WHEN 25 THEN jsonb_build_array('PROBE-OFFICE-01')
      ELSE '[]'::jsonb
    END,
    'topology_link_bandwidths_gbps', CASE i
      WHEN 1 THEN jsonb_build_array(40,25)
      WHEN 2 THEN jsonb_build_array(32,18,15)
      WHEN 3 THEN jsonb_build_array(12)
      WHEN 4 THEN jsonb_build_array(22)
      WHEN 5 THEN jsonb_build_array(18,20)
      WHEN 6 THEN jsonb_build_array(10)
      WHEN 7 THEN jsonb_build_array(16)
      WHEN 22 THEN jsonb_build_array(38)
      WHEN 23 THEN jsonb_build_array(42)
      WHEN 25 THEN jsonb_build_array(0)
      ELSE '[]'::jsonb
    END,
    'trend_labels', jsonb_build_array('21:45','22:00','22:15','22:30','22:45','23:00','23:15','23:30','23:45','00:00','00:15','00:30','00:45','01:00','01:15','01:30','01:45','02:00','02:15','02:30','02:45','03:00','03:15','03:45'),
    'bandwidth_trend', to_jsonb(ARRAY(
      SELECT round((GREATEST(0.3, (3.0 + i * 0.34) + sin((point + i)::numeric * 0.63) * (1.1 + (i % 3) * 0.35)))::numeric, 1)
      FROM generate_series(1, 24) AS point
    )),
    'batch_trend', to_jsonb(ARRAY(
      SELECT round((GREATEST(0.5, (8.0 + i * 0.48) + sin((point + i)::numeric * 0.51) * 2.8))::numeric, 1)
      FROM generate_series(1, 24) AS point
    )),
    'pps_k', round((36.0 + i * 1.24)::numeric, 1),
    'bandwidth_threshold_gbps', 30
  ),
  CASE WHEN i % 5 = 0 THEN 'v3.4.6' ELSE 'v3.4.7' END,
  CASE WHEN i = 25 THEN now() - interval '12 minutes' ELSE now() - make_interval(secs => i % 4 + 1) END,
  now() - make_interval(days => 26 - i),
  now() - make_interval(secs => i)
FROM fixture
ON CONFLICT (probe_id) DO UPDATE SET
  tenant_id = EXCLUDED.tenant_id,
  name = EXCLUDED.name,
  status = EXCLUDED.status,
  hardware_info = EXCLUDED.hardware_info,
  software_version = EXCLUDED.software_version,
  last_heartbeat = EXCLUDED.last_heartbeat,
  updated_at = EXCLUDED.updated_at
WHERE probes.hardware_info->>'fixture' = 'probes-ui-v1';

DO $probe_fixture_guard$
BEGIN
  IF (SELECT count(*) FROM probes WHERE tenant_id='default' AND hardware_info->>'fixture'='probes-ui-v1') <> 25 THEN
    RAISE EXCEPTION 'probe fixture ID collides with a non-fixture probe; no operator-owned row was overwritten';
  END IF;
END;
$probe_fixture_guard$;

COMMIT;
