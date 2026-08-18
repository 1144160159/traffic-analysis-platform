import { http, HttpResponse } from 'msw';
import { buildPageSnapshot } from '@/services/mockData';
import { findRouteById } from '@/routes/routeManifest';

const authHandlers = [
  http.post('/api/v1/auth/login', async () =>
    HttpResponse.json({
      access_token: 'mock-token-sec-analyst',
      refresh_token: 'mock-refresh-token',
      expires_in: 3600,
      token_type: 'Bearer',
      user: {
        user_id: 'mock-user-sec-analyst',
        tenant_id: 'default',
        username: 'sec_analyst',
        email: 'sec_analyst@example.local',
        roles: ['admin'],
        permissions: ['*'],
      },
    }),
  ),
  http.get('/api/v1/auth/me', () =>
    HttpResponse.json({
      user_id: 'mock-user-sec-analyst',
      tenant_id: 'default',
      username: 'sec_analyst',
      email: 'sec_analyst@example.local',
      roles: ['admin'],
      permissions: ['*'],
    }),
  ),
  http.post('/api/v1/auth/logout', () => HttpResponse.json({ message: 'Logged out successfully' })),
  http.get('/api/v1/ui/pages/:pageId', ({ params }) => {
    const pageId = String(params.pageId);
    const route = findRouteById(pageId);
    if (!route) return new HttpResponse(null, { status: 404 });
    return HttpResponse.json(buildPageSnapshot(route.page));
  }),
];

// —— 统一分析任务调度中心(§20;snake_case 契约与后端一致;Go 风格键处与现契约一致)——
const analysisDefs = [
  { id: 'd2d479ee-2e20-42c6-b4eb-1d4c1921d496', name: 'api-surface-live-demo', state: 'SUSPENDED', owner: 'default', default_scheduling_class: 'INTERACTIVE', revision: 3, active_plan_revision: null, active_schedule_revision: null, report_policy_revision: 1, created_at: '2026-08-18T09:00:00Z', updated_at: '2026-08-18T09:00:00Z' },
  { id: '4104a7bf-133b-4800-a27b-00dffd6f6134', name: 'def-live-veth-replay', state: 'ACTIVE', owner: 'default', default_scheduling_class: 'BASELINE', revision: 2, active_plan_revision: 1, active_schedule_revision: null, report_policy_revision: 1, created_at: '2026-08-17T08:58:25Z', updated_at: '2026-08-18T01:00:00Z' },
  { id: '43e642d1-3639-487b-85bb-e108d2f67585', name: 'def-live-replay-probe82', state: 'ACTIVE', owner: 'default', default_scheduling_class: 'BASELINE', revision: 2, active_plan_revision: 3, active_schedule_revision: null, report_policy_revision: null, created_at: '2026-08-17T09:00:00Z', updated_at: '2026-08-18T01:00:00Z' },
];
const analysisRuns = [
  { tenant_id: 'default', run_id: '14a42fa5-f184-4372-930a-f21e7f339f8f', task_id: 'cac71428-5d37-4a91-9672-d517e7dbe116', execution_spec_sha256: '3937277f4026d71ace95ea51b7a970f919064a10f56da4712b3ade55783299aa', state: 'SUCCEEDED', completeness: 'COMPLETE', integrity_state: 'VERIFIED', finding_conclusion: 'THREAT_FOUND', risk_severity: 'MEDIUM', report_state: 'AVAILABLE', window_start_ms: 1787009280000, window_end_ms: 1787011080000, revision: 6, created_at: '2026-08-18T00:00:00Z', finalized_at: '2026-08-18T00:05:00Z' },
  { tenant_id: 'default', run_id: '140aba85-4167-4f62-96c8-c0f57d638560', task_id: 'cac71428-5d37-4a91-9672-d517e7dbe116', execution_spec_sha256: '3937277f4026d71ace95ea51b7a970f919064a10f56da4712b3ade55783299aa', state: 'RUNNING', completeness: 'UNKNOWN', integrity_state: 'UNVERIFIED', finding_conclusion: 'NOT_EVALUATED', risk_severity: 'UNKNOWN', report_state: 'NOT_REQUESTED', window_start_ms: 1787010400000, window_end_ms: 1787011000000, revision: 3, created_at: '2026-08-18T00:26:00Z', finalized_at: null },
];
const analysisStages = [
  { business_phase_id: 'S1', execution_node_id: 'SOURCE_ACTIVATE', attempt: 1, state: 'SUCCEEDED', provider_mode: 'DEDICATED_OPERATION', activation_mode: 'PIPELINED_STREAM', skip_reason: '' },
  { business_phase_id: 'S2', execution_node_id: 'SESSIONIZATION', attempt: 1, state: 'RUNNING', provider_mode: 'SHARED_STREAM', activation_mode: 'PIPELINED_STREAM', skip_reason: '' },
  { business_phase_id: 'S5', execution_node_id: 'RECONCILE', attempt: 1, state: 'PENDING', provider_mode: 'AUTHORITY_LOCAL', activation_mode: 'AUTHORITY_LOCAL', skip_reason: '' },
];
export const analysisMockHandlers = [
  http.get('/api/v1/analysis/tasks', () => HttpResponse.json({ success: true, data: analysisDefs, meta: { count: analysisDefs.length } })),
  http.get('/api/v1/analysis/runs', () => HttpResponse.json({ success: true, data: analysisRuns, meta: { count: analysisRuns.length } })),
  http.get('/api/v1/analysis/runs/:runId', ({ params }) => {
    const run = analysisRuns.find((r) => r.run_id === params.runId) ?? analysisRuns[0];
    return HttpResponse.json({ success: true, data: { ...run, stages: analysisStages } });
  }),
  http.get('/api/v1/analysis/runs/:runId/allowed-actions', ({ params }) =>
    HttpResponse.json({ success: true, data: { run_id: params.runId, state: 'RUNNING', cancel: true, retry_stage: false, retry_task: false, request_report: false } })),
  http.get('/api/v1/analysis/schedules', () =>
    HttpResponse.json({ success: true, data: [
      { ScheduleID: '5ed955f9-e85d-4fa9-9a77-60be155cff23', TaskDefinitionID: '4104a7bf-133b-4800-a27b-00dffd6f6134', Revision: 1, ApprovedPlanRevision: 1, ExecutionSpecSHA256: '3937277f4026d71ace95ea51b7a970f919064a10f56da4712b3ade55783299aa', TriggerKind: 'CRON_WINDOW', WindowOrCron: '{"cron":"* * * * *","window_ms":45000}', MisfirePolicy: 'MISFIRE_FAIL', ConcurrencyPolicy: 'FORBID_OVERLAP', SchedulingClass: 'BASELINE', HeadState: 'PAUSED', AuthorityRevision: 4, CreatedAt: '2026-08-18T00:00:00Z' },
      { ScheduleID: '918e558a-7507-4b32-8bae-e8ba25cc8d86', TaskDefinitionID: '43e642d1-3639-487b-85bb-e108d2f67585', Revision: 2, ApprovedPlanRevision: 3, ExecutionSpecSHA256: '114d62e80825c54eb2d198c4c101e18daeb031a7e8bc6e404810d365b57bf83b', TriggerKind: 'CONTINUOUS_WINDOW', WindowOrCron: '{"window_ms":60000}', MisfirePolicy: 'MISFIRE_FAIL', ConcurrencyPolicy: 'FORBID_OVERLAP', SchedulingClass: 'BASELINE', HeadState: 'DRAFT', AuthorityRevision: 0, CreatedAt: '2026-08-18T01:14:00Z' },
    ] })),
  http.get('/api/v1/analysis/reports', () =>
    HttpResponse.json({ success: true, data: [
      { ReportID: '8c0b43c4-72db-44b5-9b9d-594c1b0c1747', RunID: '140aba85-4167-4f62-96c8-c0f57d638560', SummarySHA256: 'sum-sha', TemplateRevision: 'default-v1', Locale: 'zh-CN', State: 'AVAILABLE', ObjectKey: 'reports/default/140aba85-4167-4f62-96c8-c0f57d638560/8c0b43c4-72db-44b5-9b9d-594c1b0c1747.html', ObjectSHA256: '999ab7b7', ObjectSize: 1900, CreatedAt: '2026-08-18T01:00:00Z' },
    ] })),
  http.get('/api/v1/analysis/resources', () =>
    HttpResponse.json({ success: true, data: {
      reservations: [{ run_id: '140aba85-4167-4f62-96c8-c0f57d638560', resource_pool: 'analysis-cpu', state: 'CONSUMED', epoch: 1, expires_at: '2026-08-18T02:00:00Z' }],
      drr: [{ tenant_id: 'default', scheduling_class: 'BASELINE', deficit: 1000, quantum: 1000, last_served_at: '2026-08-18T01:00:00Z', scheduler_epoch: 5 }],
      outbox_ledger: [{ topic: 'analysis.run.events.v1', state: 'PUBLISHED', count: 118, oldest_pending_age_seconds: 0 }, { topic: 'analysis.report.requests.v1', state: 'PUBLISHED', count: 4, oldest_pending_age_seconds: 0 }],
      queue: [{ scheduling_class: 'BASELINE', state: 'RUNNING', count: 1 }],
    } })),
  http.get('/api/v1/analysis/task-definitions/:taskDefinitionId', ({ params }) => {
    const def = analysisDefs.find((d) => d.id === params.taskDefinitionId) ?? analysisDefs[0];
    return HttpResponse.json({ success: true, data: {
      ...def,
      plans: [{ plan_id: '03947378-03de-4dfa-ad86-3a28fc819dd5', plan_revision: 2, plan_source: 'MANUAL_CUSTOM', source_kind: 'PCAP_REPLAY', execution_spec_sha256: '2777f3ab94e32c3be0d65f8db5fbe4c02a287dba77a2af0a20c542f53de28790', governance_state: 'ACTIVE', created_at: '2026-08-17T08:59:34Z' }],
      schedules: [],
      report_policies: [{ policy_id: '6229e639-a68c-41d2-99de-520256e185f0', revision: 1, mode: 'AUTO_ASYNC', template_revision: 'default-v1', locale: 'zh-CN', retention_days: 30, policy_sha256: 'pol-sha' }],
      audit_records: [
        { action: 'CREATED', actor: 'default', detail: '{"name":"api-surface-live-demo"}', created_at: '2026-08-17T23:00:00Z' },
        { action: 'ACTIVATED', actor: 'default', detail: '{"authority_revision":2}', created_at: '2026-08-17T23:01:00Z' },
        { action: 'SUSPENDED', actor: 'default', detail: '{"authority_revision":3}', created_at: '2026-08-17T23:02:00Z' },
      ],
    } });
  }),
  http.get('/api/v1/analysis/task-definitions/:taskDefinitionId/allowed-actions', ({ params }) =>
    HttpResponse.json({ success: true, data: { task_definition_id: params.taskDefinitionId, state: 'SUSPENDED', revision: 3, activate: false, suspend: false } })),
  http.post('/api/v1/analysis/triggers/preflight', () =>
    HttpResponse.json({ success: true, data: { execution_spec_sha256: '3937277f4026d71ace95ea51b7a970f919064a10f56da4712b3ade55783299aa', canonical_spec: {}, source_kind: 'PCAP_REPLAY', compatible: true } })),
];

// auth 与 analysis mock 处理器合并为统一 handlers
export const handlers = [...authHandlers, ...analysisMockHandlers];
