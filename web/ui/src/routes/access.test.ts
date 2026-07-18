import { describe, expect, it } from 'vitest';
import { hasRouteAccess, visibleNavGroups } from '@/routes/access';
import { findRouteById, navGroups } from '@/routes/routeManifest';

describe('route access', () => {
  it('allows wildcard principals to see every route', () => {
    const groups = visibleNavGroups(navGroups, { username: 'admin', permissions: ['*'] });
    expect(groups.flatMap((group) => group.children)).toHaveLength(24);
  });

  it('keeps viewer users out of admin and deployment routes', () => {
    const viewer = { username: 'viewer', permissions: ['alert:read', 'rule:read', 'graph:read'] };
    expect(hasRouteAccess(findRouteById('alerts')!, viewer)).toBe(true);
    expect(hasRouteAccess(findRouteById('deployments')!, viewer)).toBe(false);
    expect(hasRouteAccess(findRouteById('settings')!, viewer)).toBe(false);
  });

  it('keeps route groups visible only when at least one child is authorized', () => {
    const analystGroups = visibleNavGroups(navGroups, {
      username: 'analyst',
      permissions: ['alert:read', 'rule:read', 'pcap:read', 'graph:read'],
    });
    expect(analystGroups.map((group) => group.id)).toContain('threat-analysis');
    expect(analystGroups.map((group) => group.id)).toContain('detection-ops');
    expect(analystGroups.map((group) => group.id)).not.toContain('audit-config');
  });

  it('requires the asset read scope independently from graph access', () => {
    expect(hasRouteAccess(findRouteById('assets')!, { username: 'graph-reader', permissions: ['graph:read'] })).toBe(false);
    expect(hasRouteAccess(findRouteById('assets')!, { username: 'asset-reader', permissions: ['asset:read'] })).toBe(true);
  });
});
