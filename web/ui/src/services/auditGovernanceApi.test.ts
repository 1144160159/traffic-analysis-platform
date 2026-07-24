import { beforeEach, describe, expect, it, vi } from 'vitest';

const get = vi.fn();
const post = vi.fn();

vi.mock('@/services/api', () => ({ api: { get, post } }));

describe('audit governance API', () => {
  beforeEach(() => {
    get.mockReset();
    post.mockReset();
  });

  it('loads filtered tenant audit records without client-side simulation rows', async () => {
    get.mockResolvedValue({ data: { data: { trails: [{ log_id: 'audit-1' }], total: 1 } } });
    const api = await import('./auditGovernanceApi');
    const filters = { limit: 200, result: 'failure', request_id: 'req-1' };
    const result = await api.fetchAuditLogs(filters);
    expect(result.trails).toEqual([{ log_id: 'audit-1' }]);
    expect(get).toHaveBeenCalledWith('/v1/audit/logs', { params: filters });
  });

  it('loads a selected operation detail through the event-id endpoint', async () => {
    get.mockResolvedValue({ data: { data: { log_id: 'audit/1', action: 'RULE_PUBLISHED' } } });
    const api = await import('./auditGovernanceApi');
    expect((await api.fetchAuditLogDetail('audit/1')).action).toBe('RULE_PUBLISHED');
    expect(get).toHaveBeenCalledWith('/v1/audit/logs/audit%2F1');
  });

  it('persists saved queries and review cases through dedicated mutations', async () => {
    post
      .mockResolvedValueOnce({ data: { data: { query_id: 'q1', name: '失败操作' } } })
      .mockResolvedValueOnce({ data: { data: { review_id: 'r1', log_id: 'a1', status: 'pending' } } });
    const api = await import('./auditGovernanceApi');
    await api.saveAuditQuery({ name: '失败操作', filters: { result: 'failure' } });
    await api.createAuditReview({ log_id: 'a1', reason: '高风险操作复核' });
    expect(post).toHaveBeenNthCalledWith(1, '/v1/audit/saved-queries', { name: '失败操作', filters: { result: 'failure' } });
    expect(post).toHaveBeenNthCalledWith(2, '/v1/audit/reviews', { log_id: 'a1', reason: '高风险操作复核' });
  });

  it('requests real export artifacts and integrity checks with the active filters', async () => {
    post
      .mockResolvedValueOnce({ data: { data: { export_id: 'e1', sha256: 'sha256:abc', content_base64: 'e30=' } } })
      .mockResolvedValueOnce({ data: { data: { check_id: 'c1', valid: true, root_sha256: 'sha256:def' } } });
    const api = await import('./auditGovernanceApi');
    const filters = { object_type: 'rule' };
    expect((await api.exportAuditLogs({ format: 'json', filters })).sha256).toBe('sha256:abc');
    expect((await api.verifyAuditIntegrity(filters)).valid).toBe(true);
    expect(post).toHaveBeenNthCalledWith(1, '/v1/audit/exports', { format: 'json', filters });
    expect(post).toHaveBeenNthCalledWith(2, '/v1/audit/integrity-checks', { filters });
  });
});
