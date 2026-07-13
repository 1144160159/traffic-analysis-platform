import {
  BarChartOutlined,
  BranchesOutlined,
  CloudUploadOutlined,
  ExperimentOutlined,
  ImportOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Descriptions, Drawer, Select, Slider, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { MouseEvent } from 'react';
import { useMemo, useState } from 'react';
import { DataQualityDonutChart, DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { submitModelAction, type ModelActionId } from '@/services/modelActionApi';
import { buildModelMetricTrend } from '@/services/modelChartData';
import { buildModelSimulationPage } from '@/services/modelSimulationData';
import { pageApiPlans, type ActionEndpointPlan } from '@/services/pageApiPlans';

const featureLabels = [
  ['异常登录聚合点', '0.182'],
  ['登录时段异常', '0.146'],
  ['命令执行异常', '0.121'],
  ['端口连接数量', '0.093'],
  ['敏感文件访问', '0.071'],
  ['数据流量大小', '0.064'],
];

const sampleRows = [
  ['06-19 21:33', '10.12.2.45', '异常登录', '0.96'],
  ['06-19 21:28', '10.12.3.78', '提权前置', '0.93'],
  ['06-19 21:19', '172.16.5.23', 'PowerShell', '0.91'],
  ['06-19 21:07', '10.23.4.65', '隧道外联', '0.89'],
  ['06-19 20:58', '10.12.2.45', '横向扫描', '0.87'],
];

const datasetRows = [
  ['训练集', '8,642,315', '2026-05-01 ~ 2026-06-18', '70%', '99.2%'],
  ['验证集', '1,852,114', '2026-06-10 ~ 2026-06-18', '15%', '98.7%'],
  ['测试集', '1,234,876', '2026-06-15 ~ 2026-06-18', '10%', '98.4%'],
  ['反馈样本', '523,645', '2026-06-20 ~ 2026-06-19', '5%', '98.1%'],
];

const reviewRows = [
  ['性能评测', '通过', 'sec_analyst', '06-19 22:10'],
  ['安全评审', '通过', 'sec_manager', '06-19 22:35'],
  ['合规评审', '待审批', 'compliance', '-'],
  ['规范确认', '待审批', 'compliance', '-'],
];

const modelOverlays: OverlayContract[] = [
  {
    id: 'drawer-model-detail',
    title: '模型详情',
    kind: 'Drawer',
    actionLabel: '模型详情',
    description: '展示模型版本、指标、特征重要性、样本分布、评审状态和上线记录。',
    audit: '记录模型详情查看、版本比较和激活上下文。',
  },
];

const modelActionPlans = pageApiPlans.models.actions ?? [];
const modelPageSize = 8;

type ModelActionState = {
  actionId: ModelActionId;
  label: string;
  plan: ActionEndpointPlan;
  endpoint: string;
  modelId: string;
  target: string;
  version: string;
};

export function ModelManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [searchValue, setSearchValue] = useState('');
  const [typeFilter, setTypeFilter] = useState('全部类型');
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [page, setPage] = useState(1);
  const [actionState, setActionState] = useState<ModelActionState>();
  const actionMutation = useMutation({ mutationFn: submitModelAction });
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, page, modelPageSize],
    queryFn: () => fetchPageSnapshot(route.id, { page, pageSize: modelPageSize }),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const declaredTotal = useMemo(() => {
    const declaredText = String(data?.evidence.find((item) => item.label === '返回记录')?.value ?? '28');
    const declaredParts = declaredText.split('/');
    const declared = Number.parseInt(declaredParts[declaredParts.length - 1] || declaredText, 10);
    return Number.isFinite(declared) ? Math.max(rows.length, declared, 28) : Math.max(rows.length, 28);
  }, [data?.evidence, rows]);
  const modelRows = useMemo(() => buildModelSimulationPage(rows, declaredTotal, page, modelPageSize), [declaredTotal, page, rows]);
  const selected = useMemo(() => modelRows.find((row) => rowKey(row) === selectedKey) ?? modelRows[0], [modelRows, selectedKey]);
  const filteredRows = useMemo(() => modelRows.filter((row) => {
    const haystack = Object.values(row).join(' ').toLowerCase();
    const matchesSearch = !searchValue.trim() || haystack.includes(searchValue.trim().toLowerCase());
    const matchesType = typeFilter === '全部类型' || String(row.类型 ?? '').includes(typeFilter);
    const matchesStatus = statusFilter === '全部状态' || String(row.状态 ?? '').includes(statusFilter);
    return matchesSearch && matchesType && matchesStatus;
  }), [modelRows, searchValue, statusFilter, typeFilter]);
  const hasLocalFilter = Boolean(searchValue.trim()) || typeFilter !== '全部类型' || statusFilter !== '全部状态';
  const filteredTotal = hasLocalFilter ? filteredRows.length : declaredTotal;
  const totalPages = Math.max(1, Math.ceil(filteredTotal / modelPageSize));
  const currentPage = Math.min(page, totalPages);
  const visibleRows = filteredRows;
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const openAction = (label: string, actionId: ModelActionId = 'model-context-action', targetRow = selected) => {
    const plan = modelActionPlans.find((item) => item.id === actionId)
      ?? modelActionPlans.find((item) => item.id === 'model-context-action')
      ?? modelActionPlans[0];
    if (!plan) return;
    const modelId = String(targetRow?.__model_id ?? targetRow?.模型名 ?? 'selected-model');
    const version = String(targetRow?.版本 ?? targetRow?.线上版本 ?? 'current');
    actionMutation.reset();
    setActionState({
      actionId,
      label,
      plan,
      endpoint: plan.endpoint.replace('{id}', encodeURIComponent(modelId)).replace('{version}', encodeURIComponent(version)),
      modelId,
      target: String(targetRow?.模型名 ?? modelId),
      version,
    });
  };

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderModelCell(column, value, record, openAction),
  }));

  const handleDelegatedAction = (event: MouseEvent<HTMLDivElement>) => {
    const button = (event.target as HTMLElement).closest('button');
    if (!button || button.hasAttribute('data-model-action-managed') || button.closest('.taf-overlay-host, .ant-drawer')) return;
    const label = button.getAttribute('aria-label') || button.getAttribute('title') || button.textContent?.trim() || '模型业务动作';
    openAction(label, (button.getAttribute('data-model-action-id') || 'model-context-action') as ModelActionId);
  };

  return (
    <div className="taf-page taf-models" data-business-action-delegate="models" onClick={handleDelegatedAction}>
      <section className="taf-models-shell">
        <main className="taf-models-main">
          <header className="taf-models-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Button size="small" icon={<ImportOutlined />} data-model-action-managed="true" onClick={() => openAction('导入模型', 'model-version-register')}>导入模型</Button>
              <Button size="small" icon={<CloudUploadOutlined />} data-model-action-managed="true" onClick={() => openAction('追加反馈样本', 'model-feedback-append')}>追加反馈样本</Button>
              <Button size="small" icon={<ExperimentOutlined />} data-model-action-managed="true" onClick={() => openAction('发起重训', 'model-retrain-request')}>发起重训</Button>
              <Button size="small" type="primary" icon={<PlayCircleOutlined />} data-model-action-managed="true" onClick={() => openAction('激活候选', 'model-version-activate')}>激活候选</Button>
              <Button size="small" danger ghost icon={<RollbackOutlined />} data-model-action-managed="true" onClick={() => openAction('回滚线上版本', 'model-version-rollback')}>回滚线上版本</Button>
              <Button size="small" icon={<BranchesOutlined />} data-model-action-managed="true" onClick={() => openAction('进入 MLOps 编排')}>进入 MLOps 编排</Button>
              <Tooltip title="刷新模型状态">
                <Button size="small" icon={<ReloadOutlined />} data-model-action-managed="true" onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={modelOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/models、APISIX 路由或 rule-manager model registry。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-models-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-models-workbench">
            <section className="taf-models-left">
              <WorkPanel
                title={`模型列表（共 ${declaredTotal} 条）`}
                className="taf-models-list-panel"
              >
                <div className="taf-models-filterbar">
                  <label><SearchOutlined /><input aria-label="搜索模型" placeholder="搜索模型名、类型或负责人" value={searchValue} onChange={(event) => { setSearchValue(event.target.value); setPage(1); }} /></label>
                  <Select size="small" value={typeFilter} onChange={(value) => { setTypeFilter(value); setPage(1); }} options={[{ value: '全部类型' }, { value: '分类' }, { value: '检测' }, { value: '聚类' }]} />
                  <Select size="small" value={statusFilter} onChange={(value) => { setStatusFilter(value); setPage(1); }} options={[{ value: '全部状态' }, { value: '线上' }, { value: '候选' }, { value: '漂移' }]} />
                  <Tooltip title="评估当前筛选模型">
                    <Button size="small" icon={<BarChartOutlined />} data-model-action-managed="true" onClick={() => openAction('评估当前筛选模型', 'model-evaluation-request')} />
                  </Tooltip>
                </div>
                <Table
                  rowKey={rowKey}
                  size="small"
                  loading={isLoading}
                  pagination={false}
                  columns={columns}
                  dataSource={visibleRows}
                  rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                  onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
                />
                <div className="taf-models-pagination">
                  <button type="button" data-model-action-managed="true" disabled={currentPage === 1} onClick={() => setPage(currentPage - 1)}>上一页</button>
                  {Array.from({ length: totalPages }, (_, index) => index + 1).map((item) => <button key={item} type="button" data-model-action-managed="true" className={item === currentPage ? 'is-active' : ''} aria-current={item === currentPage ? 'page' : undefined} onClick={() => setPage(item)}>{item}</button>)}
                  <button type="button" data-model-action-managed="true" disabled={currentPage === totalPages} onClick={() => setPage(currentPage + 1)}>下一页</button>
                  <span>{modelPageSize} 条/页</span><span>共 {filteredTotal} 条</span>
                </div>
              </WorkPanel>

              <div className="taf-models-left-bottom">
                <WorkPanel title={`模型指标（${String(selected?.模型名 ?? 'UEBA 行为分析')} ${String(selected?.版本 ?? 'v1.8.0')}）`}>
                  <ModelMetrics selected={selected} />
                </WorkPanel>
                <WorkPanel title="解释与特征">
                  <FeatureExplain selected={selected} />
                </WorkPanel>
              </div>
            </section>

            <aside className="taf-models-right">
              <WorkPanel title="Champion / Challenger 状态机">
                <ChampionState selected={selected} />
              </WorkPanel>
              <WorkPanel title="数据集与样本" extra={<span className="taf-models-dataset-version">ds_ueba_20260619_v3</span>}>
                <DatasetSamples selected={selected} />
              </WorkPanel>
              <WorkPanel title="激活与回滚">
                <ActivationRollback selected={selected} onAction={openAction} />
              </WorkPanel>
            </aside>
          </div>
        </main>
      </section>
      <Drawer
        className="taf-models-action-drawer"
        title={actionState?.label ?? '模型业务动作'}
        open={Boolean(actionState)}
        width={520}
        onClose={() => { setActionState(undefined); actionMutation.reset(); }}
        extra={<Button size="small" type="primary" loading={actionMutation.isPending} disabled={!actionState} onClick={() => actionState && actionMutation.mutate({ actionId: actionState.actionId, modelId: actionState.modelId, version: actionState.version, target: actionState.target })}>确认提交</Button>}
      >
        {actionState && (
          <div className="taf-models-action-body">
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label="接口">{actionState.plan.method} {actionState.endpoint}</Descriptions.Item>
              <Descriptions.Item label="权限">{actionState.plan.requiredScopes.join(', ')}</Descriptions.Item>
              <Descriptions.Item label="审计事件">{actionState.plan.auditEvent}</Descriptions.Item>
              <Descriptions.Item label="仿真请求体"><code>{JSON.stringify(actionState.plan.defaultBody ?? { action: actionState.label })}</code></Descriptions.Item>
            </Descriptions>
            <Alert type="info" showIcon message="API 预留与仿真执行" description={actionState.plan.guardrails.join('；')} />
            {actionMutation.data && <Alert type="success" showIcon message={`任务 ${actionMutation.data.jobId} 已进入仿真任务队列`} description={`${actionMutation.data.auditEvent}；${actionMutation.data.apiContract}`} />}
            {actionMutation.isError && <Alert type="error" showIcon message="模型动作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : '未知错误'} />}
          </div>
        )}
      </Drawer>
    </div>
  );
}

function ChampionState({ selected }: { selected?: SnapshotRow }) {
  const [grayPercent, setGrayPercent] = useState(20);
  return (
    <div className="taf-models-champion">
      <div className="taf-models-champion-card is-online">
        <span>Champion（线上）</span>
        <b>{String(selected?.线上版本 ?? 'v1.8.0')}</b>
        <em>{String(selected?.模型名 ?? 'UEBA 行为分析')}</em>
        <strong>F1 0.948 | 误报率 0.82%</strong>
      </div>
      <div className="taf-models-gray">
        <span>灰度比例</span>
        <Slider min={5} max={100} value={grayPercent} onChange={setGrayPercent} marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} />
        <div className="taf-models-gates">
          {['性能提升 >= 2%', '稳定性 > 99%', '漂移低风险', '人工评审'].map((item, index) => <span key={item}><SafetyCertificateOutlined />{item}<StatusTag value={index === 3 ? '待审批' : '通过'} /></span>)}
        </div>
      </div>
      <div className="taf-models-champion-card is-candidate">
        <span>Challenger（候选）</span>
        <b>v2.3.1</b>
        <em>{String(selected?.模型名 ?? 'UEBA 行为分析')}</em>
        <strong>F1 0.963 | 误报率 0.68%</strong>
      </div>
      <div className="taf-models-versions">
        <span>可回滚版本</span>
        {['v1.7.0', 'v1.6.0', 'v1.5.0'].map((version, index) => <b key={version}>{version}<StatusTag value="线上" /><em>2026-06-{String(5 + index * 15).padStart(2, '0')}</em></b>)}
      </div>
    </div>
  );
}

function DatasetSamples({ selected }: { selected?: SnapshotRow }) {
  const distributionRows = useMemo(() => buildDistributionRows(selected), [selected]);
  return (
    <div className="taf-models-dataset">
      <div className="taf-models-dataset-table">
        <div><span>数据集</span><span>样本数</span><span>时间范围</span><span>占比</span><span>质量</span></div>
        {datasetRows.map(([name, samples, range, ratio, quality]) => <span key={name}><b>{name}</b><em>{samples}</em><em>{range}</em><em>{ratio}</em><strong>{quality}</strong></span>)}
      </div>
      <div className="taf-models-distribution">
        <DataQualityDonutChart
          ariaLabel="模型样本分布"
          className="taf-models-distribution-echart"
          rows={distributionRows.map(([label, value, tone]) => ({
            label,
            value: Number.parseFloat(value),
            color: tone === 'ok' ? '#36d66b' : tone === 'info' ? '#18a8ff' : tone === 'risk' ? '#ff4d4f' : '#ffb020',
          }))}
        />
        <div>
          {distributionRows.map(([label, value, tone]) => <span key={label}><i className={`is-${tone}`} />{label}<b>{value}</b></span>)}
        </div>
      </div>
    </div>
  );
}

function ActivationRollback({ selected, onAction }: { selected?: SnapshotRow; onAction: OpenModelAction }) {
  const [grayPercent, setGrayPercent] = useState(20);
  return (
    <div className="taf-models-activation">
      <div className="taf-models-activation-controls">
        <span>灰度发布 <b>{grayPercent}%</b></span>
        <Slider min={5} max={100} value={grayPercent} onChange={setGrayPercent} marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} />
        <Button size="small" type="primary" data-model-action-managed="true" onClick={() => onAction('激活到线上', 'model-version-activate', selected)}>激活到线上</Button>
        <Button size="small" danger ghost data-model-action-managed="true" onClick={() => onAction('停用模型', 'model-version-deprecate', selected)}>停用模型</Button>
        <Button size="small" danger data-model-action-managed="true" onClick={() => onAction(`回滚到 ${String(selected?.线上版本 ?? 'v1.7.0')}`, 'model-version-rollback', selected)}>回滚到 {String(selected?.线上版本 ?? 'v1.7.0')}</Button>
      </div>
      <div className="taf-models-review">
        {reviewRows.map(([name, status, owner, time]) => <span key={name}><b>{name}</b><StatusTag value={status} /><em>by {owner}</em><i>{time}</i></span>)}
      </div>
      <div className="taf-models-ops"><b>最近操作记录</b><span>2026-06-19 22:10</span><span>追加反馈样本 5,231 条</span><em>by sec_analyst</em></div>
    </div>
  );
}

function ModelMetrics({ selected }: { selected?: SnapshotRow }) {
  const metricSeries = useMemo(() => buildMetricSeries(selected), [selected]);
  return (
    <div className="taf-models-metrics">
      {metricSeries.map(([label, value, delta, tone], index) => (
        <div key={label}>
          <span>{label}</span>
          <strong className={`is-${tone}`}>{value}</strong>
          <em className={`is-${tone}`}>{delta}</em>
          <DataQualityTrendChart
            ariaLabel={`${label}近 24 小时趋势`}
            className="taf-models-metric-echart"
            categories={['00:00', '04:00', '08:00', '12:00', '16:00', '20:00', '24:00']}
            series={[{
              name: label,
              color: tone === 'risk' ? '#ff4d4f' : tone === 'info' ? '#18a8ff' : '#36d66b',
              values: buildModelMetricTrend(selected, index),
            }]}
          />
        </div>
      ))}
      <footer><span>当前</span><span>基线</span><span>阈值上限</span><span>阈值下限</span></footer>
    </div>
  );
}

function FeatureExplain({ selected }: { selected?: SnapshotRow }) {
  const tabs = ['重要特征', '规则贡献', '异常解释', '样本示例'];
  const [activeTab, setActiveTab] = useState(tabs[0]);
  const tabOffset = tabs.indexOf(activeTab) * 0.012;
  const featureRows = useMemo(() => buildFeatureRows(selected), [selected]);
  return (
    <div className="taf-models-explain">
      <div className="taf-models-tabs">{tabs.map((tab) => <button key={tab} type="button" data-model-action-managed="true" className={tab === activeTab ? 'is-active' : ''} onClick={() => setActiveTab(tab)}>{tab}</button>)}</div>
      <DataQualityTrendChart
        ariaLabel={`${activeTab}贡献分布`}
        className="taf-models-feature-echart"
        categories={featureRows.map(([label]) => label)}
        series={[{
          name: activeTab,
          color: '#18a8ff',
          type: 'bar',
          values: featureRows.map(([, value], index) => Number.parseFloat(value) + tabOffset - index * 0.002),
        }]}
        valueFormatter={(value) => value.toFixed(2)}
      />
      <div className="taf-models-samples">
        <div><span>时间</span><span>源 IP</span><span>话题</span><span>得分</span></div>
        {sampleRows.map(([time, ip, topic, score]) => <span key={`${time}-${ip}`}><b>{time}</b><em>{ip}</em><em>{topic}</em><strong>{score}</strong></span>)}
      </div>
    </div>
  );
}

type OpenModelAction = (label: string, actionId?: ModelActionId, targetRow?: SnapshotRow) => void;

const renderModelCell = (column: string, value: unknown, record: SnapshotRow, onAction: OpenModelAction) => {
  if (column === '模型名') return <span className="taf-models-name"><ExperimentOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return (
    <span className="taf-models-row-actions">
      <button type="button" title="评估模型" aria-label="评估模型" data-model-action-managed="true" onClick={(event) => { event.stopPropagation(); onAction('评估模型', 'model-evaluation-request', record); }}><BarChartOutlined /></button>
      <button type="button" title="查看模型上下文" aria-label="查看模型上下文" data-model-action-managed="true" onClick={(event) => { event.stopPropagation(); onAction('查看模型上下文', 'model-context-action', record); }}><SearchOutlined /></button>
      <button type="button" title="激活模型版本" aria-label="激活模型版本" data-model-action-managed="true" onClick={(event) => { event.stopPropagation(); onAction('激活模型版本', 'model-version-activate', record); }}><BranchesOutlined /></button>
    </span>
  );
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.__model_id ?? row.模型名 ?? row.model_id ?? JSON.stringify(row));

const numericRowValue = (row: SnapshotRow | undefined, key: string, fallback: number) => {
  const value = Number(row?.[key]);
  return Number.isFinite(value) ? value : fallback;
};

const modelSeed = (row?: SnapshotRow) => Array.from(String(row?.__model_id ?? row?.模型名 ?? 'model'))
  .reduce((hash, character) => ((hash * 31) + character.charCodeAt(0)) >>> 0, 7);

const buildMetricSeries = (selected?: SnapshotRow) => {
  const f1 = numericRowValue(selected, '__f1_score', 0.948);
  const auc = numericRowValue(selected, '__auc', 0.982);
  const drift = numericRowValue(selected, '__drift', 0.12);
  const fpDelta = numericRowValue(selected, '__false_positive_delta', -6.2);
  return [
    ['准确率', Math.min(0.995, f1 + 0.023).toFixed(3), '+0.018', 'ok'],
    ['召回率', Math.max(0.7, f1 - 0.023).toFixed(3), '+0.011', 'ok'],
    ['F1', f1.toFixed(3), '+0.012', 'ok'],
    ['AUC', auc.toFixed(3), '+0.009', 'ok'],
    ['误报率', `${Math.abs(fpDelta / 7.5).toFixed(2)}%`, `${fpDelta.toFixed(1)}%`, 'risk'],
    ['漂移 (PSI)', drift.toFixed(2), drift > 0.25 ? '关注' : '正常', drift > 0.25 ? 'risk' : 'ok'],
    ['置信区间 (F1)', `[${Math.max(0, f1 - 0.006).toFixed(3)},${Math.min(1, f1 + 0.005).toFixed(3)}]`, '稳定', 'info'],
  ];
};

const buildFeatureRows = (selected?: SnapshotRow) => {
  const seed = modelSeed(selected);
  return featureLabels.map(([label, fallback], index) => [label, (Number(fallback) + ((seed + index * 7) % 9) / 1000).toFixed(3)]);
};

const buildDistributionRows = (selected?: SnapshotRow) => {
  const offset = modelSeed(selected) % 5;
  return [
    ['正常', `${78.3 - offset * 0.4}%`, 'ok'],
    ['可疑', `${11.5 + offset * 0.2}%`, 'info'],
    ['恶意', `${6.8 + offset * 0.1}%`, 'risk'],
    ['未知', `${3.4 + offset * 0.1}%`, 'warn'],
  ];
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('F1') ? '0.947' : label.includes('误报') ? '-6.2%' : '0',
  delta: 'API',
  status: 'info',
});
