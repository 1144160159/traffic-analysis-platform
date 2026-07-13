import type { SnapshotRow } from '@/services/mockData';

export function buildModelSimulationPage(apiRows: SnapshotRow[], total: number, page: number, pageSize: number): SnapshotRow[] {
  const rows = [...apiRows];
  const modelTypes = ['分类', '检测', '聚类'];
  const statuses = ['线上', '候选', '漂移', '待评估', '停用'];
  const pageOffset = (page - 1) * pageSize;
  const pageCapacity = Math.max(0, Math.min(pageSize, total - pageOffset));
  while (rows.length < pageCapacity) {
    const index = pageOffset + rows.length + 1;
    rows.push({
      __model_id: `SIM-MODEL-${String(index).padStart(3, '0')}`,
      __data_mode: 'simulated',
      __f1_score: Number((0.88 + (index % 10) * 0.009).toFixed(3)),
      __auc: Number((0.92 + (index % 7) * 0.01).toFixed(3)),
      __drift: Number((0.08 + (index % 6) * 0.05).toFixed(2)),
      __false_positive_delta: Number((-8.4 + (index % 5) * 1.1).toFixed(1)),
      模型名: `仿真检测模型 ${String(index).padStart(2, '0')}`,
      类型: modelTypes[index % modelTypes.length],
      版本: `v${1 + (index % 3)}.${index % 10}.0`,
      状态: statuses[index % statuses.length],
      线上版本: index % 4 === 0 ? '-' : `v1.${index % 8}.0`,
      训练时间: `2026-06-${String(1 + (index % 19)).padStart(2, '0')} 18:30`,
      负责人: ['安全运营组', '网络安全组', '数据安全组'][index % 3],
      操作: '详情 / 激活 / 回滚',
    });
  }
  return rows;
}
