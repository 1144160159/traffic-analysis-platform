import { describe, expect, it } from 'vitest';

import { deploymentActionAvailability, deploymentStatusLabel } from './DeploymentManagementWorkspace';

describe('deploymentStatusLabel', () => {
  it.each([
    ['planned', 0, '待发布'],
    ['gray', 20, '灰度中 20%'],
    ['active', 0, '已发布'],
    ['paused', 0, '已暂停'],
    ['rolled_back', 0, '已回滚'],
    ['failed', 0, '阻断'],
    ['cancelled', 0, '已取消'],
    ['superseded', 0, '已替代'],
  ])('maps API status %s to %s', (status, percentage, expected) => {
    expect(deploymentStatusLabel(status, percentage)).toBe(expected);
  });
});

describe('deploymentActionAvailability', () => {
  it('matches backend transition gates for an administrator', () => {
    expect(deploymentActionAvailability('planned', ['*'])).toMatchObject({ canContinue: true, canEditScope: true, canPause: false, canRollback: false });
		expect(deploymentActionAvailability('gray', ['*'])).toMatchObject({ canContinue: true, canEditScope: false, canPause: true, canRollback: true });
		expect(deploymentActionAvailability('paused', ['*'])).toMatchObject({ canContinue: true, canEditScope: false, canPause: false, canRollback: true });
    expect(deploymentActionAvailability('active', ['*'])).toMatchObject({ canContinue: false, canEditScope: false, canPause: true, canRollback: true });
    expect(deploymentActionAvailability('rolled_back', ['*'])).toMatchObject({ canContinue: false, canEditScope: false, canPause: false, canRollback: false });
  });

  it('does not expose write actions to a read-only viewer', () => {
    expect(deploymentActionAvailability('gray', ['deploy:read'])).toMatchObject({ canCreate: false, canContinue: false, canEditScope: false, canPause: false, canRollback: false });
  });
});
