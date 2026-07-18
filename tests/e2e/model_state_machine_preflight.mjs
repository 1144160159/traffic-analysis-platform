#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';

const root = process.cwd();
const uiRequire = createRequire(path.join(root, 'web/ui/package.json'));
const { chromium } = uiRequire('@playwright/test');
const baseUrl = 'http://10.0.5.8:30180';
const cdpUrl = 'http://127.0.0.1:9224';
const revision = process.env.MODEL_EVIDENCE_REVISION?.trim() || 'r318';
const outputPath = path.join(root, `evidence/ui-image-breakdowns/pages/models/state-machine-${revision}.json`);
const fixtureModelPath = path.join(root, 'tests/fixtures/model-management/model.json');
const fixtureColumnsPath = path.join(root, 'tests/fixtures/model-management/feature_columns.json');
const fixtureSha256 = crypto.createHash('sha256').update(fs.readFileSync(fixtureModelPath)).digest('hex');

for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

function psql(sql) {
  return execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-v', 'ON_ERROR_STOP=1', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', sql], {
    encoding: 'utf8', env: process.env, timeout: 20_000,
  }).trim();
}

function kubectl(args, options = {}) {
  return execFileSync('kubectl', args, {
    encoding: options.encoding ?? 'utf8', env: process.env,
    timeout: options.timeout ?? 30_000, input: options.input,
    stdio: options.stdio,
  });
}

function secretValue(namespace, key) {
  const encoded = kubectl(['-n', namespace, 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`]);
  return Buffer.from(encoded, 'base64').toString('utf8');
}

function uploadAcceptanceModel(podName, objectPrefix) {
  kubectl(['-n', 'minio', 'run', podName,
    '--image=docker.io/minio/mc@sha256:eb4ea9884b77704230e2423e9004d2fa738dc272876b9cc41a297d29443b8780',
    '--restart=Never', '--command', '--', '/bin/sh', '-c', 'sleep 600']);
  kubectl(['-n', 'minio', 'wait', '--for=condition=Ready', `pod/${podName}`, '--timeout=90s'], { timeout: 100_000 });
  kubectl(['-n', 'minio', 'exec', podName, '--', 'mc', 'alias', 'set', 'acceptance',
    'http://minio.minio.svc:9000', secretValue('minio', 'MINIO_ACCESS_KEY'), secretValue('minio', 'MINIO_SECRET_KEY')]);
  kubectl(['-n', 'minio', 'exec', podName, '--', 'mc', 'mb', '--ignore-existing', 'acceptance/traffic-models']);
  kubectl(['-n', 'minio', 'exec', '-i', podName, '--', 'mc', 'pipe',
    `acceptance/traffic-models/${objectPrefix}/model.json`], { input: fs.readFileSync(fixtureModelPath), encoding: null });
  kubectl(['-n', 'minio', 'exec', '-i', podName, '--', 'mc', 'pipe',
    `acceptance/traffic-models/${objectPrefix}/feature_columns.json`], { input: fs.readFileSync(fixtureColumnsPath), encoding: null });
}

function smokeToken(userId) {
  const encoded = execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], {
    encoding: 'utf8', env: process.env, timeout: 15_000,
  });
  const now = Math.floor(Date.now() / 1_000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: userId, jti: crypto.randomUUID(), user_id: userId,
    tenant_id: 'default', username: 'codex-model-state-preflight', roles: ['admin'],
    permissions: ['*', 'admin:*', 'model:*'], token_type: 'access', iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  const secret = Buffer.from(encoded, 'base64').toString('utf8');
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const suffix = crypto.randomUUID().slice(0, 8);
const modelId = crypto.randomUUID();
const noGateModelId = crypto.randomUUID();
const missingPathModelId = crypto.randomUUID();
const currentVersion = `preflight-current-${suffix}`;
const rollbackVersion = `preflight-rollback-${suffix}`;
const noGateVersion = `preflight-no-gate-${suffix}`;
const registeredVersion = `preflight-registered-${suffix}`;
const token = smokeToken(psql("SELECT user_id FROM users WHERE tenant_id = 'default' ORDER BY created_at LIMIT 1"));
const authHeaders = { Authorization: `Bearer ${token}` };
const fixturePod = `model-fixture-${suffix}`;
const fixturePrefix = `acceptance/${suffix}`;
const fixtureArtifactUri = `s3://traffic-models/${fixturePrefix}/model.json`;
const cleanup = () => psql(`
  DELETE FROM model_update_applied_acks WHERE model_id IN ('${modelId}', '${noGateModelId}', '${missingPathModelId}');
  DELETE FROM model_update_outbox WHERE model_id IN ('${modelId}', '${noGateModelId}', '${missingPathModelId}');
  DELETE FROM audit_logs WHERE object_id IN ('${modelId}', '${noGateModelId}', '${currentVersion}', '${rollbackVersion}', '${noGateVersion}', '${registeredVersion}');
  DELETE FROM models WHERE model_id IN ('${modelId}'::uuid, '${noGateModelId}'::uuid);
`);

let browser;
let cleanupSucceeded = false;
let cleanupError = '';
try {
  cleanup();
  uploadAcceptanceModel(fixturePod, fixturePrefix);
  psql(`
    WITH feature AS (SELECT feature_set_id FROM feature_sets ORDER BY feature_set_id LIMIT 1)
    INSERT INTO models (model_id, tenant_id, name, model_type, description)
    VALUES
      ('${modelId}'::uuid, 'default', 'codex-model-state-${suffix}', 'xgboost', 'isolated state-machine acceptance model'),
      ('${noGateModelId}'::uuid, 'default', 'codex-model-no-gate-${suffix}', 'xgboost', 'isolated fail-closed gate model');
    WITH feature AS (SELECT feature_set_id FROM feature_sets ORDER BY feature_set_id LIMIT 1)
    INSERT INTO model_versions (model_version, model_id, tenant_id, feature_set_id, artifact_uri, metrics, status)
    SELECT '${currentVersion}', '${modelId}'::uuid, 'default', feature_set_id, 's3://acceptance/current.onnx', '{"f1_score":0.96}'::jsonb, 'active' FROM feature
    UNION ALL
    SELECT '${rollbackVersion}', '${modelId}'::uuid, 'default', feature_set_id, '${fixtureArtifactUri}',
           '{"f1_score":0.95,"threshold":0.5,"artifact_sha256":"${fixtureSha256}"}'::jsonb, 'deprecated' FROM feature
    UNION ALL
    SELECT '${noGateVersion}', '${noGateModelId}'::uuid, 'default', feature_set_id, 's3://acceptance/no-gate.onnx', '{"f1_score":0.97}'::jsonb, 'registered' FROM feature;
    INSERT INTO model_workbench_items (item_id, tenant_id, model_id, category, ordinal, payload, scenario_id)
    SELECT 'preflight-${suffix}-gate-' || ordinal, 'default', '${modelId}'::uuid, 'review_gates', ordinal,
           jsonb_build_object('name', 'preflight-gate-' || ordinal, 'status', 'approved', 'owner', 'codex'), 'state-machine-preflight'
    FROM generate_series(0, 3) AS ordinal;
  `);

  const version = await (await fetch(`${cdpUrl}/json/version`)).json();
  browser = await chromium.connectOverCDP(cdpUrl);
  const context = browser.contexts()[0] ?? await browser.newContext();
  const page = await context.newPage();

  const grayResponse = await page.request.post(`${baseUrl}/api/v1/models/${modelId}/versions/${rollbackVersion}/activate`, {
    headers: authHeaders, data: { gray_percent: 20 },
  });
  const grayPayload = await grayResponse.json().catch(() => ({}));

  const noGateResponse = await page.request.post(`${baseUrl}/api/v1/models/${noGateModelId}/versions/${noGateVersion}/activate`, {
    headers: authHeaders, data: { gray_percent: 100 },
  });
  const noGatePayload = await noGateResponse.json().catch(() => ({}));

  const inlineFeedbackResponse = await page.request.post(`${baseUrl}/api/v1/models/${modelId}/feedback-samples`, {
    headers: authHeaders, data: { dataset_id: 'feedback-latest', sample_count: 1, samples: [{ src_ip: '10.0.0.1' }] },
  });

  const duplicateBodies = { dataset_id: 'ds_ueba_latest', strategy: 'incremental', reason: 'state_machine_preflight' };
  const retrainResponses = await Promise.all([
    page.request.post(`${baseUrl}/api/v1/models/${modelId}/retrain`, { headers: authHeaders, data: duplicateBodies }),
    page.request.post(`${baseUrl}/api/v1/models/${modelId}/retrain`, { headers: authHeaders, data: duplicateBodies }),
  ]);

  const registerResponse = await page.request.post(`${baseUrl}/api/v1/models/${modelId}/versions`, {
    headers: authHeaders,
    data: {
      model_id: noGateModelId, tenant_id: 'cross-tenant-body', model_type: 'xgboost', version: registeredVersion,
      artifact_uri: 's3://acceptance/registered.onnx', feature_set_id: psql('SELECT feature_set_id FROM feature_sets ORDER BY feature_set_id LIMIT 1'),
      metrics: { f1_score: 0.98 }, status: 'registered',
    },
  });
  const registeredUnderPath = await page.request.get(`${baseUrl}/api/v1/models/${modelId}/versions/${registeredVersion}`, { headers: authHeaders });
  const rejectedUnderWrongPath = await page.request.get(`${baseUrl}/api/v1/models/${noGateModelId}/versions/${registeredVersion}`, { headers: authHeaders });
  const missingPathRegisterResponse = await page.request.post(`${baseUrl}/api/v1/models/${missingPathModelId}/versions`, {
    headers: authHeaders,
    data: {
      model_id: 'body-must-not-create', tenant_id: 'default', model_type: 'xgboost', version: `missing-path-${suffix}`,
      artifact_uri: 's3://acceptance/missing-path.onnx', feature_set_id: psql('SELECT feature_set_id FROM feature_sets ORDER BY feature_set_id LIMIT 1'),
      metrics: { f1_score: 0.98 }, status: 'registered',
    },
  });

  const rollbackResponse = await page.request.post(`${baseUrl}/api/v1/models/${modelId}/versions/${rollbackVersion}/rollback`, {
    headers: authHeaders, data: { target_version: rollbackVersion, reason: 'isolated state-machine rollback acceptance' },
  });
  const rollbackPayload = await rollbackResponse.json().catch(() => ({}));
  const rollbackJobId = String(rollbackPayload?.data?.job_id ?? '');
  let rollbackJob;
  for (let attempt = 0; attempt < 90; attempt += 1) {
    const response = await page.request.get(`${baseUrl}/api/v1/models/${modelId}/workbench`, { headers: authHeaders });
    const payload = await response.json();
    rollbackJob = payload?.data?.actions?.find((item) => item.job_id === rollbackJobId);
    if (rollbackJob && ['completed', 'failed'].includes(rollbackJob.status)) break;
    await page.waitForTimeout(1000);
  }

  const stateRows = psql(`SELECT model_version || ':' || status FROM model_versions WHERE model_id = '${modelId}'::uuid ORDER BY model_version`).split('\n').filter(Boolean);
  const rollbackAudits = psql(`SELECT action FROM audit_logs WHERE object_id IN ('${modelId}', '${rollbackVersion}') ORDER BY created_at`).split('\n').filter(Boolean);
  const rollbackOutbox = psql(`SELECT event_id || ':' || status || ':' || action_job_id FROM model_update_outbox WHERE action_job_id = '${rollbackJobId}' ORDER BY id`).split('\n').filter(Boolean);
  const rollbackAppliedAcks = psql(`SELECT event_id || ':' || status || ':' || subtask_index || ':' || parallelism || ':' || artifact_sha256 FROM model_update_applied_acks WHERE model_id = '${modelId}' ORDER BY subtask_index`).split('\n').filter(Boolean);
  const rollbackCompletionEvidence = psql(`SELECT COALESCE(detail->>'event_id','') || ':' || COALESCE(detail->>'data_plane_applied','') || ':' || COALESCE(detail->>'applied_subtasks','') || ':' || COALESCE(detail->>'expected_subtasks','') || ':' || COALESCE(detail->>'reason','') FROM audit_logs WHERE object_id = '${modelId}' AND action = 'MODEL_VERSION_ROLLBACK_COMPLETED' ORDER BY created_at DESC LIMIT 1`);
  const retrainStatuses = retrainResponses.map((response) => response.status()).sort((a, b) => a - b);
  const checks = {
    stagedRegistryActivationRejected: grayResponse.status() === 400 && /requires gray_percent=100/.test(JSON.stringify(grayPayload)),
    emptyGatesFailClosed: noGateResponse.status() === 409 && /no persisted review gates/.test(JSON.stringify(noGatePayload)),
    inlineSamplesRejected: inlineFeedbackResponse.status() === 400,
    duplicateRetrainRejected: retrainStatuses.length === 2 && retrainStatuses[0] === 202 && retrainStatuses[1] === 409,
    registrationPathAuthority: registerResponse.ok() && registeredUnderPath.ok() && rejectedUnderWrongPath.status() === 404,
    missingUuidPathRejectedWithoutCreation: missingPathRegisterResponse.status() === 404 && psql(`SELECT COUNT(*) FROM models WHERE model_id = '${missingPathModelId}'::uuid OR name IN ('${missingPathModelId}', 'body-must-not-create')`) === '0',
    rollbackAccepted: rollbackResponse.status() === 202 && Boolean(rollbackJobId),
    rollbackCompletedAfterDataPlaneApply: rollbackJob?.status === 'completed' && rollbackOutbox.length === 1
      && rollbackOutbox[0].includes(':published:') && rollbackAppliedAcks.length === 4
      && rollbackAppliedAcks.every((row) => row.includes(':applied:') && row.endsWith(`:${fixtureSha256}`))
      && rollbackCompletionEvidence.includes(':true:4:4:isolated state-machine rollback acceptance'),
    rollbackStateTransition: stateRows.includes(`${rollbackVersion}:active`) && stateRows.includes(`${currentVersion}:deprecated`),
    rollbackAuditClosed: rollbackAudits.includes('MODEL_VERSION_ROLLBACK_REQUESTED')
      && rollbackAudits.includes('MODEL_VERSION_ROLLBACK_APPLIED')
      && rollbackAudits.includes('MODEL_VERSION_ROLLBACK_COMPLETED'),
  };
  const result = Object.values(checks).every(Boolean) ? 'pass' : 'fail';
  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, `${JSON.stringify({
    result, browser_path: 'Xshell tunnel -> 127.0.0.1:9224 -> Windows Chrome -> direct APISIX', browser: version.Browser,
    deployment_images: execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'deploy', 'rule-manager', 'web-ui', '-o', 'jsonpath={range .items[*]}{.metadata.name}={.spec.template.spec.containers[0].image}{"\\n"}{end}'], { encoding: 'utf8', env: process.env }).trim().split('\n'),
    isolated_fixture: { model_id: modelId, no_gate_model_id: noGateModelId, missing_path_model_id: missingPathModelId, cleaned: false },
    checks, responses: { gray: grayResponse.status(), no_gate: noGateResponse.status(), inline_feedback: inlineFeedbackResponse.status(), duplicate_retrain: retrainStatuses, register: registerResponse.status(), wrong_path_get: rejectedUnderWrongPath.status(), missing_path_register: missingPathRegisterResponse.status(), rollback: rollbackResponse.status() },
    rollback: { job_id: rollbackJobId, terminal_status: rollbackJob?.status ?? 'missing', model_states: stateRows, audits: rollbackAudits, outbox: rollbackOutbox, applied_acks: rollbackAppliedAcks, artifact_uri: fixtureArtifactUri, artifact_sha256: fixtureSha256, completion_evidence: rollbackCompletionEvidence },
    completed_at: new Date().toISOString(),
  }, null, 2)}\n`);
  console.log(JSON.stringify({ outputPath, result, checks }, null, 2));
  if (result !== 'pass') process.exitCode = 1;
} finally {
  try {
    cleanup();
    cleanupSucceeded = true;
  } catch (error) {
    cleanupError = error instanceof Error ? error.message : String(error);
  }
  try {
    kubectl(['-n', 'minio', 'exec', fixturePod, '--', 'mc', 'rm', '--recursive', '--force',
      `acceptance/traffic-models/${fixturePrefix}`], { timeout: 30_000 });
  } catch {}
  try {
    kubectl(['-n', 'minio', 'delete', 'pod', fixturePod, '--ignore-not-found=true', '--wait=false']);
  } catch {}
  if (browser) await browser.close();
  if (fs.existsSync(outputPath)) {
    const evidence = JSON.parse(fs.readFileSync(outputPath, 'utf8'));
    evidence.isolated_fixture.cleaned = cleanupSucceeded;
    evidence.isolated_fixture.cleanup_error = cleanupError;
    if (!cleanupSucceeded) {
      evidence.result = 'fail';
      evidence.checks.fixtureCleanup = false;
      process.exitCode = 1;
    } else {
      evidence.checks.fixtureCleanup = true;
    }
    fs.writeFileSync(outputPath, `${JSON.stringify(evidence, null, 2)}\n`);
  }
}
