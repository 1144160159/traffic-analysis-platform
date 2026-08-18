import type { PageSpec } from '@/routes/routeManifest';
import { adaptKnownPageSnapshot } from '@/services/pageSnapshotAdapters';
import { isRecord, readPageSnapshotEnvelope } from '@/services/pageSnapshotEnvelope';
import type { PageSnapshot } from '@/services/mockData';
import { api } from '@/services/httpClient';

export type AlertSearchCursorFilters = {
  status?: string;
  srcIp?: string;
  dstIp?: string;
  assetIp?: string;
  ruleVersion?: string;
  modelVersion?: string;
  attackPhase?: string;
  minScore?: number;
  startTime: number;
  endTime: number;
};

export type AlertSearchCursorRequest = {
  page: PageSpec;
  size: number;
  cursor?: string;
  filters: AlertSearchCursorFilters;
};

export async function fetchAlertSearchCursorSnapshot(request: AlertSearchCursorRequest): Promise<PageSnapshot> {
  const body = {
    size: request.size,
    cursor_mode: 'pit',
    cursor: request.cursor || undefined,
    sort_field: 'last_seen',
    sort_order: 'desc',
    status: request.filters.status ? [request.filters.status] : undefined,
    src_ip: request.filters.srcIp || undefined,
    dst_ip: request.filters.dstIp || undefined,
    asset_ip: request.filters.assetIp || undefined,
    rule_version: request.filters.ruleVersion || undefined,
    model_version: request.filters.modelVersion || undefined,
    attack_phase: request.filters.attackPhase || undefined,
    min_score: request.filters.minScore,
    start_time: request.filters.startTime,
    end_time: request.filters.endTime,
  };
  const secondaryParams = { start_time: request.filters.startTime, end_time: request.filters.endTime };
  const [searchResponse, statsResponse, trendResponse] = await Promise.all([
    api.post('/v1/alerts/search', body),
    api.get('/v1/alerts/stats', { params: secondaryParams }),
    api.get('/v1/alerts/trend', { params: { ...secondaryParams, interval: 'hour' } }),
  ]);
  const envelope = readPageSnapshotEnvelope(searchResponse.data);
  if (!isRecord(envelope.data) || !Array.isArray(envelope.data.alerts)) {
    throw new Error('OpenSearch 游标响应缺少 alerts 数组');
  }
  const mode = envelope.data.cursor_mode;
  const hasMore = envelope.data.has_more;
  const nextCursor = typeof envelope.data.next_cursor === 'string' ? envelope.data.next_cursor : '';
  const totalRelation = envelope.data.total_relation;
  if (mode !== 'pit' || typeof hasMore !== 'boolean' || (hasMore && !nextCursor) || (!hasMore && nextCursor)) {
    throw new Error('OpenSearch PIT 游标响应状态不完整');
  }
  if (totalRelation !== 'eq' && totalRelation !== 'gte') {
    throw new Error('OpenSearch total_relation 缺失或非法');
  }
  const targetSHA256 = envelope.sourceWatermarks['opensearch.alerts.target_sha256'] ?? '';
  if (!envelope.snapshotId || !envelope.asOf || !/^[0-9a-f]{64}$/.test(targetSHA256)) {
    throw new Error('OpenSearch PIT 响应缺少 snapshot/source watermark');
  }
  const adapted = adaptKnownPageSnapshot(request.page, searchResponse.data, [statsResponse.data, trendResponse.data]);
  if (!adapted) throw new Error('告警游标页面适配器不可用');
  return {
    ...adapted,
    snapshot: {
      contractVersion: envelope.contractVersion ?? 1,
      snapshotId: envelope.snapshotId,
      asOf: envelope.asOf,
      traceId: envelope.traceId ?? '',
      partial: envelope.partial,
      missingSections: envelope.missingSections,
      sourceWatermarks: envelope.sourceWatermarks,
    },
    cursorSearch: {
      mode,
      nextCursor,
      hasMore,
      totalRelation,
      targetSHA256,
    },
  };
}

export async function closeAlertSearchCursor(cursor: string): Promise<void> {
  if (!cursor) return;
  const response = await api.delete('/v1/alerts/search/cursor', { data: { cursor } });
  const envelope = readPageSnapshotEnvelope(response.data);
  if (!isRecord(envelope.data) || envelope.data.closed !== true) {
    throw new Error('OpenSearch PIT 未确认关闭');
  }
}
