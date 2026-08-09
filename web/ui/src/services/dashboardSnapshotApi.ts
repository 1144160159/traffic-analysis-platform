import type { DashboardVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';

type JsonRecord = Record<string, unknown>;

export function normalizeDashboardSnapshot(payload: unknown): PageSnapshot {
  const envelope = record(payload);
  const data = record(envelope.data);
  const meta = record(envelope.meta);
  const snapshotId = text(meta.snapshot_id);
  const asOf = text(meta.as_of);
  const contractVersion = number(meta.contract_version);
  if (!snapshotId || !asOf || contractVersion < 1) {
    throw new Error('仪表盘快照响应缺少 contract_version、snapshot_id 或 as_of');
  }

  const metrics = list(data.metrics).map((item) => {
    const source = record(item);
    return {
      label: requiredText(source.label, 'metric.label'),
      value: formatValue(source.value, text(source.unit)),
      delta: text(source.delta) || (source.value == null ? '数据源未知' : '同一快照'),
      status: metricStatus(source.state),
    };
  });
  const rows: SnapshotRow[] = list(data.queue).map((item) => {
    const source = record(item);
    return {
      '事件 ID': requiredText(source.event_id, 'queue.event_id'),
      风险级别: text(source.risk_level) || '未知',
      资产组: text(source.asset_group) || '--',
      业务系统: text(source.business_system) || '--',
      处置阶段: text(source.stage) || '--',
      剩余时间: text(source.remaining) || '--',
      证据状态: text(source.evidence_status) || '未知',
    };
  });

  const visuals: DashboardVisuals = {
    kpiSparks: list(data.metrics).map((item) => list(record(item).spark).map(number)),
    healthGates: list(data.health_gates).map((item) => {
      const source = record(item);
      return {
        component: requiredText(source.component, 'health_gate.component'),
        status: text(source.status) || 'unavailable',
        reason: text(source.reason) || '未提供',
        scope: text(source.scope) || 'tenant-bound',
        updated: text(source.updated) || '--',
      };
    }),
    stages: list(data.stages).map((item) => {
      const source = record(item);
      const value = nullableNumber(source.value);
      return {
        label: requiredText(source.label, 'stage.label'),
        value: value == null ? '--' : formatValue(value, text(source.unit)),
        footnote: text(source.footnote) || '同一快照',
        status: metricStatus(source.state),
        bars: list(source.bars).map(number),
        slaPercent: nullableNumber(source.sla_percent) ?? undefined,
        pressurePercent: nullableNumber(source.pressure_percent) ?? undefined,
        action: text(source.action) || undefined,
      };
    }),
    qualityRings: list(data.quality_rings).map((item) => {
      const source = record(item);
      const value = nullableNumber(source.value);
      const ringPercent = nullableNumber(source.ring_percent);
      return {
        label: requiredText(source.label, 'quality.label'),
        value: value == null ? '--' : formatValue(value, text(source.unit)),
        ringPercent: ringPercent ?? 0,
        status: metricStatus(source.state),
        subtext: value == null ? `${text(source.subtext) || '数据源'} · 未知` : text(source.subtext) || '同一快照',
      };
    }),
    topTalkers: list(data.top_talkers).map((item) => {
      const source = record(item);
      return { label: requiredText(source.label, 'top_talker.label'), value: number(source.value) };
    }),
  };

  const missingSections = list(meta.missing_sections).map(text).filter(Boolean);
  const sourceWatermarks = Object.fromEntries(
    Object.entries(record(meta.source_watermarks)).map(([key, value]) => [key, text(value)]),
  );
  const partial = meta.partial === true;
  return {
    id: 'dashboard',
    total: integer(data.queue_total, rows.length),
    metrics,
    rows,
    timeline: [
      {
        title: partial ? '统一快照部分可用' : '统一快照已就绪',
        description: partial
          ? `缺失分区：${missingSections.join('、') || '未声明'}`
          : `所有页面分区共享 ${snapshotId}`,
        status: partial ? 'warn' : 'ok',
      },
    ],
    evidence: Object.entries(sourceWatermarks).map(([label, value]) => ({
      label,
      value: value || '--',
      status: value ? 'ok' : 'warn',
    })),
    visuals: { dashboard: visuals },
    snapshot: {
      contractVersion,
      snapshotId,
      asOf,
      traceId: text(meta.trace_id),
      partial: Boolean(meta.partial),
      missingSections,
      sourceWatermarks,
    },
  };
}

const record = (value: unknown): JsonRecord => value && typeof value === 'object' && !Array.isArray(value)
  ? value as JsonRecord
  : {};
const list = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const text = (value: unknown): string => typeof value === 'string' ? value.trim() : '';
const number = (value: unknown): number => {
  const parsed = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};
const nullableNumber = (value: unknown): number | null => value == null ? null : number(value);
const integer = (value: unknown, fallback: number): number => {
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : fallback;
};
const requiredText = (value: unknown, field: string): string => {
  const normalized = text(value);
  if (!normalized) throw new Error(`仪表盘快照缺少 ${field}`);
  return normalized;
};
const metricStatus = (value: unknown): 'ok' | 'warn' | 'risk' | 'info' => {
  const normalized = text(value);
  if (normalized === 'ok' || normalized === 'warn' || normalized === 'risk') return normalized;
  return 'info';
};
const formatValue = (value: unknown, unit: string): string => {
  if (value == null) return '--';
  const normalized = number(value);
  if (unit === '%') return `${normalized.toFixed(1)}%`;
  return unit ? `${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 2 }).format(normalized)} ${unit}` : String(normalized);
};
