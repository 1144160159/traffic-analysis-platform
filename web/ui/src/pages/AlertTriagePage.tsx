import {
  BlockOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  DownloadOutlined,
  EyeOutlined,
  FilterOutlined,
  FileTextOutlined,
  MoreOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, DatePicker, Drawer, Input, Radio, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import type { Key, ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { RiskScoreRingChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { batchUpdateAlertStatus } from '@/services/alertBatchApi';
import { submitAlertFeedback } from '@/services/alertDetailApi';
import { submitAlertTriageAction } from '@/services/alertTriageApi';
import { fetchAlertSavedViews } from '@/services/alertTriageApi';
import { batchAssignAlerts, exportAlertQueueCsv } from '@/services/alertQueueActionsApi';
import { pageApiPlans } from '@/services/pageApiPlans';
import { alertAllowedNextStatuses, alertStatusLabel, alertStatusOptions, canTransitionAlertStatus, type AlertStatusCode } from '@/services/alertStatus';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type FeedbackResult = 'tp' | 'fp' | 'pending';
type AlertAction = { title: string; alertId: string; target: string; endpoint: string; auditEvent: string; kind: 'saved-view' | 'response-action' | 'investigation-note' };

export function AlertTriagePage({ route }: { route: NavRoute }) {
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const sourceEntity = searchParams.get('entity') ?? '';
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const [selectedRowKeys, setSelectedRowKeys] = useState<Key[]>([]);
  const [feedbackResult, setFeedbackResult] = useState<FeedbackResult>('tp');
  const [feedbackReason, setFeedbackReason] = useState('FALSE_ALARM');
  const [feedbackComment, setFeedbackComment] = useState('告警中心研判反馈');
  const [batchTargetStatus, setBatchTargetStatus] = useState<AlertStatusCode>('triage');
  const [batchReason, setBatchReason] = useState('批量研判状态同步');
  const [batchAssignee, setBatchAssignee] = useState('security-analyst');
  const [listPage, setListPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [view, setView] = useState('自定义视图');
  const [filterNotice, setFilterNotice] = useState('当前队列');
  const [filters, setFilters] = useState({ source: '', asset: sourceEntity || '全部资产', destination: '', rule: '全部规则', model: '全部模型', phase: '全部阶段', status: '全部状态', confidence: '全部' });
  const [appliedFilters, setAppliedFilters] = useState(filters);
  const [timeWindow, setTimeWindow] = useState<[number, number]>(() => [Date.now() - 24 * 60 * 60 * 1_000, Date.now()]);
  const [appliedTimeWindow, setAppliedTimeWindow] = useState(timeWindow);
  const [action, setAction] = useState<AlertAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const [actionReason, setActionReason] = useState('安全运营人员确认提交');
  const visualBreakdownMode = isVisualBreakdownMode();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id, sourceEntity, listPage, pageSize, appliedFilters, appliedTimeWindow],
    queryFn: () => fetchPageSnapshot(route.id, {
      sourceEntity,
      page: listPage,
      pageSize,
      alertFilters: {
        status: appliedFilters.status === '全部状态' ? undefined : appliedFilters.status,
		srcIp: appliedFilters.source.trim() || undefined,
        dstIp: appliedFilters.destination.trim() || undefined,
        assetIp: appliedFilters.asset === '全部资产' ? undefined : appliedFilters.asset.trim(),
        ruleVersion: appliedFilters.rule === '全部规则' ? undefined : appliedFilters.rule,
        modelVersion: appliedFilters.model === '全部模型' ? undefined : appliedFilters.model,
        attackPhase: appliedFilters.phase === '全部阶段' ? undefined : appliedFilters.phase,
        minScore: appliedFilters.confidence === '>=0.9' ? 0.9 : appliedFilters.confidence === '>=0.7' ? 0.7 : undefined,
        startTime: appliedTimeWindow[0],
        endTime: appliedTimeWindow[1],
      },
    }),
    refetchInterval: visualBreakdownMode ? false : 15_000,
  });
  const { data: savedViews = [], refetch: refetchSavedViews } = useQuery({ queryKey: ['alert-saved-views'], queryFn: fetchAlertSavedViews });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedRow = useMemo(() => {
    if (!rows.length) return undefined;
    return rows.find((row) => rowKey(row) === selectedRowKey) ?? rows[0];
  }, [rows, selectedRowKey]);
  const selectedAlertID = alertIdFromRow(selectedRow);
  const selectedRows = useMemo(() => rows.filter((row) => selectedRowKeys.includes(rowKey(row))), [rows, selectedRowKeys]);
  const allowedBatchStatuses = useMemo(() => {
    const candidates = selectedRows.length ? selectedRows : selectedRow ? [selectedRow] : [];
    if (!candidates.length) return [];
    return candidates
      .map((row) => alertAllowedNextStatuses(text(row, '__status', text(row, '状态', ''))))
      .reduce((common, next) => common.filter((status) => next.includes(status)));
  }, [selectedRow, selectedRows]);
  const effectiveBatchTargetStatus = allowedBatchStatuses.includes(batchTargetStatus) ? batchTargetStatus : allowedBatchStatuses[0];
  const filterFacets = useMemo(() => ({
    rules: realFacetValues(rows, '__ruleVersion'),
    models: realFacetValues(rows, '__modelVersion'),
    phases: realFacetValues(rows, '__attackPhase'),
  }), [rows]);
  const canSubmitBatchStatus = Boolean(
    selectedRows.length > 0 &&
      effectiveBatchTargetStatus &&
      selectedRows.every((row) => canTransitionAlertStatus(text(row, '__status', text(row, '状态', '')), effectiveBatchTargetStatus)) &&
      batchReason.trim().length >= 4,
  );
  const batchStatusMutation = useMutation({
    mutationFn: () => {
      if (!selectedRows.length || !effectiveBatchTargetStatus) throw new Error('请选择可迁移的告警状态');
      return batchUpdateAlertStatus(
        selectedRows.map((row) => ({ alertId: alertIdFromRow(row), stateVersion: stateVersionFromRow(row) })),
        effectiveBatchTargetStatus,
        batchReason,
      );
    },
    onSuccess: async (result) => {
      if (result.failedCount > 0) {
        message.warning(`批量状态变更失败 ${result.failedCount} 条`);
      } else {
        message.success(`批量状态变更已提交：${alertStatusLabel(effectiveBatchTargetStatus ?? '')}`);
      }
      await refetch();
    },
    onError: (mutationError) => {
      message.error(mutationError instanceof Error ? mutationError.message : '批量状态变更提交失败');
    },
  });
  const batchAssignMutation = useMutation({
    mutationFn: () => batchAssignAlerts(selectedRows.map(alertIdFromRow), batchAssignee),
    onSuccess: async (result) => { message.success(`已指派 ${result.success} 条告警`); await refetch(); },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '批量指派失败'),
  });
  const exportMutation = useMutation({
    mutationFn: () => exportAlertQueueCsv({
      status: appliedFilters.status === '全部状态' ? undefined : appliedFilters.status,
      sourceIp: appliedFilters.source.trim() || undefined,
      destinationIp: appliedFilters.destination.trim() || undefined,
      assetIp: appliedFilters.asset === '全部资产' ? undefined : appliedFilters.asset.trim(),
      ruleVersion: appliedFilters.rule === '全部规则' ? undefined : appliedFilters.rule,
      modelVersion: appliedFilters.model === '全部模型' ? undefined : appliedFilters.model,
      attackPhase: appliedFilters.phase === '全部阶段' ? undefined : appliedFilters.phase,
      minScore: appliedFilters.confidence === '>=0.9' ? 0.9 : appliedFilters.confidence === '>=0.7' ? 0.7 : undefined,
      startTime: appliedTimeWindow[0],
      endTime: appliedTimeWindow[1],
    }),
    onSuccess: () => message.success('告警队列 CSV 已生成'),
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '告警导出失败'),
  });
  const actionMutation = useMutation({
    mutationFn: () => {
      if (!action) throw new Error('请选择告警操作');
      return submitAlertTriageAction({
        kind: action.kind,
        alertId: action.kind === 'saved-view' ? undefined : action.alertId,
        action: action.title,
        target: action.target,
        reason: actionReason,
        dryRun: action.kind === 'response-action',
        detail: { filter_notice: filterNotice, view, filters: appliedFilters, time_window: appliedTimeWindow },
      });
    },
    onSuccess: async (result) => {
      setActionSubmitted(true);
      if (action?.kind === 'saved-view') await refetchSavedViews();
      if (action?.kind === 'response-action') {
        message.success(result.outbox_status === 'published' ? '响应请求已持久化并发送至审批队列' : '响应请求已持久化，后台正在重试投递');
      } else {
        message.success('告警操作已持久化并写入审计日志');
      }
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '告警操作提交失败'),
  });
  const feedbackMutation = useMutation({
    mutationFn: () => {
      if (!selectedAlertID) throw new Error('请选择告警后再提交反馈');
      if (feedbackResult === 'pending') throw new Error('待确认状态无需提交 TP/FP 反馈');
      return submitAlertFeedback(selectedAlertID, {
        label: feedbackResult === 'tp' ? 'TP' : 'FP',
        reasonCode: feedbackResult === 'fp' ? feedbackReason : undefined,
        comment: feedbackComment,
      });
    },
    onSuccess: async () => {
      message.success('告警反馈已持久化并进入反馈闭环');
      await refetch();
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '告警反馈提交失败'),
  });

  const tableColumns = useMemo(() => [...route.page.tableColumns, '操作'], [route.page.tableColumns]);
  const columns: ColumnsType<SnapshotRow> = tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: alertColumnWidth(column),
    ellipsis: true,
    render: (value, record) => renderAlertCell(column, value, record, (title) => {
      const alertId = alertIdFromRow(record);
      if (title === '查看告警详情') navigate(`/alerts/${encodeURIComponent(alertId)}`);
      else openAction(title, alertId);
    }),
  }));
  const totalRows = data?.total ?? rows.length;
  function openAction(title: string, target?: string) {
    setActionSubmitted(false);
    const isResponse = /隔离|阻断|封禁|脚本|工单|剧本|白名单/.test(title);
    const resolvedTarget = target ?? (isResponse && /IP|阻断|封禁/.test(title) ? text(selectedRow, '源 IP', selectedAlertID) : isResponse && title.includes('隔离') ? text(selectedRow, '受影响资产', selectedAlertID) : selectedAlertID);
    setAction(createAlertAction(title, resolvedTarget, selectedAlertID));
  }
  const applyFilters = () => {
    setListPage(1);
    setAppliedFilters(filters);
    setAppliedTimeWindow(timeWindow);
    setFilterNotice(`${filters.asset} / ${filters.status} / ${filters.confidence}`);
  };
  const resetFilters = () => {
    const nextFilters = { source: '', asset: sourceEntity || '全部资产', destination: '', rule: '全部规则', model: '全部模型', phase: '全部阶段', status: '全部状态', confidence: '全部' };
    const nextWindow: [number, number] = [Date.now() - 24 * 60 * 60 * 1_000, Date.now()];
    setFilters(nextFilters);
    setAppliedFilters(nextFilters);
    setTimeWindow(nextWindow);
    setAppliedTimeWindow(nextWindow);
    setFilterNotice('当前队列');
    setListPage(1);
  };
  const loadSavedView = (name: string) => {
    setView(name);
    const saved = savedViews.find((candidate) => candidate.name === name);
    const stored = saved?.filters;
    if (stored && typeof stored === 'object' && !Array.isArray(stored)) {
      const storedRecord = stored as Record<string, unknown>;
      const restored = { ...filters, ...(storedRecord.filters && typeof storedRecord.filters === 'object' ? storedRecord.filters : storedRecord) } as typeof filters;
      setFilters(restored);
      setAppliedFilters(restored);
      const storedWindow = storedRecord.time_window;
      if (Array.isArray(storedWindow) && storedWindow.length === 2 && storedWindow.every((value) => Number.isFinite(Number(value)))) {
        const restoredWindow: [number, number] = [Number(storedWindow[0]), Number(storedWindow[1])];
        setTimeWindow(restoredWindow);
        setAppliedTimeWindow(restoredWindow);
      }
      setFilterNotice(`已加载视图：${name}`);
      setListPage(1);
    }
  };
  const applyClusterFilter = (label: string) => {
    const next = { ...filters };
    if (label === '同源 IP') {
      next.source = text(selectedRow, '源 IP', '');
      next.asset = '全部资产';
    }
    if (label === '同资产') {
      next.source = '';
      next.asset = text(selectedRow, '受影响资产', '全部资产');
    }
    if (label === '同攻击链' || label === '当前阶段') next.phase = text(selectedRow, '__attackPhase', '全部阶段');
    if (label === '同规则/模型') {
      const ruleVersion = text(selectedRow, '__ruleVersion', '');
      const modelVersion = text(selectedRow, '__modelVersion', '');
      if (ruleVersion) next.rule = ruleVersion;
      else if (modelVersion) next.model = modelVersion;
    }
    setFilters(next);
    setAppliedFilters(next);
    setFilterNotice(`关联簇：${label}`);
    setListPage(1);
    message.success(`已按“${label}”收敛告警队列`);
  };

  return (
    <div className="taf-page taf-alert-triage">
      <div className="taf-alert-grid">
        <main className="taf-alert-main">
          <header className="taf-alert-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              {sourceEntity && <span className="taf-source-context" data-source-entity={sourceEntity}>关联实体：{sourceEntity}</span>}
            </div>
            <Space>
              <Select size="small" value={view} options={Array.from(new Set(['自定义视图', ...savedViews.map((saved) => saved.name)])).map((value) => ({ value }))} onChange={loadSavedView} />
              <Button size="small" onClick={() => openAction('保存告警视图', view)}>保存视图</Button>
              <Tooltip title="刷新告警队列">
                <Button icon={<ReloadOutlined />} size="small" onClick={() => void refetch()} />
              </Tooltip>
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 APISIX 路由、后端服务、鉴权或网络连通性。'}
              action={
                <Button size="small" danger onClick={() => void refetch()}>
                  重试
                </Button>
              }
            />
          )}

          <WorkPanel title="告警队列">
            <div className="taf-alert-kpis">
              {(data?.metrics ?? []).map((metric) => (
                <AlertQueueMetric key={metric.label} metric={metric} />
              ))}
            </div>
          </WorkPanel>

          <WorkPanel title="筛选检索">
            <div className="taf-alert-filter">
              <label>
                <span>时间窗</span>
                <DatePicker.RangePicker
                  size="small"
                  showTime
                  value={[dayjs(timeWindow[0]), dayjs(timeWindow[1])]}
                  onChange={(value) => {
                    if (value?.[0] && value[1]) {
                      setTimeWindow([value[0].valueOf(), value[1].valueOf()]);
                      setFilterNotice('时间窗待查询');
                    }
                  }}
                />
              </label>
              <label>
                <span>资产</span>
                <Input size="small" value={filters.asset === '全部资产' ? '' : filters.asset} prefix={<SearchOutlined />} placeholder="全部资产或资产 IP" onChange={(event) => setFilters((current) => ({ ...current, asset: event.target.value || '全部资产' }))} />
              </label>
              <label>
                <span>目的 IP</span>
                <Input size="small" value={filters.destination} prefix={<SearchOutlined />} placeholder="请输入 IP" onChange={(event) => setFilters((current) => ({ ...current, destination: event.target.value }))} />
              </label>
              <label>
                <span>规则</span>
                <Select size="small" value={filters.rule} options={['全部规则', ...filterFacets.rules].map((value) => ({ value }))} onChange={(rule) => setFilters((current) => ({ ...current, rule }))} />
              </label>
              <label>
                <span>模型</span>
                <Select size="small" value={filters.model} options={['全部模型', ...filterFacets.models].map((value) => ({ value }))} onChange={(model) => setFilters((current) => ({ ...current, model }))} />
              </label>
              <label>
                <span>攻击阶段</span>
                <Select size="small" value={filters.phase} options={['全部阶段', ...filterFacets.phases].map((value) => ({ value, label: value === '全部阶段' ? value : attackPhaseDisplayLabel(value) }))} onChange={(phase) => setFilters((current) => ({ ...current, phase }))} />
              </label>
              <label>
                <span>状态</span>
                <Select
                  size="small"
                  value={filters.status}
                  options={[{ value: '全部状态', label: '全部状态' }, ...alertStatusOptions]}
                  onChange={(status) => setFilters((current) => ({ ...current, status }))}
                />
              </label>
              <label>
                <span>置信度</span>
                <Select size="small" value={filters.confidence} options={[{ value: '全部' }, { value: '>=0.9' }, { value: '>=0.7' }]} onChange={(confidence) => setFilters((current) => ({ ...current, confidence }))} />
              </label>
              <div className="taf-alert-filter__actions">
                <Button size="small" onClick={resetFilters}>重置</Button>
                <Button size="small" type="primary" onClick={applyFilters}>查询</Button>
              </div>
            </div>
          </WorkPanel>

          <WorkPanel
            className="taf-alert-table-panel"
            title={`${route.page.tableTitle}（共 ${totalRows || 0} 条）`}
            extra={
              <Space>
                  <Select
                    size="small"
                    value={effectiveBatchTargetStatus}
                    style={{ width: 104 }}
                    options={allowedBatchStatuses.map((status) => ({ value: status, label: alertStatusLabel(status) }))}
                    disabled={!selectedRow || allowedBatchStatuses.length === 0}
                    onChange={(value) => setBatchTargetStatus(value)}
                  />
                  <Input
                    size="small"
                    value={batchReason}
                    style={{ width: 160 }}
                    onChange={(event) => setBatchReason(event.target.value)}
                  />
                  <Input size="small" value={batchAssignee} style={{ width: 120 }} onChange={(event) => setBatchAssignee(event.target.value)} placeholder="指派对象" />
                  <Button size="small" icon={<SafetyCertificateOutlined />} disabled={!selectedRows.length || !batchAssignee.trim()} loading={batchAssignMutation.isPending} onClick={() => batchAssignMutation.mutate()}>批量指派</Button>
                  <Button
                    size="small"
                    icon={<CheckCircleOutlined />}
                    disabled={!canSubmitBatchStatus}
                    loading={batchStatusMutation.isPending}
                    onClick={() => batchStatusMutation.mutate()}
                  >
                    批量状态变更
                  </Button>
                  <Button size="small" icon={<DownloadOutlined />} loading={exportMutation.isPending} onClick={() => exportMutation.mutate()}>导出</Button>
                  <Tooltip title={`筛选状态：${filterNotice}`}><Button size="small" icon={<FilterOutlined />} onClick={applyFilters} /></Tooltip>
              </Space>
            }
          >
            <Table
              className="taf-alert-table"
              rowKey={rowKey}
              size="small"
              loading={isLoading}
              columns={columns}
              dataSource={rows}
              scroll={{ x: 1080, y: 340 }}
              pagination={{
                current: listPage,
                pageSize,
                total: totalRows,
                size: 'small',
                showSizeChanger: true,
                showQuickJumper: true,
                pageSizeOptions: ['10', '20', '50'],
                onChange: (nextPage, nextPageSize) => { setListPage(nextPage); setPageSize(nextPageSize); },
              }}
              rowSelection={{ selectedRowKeys, onChange: setSelectedRowKeys }}
              onRow={(record) => ({
                onClick: () => setSelectedRowKey(rowKey(record)),
              })}
            />
          </WorkPanel>
        </main>

        <aside className="taf-alert-detail">
          <AlertSummary row={selectedRow} onAction={(title, target) => {
            if (title === '进入告警详情' && target) navigate(`/alerts/${encodeURIComponent(target)}`);
            else openAction(title, target);
          }} />
          <TriageTimeline row={selectedRow} timeline={data?.timeline ?? []} />
          <ClusterCards row={selectedRow} rows={rows} onAction={applyClusterFilter} />
          <FeedbackForm
            actions={route.page.actions}
            feedbackResult={feedbackResult}
            onFeedbackResultChange={setFeedbackResult}
            reason={feedbackReason}
            comment={feedbackComment}
            onReasonChange={setFeedbackReason}
            onCommentChange={setFeedbackComment}
            pending={feedbackMutation.isPending}
            submitDisabled={!selectedAlertID || feedbackResult === 'pending'}
            onSubmit={() => feedbackMutation.mutate()}
            onAction={openAction}
          />
        </aside>
      </div>
      <Drawer className="taf-alert-triage-action-drawer" title={action ? `${action.title}确认` : '告警操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); actionMutation.reset(); }} extra={<Button size="small" type="primary" disabled={actionSubmitted || actionReason.trim().length < 4} loading={actionMutation.isPending} onClick={() => actionMutation.mutate()}>{actionSubmitted ? '已持久化' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>{action.kind === 'response-action' ? `将创建“${action.title}”受控响应请求；当前状态为待审批，不宣称已经执行。` : `将持久化“${action.title}”并保留租户、权限和审计上下文。`}</p><dl><dt>告警对象</dt><dd>{action.target || '-'}</dd><dt>业务接口</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl><Input.TextArea rows={3} value={actionReason} onChange={(event) => setActionReason(event.target.value)} placeholder="请输入操作原因（至少 4 个字符）" />{actionMutation.isError && <Alert type="error" showIcon message="告警业务操作提交失败" description={actionMutation.error instanceof Error ? actionMutation.error.message : 'unknown error'} />}{actionSubmitted && <Alert type="success" showIcon message={action?.kind === 'response-action' ? '响应请求待审批' : '告警业务操作已持久化'} description={`记录：${actionMutation.data?.job_id ?? actionMutation.data?.view_id ?? '-'}`} />}</div>}
      </Drawer>
    </div>
  );
}

function AlertQueueMetric({ metric }: { metric: PageSnapshot['metrics'][number] }) {
  const isPositive = !metric.delta.startsWith('-');
  const title = `${metric.label} ${metric.value} ${metric.delta}`;
  return (
    <div className={`taf-alert-kpi-card is-${metric.status}`} title={title}>
      <span className="taf-alert-kpi-card__icon">{alertMetricIcon(metric.label)}</span>
      <span className="taf-alert-kpi-card__label">{metric.label}</span>
      <strong>{compactAlertMetricValue(metric.value)}</strong>
      <small className={isPositive ? 'is-up' : 'is-down'}>{metric.delta}</small>
    </div>
  );
}

const alertMetricIcon = (label: string) => {
  if (label.includes('高') || label.includes('中') || label.includes('低')) return <SafetyCertificateOutlined />;
  if (label.includes('未')) return <FileTextOutlined />;
  if (label.includes('处理')) return <ClockCircleOutlined />;
  if (label.includes('确认')) return <CheckCircleOutlined />;
  return <CloseCircleOutlined />;
};

function AlertSummary({ row, onAction }: { row?: SnapshotRow; onAction: (title: string, target?: string) => void }) {
  const score = riskScore(row);
  const alertId = text(row, '告警 ID', '-');
  const firstSeen = text(row, '首次发生', '-');
  const sourceIp = text(row, '源 IP', '-');
  const destinationIp = text(row, '目的 IP', '-');
  const ruleModel = text(row, '规则/模型', '-');
  const confidence = text(row, '置信度', '-');
  const affectedAsset = text(row, '受影响资产', '-');

  return (
    <WorkPanel title="告警详情" extra={<Button size="small" type="link" onClick={() => onAction('进入告警详情', alertId)}>进入详情</Button>}>
      <div className="taf-alert-summary">
        <AlertRiskDial score={score} />
        <div className="taf-alert-summary__facts">
          <strong title={text(row, '告警名称', '暂无告警')}>{text(row, '告警名称', '暂无告警')}</strong>
          <StatusTag value={text(row, '风险等级', '无数据')} />
          <dl>
            <dt>告警 ID</dt>
            <dd title={alertId}>{alertId}</dd>
            <dt>首次发生</dt>
            <dd title={firstSeen}>{firstSeen}</dd>
            <dt>源 IP:端口</dt>
            <dd title={sourceIp}>{sourceIp}</dd>
            <dt>目的 IP:端口</dt>
            <dd title={destinationIp}>{destinationIp}</dd>
            <dt>规则/模型</dt>
            <dd title={ruleModel}>{ruleModel}</dd>
            <dt>置信度</dt>
            <dd title={confidence}>{confidence}</dd>
            <dt>受影响资产</dt>
            <dd title={affectedAsset}>{affectedAsset}</dd>
          </dl>
        </div>
      </div>
    </WorkPanel>
  );
}

function AlertRiskDial({ score }: { score: number }) {
  return (
    <div className="taf-alert-risk-dial taf-alert-risk-echart"><RiskScoreRingChart value={score} size={96} ariaLabel="选中告警风险评分 ECharts 圆环图" /></div>
  );
}

function TriageTimeline({ row, timeline }: { row?: SnapshotRow; timeline: PageSnapshot['timeline'] }) {
  const items = timeline.length
    ? timeline
    : [
        { title: '暂无时间线', description: '当前筛选范围没有可展示的真实告警事件。', status: 'info' as const },
      ];

  return (
    <WorkPanel title="研判时间线">
      <div className="taf-alert-timeline">
        {items.slice(0, 5).map((item, index) => (
          <div key={`${item.title}-${index}`} className={`taf-alert-timeline__item is-${item.status}`}>
            <i />
            <span>{alertTimelineTime(index, row)}</span>
            <strong>{item.title}</strong>
            <em title={item.description}>{item.description}</em>
          </div>
        ))}
      </div>
    </WorkPanel>
  );
}

const alertTimelineTime = (index: number, row?: SnapshotRow) => {
  if (index === 0) {
    const value = text(row, '首次发生', '');
    const parsed = value ? new Date(value) : undefined;
    if (parsed && Number.isFinite(parsed.getTime())) return parsed.toLocaleTimeString('zh-CN', { hour12: false });
    const match = value.match(/(\d{2}:\d{2}:\d{2})/);
    return match?.[1] ?? '--:--:--';
  }
  return '--:--:--';
};

const compactAlertMetricValue = (value: string) => {
  const normalized = value.replace(/\s*条\s*$/u, '').replace(/\s+/g, '');
  const numeric = Number(normalized.replace(/,/g, ''));
  if (!Number.isFinite(numeric)) return normalized;
  if (Math.abs(numeric) >= 10_000) return `${(numeric / 10_000).toFixed(1)}万`;
  return normalized;
};

function ClusterCards({ row, rows, onAction }: { row?: SnapshotRow; rows: SnapshotRow[]; onAction: (label: string) => void }) {
  const sourceIP = text(row, '源 IP', '');
  const asset = text(row, '受影响资产', '');
  const phase = text(row, '攻击阶段', '');
  const rule = text(row, '规则/模型', '');
  const cards = [
    ['同源 IP', rows.filter((item) => text(item, '源 IP', '') === sourceIP).length],
    ['同资产', rows.filter((item) => text(item, '受影响资产', '') === asset).length],
    ['同攻击链', rows.filter((item) => text(item, '攻击阶段', '') === phase).length],
    ['同规则/模型', rows.filter((item) => text(item, '规则/模型', '') === rule).length],
    ['当前阶段', phase || '-'],
  ];

  return (
    <WorkPanel title="关联告警簇">
      <div className="taf-alert-clusters">
        {cards.map(([label, value]) => (
          <button key={label} type="button" onClick={() => onAction(String(label))}>
            <span>{label}</span>
            <strong>{value}</strong>
          </button>
        ))}
      </div>
    </WorkPanel>
  );
}

function FeedbackForm({
  actions,
  feedbackResult,
  onFeedbackResultChange,
  reason,
  comment,
  onReasonChange,
  onCommentChange,
  pending,
  submitDisabled,
  onSubmit,
  onAction,
}: {
  actions: string[];
  feedbackResult: FeedbackResult;
  onFeedbackResultChange: (value: FeedbackResult) => void;
  reason: string;
  comment: string;
  onReasonChange: (value: string) => void;
  onCommentChange: (value: string) => void;
  pending: boolean;
  submitDisabled: boolean;
  onSubmit: () => void;
  onAction: (title: string, target?: string) => void;
}) {
  return (
    <WorkPanel title="处理与反馈">
      <div className="taf-alert-actions">
        {actions.map((action) => (
          <Tooltip key={action} title={action}>
            <Button size="small" title={action} icon={action === '加入白名单' ? <CloseCircleOutlined /> : <BlockOutlined />} onClick={() => onAction(action)}>
              {action}
            </Button>
          </Tooltip>
        ))}
      </div>
      <div className="taf-alert-feedback">
        <span>反馈结果</span>
        <Radio.Group
          size="small"
          value={feedbackResult}
          onChange={(event) => onFeedbackResultChange(event.target.value as FeedbackResult)}
          options={[
            { label: 'TP（真实告警）', value: 'tp' },
            { label: 'FP（误报）', value: 'fp' },
            { label: '待确认', value: 'pending' },
          ]}
        />
        <Select
          size="small"
          value={reason}
          disabled={feedbackResult !== 'fp'}
          onChange={onReasonChange}
          options={[
            { value: 'WHITELIST', label: '已知白名单行为' },
            { value: 'FALSE_ALARM', label: '规则/模型误报' },
            { value: 'BUSINESS_NORMAL', label: '正常业务行为' },
            { value: 'INSUFFICIENT', label: '证据不足' },
            { value: 'OTHER', label: '其他原因' },
          ]}
        />
        <Input.TextArea rows={3} value={comment} onChange={(event) => onCommentChange(event.target.value)} placeholder="请输入备注信息..." />
        <div className="taf-alert-feedback__footer">
          <Button size="small" onClick={() => onFeedbackResultChange('pending')}>取消</Button>
          <Button size="small" type="primary" loading={pending} disabled={submitDisabled} onClick={onSubmit}>提交反馈</Button>
        </div>
      </div>
    </WorkPanel>
  );
}

const renderAlertCell = (column: string, value: unknown, record: SnapshotRow, onAction: (title: string) => void): ReactNode => {
  if (column === '告警 ID') {
    return (
      <span className="taf-alert-id" title={String(value)}>
        {String(value)}
        <EyeOutlined />
      </span>
    );
  }
  if (isStatusColumn(column)) return <StatusTag value={value} />;
  if (column === '操作') {
    return (
      <Space size={4} className="taf-alert-row-actions">
        <Tooltip title="查看详情">
          <Button size="small" type="text" icon={<EyeOutlined />} onClick={(event) => { event.stopPropagation(); onAction('查看告警详情'); }} />
        </Tooltip>
        <Tooltip title="重新研判">
          <Button size="small" type="text" icon={<ReloadOutlined />} onClick={(event) => { event.stopPropagation(); onAction('重新研判告警'); }} />
        </Tooltip>
        <Tooltip title="补充研判记录">
          <Button size="small" type="text" icon={<MoreOutlined />} onClick={(event) => { event.stopPropagation(); onAction('补充研判记录'); }} />
        </Tooltip>
      </Space>
    );
  }
  if (column === '置信度') return <strong className="taf-alert-confidence">{String(value || text(record, '置信度', '-'))}</strong>;
  const display = String(value ?? '');
  return <span title={display}>{display}</span>;
};

const alertColumnWidth = (column: string) => {
  const widths: Record<string, number> = {
    '告警 ID': 138,
    风险等级: 64,
    告警名称: 108,
    攻击阶段: 80,
    '源 IP': 110,
    '目的 IP': 110,
    受影响资产: 94,
    '规则/模型': 96,
    置信度: 68,
    首次发生: 96,
    状态: 70,
    操作: 58,
  };
  return widths[column] ?? 96;
};

const rowKey = (record: SnapshotRow) => String(record['告警 ID'] ?? record['事件 ID'] ?? JSON.stringify(record));

const createAlertAction = (title: string, target: string, alertId = target): AlertAction => {
  const actions = pageApiPlans['alert-detail'].actions ?? [];
  const actionId = title.includes('导出') ? 'alert-report-export'
    : /详情|证据|关联/.test(title) ? 'alert-evidence-access'
      : /隔离|阻断|剧本|白名单|重新研判/.test(title) ? 'alert-response-request'
        : 'alert-investigation-note';
  const plan = actions.find((item) => item.id === actionId);
  const kind = title.includes('保存告警视图') ? 'saved-view' : /隔离|阻断|封禁|脚本|工单|剧本|白名单|重新研判/.test(title) ? 'response-action' : 'investigation-note';
  return {
    title,
    alertId,
    target,
    endpoint: kind === 'saved-view' ? '/v1/alerts/views' : kind === 'response-action' ? `/v1/alerts/${alertId}/response-actions` : `/v1/alerts/${alertId}/investigation-notes`,
    auditEvent: kind === 'saved-view' ? 'ALERT_VIEW_SAVED' : kind === 'response-action' ? 'ALERT_RESPONSE_ACTION_REQUESTED' : plan?.auditEvent ?? 'ALERT_INVESTIGATION_NOTE_RECORDED',
    kind,
  };
};

const alertIdFromRow = (row: SnapshotRow | undefined) => text(row, '__alertId', text(row, '告警 ID', ''));

const stateVersionFromRow = (row: SnapshotRow | undefined) => {
  const value = row?.__stateVersion;
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? numeric : undefined;
};

const realFacetValues = (rows: SnapshotRow[], key: string) => Array.from(new Set(
  rows.map((row) => text(row, key, '')).filter((value) => value && value !== '-'),
)).sort((left, right) => left.localeCompare(right, 'zh-CN'));

const attackPhaseDisplayLabel = (phase: string) => ({
  reconnaissance: '侦察',
  initial_access: '初始访问',
  execution: '执行',
  persistence: '持久化',
  privilege_escalation: '权限提升',
  defense_evasion: '防御规避',
  credential_access: '凭证访问',
  discovery: '发现',
  lateral_movement: '横向移动',
  collection: '数据收集',
  command_control: '命令与控制',
  exfiltration: '数据外传',
  impact: '影响',
}[phase] ?? phase);

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const riskScore = (row: SnapshotRow | undefined) => {
  const explicitScore = Number(row?.__riskScore);
  if (Number.isFinite(explicitScore) && explicitScore > 0) return Math.min(Math.round(explicitScore), 100);
  const confidence = Number(text(row, '置信度', '0').replace('%', ''));
  if (Number.isFinite(confidence) && confidence > 0) return confidence <= 1 ? Math.round(confidence * 100) : Math.min(confidence, 100);
  const severity = text(row, '风险等级', '');
  if (severity.includes('高') || severity.includes('严重')) return 92;
  if (severity.includes('中')) return 68;
  return severity ? 42 : 0;
};

const isStatusColumn = (column: string) =>
  column.includes('状态') || column.includes('风险') || column.includes('级别') || column.includes('结果');
