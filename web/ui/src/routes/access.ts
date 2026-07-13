import type { NavGroup, NavRoute } from '@/routes/routeManifest';

export type SessionPrincipal = {
  username: string;
  tenantId?: string;
  role?: string;
  roles?: string[];
  permissions?: string[];
};

const wildcardScopes = ['*', 'admin:*'];

export const hasRequiredScope = (principal: SessionPrincipal | undefined, requiredScopes: string[]) => {
  if (requiredScopes.length === 0) return true;
  const granted = new Set(principal?.permissions ?? []);
  if (wildcardScopes.some((scope) => granted.has(scope))) return true;
  return requiredScopes.some((scope) => granted.has(scope));
};

export const hasRouteAccess = (route: NavRoute, principal: SessionPrincipal | undefined) =>
  route.authMode === 'public' || hasRequiredScope(principal, route.requiredScopes);

export const visibleNavGroups = (groups: NavGroup[], principal: SessionPrincipal | undefined): NavGroup[] =>
  groups
    .map((group) => ({
      ...group,
      children: group.children.filter((route) => hasRouteAccess(route, principal)),
    }))
    .filter((group) => group.children.length > 0);
