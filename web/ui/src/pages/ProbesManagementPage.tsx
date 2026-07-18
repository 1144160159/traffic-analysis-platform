import {
  ApiOutlined,
  CloudUploadOutlined,
  ControlOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  EyeOutlined,
  FieldTimeOutlined,
  FileProtectOutlined,
  FullscreenOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  SyncOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  ZoomInOutlined,
  ZoomOutOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Modal, Progress, Select, Space, Tooltip } from 'antd';
import type { KeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode, WheelEvent as ReactWheelEvent } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot, fetchProbeTopology, submitProbeOperation } from '@/services/api';
import type { ProbeOperationActionId, ProbeOperationResult, ProbeTopologyGraph, ProbeTopologyNode, ProbeTopologyPoint } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const actionItems: Array<[string, ReactNode]> = [
  ['批量升级', <CloudUploadOutlined key="upgrade" />],
  ['批量启停', <PoweroffOutlined key="power" />],
  ['策略下发', <SafetyCertificateOutlined key="policy" />],
  ['连通性测试', <SyncOutlined key="connect" />],
  ['证书轮换', <SettingOutlined key="cert" />],
  ['重启探针', <ReloadOutlined key="restart" />],
];

const probeMetricIcons: Record<string, ReactNode> = {
  探针总数: <DeploymentUnitOutlined />,
  在线探针: <SafetyCertificateOutlined />,
  采集网卡: <ApiOutlined />,
  采集模式: <ControlOutlined />,
  '平均 CPU': <DashboardOutlined />,
  平均内存: <ThunderboltOutlined />,
  告警探针: <WarningOutlined />,
  离线探针: <PoweroffOutlined />,
};

type TopologyMode = '2d' | '3d';

type ProbeAction = {
  id?: ProbeOperationActionId;
  title: string;
  targets: string[];
  endpoint: string;
  auditEvent: string;
  readOnly: boolean;
  payload: Record<string, unknown>;
};

export function ProbesManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [selectedTopologyId, setSelectedTopologyId] = useState('');
  const [topologyMode, setTopologyMode] = useState<TopologyMode>('3d');
  const [matrixPage, setMatrixPage] = useState(1);
  const [trendRange, setTrendRange] = useState<'6h' | '24h'>('6h');
  const [matrixExpanded, setMatrixExpanded] = useState(false);
  const [action, setAction] = useState<ProbeAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const [actionPending, setActionPending] = useState(false);
  const [actionError, setActionError] = useState('');
  const [actionResult, setActionResult] = useState<ProbeOperationResult>();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });
  const {
    data: topologyGraph,
    error: topologyError,
    isError: isTopologyError,
    isLoading: isTopologyLoading,
    refetch: refetchTopology,
  } = useQuery({
    queryKey: ['probe-topology', topologyMode],
    queryFn: () => fetchProbeTopology(topologyMode),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const probeRows = useMemo(() => buildProbeRows(rows), [rows]);
  const pageSize = 10;
  const pageCount = Math.max(1, Math.ceil(probeRows.length / pageSize));
  const visibleRows = probeRows.slice((matrixPage - 1) * pageSize, matrixPage * pageSize);
  const selected = useMemo(() => probeRows.find((row) => rowKey(row) === selectedKey) ?? probeRows[0], [probeRows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const probeIds = useMemo(() => probeRows.map((row) => rowKey(row)).filter(Boolean), [probeRows]);
  const openAction = (title: string, target: string | string[]) => {
    setActionSubmitted(false);
    setActionPending(false);
    setActionError('');
    setActionResult(undefined);
    setAction(createProbeAction(title, Array.isArray(target) ? target : [target]));
  };
  const actionUsesModal = Boolean(action && /配置|升级|证书/.test(action.title));
  const closeAction = () => {
    setAction(undefined);
    setActionSubmitted(false);
    setActionPending(false);
    setActionError('');
    setActionResult(undefined);
  };
  const submitAction = async () => {
    if (!action?.id || action.readOnly || actionPending) return;
    setActionPending(true);
    setActionError('');
    try {
      const result = await submitProbeOperation(action.id, action.targets, action.payload);
      setActionResult(result);
      setActionSubmitted(true);
      await refetch();
    } catch (submitError) {
      setActionError(submitError instanceof Error ? submitError.message : '探针运维请求失败');
    } finally {
      setActionPending(false);
    }
  };
  const selectTopology = (id: string) => {
    setSelectedTopologyId(id);
    const target = probeRows.find((row) => rowKey(row) === id);
    if (target) setSelectedKey(rowKey(target));
  };
  useEffect(() => {
    const nodes = topologyGraph?.nodes ?? [];
    if (!nodes.length || nodes.some((node) => node.id === selectedTopologyId)) return;
    const nextId = nodes[0].id;
    setSelectedTopologyId(nextId);
    const target = probeRows.find((row) => rowKey(row) === nextId);
    if (target) setSelectedKey(rowKey(target));
  }, [probeRows, selectedTopologyId, topologyGraph]);

  return (
    <div className="taf-page taf-probes">
      <section className="taf-probes-shell">
        <main className="taf-probes-main">
          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/probes、APISIX 路由或 alert-service system handler。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <section className="taf-probes-overview" aria-label={`${route.page.title}总览`}>
            <div className="taf-probes-overview-head">
              <h1>探针总览</h1>
            </div>
            <div className="taf-probes-kpis">
              {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} icon={probeMetricIcons[metric.label]} />)}
            </div>
          </section>

          <WorkPanel
            title="部署拓扑"
            extra={(
              <Space size={6}>
                <Button size="small" type={topologyMode === '2d' ? 'primary' : 'default'} aria-pressed={topologyMode === '2d'} onClick={() => setTopologyMode('2d')}>2D</Button>
                <Button size="small" type={topologyMode === '3d' ? 'primary' : 'default'} aria-pressed={topologyMode === '3d'} onClick={() => setTopologyMode('3d')}>3D</Button>
                <Tooltip title="刷新部署拓扑"><Button size="small" icon={<SyncOutlined />} aria-label="刷新部署拓扑" onClick={() => void Promise.all([refetch(), refetchTopology()])} /></Tooltip>
              </Space>
            )}
          >
            <DeploymentTopology graph={topologyGraph} loading={isTopologyLoading} error={isTopologyError ? topologyError : undefined} mode={topologyMode} selectedNodeId={selectedTopologyId} onSelect={selectTopology} />
          </WorkPanel>

          <div className="taf-probes-bottom">
            <WorkPanel
              title="探针状态矩阵"
              className="taf-probes-table-panel"
              extra={(
                <Space size={6} className="taf-probes-matrix-tools">
                  <Tooltip title="全屏查看矩阵">
                    <Button size="small" icon={<FullscreenOutlined />} aria-label="全屏查看矩阵" onClick={() => setMatrixExpanded(true)} />
                  </Tooltip>
                  <Tooltip title="刷新探针状态">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                  <span>自动刷新 15s</span>
                </Space>
              )}
            >
              <ProbeStatusMatrix
                columns={route.page.tableColumns}
                rows={visibleRows}
                isLoading={isLoading}
                onSelect={(record) => setSelectedKey(rowKey(record))}
                onAction={(title, record) => openAction(title, rowKey(record))}
              />
              <div className="taf-probes-pagination">
                <span>共 {probeRows.length} 条</span>
                <button type="button" aria-label="探针状态上一页" disabled={matrixPage === 1} onClick={() => setMatrixPage((page) => Math.max(1, page - 1))}>‹</button>
                {Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => (
                  <button key={page} type="button" className={page === matrixPage ? 'is-active' : ''} aria-label={`探针状态第 ${page} 页`} onClick={() => setMatrixPage(page)}>{page}</button>
                ))}
                <button type="button" aria-label="探针状态下一页" disabled={matrixPage === pageCount} onClick={() => setMatrixPage((page) => Math.min(pageCount, page + 1))}>›</button>
                <span>{pageSize} 条/页</span>
              </div>
            </WorkPanel>

            <WorkPanel
              title="吞吐与丢包趋势"
              className="taf-probes-trends-panel"
              extra={<Select aria-label="趋势时间范围" size="small" value={trendRange} options={[{ value: '6h', label: '近 6 小时' }, { value: '24h', label: '近 24 小时' }]} onChange={setTrendRange} />}
            >
              <ThroughputTrends rows={probeRows} range={trendRange} />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-probes-rail">
          <ProbeDetail selected={selected} />
          <WorkPanel title="批量运维" extra={<Button size="small" type="link" onClick={() => openAction('编辑批量运维', textValue(selected, '探针 ID') || probeIds[0] || '当前探针组')}>编辑</Button>}>
            <BatchOperations selected={selected} onAction={(title) => openAction(title, title.includes('批量') ? probeIds : textValue(selected, '探针 ID') || probeIds[0] || '当前探针组')} />
          </WorkPanel>
          <WorkPanel title="心跳与日志" extra={<Button size="small" type="link" onClick={() => openAction('查看全部心跳与日志', textValue(selected, '探针 ID') || '当前探针组')}>更多</Button>}>
            <HeartbeatLog rows={probeRows} onSelect={(probe) => openAction('查看心跳详情', probe)} />
          </WorkPanel>
        </aside>
      </section>
      <Drawer
        className="taf-probe-action-drawer"
        title={action ? `${action.title}确认` : '探针操作确认'}
        open={Boolean(action && !actionUsesModal)}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onClose={closeAction}
        extra={action && !action.readOnly ? <Button size="small" type="primary" loading={actionPending} disabled={actionSubmitted} onClick={() => void submitAction()}>{actionSubmitted ? '后端已受理' : '确认提交'}</Button> : undefined}
      >
        {action && <ProbeActionBody action={action} onPayloadChange={(payload) => setAction({ ...action, payload: { ...action.payload, ...payload } })} submitted={actionSubmitted} pending={actionPending} error={actionError} result={actionResult} />}
      </Drawer>
      <Modal
        className="taf-probe-action-modal"
        title={action ? `${action.title}确认` : '探针操作确认'}
        open={actionUsesModal}
        width="min(620px, calc(var(--taf-window-inner-width, 100dvw) - 64px))"
        onCancel={closeAction}
        footer={[
          <Button key="cancel" onClick={closeAction}>取消</Button>,
          <Button key="submit" type="primary" loading={actionPending} disabled={actionSubmitted} onClick={() => void submitAction()}>{actionSubmitted ? '后端已受理' : '确认提交'}</Button>,
        ]}
      >
        {action && <ProbeActionBody action={action} onPayloadChange={(payload) => setAction({ ...action, payload: { ...action.payload, ...payload } })} submitted={actionSubmitted} pending={actionPending} error={actionError} result={actionResult} />}
      </Modal>
      <Drawer title="探针状态矩阵" placement="bottom" height="72%" open={matrixExpanded} onClose={() => setMatrixExpanded(false)}>
        <ProbeStatusMatrix columns={route.page.tableColumns} rows={visibleRows} isLoading={isLoading} onSelect={(record) => setSelectedKey(rowKey(record))} onAction={(title, record) => openAction(title, rowKey(record))} />
      </Drawer>
    </div>
  );
}

function ProbeActionBody({
  action,
  onPayloadChange,
  submitted,
  pending,
  error,
  result,
}: {
  action: ProbeAction;
  onPayloadChange: (payload: Record<string, unknown>) => void;
  submitted: boolean;
  pending: boolean;
  error: string;
  result?: ProbeOperationResult;
}) {
  return (
    <div className="taf-alert-detail-action-body">
      <p>{action.readOnly ? '当前抽屉展示真实探针 API 返回的对象上下文。' : `将通过控制面 API 创建“${action.title}”任务；执行前校验租户、权限、版本兼容性和影响范围，并保留操作者与审计上下文。`}</p>
      <dl>
        <dt>操作对象</dt><dd>{action.targets.join('、')}</dd>
        <dt>真实接口</dt><dd>{action.endpoint}</dd>
        <dt>审计事件</dt><dd>{action.auditEvent}</dd>
      </dl>
      {!action.readOnly && <ProbeActionFields action={action} onChange={onPayloadChange} />}
      {pending && <Alert type="info" showIcon message="正在提交探针运维请求" />}
      {error && <Alert type="error" showIcon message="探针运维请求失败" description={error} />}
      {submitted && <Alert type="success" showIcon message="探针业务操作已由后端受理" description={`状态：${result?.status || 'completed'}；操作 ID：${result?.operation_id || result?.operation_ids?.join(', ') || result?.batch_id || '-'}`} />}
    </div>
  );
}

function ProbeActionFields({ action, onChange }: { action: ProbeAction; onChange: (payload: Record<string, unknown>) => void }) {
  if (action.id === 'probe-batch-state') return (
    <label className="taf-probe-action-field">目标状态<Select value={String(action.payload.desired_state || 'active')} options={[{ value: 'active', label: '启用' }, { value: 'inactive', label: '停用' }]} onChange={(desired_state) => onChange({ desired_state })} /></label>
  );
  if (action.id === 'probe-batch-upgrade') return (
    <label className="taf-probe-action-field">目标版本<Input value={String(action.payload.target_version || '')} onChange={(event) => onChange({ target_version: event.target.value })} /></label>
  );
  if (action.id === 'probe-config-push') return (
    <div className="taf-probe-action-fields">
      <label className="taf-probe-action-field">配置版本<Input value={String(action.payload.config_version || '')} onChange={(event) => onChange({ config_version: event.target.value })} /></label>
      <label className="taf-probe-action-field">采集模式<Select value={String(action.payload.capture_mode || 'af_packet')} options={[{ value: 'af_packet', label: 'AF_PACKET' }, { value: 'af_xdp', label: 'AF_XDP' }, { value: 'hybrid_l2_l3', label: '混合 L2+L3' }]} onChange={(capture_mode) => onChange({ capture_mode })} /></label>
      <label className="taf-probe-action-field">归档路径<Input value={String(action.payload.archive_path || '')} onChange={(event) => onChange({ archive_path: event.target.value })} /></label>
    </div>
  );
  if (action.id === 'probe-connectivity-test') return (
    <label className="taf-probe-action-field">测试目标<Input value={Array.isArray(action.payload.targets) ? action.payload.targets.join(',') : ''} onChange={(event) => onChange({ targets: event.target.value.split(',').map((item) => item.trim()).filter(Boolean) })} /></label>
  );
  if (action.id === 'probe-cert-rotate') return (
    <div className="taf-probe-action-fields">
      <label className="taf-probe-action-field">证书引用<Input value={String(action.payload.secret_ref || '')} onChange={(event) => onChange({ secret_ref: event.target.value })} /></label>
      <label className="taf-probe-action-field">轮换窗口<Select value={String(action.payload.rotation_window || 'immediate')} options={[{ value: 'immediate', label: '立即' }, { value: 'maintenance', label: '维护窗口' }]} onChange={(rotation_window) => onChange({ rotation_window })} /></label>
    </div>
  );
  return <label className="taf-probe-action-field">原因<Input value={String(action.payload.reason || '')} onChange={(event) => onChange({ reason: event.target.value })} /></label>;
}

function ProbeStatusMatrix({
  columns,
  rows,
  isLoading,
  onSelect,
  onAction,
}: {
  columns: string[];
  rows: SnapshotRow[];
  isLoading: boolean;
  onSelect: (record: SnapshotRow) => void;
  onAction: (title: string, record: SnapshotRow) => void;
}) {
  return (
    <div className="taf-probes-status-matrix" aria-busy={isLoading}>
      <div className="taf-probes-status-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      {rows.map((row) => (
        <div
          key={rowKey(row)}
          className="taf-probes-status-row"
          role="button"
          tabIndex={0}
          onClick={() => onSelect(row)}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') onSelect(row);
          }}
        >
          {columns.map((column) => (
            <span key={column} className={`taf-probes-status-cell col-${columnToClass(column)}`} title={column === '操作' ? '查看详情 / 配置 / 刷新' : textValue(row, column)}>
              {renderProbeCell(column, row[column], row, onAction)}
            </span>
          ))}
        </div>
      ))}
    </div>
  );
}

function DeploymentTopology({
  graph,
  loading,
  error,
  mode,
  selectedNodeId,
  onSelect,
}: {
  graph?: ProbeTopologyGraph;
  loading: boolean;
  error?: unknown;
  mode: TopologyMode;
  selectedNodeId: string;
  onSelect: (id: string) => void;
}) {
  if (loading && !graph) return <div className="taf-probes-empty">正在读取部署拓扑 API…</div>;
  if (error && !graph) return <div className="taf-probes-empty is-error">部署拓扑 API 不可用</div>;
  if (!graph?.nodes.length) return <div className="taf-probes-empty">部署拓扑 API 暂无可定位节点</div>;
  return (
    <div className="taf-probes-topology">
      <div className="taf-probes-legend">
        <span><i className="is-ok" />在线</span>
        <span><i className="is-risk" />离线</span>
        <span><i className="is-warn" />告警</span>
        <span><b className="is-muted" />采集镜像</span>
        <span><b className="is-ok" />交换机</span>
        <span><b className="is-info" />链路</span>
        <span><b className="is-dim" />镜像口</span>
        <span><b className="is-info" />10-40 Gbps</span>
        <span><b className="is-risk" />≥ 40 Gbps</span>
      </div>
      <div className="taf-probes-map">
        <div className="taf-probes-topology-svg">
          <ProbeDeploymentSvg graph={graph} mode={mode} selectedNodeId={selectedNodeId} onSelect={onSelect} />
        </div>
        <span className="taf-probes-topology-source" title={`revision ${graph.revision}`}>API · {graph.nodes.length} 节点 / {graph.edges.length} 链路</span>
        <div className="taf-probes-map-note">
          <span><i className="is-ok" />探针</span>
          <span><i />交换机</span>
          <span><b />链路</span>
          <span><em />镜像口</span>
        </div>
      </div>
    </div>
  );
}

function ThroughputTrends({ rows, range }: { rows: SnapshotRow[]; range: '6h' | '24h' }) {
  const trend = buildProbeTrend(rows, range);
  const avgDrop = averageRowValue(rows, '丢包率');
  const avgParse = averageRowValue(rows, '解析率');
  const totalBandwidth = rows.reduce((total, row) => total + numericValue(textValue(row, '采集带宽')), 0);
  const pps = Math.round(rows.reduce((total, row) => total + Number(row.PPS || 0), 0));
  return (
    <div className="taf-probes-trends">
      <div className="taf-probes-linechart">
        <DataQualityTrendChart ariaLabel="采集带宽趋势" className="taf-probes-trend-echart" categories={trend.categories} series={trend.bandwidthSeries} valueFormatter={(value) => `${value}G`} />
      </div>
      <div className="taf-probes-trend-kpis">
        {[
          ['PPS (K)', pps.toLocaleString(), '实时聚合'],
          ['丢包率', `${avgDrop.toFixed(2)}%`, '实时均值'],
          ['解析率', `${avgParse.toFixed(2)}%`, '实时均值'],
          ['采集带宽', `${totalBandwidth.toFixed(1)}G`, `${rows.length} 台探针`],
        ].map(([label, value, delta]) => (
          <div key={label}><span>{label}</span><strong>{value}</strong><em>{delta}</em></div>
        ))}
      </div>
      <div className="taf-probes-threshold">
        <div className="taf-probes-threshold-head">
          <span>批量发送带宽 (Gbps)</span>
          <em><i />{trend.batchSeries[0]?.name || '-'} <b />阈值线 ({trend.threshold} Gbps)</em>
        </div>
        <DataQualityTrendChart ariaLabel="批量发送带宽" className="taf-probes-trend-echart" categories={trend.categories} series={trend.batchSeries} valueFormatter={(value) => `${value}G`} />
      </div>
    </div>
  );
}

function ProbeDetail({ selected }: { selected?: SnapshotRow }) {
  const status = textValue(selected, '状态') || '未知';
  const cpu = numericPercent(textValue(selected, 'CPU'));
  const memory = numericPercent(textValue(selected, '内存'));
  const disk = numericPercent(textValue(selected, '磁盘'));
  const drop = textValue(selected, '丢包率') || '-';
  const bandwidth = textValue(selected, '采集带宽') || '-';
  const heartbeatAge = heartbeatAgeLabel(Number(selected?.['最后心跳'] ?? 0));
  return (
    <WorkPanel title="选中探针详情" extra={<span className={`taf-probes-online is-${status === '离线' ? 'risk' : status === '告警' ? 'warn' : 'ok'}`}>{status}</span>}>
      <div className="taf-probes-detail">
        <div className="taf-probes-detail-grid">
          <span>探针 ID</span><strong>{textValue(selected, '探针 ID') || '-'}</strong>
          <span>所在位置</span><strong>{textValue(selected, '位置') || '-'}</strong>
          <span>采集网卡</span><strong>{textValue(selected, '采集网卡') || '-'}</strong>
          <span>采集模式</span><strong>{textValue(selected, '采集模式') || '-'}</strong>
          <span>版本</span><strong>{textValue(selected, '版本') || '-'}</strong>
          <span>运行时长</span><strong>{textValue(selected, '运行时长') || '-'}</strong>
        </div>
        <div className="taf-probes-resource">
          <ResourceBar label="CPU 使用率" value={cpu} />
          <ResourceBar label="内存使用率" value={memory} />
          <ResourceBar label="磁盘使用率" value={disk} />
          <div><span>采集带宽</span><strong>{bandwidth} / 40 Gbps</strong></div>
          <div><span>丢包率</span><strong className={drop.startsWith('0.') ? 'is-ok' : 'is-warn'}>{drop}</strong></div>
          <div><span>解析率</span><strong className="is-ok">{textValue(selected, '解析率') || '-'}</strong></div>
        </div>
        <div className="taf-probes-gates">
          {[
            ['运行状态', status, status === '离线' ? 'risk' : status === '告警' ? 'warn' : 'ok'],
            ['mTLS', textValue(selected, 'mTLS') || '-', textValue(selected, 'mTLS') === '已启用' ? 'ok' : 'warn'],
            ['归档状态', textValue(selected, '归档路径') === '-' ? '未配置' : '已配置', textValue(selected, '归档路径') === '-' ? 'warn' : 'ok'],
            ['心跳状态', heartbeatAge, status === '离线' ? 'risk' : 'ok'],
          ].map(([label, value, gateStatus]) => (
            <div key={label}><CheckIcon status={gateStatus as PageSnapshot['evidence'][number]['status']} /><span>{label}</span><strong>{value}</strong></div>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function ResourceBar({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <span>{label}</span>
      <Progress percent={value} size="small" showInfo={false} status={value > 70 ? 'exception' : value > 55 ? 'active' : 'success'} />
      <em>{value.toFixed(1)}%</em>
    </div>
  );
}

function CheckIcon({ status }: { status: PageSnapshot['evidence'][number]['status'] }) {
  return status === 'risk' ? <WarningOutlined /> : status === 'warn' ? <FieldTimeOutlined /> : <SafetyCertificateOutlined />;
}

function BatchOperations({ selected, onAction }: { selected?: SnapshotRow; onAction: (title: string) => void }) {
  const configItems: Array<[string, string, ReactNode]> = [
    ['采集网卡', textValue(selected, '采集网卡') || '-', <ApiOutlined key="nic" />],
    ['过滤策略', '由配置下发接口管理', <ControlOutlined key="filter" />],
    ['PCAP 归档', textValue(selected, '归档路径') === '-' ? '未配置' : '已启用', <FileProtectOutlined key="pcap" />],
    ['归档路径', textValue(selected, '归档路径') || '-', <CloudUploadOutlined key="archive" />],
    ['mTLS', textValue(selected, 'mTLS') || '-', <SafetyCertificateOutlined key="mtls" />],
    ['CPU 使用率', textValue(selected, 'CPU') || '-', <DashboardOutlined key="cpu" />],
    ['采集模式', textValue(selected, '采集模式') || '-', <DeploymentUnitOutlined key="buffer" />],
    ['采集带宽', textValue(selected, '采集带宽') || '-', <ThunderboltOutlined key="batch" />],
  ];
  return (
    <div className="taf-probes-batch">
      <div className="taf-probes-config">
        {configItems.map(([label, value, icon]) => (
          <div key={String(label)}>{icon}<span>{label}</span><strong title={String(value)}>{value}</strong></div>
        ))}
      </div>
      <div className="taf-probes-actions">
        {actionItems.map(([label, icon]) => <Button key={label} size="small" icon={icon} onClick={() => onAction(label)}>{label}</Button>)}
      </div>
    </div>
  );
}

function HeartbeatLog({ rows, onSelect }: { rows: SnapshotRow[]; onSelect: (probe: string) => void }) {
  const heartbeatRows = rows.slice(0, 7).map((row) => {
    const heartbeat = Number(row['最后心跳'] ?? 0);
    const status = textValue(row, '状态');
    const detail = status === '告警' ? `丢包率 ${textValue(row, '丢包率')}` : status === '离线' ? '心跳超时' : '心跳同步';
    return [heartbeatTime(heartbeat), rowKey(row), heartbeatAgeLabel(heartbeat), status === '在线' ? '正常' : status, detail];
  });
  return (
    <div className="taf-probes-heartbeat">
      <div><span>时间</span><span>探针 ID</span><span>延迟</span><span>状态</span><span>详情</span></div>
      {heartbeatRows.map(([time, probe, latency, status, detail]) => (
        <button key={`${time}-${probe}`} type="button" onClick={() => onSelect(probe)}>
          <span>{time}</span><strong>{probe}</strong><span>{latency}</span><StatusTag value={status} /><span>{detail}</span>
        </button>
      ))}
    </div>
  );
}

function renderProbeCell(column: string, value: unknown, row: SnapshotRow, onAction: (title: string, record: SnapshotRow) => void) {
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return (
    <Space size={1} className="taf-probe-row-actions">
      <Tooltip title="查看详情"><Button type="text" size="small" icon={<EyeOutlined />} aria-label="查看探针详情" onClick={(event) => { event.stopPropagation(); onAction('查看探针详情', row); }} /></Tooltip>
      <Tooltip title="配置"><Button type="text" size="small" icon={<SettingOutlined />} aria-label="配置探针" onClick={(event) => { event.stopPropagation(); onAction('配置探针', row); }} /></Tooltip>
      <Tooltip title="刷新"><Button type="text" size="small" icon={<ReloadOutlined />} aria-label="刷新探针" onClick={(event) => { event.stopPropagation(); onAction('刷新探针状态', row); }} /></Tooltip>
    </Space>
  );
  if (column === '探针 ID') return <span className="taf-probes-id-cell" title={String(value)}>{String(value)}</span>;
  if (column === '丢包率') return <span className={String(value).startsWith('0.') ? 'is-ok' : 'is-warn'}>{String(value)}</span>;
  if (column === 'CPU' || column === '内存') return <span className={numericPercent(String(value)) > 70 ? 'is-warn' : 'is-ok'}>{String(value)}</span>;
  const text = String(value ?? '-');
  return <span className="taf-cell-ellipsis" title={text}>{text}</span>;
}

function fallbackMetric(label: string): PageSnapshot['metrics'][number] {
  return { label, value: label.includes('率') ? '0.0%' : '0', delta: '等待 API', status: 'info' };
}

function rowKey(record: SnapshotRow) {
  return String(record['探针 ID'] ?? JSON.stringify(record));
}

function textValue(row: SnapshotRow | undefined, key: string) {
  const value = row?.[key];
  return value === undefined || value === null ? '' : String(value);
}

function numericPercent(value: string) {
  const parsed = Number.parseFloat(value.replace('%', ''));
  return Number.isFinite(parsed) ? parsed : 0;
}

function columnToClass(column: string) {
  return column.replace(/\s+/g, '-').replace(/[^\w\u4e00-\u9fa5-]/g, '');
}

function buildProbeRows(rows: SnapshotRow[]) {
  return rows;
}

function ProbeDeploymentSvg({
  mode,
  graph,
  selectedNodeId,
  onSelect,
}: {
  mode: TopologyMode;
  graph: ProbeTopologyGraph;
  selectedNodeId: string;
  onSelect: (id: string) => void;
}) {
  const [viewBox, setViewBox] = useState({ x: 0, y: 0, width: 1000, height: 420 });
  const dragRef = useRef<{ pointerId: number; x: number; y: number; viewX: number; viewY: number }>();
  const nodes = useMemo(() => [...graph.nodes].sort((left, right) => topologyPosition(left, mode).y - topologyPosition(right, mode).y), [graph.nodes, mode]);
  const projected = useMemo(() => new Map(nodes.map((node) => [node.id, topologyCanvasPoint(topologyPosition(node, mode))])), [nodes, mode]);
  const labeledNodeIds = useMemo(() => new Set([...graph.nodes]
    .sort((left, right) => (right.kind === 'core' ? 100 : 0) + right.bandwidth_gbps - ((left.kind === 'core' ? 100 : 0) + left.bandwidth_gbps))
    .slice(0, 5)
    .map((node) => node.id)), [graph.nodes]);
  const activate = (event: KeyboardEvent<SVGGElement>, id: string) => {
    if (event.key === 'Enter' || event.key === ' ') onSelect(id);
  };
  const resetView = () => setViewBox({ x: 0, y: 0, width: 1000, height: 420 });
  const zoom = (factor: number, focusX = viewBox.x + viewBox.width / 2, focusY = viewBox.y + viewBox.height / 2) => {
    const width = Math.max(480, Math.min(1400, viewBox.width * factor));
    const height = width * 0.42;
    const scale = width / viewBox.width;
    setViewBox({
      x: focusX - (focusX - viewBox.x) * scale,
      y: focusY - (focusY - viewBox.y) * scale,
      width,
      height,
    });
  };
  const onWheel = (event: ReactWheelEvent<SVGSVGElement>) => {
    event.preventDefault();
    const rect = event.currentTarget.getBoundingClientRect();
    const x = viewBox.x + ((event.clientX - rect.left) / rect.width) * viewBox.width;
    const y = viewBox.y + ((event.clientY - rect.top) / rect.height) * viewBox.height;
    zoom(event.deltaY > 0 ? 1.12 : 0.88, x, y);
  };
  const onPointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if ((event.target as Element).closest('.taf-probe-deployment-svg__node')) return;
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, viewX: viewBox.x, viewY: viewBox.y };
  };
  const onPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const drag = dragRef.current;
    if (!drag || drag.pointerId !== event.pointerId) return;
    const rect = event.currentTarget.getBoundingClientRect();
    setViewBox((current) => ({ ...current, x: drag.viewX - ((event.clientX - drag.x) / rect.width) * current.width, y: drag.viewY - ((event.clientY - drag.y) / rect.height) * current.height }));
  };
  const onPointerUp = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (dragRef.current?.pointerId === event.pointerId) dragRef.current = undefined;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };
  return (
    <div className="taf-probe-deployment-stage">
      <div className="taf-probe-deployment-controls" aria-label="拓扑缩放控制">
        <Tooltip title="放大"><button type="button" aria-label="放大部署拓扑" onClick={() => zoom(0.84)}><ZoomInOutlined /></button></Tooltip>
        <Tooltip title="缩小"><button type="button" aria-label="缩小部署拓扑" onClick={() => zoom(1.18)}><ZoomOutOutlined /></button></Tooltip>
        <Tooltip title="重置视图"><button type="button" aria-label="重置部署拓扑" onClick={resetView}><FullscreenOutlined /></button></Tooltip>
      </div>
      <svg
        className={`taf-probe-deployment-svg is-${mode}`}
        viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.width} ${viewBox.height}`}
        preserveAspectRatio="xMidYMid meet"
        role="img"
        aria-label={`探针部署拓扑${mode.toUpperCase()}动态图`}
        data-topology-mode={mode}
        data-topology-source={graph.source}
        data-topology-revision={graph.revision}
        data-node-count={nodes.length}
        data-link-count={graph.edges.length}
        onWheel={onWheel}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <defs>
          <linearGradient id={`probe-ground-${mode}`} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#0b3148" stopOpacity=".82" />
            <stop offset="1" stopColor="#03111d" stopOpacity=".42" />
          </linearGradient>
          <pattern id={`probe-grid-${mode}`} width="34" height="34" patternUnits="userSpaceOnUse">
            <path d="M34 0H0V34" fill="none" stroke="#3485ad" strokeOpacity=".14" strokeWidth="1" />
          </pattern>
          <filter id={`probe-glow-${mode}`} x="-80%" y="-80%" width="260%" height="260%">
            <feGaussianBlur stdDeviation="4" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
          <marker id={`probe-arrow-${mode}`} markerWidth="10" markerHeight="10" refX="8" refY="5" orient="auto" markerUnits="userSpaceOnUse">
            <path d="M0 0L10 5 0 10Z" fill="#7fd4ff" fillOpacity=".9" />
          </marker>
        </defs>

        <rect className="taf-probe-deployment-svg__canvas" width="1000" height="420" fill={`url(#probe-grid-${mode})`} />
        <path className="taf-probe-deployment-svg__campus" d={mode === '3d' ? 'M72 145L475 24 938 158 561 395 90 310Z' : 'M40 32H960V388H40Z'} fill={`url(#probe-ground-${mode})`} />
        <g className="taf-probe-deployment-svg__roads">
          <path d="M90 286C265 222 414 220 566 195S796 134 925 157" />
          <path d="M112 330C284 274 430 279 578 309S787 354 914 307" />
          <path d="M264 69C339 151 410 211 566 195S716 234 830 335" />
        </g>

        <g className="taf-probe-deployment-svg__zones">
          {graph.zones.map((zone, index) => {
            const polygon = mode === '3d' ? zone.polygon_3d : zone.polygon_2d;
            const points = polygon.map(topologyCanvasPoint);
            const labelPoint = points[0] ?? { x: 0, y: 0 };
            return <g key={zone.id} className={`taf-probe-deployment-svg__zone zone-${index % 4} is-${zone.status}`} data-zone-id={zone.id}><polygon points={points.map((point) => `${point.x},${point.y}`).join(' ')} /><text x={labelPoint.x + 10} y={labelPoint.y + 19}>{zone.label}</text></g>;
          })}
        </g>

        <g className="taf-probe-deployment-svg__links">
          {graph.edges.map((link) => {
            const source = projected.get(link.source);
            const target = projected.get(link.target);
            if (!source || !target) return null;
            const middleX = (source.x + target.x) / 2;
            const middleY = (source.y + target.y) / 2 - (mode === '3d' ? 24 : 0);
            const path = mode === '3d' ? `M${source.x} ${source.y}Q${middleX} ${middleY} ${target.x} ${target.y}` : `M${source.x} ${source.y}L${target.x} ${target.y}`;
            return (
              <g key={link.id} className={`is-${link.status} is-${link.kind}`} data-bandwidth-gbps={link.bandwidth_gbps}>
                <path d={path} markerEnd={`url(#probe-arrow-${mode})`} />
                {link.bandwidth_gbps >= 40 && <text x={middleX} y={middleY - 7}>{link.bandwidth_gbps.toFixed(0)}G</text>}
              </g>
            );
          })}
        </g>

        <g className="taf-probe-deployment-svg__nodes">
          {nodes.map((node) => {
            const point = projected.get(node.id)!;
            const selected = node.id === selectedNodeId;
            const height = mode === '3d' ? Math.max(24, node.elevation * 3.2) : 0;
            const labelY = mode === '3d'
              ? (point.y - height < 70 ? 30 : -height - 48)
              : (point.y < 66 ? 32 : -42);
            return (
              <g key={node.id} className={`taf-probe-deployment-svg__node is-${node.status} is-${node.kind} ${selected ? 'is-selected' : ''} ${labeledNodeIds.has(node.id) ? 'is-labeled' : ''}`} transform={`translate(${point.x} ${point.y})`} role="button" tabIndex={0} data-probe-id={node.id} data-api-position={JSON.stringify(topologyPosition(node, mode))} onClick={() => onSelect(node.id)} onKeyDown={(event) => activate(event, node.id)}>
                <rect className="taf-probe-deployment-svg__click-target" x="-60" y={mode === '3d' ? -height - 66 : -58} width="120" height={mode === '3d' ? height + 84 : 82} />
                {mode === '3d' ? (
                  <g className="taf-probe-deployment-svg__building">
                    <ellipse className="is-pad" cx="0" cy="9" rx="34" ry="13" />
                    <path className="is-left" d={`M-25 0L0 11V${11 - height}L-25 ${-height}Z`} />
                    <path className="is-right" d={`M0 11L25 0V${-height}L0 ${11 - height}Z`} />
                    <path className="is-roof" d={`M-25 ${-height}L0 ${-height - 12}L25 ${-height}L0 ${11 - height}Z`} />
                    {Array.from({ length: 4 }, (_, windowIndex) => <rect key={windowIndex} className="is-window" x={-18 + windowIndex * 10} y={-height + 10} width="5" height="5" />)}
                  </g>
                ) : (
                  <g className="taf-probe-deployment-svg__flat-node">
                    <rect x="-24" y="-17" width="48" height="34" rx="6" />
                    <path d="M-14-5H14M-14 3H14M-8 11H8" />
                  </g>
                )}
                <circle className="taf-probe-deployment-svg__halo" cy={mode === '3d' ? -height - 13 : 0} r={selected ? 25 : 18} />
                <circle className="taf-probe-deployment-svg__pin" cy={mode === '3d' ? -height - 13 : 0} r="6" filter={`url(#probe-glow-${mode})`} />
                <g className="taf-probe-deployment-svg__label" transform={`translate(0 ${labelY})`}>
                  <rect x="-58" y="-15" width="116" height="34" rx="5" />
                  <text className="is-title" textAnchor="middle" y="-1">{compactTopologyLabel(node.label)}</text>
                  <text className="is-detail" textAnchor="middle" y="13">{node.role} · {node.bandwidth_gbps.toFixed(1)}G</text>
                </g>
                <title>{`${node.probe_id} · ${node.zone} · ${topologyStatusLabel(node.status)} · ${node.bandwidth_gbps.toFixed(1)} Gbps`}</title>
              </g>
            );
          })}
        </g>
      </svg>
    </div>
  );
}

const topologyPosition = (node: ProbeTopologyNode, mode: TopologyMode): ProbeTopologyPoint => mode === '3d' ? node.position_3d : node.position_2d;

const topologyCanvasPoint = (point: ProbeTopologyPoint) => ({ x: 40 + point.x * 9.2, y: 20 + point.y * 3.8 });

const topologyStatusLabel = (status: string) => status === 'risk' ? '离线' : status === 'warn' ? '告警' : '在线';

const compactTopologyLabel = (value: string) => value.length > 10 ? `${value.slice(0, 9)}…` : value;

function buildProbeTrend(rows: SnapshotRow[], range: '6h' | '24h') {
  const points = range === '6h' ? 22 : 24;
  const sourceRows = rows.filter((row) => parseNumberArray(textValue(row, '带宽序列')).length > 0);
  const labels = parseStringArray(textValue(sourceRows[0], '趋势标签')).slice(-points);
  const threshold = Number(sourceRows[0]?.['带宽阈值'] || 0);
  return {
    categories: labels,
    threshold,
    bandwidthSeries: sourceRows.slice(0, 3).map((row, index) => ({
      name: compactProbeSeriesName(rowKey(row)),
      color: ['#36d66b', '#18a8ff', '#ffb020'][index],
      values: parseNumberArray(textValue(row, '带宽序列')).slice(-points),
      area: index === 0,
    })),
    batchSeries: sourceRows.length ? [
      { name: compactProbeSeriesName(rowKey(sourceRows[0])), color: '#18a8ff', values: parseNumberArray(textValue(sourceRows[0], '批量序列')).slice(-points), area: true },
      { name: `阈值线 (${threshold} Gbps)`, color: '#ff4d4f', values: Array.from({ length: labels.length }, () => threshold), dashed: true },
    ] : [],
  };
}

function compactProbeSeriesName(id: string) {
  return id.replace('PROBE-', '').replace('BUILD-', 'B-').replace('OFFICE-', 'O-').replace('BRANCH-', 'R-');
}

function createProbeAction(title: string, targets: string[]): ProbeAction {
  const actions = pageApiPlans.probes.actions ?? [];
  const readOnly = title.includes('查看') || title.includes('心跳') || title.includes('日志');
  const actionId: ProbeOperationActionId | undefined = readOnly ? undefined
    : title.includes('升级') ? 'probe-batch-upgrade'
      : title.includes('启停') ? 'probe-batch-state'
        : title.includes('策略') || title.includes('配置') || title.includes('编辑') ? 'probe-config-push'
          : title.includes('连通') || title.includes('刷新') ? 'probe-connectivity-test'
            : title.includes('证书') ? 'probe-cert-rotate'
              : title.includes('重启') ? 'probe-restart'
                : undefined;
  const plan = actions.find((item) => item.id === actionId);
  return {
    id: actionId,
    title,
    targets: [...new Set(targets.filter(Boolean))],
    endpoint: plan?.endpoint ?? '/v1/probes',
    auditEvent: plan?.auditEvent ?? 'PROBE_READ',
    readOnly,
    payload: { ...(plan?.defaultBody ?? {}) },
  };
}

function numericValue(value: string) {
  const parsed = Number.parseFloat(value.replace(/[^\d.]/g, ''));
  return Number.isFinite(parsed) ? parsed : 0;
}

function parseStringArray(value: string) {
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.map(String).filter(Boolean) : [];
  } catch {
    return [];
  }
}

function parseNumberArray(value: string) {
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.map(Number).filter(Number.isFinite) : [];
  } catch {
    return [];
  }
}

function averageRowValue(rows: SnapshotRow[], key: string) {
  if (!rows.length) return 0;
  return rows.reduce((total, row) => total + numericValue(textValue(row, key)), 0) / rows.length;
}

function heartbeatTime(value: number) {
  if (!value) return '--:--:--';
  const milliseconds = value > 10_000_000_000 ? value : value * 1000;
  return new Date(milliseconds).toLocaleTimeString('zh-CN', { hour12: false });
}

function heartbeatAgeLabel(value: number) {
  if (!value) return '-';
  const milliseconds = value > 10_000_000_000 ? value : value * 1000;
  const seconds = Math.max(0, Math.round((Date.now() - milliseconds) / 1000));
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  return `${Math.round(seconds / 3600)}h`;
}
