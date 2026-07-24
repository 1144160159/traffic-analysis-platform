-- Behavior-baseline UI acceptance fixture for the live demo tenant.
--
-- This fixture is deliberately written to the real ClickHouse user_events
-- table so the account-baseline page exercises the production query path.
-- Run only when the prefix below is absent; the verification command records
-- that precondition before inserting.
INSERT INTO traffic.user_events
(
  event_id,
  tenant_id,
  user_id,
  username,
  event_type,
  source_ip,
  user_agent,
  resource,
  action,
  result,
  timestamp
)
SELECT
  concat('baseline-r657-', toString(number)) AS event_id,
  'default' AS tenant_id,
  concat('baseline-user-', toString(number % 5)) AS user_id,
  arrayElement(['sec_analyst', 'ops_admin', 'svc_sync', 'student_2026', 'contractor_07'], toUInt32(number % 5) + 1) AS username,
  arrayElement(['login', 'asset_access', 'permission_change', 'query', 'download'], toUInt32(number % 5) + 1) AS event_type,
  arrayElement(['10.12.4.21', '10.12.8.17', '172.20.10.8', '198.51.100.27', '203.0.113.45'], toUInt32(number % 5) + 1) AS source_ip,
  arrayElement(['Chrome/150 Windows', 'OpenSSH_9.6', 'service-agent/2.4', 'ChromeOS/146', 'Firefox/142'], toUInt32(number % 5) + 1) AS user_agent,
  arrayElement(['lab-srv-12', 'OpenSearch', 'ClickHouse', 'Kafka', '堡垒机', '统一认证'], toUInt32(number % 6) + 1) AS resource,
  arrayElement(['read', 'write', 'sudo', 'query', 'download'], toUInt32(number % 5) + 1) AS action,
  if(number % 19 = 0, 'denied', 'success') AS result,
  now() - toIntervalMinute(toInt64((number * 127) % 42000)) AS timestamp
FROM numbers(320);
