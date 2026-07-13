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
  SettingOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, DatePicker, Drawer, Input, Radio, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useMemo, useState } from 'react';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { RingChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { batchUpdateAlertStatus } from '@/services/alertBatchApi';
import { pageApiPlans } from '@/services/pageApiPlans';
import { alertAllowedNextStatuses, alertStatusLabel, alertStatusOptions, canTransitionAlertStatus, type AlertStatusCode } from '@/services/alertStatus';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type FeedbackResult = 'tp' | 'fp' | 'pending';
type AlertAction = { title: string; target: string; endpoint: string; auditEvent: string };

const alertOverlays: OverlayContract[] = [
  {
    id: 'modal-alert-batch',
    title: '告警批量操作确认',
    kind: 'Modal',
    actionLabel: '批量确认',
    description: '确认批量更新告警状态、责任人、标签或反馈结论；状态仅使用 new / triage / assigned / closed。',
    impact: '影响当前筛选范围内选中的告警闭环状态。',
    audit: '记录批量对象、筛选条件、操作者和变更前后状态。',
    danger: true,
  },
  {
    id: 'dropdown-alert-batch-actions',
    title: '告警批量操作下拉',
    kind: 'Dropdown/Menu',
    actionLabel: '批量操作',
    description: '承载批量认领、批量关闭、批量导出和批量回流样本入口。',
  },
  {
    id: 'dropdown-alert-row-actions',
    title: '告警行操作下拉',
    kind: 'Dropdown/Menu',
    actionLabel: '行操作',
    description: '承载单条告警查看证据、触发剧本、生成白名单和状态更新入口。',
  },
];

export function AlertTriagePage({ route }: { route: NavRoute }) {
  const [selectedRowKey, setSelectedRowKey] = useState<string>();
  const [feedbackResult, setFeedbackResult] = useState<FeedbackResult>('tp');
  const [batchTargetStatus, setBatchTargetStatus] = useState<AlertStatusCode>('triage');
  const [batchReason, setBatchReason] = useState('批量研判状态同步');
  const [listPage, setListPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [view, setView] = useState('自定义视图');
  const [filterNotice, setFilterNotice] = useState('当前队列');
  const [filters, setFilters] = useState({ asset: '全部资产', destination: '', rule: '全部规则', model: '全部模型', phase: '全部阶段', status: '全部状态', confidence: '全部' });
  const [action, setAction] = useState<AlertAction>();
  const [actionSubmitted, setActionSubmitted] = useState(false);
  const visualBreakdownMode = isVisualBreakdownMode();
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    refetchInterval: visualBreakdownMode ? false : 15_000,
  });

  const rows = useMemo(() => buildAlertRows(data?.rows ?? []), [data?.rows]);
  const selectedRow = useMemo(() => {
    if (!rows.length) return undefined;
    return rows.find((row) => rowKey(row) === selectedRowKey) ?? rows[0];
  }, [rows, selectedRowKey]);
  const selectedAlertID = alertIdFromRow(selectedRow);
  const selectedStateVersion = stateVersionFromRow(selectedRow);
  const allowedBatchStatuses = useMemo(() => alertAllowedNextStatuses(text(selectedRow, '__status', text(selectedRow, '状态', ''))), [selectedRow]);
  const effectiveBatchTargetStatus = allowedBatchStatuses.includes(batchTargetStatus) ? batchTargetStatus : allowedBatchStatuses[0];
  const canSubmitBatchStatus = Boolean(
    selectedAlertID &&
      effectiveBatchTargetStatus &&
      canTransitionAlertStatus(text(selectedRow, '__status', text(selectedRow, '状态', '')), effectiveBatchTargetStatus) &&
      batchReason.trim().length >= 4,
  );
  const batchStatusMutation = useMutation({
    mutationFn: () => {
      if (!selectedAlertID || !effectiveBatchTargetStatus) throw new Error('请选择可迁移的告警状态');
      return batchUpdateAlertStatus(
        [{ alertId: selectedAlertID, stateVersion: selectedStateVersion }],
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

  const tableColumns = useMemo(() => [...route.page.tableColumns, '操作'], [route.page.tableColumns]);
  const columns: ColumnsType<SnapshotRow> = tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: alertColumnWidth(column),
    ellipsis: true,
    render: (value, record) => renderAlertCell(column, value, record, (title) => openAction(title, alertIdFromRow(record))),
  }));
  const totalRows = rows.length;
  function openAction(title: string, target = selectedAlertID || 'ALERT-SIMULATED') {
    setActionSubmitted(false);
    setAction(createAlertAction(title, target));
  }
  const applyFilters = () => {
    setListPage(1);
    setFilterNotice(`${filters.asset} / ${filters.status} / ${filters.confidence}`);
  };
  const resetFilters = () => {
    setFilters({ asset: '全部资产', destination: '', rule: '全部规则', model: '全部模型', phase: '全部阶段', status: '全部状态', confidence: '全部' });
    setFilterNotice('当前队列');
    setListPage(1);
  };

  return (
    <div className="taf-page taf-alert-triage">
      <div className="taf-alert-grid">
        <main className="taf-alert-main">
          <header className="taf-alert-titlebar">
            <div>
              <h1>{route.page.title}</h1>
            </div>
            <Space>
              <Select size="small" value={view} options={[{ value: '自定义视图' }, { value: '高危优先' }, { value: '待反馈' }]} onChange={setView} />
              <Button size="small" onClick={() => openAction('保存告警视图', view)}>保存视图</Button>
              <Tooltip title="刷新告警队列">
                <Button icon={<ReloadOutlined />} size="small" onClick={() => void refetch()} />
              </Tooltip>
              {!visualBreakdownMode && <OverlayContractHost overlays={alertOverlays} compact />}
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
                <DatePicker.RangePicker size="small" showTime onChange={() => setFilterNotice('已更新告警时间窗')} />
              </label>
              <label>
                <span>资产</span>
                <Select size="small" value={filters.asset} options={[{ value: '全部资产' }, { value: '核心区' }, { value: '办公区' }]} onChange={(asset) => setFilters((current) => ({ ...current, asset }))} />
              </label>
              <label>
                <span>目的 IP</span>
                <Input size="small" value={filters.destination} prefix={<SearchOutlined />} placeholder="请输入 IP" onChange={(event) => setFilters((current) => ({ ...current, destination: event.target.value }))} />
              </label>
              <label>
                <span>规则</span>
                <Select size="small" value={filters.rule} options={[{ value: '全部规则' }, { value: 'C2_Tunnel_v3' }, { value: 'DNS_Tunnel_v2' }]} onChange={(rule) => setFilters((current) => ({ ...current, rule }))} />
              </label>
              <label>
                <span>模型</span>
                <Select size="small" value={filters.model} options={[{ value: '全部模型' }, { value: 'Lateral_Move_v2' }, { value: 'Data_Exfil_v1' }]} onChange={(model) => setFilters((current) => ({ ...current, model }))} />
              </label>
              <label>
                <span>攻击阶段</span>
                <Select size="small" value={filters.phase} options={[{ value: '全部阶段' }, { value: '命令与控制' }, { value: '横向移动' }]} onChange={(phase) => setFilters((current) => ({ ...current, phase }))} />
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
                  <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => openAction('批量指派告警')}>批量指派</Button>
                  <Button
                    size="small"
                    icon={<CheckCircleOutlined />}
                    disabled={!canSubmitBatchStatus}
                    loading={batchStatusMutation.isPending}
                    onClick={() => batchStatusMutation.mutate()}
                  >
                    批量状态变更
                  </Button>
                  <Button size="small" icon={<DownloadOutlined />} onClick={() => openAction('导出告警队列')}>导出</Button>
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
              scroll={{ x: 1240, y: 340 }}
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
              rowSelection={{ selectedRowKeys: selectedRowKey ? [selectedRowKey] : [] }}
              onRow={(record) => ({
                onClick: () => setSelectedRowKey(rowKey(record)),
              })}
            />
          </WorkPanel>
        </main>

        <aside className="taf-alert-detail">
          <AlertSummary row={selectedRow} onAction={openAction} />
          <TriageTimeline row={selectedRow} timeline={data?.timeline ?? []} />
          <ClusterCards row={selectedRow} onAction={openAction} />
          <FeedbackForm
            actions={route.page.actions}
            feedbackResult={feedbackResult}
            onFeedbackResultChange={setFeedbackResult}
            onAction={openAction}
          />
        </aside>
      </div>
      <Drawer className="taf-alert-triage-action-drawer" title={action ? `${action.title}确认` : '告警操作确认'} open={Boolean(action)} width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => { setAction(undefined); setActionSubmitted(false); }} extra={<Button size="small" type="primary" disabled={actionSubmitted} onClick={() => setActionSubmitted(true)}>{actionSubmitted ? '已写入任务队列' : '确认提交'}</Button>}>
        {action && <div className="taf-alert-detail-action-body"><p>将为告警对象创建“{action.title}”仿真任务，并保留租户、权限和审计上下文。</p><dl><dt>告警对象</dt><dd>{action.target}</dd><dt>接口预留</dt><dd>{action.endpoint}</dd><dt>审计事件</dt><dd>{action.auditEvent}</dd></dl>{actionSubmitted && <Alert type="success" showIcon message="告警业务操作已进入仿真任务队列" />}</div>}
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
      <strong>{metric.value}</strong>
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
  const alertId = text(row, '告警 ID', 'ALERT-0001');
  const firstSeen = text(row, '首次发生', '06-20 03:42:11');
  const sourceIp = text(row, '源 IP', '172.16.5.10');
  const destinationIp = text(row, '目的 IP', '185.22.14.9');
  const ruleModel = text(row, '规则/模型', 'C2_Tunnel_v3');
  const confidence = text(row, '置信度', '0.98');
  const affectedAsset = text(row, '受影响资产', '办公区-WS-1024');

  return (
    <WorkPanel title="告警详情" extra={<Button size="small" type="link" onClick={() => onAction('进入告警详情', alertId)}>进入详情</Button>}>
      <div className="taf-alert-summary">
        <AlertRiskDial score={score} />
        <div className="taf-alert-summary__facts">
          <strong title={text(row, '告警名称', '疑似 C2 隧道通信')}>{text(row, '告警名称', '疑似 C2 隧道通信')}</strong>
          <StatusTag value={text(row, '风险等级', '高危')} />
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
    <div className="taf-alert-risk-dial"><RingChart value={score} height={96} className="taf-alert-risk-echart" ariaLabel="选中告警风险评分" /></div>
  );
}

function TriageTimeline({ row, timeline }: { row?: SnapshotRow; timeline: PageSnapshot['timeline'] }) {
  const items = timeline.length
    ? timeline
    : [
        { title: '首次发生', description: `检测到 ${text(row, '攻击阶段', '异常通信')}，建议建立证据链。`, status: 'info' as const },
        { title: '异常行为', description: '心跳包特征匹配 C2 通信模式。', status: 'warn' as const },
        { title: '证据生成', description: '已生成 PCAP 证据与 Session 记录。', status: 'ok' as const },
        { title: '处置动作', description: '等待研判提交反馈结果。', status: 'risk' as const },
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
  const fixedTimes = ['03:42:11', '03:42:18', '03:43:02', '03:43:47', '03:44:12'];
  if (index === 0) return text(row, '首次发生', fixedTimes[0]).replace(/^.*?(\d{2}:\d{2}:\d{2})$/, '$1');
  return fixedTimes[index] ?? fixedTimes[0];
};

function ClusterCards({ row, onAction }: { row?: SnapshotRow; onAction: (title: string, target?: string) => void }) {
  const cards = [
    ['同源 IP', 4],
    ['同资产', 7],
    ['同攻击链', 1],
    ['同规则/模型', 12],
    ['同战役', text(row, '攻击阶段', 'APT-20260625-01')],
  ];

  return (
    <WorkPanel title="关联告警簇">
      <div className="taf-alert-clusters">
        {cards.map(([label, value]) => (
          <button key={label} type="button" onClick={() => onAction('查看关联告警簇', String(label))}>
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
  onAction,
}: {
  actions: string[];
  feedbackResult: FeedbackResult;
  onFeedbackResultChange: (value: FeedbackResult) => void;
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
        <Select size="small" placeholder="请选择误报原因" options={[{ value: '业务白名单' }, { value: '模型阈值' }, { value: '资产误归属' }]} />
        <Input.TextArea rows={3} placeholder="请输入备注信息..." />
        <div className="taf-alert-feedback__footer">
          <Button size="small" onClick={() => onFeedbackResultChange('pending')}>取消</Button>
          <Button size="small" type="primary" onClick={() => onAction('提交告警反馈', feedbackResult)}>提交反馈</Button>
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
        <Tooltip title="更多动作">
          <Button size="small" type="text" icon={<MoreOutlined />} onClick={(event) => { event.stopPropagation(); onAction('告警更多操作'); }} />
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

const buildAlertRows = (rows: SnapshotRow[]) => {
  const source = rows.length ? rows : [{ '告警 ID': 'ALERT-SIM-0001', 风险等级: '高危', 告警名称: '疑似 C2 隧道通信', 攻击阶段: '命令与控制', '源 IP': '172.16.5.10:44321', '目的 IP': '185.22.14.9:443', 受影响资产: '办公区-WS-1024', '规则/模型': 'C2_Tunnel_v3', 置信度: '0.98', 首次发生: '06-20 03:42:11', 状态: '未处理', __alertId: 'ALERT-SIM-0001', __status: 'new', __stateVersion: 1, __riskScore: 92 }];
  if (source.length >= 28) return source;
  return Array.from({ length: 28 }, (_, index) => {
    const base = source[index % source.length];
    const sequence = String(index + 1).padStart(4, '0');
    return { ...base, '告警 ID': `${String(base['告警 ID'] ?? 'ALERT-SIM')}-SIM${sequence}`, __alertId: `${String(base.__alertId ?? base['告警 ID'] ?? 'ALERT-SIM')}-SIM${sequence}`, __stateVersion: Number(base.__stateVersion ?? 0) + index + 1 };
  });
};

const createAlertAction = (title: string, target: string): AlertAction => {
  const actions = pageApiPlans['alert-detail'].actions ?? [];
  const actionId = title.includes('导出') ? 'alert-report-export'
    : /详情|证据|关联/.test(title) ? 'alert-evidence-access'
      : /隔离|阻断|剧本|白名单/.test(title) ? 'alert-response-request'
        : 'alert-investigation-note';
  const plan = actions.find((item) => item.id === actionId);
  return {
    title,
    target,
    endpoint: plan?.endpoint.replace('{id}', target) ?? `/v1/alerts/${target}/investigation-notes`,
    auditEvent: plan?.auditEvent ?? 'ALERT_INVESTIGATION_NOTE_RECORDED',
  };
};

const alertIdFromRow = (row: SnapshotRow | undefined) => text(row, '__alertId', text(row, '告警 ID', ''));

const stateVersionFromRow = (row: SnapshotRow | undefined) => {
  const value = row?.__stateVersion;
  const numeric = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? numeric : undefined;
};

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};

const riskScore = (row: SnapshotRow | undefined) => {
  const explicitScore = Number(row?.__riskScore);
  if (Number.isFinite(explicitScore) && explicitScore > 0) return Math.min(Math.round(explicitScore), 100);
  const confidence = Number(text(row, '置信度', '0').replace('%', ''));
  if (Number.isFinite(confidence) && confidence > 0) return confidence <= 1 ? Math.round(confidence * 100) : Math.min(confidence, 100);
  const severity = text(row, '风险等级', '高危');
  if (severity.includes('高') || severity.includes('严重')) return 92;
  if (severity.includes('中')) return 68;
  return 42;
};

const isStatusColumn = (column: string) =>
  column.includes('状态') || column.includes('风险') || column.includes('级别') || column.includes('结果');
