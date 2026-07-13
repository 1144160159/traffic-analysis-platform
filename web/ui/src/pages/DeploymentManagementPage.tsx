import {
  CloudUploadOutlined,
  CodeSandboxOutlined,
  DownloadOutlined,
  EyeOutlined,
  FieldTimeOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Slider, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const healthItems = [
  ['Flink Checkpoint 成功率', '98.8%', 'ok'],
  ['Kafka 消费延迟 (P95)', '320 ms', 'warn'],
  ['告警数量变化', '-18.5%', 'ok'],
  ['误报率变化', '+2.1%', 'risk'],
  ['端到端延迟 (P95)', '1.28 s', 'ok'],
  ['采集丢包率', '0.03%', 'ok'],
];

const evidenceRows = [
  ['manifest', '已通过', 'a1b2c3d4...e5f6'],
  ['镜像', '已通过', 'sha256:7e8d...9e0f'],
  ['DDL', '已通过', 'ddl_20250527_001'],
  ['topic', '已通过', 'topic_20250527_001'],
  ['规则版本', '已通过', 'rules_v2.3.1'],
  ['模型版本', '已通过', 'model_v1.8.0'],
];

const rollbackRows = [
  ['v2.2.7', '2025-05-26 16:10', '租户A / 全量', '安全运营组'],
  ['v2.2.3', '2025-05-23 11:05', '租户A / 全量', '安全运营组'],
  ['v2.1.9', '2025-05-20 09:40', '租户A / 全量', '安全运营组'],
];

const changeRows = [
  ['规则变更数', '32 条', '57 条', '+25'],
  ['模型版本', 'v1.7.3', 'v1.8.0', '升级'],
  ['DDL 变更', '2 处', '3 处', '+1'],
  ['Topic 变更', '1 个', '2 个', '+1'],
  ['风险等级', '低风险', '中风险', '升高'],
];

const deploymentOverlays: OverlayContract[] = [
  {
    id: 'modal-deployment-create',
    title: '新建部署',
    kind: 'Modal',
    actionLabel: '新建部署',
    description: '创建规则、模型、采集策略、Flink 作业或配置发布计划。',
    impact: '影响灰度范围、作业版本、规则版本和运行时配置。',
    audit: '记录 manifest、镜像、DDL、Topic 和审批 trace。',
  },
  {
    id: 'modal-deployment-rollback',
    title: '回滚部署确认',
    kind: 'Modal',
    actionLabel: '回滚确认',
    description: '确认回滚版本、影响范围、回滚窗口和恢复证据。',
    impact: '可能影响实时检测、采集策略和模型版本。',
    danger: true,
  },
];

type DeploymentAction = { title: string; target: string; endpoint: string; auditEvent: string };

export function DeploymentManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [listPage, setListPage] = useState(1);
  const [grayPercent, setGrayPercent] = useState(20);
  const [healthWindow, setHealthWindow] = useState('近 30 分钟');
  const [action, setAction] = useState<DeploymentAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => buildDeploymentRows(data?.rows ?? []), [data?.rows]);
  const pageSize = 7;
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const visibleRows = rows.slice((listPage - 1) * pageSize, listPage * pageSize);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderDeploymentCell(column, value, (title) => openAction(title, rowKey(record))),
  }));
  function openAction(title: string, target = rowKey(selected ?? {})) {
    setActionSubmitted(false);
    setAction(createDeploymentAction(title, target));
  }

  return (
    <div className="taf-page taf-deployments">
      <section className="taf-deployments-shell">
        <main className="taf-deployments-main">
          <header className="taf-deployments-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              <span>规则、模型、采集策略、Flink 作业和配置的发布与回滚</span>
            </div>
            <Space size={6}>
              <Button size="small" type="primary" icon={<CloudUploadOutlined />} onClick={() => openAction('新建发布')}>新建发布</Button>
              <Button size="small" icon={<PlayCircleOutlined />} onClick={() => openAction('继续灰度')}>继续灰度</Button>
              <Button size="small" danger ghost icon={<StopOutlined />} onClick={() => openAction('停止灰度')}>停止灰度</Button>
              <Button size="small" danger icon={<RollbackOutlined />} onClick={() => openAction('快速回滚')}>快速回滚</Button>
              <Button size="small" icon={<DownloadOutlined />} onClick={() => openAction('导出发布证据')}>导出证据</Button>
              <Tooltip title="刷新发布状态">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <OverlayContractHost overlays={deploymentOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/deployments、APISIX 路由或 rule-manager deployment handler。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-deployments-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-deployments-grid">
            <WorkPanel title="发布清单" className="taf-deployments-list-panel">
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                columns={columns}
                dataSource={visibleRows}
                scroll={{ x: 900, y: 206 }}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
              />
              <div className="taf-deployments-pagination"><span>共 {rows.length} 条</span><button type="button" aria-label="发布清单上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`发布清单第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="发布清单下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
            </WorkPanel>

            <aside className="taf-deployments-rail">
              <WorkPanel title="灰度策略" extra={<Select size="small" value="按租户+园区+探针分级" options={[{ value: '按租户+园区+探针分级' }, { value: '按资产重要性分级' }]} onChange={(value) => openAction('更新灰度分级策略', value)} />}>
                <GrayStrategy selected={selected} grayPercent={grayPercent} onGrayPercentChange={setGrayPercent} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="发布健康" extra={<Select size="small" value={healthWindow} options={[{ value: '近 30 分钟' }, { value: '近 2 小时' }]} onChange={setHealthWindow} />}>
                <ReleaseHealth grayPercent={grayPercent} healthWindow={healthWindow} />
              </WorkPanel>
            </aside>
          </div>

          <div className="taf-deployments-bottom">
            <WorkPanel title="版本对比 / 变更摘要">
              <VersionDiff selected={selected} />
            </WorkPanel>
            <WorkPanel title="回滚管理">
              <RollbackManager onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="发布证据">
              <ReleaseEvidence evidence={data?.evidence ?? []} onAction={openAction} />
            </WorkPanel>
          </div>
        </main>
      </section>
      <Drawer className="taf-deployments-action-drawer" title={action ? `${action.title}确认` : '发布操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为发布对象创建“{action.title}”仿真任务，并保留灰度范围、审批与审计上下文。</p><dl><dt>发布对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="发布业务操作已进入仿真任务队列" />}</div>}
      </Drawer>
    </div>
  );
}

function GrayStrategy({
  selected,
  grayPercent,
  onGrayPercentChange,
  onAction,
}: {
  selected?: SnapshotRow;
  grayPercent: number;
  onGrayPercentChange: (value: number) => void;
  onAction: (title: string, target?: string) => void;
}) {
  const [tenant, setTenant] = useState('租户A');
  const [campus, setCampus] = useState('华东园区');
  const [probeGroup, setProbeGroup] = useState('办公区探针组 (12)');
  const [assetGroup, setAssetGroup] = useState('核心业务资产组');

  return (
    <div className="taf-deployments-gray">
      <div className="taf-deployments-gray-form">
        <label><span>租户</span><Select size="small" value={tenant} options={[{ value: '租户A' }, { value: '租户B' }]} onChange={setTenant} /></label>
        <label><span>园区</span><Select size="small" value={campus} options={[{ value: '华东园区' }, { value: '华南园区' }]} onChange={setCampus} /></label>
        <label><span>探针组</span><Select size="small" value={probeGroup} options={[{ value: '办公区探针组 (12)' }, { value: '核心区探针组 (8)' }]} onChange={setProbeGroup} /></label>
        <label><span>资产组</span><Select size="small" value={assetGroup} options={[{ value: '核心业务资产组' }, { value: '办公终端资产组' }]} onChange={setAssetGroup} /></label>
      </div>
      <div className="taf-deployments-slider"><span>流量百分比</span><Slider min={0} max={100} value={grayPercent} marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} onChange={(value) => onGrayPercentChange(Number(value))} onChangeComplete={(value) => onAction('更新灰度流量', `${Number(value)}%`)} /></div>
      <div className="taf-deployments-gray-meta">
        <span>当前阶段<b>{String(selected?.状态 ?? '灰度中')}</b></span>
        <span>生效时间<b>{String(selected?.发布时间 ?? '2025-05-27 14:15')}</b></span>
        <span>预计观察时长<b>30 分钟</b></span>
        <span>自动推进<b>未开启</b></span>
        <Button size="small" type="primary" onClick={() => onAction('编辑灰度策略', `${tenant} / ${campus} / ${probeGroup} / ${assetGroup}`)}>编辑策略</Button>
      </div>
    </div>
  );
}

function ReleaseHealth({ grayPercent, healthWindow }: { grayPercent: number; healthWindow: string }) {
  return (
    <div className="taf-deployments-health">
      {healthItems.map(([label, value, tone], index) => (
        <div key={label}>
          <span>{label}</span>
          <strong className={`is-${tone}`}>{value}</strong>
          <i className={`is-${tone}`} />
          <div className="taf-deployments-health-echart"><DataQualityKpiSparklineChart ariaLabel={`${label}${healthWindow}趋势`} tone={tone === 'risk' ? 'risk' : tone === 'warn' ? 'warn' : 'ok'} values={buildHealthTrend(index, grayPercent)} /></div>
        </div>
      ))}
    </div>
  );
}

function VersionDiff({ selected }: { selected?: SnapshotRow }) {
  return (
    <div className="taf-deployments-diff">
      <div className="taf-deployments-version-pair">
        <span>当前版本<b>v2.2.7</b><em>已发布</em></span>
        <strong>→</strong>
        <span>目标版本<b>{String(selected?.版本 ?? 'v2.3.1')}</b><em>灰度中</em></span>
      </div>
      <div className="taf-deployments-change-list">
        {changeRows.map(([label, from, to, delta]) => <span key={label}><b>{label}</b><em>{from}</em><strong>→</strong><em>{to}</em><i>{delta}</i></span>)}
      </div>
      <p>影响评估：中，建议先在 20% 流量灰度验证误报率。</p>
    </div>
  );
}

function RollbackManager({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const [reason, setReason] = useState('');

  return (
    <div className="taf-deployments-rollback">
      <div><span>可回滚版本</span><span>发布时间</span><span>影响范围</span><span>发布人</span><span>操作</span></div>
      {rollbackRows.map(([version, time, scope, owner]) => <button key={version} type="button" onClick={() => onAction('回滚指定版本', version)}><b>{version}</b><span>{time}</span><span>{scope}</span><span>{owner}</span><em>回滚</em></button>)}
      <label><span>回滚原因（可选）</span><Input value={reason} placeholder="请输入回滚原因，支持选择预设标签" onChange={(event) => setReason(event.target.value)} /></label>
      <div className="taf-deployments-rollback-actions"><Button size="small" onClick={() => setReason('误报升高')}>误报升高</Button><Button size="small" onClick={() => setReason('性能下降')}>性能下降</Button><Button size="small" onClick={() => setReason('策略变更')}>策略变更</Button><Button size="small" danger onClick={() => onAction('执行回滚', reason || '当前灰度版本')}>执行回滚</Button></div>
    </div>
  );
}

function ReleaseEvidence({ evidence, onAction }: { evidence: PageSnapshot['evidence']; onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-deployments-evidence">
      <div><span>证据项</span><span>校验状态</span><span>哈希 / 编号</span><span>操作</span></div>
      {evidenceRows.map(([label, status, hash]) => <button key={label} type="button" onClick={() => onAction('下载发布证据', label)}><b>{label}</b><StatusTag value={status} /><span>{hash}</span><DownloadOutlined /></button>)}
      <div className="taf-deployments-evidence-strip">
        {evidence.slice(0, 6).map((item) => <span key={item.label}><SafetyCertificateOutlined className={`is-${item.status}`} />{item.label}<b>{item.value}</b></span>)}
      </div>
      <Button size="small" type="primary" onClick={() => onAction('导出合规证据包')}>导出合规证据包</Button>
    </div>
  );
}

const renderDeploymentCell = (column: string, value: unknown, onAction: (title: string) => void) => {
  if (column === '发布对象') return <span className="taf-deployments-object"><CodeSandboxOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') {
    return <span className="taf-deployments-row-actions"><Tooltip title="查看发布详情"><Button size="small" type="text" icon={<EyeOutlined />} onClick={(event) => { event.stopPropagation(); onAction('查看发布详情'); }} /></Tooltip><Tooltip title="启动灰度"><Button size="small" type="text" icon={<CloudUploadOutlined />} onClick={(event) => { event.stopPropagation(); onAction('启动灰度'); }} /></Tooltip><Tooltip title="暂停发布"><Button size="small" type="text" icon={<PauseCircleOutlined />} onClick={(event) => { event.stopPropagation(); onAction('暂停发布'); }} /></Tooltip><Tooltip title="查看发布时间线"><Button size="small" type="text" icon={<FieldTimeOutlined />} onClick={(event) => { event.stopPropagation(); onAction('查看发布时间线'); }} /></Tooltip><Tooltip title="回滚发布"><Button size="small" type="text" icon={<RollbackOutlined />} onClick={(event) => { event.stopPropagation(); onAction('回滚发布'); }} /></Tooltip></span>;
  }
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row['发布对象'] ?? row.批次 ?? JSON.stringify(row));

const buildDeploymentRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 发布对象: '规则集 / 核心检测规则', 版本: 'v2.3.1', 状态: '灰度中', 发布时间: '2026-07-11 12:00', 操作: '查看' }];
  if (source.length >= 28) return source;
  return Array.from({ length: 28 }, (_, index) => {
    const base = source[index % source.length];
    return { ...base, 发布对象: `${String(base.发布对象 ?? '发布对象')}-SIM${String(index + 1).padStart(2, '0')}`, 版本: `${String(base.版本 ?? 'v2.3.1')}.${index % 4}` };
  });
};

const buildHealthTrend = (index: number, grayPercent: number) => {
  const baseline = 76 + (index * 7 + grayPercent) % 18;
  return [baseline - 3, baseline + 1, baseline - 1, baseline + 3, baseline, baseline + 2, baseline + 1];
};

const createDeploymentAction = (title: string, target: string): DeploymentAction => {
  const actions = pageApiPlans.deployments.actions ?? [];
  const actionId = /回滚/.test(title) ? 'deployment-rollback'
    : /停止|暂停/.test(title) ? 'deployment-pause'
      : /继续/.test(title) ? 'deployment-resume'
        : /新建|启动|灰度|编辑|更新/.test(title) ? 'deployment-start-gray'
          : '';
  const plan = actions.find((item) => item.id === actionId);
  const evidenceExport = /证据|下载|导出/.test(title);
  return {
    title,
    target,
    endpoint: evidenceExport ? `/v1/deployments/${target}/evidence/export` : plan?.endpoint.replace('{id}', target) ?? `/v1/deployments/${target}`,
    auditEvent: evidenceExport ? 'DEPLOY_EVIDENCE_EXPORT_REQUESTED' : plan?.auditEvent ?? 'DEPLOY_ACTION_SIMULATED',
  };
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '98.2%' : label.includes('延迟') ? '58s' : '0',
  delta: 'API',
  status: 'info',
});
