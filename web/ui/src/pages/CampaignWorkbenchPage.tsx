import {
  ApartmentOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  DownloadOutlined,
  EyeOutlined,
  FileProtectOutlined,
  FlagOutlined,
  MoreOutlined,
  NodeIndexOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  TeamOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Drawer, Empty, Input, Popconfirm, Select, Space, Table, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { CampaignAttackGraphChart, DataQualityDonutChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { submitCampaignAction, type CampaignActionId, type CampaignActionResult } from '@/services/campaignActionApi';
import { fetchCampaignDetailSnapshot, type CampaignDetailSnapshot } from '@/services/campaignDetailApi';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const phaseNodeSpecs = [
  { phase: '初始访问', dataKey: '__phase_initial_access', fallbackCount: 3, tone: 'info', Icon: SafetyCertificateOutlined },
  { phase: '执行', dataKey: '__phase_execution', fallbackCount: 5, tone: 'warn', Icon: FlagOutlined },
  { phase: '持久化', dataKey: '__phase_persistence', fallbackCount: 4, tone: 'warn', Icon: FileProtectOutlined },
  { phase: '横向移动', dataKey: '__phase_lateral_movement', fallbackCount: 4, tone: 'warn', Icon: BranchesOutlined },
  { phase: '外联', dataKey: '__phase_command_and_control', fallbackCount: 4, tone: 'ok', Icon: NodeIndexOutlined },
  { phase: '数据外传', dataKey: '__phase_exfiltration', fallbackCount: 2, tone: 'risk', Icon: DownloadOutlined },
  { phase: '影响达成', dataKey: '__phase_impact', fallbackCount: 1, tone: 'info', Icon: CheckCircleOutlined },
];

const impactItems: Array<{ label: string; field: string; suffix: string; Icon: typeof ApartmentOutlined }> = [
  { label: '资产', field: '__entity_count', suffix: '台', Icon: ApartmentOutlined },
  { label: '账号', field: '__account_count', suffix: '个', Icon: TeamOutlined },
  { label: '服务', field: '__service_count', suffix: '个', Icon: SafetyCertificateOutlined },
  { label: '业务系统', field: '__business_system_count', suffix: '个', Icon: BranchesOutlined },
  { label: '部门', field: '__department_count', suffix: '个', Icon: UserSwitchOutlined },
  { label: '园区', field: '__campus_count', suffix: '个', Icon: NodeIndexOutlined },
];

const campaignMetricIcons = [
  NodeIndexOutlined,
  FlagOutlined,
  ApartmentOutlined,
  SafetyCertificateOutlined,
  FileProtectOutlined,
  ClockCircleOutlined,
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

export function CampaignWorkbenchPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const visualBreakdownMode = isVisualBreakdownMode();
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const [filterDraft, setFilterDraft] = useState<CampaignFilters>(emptyCampaignFilters);
  const [appliedFilters, setAppliedFilters] = useState<CampaignFilters>(emptyCampaignFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(visualBreakdownMode ? 10 : 8);
  const [actionContext, setActionContext] = useState<CampaignActionContext>();
  const [detailOpen, setDetailOpen] = useState(false);
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
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['page-snapshot', route.id] });
      message.success(`战役操作已完成：${result.jobId}`);
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '战役操作提交失败'),
  });
  const selectedCampaignId = text(selectedRow, '战役名称', '');
  const detailQuery = useQuery({
    queryKey: ['campaign-detail-drawer', selectedCampaignId],
    queryFn: () => fetchCampaignDetailSnapshot(selectedCampaignId),
    enabled: Boolean(selectedCampaignId),
    staleTime: 15_000,
  });
  const executeAction = async (
    actionId: CampaignActionId,
    title: string,
    options?: { targetId?: string; target?: string; navigateTo?: string; metadata?: Record<string, unknown>; showReceipt?: boolean },
  ) => {
    const campaignId = options?.targetId ?? selectedCampaignId;
    if (!campaignId) {
      throw new Error('当前没有可操作的战役，请调整筛选条件后重试');
    }
    const result = await actionMutation.mutateAsync({
      actionId,
      campaignId,
      target: options?.target ?? title,
      metadata: options?.metadata,
    });
    if (options?.showReceipt !== false) setActionContext({ title, result });
    if (options?.navigateTo) navigate(options.navigateTo);
    return result;
  };
  const openDetail = async () => {
    if (!selectedCampaignId) return;
    await executeAction('campaign-detail-view', '查看战役详情', { showReceipt: false });
    setDetailOpen(true);
  };
  const exportRows = async () => {
    if (!selectedCampaignId || !filteredRows.length) {
      message.info('当前查询结果为空，无可导出数据');
      return;
    }
    await executeAction('campaign-export', '导出当前页', { target: `当前页 ${filteredRows.length} 条` });
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
    render: (value, record) => renderCampaignCell(column, value, record, (action) => {
      const targetId = rowKey(record);
      setSelectedRowKey(targetId);
      if (action === 'detail') {
        void executeAction('campaign-detail-view', `查看 ${text(record, '战役名称', '战役')} 详情`, {
          targetId,
          target: '表格行查看详情',
          showReceipt: false,
        }).then(() => setDetailOpen(true)).catch(() => {});
        return;
      }
      if (action === 'status') {
        void executeAction('campaign-status-change', `变更 ${text(record, '战役名称', '战役')} 状态`, {
          targetId,
          target: '表格行状态流转',
          metadata: { next_status: nextCampaignStatus(record) },
        });
        return;
      }
      void executeAction('campaign-context-action', `查看 ${text(record, '战役名称', '战役')} 操作`, {
        targetId,
        target: '表格行更多操作',
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

      <header className="taf-campaign-titlebar">
        <h1>{route.page.title}</h1>
      </header>

      <div className="taf-campaign-grid">
        <main className="taf-campaign-main">
          <section className="taf-campaign-overview">
            <div className="taf-campaign-overview__content">
              <div className="taf-campaign-kpis">
                {(data?.metrics ?? []).slice(0, 6).map((metric, index) => {
                  const MetricIcon = campaignMetricIcons[index] ?? NodeIndexOutlined;
                  return <MetricTile key={metric.label} metric={metric} icon={<MetricIcon />} />;
                })}
              </div>
              <RiskDistribution
                rows={rows}
                riskCounts={data?.visuals?.campaigns?.riskCounts}
                visualBreakdownMode={visualBreakdownMode}
              />
            </div>
          </section>

          <div className="taf-campaign-body">
            <WorkPanel
              title={`${route.page.tableTitle}（共 ${campaignTotal || 0} 个）`}
              className="taf-campaign-list-panel"
              extra={
                <Space>
                  <Button size="small" icon={<DownloadOutlined />} loading={actionMutation.isPending} onClick={() => void exportRows()}>导出</Button>
                  <Button size="small" icon={<SettingOutlined />} aria-label="列表设置" disabled={!selectedRow} onClick={() => void executeAction('campaign-list-settings', '列表设置')} />
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
                locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无符合条件的战役" /> }}
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
                detail={detailQuery.data}
                detailLoading={detailQuery.isLoading}
                visualBreakdownMode={visualBreakdownMode}
                onInspectPhase={(phase) => void executeAction('campaign-phase-inspect', `查看${phase}阶段`, { target: phase, metadata: { phase } })}
              />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-campaign-rail">
          <CampaignSummary
            selectedRow={selectedRow}
            detail={detailQuery.data}
            onViewDetail={() => void openDetail()}
          />
          <ImpactScope
            selectedRow={selectedRow}
            disabled={!selectedRow}
            onInspect={(scope) => void executeAction('campaign-impact-inspect', `查看${scope}影响范围`, { target: scope, metadata: { scope } })}
            onViewAssets={() => void executeAction('campaign-impact-inspect', '查看资产列表', { target: '资产列表', navigateTo: `/assets?campaign=${encodeURIComponent(selectedCampaignId)}` })}
          />
          <EvidenceCompleteness
            selectedRow={selectedRow}
            detail={detailQuery.data}
            visualBreakdownMode={visualBreakdownMode}
            onViewEvidence={() => {
              if (!selectedCampaignId) return;
              void executeAction('campaign-evidence-view', '查看证据中心', {
                showReceipt: false,
                navigateTo: `/campaigns/${encodeURIComponent(selectedCampaignId)}?tab=evidence`,
              });
            }}
          />
          <StateTransition
            selectedRow={selectedRow}
            actions={route.page.actions}
            pending={actionMutation.isPending}
            onAction={(action) => {
              if (action === '查看详情') {
                void openDetail();
                return;
              }
              void handleCampaignAction(action, selectedCampaignId, selectedRow, executeAction);
            }}
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
      <Drawer
        className="taf-campaign-detail-drawer"
        title={`战役详情 · ${selectedCampaignId}`}
        width="min(900px, calc(100dvw - 220px))"
        open={detailOpen}
        onClose={() => setDetailOpen(false)}
        extra={<Button size="small" onClick={() => navigate(`/campaigns/${encodeURIComponent(selectedCampaignId)}`)}>打开完整详情</Button>}
      >
        <CampaignDetailDrawerContent
          snapshot={detailQuery.data}
          loading={detailQuery.isLoading}
          error={detailQuery.error}
          onRetry={() => void detailQuery.refetch()}
        />
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
  riskCounts,
  visualBreakdownMode,
}: {
  rows: SnapshotRow[];
  riskCounts?: RiskCounts;
  visualBreakdownMode: boolean;
}) {
  const counts: RiskCounts = visualBreakdownMode
    ? { high: 18, medium: 24, low: 16 }
    : riskCounts ?? campaignRiskCounts(rows);
  const denominator = Math.max(counts.high + counts.medium + counts.low, 1);
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
      <h2>风险分布</h2>
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
  detail,
  detailLoading,
  visualBreakdownMode,
  onInspectPhase,
}: {
  selectedRow?: SnapshotRow;
  detail?: CampaignDetailSnapshot;
  detailLoading: boolean;
  visualBreakdownMode: boolean;
  onInspectPhase: (phase: string) => void;
}) {
  if (!selectedRow) {
    return <div className="taf-campaign-attack is-empty"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择战役后查看 ATT&CK 阶段" /></div>;
  }
  const campaignId = text(selectedRow, '战役名称', '');
  const phaseNodes = buildPhaseNodes(selectedRow, visualBreakdownMode, detail);
  return (
    <div className="taf-campaign-attack" aria-busy={detailLoading}>
      <div className="taf-campaign-phase-line">
        {phaseNodes.map(({ phase, alertCount, evidenceCount, tone, Icon }, index) => (
          <div key={phase} className={`taf-campaign-phase is-${tone}`}>
            <span>{phase}</span>
            <i>
              <Icon />
              {index < phaseNodes.length - 1 && <b />}
            </i>
            <strong>{alertCount}</strong>
          </div>
        ))}
      </div>
      <CampaignAttackGraphChart
        campaignId={campaignId}
        risk={text(selectedRow, '风险等级', '高风险')}
        workflowStatus={campaignWorkflowStatus(selectedRow)}
        nodes={phaseNodes
          .filter(({ phase }) => phase !== '外联')
          .slice(0, 6)
          .map(({ phase, alertCount, evidenceCount, tone }) => ({
            name: phase,
            alertCount,
            evidenceCount,
            tone: tone === 'risk'
              ? 'risk'
              : tone === 'warn'
                ? 'warn'
                : tone === 'ok'
                  ? 'ok'
                  : 'info',
          }))}
        ariaLabel={`战役 ${campaignId} ATT&CK 阶段关联图`}
        onNodeClick={onInspectPhase}
      />
      <div className="taf-campaign-attack-legend" aria-label="ATT&CK 图谱图例">
        <span className="is-info"><i />已发现阶段</span>
        <span className="is-warn"><i />持续调查</span>
        <span className="is-risk"><i />高风险阶段</span>
        <span><b>{detailLoading
          ? '正在读取当前战役阶段聚合'
          : detail?.phaseDataBacked
            ? '告警与证据数量来自当前战役聚合'
            : '当前战役暂无可关联的阶段告警明细'}</b></span>
      </div>
    </div>
  );
}

function CampaignSummary({ selectedRow, detail, onViewDetail }: { selectedRow?: SnapshotRow; detail?: CampaignDetailSnapshot; onViewDetail: () => void }) {
  if (!selectedRow) {
    return <WorkPanel title="当前选中战役" className="taf-campaign-summary-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无选中战役" /></WorkPanel>;
  }
  return (
    <WorkPanel title="当前选中战役" className="taf-campaign-summary-panel" extra={<Button size="small" type="link" disabled={!selectedRow} onClick={onViewDetail}>查看详情</Button>}>
      <div className="taf-campaign-summary">
        <div>
          <strong title={text(selectedRow, '战役名称', '')}>
            {text(selectedRow, '战役名称', '')}
          </strong>
          <StatusTag value={text(selectedRow, '风险等级', '未知')} />
          <StatusTag value={text(selectedRow, '状态', '未设置')} />
        </div>
        <dl>
          <dt>首次发现</dt><dd>{text(selectedRow, '首次发现', '-')}</dd>
          <dt>最近活动</dt><dd>{text(selectedRow, '最近活动', '-')}</dd>
          <dt>持续时间</dt><dd>{detail?.duration || '-'}</dd>
          <dt>战役来源</dt><dd>{campaignSourceLabel(selectedRow)}</dd>
          <dt>关联告警</dt><dd>{text(selectedRow, '告警数', '0')} 条</dd>
          <dt>攻击者画像</dt><dd>{detail?.campaignType ? `${detail.campaignType}（待情报归因）` : '未提供（待威胁情报归因）'}</dd>
        </dl>
      </div>
    </WorkPanel>
  );
}

function ImpactScope({ selectedRow, disabled, onInspect, onViewAssets }: { selectedRow?: SnapshotRow; disabled: boolean; onInspect: (scope: string) => void; onViewAssets: () => void }) {
  return (
    <WorkPanel title="影响范围" className="taf-campaign-impact-panel" extra={<Button size="small" type="link" disabled={disabled} onClick={onViewAssets}>查看资产列表</Button>}>
      <div className="taf-campaign-impact">
        {impactItems.map(({ label, field, suffix, Icon }) => (
          <button key={label} type="button" disabled={disabled} onClick={() => onInspect(label)}>
            <Icon />
            <span>{label}</span>
            <strong>{text(selectedRow, field, '0')} {suffix}</strong>
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function EvidenceCompleteness({
  selectedRow,
  detail,
  visualBreakdownMode,
  onViewEvidence,
}: {
  selectedRow?: SnapshotRow;
  detail?: CampaignDetailSnapshot;
  visualBreakdownMode: boolean;
  onViewEvidence: () => void;
}) {
  const phaseEvidence = detail?.phases ?? [];
  const totalEvidence = phaseEvidence.reduce((sum, item) => sum + item.evidenceCount, 0);
  const totalAlerts = phaseEvidence.reduce((sum, item) => sum + item.alertCount, 0);
  const percentAvailable = visualBreakdownMode || Boolean(detail?.evidenceCompletenessAvailable);
  const percent = visualBreakdownMode ? 78 : (detail?.evidenceCompleteness ?? 0);
  const items = visualBreakdownMode
    ? visualCampaignEvidenceItems()
    : [
        { label: '关联告警', value: `${totalAlerts} 条`, status: totalAlerts ? 'ok' : 'warn' },
        { label: 'PCAP / Session', value: totalEvidence ? `${totalEvidence} 条` : '未提供', status: totalEvidence ? 'ok' : 'warn' },
        { label: '日志', value: '未提供', status: 'warn' },
        { label: '图谱路径', value: '未提供', status: 'warn' },
        { label: '处置记录', value: campaignWorkflowStatus(selectedRow) === '活跃中' ? '进行中' : campaignWorkflowStatus(selectedRow), status: 'info' },
      ] as PageSnapshot['evidence'];
  return (
    <WorkPanel title="证据完整度" className="taf-campaign-evidence-panel" extra={<Button size="small" type="link" disabled={!selectedRow} onClick={onViewEvidence}>查看证据中心</Button>}>
      <div className="taf-campaign-evidence">
        <div className="taf-campaign-evidence-chart">
          <DataQualityDonutChart
            ariaLabel="战役证据完整度动态图"
            rows={[
              { label: '已收集', value: percent, color: '#36d66b' },
              { label: '待补齐', value: Math.max(0, 100 - percent), color: 'rgba(56,151,201,0.18)' },
            ]}
          />
          <strong>{percentAvailable ? `${percent}%` : '--'}</strong>
          <span>{percentAvailable ? '已收集' : '口径待配置'}</span>
        </div>
        <div>
          {items.slice(0, 5).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <b>{item.label}</b>
              <i><em style={{ width: evidenceMetricWidth(item.value, item.label) }} /></i>
              <strong>{item.value}</strong>
            </span>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function StateTransition({ selectedRow, actions, pending, onAction }: { selectedRow?: SnapshotRow; actions: string[]; pending: boolean; onAction: (action: string) => void }) {
  const campaignActions = ['查看详情', '变更状态', '生成报告', '下钻攻击链', '跳转资产图谱', '进入 SOAR 处置'];
  const visibleActions = campaignActions.length ? campaignActions : actions;
  if (!selectedRow) {
    return <WorkPanel title="状态流转" className="taf-campaign-state-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择战役后执行处置" /></WorkPanel>;
  }
  const stateFlow = campaignStateFlow(selectedRow);
  return (
    <WorkPanel title="状态流转" className="taf-campaign-state-panel">
      <div className="taf-campaign-state-flow">
        {stateFlow.map(([state, time]) => (
          <span key={state} className={state === campaignWorkflowStatus(selectedRow) ? 'is-current' : ''}>
            <strong>{state}</strong>
            <small>{time}</small>
          </span>
        ))}
      </div>
      <h3 className="taf-campaign-actions-title">战役操作</h3>
      <div className="taf-campaign-actions">
        {visibleActions.map((action) => {
          const dangerous = action === '变更状态' || action === '进入 SOAR 处置';
          const button = (
            <Button key={action} size="small" disabled={!selectedRow} loading={pending} onClick={dangerous ? undefined : () => onAction(action)} icon={action.includes('报告') ? <FileProtectOutlined /> : action.includes('攻击链') ? <BranchesOutlined /> : <EyeOutlined />}>
              {action}
            </Button>
          );
          return dangerous ? (
            <Popconfirm
              key={action}
              title={`确认${action}？`}
              description={action === '变更状态'
                ? '需要 alert:write；会修改当前战役的处置状态，并写入 PostgreSQL 与 audit_logs。'
                : '需要 playbook:execute；仅进入编排页，本页不会直接执行处置动作。'}
              onConfirm={() => onAction(action)}
            >
              {button}
            </Popconfirm>
          ) : button;
        })}
      </div>
    </WorkPanel>
  );
}

const renderCampaignCell = (
  column: string,
  value: unknown,
  record: SnapshotRow,
  onAction: (action: 'detail' | 'status' | 'more') => void,
): ReactNode => {
  if (column === '战役名称') {
    const id = String(value ?? '');
    return (
      <span className="taf-campaign-id" title={id}>
        {id}
        <EyeOutlined />
      </span>
    );
  }
  if (column === '风险等级') return <StatusTag value={value} />;
  if (column === '状态') {
    const status = String(value ?? '');
    const tone = status.includes('活跃') ? 'is-active' : status.includes('调查') || status.includes('处置') ? 'is-watch' : 'is-closed';
    return <span className={`taf-campaign-row-status ${tone}`}><i />{status}</span>;
  }
  if (column === '操作') {
    const campaignName = text(record, '战役名称', '战役');
    return (
      <Space size={0} className="taf-campaign-row-actions">
        <Button
          size="small"
          type="text"
          aria-label={`查看${campaignName}详情`}
          icon={<EyeOutlined />}
          onClick={(event) => {
            event.stopPropagation();
            onAction('detail');
          }}
        />
        <Popconfirm
          title="确认变更战役处置状态？"
          description="该操作会修改当前战役状态并写入审计日志。"
          onConfirm={() => onAction('status')}
        >
          <Button
            size="small"
            type="text"
            aria-label={`变更${campaignName}状态`}
            icon={<CheckCircleOutlined />}
            onClick={(event) => event.stopPropagation()}
          />
        </Popconfirm>
        <Button
          size="small"
          type="text"
          aria-label={`打开${campaignName}更多操作`}
          icon={<MoreOutlined />}
          onClick={(event) => {
            event.stopPropagation();
            onAction('more');
          }}
        />
      </Space>
    );
  }
  if (column === '告警数') return <strong className="taf-campaign-alert-count">{String(value || text(record, '告警数', '-'))}</strong>;
  return String(value ?? '');
};

const rowKey = (record: SnapshotRow) => String(record['战役名称'] ?? JSON.stringify(record));

const campaignColumnWidth = (column: string) => {
  if (column === '战役名称') return 116;
  if (column === '阶段') return 54;
  if (column === '风险等级') return 54;
  if (column === '影响资产' || column === '告警数') return 42;
  if (column === '首次发现' || column === '最近活动') return 58;
  if (column === '状态') return 54;
  if (column === '操作') return 58;
  return 58;
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
  selectedRow: SnapshotRow | undefined,
  executeAction: (
    actionId: CampaignActionId,
    title: string,
    options?: { targetId?: string; target?: string; navigateTo?: string; metadata?: Record<string, unknown> },
  ) => Promise<CampaignActionResult>,
) => {
  if (!campaignId) return Promise.reject(new Error('当前没有可操作的战役'));
  const encodedId = encodeURIComponent(campaignId);
  if (action === '查看详情') return executeAction('campaign-detail-view', action, { navigateTo: `/campaigns/${encodedId}` });
  if (action === '变更状态') return executeAction('campaign-status-change', action, { metadata: { next_status: nextCampaignStatus(selectedRow) } });
  if (action === '生成报告') return executeAction('campaign-report-generate', action, { target: '战役复盘报告', metadata: { format: 'pdf', sections: ['攻击阶段', '影响范围', '证据链', '处置结论'], evidence_count: 5 } });
  if (action === '下钻攻击链') return executeAction('campaign-attack-chain-view', action, { navigateTo: `/attack-chains?campaign=${encodedId}` });
  if (action === '跳转资产图谱') return executeAction('campaign-graph-view', action, { navigateTo: `/graph?campaign=${encodedId}` });
  if (action === '进入 SOAR 处置') return executeAction('campaign-soar-response', action, { navigateTo: `/playbooks?campaign=${encodedId}`, metadata: { dry_run: true } });
  return executeAction('campaign-context-action', action);
};

function CampaignActionReceipt({ context }: { context: CampaignActionContext }) {
  const { result } = context;
  return (
    <div className="taf-campaign-action-receipt">
      <Alert
        type="success"
        showIcon
        message={result.mode === 'server-persisted-mutation' ? '业务操作已持久化' : '访问操作已审计'}
        description={result.mode === 'server-persisted-mutation' ? '业务状态或报告任务已写入 PostgreSQL，审计已写入 audit_logs。' : '本次查看或导出操作已写入 campaign_action_jobs 与 audit_logs。'}
      />
      <dl>
        <dt>操作</dt><dd>{context.title}</dd>
        <dt>任务编号</dt><dd>{result.jobId}</dd>
        <dt>接口</dt><dd>{result.endpoint}</dd>
        <dt>审计事件</dt><dd>{result.auditEvent}</dd>
        <dt>作业状态</dt><dd>{result.jobStatus}</dd>
        <dt>审计状态</dt><dd>{result.status}</dd>
        <dt>业务结果</dt><dd><pre>{JSON.stringify(result.result, null, 2)}</pre></dd>
        <dt>请求体</dt><dd><pre>{JSON.stringify(result.requestBody, null, 2)}</pre></dd>
      </dl>
    </div>
  );
}

function CampaignDetailDrawerContent({
  snapshot,
  loading,
  error,
  onRetry,
}: {
  snapshot?: CampaignDetailSnapshot;
  loading: boolean;
  error: Error | null;
  onRetry: () => void;
}) {
  if (error) {
    return <Alert type="error" showIcon message="战役详情加载失败" description={error.message} action={<Button size="small" danger onClick={onRetry}>重试</Button>} />;
  }
  if (loading || !snapshot) return <div className="taf-campaign-detail-drawer__loading">正在加载战役证据与影响范围…</div>;
  return (
    <div className="taf-campaign-detail-drawer__content">
      <section className="taf-campaign-detail-drawer__summary">
        <div><FlagOutlined /><strong>{snapshot.title}</strong><span>{snapshot.summary}</span></div>
        <dl>
          <dt>风险评分</dt><dd>{snapshot.riskScore}/100</dd>
          <dt>当前阶段</dt><dd>{snapshot.currentPhase}</dd>
          <dt>负责人</dt><dd>{snapshot.assignee}</dd>
          <dt>状态</dt><dd><StatusTag value={snapshot.status} /></dd>
          <dt>关联告警</dt><dd>{snapshot.alertCount} 条</dd>
          <dt>影响资产</dt><dd>{snapshot.assetCount} 台</dd>
        </dl>
      </section>
      <section className="taf-campaign-detail-drawer__section">
        <h3>攻击阶段链</h3>
        <div className="taf-campaign-detail-drawer__phases">
          {snapshot.phases.map((phase) => (
            <span key={phase.phase} className={`is-${phase.status}`}>
              <b>{phase.phase}</b><small>{phase.time}</small><em>{phase.alertCount} 告警 / {phase.evidenceCount} 证据</em>
            </span>
          ))}
        </div>
      </section>
      <div className="taf-campaign-detail-drawer__columns">
        <section className="taf-campaign-detail-drawer__section">
          <h3>影响范围</h3>
          <div className="taf-campaign-detail-drawer__impact">
            {snapshot.impactTabs.map((item) => <span key={item.label}><b>{item.label}</b><strong>{item.value}</strong></span>)}
          </div>
        </section>
        <section className="taf-campaign-detail-drawer__section">
          <h3>证据完整度 · {snapshot.evidenceCompleteness}%</h3>
          <div className="taf-campaign-detail-drawer__evidence">
            {snapshot.evidenceChecks.map((item) => <span key={item.label} className={`is-${item.status}`}><b>{item.label}</b><strong>{item.value}</strong></span>)}
          </div>
        </section>
      </div>
      <section className="taf-campaign-detail-drawer__section">
        <h3>最近关联告警</h3>
        <Table
          size="small"
          rowKey="告警ID"
          pagination={false}
          dataSource={snapshot.alerts.slice(0, 5)}
          columns={[
            { title: '告警时间', dataIndex: '告警时间', width: 126 },
            { title: '告警名称', dataIndex: '告警名称', ellipsis: true },
            { title: '攻击阶段', dataIndex: '攻击阶段', width: 100 },
            { title: '风险', dataIndex: '风险', width: 74, render: (value) => <StatusTag value={value} /> },
            { title: '状态', dataIndex: '状态', width: 84, render: (value) => <StatusTag value={value} /> },
          ]}
        />
      </section>
    </div>
  );
}

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const nextCampaignStatus = (row: SnapshotRow | undefined) => {
  const current = campaignWorkflowStatus(row);
  if (current === '活跃中') return 'investigating';
  if (current === '调查中') return 'contained';
  if (current === '处置中') return 'closed';
  return 'active';
};

const buildPhaseNodes = (
  selectedRow: SnapshotRow | undefined,
  visualBreakdownMode: boolean,
  detail?: CampaignDetailSnapshot,
) => {
  if (visualBreakdownMode) {
    return phaseNodeSpecs.map((node) => ({ ...node, alertCount: node.fallbackCount, evidenceCount: Math.max(1, node.fallbackCount - 1) }));
  }

  return phaseNodeSpecs.map((node) => {
    const phase = detail?.phases.find((item) => normalizePhaseLabel(item.phase) === normalizePhaseLabel(node.phase));
    return {
      ...node,
      alertCount: phase?.alertCount ?? 0,
      evidenceCount: phase?.evidenceCount ?? 0,
      tone: phase?.status ?? node.tone,
    };
  });
};

const normalizePhaseLabel = (value: string) => {
  if (value === 'C2通信' || value === '外联') return '外联';
  if (value === '处置闭环' || value === '影响达成') return '影响达成';
  return value;
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

const evidenceMetricWidth = (value: string, label: string) => {
  if (value.includes('/')) return evidenceWidth(value);
  const count = Number(value.replace(/[^\d.]/g, ''));
  if (!Number.isFinite(count) || count <= 0) return '0%';
  if (label.includes('告警')) return `${Math.min(100, 30 + Math.log10(count + 1) * 24)}%`;
  if (label.includes('证据')) return `${Math.min(100, 24 + Math.log10(count + 1) * 28)}%`;
  return '72%';
};

const visualCampaignEvidenceItems = (): PageSnapshot['evidence'] => {
  return [
    { label: '告警', value: '234 / 312', status: 'ok' },
    { label: 'PCAP / Session', value: '86 / 128', status: 'warn' },
    { label: '日志', value: '1,432 / 2,150', status: 'ok' },
    { label: '图谱路径', value: '12 / 18', status: 'warn' },
    { label: '处置记录', value: '8 / 10', status: 'risk' },
  ];
};

const campaignStateFlow = (row: SnapshotRow | undefined) => {
  const current = campaignWorkflowStatus(row);
  const firstSeen = text(row, '首次发现', '-');
  const recent = text(row, '最近活动', '-');
  const updated = text(row, '__workbench_updated_at', '-');
  return [
    ['新建', firstSeen],
    ['调查中', current === '调查中' ? updated : '-'],
    ['处置中', current === '处置中' ? updated : '-'],
    [current === '已结束' ? '已结束' : '活跃中', current === '已结束' || current === '活跃中' ? recent : '-'],
  ];
};

const campaignWorkflowStatus = (row: SnapshotRow | undefined) =>
  text(row, '__workflow_status', text(row, '状态', '活跃中'));

const campaignSourceLabel = (row: SnapshotRow | undefined) => {
  const rules = Number(row?.__rule_count ?? 0);
  const models = Number(row?.__model_count ?? 0);
  const sources = [
    rules > 0 ? '规则' : '',
    models > 0 ? '行为检测' : '',
    text(row, '__campaign_type', '') ? '威胁情报' : '',
  ].filter(Boolean);
  return sources.length ? sources.join(' / ') : '未提供';
};
