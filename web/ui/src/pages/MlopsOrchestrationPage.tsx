import {
  ApiOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  CloudUploadOutlined,
  DatabaseOutlined,
  FileSearchOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RocketOutlined,
  SafetyCertificateOutlined,
  TagsOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Descriptions, Drawer, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { DataQualityKpiSparklineChart, DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { buildMlopsActionRequest, fetchMlopsWorkspace, submitMlopsAction, type MlopsActionId, type MlopsActionRequest, type MlopsWorkflow } from '@/services/mlopsActionApi';
import type { ModelWorkbench } from '@/services/modelWorkbenchApi';
import { pageApiPlans, type ActionEndpointPlan } from '@/services/pageApiPlans';

type MlopsRecord = Record<string, unknown>;

const dagIcons: ReactNode[] = [
  <DatabaseOutlined key="feedback" />,
  <TagsOutlined key="label" />,
  <ApiOutlined key="feature" />,
  <BranchesOutlined key="train" />,
  <SafetyCertificateOutlined key="eval" />,
  <CheckCircleOutlined key="register" />,
  <RocketOutlined key="release" />,
  <ReloadOutlined key="loop" />,
];

const mlopsActionPlans = pageApiPlans.mlops.actions ?? [];
const mlopsPageSize = 6;

type MlopsActionState = {
  actionId: MlopsActionId;
  label: string;
  plan: ActionEndpointPlan;
  endpoint: string;
  modelId: string;
  targetId: string;
  target: string;
  version?: string;
  featureSetId?: string;
  artifactUri?: string;
  request: MlopsActionRequest;
  localReadOnly?: boolean;
  details?: SnapshotRow;
};

type OpenMlopsAction = (label: string, actionId?: MlopsActionId, targetRow?: SnapshotRow) => void;

export function MlopsOrchestrationPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [selectedKey, setSelectedKey] = useState<string>();
  const [page, setPage] = useState(1);
  const [taskFilter, setTaskFilter] = useState('全部任务');
  const [actionState, setActionState] = useState<MlopsActionState>();
  const actionMutation = useMutation({
    mutationFn: submitMlopsAction,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['mlops-workspace'] });
    },
  });
  const { data: workspace, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['mlops-workspace'],
    queryFn: fetchMlopsWorkspace,
  });

  const workbench = workspace?.workbench;
  const feedbackSamples = useMemo(() => workbenchItems(workbench, 'mlops_feedback_samples'), [workbench]);
  const evaluationGates = useMemo(() => buildEvaluationGates(workbench), [workbench]);
  const releases = useMemo(() => buildReleaseRows(workbench, workspace?.model.name ?? ''), [workbench, workspace?.model.name]);
  const pipelineStages = useMemo(() => buildPipelineStages(workbenchItems(workbench, 'mlops_pipeline_stages'), workspace?.workflows ?? [], releases), [workbench, workspace?.workflows, releases]);
  const feedbackDaily = useMemo(() => workbenchItems(workbench, 'mlops_feedback_daily'), [workbench]);
  const summary = useMemo(() => workbenchItems(workbench, 'mlops_summary')[0] ?? {}, [workbench]);
  const rows = useMemo(() => (workspace?.workflows ?? []).map(workflowTaskRow), [workspace?.workflows]);
  const filteredRows = useMemo(() => taskFilter === '全部任务' ? rows : rows.filter((row) => String(row.状态 ?? '').includes(taskFilter)), [rows, taskFilter]);
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / mlopsPageSize));
  const visibleRows = filteredRows.slice((page - 1) * mlopsPageSize, page * mlopsPageSize);
  const selected = useMemo(() => visibleRows.find((row) => rowKey(row) === selectedKey) ?? visibleRows[0], [selectedKey, visibleRows]);
  const metrics = buildMlopsMetrics(route.page.kpis, rows, releases, evaluationGates);
  const openAction: OpenMlopsAction = (label, actionId = 'mlops-context-action', targetRow = selected) => {
    const plan = mlopsActionPlans.find((item) => item.id === actionId) ?? mlopsActionPlans[0];
    const modelId = workspace?.model.model_id;
    if (!plan || !modelId) return;
    const isModelAction = actionId === 'mlops-model-register' || actionId === 'mlops-model-version-inspect';
    const targetId = String(isModelAction ? modelId : targetRow?.任务ID ?? targetRow?.['任务 ID'] ?? targetRow?.__task_id ?? targetRow?.feedback_id ?? 'new-workflow');
    const version = String(targetRow?.__version ?? targetRow?.版本 ?? workbench?.versions[0]?.model_version ?? 'current');
    const featureSetId = workbench?.versions[0]?.feature_set_id;
    const artifactUri = String(targetRow?.artifact_uri ?? workbench?.versions[0]?.artifact_uri ?? '');
    const input = { actionId, modelId, targetId, target: String(targetRow?.阶段 ?? targetRow?.任务ID ?? label), version, featureSetId, artifactUri };
    const request = buildMlopsActionRequest(input);
    actionMutation.reset();
    setActionState({
      actionId,
      label,
      plan,
      endpoint: request.endpoint,
      modelId,
      targetId,
      target: input.target,
      version,
      featureSetId,
      artifactUri,
      request,
      localReadOnly: actionId === 'mlops-feedback-inspect',
      details: targetRow,
    });
  };
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    width: column === '操作' ? 86 : column === '任务ID' || column === '任务 ID' ? 166 : column === '状态' ? 72 : 110,
    fixed: column === '操作' ? 'right' : undefined,
    render: (value, record) => renderMlopsCell(column, value, record, openAction),
  }));

  return (
    <div className="taf-page taf-mlops">
      <section className="taf-mlops-shell">
        <main className="taf-mlops-main">
          <header className="taf-mlops-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              <small>Argo {String(workspace?.orchestrator.argo_namespace ?? '-')} / {String(workspace?.orchestrator.workflow_template ?? '-')} · PostgreSQL 模型 {workspace?.model.name ?? '-'}</small>
            </div>
            <Space size={6}>
              <Button size="small" type="primary" icon={<BranchesOutlined />} onClick={() => openAction('按策略触发训练流水线', 'mlops-pipeline-create')}>触发训练流水线</Button>
              <Button size="small" icon={<PlayCircleOutlined />} onClick={() => openAction('投递训练任务', 'mlops-training-submit')}>投递训练任务</Button>
              <Button size="small" icon={<ReloadOutlined />} disabled={!selected?.__can_retry} onClick={() => openAction('失败重试', 'mlops-task-retry')}>失败重试</Button>
              <Button size="small" danger ghost icon={<PauseCircleOutlined />} disabled={!selected?.__can_stop} onClick={() => openAction('停止任务', 'mlops-task-stop')}>停止任务</Button>
              <Tooltip title="模型由已签名 Argo 训练流水线自动注册；页面不允许伪造候选版本"><Button size="small" icon={<CloudUploadOutlined />} disabled>注册模型</Button></Tooltip>
              <Button size="small" icon={<RocketOutlined />} onClick={() => navigate(`/deployments?model_id=${encodeURIComponent(workspace?.model.model_id ?? '')}&model_version=${encodeURIComponent(workbench?.versions[0]?.model_version ?? '')}`)}>进入部署管理</Button>
              <Tooltip title="刷新编排状态">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <Button size="small" icon={<FileSearchOutlined />} disabled={!selected} onClick={() => openAction('任务详情', 'mlops-task-inspect', selected)}>任务详情</Button>
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/mlops/status、/v1/mlops/conditions、APISIX 路由或 rule-manager orchestrator。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-mlops-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-mlops-workbench">
            <section className="taf-mlops-left">
              <WorkPanel
                title={`MLOps 闭环编排 DAG（当前流水线：${String(selected?.阶段 ?? '异常流量检测 v3')}）`}
                extra={<span className="taf-mlops-legend"><i className="is-ok" />成功<i className="is-info" />运行中<i className="is-warn" />待处理<i className="is-risk" />失败</span>}
              >
                <MlopsDag stages={pipelineStages} />
              </WorkPanel>

              <div className="taf-mlops-bottom">
                <WorkPanel title="反馈样本池（PostgreSQL 验收样本 Top 10）" extra={<Tooltip title="真实标注任务服务未接入，已安全禁用"><Button size="small" disabled>发起标注</Button></Tooltip>}>
                  <FeedbackPool rows={feedbackSamples} summary={summary} onAction={openAction} />
                </WorkPanel>
                <WorkPanel title="训练任务队列" extra={<Select size="small" value={taskFilter} onChange={(value) => { setTaskFilter(value); setPage(1); }} options={['全部任务', '运行中', '排队中', '失败'].map((value) => ({ value }))} />}>
                  <Table
                    rowKey={rowKey}
                    size="small"
                    loading={isLoading}
                    pagination={false}
                    scroll={{ x: 930 }}
                    columns={columns}
                    dataSource={visibleRows}
                    rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                    onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
                  />
                  <div className="taf-mlops-pagination"><span>共 {filteredRows.length} 条</span>{Array.from({ length: totalPages }, (_, index) => index + 1).map((item) => <button key={item} type="button" className={item === page ? 'is-active' : ''} aria-current={item === page ? 'page' : undefined} onClick={() => setPage(item)}>{item}</button>)}</div>
                </WorkPanel>
              </div>
            </section>

            <aside className="taf-mlops-right">
              <WorkPanel title="模型版本观测阈值（非 Argo 注册门禁）">
                <EvaluationGate rows={evaluationGates} summary={summary} />
              </WorkPanel>
              <WorkPanel title="注册与发布" extra={<Tooltip title="注册由 Argo 流水线完成"><Button size="small" disabled>自动注册</Button></Tooltip>}>
                <RegisterRelease rows={releases} modelId={workspace?.model.model_id ?? ''} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="效果回流（PostgreSQL 验收种子，近7日）">
                <FeedbackLoop rows={feedbackDaily} summary={summary} />
              </WorkPanel>
            </aside>
          </div>
        </main>
      </section>
      <Drawer
        className="taf-mlops-action-drawer"
        title={actionState?.label ?? 'MLOps 业务动作'}
        open={Boolean(actionState)}
        width={520}
        onClose={() => { setActionState(undefined); actionMutation.reset(); }}
        extra={actionState?.localReadOnly ? undefined : <Button size="small" type="primary" loading={actionMutation.isPending} disabled={!actionState?.request.supported} onClick={() => actionState && actionMutation.mutate({ actionId: actionState.actionId, modelId: actionState.modelId, targetId: actionState.targetId, target: actionState.target, version: actionState.version, featureSetId: actionState.featureSetId, artifactUri: actionState.artifactUri })}>{actionState?.request.method === 'GET' ? '查询详情' : '确认提交'}</Button>}
      >
        {actionState && <div className="taf-mlops-action-body">
          <Descriptions size="small" column={1} bordered>
            <Descriptions.Item label="接口">{actionState.request.method} {actionState.endpoint}</Descriptions.Item>
            <Descriptions.Item label="权限">{actionState.plan.requiredScopes.join(', ')}</Descriptions.Item>
            <Descriptions.Item label="审计事件">{actionState.request.method === 'GET' ? '只读操作，不伪造审计事件' : '由服务端响应返回'}</Descriptions.Item>
            <Descriptions.Item label="实际请求体"><code>{JSON.stringify(actionState.request.body ?? {})}</code></Descriptions.Item>
            {actionState.actionId === 'mlops-task-inspect' && <>
              <Descriptions.Item label="阶段">{String(actionState.details?.阶段 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="数据集">{String(actionState.details?.数据集版本 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="算法配置">{String(actionState.details?.算法配置 ?? actionState.details?.模型配置 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="特征版本">{String(actionState.details?.特征版本 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="资源占用">{String(actionState.details?.资源占用 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="任务状态">{String(actionState.details?.状态 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="数据模式">{String(actionState.details?.__data_mode ?? 'Argo live')}</Descriptions.Item>
            </>}
            {actionState.actionId === 'mlops-feedback-inspect' && <>
              <Descriptions.Item label="反馈样本">{String(actionState.details?.feedback_id ?? actionState.targetId)}</Descriptions.Item>
              <Descriptions.Item label="来源">PostgreSQL acceptance seed（明确标记，非生产标注任务）</Descriptions.Item>
            </>}
          </Descriptions>
          <Alert type="info" showIcon message={actionState.localReadOnly ? '只读本地详情' : '真实 Argo API、RBAC 与服务端审计'} description={actionState.plan.guardrails.join('；')} />
          {actionMutation.data && <Alert type="success" showIcon message={`工作流 ${actionMutation.data.jobId} 已返回`} description={`${actionMutation.data.auditEvent}；${actionMutation.data.endpoint}；状态 ${actionMutation.data.status}`} />}
          {actionMutation.isError && <Alert type="error" showIcon message="MLOps 动作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : '未知错误'} />}
        </div>}
      </Drawer>
    </div>
  );
}

function MlopsDag({ stages }: { stages: MlopsRecord[] }) {
  const activeIndex = stages.findIndex((stage) => textValue(stage, 'source') === 'Argo live' && ['info', 'risk'].includes(textValue(stage, 'tone')));
  return (
    <div className="taf-mlops-dag">
      {stages.map((stage, index) => {
        const title = textValue(stage, 'title');
        const tone = textValue(stage, 'tone') || 'info';
        return (
        <div key={title} className={`taf-mlops-dag-node is-${tone} ${index === (activeIndex >= 0 ? activeIndex : 5) ? 'is-active' : ''}`}>
          <span>{title}</span>
          {dagIcons[index] ?? <BranchesOutlined />}
          <b>{textValue(stage, 'value')}</b>
          <em>{textValue(stage, 'caption')}</em>
          <small title={`来源: ${textValue(stage, 'source')}`}>队列: {textValue(stage, 'queue')} · {textValue(stage, 'source').includes('acceptance') ? '验收种子' : textValue(stage, 'source')}</small>
          <strong>{textValue(stage, 'duration')}</strong>
        </div>
        );
      })}
    </div>
  );
}

function FeedbackPool({ rows, summary, onAction }: { rows: MlopsRecord[]; summary: MlopsRecord; onAction: OpenMlopsAction }) {
  const tp = rows.filter((row) => textValue(row, 'label') === 'TP').length;
  const fp = rows.filter((row) => textValue(row, 'label') === 'FP').length;
  return (
    <div className="taf-mlops-feedback-pool">
      <div><span>ID</span><span>TP/FP</span><span>误报原因</span><span>来源告警</span><span>白名单建议</span><span>样本质量</span><span>入池时间</span><span>操作</span></div>
      {rows.map((row) => {
        const id = textValue(row, 'feedback_id');
        const label = textValue(row, 'label');
        const cells = [id, label, textValue(row, 'reason'), textValue(row, 'alert_id'), textValue(row, 'whitelist_suggestion'), textValue(row, 'quality'), textValue(row, 'received_at'), '查看'];
        return <button key={id} type="button" onClick={() => onAction(`查看反馈样本 ${id}`, 'mlops-feedback-inspect', { 任务ID: id, 阶段: '反馈样本', feedback_id: id, __data_mode: textValue(row, 'source') })}>
          {cells.map((cell, index) => <span key={`${id}-${index}`} className={index === 1 ? `is-${cell.toLowerCase()}` : ''}>{cell}</span>)}
        </button>;
      })}
      <footer>当前展示 {rows.length} 条 <b>TP {tp}</b><b>FP {fp}</b><b>工作台总样本 {formatNumber(numberValue(summary, 'pool_samples'))}</b></footer>
    </div>
  );
}

function EvaluationGate({ rows, summary }: { rows: MlopsRecord[]; summary: MlopsRecord }) {
  const confusion = recordValue(summary, 'confusion');
  const total = Object.values(confusion).reduce<number>((sum, item) => sum + Number(item || 0), 0);
  const allPassed = rows.length > 0 && rows.every((row) => textValue(row, 'status') === '通过');
  return (
    <div className="taf-mlops-gate">
      <div className="taf-mlops-gate-table">
        <div><span>指标</span><span>当前值</span><span>阈值</span><span>结果</span><span>版本趋势</span></div>
        {rows.map((row) => {
          const label = textValue(row, 'label');
          return <span key={label}><b>{label}</b><em>{formatGateValue(label, numberValue(row, 'value'))}</em><em>{textValue(row, 'operator')} {formatGateValue(label, numberValue(row, 'threshold'))}</em><StatusTag value={textValue(row, 'status')} /><DataQualityKpiSparklineChart ariaLabel={`${label}最近模型版本趋势`} className="taf-mlops-gate-echart" tone="ok" values={numberArray(row, 'trend').map((item) => item * 100)} /></span>;
        })}
      </div>
      <div className="taf-mlops-confusion">
        <b>验收种子矩阵（非当前模型）</b>
        <div><span>预测\\真实</span><span>正常</span><span>异常</span><span>合计</span></div>
        <div><span>正常</span><b>{formatNumber(numberValue(confusion, 'tn'))}</b><b>{formatNumber(numberValue(confusion, 'fp'))}</b><b>{formatNumber(numberValue(confusion, 'tn') + numberValue(confusion, 'fp'))}</b></div>
        <div><span>异常</span><b>{formatNumber(numberValue(confusion, 'fn'))}</b><b>{formatNumber(numberValue(confusion, 'tp'))}</b><b>{formatNumber(numberValue(confusion, 'fn') + numberValue(confusion, 'tp'))}</b></div>
        <div><span>合计</span><b>{formatNumber(numberValue(confusion, 'tn') + numberValue(confusion, 'fn'))}</b><b>{formatNumber(numberValue(confusion, 'fp') + numberValue(confusion, 'tp'))}</b><b>{formatNumber(total)}</b></div>
        <strong>产品阈值结果 <StatusTag value={allPassed ? '通过' : '未通过'} /></strong>
      </div>
    </div>
  );
}

function RegisterRelease({ rows, modelId, onAction }: { rows: MlopsRecord[]; modelId: string; onAction: OpenMlopsAction }) {
  return (
    <div className="taf-mlops-register">
      <div><span>模型包</span><span>版本</span><span>签名</span><span>模型卡</span><span>线上候选</span><span>灰度策略</span><span>状态</span><span>操作</span></div>
      {rows.map((row) => {
        const name = textValue(row, 'model_name');
        const version = textValue(row, 'version');
        const cells = [name, version, textValue(row, 'signature'), '查看', textValue(row, 'candidate'), textValue(row, 'gray_policy')];
        return <button key={`${name}-${version}`} type="button" title={`${name} ${version}`} onClick={() => onAction(`查看模型包 ${name} ${version}`, 'mlops-model-version-inspect', { __model_id: modelId, __version: version, 模型包: name, 版本: version, 阶段: '注册与发布', artifact_uri: textValue(row, 'artifact_uri') })}>{cells.map((cell, cellIndex) => <span key={`${cell}-${cellIndex}`} title={String(cell)}>{cell}</span>)}<StatusTag value={textValue(row, 'status')} /><FileSearchOutlined /></button>;
      })}
    </div>
  );
}

function FeedbackLoop({ rows, summary }: { rows: MlopsRecord[]; summary: MlopsRecord }) {
  const metrics = [
    ['告警命中 (TP)', formatNumber(numberValue(summary, 'tp')), '+12.3%', 'ok'],
    ['误报 (FP)', formatNumber(numberValue(summary, 'fp')), '-8.1%', 'ok'],
    ['反馈样本量', formatNumber(numberValue(summary, 'feedback_samples')), '+15.6%', 'info'],
    ['误报率', `${numberValue(summary, 'false_positive_rate').toFixed(2)}%`, '-0.45pp', 'ok'],
    ['漂移(PSI)', numberValue(summary, 'psi').toFixed(2), '+0.07', 'warn'],
  ];
  return (
    <div className="taf-mlops-loop">
      <div className="taf-mlops-loop-metrics">
        {metrics.map(([label, value, delta, tone]) => <span key={label}><em>{label}</em><b className={`is-${tone}`}>{value}</b><strong>{delta}</strong></span>)}
      </div>
      <div className="taf-mlops-funnel">
        <span>告警总量 <b>{formatNumber(numberValue(summary, 'alerts_total'))}</b></span>
        <span>有效告警 <b>{formatNumber(numberValue(summary, 'effective_alerts'))}</b></span>
        <span>入池样本 <b>{formatNumber(numberValue(summary, 'pool_samples'))}</b></span>
        <span>已标注 <b>{formatNumber(numberValue(summary, 'labeled_samples'))}</b></span>
      </div>
      <div className="taf-mlops-trend">
        <b>误报率 & 漂移趋势</b>
        <DataQualityTrendChart
          ariaLabel="误报率与漂移近 7 日趋势"
          className="taf-mlops-feedback-echart"
          categories={rows.map((row) => textValue(row, 'day'))}
          series={[
            { name: '误报率', color: '#18a8ff', values: rows.map((row) => numberValue(row, 'false_positive_rate')) },
            { name: 'PSI', color: '#36d66b', values: rows.map((row) => numberValue(row, 'psi')) },
          ]}
          valueFormatter={(value) => value.toFixed(1)}
        />
      </div>
    </div>
  );
}

const renderMlopsCell = (column: string, value: unknown, record: SnapshotRow, onAction: OpenMlopsAction) => {
  if (column === '任务ID' || column === '任务 ID') return <span className="taf-mlops-task-id"><BranchesOutlined />{String(value)}<small>{String(record.__data_mode ?? 'Argo live')}</small></span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '资源占用') return <span className="taf-mlops-resource">{String(value)}</span>;
  if (column === '操作') return <span className="taf-mlops-row-actions">
    <button type="button" title="查看任务" aria-label="查看任务" onClick={(event) => { event.stopPropagation(); onAction('查看任务', 'mlops-task-inspect', record); }}><FileSearchOutlined /></button>
    <button type="button" title="停止任务" aria-label="停止任务" disabled={!record.__can_stop} onClick={(event) => { event.stopPropagation(); onAction('停止任务', 'mlops-task-stop', record); }}><CloseCircleOutlined /></button>
    <button type="button" title="重试任务" aria-label="重试任务" disabled={!record.__can_retry} onClick={(event) => { event.stopPropagation(); onAction('重试任务', 'mlops-task-retry', record); }}><ReloadOutlined /></button>
  </span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.任务ID ?? row['任务 ID'] ?? JSON.stringify(row));

const workbenchItems = (workbench: ModelWorkbench | undefined, category: string): MlopsRecord[] =>
  (workbench?.items[category] ?? []).map((item) => item as MlopsRecord);

const buildEvaluationGates = (workbench: ModelWorkbench | undefined): MlopsRecord[] => {
  const version = workbench?.versions.find((item) => item.status === 'active') ?? workbench?.versions[0];
  const metrics = version?.metrics ?? {};
  const historicalVersions = [...(workbench?.versions ?? [])]
    .sort((left, right) => Date.parse(left.created_at) - Date.parse(right.created_at))
    .slice(-7);
  const definitions: Array<[string, string[], number, '>=' | '<=']> = [
    ['准确率', ['accuracy'], 0.92, '>='],
    ['召回率', ['recall'], 0.90, '>='],
    ['F1', ['f1_score', 'f1'], 0.92, '>='],
    ['误报率', ['false_positive_rate', 'fp_rate'], 0.05, '<='],
    ['漂移(PSI)', ['psi', 'drift'], 0.25, '<='],
  ];
  return definitions.flatMap(([label, keys, threshold, operator]) => {
    const raw = keys.map((key) => metrics[key]).find((value) => Number.isFinite(Number(value)));
    if (raw === undefined) return [];
    const value = Number(raw);
    const passed = operator === '>=' ? value >= threshold : value <= threshold;
    const trend = historicalVersions.flatMap((historicalVersion) => {
      const historicalValue = keys.map((key) => historicalVersion.metrics?.[key]).find((candidate) => Number.isFinite(Number(candidate)));
      return historicalValue === undefined ? [] : [Number(historicalValue)];
    });
    return [{ label, value, threshold, operator, status: passed ? '通过' : '未通过', trend: trend.length > 0 ? trend : [value], source: `model_versions/${version?.model_version ?? '-'}/history` }];
  });
};

const buildReleaseRows = (workbench: ModelWorkbench | undefined, modelName: string): MlopsRecord[] =>
  [...(workbench?.versions ?? [])]
    .sort((left, right) => Number(right.status === 'active') - Number(left.status === 'active'))
    .slice(0, 4)
    .map((version) => ({
    model_name: modelName,
    version: version.model_version,
    signature: version.metrics?.artifact_sha256 ? 'SHA 已绑定' : '未验证',
    candidate: version.status === 'active' ? '线上' : '否',
    gray_policy: '进入部署编排',
    status: version.status === 'active' ? '已上线' : version.status === 'deprecated' ? '已下线' : '待评审',
    artifact_uri: version.artifact_uri,
    source: 'PostgreSQL model_versions',
    }));

const buildPipelineStages = (seedStages: MlopsRecord[], workflows: MlopsWorkflow[], releases: MlopsRecord[]): MlopsRecord[] => {
  const stages: MlopsRecord[] = seedStages.map((stage) => ({ ...stage, source: 'PostgreSQL acceptance seed' }));
  const running = workflows.filter((item) => ['Pending', 'Running'].includes(item.phase)).length;
  const failed = workflows.filter((item) => ['Failed', 'Error'].includes(item.phase)).length;
  if (stages[3]) stages[3] = { ...stages[3], value: String(running), caption: 'Argo 运行中', queue: 'argo', tone: running ? 'info' : 'ok', source: 'Argo live' };
  if (stages[4]) stages[4] = { ...stages[4], value: String(failed), caption: 'Argo 失败任务', queue: 'argo', tone: failed ? 'risk' : 'ok', source: 'Argo live' };
  if (stages[5]) stages[5] = { ...stages[5], value: String(releases.length), caption: '真实模型版本', queue: 'postgresql', tone: 'ok', source: 'PostgreSQL live' };
  return stages;
};

const workflowTaskRow = (workflow: MlopsWorkflow): SnapshotRow => ({
  __task_id: workflow.name,
  __data_mode: 'Argo live',
  __can_stop: workflow.can_stop ? 1 : 0,
  __can_retry: workflow.can_retry ? 1 : 0,
  任务ID: workflow.name,
  阶段: workflow.phase === 'Succeeded' ? '流水线完成' : workflow.phase === 'Failed' || workflow.phase === 'Error' ? '流水线失败' : '训练流水线',
  数据集版本: `${workflow.parameters?.['lookback-days'] ?? '7'}d lookback`,
  模型配置: workflow.parameters?.['model-type'] ?? 'xgboost',
  算法配置: workflow.parameters?.['model-type'] ?? 'xgboost',
  特征版本: workflow.parameters?.['feature-set-id'] ?? '-',
  资源占用: workflow.progress || '-',
  状态: workflowPhaseLabel(workflow.phase),
  trace_id: workflow.name,
  操作: '查看 / 停止 / 重试',
});

const buildMlopsMetrics = (labels: string[], tasks: SnapshotRow[], releases: MlopsRecord[], gates: MlopsRecord[]): PageSnapshot['metrics'] => {
  const values: Record<string, number | string> = {
    训练任务: tasks.filter((row) => ['运行中', '排队中'].includes(String(row.状态))).length,
    评估任务: tasks.filter((row) => String(row.阶段).includes('评估')).length,
    注册任务: releases.filter((row) => textValue(row, 'status') === '待评审').length,
    发布任务: releases.filter((row) => textValue(row, 'status') === '已上线').length,
    失败任务: tasks.filter((row) => String(row.状态).includes('失败')).length,
    门禁通过率: gates.length ? `${((gates.filter((row) => textValue(row, 'status') === '通过').length / gates.length) * 100).toFixed(1)}%` : '0%',
  };
  return labels.map((label) => ({
    label,
    value: String(values[label] ?? 0),
    delta: label.includes('任务') && !label.includes('注册') && !label.includes('发布') ? 'Argo' : 'PostgreSQL',
    status: label === '失败任务' ? (Number(values[label]) > 0 ? 'warn' : 'ok') : label.includes('率') ? 'ok' : 'info',
  }));
};

const workflowPhaseLabel = (phase: string) => (({ Pending: '排队中', Running: '运行中', Succeeded: '已完成', Failed: '失败', Error: '失败' } as Record<string, string>)[phase] ?? phase) || '未知';

const textValue = (record: MlopsRecord, key: string) => String(record[key] ?? '');
const numberValue = (record: MlopsRecord, key: string) => Number(record[key] ?? 0);
const recordValue = (record: MlopsRecord, key: string): MlopsRecord => record[key] && typeof record[key] === 'object' && !Array.isArray(record[key]) ? record[key] as MlopsRecord : {};
const numberArray = (record: MlopsRecord, key: string): number[] => Array.isArray(record[key]) ? (record[key] as unknown[]).map(Number).filter(Number.isFinite) : [];
const formatNumber = (value: number) => Math.round(value).toLocaleString('zh-CN');
const formatGateValue = (label: string, value: number) => label.includes('通过率') ? `${(value * 100).toFixed(1)}%` : value.toFixed(label.includes('漂移') ? 2 : 3);
