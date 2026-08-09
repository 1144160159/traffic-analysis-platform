import { ApiOutlined, ArrowDownOutlined, ArrowUpOutlined, CheckCircleOutlined, CloseOutlined, DatabaseOutlined, DownloadOutlined, FileSearchOutlined, FileDoneOutlined, FullscreenOutlined, FieldTimeOutlined, PrinterOutlined, ReloadOutlined, LeftOutlined, RightOutlined, SafetyCertificateOutlined, SearchOutlined, SyncOutlined } from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Form, Input, Modal, Select, Space, Switch, Tooltip, message } from 'antd';
import type { CSSProperties, MouseEvent, ReactNode } from 'react';
import { createContext, useContext, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { WorkPanel } from '@/components/WorkPanel';
import { DataQualityDonutChart, DataQualityFieldTrendChart, DataQualityHeatmapChart, DataQualityKpiSparklineChart, DataQualityTrendChart, RingChart } from '@/components/charts';
import { mergeRouteSearchParams } from '@/routes/pageRouteState';
import type { NavRoute } from '@/routes/routeManifest';
import { downloadDataQualityDailyReport, fetchDataQualityDailyReport, fetchDataQualityTablePage, fetchPageSnapshot, submitDataQualityAction, type DataQualityDailyReport, type DataQualityTableDataset, type DataQualityTimeRange } from '@/services/api';
import { buildDLQReplayDryRunRequest, requestDLQFallbackReplay, type DLQReplayFallbackRequest, type DLQReplayFallbackResult } from '@/services/dlqReplayApi';
import type { DataQualityCheck, DataQualityVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';
import { getPageActionPlan, type ActionEndpointPlan } from '@/services/pageApiPlans';

const dlqReplayActionPlan = getPageActionPlan('data-quality', 'dlq-fallback-replay');
const dataQualityContextActionPlan = getPageActionPlan('data-quality', 'data-quality-context-action');

const actionEndpointLabel = (action: ActionEndpointPlan | undefined) => (action ? `${action.method} ${action.endpoint}` : 'POST /v1/dlq/replay/fallback');

const actionScopeLabel = (action: ActionEndpointPlan | undefined) => (action?.acceptedScopes ?? action?.requiredScopes ?? ['dlq:replay']).join(' / ');

const dlqSampleDrawerWidth = 'min(720px, calc(var(--taf-window-inner-width, 100dvw) - 40px))';
const dlqReplayModalWidth = 'min(760px, calc(var(--taf-window-inner-width, 100dvw) - 64px))';
const fieldDetailDrawerWidth = 'min(480px, calc(var(--taf-window-inner-width, 100dvw) - 40px))';

const dataQualityOverlays: OverlayContract[] = [
  {
    id: 'modal-data-replay-task',
    title: '数据重放任务',
    kind: 'Modal',
    actionLabel: '数据重放',
    description: '按 Topic、offset、时间窗和 schema 校验结果创建重放任务；fallback DLQ 默认先走 dry-run 预检。',
    impact: '影响 Kafka/Flink/ClickHouse 对账链路，需限制批次、审批人和幂等策略。',
    audit: `${dlqReplayActionPlan?.auditEvent ?? 'dlq_replay_approved'} 写入 replay audit trail。`,
    danger: true,
    fields: [
      ['接口', actionEndpointLabel(dlqReplayActionPlan)],
      ['权限', actionScopeLabel(dlqReplayActionPlan)],
      ['默认模式', 'dry_run=true 预检'],
      ['幂等策略', 'idempotency_key Redis-backed 24h TTL'],
      ['审批约束', 'approved_by 必须不同于 requested_by'],
    ],
  },
];

const reportMetricLabels = ['日报评分', '验收通过率', '异常归因', '待补证据', '已导出', 'SLA 达成'];
const settingsMetricLabels = ['启用规则', '阈值组', '告警策略', '报告周期', '待审批变更', '最近保存', '审计完整'];

const dataQualityTabs = [
  { label: '质量总览', slug: 'overview' },
  { label: 'Topic 健康', slug: 'topic-health' },
  { label: 'Flink 质量', slug: 'flink-quality' },
  { label: '字段质量', slug: 'field-quality' },
  { label: '存储质量', slug: 'storage-quality' },
  { label: '重放对账', slug: 'replay-reconcile' },
  { label: '质量报告', slug: 'report' },
  { label: '质量设置', slug: 'settings' },
] as const;

type DataQualityTabSlug = (typeof dataQualityTabs)[number]['slug'];

type FieldQualityDetail = {
  title: string;
  description: string;
  columns: string[];
  rows: string[][];
  actionLabel?: string;
  actionSuccessMessage?: string;
  actionName?: string;
  actionTarget?: string;
};

type OpenFieldQualityDetail = (detail: FieldQualityDetail, focusSelector?: string) => void;

const metricToneClass = {
  ok: 'is-ok',
  warn: 'is-warn',
  risk: 'is-risk',
  info: 'is-info',
};

const resolveDataQualityTab = (param: string | null): DataQualityTabSlug => dataQualityTabs.find((item) => item.slug === param)?.slug ?? 'overview';

const dataQualityCheckTargets: Record<string, { label: string; tab: DataQualityTabSlug }> = {
  flow_rate: { label: '数据流入率', tab: 'topic-health' },
  data_completeness: { label: '数据完整性', tab: 'field-quality' },
  end_to_end_latency: { label: '端到端延迟', tab: 'flink-quality' },
  schema_drift: { label: 'Schema 漂移', tab: 'field-quality' },
  kafka_lag_proxy: { label: 'Kafka 积压代理', tab: 'topic-health' },
  kafka_consumer_lag: { label: 'Kafka 消费积压', tab: 'topic-health' },
  flink_event_time_watermark: { label: 'Flink 事件时间水位', tab: 'flink-quality' },
  sink_commit_watermark: { label: '存储提交水位', tab: 'storage-quality' },
};

const DataQualityServerPaginationContext = createContext(false);
const DataQualityScrollTablesContext = createContext(false);

export function DataQualityPage({ route }: { route: NavRoute }) {
  const [dlqReplayForm] = Form.useForm<DLQReplayFallbackRequest>();
  const [searchParams, setSearchParams] = useSearchParams();
  const [dlqSampleOpen, setDlqSampleOpen] = useState(false);
  const [dlqReplayOpen, setDlqReplayOpen] = useState(false);
  const [dlqReplayResult, setDlqReplayResult] = useState<DLQReplayFallbackResult | null>(null);
  const [fieldDetail, setFieldDetail] = useState<FieldQualityDetail | null>(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [timeRange, setTimeRange] = useState<DataQualityTimeRange>('近 24 小时');
  const isVisualBreakdown = import.meta.env.DEV && searchParams.has('__codex_ui_breakdown_production');
  const activeTab = resolveDataQualityTab(searchParams.get('tab'));
  const isTopicHealthTab = activeTab === 'topic-health';
  const isFlinkQualityTab = activeTab === 'flink-quality';
  const isFieldQualityTab = activeTab === 'field-quality';
  const isStorageQualityTab = activeTab === 'storage-quality';
  const isReplayReconcileTab = activeTab === 'replay-reconcile';
  const useScrollTables = true;
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, timeRange],
    queryFn: () => fetchPageSnapshot(route.id, { dataQualityTimeRange: timeRange }),
    refetchInterval: autoRefresh && !isVisualBreakdown ? 30_000 : false,
    refetchIntervalInBackground: autoRefresh && !isVisualBreakdown,
  });
  const dailyReportQuery = useQuery({
    queryKey: ['data-quality-daily-report', timeRange],
    queryFn: () => fetchDataQualityDailyReport(timeRange),
    enabled: activeTab === 'report' && !isVisualBreakdown,
    staleTime: 15_000,
  });
  const dlqReplayMutation = useMutation({
    mutationFn: requestDLQFallbackReplay,
    onSuccess: async (result) => {
      setDlqReplayResult(result);
      message.success(result.status === 'dry_run' ? 'DLQ dry-run 预检完成' : 'DLQ 重放请求已提交');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(errorText(mutationError));
    },
  });
  const dataQualityActionMutation = useMutation({
    mutationFn: submitDataQualityAction,
    onSuccess: async (result) => {
      message.info(`${result.action} 已受理（${result.status}，action_id=${result.action_id}）；最终执行结果仍需等待执行器回执与对账。`);
      setFieldDetail(null);
      await refetch();
    },
    onError: (mutationError) => {
      message.error(errorText(mutationError));
    },
  });
  const downloadDailyReport = async (format: 'pdf' | 'json' | 'csv') => {
    try {
      const { blob, filename } = await downloadDataQualityDailyReport(timeRange, format);
      const href = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = href;
      anchor.download = filename;
      anchor.style.display = 'none';
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(href);
      message.success(`数据质量日报已下载：${filename}`);
    } catch (downloadError) {
      message.error(errorText(downloadError));
    }
  };

  const snapshot = data;
  const rows = useMemo(() => snapshot?.rows ?? [], [snapshot?.rows]);
  const dataQualityVisuals = snapshot?.visuals?.dataQuality;
  const requiresVisualDataset = activeTab !== 'report' && activeTab !== 'settings';
  const visualDatasetUnavailable = requiresVisualDataset && !isVisualBreakdown && !isLoading && !isError && !dataQualityVisuals;
  const activeTabLabel = dataQualityTabs.find((tab) => tab.slug === activeTab)?.label ?? '质量总览';
  const topicMetricSource = dataQualityVisuals?.topicMetrics ?? [];
  const topicMetricLabels = topicMetricSource.map((item) => item.label);
  const flinkMetricSource = dataQualityVisuals?.flinkKpis ?? [];
  const flinkMetricLabels = flinkMetricSource.map((item) => item.label);
  const fieldMetricSource = dataQualityVisuals?.fieldKpis ?? [];
  const fieldMetricLabels = fieldMetricSource.map((item) => item.label);
  const fieldKpiTrends = dataQualityVisuals?.fieldKpiTrends ?? [];
  const storageMetricSource = dataQualityVisuals?.storageKpis ?? [];
  const storageMetricLabels = storageMetricSource.map((item) => item.label);
  const replayMetricSource = dataQualityVisuals?.replayKpis ?? [];
  const replayMetricLabels = replayMetricSource.map((item) => item.label);
  const metricLabels = activeTab === 'report' ? reportMetricLabels : activeTab === 'settings' ? settingsMetricLabels : isTopicHealthTab ? topicMetricLabels : isFlinkQualityTab ? flinkMetricLabels : isFieldQualityTab ? fieldMetricLabels : isStorageQualityTab ? storageMetricLabels : isReplayReconcileTab ? replayMetricLabels : route.page.kpis;
  const metricSource = activeTab === 'report'
    ? (dailyReportQuery.data?.kpis.map((item) => ({ ...item, delta: item.delta ?? '实时生成' })) ?? [])
    : activeTab === 'settings' ? [] : isTopicHealthTab ? topicMetricSource : isFlinkQualityTab ? flinkMetricSource : isFieldQualityTab ? fieldMetricSource : isStorageQualityTab ? storageMetricSource : isReplayReconcileTab ? replayMetricSource : (snapshot?.metrics ?? []);
  const metrics = metricLabels.map((label) => metricSource.find((item) => item.label === label) ?? unavailableMetric(label));
  const parsedQualityScore = Number.parseFloat(metrics[0]?.value ?? '');
  const qualityScore = Number.isFinite(parsedQualityScore) ? parsedQualityScore : null;
  const openDlqReplay = () => {
    dlqReplayForm.setFieldsValue(defaultDLQReplayRequest());
    setDlqReplayResult(null);
    setDlqReplayOpen(true);
    if (activeTab !== 'replay-reconcile') {
      setSearchParams((current) => mergeRouteSearchParams(current, { tab: 'replay-reconcile' }));
    }
  };
  const navigateToTab = (tab: DataQualityTabSlug) => {
    setSearchParams((current) => mergeRouteSearchParams(current, { tab }));
  };
  const submitDlqReplay = (values: DLQReplayFallbackRequest) => {
    dlqReplayMutation.mutate(
      buildDLQReplayDryRunRequest({
        ...values,
        requested_at_unix: Math.floor(Date.now() / 1000),
      }),
    );
  };
  const openFieldDetail: OpenFieldQualityDetail = (detail, focusSelector) => {
    setFieldDetail({
      ...detail,
      columns: [...detail.columns, '接口预留', '审计事件'],
      rows: detail.rows.map((row) => [...row, actionEndpointLabel(dataQualityContextActionPlan), dataQualityContextActionPlan?.auditEvent ?? 'DATA_QUALITY_ACTION_REQUESTED']),
    });
    if (focusSelector) {
      window.setTimeout(() => {
        document.querySelector(focusSelector)?.scrollIntoView({ behavior: 'smooth', block: 'center' });
      }, 0);
    }
  };
  const openUnboundBusinessAction = (event: MouseEvent<HTMLElement>) => {
    const target = event.target as HTMLElement;
    const button = target.closest('button');
    if (!button || button.disabled || button.dataset.dqActionManaged === 'true') return;
    if (button.closest('.ant-drawer, .ant-modal')) return;
    if (button.closest('.taf-data-quality-field-sample-table, .taf-data-quality-field-repair-table, .taf-data-quality-field-lineage, .taf-data-quality-field-rail')) return;
    const label = (button.getAttribute('aria-label') || button.getAttribute('title') || button.textContent || '').replace(/\s+/g, ' ').trim();
    if (!label) return;
    const actionable = /创建|导出|修复|保存|同步|更新|配置|审批|重放|重试|回滚|清理/.test(label);
    setFieldDetail({
      title: label,
      description: actionable ? `已加载“${label}”的 dry-run 预检；提交后只代表服务端受理，不代表动作最终执行成功。` : `已打开“${label}”的业务上下文，可继续查看当前 ${activeTabLabel} 数据。`,
      columns: ['操作', '当前视图', '接口预留', '审计事件', '处理结果'],
      rows: [[label, activeTabLabel, actionEndpointLabel(dataQualityContextActionPlan), dataQualityContextActionPlan?.auditEvent ?? 'DATA_QUALITY_ACTION_REQUESTED', actionable ? '确认后提交 dry-run 预检' : '已定位关联业务数据']],
      actionLabel: actionable ? '提交预检' : undefined,
      actionName: label,
      actionTarget: activeTabLabel,
    });
  };

  return (
    <DataQualityServerPaginationContext.Provider value={!isVisualBreakdown && Boolean(dataQualityVisuals)}>
      <DataQualityScrollTablesContext.Provider value={useScrollTables}>
      <div className="taf-page taf-data-quality" data-business-action-delegate="data-quality-context" onClick={openUnboundBusinessAction}>
        <section className={`taf-data-quality-shell is-unified-tabs${useScrollTables ? ' is-scroll-table-mode' : ''}${activeTab === 'report' ? ' is-report-tab' : ''}${activeTab === 'settings' ? ' is-settings-tab' : ''}${isTopicHealthTab ? ' is-topic-health-tab' : ''}${isFlinkQualityTab ? ' is-flink-quality-tab' : ''}${isFieldQualityTab ? ' is-field-quality-tab' : ''}${isStorageQualityTab ? ' is-storage-quality-tab' : ''}${isReplayReconcileTab ? ' is-replay-reconcile-tab' : ''}`}>
          <header className="taf-data-quality-titlebar">
            <div className="taf-data-quality-heading">
              <h1 title={route.page.title}>{route.page.title}</h1>
              <span title={`当前视图：${activeTabLabel}`}>当前视图：{activeTabLabel}</span>
            </div>
            <nav className="taf-data-quality-tabs" aria-label="数据质量视图">
              {dataQualityTabs.map((tab, index) => (
                <button
                  key={tab.slug}
                  type="button"
                  className={tab.slug === activeTab ? 'is-active' : ''}
                  aria-selected={tab.slug === activeTab}
                  aria-label={tab.label}
                  data-tab-slot={index + 1}
                  data-tab-slug={tab.slug}
                  role="tab"
                  title={tab.label}
                  data-dq-action-managed="true"
                  onClick={() => navigateToTab(tab.slug)}
                >
                  {tab.label}
                </button>
              ))}
            </nav>
            <Space className="taf-data-quality-toolbar-actions" size={6}>
              {isTopicHealthTab || isFlinkQualityTab || isFieldQualityTab || isStorageQualityTab || isReplayReconcileTab ? null : activeTab === 'report' ? (
                <>
                  <span className="taf-data-quality-report-toolbar-label">报告版本</span>
                  <span className="taf-data-quality-report-toolbar-label">{dailyReportQuery.data?.version ?? '等待服务端报告'}</span>
                </>
              ) : isVisualBreakdown ? (
                <>
                  <span className="taf-data-quality-toolbar-label">时间范围</span>
                  <Select size="small" value="近 24 小时" options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} />
                  <Tooltip title="刷新质量报表">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                  <Select size="small" value="30s" options={[{ value: '30s' }, { value: '60s' }, { value: '5min' }]} />
                  <Tooltip title="全屏查看">
                    <Button size="small" icon={<FullscreenOutlined />} />
                  </Tooltip>
                </>
              ) : (
                <>
                  <Select size="small" value="近 24 小时" options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} />
                  <Select size="small" value="全部管道" options={[{ value: '全部管道' }, { value: '采集链路' }, { value: '检测链路' }]} />
                  <Tooltip title="刷新质量报表">
                    <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
                  </Tooltip>
                </>
              )}
              {activeTab !== 'report' && !isVisualBreakdown && <OverlayContractHost overlays={dataQualityOverlays} compact />}
            </Space>
          </header>

          <main className="taf-data-quality-main">
            {!isVisualBreakdown && isError && (
              <Alert
                type="error"
                showIcon
                message="真实 API 数据加载失败"
                description={error instanceof Error ? error.message : '请检查 /v1/data-quality、APISIX 路由或 alert-service dataquality monitor。'}
                action={
                  <Button size="small" danger onClick={() => void refetch()}>
                    重试
                  </Button>
                }
              />
            )}

            {visualDatasetUnavailable && <Alert type="warning" showIcon message="数据质量可视化数据集未激活" description="实时 ClickHouse 质量检查已返回，但 PostgreSQL 中没有当前租户的激活式 data_quality_ui_fixtures 数据；页面不会使用前端静态数据替代生产 API。" />}

            <DataQualityFilterBar activeTab={activeTab} autoRefresh={autoRefresh} onAutoRefreshChange={setAutoRefresh} onRefresh={() => void refetch()} timeRange={timeRange} onTimeRangeChange={setTimeRange} />

            {!visualDatasetUnavailable && (
              <>
                <div className={`taf-data-quality-kpis is-unified${activeTab === 'report' ? ' is-report' : ''}${activeTab === 'settings' ? ' is-settings' : ''}${isFlinkQualityTab ? ' is-flink' : ''}${isFieldQualityTab ? ' is-field' : ''}${isStorageQualityTab ? ' is-storage' : ''}${isReplayReconcileTab ? ' is-replay' : ''}`}>
                  {metrics.map((metric, index) => (
                    <DataQualityMetricTile key={metric.label} metric={metric} index={index} fieldKpiTrend={isFieldQualityTab ? fieldKpiTrends[index] : undefined} />
                  ))}
                </div>

                <DataQualityTabView
                  activeTab={activeTab}
                  dailyReport={dailyReportQuery.data}
                  dailyReportError={dailyReportQuery.error}
                  dataQualityVisuals={dataQualityVisuals}
                  evidence={snapshot?.evidence ?? []}
                  isLoading={!isVisualBreakdown && isLoading}
                  isReportLoading={dailyReportQuery.isLoading}
                  onDownloadReport={downloadDailyReport}
                  onOpenFieldDetail={openFieldDetail}
                  onOpenReplay={openDlqReplay}
                  qualityScore={qualityScore}
                  rows={rows}
                />
              </>
            )}
          </main>

          {visualDatasetUnavailable ? (
            <aside className="taf-data-quality-rail">
              <WorkPanel title="数据来源">
                <Alert type="warning" showIcon message="等待激活租户数据集" />
              </WorkPanel>
            </aside>
          ) : activeTab === 'report' ? (
            <ReportSideRail report={dailyReportQuery.data} />
          ) : activeTab === 'settings' ? (
            <SettingsSideRail />
          ) : activeTab === 'topic-health' ? (
            <TopicHealthSideRail />
          ) : isFlinkQualityTab ? (
            <FlinkQualitySideRail />
          ) : isFieldQualityTab ? (
            <FieldQualitySideRail />
          ) : isStorageQualityTab ? (
            <StorageQualitySideRail dataQualityVisuals={dataQualityVisuals} />
          ) : isReplayReconcileTab ? (
            <ReplayReconcileSideRail dataQualityVisuals={dataQualityVisuals} />
          ) : (
            <aside className="taf-data-quality-rail">
              <WorkPanel title="质量异常告警（当前快照）">
                <QualityAnomalies checks={snapshot?.dataQualityChecks} onNavigate={navigateToTab} />
              </WorkPanel>
              <WorkPanel title="快速定位">
                <QualityCheckLinks checks={snapshot?.dataQualityChecks} mode="locate" onNavigate={navigateToTab} />
              </WorkPanel>
              <WorkPanel title="质量修复建议">
                <QualityCheckLinks checks={snapshot?.dataQualityChecks} mode="repair" onNavigate={navigateToTab} />
              </WorkPanel>
              <WorkPanel title="快收证据与报告">
                <EvidenceActions evidence={snapshot?.evidence ?? []} />
              </WorkPanel>
            </aside>
          )}
        </section>
        <Drawer className="taf-data-quality-dlq-sample-drawer" title="DLQ 样本详情" placement="right" width={dlqSampleDrawerWidth} open={dlqSampleOpen} closeIcon={<CloseOutlined title="关闭弹窗" />} onClose={() => setDlqSampleOpen(false)}>
          <Alert type="info" showIcon message="DLQ 样本只读预检" description={`影响范围：dlq.v1 待重放样本；${actionEndpointLabel(dlqReplayActionPlan)} 需要 ${actionScopeLabel(dlqReplayActionPlan)}，执行前必须完成 schema drift 校验、审批确认、幂等 key 和审计 trace。`} />
          <Alert type="warning" showIcon message="当前响应未提供 DLQ 样本" description="页面不会用静态任务、offset 或异常字段替代真实样本。请先由服务端返回租户、Topic、offset 窗口、schema 校验与 trace 后再执行预检。" />
        </Drawer>
        <Drawer className="taf-data-quality-field-detail-drawer" title={fieldDetail?.title ?? '字段质量详情'} placement="right" width={fieldDetailDrawerWidth} open={Boolean(fieldDetail)} closeIcon={<CloseOutlined title="关闭详情" />} onClose={() => setFieldDetail(null)}>
          {fieldDetail && (
            <>
              <Alert type="info" showIcon message={fieldDetail.title} description={fieldDetail.description} />
              <DenseRows columns={fieldDetail.columns} rows={fieldDetail.rows} />
              {fieldDetail.actionLabel && (
                <Button
                  type="primary"
                  block
                  className="taf-data-quality-field-detail-action"
                  loading={dataQualityActionMutation.isPending}
                  onClick={() => {
                    dataQualityActionMutation.mutate({
                      view: activeTab,
                      action: fieldDetail.actionName ?? fieldDetail.title,
                      target: fieldDetail.actionTarget ?? fieldDetail.title,
                      dry_run: true,
                      reason: `从${activeTabLabel}页面提交的操作预检`,
                      parameters: {
                        source: 'data-quality-workspace',
                        label: fieldDetail.actionLabel,
                      },
                    });
                  }}
                >
                  {fieldDetail.actionLabel}
                </Button>
              )}
            </>
          )}
        </Drawer>
        <Modal className="taf-data-quality-replay-modal" title="DLQ fallback replay dry-run" open={dlqReplayOpen} width={dlqReplayModalWidth} closeIcon={<CloseOutlined title="关闭弹窗" />} onCancel={() => setDlqReplayOpen(false)} onOk={() => dlqReplayForm.submit()} okText="执行 dry-run 预检" cancelText="关闭" confirmLoading={dlqReplayMutation.isPending}>
          <Alert type="warning" showIcon message="高风险重放动作默认只做 dry-run" description={`${actionEndpointLabel(dlqReplayActionPlan)} 会验证审批人、修复摘要、幂等键、scope 和审计链；切换执行模式前应先完成 dry-run 证据归档。`} />
          <Form className="taf-data-quality-replay-form" form={dlqReplayForm} layout="vertical" onFinish={submitDlqReplay}>
            <div className="taf-data-quality-replay-form-grid">
              <Form.Item label="审批人" name="approved_by" rules={[{ required: true, message: '请输入审批人' }]}>
                <Input placeholder="operator-2" />
              </Form.Item>
              <Form.Item label="审批单号" name="approval_id" rules={[{ required: true, message: '请输入审批单号' }]}>
                <Input placeholder="APPROVAL-20260629-DQ-001" />
              </Form.Item>
              <Form.Item label="幂等键" name="idempotency_key" rules={[{ required: true, message: '请输入幂等键' }]}>
                <Input placeholder="tenant-a:APPROVAL-20260629-DQ-001:dry-run" />
              </Form.Item>
              <Form.Item label="dry-run" name="dry_run" valuePropName="checked">
                <Switch checkedChildren="预检" unCheckedChildren="执行" />
              </Form.Item>
            </div>
            <Form.Item label="重放原因" name="reason" rules={[{ required: true, message: '请输入重放原因' }]}>
              <Input.TextArea rows={2} placeholder="schema repair 后验证 fallback 文件可安全回放" />
            </Form.Item>
            <Form.Item label="修复摘要" name="repair_summary" rules={[{ required: true, message: '请输入修复摘要' }]}>
              <Input.TextArea rows={2} placeholder="已修复 malformed event payload，先执行 dry-run 预检" />
            </Form.Item>
          </Form>
          {dlqReplayResult && (
            <div className="taf-data-quality-replay-result">
              <Alert type={dlqReplayResult.failed_files ? 'warning' : 'success'} showIcon message={`Replay ${dlqReplayResult.status}`} description={`replay_id=${dlqReplayResult.replay_id}，fallback 文件 ${dlqReplayResult.pre_fallback_files}，待重放字节 ${formatBytes(dlqReplayResult.pre_fallback_bytes)}，审计 ${dlqReplayResult.audit_trail.length} 条。`} />
            </div>
          )}
        </Modal>
      </div>
      </DataQualityScrollTablesContext.Provider>
    </DataQualityServerPaginationContext.Provider>
  );
}

function DataQualityMetricTile({ fieldKpiTrend, metric, index }: { fieldKpiTrend?: number[]; metric: PageSnapshot['metrics'][number]; index: number }) {
  const up = !metric.delta.includes('↓') && !metric.delta.startsWith('-');
  const isScore = index === 0;
  const isOverallScore = metric.label === '质量总分';
  const isReportScore = metric.label === '日报评分';
  const isSettingsScore = metric.label === '启用规则';
  const isTopicScore = metric.label === 'Topic 健康分';
  const isFlinkScore = metric.label === 'Flink 质量分';
  const isFieldScore = metric.label === '字段质量分';
  const isStorageScore = metric.label === '存储质量分';
  const isReplayScore = metric.label === '对账通过率';
  const trendValues = fieldKpiTrend ?? [];
  const scoreParts = isScore ? metric.value.match(/^(\d+(?:\.\d+)?)(.*)$/) : null;
  if (isOverallScore) {
    const scoreValue = scoreParts
      ? Math.max(0, Math.min(100, Number.parseFloat(scoreParts[1]) || 0))
      : null;
    return (
      <div className={`taf-metric taf-data-quality-metric is-score is-overall-score ${metricToneClass[metric.status]}`} title={`${metric.label} ${metric.value} ${metric.delta}`}>
        <span>{metric.label}</span>
        <div className={`taf-data-quality-overall-score-ring${scoreValue === null ? ' is-unavailable' : ''}`} aria-label={scoreValue === null ? '质量总分暂不可用' : undefined}>
          {scoreValue === null ? '--' : <RingChart value={scoreValue} height="100%" suffix="分" ariaLabel={`质量总分 ${scoreValue} 分`} />}
        </div>
        <div className="taf-data-quality-overall-score-copy">
          <strong>{scoreValue === null ? '暂不可用' : scoreValue >= 90 ? '良好' : scoreValue >= 75 ? '关注' : '异常'}</strong>
          <small>{up ? <ArrowUpOutlined /> : <ArrowDownOutlined />}{metric.delta}</small>
        </div>
      </div>
    );
  }
  return (
    <div className={`taf-metric taf-data-quality-metric ${metricToneClass[metric.status]}${isScore ? ' is-score' : ''}${isReportScore ? ' is-report-score' : ''}${isSettingsScore ? ' is-settings-score' : ''}${isFlinkScore ? ' is-flink-score' : ''}${isFieldScore ? ' is-field-score' : ''}${isStorageScore ? ' is-storage-score' : ''}${isReplayScore ? ' is-replay-score' : ''}`} title={`${metric.label} ${metric.value} ${metric.delta}`}>
      <span>{metric.label}</span>
      <strong>
        {scoreParts ? (
          <>
            <b>{scoreParts[1]}</b>
            <em>{scoreParts[2].trim()}</em>
          </>
        ) : (
          metric.value
        )}
      </strong>
      <small>
        {up ? <ArrowUpOutlined /> : <ArrowDownOutlined />}
        {metric.delta}
      </small>
      {!isScore && trendValues.length > 0 && <DataQualityKpiSparklineChart ariaLabel={`${metric.label}趋势`} className="taf-data-quality-field-kpi-echart taf-data-quality-kpi-echart" tone={metric.status} values={trendValues} />}
      {isTopicScore && <SafetyCertificateOutlined className="taf-data-quality-topic-score-icon" />}
      {isFlinkScore && <SafetyCertificateOutlined className="taf-data-quality-flink-score-icon" />}
      {isFieldScore && <SafetyCertificateOutlined className="taf-data-quality-field-score-icon" />}
      {isStorageScore && <SafetyCertificateOutlined className="taf-data-quality-storage-score-icon" />}
      {isReplayScore && <SafetyCertificateOutlined className="taf-data-quality-replay-score-icon" />}
      {isReportScore && <SafetyCertificateOutlined className="taf-data-quality-report-score-icon" />}
      {isSettingsScore && <SafetyCertificateOutlined className="taf-data-quality-settings-score-icon" />}
    </div>
  );
}

function DataQualityFilterBar({ activeTab, autoRefresh, onAutoRefreshChange, onRefresh, onTimeRangeChange, timeRange }: { activeTab: DataQualityTabSlug; autoRefresh: boolean; onAutoRefreshChange: (enabled: boolean) => void; onRefresh: () => void; onTimeRangeChange: (range: DataQualityTimeRange) => void; timeRange: DataQualityTimeRange }) {
  const activeTabLabel = dataQualityTabs.find((tab) => tab.slug === activeTab)?.label ?? '质量总览';
  const rangeLabel = `${timeRange}（由服务端快照确定）`;
  return (
    <div className="taf-data-quality-filterbar" data-tab={activeTab}>
      <span title="时间范围">时间范围</span>
      <Select<DataQualityTimeRange> size="small" value={timeRange} options={[{ value: '近 24 小时' }, { value: '近 7 天' }]} onChange={onTimeRangeChange} />
      <button type="button" className="taf-data-quality-filter-range" title={`${activeTabLabel} ${rangeLabel}`}>
        {rangeLabel} <FieldTimeOutlined />
      </button>
      <span className="taf-data-quality-filter-spacer" />
      <span title="自动刷新">自动刷新</span>
      <button type="button" className={`taf-data-quality-auto-toggle${autoRefresh ? '' : ' is-off'}`} title={`自动刷新 已${autoRefresh ? '开启' : '关闭'}`} aria-label={`自动刷新 已${autoRefresh ? '开启' : '关闭'}`} aria-pressed={autoRefresh} data-dq-action-managed="true" onClick={() => onAutoRefreshChange(!autoRefresh)}>
        <span />
      </button>
      <Tooltip title={`刷新${activeTabLabel}数据`}>
        <Button size="small" icon={<ReloadOutlined />} data-dq-action-managed="true" onClick={onRefresh}>
          刷新
        </Button>
      </Tooltip>
    </div>
  );
}

function DataQualityTabView({ activeTab, dailyReport, dailyReportError, dataQualityVisuals, evidence, isLoading, isReportLoading, onDownloadReport, onOpenFieldDetail, onOpenReplay, qualityScore, rows }: {
  activeTab: DataQualityTabSlug;
  dailyReport?: DataQualityDailyReport;
  dailyReportError: Error | null;
  dataQualityVisuals?: DataQualityVisuals;
  evidence: PageSnapshot['evidence'];
  isLoading: boolean;
  isReportLoading: boolean;
  onDownloadReport: (format: 'pdf' | 'json' | 'csv') => Promise<void>;
  onOpenFieldDetail: OpenFieldQualityDetail;
  onOpenReplay: () => void;
  qualityScore: number | null;
  rows: SnapshotRow[];
}) {
  if (activeTab === 'topic-health') {
    return <TopicHealthContent dataQualityVisuals={dataQualityVisuals} isLoading={isLoading} qualityScore={qualityScore} rows={rows} />;
  }
  if (activeTab === 'flink-quality') {
    return <FlinkQualityContent dataQualityVisuals={dataQualityVisuals} />;
  }
  if (activeTab === 'field-quality') {
    return <FieldQualityContent dataQualityVisuals={dataQualityVisuals} onOpenDetail={onOpenFieldDetail} />;
  }
  if (activeTab === 'storage-quality') {
    return <StorageQualityContent dataQualityVisuals={dataQualityVisuals} />;
  }
  if (activeTab === 'replay-reconcile') {
    return <ReplayReconcileContent dataQualityVisuals={dataQualityVisuals} onOpenReplay={onOpenReplay} />;
  }
  if (activeTab === 'report') {
    return <ReportContent error={dailyReportError} isLoading={isReportLoading} onDownload={onDownloadReport} report={dailyReport} />;
  }
  if (activeTab === 'settings') {
    return <DataUnavailable section="质量设置" />;
  }
  return <QualityOverviewContent dataQualityVisuals={dataQualityVisuals} evidence={evidence} isLoading={isLoading} qualityScore={qualityScore} rows={rows} />;
}

function QualityOverviewContent({ dataQualityVisuals, evidence, isLoading, qualityScore, rows }: { dataQualityVisuals?: DataQualityVisuals; evidence: PageSnapshot['evidence']; isLoading: boolean; qualityScore: number | null; rows: SnapshotRow[] }) {
  return (
    <>
      <div className="taf-data-quality-overview-grid">
        <WorkPanel title="Kafka Topic 健康 (Top 10)" className="taf-data-quality-topic-panel taf-data-quality-overview-topic">
          <DataQualityTopicGrid isLoading={isLoading} rows={rows} />
        </WorkPanel>
        <WorkPanel title="Topic 分区倾斜热力图" className="taf-data-quality-overview-heat" extra={<span className="taf-data-quality-score">{formatQualityScore(qualityScore)}</span>}>
          <TopicHeatmap visuals={dataQualityVisuals} />
        </WorkPanel>
        <WorkPanel title="Flink 处理质量概览" className="taf-data-quality-overview-flink">
          <FlinkQuality evidence={evidence} visuals={dataQualityVisuals} />
        </WorkPanel>
        <WorkPanel title="字段质量矩阵（近 24 小时）" className="taf-data-quality-overview-field">
          <FieldQuality rows={dataQualityVisuals?.fieldQualityRows} />
        </WorkPanel>
        <WorkPanel title="存储写入质量" className="taf-data-quality-overview-storage">
          <StorageQualityOverview rows={dataQualityVisuals?.storageComponentRows} />
        </WorkPanel>
        <WorkPanel title="对账报告（近 24 小时）" className="taf-data-quality-overview-reconcile">
          <ReconciliationReport rows={dataQualityVisuals?.replayReconcileSummary} />
        </WorkPanel>
      </div>
    </>
  );
}

const overviewTopicColumns = ['Topic', '分区数', '当前吞吐量', '消费延迟', '积压量', '积压趋势', '消费延迟 P95', '分区倾斜', '消息延迟 P95', '操作'];

function DataQualityTopicGrid({ isLoading, rows }: { isLoading: boolean; rows: SnapshotRow[] }) {
  if (isLoading && rows.length === 0) {
    return <div className="taf-data-quality-topic-grid is-loading">加载 Topic 健康数据...</div>;
  }

  return (
    <div className="taf-data-quality-topic-grid taf-data-quality-scroll-table" style={{ '--dq-topic-columns': overviewTopicColumns.length } as CSSProperties}>
      <div className="taf-data-quality-topic-grid-head">
        {overviewTopicColumns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      <div className="taf-data-quality-scroll-body">
        {rows.slice(0, 10).map((row, index) => (
          <div key={String(row.Topic ?? index)} className="taf-data-quality-topic-grid-row">
            {overviewTopicColumns.map((column) => {
              const value = String(row[column] ?? '--');
              if (column === 'Topic') {
                return <strong key={column} title={value}>{value}</strong>;
              }
              if (column === '积压趋势') {
                return (
                  <span key={column} className={`taf-data-quality-topic-trend ${value === '上升' ? 'is-risk' : value === '波动' ? 'is-warn' : 'is-ok'}`} title={value}>
                    <em>{value}</em>
                  </span>
                );
              }
              if (column === '操作') {
                return <span key={column} className={`taf-data-quality-topic-state ${value === '危急' ? 'is-risk' : value === '中等' ? 'is-warn' : 'is-ok'}`} title={value}>{value}</span>;
              }
              return <span key={column} title={value}>{value}</span>;
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

function TopicHealthContent({ compact = false, dataQualityVisuals, isLoading, qualityScore, rows }: { compact?: boolean; dataQualityVisuals?: DataQualityVisuals; isLoading: boolean; qualityScore: number | null; rows: SnapshotRow[] }) {
  return (
    <>
      <div className="taf-data-quality-upper">
        <WorkPanel title="Kafka Topic 健康明细" className="taf-data-quality-topic-panel taf-data-quality-topic-health-table-panel">
          {isLoading && rows.length === 0 ? <div className="taf-data-quality-topic-grid is-loading">加载 Topic 健康数据...</div> : <TopicHealthTable rows={rows} />}
        </WorkPanel>

        <WorkPanel title="消费延迟趋势" className="taf-data-quality-trend-panel" extra={<span className="taf-data-quality-trend-legend">P50 / P95 / 阈值</span>}>
          <LatencyTrend />
        </WorkPanel>

        <WorkPanel title="分区倾斜热力图（flow_original）" className="taf-data-quality-topic-health-heat-panel" extra={<span className="taf-data-quality-score">{formatQualityScore(qualityScore)}</span>}>
          <TopicHeatmap visuals={dataQualityVisuals} />
        </WorkPanel>
      </div>
      {!compact && (
        <div className="taf-data-quality-tab-grid">
          <WorkPanel title="Consumer Group 健康">
            <DenseRows columns={['Consumer Group', '当前 Lag', 'Rebalance', '最后提交', '状态']} dataset="consumerRows" rows={dataQualityVisuals?.consumerRows ?? []} />
          </WorkPanel>
          <WorkPanel title="消息大小吞吐分布（24h）">
            <MessageSizeDistribution visuals={dataQualityVisuals} />
          </WorkPanel>
          <WorkPanel title="异常分区处置队列">
            <PartitionQueue
              rows={dataQualityVisuals?.partitionQueueRows ?? []}
            />
          </WorkPanel>
        </div>
      )}
    </>
  );
}

const topicHealthColumns = ['Topic', '分区数', '当前 offset', '积压', '消费延迟P95', '分区倾斜', '消息大小', '状态', '操作'];

function TopicHealthTable({ rows }: { rows: SnapshotRow[] }) {
  return (
    <div
      className="taf-data-quality-topic-health-table taf-data-quality-scroll-table"
      style={
        {
          '--dq-topic-health-columns': topicHealthColumns.length,
        } as CSSProperties
      }
    >
      <div className="taf-data-quality-topic-health-head">
        {topicHealthColumns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      <div className="taf-data-quality-scroll-body">
        {rows.map((row) => (
          <div key={String(row.Topic)} className="taf-data-quality-topic-health-row">
            {topicHealthColumns.map((column) => {
              const rawValue = column === '分区倾斜' ? row.分区倾斜度 : row[column];
              const value = String(rawValue ?? '--');
              if (column === 'Topic') return <strong key={column} title={value}>{value}</strong>;
              if (column === '状态') return <span key={column} className={`taf-data-quality-topic-health-state ${value === '严重' ? 'is-risk' : value === '告警' ? 'is-warn' : 'is-ok'}`} title={value}>{value}</span>;
              if (column === '操作') return <span key={column} className="taf-data-quality-topic-health-op" title={`查看 ${String(row.Topic)}`}><FileSearchOutlined /></span>;
              return <span key={column} className={(column.includes('延迟') || column === '分区倾斜') && (value.includes('15') || value.includes('2.') || value.includes('5.')) ? 'is-warn' : ''} title={value}>{value}</span>;
            })}
          </div>
        ))}
      </div>
    </div>
  );
}

function FlinkQualityContent({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  return (
    <>
      <div className="taf-data-quality-flink-upper">
        <WorkPanel title="Flink 作业健康明细">
          <FlinkJobHealthTable rows={dataQualityVisuals?.flinkJobRows} />
        </WorkPanel>
        <WorkPanel title="Checkpoint 与 Watermark 趋势">
          <FlinkCheckpointWatermarkTrend trend={dataQualityVisuals?.flinkCheckpointTrend} />
        </WorkPanel>
        <WorkPanel title="Backpressure 热力图 (按作业 / Subtask)">
          <FlinkBackpressureHeatmap buckets={dataQualityVisuals?.flinkBackpressureBuckets} rows={dataQualityVisuals?.flinkBackpressureRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-flink-lower">
        <WorkPanel title="迟到数据与窗口闭合（按来源 Topic）">
          <LateWindowClosure topicRows={dataQualityVisuals?.flinkLateTopicRows} windowRows={dataQualityVisuals?.flinkWindowRows} />
        </WorkPanel>
        <WorkPanel title="异常与失败原因（Top 10）">
          <FlinkFailureTable rows={dataQualityVisuals?.flinkFailureRows} />
        </WorkPanel>
        <WorkPanel title="Sink 写入质量（近 24h）">
          <SinkQualityCards rows={dataQualityVisuals?.flinkSinkRows} />
        </WorkPanel>
      </div>
    </>
  );
}

const flinkJobColumns = ['作业', '状态', '并行度', 'Checkpoint', 'Watermark P95', 'Backpressure', '迟到率', '异常数', 'Sink 状态', '操作'];

function FlinkJobHealthTable({ rows }: { rows?: DataQualityVisuals['flinkJobRows'] }) {
  const jobRows = rows ?? [];
  const paging = useDataQualityPagination(jobRows, 5, 'flinkJobRows');
  return (
    <div className="taf-data-quality-flink-job-table taf-data-quality-paged-table">
      <div className="taf-data-quality-flink-job-head">
        {flinkJobColumns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => {
        const tone = row[1] === '背压中' ? 'warn' : row[1] === '重启中' ? 'info' : 'ok';
        return (
          <div key={row[0]} className={`taf-data-quality-flink-job-row is-${tone}`} title={row.join(' ')}>
            {row.map((cell, index) => {
              if (index === 0)
                return (
                  <strong key={`${row[0]}-${index}`} title={cell}>
                    {cell}
                  </strong>
                );
              if (index === 1)
                return (
                  <span key={`${row[0]}-${index}`} className="taf-data-quality-flink-job-state" title={cell}>
                    {cell}
                  </span>
                );
              return (
                <span key={`${row[0]}-${index}`} title={cell}>
                  {cell}
                </span>
              );
            })}
            <span className="taf-data-quality-flink-job-actions" title={`查看 ${row[0]}`}>
              <FileSearchOutlined />
              <SearchOutlined />
            </span>
          </div>
        );
      })}
      <DataQualityPagination label="Flink 作业" {...paging} />
    </div>
  );
}

function FlinkCheckpointWatermarkTrend({ trend }: { trend?: DataQualityVisuals['flinkCheckpointTrend'] }) {
  if (!trend) return <DataUnavailable section="Checkpoint 与 Watermark 趋势" />;
  const chart = trend;
  return (
    <div className="taf-data-quality-flink-checkpoint-trend">
      <div className="taf-data-quality-flink-trend-legend">
        {[
          ['Checkpoint 时长 (s)', 'duration'],
          ['Checkpoint Age (s)', 'age'],
          ['Watermark 延迟 P95 (s)', 'watermark'],
          ['Watermark SLA 阈值 (s)', 'watermark-sla'],
          ['Checkpoint SLA 阈值 (s)', 'checkpoint-sla'],
        ].map(([label, tone]) => (
          <span key={label} className={`is-${tone}`} title={label}>
            {label}
          </span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="Checkpoint 与 Watermark 趋势"
        className="taf-data-quality-flink-checkpoint-echart"
        categories={chart.times}
        series={[
          {
            name: 'Checkpoint 时长',
            color: '#18a8ff',
            values: chart.checkpointDuration,
          },
          {
            name: 'Checkpoint Age',
            color: '#40d98a',
            values: chart.checkpointAge,
          },
          {
            name: 'Watermark P95',
            color: '#ffb020',
            values: chart.watermarkP95,
          },
          {
            name: 'Watermark SLA',
            color: '#ff4d4f',
            dashed: true,
            values: chart.watermarkSla,
          },
          {
            name: 'Checkpoint SLA',
            color: '#a78bfa',
            dashed: true,
            values: chart.checkpointSla,
          },
        ]}
        valueFormatter={(value) => `${value}s`}
      />
    </div>
  );
}

function FlinkBackpressureHeatmap({ buckets, rows }: { buckets?: string[]; rows?: DataQualityVisuals['flinkBackpressureRows'] }) {
  const heatRows = rows ?? [];
  const bucketLabels = buckets ?? [];
  if (heatRows.length === 0 || bucketLabels.length === 0) return <DataUnavailable section="Backpressure 热力图" />;
  return (
    <div className="taf-data-quality-flink-backpressure">
      <DataQualityHeatmapChart
        ariaLabel="Backpressure 热力图（按作业和 Subtask）"
        className="taf-data-quality-flink-backpressure-echart"
        mode="backpressure"
        rows={heatRows}
        times={bucketLabels}
      />
    </div>
  );
}

function LateWindowClosure({ topicRows, windowRows }: { topicRows?: DataQualityVisuals['flinkLateTopicRows']; windowRows?: DataQualityVisuals['flinkWindowRows'] }) {
  const topics = topicRows ?? [];
  const windows = windowRows ?? [];
  const windowPaging = useDataQualityPagination(windows, 4, 'flinkWindowRows');
  return (
    <div className="taf-data-quality-flink-late-window">
      <div className="taf-data-quality-flink-late-bars">
        <div className="taf-data-quality-flink-late-legend">
          <span>
            <i className="is-normal" />
            正常事件
          </span>
          <span>
            <i className="is-late" />
            迟到事件 (side-output)
          </span>
          <span>
            <i className="is-severe" />
            丢弃事件
          </span>
        </div>
        {topics.map(([topic, normal, late, dropped]) => {
          const counts = [normal, late, dropped].map(parseCompactNumber);
          const total = counts.reduce((sum, value) => sum + value, 0);
          const normalWidth = total > 0 ? (counts[0] / total) * 100 : 0;
          const lateWidth = total > 0 ? (counts[1] / total) * 100 : 0;
          const droppedWidth = total > 0 ? (counts[2] / total) * 100 : 0;
          return (
            <div key={topic} className="taf-data-quality-flink-late-row" title={`${topic} 正常 ${normal} 迟到 ${late} 丢弃 ${dropped}`}>
              <strong title={topic}>{topic}</strong>
              <div>
                <span className="is-normal" style={{ width: `${normalWidth}%` }}>
                  {normal}
                </span>
                <span className="is-late" style={{ width: `${lateWidth}%` }}>
                  {late}
                </span>
                <span className="is-severe" style={{ width: `${droppedWidth}%` }}>
                  {dropped}
                </span>
              </div>
            </div>
          );
        })}
      </div>
      <div className="taf-data-quality-flink-window-table taf-data-quality-paged-table">
        <div>
          <span title="窗口大小">窗口大小</span>
          <span title="窗口闭合延迟 P95">窗口闭合延迟 P95</span>
          <span title="丢弃率">丢弃率</span>
        </div>
        {windowPaging.visibleRows.map((row) => (
          <div key={row[0]} title={row.join(' ')}>
            {row.map((cell) => (
              <span key={cell} title={cell}>
                {cell}
              </span>
            ))}
          </div>
        ))}
        <DataQualityPagination label="窗口闭合" {...windowPaging} />
      </div>
    </div>
  );
}

function FlinkFailureTable({ rows }: { rows?: DataQualityVisuals['flinkFailureRows'] }) {
  const failureRows = rows ?? [];
  const columns = ['异常类型', '作业', '算子 UID', '次数', '首次发生', '最近发生', '建议处理'];
  return (
    <div className="taf-data-quality-flink-failure-table">
      <div>
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {failureRows.slice(0, 10).map((row) => (
        <button key={`${row[0]}-${row[2]}`} type="button" title={row.join(' ')}>
          {row.map((cell) => (
            <span key={cell} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <footer className="taf-data-quality-table-actions">
        <button type="button" title="查看更多异常与失败原因">
          查看更多 <ArrowUpOutlined />
        </button>
      </footer>
    </div>
  );
}

function SinkQualityCards({ rows }: { rows?: DataQualityVisuals['flinkSinkRows'] }) {
  const sinkRows = rows ?? [];
  return (
    <div className="taf-data-quality-flink-sinks">
      {sinkRows.map((sink) => (
        <section key={sink.name} title={`${sink.name} ${sink.status} 写入 EPS ${sink.eps} 成功率 ${sink.success} P95 写入延迟 ${sink.p95} 重试次数 ${sink.retries}`}>
          <header>
            <strong title={sink.name}>{sink.name}</strong>
            <span>
              <CheckCircleOutlined /> {sink.status}
            </span>
          </header>
          <p>
            <span>写入 EPS</span>
            <b>{sink.eps}</b>
          </p>
          <p>
            <span>成功率</span>
            <b>{sink.success}</b>
          </p>
          <p>
            <span>P95 写入延迟</span>
            <b>{sink.p95}</b>
          </p>
          <p>
            <span>重试次数</span>
            <b>{sink.retries}</b>
          </p>
          <MiniTrend values={sink.trend} />
        </section>
      ))}
    </div>
  );
}

function MiniTrend({ values }: { values: number[] }) {
  return <DataQualityKpiSparklineChart ariaLabel="Sink 写入趋势" className="taf-data-quality-flink-mini-echart" tone="ok" values={values} />;
}

function FieldQualityContent({ dataQualityVisuals, onOpenDetail }: { dataQualityVisuals?: DataQualityVisuals; onOpenDetail: OpenFieldQualityDetail }) {
  return (
    <>
      <div className="taf-data-quality-field-upper">
        <WorkPanel title="关键字段质量矩阵">
          <FieldQualityMatrix rows={dataQualityVisuals?.fieldQualityRows} />
        </WorkPanel>
        <WorkPanel title="字段异常趋势（近 24 小时）">
          <FieldAnomalyTrend trend={dataQualityVisuals?.fieldTrend} summary={dataQualityVisuals?.fieldTrendSummary} />
        </WorkPanel>
        <WorkPanel title="五元组与 community_id 校验">
          <CommunityIdCheck rows={dataQualityVisuals?.communityCheckRows} mismatches={dataQualityVisuals?.communityMismatchRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-field-lower">
        <WorkPanel className="taf-data-quality-field-samples-panel" title="异常样本表（按影响时间排序）">
          <FieldAnomalySampleTable rows={dataQualityVisuals?.fieldAnomalyRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
        <WorkPanel title="字段血缘与映射">
          <FieldLineageMapping rows={dataQualityVisuals?.fieldLineageRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
        <WorkPanel className="taf-data-quality-field-repairs-panel" title="修复任务与规则建议">
          <FieldRepairTasks rows={dataQualityVisuals?.fieldRepairRows} onOpenDetail={onOpenDetail} />
        </WorkPanel>
      </div>
    </>
  );
}

function fieldQualityTone(value: string) {
  if (value === '--') return 'na';
  const numeric = Number.parseFloat(value);
  if (Number.isNaN(numeric)) return 'info';
  if (numeric < 95) return 'risk';
  if (numeric < 98) return 'warn';
  return 'ok';
}

function fieldTaskStatusClass(value: string) {
  if (value.includes('已完成')) return 'is-ok';
  if (value.includes('进行中')) return 'is-info';
  if (value.includes('待检查')) return 'is-warn';
  return 'is-risk';
}

function FieldQualityMatrix({ rows }: { rows?: DataQualityVisuals['fieldQualityRows'] }) {
  const matrixRows = rows ?? [];
  const columns = ['字段', '完整性', '格式', '枚举', '跨表一致', '时序', '来源血缘'];
  return (
    <div className="taf-data-quality-field-matrix">
      <div className="taf-data-quality-field-matrix-head">
        {columns.map((column) => <span key={column} title={column}>{column}</span>)}
      </div>
      <div className="taf-data-quality-field-matrix-scroll">
        {matrixRows.map((row) => (
          <div key={row[0]} className="taf-data-quality-field-matrix-row" title={row.join(' ')}>
            <strong title={row[0]}>{row[0]}</strong>
            {row.slice(1).map((cell, index) => (
              <span key={`${row[0]}-${index}`} className={`is-${fieldQualityTone(cell)}`} title={`${columns[index + 1]} ${cell}`}>{cell}</span>
            ))}
          </div>
        ))}
      </div>
      <footer className="taf-data-quality-table-legend">
        <span><i className="is-ok" />优秀 (&gt;=98%)</span>
        <span><i className="is-warn" />中等 (95%-98%)</span>
        <span><i className="is-risk" />较差 (&lt;95%)</span>
        <span><i className="is-na" />-- 不适用</span>
      </footer>
    </div>
  );
}

const fieldTrendSeries = [
  ['缺失值', 'missing', '#1890ff'],
  ['格式不合法', 'format', '#35d06f'],
  ['映射不一致', 'mapping', '#f59e0b'],
  ['时间漂移', 'timeDrift', '#ff4d4f'],
  ['未知协议', 'unknownProtocol', '#2f80ed'],
] as const;

function FieldAnomalyTrend({ summary, trend }: { summary?: DataQualityVisuals['fieldTrendSummary']; trend?: DataQualityVisuals['fieldTrend'] }) {
  if (!trend) return <DataUnavailable section="字段异常趋势" />;
  const chart = trend;
  const summaryRows = summary ?? [];
  return (
    <div className="taf-data-quality-field-trend-panel">
      <div className="taf-data-quality-field-trend-legend">
        {fieldTrendSeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}>
            <i />
            {label}
          </span>
        ))}
      </div>
      <DataQualityFieldTrendChart
        ariaLabel="字段异常趋势"
        threshold={2000}
        times={chart.times}
        series={fieldTrendSeries.map(([name, key, color]) => ({
          name,
          color,
          values: chart[key],
        }))}
      />
      <footer>
        {chart.times.map((time) => (
          <span key={time} title={time}>
            {time}
          </span>
        ))}
      </footer>
      <div className="taf-data-quality-field-trend-summary">
        {summaryRows.map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`} title={`${label} ${value}`}>
            <em>{label}</em>
            <strong>{value}</strong>
          </span>
        ))}
      </div>
    </div>
  );
}

function CommunityIdCheck({ mismatches, rows }: { mismatches?: DataQualityVisuals['communityMismatchRows']; rows?: DataQualityVisuals['communityCheckRows'] }) {
  const checkRows = rows ?? [];
  const mismatchRows = mismatches ?? [];
  return (
    <div className="taf-data-quality-community-check">
      {checkRows.length === 0 && <DataUnavailable section="community_id 校验摘要" />}
      <div className="taf-data-quality-community-flow">
        <span title="五元组 src_ip dst_ip src_port dst_port protocol">
          五元组
          <small>
            src_ip
            <br />
            dst_ip
            <br />
            src_port
            <br />
            dst_port
            <br />
            protocol
          </small>
        </span>
        <i />
        <span title="社区 ID 计算 SHA-1">
          社区 ID 计算<small>社区哈希 (SHA-1)</small>
        </span>
        <i />
        <span title="community_id">community_id</span>
      </div>
      <div className="taf-data-quality-community-detail taf-data-quality-scroll-table">
        <div>
          {['校验明细', '总记录数', '匹配数', '不匹配数', '匹配率'].map((column) => (
            <span key={column} title={column}>
              {column}
            </span>
          ))}
        </div>
        <div className="taf-data-quality-scroll-body">
          {checkRows.map((row) => (
            <div key={row[0]} title={row.join(' ')}>
              {row.map((cell, index) => (
                <span key={`${row[0]}-${index}`} className={index === 3 ? 'is-risk' : ''} title={cell}>{cell}</span>
              ))}
            </div>
          ))}
        </div>
      </div>
      <div className="taf-data-quality-community-mismatch taf-data-quality-scroll-table">
        <h4>不匹配样例（Top 5）</h4>
        <div>
          {['时间', 'session_id', 'src_ip:src_port', 'dst_ip:dst_port', 'protocol', '计算 cid', '原始 cid', '原因'].map((column) => (
            <span key={column} title={column}>
              {column}
            </span>
          ))}
        </div>
        <div className="taf-data-quality-scroll-body">
          {mismatchRows.map((row) => (
            <div key={`${row[0]}-${row[1]}`} title={row.join(' ')}>
              {row.map((cell, index) => (
                <span key={`${row[1]}-${index}`} className={index === 7 ? 'is-risk' : ''} title={cell}>{cell}</span>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function FieldAnomalySampleTable({ onOpenDetail, rows }: { onOpenDetail: OpenFieldQualityDetail; rows?: DataQualityVisuals['fieldAnomalyRows'] }) {
  const sampleRows = rows ?? [];
  const columns = ['时间', 'Topic', '字段', '异常类型', '原始值', '归一化值', '影响资产', '处置'];
  const paging = useDataQualityPagination(sampleRows, 5, 'fieldAnomalyRows');
  return (
    <div className="taf-data-quality-field-sample-table taf-data-quality-paged-table">
      <div className="taf-data-quality-field-table-head">
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      <div className="taf-data-quality-field-table-rows" aria-label="字段异常样本">
        {paging.visibleRows.map((row) => (
          <button
            key={`${row[0]}-${row[2]}-${row[4]}`}
            type="button"
            className="taf-data-quality-field-table-row"
            title={row.join(' ')}
            onClick={() =>
              onOpenDetail({
                title: `字段异常详情：${row[2]}`,
                description: `${row[1]} 在 ${row[0]} 发现 ${row[3]}；当前处置建议为 ${row[7]}。`,
                columns,
                rows: [row],
                actionLabel: row[7] === '创建任务' ? '创建字段修复任务' : undefined,
                actionSuccessMessage: row[7] === '创建任务' ? `已为 ${row[2]} 创建字段修复任务` : undefined,
              })
            }
          >
            {row.map((cell, index) => (
              <span key={`${row[0]}-${index}`} className={index === 3 ? 'is-risk' : index === 7 ? 'is-link' : ''} title={cell}>
                {cell}
              </span>
            ))}
          </button>
        ))}
      </div>
      <DataQualityPagination label="异常样本" {...paging} />
    </div>
  );
}

function FieldLineageMapping({ onOpenDetail, rows }: { onOpenDetail: OpenFieldQualityDetail; rows?: DataQualityVisuals['fieldLineageRows'] }) {
  const lineageRows = rows ?? [];
  const columns = ['数据源（Kafka Topic）', '处理链路（Flink）', '归一化映射', '数据落地（Sink）'];
  const paging = useDataQualityPagination(lineageRows, 5, 'fieldLineageRows');
  return (
    <div className="taf-data-quality-field-lineage taf-data-quality-paged-table">
      <div className="taf-data-quality-field-lineage-head">
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map(([source, flink, mapping, sink, tone]) => (
        <div key={source} className={`is-${tone}`} title={`${source} ${flink} ${mapping} ${sink}`}>
          {[source, flink, mapping, sink].map((cell) => (
            <span key={cell} title={cell}>
              {cell}
            </span>
          ))}
        </div>
      ))}
      <footer className="taf-data-quality-table-legend">
        <span>
          <i className="is-ok" />
          正常
        </span>
        <span>
          <i className="is-warn" />
          警告
        </span>
        <span>
          <i className="is-risk" />
          异常
        </span>
        <button
          type="button"
          onClick={() =>
            onOpenDetail({
              title: '字段映射修复任务',
              description: '已识别到异常映射链路；创建后将进入字段质量修复队列并保留审计信息。',
              columns: ['数据源', '处理链路', '建议动作'],
              rows: [['traffic_session_raw', '会话构建', '创建字段映射修复任务']],
              actionLabel: '创建字段修复任务',
              actionSuccessMessage: '字段映射修复任务已创建',
            })
          }
        >
          当前链路存在异常映射，建议创建修复任务
        </button>
      </footer>
      <DataQualityPagination label="字段血缘与映射" {...paging} />
    </div>
  );
}

function FieldRepairTasks({ onOpenDetail, rows }: { onOpenDetail: OpenFieldQualityDetail; rows?: DataQualityVisuals['fieldRepairRows'] }) {
  const repairRows = rows ?? [];
  const columns = ['任务名称', '异常字段', '建议映射 / 修复规则', '负责人', '状态', 'SLA', '验证结果', '操作'];
  const paging = useDataQualityPagination(repairRows, 5, 'fieldRepairRows');
  return (
    <div className="taf-data-quality-field-repair-table taf-data-quality-paged-table">
      <div className="taf-data-quality-field-table-head">
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      <div className="taf-data-quality-field-table-rows" aria-label="字段修复任务">
        {paging.visibleRows.map((row) => (
          <button
            key={`${row[0]}-${row[1]}`}
            type="button"
            className="taf-data-quality-field-table-row"
            title={row.join(' ')}
            onClick={() =>
              onOpenDetail({
                title: `字段修复任务：${row[0]}`,
                description: `${row[1]} 的修复规则由 ${row[3]} 负责，当前状态为 ${row[4]}。`,
                columns,
                rows: [row],
                actionLabel: row[7] === '创建' ? '创建修复任务' : undefined,
                actionSuccessMessage: row[7] === '创建' ? `${row[0]} 已创建并进入待处理队列` : undefined,
              })
            }
          >
            {row.map((cell, index) => (
              <span key={`${row[0]}-${index}`} className={index === 4 ? fieldTaskStatusClass(cell) : index === 6 && cell === '通过' ? 'is-ok' : index === 7 ? 'is-link' : ''} title={cell}>
                {cell}
              </span>
            ))}
          </button>
        ))}
      </div>
      <DataQualityPagination label="修复任务" {...paging} />
    </div>
  );
}

function StorageQualityContent({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  return (
    <>
      <div className="taf-data-quality-storage-upper">
        <WorkPanel title="存储组件健康总览">
          <StorageComponentHealthTable rows={dataQualityVisuals?.storageComponentRows} />
        </WorkPanel>
        <WorkPanel title="写入速率与延迟趋势（近 24 小时）">
          <StorageWriteTrend trend={dataQualityVisuals?.storageTrend} />
        </WorkPanel>
        <WorkPanel title="容量与水位趋势（近 7 天）">
          <StorageCapacityTrend trend={dataQualityVisuals?.storageCapacityTrend} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-storage-lower">
        <WorkPanel title="失败写入与原因列表（近 24 小时）">
          <StorageFailureTable rows={dataQualityVisuals?.storageFailureRows} />
        </WorkPanel>
        <WorkPanel title="索引与归档链路（写入链路全景）">
          <StoragePipelineFlow rows={dataQualityVisuals?.storagePipelineRows} />
        </WorkPanel>
        <WorkPanel title="副本、分片与对象健康">
          <StorageReplicaHealth indexHealth={dataQualityVisuals?.storageIndexHealth} objectRows={dataQualityVisuals?.storageObjectRows} partitionRows={dataQualityVisuals?.storagePartitionRows} replicaRows={dataQualityVisuals?.storageReplicaRows} />
        </WorkPanel>
      </div>
    </>
  );
}

function storageStatusClass(value: string | undefined) {
  if (!value) return 'info';
  if (value.includes('警告') || value.includes('高') || value.includes('异常') || value.includes('red')) return 'risk';
  if (value.includes('注意') || value.includes('重试') || value.includes('进行中') || value.includes('lag')) return 'warn';
  if (value.includes('正常') || value.includes('已结束')) return 'ok';
  return 'info';
}

function StorageComponentHealthTable({ rows }: { rows?: DataQualityVisuals['storageComponentRows'] }) {
  const tableRows = rows ?? [];
  const columns = ['组件', '状态', '写入速率', '成功率', 'P95 延迟', '积压/队列', '容量', '副本/分片', '操作'];
  const paging = useDataQualityPagination(tableRows, 5, 'storageComponentRows');
  return (
    <div className="taf-data-quality-storage-component-table taf-data-quality-paged-table">
      <div className="taf-data-quality-storage-component-head">
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <button key={row[0]} type="button" className={`is-${storageStatusClass(row[1])}`} title={row.join(' ')}>
          <strong title={row[0]}>{row[0]}</strong>
          {row.slice(1).map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 0 ? `is-${storageStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <DataQualityPagination label="存储组件健康" {...paging} />
    </div>
  );
}

const storageTrendSeries = [
  ['ClickHouse EPS', 'clickhouse', '#2f80ed'],
  ['OpenSearch docs/s', 'opensearch', '#00d6d6'],
  ['NebulaGraph edges/s', 'nebula', '#4ade80'],
  ['MinIO objects/s', 'minio', '#b85cff'],
  ['P95 延迟(毫秒)', 'latencyP95', '#ff7875'],
  ['延迟 SLA', 'latencySla', '#faad14'],
] as const;

function StorageWriteTrend({ trend }: { trend?: DataQualityVisuals['storageTrend'] }) {
  if (!trend) return <DataUnavailable section="写入速率与延迟趋势" />;
  const chart = trend;
  return (
    <div className="taf-data-quality-storage-trend">
      <div className="taf-data-quality-storage-trend-legend">
        {storageTrendSeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}>
            <i />
            {label}
          </span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="写入速率与延迟趋势"
        className="taf-data-quality-storage-echart"
        categories={chart.times}
        series={storageTrendSeries.map(([name, key, color]) => ({
          name,
          color,
          values: chart[key],
          dashed: key === 'latencySla',
        }))}
      />
    </div>
  );
}

const storageCapacitySeries = [
  ['ClickHouse 容量', 'clickhouse', '#2f80ed'],
  ['OpenSearch 索引', 'opensearch', '#00d6d6'],
  ['NebulaGraph 分区', 'nebula', '#4ade80'],
  ['MinIO Bucket', 'minio', '#b85cff'],
  ['容量阈值', 'threshold', '#faad14'],
] as const;

function StorageCapacityTrend({ trend }: { trend?: DataQualityVisuals['storageCapacityTrend'] }) {
  if (!trend) return <DataUnavailable section="容量与水位趋势" />;
  const chart = trend;
  return (
    <div className="taf-data-quality-storage-capacity">
      <div className="taf-data-quality-storage-trend-legend">
        {storageCapacitySeries.map(([label, key, color]) => (
          <span key={key} style={{ color }} title={label}>
            <i />
            {label}
          </span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="容量与水位趋势"
        className="taf-data-quality-storage-echart"
        categories={chart.days}
        series={storageCapacitySeries.map(([name, key, color]) => ({
          name,
          color,
          values: chart[key],
          dashed: key === 'threshold',
          area: key !== 'threshold',
        }))}
        valueFormatter={(value) => `${value}%`}
      />
    </div>
  );
}

function StorageFailureTable({ rows }: { rows?: DataQualityVisuals['storageFailureRows'] }) {
  const failureRows = rows ?? [];
  const columns = ['时间', '组件', '目标表/索引/Bucket', '失败原因', '影响记录', '重试', '状态', '处置'];
  const paging = useDataQualityPagination(failureRows, 5, 'storageFailureRows');
  return (
    <div className="taf-data-quality-storage-failure-table taf-data-quality-paged-table">
      <div>
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <button key={`${row[0]}-${row[1]}-${row[2]}`} type="button" className={`is-${storageStatusClass(row[6])}`} title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 6 ? `is-${storageStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <DataQualityPagination label="失败写入与原因" {...paging} />
    </div>
  );
}

function StoragePipelineFlow({ rows }: { rows?: DataQualityVisuals['storagePipelineRows'] }) {
  const flowRows = rows ?? [];
  if (flowRows.length === 0) return <DataUnavailable section="存储写入链路" />;
  const storageNodes = flowRows.filter((row) => row.from === 'Kafka / Flink').map((row) => row.to);
  const resultNodes = Array.from(new Set(flowRows.filter((row) => row.from !== 'Kafka / Flink').map((row) => row.to)));
  const nodeStatus = (label: string) => flowRows.find((row) => row.to === label || row.from === label)?.status ?? 'info';
  return (
    <div className="taf-data-quality-storage-flow" title="Kafka/Flink 到 ClickHouse、OpenSearch、NebulaGraph、MinIO 的写入链路全景">
      <section className="is-source">
        <StoragePipelineNode label="Kafka / Flink" detail="批量 Sink / Exactly-once" status="info" />
      </section>
      <section>
        {storageNodes.map((node) => (
          <StoragePipelineNode key={node} label={node} detail={flowRows.find((row) => row.to === node)?.label ?? '写入'} status={nodeStatus(node)} />
        ))}
      </section>
      <section>
        {resultNodes.map((node) => (
          <StoragePipelineNode key={node} label={node} detail={flowRows.find((row) => row.to === node)?.label ?? '状态'} status={nodeStatus(node)} />
        ))}
      </section>
      <footer>
        {flowRows.map((edge) => (
          <span key={`${edge.from}-${edge.to}`} className={`is-${edge.status}`} title={`${edge.from} → ${edge.to} ${edge.label}`}>
            {edge.from} → {edge.to}
          </span>
        ))}
      </footer>
    </div>
  );
}

function StoragePipelineNode({ detail, label, status }: { detail: string; label: string; status: 'ok' | 'info' | 'warn' | 'risk' }) {
  return (
    <span className={`taf-data-quality-storage-flow-node is-${status}`} title={`${label} ${detail}`}>
      <b>{label}</b>
      <em>{detail}</em>
    </span>
  );
}

function StorageReplicaHealth({ indexHealth, objectRows, partitionRows, replicaRows }: { indexHealth?: DataQualityVisuals['storageIndexHealth']; objectRows?: DataQualityVisuals['storageObjectRows']; partitionRows?: DataQualityVisuals['storagePartitionRows']; replicaRows?: DataQualityVisuals['storageReplicaRows'] }) {
  const replicas = replicaRows ?? [];
  const indexes = indexHealth ?? [];
  const partitions = partitionRows ?? [];
  const objects = objectRows ?? [];
  const replicaPaging = useDataQualityPagination(replicas, 4, 'storageReplicaRows');
  const partitionPaging = useDataQualityPagination(partitions, 4, 'storagePartitionRows');
  const objectPaging = useDataQualityPagination(objects, 4, 'storageObjectRows');
  return (
    <div className="taf-data-quality-storage-health">
      <div className="taf-data-quality-storage-replica-list taf-data-quality-paged-table">
        {replicaPaging.visibleRows.map((row) => (
          <button key={row[0]} type="button" className={`is-${storageStatusClass(row[4])}`} title={row.join(' ')}>
            <strong>{row[0]}</strong>
            <span>{row[1]}</span>
            <span>{row[2]}</span>
            <em>{row[3]}</em>
          </button>
        ))}
        <DataQualityPagination label="副本健康" {...replicaPaging} />
      </div>
      <div className="taf-data-quality-storage-donut-block">
        <StorageIndexDonut rows={indexes} />
        <div>
          {indexes.map((item) => (
            <span key={item.label} className={`is-${item.status}`} title={`${item.label} ${item.value}`}>
              <i />
              {item.label}
              <b>{item.value}</b>
            </span>
          ))}
        </div>
      </div>
      <div className="taf-data-quality-storage-health-tables">
        <section className="taf-data-quality-paged-table">
          <h4>分区健康</h4>
          {partitionPaging.visibleRows.map((row) => (
            <p key={row[1]} title={row.join(' ')}>
              {row.map((cell) => (
                <span key={cell}>{cell}</span>
              ))}
            </p>
          ))}
          <DataQualityPagination label="分区健康" {...partitionPaging} />
        </section>
        <section className="taf-data-quality-paged-table">
          <h4>对象生命周期</h4>
          {objectPaging.visibleRows.map(([label, value]) => (
            <p key={label} title={`${label} ${value}`}>
              <span>{label}</span>
              <b>{value}</b>
            </p>
          ))}
          <DataQualityPagination label="对象生命周期" {...objectPaging} />
        </section>
      </div>
    </div>
  );
}

function StorageIndexDonut({ rows }: { rows: DataQualityVisuals['storageIndexHealth'] }) {
  const colorMap = {
    ok: '#52c41a',
    info: '#1890ff',
    warn: '#faad14',
    risk: '#ff4d4f',
  };
  return (
    <DataQualityDonutChart
      ariaLabel="OpenSearch 索引健康"
      className="taf-data-quality-storage-donut"
      rows={rows.map((item) => ({
        label: item.label,
        value: item.value,
        color: colorMap[item.status],
      }))}
    />
  );
}

function StorageQualityOverview({ rows }: { rows?: DataQualityVisuals['storageComponentRows'] }) {
  const overviewRows = (rows ?? []).map((row) => [row[0], row[1], row[2], row[3], row[4], row[5]]);
  return <DenseRows columns={['组件', '状态', '写入速率', '成功率', 'P95', '积压']} pageSize={3} rows={overviewRows} />;
}

function ReplayReconcileContent({ dataQualityVisuals, onOpenReplay }: { dataQualityVisuals?: DataQualityVisuals; onOpenReplay: () => void }) {
  return (
    <>
      <div className="taf-data-quality-replay-upper">
        <WorkPanel
          title="DLQ 重放任务表"
          extra={
            <Button size="small" icon={<SyncOutlined />} onClick={onOpenReplay}>
              重放
            </Button>
          }
        >
          <ReplayTaskTable rows={dataQualityVisuals?.replayTaskRows} />
        </WorkPanel>
        <WorkPanel
          title="时间窗对账报告（近 24 小时）"
          extra={
            <Space className="taf-data-quality-replay-panel-tools" size={4}>
              <Button size="small">按小时</Button>
              <Button size="small" icon={<DownloadOutlined />}>
                导出图表
              </Button>
            </Space>
          }
        >
          <ReplayReconcileTrend summary={dataQualityVisuals?.replayReconcileSummary} trend={dataQualityVisuals?.replayReconcileTrend} />
        </WorkPanel>
        <WorkPanel title="幂等检查与重复检测">
          <ReplayIdempotencyTable rows={dataQualityVisuals?.replayIdempotencyRows} />
        </WorkPanel>
      </div>
      <div className="taf-data-quality-replay-lower">
        <WorkPanel title="差异样本与原因（近 24 小时）">
          <ReplayDifferenceTable rows={dataQualityVisuals?.replayDifferenceRows} />
        </WorkPanel>
        <WorkPanel title="重放链路状态">
          <ReplayFlowStatus edges={dataQualityVisuals?.replayFlowEdges} nodes={dataQualityVisuals?.replayFlowNodes} />
        </WorkPanel>
        <WorkPanel title="验收证据与导出">
          <ReplayEvidenceExport rows={dataQualityVisuals?.replayEvidenceRows} />
        </WorkPanel>
      </div>
    </>
  );
}

const replayTaskColumns = ['任务', '来源 Topic', '时间窗', '待重放', '成功率', '失败数', '幂等状态', '操作'];

function replayStatusClass(value: string | undefined) {
  if (!value) return 'info';
  if (value.includes('高') || value.includes('失败') || value.includes('异常')) return 'risk';
  if (value.includes('警告') || value.includes('待重放') || value.includes('冲突') || value.includes('重试')) return 'warn';
  if (value.includes('通过') || value.includes('已归档') || value.includes('RUNNING') || value.includes('正常')) return 'ok';
  return 'info';
}

function ReplayTaskTable({ rows }: { rows?: DataQualityVisuals['replayTaskRows'] }) {
  const taskRows = rows ?? [];
  const paging = useDataQualityPagination(taskRows, 5, 'replayTaskRows');
  return (
    <div className="taf-data-quality-replay-task-table taf-data-quality-paged-table">
      <div>
        {replayTaskColumns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button" className={`is-${replayStatusClass(row[6])}`} title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 6 ? `is-${replayStatusClass(cell)}` : index === 7 ? 'is-link' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <DataQualityPagination label="DLQ 重放任务" {...paging} />
    </div>
  );
}

function ReplayReconcileTrend({ summary, trend }: { summary?: DataQualityVisuals['replayReconcileSummary']; trend?: DataQualityVisuals['replayReconcileTrend'] }) {
  if (!trend) return <DataUnavailable section="时间窗对账趋势" />;
  const chart = trend;
  const summaryRows = summary ?? [];
  return (
    <div className="taf-data-quality-replay-trend" title="源端总数、落库总数、差异数量、差异率（%）">
      <div className="taf-data-quality-replay-trend-legend">
        {[
          ['源端总数', 'source'],
          ['落库总数', 'sink'],
          ['差异数量', 'diff'],
          ['差异率（%）', 'rate'],
          ['阈值 1.00%', 'threshold'],
        ].map(([label, tone]) => (
          <span key={label} className={`is-${tone}`} title={label}>
            <i />
            {label}
          </span>
        ))}
      </div>
      <DataQualityTrendChart
        ariaLabel="时间窗对账报告趋势"
        className="taf-data-quality-replay-echart"
        categories={chart.times}
        series={[
          { name: '源端总数', color: '#18a8ff', values: chart.sourceTotal },
          { name: '落库总数', color: '#4ade80', values: chart.sinkTotal },
          {
            name: '差异数量',
            color: '#ffb020',
            type: 'bar',
            values: chart.diffCount,
          },
          { name: '差异率', color: '#ff4d4f', values: chart.diffRate },
          {
            name: '阈值',
            color: '#a78bfa',
            dashed: true,
            values: chart.diffRateThreshold,
          },
        ]}
      />
      <div className="taf-data-quality-replay-summary">
        <strong title="汇总（近24小时）">汇总（近24小时）</strong>
        {summaryRows.map(([label, value, tone]) => (
          <span key={label} className={`is-${tone}`} title={`${label} ${value}`}>
            {label}
            <b>{value}</b>
          </span>
        ))}
      </div>
    </div>
  );
}

function ReplayIdempotencyTable({ rows }: { rows?: DataQualityVisuals['replayIdempotencyRows'] }) {
  const checkRows = rows ?? [];
  const paging = useDataQualityPagination(checkRows, 5, 'replayIdempotencyRows');
  return (
    <div className="taf-data-quality-replay-idempotency taf-data-quality-paged-table">
      <div>
        {['检查项', '规则', '状态', '命中', '操作'].map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <button key={`${row[0]}-${row[1]}`} type="button" className={`is-${replayStatusClass(row[2])}`} title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[0]}-${index}`} className={index === 2 ? `is-${replayStatusClass(cell)}` : index === 4 ? 'is-link' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <DataQualityPagination label="幂等检查" {...paging} />
    </div>
  );
}

function ReplayDifferenceTable({ rows }: { rows?: DataQualityVisuals['replayDifferenceRows'] }) {
  const sampleRows = rows ?? [];
  const paging = useDataQualityPagination(sampleRows, 5, 'replayDifferenceRows');
  return (
    <div className="taf-data-quality-replay-difference taf-data-quality-paged-table">
      <div>
        {['时间窗', 'Topic', '差异类型', 'Trace ID', '源端值', '落库值', '原因', '操作'].map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <button key={`${row[0]}-${row[1]}-${row[3]}`} type="button" title={row.join(' ')}>
          {row.map((cell, index) => (
            <span key={`${row[3]}-${index}`} className={index === 7 ? 'is-link' : index === 2 && (cell.includes('duplicate') || cell.includes('timeout')) ? 'is-warn' : ''} title={cell}>
              {cell}
            </span>
          ))}
        </button>
      ))}
      <DataQualityPagination label="差异样本" {...paging} />
    </div>
  );
}

function ReplayFlowStatus({ edges, nodes }: { edges?: DataQualityVisuals['replayFlowEdges']; nodes?: DataQualityVisuals['replayFlowNodes'] }) {
  const flowNodes = nodes ?? [];
  const flowEdges = edges ?? [];
  if (flowNodes.length === 0) return <DataUnavailable section="重放链路状态" />;
  const topNodes = flowNodes.slice(0, 4);
  const bottomNodes = flowNodes.slice(4);
  return (
    <div className="taf-data-quality-replay-flow" title="重放链路状态">
      <div className="taf-data-quality-replay-flow-chain">
        {topNodes.map((node, index) => (
          <ReplayFlowNode key={node.id} node={node} step={index} />
        ))}
      </div>
      <div className="taf-data-quality-replay-flow-bottom">
        {bottomNodes.map((node, index) => (
          <ReplayFlowNode key={node.id} node={node} step={index + topNodes.length} compact />
        ))}
      </div>
      <footer>
        {flowEdges.map((edge) => (
          <span key={`${edge.from}-${edge.to}`} className={`is-${edge.status}`} title={`${edge.from} → ${edge.to} ${edge.label}`}>
            <i />
            {edge.label}
          </span>
        ))}
        <button type="button" title="查看链路详情">
          查看链路详情 <ArrowUpOutlined />
        </button>
      </footer>
    </div>
  );
}

function ReplayFlowNode({ compact = false, node, step }: { compact?: boolean; node: DataQualityVisuals['replayFlowNodes'][number]; step: number }) {
  const details = node.detail
    .split('/')
    .map((item) => item.trim())
    .filter(Boolean)
    .slice(0, compact ? 2 : 3);
  return (
    <section className={`taf-data-quality-replay-flow-node is-${node.status}${compact ? ' is-compact' : ''}`} title={`${node.label} ${node.detail}`} style={{ '--replay-step': step } as CSSProperties}>
      <strong title={node.label}>{node.label}</strong>
      {details.map((detail) => (
        <span key={detail} title={detail}>
          {detail}
        </span>
      ))}
    </section>
  );
}

function ReplayEvidenceExport({ rows }: { rows?: DataQualityVisuals['replayEvidenceRows'] }) {
  const evidenceRows = rows ?? [];
  const paging = useDataQualityPagination(evidenceRows, 4, 'replayEvidenceRows');
  return (
    <div className="taf-data-quality-replay-evidence taf-data-quality-paged-table">
      {paging.visibleRows.map((row) => (
        <button key={row[0]} type="button" className={`is-${replayStatusClass(row[3])}`} title={row.join(' ')}>
          <CheckCircleOutlined />
          <strong title={row[0]}>{row[0]}</strong>
          <span title={`${row[1]} ${row[2]}`}>
            {row[1]} · {row[2]}
          </span>
          <em title={row[4]}>{row[4]}</em>
        </button>
      ))}
      <footer>
        <button type="button" title="查看验收历史">
          查看验收历史 <ArrowUpOutlined />
        </button>
      </footer>
      <DataQualityPagination label="证据导出" {...paging} />
    </div>
  );
}

function ReplayReconcileSideRail({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-replay-rail">
      <WorkPanel title="重放对账异常">
        <ReplayRailAlerts rows={dataQualityVisuals?.replayRailAlerts} />
      </WorkPanel>
      <WorkPanel title="快速定位">
        <ReplayRailLinks icon="search" rows={dataQualityVisuals?.replayRailLocateRows} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <ReplayRailLinks icon="sync" rows={dataQualityVisuals?.replayRailRepairRows} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <ReplayRailLinks icon="download" rows={dataQualityVisuals?.replayRailEvidenceRows} />
      </WorkPanel>
    </aside>
  );
}

function ReplayRailAlerts({ rows }: { rows?: DataQualityVisuals['replayRailAlerts'] }) {
  const alertRows = rows ?? [];
  return (
    <div className="taf-data-quality-replay-rail-alerts">
      {alertRows.map(([level, title, value, tone]) => (
        <button key={title} type="button" className={`is-${tone}`} title={`${level} ${title} ${value}`}>
          <span title={level}>{level}</span>
          <strong title={title}>{title}</strong>
          <b title={value}>{value}</b>
        </button>
      ))}
      <a href="#replay-all-alerts" title="查看全部异常">
        查看全部异常 <ArrowUpOutlined />
      </a>
    </div>
  );
}

function ReplayRailLinks({ icon, rows }: { icon: 'download' | 'search' | 'sync'; rows?: string[] }) {
  const items = rows ?? [];
  const iconNode = icon === 'download' ? <DownloadOutlined /> : icon === 'sync' ? <SyncOutlined /> : <SearchOutlined />;
  return (
    <div className="taf-data-quality-replay-rail-links">
      {items.map((label) => (
        <button key={label} type="button" title={label}>
          {iconNode}
          <span title={label}>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function ReportContent({ error, isLoading, onDownload, report }: { error: Error | null; isLoading: boolean; onDownload: (format: 'pdf' | 'json' | 'csv') => Promise<void>; report?: DataQualityDailyReport }) {
  if (isLoading) {
    return <Alert type="info" showIcon message="正在通过 API 动态生成数据质量日报" />;
  }
  if (error || !report) {
    return <Alert type="error" showIcon message="数据质量日报生成失败" description={error?.message ?? '日报 API 未返回数据'} />;
  }
  return (
    <div className="taf-data-quality-report-workspace">
      <WorkPanel title="质量报告预览" className="taf-data-quality-report-preview-panel">
        <QualityReportPreview onDownload={onDownload} report={report} />
      </WorkPanel>
      <WorkPanel title="报告章节" className="taf-data-quality-report-chapters">
        <ReportChapters report={report} />
      </WorkPanel>
      <WorkPanel title="异常归因摘要（近 24 小时）" className="taf-data-quality-report-anomaly-panel">
        <ReportPlainTable
          columns={['异常类型', '根因分析', '负责人', '影响范围', '修复状态']}
          rows={report.anomalies.map((item) => [item.type, item.root_cause, item.owner, item.scope, item.status])}
        />
      </WorkPanel>
      <WorkPanel title="导出记录" className="taf-data-quality-report-export-panel">
        <ReportExportTable onDownload={onDownload} report={report} />
      </WorkPanel>
      <WorkPanel title="验收报告与审批" className="taf-data-quality-report-approval-panel">
        <ReportApproval report={report} />
      </WorkPanel>
    </div>
  );
}

function QualityReportPreview({ onDownload, report }: { onDownload: (format: 'pdf' | 'json' | 'csv') => Promise<void>; report: DataQualityDailyReport }) {
  const trendPoints = (key: 'completeness' | 'timeliness' | 'consistency' | 'availability') => report.trend
    .map((item, index) => `${(index / Math.max(report.trend.length - 1, 1)) * 340},${128 - Math.max(0, Math.min(100, item[key])) * 1.05}`)
    .join(' ');
  const anomalyTotal = report.anomalies.length;
  return (
    <div className="taf-data-quality-report-preview">
      <article className="taf-data-quality-report-sheet">
        <header>
          <SafetyCertificateOutlined />
          <div>
            <h2 title={report.title}>{report.title}</h2>
            <p>统计时间：{formatReportTimestamp(report.period_start)} ~ {formatReportTimestamp(report.period_end)}</p>
          </div>
          <span>
            版本：{report.version}
            <br />
            生成时间：{formatReportTimestamp(report.generated_at)}
          </span>
        </header>
        <section className="taf-data-quality-report-score-strip">
          {report.scores.map((item) => (
            <div key={item.label} className={`is-${item.status}`}>
              <span>{item.label}</span>
              <strong>{item.value}</strong>
            </div>
          ))}
        </section>
        <div className="taf-data-quality-report-sheet-grid">
          <section className="taf-data-quality-report-line">
            <h3>二、质量趋势（近 24 小时）</h3>
            <svg viewBox="0 0 340 142" preserveAspectRatio="none" aria-label="质量趋势">
              {[24, 50, 76, 102, 128].map((y) => (
                <line key={y} x1="0" x2="340" y1={y} y2={y} />
              ))}
              <polyline className="is-blue" points={trendPoints('completeness')} />
              <polyline className="is-green" points={trendPoints('timeliness')} />
              <polyline className="is-orange" points={trendPoints('consistency')} />
              <polyline className="is-purple" points={trendPoints('availability')} />
            </svg>
          </section>
          <section className="taf-data-quality-report-donut">
            <h3>三、异常概览</h3>
            <div>
              <strong>{anomalyTotal}</strong>
              <span>异常总数</span>
            </div>
            <ul>
              {report.anomalies.slice(0, 4).map((item, index) => (
                <li key={`${item.type}-${index}`} title={item.root_cause}>
                  <i className={['is-blue', 'is-orange', 'is-green', 'is-purple'][index]} />
                  {item.type} <b>{item.status}</b>
                </li>
              ))}
            </ul>
          </section>
          <section>
            <h3>四、关键指标对比</h3>
            <ReportMiniTable rows={report.key_metrics} />
          </section>
          <section>
            <h3>五、存储写入质量</h3>
            <ReportMiniTable rows={report.storage_rows.map((row) => row.slice(0, 4))} />
          </section>
        </div>
        <footer>
          <section>
            <h3>六、重放对账结果</h3>
            <div className="taf-data-quality-report-conclusion-grid">
              {report.reconcile.map((item) => (
                <span key={item.label} title={`${item.label} ${item.value}`}>
                  {item.label} <b>{item.value}</b>
                </span>
              ))}
            </div>
          </section>
          <section>
            <h3>七、验收结论</h3>
            <div className="taf-data-quality-report-conclusion">
              <strong>{report.conclusion.result}</strong>
              <span title={report.conclusion.summary}>{report.conclusion.summary}</span>
              <small title={report.conclusion.suggestion}>{report.conclusion.suggestion}</small>
            </div>
          </section>
        </footer>
      </article>
      <div className="taf-data-quality-report-viewerbar">
        <Button size="small" type="text">
          ‹
        </Button>
        <span className="is-page">1</span>
        <span>/ 16</span>
        <Button size="small" type="text">
          ›
        </Button>
        <i />
        <span>100%</span>
        <Button size="small" type="text">
          +
        </Button>
        <Tooltip title="下载 PDF 日报"><Button size="small" type="text" data-dq-action-managed="true" icon={<DownloadOutlined />} onClick={() => void onDownload('pdf')} /></Tooltip>
        <Button size="small" type="text" icon={<PrinterOutlined />} />
        <Button size="small" type="text" icon={<FullscreenOutlined />} />
      </div>
    </div>
  );
}

function ReportMiniTable({ rows }: { rows: string[][] }) {
  return (
    <div className="taf-data-quality-report-mini-table">
      {rows.map((row, rowIndex) => (
        <span key={`${row.join('-')}-${rowIndex}`}>
          {row.map((cell, cellIndex) => (
            <b key={`${cell}-${cellIndex}`}>{cell}</b>
          ))}
        </span>
      ))}
    </div>
  );
}

function ReportChapters({ report }: { report: DataQualityDailyReport }) {
  return (
    <div className="taf-data-quality-report-chapter-list">
      {report.chapters.map((item) => (
        <button key={item.label} type="button" className={`is-${item.status}`}>
          <b>{item.index}</b>
          <span>{item.label}</span>
          <em>完成 {item.progress}%</em>
          <CheckCircleOutlined />
        </button>
      ))}
    </div>
  );
}

function ReportApproval({ report }: { report: DataQualityDailyReport }) {
  return (
    <div className="taf-data-quality-report-approval">
      <section>
        <h3>验收包信息</h3>
        <p>验收包：{report.approval.package_id}</p>
        <p>版本：{report.approval.version}</p>
        <p>生成时间：{formatReportTimestamp(report.approval.generated_at)}</p>
        <p>内容：{report.approval.contents.join(' + ')}</p>
      </section>
      <section className="taf-data-quality-report-sla">
        <span>SLA Gate</span>
        <strong title={`${report.approval.sla_gate.toFixed(1)}%`}>{report.approval.sla_gate.toFixed(1)}%</strong>
        <em>达成（&gt;= 95%）</em>
      </section>
      <section className="taf-data-quality-report-audit">
        <h3>审批流转</h3>
        {report.approval.flow.map((item, index) => <p key={item}>{index === 0 ? <CheckCircleOutlined /> : index === 1 ? <SyncOutlined /> : <FieldTimeOutlined />}{item}</p>)}
      </section>
      <section>
        <h3>风控 / 例外</h3>
        <p>{report.approval.risk}</p>
        <p>数据源：{report.source.monitor}</p>
        <p>版本：{report.source.fixture_version}</p>
      </section>
      <div className="taf-data-quality-report-approval-evidence">
        {report.evidence.map((item) => (
          <span key={item.label}>
            {item.label}
            <b>{item.value}</b>
          </span>
        ))}
      </div>
    </div>
  );
}

function ReportPlainTable({ columns, rows }: { columns: string[]; rows: string[][] }) {
  return (
    <div className="taf-data-quality-report-api-table" style={{ '--dq-columns': columns.length } as CSSProperties}>
      <div className="taf-data-quality-report-api-head">{columns.map((column) => <span key={column}>{column}</span>)}</div>
      {rows.map((row, rowIndex) => (
        <div key={`${row.join('-')}-${rowIndex}`} className="taf-data-quality-report-api-row">
          {row.map((cell, cellIndex) => <span key={`${cell}-${cellIndex}`} title={cell}>{cell}</span>)}
        </div>
      ))}
    </div>
  );
}

function ReportExportTable({ onDownload, report }: { onDownload: (format: 'pdf' | 'json' | 'csv') => Promise<void>; report: DataQualityDailyReport }) {
  return (
    <div className="taf-data-quality-report-api-table is-export" style={{ '--dq-columns': 6 } as CSSProperties}>
      <div className="taf-data-quality-report-api-head">{['导出时间', '格式', '申请人', '状态', '接收团队', '操作'].map((column) => <span key={column}>{column}</span>)}</div>
      {report.exports.map((item) => (
        <div key={item.export_id} className="taf-data-quality-report-api-row">
          <span title={formatReportTimestamp(item.time)}>{formatReportTimestamp(item.time)}</span>
          <span>{item.format}</span>
          <span>{item.applicant}</span>
          <span>{item.status}</span>
          <span>{item.recipient}</span>
          <button type="button" data-dq-action-managed="true" onClick={() => void onDownload(item.format.toLowerCase() as 'pdf' | 'json' | 'csv')}><DownloadOutlined /> 下载</button>
        </div>
      ))}
    </div>
  );
}

function formatReportTimestamp(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-');
}

function TopicHealthSideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-topic-health-rail">
      <WorkPanel title="质量异常告警">
        <DataUnavailable section="Topic 侧栏告警" />
      </WorkPanel>
    </aside>
  );
}

function FieldQualitySideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-field-rail">
      <WorkPanel title="字段质量异常（近 24 小时）">
        <DataUnavailable section="字段质量侧栏统计" />
      </WorkPanel>
    </aside>
  );
}

function StorageQualitySideRail({ dataQualityVisuals }: { dataQualityVisuals?: DataQualityVisuals }) {
  const alerts = dataQualityVisuals?.storageRailAlerts ?? [];
  const locateRowsForRail = dataQualityVisuals?.storageRailLocateRows ?? [];
  const repairRowsForRail = dataQualityVisuals?.storageRailRepairRows ?? [];
  const evidenceRowsForRail = dataQualityVisuals?.storageRailEvidenceRows ?? [];
  return (
    <aside className="taf-data-quality-rail taf-data-quality-storage-rail">
      <WorkPanel title="存储质量异常（近 24 小时）">
        <div className="taf-data-quality-topic-rail-alerts taf-data-quality-storage-rail-alerts">
          {alerts.map(([level, title, detail, time, tone]) => (
            <button key={`${title}-${time}`} type="button" className={`is-${tone}`} title={`${level} ${title} ${detail} ${time}`}>
              <b>{level}</b>
              <strong>{title}</strong>
              <span>{detail}</span>
              <em>{time}</em>
            </button>
          ))}
        </div>
      </WorkPanel>
      <WorkPanel title="快速定位">
        <StorageRailLinks rows={locateRowsForRail} />
      </WorkPanel>
      <WorkPanel title="修复建议">
        <StorageRailLinks rows={repairRowsForRail} />
      </WorkPanel>
      <WorkPanel title="证据与报告">
        <StorageRailLinks rows={evidenceRowsForRail} />
      </WorkPanel>
    </aside>
  );
}

function StorageRailLinks({ rows }: { rows: string[] }) {
  const icons = [<SearchOutlined />, <SyncOutlined />, <DatabaseOutlined />, <ApiOutlined />, <FileSearchOutlined />, <DownloadOutlined />];
  return (
    <div className="taf-data-quality-topic-rail-links taf-data-quality-storage-rail-links">
      {rows.map((label, index) => (
        <button key={label} type="button" title={label}>
          {icons[index % icons.length]}
          <span>{label}</span>
          <ArrowUpOutlined />
        </button>
      ))}
    </div>
  );
}

function FlinkQualitySideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-flink-rail">
      <WorkPanel title="Flink 质量异常">
        <DataUnavailable section="Flink 侧栏告警" />
      </WorkPanel>
    </aside>
  );
}

function ReportSideRail({ report }: { report?: DataQualityDailyReport }) {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-report-rail">
      <WorkPanel title="报告异常（近 24 小时）">
        {report ? <ReportPlainTable columns={['异常类型', '负责人', '状态']} rows={report.anomalies.map((item) => [item.type, item.owner, item.status])} /> : <DataUnavailable section="报告异常" />}
      </WorkPanel>
      <WorkPanel title="证据与报告">
        {report ? <DenseRows columns={['证据', '值']} rows={report.evidence.map((item) => [item.label, item.value])} /> : <DataUnavailable section="报告证据" />}
      </WorkPanel>
    </aside>
  );
}

function SettingsSideRail() {
  return (
    <aside className="taf-data-quality-rail taf-data-quality-settings-rail">
      <WorkPanel title="设置异常（近 24 小时）">
        <DataUnavailable section="质量设置" />
      </WorkPanel>
    </aside>
  );
}

function TopicHeatmap({ visuals }: { visuals?: DataQualityVisuals }) {
  const heatmap = visuals?.heatmap ?? [];
  const times = visuals?.heatmapTimes ?? [];
  if (heatmap.length === 0 || times.length === 0) return <DataUnavailable section="Topic 分区倾斜热力图" />;
  return (
    <div className="taf-data-quality-heatmap">
      <DataQualityHeatmapChart ariaLabel="Topic 分区倾斜热力图" className="taf-data-quality-heatmap-echart" rows={heatmap} times={times} />
    </div>
  );
}

function LatencyTrend() {
  return <DataUnavailable section="消费延迟趋势" />;
}

function useDataQualityPagination<T>(rows: readonly T[], pageSize = 5, dataset?: DataQualityTableDataset) {
  const [page, setPage] = useState(1);
  const useScrollTables = useContext(DataQualityScrollTablesContext);
  const serverPaginationEnabled = useContext(DataQualityServerPaginationContext) && Boolean(dataset);
  const requestPage = useScrollTables ? 1 : page;
  const requestPageSize = useScrollTables ? 100 : pageSize;
  const serverPage = useQuery({
    queryKey: ['data-quality-table-page', dataset, requestPage, requestPageSize],
    queryFn: () => fetchDataQualityTablePage<T>(dataset as DataQualityTableDataset, requestPage, requestPageSize),
    enabled: serverPaginationEnabled,
    placeholderData: (previous) => previous,
  });
  const total = serverPaginationEnabled && serverPage.data ? serverPage.data.total : rows.length;
  const totalPages = useScrollTables ? 1 : Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, totalPages);
  return {
    currentPage,
    dataset,
    pageSize,
    total,
    totalPages,
    visibleRows: serverPaginationEnabled && serverPage.data ? serverPage.data.items : useScrollTables ? rows : rows.slice((currentPage - 1) * pageSize, currentPage * pageSize),
    onChange: setPage,
  };
}

function DataQualityPagination({ currentPage, dataset, label, onChange, pageSize, total, totalPages }: { currentPage: number; dataset?: DataQualityTableDataset; label: string; onChange: (page: number) => void; pageSize: number; total: number; totalPages: number }) {
  const useScrollTables = useContext(DataQualityScrollTablesContext);
  if (useScrollTables) return null;
  return (
    <footer className="taf-data-quality-table-pagination" aria-label={`${label}分页`} data-pagination-source={dataset ? 'server' : 'local'} data-pagination-dataset={dataset}>
      <span>共 {total} 条</span>
      <div>
        <button type="button" title="上一页" aria-label={`${label}上一页`} data-dq-action-managed="true" disabled={currentPage === 1} onClick={() => onChange(currentPage - 1)}>
          <LeftOutlined />
        </button>
        {Array.from({ length: totalPages }, (_, index) => index + 1).map((page) => (
          <button key={page} type="button" data-dq-action-managed="true" className={page === currentPage ? 'is-active' : ''} aria-current={page === currentPage ? 'page' : undefined} onClick={() => onChange(page)}>
            {page}
          </button>
        ))}
        <button type="button" title="下一页" aria-label={`${label}下一页`} data-dq-action-managed="true" disabled={currentPage === totalPages} onClick={() => onChange(currentPage + 1)}>
          <RightOutlined />
        </button>
      </div>
      <span>{pageSize} 条/页</span>
    </footer>
  );
}

function DenseRows({ columns, dataset, pageSize = 5, rows }: { columns: string[]; dataset?: DataQualityTableDataset; pageSize?: number; rows: string[][] }) {
  const paging = useDataQualityPagination(rows, pageSize, dataset);
  return (
    <div className="taf-data-quality-dense-rows taf-data-quality-paged-table" style={{ '--dq-columns': columns.length } as CSSProperties}>
      <div className="taf-data-quality-dense-head">
        {columns.map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map((row) => (
        <div key={row.join('-')} className="taf-data-quality-dense-row">
          {row.map((cell, index) => (
            <span key={`${cell}-${index}`} title={cell}>
              {cell}
            </span>
          ))}
        </div>
      ))}
      <DataQualityPagination label={columns[0] ?? '数据'} {...paging} />
    </div>
  );
}

function MessageSizeDistribution({ visuals }: { visuals?: DataQualityVisuals }) {
  const bars = visuals?.messageSizeDistribution ?? [];
  const topicRows = visuals?.messageSizeTopicRows ?? [];
  if (bars.length === 0 && topicRows.length === 0) return <DataUnavailable section="消息大小吞吐分布" />;
  return (
    <div className="taf-data-quality-message-size">
      <div className="taf-data-quality-bars">
        {bars.map(({ label, value }) => (
          <div key={label} title={`${label} ${value}%`}>
            <span>{label}</span>
            <i style={{ height: `${Math.min(value * 2.25, 100)}%` }} />
            <em>{value}%</em>
          </div>
        ))}
      </div>
      <div className="taf-data-quality-dense-rows taf-data-quality-scroll-table taf-data-quality-message-size-rows" style={{ '--dq-columns': 5 } as CSSProperties}>
        <div className="taf-data-quality-dense-head">
          {['Topic', '平均大小(KB)', '最大大小(KB)', '吞吐 EPS', '压缩比'].map((column) => <span key={column} title={column}>{column}</span>)}
        </div>
        <div className="taf-data-quality-scroll-body">
          {topicRows.map((row) => (
            <div key={row.join('-')} className="taf-data-quality-dense-row">
              {row.map((cell, index) => <span key={`${cell}-${index}`} title={cell}>{cell}</span>)}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function PartitionQueue({ rows }: { rows: string[][] }) {
  const paging = useDataQualityPagination(rows, 5, 'partitionQueueRows');
  return (
    <div className="taf-data-quality-partition-queue taf-data-quality-paged-table">
      <div className="taf-data-quality-partition-head">
        {['Topic', '分区', '异常指标', '根因分析', '建议动作', '操作'].map((column) => (
          <span key={column} title={column}>
            {column}
          </span>
        ))}
      </div>
      {paging.visibleRows.map(([topic, partition, metric, reason, action, operation]) => (
        <div key={`${topic}-${partition}`} className="taf-data-quality-partition-row">
          <strong title={topic}>{topic}</strong>
          <span title={partition}>{partition}</span>
          <span title={metric}>{metric}</span>
          <span title={reason}>{reason}</span>
          <span title={action}>{action}</span>
          <em title={`${operation} ${topic} 分区 ${partition}`}>{operation}</em>
        </div>
      ))}
      <footer className="taf-data-quality-partition-actions">
        <span title="定位 Kafka Topic">
          <SearchOutlined /> 定位 Kafka Topic
        </span>
        <span title="定位 Flink 作业">
          <ApiOutlined /> 定位 Flink 作业
        </span>
        <span title="创建修复任务">
          <SafetyCertificateOutlined /> 创建修复任务
        </span>
      </footer>
      <DataQualityPagination label="异常分区处置队列" {...paging} />
    </div>
  );
}

function FlinkQuality({ evidence, visuals }: { evidence: PageSnapshot['evidence']; visuals?: DataQualityVisuals }) {
  const items = visuals?.flinkMetrics ?? [];
  return (
    <div className="taf-data-quality-flink">
      <div className="taf-data-quality-flink-list">
        {items.map(({ description, label, status, value }) => (
          <div key={label} className={`is-${status}`} title={`${label} ${value} ${description}`}>
            <span title={label}>{label}</span>
            <strong title={value}>{value}</strong>
            <em title={description}>{description}</em>
          </div>
        ))}
      </div>
      <FlinkWatermarkTrend evidence={evidence} trend={visuals?.flinkTrend} />
    </div>
  );
}

function FlinkWatermarkTrend({ evidence, trend }: { evidence: PageSnapshot['evidence']; trend?: DataQualityVisuals['flinkTrend'] }) {
  if (!trend) return <DataUnavailable section="Watermark 延迟趋势" />;
  const chart = trend;
  return (
    <div className="taf-data-quality-flink-trend" title={evidence.map((item) => `${item.label} ${item.value}`).join(' / ')}>
      <header>
        <span>Watermark 延迟趋势</span>
        <em>P50 / P95 / 阈值</em>
      </header>
      <DataQualityTrendChart
        ariaLabel="Watermark 延迟 P50 P95 阈值趋势"
        className="taf-data-quality-watermark-echart"
        categories={chart.times}
        series={[
          { name: 'P50', color: '#18a8ff', values: chart.p50 },
          { name: 'P95', color: '#ffb020', values: chart.p95 },
          {
            name: '阈值',
            color: '#ff4d4f',
            dashed: true,
            values: chart.threshold,
          },
        ]}
      />
    </div>
  );
}

function FieldQuality({ rows }: { rows?: DataQualityVisuals['fieldQualityRows'] }) {
  const paging = useDataQualityPagination(rows ?? [], 5, 'fieldQualityRows');
  return (
    <div className="taf-data-quality-field taf-data-quality-paged-table">
      <div>
        <span title="字段">字段</span>
        <span title="完整率">完整率</span>
        <span title="准确率">准确率</span>
        <span title="缺失率">缺失率</span>
        <span title="异常率">异常率</span>
        <span title="唯一值占比">唯一值占比</span>
        <span title="趋势">趋势</span>
        <span title="状态">状态</span>
        <span title="操作">操作</span>
      </div>
      {paging.visibleRows.map(([field, completeness, accuracy, missing, abnormal, uniqueRate]) => (
        <button key={field} type="button" title={`${field} 完整率 ${completeness} 准确率 ${accuracy} 缺失率 ${missing} 异常率 ${abnormal} 唯一值占比 ${uniqueRate}`}>
          <strong title={field}>{field}</strong>
          <span title={completeness}>{completeness}</span>
          <span title={accuracy}>{accuracy}</span>
          <span title={missing}>{missing}</span>
          <span title={abnormal}>{abnormal}</span>
          <span title={uniqueRate}>{uniqueRate}</span>
          <span className="taf-data-quality-field-trend" title="当前接口未提供字段趋势">--</span>
          <span title="状态由服务端字段矩阵决定">--</span>
          <span className="taf-data-quality-field-action" title="查看字段详情"><FileSearchOutlined /></span>
        </button>
      ))}
      <DataQualityPagination label="字段质量矩阵" {...paging} />
    </div>
  );
}

function ReconciliationReport({ rows }: { rows?: DataQualityVisuals['replayReconcileSummary'] }) {
  if (!rows) return <DataUnavailable section="对账报告" />;
  return <DenseRows columns={['指标', '值', '状态']} pageSize={4} rows={rows} />;
}

function QualityAnomalies({ checks, onNavigate }: { checks?: DataQualityCheck[]; onNavigate: (tab: DataQualityTabSlug) => void }) {
  if (!checks) return <DataUnavailable section="质量异常告警" />;
  const anomalies = checks.filter((check) => check.status !== 'pass');
  if (anomalies.length === 0) {
    return <Alert type="success" showIcon message="当前快照未发现质量异常" />;
  }
  return (
    <div className="taf-data-quality-anomalies">
      {anomalies.map((check) => {
        const target = dataQualityCheckTargets[check.name];
        const level = check.status === 'fail' ? '失败' : check.status === 'warn' ? '告警' : '未测量';
        const tone = check.status === 'fail' ? 'risk' : check.status === 'warn' ? 'warn' : 'info';
        const title = target?.label ?? check.name;
        return (
        <button
          key={check.name}
          type="button"
          className={`is-${tone}`}
          title={`${level} ${title} ${check.message}${check.source ? `；来源 ${check.source}` : ''}`}
          aria-label={`${level} ${title}：${check.message}`}
          data-check-name={check.name}
          data-check-status={check.status}
          data-check-measured={String(check.measured)}
          data-check-source={check.source}
          data-dq-action-managed="true"
          onClick={() => target && onNavigate(target.tab)}
          disabled={!target}
        >
          <i aria-hidden="true" />
          <strong title={title}>{title}</strong>
          <span title={level}>{level}</span>
          <em title={check.message}>{check.message}</em>
        </button>
        );
      })}
    </div>
  );
}

function QualityCheckLinks({ checks, mode, onNavigate }: { checks?: DataQualityCheck[]; mode: 'locate' | 'repair'; onNavigate: (tab: DataQualityTabSlug) => void }) {
  if (!checks) return <DataUnavailable section={mode === 'locate' ? '快速定位' : '质量修复建议'} />;
  const candidates = checks
    .filter((check) => dataQualityCheckTargets[check.name])
    .filter((check) => mode === 'locate' || (check.measured && check.status !== 'pass'))
    .sort((left, right) => qualityCheckPriority(right.status) - qualityCheckPriority(left.status));
  if (candidates.length === 0) {
    if (mode === 'repair' && checks.some((check) => !check.measured)) {
      return <Alert type="warning" showIcon message="存在未测量链路，恢复采集后再判定修复" />;
    }
    return <Alert type="success" showIcon message={mode === 'repair' ? '当前快照没有待修复质量项' : '当前快照没有可定位检查'} />;
  }
  return (
    <div className="taf-data-quality-replay-rail-links">
      {candidates.map((check) => {
        const target = dataQualityCheckTargets[check.name];
        return (
          <button
            key={check.name}
            type="button"
            title={`${target.label}：${check.message}${check.source ? `；来源 ${check.source}` : ''}`}
            aria-label={`${mode === 'locate' ? '定位' : '修复'} ${target.label}：${check.message}`}
            data-check-name={check.name}
            data-check-status={check.status}
            data-check-measured={String(check.measured)}
            data-check-source={check.source}
            data-dq-action-managed="true"
            onClick={() => onNavigate(target.tab)}
          >
            {mode === 'locate' ? <SearchOutlined /> : <SyncOutlined />}
            <span title={target.label}>{target.label}</span>
            <ArrowUpOutlined />
          </button>
        );
      })}
    </div>
  );
}

const qualityCheckPriority = (status: DataQualityCheck['status']) => status === 'fail' ? 3 : status === 'warn' ? 2 : status === 'unknown' ? 1 : 0;

function EvidenceActions({ evidence }: { evidence: PageSnapshot['evidence'] }) {
  const actions: Array<[string, ReactNode]> = [
    ['生成质量报告', <FileDoneOutlined key="report" />],
    ['导出对账报告', <DownloadOutlined key="download" />],
    ['合规审计证据', <FileSearchOutlined key="evidence" />],
    ['SLA 验收包', <SafetyCertificateOutlined key="sla" />],
  ];
  return (
    <div className="taf-data-quality-actions" title={evidence.map((item) => `${item.label} ${item.value}`).join(' / ')}>
      <div className="taf-data-quality-action-grid">
        {actions.map(([label, icon]) => (
          <Button key={String(label)} size="small" icon={icon} title={String(label)}>
            {label}
          </Button>
        ))}
      </div>
    </div>
  );
}

const defaultDLQReplayRequest = (): DLQReplayFallbackRequest => {
  const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
  const approvalId = `APPROVAL-${stamp}-DQ-DLQ`;
  return buildDLQReplayDryRunRequest({
    approved_by: 'operator-2',
    approval_id: approvalId,
    idempotency_key: `tenant-a:${approvalId}:dry-run`,
    reason: 'schema repair 后验证 fallback 文件可安全回放',
    repair_summary: '已完成 schema drift 和字段缺失修复，先执行 dry-run 预检',
  });
};

const errorText = (value: unknown) => (value instanceof Error ? value.message : 'DLQ replay dry-run 请求失败');

const formatBytes = (value: number) => {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MiB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KiB`;
  return `${value} B`;
};

const unavailableMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: '-',
  delta: '暂不可用',
  status: 'warn',
});

const formatQualityScore = (score: number | null) => (score === null ? '--' : `${score.toFixed(0)} 分`);

const parseCompactNumber = (value: string): number => {
  const normalized = value.trim().replace(/,/g, '').toUpperCase();
  const numeric = Number.parseFloat(normalized);
  if (!Number.isFinite(numeric)) return 0;
  if (normalized.endsWith('B')) return numeric * 1_000_000_000;
  if (normalized.endsWith('M')) return numeric * 1_000_000;
  if (normalized.endsWith('K')) return numeric * 1_000;
  return numeric;
};

function DataUnavailable({ section }: { section: string }) {
  return <Alert type="warning" showIcon message={`${section}暂不可用`} description="当前服务端快照未返回该分区，页面不会以静态业务数据替代。" />;
}
