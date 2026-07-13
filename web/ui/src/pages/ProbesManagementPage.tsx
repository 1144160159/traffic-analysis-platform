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
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Modal, Progress, Select, Space, Tooltip } from 'antd';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { DataQualityTrendChart, TopicTopologyGraph } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const topologyNodes = [
  { id: 'library', label: '图书馆', probe: 'PROBE-LIB-01', x: 32, y: 25, tone: 'ok' },
  { id: 'teaching', label: '教学区', probe: 'PROBE-BUILD-01', x: 14, y: 48, tone: 'ok' },
  { id: 'office', label: '办公区', probe: 'PROBE-OFFICE-01', x: 30, y: 69, tone: 'ok' },
  { id: 'core', label: '核心区', probe: 'CORE-SW-01', x: 51, y: 49, tone: 'info' },
  { id: 'datacenter', label: '数据中心', probe: 'PROBE-DC-01', x: 51, y: 78, tone: 'ok' },
  { id: 'lab', label: '实验楼', probe: 'PROBE-LAB-01', x: 78, y: 26, tone: 'ok' },
  { id: 'dorm', label: '宿舍区', probe: 'PROBE-DORM-01', x: 91, y: 49, tone: 'warn' },
  { id: 'branch', label: '汇聚区 B', probe: 'PROBE-BRANCH-01', x: 78, y: 73, tone: 'risk' },
];

const topologyLinks = [
  { source: 'teaching', target: 'core', tone: 'info' as const },
  { source: 'library', target: 'core', tone: 'ok' as const },
  { source: 'office', target: 'core', tone: 'ok' as const },
  { source: 'datacenter', target: 'core', tone: 'info' as const },
  { source: 'lab', target: 'core', tone: 'info' as const },
  { source: 'dorm', target: 'core', tone: 'risk' as const },
  { source: 'branch', target: 'core', tone: 'risk' as const },
  { source: 'office', target: 'datacenter', tone: 'ok' as const },
  { source: 'teaching', target: 'datacenter', tone: 'info' as const },
  { source: 'datacenter', target: 'dorm', tone: 'ok' as const },
];

const heartbeatRows = [
  ['03:45:00', 'PROBE-DC-01', '1s', '正常', '心跳同步'],
  ['03:44:59', 'PROBE-DC-02', '1s', '正常', '心跳同步'],
  ['03:44:58', 'PROBE-BUILD-01', '2s', '正常', '心跳同步'],
  ['03:44:57', 'PROBE-BUILD-02', '1s', '正常', '心跳同步'],
  ['03:44:56', 'PROBE-OFFICE-01', '1s', '正常', '心跳同步'],
  ['03:44:54', 'PROBE-SPORT-01', '1s', '告警', '丢包率 1.12%'],
  ['03:44:49', 'PROBE-DORM-01', '-', '离线', '超时 3m21s'],
];

const configItems = [
  ['采集网卡', 'eth2, eth3', <ApiOutlined key="nic" />],
  ['过滤策略', '办公网_全量_20260601', <ControlOutlined key="filter" />],
  ['PCAP 归档', '已启用 (MinIO)', <FileProtectOutlined key="pcap" />],
  ['归档路径', 's3://pcap-archive/probe-dc-01/', <CloudUploadOutlined key="archive" />],
  ['mTLS', '已启用', <SafetyCertificateOutlined key="mtls" />],
  ['CPU 亲和', '0, 1, 2, 3', <DashboardOutlined key="cpu" />],
  ['缓冲区大小', '4096 MB', <DeploymentUnitOutlined key="buffer" />],
  ['批量发送', '2.0 Gbps', <ThunderboltOutlined key="batch" />],
];

const actionItems: Array<[string, ReactNode]> = [
  ['批量升级', <CloudUploadOutlined key="upgrade" />],
  ['批量启停', <PoweroffOutlined key="power" />],
  ['策略下发', <SafetyCertificateOutlined key="policy" />],
  ['连通性测试', <SyncOutlined key="connect" />],
  ['证书轮换', <SettingOutlined key="cert" />],
  ['重启探针', <ReloadOutlined key="restart" />],
];

type TopologyMode = '2d' | '3d';

type ProbeAction = {
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
};

export function ProbesManagementPage({ route }: { route: NavRoute }) {
  const [selectedKey, setSelectedKey] = useState<string>();
  const [selectedTopologyId, setSelectedTopologyId] = useState('datacenter');
  const [topologyMode, setTopologyMode] = useState<TopologyMode>('3d');
  const [matrixPage, setMatrixPage] = useState(1);
  const [trendRange, setTrendRange] = useState<'6h' | '24h'>('6h');
  const [matrixExpanded, setMatrixExpanded] = useState(false);
  const [action, setAction] = useState<ProbeAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    refetchInterval: 15_000,
    refetchIntervalInBackground: true,
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const probeRows = useMemo(() => buildProbeRows(rows), [rows]);
  const pageSize = 7;
  const pageCount = Math.max(1, Math.ceil(probeRows.length / pageSize));
  const visibleRows = probeRows.slice((matrixPage - 1) * pageSize, matrixPage * pageSize);
  const selected = useMemo(() => probeRows.find((row) => rowKey(row) === selectedKey) ?? probeRows[0], [probeRows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const openAction = (title: string, target: string) => {
    setActionSubmitted(false);
    setAction(createProbeAction(title, target));
  };
  const actionUsesModal = Boolean(action && /配置|升级|证书/.test(action.title));
  const closeAction = () => {
    setAction(undefined);
    setActionSubmitted(false);
  };
  const selectTopology = (id: string) => {
    setSelectedTopologyId(id);
    const index = topologyNodes.findIndex((node) => node.id === id);
    const target = probeRows[index];
    if (target) setSelectedKey(rowKey(target));
  };

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
              {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
            </div>
          </section>

          <WorkPanel
            title="部署拓扑"
            extra={(
              <Space size={6}>
                <Button size="small" type={topologyMode === '2d' ? 'primary' : 'default'} aria-pressed={topologyMode === '2d'} onClick={() => setTopologyMode('2d')}>2D</Button>
                <Button size="small" type={topologyMode === '3d' ? 'primary' : 'default'} aria-pressed={topologyMode === '3d'} onClick={() => setTopologyMode('3d')}>3D</Button>
                <Tooltip title="刷新部署拓扑"><Button size="small" icon={<SyncOutlined />} aria-label="刷新部署拓扑" onClick={() => void refetch()} /></Tooltip>
              </Space>
            )}
          >
            <DeploymentTopology rows={probeRows} mode={topologyMode} selectedNodeId={selectedTopologyId} onSelect={selectTopology} />
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
          <WorkPanel title="批量运维" extra={<Button size="small" type="link" onClick={() => openAction('编辑批量运维', textValue(selected, '探针 ID') || '当前探针组')}>编辑</Button>}>
            <BatchOperations onAction={(title) => openAction(title, textValue(selected, '探针 ID') || '当前探针组')} />
          </WorkPanel>
          <WorkPanel title="心跳与日志" extra={<Button size="small" type="link" onClick={() => openAction('查看全部心跳与日志', textValue(selected, '探针 ID') || '当前探针组')}>更多</Button>}>
            <HeartbeatLog onSelect={(probe) => openAction('查看心跳详情', probe)} />
          </WorkPanel>
        </aside>
      </section>
      <Drawer
        className="taf-probe-action-drawer"
        title={action ? `${action.title}确认` : '探针操作确认'}
        open={Boolean(action && !actionUsesModal)}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onClose={closeAction}
        extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}
      >
        {action && <ProbeActionBody action={action} submitted={actionSubmitted} />}
      </Drawer>
      <Modal
        className="taf-probe-action-modal"
        title={action ? `${action.title}确认` : '探针操作确认'}
        open={actionUsesModal}
        width="min(620px, calc(var(--taf-window-inner-width, 100dvw) - 64px))"
        onCancel={closeAction}
        footer={[
          <Button key="cancel" onClick={closeAction}>取消</Button>,
          <Button key="submit" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>,
        ]}
      >
        {action && <ProbeActionBody action={action} submitted={actionSubmitted} />}
      </Modal>
      <Drawer title="探针状态矩阵" placement="bottom" height="72%" open={matrixExpanded} onClose={() => setMatrixExpanded(false)}>
        <ProbeStatusMatrix columns={route.page.tableColumns} rows={visibleRows} isLoading={isLoading} onSelect={(record) => setSelectedKey(rowKey(record))} onAction={(title, record) => openAction(title, rowKey(record))} />
      </Drawer>
    </div>
  );
}

function ProbeActionBody({ action, submitted }: { action: ProbeAction; submitted: boolean }) {
  return (
    <div className="taf-alert-detail-action-body">
      <p>将为探针对象创建“{action.title}”任务；执行前校验租户、权限、版本兼容性和影响范围，并保留操作者与审计上下文。</p>
      <dl>
        <dt>操作对象</dt><dd>{action.target}</dd>
        <dt>接口预留</dt><dd>{action.endpoint}</dd>
        <dt>审计事件</dt><dd>{action.auditEvent}</dd>
      </dl>
      {submitted && <Alert type="success" showIcon message="探针业务操作已进入仿真任务队列" description={`目标：${action.target}；动作：${action.title}`} />}
    </div>
  );
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
  rows,
  mode,
  selectedNodeId,
  onSelect,
}: {
  rows: SnapshotRow[];
  mode: TopologyMode;
  selectedNodeId: string;
  onSelect: (id: string) => void;
}) {
  const nodes = buildProbeTopologyNodes(rows, mode, selectedNodeId);
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
          <TopicTopologyGraph ariaLabel="探针部署拓扑" nodes={nodes} links={topologyLinks} onNodeClick={onSelect} />
        </div>
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
  return (
    <div className="taf-probes-trends">
      <div className="taf-probes-linechart">
        <DataQualityTrendChart ariaLabel="采集带宽趋势" className="taf-probes-trend-echart" categories={trend.categories} series={trend.bandwidthSeries} valueFormatter={(value) => `${value}G`} />
      </div>
      <div className="taf-probes-trend-kpis">
        {[
          ['PPS (K)', '1,286', '+8.2%'],
          ['丢包率', '0.02%', '-0.01%'],
          ['解析率', '99.18%', '+0.16%'],
          ['背压率', '0.06%', '+0.02%'],
        ].map(([label, value, delta]) => (
          <div key={label}><span>{label}</span><strong>{value}</strong><em>{delta}</em></div>
        ))}
      </div>
      <div className="taf-probes-threshold">
        <div className="taf-probes-threshold-head">
          <span>批量发送带宽 (Gbps)</span>
          <em><i />PROBE-DC-01 <b />阈值线 (30 Gbps)</em>
        </div>
        <DataQualityTrendChart ariaLabel="批量发送带宽" className="taf-probes-trend-echart" categories={trend.categories} series={trend.batchSeries} valueFormatter={(value) => `${value}G`} />
      </div>
    </div>
  );
}

function ProbeDetail({ selected }: { selected?: SnapshotRow }) {
  const status = textValue(selected, '状态') || '在线';
  const cpu = numericPercent(textValue(selected, 'CPU')) || 28.7;
  const memory = numericPercent(textValue(selected, '内存')) || 38.2;
  const drop = textValue(selected, '丢包率') || '0.02%';
  const bandwidth = textValue(selected, '采集带宽') || '18.6 Gbps';
  return (
    <WorkPanel title="选中探针详情" extra={<span className={`taf-probes-online is-${status === '离线' ? 'risk' : status === '告警' ? 'warn' : 'ok'}`}>{status}</span>}>
      <div className="taf-probes-detail">
        <div className="taf-probes-detail-grid">
          <span>探针 ID</span><strong>{textValue(selected, '探针 ID') || 'PROBE-DC-01'}</strong>
          <span>所在位置</span><strong>{textValue(selected, '位置') || '数据中心机房 A'}</strong>
          <span>采集网卡</span><strong>eth2 (100G)</strong>
          <span>采集模式</span><strong>{textValue(selected, '采集模式') || '混合 (L2+L3)'}</strong>
          <span>版本</span><strong>{textValue(selected, '版本') || 'v3.4.7'}</strong>
          <span>运行时长</span><strong>{textValue(selected, '运行时长') || '12d 14h'}</strong>
        </div>
        <div className="taf-probes-resource">
          <ResourceBar label="CPU 使用率" value={cpu} />
          <ResourceBar label="内存使用率" value={memory} />
          <ResourceBar label="磁盘使用率" value={56.1} />
          <div><span>采集带宽</span><strong>{bandwidth} / 40 Gbps</strong></div>
          <div><span>丢包率</span><strong className={drop.startsWith('0.') ? 'is-ok' : 'is-warn'}>{drop}</strong></div>
          <div><span>解析率</span><strong className="is-ok">{textValue(selected, '解析率') || '99.21%'}</strong></div>
        </div>
        <div className="taf-probes-gates">
          {[
            ['运行状态', '剩余 63 天', 'ok'],
            ['mTLS', '已启用', 'ok'],
            ['时间同步', '正常', 'ok'],
            ['心跳状态', '正常 1s 前', 'ok'],
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

function BatchOperations({ onAction }: { onAction: (title: string) => void }) {
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

function HeartbeatLog({ onSelect }: { onSelect: (probe: string) => void }) {
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
  const source = rows.length ? rows : [
    { '探针 ID': 'PROBE-DC-01', 位置: '数据中心', 状态: '在线', CPU: '28.7%', 内存: '38.2%', '丢包率': '0.02%', '采集带宽': '18.6 Gbps', '采集模式': '混合 (L2+L3)', 版本: 'v3.4.7', '运行时长': '12d 14h', '解析率': '99.21%' },
  ];
  if (source.length >= 25) return source;
  return Array.from({ length: 25 }, (_, index) => {
    const row = source[index % source.length];
    if (index < source.length) return row;
    return { ...row, '探针 ID': `${rowKey(row)}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}` };
  });
}

function buildProbeTopologyNodes(rows: SnapshotRow[], mode: TopologyMode, selectedNodeId: string) {
  return topologyNodes.map((node, index) => {
    const row = rows[index % Math.max(rows.length, 1)];
    const status = textValue(row, '状态') || (node.tone === 'risk' ? '离线' : node.tone === 'warn' ? '告警' : '在线');
    const tone: 'risk' | 'proxy' | 'protocol' | 'probe' = status === '离线' ? 'risk' : status === '告警' ? 'proxy' : node.id === 'core' ? 'protocol' : 'probe';
    const offset = mode === '3d' ? (index % 2 === 0 ? -2 : 2) : 0;
    return {
      id: node.id,
      label: textValue(row, '位置') || node.label,
      detail: `${textValue(row, '探针 ID') || node.probe} | ${status}`,
      x: node.x + offset,
      y: node.y - (mode === '3d' ? (index % 3) * 2 : 0),
      tone,
      size: [112, 42] as [number, number],
      selected: node.id === selectedNodeId,
    };
  });
}

function buildProbeTrend(rows: SnapshotRow[], range: '6h' | '24h') {
  const points = range === '6h' ? 22 : 24;
  const bandwidth = Math.max(12, Math.round(rows.reduce((total, row) => total + numericValue(textValue(row, '采集带宽')), 0) / Math.max(rows.length, 1)) || 18);
  const cpu = Math.max(8, Math.round(rows.reduce((total, row) => total + numericPercent(textValue(row, 'CPU')), 0) / Math.max(rows.length, 1)) || 22);
  const categories = Array.from({ length: points }, (_, index) => {
    const hour = range === '6h' ? 21 + Math.floor(index * 6 / Math.max(points - 1, 1)) : index;
    return `${String(hour % 24).padStart(2, '0')}:${index % 2 ? '30' : '00'}`;
  });
  const series = (base: number, amplitude: number, phase: number) => Array.from({ length: points }, (_, index) => {
    const value = base + Math.sin((index + phase) * 0.72) * amplitude + ((index * 5 + phase) % 4) - 1.5;
    return Math.max(0, Math.round(value * 10) / 10);
  });
  return {
    categories,
    bandwidthSeries: [
      { name: 'PROBE-DC-01', color: '#36d66b', values: series(bandwidth, 4.8, 0), area: true },
      { name: 'PROBE-BUILD-01', color: '#18a8ff', values: series(Math.max(6, bandwidth * 0.52), 2.5, 2) },
      { name: 'PROBE-OFFICE-01', color: '#ffb020', values: series(Math.max(4, bandwidth * 0.36), 1.8, 4) },
    ],
    batchSeries: [
      { name: 'PROBE-DC-01', color: '#18a8ff', values: series(Math.max(12, bandwidth + cpu * 0.22), 3.8, 1), area: true },
      { name: '阈值线 (30 Gbps)', color: '#ff4d4f', values: Array.from({ length: points }, () => 30), dashed: true },
    ],
  };
}

function createProbeAction(title: string, target: string): ProbeAction {
  const actions = pageApiPlans.probes.actions ?? [];
  const plan = actions.find((item) =>
    (title.includes('升级') && item.id === 'probe-batch-upgrade')
    || (title.includes('策略') || title.includes('配置')) && item.id === 'probe-config-push'
    || title.includes('连通') && item.id === 'probe-connectivity-test'
    || title.includes('证书') && item.id === 'probe-cert-rotate',
  );
  return {
    title,
    target,
    endpoint: plan?.endpoint ?? '/v1/probes/{id}/operations',
    auditEvent: plan?.auditEvent ?? 'PROBE_OPERATION_SIMULATED',
  };
}

function numericValue(value: string) {
  const parsed = Number.parseFloat(value.replace(/[^\d.]/g, ''));
  return Number.isFinite(parsed) ? parsed : 0;
}
