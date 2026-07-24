import {
  AuditOutlined,
  CheckCircleOutlined,
  DownloadOutlined,
  ExportOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  HistoryOutlined,
  LinkOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Checkbox, DatePicker, Drawer, Input, Modal, Select, Space, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { Dayjs } from 'dayjs';
import { useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { getAuthToken } from '@/services/authStorage';
import {
  createAuditReview,
  downloadAuditArtifact,
  exportAuditLogs,
  fetchAuditLogDetail,
  fetchAuditLogs,
  saveAuditQuery,
  verifyAuditIntegrity,
  type AuditLogFilters,
  type AuditLogRecord,
} from '@/services/auditGovernanceApi';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';

const { RangePicker } = DatePicker;

type AuditAction = {
  kind: 'detail' | 'save' | 'export' | 'review' | 'integrity' | 'evidence';
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
  auditLogID?: string;
};

export function AuditLogPage({ route }: { route: NavRoute }) {
  const [searchParams] = useSearchParams();
  const sourceObjectId = searchParams.get('object_id') ?? '';
  const sourceObjectType = searchParams.get('object_type') ?? '';
  const queryClient = useQueryClient();
  const detailPanelRef = useRef<HTMLDivElement>(null);
  const [selectedKey, setSelectedKey] = useState<string>();
  const [userFilter, setUserFilter] = useState('全部用户/角色');
  const [tenantFilter, setTenantFilter] = useState('全部租户');
  const [objectFilter, setObjectFilter] = useState(() => auditObjectTypeLabel(sourceObjectType));
  const [actionFilter, setActionFilter] = useState('全部');
  const [resultFilter, setResultFilter] = useState('全部');
  const [requestQuery, setRequestQuery] = useState('');
  const [traceQuery, setTraceQuery] = useState('');
  const [timeRange, setTimeRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [detailTab, setDetailTab] = useState('字段变更对比');
  const [exportFormat, setExportFormat] = useState('PDF');
  const [exportScope, setExportScope] = useState<'selected' | 'query'>('selected');
  const [maskSensitive, setMaskSensitive] = useState(true);
  const [exportConfirmed, setExportConfirmed] = useState(false);
  const [listPage, setListPage] = useState(1);
  const [action, setAction] = useState<AuditAction>();
  const [actionResult, setActionResult] = useState<string>();
  const [reviewReason, setReviewReason] = useState('高风险操作需要二次复核');
  const permissions = readAuditPermissions(getAuthToken());
  const canWrite = hasAuditScope(permissions, 'audit:write');
  const canExport = hasAuditScope(permissions, 'audit:export');
  const pageSize = 10;
  const businessFilters = useMemo<AuditLogFilters>(() => ({
    ...(userFilter !== '全部用户/角色' ? { user_id: userFilter } : {}),
    ...(actionFilter !== '全部' ? { action: actionFilter } : {}),
    ...(objectFilter !== '全部' ? { object_type: auditObjectTypeValue(objectFilter) } : {}),
    ...(resultFilter !== '全部' ? { result: auditResultValue(resultFilter) } : {}),
    ...(requestQuery.trim() ? { request_id: requestQuery.trim() } : {}),
    ...(traceQuery.trim() ? { trace_id: traceQuery.trim() } : {}),
    ...(sourceObjectId ? { object_id: sourceObjectId } : {}),
    ...(timeRange?.[0] ? { start: timeRange[0].valueOf() } : {}),
    ...(timeRange?.[1] ? { end: timeRange[1].valueOf() } : {}),
  }), [actionFilter, objectFilter, requestQuery, resultFilter, sourceObjectId, timeRange, traceQuery, userFilter]);
  const requestFilters = useMemo<AuditLogFilters>(() => ({
    ...businessFilters,
    limit: pageSize,
    offset: (listPage - 1) * pageSize,
  }), [businessFilters, listPage]);
  const { data: auditData, error, isError, isFetching, isLoading, refetch } = useQuery({
    queryKey: ['audit-governance', requestFilters],
    queryFn: () => fetchAuditLogs(requestFilters),
  });
  const records = useMemo(() => auditData?.trails ?? [], [auditData?.trails]);
  const data = useMemo(() => buildAuditSnapshot(route, auditData), [auditData, route]);
  const rows = useMemo(() => records.map(auditRecordToRow), [records]);
  const visibleRows = useMemo(() => tenantFilter === '全部租户' ? rows : rows.filter((row) => String(row.租户 ?? '') === tenantFilter), [rows, tenantFilter]);
  const pageCount = Math.max(1, Math.ceil((auditData?.total ?? visibleRows.length) / pageSize));
  const userOptions = useMemo(() => [{ value: '全部用户/角色', label: '全部用户/角色' }, ...uniqueOptions(records.map((record) => record.user_id).filter(Boolean))], [records]);
  const tenantOptions = useMemo(() => [{ value: '全部租户', label: '全部租户' }, ...uniqueOptions(records.map((record) => record.tenant_id).filter(Boolean))], [records]);
  const actionOptions = useMemo(() => [{ value: '全部', label: '全部' }, ...uniqueOptions(records.map((record) => record.action).filter(Boolean), auditActionLabel)], [records]);
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const selectedRecord = useMemo(() => records.find((record) => record.log_id === String(selected?.记录ID ?? '')) ?? records[0], [records, selected]);
  const detailLogID = detailTab === '操作详情' ? selectedRecord?.log_id : undefined;
  const detailQuery = useQuery({
    queryKey: ['audit-governance-detail', detailLogID],
    queryFn: () => fetchAuditLogDetail(detailLogID!),
    enabled: detailTab === '操作详情' && Boolean(detailLogID),
  });
  const exportSelectedRecord = useMemo(
    () => action?.auditLogID ? records.find((record) => record.log_id === action.auditLogID) : selectedRecord,
    [action?.auditLogID, records, selectedRecord],
  );
  const exportOverLimit = exportScope === 'query' && (auditData?.total ?? 0) > 10_000;
  const canActOnRecord = Boolean(selectedRecord) && !isFetching;
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: column === '操作' ? 172 : undefined,
    fixed: column === '操作' ? 'right' : undefined,
    ellipsis: column !== '操作',
    render: (value, record) => column === '操作'
      ? (
        <div className="taf-auditlog-row-actions">
          <button type="button" className={rowKey(selected) === rowKey(record) && detailTab === '操作详情' ? 'is-active' : ''} aria-pressed={rowKey(selected) === rowKey(record) && detailTab === '操作详情'} aria-label={`查看审计详情 ${rowKey(record)}`} disabled={actionMutation.isPending} onClick={(event) => { event.stopPropagation(); revealDetailTab('操作详情', rowKey(record)); }}>详情</button>
          <span aria-hidden="true">/</span>
          <button type="button" className={rowKey(selected) === rowKey(record) && detailTab === '关联链路' ? 'is-active' : ''} aria-pressed={rowKey(selected) === rowKey(record) && detailTab === '关联链路'} aria-label={`查看关联链路 ${rowKey(record)}`} disabled={actionMutation.isPending} onClick={(event) => { event.stopPropagation(); revealDetailTab('关联链路', rowKey(record)); }}>关联</button>
          <span aria-hidden="true">/</span>
          <button type="button" className={rowKey(selected) === rowKey(record) && detailTab === '复核操作' ? 'is-active' : ''} aria-pressed={rowKey(selected) === rowKey(record) && detailTab === '复核操作'} aria-label={`复核审计记录 ${rowKey(record)}`} disabled={!canWrite || actionMutation.isPending} title={canWrite ? '复核当前审计记录' : '需要 audit:write 权限'} onClick={(event) => { event.stopPropagation(); revealReview(rowKey(record)); }}>复核</button>
        </div>
      )
      : renderAuditCell(column, value),
  }));
  const actionMutation = useMutation({
    mutationFn: async (current: AuditAction) => {
      if (current.kind === 'save') return saveAuditQuery({ name: `审计查询 ${new Date().toLocaleString('zh-CN')}`, filters: businessFilters });
      if (current.kind === 'review') {
        if (!current.auditLogID) throw new Error('请先选择一条审计日志');
        return createAuditReview({ log_id: current.auditLogID, reason: reviewReason.trim() });
      }
      if (current.kind === 'integrity') return verifyAuditIntegrity(businessFilters);
      const filters = exportScope === 'query'
        ? businessFilters
        : current.auditLogID
          ? { log_id: current.auditLogID }
          : undefined;
      if (!filters) throw new Error('当前没有可导出的审计记录');
      const artifact = await exportAuditLogs({ format: exportFormat.toLowerCase() as 'pdf' | 'csv' | 'json', filters, mask_sensitive: maskSensitive });
      downloadAuditArtifact(artifact);
      return artifact;
    },
    onSuccess: (result) => {
      const checksum = 'sha256' in result ? `；${String(result.sha256)}` : 'root_sha256' in result ? `；${String(result.root_sha256)}` : '';
      if ('status' in result && 'records_checked' in result) {
        const integrity = result as { status: string; records_checked: number; matched?: number; baselined?: number; mismatched?: number; added?: number; missing?: number };
        const statusText = integrity.status === 'passed' ? '已与历史基线比对通过' : integrity.status === 'baseline_created' ? '首次建立逐条防篡改基线，尚未声称历史比对通过' : integrity.status === 'no_records' ? '时间窗内无审计记录' : '检出与防篡改基线不一致';
        setActionResult(`完整性检查：${statusText}；检查 ${integrity.records_checked} 条，匹配 ${integrity.matched ?? 0} 条，新建基线 ${integrity.baselined ?? 0} 条，新增 ${integrity.added ?? 0} 条，缺失 ${integrity.missing ?? 0} 条，不匹配 ${integrity.mismatched ?? 0} 条${checksum}`);
      } else if ('row_count' in result) {
        const artifact = result as { row_count: number; total_matching?: number; truncated?: boolean };
        setActionResult(`真实操作已完成并写入 PostgreSQL 审计记录；导出 ${artifact.row_count}/${artifact.total_matching ?? artifact.row_count} 条；${artifact.truncated ? '已截断（不允许作为完整证据）' : '完整未截断'}${checksum}`);
      } else {
        setActionResult(`真实操作已完成并写入 PostgreSQL 审计记录${checksum}`);
      }
      void queryClient.invalidateQueries({ queryKey: ['audit-governance'] });
    },
    onError: (mutationError) => setActionResult(mutationError instanceof Error ? mutationError.message : '操作失败'),
  });
  function openAction(title: string, target = String(selectedRecord?.log_id ?? '')) {
    setActionResult(undefined);
    const next = createAuditAction(title, target);
    if (next.kind === 'detail' || next.kind === 'review' || next.kind === 'export' || next.kind === 'evidence') next.auditLogID = target || selectedRecord?.log_id;
    if (next.kind === 'detail' && next.auditLogID) setSelectedKey(next.auditLogID);
    if (next.kind === 'export' || next.kind === 'evidence') setExportConfirmed(false);
    setAction(next);
  }
  function revealDetailTab(tab: string, target = String(selectedRecord?.log_id ?? '')) {
    if (target && target !== selectedKey) {
      setSelectedKey(target);
      setActionResult(undefined);
      actionMutation.reset();
    }
    setDetailTab(tab);
    setActionResult(undefined);
    if (tab !== '复核操作') setAction(undefined);
    window.requestAnimationFrame(() => detailPanelRef.current?.scrollIntoView({ block: 'nearest', behavior: 'smooth' }));
  }
  function revealReview(target = String(selectedRecord?.log_id ?? '')) {
    const next = createAuditAction('触发复核', target);
    next.auditLogID = target || selectedRecord?.log_id;
    setAction(next);
    setActionResult(undefined);
    revealDetailTab('复核操作', target);
  }
  function handleDetailAction(title: string, target?: string) {
    const next = createAuditAction(title, target || String(selectedRecord?.log_id ?? ''));
    if (next.kind === 'detail') {
      revealDetailTab(title.includes('关联对象') ? '关联链路' : '操作详情', target);
      return;
    }
    openAction(title, target);
  }
  function resetSearch() {
    setUserFilter('全部用户/角色');
    setTenantFilter('全部租户');
    setObjectFilter('全部');
    setActionFilter('全部');
    setResultFilter('全部');
    setRequestQuery('');
    setTraceQuery('');
    setTimeRange(null);
    setListPage(1);
  }
  function closeAction() {
    if (actionMutation.isPending) return;
    setAction(undefined);
    setActionResult(undefined);
    actionMutation.reset();
  }
  return (
    <div className="taf-page taf-auditlog">
      <section className="taf-auditlog-shell">
        <main className="taf-auditlog-main">
          <header className="taf-auditlog-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              {sourceObjectId && <span className="taf-source-context" data-source-object={sourceObjectId} data-source-object-type={sourceObjectType}>关联对象：{objectFilter !== '全部' ? `${objectFilter} / ` : ''}{sourceObjectId}</span>}
            </div>
            <Space size={8}>
              <Tooltip title={canWrite ? '' : '需要 audit:write 权限'}><Button size="small" disabled={!canWrite} icon={<FileSearchOutlined />} onClick={() => openAction('保存查询')}>保存查询</Button></Tooltip>
              <Tooltip title={!canExport ? '需要 audit:export 权限' : !canActOnRecord ? '请等待并选择一条审计记录' : ''}><Button size="small" disabled={!canExport || !canActOnRecord} icon={<DownloadOutlined />} onClick={() => openAction('导出取证')}>导出取证</Button></Tooltip>
              <Tooltip title={!canExport ? '需要 audit:export 权限' : !canActOnRecord ? '请等待并选择一条审计记录' : ''}><Button size="small" disabled={!canExport || !canActOnRecord} icon={<FileProtectOutlined />} onClick={() => openAction('生成合规证据')}>生成合规证据</Button></Tooltip>
              <Tooltip title={!canWrite ? '需要 audit:write 权限' : !canActOnRecord ? '请等待并选择一条审计记录' : ''}><Button size="small" disabled={!canWrite || !canActOnRecord || actionMutation.isPending} type="primary" danger icon={<WarningOutlined />} onClick={() => revealReview()}>触发复核</Button></Tooltip>
              <Tooltip title={canWrite ? '' : '需要 audit:write 权限'}><Button size="small" disabled={!canWrite} type="primary" ghost icon={<SafetyCertificateOutlined />} onClick={() => openAction('归档校验')}>归档校验</Button></Tooltip>
              <Tooltip title="刷新审计日志">
                <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
              </Tooltip>
              <Button size="small" icon={<HistoryOutlined />} disabled={!canActOnRecord} onClick={() => revealDetailTab('操作详情')}>操作详情</Button>
            </Space>
          </header>

          {isError && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={error instanceof Error ? error.message : '请检查 /v1/audit/logs、APISIX 路由或 alert-service audit_logs 查询。'}
              action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
            />
          )}

          <div className="taf-auditlog-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-auditlog-workbench">
            <WorkPanel title="日志检索" className="taf-auditlog-filter-panel" extra={<SearchOutlined />}>
              <AuditSearchBar userFilter={userFilter} tenantFilter={tenantFilter} objectFilter={objectFilter} actionFilter={actionFilter} resultFilter={resultFilter} requestQuery={requestQuery} traceQuery={traceQuery} timeRange={timeRange} userOptions={userOptions} tenantOptions={tenantOptions} actionOptions={actionOptions} onUserChange={(value) => { setUserFilter(value); setListPage(1); }} onTenantChange={(value) => { setTenantFilter(value); setListPage(1); }} onObjectChange={(value) => { setObjectFilter(value); setListPage(1); }} onActionChange={(value) => { setActionFilter(value); setListPage(1); }} onResultChange={(value) => { setResultFilter(value); setListPage(1); }} onRequestChange={(value) => { setRequestQuery(value); setListPage(1); }} onTraceChange={(value) => { setTraceQuery(value); setListPage(1); }} onTimeRangeChange={(value) => { setTimeRange(value); setListPage(1); }} onReset={resetSearch} onSearch={() => void refetch()} />
            </WorkPanel>

            <WorkPanel title={`审计日志（共 ${auditData?.total ?? visibleRows.length} 条）`} className="taf-auditlog-table-panel" extra={<AuditOutlined />}>
              <Table
                rowKey={rowKey}
                size="small"
                loading={isLoading}
                pagination={false}
                scroll={{ x: 980, y: 220 }}
                columns={columns}
                dataSource={visibleRows}
                rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], getCheckboxProps: () => ({ disabled: actionMutation.isPending }), onChange: (keys) => !actionMutation.isPending && revealDetailTab(detailTab, String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => { if (!actionMutation.isPending) revealDetailTab(detailTab, rowKey(record)); }, onDoubleClick: () => { if (!actionMutation.isPending) revealDetailTab('操作详情', String(record.记录ID)); } })}
              />
              <div className="taf-auditlog-pagination">
                <span>共 {auditData?.total ?? visibleRows.length} 条</span>
                <button type="button" aria-label="审计日志上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>
                {auditPaginationItems(listPage, pageCount).map((item) => typeof item === 'number'
                  ? <button key={item} type="button" className={item === listPage ? 'is-active' : ''} aria-label={`审计日志第 ${item} 页`} onClick={() => setListPage(item)}>{item}</button>
                  : <span key={item} className="taf-auditlog-pagination-ellipsis" aria-hidden="true">…</span>)}
                <button type="button" aria-label="审计日志下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button>
                <span>{pageSize} 条/页</span>
              </div>
            </WorkPanel>

            <div ref={detailPanelRef} className="taf-auditlog-detail-panel-host">
              <WorkPanel title="操作详情 / Diff 视图" className="taf-auditlog-detail-panel" extra={<HistoryOutlined />}>
                <AuditDetail
                  selected={selected}
                  record={selectedRecord}
                  detailRecord={detailQuery.data}
                  detailLoading={detailQuery.isLoading}
                  detailError={detailQuery.error}
                  records={records}
                  retention={auditData?.retention}
                  activeTab={detailTab}
                  reviewReason={reviewReason}
                  reviewPending={actionMutation.isPending}
                  reviewResult={detailTab === '复核操作' ? actionResult : undefined}
                  reviewError={detailTab === '复核操作' && actionMutation.isError}
                  canWrite={canWrite}
                  onReviewReasonChange={setReviewReason}
                  onReview={() => {
                    const target = selectedRecord?.log_id;
                    if (!target) return;
                    const next = createAuditAction('触发复核', target);
                    next.auditLogID = target;
                    setAction(next);
                    setActionResult(undefined);
                    actionMutation.mutate(next);
                  }}
                  onTabChange={(tab) => tab === '复核操作' ? revealReview() : revealDetailTab(tab)}
                  onAction={handleDetailAction}
                />
              </WorkPanel>
            </div>

            <div className="taf-auditlog-bottom">
              <WorkPanel title="关联链路（从当前操作追溯业务链路）" extra={<LinkOutlined />}>
                <RelatedChain selected={selected} record={selectedRecord} />
              </WorkPanel>
              <WorkPanel title="操作时间线" extra={<HistoryOutlined />}>
                <OperationTimeline records={records} onAction={openAction} />
              </WorkPanel>
              <WorkPanel title="导出取证" extra={<ExportOutlined />}>
                <ExportEvidence format={exportFormat} canExport={canExport && canActOnRecord} selectedLogID={selectedRecord?.log_id} timeRange={timeRange} objectFilter={objectFilter} userFilter={userFilter} userOptions={userOptions} onFormatChange={setExportFormat} onObjectChange={(value) => { setObjectFilter(value); setListPage(1); }} onUserChange={(value) => { setUserFilter(value); setListPage(1); }} onAction={openAction} />
              </WorkPanel>
            </div>
          </div>
        </main>
      </section>
      <Drawer rootClassName="taf-auditlog-action-drawer" title={action ? `${action.title}确认` : '审计操作确认'} open={Boolean(action && !['detail', 'review', 'export', 'evidence'].includes(action.kind))} width="min(900px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" closable={!actionMutation.isPending} keyboard={!actionMutation.isPending} maskClosable={!actionMutation.isPending} onClose={closeAction} extra={action ? <Button size="small" type="primary" loading={actionMutation.isPending} onClick={() => actionMutation.mutate(action)}>确认提交</Button> : undefined}>
        {action && <div className="taf-alert-detail-action-body"><p>该操作将使用当前租户、查询条件和所选审计记录执行真实治理动作，并原子写入 PostgreSQL 业务记录与审计留痕。</p><dl><dt>审计对象</dt><dd>{action.target || '当前查询'}</dd><dt>业务接口</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionResult && <Alert type={actionMutation.isError ? 'error' : 'success'} showIcon message={actionMutation.isError ? '真实业务操作失败' : '真实业务操作完成'} description={actionResult} />}</div>}
      </Drawer>
      <Modal rootClassName="taf-auditlog-export-modal" title={action?.kind === 'evidence' ? '生成合规审计证据' : '审计材料导出'} open={Boolean(action && (action.kind === 'export' || action.kind === 'evidence'))} width="min(960px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" closable={!actionMutation.isPending} keyboard={!actionMutation.isPending} maskClosable={!actionMutation.isPending} onCancel={closeAction} okText="生成并下载" cancelText="取消" cancelButtonProps={{ disabled: actionMutation.isPending }} confirmLoading={actionMutation.isPending} okButtonProps={{ disabled: !exportConfirmed || !canExport || exportOverLimit || (exportScope === 'selected' && !action?.auditLogID) }} onOk={() => action && actionMutation.mutate(action)}>
        {action && <AuditExportModalBody action={action} total={auditData?.total ?? 0} selected={exportSelectedRecord} format={exportFormat} scope={exportScope} maskSensitive={maskSensitive} confirmed={exportConfirmed} result={actionResult} error={actionMutation.isError} onFormatChange={setExportFormat} onScopeChange={(value) => { setExportScope(value); setExportConfirmed(false); }} onMaskChange={setMaskSensitive} onConfirmChange={setExportConfirmed} />}
      </Modal>
    </div>
  );
}

function AuditSearchBar({
  userFilter,
  tenantFilter,
  objectFilter,
  actionFilter,
  resultFilter,
  requestQuery,
  traceQuery,
  timeRange,
  userOptions,
  tenantOptions,
  actionOptions,
  onUserChange,
  onTenantChange,
  onObjectChange,
  onActionChange,
  onResultChange,
  onRequestChange,
  onTraceChange,
  onTimeRangeChange,
  onReset,
  onSearch,
}: {
  userFilter: string;
  tenantFilter: string;
  objectFilter: string;
  actionFilter: string;
  resultFilter: string;
  requestQuery: string;
  traceQuery: string;
  timeRange: [Dayjs | null, Dayjs | null] | null;
  userOptions: { value: string; label: string }[];
  tenantOptions: { value: string; label: string }[];
  actionOptions: { value: string; label: string }[];
  onUserChange: (value: string) => void;
  onTenantChange: (value: string) => void;
  onObjectChange: (value: string) => void;
  onActionChange: (value: string) => void;
  onResultChange: (value: string) => void;
  onRequestChange: (value: string) => void;
  onTraceChange: (value: string) => void;
  onTimeRangeChange: (value: [Dayjs | null, Dayjs | null] | null) => void;
  onReset: () => void;
  onSearch: () => void;
}) {
  return (
    <div className="taf-auditlog-search">
      <label><span>用户/角色</span><Select size="small" value={userFilter} options={userOptions} onChange={onUserChange} /></label>
      <label><span>租户</span><Select size="small" value={tenantFilter} options={tenantOptions} onChange={onTenantChange} /></label>
      <label className="is-wide"><span>时间</span><RangePicker size="small" showTime value={timeRange} onChange={(value) => onTimeRangeChange(value ? [value[0], value[1]] : null)} /></label>
      <label><span>对象类型</span><Select size="small" value={objectFilter} options={auditObjectTypeOptions} onChange={onObjectChange} /></label>
      <label><span>动作类型</span><Select size="small" value={actionFilter} options={actionOptions} onChange={onActionChange} /></label>
      <label><span>结果</span><Select size="small" value={resultFilter} options={[{ value: '全部' }, { value: '成功' }, { value: '失败' }, { value: '待复核' }]} onChange={onResultChange} /></label>
      <label><span>请求 ID</span><Input size="small" value={requestQuery} onChange={(event) => onRequestChange(event.target.value)} placeholder="请输入请求 ID" /></label>
      <label><span>trace_id</span><Input size="small" value={traceQuery} onChange={(event) => onTraceChange(event.target.value)} placeholder="请输入 trace_id" /></label>
      <div className="taf-auditlog-search-actions">
        <Button size="small" onClick={onReset}>重置</Button>
        <Button size="small" type="primary" icon={<SearchOutlined />} onClick={onSearch}>查询</Button>
        <Button size="small" type="text" onClick={onSearch}>收起</Button>
      </div>
    </div>
  );
}

function AuditDetail({ selected, record, detailRecord, detailLoading, detailError, records, retention, activeTab, reviewReason, reviewPending, reviewResult, reviewError, canWrite, onReviewReasonChange, onReview, onTabChange, onAction }: {
  selected?: SnapshotRow;
  record?: AuditLogRecord;
  detailRecord?: AuditLogRecord;
  detailLoading: boolean;
  detailError?: Error | null;
  records: AuditLogRecord[];
  retention?: { retention_days: number; archive_location: string; integrity_rate: number; masked_rate: number; last_checked_at?: number };
  activeTab: string;
  reviewReason: string;
  reviewPending: boolean;
  reviewResult?: string;
  reviewError: boolean;
  canWrite: boolean;
  onReviewReasonChange: (value: string) => void;
  onReview: () => void;
  onTabChange: (value: string) => void;
  onAction: (title: string, target?: string) => void;
}) {
  const resource = String(selected?.对象类型 ?? '未选择');
  const action = String(selected?.动作类型 ?? '未选择');
  const result = String(selected?.结果 ?? '未选择');
  const rows = auditDiffRows(record);
  const beforeVersion = detailText(record?.details ?? {}, ['before_version', 'source_version'], '原值');
  const afterVersion = detailText(record?.details ?? {}, ['after_version', 'target_version'], '新值');

  return (
    <div className="taf-auditlog-detail">
      <div className="taf-auditlog-detail-meta">
        <span>对象类型：<b>{resource}</b></span>
        <span>动作类型：<b>{action}</b></span>
        <span>结果：<b className={resultClass(result)}>{result}</b></span>
        <span>对象 ID：<b>{record?.resource_id || record?.log_id || '-'}</b></span>
        <span>时间：<b>{String(selected?.时间 ?? '未记录')}</b></span>
      </div>

      <div className="taf-auditlog-detail-tabs">
        {['字段变更对比', '操作上下文', '关联链路', '操作详情', '复核操作'].map((tab) => <button key={tab} type="button" className={tab === activeTab ? 'is-active' : ''} disabled={tab === '复核操作' && !canWrite} title={tab === '复核操作' && !canWrite ? '需要 audit:write 权限' : undefined} onClick={() => onTabChange(tab)}>{tab}</button>)}
      </div>

      {activeTab === '字段变更对比' && (
        <>
          <div className="taf-auditlog-diff">
            <div><span>字段</span><span>操作前（{beforeVersion}）</span><span>操作后（{afterVersion}）</span></div>
            {rows.map(([field, before, after]) => (
              <button key={field} type="button" onClick={() => onAction(`查看字段变更：${field}`, record?.log_id)}>
                <span>{field}</span>
                <span>{before}</span>
                <span className={field === '动作' || field === '优先级' ? 'is-risk' : field === '目的端口' || field === '备注' ? 'is-ok' : ''}>{after}</span>
              </button>
            ))}
          </div>
          <div className="taf-auditlog-sidecards">
            <HighRiskAudit records={records} onAction={onAction} />
            <RetentionStatus retention={retention} onAction={onAction} />
          </div>
        </>
      )}
      {activeTab === '操作上下文' && <AuditOperationContext selected={selected} record={record} result={result} />}
      {activeTab === '关联链路' && <AuditRelatedChainDetail selected={selected} record={record} onAction={onAction} />}
      {activeTab === '操作详情' && <AuditOperationDrawer record={detailRecord} loading={detailLoading} error={detailError} />}
      {activeTab === '复核操作' && (
        <div className="taf-auditlog-inline-review" data-audit-detail-state="review">
          <Alert type="warning" showIcon message="高风险操作二次复核" description="复核对象与当前选中行严格绑定；确认后写入 PostgreSQL 审计复核记录，并刷新本页状态。" />
          <dl><dt>审计记录</dt><dd>{record?.log_id || '-'}</dd><dt>对象 / 动作</dt><dd>{resource} / {action}</dd><dt>风险等级</dt><dd>{record ? auditRiskLabel(record) : '-'}</dd><dt>业务接口</dt><dd>/v1/audit/reviews</dd></dl>
          <label><span>复核原因</span><Input.TextArea value={reviewReason} maxLength={500} autoSize={{ minRows: 3, maxRows: 5 }} onChange={(event) => onReviewReasonChange(event.target.value)} /></label>
          <div className="taf-auditlog-inline-review__actions"><Button type="primary" danger disabled={!canWrite || !record?.log_id || !reviewReason.trim()} loading={reviewPending} onClick={onReview}>确认提交复核</Button></div>
          {reviewResult && <Alert type={reviewError ? 'error' : 'success'} showIcon message={reviewError ? '真实业务操作失败' : '真实业务操作完成'} description={reviewResult} />}
        </div>
      )}
    </div>
  );
}

function AuditOperationContext({ selected, record, result }: { selected?: SnapshotRow; record?: AuditLogRecord; result: string }) {
  const details = record?.details ?? {};
  const hasNetworkEvidence = Boolean(record?.ip_address && record.ip_address !== '-' && record?.user_agent && record.user_agent !== '-');
  const validationText = result.includes('失败')
    ? '权限门禁拒绝或对象版本冲突'
    : hasNetworkEvidence
      ? detailText(details, ['context_validation', 'network_validation'], '身份与网络上下文已记录，需结合策略结果复核')
      : '来源 IP 或 User-Agent 未记录，无法判定网络可信性';
  return (
    <div className="taf-auditlog-operation-context" data-audit-detail-state="operation-context">
      <div className="taf-auditlog-context">
        <span>用户：<b>{String(selected?.['用户/角色'] ?? record?.user_id ?? '未记录')}</b></span>
        <span>租户：<b>{record?.tenant_id || String(selected?.租户 ?? '-')}</b></span>
        <span>角色：<b>{detailText(details, ['role', 'actor_role'], '未记录')}</b></span>
        <span>来源 IP：<b>{record?.ip_address || '-'}</b></span>
        <span>User-Agent：<b>{record?.user_agent || '-'}</b></span>
        <span>请求 ID：<b>{String(selected?.请求ID ?? record?.request_id ?? '未记录')}</b></span>
        <span>trace_id：<b>{String(selected?.trace_id ?? record?.trace_id ?? '未记录')}</b></span>
        <span>会话 ID：<b>{detailText(details, ['session_id'], '-')}</b></span>
      </div>
      <div className="taf-auditlog-request-chain" aria-label="审计请求链路">
        {auditRequestChain(record).map((step) => <span key={step}>{step}</span>)}
      </div>
      <div className="taf-auditlog-context-summary">
        <span>来源页面：<b>{detailText(details, ['source_page', 'route'], `${record?.resource_type || '系统'} / ${record?.action || '审计事件'}`)}</b></span>
        <span>上下文校验：<b className={result.includes('失败') ? 'is-risk' : hasNetworkEvidence ? 'is-info' : 'is-warn'}>{validationText}</b></span>
      </div>
    </div>
  );
}

function AuditRelatedChainDetail({ selected, record, onAction }: { selected?: SnapshotRow; record?: AuditLogRecord; onAction: (title: string, target?: string) => void }) {
  const relations = auditRelations(record);
  return (
    <div className="taf-auditlog-related-detail" data-audit-detail-state="related-chain">
      <RelatedChain selected={selected} record={record} />
      <div className="taf-auditlog-related-table">
        <div><span>关联对象</span><span>关系</span><span>状态</span><span>时间</span><span>跳转</span></div>
        {relations.map(([object, relation, status, time, action]) => (
          <button key={object} type="button" onClick={() => onAction(`查看关联对象：${object}`, record?.log_id)}>
            <span>{object}</span><span>{relation}</span><StatusTag value={status} /><span>{time}</span><span>{action}</span>
          </button>
        ))}
      </div>
      <Alert type="info" showIcon message="审计提示" description={`当前审计事件关联 ${relations.length} 个持久化业务对象；所有跳转目标均来自事件 detail，而非页面常量。`} />
    </div>
  );
}

function HighRiskAudit({ records, onAction }: { records: AuditLogRecord[]; onAction: (title: string, target?: string) => void }) {
  const rows = records.filter((record) => ['high', 'critical'].includes(String(record.risk ?? record.details.risk ?? '').toLowerCase())).slice(0, 5).map((record) => [formatAuditTime(record.timestamp, true), `${auditResourceLabel(record.resource_type)} / ${auditActionLabel(record.action)}`, auditRiskLabel(record), detailText(record.details, ['review_status'], '待复核'), record.log_id]);
  return (
    <div className="taf-auditlog-risk">
      <h3>当前页高风险审计</h3>
      <div><span>时间</span><span>对象/动作</span><span>风险等级</span><span>复核状态</span></div>
      {rows.map((row) => (
        <button key={row[4]} type="button" onClick={() => onAction('查看高风险审计', row[4])}>
          <span>{row[0]}</span>
          <span>{row[1]}</span>
          <span className="is-risk">{row[2]}</span>
          <span className={row[3] === '已复核' ? 'is-ok' : 'is-warn'}>{row[3]}</span>
        </button>
      ))}
    </div>
  );
}

function RetentionStatus({ retention, onAction }: { retention?: { retention_days: number; archive_location: string; integrity_rate: number; masked_rate: number; last_checked_at?: number }; onAction: (title: string, target?: string) => void }) {
  const currentValues = [retention?.retention_days ?? 0, retention?.archive_location ? 100 : 0, retention?.integrity_rate ?? 0, retention?.masked_rate ?? 0];
  const rows = [
    ['日志保留周期', `已配置 ${retention?.retention_days ?? 90} 天`],
    ['归档位置', retention?.archive_location || '未配置'],
    ['完整性校验', `${(retention?.integrity_rate ?? 0).toFixed(2)}%`],
    ['脱敏状态', `已脱敏 ${(retention?.masked_rate ?? 0).toFixed(0)}%`],
  ];
  return (
    <div className="taf-auditlog-retention">
      <h3>留存状态</h3>
      {rows.map(([label, value], index) => (
        <span key={label}>
          <em>{label}</em>
          <b>{value}</b>
          <div className="taf-auditlog-retention-echart"><DataQualityKpiSparklineChart ariaLabel={`审计${label}当前值指示`} tone={index === 0 ? 'warn' : index === 1 || index === 3 ? 'ok' : retention?.integrity_rate === 100 ? 'ok' : 'risk'} values={[currentValues[index], currentValues[index]]} /></div>
        </span>
      ))}
      <footer>最后校验：{retention?.last_checked_at ? formatAuditTime(retention.last_checked_at) : '尚未执行'} <button type="button" onClick={() => onAction('归档校验')}>校验</button></footer>
    </div>
  );
}

function RelatedChain({ selected, record }: { selected?: SnapshotRow; record?: AuditLogRecord }) {
  const nodes = auditChainNodes(record, selected);
  return (
    <div className="taf-auditlog-chain">
      {nodes.map(([type, id, time, tone]) => (
        <span key={id} className={`is-${tone}`}>
          <i>{chainIcon(type)}</i>
          <b>{type}</b>
          <em>{id}</em>
          <small>{time}</small>
        </span>
      ))}
    </div>
  );
}

function OperationTimeline({ records, onAction }: { records: AuditLogRecord[]; onAction: (title: string, target?: string) => void }) {
  // Four complete entries fit the fixed audit workbench row at the supported
  // 1920x1080 acceptance viewport; rendering more produced a clipped fifth
  // and sixth entry beneath the panel boundary.
  const entries = records.slice(0, 4);
  return (
    <div className="taf-auditlog-timeline">
      {entries.map((record) => (
        <button key={record.log_id} type="button" className={record.result === 'success' ? 'is-ok' : 'is-risk'} onClick={() => onAction('查看操作时间线', record.log_id)}>
          <i />
          <span>{formatAuditTime(record.timestamp, true)}</span>
          <b>{auditResourceLabel(record.resource_type)} / {auditActionLabel(record.action)}</b>
          <em>{record.log_id} · {record.result} · {record.request_id || '-'}</em>
        </button>
      ))}
    </div>
  );
}

function ExportEvidence({ format, canExport, selectedLogID, timeRange, objectFilter, userFilter, userOptions, onFormatChange, onObjectChange, onUserChange, onAction }: { format: string; canExport: boolean; selectedLogID?: string; timeRange: [Dayjs | null, Dayjs | null] | null; objectFilter: string; userFilter: string; userOptions: { value: string; label: string }[]; onFormatChange: (value: string) => void; onObjectChange: (value: string) => void; onUserChange: (value: string) => void; onAction: (title: string, target?: string) => void }) {
  const timeRangeLabel = timeRange?.[0] && timeRange?.[1]
    ? `${timeRange[0].format('YYYY-MM-DD HH:mm:ss')} ～ ${timeRange[1].format('YYYY-MM-DD HH:mm:ss')}`
    : '未设置（全部时间）';
  return (
    <div className="taf-auditlog-export">
      <label><span>时间范围</span><Input size="small" value={timeRangeLabel} readOnly /></label>
      <label><span>对象类型</span><Select size="small" value={objectFilter} options={auditObjectTypeOptions} onChange={onObjectChange} /></label>
      <label><span>用户/角色</span><Select size="small" value={userFilter} options={userOptions} onChange={onUserChange} /></label>
      <div className="taf-auditlog-export-format">
        {['PDF', 'CSV', 'JSON'].map((item) => <button key={item} type="button" className={item === format ? 'is-active' : ''} onClick={() => onFormatChange(item)}>{item}</button>)}
      </div>
      <Button size="small" type="primary" block disabled={!canExport || !selectedLogID} icon={<DownloadOutlined />} onClick={() => onAction('导出审计材料', selectedLogID)}>导出审计材料</Button>
    </div>
  );
}

function AuditExportModalBody({ action, total, selected, format, scope, maskSensitive, confirmed, result, error, onFormatChange, onScopeChange, onMaskChange, onConfirmChange }: {
  action: AuditAction;
  total: number;
  selected?: AuditLogRecord;
  format: string;
  scope: 'selected' | 'query';
  maskSensitive: boolean;
  confirmed: boolean;
  result?: string;
  error: boolean;
  onFormatChange: (value: string) => void;
  onScopeChange: (value: 'selected' | 'query') => void;
  onMaskChange: (value: boolean) => void;
  onConfirmChange: (value: boolean) => void;
}) {
  const estimated = scope === 'selected' ? (selected ? 1 : 0) : total;
  const overLimit = scope === 'query' && total > 10_000;
  const fields = ['时间', '事件 ID', '主体', '动作', '对象', '结果', '风险', '请求 ID', 'Trace ID', '来源 IP', 'User-Agent', '详情'];
  return (
    <div className="taf-auditlog-export-dialog" data-overlay-contract="modal-audit-export">
      <div className="taf-auditlog-export-summary">
        <span><small>预计记录数</small><b>{estimated.toLocaleString('zh-CN')}</b></span>
        <span><small>风险范围</small><b className={overLimit ? 'is-risk' : 'is-warn'}>{overLimit ? '超过同步上限' : '租户内受控导出'}</b></span>
        <span><small>完整性签名</small><b className="is-ok">SHA-256</b></span>
        <span><small>审计留痕</small><b className="is-ok">事务内写入</b></span>
      </div>
      <div className="taf-auditlog-export-grid">
        <section>
          <h3>导出范围</h3>
          <label><span>范围</span><Select value={scope} options={[{ value: 'selected', label: '当前选中审计事件' }, { value: 'query', label: '当前查询结果' }]} onChange={onScopeChange} /></label>
          <label><span>筛选命中</span><Input value={scope === 'selected' ? selected?.log_id ?? '未选择记录' : `${total.toLocaleString('zh-CN')} 条`} readOnly /></label>
          <label><span>输出格式</span><div className="taf-auditlog-export-format">{['PDF', 'CSV', 'JSON'].map((item) => <button key={item} type="button" className={item === format ? 'is-active' : ''} onClick={() => onFormatChange(item)}>{item}</button>)}</div></label>
          <Checkbox checked={maskSensitive} onChange={(event) => onMaskChange(event.target.checked)}>脱敏来源 IP 与 User-Agent</Checkbox>
        </section>
        <section>
          <h3>证据字段（固定审计架构 12/12）</h3>
          <div className="taf-auditlog-export-fields">{fields.map((field) => <Tag key={field} color={field === '来源 IP' || field === 'User-Agent' ? 'gold' : 'blue'}>{field}</Tag>)}</div>
          <dl><dt>业务接口</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd><dt>导出策略</dt><dd>超过 10,000 条拒绝同步导出，不允许静默截断</dd></dl>
        </section>
      </div>
      {overLimit ? <Alert type="error" showIcon message="当前查询结果超过同步导出上限" description="请缩小时间范围或筛选条件后再导出；系统不会生成不完整证据。" /> : <Alert type="warning" showIcon message="高权限证据导出" description="文件将包含当前租户审计记录、SHA-256 和可追踪导出元数据，操作本身会写入审计日志。" />}
      <Checkbox checked={confirmed} disabled={overLimit} onChange={(event) => onConfirmChange(event.target.checked)}>我确认导出范围与脱敏策略，并承担本次审计材料使用责任</Checkbox>
      {result && <Alert type={error ? 'error' : 'success'} showIcon message={error ? '导出失败' : '导出完成'} description={result} />}
    </div>
  );
}

function AuditOperationDrawer({ record, loading, error }: { record?: AuditLogRecord; loading: boolean; error?: Error | null }) {
  if (loading) return <Alert type="info" showIcon message="正在加载真实审计详情" />;
  if (error) return <Alert type="error" showIcon message="审计详情加载失败" description={error.message} />;
  if (!record) return <Alert type="warning" showIcon message="未选择审计记录" />;
  const details = Object.entries(record.details ?? {}).slice(0, 24);
  const diffs = auditDiffRows(record);
  const relations = auditRelations(record);
  const contextRecorded = Boolean(record.ip_address || record.user_agent || record.request_id || record.trace_id);
  return (
    <div className="taf-auditlog-drawer-content" data-overlay-contract="drawer-audit-operation-detail">
      <div className="taf-auditlog-drawer-summary">
        <span><small>操作类型</small><b>{auditActionLabel(record.action)}</b></span>
        <span><small>操作人</small><b>{record.user_id || 'system'}</b></span>
        <span><small>目标资源</small><b>{record.resource_type} / {record.resource_id || '-'}</b></span>
        <span><small>结果</small><b className={record.result === 'success' ? 'is-ok' : 'is-risk'}>{record.result}</b></span>
        <span><small>Trace ID</small><b>{record.trace_id || '-'}</b></span>
      </div>
      <div className="taf-auditlog-drawer-grid">
        <section>
          <h3>字段变更 Diff</h3>
          <div className="taf-auditlog-diff">
            <div><span>字段</span><span>原值</span><span>新值</span></div>
            {diffs.map(([field, before, after]) => <div key={field}><span>{field}</span><span>{before}</span><span>{after}</span></div>)}
          </div>
          <Alert type={record.risk === 'high' || record.risk === 'critical' ? 'warning' : 'info'} showIcon message="变更与风险摘要" description={`风险级别 ${auditRiskLabel(record)}；${diffs.length} 个可展示字段；复核状态 ${detailText(record.details, ['review_status'], '未记录')}。`} />
        </section>
        <section>
          <h3>请求上下文</h3>
          <dl>
            <dt>租户</dt><dd>{record.tenant_id}</dd>
            <dt>请求 ID</dt><dd>{record.request_id || '-'}</dd>
            <dt>来源 IP</dt><dd>{record.ip_address || '未记录'}</dd>
            <dt>User-Agent</dt><dd>{record.user_agent || '未记录'}</dd>
            <dt>发生时间</dt><dd>{formatAuditTime(record.timestamp)}</dd>
          </dl>
          <Alert type={contextRecorded ? 'info' : 'warning'} showIcon message={contextRecorded ? '上下文已部分记录' : '上下文证据不足'} description="页面仅展示 audit_logs 已持久化字段，不推断可信网段或身份验证结果。" />
        </section>
        <section>
          <h3>关联链路与取证</h3>
          <div className="taf-auditlog-related-table">
            <div><span>关联对象</span><span>关系</span><span>状态</span><span>时间</span><span>动作</span></div>
            {relations.map(([object, relation, status, time, action]) => <div key={object}><span>{object}</span><span>{relation}</span><StatusTag value={status} /><span>{time}</span><span>{action}</span></div>)}
          </div>
          <dl><dt>事件 ID</dt><dd>{record.log_id}</dd><dt>完整性来源</dt><dd>audit_logs + 持久化基线</dd><dt>业务详情数</dt><dd>{details.length}</dd></dl>
        </section>
      </div>
      <div className="taf-auditlog-drawer-details">
        <h3>事件业务详情</h3>
        <div className="taf-auditlog-diff"><div><span>字段</span><span>真实值</span><span>来源</span></div>{details.map(([key, value]) => <div key={key}><span>{key}</span><span>{typeof value === 'string' ? value : JSON.stringify(value)}</span><span>audit_logs.detail</span></div>)}</div>
      </div>
      <Alert type="warning" showIcon message="审计不可篡改保护" description="当前记录参与逐条摘要与固定窗口清单复验；最终可信度仍取决于数据库权限隔离和外部归档策略。" />
    </div>
  );
}

const renderAuditCell = (column: string, value: unknown) => {
  if (column === '结果' || column === '风险标签') return <StatusTag value={value} />;
  if (column === '用户/角色') return <span className="taf-auditlog-user"><AuditOutlined />{String(value)}</span>;
  if (column === '请求ID' || column === 'trace_id') return <span className="taf-auditlog-code">{String(value)}</span>;
  return String(value);
};

const rowKey = (row: SnapshotRow) => String(row.记录ID ?? row['请求ID'] ?? row.trace_id ?? JSON.stringify(row));

const auditObjectTypeOptions = ['全部', 'PCAP', '规则', '模型', '令牌', '融合规则', '融合冲突'].map((value) => ({ value }));
const auditObjectTypeValue = (value: string) => ({ PCAP: 'pcap', 规则: 'rule', 模型: 'model', 令牌: 'token', 融合规则: 'fusion_rule', 融合冲突: 'fusion_conflict' }[value] ?? value);
const auditObjectTypeLabel = (value: string) => ({ pcap: 'PCAP', rule: '规则', model: '模型', token: '令牌', fusion_rule: '融合规则', fusion_conflict: '融合冲突' }[value] ?? (value || '全部'));
const auditResultValue = (value: string) => ({ 成功: 'success', 失败: 'failure', 待复核: 'pending_review' }[value] ?? value);

const uniqueOptions = (values: string[], labelFor: (value: string) => string = (value) => value) =>
  [...new Set(values)].map((value) => ({ value, label: labelFor(value) }));

const auditPaginationItems = (current: number, total: number): Array<number | string> => {
  if (total <= 7) return Array.from({ length: total }, (_, index) => index + 1);
  const pages = new Set([1, total, current - 1, current, current + 1]);
  if (current <= 3) pages.add(2);
  if (current >= total - 2) pages.add(total - 1);
  const ordered = [...pages].filter((page) => page >= 1 && page <= total).sort((left, right) => left - right);
  const items: Array<number | string> = [];
  ordered.forEach((page, index) => {
    if (index > 0 && page - ordered[index - 1] > 1) items.push(`ellipsis-${ordered[index - 1]}-${page}`);
    items.push(page);
  });
  return items;
};

const auditResourceLabel = (value: string) => {
  const normalized = value.toLowerCase();
  if (normalized.includes('pcap') || normalized.includes('evidence')) return 'PCAP';
  if (normalized.includes('rule')) return '规则';
  if (normalized.includes('model')) return '模型';
  if (normalized.includes('deploy')) return '部署';
  if (normalized.includes('token')) return '令牌';
  if (normalized.includes('whitelist')) return '白名单';
  if (normalized.includes('compliance')) return '合规报告';
  return value || '系统对象';
};

const auditActionLabel = (value: string) => {
  const normalized = value.toUpperCase();
  if (normalized.includes('EXPORT') || normalized.includes('DOWNLOAD')) return '导出';
  if (normalized.includes('ROLLBACK')) return '回滚';
  if (normalized.includes('ACTIVAT')) return '激活';
  if (normalized.includes('PUBLISH') || normalized.includes('DEPLOY')) return '发布';
  if (normalized.includes('UPDATE') || normalized.includes('CHANGE')) return '变更';
  if (normalized.includes('CREATE') || normalized.includes('GENERAT')) return '生成';
  if (normalized.includes('ACCESS') || normalized.includes('VIEW') || normalized.includes('READ')) return '访问';
  if (normalized.includes('FAIL') || normalized.includes('REJECT')) return '拒绝';
  return value || '操作';
};

const auditRiskLabel = (record: AuditLogRecord) => {
  const risk = String(record.risk ?? record.details.risk ?? record.details.risk_level ?? '').toLowerCase();
  if (risk === 'critical') return '严重';
  if (risk === 'high') return '高风险';
  if (risk === 'medium') return '中风险';
  return '低风险';
};

const formatAuditTime = (timestamp: number, timeOnly = false) => {
  if (!timestamp) return '-';
  const date = new Date(timestamp);
  if (Number.isNaN(date.valueOf())) return '-';
  return timeOnly ? date.toLocaleTimeString('zh-CN', { hour12: false }) : date.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-');
};

const detailText = (details: Record<string, unknown>, keys: string[], fallback = '-') => {
  for (const key of keys) {
    const value = details[key];
    if (typeof value === 'string' && value.trim()) return value;
    if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  }
  return fallback;
};

const auditRecordToRow = (record: AuditLogRecord): SnapshotRow => ({
  记录ID: record.log_id,
  时间: formatAuditTime(record.timestamp),
  '用户/角色': detailText(record.details, ['username', 'actor_name'], record.user_id || 'system'),
  租户: record.tenant_id,
  对象类型: auditResourceLabel(record.resource_type),
  动作类型: auditActionLabel(record.action),
  结果: record.result === 'success' ? '成功' : record.result === 'pending_review' ? '待复核' : '失败',
  请求ID: record.request_id || detailText(record.details, ['request_id'], record.log_id),
  trace_id: record.trace_id || detailText(record.details, ['trace_id'], '-'),
  风险标签: auditRiskLabel(record),
  操作: '详情 / 关联 / 复核',
});

const buildAuditSnapshot = (route: NavRoute, data?: { trails: AuditLogRecord[]; total: number; summary?: { today?: number; failed: number; high_risk: number; exports: number; pcap_access: number; integrity_rate: number } }): PageSnapshot => {
  const records = data?.trails ?? [];
  const summary = data?.summary ?? {
    failed: records.filter((record) => record.result !== 'success').length,
    high_risk: records.filter((record) => ['高风险', '严重'].includes(auditRiskLabel(record))).length,
    exports: records.filter((record) => auditActionLabel(record.action) === '导出').length,
    pcap_access: records.filter((record) => auditResourceLabel(record.resource_type) === 'PCAP').length,
    integrity_rate: records.length ? 100 : 0,
  };
  const values: Record<string, string> = {
    今日操作: `${summary.today ?? records.filter((record) => new Date(record.timestamp).toDateString() === new Date().toDateString()).length} 条`,
    失败操作: `${summary.failed} 条`,
    高风险操作: `${summary.high_risk} 条`,
    导出下载: `${summary.exports} 次`,
    'PCAP 访问': `${summary.pcap_access} 次`,
    完整性校验通过率: `${summary.integrity_rate.toFixed(2)}%`,
  };
  return {
    id: route.id,
    total: data?.total ?? records.length,
    metrics: route.page.kpis.map((label) => ({
      label,
      value: values[label] ?? '0',
      delta: '实时 PostgreSQL',
      status: label.includes('失败') && summary.failed
        ? 'risk'
        : label.includes('高风险') && summary.high_risk
          ? 'warn'
          : label.includes('率')
            ? summary.integrity_rate >= 99 ? 'ok' : summary.integrity_rate > 0 ? 'warn' : 'risk'
            : 'info',
    })),
    rows: records.map(auditRecordToRow),
    timeline: records.slice(0, 6).map((record) => ({ title: `${auditResourceLabel(record.resource_type)} / ${auditActionLabel(record.action)}`, description: `${record.log_id} · ${record.result} · ${record.request_id || detailText(record.details, ['request_id'], '-')}`, status: record.result === 'success' ? 'ok' : 'risk' })),
    evidence: [
      { label: 'Audit Logs API', value: `${records.length}/${data?.total ?? records.length}`, status: records.length ? 'ok' : 'warn' },
      { label: '租户隔离', value: records.every((record) => record.tenant_id === records[0]?.tenant_id) ? '单租户数据集' : '待检查', status: records.length ? 'ok' : 'warn' },
      { label: '完整性', value: `${summary.integrity_rate.toFixed(2)}%`, status: summary.integrity_rate >= 99 ? 'ok' : 'warn' },
    ],
  };
};

const auditDiffRows = (record?: AuditLogRecord): string[][] => {
  const before = record?.details.before;
  const after = record?.details.after;
  if (!before || !after || typeof before !== 'object' || typeof after !== 'object' || Array.isArray(before) || Array.isArray(after)) {
    const entries = Object.entries(record?.details ?? {}).filter(([key]) => !['relations', 'request_chain'].includes(key)).slice(0, 6);
    if (entries.length) return entries.map(([key, value]) => [key, '-', typeof value === 'string' ? value : JSON.stringify(value)]);
    return [['事件', '-', record ? `${record.resource_type}/${record.action}` : '暂无可比较记录']];
  }
  const beforeRecord = before as Record<string, unknown>;
  const afterRecord = after as Record<string, unknown>;
  const keys = [...new Set([...Object.keys(beforeRecord), ...Object.keys(afterRecord)])].slice(0, 6);
  return keys.map((key) => [key, String(beforeRecord[key] ?? '-'), String(afterRecord[key] ?? '-')]);
};

const auditRelations = (record?: AuditLogRecord): string[][] => {
  const raw = record?.details.relations;
  if (Array.isArray(raw)) {
    return raw.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item)).slice(0, 6).map((item) => [String(item.object_id ?? item.id ?? '-'), String(item.relation ?? '关联'), String(item.status ?? '已关联'), String(item.time ?? formatAuditTime(record?.timestamp ?? 0, true)), String(item.action ?? '查看详情')]);
  }
  if (!record) return [];
  return [[record.resource_id || record.log_id, '审计主体', record.result === 'success' ? '已记录' : '需复核', formatAuditTime(record.timestamp, true), '查看详情']];
};

const auditChainNodes = (record?: AuditLogRecord, selected?: SnapshotRow): string[][] => {
  const relations = auditRelations(record);
  if (!relations.length) return [];
  return relations.map(([id, relation, status, time], index) => [index === 0 ? String(selected?.对象类型 ?? auditResourceLabel(record?.resource_type ?? '')) : relation, id, time, status.includes('复核') ? 'warn' : status.includes('失败') ? 'risk' : 'ok']);
};

const auditRequestChain = (record?: AuditLogRecord) => {
  const raw = record?.details.request_chain;
  if (Array.isArray(raw)) return raw.map(String).filter(Boolean).slice(0, 6);
  return ['请求链路未记录'];
};

const createAuditAction = (title: string, target: string): AuditAction => {
  if (title === '操作详情' || title.includes('高风险审计') || title.includes('操作时间线') || title.includes('字段变更') || title.includes('关联对象')) return { kind: 'detail', title, target, endpoint: '/v1/audit/logs/{id}', auditEvent: 'AUDIT_LOG_VIEWED' };
  if (title.includes('保存')) return { kind: 'save', title, target, endpoint: '/v1/audit/saved-queries', auditEvent: 'AUDIT_SAVED_QUERY_CREATED' };
  if (title.includes('复核')) return { kind: 'review', title, target, endpoint: '/v1/audit/reviews', auditEvent: 'AUDIT_REVIEW_TRIGGERED' };
  if (title.includes('校验') || title.includes('留存')) return { kind: 'integrity', title, target, endpoint: '/v1/audit/integrity-checks', auditEvent: 'AUDIT_INTEGRITY_CHECK_COMPLETED' };
  if (title.includes('合规证据')) return { kind: 'evidence', title, target, endpoint: '/v1/audit/exports', auditEvent: 'AUDIT_EVIDENCE_EXPORTED' };
  if (title.includes('导出')) return { kind: 'export', title, target, endpoint: '/v1/audit/exports', auditEvent: 'AUDIT_EVIDENCE_EXPORTED' };
  throw new Error(`不支持的审计操作：${title}`);
};

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({
  label,
  value: label.includes('率') ? '0.00%' : '0',
  delta: 'API',
  status: 'info',
});

const resultClass = (value: string) => {
  if (value.includes('失败')) return 'is-risk';
  if (value.includes('待')) return 'is-warn';
  return 'is-ok';
};

const chainIcon = (type: string) => {
  if (type === '告警') return <WarningOutlined />;
  if (type === 'PCAP') return <FileProtectOutlined />;
  if (type === '规则') return <AuditOutlined />;
  if (type === '模型') return <FileSearchOutlined />;
  if (type === '部署') return <SafetyCertificateOutlined />;
  return <CheckCircleOutlined />;
};

const readAuditPermissions = (token: string | null): string[] => {
  if (!token) return [];
  try {
    const base64 = token.split('.')[1]?.replace(/-/g, '+').replace(/_/g, '/') ?? '';
    const payload = JSON.parse(atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, '='))) as { permissions?: unknown };
    return Array.isArray(payload.permissions) ? payload.permissions.filter((item): item is string => typeof item === 'string') : [];
  } catch {
    return [];
  }
};

const hasAuditScope = (permissions: string[], required: string) => permissions.some((permission) => permission === '*' || permission === 'admin:*' || permission === 'audit:*' || permission === required);
