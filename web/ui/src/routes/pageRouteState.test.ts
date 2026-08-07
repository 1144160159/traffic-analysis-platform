import { describe, expect, it } from 'vitest';
import {
  auditLogRoute,
  auditDetailTabSlug,
  baselineTabSlug,
  mergeRouteSearchParams,
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
    expect(resolveAuditDetailTab('operation-detail')).toBe('操作详情');
    expect(resolveAuditDetailTab('review')).toBe('复核操作');
    expect(resolveAuditDetailTab('unknown')).toBe('字段变更对比');
    expect(auditDetailTabSlug('关联链路')).toBe('related-chain');
  });

  it('updates owned query state without discarding unrelated deep-link context', () => {
    const current = new URLSearchParams('object_id=asset-1&trace_id=trace-1&tab=old');
    const next = mergeRouteSearchParams(current, { tab: 'new', detail: 'operation-detail', unused: null });
    expect(next.toString()).toBe('object_id=asset-1&trace_id=trace-1&tab=new&detail=operation-detail');
    expect(current.toString()).toBe('object_id=asset-1&trace_id=trace-1&tab=old');
  });

  it('builds the registered audit route with canonical filter names', () => {
    const current = new URLSearchParams('trace_id=trace-1');
    expect(auditLogRoute('baseline', 'baseline/1', current)).toBe(
      '/audit-log?trace_id=trace-1&object_type=baseline&object_id=baseline%2F1',
    );
    expect(current.toString()).toBe('trace_id=trace-1');
  });
});
