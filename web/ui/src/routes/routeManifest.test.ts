import { describe, expect, it } from 'vitest';
import { allRoutes, detailRoutes, findRouteByPath, navGroups, navRoutes } from '@/routes/routeManifest';

describe('routeManifest', () => {
  const expectedNav = [
    {
      id: 'overview',
      title: '综合态势',
      routes: [
        ['仪表盘', '/dashboard'],
        ['态势大屏', '/screen'],
        ['专题面板', '/topics'],
      ],
    },
    {
      id: 'collection-monitoring',
      title: '采集监测',
      routes: [
        ['探针管理', '/probes'],
        ['数据质量', '/data-quality'],
      ],
    },
    {
      id: 'threat-analysis',
      title: '威胁分析',
      routes: [
        ['告警中心', '/alerts'],
        ['战役列表', '/campaigns'],
        ['攻击链分析', '/attack-chains'],
        ['加密流量', '/encrypted-traffic'],
        ['取证分析', '/forensics'],
      ],
    },
    {
      id: 'asset-graph',
      title: '资产图谱',
      routes: [
        ['资产台账', '/assets'],
        ['实体图谱', '/graph'],
        ['数据融合', '/fusion'],
        ['行为基准', '/baselines'],
      ],
    },
    {
      id: 'detection-ops',
      title: '检测运营',
      routes: [
        ['规则管理', '/rules'],
        ['部署管理', '/deployments'],
        ['模型管理', '/models'],
        ['MLOps 编排', '/mlops'],
        ['SOAR 剧本', '/playbooks'],
        ['白名单', '/whitelist'],
      ],
    },
    {
      id: 'audit-config',
      title: '审计配置',
      routes: [
        ['合规审计', '/compliance'],
        ['审计日志', '/audit-log'],
        ['通知配置', '/notifications'],
        ['系统设置', '/settings'],
      ],
    },
  ];

  it('keeps the documented top-level information architecture', () => {
    expect(navGroups.map((group) => group.title)).toEqual([
      '综合态势',
      '采集监测',
      '威胁分析',
      '资产图谱',
      '检测运营',
      '审计配置',
    ]);
  });

  it('keeps every documented menu route in its designed domain and order', () => {
    expect(
      navGroups.map((group) => ({
        id: group.id,
        title: group.title,
        routes: group.children.map((route) => [route.title, route.path]),
      })),
    ).toEqual(expectedNav);
  });

  it('registers all menu routes and detail routes', () => {
    expect(navRoutes).toHaveLength(24);
    expect(detailRoutes.map((route) => route.path)).toEqual(['/alerts/:alertId', '/campaigns/:campaignId']);
    expect(allRoutes.map((route) => route.path)).toContain('/screen');
    expect(navRoutes.map((route) => route.path)).toContain('/topics');
    expect(navRoutes.map((route) => route.title)).toContain('专题面板');
    expect(navRoutes.map((route) => route.path)).not.toContain('/topics/tunnel');
    expect(allRoutes.map((route) => route.path)).toContain('/topics/tunnel');
    expect(allRoutes.map((route) => route.path)).toContain('/topics/exfil');
    expect(allRoutes.map((route) => route.path)).toContain('/topics/apt');
    expect(allRoutes.map((route) => route.path)).toContain('/settings');
  });

  it('keeps every page explainable with tabs, actions, and evidence hints', () => {
    for (const route of allRoutes) {
      expect(route.page.title).toBe(route.title);
      if (route.id === 'forensics') expect(route.page.tabs).toHaveLength(0);
      else expect(route.page.tabs.length).toBeGreaterThan(0);
      expect(route.page.actions.length).toBeGreaterThan(0);
      expect(route.page.apiHints.length).toBeGreaterThan(0);
      expect(route.requiredScopes.length).toBeGreaterThan(0);
      expect(route.acceptance.length).toBeGreaterThanOrEqual(4);
    }
  });

  it('marks the situational screen as a readonly protected route', () => {
    const screen = allRoutes.find((route) => route.id === 'screen');
    expect(screen?.authMode).toBe('protected');
    expect(screen?.accessMode).toBe('readonly');
    expect(screen?.requiredScopes).toEqual(['screen:view']);
  });

  it('keeps the data quality replay action tied to the DLQ replay API hint', () => {
    const dataQuality = allRoutes.find((route) => route.id === 'data-quality');
    expect(dataQuality?.page.actions).toContain('重放 DLQ');
    expect(dataQuality?.page.apiHints).toContain('/api/v1/dlq/replay/fallback');
  });

  it('tracks the protected realtime websocket contract on the dashboard route', () => {
    const dashboard = allRoutes.find((route) => route.id === 'dashboard');
    expect(dashboard?.page.apiHints).toContain('/ws/events');
  });

  it('tracks alert detail state-machine action contracts', () => {
    const alertDetail = allRoutes.find((route) => route.id === 'alert-detail');
    expect(alertDetail?.page.apiHints).toEqual(
      expect.arrayContaining([
        '/api/v1/alerts/{id}/status',
        '/api/v1/alerts/{id}/assign',
        '/api/v1/alerts/{id}/close',
        '/api/v1/alerts/{id}/reopen',
        '/api/v1/alerts/{id}/feedback',
      ]),
    );
  });

  it('keeps detail pages highlighted on their parent menu unless they own a menu route', () => {
    const menuRouteIds = new Set(navRoutes.map((route) => route.id));
    for (const route of detailRoutes) {
      expect(route.activeNavId).toBeTruthy();
      expect(menuRouteIds.has(route.activeNavId ?? '')).toBe(true);
      expect(route.activeNavId).not.toBe(route.id);
    }

    expect(findRouteByPath('/alerts/AL-20260620-000123')?.activeNavId).toBe('alerts');
    expect(findRouteByPath('/campaigns/CAMP-20260620-001')?.activeNavId).toBe('campaigns');
    expect(findRouteByPath('/alerts')?.activeNavId).toBeUndefined();
  });
});
