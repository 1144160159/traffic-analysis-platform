#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = path.resolve(path.dirname(new URL(import.meta.url).pathname), '../..');
const runId = process.env.RUN_ID || `${new Date().toISOString().replace(/[-:TZ.]/g, '').slice(0, 14)}-notification-workbench-r429`;
const apiBase = (process.env.APISIX || 'http://10.0.5.8:30180').replace(/\/$/, '');
const outputDir = path.join(root, 'doc/02_acceptance/runs', runId);
const regressionDir = path.join(root, 'doc/02_acceptance/02-regression');
fs.mkdirSync(outputDir, { recursive: true });
fs.mkdirSync(regressionDir, { recursive: true });

const cleanEnv = { ...process.env };
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete cleanEnv[key];

const kubectl = (args, options = {}) => execFileSync('kubectl', args, { encoding: 'utf8', env: cleanEnv, ...options }).trim();
const secret = (key) => Buffer.from(kubectl(['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', `jsonpath={.data.${key}}`]), 'base64').toString();
const jwtSecret = secret('JWT_SECRET');
const pgPassword = secret('PG_PASSWORD');
const suffix = runId.replace(/[^a-zA-Z0-9-]/g, '-').slice(-48);
const checks = [];
const artifacts = {};

const base64url = (value) => Buffer.from(value).toString('base64url');
const token = (tenant, role, permissions) => {
  const now = Math.floor(Date.now() / 1000);
  const header = base64url(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const payload = base64url(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: tenant, username: `codex-notification-${role}`, roles: [role], permissions,
    token_type: 'access', iat: now, exp: now + 1800,
  }));
  const signature = crypto.createHmac('sha256', jwtSecret).update(`${header}.${payload}`).digest('base64url');
  return `${header}.${payload}.${signature}`;
};

const adminToken = token('default', 'admin', ['admin:*', 'audit:read']);
const viewerToken = token('default', 'viewer', ['user:read']);
const otherTenantToken = token('tenant-b', 'admin', ['admin:*', 'audit:read']);

const record = (phase, name, passed, detail, artifact = '') => {
  checks.push({ phase, name, severity: passed ? 'info' : 'blocker', passed, detail, artifact });
  process.stdout.write(`${passed ? 'PASS' : 'FAIL'} ${phase} ${name}: ${detail}\n`);
};

const save = (name, payload) => {
  const file = path.join(outputDir, name);
  fs.writeFileSync(file, `${JSON.stringify(payload, null, 2)}\n`);
  artifacts[name] = payload;
  return name;
};

const request = async (name, method, endpoint, auth, body, expected) => {
  let response;
  let payload;
  try {
    response = await fetch(`${apiBase}${endpoint}`, {
      method,
      headers: { Authorization: `Bearer ${auth}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    try { payload = JSON.parse(text); } catch { payload = { raw: text }; }
  } catch (error) {
    record('api', name, false, `request failed: ${error instanceof Error ? error.message : String(error)}`);
    throw error;
  }
  const artifact = save(`${name.replace(/[^a-z0-9-]/gi, '-')}.json`, { status: response.status, body: payload });
  record('api', name, response.status === expected, `HTTP ${response.status}, expected ${expected}`, artifact);
  return { status: response.status, payload };
};

const assert = (name, condition, detail, artifact = '') => record('assert', name, Boolean(condition), detail, artifact);

const psql = (sql) => kubectl(['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'env', `PGPASSWORD=${pgPassword}`, 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-v', 'ON_ERROR_STOP=1', '-Atc', sql]);

let originalSettings;
let createdRule;
let createdTemplate;
let createdPolicy;
let createdSilence;
let templateDelivery;
let channelDelivery;
let retriedOriginal;

try {
  const initial = await request('workbench-initial', 'GET', '/api/v1/notifications/workbench?limit=100', adminToken, undefined, 200);
  const workbench = initial.payload?.data;
  originalSettings = workbench?.settings;
  assert('workbench has database fixtures', workbench?.rules?.length >= 28 && workbench?.templates?.length >= 4 && workbench?.escalation_policies?.length >= 2 && workbench?.deliveries?.length >= 5 && workbench?.silence_rules?.length >= 3, `counts=${JSON.stringify({ rules: workbench?.rules?.length, templates: workbench?.templates?.length, policies: workbench?.escalation_policies?.length, deliveries: workbench?.deliveries?.length, silences: workbench?.silence_rules?.length })}`, 'workbench-initial.json');
  assert('workbench contains no plaintext channel secret', originalSettings?.secret_ref && !JSON.stringify(initial.payload).match(/webhook_token|api_key|password/i), `secret_ref=${originalSettings?.secret_ref || 'missing'}`, 'workbench-initial.json');

  await request('viewer-workbench-denied', 'GET', '/api/v1/notifications/workbench?limit=10', viewerToken, undefined, 403);
  await request('viewer-settings-denied', 'GET', '/api/v1/notifications/settings', viewerToken, undefined, 403);
  await request('viewer-rule-create-denied', 'POST', '/api/v1/notifications/subscriptions', viewerToken, { name: 'denied', channels: ['email'] }, 403);
  await request('invalid-channel-rejected', 'POST', '/api/v1/notifications/subscriptions', adminToken, { name: 'invalid', channels: ['carrier-pigeon'] }, 400);
  await request('invalid-escalation-rejected', 'POST', '/api/v1/notifications/escalation-policies', adminToken, { name: 'invalid', stages: [{ after_minutes: -1, target_role: '' }] }, 400);

  const ruleCreate = await request('rule-create', 'POST', '/api/v1/notifications/subscriptions', adminToken, { name: `验收通知规则 ${suffix}`, conditions: { severity: 'high', alert_type: '攻击告警', asset_scope: '核心资产', campus: '主园区', window_start: '00:00', window_end: '08:00', escalation_policy: '夜间升级策略', silence_mode: '维护窗口' }, channels: ['email', 'wechat'], enabled: true }, 201);
  createdRule = ruleCreate.payload?.data;
  assert('created rule has tenant identity', createdRule?.rule_id && createdRule?.tenant_id === 'default', `rule_id=${createdRule?.rule_id}`, 'rule-create.json');
  const rulePatch = await request('rule-disable', 'PATCH', `/api/v1/notifications/subscriptions/${encodeURIComponent(createdRule.rule_id)}`, adminToken, { enabled: false }, 200);
  assert('rule patch persisted disabled state', rulePatch.payload?.data?.enabled === false, `enabled=${rulePatch.payload?.data?.enabled}`, 'rule-disable.json');
  assert('rule partial patch preserves conditions and channels', rulePatch.payload?.data?.conditions?.severity === 'high' && rulePatch.payload?.data?.channels?.join(',') === 'email,wechat', `conditions=${JSON.stringify(rulePatch.payload?.data?.conditions)}, channels=${JSON.stringify(rulePatch.payload?.data?.channels)}`, 'rule-disable.json');
  await request('cross-tenant-rule-hidden', 'PATCH', `/api/v1/notifications/subscriptions/${encodeURIComponent(createdRule.rule_id)}`, otherTenantToken, { enabled: true }, 404);

  const templateCreate = await request('template-create', 'POST', '/api/v1/notifications/templates', adminToken, { template_type: '告警模板', name: `验收通知模板 ${suffix}`, subject: '[{{severity}}] {{title}}', body: '告警 {{alert_id}}', variable_schema: { required: ['severity', 'title', 'alert_id'] }, enabled: true }, 201);
  createdTemplate = templateCreate.payload?.data;
  const templatePatch = await request('template-update', 'PATCH', `/api/v1/notifications/templates/${encodeURIComponent(createdTemplate.template_id)}`, adminToken, { subject: '[更新] {{title}}' }, 200);
  assert('template update increments version', templatePatch.payload?.data?.version === createdTemplate.version + 1, `version=${createdTemplate.version}->${templatePatch.payload?.data?.version}`, 'template-update.json');
  assert('template partial patch preserves variable schema', templatePatch.payload?.data?.variable_schema?.required?.join(',') === 'severity,title,alert_id', JSON.stringify(templatePatch.payload?.data?.variable_schema), 'template-update.json');
  const templateTest = await request('template-test', 'POST', `/api/v1/notifications/templates/${encodeURIComponent(createdTemplate.template_id)}/test`, adminToken, undefined, 503);
  templateDelivery = templateTest.payload?.data?.delivery;
  assert('template test renders and persists truthful provider failure', Number(templateDelivery?.notification_id) > 0 && templateDelivery?.status === 'failed' && Boolean(templateDelivery?.error_message) && templateTest.payload?.data?.rendered_subject === '[更新] 通知模板验收告警' && templateTest.payload?.data?.rendered_body === `告警 template-test-${createdTemplate.template_id}`, `notification_id=${templateDelivery?.notification_id}, status=${templateDelivery?.status}, error=${templateDelivery?.error_message}`, 'template-test.json');

  const policyCreate = await request('escalation-create', 'POST', '/api/v1/notifications/escalation-policies', adminToken, { name: `验收升级策略 ${suffix}`, stages: [{ after_minutes: 5, condition: 'SLA 超时', target_role: '安全值班组' }, { after_minutes: 15, condition: '未确认', target_role: '安全管理组' }], enabled: true }, 201);
  createdPolicy = policyCreate.payload?.data;
  const policyPatch = await request('escalation-disable', 'PATCH', `/api/v1/notifications/escalation-policies/${encodeURIComponent(createdPolicy.policy_id)}`, adminToken, { enabled: false }, 200);
  assert('escalation patch persisted disabled state', policyPatch.payload?.data?.enabled === false, `enabled=${policyPatch.payload?.data?.enabled}`, 'escalation-disable.json');
  assert('escalation partial patch preserves stages', policyPatch.payload?.data?.stages?.length === 2, `stages=${JSON.stringify(policyPatch.payload?.data?.stages)}`, 'escalation-disable.json');

  const start = new Date(Date.now() + 2 * 86_400_000);
  const silenceCreate = await request('silence-create', 'POST', '/api/v1/notifications/silence-rules', adminToken, { name: `验收维护窗口 ${suffix}`, scope: '主园区', starts_at: start.toISOString(), ends_at: new Date(start.getTime() + 3_600_000).toISOString(), affected_targets: ['core-switch'], policy: '夜间升级策略', reason: 'r429 live preflight' }, 201);
  createdSilence = silenceCreate.payload?.data;
  const silencePatch = await request('silence-disable', 'PATCH', `/api/v1/notifications/silence-rules/${encodeURIComponent(createdSilence.rule_id)}`, adminToken, { enabled: false }, 200);
  assert('silence patch persisted disabled state', silencePatch.payload?.data?.enabled === false, `enabled=${silencePatch.payload?.data?.enabled}`, 'silence-disable.json');

  const changedChannels = { ...originalSettings.channels, email: !originalSettings.channels.email };
  const settingsPatch = await request('settings-partial-update', 'PUT', '/api/v1/notifications/settings', adminToken, { channels: changedChannels }, 200);
  assert('partial settings update preserves non-channel values', settingsPatch.payload?.data?.secret_ref === originalSettings.secret_ref && settingsPatch.payload?.data?.min_severity === originalSettings.min_severity && settingsPatch.payload?.data?.rate_limit_per_min === originalSettings.rate_limit_per_min, `secret_ref=${settingsPatch.payload?.data?.secret_ref}, severity=${settingsPatch.payload?.data?.min_severity}`, 'settings-partial-update.json');
  await request('settings-restore', 'PUT', '/api/v1/notifications/settings', adminToken, originalSettings, 200);

  const channelTest = await request('channel-test', 'POST', '/api/v1/notifications/test', adminToken, { channel: 'email', target: '安全值班组', alert_type: 'scan' }, 503);
  channelDelivery = channelTest.payload?.data;
  assert('channel test persists truthful provider failure', Number(channelDelivery?.notification_id) > 0 && channelDelivery?.channel === 'email' && channelDelivery?.status === 'failed' && Boolean(channelDelivery?.error_message), `notification_id=${channelDelivery?.notification_id}, status=${channelDelivery?.status}, error=${channelDelivery?.error_message}`, 'channel-test.json');

  const failedDelivery = workbench.deliveries.find((delivery) => delivery.status === 'failed');
  assert('fixture includes failed delivery for retry', Boolean(failedDelivery), `notification_id=${failedDelivery?.notification_id ?? 'missing'}`, 'workbench-initial.json');
  if (failedDelivery) {
    retriedOriginal = failedDelivery;
    const retry = await request('delivery-retry', 'POST', `/api/v1/notifications/deliveries/${failedDelivery.notification_id}/retry`, adminToken, undefined, 503);
    assert('retry attempts provider, preserves failure truth and updates trace', retry.payload?.data?.status === 'failed' && Boolean(retry.payload?.data?.error_message) && retry.payload?.data?.retry_count === failedDelivery.retry_count + 1 && retry.payload?.data?.trace_id !== failedDelivery.trace_id, `status=${retry.payload?.data?.status}, retry_count=${retry.payload?.data?.retry_count}, error=${retry.payload?.data?.error_message}`, 'delivery-retry.json');
  }

  const finalWorkbench = await request('workbench-after-actions', 'GET', '/api/v1/notifications/workbench?limit=200', adminToken, undefined, 200);
  assert('created objects are queryable together', finalWorkbench.payload?.data?.rules?.some((item) => item.rule_id === createdRule.rule_id) && finalWorkbench.payload?.data?.templates?.some((item) => item.template_id === createdTemplate.template_id) && finalWorkbench.payload?.data?.escalation_policies?.some((item) => item.policy_id === createdPolicy.policy_id) && finalWorkbench.payload?.data?.silence_rules?.some((item) => item.rule_id === createdSilence.rule_id), 'all created ids present in workbench', 'workbench-after-actions.json');

  const audits = await request('notification-audits', 'GET', '/api/v1/audit/logs?limit=200', adminToken, undefined, 200);
  const trails = audits.payload?.data?.trails ?? [];
  const actions = new Set(trails.map((trail) => trail.action));
  for (const action of ['NOTIFICATION_RULE_CREATED', 'NOTIFICATION_RULE_UPDATED', 'NOTIFICATION_TEMPLATE_CREATED', 'NOTIFICATION_TEMPLATE_UPDATED', 'NOTIFICATION_TEMPLATE_TEST_FAILED', 'NOTIFICATION_ESCALATION_CREATED', 'NOTIFICATION_ESCALATION_UPDATED', 'NOTIFICATION_SILENCE_RULE_CREATED', 'NOTIFICATION_SILENCE_RULE_UPDATED', 'NOTIFICATION_SETTINGS_UPDATED', 'NOTIFICATION_TEST_FAILED', 'NOTIFICATION_DELIVERY_RETRY_FAILED', 'NOTIFICATION_RULE_DB_INSERT', 'NOTIFICATION_RULE_DB_UPDATE', 'NOTIFICATION_TEMPLATE_DB_INSERT', 'NOTIFICATION_TEMPLATE_DB_UPDATE', 'NOTIFICATION_ESCALATION_DB_INSERT', 'NOTIFICATION_ESCALATION_DB_UPDATE', 'NOTIFICATION_SILENCE_RULE_DB_INSERT', 'NOTIFICATION_SILENCE_RULE_DB_UPDATE', 'NOTIFICATION_SETTINGS_DB_UPDATE', 'NOTIFICATION_DELIVERY_DB_INSERT', 'NOTIFICATION_DELIVERY_DB_UPDATE']) {
    assert(`audit contains ${action}`, actions.has(action), action, 'notification-audits.json');
  }

  const dbCounts = JSON.parse(psql(`SELECT json_build_object('rule',(SELECT count(*) FROM notification_rules WHERE tenant_id='default' AND rule_id='${createdRule.rule_id}'),'template',(SELECT count(*) FROM notification_templates WHERE tenant_id='default' AND template_id='${createdTemplate.template_id}'),'policy',(SELECT count(*) FROM notification_escalation_policies WHERE tenant_id='default' AND policy_id='${createdPolicy.policy_id}'),'silence',(SELECT count(*) FROM notification_silence_rules WHERE tenant_id='default' AND rule_id='${createdSilence.rule_id}'),'template_delivery',(SELECT count(*) FROM notification_history WHERE tenant_id='default' AND notification_id=${Number(templateDelivery.notification_id)}));`));
  save('postgres-counts.json', dbCounts);
  assert('PostgreSQL contains every created object', Object.values(dbCounts).every((count) => Number(count) === 1), JSON.stringify(dbCounts), 'postgres-counts.json');
} finally {
  const statements = [];
  if (templateDelivery?.notification_id) statements.push(`DELETE FROM notification_history WHERE tenant_id='default' AND notification_id=${Number(templateDelivery.notification_id)}`);
  if (channelDelivery?.notification_id) statements.push(`DELETE FROM notification_history WHERE tenant_id='default' AND notification_id=${Number(channelDelivery.notification_id)}`);
  if (retriedOriginal?.notification_id) statements.push(`UPDATE notification_history SET status='${String(retriedOriginal.status).replaceAll("'", "''")}', error_message=${retriedOriginal.error_message ? `'${String(retriedOriginal.error_message).replaceAll("'", "''")}'` : 'NULL'}, retry_count=${Number(retriedOriginal.retry_count)}, trace_id='${String(retriedOriginal.trace_id).replaceAll("'", "''")}', sent_at=${retriedOriginal.sent_at ? `'${String(retriedOriginal.sent_at).replaceAll("'", "''")}'` : 'NULL'} WHERE tenant_id='default' AND notification_id=${Number(retriedOriginal.notification_id)}`);
  if (createdRule?.rule_id) statements.push(`DELETE FROM notification_rules WHERE tenant_id='default' AND rule_id='${createdRule.rule_id}'`);
  if (createdTemplate?.template_id) statements.push(`DELETE FROM notification_templates WHERE tenant_id='default' AND template_id='${createdTemplate.template_id}'`);
  if (createdPolicy?.policy_id) statements.push(`DELETE FROM notification_escalation_policies WHERE tenant_id='default' AND policy_id='${createdPolicy.policy_id}'`);
  if (createdSilence?.rule_id) statements.push(`DELETE FROM notification_silence_rules WHERE tenant_id='default' AND rule_id='${createdSilence.rule_id.replaceAll("'", "''")}'`);
  if (statements.length) {
    try { psql(statements.join(';') + ';'); record('cleanup', 'created notification objects removed', true, `${statements.length} scoped cleanup statements`); }
    catch (error) { record('cleanup', 'created notification objects removed', false, error instanceof Error ? error.message : String(error)); }
  }
}

const passed = checks.filter((check) => check.passed).length;
const blockers = checks.filter((check) => !check.passed && check.severity === 'blocker').length;
const summary = { run_id: runId, result: blockers === 0 ? 'pass' : 'fail', api: apiBase, total: checks.length, passed, blockers, checks };
save(`live-notification-workbench-${runId}-summary.json`, summary);
fs.writeFileSync(path.join(regressionDir, 'notification-workbench-preflight-latest.json'), `${JSON.stringify(summary, null, 2)}\n`);
fs.writeFileSync(path.join(regressionDir, 'notification-workbench-preflight-latest.md'), `# Notification Workbench Preflight\n\n- run: \`${runId}\`\n- result: **${summary.result}**\n- checks: ${passed}/${checks.length}\n- blockers: ${blockers}\n- evidence: \`doc/02_acceptance/runs/${runId}\`\n`);
process.exitCode = blockers === 0 ? 0 : 1;
