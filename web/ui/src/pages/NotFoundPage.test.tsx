import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { NavRoute } from '@/routes/routeManifest';
import { NotFoundPage } from './NotFoundPage';

const { recordNavigationMiss, requestNavigationSupport } = vi.hoisted(() => ({
  recordNavigationMiss: vi.fn(),
  requestNavigationSupport: vi.fn(),
}));

vi.mock('@/services/notFoundApi', () => ({ recordNavigationMiss, requestNavigationSupport }));

const knownRoutes = [
  { id: 'dashboard', path: '/dashboard', authMode: 'authenticated', requiredScopes: [] },
  { id: 'screen', path: '/screen', authMode: 'authenticated', requiredScopes: [] },
  { id: 'alerts', path: '/alerts', authMode: 'authenticated', requiredScopes: [] },
  { id: 'audit-log', path: '/audit-log', authMode: 'authenticated', requiredScopes: [] },
] as unknown as NavRoute[];

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  queryClient.setQueryData(['current-user'], { username: 'tester', permissions: ['*'], roles: ['admin'] });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/__missing__']}>
        <NotFoundPage knownRoutes={knownRoutes} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('NotFoundPage', () => {
  beforeEach(() => {
    recordNavigationMiss.mockReset();
    requestNavigationSupport.mockReset();
    recordNavigationMiss.mockResolvedValue({
      event_id: 'nav-evidence', trace_id: '520f7a1f-fdcb-47d5-a62c-ef012ae47f0a', occurred_at: '2026-07-20T08:00:00Z',
      tenant_id: 'default', tenant_name: '默认租户', site_name: '主园区', access_source: '内网访问', audit_action: 'navigation_not_found', persisted: true,
      statuses: [
        { id: 'gateway', label: '网关服务', state: 'healthy', value: '正常' },
        { id: 'auth', label: '鉴权服务', state: 'healthy', value: '正常' },
        { id: 'frontend-route', label: '前端路由', state: 'healthy', value: '正常' },
        { id: 'audit-write', label: '审计写入', state: 'healthy', value: '正常' },
      ],
    });
    requestNavigationSupport.mockResolvedValue({
      support_request_id: 'support-evidence', navigation_event_id: 'nav-evidence', trace_id: '18ef697a-089e-4ea8-af83-fe8442e2a304',
      occurred_at: '2026-07-20T08:05:00Z', queue: '平台值班管理员', status: 'queued', audit_action: 'navigation_support_requested', persisted: true,
    });
  });

  it('renders database-backed safe context without exposing the missing path', async () => {
    renderPage();
    expect(await screen.findByText('520f7a1f-fdcb-47d5-a62c-ef012ae47f0a')).toBeInTheDocument();
    expect(screen.getByText('默认租户 / 主园区')).toBeInTheDocument();
    expect(screen.getByText('内网访问')).toBeInTheDocument();
    expect(screen.queryByText('/__missing__')).not.toBeInTheDocument();
    expect(recordNavigationMiss).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('link', { name: /返回仪表盘/ })).toHaveAttribute('href', '/dashboard');
    expect(screen.getByRole('link', { name: /查看审计日志/ })).toHaveAttribute('href', '/audit-log');
  });

  it('persists a support request in an in-page contact interface and copies trace context', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    renderPage();
    await screen.findByText('520f7a1f-fdcb-47d5-a62c-ef012ae47f0a');
    fireEvent.click(screen.getByRole('button', { name: /联系管理员/ }));
    expect(await screen.findByText(/已提交至平台值班管理员/)).toBeInTheDocument();
    expect(requestNavigationSupport).toHaveBeenCalledWith(expect.stringMatching(/^nav-[0-9a-f-]{36}$/));
    expect(screen.getByText('support-evidence')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /复制追踪 ID/ }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('520f7a1f-fdcb-47d5-a62c-ef012ae47f0a'));
  });

  it('falls back to a selected textarea when Clipboard API writes are unavailable', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined });
    const execCommand = vi.fn().mockReturnValue(true);
    Object.defineProperty(document, 'execCommand', { configurable: true, value: execCommand });
    renderPage();
    await screen.findByText('520f7a1f-fdcb-47d5-a62c-ef012ae47f0a');
    fireEvent.click(screen.getByRole('button', { name: /复制追踪 ID/ }));
    expect(await screen.findByText('追踪 ID 已复制')).toBeInTheDocument();
    expect(execCommand).toHaveBeenCalledWith('copy');
  });

  it('shows an explicit failure state and retries the navigation audit request', async () => {
    recordNavigationMiss.mockRejectedValueOnce(new Error('database unavailable')).mockRejectedValueOnce(new Error('database unavailable')).mockResolvedValueOnce({
      event_id: 'nav-retry', trace_id: '5c67b916-a552-4480-8e4e-47caad25cc16', occurred_at: '2026-07-20T08:10:00Z',
      tenant_id: 'default', tenant_name: '默认租户', site_name: '主园区', access_source: '内网访问', audit_action: 'navigation_not_found', persisted: true,
      statuses: [],
    });
    renderPage();
    expect(await screen.findByText('追踪记录暂不可用')).toBeInTheDocument();
    expect(screen.getByText('追踪记录失败')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /重试/ }));
    expect(await screen.findByText('5c67b916-a552-4480-8e4e-47caad25cc16')).toBeInTheDocument();
    expect(recordNavigationMiss).toHaveBeenCalledTimes(3);
  });
});
