import { http, HttpResponse } from 'msw';
import { buildPageSnapshot } from '@/services/mockData';
import { findRouteById } from '@/routes/routeManifest';

export const handlers = [
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
