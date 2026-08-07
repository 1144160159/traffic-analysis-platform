import {
  ApartmentOutlined,
  BranchesOutlined,
  CaretRightOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseOutlined,
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
import { Alert, Button, Drawer, Empty, Input, Modal, Popconfirm, Select, Space, Table, Tabs, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { CampaignAttackGraphChart, DataQualityDonutChart } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { CampaignImpactModalContent } from '@/pages/CampaignDetailPage';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import {
  CampaignReportTerminalError,
  applyCampaignSOAROperation,
  classifyCampaignActionStatus,
  downloadCampaignReport,
  getCampaignSOARJob,
  saveCampaignReportArtifact,
  submitCampaignAction,
  waitForCampaignReport,
  type CampaignActionId,
  type CampaignActionResult,
  type CampaignActionStatus,
  type CampaignReportStatus,
  type CampaignSOARJob,
  type CampaignSOAROperation,
} from '@/services/campaignActionApi';
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
  report?: CampaignReportStatus;
  soar?: CampaignSOARJob;
  reportError?: string;
};

const emptyCampaignFilters: CampaignFilters = { risk: '全部', status: '全部', phase: '全部', keyword: '' };

export function CampaignWorkbenchPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const [routeSearch, setRouteSearch] = useSearchParams();
  const queryClient = useQueryClient();
  const visualBreakdownMode = import.meta.env.DEV && isVisualBreakdownMode();
  const visualPageId = routeSearch.get('__codex_page_id') ?? '';
  const requestedCampaign = routeSearch.get('campaign') ?? '';
  const drawerRequested = routeSearch.get('drawer') === 'campaign-detail';
  const [selectedRowKey, setSelectedRowKey] = useState<string | undefined>(requestedCampaign || undefined);
  const [filterDraft, setFilterDraft] = useState<CampaignFilters>(emptyCampaignFilters);
  const [appliedFilters, setAppliedFilters] = useState<CampaignFilters>(emptyCampaignFilters);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(visualBreakdownMode ? 10 : 8);
  const [actionContext, setActionContext] = useState<CampaignActionContext>();
  const [soarControlPending, setSoarControlPending] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [impactOpen, setImpactOpen] = useState(false);
  const [activeImpact, setActiveImpact] = useState('asset');
  const reportAbortRef = useRef<AbortController>();
  useEffect(() => () => reportAbortRef.current?.abort(), []);
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
    () => filterCampaignRows(rows, appliedFilters),
    [rows, appliedFilters],
  );
  const selectedRow = useMemo(() => {
    if (!filteredRows.length) return undefined;
    return filteredRows.find((row) => rowKey(row) === selectedRowKey) ?? filteredRows[0];
  }, [filteredRows, selectedRowKey]);
  useEffect(() => {
    if (requestedCampaign && requestedCampaign !== selectedRowKey) {
      setSelectedRowKey(requestedCampaign);
    }
  }, [requestedCampaign, selectedRowKey]);
  useEffect(() => {
    if ((visualPageId === 'drawer-campaign-detail' || drawerRequested) && selectedRow) setDetailOpen(true);
  }, [drawerRequested, selectedRow, visualPageId]);

  const actionMutation = useMutation({
    mutationFn: submitCampaignAction,
    onSuccess: async (result) => {
      await queryClient.invalidateQueries({ queryKey: ['page-snapshot', route.id] });
      showCampaignActionNotice(result.jobStatus, result.jobId);
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '战役操作提交失败'),
  });
  const selectedCampaignId = text(selectedRow, '战役名称', '');
  const selectedSnapshotId = text(selectedRow, '__snapshot_id', '');
  const selectCampaign = (record: SnapshotRow) => {
    const targetId = rowKey(record);
    setSelectedRowKey(targetId);
    const next = new URLSearchParams(routeSearch);
    next.set('campaign', targetId);
    next.delete('detailTab');
    setRouteSearch(next, { replace: true });
  };
  const detailQuery = useQuery({
    queryKey: ['campaign-detail-drawer', selectedCampaignId, selectedSnapshotId],
    queryFn: () => fetchCampaignDetailSnapshot(selectedCampaignId, selectedSnapshotId || undefined),
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
    const targetRow = filteredRows.find((row) => rowKey(row) === campaignId) ?? selectedRow;
    const detailRevision = detailQuery.data?.campaignId === campaignId ? detailQuery.data.stateVersion : undefined;
    const snapshotId = detailQuery.data?.campaignId === campaignId
      ? detailQuery.data.snapshotId
      : String(targetRow?.__snapshot_id ?? '');
    const expectedRevision = Number(detailRevision ?? targetRow?.__state_version ?? 0);
    const result = await actionMutation.mutateAsync({
      actionId,
      campaignId,
      target: options?.target ?? title,
      metadata: {
        ...(options?.metadata ?? {}),
        ...(snapshotId ? { snapshot_id: snapshotId } : {}),
      },
      expectedRevision: Number.isSafeInteger(expectedRevision) && expectedRevision >= 0 ? expectedRevision : 0,
      reason: `战役工作台操作：${title}`,
    });
    if (options?.showReceipt !== false) setActionContext({ title, result });
    if (actionId === 'campaign-soar-response') {
      try {
        const soar = await getCampaignSOARJob(campaignId, result.jobId);
        if (options?.showReceipt !== false) setActionContext({ title, result, soar });
      } catch (soarError) {
        const errorMessage = soarError instanceof Error ? soarError.message : 'SOAR 状态读取失败';
        if (options?.showReceipt !== false) setActionContext({ title, result, reportError: errorMessage });
        throw soarError;
      }
    }
    if (actionId === 'campaign-report-generate') {
      const reportId = typeof result.result.report_id === 'string' ? result.result.report_id.trim() : '';
      if (!reportId) throw new Error('战役报告受理响应缺少稳定 report_id');
      reportAbortRef.current?.abort();
      const controller = new AbortController();
      reportAbortRef.current = controller;
      try {
        const report = await waitForCampaignReport(campaignId, reportId, {
          signal: controller.signal,
          onStatus: (status) => {
            if (options?.showReceipt !== false) setActionContext({ title, result, report: status });
          },
        });
        const artifact = await downloadCampaignReport(campaignId, report);
        saveCampaignReportArtifact(artifact);
        if (options?.showReceipt !== false) setActionContext({ title, result, report });
        message.success(`战役报告已校验并下载：${artifact.filename}`);
      } catch (reportError) {
        if (controller.signal.aborted) throw reportError;
        const report = reportError instanceof CampaignReportTerminalError ? reportError.report : undefined;
        const errorMessage = reportError instanceof Error ? reportError.message : '战役报告执行失败';
        if (options?.showReceipt !== false) setActionContext({ title, result, report, reportError: errorMessage });
        message.error(errorMessage);
        throw reportError;
      } finally {
        if (reportAbortRef.current === controller) reportAbortRef.current = undefined;
      }
    }
    if (options?.navigateTo) navigate(options.navigateTo);
    return result;
  };
  const controlSOAR = async (operation: CampaignSOAROperation, reason: string) => {
    const current = actionContext?.soar;
    if (!current) return;
    setSoarControlPending(true);
    try {
      const soar = await applyCampaignSOAROperation(
        current.campaignId,
        current.jobId,
        operation,
        current.revision,
        reason,
      );
      setActionContext((existing) => existing ? { ...existing, soar, reportError: undefined } : existing);
      message.info(`SOAR 操作已提交：${soar.status}`);
    } catch (controlError) {
      const errorMessage = controlError instanceof Error ? controlError.message : 'SOAR 操作失败';
      setActionContext((existing) => existing ? { ...existing, reportError: errorMessage } : existing);
      message.error(errorMessage);
    } finally {
      setSoarControlPending(false);
    }
  };
  const refreshSOAR = async () => {
    const current = actionContext?.soar;
    if (!current) return;
    setSoarControlPending(true);
    try {
      const soar = await getCampaignSOARJob(current.campaignId, current.jobId);
      setActionContext((existing) => existing ? { ...existing, soar, reportError: undefined } : existing);
    } catch (refreshError) {
      const errorMessage = refreshError instanceof Error ? refreshError.message : 'SOAR 状态刷新失败';
      setActionContext((existing) => existing ? { ...existing, reportError: errorMessage } : existing);
      message.error(errorMessage);
    } finally {
      setSoarControlPending(false);
    }
  };
  const openDetail = async () => {
    if (!selectedCampaignId) return;
    await executeAction('campaign-detail-view', '查看战役详情', { showReceipt: false });
    const next = new URLSearchParams(routeSearch);
    next.set('campaign', selectedCampaignId);
    next.set('drawer', 'campaign-detail');
    setRouteSearch(next, { replace: true });
    setDetailOpen(true);
  };
  const closeDetail = () => {
    const next = new URLSearchParams(routeSearch);
    next.delete('drawer');
    next.delete('detailTab');
    setRouteSearch(next, { replace: true });
    setDetailOpen(false);
  };
  const exportRows = async () => {
    if (!selectedCampaignId || !filteredRows.length) {
      message.info('当前查询结果为空，无可导出数据');
      return;
    }
    await executeAction('campaign-export', '导出当前页', {
      target: `当前页 ${filteredRows.length} 条`,
      metadata: { selection: 'current-page', row_count: filteredRows.length, format: 'json' },
    });
    message.info('战役列表导出请求已受理；服务端制品与下载终态尚未返回，本次不会生成浏览器伪制品。');
  };

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: campaignColumnWidth(column),
    ellipsis: true,
    render: (value, record) => renderCampaignCell(column, value, record, actionMutation.isPending, (action) => {
      const targetId = rowKey(record);
      selectCampaign(record);
      if (action === 'detail') {
        void executeAction('campaign-detail-view', `查看 ${text(record, '战役名称', '战役')} 详情`, {
          targetId,
          target: '表格行查看详情',
          showReceipt: false,
        }).then(() => {
          const next = new URLSearchParams(routeSearch);
          next.set('campaign', targetId);
          next.set('drawer', 'campaign-detail');
          setRouteSearch(next, { replace: true });
          setDetailOpen(true);
        }).catch(() => {});
        return;
      }
      if (action === 'status') {
        void executeAction('campaign-status-change', `变更 ${text(record, '战役名称', '战役')} 状态`, {
          targetId,
          target: '表格行状态流转',
          metadata: { next_status: nextCampaignStatus(record) },
        }).catch(() => {});
        return;
      }
      void executeAction('campaign-context-action', `查看 ${text(record, '战役名称', '战役')} 操作`, {
        targetId,
        target: '表格行更多操作',
      }).catch(() => {});
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
                  onClick: () => selectCampaign(record),
                  'aria-selected': selectedRow ? rowKey(record) === rowKey(selectedRow) : false,
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
            detail={detailQuery.data}
            disabled={!selectedRow}
            onInspect={(scope) => {
              if (!selectedCampaignId) return;
              void executeAction('campaign-impact-inspect', `查看${scope}影响范围`, {
                target: scope,
                metadata: { scope },
                showReceipt: false,
              }).then(() => {
                setActiveImpact(campaignImpactRouteId(scope));
                setImpactOpen(true);
              }).catch(() => {});
            }}
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
            detail={detailQuery.data}
            visualBreakdownMode={visualBreakdownMode}
            actions={route.page.actions}
            pending={actionMutation.isPending}
            onAction={(action) => {
              if (action === '查看详情') {
                void openDetail();
                return;
              }
              void handleCampaignAction(action, selectedCampaignId, selectedRow, executeAction).catch(() => {});
            }}
          />
        </aside>
      </div>
      <Modal
        className="taf-campaign-action-drawer"
        title={actionContext?.title ?? '战役操作'}
        width="min(520px, calc(100dvw - 40px))"
        open={Boolean(actionContext)}
        footer={null}
        onCancel={() => {
          reportAbortRef.current?.abort();
          setActionContext(undefined);
        }}
      >
        {actionContext && <CampaignActionReceipt context={actionContext} pending={soarControlPending} onSOAROperation={controlSOAR} onSOARRefresh={refreshSOAR} />}
      </Modal>
      <Modal
        className="taf-campaign-impact-modal"
        open={impactOpen}
        width="min(1040px, calc(100dvw - 96px))"
        centered
        title={null}
        footer={null}
        styles={{ body: { padding: 0 } }}
        onCancel={() => setImpactOpen(false)}
      >
        {detailQuery.isLoading && <div className="taf-campaign-detail-drawer__loading">正在加载战役影响范围…</div>}
        {detailQuery.isError && (
          <Alert
            type="error"
            showIcon
            message="影响范围加载失败"
            description={detailQuery.error instanceof Error ? detailQuery.error.message : '请检查战役详情接口。'}
            action={<Button size="small" danger onClick={() => void detailQuery.refetch()}>重试</Button>}
          />
        )}
        {detailQuery.data && (
          <CampaignImpactModalContent
            snapshot={detailQuery.data}
            activeImpact={activeImpact}
            onImpactChange={setActiveImpact}
          />
        )}
      </Modal>
      <Drawer
        rootClassName="taf-campaign-detail-drawer"
        title={null}
        placement="right"
        width="min(1200px, calc(100dvw - 48px))"
        open={detailOpen}
        closable={false}
        onClose={closeDetail}
        styles={{ body: { padding: 0 } }}
      >
        <CampaignDetailDrawerContent
          snapshot={detailQuery.data}
          loading={detailQuery.isLoading}
          pending={actionMutation.isPending}
          error={detailQuery.error}
          onRetry={() => void detailQuery.refetch()}
          onClose={closeDetail}
          onOpenFull={() => navigate(`/campaigns/${encodeURIComponent(selectedCampaignId)}${detailQuery.data?.snapshotId ? `?snapshot_id=${encodeURIComponent(detailQuery.data.snapshotId)}` : ''}`)}
          onAction={(actionId, target, metadata) => executeAction(actionId, target, { metadata })}
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
        <Select size="small" value={value.status} onChange={(status) => onChange({ ...value, status })} options={[{ value: '全部' }, { value: '活跃中' }, { value: '调查中' }, { value: '处置中' }, { value: '已结束' }]} />
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
        {phaseNodes.map(({ phase, alertCount, tone, Icon }, index) => (
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

function ImpactScope({
  selectedRow,
  detail,
  disabled,
  onInspect,
  onViewAssets,
}: {
  selectedRow?: SnapshotRow;
  detail?: CampaignDetailSnapshot;
  disabled: boolean;
  onInspect: (scope: string) => void;
  onViewAssets: () => void;
}) {
  return (
    <WorkPanel title="影响范围" className="taf-campaign-impact-panel" extra={<Button size="small" type="link" disabled={disabled} onClick={onViewAssets}>查看资产列表</Button>}>
      <div className="taf-campaign-impact">
        {impactItems.map(({ label, field, suffix, Icon }) => (
          <button key={label} type="button" disabled={disabled} onClick={() => onInspect(label)}>
            <Icon />
            <span>{label}</span>
            <strong>{campaignImpactValue(detail, label, text(selectedRow, field, '0'))} {suffix}</strong>
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

const campaignImpactValue = (detail: CampaignDetailSnapshot | undefined, label: string, fallback: string) =>
  detail?.impactTabs.find((item) => item.label === label || (label === '园区' && item.label === '校区'))?.value ?? fallback;

const campaignImpactRouteId = (label: string) => {
  if (label === '账号') return 'account';
  if (label === '服务') return 'service';
  if (label === '业务系统') return 'business-system';
  if (label === '部门') return 'department';
  if (label === '园区' || label === '校区') return 'campus';
  return 'asset';
};

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
  const percentAvailable = visualBreakdownMode || Boolean(detail?.evidenceCompletenessAvailable);
  const percent = visualBreakdownMode ? 78 : (detail?.evidenceCompleteness ?? 0);
  const items = visualBreakdownMode
    ? visualCampaignEvidenceItems()
    : campaignEvidenceRailItems(detail, selectedRow);
  const donutRows = percentAvailable
    ? [
        { label: '已收集', value: percent, color: '#36d66b' },
        { label: '待补齐', value: Math.max(0, 100 - percent), color: 'rgba(56,151,201,0.18)' },
      ]
    : [{ label: '口径待配置', value: 100, color: 'rgba(56,151,201,0.22)' }];
  return (
    <WorkPanel title="证据完整度" className="taf-campaign-evidence-panel" extra={<Button size="small" type="link" disabled={!selectedRow} onClick={onViewEvidence}>查看证据中心</Button>}>
      <div className="taf-campaign-evidence">
        <div className="taf-campaign-evidence-chart">
          <DataQualityDonutChart
            ariaLabel="战役证据完整度动态图"
            rows={donutRows}
          />
          <strong>{percentAvailable ? `${percent}%` : '--'}</strong>
          <span>{percentAvailable ? '已收集' : '口径待配置'}</span>
        </div>
        <div className="taf-campaign-evidence-list">
          {items.slice(0, 5).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <i aria-hidden="true" />
              <b>{item.label}</b>
              <em aria-hidden="true" />
              <strong>{item.value}</strong>
            </span>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function StateTransition({
  selectedRow,
  detail,
  visualBreakdownMode,
  actions,
  pending,
  onAction,
}: {
  selectedRow?: SnapshotRow;
  detail?: CampaignDetailSnapshot;
  visualBreakdownMode: boolean;
  actions: string[];
  pending: boolean;
  onAction: (action: string) => void;
}) {
  const campaignActions = ['查看详情', '变更状态', '生成报告', '下钻攻击链', '跳转资产图谱', 'SOAR 处置'];
  const visibleActions = campaignActions.length ? campaignActions : actions;
  if (!selectedRow) {
    return <WorkPanel title="状态流转" className="taf-campaign-state-panel"><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择战役后执行处置" /></WorkPanel>;
  }
  const stateFlow = campaignStateFlow(selectedRow, detail, visualBreakdownMode);
  return (
    <WorkPanel title="状态流转" className="taf-campaign-state-panel">
      <div className="taf-campaign-state-flow">
        {stateFlow.map(([state, time, current], index) => (
          <div key={state} className="taf-campaign-state-step">
            <span className={current ? 'is-current' : ''}>
              <strong>{state}</strong>
              <small>{time}</small>
            </span>
            {index < stateFlow.length - 1 && <CaretRightOutlined aria-hidden="true" />}
          </div>
        ))}
      </div>
      <h3 className="taf-campaign-actions-title">战役操作</h3>
      <div className="taf-campaign-actions">
        {visibleActions.map((action) => {
          const dangerous = action === '变更状态' || action === 'SOAR 处置';
          const button = (
            <Button key={action} size="small" disabled={!selectedRow} loading={pending} onClick={dangerous ? undefined : () => onAction(action)} icon={campaignActionIcon(action)}>
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
              okButtonProps={{ loading: pending }}
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
  pending: boolean,
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
          disabled={pending}
          onClick={(event) => {
            event.stopPropagation();
            onAction('detail');
          }}
        />
        <Popconfirm
          title="确认变更战役处置状态？"
          description="该操作会修改当前战役状态并写入审计日志。"
          okButtonProps={{ loading: pending }}
          onConfirm={() => onAction('status')}
        >
          <Button
            size="small"
            type="text"
            aria-label={`变更${campaignName}状态`}
            icon={<CheckCircleOutlined />}
            disabled={pending}
            onClick={(event) => event.stopPropagation()}
          />
        </Popconfirm>
        <Button
          size="small"
          type="text"
          aria-label={`打开${campaignName}更多操作`}
          icon={<MoreOutlined />}
          disabled={pending}
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

const showCampaignActionNotice = (status: CampaignActionStatus, jobId: string) => {
  const statusClass = classifyCampaignActionStatus(status);
  if (statusClass === 'in_progress') {
    message.info(`战役操作已受理，尚未最终完成：${jobId}`);
  } else if (statusClass === 'succeeded') {
    message.success(`战役操作已完成：${jobId}`);
  } else if (statusClass === 'partial') {
    message.warning(`战役操作部分完成，请检查失败目标与补偿状态：${jobId}`);
  } else if (statusClass === 'cancelled') {
    message.warning(`战役操作已取消，未形成最终成功：${jobId}`);
  } else if (statusClass === 'compensated') {
    message.warning(`战役操作已补偿，原操作不应按成功关闭：${jobId}`);
  } else {
    message.error(`战役操作失败，请检查权威回执：${jobId}`);
  }
};

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
  if (action === '下钻攻击链') return executeAction('campaign-attack-chain-view', action, { navigateTo: `/attack-chains?chain=${encodedId}` });
  if (action === '跳转资产图谱') return executeAction('campaign-graph-view', action, { navigateTo: `/graph?campaign=${encodedId}` });
  if (action === 'SOAR 处置') return executeAction('campaign-soar-response', action, { metadata: { playbook_id: 'quarantine-c2' } });
  return executeAction('campaign-context-action', action);
};

function CampaignActionReceipt({
  context,
  pending,
  onSOAROperation,
  onSOARRefresh,
}: {
  context: CampaignActionContext;
  pending: boolean;
  onSOAROperation: (operation: CampaignSOAROperation, reason: string) => Promise<void>;
  onSOARRefresh: () => Promise<void>;
}) {
  const { result } = context;
  const [reason, setReason] = useState('战役工作台审批确认执行本次处置');
  const status = context.soar?.status ?? context.report?.status ?? result.jobStatus;
  const statusClass = classifyCampaignActionStatus(status as CampaignActionStatus);
  const inProgress = statusClass === 'in_progress';
  const failed = Boolean(context.reportError) || statusClass === 'failed';
  const interrupted = ['partial', 'cancelled', 'compensated'].includes(statusClass);
  const soar = context.soar;
  return (
    <div className="taf-campaign-action-receipt">
      <Alert
        type={failed ? 'error' : inProgress ? 'info' : interrupted ? 'warning' : 'success'}
        showIcon
        message={failed ? '业务操作失败' : inProgress ? '业务操作执行中' : statusClass === 'partial' ? '业务操作部分完成' : statusClass === 'cancelled' ? '业务操作已取消' : statusClass === 'compensated' ? '业务操作已补偿' : result.mode === 'server-persisted-mutation' ? '业务操作已完成' : '访问操作已审计'}
        description={context.reportError ?? (inProgress ? '命令、聚合版本、审计和 outbox 已提交；正在等待审批或权威执行终态，尚未宣告最终成功。' : statusClass === 'partial' ? '仅有部分目标完成；请检查业务结果、失败目标和补偿状态，不能按成功关闭。' : statusClass === 'cancelled' ? '作业已取消；已受理不代表产生最终业务效果。' : statusClass === 'compensated' ? '原操作已执行补偿；补偿终态不等同于原操作成功。' : context.report ? '报告对象已按 PostgreSQL manifest 校验并下载。' : soar ? 'SOAR provider 回执已持久化并与聚合事件、审计完成对账。' : result.mode === 'server-persisted-mutation' ? '业务状态、聚合历史与审计已在 PostgreSQL 完成提交。' : '本次查看或导出操作已写入 campaign_action_jobs 与 audit_logs。')}
      />
      <dl>
        <dt>操作</dt><dd>{context.title}</dd>
        <dt>任务编号</dt><dd>{result.jobId}</dd>
        <dt>接口</dt><dd>{result.endpoint}</dd>
        <dt>审计事件</dt><dd>{result.auditEvent}</dd>
        <dt>作业状态</dt><dd>{status}</dd>
        <dt>审计状态</dt><dd>{result.status}</dd>
        {context.report && <><dt>报告编号</dt><dd>{context.report.reportId}</dd><dt>快照编号</dt><dd>{context.report.snapshotId}</dd><dt>执行次数</dt><dd>{context.report.attempts}</dd><dt>对象摘要</dt><dd>{context.report.artifactSHA256 || '终态前尚未生成'}</dd></>}
        {soar && <>
          <dt>剧本</dt><dd>{soar.playbookId}</dd>
          <dt>审批状态</dt><dd>{soar.approvalStatus}</dd>
          <dt>执行器状态</dt><dd>{soar.executorStatus}</dd>
          <dt>工作流版本</dt><dd>{soar.revision}</dd>
          <dt>请求人 / 审批人</dt><dd>{soar.requestedBy} / {soar.approvedBy || '尚未审批'}</dd>
          <dt>执行回执</dt><dd><pre>{JSON.stringify(soar.executionReceipt, null, 2)}</pre></dd>
          <dt>补偿回执</dt><dd><pre>{JSON.stringify(soar.compensationReceipt, null, 2)}</pre></dd>
        </>}
        <dt>业务结果</dt><dd><pre>{JSON.stringify(result.result, null, 2)}</pre></dd>
        <dt>请求体</dt><dd><pre>{JSON.stringify(result.requestBody, null, 2)}</pre></dd>
      </dl>
      {soar && (
        <Space direction="vertical" style={{ width: '100%' }}>
          <Input.TextArea
            aria-label="SOAR 操作原因"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            maxLength={1000}
            autoSize={{ minRows: 2, maxRows: 4 }}
          />
          <Space wrap>
            <Button loading={pending} onClick={() => void onSOARRefresh()}>刷新权威状态</Button>
            {soar.status === 'pending_approval' && <>
              <Button type="primary" loading={pending} disabled={reason.trim().length < 8} onClick={() => void onSOAROperation('approve', reason)}>批准执行</Button>
              <Button danger loading={pending} disabled={reason.trim().length < 8} onClick={() => void onSOAROperation('reject', reason)}>拒绝</Button>
            </>}
            {['pending_approval', 'approved_awaiting_executor'].includes(soar.status) && (
              <Button loading={pending} disabled={reason.trim().length < 8} onClick={() => void onSOAROperation('cancel', reason)}>取消请求</Button>
            )}
            {['completed', 'partial'].includes(soar.status) && (
              <Button danger loading={pending} disabled={reason.trim().length < 8} onClick={() => void onSOAROperation('compensate', reason)}>批准补偿</Button>
            )}
          </Space>
        </Space>
      )}
    </div>
  );
}

function CampaignDetailDrawerContent({
  snapshot,
  loading,
  pending,
  error,
  onRetry,
  onClose,
  onOpenFull,
  onAction,
}: {
  snapshot?: CampaignDetailSnapshot;
  loading: boolean;
  pending: boolean;
  error: Error | null;
  onRetry: () => void;
  onClose: () => void;
  onOpenFull: () => void;
  onAction: (actionId: CampaignActionId, target: string, metadata?: Record<string, unknown>) => Promise<CampaignActionResult>;
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
  if (error) {
    return <Alert type="error" showIcon message="战役详情加载失败" description={error.message} action={<Button size="small" danger onClick={onRetry}>重试</Button>} />;
  }
  if (loading || !snapshot) return <div className="taf-campaign-detail-drawer__loading">正在加载战役证据与影响范围…</div>;
  const graphNodes = snapshot.phases.slice(0, 6).map((phase) => ({
    name: phase.phase,
    alertCount: phase.alertCount,
    evidenceCount: phase.evidenceCount,
    tone: phase.status,
  }));
  return (
    <div className="taf-campaign-detail-drawer__content">
      <header className="taf-campaign-detail-drawer__header">
        <div><h2>战役详情</h2><p>{snapshot.campaignId} / {snapshot.title} / 置信度 {snapshot.riskScore}%</p></div>
        <Space>
          <StatusTag value={snapshot.status} />
          <StatusTag value={snapshot.riskScore >= 80 ? '高危' : '中危'} />
          <b>证据 {snapshot.evidenceSummaryRows.length}</b>
          <Button type="text" aria-label="关闭战役详情" icon={<CloseOutlined />} onClick={onClose} />
        </Space>
      </header>
      <section className="taf-campaign-detail-drawer__summary">
        {[
          ['战役名称', snapshot.campaignId, <FlagOutlined />],
          ['首次发现', snapshot.firstSeen, <ClockCircleOutlined />],
          ['最近活动', snapshot.lastUpdated, <ClockCircleOutlined />],
          ['影响资产', `${snapshot.assetCount} 台`, <ApartmentOutlined />],
          ['关联告警', `${snapshot.alertCount} 条`, <SafetyCertificateOutlined />],
          ['聚类置信度', `${snapshot.riskScore}%`, <NodeIndexOutlined />],
        ].map(([label, value, icon]) => <span key={String(label)}><i>{icon}</i><b>{label}</b><strong>{value}</strong></span>)}
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
      <div className="taf-campaign-detail-drawer__workspace" data-active-tab={activeTab}>
        <aside>
          <section className="taf-campaign-detail-drawer__section" data-tab-panel="detail">
            <h3>聚类原因</h3>
            <div className="taf-campaign-detail-drawer__reasons">
              {[
                ['关联告警', `${snapshot.alertCount} 条`],
                ['受影响实体', `${snapshot.assetCount} 个`],
                ['攻击阶段', `${snapshot.phases.length} 个`],
                ['证据类型', `${snapshot.evidenceSummaryRows.length} 类`],
                ['阶段数据', snapshot.phaseDataBacked ? '真实聚合' : '接口未提供'],
              ].map(([label, value]) => <span key={label}><b>{label}</b><strong>{value}</strong></span>)}
            </div>
          </section>
          <section className="taf-campaign-detail-drawer__section" data-tab-panel="detail">
            <h3>攻击阶段链（当前：{snapshot.currentPhase}）</h3>
            <div className="taf-campaign-detail-drawer__phases">
              {snapshot.phases.map((phase) => <span key={phase.phase} className={`is-${phase.status}`}><i /><b>{phase.phase}</b><small>{phase.time}</small></span>)}
            </div>
          </section>
        </aside>
        <main>
          <section className="taf-campaign-detail-drawer__section taf-campaign-detail-drawer__graph" data-tab-panel="detail">
            <h3>证据和攻击链预览</h3>
            <CampaignAttackGraphChart campaignId={snapshot.campaignId} risk={snapshot.riskScore >= 80 ? '高危' : '中危'} workflowStatus={snapshot.workflowStatus} nodes={graphNodes} ariaLabel="战役详情证据和攻击链预览" />
          </section>
          <section className="taf-campaign-detail-drawer__section taf-campaign-detail-drawer__evidence-table" data-tab-panel="evidence">
            <h3>证据列表（{snapshot.evidenceSummaryRows.length}）</h3>
            <Table
              size="small"
              rowKey="证据类型"
              pagination={false}
              dataSource={snapshot.evidenceSummaryRows}
              columns={[
                { title: '证据类型', dataIndex: '证据类型' },
                { title: '对象', dataIndex: '文件记录' },
                { title: '完整度', dataIndex: '完整度' },
                { title: '风险', render: () => <StatusTag value={snapshot.riskScore >= 80 ? '高危' : '中危'} /> },
                {
                  title: '可操作项',
                  render: (_, row) => (
                    <Space>
                      <Button size="small" type="text" aria-label={`查看${row.证据类型}`} icon={<EyeOutlined />} onClick={() => void onAction('campaign-evidence-view', `查看${row.证据类型}`, { evidence_type: row.证据类型 })} />
                      <Button size="small" type="text" aria-label={`导出${row.证据类型}`} icon={<DownloadOutlined />} onClick={() => void onAction('campaign-export', `导出${row.证据类型}`, { evidence_type: row.证据类型, format: 'json' })} />
                    </Space>
                  ),
                },
              ]}
              locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无真实证据记录" /> }}
            />
          </section>
        </main>
        <aside>
          <section className="taf-campaign-detail-drawer__section" data-tab-panel="detail">
            <h3>影响范围</h3>
            <div className="taf-campaign-detail-drawer__impact">{snapshot.impactTabs.map((item) => <span key={item.label}><b>{item.label}</b><strong>{item.value}</strong></span>)}</div>
          </section>
          <section className="taf-campaign-detail-drawer__section" data-tab-panel="detail">
            <h3>处置建议</h3>
            <div className="taf-campaign-detail-drawer__suggestions">
              {[
                ['隔离受影响终端', 'campaign-soar-response'],
                ['吊销可疑账号令牌', 'campaign-context-action'],
                ['下发检测规则', 'campaign-context-action'],
                ['触发 SOAR 剧本', 'campaign-soar-response'],
                ['生成战役报告', 'campaign-report-generate'],
              ].map(([item, actionId]) => (
                <Popconfirm
                  key={item}
                  title={`确认执行“${item}”？`}
                  description="操作将经过服务端权限校验并写入审计留痕。"
                  okText="确认执行"
                  cancelText="取消"
                  okButtonProps={{ loading: pending }}
                  onConfirm={() => onAction(actionId as CampaignActionId, item, actionId === 'campaign-report-generate'
                    ? { format: 'pdf', sections: snapshot.phases.map((phase) => phase.phase), evidence_count: snapshot.evidenceSummaryRows.length }
                    : actionId === 'campaign-soar-response' ? { playbook_id: 'quarantine-c2' } : { dry_run: true })}
                >
                  <button type="button" disabled={pending}>
                    <CheckCircleOutlined />{item}
                  </button>
                </Popconfirm>
              ))}
            </div>
          </section>
          <section className="taf-campaign-detail-drawer__section" data-tab-panel="audit">
            <h3>审计留痕</h3>
            <dl><dt>操作人</dt><dd>{snapshot.assignee || '当前登录用户'}</dd><dt>审批策略</dt><dd>由服务端权限策略校验</dd><dt>变更摘要</dt><dd>{snapshot.summary}</dd><dt>审计编号</dt><dd>操作成功后由服务端生成</dd></dl>
          </section>
        </aside>
      </div>
      <footer className="taf-campaign-detail-drawer__footer">
        <Alert type="warning" showIcon message={`隔离动作将影响 ${snapshot.assetCount} 台资产，需审批`} description="请确认业务影响并完成审批流程后执行。" />
        <Space>
          <Button onClick={onClose}>关闭</Button>
          <Button icon={<DownloadOutlined />} disabled={pending} onClick={() => void onAction('campaign-export', '导出证据包', { format: 'json' })}>导出证据包</Button>
          <Popconfirm
            title="确认触发 SOAR 剧本？"
            description={`将针对 ${snapshot.assetCount} 台受影响资产创建审计任务。`}
            okText="确认触发"
            cancelText="取消"
            okButtonProps={{ loading: pending }}
            onConfirm={() => void onAction('campaign-soar-response', '触发剧本', { playbook_id: 'quarantine-c2' })}
          >
            <Button icon={<BranchesOutlined />} disabled={pending}>触发剧本</Button>
          </Popconfirm>
          <Button onClick={onOpenFull}>打开完整详情</Button>
          <Popconfirm
            title="确认提交处置建议？"
            description="提交后战役将进入处置中状态，并写入审计留痕。"
            okText="确认提交"
            cancelText="取消"
            okButtonProps={{ loading: pending }}
            onConfirm={() => void onAction('campaign-status-change', '提交处置建议', { next_status: 'contained' })}
          >
            <Button type="primary" disabled={pending}>提交处置建议</Button>
          </Popconfirm>
        </Space>
      </footer>
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

const visualCampaignEvidenceItems = (): PageSnapshot['evidence'] => {
  return [
    { label: '告警', value: '234 / 312', status: 'info' },
    { label: 'PCAP / Session', value: '86 / 128', status: 'ok' },
    { label: '日志', value: '1,432 / 2,150', status: 'warn' },
    { label: '图谱路径', value: '12 / 18', status: 'warn' },
    { label: '处置记录', value: '8 / 10', status: 'risk' },
  ];
};

const campaignEvidenceRailItems = (
  detail: CampaignDetailSnapshot | undefined,
  row: SnapshotRow | undefined,
): PageSnapshot['evidence'] => {
  const fallbackAlertCount = Number(row?.['告警数'] ?? 0);
  const source = detail?.evidenceRail?.length
    ? detail.evidenceRail
    : [
        { key: 'alerts', label: '告警', current: Number.isFinite(fallbackAlertCount) ? fallbackAlertCount : 0, expected: null, available: true },
        { key: 'packet_session', label: 'PCAP / Session', current: null, expected: null, available: false },
        { key: 'logs', label: '日志', current: null, expected: null, available: false },
        { key: 'graph_paths', label: '图谱路径', current: null, expected: null, available: false },
        { key: 'response_records', label: '处置记录', current: null, expected: null, available: false },
      ];
  return source.slice(0, 5).map((item, index) => {
    const current = item.available && item.current !== null ? formatCount(item.current) : '--';
    const expected = item.expected !== null ? formatCount(item.expected) : '--';
    return {
      label: item.label,
      value: `${current} / ${expected}`,
      status: (['info', 'ok', 'warn', 'warn', 'risk'][index] ?? 'info') as PageSnapshot['evidence'][number]['status'],
    };
  });
};

const campaignStateFlow = (
  row: SnapshotRow | undefined,
  detail: CampaignDetailSnapshot | undefined,
  visualBreakdownMode: boolean,
): Array<[string, string, boolean]> => {
  if (visualBreakdownMode) {
    return [
      ['新建', '06-19 09:15', false],
      ['调查中', '06-19 10:02', false],
      ['处置中', '06-19 18:33', false],
      ['活跃中', '06-20 03:22', true],
    ];
  }
  const current = campaignWorkflowStatus(row);
  const firstSeen = text(row, '首次发现', '-');
  const recent = text(row, '最近活动', '-');
  const updated = text(row, '__workbench_updated_at', '-');
  const transitionTimes = new Map(
    (detail?.statusTransitions ?? []).map((item) => [
      campaignStatusLabel(item.status),
      formatCampaignStateTime(item.changedAt),
    ]),
  );
  const finalState = current === '已结束' ? '已结束' : '活跃中';
  return [
    ['新建', transitionTimes.get('新建') || formatCampaignStateTime(firstSeen), current === '新建'],
    ['调查中', transitionTimes.get('调查中') || (current === '调查中' ? formatCampaignStateTime(updated) : '--'), current === '调查中'],
    ['处置中', transitionTimes.get('处置中') || (current === '处置中' ? formatCampaignStateTime(updated) : '--'), current === '处置中'],
    [finalState, transitionTimes.get(finalState) || (current === finalState ? formatCampaignStateTime(recent) : '--'), current === finalState],
  ];
};

const campaignStatusLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized === 'new') return '新建';
  if (normalized === 'investigating') return '调查中';
  if (normalized === 'contained') return '处置中';
  if (normalized === 'closed') return '已结束';
  if (normalized === 'active') return '活跃中';
  return value;
};

const formatCampaignStateTime = (value: string) => {
  if (!value || value === '-') return '--';
  const parsed = Date.parse(value);
  if (Number.isFinite(parsed)) {
    const date = new Date(parsed);
    return `${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')} ${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}`;
  }
  const matched = value.match(/(\d{2})[-/](\d{2}).*?(\d{2}):(\d{2})/);
  return matched ? `${matched[1]}-${matched[2]} ${matched[3]}:${matched[4]}` : value;
};

const formatCount = (value: number) => new Intl.NumberFormat('zh-CN').format(Math.max(0, value));

const campaignActionIcon = (action: string) => {
  if (action === '变更状态') return <ClockCircleOutlined />;
  if (action.includes('报告')) return <FileProtectOutlined />;
  if (action.includes('攻击链')) return <BranchesOutlined />;
  if (action.includes('资产图谱')) return <ApartmentOutlined />;
  if (action.includes('SOAR')) return <SafetyCertificateOutlined />;
  return <EyeOutlined />;
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
