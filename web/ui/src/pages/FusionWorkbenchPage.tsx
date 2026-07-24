import {
  AimOutlined,
  AppstoreOutlined,
  BankOutlined,
  BellOutlined,
  CheckCircleOutlined,
  ClusterOutlined,
  CopyOutlined,
  DatabaseOutlined,
  EditOutlined,
  FileDoneOutlined,
  FileTextOutlined,
  GlobalOutlined,
  HistoryOutlined,
  LinkOutlined,
  LineChartOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Input, InputNumber, Modal, Pagination, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { FusionPipelineConnectionsChart, FusionRuleMiniChart, FusionSourceStatusChart, type FusionPipelineGeometry } from '@/components/charts';
import { hasRequiredScope } from '@/routes/access';
import type { NavRoute } from '@/routes/routeManifest';
import { localBypassUser, type CurrentUser } from '@/services/api';
import {
  exportFusionEvidencePackage,
  fetchFusionWorkbench,
  resolveFusionConflict,
  updateFusionRule,
  type FusionAuditEvent,
  type FusionConflict,
  type FusionRuleOverride,
  type FusionSource,
  type FusionWorkbench,
} from '@/services/fusionApi';

type FusionConflictAction = 'authoritative-source' | 'manual-repair-task' | 'accept-primary';
type Tone = 'ok' | 'warn' | 'risk';

const sourcePresentation: Record<string, { prefix: string; icon: ReactNode }> = {
  traffic: { prefix: 'Flow', icon: <LineChartOutlined /> },
  asset: { prefix: 'Asset', icon: <AppstoreOutlined /> },
  log: { prefix: 'Device Log', icon: <FileTextOutlined /> },
  behavior: { prefix: 'User Event', icon: <UserOutlined /> },
  threat_intel: { prefix: 'Threat Intel', icon: <AimOutlined /> },
  vulnerability: { prefix: 'Vuln', icon: <SafetyCertificateOutlined /> },
};

const ruleIcons = [<LinkOutlined />, <UserOutlined />, <BankOutlined />, <GlobalOutlined />, <BellOutlined />, <SafetyCertificateOutlined />];

const outputIcons: Record<string, ReactNode> = {
  host: <DatabaseOutlined />,
  account: <UserOutlined />,
  asset: <AppstoreOutlined />,
  domain: <GlobalOutlined />,
  service: <ClusterOutlined />,
  alert: <BellOutlined />,
};

const ruleOrder = ['IP_MAC_BIND_V3', 'ACCOUNT_HOST_LINK', 'ASSET_DEPT_COMPLETION', 'DOMAIN_IP_RESOLUTION', 'ALERT_ASSET_JOIN', 'VULN_SERVICE_MATCH'];

const entityLabels: Array<[string, string]> = [
  ['host', '主机实体'], ['account', '账号实体'], ['asset', '资产实体'], ['domain', '域名实体'], ['service', '服务实体'], ['alert', '告警实体'],
];

async function writeSafeClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // HTTP deployments may expose the Clipboard API while denying the write.
    }
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  textarea.addEventListener('copy', (event) => {
    event.clipboardData?.setData('text/plain', value);
    event.preventDefault();
  }, { once: true });
  const copied = document.execCommand('copy');
  textarea.remove();
  if (!copied) throw new Error('clipboard unavailable');
}

export function FusionWorkbenchPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const currentUser = queryClient.getQueryData<CurrentUser>(['current-user']) ?? localBypassUser;
  const canWriteFusion = hasRequiredScope(currentUser, ['rule:write', 'rule:*']);
  const [selectedConflictId, setSelectedConflictId] = useState<string>();
  const [detailOpen, setDetailOpen] = useState(true);
  const [actionResult, setActionResult] = useState<string>();
  const [resolutionStrategy, setResolutionStrategy] = useState<FusionConflictAction>('authoritative-source');
  const [selectedSource, setSelectedSource] = useState<string>();
  const [resolutionNote, setResolutionNote] = useState('按权威来源确认主值，保留原始来源与审计链。');
  const [ruleModalOpen, setRuleModalOpen] = useState(false);
  const [editingRuleId, setEditingRuleId] = useState<string>();
  const [ruleStatus, setRuleStatus] = useState('active');
  const [ruleStrategy, setRuleStrategy] = useState('authoritative-source');
  const [ruleThreshold, setRuleThreshold] = useState(0.85);
  const [ruleNote, setRuleNote] = useState('');
  const [conflictPage, setConflictPage] = useState(1);
  const [auditPage, setAuditPage] = useState(1);
  const [rulePage, setRulePage] = useState(1);
  const [retainedConflict, setRetainedConflict] = useState<FusionConflict>();

  const query = useQuery({
    queryKey: ['fusion-workbench', rulePage, conflictPage, auditPage],
    queryFn: () => fetchFusionWorkbench({ rulePage, rulePageSize: 6, conflictPage, conflictPageSize: 3, auditPage, auditPageSize: 5 }),
    placeholderData: (previous) => previous,
  });
  const data = query.data;
  const conflicts = useMemo(() => data?.conflicts ?? [], [data?.conflicts]);
  const pendingConflicts = useMemo(() => conflicts.filter((item) => item.status !== 'resolved'), [conflicts]);
  const pendingRiskCounts = data?.pending_risk_counts ?? { high: 0, medium: 0, low: 0 };
  const threatIntelEvidence = useMemo(() => data?.threat_intel_entries ?? [], [data?.threat_intel_entries]);
  const hasAcceptanceFixture = useMemo(
    () => (data?.conflicts ?? []).some((item) => item.origin === 'acceptance_fixture')
      || [...(data?.rules ?? []), ...(data?.pipeline_rules ?? [])].some((item) => item.detail?.fixture === 'fusion-workbench-v1'),
    [data?.conflicts, data?.pipeline_rules, data?.rules],
  );
  const selectedConflict = useMemo(
    () => pendingConflicts.find((item) => item.conflict_id === selectedConflictId)
      ?? conflicts.find((item) => item.conflict_id === selectedConflictId)
      ?? (retainedConflict?.conflict_id === selectedConflictId ? retainedConflict : undefined)
      ?? pendingConflicts[0]
      ?? conflicts[0],
    [conflicts, pendingConflicts, retainedConflict, selectedConflictId],
  );
  const selectedRule = useMemo(
    () => [...(data?.rules ?? []), ...(data?.pipeline_rules ?? [])]
      .find((item) => item.rule_id === (editingRuleId ?? selectedConflict?.rule_id))
      ?? data?.rules[0]
      ?? data?.pipeline_rules?.[0],
    [data?.pipeline_rules, data?.rules, editingRuleId, selectedConflict?.rule_id],
  );

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil((data?.rule_total ?? 0) / 6));
    if (rulePage > maxPage) setRulePage(maxPage);
  }, [data?.rule_total, rulePage]);

  useEffect(() => {
    setSelectedSource(selectedConflict?.source_values[0]?.source);
  }, [selectedConflict?.conflict_id, selectedConflict?.source_values]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil((data?.pending_count ?? 0) / 3));
    if (conflictPage > maxPage) setConflictPage(maxPage);
  }, [conflictPage, data?.pending_count]);

  useEffect(() => {
    const maxPage = Math.max(1, Math.ceil((data?.audit_total ?? 0) / 5));
    if (auditPage > maxPage) setAuditPage(maxPage);
  }, [auditPage, data?.audit_total]);

  const conflictMutation = useMutation({
    mutationFn: (strategy: FusionConflictAction) => {
      if (!canWriteFusion) throw new Error('当前账号缺少 rule:write 权限');
      if (!selectedConflict) throw new Error('当前没有可处理的融合冲突');
      const sourceValue = selectedConflict.source_values.find((item) => item.source === selectedSource) ?? selectedConflict.source_values[0];
      if (!sourceValue) throw new Error('冲突来源值缺失');
      return resolveFusionConflict(selectedConflict.conflict_id, {
        object_id: selectedConflict.object_id,
        object_type: selectedConflict.object_type,
        field_name: selectedConflict.field_name,
        selected_source: sourceValue.source,
        selected_value: sourceValue.value,
        strategy,
        note: resolutionNote,
        rule_id: selectedConflict.rule_id,
        expected_state_version: selectedConflict.state_version,
        detail: { source_values: selectedConflict.source_values, previous_status: selectedConflict.status },
      });
    },
    onSuccess: async (result, strategy) => {
      setSelectedConflictId(result.resolution.conflict_id);
      if (selectedConflict?.conflict_id === result.resolution.conflict_id) {
        setRetainedConflict({
          ...selectedConflict,
          status: strategy === 'manual-repair-task' ? 'repair_pending' : 'resolved',
          state_version: result.resolution.state_version,
          updated_at: result.resolution.resolved_at,
        });
      }
      setActionResult(`冲突 ${result.resolution.conflict_id} 已写入，状态版本 ${result.resolution.state_version}`);
      message.success('融合冲突处理与审计已写入');
      await query.refetch();
    },
    onError: (error) => message.error(errorText(error)),
  });

  const ruleMutation = useMutation({
    mutationFn: () => {
      if (!canWriteFusion) throw new Error('当前账号缺少 rule:write 权限');
      if (!selectedRule) throw new Error('当前没有可编辑的融合规则');
      return updateFusionRule(selectedRule.rule_id, {
        status: ruleStatus,
        strategy: ruleStrategy,
        confidence_threshold: ruleThreshold,
        note: ruleNote,
        expected_version: selectedRule.version,
      });
    },
    onSuccess: async (result) => {
      setActionResult(`规则 ${result.rule.rule_id} 已更新到版本 ${result.rule.version}`);
      setRuleModalOpen(false);
      message.success('融合规则版本与审计已写入');
      await query.refetch();
    },
    onError: (error) => message.error(errorText(error)),
  });

  const evidenceMutation = useMutation({
    mutationFn: () => {
      if (!canWriteFusion) throw new Error('当前账号缺少 rule:write 权限');
      if (!selectedConflict) throw new Error('当前没有可导出的冲突证据');
      return exportFusionEvidencePackage(selectedConflict.conflict_id);
    },
    onSuccess: async (result) => {
      downloadBase64(result.filename, result.content_base64, 'application/json');
      setActionResult(`证据包 ${result.filename} 已生成，摘要 ${result.sha256.slice(0, 24)}…`);
      message.success('融合证据包已下载并写入审计');
      await query.refetch();
    },
    onError: (error) => message.error(errorText(error)),
  });

  const openRuleEditor = (rule?: FusionRuleOverride) => {
    if (!canWriteFusion) return;
    const target = rule ?? selectedRule;
    if (!target) return;
    setEditingRuleId(target.rule_id);
    setRuleStatus(target.status);
    setRuleStrategy(target.strategy);
    setRuleThreshold(target.confidence_threshold);
    setRuleNote(target.note ?? '');
    setRuleModalOpen(true);
  };

  const restoreServerState = () => {
    setActionResult(undefined);
    setSelectedConflictId(undefined);
    setRetainedConflict(undefined);
    setResolutionStrategy('authoritative-source');
    setResolutionNote('按权威来源确认主值，保留原始来源与审计链。');
    setSelectedSource(selectedConflict?.source_values[0]?.source);
    setEditingRuleId(undefined);
    setRuleModalOpen(false);
    setConflictPage(1);
    setAuditPage(1);
    setRulePage(1);
    void query.refetch();
  };

  const columns: ColumnsType<FusionRuleOverride> = [
    { title: '规则 ID', dataIndex: 'rule_id', key: 'rule_id', ellipsis: true, render: (value) => <Tooltip title={String(value)}><span className="taf-fusion-object">{value}</span></Tooltip> },
    { title: '映射关系', key: 'field', ellipsis: true, render: (_, item) => detailText(item, 'field') },
    { title: '输入源', key: 'sources', ellipsis: true, render: (_, item) => `${detailText(item, 'source_a')}、${detailText(item, 'source_b')}` },
    { title: '输出实体', key: 'output', ellipsis: true, render: (_, item) => ruleOutputLabel(item.rule_id) },
    { title: '置信度阈值', dataIndex: 'confidence_threshold', key: 'confidence_threshold', width: 96, render: (value) => Number(value).toFixed(2) },
    { title: '冲突策略', dataIndex: 'strategy', key: 'strategy', ellipsis: true, render: (value) => strategyLabel(String(value)) },
    { title: '更新时间', dataIndex: 'updated_at', key: 'updated_at', width: 126, render: (value) => dateTimeLabel(Number(value)) },
    { title: '状态', dataIndex: 'status', key: 'status', width: 76, render: (value) => <StatusTag value={value === 'active' ? '已启用' : value} /> },
    { title: '版本', dataIndex: 'version', key: 'version', width: 62, render: (value) => `v${value}` },
    { title: '操作', key: 'action', width: 112, render: (_, item) => <Space className="taf-fusion-rule-actions" size={2} onClick={(event) => event.stopPropagation()}><Tooltip title="编辑规则"><Button aria-label="编辑规则" type="text" size="small" icon={<EditOutlined />} disabled={!canWriteFusion} onClick={() => openRuleEditor(item)} /></Tooltip><Tooltip title="复制规则 ID"><Button aria-label="复制规则 ID" type="text" size="small" icon={<CopyOutlined />} onClick={() => void writeSafeClipboard(item.rule_id).then(() => message.success('规则 ID 已复制')).catch(() => message.error('规则 ID 复制失败'))} /></Tooltip><Tooltip title="查看规则审计"><Button aria-label="查看规则审计" type="text" size="small" icon={<HistoryOutlined />} onClick={() => navigate(`/audit-log?object_type=fusion_rule&object_id=${encodeURIComponent(item.rule_id)}`)} /></Tooltip><Tooltip title="刷新真实状态"><Button aria-label="刷新真实状态" type="text" size="small" icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()} /></Tooltip></Space> },
  ];

  return (
    <div className="taf-page taf-fusion-workbench">
      <div className={`taf-fusion-grid${detailOpen ? '' : ' is-detail-closed'}`}>
        <main className="taf-fusion-main">
          <header className="taf-fusion-titlebar">
            <div className="taf-fusion-title-ident"><h1>{route.page.title}</h1>{hasAcceptanceFixture && <span className="taf-fusion-fixture-badge">验收数据</span>}</div>
            <Space>
          <Button size="small" onClick={restoreServerState}>重置页面状态</Button>
              <Tooltip title="刷新真实融合状态"><Button size="small" icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => void query.refetch()} /></Tooltip>
              <Button size="small" icon={<EditOutlined />} disabled={!canWriteFusion} title={canWriteFusion ? '编辑融合规则' : '缺少 rule:write 权限'} onClick={() => openRuleEditor()}>规则编辑</Button>
            </Space>
          </header>
          {query.isError && <Alert type="error" showIcon message="真实 Fusion 工作台加载失败" description={errorText(query.error)} action={<Button size="small" danger onClick={() => void query.refetch()}>重试</Button>} />}
          <WorkPanel title="数据源状态"><SourceStatusGrid sources={data?.sources ?? []} threatIntelEvidence={threatIntelEvidence} /></WorkPanel>
          <WorkPanel title="多源融合编排（映射与对齐流程）" extra={<Space size={6}><Input size="small" value="主园区 / 全来源" readOnly /><Button size="small" icon={<EditOutlined />} disabled={!canWriteFusion} onClick={() => openRuleEditor()}>编辑规则</Button><Button size="small" loading={conflictMutation.isPending} disabled={!canWriteFusion || !selectedConflict || selectedConflict.status === 'repair_pending'} onClick={() => conflictMutation.mutate('manual-repair-task')}>生成修复任务</Button></Space>}>
            <FusionPipeline data={data} />
          </WorkPanel>
          {actionResult && <Alert type="success" showIcon closable onClose={() => setActionResult(undefined)} message={actionResult} />}
          <WorkPanel className="taf-fusion-rules-panel" title={`融合规则管理（共 ${data?.rule_total ?? 0} 条）`} extra={<Pagination className="taf-fusion-rule-pagination" current={rulePage} pageSize={6} total={data?.rule_total ?? 0} showSizeChanger={false} showLessItems size="small" onChange={setRulePage} />}>
            <Table rowKey="rule_id" size="small" loading={query.isLoading} columns={columns} dataSource={data?.rules ?? []} pagination={false} onRow={(record) => ({ onClick: canWriteFusion ? () => openRuleEditor(record) : undefined })} />
          </WorkPanel>
        </main>

        {detailOpen && <aside className="taf-fusion-detail"><ConflictDetail conflict={selectedConflict} events={data?.audit_events ?? []} selectedSource={selectedSource} strategy={resolutionStrategy} note={resolutionNote} canWrite={canWriteFusion} resolving={conflictMutation.isPending} exporting={evidenceMutation.isPending} onClose={() => setDetailOpen(false)} onSourceChange={setSelectedSource} onStrategyChange={setResolutionStrategy} onNoteChange={setResolutionNote} onResolve={() => conflictMutation.mutate(resolutionStrategy)} onCreateRepair={() => conflictMutation.mutate('manual-repair-task')} onAudit={() => navigate(`/audit-log?object_type=fusion_conflict&object_id=${encodeURIComponent(selectedConflict?.conflict_id ?? '')}`)} onExport={() => evidenceMutation.mutate()} /></aside>}
      </div>

      <div className="taf-fusion-bottom">
        <WorkPanel title={`冲突队列（待处理 ${data?.pending_count ?? 0} 条）`} extra={<span className="taf-fusion-risk-counts"><b className="is-high">高 {pendingRiskCounts.high}</b><b className="is-medium">中 {pendingRiskCounts.medium}</b><b className="is-low">低 {pendingRiskCounts.low}</b></span>}><ConflictQueue conflicts={pendingConflicts} page={conflictPage} total={data?.pending_count ?? 0} selectedId={selectedConflict?.conflict_id} onPageChange={setConflictPage} onSelect={(item) => { setRetainedConflict(undefined); setSelectedConflictId(item.conflict_id); setDetailOpen(true); }} /></WorkPanel>
        <WorkPanel title={`融合事件审计（近 ${Math.min(data?.audit_total ?? 0, 50)} 条）`}><FusionAuditTrail events={data?.audit_events ?? []} page={auditPage} total={data?.audit_total ?? 0} onPageChange={setAuditPage} onOpen={(item) => navigate(`/audit-log?object_type=${encodeURIComponent(item.resource_type)}&object_id=${encodeURIComponent(item.resource_id)}`)} /></WorkPanel>
        <WorkPanel title="融合质量看板（实时）"><QualityBoard data={data} /></WorkPanel>
      </div>

      <Modal
        className="taf-overlay-modal taf-fusion-rule-modal"
        title={selectedRule ? <div className="taf-fusion-rule-modal-title"><div><strong>融合规则编辑</strong><span>规则 ID {selectedRule.rule_id}　/　{selectedRule.rule_name}　/　v{selectedRule.version}</span></div><em>{ruleStatus === 'draft' ? '草稿' : ruleStatus === 'disabled' ? '已停用' : '已启用'}</em><b>影响 {formatNumber(detailNumber(selectedRule, 'matches'))} 条匹配记录</b></div> : '融合规则编辑'}
        open={ruleModalOpen}
        width="min(1200px, calc(var(--taf-window-inner-width, 100dvw) - 220px))"
        onCancel={() => setRuleModalOpen(false)}
        footer={<div className="taf-fusion-rule-modal-footer"><span><WarningOutlined /> 保存后将触发融合结果增量重算，并写入完整审计链。</span><Space><Button onClick={() => setRuleModalOpen(false)}>取消</Button><Button type="primary" loading={ruleMutation.isPending} disabled={!canWriteFusion} onClick={() => ruleMutation.mutate()}>保存新版本</Button></Space></div>}
      >
        {selectedRule && <div className="taf-fusion-rule-modal-layout">
          <section className="taf-fusion-rule-config">
            <h3>规则配置</h3>
            <div className="taf-fusion-rule-form">
              <label className="is-full"><span>规则名称</span><Input value={selectedRule.rule_name} readOnly /></label>
              <label><span>匹配对象</span><Input value={`${detailText(selectedRule, 'field')}　→　${ruleOutputLabel(selectedRule.rule_id)}`} readOnly /></label>
              <label><span>运行状态</span><Select value={ruleStatus} onChange={setRuleStatus} options={[{ value: 'active', label: '已启用' }, { value: 'draft', label: '草稿' }, { value: 'disabled', label: '已停用' }]} /></label>
              <label className="is-full"><span>主键与来源优先级</span><Input value={`${detailText(selectedRule, 'source_a')} → ${detailText(selectedRule, 'source_b')}；${strategyLabel(ruleStrategy)}`} readOnly /></label>
              <label><span>冲突字段处理策略</span><Select value={ruleStrategy} onChange={setRuleStrategy} options={[{ value: 'authoritative-source', label: '权威来源优先' }, { value: 'weighted-confidence', label: '置信度加权' }, { value: 'latest-observation', label: '最新观测优先' }, { value: 'manual-review', label: '人工复核' }]} /></label>
              <label><span>置信度阈值</span><InputNumber value={ruleThreshold} min={0} max={1} step={0.01} onChange={(value) => setRuleThreshold(value ?? 0.85)} /></label>
              <div className="taf-fusion-rule-trust is-full"><span>数据源可信度</span><div><b><i className="is-ok" />{detailText(selectedRule, 'source_a')}　{Math.min(0.99, ruleThreshold + 0.09).toFixed(2)}</b><b><i className="is-info" />{detailText(selectedRule, 'source_b')}　{ruleThreshold.toFixed(2)}</b><b><i className="is-warn" />其他来源　{Math.max(0, ruleThreshold - 0.16).toFixed(2)}</b></div></div>
              <label className="is-full"><span>变更说明</span><Input.TextArea rows={4} value={ruleNote} onChange={(event) => setRuleNote(event.target.value)} maxLength={240} showCount /></label>
            </div>
          </section>
          <aside className="taf-fusion-rule-impact">
            <section><h3>1. 权限与门禁</h3><p><SafetyCertificateOutlined /> {canWriteFusion ? `已验证 ${currentUser.username} 的 rule:write 权限` : '缺少 rule:write 权限，仅允许查看'}</p></section>
            <section><h3>2. 影响范围（实时）</h3><dl><dt>当前匹配记录</dt><dd>{formatNumber(detailNumber(selectedRule, 'matches'))}</dd><dt>待处理冲突</dt><dd>{formatNumber(data?.pending_count ?? 0)}</dd><dt>融合规则</dt><dd>{formatNumber(data?.rule_total ?? 0)}</dd><dt>审计记录</dt><dd>{formatNumber(data?.audit_total ?? 0)}</dd></dl></section>
            <section><h3>3. 状态解释</h3><ul><li><CheckCircleOutlined /> 当前成功率 <b>{percent(detailNumber(selectedRule, 'success_rate'))}</b></li><li><WarningOutlined /> 阈值调整为 <b>{ruleThreshold.toFixed(2)}</b></li><li><HistoryOutlined /> 保存后版本 <b>v{selectedRule.version + 1}</b></li></ul></section>
            <section><h3>4. 下一步动作</h3><div className="taf-fusion-rule-steps"><b>1　保存新版本</b><span>→</span><b>2　冲突预检</b><span>→</span><b>3　审计留痕</b></div></section>
            <section><h3>5. 审计留痕</h3><dl><dt>操作者</dt><dd>{currentUser.username}</dd><dt>变更摘要</dt><dd title={ruleNote}>{ruleNote || '未填写'}</dd><dt>并发校验</dt><dd>expected_version = {selectedRule.version}</dd><dt>审计动作</dt><dd>FUSION_RULE_UPDATED</dd></dl></section>
          </aside>
        </div>}
      </Modal>
    </div>
  );
}

function SourceStatusGrid({ sources, threatIntelEvidence }: { sources: FusionSource[]; threatIntelEvidence: FusionWorkbench['threat_intel_entries'] }) {
  return <div className="taf-fusion-sources">{sources.map((source) => {
    const presentation = sourcePresentation[source.source_id] ?? { prefix: source.source_id, icon: <DatabaseOutlined /> };
    const tone = sourceTone(source);
    const anomalyCount = source.source_id === 'threat_intel'
      ? threatIntelEvidence.filter((item) => item.reputation === 'malicious' || item.reputation === 'suspicious').length
      : tone === 'ok' ? 0 : 1;
    const stateLabel = source.status === 'unavailable' ? '不可用' : tone === 'ok' ? '正常' : tone === 'risk' ? '延迟' : '待同步';
    return <article key={source.source_id} className={`taf-fusion-source is-${tone} is-source-${source.source_id}`}><span className="taf-fusion-source-icon">{presentation.icon}</span><strong title={`${presentation.prefix} ${source.name}`}>{presentation.prefix} {source.name}</strong><em className="taf-fusion-source-state"><CheckCircleOutlined />{stateLabel}</em><dl><dt>接入批次</dt><dd>{batchLabel(source.last_ingest_at)}</dd><dt>更新延迟</dt><dd>{latencyLabel(source.last_ingest_at)}</dd><dt>字段覆盖</dt><dd className="taf-fusion-source-coverage"><span>{sourceCompleteness(source)}</span><FusionSourceStatusChart ariaLabel={`${presentation.prefix} 最近80分钟接入趋势`} trend={source.recent_trend ?? []} tone={tone} /></dd><dt>最新同步</dt><dd>{timeLabel(source.last_ingest_at)}</dd><dt>异常源数</dt><dd>{anomalyCount}</dd></dl></article>;
  })}</div>;
}

function FusionPipeline({ data }: { data?: FusionWorkbench }) {
  const stageRef = useRef<HTMLDivElement>(null);
  const sources = data?.sources ?? [];
  const ruleRank = (ruleId: string) => {
    const index = ruleOrder.indexOf(ruleId);
    return index < 0 ? ruleOrder.length + 1 : index;
  };
  const rules = [...(data?.pipeline_rules ?? data?.rules ?? [])].sort((left, right) => ruleRank(left.rule_id) - ruleRank(right.rule_id) || left.rule_id.localeCompare(right.rule_id));
  const geometry = useFusionPipelineGeometry(stageRef, sources.length, Math.min(rules.length, 6), entityLabels.length);
  return <div className="taf-fusion-pipeline"><div className="taf-fusion-pipeline-labels"><strong>数据源（输入）</strong><strong>融合任务（规则驱动与对齐）</strong><strong>统一实体（输出）</strong></div><div className="taf-fusion-pipeline-stage" ref={stageRef}><FusionPipelineConnectionsChart geometry={geometry} /><div className="taf-fusion-source-stack">{sources.map((source) => { const p = sourcePresentation[source.source_id]; const label = p ? `${p.prefix} ${source.name}` : source.name; return <span data-fusion-source key={source.source_id} className={`is-source-${source.source_id}`} title={label}><i>{p?.icon ?? <DatabaseOutlined />}</i><b>{label}</b></span>; })}</div><div className="taf-fusion-rule-flow">{rules.slice(0, 6).map((rule, index) => { const successRate = detailNumber(rule, 'success_rate'); const count = detailNumber(rule, 'rule_count'); const recentHits = detailNumberArray(rule, 'recent_hits'); return <article data-fusion-rule key={rule.rule_id} className={`taf-fusion-rule-node is-rule-${index + 1}`}><header>{ruleIcons[index]}<strong>{rule.rule_name}</strong></header><dl><dt>规则</dt><dd>{formatNumber(count)} 条</dd><dt>成功率</dt><dd>{percent(successRate)}</dd><dt>置信度</dt><dd>{detailText(rule, 'confidence_label')}</dd></dl><FusionRuleMiniChart ariaLabel={`${rule.rule_name} 最近六个周期命中趋势`} values={recentHits.length ? recentHits : [detailNumber(rule, 'matches')]} /></article>; })}</div><div className="taf-fusion-output-stack">{entityLabels.map(([key, label]) => <span data-fusion-output key={key} className={`is-output-${key}`}><i>{outputIcons[key] ?? <CheckCircleOutlined />}</i><b>{label}</b><em>{formatNumber(data?.entity_counts[key] ?? 0)}</em></span>)}</div></div></div>;
}

function useFusionPipelineGeometry(stageRef: React.RefObject<HTMLDivElement>, sourceCount: number, ruleCount: number, outputCount: number) {
  const [geometry, setGeometry] = useState<FusionPipelineGeometry>();

  useLayoutEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    let frame = 0;
    const measure = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const stageRect = stage.getBoundingClientRect();
        const sourceNodes = Array.from(stage.querySelectorAll<HTMLElement>('[data-fusion-source]'));
        const ruleNodes = Array.from(stage.querySelectorAll<HTMLElement>('[data-fusion-rule]'));
        const outputNodes = Array.from(stage.querySelectorAll<HTMLElement>('[data-fusion-output]'));
        if (!stageRect.width || !stageRect.height || !ruleNodes.length) return;
        const relativeRect = (node: HTMLElement) => {
          const rect = node.getBoundingClientRect();
          return { left: rect.left - stageRect.left, right: rect.right - stageRect.left, top: rect.top - stageRect.top, height: rect.height };
        };
        const sourceRects = sourceNodes.map(relativeRect);
        const ruleRects = ruleNodes.map(relativeRect);
        const outputRects = outputNodes.map(relativeRect);
        const firstRule = ruleRects[0];
        const lastRule = ruleRects[ruleRects.length - 1];
        const nextGeometry: FusionPipelineGeometry = {
          width: stageRect.width,
          height: stageRect.height,
          sourceLinks: sourceRects.map((rect, index) => ({
            start: [rect.right, rect.top + rect.height / 2],
            end: [firstRule.left, firstRule.top + firstRule.height * ((index + 0.5) / Math.max(1, sourceRects.length))],
          })),
          ruleLinks: ruleRects.slice(0, -1).map((rect, index) => {
            const next = ruleRects[index + 1];
            return { start: [rect.right, rect.top + rect.height / 2], end: [next.left, next.top + next.height / 2] };
          }),
          outputLinks: outputRects.map((rect, index) => ({
            start: [lastRule.right, lastRule.top + lastRule.height * ((index + 0.5) / Math.max(1, outputRects.length))],
            end: [rect.left, rect.top + rect.height / 2],
          })),
        };
        setGeometry((current) => geometryEqual(current, nextGeometry) ? current : nextGeometry);
      });
    };
    const observer = new ResizeObserver(measure);
    observer.observe(stage);
    stage.querySelectorAll<HTMLElement>('[data-fusion-source], [data-fusion-rule], [data-fusion-output]').forEach((node) => observer.observe(node));
    measure();
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, [stageRef, sourceCount, ruleCount, outputCount]);

  return geometry;
}

function geometryEqual(left: FusionPipelineGeometry | undefined, right: FusionPipelineGeometry) {
  if (!left || Math.abs(left.width - right.width) > 0.5 || Math.abs(left.height - right.height) > 0.5) return false;
  const flatten = (geometry: FusionPipelineGeometry) => [...geometry.sourceLinks, ...geometry.ruleLinks, ...geometry.outputLinks].flatMap((link) => [...link.start, ...link.end]);
  const leftValues = flatten(left);
  const rightValues = flatten(right);
  return leftValues.length === rightValues.length && leftValues.every((value, index) => Math.abs(value - rightValues[index]) <= 0.5);
}

function ConflictQueue({ conflicts, page, total, selectedId, onPageChange, onSelect }: { conflicts: FusionConflict[]; page: number; total: number; selectedId?: string; onPageChange: (page: number) => void; onSelect: (item: FusionConflict) => void }) {
  const pageSize = 3;
  return <div className="taf-fusion-conflicts"><div className="taf-fusion-conflict-head"><span>冲突ID</span><span>冲突字段</span><span>来源数</span><span>置信度</span><span>级别</span></div>{conflicts.length ? conflicts.map((item) => <button className={item.conflict_id === selectedId ? 'is-selected' : ''} key={item.conflict_id} type="button" onClick={() => onSelect(item)}><strong>{item.conflict_id}</strong><span>{item.field_name}</span><span>{item.source_count}</span><span>{item.confidence.toFixed(2)}</span><StatusTag value={severityLabel(item.severity)} /></button>) : <div className="taf-fusion-empty">暂无待处理冲突</div>}{total > pageSize && <Pagination className="taf-fusion-panel-pagination" size="small" current={page} pageSize={pageSize} total={total} showSizeChanger={false} showLessItems onChange={onPageChange} />}</div>;
}

function FusionAuditTrail({ events, page, total, onPageChange, onOpen }: { events: FusionAuditEvent[]; page: number; total: number; onPageChange: (page: number) => void; onOpen: (item: FusionAuditEvent) => void }) {
  const pageSize = 5;
  return <div className="taf-fusion-audit" role="table" aria-label="融合事件审计"><div className="taf-fusion-audit-head" role="row"><span>时间</span><span>事件类型</span><span>规则 ID</span><span>实体类型 → 变更后</span><span>操作者</span><span>结果</span></div>{events.length ? events.map((item) => <button key={item.log_id} type="button" role="row" onClick={() => onOpen(item)} title="打开完整审计记录"><span>{timeLabel(item.timestamp)}</span><strong>{auditLabel(item.action)}</strong><em title={auditRuleId(item)}>{auditRuleId(item)}</em><span title={auditEntityChange(item)}>{auditEntityChange(item)}</span><span title={auditOperator(item)}>{auditOperator(item)}</span><b className={item.result === 'failure' ? 'is-failure' : ''}>{item.result === 'failure' ? '失败' : '成功'}</b></button>) : <div className="taf-fusion-empty">暂无真实审计记录</div>}{total > pageSize && <Pagination className="taf-fusion-panel-pagination" size="small" current={page} pageSize={pageSize} total={total} showSizeChanger={false} showLessItems onChange={onPageChange} />}</div>;
}

function QualityBoard({ data }: { data?: FusionWorkbench }) {
  const stats = data?.stats;
  const rules = data?.pipeline_rules ?? data?.rules ?? [];
  const activeRules = rules.filter((item) => item.status === 'active').length;
  const averageConfidence = rules.length ? rules.reduce((sum, item) => sum + item.confidence_threshold, 0) / rules.length : 0;
  const quality = stats?.quality_metrics;
  const items: Array<[string, string, string]> = [
    ['实体总数', formatNumber(stats?.entities_aligned ?? 0), '实时'], ['待处理冲突', formatNumber(data?.pending_count ?? 0), '队列'], ['已处理冲突', formatNumber(data?.resolved_count ?? 0), '累计'], ['核心编排规则', `${activeRules}/${rules.length}`, '版本化'],
    ['自动融合成功率', percent(quality?.accuracy ?? 0), '实时'], ['平均置信度', averageConfidence.toFixed(2), '规则'], ['覆盖率', percent(quality?.completeness ?? 0), '实时'], ['重复率', percent(quality?.duplication_rate ?? 0), '实时'],
  ];
  return <div className="taf-fusion-quality">{items.map(([label, value, hint]) => <span key={label}><strong>{value}</strong><small>{label}</small><em>{hint}</em></span>)}</div>;
}

function ConflictDetail({ conflict, events, selectedSource, strategy, note, canWrite, resolving, exporting, onClose, onSourceChange, onStrategyChange, onNoteChange, onResolve, onCreateRepair, onAudit, onExport }: { conflict?: FusionConflict; events: FusionAuditEvent[]; selectedSource?: string; strategy: FusionConflictAction; note: string; canWrite: boolean; resolving: boolean; exporting: boolean; onClose: () => void; onSourceChange: (value: string) => void; onStrategyChange: (value: FusionConflictAction) => void; onNoteChange: (value: string) => void; onResolve: () => void; onCreateRepair: () => void; onAudit: () => void; onExport: () => void }) {
  const operation = events.find((item) => item.resource_id === conflict?.conflict_id);
  const repairPending = conflict?.status === 'repair_pending';
  return <WorkPanel title={`冲突处理 #${conflict?.conflict_id ?? '-'}`} extra={<Space size={6}>{conflict && <StatusTag value={severityLabel(conflict.severity)} />}<Button aria-label="关闭冲突详情" size="small" type="text" onClick={onClose}>×</Button></Space>}>{conflict && <><section className="taf-fusion-detail-section"><h4>冲突概览</h4><dl className="taf-fusion-conflict-facts"><dt>实体类型</dt><dd>{objectTypeLabel(conflict.object_type)}</dd><dt>实体 ID</dt><dd title={conflict.object_id}>{conflict.object_id}</dd><dt>冲突字段</dt><dd>{conflict.field_name}</dd><dt>冲突级别</dt><dd>{severityLabel(conflict.severity)}</dd><dt>创建时间</dt><dd>{dateTimeLabel(conflict.detected_at)}</dd><dt>状态</dt><dd>{statusLabel(conflict.status)}</dd></dl></section><section className="taf-fusion-detail-section"><h4>冲突值对比（{conflict.field_name}）</h4><div className="taf-fusion-value-head"><span>来源</span><span>值</span><span>置信度</span><span>更新时间</span></div><div className="taf-fusion-value-compare">{conflict.source_values.map((item) => <button type="button" className={selectedSource === item.source ? 'is-selected' : ''} key={item.source} disabled={repairPending} onClick={() => onSourceChange(item.source)}><span>{item.source}</span><strong>{item.value}</strong><em>{item.confidence.toFixed(2)}</em><small>{timeLabel(item.observed_at ?? conflict.updated_at)}</small></button>)}</div></section><section className="taf-fusion-detail-section"><h4>冲突原因</h4><p>{String(conflict.detail?.reason ?? '来源观测值不一致，需要按权威来源确认。')}</p></section><div className="taf-fusion-confidence-line"><span>置信度阈值</span><strong>{conflict.confidence.toFixed(2)}</strong><StatusTag value={conflict.confidence >= 0.8 ? '中高' : '中'} /></div><label className="taf-fusion-resolution"><span>覆盖策略</span><Select size="small" value={strategy} disabled={!canWrite || repairPending} onChange={onStrategyChange} options={[{ value: 'authoritative-source', label: '优先 CMDB，其次 EDR，再次 DHCP，最后 Flow' }, { value: 'accept-primary', label: '接受当前主值' }, { value: 'manual-repair-task', label: '人工确认后回写' }]} /></label><label className="taf-fusion-resolution"><span>节点备注</span><Input.TextArea rows={2} value={note} disabled={!canWrite || repairPending} onChange={(event) => onNoteChange(event.target.value)} /></label><section className="taf-fusion-detail-section taf-fusion-operation"><h4>操作记录</h4><p>{operation ? `${dateTimeLabel(operation.timestamp)} · ${auditLabel(operation.action)} · ${operation.result === 'failure' ? '失败' : '成功'}` : '暂无操作记录'}</p></section><div className="taf-fusion-drawer-actions"><Button type="primary" loading={resolving} disabled={!canWrite || repairPending} onClick={onResolve}>确认主值</Button><Button loading={resolving} disabled={!canWrite || repairPending} onClick={onCreateRepair}>{repairPending ? '修复任务已创建' : '创建修复任务'}</Button><Button onClick={onAudit}>查看审计记录</Button><Button loading={exporting} disabled={!canWrite} icon={<FileDoneOutlined />} onClick={onExport}>输出到验收证据</Button></div></>}</WorkPanel>;
}

const sourceTone = (source: FusionSource): Tone => source.status === 'unavailable' ? 'risk' : source.status === 'active' ? (source.error_rate != null && source.error_rate > 0.03 ? 'warn' : 'ok') : source.last_ingest_at > 0 ? 'risk' : 'warn';
const batchLabel = (value: number) => {
  if (!value) return '待接入';
  const date = new Date(value);
  return `${date.getFullYear()}${String(date.getMonth() + 1).padStart(2, '0')}${String(date.getDate()).padStart(2, '0')}${String(date.getHours()).padStart(2, '0')}`;
};
const latencyLabel = (value: number) => {
  if (!value) return '-';
  const seconds = Math.max(0, Math.round((Date.now() - value) / 1000));
  if (seconds < 120) return `${seconds}s`;
  if (seconds < 7200) return `${Math.round(seconds / 60)}分钟`;
  if (seconds < 172800) return `${Math.round(seconds / 3600)}小时`;
  return `${Math.round(seconds / 86400)}天`;
};
const sourceCompleteness = (source: FusionSource) => source.field_coverage == null ? '未计算' : `${Math.max(0, Math.min(100, source.field_coverage * 100)).toFixed(1)}%`;
const detailText = (rule: FusionRuleOverride, key: string) => String(rule.detail?.[key] ?? '-');
const detailNumber = (rule: FusionRuleOverride, key: string) => Number(rule.detail?.[key] ?? 0);
const detailNumberArray = (rule: FusionRuleOverride, key: string) => Array.isArray(rule.detail?.[key]) ? (rule.detail[key] as unknown[]).map(Number).filter(Number.isFinite) : [];
const percent = (value: number) => `${(value <= 1 ? value * 100 : value).toFixed(2)}%`;
const formatNumber = (value: number) => new Intl.NumberFormat('zh-CN').format(Math.round(value));
const timeLabel = (value: number) => value ? new Date(value).toLocaleTimeString('zh-CN', { hour12: false }) : '-';
const severityLabel = (value?: string) => value === 'high' ? '高' : value === 'critical' ? '严重' : value === 'low' ? '低' : '中';
const statusLabel = (value: string) => value === 'pending' ? '待确认' : value === 'repair_pending' ? '修复中' : value === 'resolved' ? '已处理' : value;
const objectTypeLabel = (value: string) => ({ host: '主机', asset: '资产', account: '账号', entity: '实体' }[value] ?? value);
const auditLabel = (value: string) => ({
  FUSION_CONFLICT_RESOLVED: '冲突解决',
  FUSION_RULE_UPDATED: '规则更新',
  FUSION_EVIDENCE_EXPORTED: '证据导出',
  FUSION_SOURCE_SYNC_REQUESTED: '来源同步',
  FUSION_ENTITY_MERGED: '实体合并',
  FUSION_FIELD_COMPLETED: '字段补全',
  FUSION_CONFLICT_CREATED: '冲突解决',
  FUSION_RELATION_ESTABLISHED: '关联建立',
  FUSION_RESOLUTION_UPDATED: '解析更新',
}[value] ?? value);
const auditDetailText = (event: FusionAuditEvent, key: string) => {
  const value = event.details?.[key];
  return value == null || value === '' ? '' : String(value);
};
const auditRuleId = (event: FusionAuditEvent) => auditDetailText(event, 'rule_id') || (event.resource_type === 'fusion_rule' ? event.resource_id : '-');
const auditOperator = (event: FusionAuditEvent) => {
  const label = auditDetailText(event, 'operator_label');
  if (label) return label;
  const userId = event.user_id;
  return !userId || userId === 'system' ? '系统' : userId.length > 12 ? `${userId.slice(0, 10)}…` : userId;
};
const auditEntityChange = (event: FusionAuditEvent) => {
  const entity = objectTypeLabel(auditDetailText(event, 'object_type') || event.resource_type.replace(/^fusion_/, ''));
  const field = auditDetailText(event, 'field_name');
  const before = auditDetailText(event, 'before_value');
  const changed = auditDetailText(event, 'selected_value') || auditDetailText(event, 'status') || (auditDetailText(event, 'version') ? `v${auditDetailText(event, 'version')}` : '已记录');
  return `${entity}${before ? ` ${before}` : field ? ` ${field}` : ''} → ${changed}`;
};
const strategyLabel = (value: string) => ({ 'authoritative-source': '权威优先', 'weighted-confidence': '置信度优先', 'latest-observation': '最新优先', 'manual-review': '人工复核' }[value] ?? value);
const ruleOutputLabel = (value: string) => ({ IP_MAC_BIND_V3: '主机实体', ACCOUNT_HOST_LINK: '账号实体', ASSET_DEPT_COMPLETION: '资产实体', DOMAIN_IP_RESOLUTION: '域名实体', ALERT_ASSET_JOIN: '告警实体', VULN_SERVICE_MATCH: '服务实体' }[value] ?? '融合实体');
const dateTimeLabel = (value: number) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-';
const errorText = (error: unknown) => error instanceof Error ? error.message : 'Fusion 操作失败';

function downloadBase64(filename: string, content: string, mime: string) {
  const binary = window.atob(content);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  const url = URL.createObjectURL(new Blob([bytes], { type: mime }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
