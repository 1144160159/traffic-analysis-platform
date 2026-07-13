import {
  ApartmentOutlined,
  BranchesOutlined,
  CloudServerOutlined,
  CloseOutlined,
  ClusterOutlined,
  DeploymentUnitOutlined,
  DownloadOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  ProfileOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  ShareAltOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, message, Pagination, Progress, Select, Space, Table, Tabs, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { Fragment } from 'react';
import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { AssetDetailWorkspace } from './AssetDetailWorkspace';
import {
  assetDetailTabs,
  assetSearchParams,
  assetTabs,
  canOpenAssetDetail,
  defaultAssetIdByTab,
  resolveAssetDetail,
  resolveAssetTab,
  type AssetDetailSlug,
  type AssetTabSlug,
} from './assetInventoryState';

const assetKpis: Record<AssetTabSlug, PageSnapshot['metrics']> = {
  endpoint: [
    { label: '已识别终端', value: '2,416', delta: '+38', status: 'ok' },
    { label: '在线终端', value: '2,231', delta: '+24', status: 'ok' },
    { label: '漂移终端', value: '87', delta: '+12', status: 'warn' },
    { label: '长期离线', value: '64', delta: '-8', status: 'info' },
    { label: '高风险终端', value: '31', delta: '+5', status: 'risk' },
  ],
  server: [
    { label: '服务器资产', value: '482', delta: '+12', status: 'ok' },
    { label: '在线服务器', value: '456', delta: '+9', status: 'ok' },
    { label: '暴露端口', value: '1,286', delta: '+73', status: 'warn' },
    { label: '高危服务', value: '73', delta: '+8', status: 'risk' },
    { label: '探针异常', value: '16', delta: '+3', status: 'warn' },
  ],
  'network-device': [
    { label: '网络设备', value: '326', delta: '+4', status: 'ok' },
    { label: '在线设备', value: '309', delta: '+3', status: 'ok' },
    { label: '镜像口', value: '72', delta: '+6', status: 'info' },
    { label: '异常链路', value: '19', delta: '+5', status: 'risk' },
    { label: '配置变更', value: '41', delta: '+11', status: 'warn' },
  ],
  'business-system': [
    { label: '业务系统', value: '146', delta: '+5', status: 'ok' },
    { label: '关键系统', value: '38', delta: '+2', status: 'info' },
    { label: '依赖资产', value: '1,284', delta: '+46', status: 'ok' },
    { label: '高风险系统', value: '21', delta: '+4', status: 'risk' },
    { label: 'SLA 临近', value: '14', delta: '+3', status: 'warn' },
  ],
  unknown: [
    { label: '未知资产', value: '128', delta: '+19', status: 'warn' },
    { label: '未归属 IP/MAC', value: '76', delta: '+12', status: 'risk' },
    { label: '临时主机', value: '34', delta: '+7', status: 'warn' },
    { label: '待确认责任人', value: '42', delta: '+9', status: 'warn' },
    { label: '已生成工单', value: '27', delta: '+6', status: 'ok' },
  ],
};

const filterLabels: Record<AssetTabSlug, string[]> = {
  endpoint: ['园区', '网段', '资产类型', '重要性', '风险等级', '最近活跃'],
  server: ['园区', '部门', '业务系统', '操作系统', '重要性', '风险等级', '探针状态', '最近活跃'],
  'network-device': ['园区', '楼宇', '设备类型', '厂商', '链路角色', '接口状态', '配置变更', '最近活跃'],
  'business-system': ['园区', '责任部门', '系统等级', '业务域', '数据域', '风险等级', 'SLA', '最近活跃'],
  unknown: ['园区', '网段', '来源', '疑似类型', '置信度', '风险等级', '工单状态', '最近发现'],
};

const serverRows = [
  ['SRV-0007', '10.12.4.12', '实验楼-SRV-12', '教学管理系统', '计算中心 / 实验楼', 'Ubuntu 22.04', '在线', '22,443,3306', '高', '高风险', '计算中心-资产岗'],
  ['SRV-0012', '10.12.6.18', 'K8S-NODE-02', '采集分析平台', '安全运营 / 主园区', 'openEuler 22.03', '在线', '6443,10250', '高', '中风险', '安全运营组'],
  ['SRV-0021', '10.14.8.42', 'FILE-SRV-09', '科研文件服务', '科研处 / 实验楼', 'CentOS 7.9', '降级', '22,80,443,9200', '高', '高风险', '科研数据岗'],
  ['SRV-0033', '10.9.3.22', 'AUTH-SRV-03', '统一身份认证', '信息化办 / 主园区', 'Windows Server', '在线', '443,389,636', '高', '中风险', '身份平台岗'],
];

const openServiceRows = [
  ['22', 'TCP', 'SSH / OpenSSH 7.4', '内网+运维网', '堡垒机+管理员', '弱口令疑似', '5', '生成工单'],
  ['443', 'TCP', 'HTTPS / Nginx TLS1.2', '外网暴露', 'Internet+办公区', '证书即将过期', '6', '进入取证'],
  ['3306', 'TCP', 'MySQL 8.0.32', '应用网', '应用服务器', '高危端口', '4', '查看告警'],
  ['9200', 'TCP', 'OpenSearch 2.8', '管理网', '运维网段', '版本过期', '1', '查看漏洞'],
];

const assetOverlays: OverlayContract[] = [
  {
    id: 'modal-asset-edit',
    title: '编辑资产',
    kind: 'Modal',
    actionLabel: '编辑资产',
    description: '编辑资产责任人、业务系统、重要性、标签和维护状态。',
    impact: '影响资产画像、告警归属、图谱关联和合规责任边界。',
    audit: '记录资产字段变更前后值、操作者和审批上下文。',
  },
];

const networkRows = [
  ['NET-0001', '10.1.0.1', '核心交换机-CSW-01', '核心交换机', 'H3C S6850', '主园区 / 核心机房', '核心汇聚', '96', '8', 'v20260620', '在线', '高风险'],
  ['NET-0018', '10.2.8.1', '接入交换机-ASW-18', '接入交换机', 'Huawei S5735', '教学楼 A', '接入层', '48', '2', 'v20260618', '在线', '低风险'],
  ['NET-0042', '10.1.0.254', '边界路由-BR-01', '路由器', 'Cisco ASR', '出口区', '边界出口', '32', '0', 'v20260612', '异常', '中风险'],
  ['NET-0056', '10.1.1.1', '园区防火墙-FW-01', '防火墙', 'Sangfor AF', '出口区', '安全边界', '24', '0', 'v20260617', '在线', '中风险'],
];

const mirrorRows = [
  ['Mirror-01', 'GE0/1/48', 'probe-edge-01', 'VLAN 120', '38.6 Gbps', '0.02%', '18 秒前', '健康'],
  ['Mirror-02', '10GE1/2', 'probe-core-02', 'VLAN 200', '61.4 Gbps', '0.08%', '24 秒前', '告警'],
  ['SPAN-DB', 'GE0/2/12', 'probe-db-01', 'VLAN 330', '9.8 Gbps', '0.01%', '12 秒前', '健康'],
];

const businessRows = [
  ['BIZ-0001', '教学管理系统', '教学业务', '教务处', '教学应用组', '关键', '186', '34', '86', '19', '临近', '依赖图'],
  ['BIZ-0014', '科研数据平台', '科研业务', '科研处', '科研数据岗', '关键', '214', '46', '78', '12', '正常', '风险聚合'],
  ['BIZ-0020', '统一身份认证', '基础平台', '信息化办', '身份平台岗', '关键', '93', '18', '82', '16', '临近', '生成工单'],
  ['BIZ-0032', '视频监控平台', '安防业务', '保卫处', '安防运维组', '重要', '152', '22', '69', '7', '正常', '依赖图'],
];

const keyServiceRows = [
  ['SSO API', 'HTTPS/443', 'AUTH-SRV-03', 'v2.8', '校内+VPN', '48.2K', '16', '降级', '进入取证'],
  ['教务 Web', 'HTTPS/443', 'SRV-0007', 'Nginx 1.20', '园区网', '86.4K', '19', '告警', '查看告警'],
  ['MySQL', 'TCP/3306', 'DB-SRV-07', '8.0.32', '应用网', '18.6K', '4', '健康', '生成工单'],
  ['Kafka', 'TCP/9092', 'K8S-NODE-02', '3.7', '平台内网', '128.7K', '1', '健康', '跳转图谱'],
];

const unknownRows = [
  ['UNK-10.12.88.45', '2026-06-26 09:12', '10.12.88.45 / 3C:2A:F4:91:28:10', 'LAPTOP-8845', 'DHCP+Flow', '教学区 / VLAN 88', 'Win DHCP + TLS', '临时主机', '高风险', '87%', '确认归属'],
  ['UNK-10.9.44.21', '2026-06-26 08:47', '10.9.44.21 / B8:27:EB:14:66:7A', 'raspberrypi', 'ARP+DNS', '实验楼 / IoT 网段', 'Linux TTL + mDNS', '未识别设备', '中风险', '76%', '创建工单'],
  ['UNK-10.5.17.90', '2026-06-26 08:06', '10.5.17.90 / 00:1B:A9:10:17:90', 'HP-PRN-1790', 'Probe', '办公区', 'JetDirect 9100', '打印设备候选', '低风险', '69%', '加入观察'],
  ['UNK-10.7.66.13', '2026-06-26 07:35', '10.7.66.13 / E8:2A:EA:66:13:01', 'camera-6613', 'DHCP+Flow', '安防网段', 'RTSP + ONVIF', '摄像头候选', '中风险', '72%', '请求确认'],
];

const candidateRows = [
  ['教务处', '教学终端组', '教学管理系统', '教学应用组', 'DHCP 网段 + TLS SNI', '87%', '确认'],
  ['实验中心', 'IoT 设备组', '实验传感平台', '实验室管理员', 'mDNS + MAC OUI', '76%', '请求确认'],
  ['保卫处', '安防摄像组', '视频监控平台', '安防运维组', 'RTSP + ONVIF', '72%', '排除'],
];

type AssetSelectionContext = {
  tab: AssetTabSlug;
  id: string;
  name: string;
  ip: string;
  type: string;
  location: string;
  os: string;
  importance: string;
  risk: string;
  status: string;
  owner: string;
  ports: string;
  sourceRow?: SnapshotRow;
};

const contextFallbacks: Record<AssetTabSlug, Omit<AssetSelectionContext, 'tab' | 'sourceRow'>> = {
  endpoint: {
    id: 'PC-0082', name: '实验楼-PC-0082', ip: '10.12.8.82 / 00:50:56:AA:08:82', type: '终端',
    location: '实验楼 / 计算中心', os: 'Windows 11', importance: '高', risk: '中风险', status: '在线',
    owner: '计算中心-终端岗', ports: '3',
  },
  server: {
    id: 'SRV-0007', name: '实验楼-SRV-12', ip: '10.12.4.12', type: '服务器',
    location: '计算中心 / 实验楼', os: 'Ubuntu 22.04', importance: '高', risk: '高风险', status: '在线',
    owner: '计算中心-资产岗', ports: '22,443,3306',
  },
  'network-device': {
    id: 'NET-0001', name: '核心交换机-CSW-01', ip: '10.1.0.1', type: '核心交换机',
    location: '主园区 / 核心机房', os: 'H3C S6850', importance: '高', risk: '高风险', status: '在线',
    owner: '网络运维组', ports: '96 个接口 / 8 个镜像口',
  },
  'business-system': {
    id: 'BIZ-0001', name: '教学管理系统', ip: '186 个依赖资产', type: '关键业务系统',
    location: '教务处 / 教学业务', os: '34 个关键服务', importance: '关键', risk: '高风险', status: 'SLA 临近',
    owner: '教学应用组', ports: '19 个关联告警',
  },
  unknown: {
    id: 'UNK-10.12.88.45', name: 'LAPTOP-8845', ip: '10.12.88.45 / 3C:2A:F4:91:28:10', type: '临时主机',
    location: '教学区 / VLAN 88', os: 'Win DHCP + TLS', importance: '待确认', risk: '高风险', status: '待确认',
    owner: '候选：教务处', ports: '置信度 87%',
  },
};

function buildSelectionContext(tab: AssetTabSlug, assetId: string, endpointRow?: SnapshotRow): AssetSelectionContext {
  const fallback = contextFallbacks[tab];
  if (tab === 'endpoint') {
    return {
      ...fallback,
      tab,
      id: assetId,
      name: text(endpointRow, '主机名', fallback.name),
      ip: text(endpointRow, 'IP/MAC', fallback.ip),
      type: text(endpointRow, '类型', fallback.type),
      location: text(endpointRow, '园区/部门', fallback.location),
      os: text(endpointRow, '操作系统', fallback.os),
      importance: text(endpointRow, '重要性', fallback.importance),
      risk: riskLevel(endpointRow) === '低风险' && !endpointRow ? fallback.risk : riskLevel(endpointRow),
      ports: text(endpointRow, '暴露端口', fallback.ports),
      sourceRow: endpointRow,
    };
  }
  const source = tab === 'server'
    ? serverRows.find((row) => row[0] === assetId)
    : tab === 'network-device'
      ? networkRows.find((row) => row[0] === assetId)
      : tab === 'business-system'
        ? businessRows.find((row) => row[0] === assetId)
        : unknownRows.find((row) => row[0] === assetId);
  if (!source) return { ...fallback, tab, id: assetId };
  if (tab === 'server') return {
    tab, id: source[0], ip: source[1], name: source[2], type: '服务器', location: source[4], os: source[5],
    status: source[6], ports: source[7], importance: source[8], risk: source[9], owner: source[10],
  };
  if (tab === 'network-device') return {
    tab, id: source[0], ip: source[1], name: source[2], type: source[3], os: `${source[4]} / ${source[9]}`,
    location: source[5], ports: `${source[7]} 个接口 / ${source[8]} 个镜像口`, status: source[10], risk: source[11],
    importance: source[6], owner: '网络运维组',
  };
  if (tab === 'business-system') return {
    tab, id: source[0], name: source[1], type: source[2], location: `${source[3]} / ${source[2]}`, owner: source[4],
    importance: source[5], ip: `${source[6]} 个依赖资产`, os: `${source[7]} 个关键服务`, risk: `${source[8]} 分`,
    status: `SLA ${source[10]}`, ports: `${source[9]} 个关联告警`,
  };
  return {
    tab, id: source[0], name: source[3], ip: source[2], type: source[7], location: source[5], os: source[6],
    importance: '待确认', risk: source[8], status: '待确认', owner: '候选：教务处', ports: `置信度 ${source[9]}`,
  };
}

const notifyAssetAction = (label: string) => void message.success(`${label}已触发，处理结果将写入资产审计。`);

export function AssetInventoryPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = resolveAssetTab(searchParams.get('tab'));
  const activeDetail = resolveAssetDetail(searchParams.get('detail'));
  const selectedAssetId = searchParams.get('assetId') || defaultAssetIdByTab[activeTab];
  const [filtersCollapsed, setFiltersCollapsed] = useState(false);
  const [detailRailVisible, setDetailRailVisible] = useState(true);
  const [actionFeedback, setActionFeedback] = useState<string>();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedRow = useMemo(() => {
    if (!rows.length) return undefined;
    return rows.find((row) => rowKey(row) === selectedAssetId);
  }, [rows, selectedAssetId]);
  const selectedContext = useMemo(
    () => buildSelectionContext(activeTab, selectedAssetId, selectedRow),
    [activeTab, selectedAssetId, selectedRow],
  );

  const selectAsset = (assetId: string) => setSearchParams(assetSearchParams({ tab: activeTab, assetId }));
  const openDetail = (detail: AssetDetailSlug = 'basic') => {
    if (!canOpenAssetDetail(activeTab, selectedAssetId)) {
      setActionFeedback('当前分类不使用服务器详情页，请使用该分类的业务操作。');
      return;
    }
    setSearchParams(assetSearchParams({ tab: activeTab, assetId: selectedAssetId, detail }));
  };
  const closeDetail = () => setSearchParams(assetSearchParams({ tab: activeTab, assetId: selectedAssetId }));

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderAssetCell(column, value),
  }));

  return (
    <div className={`taf-page taf-asset-inventory taf-asset-tab-${activeTab}`}>
      <header className="taf-asset-titlebar">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Button size="small" onClick={() => setFiltersCollapsed((value) => !value)}>{filtersCollapsed ? '展开' : '收起'}</Button>
          {!detailRailVisible && <Button size="small" icon={<ProfileOutlined />} onClick={() => setDetailRailVisible(true)}>显示摘要</Button>}
          <Tooltip title="刷新资产台账">
            <Button icon={<ReloadOutlined />} size="small" onClick={() => void refetch()} />
          </Tooltip>
          <OverlayContractHost overlays={assetOverlays} compact />
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/assets、APISIX 路由、后端服务、鉴权或网络连通性。'}
          action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
        />
      )}
      {actionFeedback && <Alert closable showIcon type="success" message={actionFeedback} onClose={() => setActionFeedback(undefined)} />}

      <div className={`taf-asset-grid${detailRailVisible ? '' : ' is-detail-hidden'}`}>
        <main className="taf-asset-main">
          <WorkPanel title="资产台账">
            <Tabs
              className="taf-asset-tabs"
              activeKey={activeTab}
              onChange={(key) => {
                const tab = resolveAssetTab(key);
                setSearchParams(assetSearchParams({ tab, assetId: defaultAssetIdByTab[tab] }));
              }}
              items={assetTabs.map((tab) => ({ key: tab.slug, label: tab.label }))}
            />
            {!filtersCollapsed && <AssetFilter activeTab={activeTab} onFeedback={setActionFeedback} />}
            {!filtersCollapsed && (
              <div className="taf-asset-kpis">
                {assetKpis[activeTab].map((metric) => <MetricTile key={metric.label} metric={metric} />)}
              </div>
            )}
          </WorkPanel>

          <AssetTabContent
            activeTab={activeTab}
            columns={columns}
            isLoading={isLoading}
            rows={rows}
            selectedRow={selectedRow}
            selectedAssetId={selectedAssetId}
            selectAsset={selectAsset}
          />
        </main>

        {detailRailVisible && <aside className="taf-asset-detail">
          <AssetDetailCard context={selectedContext} onClose={() => setDetailRailVisible(false)} />
          <RiskSummary context={selectedContext} />
          {activeTab === 'server' && <AssetTabsDetail context={selectedContext} onOpenDetail={openDetail} />}
          <AssetActionRail activeTab={activeTab} onOpenDetail={openDetail} onFeedback={setActionFeedback} />
        </aside>}
      </div>
      <Drawer
        className="taf-asset-detail-drawer"
        title={null}
        placement="right"
        width={activeDetail === 'basic' || activeDetail === 'ownership' ? 1040 : 1280}
        open={Boolean(activeDetail && canOpenAssetDetail(activeTab, selectedAssetId))}
        onClose={closeDetail}
        destroyOnClose
        maskClosable
        styles={{ body: { padding: 0 } }}
      >
        {activeDetail && (
          <AssetDetailWorkspace
            assetId={selectedAssetId}
            detail={activeDetail}
            onClose={closeDetail}
            onDetailChange={(detail) => setSearchParams(assetSearchParams({ tab: 'server', assetId: selectedAssetId, detail }))}
          />
        )}
      </Drawer>
    </div>
  );
}

function AssetFilter({ activeTab, onFeedback }: { activeTab: AssetTabSlug; onFeedback: (message: string) => void }) {
  const [filters, setFilters] = useState<Record<string, string>>({});
  const reset = () => {
    setFilters({});
    onFeedback('资产筛选条件已重置。');
  };
  return (
    <div className="taf-asset-filter">
      {filterLabels[activeTab].map((label) => (
        <label key={label}>
          <span>{label}</span>
          <Select
            size="small"
            value={filters[label] ?? '全部'}
            onChange={(value) => setFilters((current) => ({ ...current, [label]: value }))}
            options={[{ value: '全部' }, { value: '高风险' }, { value: '近 24 小时' }]}
          />
        </label>
      ))}
      <div className="taf-asset-filter__actions">
        <Button size="small" onClick={reset}>重置</Button>
        <Button size="small" type="primary" onClick={() => onFeedback(`已应用 ${Object.values(filters).filter((value) => value !== '全部').length} 个筛选条件。`)}>查询</Button>
        <Button size="small" icon={<DeploymentUnitOutlined />} onClick={() => onFeedback('批量更新面板已就绪，请先选择资产。')}>批量更新</Button>
        <Button size="small" icon={<DownloadOutlined />} onClick={() => onFeedback(`正在导出${assetTabs.find((item) => item.slug === activeTab)?.label ?? ''}资产清单。`)}>导出</Button>
      </div>
    </div>
  );
}

function AssetTabContent({
  activeTab,
  columns,
  isLoading,
  rows,
  selectedRow,
  selectedAssetId,
  selectAsset,
}: {
  activeTab: AssetTabSlug;
  columns: ColumnsType<SnapshotRow>;
  isLoading: boolean;
  rows: SnapshotRow[];
  selectedRow?: SnapshotRow;
  selectedAssetId: string;
  selectAsset: (assetId: string) => void;
}) {
  if (activeTab === 'server') return <ServerContent selectedAssetId={selectedAssetId} selectAsset={selectAsset} />;
  if (activeTab === 'network-device') return <NetworkDeviceContent selectedAssetId={selectedAssetId} selectAsset={selectAsset} />;
  if (activeTab === 'business-system') return <BusinessSystemContent selectedAssetId={selectedAssetId} selectAsset={selectAsset} />;
  if (activeTab === 'unknown') return <UnknownAssetContent selectedAssetId={selectedAssetId} selectAsset={selectAsset} />;
  return (
    <EndpointContent
      columns={columns}
      isLoading={isLoading}
      rows={rows}
      selectedRow={selectedRow}
      selectAsset={selectAsset}
    />
  );
}

function EndpointContent({
  columns,
  isLoading,
  rows,
  selectedRow,
  selectAsset,
}: {
  columns: ColumnsType<SnapshotRow>;
  isLoading: boolean;
  rows: SnapshotRow[];
  selectedRow?: SnapshotRow;
  selectAsset: (assetId: string) => void;
}) {
  return (
    <>
      <WorkPanel
        title="资产台账（终端）"
        extra={<Space><Button size="small" icon={<EditOutlined />} onClick={() => notifyAssetAction('新增资产')}>新增资产</Button><Button size="small" icon={<DownloadOutlined />} onClick={() => notifyAssetAction('导出终端资产')}>导出</Button></Space>}
      >
        <Table
          rowKey={rowKey}
          size="small"
          loading={isLoading}
          columns={columns}
          dataSource={rows}
          pagination={{ pageSize: 5, size: 'small' }}
          rowSelection={{ selectedRowKeys: selectedRow ? [rowKey(selectedRow)] : [] }}
          onRow={(record) => ({ onClick: () => selectAsset(rowKey(record)) })}
        />
      </WorkPanel>
      <div className="taf-asset-observability">
        <WorkPanel title={`流量画像（${text(selectedRow, '主机名', '选中终端')}）`}><TrafficProfile /></WorkPanel>
        <WorkPanel title="协议分布"><ProtocolMix /></WorkPanel>
        <WorkPanel title="Top 对端（按流量）"><PeerRanking /></WorkPanel>
        <WorkPanel title="周期性连接（最近 7 天）"><PeriodicHeatmap /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文"><EvidenceStrip selectedRow={selectedRow} /></WorkPanel>
    </>
  );
}

function ServerContent({ selectedAssetId, selectAsset }: { selectedAssetId: string; selectAsset: (assetId: string) => void }) {
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title="服务器资产清单" className="taf-asset-wide">
          <AssetDenseRows
            columns={['资产ID', 'IP/MAC', '主机名', '业务系统', '部门/园区', '操作系统', '探针状态', '开放端口', '重要性', '风险标签', '负责人']}
            rows={serverRows}
            selectedKey={selectedAssetId}
            onSelect={selectAsset}
          />
        </WorkPanel>
        <WorkPanel title="服务端口风险矩阵"><AssetPortMatrix /></WorkPanel>
        <WorkPanel title="服务拓扑"><AssetMiniTopology mode="server" /></WorkPanel>
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="开放服务明细"><AssetDenseRows columns={['端口', '协议', '服务/版本', '暴露范围', '访问来源', '风险标签', '告警', '处置']} rows={openServiceRows} /></WorkPanel>
        <WorkPanel title="OS / 探针状态"><OsProbeState /></WorkPanel>
        <WorkPanel title="业务归属与负责人"><OwnershipCards mode="server" /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文"><EvidenceStrip /></WorkPanel>
    </>
  );
}

function NetworkDeviceContent({ selectedAssetId, selectAsset }: { selectedAssetId: string; selectAsset: (assetId: string) => void }) {
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title="网络设备清单" className="taf-asset-wide">
          <AssetDenseRows
            columns={['设备ID', '管理IP', '设备名', '类型', '厂商/型号', '园区/楼宇', '链路角色', '接口数', '镜像口', '配置版本', '状态', '风险标签']}
            rows={networkRows}
            selectedKey={selectedAssetId}
            onSelect={selectAsset}
          />
        </WorkPanel>
        <WorkPanel title="接口状态矩阵"><InterfaceMatrix /></WorkPanel>
        <WorkPanel title="链路拓扑"><AssetMiniTopology mode="network" /></WorkPanel>
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="镜像口与采集链路"><AssetDenseRows columns={['镜像口', '源接口', '目的探针', '采集 VLAN', '流量', '丢包', '最近心跳', '状态']} rows={mirrorRows} /></WorkPanel>
        <WorkPanel title="配置变更入口"><AssetTimeline /></WorkPanel>
        <WorkPanel title="链路归属与业务影响"><OwnershipCards mode="network" /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文"><EvidenceStrip /></WorkPanel>
    </>
  );
}

function BusinessSystemContent({ selectedAssetId, selectAsset }: { selectedAssetId: string; selectAsset: (assetId: string) => void }) {
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title="业务系统表" className="taf-asset-wide">
          <AssetDenseRows
            columns={['系统ID', '系统名称', '业务域', '责任部门', '负责人', '关键等级', '依赖资产', '关键服务', '风险评分', '最近告警', 'SLA', '操作']}
            rows={businessRows}
            selectedKey={selectedAssetId}
            onSelect={selectAsset}
          />
        </WorkPanel>
        <WorkPanel title="依赖关系图"><AssetMiniTopology mode="business" /></WorkPanel>
        <WorkPanel title="风险评分"><BusinessRiskScore /></WorkPanel>
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="关键服务清单"><AssetDenseRows columns={['服务名', '协议/端口', '承载资产', '版本', '暴露范围', '调用量', '告警', '健康', '处置']} rows={keyServiceRows} /></WorkPanel>
        <WorkPanel title="依赖资产与健康"><DependencyHealth /></WorkPanel>
        <WorkPanel title="责任部门与 SLA"><OwnershipCards mode="business" /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文"><EvidenceStrip /></WorkPanel>
    </>
  );
}

function UnknownAssetContent({ selectedAssetId, selectAsset }: { selectedAssetId: string; selectAsset: (assetId: string) => void }) {
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title="未知资产队列" className="taf-asset-wide">
          <AssetDenseRows
            columns={['资产ID', '发现时间', 'IP/MAC', '临时主机名', '探针/来源', '园区/网段', '指纹特征', '疑似类型', '风险标签', '置信度', '处置']}
            rows={unknownRows}
            selectedKey={selectedAssetId}
            onSelect={selectAsset}
          />
        </WorkPanel>
        <WorkPanel title="发现时间线"><DiscoveryTimeline /></WorkPanel>
        <WorkPanel title="未识别设备画像"><UnknownFingerprint /></WorkPanel>
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="归属候选与匹配"><AssetDenseRows columns={['部门', '资产组', '业务系统', '负责人', '匹配依据', '置信度', '操作']} rows={candidateRows} /></WorkPanel>
        <WorkPanel title="风险与暴露面"><ExposureCards /></WorkPanel>
        <WorkPanel title="工单与处置闭环"><TicketClosure /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文"><EvidenceStrip /></WorkPanel>
    </>
  );
}

function AssetDenseRows({
  columns,
  rows,
  selectedKey,
  onSelect,
}: {
  columns: string[];
  rows: string[][];
  selectedKey?: string;
  onSelect?: (key: string) => void;
}) {
  const [page, setPage] = useState(1);
  const pageSize = 5;
  const visibleRows = rows.slice((page - 1) * pageSize, page * pageSize);
  return (
    <div className="taf-asset-dense" style={{ '--asset-cols': `repeat(${columns.length}, minmax(80px, 1fr))`, '--asset-cols-count': columns.length } as CSSProperties}>
      <div className="taf-asset-dense__head">
        {columns.map((column) => <span key={column}>{column}</span>)}
      </div>
      {visibleRows.map((row, rowIndex) => (
        <div
          key={`${row[0]}-${rowIndex}`}
          className={`taf-asset-dense__row${selectedKey === row[0] ? ' is-selected' : ''}`}
          role={onSelect ? 'button' : undefined}
          tabIndex={onSelect ? 0 : undefined}
          onClick={() => onSelect?.(row[0])}
          onKeyDown={(event) => {
            if (onSelect && (event.key === 'Enter' || event.key === ' ')) onSelect(row[0]);
          }}
        >
          {row.map((cell, cellIndex) => {
            const statusLike = /高危|高风险|中风险|低风险|告警|异常|降级|在线|健康|待确认|临近/.test(cell);
            return <span key={`${cell}-${cellIndex}`}>{statusLike ? <StatusTag value={cell} /> : cell}</span>;
          })}
        </div>
      ))}
      <div className="taf-asset-dense__pagination">
        <Pagination current={page} pageSize={pageSize} total={rows.length} size="small" showSizeChanger={false} onChange={setPage} />
        <span>共 {rows.length} 条</span>
      </div>
    </div>
  );
}

function AssetPortMatrix() {
  const ports = ['22', '80', '443', '3306', '5432', '6379', '9200', '9092', '30000'];
  const groups = ['Web', 'DB', 'K8s', 'File', 'Auth'];
  return (
    <div className="taf-asset-matrix">
      <span />
      {ports.map((port) => <b key={port}>{port}</b>)}
      {groups.map((group, rowIndex) => (
        <div key={group} className="taf-asset-matrix__row">
          <strong>{group}</strong>
          {ports.map((port, index) => <i key={port} className={`is-${(index + rowIndex) % 5 === 0 ? 'risk' : (index + rowIndex) % 4 === 0 ? 'warn' : (index + rowIndex) % 3 === 0 ? 'info' : 'ok'}`} />)}
        </div>
      ))}
    </div>
  );
}

function InterfaceMatrix() {
  const interfaces = ['GE0/1', 'GE0/2', 'GE0/3', '10GE1/1', 'Mirror-1', 'WAN', 'HA'];
  return (
    <div className="taf-asset-interface-matrix">
      {['CSW-01', 'ASW-18', 'BR-01', 'FW-01'].map((device, rowIndex) => (
        <div key={device}>
          <strong>{device}</strong>
          {interfaces.map((item, index) => <span key={item} className={`is-${(rowIndex + index) % 6 === 0 ? 'down' : item.includes('Mirror') ? 'mirror' : (rowIndex + index) % 4 === 0 ? 'warn' : 'up'}`}>{item}</span>)}
        </div>
      ))}
    </div>
  );
}

function AssetMiniTopology({ mode }: { mode: 'server' | 'network' | 'business' }) {
  const center = mode === 'network' ? '核心交换机' : mode === 'business' ? '教学管理系统' : '核心数据库';
  const nodes = mode === 'business'
    ? ['Web/API', '数据库', 'Redis', 'Kafka', 'MinIO', '告警', '证据']
    : mode === 'network'
      ? ['汇聚交换机', '接入交换机', '防火墙', '探针', '镜像口', '出口路由', '服务器网段']
      : ['业务系统', 'API 服务', '数据库', '探针', '告警', '实体图谱', '取证'];
  return (
    <div className="taf-asset-topology">
      <div className="taf-asset-topology__center"><ClusterOutlined /><strong>{center}</strong></div>
      {nodes.map((node, index) => <span key={node} className={`node-${index}`}>{node}</span>)}
      <Button size="small" type="link" icon={<BranchesOutlined />} onClick={() => notifyAssetAction('跳转实体图谱')}>跳转实体图谱</Button>
    </div>
  );
}

function OsProbeState() {
  const items = [
    ['Ubuntu 22.04', '148', '健康'],
    ['openEuler', '96', '健康'],
    ['CentOS 7', '74', '待升级'],
    ['Windows Server', '52', '中风险'],
    ['Probe v2.6', '456', '在线'],
    ['丢包 >0.1%', '16', '告警'],
  ];
  return (
    <div className="taf-asset-state-cards">
      {items.map(([label, value, status]) => (
        <div key={label}>
          <strong>{value}</strong>
          <span>{label}</span>
          <StatusTag value={status} />
        </div>
      ))}
    </div>
  );
}

function AssetTimeline() {
  const items = [
    ['v20260620', '核心交换机 ACL 调整', '安全复核中'],
    ['v20260618', '新增 Mirror-02 采集路径', '已通过'],
    ['v20260617', '出口路由策略变更', '待复核'],
    ['v20260612', '防火墙对象组同步', '已归档'],
  ];
  return (
    <div className="taf-asset-timeline">
      {items.map(([time, detail, status]) => (
        <div key={time}>
          <i />
          <strong>{time}</strong>
          <span>{detail}</span>
          <StatusTag value={status} />
        </div>
      ))}
    </div>
  );
}

function OwnershipCards({ mode }: { mode: 'server' | 'network' | 'business' }) {
  const base = mode === 'network'
    ? [['园区', '主园区'], ['楼宇', '核心机房'], ['链路角色', '核心汇聚'], ['负责人', '网络运维组'], ['SLA', '99.95%']]
    : mode === 'business'
      ? [['责任部门', '教务处'], ['负责人', '教学应用组'], ['数据域', '教学业务域'], ['SLA', '临近超时'], ['最近变更', '09:28']]
      : [['业务系统', '教学管理系统'], ['责任部门', '计算中心'], ['负责人', '计算中心-资产岗'], ['数据域', '运维日志域'], ['SLA', '99.9%']];
  return (
    <div className="taf-asset-owner-cards">
      {base.map(([label, value]) => (
        <div key={label}>
          <span>{label}</span>
          <strong>{value}</strong>
        </div>
      ))}
    </div>
  );
}

function BusinessRiskScore() {
  return (
    <div className="taf-asset-business-risk">
      <Progress type="circle" size={96} percent={86} format={() => 86} strokeColor="#ff4d4f" />
      {['漏洞暴露 28%', '异常外联 24%', '弱口令 18%', '证据缺口 16%', 'SLA 风险 14%'].map((item, index) => (
        <div key={item}><span>{item}</span><i style={{ width: `${72 - index * 8}%` }} /></div>
      ))}
    </div>
  );
}

function DependencyHealth() {
  const items = [
    ['服务器', '186 / 9 降级'],
    ['网络设备', '42 / 2 异常'],
    ['终端', '824 / 31 风险'],
    ['数据库', '18 / 4 高危'],
    ['探针', '12 / 1 离线'],
    ['存储', '9 / 健康'],
  ];
  return (
    <div className="taf-asset-dependency-health">
      {items.map(([label, value]) => <div key={label}><CloudServerOutlined /><span>{label}</span><strong>{value}</strong></div>)}
    </div>
  );
}

function DiscoveryTimeline() {
  return (
    <div className="taf-asset-discovery">
      {['DHCP 首次发现', 'ARP 绑定', 'DNS Query', 'Flow TLS', 'Probe 确认', '工单创建'].map((item, index) => (
        <div key={item} className={index > 3 ? 'is-warn' : 'is-ok'}>
          <i />
          <strong>{item}</strong>
          <span>{`0${index + 7}:1${index}`}</span>
        </div>
      ))}
    </div>
  );
}

function UnknownFingerprint() {
  const items = [
    ['MAC OUI', '3C:2A:F4 / Dell'],
    ['DHCP 主机名', 'LAPTOP-8845'],
    ['TTL / OS', '128 / Windows'],
    ['开放端口', '135, 445, 3389'],
    ['TLS / SNI', 'login.example.edu'],
    ['DNS Query', 'cdn.office.net'],
  ];
  return (
    <div className="taf-asset-fingerprint">
      {items.map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong></div>)}
    </div>
  );
}

function ExposureCards() {
  return (
    <div className="taf-asset-exposure-cards">
      {[
        ['暴露端口', '4', 'risk'],
        ['异常外联', '7', 'warn'],
        ['弱口令疑似', '2', 'risk'],
        ['未安装探针', '1', 'warn'],
        ['长期在线', '36h', 'info'],
        ['策略缺口', '3', 'warn'],
      ].map(([label, value, tone]) => <div key={label} className={`is-${tone}`}><strong>{value}</strong><span>{label}</span></div>)}
    </div>
  );
}

function TicketClosure() {
  return (
    <div className="taf-asset-ticket">
      {['归属确认 / 教学应用组 / 2h', '风险复核 / 安全运营 / 4h', '探针安装 / 终端运维 / 1d'].map((item) => <span key={item}>{item}</span>)}
      <Button size="small" type="primary" icon={<ThunderboltOutlined />} onClick={() => notifyAssetAction('生成归属工单')}>生成归属工单</Button>
      <Button size="small" onClick={() => notifyAssetAction('批量确认')}>批量确认</Button>
    </div>
  );
}

function TrafficProfile() {
  const bars = [28, 24, 31, 22, 26, 29, 24, 34, 20, 27, 31, 25];
  return (
    <div className="taf-asset-traffic">
      {bars.map((value, index) => <i key={index} style={{ height: `${value + 18}px` }}><b style={{ height: `${Math.max(10, value - 8)}px` }} /></i>)}
    </div>
  );
}

function ProtocolMix() {
  return (
    <div className="taf-asset-protocol">
      <Progress type="circle" size={90} percent={74} format={() => '286.7 Gbps'} />
      <div>{['TCP 58.6%', 'HTTP/HTTPS 22.4%', 'DNS 7.8%', 'MySQL 4.6%', 'SSH 2.3%'].map((item) => <span key={item}>{item}</span>)}</div>
    </div>
  );
}

function PeerRanking() {
  const peers = [['10.12.6.15', '服务器', 18], ['10.12.4.7', '服务器', 13], ['172.16.1.20', '业务系统', 10], ['8.8.8.8', 'DNS', 4]];
  return (
    <div className="taf-asset-peer-list">
      {peers.map(([ip, type, value]) => <div key={ip} className="taf-asset-peer-row"><span>{ip}</span><em>{type}</em><strong>{value}%</strong></div>)}
    </div>
  );
}

function PeriodicHeatmap() {
  return (
    <div className="taf-asset-heatmap">
      {['周一', '周二', '周三', '周四', '周五', '周六', '周日'].map((day, dayIndex) => (
        <div key={day}>
          <span>{day}</span>
          {Array.from({ length: 24 }, (_, index) => <i key={index} className={(index + dayIndex) % 5 === 0 ? 'is-hot' : 'is-cold'} />)}
        </div>
      ))}
    </div>
  );
}

function EvidenceStrip({ selectedRow }: { selectedRow?: SnapshotRow }) {
  const items = ['关联告警', 'PCAP 证据', 'Session 记录', 'DNS 日志', 'TLS 会话', '变更审计'];
  return (
    <div className="taf-asset-evidence-strip">
      {items.map((label, index) => (
        <button key={label} type="button" onClick={() => notifyAssetAction(`打开${label}`)}>
          <ShareAltOutlined />
          <span>{label}</span>
          <strong>{[12, 23, 186, 342, 97, 28][index]} 条</strong>
          <small>{text(selectedRow, '主机名', '当前资产')}</small>
        </button>
      ))}
    </div>
  );
}

function AssetDetailCard({ context, onClose }: { context: AssetSelectionContext; onClose: () => void }) {
  const { tab: activeTab } = context;
  const drawerTitle = {
    endpoint: '资产详情',
    server: '服务器详情',
    'network-device': '网络设备详情',
    'business-system': '业务系统详情',
    unknown: '归属确认抽屉',
  }[activeTab];
  const facts = {
    endpoint: [
      ['资产 ID', context.id], ['类型', context.type], ['IP/MAC', context.ip], ['园区/部门', context.location],
      ['发现任务', text(context.sourceRow, '__discoveryRunId', '待接入')],
      ['LLDP 邻居', text(context.sourceRow, '__topologyNeighbor', '待接入')],
    ],
    server: [['资产ID', context.id], ['IP', context.ip], ['操作系统', context.os], ['业务系统', '教学管理系统'], ['负责人', context.owner], ['开放端口', context.ports]],
    'network-device': [['设备ID', context.id], ['管理IP', context.ip], ['设备类型', context.type], ['园区/楼宇', context.location], ['厂商/版本', context.os], ['接口/镜像口', context.ports]],
    'business-system': [['系统ID', context.id], ['业务域', context.type], ['责任归属', context.location], ['负责人', context.owner], ['运行状态', context.status], ['资产/服务', `${context.ip} / ${context.os}`]],
    unknown: [['资产ID', context.id], ['IP/MAC', context.ip], ['临时主机名', context.name], ['发现特征', context.os], ['候选部门', context.owner], ['识别置信度', context.ports]],
  }[activeTab];

  return (
    <WorkPanel title={drawerTitle} extra={<Tooltip title="关闭摘要"><Button aria-label="关闭摘要" icon={<CloseOutlined />} size="small" type="text" onClick={onClose} /></Tooltip>}>
      <div className="taf-asset-detail-head">
        <strong>{context.name}</strong>
        <StatusTag value={context.risk} />
        {activeTab === 'endpoint' && <StatusTag value={text(context.sourceRow, '__discoveryRunStatus', '待接入')} />}
        <span className="is-online">{context.status}</span>
      </div>
      <dl className="taf-asset-facts">
        {facts.map(([label, value]) => (
          <Fragment key={label}>
            <dt>{label}</dt>
            <dd>{value}</dd>
          </Fragment>
        ))}
      </dl>
    </WorkPanel>
  );
}

function RiskSummary({ context }: { context: AssetSelectionContext }) {
  const { tab: activeTab, sourceRow: selectedRow } = context;
  const score = activeTab === 'unknown' ? 87 : context.risk.includes('高') || activeTab === 'server' || activeTab === 'business-system' ? 86 : 62;
  const items = activeTab === 'network-device'
    ? [['接口异常', '3', 'risk'], ['镜像口', '8', 'info'], ['配置变更', '11', 'warn'], ['关联告警', '7', 'warn']]
    : activeTab === 'business-system'
      ? [['依赖资产', '186', 'info'], ['关键服务', '34', 'warn'], ['关联告警', '19', 'risk'], ['证据缺口', '2', 'warn']]
      : activeTab === 'unknown'
        ? [['会话数', '1,286', 'info'], ['外联目的地', '18', 'warn'], ['开放端口', '4', 'risk'], ['候选归属', '3', 'info']]
        : [
            ['暴露端口', numberText(selectedRow, '暴露端口', 8), 'risk'],
            ['弱口令疑似', '2', 'warn'],
            ['拓扑邻居', numberText(selectedRow, '__topologyNeighborCount', 0), Number(selectedRow?.__topologyNeighborCount ?? 0) > 0 ? 'info' : 'warn'],
            ['发现链路', numberText(selectedRow, '__discoveryLinks', 0), Number(selectedRow?.__discoveryLinks ?? 0) > 0 ? 'info' : 'warn'],
          ];
  return (
    <WorkPanel title="风险画像">
      <div className="taf-asset-risk">
        <Progress type="circle" size={86} percent={score} format={() => score} strokeColor="#ff4d4f" />
        {items.map(([label, value, tone]) => <div key={label} className={`taf-asset-risk__item is-${tone}`}><strong>{value}</strong><span>{label}</span></div>)}
      </div>
    </WorkPanel>
  );
}

function AssetTabsDetail({
  context,
  onOpenDetail,
}: {
  context: AssetSelectionContext;
  onOpenDetail: (detail: AssetDetailSlug) => void;
}) {
  return (
    <WorkPanel title="资产上下文">
      <Tabs
        className="taf-asset-detail-tabs"
        activeKey="basic"
        onChange={(key) => onOpenDetail(key as AssetDetailSlug)}
        items={assetDetailTabs.map((tab) => ({
          key: tab.slug,
          label: tab.label,
          children: (
            <dl className="taf-asset-context-list">
              <dt>资产状态</dt><dd>{context.status}</dd>
              <dt>风险等级</dt><dd>{context.risk}</dd>
              <dt>标签</dt><dd>生产环境 / 关键业务</dd>
              <dt>证据入口</dt><dd>PCAP / Session / 审计</dd>
              <dt>资产标识</dt><dd>{context.id}</dd>
              <dt>负责人</dt><dd>{context.owner}</dd>
              <dt>网络地址</dt><dd>{context.ip}</dd>
              <dt>开放端口</dt><dd>{context.ports}</dd>
            </dl>
          ),
        }))}
      />
    </WorkPanel>
  );
}

function AssetActionRail({
  activeTab,
  onOpenDetail,
  onFeedback,
}: {
  activeTab: AssetTabSlug;
  onOpenDetail: (detail?: AssetDetailSlug) => void;
  onFeedback: (message: string) => void;
}) {
  const actions: Record<AssetTabSlug, Array<[string, ReactNode]>> = {
    endpoint: [['编辑终端资产', <EditOutlined key="edit" />], ['启动资产发现', <SearchOutlined key="discover" />], ['跳转实体图谱', <ClusterOutlined key="graph" />], ['生成整改工单', <ApartmentOutlined key="ticket" />], ['进入取证分析', <ShareAltOutlined key="forensics" />]],
    server: [['进入资产详情', <ProfileOutlined key="detail" />], ['跳转实体图谱', <ClusterOutlined key="graph" />], ['生成整改工单', <EditOutlined key="ticket" />], ['进入取证分析', <SearchOutlined key="forensics" />], ['更新资产归属', <ApartmentOutlined key="owner" />]],
    'network-device': [['查看接口状态', <BranchesOutlined key="interfaces" />], ['同步设备配置', <ReloadOutlined key="sync" />], ['跳转链路图谱', <ClusterOutlined key="graph" />], ['生成变更工单', <EditOutlined key="ticket" />], ['检查采集链路', <SearchOutlined key="probe" />]],
    'business-system': [['查看依赖关系', <BranchesOutlined key="dependencies" />], ['风险聚合分析', <ThunderboltOutlined key="risk" />], ['跳转业务图谱', <ClusterOutlined key="graph" />], ['生成 SLA 工单', <EditOutlined key="ticket" />], ['更新责任归属', <ApartmentOutlined key="owner" />]],
    unknown: [['确认归属', <SafetyCertificateOutlined key="owner" />], ['生成工单', <EditOutlined key="ticket" />], ['加入观察', <ExclamationCircleOutlined key="watch" />], ['跳转实体图谱', <ClusterOutlined key="graph" />], ['进入取证分析', <SearchOutlined key="forensics" />]],
  };
  return (
    <div className="taf-asset-action-rail">
      {actions[activeTab].map(([label, icon]) => (
        <Button
          key={String(label)}
          size="small"
          icon={icon as ReactNode}
          onClick={() => {
            if (label === '进入资产详情') onOpenDetail('basic');
            else if (activeTab === 'server' && label === '更新资产归属') onOpenDetail('ownership');
            else onFeedback(`${label}已触发，当前资产上下文保持不变。`);
          }}
        >
          {label}
        </Button>
      ))}
    </div>
  );
}

const renderAssetCell = (column: string, value: unknown): ReactNode => {
  if (column === '资产 ID') return <span className="taf-asset-id">{String(value ?? '')}</span>;
  if (column === '重要性' || column === '风险标签') return <StatusTag value={value} />;
  if (column === '暴露端口') return <strong className="taf-asset-port">{String(value ?? '-')}</strong>;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record['资产 ID'] ?? JSON.stringify(record));

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const numberText = (row: SnapshotRow | undefined, key: string, fallback: number) => {
  const value = Number(row?.[key]);
  return Number.isFinite(value) && value > 0 ? String(value) : String(fallback);
};

const riskLevel = (row: SnapshotRow | undefined) => {
  const risk = text(row, '风险标签', '');
  const importance = text(row, '重要性', '');
  if (risk.includes('高') || risk.includes('漏洞') || importance.includes('高')) return '高风险';
  if (risk.includes('弱') || risk.includes('暴露') || importance.includes('中')) return '中风险';
  return '低风险';
};
