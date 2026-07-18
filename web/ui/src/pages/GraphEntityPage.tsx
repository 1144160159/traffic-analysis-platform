import {
  AimOutlined,
  ApartmentOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  ClusterOutlined,
  DatabaseOutlined,
  ExpandOutlined,
  FileSearchOutlined,
  HistoryOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Input, Select, Space, Table, Tabs, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchAsset, fetchPageSnapshot } from '@/services/api';
import type { SnapshotRow } from '@/services/mockData';

const graphNodes = [
  { id: 'edge-ip', label: '可疑外部IP', detail: '185.234.15.23', type: 'ip', x: 22, y: 18, tone: 'risk', icon: <RadarChartOutlined /> },
  { id: 'account', label: 'biz_admin', detail: '账号', type: 'account', x: 48, y: 16, tone: 'ok', icon: <ApartmentOutlined /> },
  { id: 'web', label: 'WEB-SRV-02', detail: '10.20.4.19', type: 'host', x: 70, y: 20, tone: 'info', icon: <DatabaseOutlined /> },
  { id: 'gateway', label: '边界网关', detail: '10.20.0.1', type: 'host', x: 16, y: 46, tone: 'info', icon: <NodeIndexOutlined /> },
  { id: 'center', label: '核心业务服务器', detail: '10.20.4.18', type: 'host', x: 48, y: 46, tone: 'center', icon: <DatabaseOutlined /> },
  { id: 'db', label: 'DB-SRV-01', detail: '10.20.4.20', type: 'service', x: 74, y: 46, tone: 'warn', icon: <DatabaseOutlined /> },
  { id: 'domain', label: 'erp.corp.edu.cn', detail: '域名', type: 'domain', x: 28, y: 70, tone: 'warn', icon: <ClusterOutlined /> },
  { id: 'pcap', label: 'PCAP-0156', detail: '证据', type: 'evidence', x: 48, y: 78, tone: 'info', icon: <FileSearchOutlined /> },
  { id: 'alert', label: 'ALERT-1287', detail: '告警', type: 'alert', x: 72, y: 70, tone: 'risk', icon: <BranchesOutlined /> },
];

const graphEdges = [
  ['edge-ip', 'center', '通信', 'risk'],
  ['account', 'center', '登录', 'ok'],
  ['web', 'center', '通信', 'info'],
  ['gateway', 'center', '通信', 'info'],
  ['center', 'db', '通信', 'warn'],
  ['domain', 'center', 'DNS解析', 'warn'],
  ['center', 'pcap', '证据引用', 'info'],
  ['center', 'alert', '关联告警', 'risk'],
];

const graphOverlays: OverlayContract[] = [
  {
    id: 'drawer-graph-entity',
    title: '图谱实体详情',
    kind: 'Drawer',
    actionLabel: '实体详情',
    description: '展示实体属性、关系、风险标签、证据和关联告警。',
  },
  {
    id: 'drawer-graph-path-analysis',
    title: '图谱路径分析',
    kind: 'Drawer',
    actionLabel: '路径分析',
    description: '分析源实体到目标实体的最短路径、风险权重、证据链和处置建议。',
  },
];

export function GraphEntityPage({ route }: { route: NavRoute }) {
  const [searchParams] = useSearchParams();
  const sourceAssetId = searchParams.get('assetId') ?? '';
  const [activePath, setActivePath] = useState(route.page.tabs[0]);
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const sourceAsset = useQuery({
    queryKey: ['asset', sourceAssetId],
    queryFn: () => fetchAsset(sourceAssetId),
    enabled: Boolean(sourceAssetId),
  });
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, sourceAssetId, sourceAsset.data?.ip_address],
    queryFn: () => fetchPageSnapshot(route.id, { sourceAssetIp: sourceAsset.data?.ip_address }),
    enabled: !sourceAssetId || Boolean(sourceAsset.data?.ip_address),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedRow = useMemo(() => {
    if (!rows.length) return undefined;
    return rows.find((row) => rowKey(row) === selectedRowKey) ?? rows[0];
  }, [rows, selectedRowKey]);

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderGraphCell(column, value),
  }));

  return (
    <div className="taf-page taf-graph-entity">
      <header className="taf-graph-titlebar">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Tooltip title="定位中心节点">
            <Button size="small" icon={<AimOutlined />}>定位中心节点</Button>
          </Tooltip>
          <Tooltip title="刷新图谱">
            <Button size="small" icon={<SearchOutlined />} onClick={() => void refetch()} />
          </Tooltip>
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 APISIX 图谱路由、Graph Service、NebulaGraph/ClickHouse 或鉴权状态。'}
          action={
            <Button size="small" danger onClick={() => void refetch()}>
              重试
            </Button>
          }
        />
      )}

      {sourceAssetId && <Alert showIcon type={sourceAsset.isError ? 'error' : 'info'} message={sourceAsset.isError ? '资产上下文解析失败' : '已接收资产台账上下文'} description={sourceAsset.isError ? '无法从资产服务解析中心实体 IP。' : `中心实体资产 ID：${sourceAssetId}${sourceAsset.data?.ip_address ? ` · IP：${sourceAsset.data.ip_address}` : ' · 正在解析 IP'}`} />}

      <div className="taf-graph-toolbar">
        <Input prefix={<SearchOutlined />} placeholder="搜索 IP / 账号 / 主机 / 域名 / 服务 / 告警 ID / 资产 ID" value={sourceAssetId} readOnly />
        <Select size="small" value="近24小时" options={[{ value: '近24小时' }, { value: '近7天' }, { value: '自定义' }]} />
        <Select size="small" value="主园区" options={[{ value: '主园区' }, { value: '实验楼' }, { value: '全部园区' }]} />
        <Select size="small" value="实体类型：全部" options={[{ value: '实体类型：全部' }, { value: '主机' }, { value: '账号' }, { value: '告警' }]} />
        <Select size="small" value="关系深度：二跳" options={[{ value: '关系深度：一跳' }, { value: '关系深度：二跳' }]} />
        <Button size="small" type="primary" icon={<BranchesOutlined />}>路径分析</Button>
        <Button size="small">保存视图</Button>
        <Button size="small">导出证据</Button>
        <OverlayContractHost overlays={graphOverlays} compact />
      </div>

      <div className="taf-graph-kpis">
        {(data?.metrics ?? []).map((metric) => (
          <MetricTile key={metric.label} metric={metric} />
        ))}
      </div>

      <div className="taf-graph-grid">
        <main className="taf-graph-main">
          <WorkPanel
            title="邻居图谱"
            extra={
              <Space size={6}>
                <span className="taf-graph-query-stat">查询耗时 278 ms</span>
                <span className="taf-graph-query-stat">节点 10</span>
                <span className="taf-graph-query-stat">关系 12</span>
                <Button size="small" icon={<ExpandOutlined />} />
              </Space>
            }
          >
            <GraphCanvas />
          </WorkPanel>

          <div className="taf-graph-bottom">
            <WorkPanel title="路径分析结果">
              <Tabs
                className="taf-graph-tabs"
                activeKey={activePath}
                onChange={setActivePath}
                items={route.page.tabs.map((tab) => ({ key: tab, label: tab }))}
              />
              <div className="taf-graph-path-strip">
                <strong>{text(selectedRow, '源实体', '185.234.15.23')}</strong>
                <i />
                <strong>{text(selectedRow, '目标实体', 'DB-SRV-01')}</strong>
                <StatusTag value={text(selectedRow, '风险', '高危')} />
              </div>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={rows.slice(0, 3)}
                pagination={false}
                onRow={(record) => ({ onClick: () => setSelectedRowKey(rowKey(record)) })}
              />
            </WorkPanel>

            <WorkPanel title="查询治理">
              <QueryGovernance />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-graph-detail">
          <EntityDetail selectedRow={selectedRow} />
          <WorkPanel title="关联证据">
            <EvidenceList />
          </WorkPanel>
          <GraphActionRail />
        </aside>
      </div>
    </div>
  );
}

function GraphCanvas() {
  return (
    <div className="taf-graph-canvas">
      <div className="taf-graph-legend">
        {['IP地址', '主机', '账号', '域名', '服务', '告警', '证据'].map((item) => (
          <span key={item}>{item}</span>
        ))}
      </div>
      <svg className="taf-graph-lines" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
        {graphEdges.map(([source, target, label, tone]) => {
          const from = graphNodes.find((node) => node.id === source)!;
          const to = graphNodes.find((node) => node.id === target)!;
          return (
            <g key={`${source}-${target}`} className={`is-${tone}`}>
              <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} />
              <text x={(from.x + to.x) / 2} y={(from.y + to.y) / 2 - 1}>{label}</text>
            </g>
          );
        })}
      </svg>
      {graphNodes.map((node) => (
        <button key={node.id} className={`taf-graph-node is-${node.tone}`} style={{ left: `${node.x}%`, top: `${node.y}%` }} type="button">
          {node.icon}
          <strong>{node.label}</strong>
          <span>{node.detail}</span>
        </button>
      ))}
    </div>
  );
}

function EntityDetail({ selectedRow }: { selectedRow?: SnapshotRow }) {
  return (
    <WorkPanel title="实体详情" extra={<Button size="small" type="text">关闭</Button>}>
      <div className="taf-graph-entity-head">
        <NodeIndexOutlined />
        <div>
          <strong>{text(selectedRow, '目标实体', '核心业务服务器')}</strong>
          <span>10.20.4.18</span>
        </div>
        <b>85</b>
      </div>
      <dl className="taf-graph-facts">
        <dt>实体类型</dt>
        <dd>主机</dd>
        <dt>操作系统</dt>
        <dd>CentOS Linux 7.9</dd>
        <dt>资产分组</dt>
        <dd>核心业务区 / 服务器</dd>
        <dt>资产负责人</dt>
        <dd>张伟（信息化中心）</dd>
        <dt>开放服务</dt>
        <dd>TCP/22、TCP/80、TCP/443、TCP/3306</dd>
        <dt>最近活跃</dt>
        <dd>2026-06-25 09:43:18</dd>
      </dl>
    </WorkPanel>
  );
}

function QueryGovernance() {
  const stats = [
    ['慢查询数', '3', 'warn'],
    ['节点上限', '5,000', 'ok'],
    ['图缓存命中率', '92.7%', 'ok'],
    ['平均查询耗时', '278 ms', 'info'],
  ];
  const history = ['核心业务服务器关系分析', '可疑外部IP路径分析', 'biz_admin账号活动分析', 'erp.corp.edu.cn访问关系'];
  return (
    <div className="taf-graph-governance">
      <div className="taf-graph-governance__stats">
        {stats.map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`}>
            <strong>{value}</strong>
            <small>{label}</small>
          </span>
        ))}
      </div>
      <div className="taf-graph-history">
        {history.map((item, index) => (
          <div key={item}>
            <ClockCircleOutlined />
            <span>{item}</span>
            <em>{index === 0 ? '已通过' : '缓存命中'}</em>
          </div>
        ))}
      </div>
    </div>
  );
}

function EvidenceList() {
  const items = [
    ['PCAP-20260620-0156', '03:31:22'],
    ['SESSION-20260620-3381', '03:28:41'],
    ['ALERT-20260620-1287', '03:31:09'],
    ['AUDIT-GRAPH-0042', '03:32:01'],
  ];
  return (
    <div className="taf-graph-evidence">
      {items.map(([label, time]) => (
        <button key={label} type="button">
          <FileSearchOutlined />
          <span>{label}</span>
          <em>{time}</em>
        </button>
      ))}
    </div>
  );
}

function GraphActionRail() {
  return (
    <div className="taf-graph-action-rail">
      <Button type="primary">查看资产</Button>
      <Button type="primary">查看告警</Button>
      <Button type="primary">进入取证</Button>
      <Button>跳转攻击链</Button>
      <Button icon={<HistoryOutlined />}>审计日志</Button>
    </div>
  );
}

const renderGraphCell = (column: string, value: unknown) => {
  if (column === '路径 ID') return <span className="taf-graph-path-id">{String(value ?? '')}</span>;
  if (column === '风险') return <StatusTag value={value} />;
  if (column === '证据') return <span className="taf-graph-evidence-cell">{String(value ?? '-')}</span>;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record['路径 ID'] ?? JSON.stringify(record));

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};
