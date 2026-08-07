import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { fetchDashboardTask, submitDashboardTask } from './dashboardTaskApi';

describe('dashboardTaskApi', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('submits a real task without client tenant identity and preserves the receipt', async () => {
    const post = vi.spyOn(api, 'post').mockResolvedValue({ data: { data: {
      task_id: 'task-1', job_id: 'task-1', event_id: 'event-1', action_id: 'dashboard-evidence-task-create',
      status: 'accepted', revision: 1, snapshot_id: 'snapshot-1', trace_id: 'trace-1',
      idempotency_key: 'dashboard:key', outbox_status: 'pending', replayed: false,
    } } } as never);
    const result = await submitDashboardTask({
      actionId: 'dashboard-evidence-task-create', target: 'evidence-gap', priority: 'high',
      snapshotId: 'snapshot-1', reason: '补齐当前证据缺口', context: { count: 3 },
    });
    expect(post).toHaveBeenCalledWith('/v1/dashboard/tasks/evidence', {
      target: 'evidence-gap', priority: 'high', snapshot_id: 'snapshot-1', reason: '补齐当前证据缺口', context: { count: 3 },
    }, { headers: {
      'X-Action-ID': 'dashboard-evidence-task-create',
      'Idempotency-Key': expect.stringMatching(/^dashboard:dashboard-evidence-task-create:request-fingerprint:/),
    } });
    expect(post).not.toHaveBeenCalledWith(expect.anything(), expect.objectContaining({ tenant_id: expect.anything() }), expect.anything());
    expect(result).toMatchObject({ taskId: 'task-1', jobId: 'task-1', status: 'accepted', traceId: 'trace-1' });
  });

  it('loads authoritative task status by escaped task id', async () => {
    const get = vi.spyOn(api, 'get').mockResolvedValue({ data: { data: {
      task_id: 'task/1', job_id: 'task/1', action_id: 'dashboard-task-create', status: 'running', revision: 2,
      snapshot_id: 'snapshot-1', trace_id: 'trace-1', task_type: 'closure', target: 'dashboard', priority: 'high',
      reason: '运营闭环', requested_by: 'operator', context: {}, result: {}, created_at: '2026-08-03T00:00:00Z', updated_at: '2026-08-03T00:01:00Z',
    } } } as never);
    const task = await fetchDashboardTask('task/1');
    expect(get).toHaveBeenCalledWith('/v1/dashboard/tasks/task%2F1');
    expect(task).toMatchObject({ taskId: 'task/1', status: 'running', revision: 2, taskType: 'closure' });
  });

  it('rejects incomplete receipts', async () => {
    vi.spyOn(api, 'post').mockResolvedValue({ data: { data: { status: 'accepted' } } } as never);
    await expect(submitDashboardTask({
      actionId: 'dashboard-task-create', target: 'dashboard', priority: 'high', snapshotId: 'snapshot-1', reason: '创建闭环任务',
    })).rejects.toThrow('缺少稳定');
  });
});
