import {
  AuditOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  DownloadOutlined,
  EditOutlined,
  FilterOutlined,
  HistoryOutlined,
  LockOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SaveOutlined,
  SearchOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Input, Select, Space, Switch, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const flowNodes: Array<{ id: string; title: string; caption: string; icon: ReactNode; tone: string }> = [
  { id: 'start', title: '开始', caption: '告警触发', icon: <PlayCircleOutlined />, tone: 'ok' },
  { id: 'condition', title: '条件节点', caption: 'C2 告警触发', icon: <BranchesOutlined />, tone: 'ok' },
  { id: 'confirm', title: '人工确认节点', caption: '二次确认', icon: <LockOutlined />, tone: 'warn' },
  { id: 'isolate', title: '隔离节点', caption: '主机隔离', icon: <ThunderboltOutlined />, tone: 'warn' },
  { id: 'block', title: '阻断节点', caption: '阻断 C2 连接', icon: <CloseCircleOutlined />, tone: 'risk' },
  { id: 'rollback', title: '回滚节点', caption: '自动回滚', icon: <RollbackOutlined />, tone: 'info' },
  { id: 'script', title: '脚本节点', caption: '脚本下发处理', icon: <SettingOutlined />, tone: 'info' },
  { id: 'end', title: '结束', caption: '写入审计', icon: <CheckCircleOutlined />, tone: 'ok' },
];

const executionRows = [
  ['14:31:22', '10.12.4.45', '执行中 (4/7)', '03分12秒', '-', '安全运营组', 'AL-20250527-01431'],
  ['13:52:18', '10.12.3.33', '成功', '05分48秒', '-', '安全运营组', 'AL-20250527-01352'],
  ['11:08:44', '10.13.7.77', '失败', '02分31秒', '阻断配置错误', '安全运营组', 'AL-20250527-01108'],
  ['10:22:09', '10.12.1.19', '已回滚', '04分25秒', '-', '安全运营组', 'AL-20250527-01022'],
  ['09:41:33', '10.14.5.56', '成功', '03分21秒', '-', '安全运营组', 'AL-20250527-00941'],
];

const riskRows = [
  ['高危动作二次确认', '已启用'],
  ['授权边界', '安全运营组（L2）及以上'],
  ['执行前影响评估', '2 台主机、3 条连接'],
  ['潜在影响', '业务中断风险：低'],
  ['冷却时间', '30 分钟（同动作）'],
  ['审批状态', '已审批 APP-20250527-00123'],
  ['可回滚', '支持自动回滚与手动回滚'],
];

const auditRows = [
  ['APP-20250527-00123', 'AL-20250527-01431', '查看', '查看', '2025-05-27 14:31'],
  ['APP-20250527-00098', 'AL-20250527-01352', '查看', '-', '2025-05-27 13:52'],
  ['APP-20250527-00077', 'AL-20250527-01108', '查看', '下载', '2025-05-27 11:09'],
  ['APP-20250527-00055', 'AL-20250527-00963', '查看', '下载', '2025-05-27 10:23'],
];

const effectRows = [
  ['告警数量变化', '128', '35', '-72.3%', 'ok'],
  ['C2 连接数变化', '258', '22', '-91.6%', 'ok'],
  ['主机隔离状态', '3', '3', '隔离中', 'warn'],
  ['误操作反馈', '0.35%', '0.17%', '-0.18%', 'ok'],
];

const playbookOverlays: OverlayContract[] = [
  {
    id: 'modal-playbook-edit',
    title: '剧本编辑',
    kind: 'Modal',
    actionLabel: '剧本编辑',
    description: '编辑 SOAR 节点、条件分支、人工审批、动作参数和回滚策略。',
    impact: '影响自动隔离、阻断、脚本下发和审计闭环。',
    audit: '记录剧本版本、节点差异、审批结果和执行 trace。',
  },
];

export function PlaybookAutomationPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderPlaybookCell(column, value),
  }));

  return (
    <div className="taf-page taf-playbooks">
      <section className="taf-playbooks-shell">
        <main className="taf-playbooks-main">
          <header className="taf-playbooks-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Button size="small" type="primary" icon={<ThunderboltOutlined />}>新建剧本</Button>
              <Button size="small" icon={<SaveOutlined />}>保存草稿</Button>
              <Button size="small" icon={<AuditOutlined />}>提交审批</Button>
              <Button size="small" icon={<PlayCircleOutlined />}>执行演练</Button>
              <Button size="small" danger ghost icon={<RollbackOutlined />}>回滚执行</Button>
              <Button size="small" icon={<DownloadOutlined />}>导出审计</Button>
              <Tooltip title="刷新剧本目录">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={playbookOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/playbooks/catalog、/v1/playbooks/executions、APISIX 路由或 alert-service。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-playbooks-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-playbooks-workbench">
            <section className="taf-playbooks-left">
              <WorkPanel title="A. 剧本列表" extra={<Button size="small" icon={<FilterOutlined />}>筛选视图</Button>}>
                <div className="taf-playbooks-filterbar">
                  <Input size="small" prefix={<SearchOutlined />} placeholder="搜索剧本名称" />
                  <Select size="small" value="适用告警" options={[{ value: '适用告警' }, { value: 'C2 连接告警' }, { value: '扫描告警' }]} />
                  <Select size="small" value="动作类型" options={[{ value: '动作类型' }, { value: '隔离' }, { value: '阻断' }]} />
                  <Select size="small" value="风险级别" options={[{ value: '风险级别' }, { value: '高危' }, { value: '中危' }]} />
                  <Button size="small" icon={<SettingOutlined />} />
                </div>
                <Table
                  rowKey={rowKey}
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  columns={columns}
                  dataSource={rows.slice(0, 5)}
                  rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                  onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
                />
                <div className="taf-playbooks-pagination"><span>共 18 条</span><button type="button" className="is-active">1</button><button type="button">2</button><button type="button">10 条/页</button></div>
              </WorkPanel>

              <WorkPanel title="E. 执行历史">
                <ExecutionHistory />
              </WorkPanel>
            </section>

            <section className="taf-playbooks-center">
              <WorkPanel
                title={`B. 剧本编排：${String(selected?.剧本名称 ?? 'C2 连接阻断剧本')}（当前版本 v3.2.1）`}
                extra={<span className="taf-playbooks-canvas-tools"><SearchOutlined />放大<SearchOutlined />缩小<SettingOutlined />模拟</span>}
              >
                <PlaybookFlow />
              </WorkPanel>
              <WorkPanel title={`F. 处置效果（${String(selected?.剧本名称 ?? 'C2 连接阻断剧本')}）`} extra={<Select size="small" value="执行前 30 分钟" options={[{ value: '执行前 30 分钟' }]} />}>
                <EffectComparison />
              </WorkPanel>
            </section>

            <aside className="taf-playbooks-right">
              <WorkPanel title="C. 节点配置 / 触发策略（人工确认节点）">
                <TriggerPolicy />
              </WorkPanel>
              <WorkPanel title="D. 风险控制">
                <RiskControl />
              </WorkPanel>
              <WorkPanel title="G. 审计与证据">
                <AuditEvidence data={data} />
              </WorkPanel>
            </aside>
          </div>
        </main>
      </section>
    </div>
  );
}

function PlaybookFlow() {
  return (
    <div className="taf-playbooks-flow">
      {flowNodes.map((node, index) => (
        <button key={node.id} type="button" className={`taf-playbooks-flow-node is-${node.tone} is-${node.id}`}>
          <span>{node.icon}</span>
          <b>{node.title}</b>
          <em>{node.caption}</em>
          <small>{index === 2 ? '二次确认' : index === 4 ? '执行失败' : index === 5 ? '自动回滚' : '通过'}</small>
        </button>
      ))}
      <div className="taf-playbooks-flow-legend"><i className="is-ok" />正常<i className="is-warn" />待处理<i className="is-risk" />失败<i className="is-info" />回滚流</div>
    </div>
  );
}

function TriggerPolicy() {
  return (
    <div className="taf-playbooks-policy">
      <label><span>告警严重级别</span><Select size="small" value="高危" options={[{ value: '高危' }]} /></label>
      <label><span>资产重要性</span><Select size="small" value="核心资产" options={[{ value: '核心资产' }]} /></label>
      <label><span>规则/模型命中</span><Select size="small" value=">= 3 次" options={[{ value: '>= 3 次' }]} /></label>
      <label><span>规则组</span><Select size="small" value="C2 行为检测组" options={[{ value: 'C2 行为检测组' }]} /></label>
      <label><span>模型</span><Select size="small" value="C2 检测模型 v2.1" options={[{ value: 'C2 检测模型 v2.1' }]} /></label>
      <label><span>时间窗</span><Select size="small" value="最近 15 分钟" options={[{ value: '最近 15 分钟' }]} /></label>
      <label><span>阈值条件</span><Select size="small" value="命中次数 >= 3" options={[{ value: '命中次数 >= 3' }]} /></label>
      <label><span>确认角色</span><Select size="small" value="安全运营组" options={[{ value: '安全运营组' }]} /></label>
      <label><span>确认方式</span><Select size="small" value="人工确认" options={[{ value: '人工确认' }]} /></label>
      <label><span>冷却时间</span><Select size="small" value="30 分钟" options={[{ value: '30 分钟' }]} /></label>
      <label className="taf-playbooks-switch"><span>允许自动通过</span><Switch size="small" /></label>
    </div>
  );
}

function RiskControl() {
  return (
    <div className="taf-playbooks-risk">
      {riskRows.map(([label, value]) => (
        <span key={label}>
          <em>{label}</em>
          <b>{value}</b>
        </span>
      ))}
    </div>
  );
}

function ExecutionHistory() {
  return (
    <div className="taf-playbooks-history">
      <div><span>执行时间</span><span>执行对象</span><span>步骤状态</span><span>耗时</span><span>失败原因</span><span>操作者</span><span>关联告警</span></div>
      {executionRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button">
          {row.map((cell, index) => <span key={`${cell}-${index}`} className={index === 2 ? statusClass(cell) : ''}>{cell}</span>)}
        </button>
      ))}
      <footer><HistoryOutlined /> 查看全部执行历史</footer>
    </div>
  );
}

function EffectComparison() {
  return (
    <div className="taf-playbooks-effect">
      {effectRows.map(([label, before, after, delta, tone]) => (
        <span key={label}>
          <em>{label}</em>
          <b className={`is-${tone}`}>{delta}</b>
          <i><strong style={{ height: `${Math.max(18, Number.parseInt(before, 10) / 2)}px` }} /><strong style={{ height: `${Math.max(14, Number.parseInt(after, 10) / 2)}px` }} /></i>
          <small>执行前 {before} / 执行后 {after}</small>
        </span>
      ))}
      <footer>数据来源：检测平台 & 处置平台 & 告警中心 <a>查看趋势分析</a></footer>
    </div>
  );
}

function AuditEvidence({ data }: { data?: PageSnapshot }) {
  return (
    <div className="taf-playbooks-audit">
      <div><span>授权单号</span><span>关联告警</span><span>执行记录</span><span>回滚记录</span><span>审计时间</span><span>操作</span></div>
      {auditRows.map((row) => (
        <button key={row[0]} type="button">
          {row.map((cell, index) => <span key={`${row[0]}-${index}`}>{cell}</span>)}
          <DownloadOutlined />
        </button>
      ))}
      <footer>
        {(data?.evidence ?? []).slice(0, 4).map((item) => <span key={item.label}><AuditOutlined />{item.label}<b>{item.value}</b></span>)}
      </footer>
    </div>
  );
}

const renderPlaybookCell = (column: string, value: unknown) => {
  if (column === '剧本名称') return <span className="taf-playbooks-name"><ThunderboltOutlined />{String(value)}</span>;
  if (column === '动作类型') return <span className="taf-playbooks-action-tags">{String(value).split(' / ').map((item) => <em key={item}>{item}</em>)}</span>;
  if (column === '风险级别' || column === '启用状态') return <StatusTag value={value} />;
  if (column === '操作') return <span className="taf-playbooks-row-actions"><PlayCircleOutlined /><EditOutlined /><AuditOutlined /></span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.剧本名称 ?? row['剧本 ID'] ?? JSON.stringify(row));

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '96.8%' : '0',
  delta: 'API',
  status: 'info',
});

const statusClass = (value: string) => {
  if (value.includes('失败')) return 'is-risk';
  if (value.includes('回滚') || value.includes('执行中')) return 'is-warn';
  return 'is-ok';
};
