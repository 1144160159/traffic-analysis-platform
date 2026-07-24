import {
  ApiOutlined,
  ApartmentOutlined,
  AppstoreOutlined,
  CheckCircleOutlined,
  ClusterOutlined,
  CloseOutlined,
  DesktopOutlined,
  DownOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileOutlined,
  FileSearchOutlined,
  HddOutlined,
  RadarChartOutlined,
  MoreOutlined,
  ProfileOutlined,
  QuestionCircleOutlined,
  ReloadOutlined,
  RightOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  ShareAltOutlined,
  UnlockOutlined,
  UpOutlined,
  UserDeleteOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Modal, Progress, Select, Space, Table, Tabs, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { AssetTypeIcon } from '@/components/AssetTypeIcon';
import {
  AssetDiscoveryActivityChart,
  AssetDistributionDonutChart,
  AssetMetricRingsChart,
  AssetPeriodicHeatmapChart,
  AssetProtocolDistributionChart,
  AssetTrafficProfileChart,
  type AssetDistributionItem,
  type AssetMetricRingItem,
} from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchAssetTopology, fetchPageSnapshot, type AssetSnapshotFilters, type AssetTopologyGraph } from '@/services/api';
import type { SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';
import { AssetDetailWorkspace } from './AssetDetailWorkspace';
import {
  assetSearchParams,
  assetTabs,
  canOpenAssetDetail,
  resolveAssetDetail,
  resolveAssetTab,
  type AssetDetailSlug,
  type AssetTabSlug,
} from './assetInventoryState';

const pageSize = 10;
const enabledDetails: AssetDetailSlug[] = ['basic', 'network-interface', 'open-services', 'ownership', 'history'];

const assetMetricDefinitions: Record<AssetTabSlug, Array<{ source: string; label: string }>> = {
  endpoint: [
    { source: '分类资产总数', label: '已识别终端' }, { source: '活跃资产', label: '在线终端' },
    { source: '高风险资产', label: '高风险终端' }, { source: '未归属资产', label: '未归属终端' },
    { source: '分类观测记录', label: '流量观测点' },
  ],
  server: [
    { source: '分类资产总数', label: '服务器资产' }, { source: '活跃资产', label: '在线服务器' },
    { source: '暴露服务数', label: '暴露端口' }, { source: '高危服务数', label: '高危服务' },
    { source: '弱口令疑似', label: '弱口令疑似' }, { source: '未归属资产', label: '未归属服务器' },
  ],
  'network-device': [
    { source: '分类资产总数', label: '网络设备' }, { source: '活跃资产', label: '在线设备' },
    { source: '网络接口数', label: '接口总数' }, { source: '高风险资产', label: '高风险设备' },
    { source: '未归属资产', label: '未归属设备' }, { source: '离线资产', label: '离线设备' },
    { source: '分类观测记录', label: '观测接口' },
  ],
  'business-system': [
    { source: '分类资产总数', label: '业务系统' }, { source: '关键资产', label: '关键系统' },
    { source: '依赖资产数', label: '依赖资产' }, { source: '关键服务数', label: '关键服务' },
    { source: '高风险资产', label: '高风险系统' }, { source: 'SLA 临近', label: 'SLA 临近' },
    { source: '未归属资产', label: '待确认归属' },
  ],
  unknown: [
    { source: '分类资产总数', label: '未知资产' }, { source: '活跃资产', label: '活跃未知资产' },
    { source: '未归属资产', label: '未归属 IP/MAC' }, { source: '高风险资产', label: '高风险资产' },
    { source: '归属候选数', label: '归属候选' }, { source: '待处理工单', label: '待处理工单' },
    { source: '离线资产', label: '已失活资产' },
  ],
};

const assetMetricIcons: Record<AssetTabSlug, ReactNode[]> = {
  endpoint: [<DesktopOutlined />, <CheckCircleOutlined />, <WarningOutlined />, <UserDeleteOutlined />, <RadarChartOutlined />],
  server: [<HddOutlined />, <CheckCircleOutlined />, <UnlockOutlined />, <WarningOutlined />, <UserDeleteOutlined />, <DownOutlined />],
  'network-device': [<ApartmentOutlined />, <CheckCircleOutlined />, <ApiOutlined />, <WarningOutlined />, <UserDeleteOutlined />, <DownOutlined />, <RadarChartOutlined />],
  'business-system': [<AppstoreOutlined />, <SafetyCertificateOutlined />, <ApartmentOutlined />, <ApiOutlined />, <WarningOutlined />, <RadarChartOutlined />, <UserDeleteOutlined />],
  unknown: [<QuestionCircleOutlined />, <RadarChartOutlined />, <ClusterOutlined />, <WarningOutlined />, <UserDeleteOutlined />, <ShareAltOutlined />, <DownOutlined />],
};

const assetTableColumns: Record<AssetTabSlug, Array<{ title: string; width?: number }>> = {
  endpoint: ['资产 ID', 'IP/MAC', '主机名', '类型', '园区/部门', '操作系统', '重要性', '最近活跃', '暴露端口', '风险标签'].map((title) => ({ title })),
  server: [
    { title: '资产 ID', width: 92 }, { title: 'IP/MAC', width: 126 }, { title: '主机名', width: 112 },
    { title: '业务系统', width: 108 }, { title: '园区/部门', width: 120 }, { title: '操作系统', width: 104 },
    { title: '重要性', width: 66 }, { title: '资产状态', width: 76 }, { title: '最近活跃', width: 126 },
    { title: '暴露端口', width: 72 }, { title: '高危服务', width: 72 }, { title: '风险标签', width: 84 },
  ],
  'network-device': [
    { title: '资产 ID', width: 76 }, { title: '主机名', width: 96 }, { title: '类型', width: 72 },
    { title: '厂商', width: 56 }, { title: '园区/部门', width: 96 }, { title: '管理IP', width: 82 },
    { title: '设备角色', width: 74 }, { title: '接口数', width: 54 }, { title: '资产状态', width: 62 },
  ],
  'business-system': [
    { title: '资产 ID', width: 88 }, { title: '主机名', width: 116 }, { title: '业务域', width: 92 },
    { title: '系统等级', width: 76 }, { title: '责任部门', width: 96 }, { title: '关键服务', width: 76 },
    { title: '依赖资产', width: 76 }, { title: '风险评分', width: 76 }, { title: 'SLA', width: 72 },
    { title: '最近活跃', width: 124 }, { title: '风险标签', width: 84 },
  ],
  unknown: [
    { title: '资产 ID', width: 92 }, { title: 'IP/MAC', width: 126 }, { title: '主机名', width: 106 },
    { title: '来源', width: 94 }, { title: '疑似类型', width: 86 }, { title: '置信度', width: 70 },
    { title: '风险标签', width: 78 }, { title: '首次发现', width: 120 }, { title: '最近活跃', width: 120 },
    { title: '工单状态', width: 80 },
  ],
};

type AssetSelectionContext = {
  tab: AssetTabSlug;
  id: string;
  displayCode: string;
  name: string;
  ip: string;
  type: string;
  location: string;
  os: string;
  importance: string;
  risk: string;
  status: string;
  owner: string;
  lastSeen: string;
  sourceRow?: SnapshotRow;
};

export function AssetInventoryPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const visualBreakdownMode = isVisualBreakdownMode();
  const activeTab = resolveAssetTab(searchParams.get('tab'));
  const activeDetail = resolveAssetDetail(searchParams.get('detail'));
  const requestedAssetId = searchParams.get('assetId') ?? '';
  const requestedSearch = searchParams.get('search') ?? '';
  const [filtersCollapsed, setFiltersCollapsed] = useState(false);
  const [detailRailVisible, setDetailRailVisible] = useState(true);
  const [page, setPage] = useState(1);
  const [draftFilters, setDraftFilters] = useState<AssetSnapshotFilters>(() => requestedSearch ? { search: requestedSearch } : {});
  const [appliedFilters, setAppliedFilters] = useState<AssetSnapshotFilters>(() => requestedSearch ? { search: requestedSearch } : {});
  const nextSearchParams = useCallback((state: Parameters<typeof assetSearchParams>[0]) => {
    const next = assetSearchParams({ ...state, search: state.search ?? requestedSearch });
    if (visualBreakdownMode) next.set('__codex_ui_breakdown_production', '1');
    return next;
  }, [requestedSearch, visualBreakdownMode]);

  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, activeTab, page, appliedFilters],
    queryFn: () => fetchPageSnapshot(route.id, { assetType: activeTab, page, pageSize, assetFilters: appliedFilters }),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedAssetId = useMemo(() => {
    if (!rows.length) return '';
    if (requestedAssetId && rows.some((row) => rowKey(row) === requestedAssetId)) return requestedAssetId;
    return rowKey(rows[0]);
  }, [requestedAssetId, rows]);
  const selectedRow = useMemo(() => rows.find((row) => rowKey(row) === selectedAssetId), [rows, selectedAssetId]);
  const topologyEnabled = Boolean(selectedAssetId && ['server', 'network-device', 'business-system'].includes(activeTab));
  const topologyQuery = useQuery({
    queryKey: ['asset-topology', selectedAssetId],
    queryFn: () => fetchAssetTopology(selectedAssetId),
    enabled: topologyEnabled,
  });
  const selectedContext = useMemo(
    () => buildSelectionContext(activeTab, selectedAssetId, selectedRow),
    [activeTab, selectedAssetId, selectedRow],
  );

  useEffect(() => {
    if (isLoading || isError) return;
    if (!selectedAssetId) {
      if (requestedAssetId || activeDetail) setSearchParams(nextSearchParams({ tab: activeTab }), { replace: true });
      return;
    }
    const validDetail = activeDetail && canOpenAssetDetail(activeTab, selectedAssetId) && enabledDetails.includes(activeDetail)
      ? activeDetail
      : null;
    if (requestedAssetId !== selectedAssetId || activeDetail !== validDetail) {
      setSearchParams(nextSearchParams({ tab: activeTab, assetId: selectedAssetId, detail: validDetail }), { replace: true });
    }
  }, [activeDetail, activeTab, isError, isLoading, nextSearchParams, requestedAssetId, selectedAssetId, setSearchParams]);

  const columns: ColumnsType<SnapshotRow> = [
    ...assetTableColumns[activeTab].map((column) => ({
      title: column.title,
      dataIndex: column.title,
      key: column.title,
      width: column.width,
      ellipsis: true,
      render: (value: unknown) => renderAssetCell(column.title, value),
    })),
    {
      title: '操作',
      key: '操作',
      width: 64,
      render: (_value, record) => (
        <Space size={2} onClick={(event) => event.stopPropagation()}>
          <Button type="text" size="small" icon={<EyeOutlined />} aria-label={`查看 ${text(record, '资产 ID', '资产')}`} onClick={() => setSearchParams(nextSearchParams({ tab: activeTab, assetId: rowKey(record) }))} />
          <Tooltip title="更多资产动作尚未接入"><Button type="text" size="small" disabled icon={<MoreOutlined />} aria-label={`更多 ${text(record, '资产 ID', '资产')}`} /></Tooltip>
        </Space>
      ),
    },
  ];

  const selectAsset = (assetId: string) => setSearchParams(nextSearchParams({ tab: activeTab, assetId }));
  const openDetail = (detail: AssetDetailSlug = 'basic') => {
    if (!canOpenAssetDetail(activeTab, selectedAssetId) || !enabledDetails.includes(detail)) return;
    setSearchParams(nextSearchParams({ tab: activeTab, assetId: selectedAssetId, detail }));
  };
  const closeDetail = () => setSearchParams(nextSearchParams({ tab: activeTab, assetId: selectedAssetId }));

  const metrics = assetMetricDefinitions[activeTab].map(({ source, label }) => {
    const metric = data?.metrics?.find((item) => item.label === source);
    return metric ? { ...metric, label } : { label, value: '-', delta: '等待真实数据', status: 'info' as const };
  });

  return (
    <div className={`taf-page taf-asset-inventory taf-asset-tab-${activeTab}${visualBreakdownMode ? ' is-visual-target' : ''}`} data-asset-presentation={activeTab}>
      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实资产数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/assets、资产服务、鉴权和租户上下文。'}
          action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
        />
      )}

      <div className={`taf-asset-grid${detailRailVisible ? '' : ' is-detail-hidden'}`}>
        <main className="taf-asset-main">
          <header className="taf-asset-titlebar">
            <div><h1>{route.page.title}</h1></div>
            <Space>
              <Button size="small" icon={filtersCollapsed ? <DownOutlined /> : <UpOutlined />} onClick={() => setFiltersCollapsed((value) => !value)}>{filtersCollapsed ? '展开' : '收起'}</Button>
              {!detailRailVisible && <Button size="small" icon={<ProfileOutlined />} onClick={() => setDetailRailVisible(true)}>显示摘要</Button>}
              <Tooltip title="刷新资产台账"><Button icon={<ReloadOutlined />} size="small" loading={isLoading} onClick={() => void refetch()} /></Tooltip>
            </Space>
          </header>
          <WorkPanel title="资产台账">
            <Tabs
              className="taf-asset-tabs"
              activeKey={activeTab}
              onChange={(key) => {
                setPage(1);
                setSearchParams(nextSearchParams({ tab: resolveAssetTab(key) }));
              }}
              items={assetTabs.map((tab) => ({ key: tab.slug, label: tab.label }))}
            />
            {!filtersCollapsed && (
              <AssetFilter
                activeTab={activeTab}
                value={draftFilters}
                onChange={setDraftFilters}
                onApply={() => {
                  setPage(1);
                  setAppliedFilters(draftFilters);
                }}
                onReset={() => {
                  setPage(1);
                  setDraftFilters({});
                  setAppliedFilters({});
                }}
              />
            )}
            {!filtersCollapsed && <div className={`taf-asset-kpis is-${activeTab}`} data-kpi-count={metrics.length}>{metrics.map((metric, index) => <MetricTile key={metric.label} metric={metric} icon={assetMetricIcons[activeTab][index]} />)}</div>}
          </WorkPanel>

          <AssetCategoryContent
            activeTab={activeTab}
            columns={columns}
            isLoading={isLoading}
            rows={rows}
            selectedRow={selectedRow}
            topologyGraph={topologyQuery.data}
            topologyLoading={topologyEnabled && topologyQuery.isLoading}
            topologyError={topologyEnabled && topologyQuery.isError}
            page={page}
            total={data?.total ?? rows.length}
            onPageChange={setPage}
            selectAsset={selectAsset}
            onRefresh={() => void refetch()}
          />
        </main>

        {detailRailVisible && (
          <aside className="taf-asset-detail">
            {selectedRow ? (
              <AssetSummaryRail
                context={selectedContext}
                activeTab={activeTab}
                assetId={selectedAssetId}
                onClose={() => setDetailRailVisible(false)}
                onOpenDetail={openDetail}
              />
            ) : (
              <WorkPanel title="资产上下文"><Alert type={isError ? 'error' : 'info'} showIcon message={isLoading ? '正在加载真实资产数据' : isError ? '资产上下文不可用' : '当前条件下暂无资产'} /></WorkPanel>
            )}
          </aside>
        )}
      </div>

      <Drawer
        className="taf-asset-detail-drawer"
        title={null}
        width={880}
        placement="right"
        closable={false}
        open={Boolean(activeDetail && enabledDetails.includes(activeDetail) && canOpenAssetDetail(activeTab, selectedAssetId))}
        onClose={closeDetail}
        destroyOnClose
        maskClosable
        styles={{ body: { padding: 0 } }}
      >
        {activeDetail && enabledDetails.includes(activeDetail) && (
          <AssetDetailWorkspace
            assetId={selectedAssetId}
            detail={activeDetail}
            onClose={closeDetail}
            onDetailChange={(detail) => setSearchParams(nextSearchParams({ tab: 'server', assetId: selectedAssetId, detail }))}
          />
        )}
      </Drawer>
    </div>
  );
}

function AssetFilter({ activeTab, value, onChange, onApply, onReset }: {
  activeTab: AssetTabSlug;
  value: AssetSnapshotFilters;
  onChange: (value: AssetSnapshotFilters) => void;
  onApply: () => void;
  onReset: () => void;
}) {
  return (
    <div className="taf-asset-filter">
      <label><span>{assetTabs.find((item) => item.slug === activeTab)?.label}关键词</span><Input size="small" allowClear value={value.search ?? ''} placeholder="资产 ID / IP / 名称" onChange={(event) => onChange({ ...value, search: event.target.value || undefined })} /></label>
      <label><span>状态</span><Select size="small" value={value.status ?? '全部'} options={[{ value: '全部' }, { value: 'active', label: '活跃' }, { value: 'inactive', label: '离线' }, { value: 'unknown', label: '未知' }]} onChange={(status) => onChange({ ...value, status: status === '全部' ? undefined : status })} /></label>
      <label><span>园区</span><Input size="small" allowClear value={value.campus ?? ''} placeholder="精确园区" onChange={(event) => onChange({ ...value, campus: event.target.value || undefined })} /></label>
      <label><span>部门</span><Input size="small" allowClear value={value.department ?? ''} placeholder="精确部门" onChange={(event) => onChange({ ...value, department: event.target.value || undefined })} /></label>
      <div className="taf-asset-filter__actions"><Button size="small" onClick={onReset}>重置</Button><Button size="small" type="primary" icon={<SearchOutlined />} onClick={onApply}>查询</Button></div>
    </div>
  );
}

function AssetCategoryContent({
  activeTab,
  columns,
  isLoading,
  rows,
  selectedRow,
  topologyGraph,
  topologyLoading,
  topologyError,
  page,
  total,
  onPageChange,
  selectAsset,
  onRefresh,
}: {
  activeTab: AssetTabSlug;
  columns: ColumnsType<SnapshotRow>;
  isLoading: boolean;
  rows: SnapshotRow[];
  selectedRow?: SnapshotRow;
  topologyGraph?: AssetTopologyGraph;
  topologyLoading: boolean;
  topologyError: boolean;
  page: number;
  total: number;
  onPageChange: (page: number) => void;
  selectAsset: (assetId: string) => void;
  onRefresh: () => void;
}) {
  const label = assetTabs.find((item) => item.slug === activeTab)?.label ?? '资产';
  const ledgerTitle = `${label}资产清单（共 ${total.toLocaleString()} 条）`;
  const ledger = (
    <WorkPanel
      title={ledgerTitle}
      className="taf-asset-ledger-panel"
      extra={<Space><Button size="small" icon={<ReloadOutlined />} onClick={onRefresh}>刷新</Button><Button size="small" icon={<DownloadOutlined />} disabled={!rows.length} onClick={() => exportAssetRows(rows)}>导出</Button><Tooltip title="列设置与用户偏好存储尚未接入"><Button size="small" disabled icon={<SettingOutlined />} aria-label="清单设置" /></Tooltip></Space>}
    >
      <Table
        rowKey={rowKey}
        size="small"
        loading={isLoading}
        columns={columns}
        dataSource={rows}
        pagination={{ current: page, pageSize, total, size: 'small', hideOnSinglePage: false, showSizeChanger: false, showQuickJumper: true, showTotal: (count) => `共 ${count.toLocaleString()} 条`, onChange: onPageChange }}
        rowSelection={{
          type: 'radio',
          selectedRowKeys: selectedRow ? [rowKey(selectedRow)] : [],
          onChange: (keys) => keys[0] && selectAsset(String(keys[0])),
        }}
        onRow={(record) => ({ onClick: () => selectAsset(rowKey(record)) })}
        locale={{ emptyText: '当前分类或筛选条件下暂无真实资产记录' }}
      />
    </WorkPanel>
  );
  return (
    <>
      {activeTab === 'network-device' ? (
        <div className="taf-asset-network-ledger-layout" data-layout-signature="network-ledger-interface-horizontal">
          {ledger}
          <InterfaceStatusPanel title={`接口状态矩阵（${text(selectedRow, '主机名', '未选择')}）`} items={metadataList(selectedRow, 'network_interfaces')} />
        </div>
      ) : ledger}
      <AssetCategoryWorkspace activeTab={activeTab} selectedRow={selectedRow} topologyGraph={topologyGraph} topologyLoading={topologyLoading} topologyError={topologyError} />
    </>
  );
}

function AssetCategoryWorkspace({ activeTab, selectedRow, topologyGraph, topologyLoading, topologyError }: { activeTab: AssetTabSlug; selectedRow?: SnapshotRow; topologyGraph?: AssetTopologyGraph; topologyLoading: boolean; topologyError: boolean }) {
  const content = activeTab === 'server'
    ? <ServerWorkspace row={selectedRow} topologyGraph={topologyGraph} topologyLoading={topologyLoading} topologyError={topologyError} />
    : activeTab === 'network-device'
      ? <NetworkDeviceWorkspace row={selectedRow} topologyGraph={topologyGraph} topologyLoading={topologyLoading} topologyError={topologyError} />
      : activeTab === 'business-system'
        ? <BusinessSystemWorkspace row={selectedRow} topologyGraph={topologyGraph} topologyLoading={topologyLoading} topologyError={topologyError} />
        : activeTab === 'unknown'
          ? <UnknownAssetWorkspace row={selectedRow} />
          : <EndpointWorkspace row={selectedRow} />;
  return <div className={`taf-asset-category-workspace is-${activeTab}`}>{content}</div>;
}

function EndpointWorkspace({ row }: { row?: SnapshotRow }) {
  const protocols = metadataList(row, 'protocols').map((item) => ({ label: stringValue(item.name), value: numberValue(item.percent) }));
  return (
    <>
      <div className="taf-asset-observability">
        <WorkPanel title={`流量画像（${text(row, '主机名', '未选择')}）`}><AssetTrafficProfileChart inbound={metadataNumbers(row, 'traffic_profile')} outbound={metadataNumbers(row, 'traffic_outbound')} eastWest={metadataNumbers(row, 'traffic_east_west')} labels={metadataArray(row, 'traffic_time_labels').map(stringValue)} ariaLabel={`${text(row, '主机名', '当前终端')}流量画像：入站、出站与东西向流量`} /></WorkPanel>
        <WorkPanel title="协议分布"><AssetProtocolDistributionChart items={protocols} totalLabel={stringValue(metadataOf(row).protocol_total_throughput) || '-'} ariaLabel={`${text(row, '主机名', '当前终端')}协议流量分布`} /></WorkPanel>
        <WorkPanel title="Top 对端（按流量）"><PeerRanking items={metadataList(row, 'top_peers')} /></WorkPanel>
        <WorkPanel title="周期性连接（最近 7 天）"><AssetPeriodicHeatmapChart values={metadataNumbers(row, 'periodic_activity')} ariaLabel={`${text(row, '主机名', '当前终端')}最近七天周期性连接热力图`} /></WorkPanel>
      </div>
      <WorkPanel title="关联证据与上下文" className="taf-asset-evidence-panel"><EvidenceStrip selectedRow={row} /></WorkPanel>
    </>
  );
}

function ServerWorkspace({ row, topologyGraph, topologyLoading, topologyError }: { row?: SnapshotRow; topologyGraph?: AssetTopologyGraph; topologyLoading: boolean; topologyError: boolean }) {
  const services = metadataList(row, 'open_services');
  const osDistribution = distributionItems(row, 'os_distribution');
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title="服务端口风险矩阵" className="taf-asset-wide"><PortRiskMatrix services={services} /></WorkPanel>
        <WorkPanel title="服务拓扑"><AssetTopology row={row} graph={topologyGraph} loading={topologyLoading} error={topologyError} mode="server" /></WorkPanel>
        <MetadataRecordsPanel title={`开放服务明细（${text(row, '主机名', '未选择')}）`} viewAllLabel="查看全部服务" maxRows={7} items={services} fields={[['端口','port'],['服务','service'],['版本','version'],['风险','risk_level']]} />
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="当前分类 OS / 探针状态"><AssetDistributionDonutChart items={osDistribution} centerLabel="分类资产" centerValue={String(osDistribution.reduce((sum, item) => sum + item.value, 0))} ariaLabel="当前服务器分类的操作系统与探针状态分布" /></WorkPanel>
        <WorkPanel title="业务归属与负责人"><OwnershipSummary row={row} /></WorkPanel>
        <WorkPanel title="关联证据与上下文" className="taf-asset-evidence-panel"><EvidenceStrip selectedRow={row} /></WorkPanel>
      </div>
    </>
  );
}

function NetworkDeviceWorkspace({ row, topologyGraph, topologyLoading, topologyError }: { row?: SnapshotRow; topologyGraph?: AssetTopologyGraph; topologyLoading: boolean; topologyError: boolean }) {
  const links = metadataList(row, 'mirror_links');
  const changes = metadataList(row, 'config_changes');
  const impacts = metadataList(row, 'business_impacts');
  return (
    <>
      <div className="taf-asset-network-modules">
        <WorkPanel title="链路拓扑"><AssetTopology row={row} graph={topologyGraph} loading={topologyLoading} error={topologyError} mode="network" /></WorkPanel>
        <MetadataRecordsPanel title="镜像口与采集链路" viewAllLabel="查看全部镜像口" maxRows={6} items={links} fields={[['接口','interface'],['方向','direction'],['模式','mode'],['探针','target'],['带宽','bandwidth'],['状态','status']]} />
        <MetadataRecordsPanel title="配置变更记录" viewAllLabel="查看全部变更记录" maxRows={5} items={changes} fields={[['时间','time'],['操作人','actor'],['变更','change'],['风险','risk']]} />
        <MetadataRecordsPanel title="链路归属与业务影响" viewAllLabel="查看全部业务影响" maxRows={5} items={impacts} fields={[['业务系统','name'],['链路','links'],['流量','traffic'],['影响','risk']]} />
      </div>
      <WorkPanel title="关联证据与上下文" className="taf-asset-evidence-panel"><EvidenceStrip selectedRow={row} /></WorkPanel>
    </>
  );
}

function BusinessSystemWorkspace({ row, topologyGraph, topologyLoading, topologyError }: { row?: SnapshotRow; topologyGraph?: AssetTopologyGraph; topologyLoading: boolean; topologyError: boolean }) {
  const factors = metadataList(row, 'risk_factors');
  const riskDistribution = distributionItems(row, 'risk_distribution');
  const services = metadataList(row, 'key_services');
  const health = metadataList(row, 'dependency_health');
  const responsibility = metadataList(row, 'responsibility');
  return (
    <>
      <div className="taf-asset-work-grid">
        <WorkPanel title={`依赖关系图（${text(row, '主机名', '未选择')}）`} className="taf-asset-wide"><AssetTopology row={row} graph={topologyGraph} loading={topologyLoading} error={topologyError} mode="business" /></WorkPanel>
        <WorkPanel title="风险评分"><BusinessRisk row={row} items={riskDistribution.length ? riskDistribution : factors.map((item) => ({ label: stringValue(item.name), value: numberValue(item.percent) }))} /></WorkPanel>
        <MetadataRecordsPanel title="关键服务清单" viewAllLabel="查看全部服务" maxRows={5} items={services} fields={[['服务','name'],['端口/协议','endpoint'],['依赖','dependency'],['风险','risk'],['健康','health']]} />
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="依赖资产与健康"><DependencyHealth items={health} /></WorkPanel>
        <MetadataRecordsPanel title="责任部门与 SLA" viewAllLabel="查看 SLA 明细" maxRows={4} items={responsibility} fields={[['部门','department'],['角色','role'],['负责人','owner'],['SLA','sla'],['状态','status']]} />
        <WorkPanel title="关联证据与上下文" className="taf-asset-evidence-panel"><EvidenceStrip selectedRow={row} /></WorkPanel>
      </div>
    </>
  );
}

function UnknownAssetWorkspace({ row }: { row?: SnapshotRow }) {
  const activity = metadataRecord(row, 'discovery_activity');
  const candidates = metadataList(row, 'ownership_candidates');
  const sources = distributionItems(row, 'source_distribution');
  const profiles = distributionItems(row, 'device_profile_distribution');
  const exposure = distributionItems(row, 'risk_distribution');
  return (
    <>
      <div className="taf-asset-unknown-main-grid">
        <WorkPanel title="发现时间线"><AssetDiscoveryActivityChart labels={recordNumbers(activity.labels).map(stringValue)} discovered={recordNumbers(activity.discovered).map(numberValue)} pending={recordNumbers(activity.pending_rate).map(numberValue)} ariaLabel="未知资产按小时发现与待归属趋势" /></WorkPanel>
        <WorkPanel title="近 7 天未识别画像样本"><AssetDistributionDonutChart items={profiles} centerLabel="画像样本" centerValue={String(profiles.reduce((sum, item) => sum + item.value, 0))} ariaLabel="近七天未识别设备画像样本分类分布" /></WorkPanel>
        <MetadataRecordsPanel title="归属候选与匹配" viewAllLabel="查看全部候选" maxRows={5} items={candidates} fields={[['部门','department'],['负责人','owner'],['匹配数','matched'],['置信度','confidence']]} />
        <WorkPanel title="风险与暴露面"><UnknownExposure row={row} items={exposure} /></WorkPanel>
      </div>
      <div className="taf-asset-lower-grid">
        <WorkPanel title="工单与处置闭环"><TicketSteps row={row} /></WorkPanel>
        <WorkPanel title="关联证据与上下文" className="taf-asset-evidence-panel"><EvidenceStrip selectedRow={row} /></WorkPanel>
        <WorkPanel title="近 7 天来源样本"><AssetDistributionDonutChart items={sources} centerLabel="来源样本" centerValue={String(sources.reduce((sum, item) => sum + item.value, 0))} ariaLabel="近七天未知资产来源样本与发现方式分布" /></WorkPanel>
      </div>
    </>
  );
}

function PeerRanking({ items }: { items: Record<string, unknown>[] }) {
  const peak = Math.max(1, ...items.map((item) => numberValue(item.share)));
  return <div className="taf-asset-peer-list">{items.map((item) => {
    const share = numberValue(item.share);
    return <div key={stringValue(item.name)} className="taf-asset-peer-row"><span>{stringValue(item.name)}</span><i><b style={{ width: `${Math.max(3, (share / peak) * 100)}%` }} /></i><em>{stringValue(item.type)}</em><strong>{share.toFixed(1)}%</strong></div>;
  })}</div>;
}

function PeriodicHeatmap({ values }: { values: number[] }) {
  return (
    <div className="taf-asset-heatmap" aria-label="最近七天周期性连接热力图">
      {['周一', '周二', '周三', '周四', '周五', '周六', '周日'].map((day, dayIndex) => (
        <div key={day}><span>{day}</span>{Array.from({ length: 14 }, (_, index) => <i key={index} className={(values[index] ?? 0) + dayIndex >= 38 ? 'is-hot' : 'is-cold'} />)}</div>
      ))}
    </div>
  );
}

function EvidenceStrip({ selectedRow }: { selectedRow?: SnapshotRow }) {
  const navigate = useNavigate();
  const evidence = metadataRecord(selectedRow, 'evidence');
  const evidenceCount = (key: string) => Object.prototype.hasOwnProperty.call(evidence, key) ? numberValue(evidence[key]) : null;
  const items: Array<[string, number | null, ReactNode]> = [
    ['PCAP 证据', evidenceCount('pcap'), <FileOutlined key="pcap" />],
    ['Session 记录', evidenceCount('session'), <ProfileOutlined key="session" />],
    ['DNS 日志', evidenceCount('dns'), <ClusterOutlined key="dns" />],
    ['TLS 会话', evidenceCount('tls'), <SafetyCertificateOutlined key="tls" />],
    ['告警事件', evidenceCount('alerts'), <FileSearchOutlined key="alert" />],
    ['配置变更', evidenceCount('config'), <ShareAltOutlined key="config" />],
  ];
  return (
    <>
      <div className="taf-asset-evidence-strip">
        {items.map(([label, count, icon]) => <button type="button" className={`taf-asset-evidence-item${count === null ? ' is-unavailable' : ''}`} key={label} disabled={!selectedRow || count === null} title={`${text(selectedRow, '主机名', '当前资产')} · ${label}`} onClick={() => navigate(evidenceRoute(label, selectedRow))}>{icon}<span>{label}</span><strong>{count === null ? '未接入' : `${count} 条`}</strong><RightOutlined className="taf-asset-item-chevron" /></button>)}
      </div>
      <Button type="link" size="small" className="taf-asset-panel-view-all" disabled={!selectedRow} onClick={() => navigate(evidenceRoute('全部证据', selectedRow))}>查看全部证据 <RightOutlined /></Button>
    </>
  );
}

function MetadataRecordsPanel({ title, viewAllLabel, items, fields, maxRows = 7 }: { title: string; viewAllLabel: string; items: Record<string, unknown>[]; fields: Array<[string, string]>; maxRows?: number }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <WorkPanel title={title} className="taf-asset-records-panel">
        <MetadataTable maxRows={maxRows} items={items} fields={fields} />
        <Button type="link" size="small" className="taf-asset-panel-view-all" disabled={!items.length} onClick={() => setOpen(true)}>{viewAllLabel} <RightOutlined /></Button>
      </WorkPanel>
      <Modal title={`${title} · 全部记录`} open={open} footer={null} onCancel={() => setOpen(false)} destroyOnClose width={920}>
        <MetadataTable maxRows={Math.max(items.length, 1)} items={items} fields={fields} />
      </Modal>
    </>
  );
}

function MetadataTable({ items, fields, maxRows = 7 }: { items: Record<string, unknown>[]; fields: Array<[string, string]>; maxRows?: number }) {
  if (!items.length) return <Alert type="info" showIcon message="该资产暂无已持久化记录" />;
  return (
    <div className="taf-asset-data-table">
      <div className="taf-asset-data-table__head">{fields.map(([label]) => <strong key={label}>{label}</strong>)}</div>
      {items.slice(0, maxRows).map((item, index) => (
        <div key={`${stringValue(item[fields[0][1]])}-${index}`} className="taf-asset-data-table__row">
          {fields.map(([label, key]) => {
            const value = stringValue(item[key]);
            return <span key={key} title={value}>{/(高危|高风险|异常|降级|中危|中风险|在线|健康|正常)/.test(value) ? <StatusTag value={value} /> : value || '-'}</span>;
          })}
        </div>
      ))}
    </div>
  );
}

function PortRiskMatrix({ services }: { services: Record<string, unknown>[] }) {
  if (!services.length) return <Alert type="info" showIcon message="该资产暂无端口风险统计" />;
  return (
    <div className="taf-asset-port-risk-matrix">
      <div className="taf-asset-port-risk-matrix__head"><strong>端口</strong><strong>高危</strong><strong>中危</strong><strong>低危</strong><strong>安全</strong><strong>总计</strong></div>
      {services.slice(0, 8).map((item) => {
        const risk = stringValue(item.risk_level) || '未分级';
        const counts = {
          high: risk.includes('高') ? 1 : 0,
          medium: risk.includes('中') ? 1 : 0,
          low: risk.includes('低') ? 1 : 0,
          safe: /(安全|正常)/.test(risk) ? 1 : 0,
        };
        return (
          <div key={`${stringValue(item.protocol)}-${stringValue(item.port)}`} title={`${stringValue(item.service)} · ${risk}`}>
            <strong>{stringValue(item.port)}</strong>
            <span className="is-high">{counts.high}</span>
            <span className="is-medium">{counts.medium}</span>
            <span className="is-low">{counts.low}</span>
            <span className="is-safe">{counts.safe}</span>
            <b>{counts.high + counts.medium + counts.low + counts.safe || 1}</b>
          </div>
        );
      })}
      <div className="taf-asset-port-risk-matrix__legend"><i className="is-high" />高危<i className="is-medium" />中危<i className="is-low" />低危<i className="is-safe" />安全</div>
    </div>
  );
}

function AssetTopology({ row, graph, loading, error, mode }: { row?: SnapshotRow; graph?: AssetTopologyGraph; loading: boolean; error: boolean; mode: 'server' | 'network' | 'business' }) {
  const fallback = mode === 'network' ? '网络设备' : mode === 'business' ? '业务系统' : '服务器';
  const center = text(row, '资产 ID', fallback);
  const centerName = text(row, '主机名', center);
  const palette = mode === 'server'
    ? { primary: '#18a8ff', secondary: '#35e08b' }
    : mode === 'network'
      ? { primary: '#35e08b', secondary: '#18a8ff' }
      : { primary: '#9d7bff', secondary: '#18c7d9' };
  const markerId = `asset-topology-arrow-${mode}`;
  const gradientId = `asset-topology-gradient-${mode}`;
  const centerID = graph?.asset_id || rowKey(row ?? {});
  const centerNode = graph?.nodes.find((node) => node.id === centerID);
  const nodes = (graph?.nodes ?? []).filter((node) => node.id !== centerID).slice(0, 8);
  const nodeLayout = nodes.map((node, index) => {
    const angle = -Math.PI / 2 + (Math.PI * 2 * index) / Math.max(nodes.length, 1);
    return { ...node, displayLabel: node.label.length > 10 ? `${node.label.slice(0, 9)}…` : node.label, x: 180 + Math.cos(angle) * 126, y: 70 + Math.sin(angle) * 46 };
  });
  const positions = new Map<string, { x: number; y: number }>([
    [centerID, { x: 180, y: 70 }],
    ...nodeLayout.map((node): [string, { x: number; y: number }] => [node.id, { x: node.x, y: node.y }]),
  ]);
  const edges = (graph?.edges ?? []).filter((edge) => positions.has(edge.source) && positions.has(edge.target));
  const navigate = useNavigate();
  return (
    <div
      className={`taf-asset-topology is-${mode}${error ? ' has-error' : ''}`}
      aria-label={`${centerName} 动态拓扑`}
      data-api-asset-id={graph?.asset_id || ''}
      data-api-source={graph?.source || (loading ? 'loading' : error ? 'error' : 'empty')}
      data-fixture-mode={graph?.fixture_mode ? 'true' : 'false'}
      data-node-count={graph?.nodes.length ?? 0}
      data-edge-count={graph?.edges.length ?? 0}
    >
      <svg viewBox="0 0 360 140" role="img" aria-labelledby={`${markerId}-title`} preserveAspectRatio="xMidYMid meet">
        <title id={`${markerId}-title`}>{centerName} API 动态拓扑，共 {graph?.nodes.length ?? 0} 个节点、{graph?.edges.length ?? 0} 条关系</title>
        <defs>
          <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0%" stopColor={palette.primary} />
            <stop offset="100%" stopColor={palette.secondary} />
          </linearGradient>
          <marker id={markerId} viewBox="0 0 10 10" refX="8" refY="5" markerWidth="5" markerHeight="5" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" fill={palette.primary} />
          </marker>
          <filter id={`${markerId}-glow`} x="-40%" y="-40%" width="180%" height="180%">
            <feGaussianBlur stdDeviation="2.6" result="blur" />
            <feMerge><feMergeNode in="blur" /><feMergeNode in="SourceGraphic" /></feMerge>
          </filter>
        </defs>

        <ellipse className="taf-asset-topology__orbit" cx="180" cy="70" rx="132" ry="50" />
        {edges.map((edge, index) => {
          const source = positions.get(edge.source)!;
          const target = positions.get(edge.target)!;
          const middleX = (source.x + target.x) / 2;
          const middleY = (source.y + target.y) / 2;
          const dx = target.x - source.x;
          const dy = target.y - source.y;
          const length = Math.max(Math.hypot(dx, dy), 1);
          const bend = index % 2 === 0 ? 4 : -4;
          const controlX = middleX + (-dy / length) * bend;
          const controlY = middleY + (dx / length) * bend;
          const healthClass = ['down', 'critical', 'risk'].some((value) => edge.health?.toLowerCase().includes(value))
            ? 'is-risk'
            : edge.health?.toLowerCase().includes('warn') ? 'is-warn' : 'is-healthy';
          return (
            <path
              key={edge.id || `${edge.source}-${edge.target}-${index}`}
              className={`taf-asset-topology__edge ${healthClass}`}
              data-edge-id={edge.id}
              data-source={edge.source}
              data-target={edge.target}
              data-relationship={edge.relationship}
              d={`M ${source.x.toFixed(1)} ${source.y.toFixed(1)} Q ${controlX.toFixed(1)} ${controlY.toFixed(1)} ${target.x.toFixed(1)} ${target.y.toFixed(1)}`}
              markerStart={edge.direction === 'bidirectional' ? `url(#${markerId})` : undefined}
              markerEnd={edge.direction === 'undirected' ? undefined : `url(#${markerId})`}
            >
              <title>{edge.relationship}{edge.protocol ? ` · ${edge.protocol}` : ''}{edge.health ? ` · ${edge.health}` : ''}</title>
            </path>
          );
        })}

        <g className="taf-asset-topology__hub" data-node-id={centerID} filter={`url(#${markerId}-glow)`}>
          <rect x="135" y="49" width="90" height="42" rx="12" fill={`url(#${gradientId})`} />
          <rect x="137" y="51" width="86" height="38" rx="10" />
          <TopologyGlyph kind={centerNode?.kind || mode} x={150} y={70} />
          <text x="181" y="66" textAnchor="middle">{center}</text>
          <text className="taf-asset-topology__hub-name" x="181" y="80" textAnchor="middle">{(centerNode?.label || centerName).length > 13 ? `${(centerNode?.label || centerName).slice(0, 12)}…` : centerNode?.label || centerName}</text>
        </g>

        {nodeLayout.map((node) => (
          <g key={node.id} className={`taf-asset-topology__node is-${node.status || 'unknown'}`} data-node-id={node.id} transform={`translate(${node.x.toFixed(1)} ${node.y.toFixed(1)})`}>
            <title>{node.label}{node.kind ? ` · ${node.kind}` : ''}{node.status ? ` · ${node.status}` : ''}</title>
            <rect x="-41" y="-13" width="82" height="26" rx="7" />
            <TopologyGlyph kind={node.kind} x={-30} y={0} />
            <text x="5" y="1" textAnchor="middle" dominantBaseline="middle">{node.displayLabel}</text>
          </g>
        ))}

        {!loading && !error && !nodes.length && <text className="taf-asset-topology__empty" x="180" y="118" textAnchor="middle">API 暂无拓扑关系</text>}
        {loading && <text className="taf-asset-topology__empty" x="180" y="118" textAnchor="middle">正在读取拓扑 API…</text>}
        {error && <text className="taf-asset-topology__empty is-error" x="180" y="118" textAnchor="middle">拓扑 API 不可用</text>}
      </svg>
      <span className="taf-asset-topology__source">{graph?.source === 'discovery_neighbors' ? '邻居发现' : graph?.source === 'asset_metadata_graph' ? '资产关系 API' : graph?.source === 'legacy_asset_metadata' ? '旧版元数据' : loading ? '加载中' : error ? 'API 异常' : '空拓扑'}{graph?.fixture_mode ? ' · 验收夹具' : ''}</span>
      <Tooltip title="跳转实体图谱">
        <Button aria-label="跳转实体图谱" size="small" type="text" icon={<ShareAltOutlined />} onClick={() => navigate(`/graph?assetId=${encodeURIComponent(rowKey(row ?? {}))}`)} />
      </Tooltip>
    </div>
  );
}

function StateCards({ items }: { items: Record<string, unknown>[] }) {
  return <div className="taf-asset-state-cards">{items.map((item, index) => <div key={`${stringValue(item.label)}-${index}`}><strong>{stringValue(item.value)}</strong><span>{stringValue(item.label)}</span><StatusTag value={stringValue(item.status)} /></div>)}</div>;
}

function TopologyGlyph({ kind, x, y }: { kind?: string; x: number; y: number }) {
  const value = String(kind ?? '').toLowerCase();
  const transform = `translate(${x} ${y})`;
  if (/(database|storage|cache)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-database" data-node-icon={kind || 'database'} transform={transform}><ellipse cx="0" cy="-3" rx="5" ry="2.2" /><path d="M-5-3v6c0 1.2 2.2 2.2 5 2.2S5 4.2 5 3v-6M-5 0c0 1.2 2.2 2.2 5 2.2S5 1.2 5 0" /></g>;
  }
  if (/(router|switch|network|subnet)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-network" data-node-icon={kind || 'network'} transform={transform}><rect x="-5.5" y="-4" width="11" height="8" rx="1.5" /><path d="M-3-1h2m2 0h2m-6 3h6" /></g>;
  }
  if (/(firewall|security|forensic)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-security" data-node-icon={kind || 'security'} transform={transform}><path d="M0-6 5-4v3.5C5 3 2.8 5.1 0 6-2.8 5.1-5 3-5-.5V-4z" /><path d="m-2 0 1.4 1.5L2.5-2" /></g>;
  }
  if (/(business|service|app)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-application" data-node-icon={kind || 'application'} transform={transform}><rect x="-5" y="-5" width="4" height="4" rx=".8" /><rect x="1" y="-5" width="4" height="4" rx=".8" /><rect x="-5" y="1" width="4" height="4" rx=".8" /><rect x="1" y="1" width="4" height="4" rx=".8" /></g>;
  }
  if (/(probe|sensor|radar)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-probe" data-node-icon={kind || 'probe'} transform={transform}><circle cx="0" cy="0" r="5" /><circle cx="0" cy="0" r="1.5" /><path d="M0-7v2M0 5v2M-7 0h2M5 0h2" /></g>;
  }
  if (/(message|queue|bus|kafka)/.test(value)) {
    return <g className="taf-asset-topology__glyph is-message" data-node-icon={kind || 'message'} transform={transform}><path d="M-5-4h10v3H-5zM-5 1h10v3H-5z" /><circle cx="3" cy="-2.5" r=".7" /><circle cx="3" cy="2.5" r=".7" /></g>;
  }
  return <g className="taf-asset-topology__glyph is-asset" data-node-icon={kind || 'asset'} transform={transform}><rect x="-5" y="-4" width="10" height="8" rx="1.5" /><path d="M-2 6h4M0 4v2" /></g>;
}

function OwnershipSummary({ row }: { row?: SnapshotRow }) {
  const ownership = metadataRecord(row, 'ownership');
  const items = [
    ['园区', stringValue(ownership.campus) || text(row, '园区/部门', '-')],
    ['部门', stringValue(ownership.department) || text(row, '园区/部门', '-')],
    ['负责人', stringValue(ownership.owner) || text(row, '__owner', '未分配')],
    ['业务系统', stringValue(recordArray(ownership.business_systems)[0]?.name) || '-'],
    ['数据域', stringValue(recordArray(ownership.data_domains)[0]?.name) || '-'],
  ];
  return <div className="taf-asset-owner-cards">{items.map(([label, value]) => <div key={label}><span>{label}</span><strong>{value}</strong></div>)}</div>;
}

function ServiceExposure({ services }: { services: Record<string, unknown>[] }) {
  const high = services.filter((item) => stringValue(item.risk_level).includes('高')).length;
  const external = services.filter((item) => stringValue(item.exposure_scope).includes('外网')).length;
  const alerts = services.reduce((sum, item) => sum + numberValue(item.alert_count), 0);
  const sources = services.reduce((sum, item) => sum + numberValue(item.access_source_count), 0);
  return <div className="taf-asset-exposure-cards">{[['开放服务',services.length,'info'],['高危服务',high,'risk'],['外网暴露',external,'warn'],['关联告警',alerts,'risk'],['访问来源',sources,'info']].map(([label,value,tone]) => <div key={String(label)} className={`is-${tone}`}><strong>{String(value)}</strong><span>{String(label)}</span></div>)}</div>;
}

function InterfaceStatusPanel({ title, items }: { title: string; items: Record<string, unknown>[] }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <WorkPanel title={title} className="taf-asset-network-interface-panel">
        <InterfaceStatusMatrix items={items} />
        <Button type="link" size="small" className="taf-asset-panel-view-all" disabled={!items.length} onClick={() => setOpen(true)}>查看全部接口 <RightOutlined /></Button>
      </WorkPanel>
      <Modal title={`${title} · 全部接口`} open={open} footer={null} onCancel={() => setOpen(false)} destroyOnClose width={980}>
        <InterfaceStatusMatrix items={items} />
      </Modal>
    </>
  );
}

function InterfaceStatusMatrix({ items }: { items: Record<string, unknown>[] }) {
  if (!items.length) return <Alert type="info" showIcon message="该设备暂无接口观测数据" />;
  const ports = items.map((item, index) => ({
    number: Number(stringValue(item.name).match(/(\d+)$/)?.[1]) || index + 1,
    status: stringValue(item.status) || 'unknown',
    name: stringValue(item.name) || `接口 ${index + 1}`,
  })).sort((left, right) => left.number - right.number);
  return (
    <div className="taf-asset-interface-matrix">
      <div className="taf-asset-interface-matrix__ports">
        {ports.map((port) => <span key={port.number} className={`is-${port.status}`} title={`${port.name} · ${port.status}`}>{String(port.number).padStart(2, '0')}</span>)}
      </div>
      <div className="taf-asset-interface-matrix__legend"><i className="is-up" />Up<i className="is-down" />Down<i className="is-error" />Err-Disable<i className="is-disabled" />未启用</div>
    </div>
  );
}

function InterfaceSummary({ items }: { items: Record<string, unknown>[] }) {
  const up = items.filter((item) => stringValue(item.status) === 'up').length;
  const down = items.filter((item) => stringValue(item.status) === 'down').length;
  const mirror = items.filter((item) => !['', 'no'].includes(stringValue(item.mirror_mode))).length;
  return <div className="taf-asset-state-cards">{[['接口总数',items.length,'info'],['Up 接口',up,'健康'],['Down 接口',down,'高风险'],['镜像口',mirror,'中风险']].map(([label,value,status]) => <div key={String(label)}><strong>{String(value)}</strong><span>{String(label)}</span><StatusTag value={String(status)} /></div>)}</div>;
}

function BusinessRisk({ row, items }: { row?: SnapshotRow; items: AssetDistributionItem[] }) {
  const score = Math.max(0, Math.min(100, numberValue(metadataOf(row).risk_score)));
  return <AssetDistributionDonutChart items={items} centerLabel={score >= 80 ? '高风险' : score >= 60 ? '中风险' : '低风险'} centerValue={String(score)} ariaLabel={`${text(row, '主机名', '业务系统')}风险评分与风险区间分布`} tone="risk" />;
}

function DependencyHealth({ items }: { items: Record<string, unknown>[] }) {
  const total = items.reduce((sum, item) => sum + numberValue(item.total), 0);
  const abnormal = items.reduce((sum, item) => sum + numberValue(item.abnormal), 0);
  const health = total ? Number((((total - abnormal) / total) * 100).toFixed(1)) : 0;
  return <div className="taf-asset-dependency-health-panel"><MetadataTable maxRows={4} items={items} fields={[['类型','type'],['总数','total'],['异常','abnormal']]} /><AssetMetricRingsChart items={[{ label: '综合健康度', value: health, max: 100, suffix: '%', color: health >= 95 ? '#39c978' : '#ffb020' }]} ariaLabel="业务系统依赖资产综合健康度" /></div>;
}

function BusinessSummary({ row }: { row?: SnapshotRow }) {
  const metadata = metadataOf(row);
  const items = [['业务域',stringValue(metadata.business_domain)],['系统等级',stringValue(metadata.system_level)],['SLA 目标',stringValue(metadata.sla_target)],['当前 SLA',stringValue(metadata.sla_current)],['责任部门',text(row, '园区/部门', '-')],['负责人',text(row, '__owner', '未分配')]];
  return <div className="taf-asset-owner-cards">{items.map(([label,value]) => <div key={label}><span>{label}</span><strong>{value || '-'}</strong></div>)}</div>;
}

function DiscoveryTimeline({ items }: { items: Record<string, unknown>[] }) {
  return <div className="taf-asset-discovery">{items.map((item, index) => <div key={`${stringValue(item.event)}-${index}`} className={stringValue(item.status).includes('完成') ? 'is-ok' : 'is-warn'}><i /><strong>{stringValue(item.event)}</strong><StatusTag value={stringValue(item.status)} /><span>{stringValue(item.time)}</span></div>)}</div>;
}

function Fingerprint({ row }: { row?: SnapshotRow }) {
  const fingerprint = metadataRecord(row, 'fingerprint');
  const labels: Array<[string, string]> = [['MAC OUI','mac_oui'],['DHCP 主机名','dhcp_hostname'],['TTL / OS','ttl_os'],['开放端口','open_ports'],['JA3 指纹','ja3'],['通信特征','behavior']];
  return <div className="taf-asset-fingerprint">{labels.map(([label,key]) => <div key={key}><span>{label}</span><strong>{stringValue(fingerprint[key]) || '-'}</strong></div>)}</div>;
}

function ExposureSummary({ row }: { row?: SnapshotRow }) {
  const exposure = metadataRecord(row, 'exposure');
  const items: Array<[string, string, string]> = [['风险评分','risk_score','risk'],['暴露端口','open_ports','risk'],['高危服务','high_services','warn'],['弱口令','weak_password','risk'],['关联告警','related_alerts','warn'],['识别置信度','confidence','info']];
  return <div className="taf-asset-exposure-cards">{items.map(([label,key,tone]) => <div key={key} className={`is-${tone}`}><strong>{key === 'confidence' ? stringValue(metadataOf(row).confidence) : stringValue(exposure[key])}</strong><span>{label}</span></div>)}</div>;
}

function UnknownExposure({ row, items }: { row?: SnapshotRow; items: AssetDistributionItem[] }) {
  const exposure = metadataRecord(row, 'exposure');
  const top = [
    ['暴露端口', numberValue(exposure.open_ports)],
    ['高危服务', numberValue(exposure.high_services)],
    ['关联告警', numberValue(exposure.related_alerts)],
  ] as const;
  return <div className="taf-asset-unknown-exposure"><AssetDistributionDonutChart items={items} centerLabel="风险评分" centerValue={stringValue(exposure.risk_score) || stringValue(metadataOf(row).risk_score)} ariaLabel="未知资产风险等级分布" tone="risk" /><div>{top.map(([label, value], index) => <span key={label}><em>TOP {index + 1}</em><strong>{label}</strong><b>{value}</b></span>)}</div></div>;
}

function TicketSteps({ row }: { row?: SnapshotRow }) {
  const steps = metadataArray(row, 'ticket_steps').map(stringValue);
  return <div className="taf-asset-ticket">{steps.map((step) => <span key={step}>{step}</span>)}</div>;
}

const metadataOf = (row?: SnapshotRow): Record<string, unknown> => parseJsonRecord(row?.__metadataJson);
const metadataArray = (row: SnapshotRow | undefined, key: string): unknown[] => Array.isArray(metadataOf(row)[key]) ? metadataOf(row)[key] as unknown[] : [];
const metadataList = (row: SnapshotRow | undefined, key: string): Record<string, unknown>[] => metadataArray(row, key).filter(isPlainRecord);
const metadataRecord = (row: SnapshotRow | undefined, key: string): Record<string, unknown> => isPlainRecord(metadataOf(row)[key]) ? metadataOf(row)[key] as Record<string, unknown> : {};
const metadataNumbers = (row: SnapshotRow | undefined, key: string): number[] => metadataArray(row, key).map(numberValue);
const recordNumbers = (value: unknown): unknown[] => Array.isArray(value) ? value : [];
const distributionItems = (row: SnapshotRow | undefined, key: string): AssetDistributionItem[] => metadataList(row, key).map((item) => ({
  label: stringValue(item.label || item.name || item.range),
  value: numberValue(item.value ?? item.count ?? item.total),
  color: stringValue(item.color) || undefined,
  detail: stringValue(item.detail || item.percent_label) || undefined,
})).filter((item) => item.label && item.value >= 0);
const rowArray = (row: SnapshotRow | undefined, key: string): Record<string, unknown>[] => parseJsonArray(row?.[`${key}Json`]).filter(isPlainRecord);
const recordArray = (value: unknown): Record<string, unknown>[] => Array.isArray(value) ? value.filter(isPlainRecord) : [];
const isPlainRecord = (value: unknown): value is Record<string, unknown> => Boolean(value) && typeof value === 'object' && !Array.isArray(value);
const parseJsonRecord = (value: unknown): Record<string, unknown> => {
  if (typeof value !== 'string' || !value) return {};
  try {
    const parsed: unknown = JSON.parse(value);
    return isPlainRecord(parsed) ? parsed : {};
  } catch {
    return {};
  }
};
const parseJsonArray = (value: unknown): unknown[] => {
  if (typeof value !== 'string' || !value) return [];
  try {
    const parsed: unknown = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
};
const stringValue = (value: unknown) => value === undefined || value === null ? '' : String(value);
const numberValue = (value: unknown) => Number.isFinite(Number(value)) ? Number(value) : 0;

function AssetSummaryRail({ context, activeTab, assetId, onClose, onOpenDetail }: {
  context: AssetSelectionContext;
  activeTab: AssetTabSlug;
  assetId: string;
  onClose: () => void;
  onOpenDetail: (detail?: AssetDetailSlug) => void;
}) {
  return (
    <div className="taf-asset-summary-rail">
      <AssetDetailCard context={context} onClose={onClose} />
      <RiskSummary context={context} />
      <AssetGovernanceCard context={context} />
      <AssetActionRail activeTab={activeTab} assetId={assetId} onOpenDetail={onOpenDetail} />
    </div>
  );
}

function AssetDetailCard({ context, onClose }: { context: AssetSelectionContext; onClose: () => void }) {
  return (
    <WorkPanel title={context.tab === 'endpoint' ? '选中终端摘要' : '资产上下文'} extra={<Button size="small" type="text" icon={<CloseOutlined />} aria-label="关闭资产上下文" onClick={onClose} />}>
      <div className="taf-asset-detail-card">
        <div className="taf-asset-detail-card__icon"><AssetTypeIcon kind={context.tab} /></div>
        <div className="taf-asset-detail-card__identity"><strong>{context.name}</strong><span>{context.displayCode}</span></div>
        <StatusTag value={context.risk} />
      </div>
      <dl className="taf-asset-context-list">
        <dt>资产 ID</dt><dd>{context.displayCode}</dd>
        <dt>资产类型</dt><dd>{context.type}</dd>
        <dt>IP / MAC</dt><dd>{context.ip}</dd>
        <dt>主机名</dt><dd>{context.name}</dd>
        <dt>园区 / 部门</dt><dd>{context.location}</dd>
        <dt>操作系统</dt><dd>{context.os}</dd>
        <dt>责任人</dt><dd>{context.owner}</dd>
        <dt>最近活跃</dt><dd>{context.lastSeen}</dd>
      </dl>
    </WorkPanel>
  );
}

function RiskSummary({ context }: { context: AssetSelectionContext }) {
  const metadata = metadataOf(context.sourceRow);
  const evidence = metadataRecord(context.sourceRow, 'evidence');
  const exposure = metadataRecord(context.sourceRow, 'exposure');
  const services = metadataList(context.sourceRow, 'open_services');
  const interfaces = metadataList(context.sourceRow, 'network_interfaces');
  const keyServices = metadataList(context.sourceRow, 'key_services');
  const dependencies = metadataList(context.sourceRow, 'dependency_health');
  const riskScore = Math.min(100, Math.max(0, numberValue(metadata.risk_score)));
  const explicitMetrics = metadataList(context.sourceRow, 'governance_metrics').map((item) => ({
    label: stringValue(item.label),
    value: numberValue(item.value),
    max: Math.max(1, numberValue(item.max)),
    color: stringValue(item.color) || '#1688ff',
    suffix: stringValue(item.suffix) || undefined,
  })).filter((item) => item.label);
  const items: AssetMetricRingItem[] = context.tab === 'business-system'
    ? [
        { label: '风险评分', value: riskScore, max: 100, color: '#ff4d4f' },
        { label: '依赖资产', value: dependencies.reduce((sum, item) => sum + numberValue(item.total), 0), max: 220, color: '#1688ff' },
        { label: '关键服务', value: keyServices.length, max: 30, color: '#7a8ff5' },
        { label: '高风险服务', value: keyServices.filter((item) => stringValue(item.risk).includes('高')).length, max: 20, color: '#ff8a34' },
      ]
    : explicitMetrics.length === 4 ? explicitMetrics : context.tab === 'network-device'
    ? [
        { label: '接口总数', value: interfaces.length, max: Math.max(interfaces.length, 1), color: '#1688ff' },
        { label: 'Up 接口', value: interfaces.filter((item) => stringValue(item.status) === 'up').length, max: Math.max(interfaces.length, 1), color: '#39c978' },
        { label: 'Err-Disable', value: interfaces.filter((item) => stringValue(item.status) === 'error').length, max: 8, color: '#ffb020' },
        { label: 'Down 接口', value: interfaces.filter((item) => stringValue(item.status) === 'down').length, max: 8, color: '#ff4d4f' },
      ]
    : context.tab === 'server'
        ? [
            { label: '暴露端口', value: services.length, max: 20, color: '#1688ff' },
            { label: '高危服务', value: services.filter((item) => stringValue(item.risk_level).includes('高')).length, max: 10, color: '#ff4d4f' },
            { label: '弱口令', value: numberValue(exposure.weak_password), max: 8, color: '#ffb020' },
            { label: '关联告警', value: numberValue(evidence.alerts), max: 30, color: '#7a8ff5' },
          ]
        : context.tab === 'unknown'
          ? [
              { label: '风险评分', value: riskScore, max: 100, color: '#ffb020' },
              { label: '暴露端口', value: numberValue(exposure.open_ports), max: 20, color: '#1688ff' },
              { label: '高危服务', value: numberValue(exposure.high_services), max: 10, color: '#ff4d4f' },
              { label: '关联告警', value: numberValue(exposure.related_alerts), max: 30, color: '#7a8ff5' },
            ]
          : [
              { label: '风险评分', value: riskScore, max: 100, color: '#1688ff' },
              { label: '关联告警', value: numberValue(evidence.alerts), max: 30, color: '#ff4d4f' },
              { label: 'DNS 证据', value: numberValue(evidence.dns), max: 500, color: '#ffb020' },
              { label: 'TLS 会话', value: numberValue(evidence.tls), max: 200, color: '#7a8ff5' },
            ];
  return (
    <WorkPanel title="风险与治理状态">
      <AssetMetricRingsChart items={items} ariaLabel={`${context.name}风险与治理指标`} />
    </WorkPanel>
  );
}

function AssetGovernanceCard({ context }: { context: AssetSelectionContext }) {
  const identityChecks = [
    ['展示编号', context.displayCode],
    ['规范主键', context.id],
    ['IP / MAC', context.ip],
    ['园区 / 部门', context.location],
    ['操作系统', context.os],
    ['责任人', context.owner],
    ['最近活跃', context.lastSeen],
  ] as const;
  const availableCount = identityChecks.filter(([, value]) => !['', '-', '未归属', '未分配'].includes(value)).length;
  const coverage = Math.round((availableCount / identityChecks.length) * 100);
  return (
    <WorkPanel title="关联数据与治理边界" className="taf-asset-governance-panel">
      <div className="taf-asset-governance-summary">
        <div><strong>{coverage}%</strong><span>真实档案覆盖度</span></div>
        <Progress percent={coverage} size="small" showInfo={false} strokeColor="#18a8ff" />
      </div>
      <div className="taf-asset-governance-list">
        {identityChecks.map(([label, value]) => (
          <div key={label}><span>{label}</span><strong title={value}>{value}</strong><StatusTag value={['', '-', '未归属', '未分配'].includes(value) ? '待补充' : '已接入'} /></div>
        ))}
      </div>
      <RailEvidence row={context.sourceRow} />
    </WorkPanel>
  );
}

function RailEvidence({ row }: { row?: SnapshotRow }) {
  const navigate = useNavigate();
  const evidence = metadataRecord(row, 'evidence');
  const items = [['PCAP','pcap'],['Session','session'],['DNS','dns'],['TLS','tls'],['告警','alerts'],['变更','config']];
  return (
    <>
      <div className="taf-asset-rail-evidence-grid" aria-label="关联证据（近7天）">
        {items.map(([label, key]) => {
          const available = Object.prototype.hasOwnProperty.call(evidence, key);
          return <button type="button" key={key} className={available ? '' : 'is-unavailable'} disabled={!row || !available} onClick={() => navigate(evidenceRoute(label, row))}><span>{evidenceIcon(key)}<em>{label}</em></span><strong>{available ? numberValue(evidence[key]) : '未接入'}</strong><RightOutlined /></button>;
        })}
      </div>
      <Button type="link" size="small" className="taf-asset-panel-view-all" disabled={!row} onClick={() => navigate(evidenceRoute('全部证据', row))}>查看更多证据 <RightOutlined /></Button>
    </>
  );
}

function evidenceIcon(key: string) {
  if (key === 'pcap') return <FileOutlined />;
  if (key === 'session') return <ProfileOutlined />;
  if (key === 'dns') return <ClusterOutlined />;
  if (key === 'tls') return <SafetyCertificateOutlined />;
  if (key === 'alerts') return <WarningOutlined />;
  return <ShareAltOutlined />;
}

function evidenceRoute(label: string, row?: SnapshotRow) {
  const assetId = row ? rowKey(row) : '';
  const query = new URLSearchParams({ assetId });
  if (/告警/.test(label)) return `/alerts?${query.toString()}`;
  if (/配置|变更/.test(label)) return `/audit-log?${query.toString()}`;
  if (/DNS/.test(label)) query.set('evidenceType', 'dns');
  else if (/TLS/.test(label)) query.set('evidenceType', 'tls');
  else if (/Session/.test(label)) query.set('evidenceType', 'session');
  else if (/PCAP/.test(label)) query.set('evidenceType', 'pcap');
  return `/forensics?${query.toString()}`;
}

function AssetActionRail({ activeTab, assetId, onOpenDetail }: { activeTab: AssetTabSlug; assetId: string; onOpenDetail: (detail?: AssetDetailSlug) => void }) {
  const navigate = useNavigate();
  return (
    <div className="taf-asset-action-rail">
      <Tooltip title={activeTab === 'server' ? '打开服务器资产详情' : '该分类的详情工作区尚未接入'}>
        <Button size="small" type="primary" icon={<ProfileOutlined />} disabled={activeTab !== 'server'} onClick={() => onOpenDetail('basic')}>打开资产详情</Button>
      </Tooltip>
      <Button size="small" icon={<ClusterOutlined />} onClick={() => navigate(`/graph?assetId=${encodeURIComponent(assetId)}`)}>跳转实体图谱</Button>
      <Tooltip title="整改工单后端尚未接入"><Button size="small" danger disabled icon={<FileSearchOutlined />}>生成整改工单</Button></Tooltip>
      <Button size="small" className="is-forensics" icon={<FileSearchOutlined />} onClick={() => navigate(`/forensics?assetId=${encodeURIComponent(assetId)}`)}>进入取证分析</Button>
    </div>
  );
}

function buildSelectionContext(tab: AssetTabSlug, assetId: string, row?: SnapshotRow): AssetSelectionContext {
  const typeLabels: Record<AssetTabSlug, string> = { endpoint: '终端', server: '服务器', 'network-device': '网络设备', 'business-system': '业务系统', unknown: '未知资产' };
  return {
    tab,
    id: assetId,
    displayCode: text(row, '资产 ID', '-'),
    name: text(row, '主机名', '未命名资产'),
    ip: text(row, 'IP/MAC', '-'),
    type: text(row, '类型', typeLabels[tab]),
    location: text(row, '园区/部门', '未归属'),
    os: text(row, '操作系统', '-'),
    importance: text(row, '重要性', '0'),
    risk: row ? riskLevel(row) : '未评估',
    status: text(row, '__status', '未知'),
    owner: text(row, '__owner', '未分配'),
    lastSeen: formatDateTime(text(row, '最近活跃', '-')),
    sourceRow: row,
  };
}

const renderAssetCell = (column: string, value: unknown): ReactNode => {
  if (column === '资产 ID') return <span className="taf-asset-id">{String(value ?? '')}</span>;
  if (column === '重要性' || column === '风险标签') return <StatusTag value={value} />;
  if (column === '暴露端口') return <strong className="taf-asset-port">{String(value ?? '-')}</strong>;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record.__assetId ?? record['资产 ID'] ?? JSON.stringify(record));
const exportAssetRows = (rows: SnapshotRow[]) => {
  const headers = ['资产 ID', 'IP/MAC', '主机名', '类型', '园区/部门', '操作系统', '重要性', '风险标签', '最近活跃', '暴露端口'];
  const quote = (value: unknown) => `"${String(value ?? '').replace(/"/g, '""')}"`;
  const csv = `\uFEFF${[headers, ...rows.map((row) => headers.map((header) => row[header]))].map((line) => line.map(quote).join(',')).join('\n')}`;
  const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `asset-inventory-${new Date().toISOString().slice(0, 10)}.csv`;
  anchor.click();
  URL.revokeObjectURL(url);
};
const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};
const riskLevel = (row: SnapshotRow | undefined) => {
  const explicit = text(row, '风险标签', '');
  if (explicit) return explicit;
  const criticality = Number(row?.重要性 ?? 0);
  if (criticality >= 80) return '高风险';
  if (criticality >= 50) return '中风险';
  if (criticality > 0) return '低风险';
  return '未评估';
};
const formatDateTime = (value: string) => {
  if (!value || value === '-') return '-';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-');
};
