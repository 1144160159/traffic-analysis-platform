import {
  ApiOutlined,
  AuditOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  ClusterOutlined,
  DatabaseOutlined,
  EditOutlined,
  FileDoneOutlined,
  ForkOutlined,
  HistoryOutlined,
  LinkOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Input, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { resolveFusionConflict, updateFusionRule } from '@/services/fusionApi';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const sources: Array<{ name: string; batch: string; completeness: string; latency: string; tone: 'ok' | 'warn' | 'risk'; icon: ReactNode }> = [
  { name: 'Flow 流量元数据', batch: '2026062507', completeness: '92.3%', latency: '32s', tone: 'ok', icon: <ApiOutlined /> },
  { name: 'Asset 资产信息', batch: '2026062506', completeness: '94.7%', latency: '18s', tone: 'ok', icon: <ClusterOutlined /> },
  { name: 'Device Log 设备日志', batch: '2026062506', completeness: '88.1%', latency: '56s', tone: 'warn', icon: <DatabaseOutlined /> },
  { name: 'User Event 用户事件', batch: '2026062507', completeness: '98.8%', latency: '21s', tone: 'ok', icon: <AuditOutlined /> },
  { name: 'Threat Intel 威胁情报', batch: '2026062506', completeness: '98.5%', latency: '45s', tone: 'risk', icon: <SafetyCertificateOutlined /> },
  { name: 'Vuln 漏洞信息', batch: '2026062505', completeness: '86.2%', latency: '67s', tone: 'warn', icon: <WarningOutlined /> },
];

const ruleNodes: Array<{ name: string; count: string; success: string; confidence: string; icon: ReactNode }> = [
  { name: 'IP-MAC 对齐', count: '6 条', success: '99.12%', confidence: '中高', icon: <LinkOutlined /> },
  { name: '账号-主机关联', count: '5 条', success: '98.35%', confidence: '中高', icon: <ForkOutlined /> },
  { name: '资产-部门补全', count: '4 条', success: '97.61%', confidence: '中高', icon: <ClusterOutlined /> },
  { name: '域名-IP 解析', count: '7 条', success: '98.72%', confidence: '高', icon: <BranchesOutlined /> },
  { name: '告警-资产关联', count: '8 条', success: '98.48%', confidence: '中高', icon: <WarningOutlined /> },
  { name: '漏洞-服务命中', count: '6 条', success: '95.87%', confidence: '中高', icon: <SafetyCertificateOutlined /> },
];

const fusionOverlays: OverlayContract[] = [
  {
    id: 'modal-fusion-rule-edit',
    title: '融合规则编辑',
    kind: 'Modal',
    actionLabel: '规则编辑',
    description: '编辑多源映射、冲突处理、置信度阈值和回放校验条件。',
    impact: '影响资产、账号、告警和图谱关系的融合结果。',
    audit: 'PATCH /v1/fusion/rules/{id} 记录规则版本、字段映射、阈值调整和审批 trace。',
    fields: [
      ['冲突处理', 'POST /v1/fusion/conflicts/{id}/resolve'],
      ['规则编辑', 'PATCH /v1/fusion/rules/{id}'],
      ['权限', 'rule:write / admin:* / *'],
      ['审计', 'FUSION_CONFLICT_RESOLVED / FUSION_RULE_UPDATED'],
    ],
  },
];

type FusionConflictAction = 'authoritative-source' | 'manual-repair-task' | 'accept-primary';

export function FusionWorkbenchPage({ route }: { route: NavRoute }) {
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedRow = useMemo(() => {
    if (!rows.length) return undefined;
    return rows.find((row) => rowKey(row) === selectedRowKey) ?? rows[0];
  }, [rows, selectedRowKey]);
  const [actionResult, setActionResult] = useState<string>();
  const conflictMutation = useMutation({
    mutationFn: (strategy: FusionConflictAction) =>
      resolveFusionConflict(fusionConflictId(selectedRow), buildFusionConflictPayload(selectedRow, strategy)),
    onSuccess: async (result) => {
      setActionResult(`冲突 ${result.resolution.conflict_id} 已处理，版本 ${result.resolution.state_version}`);
      message.success('融合冲突处理已写入');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(errorText(mutationError));
    },
  });
  const ruleMutation = useMutation({
    mutationFn: () => updateFusionRule(fusionRuleId(selectedRow), buildFusionRulePayload(selectedRow)),
    onSuccess: async (result) => {
      setActionResult(`规则 ${result.rule.rule_id} 已更新，版本 ${result.rule.version}`);
      message.success('融合规则编辑已写入');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(errorText(mutationError));
    },
  });

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderFusionCell(column, value),
  }));

  return (
    <div className="taf-page taf-fusion-workbench">
      <header className="taf-fusion-titlebar">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Button size="small">原始状态</Button>
          <Tooltip title="刷新融合状态">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
          </Tooltip>
          <OverlayContractHost overlays={fusionOverlays} compact />
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/fusion/stats、/v1/fusion/entities、APISIX 路由或后端服务。'}
          action={
            <Button size="small" danger onClick={() => void refetch()}>
              重试
            </Button>
          }
        />
      )}

      <div className="taf-fusion-grid">
        <main className="taf-fusion-main">
          <WorkPanel title="数据源状态">
            <SourceStatusGrid evidence={data?.evidence ?? []} metrics={data?.metrics ?? []} />
          </WorkPanel>

          <WorkPanel
            title="多源融合编排（映射与对齐流程）"
            extra={
              <Space size={6}>
                <Input size="small" value="主园区 / 全来源" readOnly />
                <Button size="small" icon={<EditOutlined />} loading={ruleMutation.isPending} onClick={() => ruleMutation.mutate()}>
                  编辑规则
                </Button>
                <Button size="small" loading={conflictMutation.isPending} onClick={() => conflictMutation.mutate('manual-repair-task')}>
                  生成修复任务
                </Button>
              </Space>
            }
          >
            <FusionPipeline />
          </WorkPanel>

          {actionResult && <Alert type="success" showIcon message={actionResult} />}

          <WorkPanel title="融合规则管理（共 26 条）">
            <Table
              rowKey={rowKey}
              size="small"
              loading={isLoading}
              columns={columns}
              dataSource={rows.slice(0, 4)}
              pagination={false}
              onRow={(record) => ({ onClick: () => setSelectedRowKey(rowKey(record)) })}
            />
          </WorkPanel>

          <div className="taf-fusion-bottom">
            <WorkPanel title="冲突队列（待处理 18 条）">
              <ConflictQueue selectedRow={selectedRow} />
            </WorkPanel>
            <WorkPanel title="融合事件审计（近 50 条）">
              <FusionAuditTrail />
            </WorkPanel>
            <WorkPanel title="融合质量看板（实时）">
              <QualityBoard metrics={data?.metrics ?? []} />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-fusion-detail">
          <ConflictDrawer
            selectedRow={selectedRow}
            resolving={conflictMutation.isPending}
            updatingRule={ruleMutation.isPending}
            onResolve={() => conflictMutation.mutate('authoritative-source')}
            onCreateRepair={() => conflictMutation.mutate('manual-repair-task')}
            onUpdateRule={() => ruleMutation.mutate()}
          />
        </aside>
      </div>
    </div>
  );
}

function SourceStatusGrid({
  evidence,
  metrics,
}: {
  evidence: PageSnapshot['evidence'];
  metrics: PageSnapshot['metrics'];
}) {
  const threatIntelEvidence = evidence.find((item) => item.label === 'Threat Intel API');
  const threatIntelMetric = metrics.find((item) => item.label === '情报命中');
  const sourceItems = sources.map((source) => {
    if (!source.name.includes('Threat Intel')) return source;
    return {
      ...source,
      batch: threatIntelEvidence?.value ?? source.batch,
      completeness: threatIntelMetric?.value ?? source.completeness,
      latency: threatIntelEvidence?.status === 'ok' ? 'live' : source.latency,
      tone: threatIntelEvidence?.status === 'ok' ? ('ok' as const) : source.tone,
    };
  });

  return (
    <div className="taf-fusion-sources">
      {sourceItems.map(({ name, batch, completeness, latency, tone, icon }) => (
        <div key={name} className={`taf-fusion-source is-${tone}`}>
          <span>{icon}</span>
          <strong>{name}</strong>
          <em>{tone === 'ok' ? '正常' : tone === 'risk' ? '延迟' : '波动'}</em>
          <dl>
            <dt>接入批次</dt>
            <dd>{batch}</dd>
            <dt>更新延迟</dt>
            <dd>{latency}</dd>
            <dt>字段覆盖</dt>
            <dd>{completeness}</dd>
          </dl>
        </div>
      ))}
    </div>
  );
}

function FusionPipeline() {
  return (
    <div className="taf-fusion-pipeline">
      <div className="taf-fusion-source-stack">
        {sources.map(({ name, tone, icon }) => (
          <span key={name} className={`is-${tone}`}>
            {icon}
            <b>{name.split(' ')[0]}</b>
          </span>
        ))}
      </div>
      <div className="taf-fusion-rule-flow">
        {ruleNodes.map(({ name, count, success, confidence, icon }, index) => (
          <div key={name} className="taf-fusion-rule-node">
            {index > 0 && <i />}
            {icon}
            <strong>{name}</strong>
            <small>规则 {count}</small>
            <small>成功率 {success}</small>
            <em>置信度 {confidence}</em>
          </div>
        ))}
      </div>
      <div className="taf-fusion-output-stack">
        {['主机实体 2,817', '账号实体 1,428', '资产实体 2,146', '域名实体 1,976', '服务实体 3,112', '告警实体 4,231'].map((item) => (
          <span key={item}>
            <CheckCircleOutlined />
            {item}
          </span>
        ))}
      </div>
    </div>
  );
}

function ConflictQueue({ selectedRow }: { selectedRow?: SnapshotRow }) {
  const items = [
    ['CF-20260625-018', text(selectedRow, '对象', '主机名'), '4', '0.86', '高'],
    ['CF-20260625-017', '所属资产', '3', '0.72', '中'],
    ['CF-20260625-016', '显示名', '3', '0.68', '中'],
  ];
  return (
    <div className="taf-fusion-conflicts">
      <div className="taf-fusion-conflict-head">
        <span>冲突ID</span>
        <span>冲突字段</span>
        <span>来源数</span>
        <span>置信度</span>
        <span>级别</span>
      </div>
      {items.map(([id, field, count, confidence, level]) => (
        <button key={id} type="button">
          <strong>{id}</strong>
          <span>{field}</span>
          <span>{count}</span>
          <span>{confidence}</span>
          <StatusTag value={level} />
        </button>
      ))}
    </div>
  );
}

function FusionAuditTrail() {
  const events = [
    ['03:44:52', '实体合并', 'ACCOUNT_HOST_LINK', '成功'],
    ['03:44:37', '字段补全', 'ASSET_DEPT_COMPLETION', '成功'],
    ['03:44:21', '冲突解决', 'IP_MAC_BIND_V3', '成功'],
    ['03:43:58', '关联重建', 'ALERT_ASSET_JOIN', '成功'],
  ];
  return (
    <div className="taf-fusion-audit">
      {events.map(([time, type, rule, result]) => (
        <div key={`${time}-${rule}`}>
          <HistoryOutlined />
          <span>{time}</span>
          <strong>{type}</strong>
          <em>{rule}</em>
          <b>{result}</b>
        </div>
      ))}
    </div>
  );
}

function QualityBoard({ metrics }: { metrics: Array<{ label: string; value: string }> }) {
  const fallback = [
    ['实体总数', '12,846'],
    ['今日新增实体', '356'],
    ['待处理冲突', '18'],
    ['人工确认率', '6.2%'],
    ['自动融合成功率', '97.6%'],
    ['平均置信度', '0.82'],
    ['覆盖率', '91.3%'],
    ['规则命中率', '93.7%'],
  ];
  const items = metrics.length ? metrics.map((metric) => [metric.label, metric.value]) : fallback;
  return (
    <div className="taf-fusion-quality">
      {items.slice(0, 8).map(([label, value], index) => (
        <span key={label}>
          <strong>{value}</strong>
          <small>{label}</small>
          <em>{index % 2 === 0 ? '↑ 3.2%' : '↓ 1.3%'}</em>
        </span>
      ))}
    </div>
  );
}

function ConflictDrawer({
  onCreateRepair,
  onResolve,
  onUpdateRule,
  resolving,
  selectedRow,
  updatingRule,
}: {
  onCreateRepair: () => void;
  onResolve: () => void;
  onUpdateRule: () => void;
  resolving: boolean;
  selectedRow?: SnapshotRow;
  updatingRule: boolean;
}) {
  return (
    <WorkPanel title={`冲突处理 #${text(selectedRow, '对象', 'CF-20260625-018')}`} extra={<Button size="small" type="text">关闭</Button>}>
      <div className="taf-fusion-conflict-summary">
        <strong>{text(selectedRow, '冲突字段', '主机名')}</strong>
        <StatusTag value="高风险" />
      </div>
      <dl className="taf-fusion-conflict-facts">
        <dt>实体类型</dt>
        <dd>{text(selectedRow, '对象', '主机')}</dd>
        <dt>实体 ID</dt>
        <dd>10.12.4.12</dd>
        <dt>置信度</dt>
        <dd>{text(selectedRow, '可信度', '0.86')}</dd>
        <dt>处理状态</dt>
        <dd>{text(selectedRow, '处理状态', '待确认')}</dd>
      </dl>
      <div className="taf-fusion-value-compare">
        <div>
          <span>Flow 流量</span>
          <strong>实验楼-SRV-12</strong>
          <em>0.78</em>
        </div>
        <div>
          <span>CMDB 资产库</span>
          <strong>exp-lab-srv12</strong>
          <em>0.92</em>
        </div>
        <div>
          <span>DHCP 日志</span>
          <strong>EXP-LAB-12</strong>
          <em>0.65</em>
        </div>
        <div>
          <span>EDR 终端</span>
          <strong>SRV-12.EXP.LAB</strong>
          <em>0.70</em>
        </div>
      </div>
      <label className="taf-fusion-resolution">
        <span>覆盖策略</span>
        <Select
          size="small"
          value="优先 CMDB，其次 EDR，再次 DHCP，最后 Flow"
          options={[{ value: '优先 CMDB，其次 EDR，再次 DHCP，最后 Flow' }, { value: '人工确认后回写' }]}
        />
      </label>
      <label className="taf-fusion-resolution">
        <span>节点备注</span>
        <Input.TextArea rows={3} value="CMDB 为权威资产库，命名规则为小写-短横线。" readOnly />
      </label>
      <div className="taf-fusion-drawer-actions">
        <Button type="primary" loading={resolving} onClick={onResolve}>
          确认主值
        </Button>
        <Button loading={resolving} onClick={onCreateRepair}>
          创建修复任务
        </Button>
        <Button loading={updatingRule} icon={<EditOutlined />} onClick={onUpdateRule}>
          编辑规则
        </Button>
        <Button href="/audit-log">查看审计记录</Button>
        <Button icon={<FileDoneOutlined />}>输出到验收证据</Button>
      </div>
    </WorkPanel>
  );
}

const renderFusionCell = (column: string, value: unknown) => {
  if (column === '对象') return <span className="taf-fusion-object">{String(value ?? '')}</span>;
  if (column === '可信度') return <strong className="taf-fusion-confidence">{String(value ?? '-')}</strong>;
  if (column === '处理状态') return <StatusTag value={value} />;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record['对象'] ?? JSON.stringify(record));

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const buildFusionConflictPayload = (row: SnapshotRow | undefined, strategy: FusionConflictAction) => ({
  object_id: text(row, '对象', 'unknown-object'),
  object_type: text(row, '来源 A', '').includes('Threat Intel') ? 'threat_intel' : 'entity',
  field_name: text(row, '冲突字段', '主机名'),
  selected_source: strategy === 'accept-primary' ? text(row, '来源 A', 'Flow 流量') : text(row, '来源 B', 'CMDB 资产库'),
  selected_value: selectedFusionValue(row, strategy),
  strategy,
  note: strategy === 'manual-repair-task' ? '创建人工修复任务并保留当前主值。' : '按权威来源确认 Fusion 主值。',
  rule_id: fusionRuleId(row),
  detail: {
    source_a: text(row, '来源 A', ''),
    source_b: text(row, '来源 B', ''),
    confidence: text(row, '可信度', ''),
    previous_status: text(row, '处理状态', ''),
  },
});

const buildFusionRulePayload = (row: SnapshotRow | undefined) => ({
  rule_name: `${text(row, '冲突字段', 'IP-MAC')} 融合规则`,
  status: 'draft',
  strategy: 'authoritative-source',
  confidence_threshold: confidenceNumber(row),
  note: '由 Fusion 工作台规则编辑动作生成，等待发布门禁复核。',
  detail: {
    object_id: text(row, '对象', ''),
    source_a: text(row, '来源 A', ''),
    source_b: text(row, '来源 B', ''),
  },
});

const selectedFusionValue = (row: SnapshotRow | undefined, strategy: FusionConflictAction) => {
  if (strategy === 'manual-repair-task') return `${text(row, '对象', 'unknown-object')}::repair-required`;
  if (strategy === 'accept-primary') return text(row, '对象', 'unknown-object');
  return text(row, '来源 B', '').includes('CMDB') ? 'exp-lab-srv12' : text(row, '对象', 'unknown-object');
};

const confidenceNumber = (row: SnapshotRow | undefined) => {
  const raw = Number.parseFloat(text(row, '可信度', '0.86').replace('%', ''));
  if (!Number.isFinite(raw)) return 0.86;
  return raw > 1 ? Math.min(raw / 100, 1) : Math.max(raw, 0);
};

const fusionConflictId = (row: SnapshotRow | undefined) =>
  `cf-${slug(text(row, '对象', 'unknown'))}-${slug(text(row, '冲突字段', 'field'))}`;

const fusionRuleId = (row: SnapshotRow | undefined) => `fusion-${slug(text(row, '冲突字段', 'ip-mac'))}`;

const slug = (value: string) =>
  value
    .toLowerCase()
    .replace(/[^a-z0-9\u4e00-\u9fa5]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64) || 'item';

const errorText = (error: unknown) => {
  if (error instanceof Error) return error.message;
  return 'Fusion 写操作提交失败';
};
