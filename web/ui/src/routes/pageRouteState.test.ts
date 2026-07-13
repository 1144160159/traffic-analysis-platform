import { describe, expect, it } from 'vitest';
import {
  auditDetailTabSlug,
  baselineTabSlug,
  resolveAuditDetailTab,
  resolveBaselineTab,
} from './pageRouteState';

const baselineTabs = ['资产基线', '账号基线', '端口基线', '协议基线', '时间段基线'];

describe('page route state', () => {
  it('maps every behavior-baseline route slug to a deterministic tab', () => {
    expect(resolveBaselineTab('account', baselineTabs)).toBe('账号基线');
    expect(resolveBaselineTab('port', baselineTabs)).toBe('端口基线');
    expect(resolveBaselineTab('protocol', baselineTabs)).toBe('协议基线');
    expect(resolveBaselineTab('time-window', baselineTabs)).toBe('时间段基线');
    expect(resolveBaselineTab('unknown', baselineTabs)).toBe('资产基线');
    expect(baselineTabSlug('账号基线')).toBe('account');
  });

  it('maps audit detail substates without inventing an acceptance state', () => {
    expect(resolveAuditDetailTab('operation-context')).toBe('操作上下文');
    expect(resolveAuditDetailTab('related-chain')).toBe('关联链路');
    expect(resolveAuditDetailTab('unknown')).toBe('字段变更对比');
    expect(auditDetailTabSlug('关联链路')).toBe('related-chain');
  });
});
