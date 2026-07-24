import {
  AimOutlined,
  AlertOutlined,
  ApartmentOutlined,
  ArrowRightOutlined,
  BranchesOutlined,
  ClusterOutlined,
  CloseOutlined,
  DatabaseOutlined,
  ExpandOutlined,
  FileSearchOutlined,
  GlobalOutlined,
  HistoryOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SearchOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Input, Segmented, Select, Space, Tabs, Tooltip } from 'antd';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { EntityTopologyGraph, QueryHistoryBarChart, RiskScoreRingChart, SparklineChart } from '@/components/charts';
import type { NavRoute } from '@/routes/routeManifest';
import {
  fetchAsset,
  fetchEntityGraphWorkbench,
  fetchEntityGraphWorkbenchPath,
  type EntityGraphWorkbenchEdge,
  type EntityGraphWorkbenchFilters,
  type EntityGraphWorkbench,
  type EntityGraphWorkbenchNode,
  type EntityGraphWorkbenchPath,
} from '@/services/api';

type GraphQueryHistoryItem = {
  id: string;
  label: string;
  duration_ms: number;
  node_count: number;
  edge_count: number;
  created_at: string;
};

export function GraphEntityPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const sourceAssetId = searchParams.get('assetId') ?? '';
  const [activePath, setActivePath] = useState(route.page.tabs[0]);
  const [selectedNodeId, setSelectedNodeId] = useState<string>();
  const [searchValue, setSearchValue] = useState('');
  const [feedback, setFeedback] = useState<string>();
  const [timeRange, setTimeRange] = useState<EntityGraphWorkbenchFilters['timeRange']>('24h');
  const [site, setSite] = useState<EntityGraphWorkbenchFilters['site']>('main');
  const [entityType, setEntityType] = useState<EntityGraphWorkbenchFilters['entityType']>('all');
  const [depth, setDepth] = useState<EntityGraphWorkbenchFilters['depth']>(2);
  const [detailOpen, setDetailOpen] = useState(true);
  const [canvasVersion, setCanvasVersion] = useState(0);
  const [queryHistory, setQueryHistory] = useState<GraphQueryHistoryItem[]>(() => {
    try {
      const stored = JSON.parse(localStorage.getItem('traffic-graph-query-history') || '[]');
      return Array.isArray(stored) ? stored.slice(0, 8) : [];
    } catch {
      return [];
    }
  });
  const sourceAsset = useQuery({
    queryKey: ['asset', sourceAssetId],
    queryFn: () => fetchAsset(sourceAssetId),
    enabled: Boolean(sourceAssetId),
  });
  const centerId = sourceAsset.data?.ip_address ? `host:${sourceAsset.data.ip_address}` : 'host:10.20.4.18';
  const filters = useMemo<EntityGraphWorkbenchFilters>(() => ({ timeRange, site, entityType, depth }), [depth, entityType, site, timeRange]);
  const graphQuery = useQuery({
    queryKey: ['entity-graph-workbench', centerId, filters],
    queryFn: () => fetchEntityGraphWorkbench(centerId, filters),
    enabled: !sourceAssetId || Boolean(sourceAsset.data?.ip_address),
  });
  const graph = graphQuery.data;
  useEffect(() => {
    if (!graphQuery.dataUpdatedAt || !graph?.meta) return;
    const centerLabel = graph.nodes.find((node) => node.entity_id === graph.center_id)?.label ?? graph.center_id;
    const entityScope = graph.meta.entity_type === 'all' ? '关系分析' : `${entityTypeLabel(graph.meta.entity_type)}筛选`;
    const item: GraphQueryHistoryItem = {
      id: `${graphQuery.dataUpdatedAt}-${graph.meta.entity_type}-${graph.meta.depth}`,
      label: `${centerLabel} · ${entityScope}`,
      duration_ms: graph.meta.query_duration_ms,
      node_count: graph.meta.node_count,
      edge_count: graph.meta.edge_count,
      created_at: new Date(graphQuery.dataUpdatedAt).toISOString(),
    };
    setQueryHistory((current) => {
      if (current.some((entry) => entry.id === item.id)) return current;
      const next = [item, ...current].slice(0, 8);
      localStorage.setItem('traffic-graph-query-history', JSON.stringify(next));
      return next;
    });
  }, [graph, graphQuery.dataUpdatedAt]);
  const visibleNodes = useMemo(() => {
    const query = searchValue.trim().toLowerCase();
    if (!query) return graph?.nodes ?? [];
    return (graph?.nodes ?? []).filter((node) => `${node.label} ${node.detail} ${node.entity_type}`.toLowerCase().includes(query));
  }, [graph?.nodes, searchValue]);
  const visibleNodeIds = useMemo(() => new Set(visibleNodes.map((node) => node.entity_id)), [visibleNodes]);
  const visibleEdges = useMemo(
    () => (graph?.edges ?? []).filter((edge) => visibleNodeIds.has(edge.source_id) && visibleNodeIds.has(edge.target_id)),
    [graph?.edges, visibleNodeIds],
  );
  const selectedNode = useMemo(() => {
    if (!graph?.nodes.length) return undefined;
    return graph.nodes.find((node) => node.entity_id === selectedNodeId)
      ?? graph.nodes.find((node) => node.entity_id === graph.center_id)
      ?? graph.nodes[0];
  }, [graph, selectedNodeId]);
  const pathMode = pathModeForTab(activePath);
  const pathSourceNode = useMemo(() => choosePathSource(graph?.nodes ?? [], graph?.center_id, pathMode, selectedNode), [graph?.center_id, graph?.nodes, pathMode, selectedNode]);
  const pathTargetNode = useMemo(() => choosePathTarget(graph?.nodes ?? [], graph?.center_id, pathMode, selectedNode), [graph?.center_id, graph?.nodes, pathMode, selectedNode]);
  const pathTargetId = pathTargetNode?.entity_id ?? graph?.center_id ?? centerId;
  const pathQuery = useQuery({
    queryKey: ['entity-graph-workbench-path', pathMode, pathSourceNode?.entity_id, pathTargetId, selectedNode?.entity_id, filters],
    queryFn: () => fetchEntityGraphWorkbenchPath({
      sourceId: pathSourceNode?.entity_id ?? '',
      targetId: pathTargetId,
      anchorId: selectedNode?.entity_id,
      mode: pathMode,
      maxDepth: depth,
      filters,
    }),
    enabled: Boolean(pathSourceNode?.entity_id && pathTargetId && pathSourceNode?.entity_id !== pathTargetId),
  });

  const saveView = () => {
    localStorage.setItem('traffic-graph-saved-view', JSON.stringify({ center_id: graph?.center_id, search: searchValue, filters, saved_at: new Date().toISOString() }));
    setFeedback('当前图谱视图已保存到本地工作区。');
  };

  const exportEvidence = () => {
    if (!graph) return;
    const blob = new Blob([JSON.stringify(graph, null, 2)], { type: 'application/json' });
    const href = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = href;
    link.download = `entity-graph-${graph.center_id.replace(/[^a-zA-Z0-9.-]+/g, '-')}.json`;
    link.click();
    URL.revokeObjectURL(href);
    setFeedback('实体、关系与证据索引已导出。');
  };

  const showPathAnalysis = () => {
    void pathQuery.refetch();
    document.querySelector('.taf-graph-path-results')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    setFeedback(`正在刷新${activePath}，结果将在当前面板内更新。`);
  };

  return (
    <div className="taf-page taf-graph-entity">
      <header className="taf-graph-titlebar">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Tooltip title="定位中心节点">
            <Button size="small" icon={<AimOutlined />} onClick={() => { setSelectedNodeId(graph?.center_id); setDetailOpen(true); }}>定位中心节点</Button>
          </Tooltip>
          <Tooltip title="刷新图谱">
            <Button size="small" icon={<SearchOutlined />} onClick={() => void graphQuery.refetch()} />
          </Tooltip>
          {!detailOpen && <Button size="small" onClick={() => setDetailOpen(true)}>显示实体详情</Button>}
        </Space>
      </header>

      {graphQuery.isError && (
        <Alert
          type="error"
          showIcon
          message="实体图谱工作台数据加载失败"
          description={graphQuery.error instanceof Error ? graphQuery.error.message : '请检查 APISIX 图谱路由、Graph Service、NebulaGraph 集群或鉴权状态。'}
          action={
            <Button size="small" danger onClick={() => void graphQuery.refetch()}>
              重试
            </Button>
          }
        />
      )}

      {sourceAssetId && <Alert showIcon type={sourceAsset.isError ? 'error' : 'info'} message={sourceAsset.isError ? '资产上下文解析失败' : '已接收资产台账上下文'} description={sourceAsset.isError ? '无法从资产服务解析中心实体 IP。' : `中心实体资产 ID：${sourceAssetId}${sourceAsset.data?.ip_address ? ` · IP：${sourceAsset.data.ip_address}` : ' · 正在解析 IP'}`} />}

      <div className={`taf-graph-grid ${detailOpen ? '' : 'is-detail-closed'}`}>
        <main className="taf-graph-main">
          <div className="taf-graph-toolbar">
            <Input prefix={<SearchOutlined />} placeholder="搜索 IP / 账号 / 主机 / 域名 / 服务 / 告警 ID / 资产 ID" value={searchValue} onChange={(event) => setSearchValue(event.target.value)} allowClear />
            <Select size="small" value={timeRange} onChange={setTimeRange} options={[{ value: '24h', label: '近24小时' }, { value: '7d', label: '近7天' }, { value: 'all', label: '全部时间' }]} />
            <Select size="small" value={site} onChange={setSite} options={[{ value: 'main', label: '主园区' }, { value: 'all', label: '全部园区' }]} />
            <Select size="small" value={entityType} onChange={setEntityType} options={[{ value: 'all', label: '实体类型：全部' }, { value: 'host', label: '主机' }, { value: 'ip', label: 'IP地址' }, { value: 'account', label: '账号' }, { value: 'domain', label: '域名' }, { value: 'service', label: '服务' }, { value: 'alert', label: '告警' }, { value: 'evidence', label: '证据' }]} />
            <Button size="small" type="primary" icon={<BranchesOutlined />} onClick={showPathAnalysis}>路径分析</Button>
            <Button size="small" onClick={saveView}>保存视图</Button>
            <Button size="small" onClick={exportEvidence} disabled={!graph}>导出证据</Button>
          </div>
          {feedback && <div className="taf-graph-inline-feedback" role="status"><SafetyCertificateOutlined />{feedback}<button type="button" onClick={() => setFeedback(undefined)}>关闭</button></div>}
          <WorkPanel
            title="邻居图谱"
            extra={
              <Space size={6}>
                <Segmented
                  aria-label="关系深度"
                  className="taf-graph-depth-switch"
                  size="small"
                  value={depth}
                  onChange={(value) => setDepth(value as EntityGraphWorkbenchFilters['depth'])}
                  options={[
                    { value: 1, label: '一跳' },
                    { value: 2, label: '二跳' },
                    { value: 3, label: '三跳' },
                  ]}
                />
                <span className="taf-graph-query-stat">数据源 {graph?.meta.source === 'nebula_graph' ? 'NebulaGraph' : '图谱服务'}</span>
                <span className="taf-graph-query-stat">节点 {visibleNodes.length}</span>
                <span className="taf-graph-query-stat">关系 {visibleEdges.length}</span>
                <Select
                  aria-label="选择实体"
                  size="small"
                  className="taf-graph-node-select"
                  value={selectedNode?.entity_id}
                  onChange={(nodeId) => { setSelectedNodeId(nodeId); setDetailOpen(true); }}
                  options={visibleNodes.map((node) => ({ value: node.entity_id, label: node.label }))}
                />
                <Tooltip title="适配当前图谱视图"><Button size="small" aria-label="适配当前图谱视图" icon={<ExpandOutlined />} onClick={() => setCanvasVersion((current) => current + 1)} /></Tooltip>
              </Space>
            }
          >
            <GraphCanvas key={canvasVersion} nodes={visibleNodes} edges={visibleEdges} centerId={graph?.center_id} selectedNodeId={selectedNode?.entity_id} onSelect={(nodeId) => { setSelectedNodeId(nodeId); setDetailOpen(true); }} />
          </WorkPanel>

          <div className="taf-graph-bottom">
            <WorkPanel title="路径分析结果" className="taf-graph-path-results">
              <Tabs
                className="taf-graph-tabs"
                activeKey={activePath}
                onChange={setActivePath}
                items={route.page.tabs.map((tab) => ({ key: tab, label: tab }))}
              />
              <PathResultView
                tab={activePath}
                path={pathQuery.data}
                graph={graph}
                loading={pathQuery.isLoading || pathQuery.isFetching}
                error={pathQuery.isError}
                onRetry={() => void pathQuery.refetch()}
                navigate={navigate}
              />
            </WorkPanel>

            <WorkPanel title="查询治理" className="taf-graph-query-governance-panel">
              <QueryGovernance graph={graph} history={queryHistory} />
            </WorkPanel>
          </div>
        </main>

        {detailOpen && <aside className="taf-graph-detail">
          <EntityDetail node={selectedNode} edges={graph?.edges ?? []} timeRange={timeRange} onClose={() => setDetailOpen(false)} />
          <WorkPanel title="关联证据" className="taf-graph-evidence-panel">
            <EvidenceList edges={visibleEdges} navigate={navigate} />
          </WorkPanel>
          <GraphActionRail node={selectedNode} navigate={navigate} />
        </aside>}
      </div>
    </div>
  );
}

function GraphCanvas({
  nodes,
  edges,
  centerId,
  selectedNodeId,
  onSelect,
}: {
  nodes: EntityGraphWorkbenchNode[];
  edges: EntityGraphWorkbenchEdge[];
  centerId?: string;
  selectedNodeId?: string;
  onSelect: (nodeId: string) => void;
}) {
  const topologyEdges = edges;
  const layoutNodes = layeredTopologyNodes(nodes, topologyEdges, centerId);
  return (
    <div className="taf-graph-canvas">
      <div className="taf-graph-legend">
        <strong>节点类型</strong>
        {[
          ['IP地址', <GlobalOutlined />],
          ['主机', <DatabaseOutlined />],
          ['账号', <UserOutlined />],
          ['域名', <ClusterOutlined />],
          ['服务', <NodeIndexOutlined />],
          ['告警', <AlertOutlined />],
          ['证据', <FileSearchOutlined />],
        ].map(([item, icon]) => <span key={String(item)}>{icon}{item}</span>)}
        <strong>关系类型</strong>
        {['通信', '登录', 'DNS解析', '行为服务', '关联告警', '证据引用'].map((item) => <span key={item} className="is-relation">{item}</span>)}
        <strong>风险等级</strong>
        {['高风险', '中风险', '低风险', '未知'].map((item) => <span key={item} className="is-risk-level">{item}</span>)}
      </div>
      <div className="taf-graph-echart">
        <EntityTopologyGraph
          ariaLabel={`邻居图谱，共 ${layoutNodes.length} 个节点、${topologyEdges.length} 条关系`}
          nodes={layoutNodes.map(({ node, x, y, hop }) => ({
            id: node.entity_id,
            label: node.label,
            detail: node.detail,
            x,
            y,
            hop,
            riskLevel: node.risk_level,
            entityType: node.entity_type,
            icon: node.icon,
            center: node.entity_id === centerId,
            selected: node.entity_id === selectedNodeId,
          }))}
          links={topologyEdges.map((edge) => ({
            id: edge.relation_id,
            source: edge.source_id,
            target: edge.target_id,
            label: edge.relation_type,
            riskLevel: edge.risk_level,
            weight: edge.weight,
          }))}
          onNodeClick={onSelect}
        />
      </div>
      {!nodes.length && <div className="taf-graph-empty">当前筛选条件下没有实体关系</div>}
    </div>
  );
}

function layeredTopologyNodes(
  nodes: EntityGraphWorkbenchNode[],
  edges: EntityGraphWorkbenchEdge[],
  centerId?: string,
) {
  if (!nodes.length) return [];
  const center = nodes.find((node) => node.entity_id === centerId) ?? nodes[0];
  const neighbors = new Map<string, Set<string>>();
  nodes.forEach((node) => neighbors.set(node.entity_id, new Set()));
  edges.forEach((edge) => {
    neighbors.get(edge.source_id)?.add(edge.target_id);
    neighbors.get(edge.target_id)?.add(edge.source_id);
  });

  const hopById = new Map<string, number>([[center.entity_id, 0]]);
  const queue = [center.entity_id];
  while (queue.length) {
    const current = queue.shift()!;
    const nextHop = (hopById.get(current) ?? 0) + 1;
    neighbors.get(current)?.forEach((neighborId) => {
      if (hopById.has(neighborId)) return;
      hopById.set(neighborId, nextHop);
      queue.push(neighborId);
    });
  }

  const byHop = new Map<number, EntityGraphWorkbenchNode[]>();
  nodes.forEach((node) => {
    const hop = Math.min(3, hopById.get(node.entity_id) ?? 3);
    const group = byHop.get(hop) ?? [];
    group.push(node);
    byHop.set(hop, group);
  });

  const positions = new Map<string, { x: number; y: number; hop: number; angle: number }>();
  positions.set(center.entity_id, { x: 50, y: 50, hop: 0, angle: 0 });
  const centerX = Number(center.x) || 50;
  const centerY = Number(center.y) || 50;
  const radiusX = [0, 27, 39, 47];
  const radiusY = [0, 35, 41, 45];

  for (const hop of [1, 2, 3]) {
    const group = (byHop.get(hop) ?? []).slice();
    group.sort((left, right) => topologyAngle(left, centerX, centerY) - topologyAngle(right, centerX, centerY));
    group.forEach((node, index) => {
      const parentAngles = Array.from(neighbors.get(node.entity_id) ?? [])
        .map((neighborId) => positions.get(neighborId))
        .filter((position): position is { x: number; y: number; hop: number; angle: number } => Boolean(position && position.hop === hop - 1))
        .map((position) => position.angle);
      const preferredAngle = hop > 1 && parentAngles.length
        ? parentAngles.reduce((sum, angle) => sum + angle, 0) / parentAngles.length
        : topologyAngle(node, centerX, centerY);
      const spacing = hop > 1 && group.length > 1 ? Math.min(0.24, Math.PI / (group.length * 1.8)) : 0;
      const angle = preferredAngle + (index - (group.length - 1) / 2) * spacing;
      positions.set(node.entity_id, {
        x: 50 + Math.cos(angle) * radiusX[hop],
        y: 50 + Math.sin(angle) * radiusY[hop],
        hop,
        angle,
      });
    });
  }

  return nodes.map((node) => {
    const position = positions.get(node.entity_id) ?? { x: Number(node.x) || 50, y: Number(node.y) || 50, hop: 3, angle: 0 };
    return { node, x: position.x, y: position.y, hop: position.hop };
  });
}

const topologyAngle = (node: EntityGraphWorkbenchNode, centerX: number, centerY: number) => (
  Math.atan2((Number(node.y) || centerY) - centerY, (Number(node.x) || centerX) - centerX)
);

function EntityDetail({ node, edges, timeRange, onClose }: { node?: EntityGraphWorkbenchNode; edges: EntityGraphWorkbenchEdge[]; timeRange: EntityGraphWorkbenchFilters['timeRange']; onClose: () => void }) {
  const incidentEdges = node ? edges.filter((edge) => edge.source_id === node.entity_id || edge.target_id === node.entity_id) : [];
  const services = entityServices(node, incidentEdges);
  const tags = metadataList(node, 'tags');
  const displayTags = tags.length ? tags : [entityTypeLabel(node?.entity_type), riskLabel(node?.risk_level ?? 'unknown')];
  const trafficTrend = entityTrend(node, 'traffic', timeRange);
  const alertTrend = entityTrend(node, 'alert', timeRange);
  const relatedAlerts = incidentEdges.reduce((total, edge) => total + Number(edge.attributes?.alert_count ?? (edge.relation_type === '关联告警' ? 1 : 0)), 0);
  return (
    <WorkPanel className="taf-graph-entity-detail-panel" title="实体详情" extra={<Button aria-label="关闭实体详情" size="small" type="text" icon={<CloseOutlined />} onClick={onClose} />}>
      <div className="taf-graph-entity-head">
        {node ? iconForNode(node) : <NodeIndexOutlined />}
        <div>
          <strong>{node?.label ?? '未选择实体'}</strong>
          <span>{node?.detail ?? '-'}</span>
        </div>
        <RiskScoreRingChart value={node?.risk_score ?? 0} />
      </div>
      <dl className="taf-graph-facts">
        <dt>实体类型</dt>
        <dd>{entityTypeLabel(node?.entity_type)}</dd>
        <dt>操作系统</dt>
        <dd>{metadataText(node, 'operating_system', '-')}</dd>
        <dt>资产分组</dt>
        <dd>{metadataText(node, 'asset_group', '-')}</dd>
        <dt>资产负责人</dt>
        <dd>{metadataText(node, 'owner', '-')}</dd>
        <dt>所属区域</dt>
        <dd>{metadataText(node, 'site_label', metadataText(node, 'site', '-'))}</dd>
        <dt>所属部门</dt>
        <dd>{metadataText(node, 'department', '-')}</dd>
      </dl>
      <div className="taf-graph-detail-section">
        <strong>标签</strong>
        <div className="taf-graph-entity-tags">{displayTags.map((tag) => <span key={tag}>{tag}</span>)}</div>
      </div>
      <div className="taf-graph-detail-section">
        <strong>开放服务</strong>
        <div className="taf-graph-service-chips">
          {services.map((service) => <span key={service} className={service === '无开放服务' ? 'is-empty' : ''}>{service}</span>)}
        </div>
      </div>
      <div className="taf-graph-activity-metrics">
        <span><small>{timeRange === '7d' ? '最近7天流量' : timeRange === 'all' ? '全部时间流量' : '最近24小时流量'}</small><b>{formatTrafficTotal(incidentEdges)}</b><i><SparklineChart ariaLabel={`${timeRange === '7d' ? '最近7天' : timeRange === 'all' ? '全部时间' : '最近24小时'}流量 ECharts 趋势图`} trend={trafficTrend} tone="info" dataSource={`nebula_graph:metadata.traffic_trend_${timeRange}`} /></i></span>
        <span><small>相关告警</small><b>{relatedAlerts}</b><i><SparklineChart ariaLabel="相关告警 ECharts 趋势图" trend={alertTrend} tone={relatedAlerts ? 'warn' : 'ok'} dataSource={`nebula_graph:metadata.alert_trend_${timeRange}`} /></i></span>
        <span><small>最近活跃时间</small><b>{node?.updated_at ? new Date(node.updated_at).toLocaleString('zh-CN', { hour12: false }) : '-'}</b></span>
      </div>
    </WorkPanel>
  );
}

function QueryGovernance({ graph, history }: { graph?: EntityGraphWorkbench; history: GraphQueryHistoryItem[] }) {
  const averageDuration = history.length
    ? Math.round(history.reduce((total, item) => total + item.duration_ms, 0) / history.length)
    : graph?.meta.query_duration_ms ?? 0;
  const slowQueries = history.filter((item) => item.duration_ms >= 500).length;
  const stats = [
    ['慢查询数', String(slowQueries), slowQueries ? 'warn' : 'ok'],
    ['节点上限', (graph?.meta.node_limit ?? 500).toLocaleString('zh-CN'), 'ok'],
    [graph?.meta.cache_applicable ? '图缓存命中率' : '图缓存状态', graph?.meta.cache_applicable ? graph.meta.cache_hit_rate : '未启用', 'ok'],
    ['平均查询耗时', `${averageDuration} ms`, averageDuration >= 500 ? 'warn' : 'info'],
  ];
  const recent = history.slice(0, 4);
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
      <div className="taf-graph-query-history-grid">
        <section className="taf-graph-query-chart">
          <strong>查询历史</strong>
          <div>
            {recent.length ? (
              <QueryHistoryBarChart
                ariaLabel={`查询历史耗时，共 ${recent.length} 次查询`}
                items={recent.slice().reverse().map((item) => ({
                  label: new Date(item.created_at).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }),
                  value: item.duration_ms,
                }))}
              />
            ) : <small>暂无查询</small>}
          </div>
        </section>
        <section className="taf-graph-recent-queries">
          <strong>最近查询</strong>
          {recent.map((item) => (
            <div key={item.id}>
              <span title={item.label}>{item.label}</span>
              <time>{new Date(item.created_at).toLocaleTimeString('zh-CN', { hour12: false })}</time>
              <b>{item.duration_ms} ms</b>
              <em>通过</em>
            </div>
          ))}
          {!recent.length && <div><span>当前筛选条件无查询记录</span><time>-</time><b>-</b><em>-</em></div>}
        </section>
      </div>
    </div>
  );
}

function EvidenceList({ edges, navigate }: { edges: EntityGraphWorkbenchEdge[]; navigate: (to: string) => void }) {
  const items = Array.from(new Map(edges
    .filter((edge) => edge.evidence_id)
    .map((edge) => [edge.evidence_id, edge])).values()).slice(0, 5);
  return (
    <div className="taf-graph-evidence">
      {items.map((edge) => (
        <button key={edge.evidence_id} type="button" title={`关系：${edge.relation_type}`} onClick={() => navigate(evidenceDestination(edge.evidence_id!))}>
          <FileSearchOutlined />
          <span>{edge.evidence_id}</span>
          <em>{new Date(edge.observed_at).toLocaleTimeString('zh-CN', { hour12: false })}</em>
        </button>
      ))}
      {!items.length && <span className="taf-graph-evidence-empty">暂无关联证据</span>}
    </div>
  );
}

function PathResultView({
  tab,
  path,
  graph,
  loading,
  error,
  onRetry,
  navigate,
}: {
  tab: string;
  path?: EntityGraphWorkbenchPath;
  graph?: { nodes: EntityGraphWorkbenchNode[]; edges: EntityGraphWorkbenchEdge[] };
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  navigate: (to: string) => void;
}) {
  if (loading) return <div className="taf-graph-path-state is-loading">正在从 NebulaGraph 计算{tab}…</div>;
  if (error) return <div className="taf-graph-path-state is-error">{tab}查询失败 <Button size="small" danger onClick={onRetry}>重试</Button></div>;
  if (!path?.length) return <div className="taf-graph-path-state is-empty">当前实体之间没有符合“{tab}”规则的路径</div>;
  if (path.mode === 'attack') return <AttackPathResult path={path} graph={graph} navigate={navigate} />;
  if (path.mode === 'communication') return <CommunicationPathResult path={path} graph={graph} navigate={navigate} />;
  if (path.mode === 'account') return <AccountPathResult path={path} graph={graph} navigate={navigate} />;
  return <ShortestPathResult path={path} graph={graph} />;
}

function ShortestPathResult({ path, graph }: { path: EntityGraphWorkbenchPath; graph?: { nodes: EntityGraphWorkbenchNode[] } }) {
  const nodeLabels = path.node_ids.map((id) => graphNodeLabel(graph, id));
  return (
    <div className="taf-graph-mode-view is-shortest" data-path-mode="shortest">
      <div className="taf-graph-mode-meta">
        <span>源节点 <strong>{nodeLabels[0]}</strong></span>
        <span>目标节点 <strong>{nodeLabels[nodeLabels.length - 1]}</strong></span>
        <span>路径长度 <strong>{path.length}</strong></span>
        <StatusTag value={riskLabel(path.risk_level)} />
      </div>
      <div className="taf-graph-pathline is-shortest" aria-label={`最短路径，共 ${path.length} 跳`}>
        {path.node_ids.map((nodeId, index) => {
          const node = graphNodeForPath(graph, nodeId);
          const edge = path.edges[index];
          return (
            <div className="taf-graph-path-step" key={`${nodeId}-${nodeLabels[index]}`}>
              <span className="taf-graph-path-node">
                {node ? iconForNode(node) : index === 0 ? <GlobalOutlined /> : <DatabaseOutlined />}
                <b>{nodeLabels[index]}</b>
                <small>{index === 0 ? '源实体' : index === nodeLabels.length - 1 ? '目标实体' : '中继实体'}</small>
              </span>
              {edge && <PathConnector tone="normal" title={edge.relation_type} badges={[attributeText(edge, 'service', edge.evidence_id || '关系路径')]} />}
            </div>
          );
        })}
      </div>
      <CompactPathRow labels={['路径 ID', '关系类型', '跳数', '风险', '证据']} values={[path.edges.map((edge) => edge.relation_id).join(' → '), path.edges.map((edge) => edge.relation_type).join(' → '), String(path.length), riskLabel(path.risk_level), pathEvidenceSummary(path)]} />
    </div>
  );
}

function AttackPathResult({ path, graph, navigate }: PathModeProps) {
  const attackStages = ['初始访问', '凭证访问', '横向移动', '数据外传'];
  const activeStages = new Set(pathAttributeValues(path, 'attack_stage'));
  const stages = pathAttributeSummary(path, 'attack_stage', path.edges.map((edge) => edge.relation_type).join(' → '));
  const alertCount = path.edges.reduce((total, edge) => total + Number(edge.attributes?.alert_count ?? 0), 0);
  return (
    <div className="taf-graph-mode-view is-attack" data-path-mode="attack">
      <div className="taf-graph-mode-meta">
        <span>攻击阶段 <strong>{stages}</strong></span>
        <span>告警节点 <strong>{alertCount}</strong></span>
        <span>证据锚点 <strong>{path.evidence_ids.length}</strong></span>
        <Button size="small" danger onClick={() => navigate(`/attack-chains?entity=${encodeURIComponent(path.target_id)}`)}>跳转攻击链</Button>
      </div>
      <div className="taf-graph-attack-workbench">
        <div className="taf-graph-attack-ribbon" aria-label="攻击阶段链">
          {attackStages.map((item) => <span key={item} className={activeStages.has(item) ? 'is-active' : ''}>{item}</span>)}
        </div>
        <div className="taf-graph-pathline is-attack" aria-label={`攻击路径，共 ${path.length} 跳`}>
          {path.node_ids.map((nodeId, index) => {
            const node = graphNodeForPath(graph, nodeId);
            const edge = path.edges[index];
            return (
              <div className="taf-graph-path-step" key={nodeId}>
                <span className="taf-graph-path-node">
                  {node ? iconForNode(node) : index === 0 ? <AlertOutlined /> : <DatabaseOutlined />}
                  <b>{graphNodeLabel(graph, nodeId)}</b>
                  <small>{index === 0
                    ? '攻击源'
                    : index === path.node_ids.length - 1
                      ? node?.entity_type === 'alert' ? '告警锚点' : '受影响实体'
                      : '中继实体'}</small>
                </span>
                {edge && <PathConnector tone="danger" title={attributeText(edge, 'action', edge.relation_type)} badges={[edge.evidence_id || attributeText(edge, 'attack_stage', edge.relation_type)]} />}
              </div>
            );
          })}
        </div>
      </div>
      <CompactPathRow labels={['攻击动作', '关系链', '风险等级', '证据锚点', '路径长度']} values={[pathAttributeSummary(path, 'action', '-'), path.edges.map((edge) => edge.relation_type).join(' → '), riskLabel(path.risk_level), pathEvidenceSummary(path), String(path.length)]} />
    </div>
  );
}

function CommunicationPathResult({ path, graph, navigate }: PathModeProps) {
  const services = path.edges.map((edge) => `${attributeText(edge, 'service', edge.relation_type)} / ${attributeText(edge, 'port', '-')}`).join(' → ');
  const totalFrequency = pathAttributeTotal(path, 'frequency');
  const protocols = pathAttributeSummary(path, 'protocol', '-');
  const peers = (graph?.edges ?? [])
    .filter((candidate) => candidate.attributes?.frequency !== undefined)
    .sort((left, right) => Number(right.attributes?.frequency ?? 0) - Number(left.attributes?.frequency ?? 0))
    .slice(0, 3);
  return (
    <div className="taf-graph-mode-view is-communication" data-path-mode="communication">
      <div className="taf-graph-mode-meta">
        <span>源实体 <strong>{graphNodeLabel(graph, path.source_id)}</strong></span>
        <span>服务端口 <strong>{services}</strong></span>
        <span>通信频次 <strong>{totalFrequency} 次</strong></span>
        <span>协议 <strong>{protocols}</strong></span>
      </div>
      <div className="taf-graph-communication-workbench">
        <div className="taf-graph-pathline is-communication" aria-label={`通信路径，共 ${path.length} 跳`}>
          {path.node_ids.map((nodeId, index) => {
            const node = graphNodeForPath(graph, nodeId);
            const edge = path.edges[index];
            return (
              <div className="taf-graph-path-step" key={nodeId}>
                <span className="taf-graph-path-node">
                  {node ? iconForNode(node) : index === 0 ? <GlobalOutlined /> : <DatabaseOutlined />}
                  <b>{graphNodeLabel(graph, nodeId)}</b>
                  <small>{index === 0 ? '源实体' : index === path.node_ids.length - 1 ? '目标资产' : '中继实体'}</small>
                </span>
                {edge && <PathConnector
                  tone="traffic"
                  title={`${attributeText(edge, 'service', edge.relation_type)} ${attributeText(edge, 'port', '')}`}
                  weight={Math.max(1, Math.round(edge.weight * 5))}
                  badges={[`${attributeText(edge, 'latency_ms', '-')} ms · ${attributeText(edge, 'bytes', '-')} · ${attributeText(edge, 'protocol', '-')}`]}
                />}
              </div>
            );
          })}
        </div>
        <div className="taf-graph-peer-list">
          <strong>Top 对端</strong>
          {peers.map((peer, index) => (
            <span key={peer.relation_id}>
              <i>{index + 1}</i>
              <b title={graphNodeLabel(graph, peer.source_id)}>{graphNodeLabel(graph, peer.source_id)}</b>
              <em>{attributeText(peer, 'frequency', '-')}次</em>
            </span>
          ))}
        </div>
      </div>
      <CompactPathRow labels={['通信关系', '通信频次', '字节数', '平均延迟', '证据']} values={[path.edges.map((edge) => edge.relation_type).join(' → '), String(totalFrequency), pathAttributeSummary(path, 'bytes', '-'), `${pathAttributeSummary(path, 'latency_ms', '-')} ms`, pathEvidenceSummary(path)]} action={<Button size="small" onClick={() => navigate(pathEvidenceDestination(path))}>查看证据</Button>} />
    </div>
  );
}

function AccountPathResult({ path, graph, navigate }: PathModeProps) {
  const services = path.edges.map((edge) => `${attributeText(edge, 'service', edge.relation_type)} ${attributeText(edge, 'port', '')}`).join(' → ');
  const anomalies = (graph?.edges ?? [])
    .filter((candidate) => candidate.attributes?.identity_label || candidate.attributes?.anomaly_reason)
    .slice(0, 3);
  return (
    <div className="taf-graph-mode-view is-account" data-path-mode="account">
      <div className="taf-graph-mode-meta">
        <span>账号 <strong>{graphNodeLabel(graph, path.source_id)}</strong></span>
        <span>访问服务 <strong>{services}</strong></span>
        <span>身份标签 <strong>{pathAttributeSummary(path, 'identity_label', '-')}</strong></span>
        <span>异常原因 <strong>{pathAttributeSummary(path, 'anomaly_reason', '-')}</strong></span>
      </div>
      <div className="taf-graph-account-workbench">
        <div className="taf-graph-pathline is-account" aria-label={`账号访问路径，共 ${path.length} 跳`}>
          {path.node_ids.map((nodeId, index) => {
            const node = graphNodeForPath(graph, nodeId);
            const edge = path.edges[index];
            return (
              <div className="taf-graph-path-step" key={nodeId}>
                <span className="taf-graph-path-node">
                  {node ? iconForNode(node) : index === 0 ? <UserOutlined /> : <DatabaseOutlined />}
                  <b>{graphNodeLabel(graph, nodeId)}</b>
                  <small>{index === 0 ? pathAttributeSummary(path, 'identity_label', '账号') : index === path.node_ids.length - 1 ? '访问资产' : '中继主机'}</small>
                </span>
                {edge && <PathConnector tone="account" title={`${attributeText(edge, 'service', edge.relation_type)} ${attributeText(edge, 'port', '')}`} badges={[attributeText(edge, 'anomaly_reason', edge.relation_type)]} />}
              </div>
            );
          })}
        </div>
        <div className="taf-graph-anomaly-list">
          <strong>异常访问</strong>
          {anomalies.map((candidate) => (
            <span key={candidate.relation_id}>
              <b>{attributeText(candidate, 'anomaly_reason', '-')}</b>
              <em>{attributeText(candidate, 'frequency', '1')}次</em>
            </span>
          ))}
        </div>
      </div>
      <CompactPathRow labels={['访问关系', '身份标签', '异常原因', '访问频次', '证据']} values={[path.edges.map((edge) => edge.relation_type).join(' → '), pathAttributeSummary(path, 'identity_label', '-'), pathAttributeSummary(path, 'anomaly_reason', '-'), String(pathAttributeTotal(path, 'frequency')), pathEvidenceSummary(path)]} action={<Space size={4}><Button size="small" onClick={() => navigate(`/assets?search=${encodeURIComponent(path.source_id)}`)}>账号画像</Button><Button size="small" onClick={() => navigate(`/audit-log?object_id=${encodeURIComponent(path.evidence_ids[0] || path.source_id)}&evidence_ids=${encodeURIComponent(path.evidence_ids.join(','))}`)}>查看审计</Button></Space>} />
    </div>
  );
}

function PathConnector({
  title,
  badges,
  tone,
  weight = 2,
}: {
  title: string;
  badges: string[];
  tone: 'normal' | 'danger' | 'traffic' | 'account';
  weight?: number;
}) {
  return (
    <span className={`taf-graph-path-connector is-${tone}`} style={{ '--taf-flow-weight': String(weight) } as React.CSSProperties}>
      <small title={title}>{title}</small>
      <i aria-hidden="true"><ArrowRightOutlined /></i>
      <em>{badges.filter(Boolean).map((badge, index) => <b key={`${badge}-${index}`}>{badge}</b>)}</em>
    </span>
  );
}

function CompactPathRow({ labels, values, action }: { labels: string[]; values: string[]; action?: React.ReactNode }) {
  return (
    <div className="taf-graph-compact-row">
      {labels.map((label, index) => <span key={label}><small>{label}</small><b title={values[index]}>{values[index]}</b></span>)}
      {action && <em>{action}</em>}
    </div>
  );
}

type PathModeProps = {
  path: EntityGraphWorkbenchPath;
  graph?: { nodes: EntityGraphWorkbenchNode[]; edges: EntityGraphWorkbenchEdge[] };
  navigate: (to: string) => void;
};

function GraphActionRail({ node, navigate }: { node?: EntityGraphWorkbenchNode; navigate: (to: string) => void }) {
  const businessTarget = node?.entity_type === 'account'
    ? node.label
    : node?.detail && node.detail !== entityTypeLabel(node.entity_type)
      ? node.detail
      : node?.label ?? '';
  const target = encodeURIComponent(businessTarget);
  return (
    <div className="taf-graph-action-rail">
      <Button type="primary" disabled={!node} onClick={() => navigate(`/assets?search=${target}`)}>查看资产</Button>
      <Button type="primary" disabled={!node} onClick={() => navigate(`/alerts?entity=${target}`)}>查看告警</Button>
      <Button type="primary" disabled={!node} onClick={() => navigate(`/forensics?assetId=${target}`)}>进入取证</Button>
      <Button disabled={!node} onClick={() => navigate(`/attack-chains?entity=${target}`)}>跳转攻击链</Button>
      <Button disabled={!node} icon={<HistoryOutlined />} onClick={() => navigate(`/audit-log?object_type=graph&object_id=${encodeURIComponent(node?.entity_id || '')}`)}>审计日志</Button>
    </div>
  );
}

const pathModeForTab = (tab: string): EntityGraphWorkbenchPath['mode'] => {
  if (tab.includes('攻击')) return 'attack';
  if (tab.includes('通信')) return 'communication';
  if (tab.includes('账号')) return 'account';
  return 'shortest';
};

const choosePathSource = (
  nodes: EntityGraphWorkbenchNode[],
  centerId: string | undefined,
  mode: EntityGraphWorkbenchPath['mode'],
  selected?: EntityGraphWorkbenchNode,
) => {
  if (selected && selected.entity_id !== centerId && mode === 'attack' && selected.entity_type === 'alert') {
    return nodes.find((node) => node.entity_type === 'ip' && node.risk_level === 'high');
  }
  if (selected && selected.entity_id !== centerId && mode !== 'account') return selected;
  if (selected?.entity_type === 'account' && mode === 'account') return selected;
  const candidates = nodes.filter((node) => node.entity_id !== centerId);
  const preferred = {
    shortest: (node: EntityGraphWorkbenchNode) => node.entity_type === 'ip' && node.risk_level === 'high',
    attack: (node: EntityGraphWorkbenchNode) => node.entity_type === 'ip' && node.risk_level === 'high',
    communication: (node: EntityGraphWorkbenchNode) => node.entity_type === 'domain',
    account: (node: EntityGraphWorkbenchNode) => node.entity_type === 'account',
  }[mode];
  return candidates.find(preferred) ?? candidates[0];
};

const choosePathTarget = (
  nodes: EntityGraphWorkbenchNode[],
  centerId: string | undefined,
  mode: EntityGraphWorkbenchPath['mode'],
  selected?: EntityGraphWorkbenchNode,
) => {
  if (selected && selected.entity_id !== centerId) {
    if (mode === 'account') return selected.entity_type === 'account'
      ? nodes.find((node) => node.entity_id === centerId)
      : selected;
    if (mode === 'attack') return selected.entity_type === 'alert'
      ? selected
      : nodes.find((node) => node.entity_type === 'alert');
    return nodes.find((node) => node.label === 'DB-SRV-01' && node.entity_id !== selected.entity_id)
      ?? nodes.find((node) => node.entity_id === centerId && node.entity_id !== selected.entity_id);
  }
  if (mode === 'account') return nodes.find((node) => node.entity_id === centerId);
  if (mode === 'attack') return nodes.find((node) => node.entity_type === 'alert');
  return nodes.find((node) => node.label === 'DB-SRV-01')
    ?? nodes.find((node) => node.entity_type === 'host' && node.entity_id !== centerId);
};

const evidenceDestination = (evidenceId: string) => {
  const encoded = encodeURIComponent(evidenceId);
  if (evidenceId.startsWith('ALERT-')) return `/alerts?search=${encoded}`;
  if (evidenceId.startsWith('AUDIT-')) return `/audit-log?object_id=${encoded}`;
  return `/forensics?evidence=${encoded}`;
};

const riskLabel = (risk: string) => {
  if (risk === 'high') return '高危';
  if (risk === 'medium') return '中危';
  if (risk === 'low') return '低危';
  return '未知';
};

const entityTypeLabel = (type?: string) => ({
  ip: 'IP 地址',
  host: '主机',
  account: '账号',
  domain: '域名',
  service: '服务',
  alert: '告警',
  evidence: '证据',
}[type ?? ''] ?? type ?? '-');

const iconForNode = (node: EntityGraphWorkbenchNode) => {
  if (node.entity_type === 'ip') return <GlobalOutlined />;
  if (node.entity_type === 'account') return <UserOutlined />;
  if (node.entity_type === 'domain') return <ClusterOutlined />;
  if (node.entity_type === 'alert') return <RadarChartOutlined />;
  if (node.entity_type === 'evidence') return <FileSearchOutlined />;
  if (node.entity_type === 'service') return <DatabaseOutlined />;
  if (node.icon === 'gateway') return <SafetyCertificateOutlined />;
  if (node.entity_type === 'host') return <DatabaseOutlined />;
  return <ApartmentOutlined />;
};

const metadataText = (node: EntityGraphWorkbenchNode | undefined, key: string, fallback: string) => {
  const value = node?.metadata?.[key];
  if (Array.isArray(value)) return value.map(String).join('、');
  return typeof value === 'string' && value ? value : fallback;
};

const metadataList = (node: EntityGraphWorkbenchNode | undefined, key: string) => {
  const value = node?.metadata?.[key];
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === 'string' && value) return value.split(/[、,]/).map((item) => item.trim()).filter(Boolean);
  return [];
};

const entityTrend = (
  node: EntityGraphWorkbenchNode | undefined,
  metric: 'traffic' | 'alert',
  timeRange: EntityGraphWorkbenchFilters['timeRange'],
) => {
  const value = node?.metadata?.[`${metric}_trend_${timeRange}`];
  if (!Array.isArray(value)) return [];
  return value.map(Number).filter(Number.isFinite);
};

const entityServices = (node: EntityGraphWorkbenchNode | undefined, edges: EntityGraphWorkbenchEdge[]) => {
  const explicit = metadataList(node, 'services');
  if (explicit.length) return explicit;
  const metadataProtocol = metadataText(node, 'protocol', '');
  const metadataPort = node?.metadata?.port;
  const related = edges.flatMap((edge) => {
    const service = edge.attributes?.service;
    const port = edge.attributes?.port;
    return service ? [`${String(service)}${port ? `/${String(port)}` : ''}`] : [];
  });
  if (metadataProtocol) related.push(`${metadataProtocol}${metadataPort ? `/${String(metadataPort)}` : ''}`);
  const unique = Array.from(new Set(related));
  return unique.length ? unique : ['无开放服务'];
};

const formatTrafficTotal = (edges: EntityGraphWorkbenchEdge[]) => {
  const totalMB = edges.reduce((total, edge) => {
    const raw = String(edge.attributes?.bytes ?? '').trim();
    const match = raw.match(/^([\d.]+)\s*(KB|MB|GB|TB)$/i);
    if (!match) return total;
    const value = Number(match[1]);
    const unit = match[2].toUpperCase();
    const factor = unit === 'TB' ? 1024 * 1024 : unit === 'GB' ? 1024 : unit === 'KB' ? 1 / 1024 : 1;
    return total + value * factor;
  }, 0);
  if (totalMB >= 1024) return `${(totalMB / 1024).toFixed(1)} GB`;
  return `${Math.round(totalMB)} MB`;
};

const graphNodeLabel = (graph: { nodes: EntityGraphWorkbenchNode[] } | undefined, entityId: string) => (
  graph?.nodes.find((node) => node.entity_id === entityId)?.label ?? entityId
);

const graphNodeForPath = (graph: { nodes: EntityGraphWorkbenchNode[] } | undefined, entityId: string) => (
  graph?.nodes.find((node) => node.entity_id === entityId)
);

const pathAttributeValues = (path: EntityGraphWorkbenchPath, key: string) => (
  Array.from(new Set(path.edges
    .map((edge) => edge.attributes?.[key])
    .filter((value) => value !== undefined && value !== null && value !== '')
    .map(String)))
);

const pathAttributeSummary = (path: EntityGraphWorkbenchPath, key: string, fallback: string) => {
  const values = pathAttributeValues(path, key);
  return values.length ? values.join(' → ') : fallback;
};

const pathAttributeTotal = (path: EntityGraphWorkbenchPath, key: string) => (
  path.edges.reduce((total, edge) => total + Number(edge.attributes?.[key] ?? 0), 0)
);

const pathEvidenceSummary = (path: EntityGraphWorkbenchPath) => (
  path.evidence_ids.length ? path.evidence_ids.join(' / ') : '-'
);

const pathEvidenceDestination = (path: EntityGraphWorkbenchPath) => {
  const evidenceId = path.evidence_ids[0] ?? '';
  const base = evidenceDestination(evidenceId);
  return `${base}&evidence_ids=${encodeURIComponent(path.evidence_ids.join(','))}`;
};

const attributeText = (edge: EntityGraphWorkbenchEdge | undefined, key: string, fallback: string) => {
  const value = edge?.attributes?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};
