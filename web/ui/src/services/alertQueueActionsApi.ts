import { api } from '@/services/api';

export async function batchAssignAlerts(alertIds: string[], assignee: string) {
  const targets = alertIds.filter(Boolean);
  if (!targets.length) throw new Error('请至少勾选一条告警');
  if (!assignee.trim()) throw new Error('请输入指派对象');
  const settled = await Promise.allSettled(targets.map((alertId) => api.put(`/v1/alerts/${encodeURIComponent(alertId)}/assign`, { assignee: assignee.trim() })));
  const failed = settled.filter((item) => item.status === 'rejected').length;
  if (failed) throw new Error(`批量指派失败 ${failed} 条，成功 ${targets.length - failed} 条`);
  return { total: targets.length, success: targets.length };
}

export type AlertExportFilters = {
  status?: string;
  sourceIp?: string;
  ruleVersion?: string;
  modelVersion?: string;
  attackPhase?: string;
  assetIp?: string;
  destinationIp?: string;
  minScore?: number;
  startTime?: number;
  endTime?: number;
};

export async function exportAlertQueueCsv(filters: AlertExportFilters) {
  const response = await api.post('/v1/alerts/export/csv', {
    status: filters.status ? [filters.status] : [],
    src_ip: filters.sourceIp ?? '',
    rule_version: filters.ruleVersion ?? '',
    model_version: filters.modelVersion ?? '',
    attack_phase: filters.attackPhase ?? '',
    asset_ip: filters.assetIp ?? '',
    dst_ip: filters.destinationIp ?? '',
    min_score: filters.minScore ?? 0,
    start_time: filters.startTime,
    end_time: filters.endTime,
    max_count: 10_000,
  }, { responseType: 'blob' });
  const url = URL.createObjectURL(response.data);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `alerts-${new Date().toISOString().slice(0, 10)}.csv`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}
