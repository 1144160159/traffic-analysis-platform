import { describe, expect, it } from 'vitest';
import { detailRoutes, legacyTopicRoutes, navRoutes } from '@/routes/routeManifest';
import {
  findCompatibilityAlias,
  findPageDesignContract,
  pageDesignContractRegistry,
} from '@/routes/pageDesignContracts';
import { pageApiPlans } from '@/services/pageApiPlans';

describe('page design contract registry', () => {
  const pages = pageDesignContractRegistry.pages;
  const aliases = pageDesignContractRegistry.compatibility_aliases;
  // analysis 域(任务调度)属核心卷功能页;视觉验收合同(UI_D1 八页候选)尚未实现,
  // 不纳入 28 页视觉验收集合(不把未视觉验收的页面写成已验收)。
  const visuallyAcceptanceNavRoutes = navRoutes.filter((route) => route.domain !== 'analysis');
  const visuallyAcceptanceDetailRoutes = detailRoutes.filter((route) => route.domain !== 'analysis');
  const acceptanceRoutes = [
    { id: 'login', path: '/login', requiredScopes: [] as string[], authMode: 'public' },
    ...visuallyAcceptanceNavRoutes.map((route) => ({
      id: route.id,
      path: route.path,
      requiredScopes: route.requiredScopes,
      authMode: route.accessMode === 'readonly' ? 'protected-readonly' : 'protected',
    })),
    ...visuallyAcceptanceDetailRoutes.map((route) => ({
      id: route.id,
      path: route.path,
      requiredScopes: route.requiredScopes,
      authMode: 'protected',
    })),
    { id: 'not-found', path: '*', requiredScopes: [] as string[], authMode: 'protected' },
  ];

  it('covers exactly the 28 visual acceptance pages without duplicates or orphans', () => {
    expect(pages).toHaveLength(28);
    expect(new Set(pages.map((page) => page.page_id)).size).toBe(28);
    expect(new Set(pages.map((page) => page.route)).size).toBe(28);
    expect(pages.map((page) => page.page_id).sort()).toEqual(
      acceptanceRoutes.map((route) => route.id).sort(),
    );
  });

  it('locks runtime paths, access modes and required scopes', () => {
    for (const route of acceptanceRoutes) {
      const contract = findPageDesignContract(route.id);
      expect(contract, route.id).toBeDefined();
      expect(contract?.route).toBe(route.path);
      expect(contract?.auth_mode).toBe(route.authMode);
      expect(contract?.required_scopes).toEqual(route.requiredScopes);
    }
  });

  it('requires every page to declare task, visual truth, IA, types, owner and reviewer', () => {
    for (const page of pages) {
      expect(page.target_task.length, page.page_id).toBeGreaterThan(7);
      expect(page.visual_truth.length, page.page_id).toBeGreaterThan(0);
      expect(page.information_architecture.length, page.page_id).toBeGreaterThanOrEqual(3);
      expect(page.typescript_contracts.length, page.page_id).toBeGreaterThan(0);
      expect(page.feature_ids.length, page.page_id).toBeGreaterThan(0);
      expect(page.must_preserve.length, page.page_id).toBeGreaterThan(0);
      expect(page.owner, page.page_id).toMatch(/^WP-[0-9]{2}-/u);
      expect(page.reviewer, page.page_id).toBe('QA-UI-INDEPENDENT');
    }
  });

  it('keeps every API plan owned by one page contract or compatibility alias', () => {
    // 核心卷任务调度功能页:API 计划先于视觉验收合同登记(UI_D1 八页候选 NOT_IMPLEMENTED),
    // 此处显式声明所有权;视觉合同实现后迁移至 registry。
    const functionalAnalysisPlanKeys = [
      'analysis-tasks', 'analysis-task-definition-detail', 'analysis-schedules', 'analysis-orchestration', 'analysis-runs',
      'analysis-run-detail', 'analysis-reports', 'analysis-resources',
    ];
    const ownedPlanKeys = [
      ...pages.flatMap((page) => page.api_plan_key ? [page.api_plan_key] : []),
      ...aliases.map((alias) => alias.api_plan_key),
      ...functionalAnalysisPlanKeys,
    ];
    expect(new Set(ownedPlanKeys).size).toBe(ownedPlanKeys.length);
    expect(ownedPlanKeys.sort()).toEqual(Object.keys(pageApiPlans).sort());
  });

  it('keeps the three legacy topic routes as redirects instead of extra visual pages', () => {
    expect(aliases).toHaveLength(3);
    expect(aliases.map((alias) => alias.route_id).sort()).toEqual(
      legacyTopicRoutes.map((route) => route.id).sort(),
    );
    for (const route of legacyTopicRoutes) {
      const alias = findCompatibilityAlias(route.id);
      expect(alias?.legacy_path).toBe(route.path);
      expect(alias?.canonical_page_id).toBe('topics');
      expect(pages.some((page) => page.page_id === route.id)).toBe(false);
    }
  });

  it('requires a production candidate manifest and the complete UI state vocabulary', () => {
    expect(pageDesignContractRegistry.candidate_binding.required).toBe(true);
    expect(pageDesignContractRegistry.candidate_binding.required_hashes).toEqual(
      expect.arrayContaining([
        'contract_sha256',
        'route_manifest_sha256',
        'page_api_plans_sha256',
        'dist_sha256',
        'image_id',
      ]),
    );
    expect(pageDesignContractRegistry.baseline_viewport).toMatchObject({
      width: 1920,
      height: 1080,
      device_scale_factor: 1,
      browser: 'Windows Chrome',
      mock_enabled: false,
    });
    expect(pageDesignContractRegistry.shared_rules.required_states).toEqual(
      expect.arrayContaining([
        'loading',
        'empty',
        'partial',
        'forbidden',
        'error',
        'accepted',
        'running',
        'succeeded',
        'failed',
        'cancelled',
        'compensated',
      ]),
    );
  });
});
