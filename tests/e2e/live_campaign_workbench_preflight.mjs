#!/usr/bin/env node
import crypto from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';

const root = process.cwd();
const baseUrl = process.env.BASE_URL || 'http://10.0.5.8:30180';
const outputPath = path.join(root, 'doc/02_acceptance/02-regression/campaign-workbench-live-preflight-latest.json');
for (const key of ['HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'http_proxy', 'https_proxy', 'all_proxy']) delete process.env[key];
process.env.NO_PROXY = '127.0.0.1,localhost,10.0.5.8';

const secret = Buffer.from(execFileSync('kubectl', ['-n', 'traffic-analysis', 'get', 'secret', 'traffic-credentials', '-o', 'jsonpath={.data.JWT_SECRET}'], { encoding: 'utf8', env: process.env }), 'base64').toString('utf8');
function token({ tenantId = 'default', permissions = ['*', 'admin:*', 'alert:*', 'graph:read', 'playbook:execute'], username = 'codex-campaign-acceptance' } = {}) {
  const now = Math.floor(Date.now() / 1000);
  const header = Buffer.from(JSON.stringify({ alg: 'HS256', typ: 'JWT' })).toString('base64url');
  const claims = Buffer.from(JSON.stringify({
    iss: 'traffic-auth-service', sub: crypto.randomUUID(), jti: crypto.randomUUID(), user_id: crypto.randomUUID(),
    tenant_id: tenantId, username, roles: permissions.includes('*') ? ['admin'] : ['viewer'], permissions,
    token_type: 'access', session_id: crypto.randomUUID(), iat: now, exp: now + 1800,
  })).toString('base64url');
  const input = `${header}.${claims}`;
  return `${input}.${crypto.createHmac('sha256', secret).update(input).digest('base64url')}`;
}

const adminToken = token();
const checks = [];
const check = (name, passed, detail) => checks.push({ name, passed: Boolean(passed), detail });
async function request(url, options = {}) {
  const response = await fetch(new URL(url, baseUrl), {
    ...options,
    headers: { Authorization: `Bearer ${adminToken}`, ...(options.headers || {}) },
  });
  const body = await response.json().catch(() => ({}));
  return { response, body, data: body.data ?? body };
}
function sql(query) {
  return execFileSync('kubectl', ['-n', 'databases', 'exec', 'postgres-primary-0', '--', 'psql', '-U', 'postgres', '-d', 'traffic_platform', '-Atc', query], { encoding: 'utf8', env: process.env }).trim();
}
const escapeSQL = (value) => String(value).replaceAll("'", "''");

const list = await request('/api/v1/campaigns?limit=8&offset=0');
const campaigns = list.data.campaigns ?? [];
const campaign = campaigns.find((item) => String(item.campaign_id || '').startsWith('campaign-')) ?? campaigns[0];
const campaignId = String(campaign?.campaign_id ?? '');
check('campaign list HTTP 200', list.response.status === 200, list.response.status);
check('campaign list uses real ClickHouse rows', campaigns.length === 8 && campaignId.length > 0, { count: campaigns.length, campaign_id: campaignId });
check('campaign list total supports server pagination', Number(list.data.total) >= campaigns.length, list.data.total);
check('campaign DTO exposes operational state contract', campaign && 'assignee' in campaign && 'state_version' in campaign, { assignee: campaign?.assignee, state_version: campaign?.state_version });

const phaseList = await request('/api/v1/campaigns?limit=5&phase=exfiltration');
const phaseRows = phaseList.data.campaigns ?? [];
check('phase filter HTTP 200', phaseList.response.status === 200, phaseList.response.status);
check('phase filter is result-level exact', phaseRows.length > 0 && phaseRows.every((item) => item.attack_phases?.includes('exfiltration')), phaseRows.map((item) => item.attack_phases));

const assignment = await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    action_id: 'campaign-assign-owner', target: campaignId,
    metadata: { campaign_id: campaignId, assignee: 'campaign_acceptance', dry_run: false },
    simulation: false, dry_run: false,
  }),
});
check('owner assignment HTTP 200', assignment.response.status === 200, assignment.response.status);
check('owner assignment is a real mutation', assignment.data.simulation === false && assignment.data.dry_run === false && assignment.data.status === 'completed', assignment.data);
check('owner assignment returns versioned result', assignment.data.result?.assignee === 'campaign_acceptance' && Number(assignment.data.result?.state_version) >= 1, assignment.data.result);

const afterAssignment = await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}`);
check('detail reflects persisted assignee', afterAssignment.data.assignee === 'campaign_acceptance', afterAssignment.data.assignee);
const phaseSummaries = afterAssignment.data.phase_summaries ?? [];
check(
  'detail exposes selected-campaign phase aggregation',
  Array.isArray(phaseSummaries)
    && phaseSummaries.length > 0
    && typeof afterAssignment.data.phase_data_backed === 'boolean'
    && phaseSummaries.every((item) => typeof item.phase === 'string' && Number(item.alert_count) >= 0 && Number(item.evidence_count) >= 0),
  { phase_data_backed: afterAssignment.data.phase_data_backed, phase_summaries: phaseSummaries },
);
check(
  'detail separates activity and workflow status',
  typeof afterAssignment.data.activity_status === 'string'
    && afterAssignment.data.activity_status.length > 0
    && typeof afterAssignment.data.status === 'string'
    && afterAssignment.data.status.length > 0,
  { activity_status: afterAssignment.data.activity_status, workflow_status: afterAssignment.data.status },
);
const assignmentVersion = Number(afterAssignment.data.state_version);

const statusChange = await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    action_id: 'campaign-status-change', target: campaignId,
    metadata: { campaign_id: campaignId, next_status: 'contained', dry_run: false },
    simulation: false, dry_run: false,
  }),
});
check('status change HTTP 200', statusChange.response.status === 200, statusChange.response.status);
check('status change returns new operational state', statusChange.data.result?.campaign_status === 'contained', statusChange.data.result);
check('status change increments state_version', Number(statusChange.data.result?.state_version) > assignmentVersion, { before: assignmentVersion, after: statusChange.data.result?.state_version });

const afterStatus = await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}`);
check('detail reflects contained state', afterStatus.data.status === 'contained', afterStatus.data.status);

const report = await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    action_id: 'campaign-report-generate', target: '战役复盘报告',
    metadata: { campaign_id: campaignId, format: 'pdf', sections: ['攻击阶段', '影响范围', '证据链', '处置结论'], evidence_count: 5, dry_run: false },
    simulation: false, dry_run: false,
  }),
});
const reportId = String(report.data.result?.report_id ?? '');
check('report generation HTTP 200', report.response.status === 200, report.response.status);
check('report generation returns durable report ID', reportId.startsWith('campaign-report-') && report.data.result?.report_status === 'completed', report.data.result);

const job = await request(`/api/v1/campaigns/jobs/${encodeURIComponent(statusChange.data.job_id)}`);
check('action job can be read back', job.response.status === 200 && job.data.job_id === statusChange.data.job_id && job.data.status === 'completed', job.data);
check('action job records non-simulation mutation', job.data.simulation === false && job.data.dry_run === false, { simulation: job.data.simulation, dry_run: job.data.dry_run });

const viewerResponse = await fetch(new URL(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, baseUrl), {
  method: 'POST', headers: { Authorization: `Bearer ${token({ permissions: ['alert:read'], username: 'campaign-viewer' })}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ action_id: 'campaign-status-change', target: campaignId, metadata: { campaign_id: campaignId, next_status: 'closed', dry_run: false }, simulation: false, dry_run: false }),
});
check('viewer cannot mutate campaigns', viewerResponse.status === 403, viewerResponse.status);

const crossTenant = await fetch(new URL(`/api/v1/campaigns/${encodeURIComponent(campaignId)}`, baseUrl), {
  headers: { Authorization: `Bearer ${token({ tenantId: 'campaign-isolation-check', permissions: ['alert:read'], username: 'campaign-isolation' })}` },
});
check('campaign detail is tenant isolated', crossTenant.status === 404, crossTenant.status);

const escapedId = escapeSQL(campaignId);
const stateRow = sql(`SELECT assignee || '|' || status || '|' || state_version FROM campaign_workbench_state WHERE tenant_id='default' AND campaign_id='${escapedId}'`);
check('PostgreSQL contains versioned workbench state', /^campaign_acceptance\|contained\|\d+$/.test(stateRow), stateRow);
const reportRow = sql(`SELECT report_id || '|' || status || '|' || format FROM campaign_reports WHERE tenant_id='default' AND report_id='${escapeSQL(reportId)}'`);
check('PostgreSQL contains completed report', reportRow === `${reportId}|completed|pdf`, reportRow);
const persistedJobs = sql(`SELECT count(*) FROM campaign_action_jobs WHERE tenant_id='default' AND job_id IN ('${escapeSQL(assignment.data.job_id)}','${escapeSQL(statusChange.data.job_id)}','${escapeSQL(report.data.job_id)}') AND simulation=false AND dry_run=false AND status='completed'`);
check('all mutating jobs are durable', persistedJobs === '3', persistedJobs);

const audit = await request('/api/v1/audit/logs?object_type=campaign&limit=100');
const auditRows = audit.data.trails ?? [];
const jobIds = new Set([assignment.data.job_id, statusChange.data.job_id, report.data.job_id]);
const auditedJobIds = new Set(auditRows.map((row) => row.details?.job_id).filter((id) => jobIds.has(id)));
check('all mutating jobs have audit trails', auditedJobIds.size === 3, [...auditedJobIds]);

// Leave the synthetic campaign in a normal operator-owned state while retaining
// the append-only action and audit evidence produced above.
await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ action_id: 'campaign-assign-owner', target: campaignId, metadata: { campaign_id: campaignId, assignee: 'sec_analyst', dry_run: false }, simulation: false, dry_run: false }),
});
await request(`/api/v1/campaigns/${encodeURIComponent(campaignId)}/actions`, {
  method: 'POST', headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ action_id: 'campaign-status-change', target: campaignId, metadata: { campaign_id: campaignId, next_status: 'active', dry_run: false }, simulation: false, dry_run: false }),
});

const result = checks.every((item) => item.passed) ? 'pass' : 'fail';
const output = {
  run_id: `campaign-workbench-r654-${Date.now()}`,
  result,
  base_url: baseUrl,
  campaign_id: campaignId,
  check_count: checks.length,
  passed: checks.filter((item) => item.passed).length,
  failed: checks.filter((item) => !item.passed).length,
  checks,
  timestamp: new Date().toISOString(),
};
fs.mkdirSync(path.dirname(outputPath), { recursive: true });
fs.writeFileSync(outputPath, `${JSON.stringify(output, null, 2)}\n`);
console.log(JSON.stringify(output, null, 2));
process.exit(result === 'pass' ? 0 : 1);
