#!/usr/bin/env bash
set -euo pipefail

if ! command -v clickhouse-local >/dev/null 2>&1; then
  echo "clickhouse-local is required for this isolated SQL-engine check" >&2
  exit 77
fi

result="$({ clickhouse-local --multiquery --query "
CREATE DATABASE traffic;
CREATE TABLE traffic.sessions (
  tenant_id String, ts_start Int64, ts_end Int64, src_ip String, dst_ip String,
  protocol UInt32, bytes_total UInt64, num_pkts UInt32
) ENGINE=Memory;
CREATE TABLE traffic.alerts (
  tenant_id String, alert_id String, severity String, status String, alert_type String,
  src_ip String, dst_ip String, src_port UInt32, dst_port UInt32, protocol UInt32,
  score Float32, evidence_ids Array(String), first_seen Int64, last_seen Int64,
  state_version UInt64, event_id String, updated_at Int64
) ENGINE=Memory;
CREATE TABLE traffic.evidence (
  tenant_id String, evidence_id String, alert_id String, ts Int64, type String,
  summary String, metrics_json String, snippet_ref_json String, arkime_link String,
  confidence Float32, event_id String, ingest_ts Int64, visualization_url String
) ENGINE=Memory;
INSERT INTO traffic.sessions VALUES
  ('tenant-a',1785556800000,1785556801000,'192.0.2.8','203.0.113.9',6,1024,10),
  ('tenant-a',1785556802000,1785556803000,'198.51.100.2','192.0.2.8',17,2048,20),
  ('tenant-b',1785556800000,1785556801000,'192.0.2.8','203.0.113.10',6,9999,99);
INSERT INTO traffic.alerts VALUES
  ('tenant-a','alert-1','medium','open','exfil','192.0.2.8','203.0.113.9',44000,443,6,50,['evidence-old'],1785556800000,1785556801000,1,'event-1',1785556801000),
  ('tenant-a','alert-1','high','investigating','exfil','192.0.2.8','203.0.113.9',44000,443,6,90,['evidence-new'],1785556800000,1785556803000,2,'event-2',1785556803000),
  ('tenant-b','alert-2','critical','open','other','192.0.2.8','203.0.113.10',1,2,6,100,[],1785556800000,1785556803000,1,'event-x',1785556803000);
INSERT INTO traffic.evidence VALUES
  ('tenant-a','evidence-new','alert-1',1785556803000,'pcap','current evidence','{}','{"bucket":"evidence-bucket","object":"tenant-a/evidence-new.pcap","sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}','',1.0,'evidence-event-2',1785556803001,''),
  ('tenant-a','evidence-new','alert-1',1785556801000,'pcap','old evidence','{}','{}','',1.0,'evidence-event-1',1785556801001,''),
  ('tenant-b','evidence-new','alert-2',1785556803000,'pcap','wrong tenant','{}','{}','',1.0,'evidence-event-x',1785556803001,'');
SELECT
  'observation', count(), coalesce(sum(bytes_total),0), coalesce(sum(num_pkts),0),
  uniqExact(if(src_ip='192.0.2.8',dst_ip,src_ip)),
  toJSONString(arraySort(groupUniqArray(32)(protocol))),
  coalesce(min(ts_start),0), coalesce(max(ts_end),0)
FROM traffic.sessions
WHERE tenant_id='tenant-a'
  AND ts_start>=1785470400000 AND ts_start<=1785560400000
  AND (src_ip='192.0.2.8' OR dst_ip='192.0.2.8');
SELECT
  'alert', alert_id,
  argMax(severity,tuple(state_version,updated_at,event_id)) AS latest_severity,
  argMax(status,tuple(state_version,updated_at,event_id)) AS latest_status,
  toJSONString(argMax(evidence_ids,tuple(state_version,updated_at,event_id))) AS evidence_ids_json,
  argMax(last_seen,tuple(state_version,updated_at,event_id)) AS latest_last_seen,
  argMax(state_version,tuple(state_version,updated_at,event_id)) AS latest_state_version,
  argMax(event_id,tuple(state_version,updated_at,event_id)) AS latest_event_id
FROM traffic.alerts
WHERE tenant_id='tenant-a'
  AND last_seen>=1785470400000 AND last_seen<=1785560400000
  AND (src_ip='192.0.2.8' OR dst_ip='192.0.2.8')
GROUP BY alert_id
ORDER BY latest_last_seen DESC,alert_id ASC
LIMIT 51;
WITH ['evidence-new'] AS requested_evidence_ids
SELECT
  'evidence', evidence_id,
  argMax(alert_id,tuple(ts,ingest_ts,event_id)) AS latest_alert_id,
  argMax(ts,tuple(ts,ingest_ts,event_id)) AS latest_evidence_ts,
  argMax(type,tuple(ts,ingest_ts,event_id)) AS latest_evidence_type,
  argMax(summary,tuple(ts,ingest_ts,event_id)) AS latest_summary,
  argMax(snippet_ref_json,tuple(ts,ingest_ts,event_id)) AS latest_snippet_ref_json
FROM traffic.evidence
WHERE tenant_id='tenant-a' AND ts<=1785560400000 AND has(requested_evidence_ids,evidence_id)
GROUP BY evidence_id
ORDER BY evidence_id ASC;
"; } 2>&1)"

expected_observation=$'observation\t2\t3072\t30\t2\t[6,17]\t1785556800000\t1785556803000'
expected_alert=$'alert\talert-1\thigh\tinvestigating\t["evidence-new"]\t1785556803000\t2\tevent-2'
expected_evidence=$'evidence\tevidence-new\talert-1\t1785556803000\tpcap\tcurrent evidence\t{bucket:evidence-bucket,object:tenant-a/evidence-new.pcap,sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}'
if ! grep -Fqx "$expected_observation" <<<"$result"; then
  echo "observation reconciliation failed" >&2
  echo "$result" >&2
  exit 1
fi
if ! grep -Fqx "$expected_evidence" <<<"$result"; then
  echo "evidence latest-state reconciliation or tenant isolation failed" >&2
  echo "$result" >&2
  exit 1
fi
if ! grep -Fqx "$expected_alert" <<<"$result"; then
  echo "latest alert reconciliation or tenant isolation failed" >&2
  echo "$result" >&2
  exit 1
fi

echo "$result"
echo "asset_detail_clickhouse_local=PASS"
