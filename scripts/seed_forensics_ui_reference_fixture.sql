-- Canonical, tenant-scoped and reversible forensics UI reference data.
-- Activate with: psql ... -v tenant_id=default -f scripts/seed_forensics_ui_reference_fixture.sql
-- Remove with:   DELETE FROM forensics_ui_fixtures WHERE tenant_id=:'tenant_id';

\if :{?tenant_id}
\else
\set tenant_id default
\endif

CREATE TABLE IF NOT EXISTS forensics_ui_fixtures (
  tenant_id TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
  endpoint TEXT NOT NULL CHECK (endpoint IN ('jobs','stats')),
  fixture_version TEXT NOT NULL,
  payload JSONB NOT NULL,
  active BOOLEAN NOT NULL DEFAULT false,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, endpoint)
);

WITH generated_jobs AS (
  SELECT jsonb_agg(jsonb_build_object(
    'job_id', format('F-20260620-%s', lpad((129-n)::text, 6, '0')),
    'task_id', format('F-20260620-%s', lpad((129-n)::text, 6, '0')),
    'status', CASE WHEN n % 37 = 0 THEN 'failed' WHEN n % 11 = 0 THEN 'processing' ELSE 'completed' END,
    'progress', CASE WHEN n % 37 = 0 THEN 71 WHEN n % 11 = 0 THEN 64 ELSE 100 END,
    'params', jsonb_build_object(
      'alert_id', CASE WHEN n <= 3 THEN 'AL-20260620-000122' ELSE format('AL-20260620-%s', lpad((123-n)::text, 6, '0')) END,
      'campaign_id', 'APT-20260619-001',
      'asset_id', (ARRAY['办公区-WS-1024','财务部-SRV-2003','核心区-DC-01','研发区-PC-3056','办公区-WS-2011'])[((n-1)%5)+1],
      'src_ip', (ARRAY['172.16.5.10','10.12.8.45','172.16.1.77','192.168.3.55','10.12.36.53'])[((n-1)%5)+1],
      'src_port', 44220+n,
      'dst_ip', (ARRAY['185.22.14.9','10.12.9.33','198.51.100.27','203.0.113.45','8.8.8.8'])[((n-1)%5)+1],
      'dst_port', (ARRAY[443,445,80,443,53])[((n-1)%5)+1],
      'protocol', (ARRAY['TLS','SMB','HTTP','TLS','DNS'])[((n-1)%5)+1],
      'start_time', 1781827200000 + n*300000,
      'end_time', 1781830800000 + n*300000
    ),
    'result_file_key', format('results/%s/F-20260620-%s/evidence.pcap', :'tenant_id', lpad((129-n)::text, 6, '0')),
    'sha256', md5(format('forensics-job-%s-a', n)) || md5(format('forensics-job-%s-b', n)),
    'total_bytes', 1048576 * (18 + (n % 24)),
    'total_packets', 4000 + n * 17,
    'files_scanned', 8 + (n % 18),
    'created_at', 1781838000000 - n*60000,
    'updated_at', 1781838060000 - n*60000
  ) ORDER BY n) AS rows
  FROM generate_series(1,190) AS n
)
INSERT INTO forensics_ui_fixtures (tenant_id, endpoint, fixture_version, payload, active, updated_at)
SELECT :'tenant_id', 'jobs', 'forensics-canonical-v1', jsonb_build_object('jobs', rows, 'total', 190), true, now()
FROM generated_jobs
ON CONFLICT (tenant_id, endpoint) DO UPDATE
SET fixture_version=EXCLUDED.fixture_version, payload=EXCLUDED.payload, active=true, updated_at=now();

WITH
generated_jobs AS (
  SELECT jsonb_agg(jsonb_build_object(
    'id', format('F-20260620-%s', lpad((129-n)::text, 6, '0')),
    'status', CASE WHEN n % 37 = 0 THEN '失败' WHEN n % 11 = 0 THEN '采集中' ELSE '完成' END,
    'progress', CASE WHEN n % 37 = 0 THEN 71 WHEN n % 11 = 0 THEN 64 ELSE 100 END,
    'resultKey', format('results/%s/F-20260620-%s/evidence.pcap', :'tenant_id', lpad((129-n)::text, 6, '0')),
    'sha256', md5(format('forensics-job-%s-a', n)) || md5(format('forensics-job-%s-b', n)),
    'totalBytes', 1048576 * (18 + (n % 24)), 'totalPackets', 4000 + n*17,
    'filesScanned', 8 + (n % 18), 'downloadUrl', '', 'expiresAt', 0, 'errorMessage', ''
  ) ORDER BY n) AS rows FROM generate_series(1,190) n
),
pcaps AS (
  SELECT jsonb_agg(jsonb_build_object(
    'fileKey', format('20260620/000123/%s', lpad(n::text, 3, '0')),
    'storagePath', format('evidence/pcap/000123/%s.pcap', lpad(n::text, 3, '0')),
    'probeId', '主校区-probe-01',
    'sizeBytes', 1048576 * (18 + (n % 24)),
    'sha256', md5(format('pcap-000123-%s-a', n)) || md5(format('pcap-000123-%s-b', n)),
    'startTime', format('06-19 %s:00', lpad(((n-1)%24)::text, 2, '0')),
    'endTime', format('06-19 %s:00', lpad((n%24)::text, 2, '0')),
    'packetCount', 1000+n*13, 'status', '已索引'
  ) ORDER BY n) AS rows FROM generate_series(1,1256) n
),
sessions AS (
  SELECT jsonb_agg(jsonb_build_object(
    'sessionId', format('session-20260620-%s', lpad(n::text, 4, '0')),
    'time', (ARRAY['03:41:55','03:41:20','03:39:44','03:38:31','03:38:12','03:37:05'])[((n-1)%6)+1],
    'protocol', (ARRAY['TLS','TLS','TLS','DNS','HTTP','TLS'])[((n-1)%6)+1],
    'source', format('172.16.5.10:%s', 44220+n),
    'destination', (ARRAY['185.22.14.9:443','185.22.14.9:443','104.16.12.34:443','8.8.8.8:53','198.51.100.27:80','203.0.113.45:443'])[((n-1)%6)+1],
    'byteCount', (ARRAY[1289748,524288,865280,2150,3365929,1101005])[((n-1)%6)+1],
    'packetCount', 48+n*7, 'duration', (ARRAY['12.45 s','5.16 s','8.37 s','0.21 s','14.82 s','7.03 s'])[((n-1)%6)+1],
    'risk', CASE WHEN n%7=0 THEN '中危' ELSE '低危' END,
    'sni', 'update.example.com', 'ja3', '771,4865-4866-4867,0-11-10,23-24,0'
  ) ORDER BY n) AS rows FROM generate_series(1,24) n
),
exports AS (
  SELECT jsonb_agg(jsonb_build_object(
    'id', format('PKG-20260620-%s', lpad(n::text, 4, '0')),
    'content', (ARRAY['PCAP+Session+日志+报告','Session+日志(CSV)','PCAP 原始包'])[((n-1)%3)+1],
    'files', 8+n*2, 'sizeBytes', 1048576*(128+n*32), 'status', '完成',
    'resultKey', format('results/%s/export/PKG-20260620-%s.zip', :'tenant_id', lpad(n::text, 4, '0'))
  ) ORDER BY n) AS rows FROM generate_series(1,12) n
),
hashes AS (
  SELECT jsonb_agg(jsonb_build_object(
    'fileKey', format('results/%s/F-20260620-000128/%s.pcap', :'tenant_id', lpad(n::text, 6, '0')),
    'sha256', CASE WHEN n=1 THEN '16f2fd56abfe05d2048fad5c18377e8990ca928b00e9e2c05ffdc420a42c8660' ELSE md5(format('pcap-hash-%s-a', n)) || md5(format('pcap-hash-%s-b', n)) END,
    'status', '匹配', 'checkedAt', format('06-20 03:44:%s', lpad((22-n)::text, 2, '0'))
  ) ORDER BY n) AS rows FROM generate_series(1,20) n
),
visuals AS (
  SELECT jsonb_build_object(
    'availability', jsonb_build_object('jobs','live','sessions','live','pcap','live','audit','live'),
    'totals', jsonb_build_object('jobs',190,'pcapIndexes',1256,'sessions',24,'exportRows',12,'hashRows',20),
    'stateCounts', jsonb_build_array(
      jsonb_build_object('label','新建','value',12,'status','info'),
      jsonb_build_object('label','排队中','value',5,'status','info'),
      jsonb_build_object('label','采集中','value',8,'status','warn'),
      jsonb_build_object('label','解析中','value',6,'status','warn'),
      jsonb_build_object('label','完成','value',156,'status','ok'),
      jsonb_build_object('label','失败','value',3,'status','risk')
    ),
    'jobs', generated_jobs.rows,
    'pcapIndexes', pcaps.rows,
    'pcapTrend', jsonb_build_array(
      jsonb_build_object('label','03:36','value',18), jsonb_build_object('label','03:38','value',33),
      jsonb_build_object('label','03:40','value',26), jsonb_build_object('label','03:42','value',39),
      jsonb_build_object('label','03:44','value',31)
    ),
    'sessions', sessions.rows,
    'completeness', jsonb_build_array(
      jsonb_build_object('label','原始文件校验','complete',100,'total',100,'status','ok'),
      jsonb_build_object('label','文件 hash 校验 (SHA256)','complete',100,'total',100,'status','ok'),
      jsonb_build_object('label','数字签名验证','complete',100,'total',100,'status','ok'),
      jsonb_build_object('label','签名人','complete',100,'total',100,'status','ok'),
      jsonb_build_object('label','证书有效期','complete',100,'total',100,'status','ok')
    ),
    'hashRows', hashes.rows,
    'signedUrls', jsonb_build_array(
      jsonb_build_object('type','PCAP','key','F-20260620-000128','url','https://minio.local/signed/pcap/000128','expiresAt','2026-06-27 03:45','status','有效'),
      jsonb_build_object('type','Session','key','session-000128','url','https://minio.local/signed/session/000128','expiresAt','2026-06-27 03:45','status','有效'),
      jsonb_build_object('type','日志','key','audit-000128','url','https://minio.local/signed/log/000128','expiresAt','2026-06-27 03:45','status','有效')
    ),
    'exportRows', exports.rows,
    'auditRows', jsonb_build_array(
      jsonb_build_object('time','06-20 03:44:58','user','sec_analyst','action','下载 PCAP','target','F-20260620-000128','result','成功'),
      jsonb_build_object('time','06-20 03:44:21','user','sec_analyst','action','导出 CSV','target','F-20260620-000128','result','成功'),
      jsonb_build_object('time','06-20 03:43:50','user','sec_analyst','action','校验 hash','target','F-20260620-000128','result','成功'),
      jsonb_build_object('time','06-20 03:42:31','user','system','action','生成签名 URL','target','F-20260620-000128','result','成功'),
      jsonb_build_object('time','06-20 03:41:12','user','sec_analyst','action','新建取证任务','target','F-20260620-000128','result','成功')
    )
  ) AS payload
  FROM generated_jobs, pcaps, sessions, exports, hashes
)
INSERT INTO forensics_ui_fixtures (tenant_id, endpoint, fixture_version, payload, active, updated_at)
SELECT :'tenant_id', 'stats', 'forensics-canonical-v1', jsonb_build_object(
  'task_stats', jsonb_build_object('new',12,'queued',5,'collecting',8,'parsing',6,'completed',156,'failed',3),
  'worker_stats', jsonb_build_object('workers',3,'queue_size',5),
  'ui_reference_visuals', visuals.payload
), true, now()
FROM visuals
ON CONFLICT (tenant_id, endpoint) DO UPDATE
SET fixture_version=EXCLUDED.fixture_version, payload=EXCLUDED.payload, active=true, updated_at=now();

SELECT tenant_id, endpoint, fixture_version, active, pg_column_size(payload) AS payload_bytes
FROM forensics_ui_fixtures
WHERE tenant_id=:'tenant_id'
ORDER BY endpoint;
