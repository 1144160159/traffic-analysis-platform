import {
  AlertOutlined,
  ApiOutlined,
  BlockOutlined,
  BranchesOutlined,
  CalendarOutlined,
  CloseOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileSearchOutlined,
  FullscreenOutlined,
  LinkOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Empty, Popconfirm, Select, Space, Table, Tabs, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { fetchAttackChainDetail, fetchAttackChains, type AttackChainDetail, type AttackChainPhase } from '@/services/attackChainApi';
import { submitCampaignAction, type CampaignActionId } from '@/services/campaignActionApi';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const visualPhases = [
  ['侦察', 'TA0043', '203.0.113.45', '端口扫描探测', 'DNS 解析记录', '封禁源 IP', 'info'],
  ['初始访问', 'TA0001', '边界防火墙 FW-01', 'Web 漏洞利用', 'HTTP 请求包', 'WAF 规则加固', 'ok'],
  ['执行', 'TA0002', 'WEB 服务器 10.12.5.23', '恶意命令执行', '进程创建日志', '终止恶意进程', 'ok'],
  ['横向移动', 'TA0008', '域控服务器 10.12.1.10', '凭证窃取', 'LSASS 访问', '重置域控凭证', 'warn'],
  ['C2 通信', 'TA0011', '内网主机 10.12.8.45', 'C2 隧道通信', 'TLS 流量会话', '阻断 C2 域名', 'warn'],
  ['数据外传', 'TA0010', '外部域名 c2.example.com', '数据外传尝试', '外传流量样本', '阻断外传通道', 'risk'],
];

const visualEvidenceRows = [
  ['1', 'PCAP', 'dns-20260619-0112.pcap', '01:12:08', '100%'],
  ['2', 'PCAP', 'web-20260619-0114.pcap', '01:14:22', '100%'],
  ['3', '日志', 'sysmon-4688.log', '01:15:03', '100%'],
  ['4', '日志', 'sysmon-10.log', '01:18:47', '95%'],
  ['5', 'Session', 'tls-session-012511.json', '01:25:11', '100%'],
  ['6', 'PCAP', 'exfil-20260619-0143.pcap', '01:43:02', '98%'],
];

const visualRecommendations = [
  ['高', 'c2.example.com', '封禁域名', '低影响'],
  ['高', '198.51.100.27', '阻断 IP', '低影响'],
  ['中', '10.12.8.45', '隔离主机', '中等影响'],
  ['中', '10.12.5.23', '加强访问控制', '低影响'],
  ['低', 'SMB 445', '收紧防火墙策略', '低影响'],
  ['低', 'RDP 3389', '限制管理网段', '低影响'],
];

export function AttackChainAnalysisPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const visualBreakdownMode = isVisualBreakdownMode();
  const sourceEntity = searchParams.get('entity') ?? '';
  const sourceChain = searchParams.get('chain') ?? '';
  const sourceCampaign = searchParams.get('campaign') ?? '';
  const sourceViewMode = searchParams.get('view') ?? '攻击链视图';
  const drawerRequested = searchParams.get('drawer') === 'attack-chain-detail';
  const visualPageId = searchParams.get('__codex_page_id') ?? '';
  const [assetScope, setAssetScope] = useState(sourceEntity || '全部资产');
  const [selectedChainId, setSelectedChainId] = useState(sourceChain);
  const [viewMode, setViewMode] = useState(sourceViewMode);
  const [detailOpen, setDetailOpen] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, sourceEntity],
    queryFn: () => fetchPageSnapshot(route.id),
  });
  const chainListQuery = useQuery({
    queryKey: ['attack-chain-list'],
    queryFn: () => fetchAttackChains(),
    enabled: !visualBreakdownMode,
  });
  useEffect(() => {
    if (!visualBreakdownMode && sourceChain) setSelectedChainId(sourceChain);
  }, [sourceChain, visualBreakdownMode]);
  useEffect(() => {
    if (sourceEntity) setAssetScope(sourceEntity);
  }, [sourceEntity]);
  useEffect(() => {
    setViewMode(sourceViewMode);
  }, [sourceViewMode]);
  useEffect(() => {
    if (visualBreakdownMode || sourceChain) return;
    const chains = chainListQuery.data?.chains ?? [];
    const campaignChainId = sourceCampaign
      ? chains.find((chain) => chain.chain_id === sourceCampaign)?.chain_id
      : '';
    const resolvedChainId = campaignChainId || chains[0]?.chain_id;
    if (resolvedChainId && resolvedChainId !== selectedChainId) setSelectedChainId(resolvedChainId);
  }, [chainListQuery.data?.chains, selectedChainId, sourceCampaign, sourceChain, visualBreakdownMode]);
  const detailQuery = useQuery({
    queryKey: ['attack-chain-detail', selectedChainId],
    queryFn: () => fetchAttackChainDetail(selectedChainId),
    enabled: !visualBreakdownMode && Boolean(selectedChainId),
  });
  const actionMutation = useMutation({
    mutationFn: submitCampaignAction,
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '攻击链操作失败'),
  });
  const runChainAction = async (actionId: CampaignActionId, target: string, metadata?: Record<string, unknown>) => {
    const chainId = selectedChainId || selectedChain?.chain_id;
    if (!chainId) throw new Error('当前没有可操作的攻击链');
    return actionMutation.mutateAsync({ actionId, campaignId: chainId, target, metadata });
  };
  const refreshAttackChain = async () => {
    await Promise.all([
      refetch(),
      chainListQuery.refetch(),
      selectedChainId ? detailQuery.refetch() : Promise.resolve(),
    ]);
  };

  const detail = detailQuery.data;
  const rows = useMemo(
    () => visualBreakdownMode
      ? data?.rows ?? []
      : detail?.phases.map((phase, index) => attackPhaseToRow(phase, detail, index)) ?? [],
    [data?.rows, detail, visualBreakdownMode],
  );
  const phaseRows = useMemo(
    () => visualBreakdownMode ? visualPhases : rows.slice(0, 6).map((row, index) => [
      String(row['阶段'] ?? `阶段 ${index + 1}`),
      String(row['技术'] ?? row['MITRE'] ?? '未提供'),
      String(row['实体'] ?? '未提供'),
      String(row['告警'] ?? '未提供'),
      String(row['证据'] ?? '未提供'),
      String(row['处置建议'] ?? '未提供'),
      attackPhaseStatusTone(row['状态']),
    ]),
    [rows, visualBreakdownMode],
  );
  const evidenceAnchorRows = useMemo(() => {
    if (visualBreakdownMode) return visualEvidenceRows;
    const evidence = detail?.phases.flatMap((phase, phaseIndex) => phase.key_events.map((event, eventIndex) => [
      String(phaseIndex + 1),
      evidenceType(event.technique || event.description),
      event.event_id || event.description || `事件 ${eventIndex + 1}`,
      formatAttackTimestamp(event.timestamp),
      event.event_id ? '已登记' : '未提供',
    ])) ?? [];
    return evidence.slice(0, 12);
  }, [detail?.phases, visualBreakdownMode]);
  const responseRows = useMemo(
    () => visualBreakdownMode ? visualRecommendations : rows.slice(0, 6).map((row) => [
      attackPhaseStatusTone(row['状态']) === 'ok' ? '中' : '高', String(row['实体'] ?? '未提供'), String(row['处置建议'] ?? '未提供'), '待评估',
    ]),
    [rows, visualBreakdownMode],
  );
  useEffect(() => {
    if (visualPageId === 'drawer-attack-chain-detail' || drawerRequested) setDetailOpen(true);
  }, [drawerRequested, visualPageId]);
  const selectedChain = visualBreakdownMode ? visualAttackChainDetail() : detail;
  const updateRouteState = (key: 'chain' | 'entity' | 'view', value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value && value !== '全部资产' && value !== '攻击链视图') next.set(key, value);
    else next.delete(key);
    if (key === 'chain') next.delete('campaign');
    setSearchParams(next, { replace: true });
  };
  const openAttackDetail = async () => {
    await runChainAction('campaign-context-action', '查看攻击链详情');
    const next = new URLSearchParams(searchParams);
    next.set('chain', selectedChain?.chain_id ?? selectedChainId);
    next.delete('campaign');
    next.set('drawer', 'attack-chain-detail');
    setSearchParams(next, { replace: true });
    setDetailOpen(true);
  };
  const closeAttackDetail = () => {
    const next = new URLSearchParams(searchParams);
    next.delete('drawer');
    next.delete('detailTab');
    setSearchParams(next, { replace: true });
    setDetailOpen(false);
  };
  const chainOptions = visualBreakdownMode
    ? [{ value: 'visual-attack-chain', label: '疑似 C2 隧道通信' }]
    : (chainListQuery.data?.chains ?? []).map((chain) => ({ value: chain.chain_id, label: chain.title || chain.chain_id }));
  const scopedPhaseRows = assetScope === '全部资产'
    ? phaseRows
    : phaseRows.filter((row, index) => {
      if (row.some((value) => value.includes(assetScope))) return true;
      const phase = selectedChain?.phases[index];
      return phase?.key_events.some((event) => event.src_ip === assetScope || event.dst_ip === assetScope) ?? false;
    });
  const activeError = visualBreakdownMode
    ? error
    : chainListQuery.error ?? detailQuery.error;
  const activeIsError = visualBreakdownMode
    ? isError
    : chainListQuery.isError || detailQuery.isError;
  const activeLoading = visualBreakdownMode
    ? isLoading
    : chainListQuery.isLoading || detailQuery.isLoading;
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => renderAttackCell(column, value),
  }));

  return (
    <div className="taf-page taf-attack-chain">
      <section className="taf-attack-shell">
        <header className="taf-attack-toolbar">
          <div><h1>{route.page.title}</h1>{sourceEntity && <span className="taf-source-context" data-source-entity={sourceEntity}>关联实体：{sourceEntity}</span>}</div>
          <div className="taf-attack-filters">
            <label>
              <span>选择战役</span>
              <Select
                size="small"
                value={visualBreakdownMode ? 'visual-attack-chain' : selectedChainId || undefined}
                options={chainOptions}
                loading={chainListQuery.isLoading}
                onChange={(value) => {
                  setSelectedChainId(value);
                  if (!visualBreakdownMode) updateRouteState('chain', value);
                }}
              />
            </label>
            <label>
              <span>时间范围</span>
              <Button size="small" icon={<CalendarOutlined />} onClick={() => message.info(`当前战役时间窗：${formatAttackTimestamp(selectedChain?.start_time)} ~ ${formatAttackTimestamp(selectedChain?.end_time)}`)}>{formatAttackTimestamp(selectedChain?.start_time)} ~ {formatAttackTimestamp(selectedChain?.end_time)}</Button>
            </label>
            <label>
              <span>资产范围</span>
              <Select
                size="small"
                value={assetScope}
                options={Array.from(new Set(['全部资产', sourceEntity, ...(selectedChain?.phases.flatMap((phase) => phase.key_events.flatMap((event) => [event.src_ip, event.dst_ip])) ?? [])].filter(Boolean))).map((value) => ({ value }))}
                onChange={(value) => {
                  setAssetScope(value);
                  updateRouteState('entity', value);
                }}
              />
            </label>
            <label>
              <span>视图模式</span>
              <Select
                size="small"
                value={viewMode}
                options={[{ value: '攻击链视图' }, { value: '泳道视图' }, { value: '矩阵视图' }]}
                onChange={(value) => {
                  setViewMode(value);
                  updateRouteState('view', value);
                }}
              />
            </label>
          </div>
          <Space>
            <Button
              size="small"
              icon={<DownloadOutlined />}
              loading={actionMutation.isPending}
              onClick={() => void runChainAction('campaign-export', '导出攻击链报告', { format: 'json' })
                .then(() => exportAttackChain(rows))
                .catch(() => {})}
            >
              导出报告
            </Button>
            <Button size="small" icon={<LinkOutlined />} onClick={() => void runChainAction('campaign-graph-view', '攻击链下钻图谱').then(() => navigate(`/graph?campaign=${encodeURIComponent(selectedChain?.chain_id ?? selectedChainId)}`))}>下钻图谱</Button>
            <Popconfirm
              title="确认触发攻击链响应？"
              description="将创建 SOAR 响应任务并写入审计留痕。"
              okText="确认触发"
              cancelText="取消"
              okButtonProps={{ loading: actionMutation.isPending }}
              onConfirm={() => void runChainAction('campaign-soar-response', '攻击链触发响应', { dry_run: true }).then(() => navigate(`/playbooks?campaign=${encodeURIComponent(selectedChain?.chain_id ?? selectedChainId)}`))}
            >
              <Button size="small" type="primary" icon={<BlockOutlined />} loading={actionMutation.isPending} disabled={actionMutation.isPending}>触发响应</Button>
            </Popconfirm>
            <Tooltip title="刷新攻击链">
              <Button size="small" icon={<ReloadOutlined />} loading={activeLoading} onClick={() => void refreshAttackChain()} />
            </Tooltip>
            <Button size="small" icon={<FullscreenOutlined />} onClick={() => document.querySelector<HTMLElement>('.taf-attack-shell')?.requestFullscreen?.()}>全屏画布</Button>
            <Button size="small" icon={<EyeOutlined />} onClick={() => void openAttackDetail().catch(() => {})}>链路详情</Button>
          </Space>
        </header>

        {activeIsError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={activeError instanceof Error ? activeError.message : '请检查 /v1/attack-chains、ClickHouse campaigns 或后端服务。'}
            action={<Button size="small" danger onClick={() => void refreshAttackChain()}>重试</Button>}
          />
        )}

        <div className="taf-attack-grid">
          <main className="taf-attack-main">
            <WorkPanel title="攻击链画布" className="taf-attack-canvas-panel">
              <AttackCanvas phases={scopedPhaseRows} viewMode={viewMode} />
            </WorkPanel>
            <div className="taf-attack-bottom">
              <WorkPanel title="ATT&CK 阶段矩阵">
                <PhaseMatrix metrics={detail ? [{ label: '置信度', value: `${detail.risk_score}%`, delta: '服务端聚合', status: 'info' }] : data?.metrics ?? []} phases={phaseRows} />
              </WorkPanel>
              <WorkPanel title="路径明细（关键跳转）">
                <PathDetail rows={rows} columns={columns} isLoading={activeLoading} />
              </WorkPanel>
            </div>
          </main>
          <aside className="taf-attack-rail">
            <EvidenceAnchorList rows={evidenceAnchorRows} onInspect={(target) => void runChainAction('campaign-evidence-view', '查看攻击链证据', { evidence: target })} />
            <ResponseRecommendations rows={responseRows} onInspect={(target) => void runChainAction('campaign-context-action', '查看攻击链处置建议', { recommendation: target })} />
          </aside>
        </div>
      </section>
      <Drawer
        rootClassName="taf-attack-chain-detail-drawer"
        title={null}
        placement="right"
        width="min(900px, calc(100dvw - 32px))"
        open={detailOpen}
        closable={false}
        styles={{ body: { padding: 0 } }}
        onClose={closeAttackDetail}
      >
        <AttackChainDetailDrawer
          chain={selectedChain}
          phases={phaseRows}
          evidenceRows={evidenceAnchorRows}
          sourceEntity={sourceEntity}
          pending={actionMutation.isPending}
          onClose={closeAttackDetail}
          onInvestigate={(action) => {
            const encodedChain = encodeURIComponent(selectedChain?.chain_id ?? selectedChainId);
            if (action === '查看 Session 复放') return void runChainAction('campaign-evidence-view', action, { evidence_type: 'session' }).then(() => navigate(`/forensics?campaign=${encodedChain}&tab=session`));
            if (action === '拉取 PCAP') return void runChainAction('campaign-evidence-view', action, { evidence_type: 'pcap' }).then(() => navigate(`/forensics?campaign=${encodedChain}&tab=pcap`));
            if (action === '打开图谱路径') return void runChainAction('campaign-graph-view', action).then(() => navigate(`/graph?campaign=${encodedChain}`));
            if (action === '触发 SOAR 剧本') return void runChainAction('campaign-soar-response', action, { dry_run: true }).then(() => navigate(`/playbooks?campaign=${encodedChain}`));
            return void runChainAction('campaign-context-action', action).then(() => navigate(`/forensics?campaign=${encodedChain}&create=1`));
          }}
          onSubmit={() => void runChainAction('campaign-status-change', '提交攻击链调查结论', { next_status: 'contained' }).then(closeAttackDetail).catch(() => {})}
        />
      </Drawer>
    </div>
  );
}

function AttackCanvas({ phases, viewMode }: { phases: string[][]; viewMode: string }) {
  if (!phases.length) {
    return <div className="taf-attack-canvas is-empty"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前资产范围没有匹配的攻击链阶段" /></div>;
  }
  return (
    <div className={`taf-attack-canvas ${attackViewClass(viewMode)}`} data-view-mode={viewMode}>
      <div className="taf-attack-lane-head">
        <span>攻击阶段</span>
        <small>MITRE ATT&CK</small>
      </div>
      <div className="taf-attack-lanes">
        {['攻击阶段', '实体 / 资产', '告警事件', '证据锚点', '处置动作'].map((lane) => (
          <strong key={lane}>{lane}</strong>
        ))}
      </div>
      <div className="taf-attack-chain-columns">
        {phases.map(([phase, technique, entity, alert, evidence, action, tone], index) => (
          <div key={phase} className={`taf-attack-column is-${tone}`}>
            <div className="taf-attack-phase-card">
              <b>{index + 1}</b>
              <span>{phase}</span>
              <small>{technique}</small>
            </div>
            <div className="taf-attack-entity-card">
              <NodeIcon tone={tone} />
              <span>{entity}</span>
            </div>
            <div className="taf-attack-alert-card">
              <AlertOutlined />
              <span>{alert}</span>
              <small>06-19 01:{String(12 + index * 6).padStart(2, '0')}:08</small>
            </div>
            <div className="taf-attack-evidence-card">
              <FileSearchOutlined />
              <span>{evidence}</span>
              <small>pcap / sysmon / session</small>
            </div>
            <div className="taf-attack-action-card">
              <SafetyCertificateOutlined />
              <span>{action}</span>
              <small>{index < 2 ? '低影响' : index < 4 ? '中影响' : '需审批'}</small>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function PhaseMatrix({ metrics, phases }: { metrics: PageSnapshot['metrics']; phases: string[][] }) {
  const confidence = metrics.find((item) => item.label === '置信度')?.value ?? '92%';
  return (
    <div className="taf-attack-matrix">
      {phases.map(([phase, technique, , , , , tone]) => (
        <button key={phase} type="button" className={`is-${tone}`}>
          <strong>{phase}</strong>
          <span>{technique}</span>
          <i />
          <small>已发生</small>
        </button>
      ))}
      <div className="taf-attack-confidence">
        <span>链路置信度</span>
        <strong>{confidence}</strong>
      </div>
    </div>
  );
}

function PathDetail({ rows, columns, isLoading }: { rows: SnapshotRow[]; columns: ColumnsType<SnapshotRow>; isLoading: boolean }) {
  return (
    <Table
      rowKey={(record) => String(record['阶段'] ?? JSON.stringify(record))}
      size="small"
      loading={isLoading}
      pagination={false}
      columns={columns}
      dataSource={rows.slice(0, 5)}
    />
  );
}

function EvidenceAnchorList({ rows, onInspect }: { rows: string[][]; onInspect: (target: string) => void }) {
  const [selectedTab, setSelectedTab] = useState('全部');
  const filteredRows = selectedTab === '全部'
    ? rows
    : rows.filter(([, type]) => type === selectedTab || (selectedTab === '告警' && type.includes('告警')));
  return (
    <WorkPanel title="证据锚点">
      <div className="taf-attack-tabs">
        {['全部', '告警', 'PCAP', 'Session', '日志', '图谱', '规则/模型'].map((tab) => (
          <button key={tab} type="button" className={selectedTab === tab ? 'is-active' : ''} onClick={() => setSelectedTab(tab)}>{tab}</button>
        ))}
      </div>
      <div className="taf-attack-evidence-table">
        <div>
          <span>阶段</span>
          <span>类型</span>
          <span>名称</span>
          <span>时间</span>
          <span>完整度</span>
        </div>
        {filteredRows.map(([phase, type, name, time, integrity]) => (
          <button key={name} type="button" onClick={() => onInspect(name)}>
            <StatusTag value={phase} />
            <span>{type}</span>
            <strong>{name}</strong>
            <em>{time}</em>
            <em>{integrity}</em>
          </button>
        ))}
        {!filteredRows.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`暂无${selectedTab}证据`} />}
      </div>
    </WorkPanel>
  );
}

function ResponseRecommendations({ rows, onInspect }: { rows: string[][]; onInspect: (target: string) => void }) {
  const [selectedTab, setSelectedTab] = useState('阻断点');
  return (
    <WorkPanel title="处置建议">
      <div className="taf-attack-suggestion-tabs">
        {['阻断点', '隔离建议', '白名单风险', '剧本推荐'].map((item) => (
          <button key={item} type="button" className={selectedTab === item ? 'is-active' : ''} onClick={() => setSelectedTab(item)}>{item}</button>
        ))}
      </div>
      <div className="taf-attack-recommendations">
        {rows.map(([priority, target, action, impact], index) => (
          <button key={`${target}-${action}`} type="button" onClick={() => onInspect(`${selectedTab}:${target}:${action}`)}>
            <b>{index + 1}</b>
            <StatusTag value={priority} />
            <span>{target}</span>
            <strong>{action}</strong>
            <em>{impact}</em>
            <EyeOutlined />
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function AttackChainDetailDrawer({
  chain,
  phases,
  evidenceRows,
  sourceEntity,
  pending,
  onClose,
  onInvestigate,
  onSubmit,
}: {
  chain?: AttackChainDetail;
  phases: string[][];
  evidenceRows: string[][];
  sourceEntity: string;
  pending: boolean;
  onClose: () => void;
  onInvestigate: (action: string) => void;
  onSubmit: () => void;
}) {
  const [drawerSearch, setDrawerSearch] = useSearchParams();
  const requestedTab = drawerSearch.get('detailTab');
  const [activeTab, setActiveTab] = useState(requestedTab && ['detail', 'evidence', 'audit'].includes(requestedTab) ? requestedTab : 'detail');
  useEffect(() => {
    setActiveTab(requestedTab && ['detail', 'evidence', 'audit'].includes(requestedTab) ? requestedTab : 'detail');
  }, [requestedTab]);
  const changeTab = (tab: string) => {
    setActiveTab(tab);
    const next = new URLSearchParams(drawerSearch);
    if (tab === 'detail') next.delete('detailTab');
    else next.set('detailTab', tab);
    setDrawerSearch(next, { replace: true });
  };
  const current = phases[Math.max(0, phases.length - 1)];
  const source = sourceEntity || phases[0]?.[2] || '未提供';
  const target = phases[phases.length - 1]?.[2] || '未提供';
  const chainId = chain?.chain_id || '未提供';
  const confidence = Number.isFinite(chain?.risk_score) ? `${chain?.risk_score}%` : '未提供';
  const latestHit = formatAttackTimestamp(chain?.end_time);
  const status = chain?.status || '待调查';
  const severity = (chain?.risk_score ?? 0) >= 80 ? '高危' : (chain?.risk_score ?? 0) >= 50 ? '中危' : '低危';
  const assetCount = Math.max(0, chain?.entity_count ?? 0);
  return (
    <div className="taf-attack-chain-drawer-content">
      <header>
        <div><h2>攻击链详情</h2><p>链路 {chainId} / 节点：{current?.[0] || '未提供'} / 置信度 {confidence}</p></div>
        <Space>
          <StatusTag value={status} />
          <StatusTag value={severity} />
          <b>证据 {evidenceRows.length}</b>
          <Button type="text" aria-label="关闭攻击链详情" icon={<CloseOutlined />} onClick={onClose} />
        </Space>
      </header>
      <section className="taf-attack-chain-drawer-summary">
        {[
          ['链路名称', chain?.title || chainId],
          ['当前阶段', current?.[0] || '未提供'],
          ['起点资产', source],
          ['终点资产', target],
          ['最近命中', latestHit],
          ['关联告警', `${chain?.alert_count ?? 0} 条`],
        ].map(([label, value]) => <span key={label}><b>{label}</b><strong>{value}</strong></span>)}
      </section>
      <Tabs
        className="taf-detail-drawer-tabs"
        activeKey={activeTab}
        onChange={changeTab}
        items={[
          { key: 'detail', label: '详情' },
          { key: 'evidence', label: '证据' },
          { key: 'audit', label: '审计' },
        ]}
      />
      <div className="taf-attack-chain-drawer-grid" data-active-tab={activeTab}>
        <aside>
          <WorkPanel title="节点上下文" className="is-tab-detail">
            <div className="taf-attack-chain-drawer-phases">
              {phases.map(([phase], index) => <span key={phase} className={phase === current?.[0] ? 'is-current' : ''}><i />{phase}<b>{index === 3 ? '›' : ''}</b></span>)}
            </div>
          </WorkPanel>
          <WorkPanel title="节点属性" className="is-tab-detail">
            <dl><dt>源资产</dt><dd>{chain?.source_ip || source}</dd><dt>目标资产</dt><dd>{target}</dd><dt>MITRE 技术</dt><dd>{chain?.mitre_techniques?.join('、') || '未提供'}</dd><dt>实体数量</dt><dd>{assetCount}</dd><dt>根告警</dt><dd>{chain?.root_alert_id || '未提供'}</dd><dt>时间窗口</dt><dd>{formatAttackTimestamp(chain?.start_time)} ~ {latestHit}</dd><dt>置信度</dt><dd>{confidence}</dd></dl>
          </WorkPanel>
        </aside>
        <main>
          <WorkPanel title="证据与命中规则" className="is-tab-evidence">
            <h4>证据列表（{evidenceRows.length}）</h4>
            <div className="taf-attack-chain-drawer-evidence">
              <div><span>证据类型</span><span>对象</span><span>命中时间</span><span>风险</span><span>Hash 校验</span></div>
              {evidenceRows.map(([phase, type, name, time, integrity]) => (
                <span key={`${phase}-${name}`}><b>{type}</b><em>{name}</em><em>{time}</em><StatusTag value={integrity} /><em>--</em></span>
              ))}
              {!evidenceRows.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无真实证据" />}
            </div>
            <h4>命中规则（{Math.min(3, phases.length)}）</h4>
            <div className="taf-attack-chain-drawer-rules">
              {phases.slice(0, 3).map(([phase, technique, , alert]) => <span key={phase}><b>{technique}</b><strong>{alert}</strong><StatusTag value="待复核" /></span>)}
            </div>
          </WorkPanel>
          <WorkPanel title="链路预览" className="is-tab-detail">
            <div className="taf-attack-chain-drawer-preview">
              {phases.slice(0, 5).map(([phase, , entity], index) => <span key={phase} className={phase === current?.[0] ? 'is-current' : ''}><i><NodeIndexOutlined /></i><b>{entity}</b><small>{phase}</small>{index < Math.min(4, phases.length - 1) && <em>→</em>}</span>)}
            </div>
          </WorkPanel>
        </main>
        <aside>
          <WorkPanel title="关联资产（影响数量）" className="is-tab-detail">
            <div className="taf-attack-chain-drawer-assets">{[
              ['受影响实体', assetCount],
              ['阶段节点', phases.length],
              ['关联告警', chain?.alert_count ?? 0],
              ['证据锚点', evidenceRows.length],
              ['MITRE 技术', chain?.mitre_techniques?.length ?? 0],
            ].map(([item, count]) => <span key={String(item)}><b>{item}</b><strong>{count} 个</strong></span>)}</div>
          </WorkPanel>
          <WorkPanel title="下一步调查" className="is-tab-detail">
            <div className="taf-attack-chain-drawer-next">
              {['查看 Session 复放', '拉取 PCAP', '打开图谱路径', '触发 SOAR 剧本', '生成取证任务'].map((item) => (
                <Popconfirm
                  key={item}
                  title={`确认执行“${item}”？`}
                  description="操作将经过服务端权限校验并写入审计留痕。"
                  okText="确认执行"
                  cancelText="取消"
                  okButtonProps={{ loading: pending }}
                  onConfirm={() => onInvestigate(item)}
                >
                  <button type="button" disabled={pending}>{item}<b>›</b></button>
                </Popconfirm>
              ))}
            </div>
          </WorkPanel>
          <WorkPanel title="权限与审批" className="is-tab-audit">
            <Alert type="warning" showIcon message="拉取原始载荷需要二次审批" />
            <dl><dt>权限校验</dt><dd>由服务端按当前令牌校验</dd><dt>操作人</dt><dd>当前登录用户</dd><dt>审计编号</dt><dd>操作成功后由服务端生成</dd></dl>
          </WorkPanel>
        </aside>
      </div>
      <footer>
        <Alert type="warning" showIcon message="继续调查将访问脱敏载荷与原始 PCAP，需要审批留痕" />
        <Space>
          <Button onClick={onClose}>关闭</Button>
          <Button onClick={() => navigator.clipboard?.writeText(chainId)}>复制链路 ID</Button>
          <Popconfirm
            title="确认生成取证任务？"
            description="任务将访问受控证据并写入审计留痕。"
            okText="确认生成"
            cancelText="取消"
            okButtonProps={{ loading: pending }}
            onConfirm={() => onInvestigate('生成取证任务')}
          >
            <Button disabled={pending}>生成取证任务</Button>
          </Popconfirm>
          <Popconfirm
            title="确认提交调查结论？"
            description="提交后攻击链状态将进入处置流程。"
            okText="确认提交"
            cancelText="取消"
            okButtonProps={{ loading: pending }}
            onConfirm={onSubmit}
          >
            <Button type="primary" disabled={pending}>提交调查结论</Button>
          </Popconfirm>
        </Space>
      </footer>
    </div>
  );
}

function exportAttackChain(rows: SnapshotRow[]) {
  const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `attack-chain-${new Date().toISOString().slice(0, 10)}.json`;
  anchor.click();
  URL.revokeObjectURL(url);
}

function NodeIcon({ tone }: { tone: unknown }) {
  if (tone === 'risk') return <ApiOutlined />;
  if (tone === 'warn') return <BranchesOutlined />;
  if (tone === 'ok') return <SafetyCertificateOutlined />;
  return <NodeIndexOutlined />;
}

const renderAttackCell = (column: string, value: unknown): ReactNode => {
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '证据') return <span className="taf-attack-evidence-cell">{String(value ?? '')}</span>;
  return String(value ?? '');
};

const formatAttackTimestamp = (value?: number) => {
  if (!value || !Number.isFinite(value)) return '未提供';
  const milliseconds = value < 10_000_000_000 ? value * 1000 : value;
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(milliseconds)).replace(/\//g, '-');
};

const evidenceType = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('pcap') || normalized.includes('packet')) return 'PCAP';
  if (normalized.includes('session') || normalized.includes('会话')) return 'Session';
  if (normalized.includes('graph') || normalized.includes('图谱')) return '图谱';
  if (normalized.includes('rule') || normalized.includes('model') || normalized.includes('规则') || normalized.includes('模型')) return '规则/模型';
  if (normalized.includes('log') || normalized.includes('日志') || normalized.includes('sysmon')) return '日志';
  return '告警';
};

const phaseTone = (confidence: number, severity?: string) => {
  if (severity?.toLowerCase().includes('high') || confidence >= 0.85 || confidence >= 85) return 'risk';
  if (severity?.toLowerCase().includes('medium') || confidence >= 0.6 || confidence >= 60) return 'warn';
  return confidence > 0 ? 'ok' : 'info';
};

const attackPhaseStatusTone = (status: unknown) => {
  const normalized = String(status ?? '').trim();
  if (/待确认|未确认|待复核|未知/.test(normalized)) return 'warn';
  if (/已确认|已完成|稳定|已处置/.test(normalized)) return 'ok';
  return 'warn';
};

const attackViewClass = (viewMode: string) => {
  if (viewMode === '泳道视图') return 'is-swimlane-view';
  if (viewMode === '矩阵视图') return 'is-matrix-view';
  return 'is-chain-view';
};

const recommendedAction = (phase: string, entity: string) => {
  if (phase.includes('外传') || phase.includes('C2')) return `阻断 ${entity}`;
  if (phase.includes('横向')) return `隔离 ${entity}`;
  if (phase.includes('执行')) return `终止 ${entity} 可疑进程`;
  if (phase.includes('访问')) return `加固 ${entity} 入口规则`;
  return `复核 ${entity}`;
};

function attackPhaseToRow(phase: AttackChainPhase, detail: AttackChainDetail, index: number): SnapshotRow {
  const event = phase.key_events[0];
  const entity = event?.dst_ip || event?.src_ip || detail.source_ip || '未提供';
  const evidence = event?.event_id || event?.description || '未提供';
  const confidence = Number(phase.confidence ?? 0);
  return {
    阶段: phase.phase || `阶段 ${index + 1}`,
    技术: event?.technique || detail.mitre_techniques[index] || '未提供',
    实体: entity,
    告警: event?.description || phase.alert_ids[0] || '未提供',
    证据: evidence,
    处置建议: recommendedAction(phase.phase, entity),
    状态: phase.alert_ids.length || phase.key_events.length ? '已确认' : '待确认',
    风险: phaseTone(confidence, event?.severity),
  };
}

function visualAttackChainDetail(): AttackChainDetail {
  const now = Date.now();
  return {
    chain_id: 'AC-2026-019',
    tenant_id: 'visual-tenant',
    title: '疑似 C2 隧道通信攻击链',
    description: 'UI 验收态攻击链',
    phases: visualPhases.map(([phase, technique, entity, alert], index) => ({
      phase,
      alert_ids: [`ALERT-${String(index + 1).padStart(3, '0')}`],
      start_time: now - (visualPhases.length - index) * 3600_000,
      end_time: now - (visualPhases.length - index - 1) * 3600_000,
      confidence: 92 - index,
      key_events: [{
        event_id: visualEvidenceRows[index]?.[2] ?? `evidence-${index + 1}`,
        timestamp: now - (visualPhases.length - index - 1) * 3600_000,
        description: alert,
        src_ip: index ? visualPhases[index - 1][2] : entity,
        dst_ip: entity,
        technique,
        severity: index >= 4 ? 'high' : 'medium',
      }],
    })),
    risk_score: 88,
    root_alert_id: 'ALERT-001',
    source_ip: visualPhases[0][2],
    entity_count: 6,
    alert_count: 6,
    start_time: now - 6 * 3600_000,
    end_time: now,
    status: '调查中',
    mitre_techniques: visualPhases.map((row) => row[1]),
  };
}
