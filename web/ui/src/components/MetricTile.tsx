import { ArrowDownOutlined, ArrowUpOutlined } from '@ant-design/icons';
import type { ReactNode } from 'react';
import type { PageSnapshot } from '@/services/mockData';

const toneClass = {
  ok: 'is-ok',
  warn: 'is-warn',
  risk: 'is-risk',
  info: 'is-info',
};

export function MetricTile({ metric, icon }: { metric: PageSnapshot['metrics'][number]; icon?: ReactNode }) {
  const up = !metric.delta.startsWith('-');
  return (
    <div className={`taf-metric ${toneClass[metric.status]}`}>
      {icon ? <span className="taf-metric__icon" aria-hidden="true">{icon}</span> : null}
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
