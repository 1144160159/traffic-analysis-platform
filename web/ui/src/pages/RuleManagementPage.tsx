import {
  ApiOutlined,
  ArrowRightOutlined,
  AuditOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  CheckCircleOutlined,
  CloseOutlined,
  CloudUploadOutlined,
  CodeOutlined,
  DatabaseOutlined,
  DownloadOutlined,
  EditOutlined,
  ExclamationCircleOutlined,
  EyeOutlined,
  ExperimentOutlined,
  FileTextOutlined,
  FilterOutlined,
  FlagOutlined,
  HistoryOutlined,
  ImportOutlined,
  PlayCircleOutlined,
  PlaySquareOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SettingOutlined,
  StopOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Select, Space, Switch, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { Fragment, useEffect, useMemo, useState } from 'react';
import { CloseableConfirmButton } from '@/components/CloseableConfirmButton';
import { DataQualityKpiSparklineChart, RuleDependencyGraphChart, RuleHitComparisonChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  fetchPageSnapshot,
  fetchRulesPage,
  fetchRuleWorkbench,
  submitRuleWorkbenchAction,
  type RuleRecord,
  type RuleWorkbench,
} from '@/services/api';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const ruleConditions = [
  ['协议 (proto)', '在', 'TLS_SSH'],
  ['JA3 指纹 (ja3_score)', '大于', '0.82'],
  ['目的IP信誉 (dst_reputation)', '等于', 'high'],
  ['出站流量 P95 (bytes_out_p95)', '大于', '5 MB'],
];

const ruleFieldOptions = ruleConditions.map(([field]) => ({ label: field, value: field }));
const ruleOperatorOptions = ['在', '大于', '等于', '不在', '小于'].map((value) => ({ label: value, value }));
const ruleValueOptions: Record<string, string[]> = {
  '协议 (proto)': ['TLS_SSH', 'TLS', 'SSH', 'QUIC'],
  'JA3 指纹 (ja3_score)': ['0.82', '0.75', '0.90'],
  '目的IP信誉 (dst_reputation)': ['high', 'medium', 'critical'],
  '出站流量 P95 (bytes_out_p95)': ['5 MB', '10 MB', '20 MB'],
};

const buildRuleDsl = (ruleId: string) => `when
  proto in {"TLS", "SSH"}
  and ja3_score > 0.82
  and dst_reputation == "high"
  and bytes_out_p95 > 5 MB
then
  alert("${ruleId}")
  level = high
  category = "C2"
  mitre = ["TA0011"]
end`;

const sampleRows = [
  ['c2_tunnel_01.pcap', '12.4 MB', '探针-01', '06-19 22:18'],
  ['c2_tunnel_02.pcap', '8.7 MB', '探针-07', '06-19 19:41'],
  ['c2_tunnel_03.pcap', '15.2 MB', '探针-03', '06-19 16:02'],
  ['c2_tunnel_04.pcap', '6.3 MB', '探针-12', '06-19 13:37'],
];

const sessionSampleRows = [
  ['ses_20260619_001', '10.12.4.12:53120 → 10.20.4.19:443', 'TLS / JA3命中', 'tls.sni · ja3_hash'],
  ['ses_20260619_002', '10.12.4.12:49102 → 10.20.4.20:3306', 'MySQL 异常外联', 'dst_port · bytes_out'],
  ['ses_20260619_003', '10.12.4.18:55221 → 10.20.0.53:53', 'DNS 查询突增', 'qname · qtype'],
  ['ses_20260619_004', '10.12.7.23:3389 → 10.12.4.21:445', 'RDP/SMB 横向', 'proto · duration'],
];

const logSampleRows = [
  ['log_20260619_001', 'FW-北区-01', 'dst_port · action', 'C2端口命中', false],
  ['log_20260619_002', '用户事件', 'account · login_time', '非工作时间登录', true],
  ['log_20260619_003', 'WAF-实验楼', 'uri · method', 'WebShell 字段命中', false],
  ['log_20260619_004', 'DNS-01', 'qname · qtype', '隧道域名特征', false],
] as const;

const validationRows = [
  ['SMP-0001', 'PCAP', 'dest_ip, dport, proto', '告警（高危）', '告警（高危）', '—', 'PCAP 包 32'],
  ['SMP-0002', 'Session', 'http_host, uri, method', '告警（中危）', '忽略', '误报', 'Session 78'],
  ['SMP-0003', 'PCAP', 'dns.qname, qtype', '忽略', '忽略', '—', 'PCAP 包 12'],
  ['SMP-0004', 'Session', 'tls.sni, ja3.hash', '告警（低危）', '告警（低危）', '—', 'Session 45'],
  ['SMP-0005', 'PCAP', 'src_ip, bytes, duration', '告警（高危）', '告警（中危）', '级别差异', 'PCAP 包 56'],
];

const dependencyRows = [
  ['模型', 'Model-XGB-17', 'v2.3.1 / 生效中', '全量告警评分与命中', '高', '2026-06-18 11:23:15'],
  ['白名单', 'WL-VPN-003', '生效中（12 条）', '受影响规则命中结果', '中', '2026-06-18 16:42:31'],
  ['部署', 'PROD-北区', '生产 / 运行中', '1,256 台资产', '高', '2026-06-19 09:10:02'],
  ['数据源', 'detections.v1', 'v1.8.2 / 实时', '全量流量解析', '低', '2026-06-18 22:51:17'],
  ['字段', 'src_ip / dst_port / tls.sni', '映射完整 / 生效', '规则匹配与提取', '低', '2026-06-18 10:05:43'],
  ['告警类型', 'C2 Beacon', 'v1.2.0 / 生效中', '告警联动与分级', '中', '2026-06-18 14:33:20'],
];

const ruleMetricIcons: Record<string, ReactNode> = {
  规则草稿: <FileTextOutlined />,
  待审核规则: <SafetyCertificateOutlined />,
  灰度规则: <ApiOutlined />,
  启用规则: <PlayCircleOutlined />,
  回滚候选: <RollbackOutlined />,
  高耗时规则: <ClockCircleOutlined />,
};

const matrixRows = [
  ['C2_Tunnel_v3', '1,142', '4', '18,932', '8', '0.35%'],
  ['Lateral_Move_v2', '837', '3', '19,102', '6', '0.25%'],
  ['DNS_Tunnel_v2', '598', '3', '19,144', '4', '0.33%'],
  ['Data_Exfil_v1', '1,034', '5', '18,706', '7', '0.48%'],
];

const fpRows = [
  ['fp_20260619_001', '7', '正常流量', '探针-05'],
  ['fp_20260619_002', '5', '软件更新', '探针-02'],
  ['fp_20260618_009', '4', '备份同步', '探针-09'],
  ['fp_20260618_015', '3', '远程管理', '探针-11'],
];

const whitelistRows = [
  ['10.12.2.45', '156'],
  ['10.12.3.78', '121'],
  ['172.16.5.23', '98'],
  ['update.campus.local', '86'],
  ['backup.internal.local', '65'],
];

const ruleOverlays: OverlayContract[] = [
  {
    id: 'modal-rule-edit',
    title: '新建/编辑规则',
    kind: 'Modal',
    actionLabel: '规则编辑',
    description: '编辑规则条件、样本验证、命中阈值、标签和回滚版本。',
    impact: '影响实时检测命中和告警生成。',
    audit: '记录规则草稿、测试结果、发布版本和操作者 trace。',
  },
  {
    id: 'drawer-rule-detail',
    title: '规则详情',
    kind: 'Drawer',
    actionLabel: '规则详情',
    description: '展示规则版本、命中趋势、误报样本、白名单建议和审计记录。',
  },
  {
    id: 'modal-rule-publish',
    title: '规则发布确认',
    kind: 'Modal',
    actionLabel: '发布确认',
    description: '确认发布范围、灰度比例、回滚条件和影响评估。',
    impact: '影响 Flink rule job 与在线告警策略。',
    danger: true,
  },
];

type RuleAction = {
  title: string;
  target: string;
  actionId: string;
  endpoint: string;
  auditEvent: string;
};

export function RuleManagementPage({ route }: { route: NavRoute }) {
  const visualMode = isVisualBreakdownMode();
  const [editorTab, setEditorTab] = useState('规则定义');
  const [sampleTab, setSampleTab] = useState('PCAP 样本 32');
  const [selectedKey, setSelectedKey] = useState<string>();
  const [typeFilter, setTypeFilter] = useState('全部类型');
  const [statusFilter, setStatusFilter] = useState('全部状态');
  const [query, setQuery] = useState('');
  const [listPage, setListPage] = useState(1);
  const [action, setAction] = useState<RuleAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const pageSize = 7;
  const ruleListQuery = useQuery({
    queryKey: ['rules', listPage, pageSize, typeFilter, statusFilter, query],
    queryFn: () => fetchRulesPage({
      page: listPage,
      pageSize,
      keyword: query.trim() || undefined,
      type: apiRuleType(typeFilter),
      enabled: apiRuleEnabled(statusFilter),
      labels: statusFilter === '灰度' ? 'gray' : undefined,
    }),
    enabled: !visualMode,
  });
  const rows = useMemo(
    () => visualMode
      ? buildVisualRuleRows(data?.rows ?? [])
      : (ruleListQuery.data?.items ?? []).map(ruleRecordToSnapshotRow),
    [data?.rows, ruleListQuery.data?.items, visualMode],
  );
  const filteredRows = useMemo(() => rows.filter((row) => {
    if (!visualMode) return true;
    const typeMatches = typeFilter === '全部类型' || String(row.类型).includes(typeFilter);
    const statusMatches = statusFilter === '全部状态' || String(row.状态).includes(statusFilter);
    const text = `${row.规则ID ?? ''} ${row.规则名称 ?? ''} ${row.MITRE阶段 ?? ''}`.toLowerCase();
    return typeMatches && statusMatches && (!query || text.includes(query.toLowerCase()));
  }), [query, rows, statusFilter, typeFilter, visualMode]);
  const totalRows = visualMode ? filteredRows.length : (ruleListQuery.data?.total ?? 0);
  const pageCount = Math.max(1, Math.ceil(totalRows / pageSize));
  const visibleRows = visualMode ? filteredRows.slice((listPage - 1) * pageSize, listPage * pageSize) : filteredRows;
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const selectedRuleId = String(selected?.['规则ID'] ?? '');
  const workbenchQuery = useQuery({
    queryKey: ['rule-workbench', selectedRuleId],
    queryFn: () => fetchRuleWorkbench(selectedRuleId),
    enabled: !visualMode && Boolean(selectedRuleId),
  });
  const workbench = visualMode ? undefined : workbenchQuery.data;
  const actionMutation = useMutation({
    mutationFn: (current: RuleAction) => submitRuleWorkbenchAction({
      ruleId: String(selected?.['规则ID'] ?? current.target),
      action: current.actionId,
      target: current.target,
      payload: { title: current.title },
    }),
    onSuccess: () => setActionSubmitted(true),
  });
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: ruleColumnWidths[column] ?? 56,
    ellipsis: true,
    render: (value) => renderRuleCell(column, value),
  }));
  function changeEditorTab(tab: string) {
    setEditorTab(tab);
  }
  function changeSampleTab(tab: string) {
    setSampleTab(tab);
  }
  function openAction(title: string, target = String(selected?.['规则ID'] ?? 'C2_Tunnel_v3')) {
    setActionSubmitted(false);
    actionMutation.reset();
    setAction(createRuleAction(title, target));
  }
  function refreshAll() {
    void Promise.all([refetch(), ruleListQuery.refetch(), workbenchQuery.refetch()]);
  }

  return (
    <div className="taf-page taf-rules">
      <section className="taf-rules-shell">
        <main className="taf-rules-main">
          <header className="taf-rules-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space size={6}>
              <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => openAction('规则库')}>规则库</Button>
              <Button size="small" icon={<ImportOutlined />} onClick={() => openAction('导入规则')}>导入规则</Button>
              <Button size="small" icon={<DownloadOutlined />} onClick={() => openAction('导出规则')}>导出规则</Button>
              <Button size="small" icon={<SettingOutlined />} onClick={() => openAction('规则包管理')}>规则包管理</Button>
              <Button size="small" type="primary" icon={<EditOutlined />} onClick={() => openAction('新建规则')}>新建规则</Button>
              <Tooltip title="刷新规则库">
                <Button size="small" icon={<ReloadOutlined />} onClick={refreshAll} />
              </Tooltip>
              <OverlayContractHost overlays={ruleOverlays} compact />
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/rules、APISIX 路由或 rule-manager。'}
              action={<Button size="small" danger onClick={refreshAll}>重试</Button>}
            />
          )}
          {!visualMode && workbenchQuery.isError && (
            <Alert
              type="error"
              showIcon
              message="规则工作台数据加载失败"
              description={workbenchQuery.error instanceof Error ? workbenchQuery.error.message : '请检查 /v1/rules/{id}/workbench 与 PostgreSQL seed。'}
              action={<Button size="small" danger onClick={() => void workbenchQuery.refetch()}>重试</Button>}
            />
          )}
          {!visualMode && ruleListQuery.isError && (
            <Alert
              type="error"
              showIcon
              message="规则列表分页加载失败"
              description={ruleListQuery.error instanceof Error ? ruleListQuery.error.message : '请检查 /v1/rules 分页接口。'}
              action={<Button size="small" danger onClick={() => void ruleListQuery.refetch()}>重试</Button>}
            />
          )}

          <div className="taf-rules-kpis">
                {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} icon={ruleMetricIcons[metric.label]} />)}
          </div>

          <div className="taf-rules-workbench">
            <WorkPanel title={`规则列表（共 ${totalRows} 条）`} className="taf-rules-list-panel">
              <div className="taf-rules-filterbar">
                <label><SearchOutlined /><input aria-label="搜索规则" value={query} onChange={(event) => { setQuery(event.target.value); setListPage(1); }} placeholder="搜索规则名称 / 规则ID" /></label>
                <Select size="small" value={typeFilter} options={[{ value: '全部类型' }, { value: '流量' }, { value: '文件' }]} onChange={(value) => { setTypeFilter(value); setListPage(1); }} />
                <Select size="small" value={statusFilter} options={[{ value: '全部状态' }, { value: '启用' }, { value: '灰度' }]} onChange={(value) => { setStatusFilter(value); setListPage(1); }} />
                <Button size="small" icon={<FilterOutlined />} aria-label="保存规则筛选条件" onClick={() => openAction('保存筛选条件')} />
                <Button size="small" icon={<ReloadOutlined />} onClick={refreshAll} />
              </div>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading || ruleListQuery.isLoading}
                pagination={false}
                tableLayout="fixed"
                columns={columns}
                dataSource={visibleRows}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })}
              />
              <div className="taf-rules-pagination"><span>共 {totalRows} 条</span><button type="button" aria-label="规则上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`规则第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="规则下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
            </WorkPanel>

            <WorkPanel title={`规则编辑：${String(selected?.['规则ID'] ?? 'C2_Tunnel_v3')}`} extra={<StatusTag value={selected?.状态 ?? '已启用'} />} className="taf-rules-editor-panel">
              <RuleEditor selected={selected} workbench={workbench} visualMode={visualMode} activeTab={editorTab} onTabChange={changeEditorTab} onAction={openAction} />
            </WorkPanel>

            <aside className="taf-rules-rail">
              <WorkPanel title="生命周期">
                <Lifecycle selected={selected} />
              </WorkPanel>
              <WorkPanel title="版本历史">
                <VersionHistory selected={selected} workbench={workbench} visualMode={visualMode} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="审批清单（当前版本 v3.0）">
                <ApprovalList workbench={workbench} visualMode={visualMode} />
              </WorkPanel>
              <WorkPanel title="发布控制">
                <ReleaseControl onAction={openAction} />
              </WorkPanel>
            </aside>
          </div>

          <div className="taf-rules-bottom">
            <WorkPanel title="样本回放验证（近 7 天）">
              <SampleReplay workbench={workbench} visualMode={visualMode} activeTab={sampleTab} onTabChange={changeSampleTab} onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="命中结果矩阵（近 7 天）">
              <HitMatrix workbench={workbench} visualMode={visualMode} onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="误报样本 Top5">
              <FalsePositiveTop workbench={workbench} visualMode={visualMode} onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="性能影响（近 7 天）">
              <PerformanceImpact workbench={workbench} visualMode={visualMode} />
            </WorkPanel>
            <WorkPanel title="白名单草案（命中高但低风险）">
              <WhitelistDraft workbench={workbench} visualMode={visualMode} onAction={openAction} />
            </WorkPanel>
            <WorkPanel title="相关操作">
              <RelatedActions onAction={openAction} />
            </WorkPanel>
          </div>
        </main>
      </section>
      <Drawer className="taf-rules-action-drawer" title={action ? `${action.title}确认` : '规则操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); actionMutation.reset(); }} extra={<Button size="small" type="primary" loading={actionMutation.isPending} disabled={actionSubmitted || visualMode} onClick={() => action && actionMutation.mutate(action)}>{visualMode ? '视觉模式不可提交' : actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为规则对象创建“{action.title}”持久化任务，并保留租户、审批与审计上下文。</p><dl><dt>规则对象</dt><dd>{action.target}</dd><dt>真实接口</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionMutation.isError && <Alert type="error" showIcon message="规则业务操作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : '请稍后重试'} />}{actionSubmitted && <Alert type="success" showIcon message="规则业务操作已写入任务队列与审计日志" description={`任务 ${actionMutation.data?.job_id ?? '-'}；目标：${action.target}`} />}</div>}
      </Drawer>
    </div>
  );
}

function RuleEditor({
  selected,
  workbench,
  visualMode,
  activeTab,
  onTabChange,
  onAction,
}: {
  selected?: SnapshotRow;
  workbench?: RuleWorkbench;
  visualMode: boolean;
  activeTab: string;
  onTabChange: (value: string) => void;
  onAction: (title: string, target?: string) => void;
}) {
  const definition = firstWorkbenchItem(workbench, 'rule_definition');
  const conditions = visualMode ? ruleConditions : ruleDefinitionConditions(definition, workbench?.rule.conditions);
  const defaultDsl = textValue(definition.dsl) || buildRuleDsl(String(selected?.['规则ID'] ?? 'C2_Tunnel_v3'));
  const mitreLabel = textValue(definition.mitre) || 'TA0011 指挥与控制';
  const [dsl, setDsl] = useState(defaultDsl);
  const [mitreVisible, setMitreVisible] = useState(true);
  useEffect(() => {
    setDsl(defaultDsl);
    setMitreVisible(true);
  }, [defaultDsl, selected?.['规则ID']]);
  return (
    <div className="taf-rules-editor">
      <nav className="taf-rules-editor-tabs">
        {['规则定义', '测试验证', '依赖引用'].map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}>{tab}</button>)}
      </nav>
      {activeTab === '测试验证' ? (
        <RuleTestValidation selected={selected} workbench={workbench} visualMode={visualMode} onAction={onAction} />
      ) : activeTab === '依赖引用' ? (
        <RuleDependencies selected={selected} workbench={workbench} visualMode={visualMode} onAction={onAction} />
      ) : (
        <div className="taf-rules-editor-grid">
        <section>
          <h3>条件构建器</h3>
          <div className="taf-rules-condition-actions"><Button size="small" icon={<BranchesOutlined />} onClick={() => onAction('添加条件组')}>添加条件组</Button><Button size="small" icon={<ApiOutlined />} onClick={() => onAction('添加条件')}>添加条件</Button></div>
          <div className="taf-rules-conditions">
            <strong>且（AND）</strong>
            {conditions.map(([field, op, value]) => <RuleConditionSelectRow key={`${field}-${op}-${value}`} field={field} operator={op} value={value} onRemove={() => onAction('移除条件', field)} />)}
          </div>
          <h3>例外条件（任意满足则排除）</h3>
          <div className="taf-rules-exception"><Select aria-label="例外字段" size="small" value="目的IP" title="目的IP" popupMatchSelectWidth={false} options={[{ label: '目的IP', value: '目的IP' }, { label: '源IP', value: '源IP' }]} /><Select aria-label="例外操作符" size="small" value="在" title="在" popupMatchSelectWidth={false} options={ruleOperatorOptions} /><Select aria-label="例外值" size="small" value="信任列表 (Whitelist_IP_Group)" title="信任列表 (Whitelist_IP_Group)" popupMatchSelectWidth={false} options={[{ label: '信任列表 (Whitelist_IP_Group)', value: '信任列表 (Whitelist_IP_Group)' }, { label: '白名单 (WL-VPN-003)', value: '白名单 (WL-VPN-003)' }]} /><button type="button" aria-label="移除规则例外条件" onClick={() => onAction('移除例外条件')}><CloseOutlined /></button></div>
        </section>
        <section>
          <div className="taf-rules-dsl-title"><h3>DSL 表达式（自动生成）</h3><Button size="small" onClick={() => { setDsl(dsl.split('\n').map((line) => line.trimEnd()).join('\n').trim()); onAction('格式化 DSL'); }}>格式化</Button></div>
          <textarea className="taf-rules-dsl-editor" aria-label="DSL 表达式编辑器" value={dsl} spellCheck={false} onChange={(event) => setDsl(event.target.value)} />
          <h3 className="taf-rules-mitre-title">MITRE 阶段（点击删除）</h3>
          <div className="taf-rules-mitre">
            {mitreVisible ? <span><b>{mitreLabel}</b><button type="button" aria-label={`删除 MITRE 阶段 ${mitreLabel}`} onClick={() => { setMitreVisible(false); onAction('删除 MITRE 阶段', mitreLabel); }}><CloseOutlined /></button></span> : <em>尚未选择 MITRE 阶段</em>}
            <Button size="small" onClick={() => { setMitreVisible(true); onAction('添加 MITRE 阶段'); }}>添加阶段</Button>
          </div>
        </section>
        </div>
      )}
    </div>
  );
}

function RuleConditionSelectRow({ field, operator, value, onRemove }: { field: string; operator: string; value: string; onRemove: () => void }) {
  const [selectedField, setSelectedField] = useState(field);
  const [selectedOperator, setSelectedOperator] = useState(operator);
  const [selectedValue, setSelectedValue] = useState(value);
  const valueOptions = (ruleValueOptions[selectedField] ?? [selectedValue]).map((item) => ({ label: item, value: item }));
  return (
    <div className="taf-rules-condition-row">
      <Select aria-label={`${field} 字段`} size="small" value={selectedField} title={selectedField} popupMatchSelectWidth={false} options={ruleFieldOptions} onChange={(nextField) => { setSelectedField(nextField); setSelectedValue(ruleValueOptions[nextField]?.[0] ?? ''); }} />
      <Select aria-label={`${field} 操作符`} size="small" value={selectedOperator} title={selectedOperator} popupMatchSelectWidth={false} options={ruleOperatorOptions} onChange={setSelectedOperator} />
      <Select aria-label={`${field} 值`} size="small" value={selectedValue} title={selectedValue} popupMatchSelectWidth={false} options={valueOptions} onChange={setSelectedValue} />
      <button type="button" aria-label={`移除条件 ${field}`} onClick={onRemove}><CloseOutlined /></button>
    </div>
  );
}

function RuleTestValidation({ selected, workbench, visualMode, onAction }: { selected?: SnapshotRow; workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const target = String(selected?.['规则ID'] ?? 'C2_Tunnel_v3');
  const rows = visualMode
    ? validationRows
    : workbenchItems(workbench, 'validation_results').map((item) => [
      textValue(item.sample), textValue(item.type), textValue(item.fields), textValue(item.actual),
      textValue(item.expected), textValue(item.difference), textValue(item.source),
    ]);
  return (
    <div className="taf-rules-test-validation">
      <header className="taf-rules-test-toolbar">
        <span><DatabaseOutlined />样本集 <b>PCAP+Session混合样本</b></span>
        <span><ClockCircleOutlined />时间窗 <b>02:00–04:00</b></span>
        <span><HistoryOutlined />回放批次 <b>RB-20260626-031</b></span>
        <Button size="small" type="primary" icon={<PlaySquareOutlined />} onClick={() => onAction('开始回放', target)}>开始回放</Button>
        <Button size="small" danger icon={<StopOutlined />} onClick={() => onAction('停止回放', target)}>停止</Button>
      </header>
      <div className="taf-rules-test-upper">
        <section className="taf-rules-replay-card">
          <h3>样本回放</h3>
          <div className="taf-rules-replay-progress"><span>回放进度</span><progress value="64.3" max="100" /><b>64.3%</b></div>
          <div className="taf-rules-replay-totals">
            {[['PCAP 样本', '32'], ['Session 样本', '128'], ['日志样本', '256'], ['总计', '416']].map(([label, value]) => <span key={label}><em>{label}</em><b>{value}</b></span>)}
          </div>
          <div className="taf-rules-replay-status"><span>回放状态 <b>回放中</b></span><span>解析状态 <b>正常</b></span><span>规则状态 <b>生效中</b></span></div>
          <div className="taf-rules-replay-result"><span><SafetyCertificateOutlined /><em>已命中</em><b>1,568</b></span><span><ExclamationCircleOutlined /><em>误报样本</em><b>23</b></span></div>
          <footer><span>已耗时：00:24:18</span><span>剩余时间：00:13:42</span><span>回放速率：1.25x</span></footer>
        </section>
        <section className="taf-rules-test-analysis">
          <div className="taf-rules-hit-diff"><h3>命中差异</h3><RuleHitComparisonChart ariaLabel="规则生效前后命中差异" /></div>
          <div className="taf-rules-test-performance"><h3>性能影响</h3><PerformanceImpact workbench={workbench} visualMode={visualMode} /></div>
        </section>
      </div>
      <section className="taf-rules-validation-results">
        <h3>验证结果</h3>
        <div className="taf-rules-validation-head">{['样本ID', '类型', '命中字段', '规则结果', '期望结果', '差异', '证据', '操作'].map((item) => <span key={item}>{item}</span>)}</div>
        <section className="taf-rules-validation-scroll">
          {rows.map((row) => <div key={row[0]}>{row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index >= 3 && index <= 5 ? `is-result is-${String(cell).includes('误报') || String(cell).includes('差异') ? 'warn' : String(cell).includes('忽略') ? 'ok' : 'risk'}` : ''}>{cell}</span>)}<span><Button size="small" icon={<EyeOutlined />} onClick={() => onAction('查看样本', row[0])}>查看样本</Button><Button size="small" onClick={() => onAction('标记误报', row[0])}>标记误报</Button></span></div>)}
        </section>
      </section>
      <footer className="taf-rules-focus-actions"><Button icon={<FileTextOutlined />} onClick={() => onAction('生成验证报告', target)}>生成验证报告</Button><Button icon={<EditOutlined />} onClick={() => onAction('回写阈值', target)}>回写阈值</Button></footer>
    </div>
  );
}

function RuleDependencies({ selected, workbench, visualMode, onAction }: { selected?: SnapshotRow; workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const target = String(selected?.['规则ID'] ?? 'C2_Tunnel_v3');
  const rows = visualMode
    ? dependencyRows
    : workbenchItems(workbench, 'dependencies').map((item) => [
      textValue(item.type), textValue(item.name), textValue(item.version), textValue(item.impact),
      textValue(item.risk), textValue(item.updated_at),
    ]);
  return (
    <div className="taf-rules-dependencies">
      <header className="taf-rules-dependency-summary">
        <span><SafetyCertificateOutlined />当前规则：<b>{target}</b></span>
        <span><FileTextOutlined />版本：<b>{String(selected?.版本 ?? 'v3.0')}</b></span>
        <span><CheckCircleOutlined />状态：<b>已启用</b></span>
        <span><ClockCircleOutlined />最近变更：<b>2026-06-19 17:28:45</b></span>
      </header>
      <div className="taf-rules-dependency-upper">
        <section className="taf-rules-dependency-graph"><h3>依赖引用图</h3><RuleDependencyGraphChart ruleId={target} ariaLabel={`${target} 依赖引用图`} /><footer className="taf-rules-dependency-legend">{[
          ['模型', 'model'], ['白名单', 'whitelist'], ['部署', 'deploy'], ['数据源', 'source'], ['字段', 'field'], ['告警类型', 'alert'],
        ].map(([name, type]) => <span key={name} className={`is-${type}`}><i />{name}</span>)}</footer></section>
        <section className="taf-rules-impact-list"><h3>影响范围提示</h3>{[
          ['生产部署受影响', '规则已部署在生产环境（PROD-北区），关联 1,256 台资产，变更可能影响实际告警行为', '高', '定位部署'],
          ['白名单冲突', '引用白名单 WL-VPN-003 包含 12 条近期变更的条目，可能导致命中结果差异', '中', '打开白名单'],
          ['字段映射缺失', '字段 tls.sni 在部分数据源中未完全映射（缺失率 8.7%）', '低', '校验字段'],
          ['告警类型联动', 'C2 Beacon 与 2 条其他告警类型存在联动，调整可能影响分级策略', '低', '查看联动'],
        ].map(([title, description, risk, action]) => <div key={title} className={`is-${risk === '高' ? 'risk' : risk === '中' ? 'warn' : 'info'}`}><ExclamationCircleOutlined /><span><b>{title}</b><em>{description}</em></span><i>{risk}</i><Button size="small" onClick={() => onAction(action, target)}>{action}</Button></div>)}</section>
      </div>
      <section className="taf-rules-dependency-table">
        <h3>依赖关系</h3>
        <div className="taf-rules-dependency-head">{['类型', '引用对象', '版本/状态', '影响范围', '风险', '最近变更', '操作'].map((item) => <span key={item}>{item}</span>)}</div>
        <section className="taf-rules-dependency-scroll">
          {rows.map((row) => <div key={`${row[0]}-${row[1]}`}>{row.map((cell, index) => <span key={`${row[0]}-${index}`} className={index === 4 ? `is-risk-${cell}` : ''}>{cell}</span>)}<Button size="small" onClick={() => onAction(`查看${row[0]}`, row[1])}>查看{row[0]}</Button></div>)}
        </section>
      </section>
      <footer className="taf-rules-focus-actions"><Button icon={<FileTextOutlined />} onClick={() => onAction('生成影响报告', target)}>生成影响报告</Button><Button icon={<SyncOutlined />} onClick={() => onAction('重新校验依赖', target)}>重新校验依赖</Button></footer>
    </div>
  );
}

function Lifecycle({ selected }: { selected?: SnapshotRow }) {
  const steps = [
    ['草稿', <FileTextOutlined />],
    ['待审', <AuditOutlined />],
    ['灰度', <ExperimentOutlined />],
    ['启用', <PoweroffOutlined />],
    ['停用', <StopOutlined />],
    ['回滚', <RollbackOutlined />],
  ] as const;
  const current = resolveLifecycleStatus(selected?.状态);
  const changedAt = String(selected?.['最近状态变更'] ?? '暂无 API 变更时间');
  const operator = String(selected?.['状态操作人'] ?? 'system');
  return (
    <div className="taf-rules-lifecycle" data-current-status={current.label} data-current-index={current.index}>
      <div className="taf-rules-lifecycle-track">
        {steps.map(([title, icon], index) => <Fragment key={title}><div data-stage={title} aria-current={index === current.index ? 'step' : undefined} className={`taf-rules-lifecycle-segment${index === current.index ? ' is-current' : ''}${index < current.index ? ' is-complete' : ''}`}><span className="taf-rules-lifecycle-stage"><i>{icon}</i><em>{title}</em></span></div>{index < steps.length - 1 && <div className="taf-rules-lifecycle-connector" aria-hidden="true"><span className="taf-rules-lifecycle-line" /><ArrowRightOutlined /><span className="taf-rules-lifecycle-line" /></div>}</Fragment>)}
      </div>
      <div className="taf-rules-lifecycle-meta"><span>当前状态：<b>{current.label}</b></span><span>最近状态变更：<b title={changedAt}>{changedAt}</b><em title={operator}>操作人&nbsp; {operator}</em></span></div>
    </div>
  );
}

function resolveLifecycleStatus(value: SnapshotRow[string] | undefined) {
  const label = ruleLifecycleLabel(String(value ?? ''));
  const index = ({ 草稿: 0, 待审: 1, 灰度: 2, 启用: 3, 停用: 4, 回滚: 5 } as const)[label];
  return { index, label } as const;
}

function VersionHistory({ selected, workbench, visualMode, onAction }: { selected?: SnapshotRow; workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const version = String(selected?.版本 ?? 'v3.0');
  const fallbackRows = [
    [version, '当前', '2026-06-19 17:28:45', 'sec_analyst'],
    ['v2.9', '启用', '2026-06-15 16:42:12', 'sec_analyst'],
    ['v2.8', '停用', '2026-06-12 10:11:33', 'system'],
    ['v2.7', '启用', '2026-06-08 09:31:22', 'sec_analyst'],
  ];
  const rows = !visualMode && workbench?.versions?.length
    ? workbench.versions.slice(0, 4).map((item, index) => [
      `v${item.version}.0`, index === 0 ? '当前' : ruleLifecycleLabel(item.status), formatRuleTime(item.created_at), item.created_by || 'system',
    ])
    : visualMode ? fallbackRows : [];
  return (
    <div className="taf-rules-version">
      {rows.map(([item, status, time, operator]) => (
        <div key={item}><HistoryOutlined /><strong>{item}</strong><span>{status}</span><time>{time}</time><em>{operator}</em></div>
      ))}
      <Button size="small" type="link" onClick={() => onAction('查看更多版本', String(selected?.['规则ID'] ?? 'C2_Tunnel_v3'))}>查看更多版本</Button>
    </div>
  );
}

function ApprovalList({ workbench, visualMode }: { workbench?: RuleWorkbench; visualMode: boolean }) {
  const rows = visualMode
    ? ['语法校验', '逻辑评审', '安全评审', '运营评审', '最终审核'].map((name, index) => ({ name, status: index < 2 ? '已通过' : index < 4 ? '待评审' : '待提交' }))
    : workbenchItems(workbench, 'approvals').map((item) => ({ name: textValue(item.name), status: textValue(item.status) }));
  return (
    <div className="taf-rules-approval">
      {rows.map((item) => (
        <span key={item.name} className={item.status.includes('通过') ? 'is-ok' : item.status.includes('待评审') ? 'is-warn' : ''}><CheckCircleOutlined />{item.name}<b>{item.status}</b></span>
      ))}
    </div>
  );
}

function ReleaseControl({ onAction }: { onAction: (title: string, target?: string) => void }) {
  return (
    <div className="taf-rules-release">
      <Button size="small" onClick={() => onAction('灰度设置')}>灰度设置</Button>
      <Button size="small" type="primary" onClick={() => onAction('全量发布')}>全量发布</Button>
      <Button size="small" danger ghost icon={<RollbackOutlined />} onClick={() => onAction('回滚版本')}>回滚版本</Button>
      <CloseableConfirmButton
        title="删除规则"
        description="影响范围：当前规则草稿及关联测试记录；确认删除后写入 audit trace，可从版本历史追溯。"
        confirmText="确认删除"
        danger
        buttonProps={{ className: 'taf-rules-delete', size: 'small', danger: true }}
        onConfirm={() => onAction('删除规则')}
      >
        删除规则
      </CloseableConfirmButton>
      <Button size="small" type="link" onClick={() => onAction('进入部署管理')}>进入部署管理</Button>
    </div>
  );
}

function SampleReplay({ workbench, visualMode, activeTab, onTabChange, onAction }: { workbench?: RuleWorkbench; visualMode: boolean; activeTab: string; onTabChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  const tabs = ['PCAP 样本 32', 'Session 样本 128', '日志样本 256'];
  const isSession = activeTab.startsWith('Session');
  const isLogs = activeTab.startsWith('日志');
  const headers = isSession
    ? ['会话ID', '五元组', '协议摘要', '命中字段', '操作']
    : isLogs
      ? ['日志ID', '来源', '规则字段', '命中原因', '操作']
      : ['文件名', '大小', '来源', '时间', '操作'];
  const pcapRows = visualMode ? sampleRows : workbenchItems(workbench, 'pcap_samples').map((item) => [textValue(item.id), textValue(item.size), textValue(item.source), textValue(item.time)]);
  const sessionRows = visualMode ? sessionSampleRows : workbenchItems(workbench, 'session_samples').map((item) => [textValue(item.id), textValue(item.tuple), textValue(item.protocol), stringArray(item.fields).join(' · ')]);
  const logsRows = visualMode ? logSampleRows : workbenchItems(workbench, 'log_samples').map((item) => [textValue(item.id), textValue(item.source), stringArray(item.fields).join(' · '), textValue(item.reason), Boolean(item.false_positive)] as const);
  return (
    <div className={`taf-rules-samples is-${isSession ? 'session' : isLogs ? 'logs' : 'pcap'}`}>
      <nav className="taf-rules-sample-tabs">{tabs.map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} onClick={() => onTabChange(tab)}><span>{tab}</span></button>)}</nav>
      <div className="taf-rules-sample-table">
        <header className="taf-rules-sample-head">{headers.map((header) => <span key={header}>{header}</span>)}</header>
        {isSession
          ? sessionRows.map(([id, tuple, protocol, fields], index) => <div className="taf-rules-sample-row" key={id}><b>{id}</b><em>{tuple}</em><em>{protocol}</em><span className={`taf-rules-sample-tags${index % 2 ? ' is-warn' : ''}`}>{fields.split(' · ').map((field) => <i key={field}>{field}</i>)}</span><span className="taf-rules-sample-actions"><Button type="text" size="small" icon={<PlayCircleOutlined />} aria-label={`回放会话 ${id}`} onClick={() => onAction('回放会话', id)} /><Button type="text" size="small" icon={<DownloadOutlined />} aria-label={`下载会话 ${id}`} onClick={() => onAction('下载会话', id)} /></span></div>)
          : isLogs
            ? logsRows.map(([id, source, fields, reason, falsePositive], index) => <div className="taf-rules-sample-row" key={id}><b>{id}</b><em>{source}</em><span className="taf-rules-sample-tags">{fields.split(' · ').map((field) => <i key={field}>{field}</i>)}</span><em className={`taf-rules-sample-reason${index === 2 ? ' is-risk' : ' is-warn'}`}>{reason}</em><span className="taf-rules-sample-actions is-logs"><Button type="text" size="small" icon={<EyeOutlined />} aria-label={`查看日志 ${id}`} onClick={() => onAction('查看日志', id)} /><Button type="text" size="small" danger icon={<FlagOutlined />} aria-label={`标记日志 ${id}`} onClick={() => onAction('标记日志', id)} /><Switch size="small" aria-label={`误报标记 ${id}`} defaultChecked={falsePositive} onChange={() => onAction('切换误报标记', id)} /></span></div>)
            : pcapRows.map(([name, size, source, time]) => <div className="taf-rules-sample-row" key={name}><b>{name}</b><em>{size}</em><em>{source}</em><em>{time}</em><span className="taf-rules-sample-actions"><Button type="text" size="small" icon={<PlayCircleOutlined />} aria-label={`回放样本 ${name}`} onClick={() => onAction('回放样本', name)} /><Button type="text" size="small" icon={<DownloadOutlined />} aria-label={`下载样本 ${name}`} onClick={() => onAction('下载样本', name)} /></span></div>)}
      </div>
      <footer><Button type="link" size="small" onClick={() => onAction('查看全部样本')}>{`查看全部样本 >`}</Button></footer>
    </div>
  );
}

function HitMatrix({ workbench, visualMode, onAction }: { workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const rows = visualMode ? matrixRows : workbenchItems(workbench, 'hit_matrix').map((item) => [textValue(item.rule), textValue(item.tp), textValue(item.fp), textValue(item.tn), textValue(item.fn), textValue(item.false_positive_rate)]);
  return (
    <div className="taf-rules-matrix">
      <div><span>规则</span><span>TP</span><span>FP</span><span>TN</span><span>FN</span><span>误报率</span></div>
      {rows.map((row) => <button key={row[0]} type="button" onClick={() => onAction('查看命中矩阵', row[0])}>{row.map((cell) => <b key={cell}>{cell}</b>)}</button>)}
    </div>
  );
}

function FalsePositiveTop({ workbench, visualMode, onAction }: { workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const rows = visualMode ? fpRows : workbenchItems(workbench, 'false_positives').map((item) => [textValue(item.id), textValue(item.count), textValue(item.type), textValue(item.source)]);
  return (
    <div className="taf-rules-fp">
      {rows.map(([id, count, type, source]) => <button key={id} type="button" onClick={() => onAction('查看误报样本', id)}><b>{id}</b><em>{count}</em><em>{type}</em><em>{source}</em></button>)}
      <Button size="small" type="link" onClick={() => onAction('导出 Top5')}>导出 Top5</Button>
    </div>
  );
}

function PerformanceImpact({ workbench, visualMode }: { workbench?: RuleWorkbench; visualMode: boolean }) {
  const fallbackItems = [
    ['平均延时', '18 ms', '+2 ms', 'info' as const, [14, 16, 15, 17, 16, 18, 18]],
    ['P95 延时', '46 ms', '+5 ms', 'warn' as const, [36, 39, 42, 40, 44, 41, 46]],
    ['CPU占用', '7.4%', '-0.6%', 'ok' as const, [8.1, 8, 7.8, 7.6, 7.9, 7.5, 7.4]],
    ['内存占用', '1.2 GB', '+0.1 GB', 'info' as const, [1, 1.1, 1.1, 1.2, 1.1, 1.2, 1.2]],
  ];
  const items = visualMode ? fallbackItems : workbenchItems(workbench, 'performance').map((item) => [textValue(item.label), textValue(item.value), textValue(item.delta), textValue(item.tone) || 'info', numberArray(item.values)]);
  return <div className="taf-rules-performance">{items.map(([label, value, delta, tone, values]) => <div key={label as string}><span>{label as string}</span><strong>{value as string}</strong><em>{delta as string}</em><div className="taf-rules-performance-echart"><DataQualityKpiSparklineChart ariaLabel={`规则${label as string}趋势`} tone={tone as 'ok' | 'info' | 'warn' | 'risk'} values={values as number[]} /></div></div>)}</div>;
}

function WhitelistDraft({ workbench, visualMode, onAction }: { workbench?: RuleWorkbench; visualMode: boolean; onAction: (title: string, target?: string) => void }) {
  const rows = visualMode ? whitelistRows : workbenchItems(workbench, 'whitelist_suggestions').map((item) => [textValue(item.entity), textValue(item.count)]);
  return (
    <div className="taf-rules-whitelist">
      {rows.map(([entity, count]) => <button key={entity} type="button" onClick={() => onAction('查看白名单建议', entity)}><b>{entity}</b><em>{count}</em></button>)}
      <Button size="small" type="link" onClick={() => onAction('生成白名单草案')}>生成白名单草案</Button>
    </div>
  );
}

function RelatedActions({ onAction }: { onAction: (title: string, target?: string) => void }) {
  const actions: Array<[string, ReactNode]> = [
    ['查看命中规则', <SearchOutlined key="search" />],
    ['进入部署管理', <CloudUploadOutlined key="deploy" />],
    ['生成白名单草案', <SafetyCertificateOutlined key="white" />],
    ['回滚版本', <RollbackOutlined key="rollback" />],
    ['导出规则包', <DownloadOutlined key="download" />],
  ];
  return <div className="taf-rules-actions">{actions.map(([label, icon]) => <Button key={label} size="small" icon={icon} onClick={() => onAction(label)}>{label}</Button>)}</div>;
}

const renderRuleCell = (column: string, value: unknown) => {
  if (column === '规则ID') return <span className="taf-rules-id-cell"><CodeOutlined />{String(value)}</span>;
  if (column === '严重级别' || column === '状态') return <StatusTag value={value} />;
  if (column === '误报率' || column === '平均延时') return <span className="taf-rules-warn">{String(value)}</span>;
  return String(value);
};

const ruleColumnWidths: Record<string, number> = {
  规则ID: 82,
  规则名称: 90,
  类型: 44,
  严重级别: 48,
  MITRE阶段: 72,
  状态: 44,
  版本: 42,
  命中数: 54,
  误报率: 48,
  平均延时: 52,
};

const rowKey = (row: SnapshotRow) => String(row['规则ID'] ?? row['规则名称'] ?? JSON.stringify(row));

const buildVisualRuleRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ 规则ID: 'C2_Tunnel_v3', 规则名称: 'C2 隧道通信检测', 类型: '流量', 严重级别: '高', MITRE阶段: '指挥与控制', 状态: '启用', 版本: 'v3.0', 命中数: '1.3K', 误报率: '0.38%', 平均延时: '18 ms' }];
  if (source.length >= 21) return source;
  return Array.from({ length: 21 }, (_, index) => index < source.length ? source[index] : {
    ...source[index % source.length],
    规则ID: `${String(source[index % source.length]['规则ID'] ?? 'RULE')}-SIM${String(Math.floor(index / source.length) + 1).padStart(2, '0')}`,
  });
};

const createRuleAction = (title: string, target: string): RuleAction => {
  return {
    title,
    target,
    actionId: ruleActionSlug(title),
    endpoint: '/v1/rules/{id}/actions',
    auditEvent: 'RULE_WORKBENCH_ACTION',
  };
};

const ruleActionSlug = (title: string) => {
  if (title.includes('回滚')) return 'rule-rollback';
  if (title.includes('发布') || title.includes('启用')) return 'rule-publish';
  if (title.includes('停用')) return 'rule-disable';
  if (title.includes('删除') || title.includes('移除')) return 'rule-remove';
  if (title.includes('下载')) return 'sample-download';
  if (title.includes('回放')) return 'sample-replay';
  if (title.includes('导出')) return 'rule-export';
  if (title.includes('导入')) return 'rule-import';
  if (title.includes('查看') || title.includes('打开') || title.includes('定位')) return 'rule-inspect';
  if (title.includes('生成')) return 'rule-generate';
  if (title.includes('添加')) return 'rule-add';
  if (title.includes('校验') || title.includes('验证')) return 'rule-validate';
  if (title.includes('标记') || title.includes('切换')) return 'rule-mark';
  return 'rule-configure';
};

const apiRuleType = (value: string) => value === '流量' ? 'threshold' : value === '文件' ? 'signature' : undefined;
const apiRuleEnabled = (value: string) => value === '启用' ? true : value === '停用' ? false : undefined;

const ruleRecordToSnapshotRow = (rule: RuleRecord): SnapshotRow => ({
  规则ID: rule.rule_id,
  规则名称: rule.name,
  类型: rule.type === 'signature' ? '文件' : '流量',
  严重级别: ({ critical: '严重', high: '高', medium: '中', low: '低' } as Record<string, string>)[rule.severity] ?? rule.severity,
  MITRE阶段: rule.labels?.find((label) => /^TA\d+/i.test(label)) ?? '—',
  状态: ruleLifecycleLabel(rule.status),
  版本: `v${rule.version}.0`,
  命中数: '—',
  误报率: '—',
  平均延时: '—',
  最近状态变更: formatRuleTime(rule.updated_at),
  状态操作人: rule.updated_by || rule.created_by || 'system',
});

export function ruleLifecycleLabel(status: string): '草稿' | '待审' | '灰度' | '启用' | '停用' | '回滚' {
  const normalized = status.trim().toLowerCase();
  if (['rollback', '回滚'].some((value) => normalized.includes(value))) return '回滚';
  if (['disabled', 'inactive', 'deprecated', 'archived', '停用', '禁用'].some((value) => normalized.includes(value))) return '停用';
  if (['gray', 'canary', '灰度'].some((value) => normalized.includes(value))) return '灰度';
  if (['pending', 'review', '待审'].some((value) => normalized.includes(value))) return '待审';
  if (['active', 'enabled', '启用'].some((value) => normalized.includes(value))) return '启用';
  return '草稿';
}

const formatRuleTime = (value: string) => {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value || '—';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(timestamp).replace(/\//g, '-');
};

const workbenchItems = (workbench: RuleWorkbench | undefined, category: string) => workbench?.items?.[category] ?? [];
const firstWorkbenchItem = (workbench: RuleWorkbench | undefined, category: string): Record<string, unknown> => workbenchItems(workbench, category)[0] ?? {};
const textValue = (value: unknown) => value == null ? '' : String(value);
const stringArray = (value: unknown) => Array.isArray(value) ? value.map(textValue) : [];
const numberArray = (value: unknown) => Array.isArray(value) ? value.map(Number).filter(Number.isFinite) : [];

const ruleDefinitionConditions = (definition: Record<string, unknown>, ruleConditionsPayload?: Record<string, unknown>): string[][] => {
  if (Array.isArray(definition.conditions)) {
    const rows = definition.conditions.map((item) => {
      const row = item && typeof item === 'object' ? item as Record<string, unknown> : {};
      return [textValue(row.field), textValue(row.operator), textValue(row.value)];
    }).filter((row) => row.every(Boolean));
    if (rows.length) return rows;
  }
  const rows = Object.entries(ruleConditionsPayload ?? {}).slice(0, 4).map(([field, value]) => [field, '等于', textValue(value)]);
  return rows.length ? rows : [['未配置字段', '等于', '未配置']];
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '92.0%' : '0',
  delta: 'API',
  status: 'info',
});
