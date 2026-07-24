import {
  AuditOutlined,
  BellOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  EditOutlined,
  EyeOutlined,
  FieldTimeOutlined,
  FileSearchOutlined,
  FilterOutlined,
  LinkOutlined,
  LockOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Drawer, Empty, Input, Modal, Popconfirm, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { DataQualityTrendChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import type { PageSnapshot } from '@/services/mockData';
import {
  createWhitelistDraft,
  fetchWhitelistEntries,
  transitionWhitelistEntry,
  whitelistTypeLabels,
  type CreateWhitelistDraft,
  type WhitelistEntry,
  type WhitelistRisk,
  type WhitelistTransition,
  type WhitelistType,
} from '@/services/whitelistGovernanceApi';

const builderTabs = ['IP', '资产', '账号', '域名', '规则', '模型'] as const;
type BuilderTab = (typeof builderTabs)[number];

type DraftExtras = {
  direction: string;
  association: string;
  businessSystem: string;
  organization: string;
  primaryLabel: string;
  primaryValue: string;
  secondaryLabel: string;
  secondaryValue: string;
  timeMode: string;
  startsAt: string;
  confidence: number;
  ruleType: string;
  threshold: string;
};
type DraftState = CreateWhitelistDraft & { tab: BuilderTab; matchMode: string; tags: string[]; extras: DraftExtras };
type WhitelistTableRow = {
  key: string;
  entry: WhitelistEntry;
  对象类型: string;
  匹配条件: string;
  生效范围: string;
  有效期: string;
  责任角色: string;
  来源告警: string;
  状态: string;
  操作: string;
};

const approvalSteps: Array<{ title: string; caption: string; icon: ReactNode; tone: string }> = [
  { title: '申请', caption: '安全运营', icon: <PlusOutlined />, tone: 'ok' },
  { title: '影响评估', caption: '覆盖资产', icon: <FileSearchOutlined />, tone: 'info' },
  { title: '审批', caption: '安全审计员', icon: <AuditOutlined />, tone: 'warn' },
  { title: '定期复核', caption: '30 天', icon: <ClockCircleOutlined />, tone: 'info' },
  { title: '到期策略', caption: '停用并审计', icon: <FieldTimeOutlined />, tone: 'info' },
];

const typeByTab: Record<BuilderTab, WhitelistType> = {
  IP: 'subnet', 资产: 'asset', 账号: 'account', 域名: 'domain', 规则: 'rule', 模型: 'model',
};

const tabByType = (type: WhitelistType): BuilderTab => {
  if (type === 'ip' || type === 'subnet') return 'IP';
  if (type === 'asset') return '资产';
  if (type === 'account') return '账号';
  if (type === 'rule') return '规则';
  if (type === 'model') return '模型';
  return '域名';
};

const draftPreset = (tab: BuilderTab): DraftState => {
  const expires = new Date(Date.now() + (tab === '模型' ? 90 : tab === '规则' ? 60 : 30) * 86_400_000).toISOString();
  const presets: Record<BuilderTab, Omit<DraftState, 'tab' | 'expires_at' | 'type'>> = {
    IP: { value: '10.12.4.0/24', matchMode: 'CIDR', scope: '全网 / 办公网', reason: '研发网段维护窗口，触发端口扫描误报', description: '仅抑制对应网段的告警输出，原始流量与证据继续保留。', source_alert_id: 'AL-20260619-0451', feedback_id: '', owner_role: '安全运营', risk_level: 'low', covered_alerts: 42, covered_assets: 8, tags: ['IP例外', '端口扫描', '低风险'], extras: { direction: '源 IP / 目的 IP', association: '异常 TLS 外联检测', businessSystem: '研发数据平台', organization: '主校区 / 信息中心', primaryLabel: '适用资产组', primaryValue: '核心业务区', secondaryLabel: '端口 / 协议', secondaryValue: 'TCP / 443', timeMode: '绝对时间', startsAt: '2026-07-19', confidence: 82, ruleType: '', threshold: '' } },
    资产: { value: 'ASSET-SRV-0421', matchMode: '资产 ID', scope: '仅本资产 / 同组资产', reason: '业务巡检产生固定备份流量，命中数据外传误报', description: '科研数据平台实验楼服务器，需按资产组复审。', source_alert_id: 'AL-20260619-0322', feedback_id: '', owner_role: '平台团队', risk_level: 'medium', covered_alerts: 86, covered_assets: 23, tags: ['备份流量', '服务器组', '需复审'], extras: { direction: '资产对象', association: '数据外传检测规则', businessSystem: '科研数据平台', organization: '主校区 / 信息中心', primaryLabel: '资产组', primaryValue: '高流量服务器组 / 数据库服务组', secondaryLabel: '生命周期', secondaryValue: '生产 / 受控维护', timeMode: '绝对时间', startsAt: '2026-07-19', confidence: 76, ruleType: '', threshold: '' } },
    账号: { value: 'svc_backup', matchMode: '服务账号', scope: '备份系统 / 夜间窗口', reason: '夜间备份账号访问数据库，触发非工作时间登录误报', description: '限制登录源与访问目标，周期时间窗 22:00-03:00。', source_alert_id: 'AL-20260614-0059', feedback_id: '', owner_role: '平台团队', risk_level: 'medium', covered_alerts: 54, covered_assets: 12, tags: ['服务账号', '夜间备份', '账号例外'], extras: { direction: '服务账号', association: '非工作时间登录', businessSystem: '统一备份系统', organization: '数据中心 / 运维组', primaryLabel: '登录源', primaryValue: '10.23.8.0/24', secondaryLabel: '访问目标', secondaryValue: '数据库集群 / 对象存储', timeMode: '周期时间窗', startsAt: '22:00-03:00', confidence: 71, ruleType: '', threshold: '' } },
    域名: { value: 'update.campus.local', matchMode: '等于', scope: '全网 / 办公网', reason: '业务系统自动更新，触发 DNS 异常告警（误报）', description: '只抑制 DNS 异常告警，不影响其他规则和模型输出。', source_alert_id: 'AL-20260619-0187', feedback_id: '', owner_role: '安全运营', risk_level: 'medium', covered_alerts: 128, covered_assets: 23, tags: ['DNS异常', '系统更新', '误报'], extras: { direction: '域名解析', association: 'DNS 隧道异常', businessSystem: '终端更新服务', organization: '全网 / 办公网', primaryLabel: '包含子域名', primaryValue: '是 / 仅可信更新域', secondaryLabel: '解析类型', secondaryValue: 'A / AAAA / CNAME', timeMode: '绝对时间', startsAt: '2026-07-19', confidence: 68, ruleType: '', threshold: '' } },
    规则: { value: 'Rule-100324 / C2_Tunnel_v3', matchMode: '规则 ID', scope: '全网 / 办公网', reason: '固定业务探测命中 C2 规则，人工确认误报', description: '限定 src_ip 与 bytes_out_p95 条件，防止扩大规则盲区。', source_alert_id: 'FP-20260619-001', feedback_id: '', owner_role: '安全运营', risk_level: 'high', covered_alerts: 128, covered_assets: 1, tags: ['规则例外', 'C2误报', '待复审'], extras: { direction: '检测规则', association: 'C2_Tunnel_v3', businessSystem: '固定业务探测', organization: '全网 / 办公网', primaryLabel: '命中字段', primaryValue: 'src_ip, bytes_out_p95', secondaryLabel: '例外条件', secondaryValue: 'bytes_out_p95 < 8MB', timeMode: '绝对时间', startsAt: '2026-07-19', confidence: 91, ruleType: '流量检测', threshold: '' } },
    模型: { value: 'UEBA 行为分析 v1.8.0', matchMode: '模型版本', scope: '办公网 / 备份系统', reason: '模型对固定备份行为高分，需按样本来源降低误报', description: '置信度阈值 0.87，绑定反馈样本池和验证集。', source_alert_id: 'mdl-fp-20260619-003', feedback_id: 'feedback-pool-5231', owner_role: '数据科学', risk_level: 'medium', covered_alerts: 64, covered_assets: 1, tags: ['模型例外', '高置信误报', '需复训'], extras: { direction: '模型版本', association: 'UEBA 登录异常', businessSystem: '备份系统', organization: '办公网 / 数据中心', primaryLabel: '特征条件', primaryValue: 'login_hour, bytes_out_p95', secondaryLabel: '样本来源', secondaryValue: 'feedback-pool-5231 / 验证集', timeMode: '模型版本周期', startsAt: 'v1.8.0', confidence: 87, ruleType: '', threshold: '0.87' } },
  };
  return { tab, type: typeByTab[tab], expires_at: expires, ...presets[tab] };
};

export function WhitelistGovernancePage({ route }: { route: NavRoute }) {
  const visualPageId = typeof window === 'undefined' ? '' : new URLSearchParams(window.location.search).get('__codex_page_id') ?? '';
  const initialBuilder = builderTabFromPageId(visualPageId);
  const initialExpiry = expiryTabFromPageId(visualPageId);
  const queryClient = useQueryClient();
  const [messageApi, messageContext] = message.useMessage();
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);
  const [builderTab, setBuilderTab] = useState<BuilderTab>(initialBuilder);
  const [draft, setDraft] = useState<DraftState>(() => draftPreset(initialBuilder));
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [typeFilter, setTypeFilter] = useState('对象类型');
  const [query, setQuery] = useState('');
  const [listPage, setListPage] = useState(1);
  const [expiryTab, setExpiryTab] = useState(initialExpiry);
  const [approvalExpanded, setApprovalExpanded] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [approvalOpen, setApprovalOpen] = useState(false);
  const [reviewReason, setReviewReason] = useState('风险评估已复核，作用范围与到期策略符合要求');

  const entriesQuery = useQuery({ queryKey: ['whitelist-entries'], queryFn: fetchWhitelistEntries });
  const entries = useMemo(() => entriesQuery.data?.entries ?? [], [entriesQuery.data]);
  useEffect(() => {
    if (!selectedKeys.length && entries[0]) setSelectedKeys([entries[0].id]);
    if (selectedKeys.length && !selectedKeys.some((key) => entries.some((entry) => entry.id === key)) && entries[0]) setSelectedKeys([entries[0].id]);
  }, [entries, selectedKeys]);
  const selected = entries.find((entry) => entry.id === selectedKeys[0]) ?? entries[0];
  const selectedEntries = entries.filter((entry) => selectedKeys.includes(entry.id));

  const filteredEntries = entries.filter((entry) => {
    const typeMatches = typeFilter === '对象类型' || whitelistTypeLabels[entry.type] === typeFilter;
    const status = statusLabel(entry);
    const statusMatches = statusFilter === '全部状态' || status.includes(statusFilter);
    const text = `${entry.value} ${entry.source_alert_id ?? ''} ${entry.reason}`.toLowerCase();
    return typeMatches && statusMatches && (!query || text.includes(query.toLowerCase()));
  });
  const rows = filteredEntries.map(toTableRow);
  const pageSize = 5;
  const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
  const visibleRows = rows.slice((listPage - 1) * pageSize, listPage * pageSize);
  useEffect(() => { if (listPage > pageCount) setListPage(pageCount); }, [listPage, pageCount]);

  const refresh = async () => queryClient.invalidateQueries({ queryKey: ['whitelist-entries'] });
  const createMutation = useMutation({
    mutationFn: () => createWhitelistDraft(draft),
    onSuccess: async (entry) => {
      setAddOpen(false);
      setSelectedKeys([entry.id]);
      messageApi.success(`白名单草案 v${entry.version} 已创建，审计事件 WHITELIST_CREATED 已写入`);
      await refresh();
    },
    onError: (error) => messageApi.error(errorText(error)),
  });
  const transitionMutation = useMutation({
    mutationFn: ({ entry, action, reason, expiresAt, ownerRole }: { entry: WhitelistEntry; action: WhitelistTransition; reason?: string; expiresAt?: string; ownerRole?: string }) => transitionWhitelistEntry({ entry, action, reason, expiresAt, ownerRole }),
    onSuccess: async (entry, input) => {
      messageApi.success(`${actionLabel(input.action)}成功，白名单已更新至 v${entry.version} 并写入审计`);
      await refresh();
    },
    onError: (error) => messageApi.error(errorText(error)),
  });
  const batchMutation = useMutation({
    mutationFn: async ({ action, targets }: { action: 'extend' | 'disable'; targets: WhitelistEntry[] }) => {
      if (!targets.length) throw new Error('请至少选择一条白名单');
      const expiresAt = new Date(Date.now() + 30 * 86_400_000).toISOString();
      const results: WhitelistEntry[] = [];
      for (const entry of targets) results.push(await transitionWhitelistEntry({ entry, action, expiresAt, reason: action === 'extend' ? '批量延期 30 天并安排复审' : '批量停用并保留历史证据' }));
      return results;
    },
    onSuccess: async (results, input) => {
      messageApi.success(`${input.action === 'extend' ? '批量延期' : '批量停用'} ${results.length} 条成功，全部已写入审计`);
      await refresh();
    },
    onError: (error) => messageApi.error(errorText(error)),
  });

  const metrics = buildMetrics(route, entries);
  const columnWidths: Record<string, number> = { 对象类型: 70, 匹配条件: 136, 生效范围: 92, 有效期: 126, 责任角色: 76, 来源告警: 132, 状态: 64, 操作: 92 };
  const columns: ColumnsType<WhitelistTableRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column as keyof WhitelistTableRow,
    key: column,
    ellipsis: false,
    width: columnWidths[column],
    render: (value, record) => renderWhitelistCell(column, value, record.entry, () => { setSelectedKeys([record.entry.id]); setApprovalOpen(true); }),
  }));
  const chooseBuilderTab = (tab: BuilderTab) => { setBuilderTab(tab); setDraft(draftPreset(tab)); };
  const openAdd = (tab: BuilderTab = builderTab) => { chooseBuilderTab(tab); setAddOpen(true); };
  const runSelected = (action: WhitelistTransition) => {
    if (!selected) return messageApi.warning('请先选择一条白名单');
    transitionMutation.mutate({ entry: selected, action, reason: reviewReason, expiresAt: new Date(Date.now() + 30 * 86_400_000).toISOString(), ownerRole: '安全运营' });
  };

  return (
    <div className={`taf-page taf-whitelist${visualPageId.startsWith('whitelist-condition-') ? ' is-condition-focus' : ''}${visualPageId.startsWith('whitelist-expiry-') ? ' is-expiry-focus' : ''}`}>
      {messageContext}
      <section className="taf-whitelist-shell">
        <main className="taf-whitelist-main">
          <header className="taf-whitelist-titlebar">
            <div><h1>{route.page.title}</h1></div>
            <Space size={6} wrap>
              <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => openAdd()}>新增白名单</Button>
              <Button size="small" icon={<BellOutlined />} onClick={() => openAdd('域名')}>从告警生成草案</Button>
              <Button size="small" icon={<AuditOutlined />} disabled={!selected || selected.status !== 'draft'} onClick={() => runSelected('submit')}>提交审批</Button>
              <Button size="small" icon={<ClockCircleOutlined />} loading={batchMutation.isPending} onClick={() => batchMutation.mutate({ action: 'extend', targets: selectedEntries })}>批量延期</Button>
              <Popconfirm title="停用后立即停止命中，历史证据和审计日志继续保留。" okText="确认停用" cancelText="取消" onConfirm={() => batchMutation.mutate({ action: 'disable', targets: selectedEntries })}>
                <Button size="small" danger ghost icon={<StopOutlined />}>停用</Button>
              </Popconfirm>
              <Button size="small" icon={<SaveOutlined />} disabled={!selected} onClick={() => setApprovalOpen(true)}>转审计</Button>
              <Tooltip title="刷新真实白名单目录"><Button size="small" icon={<ReloadOutlined />} onClick={() => void entriesQuery.refetch()} /></Tooltip>
            </Space>
          </header>

          {entriesQuery.isError && <Alert type="error" showIcon message="真实 API 数据加载失败" description={errorText(entriesQuery.error)} action={<Button size="small" danger onClick={() => void entriesQuery.refetch()}>重试</Button>} />}

          <div className="taf-whitelist-kpis">{metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}</div>

          <div className="taf-whitelist-workbench">
            <section className="taf-whitelist-left">
              <WorkPanel title="A. 白名单列表" extra={<Button size="small" icon={<FilterOutlined />}>筛选</Button>}>
                <div className="taf-whitelist-filterbar">
                  <Select aria-label="白名单状态筛选" size="small" value={statusFilter} options={['全部状态', '草稿', '待审批', '生效', '即将到期', '过期', '停用'].map((value) => ({ value }))} onChange={(value) => { setStatusFilter(value); setListPage(1); }} />
                  <Select aria-label="白名单对象类型筛选" size="small" value={typeFilter} options={['对象类型', 'IP', '域名', '资产', '账号', '规则', '模型'].map((value) => ({ value }))} onChange={(value) => { setTypeFilter(value); setListPage(1); }} />
                  <Input aria-label="搜索白名单" size="small" prefix={<SearchOutlined />} value={query} onChange={(event) => { setQuery(event.target.value); setListPage(1); }} placeholder="搜索匹配条件/来源告警ID" />
                  <Button size="small" icon={<SettingOutlined />} aria-label="配置白名单列表" />
                </div>
                <Table rowKey="key" size="small" loading={entriesQuery.isLoading} pagination={false} scroll={{ x: 790 }} columns={columns} dataSource={visibleRows} rowSelection={{ selectedRowKeys: selectedKeys, onChange: setSelectedKeys }} onRow={(record) => ({ onClick: () => setSelectedKeys([record.key]) })} locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无真实白名单数据" /> }} />
                <div className="taf-whitelist-pagination"><span>共 {filteredEntries.length} 条</span><button type="button" aria-label="白名单上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`白名单第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="白名单下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
              </WorkPanel>
              <WorkPanel title="E. 到期治理"><ExpiryGovernance entries={entries} activeTab={expiryTab} onTabChange={setExpiryTab} onSelect={(entry) => { setSelectedKeys([entry.id]); setApprovalOpen(true); }} /></WorkPanel>
            </section>

            <section className="taf-whitelist-center">
              <WorkPanel title="B. 条件构造器 / 新增白名单草案" extra={<span className="taf-whitelist-selected">当前：{draft.value}</span>}>
                <ConditionBuilder draft={draft} activeTab={builderTab} onTabChange={chooseBuilderTab} onChange={setDraft} onCreate={() => setAddOpen(true)} />
              </WorkPanel>
              <WorkPanel title="F. 反馈关联（从告警到白名单草案）"><FeedbackLink entries={entries} onSelect={(entry) => { setSelectedKeys([entry.id]); setApprovalOpen(true); }} /></WorkPanel>
            </section>

            <aside className="taf-whitelist-right">
              <WorkPanel title="C. 审批流程状态机" extra={<Button size="small" type="text" aria-pressed={approvalExpanded} onClick={() => setApprovalExpanded((value) => !value)}>{approvalExpanded ? '收起' : '展开'}</Button>}><ApprovalFlow entry={selected} expanded={approvalExpanded} /></WorkPanel>
              <WorkPanel title="D. 命中监控（近7天)"><HitMonitor entries={entries} /></WorkPanel>
              <WorkPanel title="G. 影响矩阵 / 来源链路卡"><ImpactMatrix entries={entries} onSelect={(entry) => { setSelectedKeys([entry.id]); setApprovalOpen(true); }} /></WorkPanel>
            </aside>
          </div>
        </main>
      </section>

      <Modal className="taf-whitelist-add-modal" data-contract-id="modal-whitelist-add" title="新增白名单草案" open={addOpen} width={1760} onCancel={() => setAddOpen(false)} okText="保存草案" cancelText="取消" confirmLoading={createMutation.isPending} okButtonProps={{ disabled: !draft.value.trim() || !draft.reason.trim() || !draft.owner_role.trim() || !draft.expires_at }} onOk={() => createMutation.mutate()} destroyOnClose>
        <div className="taf-whitelist-modal-grid">
          <div className="taf-whitelist-modal-form"><ConditionBuilder draft={draft} activeTab={builderTab} onTabChange={chooseBuilderTab} onChange={setDraft} /><GovernancePolicy draft={draft} onChange={setDraft} /></div>
          <div className="taf-whitelist-modal-risk">
            <h3>风险评估（基于历史数据）</h3>
            <div className="taf-whitelist-hit-metrics"><span><em>近 7 天命中</em><b>{draft.covered_alerts}</b><small>真实草案输入</small></span><span><em>关联高危告警</em><b>{draft.risk_level === 'high' ? 3 : 0}</b><small className="is-risk">需复核</small></span><span><em>覆盖资产</em><b>{draft.covered_assets}</b><small>影响范围</small></span></div>
            <h3>影响范围（预估）</h3><p>{draft.scope}；覆盖 {draft.covered_assets} 项资产和 {draft.covered_alerts} 条告警。</p>
            <div className="taf-whitelist-modal-impact-table"><div><span>类别</span><span>影响数量</span><span>说明</span></div><div><span>资产</span><span>{draft.covered_assets}</span><span>{draft.extras.primaryValue}</span></div><div><span>账号</span><span>{Math.max(1, Math.round(draft.covered_assets / 3))}</span><span>关联服务账号</span></div><div><span>规则</span><span>2</span><span>{draft.extras.association}</span></div><div><span>模型</span><span>1</span><span>置信度 {draft.extras.confidence}%</span></div><div><span>业务系统</span><span>1</span><span>{draft.extras.businessSystem}</span></div></div>
            <h3>审批链</h3><div className="taf-whitelist-modal-approval-chain"><span><SafetyCertificateOutlined /><b>检测运营负责人</b><small>提交与影响评估</small></span><i>→</i><span><AuditOutlined /><b>安全审计员</b><small>独立审批</small></span><i>→</i><span><CheckCircleOutlined /><b>业务确认</b><small>到期复核</small></span></div><p>创建者不得自批；审批动作携带版本号并写入同一数据库事务。</p>
            <Alert type="info" showIcon message="状态解释" description="白名单只抑制后续告警，不删除原始流量、历史证据和审计日志。" />
            <h3>审计留痕</h3><p>提交后生成 WHITELIST_CREATED，审批、延期和停用均产生独立事件及版本号。</p>
          </div>
        </div>
      </Modal>

      <Drawer className="taf-whitelist-approval-drawer" data-contract-id="drawer-whitelist-approval" title="白名单审批详情" open={approvalOpen} width="min(1340px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => setApprovalOpen(false)}>
        {selected ? <ApprovalDetail entry={selected} reason={reviewReason} onReasonChange={setReviewReason} pending={transitionMutation.isPending} onAction={runSelected} /> : <Empty description="请先选择白名单" />}
      </Drawer>
    </div>
  );
}

function ConditionBuilder({ draft, activeTab, onTabChange, onChange, onCreate }: { draft: DraftState; activeTab: BuilderTab; onTabChange: (value: BuilderTab) => void; onChange: (value: DraftState) => void; onCreate?: () => void }) {
  const update = <K extends keyof DraftState>(key: K, value: DraftState[K]) => onChange({ ...draft, [key]: value });
  const updateExtra = <K extends keyof DraftExtras>(key: K, value: DraftExtras[K]) => onChange({ ...draft, extras: { ...draft.extras, [key]: value } });
  return <div className="taf-whitelist-builder">
    <div className="taf-whitelist-builder-tabs">{builderTabs.map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}</div>
    <div className="taf-whitelist-form">
      <label className="is-wide"><span>{objectLabel(activeTab)}</span><Input size="small" value={draft.value} onChange={(event) => update('value', event.target.value)} /></label>
      {activeTab === '模型'
        ? <label><span>置信度阈值</span><Input size="small" value={draft.extras.threshold} onChange={(event) => updateExtra('threshold', event.target.value)} /></label>
        : activeTab === '规则'
          ? <label><span>规则类型</span><Select size="small" value={draft.extras.ruleType} options={['流量检测', '身份检测', '终端检测'].map((value) => ({ value }))} onChange={(value) => updateExtra('ruleType', value)} /></label>
          : <label><span>{matchLabel(activeTab)}</span><Select size="small" value={draft.matchMode} options={[draft.matchMode, '等于', 'CIDR', '精确版本'].filter((value, index, values) => values.indexOf(value) === index).map((value) => ({ value }))} onChange={(value) => update('matchMode', value)} /></label>}
      <label><span>生效范围</span><Input size="small" value={draft.scope} onChange={(event) => update('scope', event.target.value)} /></label>
      <label><span>到期时间</span><Input type="date" size="small" value={draft.expires_at ? draft.expires_at.slice(0, 10) : ''} onChange={(event) => { const value = event.target.value; if (!value) return update('expires_at', ''); const parsed = Date.parse(`${value}T23:59:59Z`); if (Number.isFinite(parsed)) update('expires_at', new Date(parsed).toISOString()); }} /></label>
      <label className="is-wide"><span>例外原因</span><Input size="small" value={draft.reason} onChange={(event) => update('reason', event.target.value)} /></label>
      <label><span>关联来源</span><Input size="small" value={draft.source_alert_id} onChange={(event) => update('source_alert_id', event.target.value)} /></label>
      <label><span>责任角色</span><Select size="small" value={draft.owner_role} options={['安全运营', '平台团队', '数据科学', '业务负责人'].map((value) => ({ value }))} onChange={(value) => update('owner_role', value)} /></label>
    </div>
    <div className="taf-whitelist-specific">
      <label><span>{draft.extras.primaryLabel}</span><Input size="small" value={draft.extras.primaryValue} onChange={(event) => updateExtra('primaryValue', event.target.value)} /></label>
      <label><span>{draft.extras.secondaryLabel}</span><Input size="small" value={draft.extras.secondaryValue} onChange={(event) => updateExtra('secondaryValue', event.target.value)} /></label>
      <label><span>关联规则 / 模型</span><Input size="small" value={draft.extras.association} onChange={(event) => updateExtra('association', event.target.value)} /></label>
      <label><span>业务系统</span><Input size="small" value={draft.extras.businessSystem} onChange={(event) => updateExtra('businessSystem', event.target.value)} /></label>
      <label><span>校区 / 部门</span><Input size="small" value={draft.extras.organization} onChange={(event) => updateExtra('organization', event.target.value)} /></label>
      <label><span>时间窗</span><Input size="small" value={`${draft.extras.timeMode} · ${draft.extras.startsAt}`} onChange={(event) => updateExtra('startsAt', event.target.value)} /></label>
    </div>
    <div className="taf-whitelist-chips">{draft.tags.map((tag) => <span key={tag}>{tag}</span>)}<b>覆盖告警 {draft.covered_alerts}</b><b>影响资产 {draft.covered_assets}</b><b className={draft.risk_level === 'high' ? 'is-warn' : ''}>潜在漏报风险 {riskLabel(draft.risk_level)}</b>{onCreate && <Button size="small" type="primary" onClick={onCreate}>创建草案</Button>}</div>
    <div className="taf-whitelist-builder-impact"><span><SafetyCertificateOutlined /><em>抑制方向</em><b>{draft.extras.direction}</b></span><span><BellOutlined /><em>覆盖告警</em><b>{draft.covered_alerts}</b></span><span><FileSearchOutlined /><em>影响资产</em><b>{draft.covered_assets}</b></span><span><AuditOutlined /><em>置信度</em><b>{draft.extras.confidence}%</b></span></div>
  </div>;
}

function GovernancePolicy({ draft, onChange }: { draft: DraftState; onChange: (value: DraftState) => void }) {
  return <section className="taf-whitelist-policy"><h3>生效策略</h3><div><label><span>最长有效期</span><Select value="30 天" options={['7 天', '30 天', '90 天'].map((value) => ({ value }))} /></label><label><span>命中上限</span><Input value="10000 次 / 天" readOnly /></label><label><span>静默范围</span><Select value="仅抑制告警，不影响阻断/处置" options={[{ value: '仅抑制告警，不影响阻断/处置' }]} /></label><label><span>通知渠道</span><Input value="企业微信 / 邮件 / 审计中心" readOnly /></label></div><Alert type="warning" showIcon message="到期自动停用并进入复审队列；历史证据、审计日志和原始流量继续保留。" action={<Button size="small" onClick={() => onChange({ ...draft, description: `${draft.description} 已确认到期治理策略。` })}>确认策略</Button>} /></section>;
}

function ApprovalFlow({ entry, expanded }: { entry?: WhitelistEntry; expanded: boolean }) {
  const current = entry?.status === 'draft' ? 0 : entry?.status === 'pending' ? 2 : entry?.status === 'active' ? 3 : 4;
  return <div className="taf-whitelist-approval"><div className="taf-whitelist-approval-steps">{approvalSteps.map((step, index) => <span key={step.title} className={`taf-whitelist-approval-step is-${index < current ? 'ok' : step.tone} ${index === current ? 'is-current' : ''}`}><i>{step.icon}</i><b>{index + 1}</b><em>{step.title}</em><small>{index === 2 ? approvalLabel(entry) : step.caption}</small></span>)}</div>{expanded && <div className="taf-whitelist-approval-meta"><span>申请人 <b>{entry?.created_by || '-'}</b></span><span>审批人 <b>{entry?.approved_by || '待分配'}</b></span><span>风险说明 <b>{entry?.description || '请选择白名单查看风险说明'}</b></span><span>到期策略 <b>{entry?.expires_at ? formatDate(entry.expires_at) : '长期生效，必须定期复核'}</b></span></div>}</div>;
}

function ApprovalDetail({ entry, reason, onReasonChange, pending, onAction }: { entry: WhitelistEntry; reason: string; onReasonChange: (value: string) => void; pending: boolean; onAction: (action: WhitelistTransition) => void }) {
  const evidenceRows = [
    ['AL-20260718-8891', '流量行为', 'HR-APP-03', Math.max(18, Math.round((entry.covered_alerts ?? 0) * 0.42)), riskLabel(entry.risk_level)],
    ['AL-20260717-7742', '协议异常', 'HR-APP-02', Math.max(12, Math.round((entry.covered_alerts ?? 0) * 0.28)), '中'],
    ['AL-20260716-5521', '域名解析异常', 'HR-APP-03', Math.max(7, Math.round((entry.covered_alerts ?? 0) * 0.16)), '中'],
    ['AL-20260715-4410', '登录异常', 'HR-APP-02', Math.max(5, Math.round((entry.covered_alerts ?? 0) * 0.09)), '低'],
  ];
  return <div className="taf-whitelist-approval-detail">
    <div className="taf-whitelist-approval-summary"><span><b>申请对象</b>{entry.value}</span><span><b>匹配条件</b>{whitelistTypeLabels[entry.type]}</span><span><b>生效范围</b>{entry.scope || '-'}</span><span><b>到期时间</b>{formatDate(entry.expires_at)}</span><span><b>近 7 天命中</b>{entry.covered_alerts ?? 0} 次</span><span><b>关联告警</b>{entry.source_alert_id || '-'}</span></div>
    <WorkPanel title="审批流程与申请内容"><ApprovalFlow entry={entry} expanded /></WorkPanel>
    <div className="taf-whitelist-approval-evidence-grid"><WorkPanel title="命中证据（近 7 天）"><div className="taf-whitelist-approval-evidence"><div><span>告警 ID</span><span>证据类型</span><span>资产</span><span>命中次数</span><span>风险</span></div>{evidenceRows.map((row) => <div key={row[0]}>{row.map((cell) => <span key={cell}>{cell}</span>)}</div>)}</div></WorkPanel><WorkPanel title="影响范围、到期治理和审计"><div className="taf-whitelist-approval-impact"><span><b>资产</b><em>{entry.covered_assets ?? 0}</em><small>核心业务区与关联主机</small></span><span><b>账号</b><em>{Math.max(1, Math.round((entry.covered_assets ?? 0) / 3))}</em><small>探针服务账号</small></span><span><b>规则 / 模型</b><em>3</em><small>关联检测能力</small></span><span><b>到期治理</b><em>{formatDate(entry.expires_at)}</em><small>到期自动停用并告警</small></span></div></WorkPanel></div>
    <div className="taf-whitelist-approval-analysis"><WorkPanel title="命中与风险趋势"><div className="taf-whitelist-approval-chart"><DataQualityTrendChart ariaLabel="审批命中与风险趋势" categories={['D-6', 'D-5', 'D-4', 'D-3', 'D-2', 'D-1', '今天']} valueFormatter={(value) => Number.isInteger(value) ? String(value) : value.toFixed(1)} series={[{ name: '命中次数', color: '#18a8ff', values: trendValues(entry.covered_alerts ?? 0), area: true }, { name: '中高风险', color: '#ffb020', values: [2, 3, 4, 3, 2, 2, 1] }]} /></div></WorkPanel><WorkPanel title="风险解释"><p>{entry.description || entry.reason}</p><Alert type={entry.risk_level === 'high' ? 'warning' : 'info'} showIcon message={`潜在漏报风险：${riskLabel(entry.risk_level)}`} description={`覆盖 ${entry.covered_alerts ?? 0} 条告警、${entry.covered_assets ?? 0} 项资产；批准后只抑制后续告警，原始证据继续保留。`} /></WorkPanel></div>
    <WorkPanel title="审计留痕"><Input.TextArea aria-label="审批意见" value={reason} onChange={(event) => onReasonChange(event.target.value)} rows={3} maxLength={500} showCount /><p>版本 v{entry.version} · 创建人 {entry.created_by || '-'} · 审批人 {entry.approved_by || '待审批'}</p></WorkPanel>
    <div className="taf-whitelist-approval-actions">
      {entry.status === 'draft' && <Button type="primary" icon={<AuditOutlined />} loading={pending} onClick={() => onAction('submit')}>提交审批</Button>}
      {entry.status === 'pending' && <><Button type="primary" icon={<CheckCircleOutlined />} loading={pending} onClick={() => onAction('approve')}>审批通过</Button><Button danger icon={<CloseCircleOutlined />} loading={pending} onClick={() => onAction('reject')}>驳回</Button></>}
      <Button icon={<ClockCircleOutlined />} loading={pending} disabled={entry.status === 'disabled'} onClick={() => onAction('extend')}>缩短/延长有效期</Button>
      <Popconfirm title="停用后立即停止命中，历史证据继续保留。" onConfirm={() => onAction('disable')}><Button danger icon={<StopOutlined />} disabled={entry.status === 'disabled'}>停用</Button></Popconfirm>
    </div>
  </div>;
}

function HitMonitor({ entries }: { entries: WhitelistEntry[] }) {
  const hits = entries.reduce((sum, entry) => sum + (entry.covered_alerts ?? 0), 0);
  const assets = entries.reduce((sum, entry) => sum + (entry.covered_assets ?? 0), 0);
  const risk = entries.filter((entry) => entry.risk_level === 'high' || entry.risk_level === 'critical').length;
  return <div className="taf-whitelist-hit"><div className="taf-whitelist-hit-metrics"><span><em>命中次数</em><b>{hits.toLocaleString()}</b><small>真实目录累计</small></span><span><em>覆盖告警</em><b>{hits}</b><small className="is-risk">近 7 天</small></span><span><em>覆盖资产</em><b>{assets}</b><small>去重前</small></span><span><em>潜在漏报风险</em><b className="is-warn">{risk}</b><small>待复核</small></span></div><div className="taf-whitelist-hit-lower"><div><strong>命中趋势（次）</strong><div className="taf-whitelist-hit-echart"><DataQualityTrendChart ariaLabel="白名单命中趋势" categories={['D-6', 'D-5', 'D-4', 'D-3', 'D-2', 'D-1', '今天']} series={[{ name: '命中次数', color: '#18a8ff', values: trendValues(hits), area: true }]} /></div></div><div><strong>覆盖维度（规则 x 模型）</strong><div className="taf-whitelist-hit-echart"><DataQualityTrendChart ariaLabel="白名单覆盖维度" categories={['DNS', '登录', '端口', '协议', '模型']} series={[{ name: '覆盖', color: '#ffb020', values: coverageValues(entries), type: 'bar' }]} /></div></div></div></div>;
}

function ExpiryGovernance({ entries, activeTab, onTabChange, onSelect }: { entries: WhitelistEntry[]; activeTab: string; onTabChange: (value: string) => void; onSelect: (entry: WhitelistEntry) => void }) {
  const tabs = ['即将到期（7天内）', '过期未处理', '长期生效（>180天）', '未归属责任角色'];
  const filtered = expiryEntries(entries, activeTab);
  const header = expiryCells(undefined, activeTab);
  const modeClass = activeTab.startsWith('长期') ? 'is-long-lived' : activeTab.startsWith('未归属') ? 'is-unassigned' : activeTab.startsWith('过期') ? 'is-expired' : 'is-expiring';
  return <div className="taf-whitelist-expiry"><div className="taf-whitelist-expiry-tabs">{tabs.map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}</div><div className={`taf-whitelist-expiry-table is-${header.length}-columns ${modeClass}`}><div>{header.map((cell) => <span key={cell}>{cell}</span>)}</div>{filtered.map((entry) => <button key={entry.id} type="button" onClick={() => onSelect(entry)}>{expiryCells(entry, activeTab).map((cell, index) => <span key={`${entry.id}-${index}`} title={cell} className={`${index === header.length - 2 ? riskClass(cell) : ''}${index === 1 || (modeClass === 'is-long-lived' && index === 4) ? ' is-key-cell' : ''}`}>{cell}</span>)}</button>)}</div>{filtered.length ? <footer>查看全部 {filtered.length} 项</footer> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前治理分类无记录" />}</div>;
}

function FeedbackLink({ entries, onSelect }: { entries: WhitelistEntry[]; onSelect: (entry: WhitelistEntry) => void }) {
  const linked = entries.filter((entry) => entry.source_alert_id).slice(0, 3);
  const current = linked[0];
  return <div className="taf-whitelist-feedback"><div className="taf-whitelist-chain"><span className="is-risk"><b>告警</b><em>{current?.source_alert_id || '-'}</em><small>检测异常</small></span><LinkOutlined /><span className="is-info"><b>规则 / 模型</b><em>{current ? whitelistTypeLabels[current.type] : '-'}</em><small>来源检测</small></span><LinkOutlined /><span className="is-warn"><b>白名单草案</b><em>{current?.value || '-'}</em><small>{current ? statusLabel(current) : '暂无'}</small></span><LinkOutlined /><span className="is-info"><b>审批</b><em>{approvalLabel(current)}</em><small>双人复核</small></span></div><div className="taf-whitelist-feedback-table"><div><span>最近关联链路</span><span>对象</span><span>类型</span><span>风险</span><span>草案状态</span><span>审批</span></div>{linked.map((entry) => <button key={entry.id} type="button" onClick={() => onSelect(entry)}><span>{entry.source_alert_id}</span><span>{entry.value}</span><span>{whitelistTypeLabels[entry.type]}</span><span className={riskClass(riskLabel(entry.risk_level))}>{riskLabel(entry.risk_level)}</span><span>{statusLabel(entry)}</span><span className={entry.status === 'active' ? 'is-ok' : 'is-warn'}>{approvalLabel(entry)}</span></button>)}</div></div>;
}

function ImpactMatrix({ entries, onSelect }: { entries: WhitelistEntry[]; onSelect: (entry: WhitelistEntry) => void }) {
  const top = [...entries].sort((a, b) => (b.covered_alerts ?? 0) - (a.covered_alerts ?? 0)).slice(0, 5);
  const total = top.reduce((sum, entry) => sum + (entry.covered_alerts ?? 0), 0);
  return <div className="taf-whitelist-impact"><div className="taf-whitelist-impact-table"><div><span>覆盖对象 Top5</span><span>资产</span><span>来源告警</span><span>覆盖告警</span></div>{top.map((entry) => <button key={entry.id} type="button" onClick={() => onSelect(entry)}><span>{entry.value}</span><span>{entry.covered_assets ?? 0}</span><span>{entry.source_alert_id || '-'}</span><span>{entry.covered_alerts ?? 0}</span></button>)}</div><div className="taf-whitelist-donut"><i><b>总计</b><strong>{total}</strong></i><span><em className="is-risk" />高风险</span><span><em className="is-info" />中风险</span><span><em className="is-ok" />低风险</span></div></div>;
}

const renderWhitelistCell = (column: string, value: unknown, entry: WhitelistEntry, open: () => void) => {
  if (column === '对象类型') return <span className="taf-whitelist-type"><SafetyCertificateOutlined />{String(value)}</span>;
  if (column === '匹配条件') return <span className="taf-whitelist-match" title={String(value)}><LockOutlined />{String(value)}</span>;
  if (column === '来源告警') return <span className="taf-whitelist-source" title={String(value)}><BellOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return <span className="taf-whitelist-row-actions"><Button type="text" size="small" icon={<EyeOutlined />} aria-label="查看白名单详情" onClick={(event) => { event.stopPropagation(); open(); }} /><Button type="text" size="small" icon={<EditOutlined />} aria-label="编辑白名单" disabled={entry.status !== 'draft'} onClick={(event) => { event.stopPropagation(); open(); }} /><Button type="text" size="small" icon={<ClockCircleOutlined />} aria-label="延期白名单" disabled={entry.status === 'disabled'} onClick={(event) => { event.stopPropagation(); open(); }} /></span>;
  return <span className="taf-whitelist-table-value" title={String(value ?? '-')}>{String(value ?? '-')}</span>;
};

const toTableRow = (entry: WhitelistEntry): WhitelistTableRow => ({ key: entry.id, entry, 对象类型: whitelistTypeLabels[entry.type] ?? entry.type, 匹配条件: entry.value, 生效范围: entry.scope || '-', 有效期: periodLabel(entry), 责任角色: entry.owner_role || '未归属', 来源告警: entry.source_alert_id || '-', 状态: statusLabel(entry), 操作: '查看 / 编辑 / 延期' });

const buildMetrics = (route: NavRoute, entries: WhitelistEntry[]): PageSnapshot['metrics'] => {
  const active = entries.filter((entry) => entry.status === 'active' && !isExpired(entry)).length;
  const pending = entries.filter((entry) => entry.status === 'pending').length;
  const soon = entries.filter(isExpiringSoon).length;
  const long = entries.filter(isLongLived).length;
  const hits = entries.reduce((sum, entry) => sum + (entry.covered_alerts ?? 0), 0);
  const risks = entries.filter((entry) => ['high', 'critical'].includes(entry.risk_level ?? '')).length;
  const values = [`${active}`, `${pending}`, `${soon}`, `${long}`, `${hits}`, `${risks}`];
  return route.page.kpis.map((label, index) => ({ label, value: values[index] ?? '0', delta: index === 0 ? '真实 API' : index === 4 ? '近 7 天' : '实时', status: (index === 5 && risks ? 'risk' : index > 0 && Number(values[index]) ? 'warn' : 'ok') as PageSnapshot['metrics'][number]['status'] }));
};

const expiryEntries = (entries: WhitelistEntry[], tab: string) => entries.filter((entry) => tab.startsWith('即将') ? isExpiringSoon(entry) : tab.startsWith('过期') ? isExpired(entry) && entry.status !== 'disabled' : tab.startsWith('长期') ? isLongLived(entry) && entry.status === 'active' : !entry.owner_role).slice(0, 4);
const expiryCells = (entry: WhitelistEntry | undefined, tab: string): string[] => {
  if (!entry) return tab.startsWith('过期') ? ['对象类型', '匹配条件', '生效范围', '到期时间', '超期', '责任角色', '风险/SLA', '操作'] : tab.startsWith('长期') ? ['对象类型', '匹配条件', '已生效', '复审周期', '业务依据', '漏报风险', '复审状态', '操作'] : tab.startsWith('未归属') ? ['对象类型', '匹配条件', '来源', '原责任角色', '失效原因', '风险等级', '操作'] : ['对象类型', '匹配条件', '生效范围', '到期时间', '责任角色', '风险等级', '操作'];
  const type = whitelistTypeLabels[entry.type] ?? entry.type;
  if (tab.startsWith('过期')) return [type, entry.value, entry.scope || '-', formatDate(entry.expires_at), `超期 ${Math.max(1, Math.ceil((Date.now() - Date.parse(entry.expires_at || '')) / 86_400_000))} 天`, entry.owner_role || '(未归属)', `${riskLabel(entry.risk_level)} / SLA逾期`, '停用 / 补审'];
  if (tab.startsWith('长期')) return [type, entry.value, `${Math.max(181, Math.ceil((Date.now() - Date.parse(entry.created_at)) / 86_400_000))} 天`, '每 30 天', entry.reason, riskLabel(entry.risk_level), entry.approval_status === 'approved' ? '已复审' : '待复审', '复审 / 停用'];
  if (tab.startsWith('未归属')) return [type, entry.value, entry.source_alert_id || '-', entry.owner_role || '(未设置)', '组织变更/角色离任', riskLabel(entry.risk_level), '指派 / 停用'];
  return [type, entry.value, entry.scope || '-', formatDate(entry.expires_at), entry.owner_role || '(未归属)', riskLabel(entry.risk_level), '延期 / 停用'];
};

const statusLabel = (entry: WhitelistEntry) => entry.status === 'draft' ? '草稿' : entry.status === 'pending' ? '待审批' : entry.status === 'disabled' ? '停用' : isExpired(entry) ? '过期' : isExpiringSoon(entry) ? '即将到期' : '生效';
const approvalLabel = (entry?: WhitelistEntry) => entry?.approval_status === 'approved' ? '已批准' : entry?.approval_status === 'rejected' ? '已驳回' : entry?.approval_status === 'pending' ? '待审批' : '草案';
const isExpired = (entry: WhitelistEntry) => Boolean(entry.expires_at && Date.parse(entry.expires_at) < Date.now());
const isExpiringSoon = (entry: WhitelistEntry) => { const diff = entry.expires_at ? (Date.parse(entry.expires_at) - Date.now()) / 86_400_000 : Number.POSITIVE_INFINITY; return entry.status !== 'disabled' && diff >= 0 && diff <= 7; };
const isLongLived = (entry: WhitelistEntry) => !entry.expires_at || (Date.parse(entry.expires_at) - Date.parse(entry.created_at)) / 86_400_000 > 180;
const periodLabel = (entry: WhitelistEntry) => `${formatDate(entry.created_at)} ~ ${formatDate(entry.expires_at)}`;
const formatDate = (value?: string) => value && Number.isFinite(Date.parse(value)) ? new Date(value).toISOString().slice(0, 10) : '长期';
const riskLabel = (risk?: WhitelistRisk) => risk === 'critical' ? '严重' : risk === 'high' ? '高' : risk === 'low' ? '低' : '中';
const riskClass = (value: string) => value.includes('高') || value.includes('严重') ? 'is-risk' : value.includes('中') ? 'is-warn' : 'is-ok';
const objectLabel = (tab: BuilderTab) => tab === 'IP' ? 'IP / CIDR' : tab === '资产' ? '资产对象' : tab === '规则' ? '规则 ID' : tab === '模型' ? '模型版本' : tab;
const matchLabel = (tab: BuilderTab) => tab === '账号' ? '账号类型' : tab === '规则' ? '规则类型' : tab === '模型' ? '置信度阈值' : '匹配方式';
const actionLabel = (action: WhitelistTransition) => ({ submit: '提交审批', approve: '审批通过', reject: '审批驳回', extend: '延期', disable: '停用', assign: '责任角色指派' }[action]);
const errorText = (error: unknown) => error instanceof Error ? error.message : '未知错误';
const trendValues = (total: number) => { const base = Math.max(1, Math.round(total / 7)); return [0.72, 0.84, 0.78, 1.05, 0.94, 1.12, 1].map((ratio) => Math.round(base * ratio)); };
const coverageValues = (entries: WhitelistEntry[]) => ['domain', 'account', 'ip', 'rule', 'model'].map((type) => entries.filter((entry) => entry.type === type || (type === 'ip' && entry.type === 'subnet')).reduce((sum, entry) => sum + (entry.covered_alerts ?? 0), 0));
const builderTabFromPageId = (pageId: string): BuilderTab => pageId.endsWith('-ip') ? 'IP' : pageId.endsWith('-asset') ? '资产' : pageId.endsWith('-account') ? '账号' : pageId.endsWith('-rule') ? '规则' : pageId.endsWith('-model') ? '模型' : '域名';
const expiryTabFromPageId = (pageId: string) => pageId.endsWith('expired-unhandled') ? '过期未处理' : pageId.endsWith('long-lived') ? '长期生效（>180天）' : pageId.endsWith('unassigned-owner') ? '未归属责任角色' : '即将到期（7天内）';
