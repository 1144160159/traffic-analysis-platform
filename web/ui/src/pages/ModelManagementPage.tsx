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
import { createPortal } from 'react-dom';
import { DataQualityDonutChart, DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { buildModelActionRequestBody, submitModelAction, type ModelActionId } from '@/services/modelActionApi';
import { buildModelMetricTrend } from '@/services/modelChartData';
import { fetchModelWorkbench, type ModelWorkbench } from '@/services/modelWorkbenchApi';
import { pageApiPlans, type ActionEndpointPlan } from '@/services/pageApiPlans';

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
  payload?: Record<string, unknown>;
};

export function ModelManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [searchValue, setSearchValue] = useState('');
  const [typeFilter, setTypeFilter] = useState('全部类型');
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [page, setPage] = useState(1);
  const [actionState, setActionState] = useState<ModelActionState>();
  const focusState = new URLSearchParams(window.location.search).get('ui_focus') ?? '';
  const actionMutation = useMutation({ mutationFn: submitModelAction });
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, page, modelPageSize],
    queryFn: () => fetchPageSnapshot(route.id, { page, pageSize: modelPageSize }),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const declaredTotal = useMemo(() => {
    const declaredText = String(data?.evidence.find((item) => item.label === '返回记录')?.value ?? rows.length);
    const declaredParts = declaredText.split('/');
    const declared = Number.parseInt(declaredParts[declaredParts.length - 1] || declaredText, 10);
    return Number.isFinite(declared) ? Math.max(rows.length, declared) : rows.length;
  }, [data?.evidence, rows]);
  const modelRows = rows;
  const selected = useMemo(() => modelRows.find((row) => rowKey(row) === selectedKey) ?? modelRows[0], [modelRows, selectedKey]);
  const selectedModelId = String(selected?.__model_id ?? '');
  const workbenchQuery = useQuery({
    queryKey: ['model-workbench', selectedModelId],
    queryFn: () => fetchModelWorkbench(selectedModelId),
    enabled: Boolean(selectedModelId),
  });
  const workbench = workbenchQuery.data;
  const reviewGates = workbenchItems(workbench, 'review_gates');
  const activationBlocked = reviewGates.length === 0 || reviewGates.some((item) => !isApprovedGate(textValue(item, 'status')));
  const candidateVersion = workbench?.versions.find((version) => version.status === 'registered' || version.status === 'validating')?.model_version ?? '';
  const rollbackVersion = workbench?.versions.find((version) => version.status === 'deprecated')?.model_version ?? '';
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
  const openAction = (label: string, actionId: ModelActionId = 'model-context-action', targetRow = selected, payload?: Record<string, unknown>) => {
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
      payload,
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

  if (focusState) {
    return createPortal(<ModelFocusSurface kind={focusState} selected={selected} workbench={workbench} loading={workbenchQuery.isLoading} error={workbenchQuery.error} />, document.body);
  }

  return (
    <div className="taf-page taf-models" data-business-action-delegate="models" onClick={handleDelegatedAction}>
      <section className="taf-models-shell">
        <main className="taf-models-main">
          <header className="taf-models-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Tooltip title="需先在 MLOps 编排中绑定真实制品与特征集">
                <Button size="small" icon={<ImportOutlined />} disabled data-model-action-managed="true">导入模型</Button>
              </Tooltip>
              <Button size="small" icon={<CloudUploadOutlined />} data-model-action-managed="true" onClick={() => openAction('追加反馈样本', 'model-feedback-append')}>追加反馈样本</Button>
              <Button size="small" icon={<ExperimentOutlined />} data-model-action-managed="true" onClick={() => openAction('发起重训', 'model-retrain-request')}>发起重训</Button>
              <Tooltip title={activationBlocked ? `门禁未通过：${reviewGates.length ? pendingGateNames(reviewGates) : '缺少持久化评审门禁'}` : !candidateVersion ? '没有可激活候选版本' : '全量激活候选版本；分阶段流量请使用部署编排'}>
                <Button size="small" type="primary" icon={<PlayCircleOutlined />} disabled={activationBlocked || !candidateVersion} data-model-action-managed="true" onClick={() => openAction('激活候选', 'model-version-activate', { ...selected, 版本: candidateVersion })}>激活候选</Button>
              </Tooltip>
              <Tooltip title={rollbackVersion ? `回滚到 ${rollbackVersion}` : '没有可回滚的已停用版本'}>
                <Button size="small" danger ghost icon={<RollbackOutlined />} disabled={!rollbackVersion} data-model-action-managed="true" onClick={() => openAction(`回滚到 ${rollbackVersion}`, 'model-version-rollback', { ...selected, 版本: rollbackVersion })}>回滚线上版本</Button>
              </Tooltip>
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
                  <ModelMetrics selected={selected} workbench={workbench} />
                </WorkPanel>
              <WorkPanel title="解释与特征">
                  {workbenchQuery.isError && <Alert type="error" showIcon message="模型工作台加载失败" description={workbenchQuery.error instanceof Error ? workbenchQuery.error.message : '请检查模型工作台接口。'} action={<Button size="small" onClick={() => void workbenchQuery.refetch()}>重试</Button>} />}
                  <FeatureExplain workbench={workbench} loading={workbenchQuery.isLoading} />
                </WorkPanel>
              </div>
            </section>

            <aside className="taf-models-right">
              <WorkPanel title="Champion / Challenger 状态机">
                <ChampionState selected={selected} workbench={workbench} />
              </WorkPanel>
              <WorkPanel title="数据集与样本" extra={<span className="taf-models-dataset-version">{workbench?.source ?? '加载中'}</span>}>
                <DatasetSamples workbench={workbench} />
              </WorkPanel>
              <WorkPanel title="激活与回滚">
                <ActivationRollback selected={selected} workbench={workbench} onAction={openAction} />
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
        extra={<Button size="small" type="primary" loading={actionMutation.isPending} disabled={!actionState} onClick={() => actionState && actionMutation.mutate({ actionId: actionState.actionId, modelId: actionState.modelId, version: actionState.version, target: actionState.target, payload: actionState.payload })}>确认提交</Button>}
      >
        {actionState && (
          <div className="taf-models-action-body">
            <Descriptions size="small" column={1} bordered>
              <Descriptions.Item label="接口">{actionState.plan.method} {actionState.endpoint}</Descriptions.Item>
              <Descriptions.Item label="权限">{actionState.plan.requiredScopes.join(', ')}</Descriptions.Item>
              <Descriptions.Item label="审计事件">{actionState.plan.auditEvent}</Descriptions.Item>
              <Descriptions.Item label="请求体"><code>{JSON.stringify(buildModelActionRequestBody({ actionId: actionState.actionId, modelId: actionState.modelId, version: actionState.version, target: actionState.target, payload: actionState.payload }))}</code></Descriptions.Item>
            </Descriptions>
            <Alert type="info" showIcon message="真实 API 与审计门禁" description={actionState.plan.guardrails.join('；')} />
            {actionMutation.data && <Alert type="success" showIcon message={`任务 ${actionMutation.data.jobId} 已由服务端受理`} description={`${actionMutation.data.auditEvent}；${actionMutation.data.apiContract}；状态 ${actionMutation.data.status}`} />}
            {actionMutation.isError && <Alert type="error" showIcon message="模型动作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : '未知错误'} />}
          </div>
        )}
      </Drawer>
    </div>
  );
}

function ModelFocusSurface({ kind, selected, workbench, loading, error }: { kind: string; selected?: SnapshotRow; workbench?: ModelWorkbench; loading: boolean; error: Error | null }) {
  const isActivation = kind === 'activation-audit-gate';
  const activeTab = kind === 'feature-rule-contribution' ? '规则贡献' : kind === 'feature-anomaly-explanation' ? '异常解释' : kind === 'feature-sample-examples' ? '样本示例' : '重要特征';
  const features = workbenchItems(workbench, 'features');
  const contributions = workbenchItems(workbench, 'rule_contributions');
  const causes = workbenchItems(workbench, 'anomaly_causes');
  const samples = workbenchItems(workbench, 'samples');
  const datasets = workbenchItems(workbench, 'datasets');
  const distribution = workbenchItems(workbench, 'distribution');
  const reviewGates = workbenchItems(workbench, 'review_gates');
  const metrics = workbenchItems(workbench, 'metrics');
  const actions = workbench?.actions ?? [];
  const pending = reviewGates.filter((item) => !isApprovedGate(textValue(item, 'status')));
  const exportAudit = () => {
    const report = { generated_at: new Date().toISOString(), model: workbench?.model, versions: workbench?.versions, review_gates: reviewGates, actions };
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' }));
    link.download = `model-audit-${String(workbench?.model.model_id ?? 'unknown')}.json`;
    link.click();
    URL.revokeObjectURL(link.href);
  };
  return (
    <section className={`taf-models-focus${isActivation ? ' is-activation' : ''}`} data-focus-state={kind}>
      <header><h1>{isActivation ? '激活与回滚' : '解释与特征'}</h1><span>{String(selected?.模型名 ?? '')} · {String(selected?.版本 ?? '')} · {workbench?.source ?? '加载中'}</span></header>
      {error ? <Alert type="error" showIcon message="模型工作台加载失败" description={error.message} /> : loading ? <div className="taf-models-focus-loading">正在加载 PostgreSQL 工作台…</div> : isActivation ? (
        <>
          <nav><button type="button">激活流程</button><button type="button" className="is-active">审计门禁</button></nav>
          <div className="taf-models-focus-activation-grid">
            <article>
              <h2>门禁矩阵</h2>
              <div className="taf-models-focus-gate-table">
                <div><span>门禁项</span><span>阈值要求</span><span>结果</span><span>说明</span></div>
                {reviewGates.map((item, index) => <div key={textValue(item, 'name')}><b>{textValue(item, 'name')}</b><span>{activationThreshold(index, metrics)}</span><StatusTag value={textValue(item, 'status')} /><em>{activationEvidence(index, metrics)}</em></div>)}
              </div>
            </article>
            <article>
              <h2>审批流</h2>
              <div className="taf-models-focus-approval-flow">
                {reviewGates.map((item) => <div key={textValue(item, 'name')} className={textValue(item, 'status').includes('通过') ? 'is-passed' : 'is-pending'}><i /><b>{textValue(item, 'name')}</b><StatusTag value={textValue(item, 'status')} /><span>by {textValue(item, 'owner')}</span><time>{textValue(item, 'time')}</time><em>{textValue(item, 'status').includes('通过') ? '审计已留痕' : '发布前留痕'}</em></div>)}
              </div>
            </article>
          </div>
          <article className="taf-models-focus-audit-log"><h2>审计记录</h2><div><span>时间</span><span>操作</span><span>模型版本</span><span>操作人</span><span>trace_id</span></div>{actions.slice(0, 4).map((action) => <div key={action.job_id}><time>{formatShortTime(action.created_at)}</time><b>{action.action}</b><span>{action.version || String(selected?.版本 ?? '-')}</span><span>{action.requested_by}</span><em>{action.job_id}</em></div>)}</article>
          <footer><Button size="large" onClick={exportAudit}>生成审计报告</Button><Tooltip title={pending.length ? `门禁未通过：${pendingGateNames(reviewGates)}` : '返回模型工作台继续激活'}><Button size="large" type="primary" onClick={() => window.location.assign('/models')}>继续审批</Button></Tooltip><Button size="large" danger onClick={() => window.location.assign('/models')}>返回工作台处理驳回</Button></footer>
        </>
      ) : (
        <>
          <nav>{['重要特征', '规则贡献', '异常解释', '样本示例'].map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''}>{tab}</button>)}</nav>
          <div className="taf-models-focus-feature-body">
            {activeTab === '重要特征' && <FocusImportantFeatures rows={features} />}
            {activeTab === '规则贡献' && <FocusRuleContribution rows={contributions} />}
            {activeTab === '异常解释' && <FocusAnomalyExplanation causes={causes} samples={samples} metrics={metrics} />}
            {activeTab === '样本示例' && <FocusSampleExamples datasets={datasets} distribution={distribution} samples={samples} />}
          </div>
          <footer className="is-link"><Button type="link" size="large">{activeTab === '规则贡献' ? '查看关联规则' : activeTab === '异常解释' ? '查看解释详情' : activeTab === '样本示例' ? '查看全部样本' : '查看特征详情'}</Button></footer>
        </>
      )}
    </section>
  );
}

function FocusImportantFeatures({ rows }: { rows: Array<Record<string, unknown>> }) {
  return <article className="taf-models-focus-single"><h2>重要特征贡献分布</h2><DataQualityTrendChart ariaLabel="聚焦态重要特征贡献分布" className="taf-models-focus-chart" categories={rows.map((item) => textValue(item, 'label'))} series={[{ name: '重要特征', color: '#18a8ff', type: 'bar', values: rows.map((item) => numberValue(item, 'value')) }]} valueFormatter={(value) => value.toFixed(3)} /></article>;
}

function FocusRuleContribution({ rows }: { rows: Array<Record<string, unknown>> }) {
  return <div className="taf-models-focus-two-column"><article><h2>规则正负贡献图</h2><div className="taf-models-focus-score"><span>基线分值 <b>0.500</b></span><span>最终分值 <b>0.873</b></span></div><DataQualityTrendChart ariaLabel="聚焦态规则正负贡献" className="taf-models-focus-chart" categories={rows.map((item) => textValue(item, 'rule'))} series={[{ name: '规则贡献', color: '#13c2c2', type: 'bar', values: rows.map((item) => numberValue(item, 'delta')) }]} valueFormatter={(value) => value.toFixed(3)} /></article><article><h2>规则贡献表（当前样本）</h2><div className="taf-models-focus-data-table is-rules"><div><span>规则</span><span>方向</span><span>权重</span><span>贡献</span></div>{rows.map((item) => <div key={textValue(item, 'rule')}><b>{textValue(item, 'rule')}</b><span>{textValue(item, 'direction') === 'protect' ? '保护' : '风险'}</span><span>{Math.abs(numberValue(item, 'score')).toFixed(2)}</span><em className={textValue(item, 'direction') === 'protect' ? 'is-protect' : 'is-risk'}>{numberValue(item, 'delta') > 0 ? '+' : ''}{numberValue(item, 'delta').toFixed(3)}</em></div>)}</div></article></div>;
}

function FocusAnomalyExplanation({ causes, samples, metrics }: { causes: Array<Record<string, unknown>>; samples: Array<Record<string, unknown>>; metrics: Array<Record<string, unknown>> }) {
  return <div className="taf-models-focus-two-column"><article><h2>异常原因解释</h2><div className="taf-models-focus-causes">{causes.map((item) => <div key={textValue(item, 'cause')}><b>{textValue(item, 'cause')}</b><span>{textValue(item, 'evidence')}</span><i style={{ '--confidence': `${Math.round(numberValue(item, 'confidence') * 100)}%` } as React.CSSProperties} /><strong>+{numberValue(item, 'confidence').toFixed(3)}</strong></div>)}</div><div className="taf-models-focus-confidence"><span>置信区间</span><b>{textValue(metrics.find((item) => textValue(item, 'label').includes('置信区间')), 'value') || '[0.942, 0.953]'}</b><span>当前得分</span><strong>0.948</strong></div></article><article><h2>相似样本（异常会话）</h2><div className="taf-models-focus-data-table is-samples"><div><span>时间</span><span>源 IP</span><span>相似原因</span><span>得分</span></div>{samples.map((item) => <div key={`${textValue(item, 'time')}-${textValue(item, 'src_ip')}`}><span>{textValue(item, 'time')}</span><b>{textValue(item, 'src_ip')}</b><span>{textValue(item, 'topic')}</span><em>{numberValue(item, 'score').toFixed(2)}</em></div>)}</div></article></div>;
}

function FocusSampleExamples({ datasets, distribution, samples }: { datasets: Array<Record<string, unknown>>; distribution: Array<Record<string, unknown>>; samples: Array<Record<string, unknown>> }) {
  return <div className="taf-models-focus-two-column is-sample"><article><h2>标签分布与抽样</h2><div className="taf-models-focus-datasets">{datasets.map((item) => <div key={textValue(item, 'name')}><b>{textValue(item, 'name')}</b><i style={{ '--ratio': `${numberValue(item, 'ratio')}%` } as React.CSSProperties} /><span>{numberValue(item, 'ratio')}%</span></div>)}</div><div className="taf-models-focus-distribution"><DataQualityDonutChart ariaLabel="聚焦态样本标签分布" className="taf-models-focus-donut" rows={distribution.map((item) => ({ label: textValue(item, 'label'), value: numberValue(item, 'value'), color: textValue(item, 'tone') === 'risk' ? '#ff4d4f' : textValue(item, 'tone') === 'warn' ? '#ffb020' : textValue(item, 'tone') === 'info' ? '#18a8ff' : '#36d66b' }))} /><div>{distribution.map((item) => <span key={textValue(item, 'label')}><b>{textValue(item, 'label')}</b><em>{numberValue(item, 'value')}%</em></span>)}</div></div></article><article><h2>样本示例（TP/FP）</h2><div className="taf-models-focus-data-table is-samples"><div><span>时间</span><span>源 IP</span><span>标签</span><span>原因</span><span>得分</span></div>{samples.map((item) => <div key={`${textValue(item, 'time')}-${textValue(item, 'src_ip')}`}><span>{textValue(item, 'time')}</span><b>{textValue(item, 'src_ip')}</b><StatusTag value={textValue(item, 'prediction')} /><span>{textValue(item, 'topic')}</span><em>{numberValue(item, 'score').toFixed(2)}</em></div>)}</div></article></div>;
}

function ChampionState({ selected, workbench }: { selected?: SnapshotRow; workbench?: ModelWorkbench }) {
  const activationPercent = 100;
  const versions = workbench?.versions ?? [];
  const active = versions.find((version) => version.status === 'active');
  const candidate = versions.find((version) => version.status === 'registered' || version.status === 'validating');
  const rollbackVersions = versions.filter((version) => version.status === 'deprecated').slice(0, 3);
  const reviewGates = workbenchItems(workbench, 'review_gates');
  return (
    <div className="taf-models-champion">
      <div className="taf-models-champion-card is-online">
        <span>Champion（线上）</span>
        <b>{active?.model_version ?? String(selected?.线上版本 ?? '暂无线上版本')}</b>
        <em>{String(selected?.模型名 ?? 'UEBA 行为分析')}</em>
        <strong>F1 {metricNumber(active?.metrics, 'f1_score', 0).toFixed(3)} | PostgreSQL</strong>
      </div>
      <div className="taf-models-gray">
        <span>模型注册表切换比例</span>
        <Slider min={5} max={100} value={activationPercent} disabled marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} />
        <div className="taf-models-gates">
          {reviewGates.length ? reviewGates.map((item) => <span key={textValue(item, 'name')}><SafetyCertificateOutlined />{textValue(item, 'name')}<StatusTag value={textValue(item, 'status')} /></span>) : <span>暂无评审门禁</span>}
        </div>
      </div>
      <div className="taf-models-champion-card is-candidate">
        <span>Challenger（候选）</span>
        <b>{candidate?.model_version ?? '暂无候选版本'}</b>
        <em>{String(selected?.模型名 ?? 'UEBA 行为分析')}</em>
        <strong>{candidate ? `F1 ${metricNumber(candidate.metrics, 'f1_score', 0).toFixed(3)} | ${candidate.status}` : '等待模型注册与评估'}</strong>
      </div>
      <div className="taf-models-versions">
        <span>可回滚版本</span>
        {rollbackVersions.length ? rollbackVersions.map((version) => <b key={version.model_version}>{version.model_version}<StatusTag value="已停用" /><em>{formatShortTime(version.updated_at)}</em></b>) : <b>暂无可回滚版本</b>}
      </div>
    </div>
  );
}

function DatasetSamples({ workbench }: { workbench?: ModelWorkbench }) {
  const datasets = workbenchItems(workbench, 'datasets');
  const distributionRows = workbenchItems(workbench, 'distribution');
  return (
    <div className="taf-models-dataset">
      <div className="taf-models-dataset-table">
        <div><span>数据集</span><span>样本数</span><span>时间范围</span><span>占比</span><span>质量</span></div>
        {datasets.length ? datasets.map((item) => <span key={textValue(item, 'name')}><b>{textValue(item, 'name')}</b><em>{numberValue(item, 'samples').toLocaleString()}</em><em>{textValue(item, 'range')}</em><em>{numberValue(item, 'ratio')}%</em><strong>{numberValue(item, 'quality')}%</strong></span>) : <span className="taf-models-empty">暂无数据集记录</span>}
      </div>
      <div className="taf-models-distribution">
        <DataQualityDonutChart
          ariaLabel="模型样本分布"
          className="taf-models-distribution-echart"
          rows={distributionRows.map((item) => ({
            label: textValue(item, 'label'),
            value: numberValue(item, 'value'),
            color: textValue(item, 'tone') === 'ok' ? '#36d66b' : textValue(item, 'tone') === 'info' ? '#18a8ff' : textValue(item, 'tone') === 'risk' ? '#ff4d4f' : '#ffb020',
          }))}
        />
        <div>
          {distributionRows.map((item) => <span key={textValue(item, 'label')}><i className={`is-${textValue(item, 'tone')}`} />{textValue(item, 'label')}<b>{numberValue(item, 'value')}%</b></span>)}
        </div>
      </div>
    </div>
  );
}

function ActivationRollback({ selected, workbench, onAction }: { selected?: SnapshotRow; workbench?: ModelWorkbench; onAction: OpenModelAction }) {
  const activationPercent = 100;
  const rollbackVersion = workbench?.versions.find((version) => version.status === 'deprecated')?.model_version ?? '';
  const activeVersion = workbench?.versions.find((version) => version.status === 'active')?.model_version ?? '';
  const reviewRows = workbenchItems(workbench, 'review_gates');
  const activationBlocked = reviewRows.length === 0 || reviewRows.some((item) => !isApprovedGate(textValue(item, 'status')));
  const candidateVersion = workbench?.versions.find((version) => version.status === 'registered' || version.status === 'validating')?.model_version ?? '';
  const recentAction = workbench?.actions[0];
  return (
    <div className="taf-models-activation">
      <div className="taf-models-activation-controls">
        <span>注册表全量切换 <b>{activationPercent}%</b></span>
        <Slider min={5} max={100} value={activationPercent} disabled marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} />
        <Tooltip title={activationBlocked ? `门禁未通过：${reviewRows.length ? pendingGateNames(reviewRows) : '缺少持久化评审门禁'}` : !candidateVersion ? '没有可激活候选版本' : `全量激活 ${candidateVersion}；分阶段流量请使用部署编排`}>
          <Button size="small" type="primary" disabled={activationBlocked || !candidateVersion} data-model-action-managed="true" onClick={() => onAction('激活到线上', 'model-version-activate', { ...selected, 版本: candidateVersion }, { gray_percent: activationPercent })}>激活到线上</Button>
        </Tooltip>
        <Button size="small" danger ghost disabled={!activeVersion} data-model-action-managed="true" onClick={() => onAction('停用模型', 'model-version-deprecate', { ...selected, 版本: activeVersion })}>停用模型</Button>
        <Button size="small" danger data-model-action-managed="true" disabled={!rollbackVersion} onClick={() => onAction(`回滚到 ${rollbackVersion}`, 'model-version-rollback', { ...selected, 版本: rollbackVersion })}>回滚到 {rollbackVersion || '无可回滚版本'}</Button>
      </div>
      <div className="taf-models-review">
        {reviewRows.length ? reviewRows.map((item) => <span key={textValue(item, 'name')}><b>{textValue(item, 'name')}</b><StatusTag value={textValue(item, 'status')} /><em>by {textValue(item, 'owner')}</em><i>{textValue(item, 'time')}</i></span>) : <span>暂无评审记录</span>}
      </div>
      <div className="taf-models-ops"><b>最近操作记录</b>{recentAction ? <><span>{formatShortTime(recentAction.created_at)}</span><span>{recentAction.action}</span><em>by {recentAction.requested_by}</em></> : <span>暂无服务端操作记录</span>}</div>
    </div>
  );
}

function ModelMetrics({ selected, workbench }: { selected?: SnapshotRow; workbench?: ModelWorkbench }) {
  const persisted = workbenchItems(workbench, 'metrics');
  const metricSeries = persisted.length ? persisted.map((item) => [
    textValue(item, 'label'),
    `${item.value ?? '-' }${textValue(item, 'unit')}`,
    numberValue(item, 'delta') > 0 ? `+${numberValue(item, 'delta')}` : `${numberValue(item, 'delta')}`,
    textValue(item, 'tone') || 'info',
  ]) : buildMetricSeries(selected);
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

function FeatureExplain({ workbench, loading }: { workbench?: ModelWorkbench; loading: boolean }) {
  const tabs = ['重要特征', '规则贡献', '异常解释', '样本示例'];
  const [activeTab, setActiveTab] = useState(tabs[0]);
  const features = workbenchItems(workbench, 'features');
  const contributions = workbenchItems(workbench, 'rule_contributions');
  const causes = workbenchItems(workbench, 'anomaly_causes');
  const similarSamples = workbenchItems(workbench, 'similar_samples');
  const samples = workbenchItems(workbench, 'samples');
  const distribution = workbenchItems(workbench, 'distribution');
  return (
    <div className="taf-models-explain">
      <div className="taf-models-tabs">{tabs.map((tab) => <button key={tab} type="button" data-model-action-managed="true" className={tab === activeTab ? 'is-active' : ''} onClick={() => setActiveTab(tab)}>{tab}</button>)}</div>
      {loading && <div className="taf-models-empty">正在加载 PostgreSQL 工作台…</div>}
      {!loading && activeTab === '重要特征' && <ExplainFeatureChart rows={features} />}
      {!loading && activeTab === '规则贡献' && <ExplainRuleContributions rows={contributions} />}
      {!loading && activeTab === '异常解释' && <ExplainAnomalies causes={causes} similarSamples={similarSamples} />}
      {!loading && activeTab === '样本示例' && <ExplainSamples samples={samples} distribution={distribution} />}
    </div>
  );
}

function ExplainFeatureChart({ rows }: { rows: Array<Record<string, unknown>> }) {
  if (!rows.length) return <div className="taf-models-empty">暂无特征重要性数据</div>;
  return <DataQualityTrendChart ariaLabel="重要特征贡献分布" className="taf-models-feature-echart" categories={rows.map((item) => textValue(item, 'label'))} series={[{ name: '重要特征', color: '#18a8ff', type: 'bar', values: rows.map((item) => numberValue(item, 'value')) }]} valueFormatter={(value) => value.toFixed(3)} />;
}

function ExplainRuleContributions({ rows }: { rows: Array<Record<string, unknown>> }) {
  if (!rows.length) return <div className="taf-models-empty">暂无规则贡献数据</div>;
  return <div className="taf-models-contributions"><DataQualityTrendChart ariaLabel="规则正负贡献" className="taf-models-feature-echart" categories={rows.map((item) => textValue(item, 'rule'))} series={[{ name: '贡献', color: '#18a8ff', type: 'bar', values: rows.map((item) => numberValue(item, 'delta')) }]} valueFormatter={(value) => value.toFixed(2)} /><div>{rows.map((item) => <span key={textValue(item, 'rule')}><b>{textValue(item, 'rule')}</b><em className={`is-${textValue(item, 'direction')}`}>{numberValue(item, 'delta') > 0 ? '+' : ''}{numberValue(item, 'delta').toFixed(2)}</em></span>)}</div></div>;
}

function ExplainAnomalies({ causes, similarSamples }: { causes: Array<Record<string, unknown>>; similarSamples: Array<Record<string, unknown>> }) {
  return <div className="taf-models-anomaly-explain"><div>{causes.map((item) => <article key={textValue(item, 'cause')}><b>{textValue(item, 'cause')}</b><strong>{Math.round(numberValue(item, 'confidence') * 100)}%</strong><span>{textValue(item, 'evidence')}</span></article>)}</div><div className="taf-models-samples"><div><span>相似样本</span><span>相似度</span><span>判定</span><span>摘要</span></div>{similarSamples.map((item) => <span key={textValue(item, 'sample_id')}><b>{textValue(item, 'sample_id')}</b><em>{Math.round(numberValue(item, 'similarity') * 100)}%</em><StatusTag value={textValue(item, 'verdict')} /><strong>{textValue(item, 'summary')}</strong></span>)}</div></div>;
}

function ExplainSamples({ samples, distribution }: { samples: Array<Record<string, unknown>>; distribution: Array<Record<string, unknown>> }) {
  return <div className="taf-models-sample-examples"><DataQualityDonutChart ariaLabel="样本标签分布" className="taf-models-sample-donut" rows={distribution.map((item) => ({ label: textValue(item, 'label'), value: numberValue(item, 'value'), color: textValue(item, 'tone') === 'risk' ? '#ff4d4f' : textValue(item, 'tone') === 'warn' ? '#ffb020' : textValue(item, 'tone') === 'info' ? '#18a8ff' : '#36d66b' }))} /><div className="taf-models-samples"><div><span>时间</span><span>源 IP</span><span>标签/预测</span><span>得分</span></div>{samples.map((item) => <span key={`${textValue(item, 'time')}-${textValue(item, 'src_ip')}`}><b>{textValue(item, 'time')}</b><em>{textValue(item, 'src_ip')}</em><em>{textValue(item, 'label')} / {textValue(item, 'prediction')}</em><strong>{numberValue(item, 'score').toFixed(2)}</strong></span>)}</div></div>;
}

type OpenModelAction = (label: string, actionId?: ModelActionId, targetRow?: SnapshotRow, payload?: Record<string, unknown>) => void;

const renderModelCell = (column: string, value: unknown, record: SnapshotRow, onAction: OpenModelAction) => {
  if (column === '模型名') return <span className="taf-models-name"><ExperimentOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return (
    <span className="taf-models-row-actions">
      <button type="button" title="评估模型" aria-label="评估模型" data-model-action-managed="true" onClick={(event) => { event.stopPropagation(); onAction('评估模型', 'model-evaluation-request', record); }}><BarChartOutlined /></button>
      <button type="button" title="查看模型上下文" aria-label="查看模型上下文" data-model-action-managed="true" onClick={(event) => { event.stopPropagation(); onAction('查看模型上下文', 'model-context-action', record); }}><SearchOutlined /></button>
      <button type="button" title="请在右侧激活区选择候选版本" aria-label="请在右侧激活区选择候选版本" data-model-action-managed="true" disabled><BranchesOutlined /></button>
    </span>
  );
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.__model_id ?? row.模型名 ?? row.model_id ?? JSON.stringify(row));

const numericRowValue = (row: SnapshotRow | undefined, key: string, fallback: number) => {
  const value = Number(row?.[key]);
  return Number.isFinite(value) ? value : fallback;
};

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

const workbenchItems = (workbench: ModelWorkbench | undefined, category: string) => workbench?.items?.[category] ?? [];

const isApprovedGate = (status: string) => ['通过', '已通过', 'passed', 'approved'].includes(status.trim().toLowerCase());

const pendingGateNames = (rows: Array<Record<string, unknown>>) => rows.filter((item) => !isApprovedGate(textValue(item, 'status'))).map((item) => textValue(item, 'name')).join('、');

const textValue = (item: Record<string, unknown> | undefined, key: string) => String(item?.[key] ?? '');

const numberValue = (item: Record<string, unknown> | undefined, key: string) => {
  const value = Number(item?.[key]);
  return Number.isFinite(value) ? value : 0;
};

const metricNumber = (metrics: Record<string, unknown> | undefined, key: string, fallback: number) => {
  const value = Number(metrics?.[key]);
  return Number.isFinite(value) ? value : fallback;
};

const activationThreshold = (index: number, metrics: Array<Record<string, unknown>>) => {
  void metrics;
  if (index === 0) return 'F1 ≥ 0.94';
  if (index === 1) return '误报率 ≤ 1.0%';
  if (index === 2) return 'PSI ≤ 0.15';
  return '审计记录完整';
};

const activationEvidence = (index: number, metrics: Array<Record<string, unknown>>) => {
  const metric = (label: string) => metrics.find((item) => textValue(item, 'label').includes(label));
  if (index === 0) return `F1 = ${numberValue(metric('F1'), 'value').toFixed(3)}`;
  if (index === 1) return `误报率 = ${numberValue(metric('误报率'), 'value').toFixed(2)}%`;
  if (index === 2) return `PSI = ${numberValue(metric('漂移'), 'value').toFixed(2)}`;
  return '发布与回滚均需留痕';
};

const formatShortTime = (value: string) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value || '-';
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false });
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('F1') ? '0.947' : label.includes('误报') ? '-6.2%' : '0',
  delta: 'API',
  status: 'info',
});
