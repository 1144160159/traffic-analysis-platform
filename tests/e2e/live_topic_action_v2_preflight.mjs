#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.BASE_URL || 'http://10.0.5.8:30180/api/v1';
const runId = process.env.RUN_ID || `topic-action-v2-${Date.now()}`;
const outputPath = path.resolve(
  root,
  process.env.OUTPUT_PATH || path.join('doc/02_acceptance/runs', runId, 'report.json'),
);
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) {
  delete process.env[key];
}
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const secret = Buffer.from(execFileSync(
  'kubectl',
  ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'],
  { encoding: 'utf8', env: process.env, timeout: 15_000 },
), 'base64').toString('utf8');

function token({
  tenantId = 'default',
  permissions = ['*', 'admin:*', 'topic:read', 'topic:write'],
} = {}) {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service',
    sub: crypto.randomUUID(),
    jti: crypto.randomUUID(),
    user_id: crypto.randomUUID(),
    tenant_id: tenantId,
    username: `codex-${runId}`,
    roles: permissions.includes('*') ? ['admin'] : ['viewer'],
    permissions,
    token_type: 'access',
    session_id: crypto.randomUUID(),
    iat: now,
    exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const adminToken = token();
const viewerToken = token({ permissions: [] });
const otherTenantToken = token({ tenantId: 'tenant-b' });
const checks = [];
const traces = [];
const check = (name, passed, detail = {}) => checks.push({ name, passed: Boolean(passed), detail });

async function request(endpoint, {
  expected = 200,
  tokenValue = adminToken,
  ...options
} = {}) {
  const response = await fetch(`${baseUrl}${endpoint}`, {
    ...options,
    headers: {
      Authorization: `Bearer ${tokenValue}`,
      'Content-Type': 'application/json',
      'X-Request-ID': `${runId}-${crypto.randomUUID()}`,
      ...(options.headers || {}),
    },
    signal: AbortSignal.timeout(45_000),
  });
  const body = await response.json().catch(() => ({}));
  traces.push({
    method: options.method || 'GET',
    endpoint,
    status: response.status,
    trace_id: response.headers.get('x-trace-id') || body?.meta?.trace_id || '',
    error_code: body?.error?.code || '',
  });
  if (response.status !== expected) {
    throw new Error(`${options.method || 'GET'} ${endpoint}: expected ${expected}, got ${response.status}: ${JSON.stringify(body).slice(0, 700)}`);
  }
  return { response, body, data: body.data ?? body };
}

function sql(query) {
  return execFileSync(
    'kubectl',
    ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', query],
    { encoding: 'utf8', env: process.env, timeout: 45_000 },
  ).trim().split('\n').at(-1);
}

const escapeSQL = (value) => String(value).replaceAll("'", "''");

function deploymentState() {
  const raw = execFileSync(
    'kubectl',
    ['-n', 'traffic-analysis', 'get', 'deployment', 'alert-service', '-o', 'json'],
    { encoding: 'utf8', env: process.env, timeout: 15_000 },
  );
  const deployment = JSON.parse(raw);
  return {
    image: deployment.spec.template.spec.containers[0].image,
    image_pull_policy: deployment.spec.template.spec.containers[0].imagePullPolicy,
    replicas: deployment.status.replicas ?? 0,
    ready_replicas: deployment.status.readyReplicas ?? 0,
    updated_replicas: deployment.status.updatedReplicas ?? 0,
    unavailable_replicas: deployment.status.unavailableReplicas ?? 0,
    rollback_image: deployment.metadata.annotations?.['remediation.openai/rollback-image'] ?? '',
  };
}

function topicKafkaMessages(jobId) {
  const command = [
    'set -euo pipefail',
    'PROPS=/tmp/codex-topic-action-consumer.properties',
    'trap \'rm -f "$PROPS"\' EXIT',
    'cat > "$PROPS" <<EOF',
    'security.protocol=SASL_SSL',
    'sasl.mechanism=SCRAM-SHA-512',
    'sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="${KAFKA_INTER_BROKER_USERNAME}" password="${KAFKA_INTER_BROKER_PASSWORD}";',
    'ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12',
    'ssl.truststore.type=PKCS12',
    'ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}',
    'EOF',
    '/opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --consumer.config "$PROPS" --topic traffic.topic.action.v2 --from-beginning --timeout-ms 10000 --property print.partition=true --property print.offset=true --property print.key=true --property print.headers=true',
  ].join('\n');
  const output = execFileSync(
    'kubectl',
    ['-n', 'middleware', 'exec', 'kafka-0', '--', 'bash', '-lc', command],
    {
      encoding: 'utf8',
      env: process.env,
      timeout: 30_000,
      maxBuffer: 16 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );
  return output.split('\n')
    .filter((line) => line.includes(jobId))
    .map((line) => {
      const partition = Number(line.match(/Partition:(\d+)/)?.[1] ?? -1);
      const offset = Number(line.match(/Offset:(\d+)/)?.[1] ?? -1);
      const payloadStart = line.indexOf('{');
      let payload = {};
      if (payloadStart >= 0) {
        payload = JSON.parse(line.slice(payloadStart));
      }
      return {
        partition,
        offset,
        key: line.includes('\tdefault:tunnel\t') ? 'default:tunnel' : '',
        event_id: payload.event_id ?? '',
        event_type: payload.event_type ?? '',
        job_id: payload.job_id ?? '',
        trace_id: payload.trace_id ?? '',
        schema_version_header: line.includes('schema_version:2') ? 2 : 0,
        aggregate_version_header: Number(line.match(/aggregate_version:(\d+)/)?.[1] ?? 0),
      };
    });
}

function topicKafkaDescription() {
  const command = [
    'set -euo pipefail',
    'PROPS=/tmp/codex-topic-action-admin.properties',
    'trap \'rm -f "$PROPS"\' EXIT',
    'cat > "$PROPS" <<EOF',
    'security.protocol=SASL_SSL',
    'sasl.mechanism=SCRAM-SHA-512',
    'sasl.jaas.config=org.apache.kafka.common.security.scram.ScramLoginModule required username="${KAFKA_INTER_BROKER_USERNAME}" password="${KAFKA_INTER_BROKER_PASSWORD}";',
    'ssl.truststore.location=/etc/kafka/tls/kafka.truststore.p12',
    'ssl.truststore.type=PKCS12',
    'ssl.truststore.password=${KAFKA_TLS_TRUSTSTORE_PASSWORD}',
    'EOF',
    '/opt/kafka/bin/kafka-topics.sh --bootstrap-server kafka-bootstrap.middleware.svc:9092 --command-config "$PROPS" --describe --topic traffic.topic.action.v2',
  ].join('\n');
  const output = execFileSync(
    'kubectl',
    ['-n', 'middleware', 'exec', 'kafka-0', '--', 'bash', '-lc', command],
    {
      encoding: 'utf8',
      env: process.env,
      timeout: 30_000,
      maxBuffer: 4 * 1024 * 1024,
      stdio: ['ignore', 'pipe', 'pipe'],
    },
  );
  const lines = output.split('\n').map((line) => line.trim()).filter(Boolean);
  const summary = lines.find((line) => line.includes('PartitionCount:')) ?? '';
  const partitionLines = lines.filter((line) => /Partition:\s*\d+/.test(line));
  let underReplicated = 0;
  for (const line of partitionLines) {
    const replicas = (line.match(/Replicas:\s*([0-9,]+)/)?.[1] ?? '').split(',').filter(Boolean);
    const isr = (line.match(/Isr:\s*([0-9,]+)/)?.[1] ?? '').split(',').filter(Boolean);
    if (replicas.length !== isr.length) underReplicated += 1;
  }
  return {
    name: 'traffic.topic.action.v2',
    partition_count: Number(summary.match(/PartitionCount:\s*(\d+)/)?.[1] ?? 0),
    replication_factor: Number(summary.match(/ReplicationFactor:\s*(\d+)/)?.[1] ?? 0),
    retention_ms: Number(summary.match(/retention\.ms=(\d+)/)?.[1] ?? 0),
    retention_bytes: Number(summary.match(/retention\.bytes=(\d+)/)?.[1] ?? 0),
    partitions_observed: partitionLines.length,
    under_replicated_partitions: underReplicated,
  };
}

let result;
try {
  const deployment = deploymentState();
  check('candidate alert deployment is ready', deployment.replicas === 1
    && deployment.ready_replicas === 1
    && deployment.updated_replicas === 1
    && deployment.unavailable_replicas === 0
    && deployment.image.endsWith(':remediation-7c3110d2d17f')
    && deployment.image_pull_policy === 'Never', deployment);

  const snapshot = await request('/topics/tunnel/snapshot');
  const snapshotId = snapshot.data.snapshot_id;
  const revision = Number(snapshot.data.revision);
  const catalog = Array.isArray(snapshot.data.action_catalog) ? snapshot.data.action_catalog : [];
  const traceSpec = catalog.find((entry) => entry.action_id === 'trace');
  const containSpec = catalog.find((entry) => entry.action_id === 'contain');
  check('unified snapshot exposes immutable contract metadata', Boolean(snapshotId)
    && revision > 0
    && Number(snapshot.body.meta?.contract_version) === 1
    && snapshot.body.meta?.snapshot_id === snapshotId
    && Boolean(snapshot.body.meta?.as_of)
    && Boolean(snapshot.body.meta?.trace_id)
    && typeof snapshot.body.meta?.partial === 'boolean'
    && Array.isArray(snapshot.body.meta?.missing_sections)
    && typeof snapshot.body.meta?.source_watermarks === 'object', {
    snapshot_id: snapshotId,
    revision,
    partial: snapshot.body.meta?.partial,
    missing_sections: snapshot.body.meta?.missing_sections,
    source_watermarks: snapshot.body.meta?.source_watermarks,
  });
  check('action catalog distinguishes bound and unavailable executors',
    traceSpec?.enabled === true
      && traceSpec?.executor === 'internal_receipt'
      && containSpec?.enabled === false
      && containSpec?.executor === 'soar'
      && Boolean(containSpec?.unavailable_cause),
    { trace: traceSpec, contain: containSpec });

  await request('/topics/tunnel/snapshot', { expected: 403, tokenValue: viewerToken });
  check('topic snapshot requires topic read permission', true);

  const reason = `confirmed ${runId} internal receipt`;
  const target = `topic-target-${runId}`;
  const actionBody = {
    action_id: 'trace',
    target,
    snapshot_id: snapshotId,
    expected_revision: revision,
    reason,
    detail: { run_id: runId, evidence_mode: 'live' },
  };

  await request('/topics/tunnel/actions', {
    expected: 409,
    method: 'POST',
    headers: { 'Idempotency-Key': `topic-disabled-${runId}` },
    body: JSON.stringify({ ...actionBody, action_id: 'contain' }),
  });
  check('unbound SOAR executor is rejected without side effect', true);

  await request('/topics/tunnel/actions', {
    expected: 409,
    method: 'POST',
    headers: { 'Idempotency-Key': `topic-revision-${runId}` },
    body: JSON.stringify({ ...actionBody, expected_revision: revision + 1 }),
  });
  check('optimistic revision conflict is rejected', true);

  await request('/topics/tunnel/actions', {
    expected: 404,
    tokenValue: otherTenantToken,
    method: 'POST',
    headers: { 'Idempotency-Key': `topic-cross-tenant-${runId}` },
    body: JSON.stringify(actionBody),
  });
  check('snapshot cannot cross tenant boundary', true);

  const idempotencyKey = `topic-action:${runId}:${crypto.randomUUID()}`;
  const accepted = await request('/topics/tunnel/actions', {
    expected: 202,
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(actionBody),
  });
  const jobId = accepted.data.job_id;
  const traceId = accepted.data.trace_id;
  check('topic action commits an accepted durable job', Boolean(jobId)
    && accepted.data.status === 'accepted'
    && accepted.data.action_id === 'trace'
    && accepted.data.executor === 'internal_receipt'
    && accepted.data.snapshot_id === snapshotId
    && accepted.data.expected_revision === revision
    && accepted.data.revision === 1
    && Boolean(traceId)
    && accepted.body.meta?.trace_id === traceId, accepted.data);

  const replay = await request('/topics/tunnel/actions', {
    expected: 202,
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify(actionBody),
  });
  check('exact idempotent replay reuses the original job',
    replay.data.job_id === jobId && replay.data.trace_id === traceId, replay.data);

  const conflict = await request('/topics/tunnel/actions', {
    expected: 409,
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ ...actionBody, target: `${target}-different` }),
  });
  check('same idempotency key with a different payload is rejected',
    conflict.body?.error?.code === 'IDEMPOTENCY_KEY_CONFLICT', conflict.body?.error ?? {});

  let completed;
  for (let attempt = 0; attempt < 30; attempt += 1) {
    const current = await request(`/topics/tunnel/actions/${encodeURIComponent(jobId)}`);
    completed = current;
    if (['completed', 'partial', 'failed', 'cancelled'].includes(current.data.status)) break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  check('internal executor reaches final success with receipt', completed?.data?.status === 'completed'
    && completed.data.revision === 3
    && completed.data.attempts === 1
    && completed.data.receipt?.executor === 'internal_receipt'
    && completed.data.receipt?.operation === 'trace'
    && completed.data.receipt?.snapshot_id === snapshotId
    && completed.data.receipt?.target === target
    && completed.data.trace_id === traceId
    && completed.body.meta?.partial === false, completed?.data ?? {});

  let delivered = completed;
  for (let attempt = 0; attempt < 30; attempt += 1) {
    const current = await request(`/topics/tunnel/actions/${encodeURIComponent(jobId)}`);
    delivered = current;
    if (Number(current.data?.outbox?.published) === 2
      && Number(current.data?.outbox?.pending) === 0) break;
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  check('Kafka outbox reaches producer-acknowledged state', Number(delivered?.data?.outbox?.total) === 2
    && Number(delivered.data.outbox.published) === 2
    && Number(delivered.data.outbox.pending) === 0
    && Number(delivered.data.outbox.attempts) === 2
    && delivered.data.outbox.last_error === ''
    && delivered.data.outbox.topic === 'traffic.topic.action.v2'
    && String(delivered.body.meta?.source_watermarks?.['kafka.traffic.topic.action.v2.offset'])
      .startsWith('producer_acked_without_observed_offset:2/2@'), {
    outbox: delivered?.data?.outbox,
    watermark: delivered?.body?.meta?.source_watermarks?.['kafka.traffic.topic.action.v2.offset'],
  });

  await request(`/topics/tunnel/actions/${encodeURIComponent(jobId)}`, {
    expected: 404,
    tokenValue: otherTenantToken,
  });
  check('topic action job cannot cross tenant boundary', true);

  const pg = JSON.parse(sql(`SELECT json_build_object(
    'job',(SELECT count(*) FROM topic_actions WHERE tenant_id='default' AND action_id='${escapeSQL(jobId)}'::uuid AND status='completed' AND revision=3 AND attempts=1 AND trace_id='${escapeSQL(traceId)}'),
    'history',(SELECT count(*) FROM topic_action_history WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid),
    'history_states',(SELECT array_agg(to_status ORDER BY revision) FROM topic_action_history WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid),
    'receipts',(SELECT count(*) FROM topic_action_receipts WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid AND executor='internal_receipt'),
    'outbox',(SELECT count(*) FROM topic_action_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid),
    'outbox_published',(SELECT count(*) FROM topic_action_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid AND published),
    'outbox_pending',(SELECT count(*) FROM topic_action_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid AND NOT published),
    'outbox_types',(SELECT array_agg(event_type ORDER BY event_type) FROM topic_action_outbox WHERE tenant_id='default' AND job_id='${escapeSQL(jobId)}'::uuid),
    'audit',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(jobId)}' AND action IN ('TOPIC_ACTION_REQUESTED','TOPIC_ACTION_COMPLETED')),
    'audit_trace',(SELECT count(*) FROM audit_logs WHERE tenant_id='default' AND object_id='${escapeSQL(jobId)}' AND detail->>'trace_id'='${escapeSQL(traceId)}'),
    'snapshot_manifest',(SELECT count(*) FROM topic_snapshot_manifests WHERE tenant_id='default' AND topic='tunnel' AND snapshot_id='${escapeSQL(snapshotId)}'::uuid AND resource_revision=${revision} AND length(payload_sha256)=64)
  )`));
  check('PG history receipt outbox audit and snapshot reconcile',
    Number(pg.job) === 1
      && Number(pg.history) === 3
      && JSON.stringify(pg.history_states) === JSON.stringify(['accepted', 'running', 'completed'])
      && Number(pg.receipts) === 1
      && Number(pg.outbox) === 2
      && Number(pg.outbox_published) === 2
      && Number(pg.outbox_pending) === 0
      && JSON.stringify(pg.outbox_types) === JSON.stringify([
        'traffic.topic.v2.ActionRequested',
        'traffic.topic.v2.ActionResult',
      ])
      && Number(pg.audit) === 2
      && Number(pg.audit_trace) === 2
      && Number(pg.snapshot_manifest) === 1,
    pg);

  const kafkaTopic = topicKafkaDescription();
  check('Kafka lifecycle topic matches the versioned catalog and all ISR are present',
    kafkaTopic.partition_count === 6
      && kafkaTopic.replication_factor === 3
      && kafkaTopic.retention_ms === 604800000
      && kafkaTopic.retention_bytes === 134217728
      && kafkaTopic.partitions_observed === 6
      && kafkaTopic.under_replicated_partitions === 0,
    kafkaTopic);

  const kafkaMessages = topicKafkaMessages(jobId);
  const kafkaEventTypes = kafkaMessages.map((item) => item.event_type).sort();
  check('Kafka contains both stable lifecycle events at observed offsets',
    kafkaMessages.length === 2
      && kafkaMessages.every((item) => item.partition >= 0
        && item.offset >= 0
        && item.key === 'default:tunnel'
        && item.job_id === jobId
        && item.trace_id === traceId
        && item.event_id
        && item.schema_version_header === 2
        && item.aggregate_version_header === 1)
      && JSON.stringify(kafkaEventTypes) === JSON.stringify([
        'traffic.topic.v2.ActionRequested',
        'traffic.topic.v2.ActionResult',
      ].sort()),
    { messages: kafkaMessages });

  result = {
    schema_version: 1,
    run_id: runId,
    result: checks.every((item) => item.passed) ? 'pass' : 'fail',
    candidate: deployment,
    snapshot: {
      topic: 'tunnel',
      snapshot_id: snapshotId,
      revision,
      partial: snapshot.body.meta?.partial,
      missing_sections: snapshot.body.meta?.missing_sections ?? [],
      source_watermarks: snapshot.body.meta?.source_watermarks ?? {},
    },
    action: {
      job_id: jobId,
      action_id: 'trace',
      target,
      trace_id: traceId,
      final_status: completed?.data?.status ?? '',
      final_revision: completed?.data?.revision ?? 0,
      receipt_id: completed?.data?.receipt?.receipt_id ?? '',
      outbox: delivered?.data?.outbox ?? {},
      kafka_topic: kafkaTopic,
      kafka_messages: kafkaMessages,
      idempotency_key_redacted: true,
    },
    checks,
    traces,
    token_material_redacted: true,
    captured_at: new Date().toISOString(),
  };
} catch (error) {
  check('preflight execution', false, {
    error: error instanceof Error ? error.message : String(error),
  });
  result = {
    schema_version: 1,
    run_id: runId,
    result: 'fail',
    checks,
    traces,
    token_material_redacted: true,
    captured_at: new Date().toISOString(),
  };
}

fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
console.log(JSON.stringify({
  result: result.result,
  output: path.relative(root, outputPath),
  checks: `${checks.filter((item) => item.passed).length}/${checks.length}`,
  job_id: result.action?.job_id ?? '',
  snapshot_id: result.snapshot?.snapshot_id ?? '',
}, null, 2));
if (result.result !== 'pass') process.exitCode = 1;
