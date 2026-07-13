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
import { useMutation, useQuery } from '@tanstack/react-query';
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
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { submitMlopsAction, type MlopsActionId } from '@/services/mlopsActionApi';
import { pageApiPlans, type ActionEndpointPlan } from '@/services/pageApiPlans';

const dagNodes: Array<[string, string, string, string, string, ReactNode, string]> = [
  ['反馈样本池', '1,238', '待处理样本', 'feedback_q', '2m', <DatabaseOutlined key="feedback" />, 'ok'],
  ['标注管理', '568', '待标注样本', 'label_q', '18m', <TagsOutlined key="label" />, 'ok'],
  ['特征构建', 'v1.8.7', '特征版本', 'feature_q', '6m', <ApiOutlined key="feature" />, 'ok'],
  ['训练任务', '6', '运行中任务', 'train_q', '1h 32m', <BranchesOutlined key="train" />, 'info'],
  ['评估门禁', '3', '评估中任务', 'eval_q', '23m', <SafetyCertificateOutlined key="eval" />, 'info'],
  ['模型注册', '2', '待注册模型', 'register_q', '6m', <CheckCircleOutlined key="register" />, 'ok'],
  ['灰度发布', '1', '灰度中', 'release_q', '45m', <RocketOutlined key="release" />, 'warn'],
  ['效果回流', '542', '未反馈样本', 'feedback_q', '持续回流', <ReloadOutlined key="loop" />, 'ok'],
];

const samplePoolRows = [
  ['FB-000982', 'FP', '端口扫风暴', 'AL-20250527-00123', '建议忽略', '★★★★☆', '05-27 14:28', '查看'],
  ['FB-000958', 'FP', 'CDN 误报', 'AL-20250527-00124', '高误报', '★★★☆☆', '05-27 14:22', '查看'],
  ['FB-000946', 'TP', 'C2 连接', 'AL-20250527-00126', '无需', '★★★★★', '05-27 14:26', '查看'],
  ['FB-000985', 'FP', '健康检查', 'AL-20250527-00126', '建议忽略', '★★★★☆', '05-27 14:26', '查看'],
  ['FB-000936', 'TP', 'NAT 变更', 'AL-20250527-00127', '高置信', '★★★☆☆', '05-27 14:25', '查看'],
  ['FB-000707', 'FP', '异常域名', 'AL-20250527-00128', '无需', '★★★★★', '05-27 14:24', '查看'],
  ['FB-000988', 'FP', 'P2P 通讯', 'AL-20250527-00129', '误报忽略', '★★★☆☆', '05-27 14:24', '查看'],
  ['FB-000989', 'FP', '系统更新', 'AL-20250527-00130', '无需', '★★★☆☆', '05-27 14:23', '查看'],
  ['FB-000991', 'FP', '跨网段扫描', 'AL-20250527-00131', '待复核', '★★★★☆', '05-27 14:22', '查看'],
  ['FB-000992', 'TP', '广告流量', 'AL-20250527-00132', '建议忽略', '★★★☆☆', '05-27 14:21', '查看'],
];

const gateRows = [
  ['准确率', '0.958', '>= 0.920', '通过', [91, 93, 92, 95, 96, 95, 95.8]],
  ['召回率', '0.932', '>= 0.900', '通过', [89, 90, 91, 92, 93, 92, 93.2]],
  ['F1', '0.944', '>= 0.920', '通过', [90, 92, 93, 94, 93, 94, 94.4]],
  ['误报率', '0.021', '<= 0.050', '通过', [4.8, 4.1, 3.6, 3.1, 2.8, 2.4, 2.1]],
  ['漂移(PSI)', '0.18', '<= 0.25', '通过', [15, 17, 16, 19, 18, 17, 18]],
  ['回归集通过率', '92.3%', '>= 90%', '通过', [88, 89, 91, 90, 92, 91, 92.3]],
];

const packageRows = [
  ['abnorm_pkt_v3', '1.2.3', '已签名', '查看', '是', '10% 30m', '灰度中'],
  ['abnorm_pkt_v3', '1.2.2', '已签名', '查看', '否', '-', '待发布'],
  ['abnorm_pkt_v3', '1.2.1', '已签名', '查看', '否', '-', '已回滚'],
  ['abnorm_pkt_v3', '1.2.0', '已签名', '查看', '否', '-', '已下线'],
];

const feedbackMetrics = [
  ['告警命中 (TP)', '1,256', '+12.3%', 'ok'],
  ['误报 (FP)', '218', '-8.1%', 'ok'],
  ['反馈样本量', '542', '+15.6%', 'info'],
  ['误报率', '2.15%', '-0.45pp', 'ok'],
  ['漂移(PSI)', '0.18', '+0.07', 'warn'],
];

const mlopsActionPlans = pageApiPlans.mlops.actions ?? [];
const mlopsPageSize = 6;

type MlopsActionState = {
  actionId: MlopsActionId;
  label: string;
  plan: ActionEndpointPlan;
  endpoint: string;
  targetId: string;
  target: string;
  version?: string;
  details?: SnapshotRow;
};

type OpenMlopsAction = (label: string, actionId?: MlopsActionId, targetRow?: SnapshotRow) => void;

export function MlopsOrchestrationPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [selectedKey, setSelectedKey] = useState<string>();
  const [page, setPage] = useState(1);
  const [taskFilter, setTaskFilter] = useState('全部任务');
  const [actionState, setActionState] = useState<MlopsActionState>();
  const actionMutation = useMutation({ mutationFn: submitMlopsAction });
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildMlopsTaskRows(data?.rows ?? [], 32), [data?.rows]);
  const filteredRows = useMemo(() => taskFilter === '全部任务' ? rows : rows.filter((row) => String(row.状态 ?? '').includes(taskFilter)), [rows, taskFilter]);
  const totalPages = Math.max(1, Math.ceil(filteredRows.length / mlopsPageSize));
  const visibleRows = filteredRows.slice((page - 1) * mlopsPageSize, page * mlopsPageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? visibleRows[0], [rows, selectedKey, visibleRows]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const openAction: OpenMlopsAction = (label, actionId = 'mlops-context-action', targetRow = selected) => {
    const plan = mlopsActionPlans.find((item) => item.id === actionId) ?? mlopsActionPlans[0];
    if (!plan) return;
    const isModelAction = actionId === 'mlops-model-register' || actionId === 'mlops-model-version-inspect';
    const targetId = String(isModelAction ? targetRow?.__model_id ?? targetRow?.模型包 ?? targetRow?.模型配置 ?? targetRow?.算法配置 ?? 'selected-model' : targetRow?.任务ID ?? targetRow?.['任务 ID'] ?? targetRow?.__task_id ?? 'current-pipeline');
    const version = String(targetRow?.__version ?? targetRow?.版本 ?? 'current');
    actionMutation.reset();
    setActionState({
      actionId,
      label,
      plan,
      endpoint: plan.endpoint.replace('{id}', encodeURIComponent(targetId)).replace('{version}', encodeURIComponent(version)),
      targetId,
      target: String(targetRow?.阶段 ?? targetRow?.任务ID ?? label),
      version,
      details: targetRow,
    });
  };
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderMlopsCell(column, value, record, openAction),
  }));

  return (
    <div className="taf-page taf-mlops">
      <section className="taf-mlops-shell">
        <main className="taf-mlops-main">
          <header className="taf-mlops-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Button size="small" type="primary" icon={<BranchesOutlined />} onClick={() => openAction('新建流水线', 'mlops-pipeline-create')}>新建流水线</Button>
              <Button size="small" icon={<PlayCircleOutlined />} onClick={() => openAction('投递训练任务', 'mlops-training-submit')}>投递训练任务</Button>
              <Button size="small" icon={<ReloadOutlined />} onClick={() => openAction('失败重试', 'mlops-task-retry')}>失败重试</Button>
              <Button size="small" danger ghost icon={<PauseCircleOutlined />} onClick={() => openAction('停止任务', 'mlops-task-stop')}>停止任务</Button>
              <Button size="small" icon={<CloudUploadOutlined />} onClick={() => openAction('注册模型', 'mlops-model-register')}>注册模型</Button>
              <Button size="small" icon={<RocketOutlined />} onClick={() => navigate('/deployments')}>进入部署管理</Button>
              <Tooltip title="刷新编排状态">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <Button size="small" icon={<FileSearchOutlined />} onClick={() => openAction('任务详情', 'mlops-task-inspect', selected)}>任务详情</Button>
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
                <MlopsDag />
              </WorkPanel>

              <div className="taf-mlops-bottom">
                <WorkPanel title="反馈样本池（未处理样本 Top 10）" extra={<Button size="small" onClick={() => openAction('发起标注', 'mlops-label-request')}>发起标注</Button>}>
                  <FeedbackPool onAction={openAction} />
                </WorkPanel>
                <WorkPanel title="训练任务队列" extra={<Select size="small" value={taskFilter} onChange={(value) => { setTaskFilter(value); setPage(1); }} options={['全部任务', '运行中', '排队中', '失败'].map((value) => ({ value }))} />}>
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
                  <div className="taf-mlops-pagination"><span>共 {filteredRows.length} 条</span>{Array.from({ length: totalPages }, (_, index) => index + 1).map((item) => <button key={item} type="button" className={item === page ? 'is-active' : ''} aria-current={item === page ? 'page' : undefined} onClick={() => setPage(item)}>{item}</button>)}</div>
                </WorkPanel>
              </div>
            </section>

            <aside className="taf-mlops-right">
              <WorkPanel title="评估与门禁（当前途中：评估门禁）">
                <EvaluationGate />
              </WorkPanel>
              <WorkPanel title="注册与发布" extra={<Button size="small" onClick={() => openAction('注册模型', 'mlops-model-register')}>注册模型</Button>}>
                <RegisterRelease onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="效果回流（近7日）">
                <FeedbackLoop />
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
        extra={<Button size="small" type="primary" loading={actionMutation.isPending} disabled={!actionState} onClick={() => actionState && actionMutation.mutate({ actionId: actionState.actionId, targetId: actionState.targetId, target: actionState.target, version: actionState.version })}>确认提交</Button>}
      >
        {actionState && <div className="taf-mlops-action-body">
          <Descriptions size="small" column={1} bordered>
            <Descriptions.Item label="接口">{actionState.plan.method} {actionState.endpoint}</Descriptions.Item>
            <Descriptions.Item label="权限">{actionState.plan.requiredScopes.join(', ')}</Descriptions.Item>
            <Descriptions.Item label="审计事件">{actionState.plan.auditEvent}</Descriptions.Item>
            <Descriptions.Item label="仿真请求体"><code>{JSON.stringify(actionState.plan.defaultBody ?? {})}</code></Descriptions.Item>
            {actionState.actionId === 'mlops-task-inspect' && <>
              <Descriptions.Item label="阶段">{String(actionState.details?.阶段 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="数据集">{String(actionState.details?.数据集版本 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="算法配置">{String(actionState.details?.算法配置 ?? actionState.details?.模型配置 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="特征版本">{String(actionState.details?.特征版本 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="资源占用">{String(actionState.details?.资源占用 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="任务状态">{String(actionState.details?.状态 ?? '-')}</Descriptions.Item>
              <Descriptions.Item label="数据模式">{actionState.details?.__data_mode === 'api-derived-simulation' ? 'Status API 派生仿真' : actionState.details?.__data_mode === 'api-condition-derived' ? 'Conditions API 派生' : 'Typed simulation'}</Descriptions.Item>
            </>}
          </Descriptions>
          <Alert type="info" showIcon message="API 预留与仿真执行" description={actionState.plan.guardrails.join('；')} />
          {actionMutation.data && <Alert type="success" showIcon message={`任务 ${actionMutation.data.jobId} 已进入仿真任务队列`} description={`${actionMutation.data.auditEvent}；${actionMutation.data.endpoint}`} />}
          {actionMutation.isError && <Alert type="error" showIcon message="MLOps 动作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : '未知错误'} />}
        </div>}
      </Drawer>
    </div>
  );
}

function MlopsDag() {
  return (
    <div className="taf-mlops-dag">
      {dagNodes.map(([title, value, caption, queue, time, icon, tone], index) => (
        <div key={title} className={`taf-mlops-dag-node is-${tone} ${index === 4 ? 'is-active' : ''}`}>
          <span>{title}</span>
          {icon}
          <b>{value}</b>
          <em>{caption}</em>
          <small>队列: {queue}</small>
          <strong>{time}</strong>
        </div>
      ))}
    </div>
  );
}

function FeedbackPool({ onAction }: { onAction: OpenMlopsAction }) {
  return (
    <div className="taf-mlops-feedback-pool">
      <div><span>ID</span><span>TP/FP</span><span>误报原因</span><span>来源告警</span><span>白名单建议</span><span>样本质量</span><span>入池时间</span><span>操作</span></div>
      {samplePoolRows.map((row) => (
        <button key={row[0]} type="button" onClick={() => onAction(`查看反馈样本 ${row[0]}`, 'mlops-feedback-inspect', { 任务ID: row[0], 阶段: '反馈样本' })}>
          {row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 1 ? `is-${cell.toLowerCase()}` : ''}>{cell}</span>)}
        </button>
      ))}
      <footer>共 1,238 条 <b>TP 742</b><b>FP 496</b><b>待复核 86</b></footer>
    </div>
  );
}

function EvaluationGate() {
  return (
    <div className="taf-mlops-gate">
      <div className="taf-mlops-gate-table">
        <div><span>指标</span><span>最新值</span><span>阈值</span><span>结果</span><span>趋势(7日)</span></div>
        {gateRows.map(([label, value, threshold, result, values]) => <span key={String(label)}><b>{label}</b><em>{value}</em><em>{threshold}</em><StatusTag value={result} /><DataQualityKpiSparklineChart ariaLabel={`${label}近 7 日趋势`} className="taf-mlops-gate-echart" tone="ok" values={values as number[]} /></span>)}
      </div>
      <div className="taf-mlops-confusion">
        <b>混淆矩阵（回归集）</b>
        <div><span>预测\\真实</span><span>正常</span><span>异常</span><span>合计</span></div>
        <div><span>正常</span><b>4,582</b><b>218</b><b>4,800</b></div>
        <div><span>异常</span><b>143</b><b>1,257</b><b>1,400</b></div>
        <div><span>合计</span><b>4,725</b><b>1,475</b><b>6,200</b></div>
        <strong>门禁结果 <StatusTag value="通过" /></strong>
      </div>
    </div>
  );
}

function RegisterRelease({ onAction }: { onAction: OpenMlopsAction }) {
  return (
    <div className="taf-mlops-register">
      <div><span>模型包</span><span>版本</span><span>签名</span><span>模型卡</span><span>线上候选</span><span>灰度策略</span><span>状态</span><span>操作</span></div>
      {packageRows.map((row) => <button key={`${row[0]}-${row[1]}`} type="button" onClick={() => onAction(`查看模型包 ${row[0]} ${row[1]}`, 'mlops-model-version-inspect', { __model_id: row[0], __version: row[1], 模型包: row[0], 版本: row[1], 阶段: '注册与发布' })}>{row.map((cell, index) => index === 6 ? <StatusTag key={cell} value={cell} /> : <span key={`${cell}-${index}`}>{cell}</span>)}<FileSearchOutlined /></button>)}
    </div>
  );
}

function FeedbackLoop() {
  return (
    <div className="taf-mlops-loop">
      <div className="taf-mlops-loop-metrics">
        {feedbackMetrics.map(([label, value, delta, tone]) => <span key={label}><em>{label}</em><b className={`is-${tone}`}>{value}</b><strong>{delta}</strong></span>)}
      </div>
      <div className="taf-mlops-funnel">
        <span>告警总量 <b>24,681</b></span>
        <span>有效告警 <b>6,214</b></span>
        <span>入池样本 <b>1,238</b></span>
        <span>已标注 <b>568</b></span>
      </div>
      <div className="taf-mlops-trend">
        <b>误报率 & 漂移趋势</b>
        <DataQualityTrendChart
          ariaLabel="误报率与漂移近 7 日趋势"
          className="taf-mlops-feedback-echart"
          categories={['05-21', '05-22', '05-23', '05-24', '05-25', '05-26', '05-27']}
          series={[
            { name: '误报率', color: '#18a8ff', values: [4.6, 5.2, 3.8, 4.4, 3.2, 4.1, 3.5] },
            { name: 'PSI', color: '#36d66b', values: [1.6, 1.8, 2.4, 1.5, 2.0, 1.4, 1.8] },
          ]}
          valueFormatter={(value) => value.toFixed(1)}
        />
      </div>
    </div>
  );
}

const renderMlopsCell = (column: string, value: unknown, record: SnapshotRow, onAction: OpenMlopsAction) => {
  if (column === '任务ID' || column === '任务 ID') return <span className="taf-mlops-task-id"><BranchesOutlined />{String(value)}<small>{record.__data_mode === 'api-derived-simulation' ? 'API 仿真' : record.__data_mode === 'simulated' ? '仿真' : 'API'}</small></span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '资源占用') return <span className="taf-mlops-resource">{String(value)}</span>;
  if (column === '操作') return <span className="taf-mlops-row-actions">
    <button type="button" title="查看任务" aria-label="查看任务" onClick={(event) => { event.stopPropagation(); onAction('查看任务', 'mlops-task-inspect', record); }}><FileSearchOutlined /></button>
    <button type="button" title="停止任务" aria-label="停止任务" onClick={(event) => { event.stopPropagation(); onAction('停止任务', 'mlops-task-stop', record); }}><CloseCircleOutlined /></button>
    <button type="button" title="重试任务" aria-label="重试任务" onClick={(event) => { event.stopPropagation(); onAction('重试任务', 'mlops-task-retry', record); }}><ReloadOutlined /></button>
  </span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.任务ID ?? row['任务 ID'] ?? JSON.stringify(row));

const buildMlopsTaskRows = (apiRows: SnapshotRow[], total: number) => {
  const rows = [...apiRows];
  const stages = ['训练任务', '评估门禁', '模型注册', '灰度发布', '效果回流'];
  while (rows.length < total) {
    const index = rows.length + 1;
    rows.push({
      __task_id: `SIM-MLOPS-${String(index).padStart(3, '0')}`,
      __data_mode: 'simulated',
      任务ID: `SIM-MLOPS-${String(index).padStart(3, '0')}`,
      阶段: stages[index % stages.length],
      数据集版本: `ds_v1.${index % 9}.${index % 4}`,
      模型配置: `xgb_v${1 + (index % 3)}.${index % 6}`,
      特征版本: `feat_v1.8.${index % 9}`,
      资源占用: `CPU ${20 + (index * 7) % 70}% / MEM ${30 + (index * 5) % 60}%`,
      状态: index % 7 === 0 ? '失败' : index % 4 === 0 ? '排队中' : '运行中',
      操作: '查看 / 停止 / 重试',
    });
  }
  return rows;
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '86.7%' : '0',
  delta: 'API',
  status: 'info',
});
