import { beforeEach, describe, expect, it, vi } from 'vitest';

const post = vi.fn();

vi.mock('@/services/api', () => ({ api: { post } }));

describe('not-found API', () => {
  beforeEach(() => post.mockReset());

  it('records only an opaque event id and the fixed web-ui source', async () => {
    post.mockResolvedValue({ data: { event_id: 'nav-1', trace_id: 'trace-1', persisted: true, statuses: [] } });
    const { recordNavigationMiss } = await import('./notFoundApi');
    const result = await recordNavigationMiss('nav-1');
    expect(result.persisted).toBe(true);
    expect(post).toHaveBeenCalledWith('/v1/auth/navigation-miss', { event_id: 'nav-1', source: 'web-ui' });
  });

  it('submits an observable support request for the opaque navigation event', async () => {
    post.mockResolvedValue({ data: { support_request_id: 'support-1', navigation_event_id: 'nav-1', status: 'queued', persisted: true } });
    const { requestNavigationSupport } = await import('./notFoundApi');
    const result = await requestNavigationSupport('nav-1');
    expect(result.persisted).toBe(true);
    expect(post).toHaveBeenCalledWith('/v1/auth/navigation-miss/support', { event_id: 'nav-1' });
  });
});
