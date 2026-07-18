import { describe, expect, it } from 'vitest';

import { ruleLifecycleLabel } from './RuleManagementPage';

describe('ruleLifecycleLabel', () => {
  it.each([
    ['draft', '草稿'],
    ['pending_review', '待审'],
    ['canary', '灰度'],
    ['active', '启用'],
    ['enabled', '启用'],
    ['inactive', '停用'],
    ['disabled', '停用'],
    ['deprecated', '停用'],
    ['archived', '停用'],
    ['rollback', '回滚'],
  ])('maps API status %s without substring collisions', (status, expected) => {
    expect(ruleLifecycleLabel(status)).toBe(expected);
  });
});
