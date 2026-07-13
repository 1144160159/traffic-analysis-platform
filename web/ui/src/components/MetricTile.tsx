import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons';
import type { PageSnapshot } from '@/services/mockData';

const toneClass = {
  ok: 'is-ok',
  warn: 'is-warn',
  risk: 'is-risk',
  info: 'is-info',
};

export function MetricTile({ metric }: { metric: PageSnapshot['metrics'][number] }) {
  const up = !metric.delta.startsWith('-');
  return (
    <div className={`taf-metric ${toneClass[metric.status]}`}>
      <span>{metric.label}</span>
      <strong>{metric.value}</strong>
      {metric.delta ? (
        <small>
          {up ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
          {metric.delta}
        </small>
      ) : null}
    </div>
  );
}
