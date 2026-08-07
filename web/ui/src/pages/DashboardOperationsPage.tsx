import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CommentOutlined,
  DatabaseOutlined,
  ExclamationCircleOutlined,
  FileDoneOutlined,
  FileSearchOutlined,
  LockOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Drawer, Empty, Input, Space, Tooltip } from 'antd';
import { useMemo, useState } from 'react';
import { DashboardKpiSparklineChart, DashboardStageRateCardChart, DashboardTopTalkersChart, EvidenceClosureRingChart } from '@/components/charts';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import {
  fetchDashboardTask,
  submitDashboardTask,
  type DashboardTaskActionId,
  type DashboardTaskStatus,
} from '@/services/dashboardTaskApi';
import { pageApiPlans } from '@/services/pageApiPlans';
import type { DashboardVisuals, PageSnapshot, SnapshotRow } from '@/services/mockData';
import { formatDashboardDeficitContext, formatDashboardMetricDisplay } from './dashboardDisplay';

const DASHBOARD_QUEUE_PAGE_SIZE = 8;

type DashboardAction = {
  actionId: DashboardTaskActionId;
  title: string;
  target: string;
  endpoint: string;
  auditEvent: string;
};

const dashboardOverlays: OverlayContract[] = [
  {
    id: 'modal-global-search',
    title: '全局搜索弹窗',
    kind: 'Modal',
    actionLabel: '全局搜索',
    description: '跨事件、资产、规则、证据和审计日志检索，保留当前租户与权限过滤。',
    impact: '只读检索，不改变业务状态。',
  },
  {
    id: 'drawer-dashboard-kpi-detail',
    title: '仪表盘 KPI 详情',
    kind: 'Drawer',
    actionLabel: 'KPI 详情',
    description: '展示 KPI 口径、采样窗口、来源 API 和异常贡献项。',
    impact: '用于解释当前运营指标，关联 dashboard stats 与 evidence 汇总。',
  },
  {
    id: 'drawer-dashboard-task-detail',
    title: '待办任务详情',
    kind: 'Drawer',
    actionLabel: '任务详情',
    description: '查看待办任务的 SLA、责任人、影响资产和下一步处置动作。',
    audit: '记录任务查看、认领和跳转处置 trace。',
  },
];

export function DashboardOperationsPage({ route }: { route: NavRoute }) {
  const [action, setAction] = useState<DashboardAction>();
  const [actionReason, setActionReason] = useState('');
  const [acceptedTaskId, setAcceptedTaskId] = useState('');
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    refetchInterval: 5000,
    refetchIntervalInBackground: true,
  });

  const dashboardVisuals = data?.visuals?.dashboard;
  const highRisk = metricValue(data, '高危未处理');
  const timeoutSla = metricValue(data, '超时 SLA');
  const evidenceGap = metricValue(data, '待取证');
  const feedbackGap = metricValue(data, '待反馈');
  const auditGap = metricValue(data, '审计留痕缺口');
  const complianceGap = metricValue(data, '合规门禁缺口');
  const pageSnapshotId = data?.snapshot?.snapshotId ?? '';
  const taskMutation = useMutation({
    mutationFn: submitDashboardTask,
    onSuccess: (receipt) => setAcceptedTaskId(receipt.taskId),
  });
  const taskQuery = useQuery({
    queryKey: ['dashboard-task', acceptedTaskId],
    queryFn: () => fetchDashboardTask(acceptedTaskId),
    enabled: Boolean(acceptedTaskId),
    refetchInterval: (query) => isTerminalDashboardTaskStatus(query.state.data?.status) ? false : 3000,
  });
  const taskStatus = taskQuery.data?.status ?? taskMutation.data?.status;

  const openAction = (title: string, target: string) => {
    taskMutation.reset();
    setAcceptedTaskId('');
    setActionReason(`仪表盘运营动作：${title}`);
    setAction(createDashboardAction(title, target));
  };

  const submitAction = () => {
    if (!action || !pageSnapshotId) return;
    taskMutation.mutate({
      actionId: action.actionId,
      target: action.target,
      priority: 'high',
      snapshotId: pageSnapshotId,
      reason: actionReason,
      context: { page_id: route.id, source: 'dashboard-operations', audit_event: action.auditEvent },
    });
  };

  return (
    <div className="taf-page taf-dashboard-workbench">
      <header className="taf-dashboard-titlebar" aria-label="仪表盘辅助操作">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Input size="small" prefix={<SearchOutlined />} placeholder="事件、资产组、业务系统" />
          <Tooltip title="刷新仪表盘数据">
            <Button icon={<ReloadOutlined />} size="small" onClick={() => void refetch()} />
          </Tooltip>
          <Button type="primary" size="small" onClick={() => openAction('创建闭环任务', '仪表盘运营闭环')}>创建闭环任务</Button>
          <OverlayContractHost overlays={dashboardOverlays} compact />
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

      <WorkPanel title="脱敏运营 KPI">
        {data?.snapshot?.partial && (
          <Alert
            showIcon
            type="warning"
            message="统一快照部分可用"
            description={`缺失分区：${data.snapshot.missingSections.join('、') || '未声明'}`}
          />
        )}
        <div className="taf-dashboard-kpis">
          {(data?.metrics ?? []).map((metric, index) => (
            <DashboardKpiTile key={metric.label} metric={metric} index={index} sparkValues={dashboardVisuals?.kpiSparks[index]} />
          ))}
        </div>
      </WorkPanel>

      <div className="taf-dashboard-grid">
        <WorkPanel title={route.page.tableTitle} className="taf-dashboard-queue">
          <DashboardQueueTable
            columns={route.page.tableColumns}
            rows={data?.rows ?? []}
            total={data?.total}
            loading={isLoading}
          />
        </WorkPanel>

        <WorkPanel title="采集与数据健康门禁" className="taf-dashboard-gates">
          <HealthGateMatrix gates={dashboardVisuals?.healthGates ?? []} />
        </WorkPanel>

        <WorkPanel title={route.page.rightRailTitle} className="taf-dashboard-deficits">
          <DeficitList
            items={[
              ['高危未处理数', highRisk, metricDelta(data, '高危未处理'), '处置告警'],
              ['待补证据数', evidenceGap, metricDelta(data, '待取证'), '补齐证据'],
              ['待回流样本数', feedbackGap, metricDelta(data, '待反馈'), '回流样本'],
              ['审计留痕缺口', auditGap, metricDelta(data, '审计留痕缺口'), '完善留痕'],
              ['工单逾期数', timeoutSla, metricDelta(data, '超时 SLA'), '跟进处理'],
              ['合规门禁缺口', complianceGap, metricDelta(data, '合规门禁缺口'), '修复门禁'],
            ]}
            onAction={openAction}
          />
        </WorkPanel>

        <WorkPanel title="告警处置阶段工作篮" className="taf-dashboard-stage">
          <StageBasket stages={dashboardVisuals?.stages ?? []} />
        </WorkPanel>

        <WorkPanel title="证据与反馈质量摘要" className="taf-dashboard-quality">
          <EvidenceQuality items={dashboardVisuals?.qualityRings ?? []} />
        </WorkPanel>

        <WorkPanel title="Top Talkers 风险贡献" className="taf-dashboard-talkers">
          <TopTalkers talkers={dashboardVisuals?.topTalkers ?? []} />
        </WorkPanel>
      </div>
      <Drawer
        className="taf-dashboard-action-drawer"
        title={action ? `${action.title}确认` : '仪表盘任务确认'}
        open={Boolean(action)}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onClose={() => {
          setAction(undefined);
          setActionReason('');
          setAcceptedTaskId('');
          taskMutation.reset();
        }}
        extra={(
          <Button
            size="small"
            type="primary"
            loading={taskMutation.isPending}
            disabled={!actionReason.trim() || !pageSnapshotId || Boolean(taskMutation.data)}
            onClick={submitAction}
          >
            {taskMutation.data ? '任务已受理' : '确认提交'}
          </Button>
        )}
      >
        {action && (
          <div className="taf-alert-detail-action-body">
            <p>将创建“{action.title}”真实任务。提交成功只表示后端已受理，最终状态以任务查询结果为准。</p>
            <dl>
              <dt>任务目标</dt><dd>{action.target}</dd>
              <dt>请求端点</dt><dd>{action.endpoint}</dd>
              <dt>页面数据版本</dt><dd>{pageSnapshotId}</dd>
              <dt>审计事件</dt><dd>{action.auditEvent}</dd>
            </dl>
            <Input.TextArea
              aria-label="任务提交原因"
              value={actionReason}
              onChange={(event) => setActionReason(event.target.value)}
              rows={3}
              maxLength={500}
              showCount
              disabled={Boolean(taskMutation.data)}
            />
            {taskMutation.isError && (
              <Alert
                type="error"
                showIcon
                message="真实任务提交失败"
                description={taskMutation.error instanceof Error ? taskMutation.error.message : '请检查权限、数据库Schema和服务状态。'}
              />
            )}
            {taskMutation.data && (
              <Alert
                type={dashboardTaskAlertType(taskStatus)}
                showIcon
                message={dashboardTaskStatusMessage(taskStatus)}
                description={`任务 ${taskMutation.data.jobId} · trace ${taskMutation.data.traceId} · revision ${taskQuery.data?.revision ?? taskMutation.data.revision}${taskMutation.data.replayed ? ' · 幂等重放' : ''}`}
              />
            )}
            {taskQuery.isError && (
              <Alert
                type="warning"
                showIcon
                message="任务状态恢复失败"
                description={taskQuery.error instanceof Error ? taskQuery.error.message : '任务已受理，但当前无法读取最终状态。'}
                action={<Button size="small" onClick={() => void taskQuery.refetch()}>重试</Button>}
              />
            )}
          </div>
        )}
      </Drawer>
    </div>
  );
}

function DashboardKpiTile({
  metric,
  index,
  sparkValues,
}: {
  metric: PageSnapshot['metrics'][number];
  index: number;
  sparkValues?: number[];
}) {
  const icons = [
    <ClockCircleOutlined />,
    <ClockCircleOutlined />,
    <WarningOutlined />,
    <FileSearchOutlined />,
    <CommentOutlined />,
    <LockOutlined />,
    <DatabaseOutlined />,
    <SafetyCertificateOutlined />,
  ];
  const isProgress = metric.label.includes('闭环');

  return (
    <article className={`taf-dashboard-kpi is-${metric.status}${isProgress ? ' is-progress' : ''}`}>
      <span className="taf-dashboard-kpi__icon">{icons[index] ?? <ExclamationCircleOutlined />}</span>
          <span className="taf-dashboard-kpi__label" title={metric.label}>{metric.label}</span>
      {isProgress ? (
        <div className="taf-dashboard-kpi__progress">
          <span className="taf-dashboard-kpi__ring">
            <EvidenceClosureRingChart
              item={{
                label: metric.label,
                value: dashboardPercent(metric.value),
                level: dashboardRingLevel(metric.status),
              }}
              ariaLabel={`${metric.label}实时闭环率`}
            />
          </span>
          <small title={metric.delta}>{metric.delta}</small>
        </div>
      ) : (
        <>
          <strong title={metric.value}>{formatDashboardMetricDisplay(metric.value)}</strong>
          <small title={metric.delta}>{metric.delta}</small>
          <div className="taf-dashboard-kpi__spark">
            <DashboardKpiSparklineChart
              item={{
                label: metric.label,
                values: sparkValues ?? [],
                level: dashboardChartLevel(metric.status),
              }}
            />
          </div>
        </>
      )}
    </article>
  );
}

function DashboardQueueTable({
  columns,
  rows,
  total,
  loading,
}: {
  columns: string[];
  rows: SnapshotRow[];
  total?: number;
  loading: boolean;
}) {
  const [page, setPage] = useState(1);
  const totalRows = Math.max(total ?? rows.length, rows.length);
  const totalPages = Math.max(1, Math.ceil(totalRows / DASHBOARD_QUEUE_PAGE_SIZE));
  const currentPage = Math.min(page, totalPages);
  const pageRows = useMemo(
    () => rows.slice((currentPage - 1) * DASHBOARD_QUEUE_PAGE_SIZE, currentPage * DASHBOARD_QUEUE_PAGE_SIZE),
    [currentPage, rows],
  );
  const pages = dashboardPaginationItems(currentPage, totalPages);

  if (loading && !rows.length) {
    return <div className="taf-dashboard-queue-table is-loading">正在加载</div>;
  }
  if (!rows.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;

  return (
    <div className="taf-dashboard-queue-table">
      <div className="taf-dashboard-queue-table__head">
        {columns.map((column) => (
          <span key={column} title={column}>{column.replace('事件 ID', '事件ID')}</span>
        ))}
      </div>
      {pageRows.map((row, rowIndex) => (
        <div key={`${String(row[columns[0]])}-${currentPage}-${rowIndex}`} className="taf-dashboard-queue-table__row">
          {columns.map((column) => {
            const value = row[column];
            return (
              <span key={column} title={String(value)}>
                {isDashboardStatusColumn(column) ? <DashboardPill value={value} /> : value}
              </span>
            );
          })}
        </div>
      ))}
      <div className="taf-dashboard-queue-table__footer">
        <span>共 {formatDashboardNumber(totalRows)} 条</span>
        <nav className="taf-dashboard-pages" aria-label="优先级待办队列分页">
          <button type="button" title="上一页" disabled={currentPage <= 1} onClick={() => setPage(currentPage - 1)}>‹</button>
          {pages.map((item, index) => (
            item === 'ellipsis'
              ? <i key={`ellipsis-${index}`}>...</i>
              : (
                <button
                  key={item}
                  type="button"
                  title={`第 ${item} 页`}
                  className={item === currentPage ? 'is-active' : undefined}
                  aria-current={item === currentPage ? 'page' : undefined}
                  onClick={() => setPage(item)}
                >
                  {item}
                </button>
              )
          ))}
          <button type="button" title="下一页" disabled={currentPage >= totalPages} onClick={() => setPage(currentPage + 1)}>›</button>
        </nav>
      </div>
    </div>
  );
}

function HealthGateMatrix({ gates }: { gates: DashboardVisuals['healthGates'] }) {
  if (!gates.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  const summary = gates.reduce(
    (acc, item) => {
      const tone = dashboardTone(item.status);
      acc[tone] += 1;
      return acc;
    },
    { ok: 0, warn: 0, risk: 0 },
  );

  return (
    <div className="taf-health-gates">
      <div className="taf-health-gates__head">
        <span title="组件">组件</span>
        <span title="健康状态">健康状态</span>
        <span title="失败原因">失败原因</span>
        <span title="影响范围">影响范围</span>
        <span title="更新时间">更新时间</span>
      </div>
      {gates.map((item) => (
        <div key={item.component} className="taf-health-gates__row">
          <strong title={item.component}>{item.component}</strong>
          <DashboardPill value={item.status} />
          <span title={item.reason}>{item.reason}</span>
          <span title={item.scope}>{item.scope}</span>
          <span title={item.updated}>{item.updated}</span>
        </div>
      ))}
      <div className="taf-health-gates__summary">
        <span className="is-ok"><CheckCircleOutlined /> 正常 {summary.ok}</span>
        <span className="is-warn"><WarningOutlined /> 告警 {summary.warn}</span>
        <span className="is-risk"><SafetyCertificateOutlined /> 异常 {summary.risk}</span>
      </div>
    </div>
  );
}

function DeficitList({
  items,
  onAction,
}: {
  items: Array<[string, string, string, string]>;
  onAction: (title: string, target: string) => void;
}) {
  return (
    <div className="taf-deficit-list">
      {items.map(([label, value, delta, action]) => (
        <div key={label} className="taf-deficit-item">
          <span className="taf-deficit-item__icon"><FileDoneOutlined /></span>
          <span title={label}>{label}</span>
          <strong title={value}>{formatDashboardMetricDisplay(value)}</strong>
          <em title={delta || '未提供口径'}>{formatDashboardDeficitContext(delta || '未提供口径')}</em>
          <button
            type="button"
            className="taf-deficit-action"
            title={isActionableDeficit(value) ? action : `${action}（当前无可操作缺口）`}
            disabled={!isActionableDeficit(value)}
            onClick={() => onAction(action, label)}
          >
            {compactActionLabel(action)}
          </button>
        </div>
      ))}
    </div>
  );
}

function StageBasket({ stages }: { stages: DashboardVisuals['stages'] }) {
  if (!stages.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;

  return (
    <div className="taf-stage-basket">
      <div className="taf-stage-basket__cards" data-stage-count={stages.length}>
        {stages.map((stage, index) => (
          <div key={stage.label} className={`taf-stage-card is-${stage.status}`}>
            <span title={stage.label}><StageIcon index={index} />{stage.label}</span>
            <strong title={stage.value}>{formatDashboardMetricDisplay(stage.value)}</strong>
            <em title={dashboardStageStateLabel(stage.status)}>{dashboardStageStateLabel(stage.status)}</em>
            <div className="taf-stage-rate-chart">
              <DashboardStageRateCardChart
                label={stage.label}
                values={stage.bars}
                level={dashboardChartLevel(stage.status)}
              />
            </div>
            <small title={stage.footnote}>{stage.footnote}</small>
          </div>
        ))}
      </div>
      <small className="taf-stage-basket__note">注：统计为当前处理阶段的事件数量</small>
    </div>
  );
}

function EvidenceQuality({ items }: { items: DashboardVisuals['qualityRings'] }) {
  if (!items.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;

  return (
    <div className="taf-quality-rings">
      {items.slice(0, 5).map((item) => (
        <div key={item.label} className={`taf-quality-ring is-${item.status}`}>
          <span className="taf-quality-ring__dial">
            <EvidenceClosureRingChart
              item={{
                label: item.label,
                value: item.ringPercent,
                level: dashboardRingLevel(item.status),
              }}
              ariaLabel={`${item.label}实时质量环`}
            />
          </span>
          <span title={item.label}>{item.label}</span>
          <small title={item.subtext}>{item.subtext}</small>
        </div>
      ))}
    </div>
  );
}

function TopTalkers({ talkers }: { talkers: DashboardVisuals['topTalkers'] }) {
  if (!talkers.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;

  return (
    <div className="taf-talker-chart">
      <DashboardTopTalkersChart items={talkers} />
    </div>
  );
}

function DashboardPill({ value }: { value: unknown }) {
  const text = String(value);
  const tone = text.includes('高') || text.includes('异常') || text.includes('缺失')
    ? 'risk'
    : text.includes('中') || text.includes('警') || text.includes('不完整')
      ? 'warn'
      : text.includes('低')
        ? 'info'
        : 'ok';
  return <em className={`taf-dashboard-pill is-${tone}`} title={text}>{text}</em>;
}

function StageIcon({ index }: { index: number }) {
  const icons = [
    <FileDoneOutlined />,
    <ClockCircleOutlined />,
    <CommentOutlined />,
    <FileSearchOutlined />,
    <LockOutlined />,
    <SafetyCertificateOutlined />,
  ];
  return icons[index] ?? <FileDoneOutlined />;
}

const dashboardStageStateLabel = (status: DashboardVisuals['stages'][number]['status']) => ({
  risk: '需优先处理',
  warn: '需要关注',
  ok: '状态正常',
  info: '状态信息',
}[status]);

const isActionableDeficit = (value: string) => {
  if (!value || value === '--') return false;
  const parsed = Number.parseFloat(value.replace(/[^\d.-]/g, ''));
  return Number.isFinite(parsed) && parsed > 0;
};

const compactActionLabel = (action: string) => {
  if (action.includes('补齐')) return '补证';
  if (action.includes('回流')) return '回流';
  if (action.includes('留痕')) return '留痕';
  if (action.includes('跟进')) return '跟进';
  if (action.includes('修复')) return '修复';
  return action.slice(0, 2);
};

const metricValue = (data: PageSnapshot | undefined, label: string, fallback = '--') =>
  data?.metrics.find((metric) => metric.label === label)?.value ?? fallback;

const metricDelta = (data: PageSnapshot | undefined, label: string, fallback = '实时') =>
  data?.metrics.find((metric) => metric.label === label)?.delta ?? fallback;

const dashboardPercent = (value: string) => {
  const parsed = Number.parseFloat(value.replace(/[^\d.-]/g, ''));
  return Number.isFinite(parsed) ? Math.max(0, Math.min(100, parsed)) : 0;
};

const dashboardRingLevel = (status: PageSnapshot['metrics'][number]['status']) => {
  if (status === 'risk') return 'high';
  if (status === 'warn') return 'medium';
  return 'low';
};

const dashboardChartLevel = (status: DashboardVisuals['stages'][number]['status']) => {
  if (status === 'risk') return 'high';
  if (status === 'warn') return 'medium';
  if (status === 'info') return 'info';
  return 'low';
};

const dashboardTone = (value: string): 'ok' | 'warn' | 'risk' => {
  if (value.includes('异常') || value.includes('高')) return 'risk';
  if (value.includes('告警') || value.includes('中')) return 'warn';
  return 'ok';
};

const isDashboardStatusColumn = (column: string) =>
  column.includes('状态') || column.includes('风险') || column.includes('级别') || column.includes('结果');

const dashboardPaginationItems = (currentPage: number, totalPages: number): Array<number | 'ellipsis'> => {
  if (totalPages <= 7) return Array.from({ length: totalPages }, (_, index) => index + 1);
  const middle = new Set([1, totalPages, currentPage - 1, currentPage, currentPage + 1, 2, 3].filter((item) => item >= 1 && item <= totalPages));
  const sorted = Array.from(middle).sort((left, right) => left - right);
  return sorted.flatMap((item, index) => {
    const previous = sorted[index - 1];
    return previous && item - previous > 1 ? ['ellipsis' as const, item] : [item];
  });
};

const formatDashboardNumber = (value: number) =>
  new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(Math.max(0, value));

const createDashboardAction = (title: string, target: string): DashboardAction => {
  const plan = pageApiPlans.dashboard.actions?.find((item) => item.label === title);
  return {
    actionId: (plan?.id ?? 'dashboard-task-create') as DashboardTaskActionId,
    title,
    target,
    endpoint: plan?.endpoint ?? '/v1/dashboard/tasks',
    auditEvent: plan?.auditEvent ?? 'DASHBOARD_TASK_CREATED',
  };
};

const isTerminalDashboardTaskStatus = (status?: DashboardTaskStatus) =>
  status === 'completed' || status === 'partial' || status === 'failed' || status === 'cancelled';

const dashboardTaskAlertType = (status?: DashboardTaskStatus): 'info' | 'success' | 'warning' | 'error' => {
  if (status === 'completed') return 'success';
  if (status === 'failed') return 'error';
  if (status === 'partial' || status === 'cancelled' || status === 'running') return 'warning';
  return 'info';
};

const dashboardTaskStatusMessage = (status?: DashboardTaskStatus) => ({
  accepted: '任务已受理，尚未最终完成',
  running: '任务执行中，尚未最终完成',
  completed: '任务最终完成',
  partial: '任务部分完成，请检查缺失项',
  failed: '任务最终失败',
  cancelled: '任务已取消或补偿',
}[status ?? 'accepted']);
