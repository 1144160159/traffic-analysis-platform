import {
  ApartmentOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileProtectOutlined,
  FlagOutlined,
  MoreOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  TeamOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { MouseEvent, ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { DataQualityDonutChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { submitCampaignAction, type CampaignActionId, type CampaignActionResult } from '@/services/campaignActionApi';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const phaseNodeSpecs = [
  { phase: '初始访问', fallbackCount: 3, tone: 'info', aliases: ['初始访问', '信息收集'], Icon: SafetyCertificateOutlined },
  { phase: '执行', fallbackCount: 5, tone: 'warn', aliases: ['执行', '执行活动'], Icon: FlagOutlined },
  { phase: '持久化', fallbackCount: 4, tone: 'warn', aliases: ['持久化'], Icon: FileProtectOutlined },
  { phase: '横向移动', fallbackCount: 4, tone: 'warn', aliases: ['横向移动'], Icon: BranchesOutlined },
  { phase: '外联', fallbackCount: 4, tone: 'ok', aliases: ['外联', '外联通信', 'C2 通信'], Icon: NodeIndexOutlined },
  { phase: '数据外传', fallbackCount: 2, tone: 'risk', aliases: ['数据外传'], Icon: DownloadOutlined },
  { phase: '影响达成', fallbackCount: 1, tone: 'info', aliases: ['影响达成'], Icon: CheckCircleOutlined },
];

const impactItems = [
  ['资产', '42 台', ApartmentOutlined],
  ['账号', '18 个', TeamOutlined],
  ['服务', '27 个', SafetyCertificateOutlined],
  ['业务系统', '6 个', BranchesOutlined],
  ['部门', '3 个', UserSwitchOutlined],
  ['园区', '1 个', NodeIndexOutlined],
];

const stateFlow = [
  ['新建', '06-19 09:15'],
  ['调查中', '06-19 10:02'],
  ['处置中', '06-19 18:33'],
  ['活跃中', '06-20 03:22'],
];

type RiskCounts = {
  high: number;
  medium: number;
  low: number;
};

type CampaignFilters = {
  risk: string;
  status: string;
  phase: string;
  keyword: string;
};

type CampaignActionContext = {
  title: string;
  result: CampaignActionResult;
};

const emptyCampaignFilters: CampaignFilters = { risk: '全部', status: '全部', phase: '全部', keyword: '' };

const campaignOverlays: OverlayContract[] = [
  {
    id: 'drawer-campaign-detail',
    title: '战役详情抽屉',
    kind: 'Drawer',
    actionLabel: '战役详情',
    description: '展示战役阶段、关联告警、影响范围、证据完整度和处置进度。',
    audit: '记录战役详情下钻、筛选上下文和操作者 trace。',
  },
];

export function CampaignWorkbenchPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const visualBreakdownMode = isVisualBreakdownMode();
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const [filterDraft, setFilterDraft] = useState<CampaignFilters>(emptyCampaignFilters);
  const [appliedFilters, setAppliedFilters] = useState<CampaignFilters>(emptyCampaignFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(visualBreakdownMode ? 10 : 8);
  const [actionContext, setActionContext] = useState<CampaignActionContext>();
  useEffect(() => {
    setPage(1);
    setPageSize(visualBreakdownMode ? 10 : 8);
  }, [visualBreakdownMode]);
  const requestPage = page;
  const requestPageSize = pageSize;
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, requestPage, requestPageSize, appliedFilters],
    queryFn: () => fetchPageSnapshot(route.id, { page: requestPage, pageSize: requestPageSize, campaignFilters: appliedFilters }),
    refetchInterval: visualBreakdownMode ? false : 30_000,
    refetchIntervalInBackground: true,
  });

  const apiRows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const campaignTotal = data?.total ?? apiRows.length;
  const rows = useMemo(
    () => visualBreakdownMode ? buildCampaignSimulationRows(apiRows, campaignTotal) : apiRows,
    [apiRows, campaignTotal, visualBreakdownMode],
  );
  const filteredRows = useMemo(
    () => visualBreakdownMode ? filterCampaignRows(rows, appliedFilters) : rows,
    [rows, appliedFilters, visualBreakdownMode],
  );
  const selectedRow = useMemo(() => {
    if (!filteredRows.length) return undefined;
    return filteredRows.find((row) => rowKey(row) === selectedRowKey) ?? filteredRows[0];
  }, [filteredRows, selectedRowKey]);

  const actionMutation = useMutation({
    mutationFn: submitCampaignAction,
    onSuccess: (result) => message.success(`模拟作业已记录：${result.jobId}`),
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '战役操作提交失败'),
  });
  const selectedCampaignId = text(selectedRow, '战役名称', 'APT-20260619-RedLync');
  const executeAction = async (
    actionId: CampaignActionId,
    title: string,
    options?: { targetId?: string; target?: string; navigateTo?: string; metadata?: Record<string, unknown> },
  ) => {
    const result = await actionMutation.mutateAsync({
      actionId,
      campaignId: options?.targetId ?? selectedCampaignId,
      target: options?.target ?? title,
      metadata: options?.metadata,
    });
    setActionContext({ title, result });
    if (options?.navigateTo) navigate(options.navigateTo);
    return result;
  };
  const exportRows = async () => {
	await executeAction('campaign-export', '模拟导出当前页', { target: `当前页 ${filteredRows.length} 条` });
    const blob = new Blob([JSON.stringify(filteredRows, null, 2)], { type: 'application/json;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `campaigns-${new Date().toISOString().slice(0, 10)}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: campaignColumnWidth(column),
    ellipsis: true,
    render: (value, record) => renderCampaignCell(column, value, record, (event) => {
      event.stopPropagation();
      void executeAction('campaign-context-action', `查看 ${text(record, '战役名称', '战役')} 操作`, {
        targetId: rowKey(record),
        target: '表格行操作',
      });
    }),
  }));
  return (
    <div className={`taf-page taf-campaign-workbench${visualBreakdownMode ? ' is-visual-target' : ''}`}>
      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/campaigns、APISIX 路由、ClickHouse campaigns 表或后端服务。'}
          action={
            <Button size="small" danger onClick={() => void refetch()}>
              重试
            </Button>
          }
        />
      )}

      <div className="taf-campaign-grid">
        <main className="taf-campaign-main">
          <section className="taf-campaign-overview">
            <div className="taf-campaign-overview__head">
              <h1>{route.page.title}</h1>
              {!visualBreakdownMode && (
                <Space size={6}>
                  <Tooltip title="刷新战役聚合">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                  <OverlayContractHost overlays={campaignOverlays} compact />
                </Space>
              )}
            </div>
            <div className="taf-campaign-overview__content">
              <div className="taf-campaign-kpis">
                {(data?.metrics ?? []).slice(0, 6).map((metric) => (
                  <MetricTile key={metric.label} metric={metric} />
                ))}
              </div>
              <RiskDistribution rows={rows} visualBreakdownMode={visualBreakdownMode} />
            </div>
          </section>

          <div className="taf-campaign-body">
            <WorkPanel
              title={`${route.page.tableTitle}（共 ${campaignTotal || 0} 个）`}
              className="taf-campaign-list-panel"
              extra={
                <Space>
                  {visualBreakdownMode ? (
                    <>
                      <Button size="small" icon={<DownloadOutlined />} loading={actionMutation.isPending} onClick={() => void exportRows()}>导出当前页</Button>
                      <Button size="small" icon={<SettingOutlined />} aria-label="模拟列表设置" onClick={() => void executeAction('campaign-list-settings', '模拟列表设置')} />
                    </>
                  ) : (
                    <>
                      <Button size="small" icon={<FlagOutlined />} onClick={() => void executeAction('campaign-assign-owner', '模拟指派负责人')}>模拟指派负责人</Button>
                      <Button size="small" icon={<CheckCircleOutlined />} onClick={() => void executeAction('campaign-status-change', '模拟变更状态')}>模拟变更状态</Button>
                      <Button size="small" icon={<MoreOutlined />} aria-label="更多模拟战役操作" onClick={() => void executeAction('campaign-context-action', '更多模拟战役操作')} />
                    </>
                  )}
                </Space>
              }
            >
              <CampaignFilter
                value={filterDraft}
                onChange={setFilterDraft}
                onReset={() => {
                  setFilterDraft(emptyCampaignFilters);
                  setAppliedFilters(emptyCampaignFilters);
                  setPage(1);
                  message.info('筛选条件已重置');
                }}
                onSubmit={() => {
                  setAppliedFilters(filterDraft);
                  setPage(1);
                  message.success('已提交服务端查询');
                }}
              />
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={filteredRows}
                tableLayout="fixed"
                pagination={{
                  current: page,
                  pageSize,
                  total: visualBreakdownMode ? filteredRows.length : campaignTotal,
                  size: 'small',
                  showSizeChanger: true,
                  pageSizeOptions: [5, 8, 10, 20],
                  showTotal: (total) => `共 ${total} 条`,
                  onChange: (nextPage, nextPageSize) => {
                    setPage(nextPageSize === pageSize ? nextPage : 1);
                    setPageSize(nextPageSize);
                  },
                }}
                rowClassName={(record) => (selectedRow && rowKey(record) === rowKey(selectedRow) ? 'is-selected' : '')}
                onRow={(record) => ({
                  onClick: () => setSelectedRowKey(rowKey(record)),
                })}
              />
            </WorkPanel>

            <WorkPanel title="战役阶段视图（ATT&CK）" className="taf-campaign-attack-panel">
              <AttackPhaseView
                selectedRow={selectedRow}
                rows={rows}
                timeline={data?.timeline ?? []}
                visualBreakdownMode={visualBreakdownMode}
                onInspectPhase={(phase) => void executeAction('campaign-phase-inspect', `查看${phase}阶段`, { target: phase, metadata: { phase } })}
              />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-campaign-rail">
          <CampaignSummary
            selectedRow={selectedRow}
            onViewDetail={() => void executeAction('campaign-detail-view', '查看战役详情', { navigateTo: `/campaigns/${encodeURIComponent(selectedCampaignId)}` })}
          />
          <ImpactScope
            selectedRow={selectedRow}
            onInspect={(scope) => void executeAction('campaign-impact-inspect', `查看${scope}影响范围`, { target: scope, metadata: { scope } })}
            onViewAssets={() => void executeAction('campaign-impact-inspect', '查看资产列表', { target: '资产列表', navigateTo: `/assets?campaign=${encodeURIComponent(selectedCampaignId)}` })}
          />
          <EvidenceCompleteness
            evidence={data?.evidence ?? []}
            visualBreakdownMode={visualBreakdownMode}
            onViewEvidence={() => void executeAction('campaign-evidence-view', '查看证据中心')}
          />
          <StateTransition
            selectedRow={selectedRow}
            actions={route.page.actions}
            pending={actionMutation.isPending}
            onAction={(action) => void handleCampaignAction(action, selectedCampaignId, executeAction)}
          />
        </aside>
      </div>
      <Drawer
        className="taf-campaign-action-drawer"
        title={actionContext?.title ?? '战役操作'}
        width="min(520px, calc(100dvw - 40px))"
        open={Boolean(actionContext)}
        onClose={() => setActionContext(undefined)}
      >
        {actionContext && <CampaignActionReceipt context={actionContext} />}
      </Drawer>
    </div>
  );
}

function CampaignFilter({ value, onChange, onReset, onSubmit }: { value: CampaignFilters; onChange: (value: CampaignFilters) => void; onReset: () => void; onSubmit: () => void }) {
  return (
    <div className="taf-campaign-filter">
      <label>
        <span>风险等级</span>
        <Select size="small" value={value.risk} onChange={(risk) => onChange({ ...value, risk })} options={[{ value: '全部' }, { value: '高风险' }, { value: '中风险' }, { value: '低风险' }]} />
      </label>
      <label>
        <span>状态</span>
        <Select size="small" value={value.status} onChange={(status) => onChange({ ...value, status })} options={[{ value: '全部' }, { value: '活跃中' }, { value: '调查中' }, { value: '已结束' }]} />
      </label>
      <label>
        <span>阶段</span>
        <Select size="small" value={value.phase} onChange={(phase) => onChange({ ...value, phase })} options={[{ value: '全部' }, { value: '执行' }, { value: '横向移动' }, { value: '数据外传' }]} />
      </label>
      <label>
        <span>战役名称 / 关键字</span>
        <Input size="small" value={value.keyword} placeholder="战役名称 / 关键字" allowClear onChange={(event) => onChange({ ...value, keyword: event.target.value })} onPressEnter={onSubmit} />
      </label>
      <Button size="small" onClick={onReset}>重置</Button>
      <Button size="small" type="primary" onClick={onSubmit}>查询</Button>
    </div>
  );
}

function RiskDistribution({
  rows,
  visualBreakdownMode,
}: {
  rows: SnapshotRow[];
  visualBreakdownMode: boolean;
}) {
  const counts: RiskCounts = visualBreakdownMode ? { high: 18, medium: 24, low: 16 } : campaignRiskCounts(rows);
  const denominator = visualBreakdownMode ? 58 : Math.max(rows.length, counts.high + counts.medium + counts.low, 1);
  const items = [
    ['高风险', formatRiskShare(counts.high, denominator), 'risk'],
    ['中风险', formatRiskShare(counts.medium, denominator), 'warn'],
    ['低风险', formatRiskShare(counts.low, denominator), 'ok'],
  ];
  return (
    <div
      className="taf-campaign-risk-distribution"
      data-chart-values={`${counts.high},${counts.medium},${counts.low}`}
      data-chart-total={counts.high + counts.medium + counts.low}
    >
      <h2>{visualBreakdownMode ? '风险分布' : '当前页风险分布'}</h2>
      <div>
        <DataQualityDonutChart
          ariaLabel="战役风险分布动态图"
          className="taf-campaign-risk-chart"
          rows={[
            { label: '高风险', value: counts.high, color: '#ff4d4f' },
            { label: '中风险', value: counts.medium, color: '#ffb020' },
            { label: '低风险', value: counts.low, color: '#65d152' },
          ]}
        />
        <ul>
          {items.map(([label, value, tone]) => (
            <li key={label} className={`is-${tone}`}>
              <i />
              <span>{label}</span>
              <strong>{value}</strong>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function AttackPhaseView({
  selectedRow,
  rows,
  timeline,
  visualBreakdownMode,
  onInspectPhase,
}: {
  selectedRow?: SnapshotRow;
  rows: SnapshotRow[];
  timeline: PageSnapshot['timeline'];
  visualBreakdownMode: boolean;
  onInspectPhase: (phase: string) => void;
}) {
  const campaignId = text(selectedRow, '战役名称', 'APT-20260619-RedLync');
  const phaseNodes = buildPhaseNodes(rows, visualBreakdownMode);
  return (
    <div className="taf-campaign-attack">
      <div className="taf-campaign-phase-line">
        {phaseNodes.map(({ phase, count, tone, Icon }, index) => (
          <div key={phase} className={`taf-campaign-phase is-${tone}`}>
            <span>{phase}</span>
            <i>
              <Icon />
              {index < phaseNodes.length - 1 && <b />}
            </i>
            <strong>{count}</strong>
          </div>
        ))}
      </div>
      <div className="taf-campaign-radar">
        <div className="taf-campaign-center">
          <FlagOutlined />
          <strong title={campaignId}>{campaignId}</strong>
          <span>{text(selectedRow, '风险等级', '高风险')} / {text(selectedRow, '状态', '活跃中')}</span>
        </div>
        {phaseNodes.slice(0, 6).map(({ phase, count, tone, Icon }, index) => (
          <button key={phase} type="button" className={`taf-campaign-node node-${index} is-${tone}`} onClick={() => onInspectPhase(phase)}>
            <Icon />
            <span>{phase}</span>
            <strong>{count}</strong>
            <small>告警</small>
          </button>
        ))}
      </div>
      <div className="taf-campaign-timeline">
        {(timeline.length ? timeline : defaultTimeline()).slice(0, 5).map((item) => (
          <div key={item.title} className={`is-${item.status}`}>
            <i />
            <strong>{item.title}</strong>
            <span>{item.description}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function CampaignSummary({ selectedRow, onViewDetail }: { selectedRow?: SnapshotRow; onViewDetail: () => void }) {
  return (
    <WorkPanel title="当前选中战役" extra={<Button size="small" type="link" onClick={onViewDetail}>查看详情</Button>}>
      <div className="taf-campaign-summary">
        <div>
          <strong title={text(selectedRow, '战役名称', 'APT-20260619-RedLync')}>
            {text(selectedRow, '战役名称', 'APT-20260619-RedLync')}
          </strong>
          <StatusTag value={text(selectedRow, '风险等级', '高风险')} />
          <StatusTag value={text(selectedRow, '状态', '活跃中')} />
        </div>
        <dl>
          <dt>阶段</dt>
          <dd>{text(selectedRow, '阶段', '横向移动')}</dd>
          <dt>影响资产</dt>
          <dd>{text(selectedRow, '影响资产', '42')}</dd>
          <dt>告警数量</dt>
          <dd>{text(selectedRow, '告警数', '234')}</dd>
          <dt>负责人</dt>
          <dd>sec_analyst</dd>
          <dt>战役来源</dt>
          <dd>规则 / 行为检测 / 威胁情报</dd>
          <dt>攻击者画像</dt>
          <dd>RedLync APT 组织（高置信）</dd>
        </dl>
      </div>
    </WorkPanel>
  );
}

function ImpactScope({ selectedRow, onInspect, onViewAssets }: { selectedRow?: SnapshotRow; onInspect: (scope: string) => void; onViewAssets: () => void }) {
  const assetCount = Number(text(selectedRow, '影响资产', '42').replace(/[^\d.]/g, '')) || 42;
  return (
    <WorkPanel title="影响范围" extra={<Button size="small" type="link" onClick={onViewAssets}>查看资产列表</Button>}>
      <div className="taf-campaign-impact">
        {impactItems.map(([label, value, Icon], index) => (
          <button key={label as string} type="button" onClick={() => onInspect(label as string)}>
            <Icon />
            <span>{label as string}</span>
            <strong>{index === 0 ? `${assetCount} 台` : value as string}</strong>
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function EvidenceCompleteness({ evidence, visualBreakdownMode, onViewEvidence }: { evidence: PageSnapshot['evidence']; visualBreakdownMode: boolean; onViewEvidence: () => void }) {
  const items = evidence.length
    ? evidence
	: visualBreakdownMode ? [
        { label: '告警', value: '234 / 312', status: 'ok' as const },
        { label: 'PCAP / Session', value: '86 / 128', status: 'warn' as const },
        { label: '日志', value: '1,432 / 2,150', status: 'ok' as const },
        { label: '图谱路径', value: '12 / 18', status: 'warn' as const },
        { label: '处置记录', value: '8 / 10', status: 'risk' as const },
      ] : [];
  const percent = evidence.length ? evidenceCompletionPercent(items) : visualBreakdownMode ? 78 : undefined;
  return (
    <WorkPanel title="证据完整度" extra={<Button size="small" type="link" onClick={onViewEvidence}>查看证据中心</Button>}>
      <div className="taf-campaign-evidence">
        <div className="taf-campaign-evidence-chart">
          <DataQualityDonutChart
            ariaLabel="战役证据完整度动态图"
            rows={percent === undefined ? [] : [
              { label: '已收集', value: percent, color: '#36d66b' },
              { label: '待补齐', value: Math.max(0, 100 - percent), color: 'rgba(56,151,201,0.18)' },
            ]}
          />
          <strong>{percent === undefined ? '待接入' : `${percent}%`}</strong>
          <span>{percent === undefined ? '接口字段' : '已收集'}</span>
        </div>
        <div>
          {items.slice(0, 5).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <b>{item.label}</b>
              <i><em style={{ width: evidenceWidth(item.value) }} /></i>
              <strong>{item.value}</strong>
            </span>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function StateTransition({ selectedRow, actions, pending, onAction }: { selectedRow?: SnapshotRow; actions: string[]; pending: boolean; onAction: (action: string) => void }) {
  const campaignActions = ['查看详情', '模拟变更状态', '模拟生成报告', '下钻攻击链', '跳转资产图谱', '模拟 SOAR 处置'];
  const visibleActions = campaignActions.length ? campaignActions : actions;
  return (
    <WorkPanel title="状态流转">
      <div className="taf-campaign-state-flow">
        {stateFlow.map(([state, time]) => (
          <span key={state} className={state === text(selectedRow, '状态', '活跃中') ? 'is-current' : ''}>
            <strong>{state}</strong>
            <small>{time}</small>
          </span>
        ))}
      </div>
      <h3 className="taf-campaign-actions-title">战役操作</h3>
      <div className="taf-campaign-actions">
        {visibleActions.map((action) => (
          <Button key={action} size="small" loading={pending} onClick={() => onAction(action)} icon={action.includes('报告') ? <FileProtectOutlined /> : action.includes('攻击链') ? <BranchesOutlined /> : <EyeOutlined />}>
            {action}
          </Button>
        ))}
      </div>
    </WorkPanel>
  );
}

const defaultTimeline = (): PageSnapshot['timeline'] => [
  { title: '初始访问', description: '外部入口触发多条异常连接，已关联首个告警簇。', status: 'info' },
  { title: '执行', description: '执行阶段出现脚本投递与异常进程链。', status: 'warn' },
  { title: '横向移动', description: '跨 VLAN 访问核心业务服务器，建议下钻攻击链。', status: 'risk' },
  { title: '外联', description: '低频 C2 心跳与 JA3 指纹持续命中。', status: 'warn' },
  { title: '数据外传', description: '外传证据仍需补齐 PCAP 与日志上下文。', status: 'risk' },
];

const renderCampaignCell = (column: string, value: unknown, record: SnapshotRow, onAction: (event: MouseEvent<HTMLElement>) => void): ReactNode => {
  if (column === '战役名称') {
    const id = String(value ?? '');
    return (
      <span className="taf-campaign-id" title={id}>
        {id}
        <EyeOutlined />
      </span>
    );
  }
  if (column === '风险等级' || column === '状态') return <StatusTag value={value} />;
  if (column === '操作') return <Button size="small" type="text" aria-label={`打开${text(record, '战役名称', '战役')}操作`} icon={<MoreOutlined />} onClick={onAction} />;
  if (column === '告警数') return <strong className="taf-campaign-alert-count">{String(value || text(record, '告警数', '-'))}</strong>;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record['战役名称'] ?? JSON.stringify(record));

const campaignColumnWidth = (column: string) => {
  if (column === '战役名称') return 156;
  if (column === '风险等级') return 78;
  if (column === '影响资产' || column === '告警数') return 62;
  if (column === '首次发现' || column === '最近活动') return 72;
  if (column === '操作') return 42;
  return 86;
};

const buildCampaignSimulationRows = (apiRows: SnapshotRow[], total: number) => {
  const seedRows = apiRows.length ? apiRows : campaignFallbackRows;
  const targetLength = Math.max(seedRows.length, Math.min(Math.max(total, seedRows.length), 60));
  if (targetLength <= seedRows.length) return seedRows;
  const risks = ['高风险', '中风险', '低风险'];
  const phases = ['横向移动', '数据外传', '执行', '初始访问', '外联通信'];
  const statuses = ['活跃中', '调查中', '处置中', '已结束'];
  return Array.from({ length: targetLength }, (_, index) => {
    if (index < seedRows.length) return seedRows[index];
    const seed = seedRows[index % seedRows.length];
    return {
      ...seed,
      战役名称: `${text(seed, '战役名称', 'CAMPAIGN').replace(/-SIM-\d+$/, '')}-SIM-${String(index + 1).padStart(2, '0')}`,
      阶段: phases[index % phases.length],
      风险等级: risks[index % risks.length],
      状态: statuses[index % statuses.length],
      影响资产: String(8 + ((index * 7) % 51)),
      告警数: String(24 + ((index * 31) % 240)),
    };
  });
};

const campaignFallbackRows: SnapshotRow[] = [
  { 战役名称: 'APT-20260619-RedLync', 阶段: '横向移动', 风险等级: '高风险', 影响资产: '42', 告警数: '234', 首次发现: '06-19 09:12:45', 最近活动: '06-20 03:22:11', 状态: '活跃中', 操作: '查看' },
  { 战役名称: 'DataExfil-20250618-Office', 阶段: '数据外传', 风险等级: '高风险', 影响资产: '31', 告警数: '187', 首次发现: '06-18 14:32:11', 最近活动: '06-20 01:45:33', 状态: '活跃中', 操作: '查看' },
  { 战役名称: 'Ransom-20260617-LocalSM', 阶段: '执行', 风险等级: '高风险', 影响资产: '18', 告警数: '96', 首次发现: '06-17 21:18:04', 最近活动: '06-19 23:12:17', 状态: '活跃中', 操作: '查看' },
  { 战役名称: 'Recon-20260616-ScanWave', 阶段: '信息收集', 风险等级: '中风险', 影响资产: '56', 告警数: '145', 首次发现: '06-16 11:07:52', 最近活动: '06-18 17:33:21', 状态: '调查中', 操作: '查看' },
  { 战役名称: 'Lateral-20260615-PSExec', 阶段: '横向移动', 风险等级: '中风险', 影响资产: '27', 告警数: '102', 首次发现: '06-15 19:43:18', 最近活动: '06-18 10:22:05', 状态: '调查中', 操作: '查看' },
  { 战役名称: 'BruteForce-20250614-SSH', 阶段: '初始访问', 风险等级: '中风险', 影响资产: '12', 告警数: '78', 首次发现: '06-14 22:14:36', 最近活动: '06-17 08:11:54', 状态: '调查中', 操作: '查看' },
  { 战役名称: 'DNS-Tunnel-20260614-lodine', 阶段: '外联通信', 风险等级: '低风险', 影响资产: '8', 告警数: '34', 首次发现: '06-14 15:36:21', 最近活动: '06-16 20:43:33', 状态: '已结束', 操作: '查看' },
  { 战役名称: 'MalDoc-20260613-Macro', 阶段: '执行', 风险等级: '低风险', 影响资产: '16', 告警数: '56', 首次发现: '06-13 10:11:09', 最近活动: '06-15 16:22:40', 状态: '已结束', 操作: '查看' },
];

const filterCampaignRows = (rows: SnapshotRow[], filters: CampaignFilters) => {
  const keyword = filters.keyword.trim().toLowerCase();
  return rows.filter((row) => {
    if (filters.risk !== '全部' && text(row, '风险等级', '') !== filters.risk) return false;
    if (filters.status !== '全部' && text(row, '状态', '') !== filters.status) return false;
    if (filters.phase !== '全部' && !text(row, '阶段', '').includes(filters.phase)) return false;
    if (keyword && !Object.values(row).some((value) => String(value ?? '').toLowerCase().includes(keyword))) return false;
    return true;
  });
};

const handleCampaignAction = async (
  action: string,
  campaignId: string,
  executeAction: (
    actionId: CampaignActionId,
    title: string,
    options?: { targetId?: string; target?: string; navigateTo?: string; metadata?: Record<string, unknown> },
  ) => Promise<CampaignActionResult>,
) => {
  const encodedId = encodeURIComponent(campaignId);
  if (action === '查看详情') return executeAction('campaign-detail-view', action, { navigateTo: `/campaigns/${encodedId}` });
  if (action === '模拟变更状态') return executeAction('campaign-status-change', action, { metadata: { next_status: 'investigating' } });
  if (action === '模拟生成报告') return executeAction('campaign-report-generate', action, { target: '模拟战役复盘报告' });
  if (action === '下钻攻击链') return executeAction('campaign-attack-chain-view', action, { navigateTo: `/attack-chains?campaign=${encodedId}` });
  if (action === '跳转资产图谱') return executeAction('campaign-graph-view', action, { navigateTo: `/graph?campaign=${encodedId}` });
  if (action === '模拟 SOAR 处置') return executeAction('campaign-soar-response', action, { navigateTo: `/playbooks?campaign=${encodedId}`, metadata: { dry_run: true } });
  return executeAction('campaign-context-action', action);
};

function CampaignActionReceipt({ context }: { context: CampaignActionContext }) {
  const { result } = context;
  return (
    <div className="taf-campaign-action-receipt">
      <Alert type="success" showIcon message="模拟作业已记录" description="本次 dry-run 未修改业务状态；模拟作业已写入 campaign_action_jobs，审计已写入 audit_logs。" />
      <dl>
        <dt>操作</dt><dd>{context.title}</dd>
        <dt>任务编号</dt><dd>{result.jobId}</dd>
        <dt>接口</dt><dd>{result.endpoint}</dd>
        <dt>审计事件</dt><dd>{result.auditEvent}</dd>
        <dt>作业状态</dt><dd>{result.jobStatus}</dd>
        <dt>审计状态</dt><dd>{result.status}</dd>
        <dt>请求体</dt><dd><pre>{JSON.stringify(result.requestBody, null, 2)}</pre></dd>
      </dl>
    </div>
  );
}

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const buildPhaseNodes = (rows: SnapshotRow[], visualBreakdownMode: boolean) => {
  if (visualBreakdownMode || !rows.length) {
    return phaseNodeSpecs.map((node) => ({ ...node, count: node.fallbackCount }));
  }

  return phaseNodeSpecs.map((node) => {
    const count = rows.filter((row) => {
      const phase = text(row, '阶段', '');
      return node.aliases.some((alias) => phase.includes(alias));
    }).length;
    return { ...node, count: count || node.fallbackCount };
  });
};

const campaignRiskCounts = (rows: SnapshotRow[]): RiskCounts => rows.reduce<RiskCounts>(
  (acc, row) => {
    const risk = text(row, '风险等级', '');
    if (risk.includes('高')) acc.high += 1;
    else if (risk.includes('中')) acc.medium += 1;
    else if (risk.includes('低')) acc.low += 1;
    return acc;
  },
  { high: 0, medium: 0, low: 0 },
);

const formatRiskShare = (count: number, denominator: number) => `${count} (${((count / denominator) * 100).toFixed(1)}%)`;

const evidenceWidth = (value: string) => {
  const [done, total] = value.split('/').map((part) => Number(part.replace(/[^\d.]/g, '')));
  if (!done || !total) return '0%';
  return `${Math.max(16, Math.min(100, (done / total) * 100))}%`;
};

const evidenceCompletionPercent = (items: PageSnapshot['evidence']) => {
  const explicitEvidenceRate = items.find((item) => item.label.includes('证据完整度'));
  if (explicitEvidenceRate) {
    const percent = String(explicitEvidenceRate.value).match(/^\s*([\d.]+)\s*%\s*$/);
    if (percent) return Math.round(Number(percent[1]));
  }

  return undefined;
};
