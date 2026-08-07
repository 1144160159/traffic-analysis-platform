import {
  ArrowLeftOutlined,
  BranchesOutlined,
  CloseOutlined,
  FileSearchOutlined,
  ProfileOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Descriptions, Empty, Space, Spin, Table, Tabs, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { RingChart } from '@/components/charts';
import { AssetTypeIcon } from '@/components/AssetTypeIcon';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { appConfig } from '@/config/runtime';
import {
  fetchAsset,
  fetchAssetDetailSnapshot,
  fetchAssetDetails,
  fetchAssetHistory,
  type AssetDetails,
  type AssetAlertContext,
  type AssetAlertSummary,
  type AssetEvent,
  type AssetNetworkInterface,
  type AssetOpenService,
  type AssetOwnership,
  type AssetOwnershipLink,
  type AssetObservationSummary,
  type AssetGraphProjection,
  type AssetGraphProjectionRelation,
  type AssetEvidenceObjectManifest,
  type AssetEvidenceObjectSet,
  type AssetRecord,
  type AssetResponsibility,
  type AssetTopologyGraph,
} from '@/services/api';
import { assetDetailTabs, type AssetDetailSlug } from './assetInventoryState';

const enabledDetailTabs: AssetDetailSlug[] = ['basic', 'network-interface', 'open-services', 'ownership', 'history'];

const assetTypeLabels: Record<AssetRecord['asset_type'], string> = {
  endpoint: '终端',
  server: '服务器',
  'network-device': '网络设备',
  'business-system': '业务系统',
  unknown: '未知资产',
};

const formatTime = (value?: string) => {
  if (!value) return '-';
  const time = new Date(value);
  return Number.isNaN(time.getTime()) ? value : time.toLocaleString('zh-CN', { hour12: false });
};

const riskLabel = (criticality?: number) => {
  if (!criticality) return '未分级';
  if (criticality <= 5) {
    if (criticality >= 4) return '高风险';
    if (criticality === 3) return '中风险';
    return '低风险';
  }
  if (criticality >= 80) return '高风险';
  if (criticality >= 50) return '中风险';
  return '低风险';
};

const completeness = (asset?: AssetRecord) => {
  if (!asset) return 0;
  const values = [asset.display_code, asset.hostname, asset.ip_address, asset.mac_address, asset.department, asset.campus, asset.owner, asset.source];
  return Math.round((values.filter(Boolean).length / values.length) * 100);
};

const tagValues = (asset?: AssetRecord) => {
  if (!asset?.tags) return [];
  return Object.entries(asset.tags).flatMap(([key, value]) => {
    if (value === false || value === null || value === undefined || value === '') return [];
    if (value === true) return [key];
    if (Array.isArray(value)) return value.map((item) => String(item));
    return [`${key}: ${String(value)}`];
  });
};

export function AssetDetailWorkspace({
  assetId,
  detail,
  onClose,
  onDetailChange,
}: {
  assetId: string;
  detail: AssetDetailSlug;
  onClose: () => void;
  onDetailChange: (detail: AssetDetailSlug) => void;
}) {
  const navigate = useNavigate();
  const snapshotEnabled = appConfig.enableAssetDetailSnapshotV1;
  const snapshotQuery = useQuery({
    queryKey: ['asset-detail-snapshot-v1', assetId],
    queryFn: () => fetchAssetDetailSnapshot(assetId),
    enabled: Boolean(assetId) && snapshotEnabled,
  });
  const assetQuery = useQuery({
    queryKey: ['asset-detail', assetId],
    queryFn: () => fetchAsset(assetId),
    enabled: Boolean(assetId) && !snapshotEnabled,
  });
  const historyQuery = useQuery({
    queryKey: ['asset-history', assetId],
    queryFn: () => fetchAssetHistory(assetId),
    enabled: Boolean(assetId) && !snapshotEnabled && (detail === 'history' || detail === 'ownership'),
  });
  const detailsQuery = useQuery({
    queryKey: ['asset-details', assetId],
    queryFn: () => fetchAssetDetails(assetId),
    enabled: Boolean(assetId) && !snapshotEnabled && detail !== 'basic' && detail !== 'history',
  });
  const snapshotEnvelope = snapshotQuery.data;
  const snapshot = snapshotEnvelope?.data;
  const asset = snapshotEnabled ? snapshot?.asset : assetQuery.data;
  const details = snapshotEnabled ? snapshot?.details : detailsQuery.data;
  const events = snapshotEnabled ? snapshot?.history ?? [] : historyQuery.data ?? [];
  const topology = snapshotEnabled ? snapshot?.topology : undefined;
  const primaryQuery = snapshotEnabled ? snapshotQuery : assetQuery;
  const displayName = asset?.hostname || asset?.display_code || assetId;
  const tags = useMemo(() => tagValues(asset), [asset]);
  const detailAvailable = enabledDetailTabs.includes(detail);

  const refresh = async () => {
    if (snapshotEnabled) {
      await snapshotQuery.refetch();
      return;
    }
    await assetQuery.refetch();
    if (detail === 'history') await historyQuery.refetch();
    if (detail !== 'basic' && detail !== 'history') await detailsQuery.refetch();
  };

  return (
    <section className={`taf-asset-detail-workspace taf-asset-detail-${detail}`} data-breakdown-page-id={`assets-detail-${detail}`}>
      <header className="taf-asset-detail-workspace__title">
        <div>
          <span>资产台账 / {asset?.display_code || assetId}</span>
          <h1>{displayName}</h1>
        </div>
        <Space>
          <Tooltip title="重新读取真实资产数据">
            <Button aria-label="刷新当前资产" loading={primaryQuery.isFetching || historyQuery.isFetching || detailsQuery.isFetching} icon={<ReloadOutlined />} onClick={() => void refresh()} />
          </Tooltip>
          <Tooltip title="返回资产列表"><Button aria-label="返回资产列表" icon={<ArrowLeftOutlined />} onClick={onClose} /></Tooltip>
          <Tooltip title="关闭详情"><Button aria-label="关闭详情" icon={<CloseOutlined />} onClick={onClose} /></Tooltip>
        </Space>
      </header>

      {primaryQuery.isError && (
        <Alert
          showIcon
          type="error"
          message="资产详情读取失败"
          description={primaryQuery.error instanceof Error ? primaryQuery.error.message : '请检查资产服务、鉴权和租户上下文。'}
          action={<Button size="small" danger onClick={() => void primaryQuery.refetch()}>重试</Button>}
        />
      )}

      {snapshotEnabled && snapshotEnvelope ? (
        <SnapshotContractStatus
          snapshotId={snapshotEnvelope.meta.snapshot_id || snapshotEnvelope.data.snapshot_id}
          traceId={snapshotEnvelope.meta.trace_id}
          asOf={snapshotEnvelope.meta.as_of || snapshotEnvelope.data.as_of}
          partial={snapshotEnvelope.meta.partial || snapshotEnvelope.data.partial}
          missingSections={snapshotEnvelope.meta.missing_sections.length ? snapshotEnvelope.meta.missing_sections : snapshotEnvelope.data.missing_sections}
          sourceWatermarks={Object.keys(snapshotEnvelope.meta.source_watermarks).length ? snapshotEnvelope.meta.source_watermarks : snapshotEnvelope.data.source_watermarks}
          topology={topology}
        />
      ) : null}

      {snapshotEnabled && snapshot ? (
        <CrossStoreSnapshotSections
          observations={snapshot.observations}
          alertContext={snapshot.alert_context}
          graphProjection={snapshot.graph_projection}
          evidenceObjects={snapshot.evidence_objects}
          missingSections={snapshot.missing_sections}
        />
      ) : null}

      <div className="taf-asset-detail-workspace__identity">
        <span className="taf-asset-detail-workspace__asset-icon"><AssetTypeIcon kind={asset?.asset_type} /></span>
        <strong>{displayName}</strong>
        <StatusTag value={riskLabel(asset?.criticality)} />
        <StatusTag value={asset?.status || '状态未知'} />
        <span>{asset?.display_code || '-'} · {asset?.ip_address || '-'} · {asset?.os_type || assetTypeLabels[asset?.asset_type ?? 'unknown']}</span>
      </div>

      <DetailMetrics detail={detail} asset={asset} details={details} events={events} />

      <Tabs
        className="taf-asset-detail-workspace__tabs"
        activeKey={detail}
        onChange={(key) => onDetailChange(key as AssetDetailSlug)}
        items={assetDetailTabs.map((item) => ({
          key: item.slug,
          label: item.label,
          disabled: !enabledDetailTabs.includes(item.slug),
        }))}
      />

      <div className="taf-asset-detail-workspace__content">
        {primaryQuery.isLoading ? <div className="taf-asset-detail-loading"><Spin tip="正在读取资产详情" /></div> : null}
        {!primaryQuery.isLoading && !detailAvailable ? <Alert showIcon type="warning" message="未知详情状态" /> : null}
        {!snapshotEnabled && detailsQuery.isError && detail !== 'basic' && detail !== 'history' ? <Alert showIcon type="error" message="资产上下文读取失败" description={detailsQuery.error instanceof Error ? detailsQuery.error.message : '请检查资产详情接口。'} action={<Button size="small" danger onClick={() => void detailsQuery.refetch()}>重试</Button>} /> : null}
        {!primaryQuery.isLoading && detail === 'basic' && asset ? <BasicDetail asset={asset} tags={tags} /> : null}
        {!primaryQuery.isLoading && detail === 'network-interface' ? <NetworkInterfaceDetail rows={details?.network_interfaces ?? []} loading={snapshotEnabled ? snapshotQuery.isLoading : detailsQuery.isLoading} /> : null}
        {!primaryQuery.isLoading && detail === 'open-services' ? <OpenServicesDetail rows={details?.open_services ?? []} loading={snapshotEnabled ? snapshotQuery.isLoading : detailsQuery.isLoading} /> : null}
        {!primaryQuery.isLoading && detail === 'ownership' ? <OwnershipDetail ownership={details?.ownership} loading={snapshotEnabled ? snapshotQuery.isLoading : detailsQuery.isLoading} /> : null}
        {!primaryQuery.isLoading && detail === 'history' ? <HistoryDetail events={events} loading={snapshotEnabled ? snapshotQuery.isLoading : historyQuery.isLoading} error={snapshotEnabled ? null : historyQuery.error} onRetry={() => void (snapshotEnabled ? snapshotQuery.refetch() : historyQuery.refetch())} /> : null}
      </div>

      <footer className="taf-asset-detail-workspace__actions">
        <Button icon={<BranchesOutlined />} onClick={() => navigate(`/graph?assetId=${encodeURIComponent(assetId)}`)}>跳转实体图谱</Button>
        <Button icon={<FileSearchOutlined />} onClick={() => navigate(`/forensics?assetId=${encodeURIComponent(assetId)}`)}>进入取证分析</Button>
        <Button icon={<ProfileOutlined />} onClick={() => onDetailChange('history')}>查看变更历史</Button>
      </footer>
    </section>
  );
}

function CrossStoreSnapshotSections({
  observations,
  alertContext,
  graphProjection,
  evidenceObjects,
  missingSections,
}: {
  observations?: AssetObservationSummary;
  alertContext?: AssetAlertContext;
  graphProjection?: AssetGraphProjection;
  evidenceObjects?: AssetEvidenceObjectSet;
  missingSections: string[];
}) {
  const alertColumns: ColumnsType<AssetAlertSummary> = [
    { title: '告警ID', dataIndex: 'alert_id', key: 'alert_id', width: 160, ellipsis: true },
    { title: '类型', dataIndex: 'alert_type', key: 'alert_type', width: 120, ellipsis: true },
    { title: '级别', dataIndex: 'severity', key: 'severity', width: 86, render: (value: string) => <StatusTag value={value} /> },
    { title: '状态', dataIndex: 'status', key: 'status', width: 90, render: (value: string) => <StatusTag value={value} /> },
    { title: '通信对端', key: 'peer', width: 180, render: (_, row) => `${row.src_ip}:${row.src_port} → ${row.dst_ip}:${row.dst_port}` },
    { title: '证据', key: 'evidence', width: 72, render: (_, row) => row.evidence_ids.length },
    { title: '最后发生', dataIndex: 'last_seen', key: 'last_seen', width: 170, render: formatTime },
  ];
  const relationColumns: ColumnsType<AssetGraphProjectionRelation> = [
    { title: '关系ID', dataIndex: 'relation_id', key: 'relation_id', width: 150, ellipsis: true },
    { title: '源实体', dataIndex: 'source_id', key: 'source_id', width: 150, ellipsis: true },
    { title: '关系', dataIndex: 'relation_type', key: 'relation_type', width: 110 },
    { title: '目标实体', dataIndex: 'target_id', key: 'target_id', width: 150, ellipsis: true },
    { title: '风险', dataIndex: 'risk_level', key: 'risk_level', width: 86, render: (value?: string) => <StatusTag value={value || 'unknown'} /> },
    { title: '证据ID', dataIndex: 'evidence_id', key: 'evidence_id', width: 130, render: emptyDash },
    { title: '观测时间', dataIndex: 'observed_at', key: 'observed_at', width: 170, render: formatTime },
  ];
  const evidenceColumns: ColumnsType<AssetEvidenceObjectManifest> = [
    { title: '证据ID', dataIndex: 'evidence_id', key: 'evidence_id', width: 150, ellipsis: true },
    { title: '类型', dataIndex: 'evidence_type', key: 'evidence_type', width: 88 },
    { title: '对象', key: 'object', width: 240, ellipsis: true, render: (_, row) => `${row.bucket}/${row.object_key}` },
    { title: '大小', dataIndex: 'size_bytes', key: 'size_bytes', width: 90, render: formatBytes },
    { title: '完整性', dataIndex: 'integrity_status', key: 'integrity_status', width: 150, render: (value: string) => <StatusTag value={value === 'verified_metadata' ? 'SHA256已验证元数据' : '缺少SHA256'} /> },
    { title: 'SHA256', dataIndex: 'sha256', key: 'sha256', width: 180, ellipsis: true, render: emptyDash },
    { title: '对象水位', dataIndex: 'last_modified', key: 'last_modified', width: 170, render: formatTime },
  ];
  return (
    <div className="taf-asset-detail-dense-grid" data-testid="asset-cross-store-snapshot">
      <WorkPanel title="ClickHouse 流量观测">
        {missingSections.includes('clickhouse_observations') || !observations ? (
          <Alert showIcon type="warning" message="流量观测分区缺失" description="接口已明确标记 partial；页面不会用资产元数据或零值冒充ClickHouse结果。" />
        ) : (
          <Descriptions size="small" column={1} bordered items={[
            { key: 'identity', label: '解析身份', children: `${observations.resolved_identity.kind}:${observations.resolved_identity.value} @ rev ${observations.resolved_identity.asset_revision}` },
            { key: 'window', label: '查询窗口', children: `${formatTime(observations.window_start)} — ${formatTime(observations.window_end)}` },
            { key: 'sessions', label: '会话数', children: observations.session_count.toLocaleString() },
            { key: 'traffic', label: '流量 / 报文', children: `${formatBytes(observations.bytes_total)} / ${observations.packets_total.toLocaleString()}` },
            { key: 'peers', label: '通信对端', children: observations.distinct_peers.toLocaleString() },
            { key: 'protocols', label: '协议号', children: observations.protocols.join('、') || '窗口内无会话' },
            { key: 'watermark', label: '最后观测', children: formatTime(observations.last_observed_at) },
            { key: 'source', label: '权威来源', children: observations.source },
          ]} />
        )}
      </WorkPanel>
      <WorkPanel title={`告警上下文（${alertContext?.alerts.length ?? 0}）`} className="taf-asset-detail-span-2">
        {missingSections.includes('alert_context') || !alertContext ? (
          <Alert showIcon type="warning" message="告警上下文分区缺失" description="告警读取失败或未启用时保持missing，不根据开放服务或风险标签推导告警。" />
        ) : (
          <>
            {alertContext.truncated ? <Alert showIcon type="warning" message="告警结果已按服务端预算截断" /> : null}
            <Table<AssetAlertSummary>
              size="small"
              rowKey="alert_id"
              columns={alertColumns}
              dataSource={alertContext.alerts}
              pagination={false}
              scroll={{ x: 920 }}
              locale={{ emptyText: '该时间窗口内没有关联告警' }}
            />
          </>
        )}
      </WorkPanel>
      <WorkPanel title={`NebulaGraph资产投影（${graphProjection?.relations.length ?? 0}条关系）`} className="taf-asset-detail-span-3">
        {missingSections.includes('nebulagraph_projection') && !graphProjection ? (
          <Alert showIcon type="warning" message="图投影分区缺失" description="服务端未执行整租户扫描；无当前资产投影时保持partial。" />
        ) : graphProjection ? (
          <Space direction="vertical" size="small" style={{ width: '100%' }}>
            {graphProjection.stale || graphProjection.truncated ? (
              <Alert
                showIcon
                type="warning"
                message={graphProjection.stale ? 'NebulaGraph投影revision落后或超前于PG快照' : 'NebulaGraph关系已按服务端预算截断'}
                description={`PG revision=${graphProjection.postgres_revision}；Nebula revision=${graphProjection.projected_revision}；该分区仍保持partial。`}
              />
            ) : null}
            <Descriptions size="small" column={3} bordered items={[
              { key: 'asset', label: '稳定资产ID', children: graphProjection.asset_id },
              { key: 'revision', label: '投影 / PG revision', children: `${graphProjection.projected_revision} / ${graphProjection.postgres_revision}` },
              { key: 'watermark', label: '投影水位', children: formatTime(graphProjection.updated_at) },
              { key: 'label', label: '图实体', children: graphProjection.label || '-' },
              { key: 'risk', label: '风险', children: <StatusTag value={graphProjection.risk_level || 'unknown'} /> },
              { key: 'source', label: '来源', children: graphProjection.source },
            ]} />
            <Table<AssetGraphProjectionRelation>
              size="small"
              rowKey="relation_id"
              columns={relationColumns}
              dataSource={graphProjection.relations}
              pagination={false}
              scroll={{ x: 980 }}
              locale={{ emptyText: '该资产当前没有NebulaGraph邻接关系' }}
            />
          </Space>
        ) : null}
      </WorkPanel>
      <WorkPanel title={`MinIO证据对象（${evidenceObjects?.objects.length ?? 0}）`} className="taf-asset-detail-span-3">
        {missingSections.includes('evidence_objects') && !evidenceObjects ? (
          <Alert showIcon type="warning" message="证据对象分区缺失" description="未获得ClickHouse证据引用与MinIO对象元数据的可对账结果。" />
        ) : evidenceObjects ? (
          <Space direction="vertical" size="small" style={{ width: '100%' }}>
            {evidenceObjects.partial ? (
              <Alert
                showIcon
                type="warning"
                message="证据对象清单为partial"
                description={`缺失或未验证证据：${evidenceObjects.missing_evidence_ids.join('、') || '结果被截断'}。没有SHA256的对象不会被标记为完整。`}
              />
            ) : null}
            <Table<AssetEvidenceObjectManifest>
              size="small"
              rowKey="evidence_id"
              columns={evidenceColumns}
              dataSource={evidenceObjects.objects}
              pagination={false}
              scroll={{ x: 1100 }}
              locale={{ emptyText: '当前告警上下文没有对象化证据引用' }}
            />
          </Space>
        ) : null}
      </WorkPanel>
    </div>
  );
}

function SnapshotContractStatus({
  snapshotId,
  traceId,
  asOf,
  partial,
  missingSections,
  sourceWatermarks,
  topology,
}: {
  snapshotId: string;
  traceId: string;
  asOf: string;
  partial: boolean;
  missingSections: string[];
  sourceWatermarks: Record<string, string>;
  topology?: AssetTopologyGraph;
}) {
  const watermarkSummary = Object.entries(sourceWatermarks)
    .filter(([, value]) => value)
    .map(([key, value]) => `${key}=${value}`)
    .join('；');
  return (
    <Alert
      showIcon
      type={partial ? 'warning' : 'success'}
      message={partial ? '统一资产快照为 partial，缺失数据未按零值展示' : '统一资产快照完整'}
      description={(
        <Space direction="vertical" size={2}>
          <span>快照 {snapshotId} · trace {traceId || '-'} · 截止 {formatTime(asOf)}</span>
          <span>缺失分区：{missingSections.length ? missingSections.join('、') : '无'}</span>
          <span>PG 拓扑：{topology?.nodes.length ?? 0} 节点 / {topology?.edges.length ?? 0} 边；来源：{topology?.source || '-'}</span>
          <span>水位：{watermarkSummary || '无可用水位'}</span>
        </Space>
      )}
    />
  );
}

function BasicDetail({ asset, tags }: { asset: AssetRecord; tags: string[] }) {
  return (
    <div className="taf-asset-detail-basic-grid">
      <WorkPanel title="基础信息" className="taf-asset-detail-span-2">
        <Descriptions size="small" column={2} bordered items={[
          { key: 'id', label: '规范资产 ID', children: asset.asset_id },
          { key: 'code', label: '展示编号', children: asset.display_code || '-' },
          { key: 'type', label: '资产类型', children: assetTypeLabels[asset.asset_type] },
          { key: 'status', label: '生命周期状态', children: <StatusTag value={asset.status || '未知'} /> },
          { key: 'ip', label: 'IP 地址', children: asset.ip_address || '-' },
          { key: 'mac', label: 'MAC 地址', children: asset.mac_address || '-' },
          { key: 'host', label: '主机名', children: asset.hostname || '-' },
          { key: 'os', label: '操作系统', children: asset.os_type || '-' },
          { key: 'vendor', label: '厂商', children: asset.vendor || '-' },
          { key: 'source', label: '采集来源', children: asset.source || '-' },
          { key: 'network', label: 'VLAN / 交换端口', children: `${asset.vlan_id || '-'} / ${asset.switch_port || '-'}` },
          { key: 'risk', label: '风险等级', children: <StatusTag value={riskLabel(asset.criticality)} /> },
          { key: 'org', label: '园区 / 部门', children: `${asset.campus || '未归属'} / ${asset.department || '未归属'}` },
          { key: 'owner', label: '责任人', children: asset.owner || '未分配' },
          { key: 'first', label: '首次发现', children: formatTime(asset.first_seen) },
          { key: 'last', label: '最近活跃', children: formatTime(asset.last_seen) },
        ]} />
      </WorkPanel>
      <WorkPanel title="档案完整度"><RingChart value={completeness(asset)} height={210} ariaLabel="资产档案完整度" /></WorkPanel>
      <WorkPanel title="标签与数据边界" className="taf-asset-detail-span-3">
        {tags.length ? <div className="taf-asset-detail-tags">{tags.map((item) => <StatusTag key={item} value={item} />)}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前资产尚无真实标签" />}
      </WorkPanel>
    </div>
  );
}

function DetailMetrics({
  detail,
  asset,
  details,
  events,
}: {
  detail: AssetDetailSlug;
  asset?: AssetRecord;
  details?: AssetDetails;
  events: AssetEvent[];
}) {
  const interfaces = details?.network_interfaces ?? [];
  const services = details?.open_services ?? [];
  const ownership = details?.ownership;
  const metrics = detail === 'network-interface'
    ? [
        [interfaces.length, '网卡', 'is-info'],
        [interfaces.filter((item) => item.status === 'up').length, '在线接口', 'is-ok'],
        [interfaces.filter((item) => item.mirror_mode !== 'no').length, '镜像口', 'is-info'],
        [interfaces.filter((item) => item.ip_address).length, 'IP 绑定', 'is-info'],
        [new Set(interfaces.map((item) => item.vlan_id).filter(Boolean)).size, 'VLAN', 'is-warn'],
        [interfaces.filter((item) => item.status === 'down' || item.error_count > 20).length, '异常接口', 'is-risk'],
      ]
    : detail === 'open-services'
      ? [
          [services.length, '开放端口', 'is-info'],
          [services.filter((item) => item.exposure_scope.includes('外网')).length, '外网暴露', 'is-risk'],
          [services.filter((item) => item.risk_level === '高危').length, '高危服务', 'is-risk'],
          [services.reduce((sum, item) => sum + item.alert_count, 0), '关联告警', 'is-warn'],
          [services.reduce((sum, item) => sum + item.access_source_count, 0), '访问来源', 'is-info'],
          [new Set(services.map((item) => item.service)).size, '服务类型', 'is-ok'],
        ]
      : detail === 'ownership'
        ? [
            [ownership?.responsibilities.length ?? 0, '责任角色', 'is-info'],
            [ownership?.business_systems.length ?? 0, '业务系统', 'is-ok'],
            [ownership?.asset_groups.length ?? 0, '资产组', 'is-warn'],
            [ownership?.data_domains.length ?? 0, '数据域', 'is-info'],
            [ownership?.pending_fields.length ?? 0, '待确认字段', 'is-warn'],
        [events.length, '近期开变更', 'is-info'],
          ]
        : detail === 'history'
          ? [
              [events.length, '变更总数', 'is-info'],
              [new Set(events.map((event) => event.event_type)).size, '变更类型', 'is-ok'],
              [events.filter((event) => event.event_type.includes('governance')).length, '归属变更', 'is-warn'],
              [events.filter((event) => event.event_type.includes('service')).length, '服务变更', 'is-risk'],
              [events.filter((event) => event.old_value).length, '可对比', 'is-ok'],
              [events.filter((event) => !event.old_value).length, '待补审计', 'is-warn'],
            ]
          : [
              [`${completeness(asset)}%`, '档案完整度', 'is-info'],
              [asset?.owner || '未分配', '责任人', 'is-ok'],
              [asset?.criticality || 0, '重要性等级', 'is-warn'],
              [asset?.status || '-', '生命周期', 'is-info'],
            ];
  return (
    <div className={`taf-asset-detail-workspace__metrics is-${metrics.length}`}>
      {metrics.map(([value, label, tone]) => <div key={String(label)} className={String(tone)}><strong>{value}</strong><span>{label}</span></div>)}
    </div>
  );
}

function NetworkInterfaceDetail({ rows, loading }: { rows: AssetNetworkInterface[]; loading: boolean }) {
  const columns: ColumnsType<AssetNetworkInterface> = [
    { title: '接口', dataIndex: 'name', key: 'name', width: 84 },
    { title: '网卡名称', dataIndex: 'adapter', key: 'adapter', width: 150, ellipsis: true },
    { title: 'IP', dataIndex: 'ip_address', key: 'ip_address', width: 120, render: emptyDash },
    { title: 'MAC', dataIndex: 'mac_address', key: 'mac_address', width: 150, render: emptyDash },
    { title: 'VLAN', dataIndex: 'vlan_id', key: 'vlan_id', width: 72, render: emptyDash },
    { title: '镜像口', dataIndex: 'mirror_mode', key: 'mirror_mode', width: 82, render: (value: string) => value === 'no' ? '否' : value },
    { title: '接口状态', dataIndex: 'status', key: 'status', width: 96, render: (value: string) => <StatusTag value={value === 'up' ? '在线' : value === 'monitor' ? '监控中' : '离线'} /> },
    { title: '速率/双工', key: 'speed', width: 112, render: (_, row) => `${row.speed} / ${row.duplex}` },
    { title: '入站/出站', key: 'traffic', width: 145, render: (_, row) => `${formatBytes(row.ingress_bytes)} / ${formatBytes(row.egress_bytes)}` },
    { title: '丢包/错误', key: 'errors', width: 105, render: (_, row) => `${row.packet_loss_pct}% / ${row.error_count}` },
  ];
  const vlanRows = rows.filter((row) => row.vlan_id);
  return (
    <div className="taf-asset-detail-dense-grid">
      <WorkPanel title={`网络接口表（${rows.length} 项）`} className="taf-asset-detail-span-3">
        <Table size="small" loading={loading} rowKey="name" columns={columns} dataSource={rows} pagination={false} scroll={{ x: 1120 }} locale={{ emptyText: '该资产暂无接口观测' }} />
      </WorkPanel>
      <WorkPanel title="接口状态灯">
        <div className="taf-asset-interface-lights">{rows.map((row) => <span key={row.name} className={`is-${row.status}`}><strong>{row.name}</strong><StatusTag value={row.status === 'up' ? 'Up' : row.status === 'monitor' ? 'Monitor' : 'Down'} /></span>)}</div>
      </WorkPanel>
      <WorkPanel title="吞吐观测">
        <div className="taf-asset-detail-bars">{rows.slice(0, 5).map((row) => <div key={row.name}><span>{row.name}</span><progress max={Math.max(row.ingress_bytes + row.egress_bytes, 1)} value={row.ingress_bytes} /><em>{formatBytes(row.ingress_bytes + row.egress_bytes)}</em></div>)}</div>
      </WorkPanel>
      <WorkPanel title="VLAN 与 IP 绑定">
        <div className="taf-asset-detail-mini-table">{vlanRows.map((row) => <div key={row.name}><strong>VLAN {row.vlan_id}</strong><span>{row.ip_address || '-'}</span><em>{row.name}</em><StatusTag value={row.ip_address ? '已绑定' : '监控用'} /></div>)}</div>
      </WorkPanel>
      <WorkPanel title="镜像口与采集链路">
        <div className="taf-asset-detail-mini-table">{rows.filter((row) => row.mirror_mode !== 'no').map((row) => <div key={row.name}><strong>{row.name}</strong><span>{row.mirror_mode}</span><em>{row.probe_id}</em><StatusTag value="正常" /></div>)}</div>
      </WorkPanel>
      <WorkPanel title="异常与告警">
        <div className="taf-asset-detail-risk-list">{rows.filter((row) => row.error_count > 0).map((row) => <span key={row.name}><strong>{row.name}</strong><em>错误 {row.error_count} · 丢包 {row.packet_loss_pct}%</em><StatusTag value={row.error_count > 20 ? '高风险' : '中风险'} /></span>)}</div>
      </WorkPanel>
      <WorkPanel title="数据契约与观测口径">
        <Alert showIcon type="info" message="接口观测来自资产服务持久化详情" description="状态、速率、流量、VLAN、镜像和错误均由 /v1/assets/{id}/details 返回；空值保持真实空态。" />
      </WorkPanel>
    </div>
  );
}

function OpenServicesDetail({ rows, loading }: { rows: AssetOpenService[]; loading: boolean }) {
  const columns: ColumnsType<AssetOpenService> = [
    { title: '端口', dataIndex: 'port', key: 'port', width: 72 },
    { title: '协议', dataIndex: 'protocol', key: 'protocol', width: 78 },
    { title: '服务', dataIndex: 'service', key: 'service', width: 120 },
    { title: '版本', dataIndex: 'version', key: 'version', width: 145, ellipsis: true },
    { title: '暴露范围', dataIndex: 'exposure_scope', key: 'exposure_scope', width: 116 },
    { title: '访问来源', dataIndex: 'access_source_count', key: 'access_source_count', width: 96, render: (value: number) => `${value} 个 IP` },
    { title: '风险标签', dataIndex: 'risk_level', key: 'risk_level', width: 92, render: (value: string) => <StatusTag value={value} /> },
    { title: '关联告警', dataIndex: 'alert_count', key: 'alert_count', width: 90 },
  ];
  return (
    <div className="taf-asset-detail-dense-grid">
      <WorkPanel title={`开放服务表（${rows.length} 项）`} className="taf-asset-detail-span-2">
        <Table size="small" loading={loading} rowKey={(row) => `${row.protocol}-${row.port}`} columns={columns} dataSource={rows} pagination={false} scroll={{ x: 900 }} locale={{ emptyText: '该资产暂无开放服务观测' }} />
      </WorkPanel>
      <WorkPanel title="端口矩阵">
        <div className="taf-asset-port-matrix">{['0–255', '256–511', '512–1023', '1024+'].map((range, index) => <span key={range} className={index < 2 ? 'is-risk' : 'is-info'}><strong>{range}</strong><b>{rows.filter((row) => index === 0 ? row.port <= 255 : index === 1 ? row.port >= 256 && row.port <= 511 : index === 2 ? row.port >= 512 && row.port <= 1023 : row.port >= 1024).length}</b><small>TCP/UDP</small></span>)}</div>
      </WorkPanel>
      <WorkPanel title="暴露面分析">
        <div className="taf-asset-detail-risk-list">{rows.slice(0, 5).map((row) => <span key={row.port}><strong>{row.service}:{row.port}</strong><em>{row.exposure_scope}</em><StatusTag value={row.risk_level} /></span>)}</div>
      </WorkPanel>
      <WorkPanel title="高风险服务复核">
        <div className="taf-asset-detail-risk-list">{rows.filter((row) => row.risk_level === '高危').map((row) => <span key={row.port}><strong>{row.service} 风险复核</strong><em>{row.version}</em><StatusTag value="待核验" /></span>)}</div>
      </WorkPanel>
      <WorkPanel title="服务告警入口">
        <div className="taf-asset-detail-mini-table">{rows.filter((row) => row.alert_count).map((row) => <div key={row.port}><strong>{row.service}</strong><span>{row.port}/{row.protocol}</span><em>{row.alert_count} 条</em><StatusTag value={row.alert_count >= 5 ? '高风险' : '中风险'} /></div>)}</div>
      </WorkPanel>
      <WorkPanel title="访问来源观测">
        <div className="taf-asset-detail-bars">{rows.slice(0, 6).map((row) => <div key={row.port}><span>{row.service}</span><progress max={30} value={row.access_source_count} /><em>{row.access_source_count} 来源</em></div>)}</div>
      </WorkPanel>
    </div>
  );
}

function OwnershipDetail({ ownership, loading }: { ownership?: AssetOwnership; loading: boolean }) {
  if (loading) return <div className="taf-asset-detail-loading"><Spin tip="正在读取归属信息" /></div>;
  if (!ownership) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该资产暂无归属信息" />;
  return (
    <div className="taf-asset-detail-dense-grid">
      <WorkPanel title="归属信息卡">
        <Descriptions size="small" column={1} bordered items={[
          { key: 'campus', label: '园区', children: ownership.campus || '待确认' },
          { key: 'department', label: '部门', children: ownership.department || '待确认' },
          { key: 'owner', label: '责任人', children: ownership.owner || '待确认' },
          { key: 'group', label: '资产组', children: ownership.asset_groups.map((item) => item.name).join(' / ') || '待确认' },
          { key: 'domain', label: '数据域', children: ownership.data_domains.map((item) => item.name).join(' / ') || '待确认' },
        ]} />
      </WorkPanel>
      <WorkPanel title="责任角色" className="taf-asset-detail-span-2"><ResponsibilityList rows={ownership.responsibilities} /></WorkPanel>
      <WorkPanel title="业务系统与依赖归属"><OwnershipLinkList rows={ownership.business_systems} /></WorkPanel>
      <WorkPanel title="资产组"><OwnershipLinkList rows={ownership.asset_groups} /></WorkPanel>
      <WorkPanel title="数据域"><OwnershipLinkList rows={ownership.data_domains} /></WorkPanel>
      <WorkPanel title="组织树与责任边界" className="taf-asset-detail-span-2">
        <div className="taf-asset-ownership-tree"><strong>{ownership.campus || '待确认园区'}</strong><span>信息化办公室</span><b>{ownership.department || '待确认部门'}</b>{ownership.responsibilities.map((item) => <em key={item.role}>{item.role}：{item.owner}</em>)}</div>
      </WorkPanel>
      <WorkPanel title={`待确认字段（${ownership.pending_fields.length}）`}>
        <div className="taf-asset-detail-risk-list">{ownership.pending_fields.map((field) => <span key={field}><strong>{field}</strong><StatusTag value="待确认" /></span>)}</div>
      </WorkPanel>
    </div>
  );
}

function OwnershipLinkList({ rows }: { rows: AssetOwnershipLink[] }) {
  return <div className="taf-asset-detail-mini-table">{rows.map((row) => <div key={`${row.name}-${row.role}`}><strong>{row.name}</strong><span>{row.role}</span><em>{row.owner}</em><StatusTag value={row.status} /></div>)}</div>;
}

function ResponsibilityList({ rows }: { rows: AssetResponsibility[] }) {
  return <div className="taf-asset-responsibilities">{rows.map((row) => <span key={row.role}><strong>{row.role}</strong><em>{row.owner}</em><StatusTag value={row.status} /></span>)}</div>;
}

const emptyDash = (value: unknown) => value === undefined || value === null || value === '' ? '-' : String(value);

const formatBytes = (value: number) => {
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GB`;
  if (value >= 1024 ** 2) return `${(value / 1024 ** 2).toFixed(0)} MB`;
  return `${value} B`;
};

function HistoryDetail({
  events,
  loading,
  error,
  onRetry,
}: {
  events: AssetEvent[];
  loading: boolean;
  error: Error | null;
  onRetry: () => void;
}) {
  const columns: ColumnsType<AssetEvent> = [
    { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 180, render: formatTime },
    { title: '事件类型', dataIndex: 'event_type', key: 'event_type', width: 180, render: (value: string) => <StatusTag value={value} /> },
    { title: '变更前', dataIndex: 'old_value', key: 'old_value', ellipsis: true, render: (value?: string) => value || '-' },
    { title: '变更后', dataIndex: 'new_value', key: 'new_value', ellipsis: true, render: (value?: string) => value || '-' },
    { title: '事件 ID', dataIndex: 'event_id', key: 'event_id', width: 100 },
  ];
  const changedTypes = new Set(events.map((event) => event.event_type)).size;
  const latest = events[0]?.created_at;

  if (error) return <Alert showIcon type="error" message="变更历史读取失败" description={error.message} action={<Button size="small" danger onClick={onRetry}>重试</Button>} />;

  return (
    <div className="taf-asset-detail-layout">
      <WorkPanel title="真实变更记录" className="taf-asset-detail-span-3">
        <Table<AssetEvent>
          size="small"
          loading={loading}
          rowKey={(event) => String(event.event_id)}
          columns={columns}
          dataSource={events}
          pagination={{ pageSize: 8, size: 'small', hideOnSinglePage: true }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该资产暂无变更事件" /> }}
          scroll={{ x: 900 }}
        />
      </WorkPanel>
      <WorkPanel title="历史摘要">
        <Descriptions size="small" column={1} bordered items={[
          { key: 'count', label: '已加载事件', children: events.length },
          { key: 'types', label: '事件类型', children: changedTypes },
          { key: 'latest', label: '最近变更', children: formatTime(latest) },
          { key: 'source', label: '数据来源', children: '/v1/assets/{id}/history' },
        ]} />
      </WorkPanel>
      <WorkPanel title="审计说明" className="taf-asset-detail-span-2">
        <Alert showIcon type="info" message="历史页仅展示资产服务已持久化事件" description="当前接口不返回操作者、审批或回滚能力，页面不会自行推断这些字段；相关动作在后端契约完成前保持不可用。" />
      </WorkPanel>
    </div>
  );
}
