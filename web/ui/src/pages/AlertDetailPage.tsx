import {
  ApiOutlined,
  ArrowLeftOutlined,
  AuditOutlined,
  BlockOutlined,
  CheckCircleOutlined,
  CloudDownloadOutlined,
  CodeOutlined,
  CopyOutlined,
  ClusterOutlined,
  DatabaseOutlined,
  DesktopOutlined,
  EyeOutlined,
  FileTextOutlined,
  GlobalOutlined,
  InfoCircleOutlined,
  LinkOutlined,
  MoreOutlined,
  NodeIndexOutlined,
  PaperClipOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  SendOutlined,
  StopOutlined,
  UserOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Checkbox, Empty, Input, Modal, Popconfirm, Radio, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { Fragment, useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { RiskScoreRingChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { ALERT_DETAIL_EVIDENCE_PAGE_SIZE, evidenceFocusRoute, evidenceFocusView, evidenceViewRoute } from '@/pages/alertDetailLogic';
import type { NavRoute } from '@/routes/routeManifest';
import {
  fetchAlertDetailSnapshot,
  submitAlertFeedback,
  updateAlertStatus,
  type AlertDetailEvidenceRow,
  type AlertDetailSnapshot,
} from '@/services/alertDetailApi';
import {
  alertDetailActionErrorMessage,
  cancelAlertReportWithRevisionRefresh,
  compensateAlertReport,
  downloadAlertEvidenceFile,
  fetchAlertCampaignLinks,
  fetchAlertReportJob,
  submitAlertDetailAction,
  submitAlertReportWithSnapshotRetry,
  type AlertCampaignLink,
  type AlertDetailActionId,
  type AlertDetailActionResult,
} from '@/services/alertDetailActionApi';
import {
  alertAllowedNextStatuses,
  alertStatusLabel,
  canTransitionAlertStatus,
  type AlertStatusCode,
} from '@/services/alertStatus';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type AlertDetailBusinessAction = {
  id: AlertDetailActionId;
  label: string;
  target: string;
  description: string;
};

function buildEvidenceColumns(
  onDownload: (row: AlertDetailEvidenceRow) => void,
  onView: (row: AlertDetailEvidenceRow) => void,
  downloading: boolean,
): ColumnsType<AlertDetailEvidenceRow> {
  return [
  { title: '证据类型', dataIndex: '证据类型', key: '证据类型', width: 96, render: renderTextCell },
  { title: '文件 / 记录', dataIndex: '文件记录', key: '文件记录', ellipsis: true, render: renderTextCell },
  { title: '内容摘要', dataIndex: '内容摘要', key: '内容摘要', ellipsis: true, render: renderTextCell },
  { title: '大小', dataIndex: '大小', key: '大小', width: 92, render: renderTextCell },
  { title: '生成时间', dataIndex: '生成时间', key: '生成时间', width: 150, render: renderTextCell },
  { title: '状态', dataIndex: '状态', key: '状态', width: 92, render: (value) => <StatusTag value={value} /> },
    {
      title: '操作',
      dataIndex: '操作',
      key: '操作',
      width: 108,
      render: (_value, row) => (
        <span className="taf-alert-detail-evidence-actions">
          <Tooltip title={`下载：${row.文件记录}`}>
            <Button
              type="text"
              size="small"
              className="taf-alert-detail-evidence-action is-download"
              aria-label={`下载证据 ${row.文件记录}`}
              icon={<CloudDownloadOutlined />}
              loading={downloading}
              onClick={() => onDownload(row)}
            />
          </Tooltip>
          <Tooltip title={`查看：${row.文件记录}`}>
            <Button
              type="text"
              size="small"
              className="taf-alert-detail-evidence-action is-view"
              aria-label={`查看证据 ${row.文件记录}`}
              icon={<EyeOutlined />}
              onClick={() => onView(row)}
            />
          </Tooltip>
        </span>
      ),
    },
  ];
}

type FeedbackChoice = 'tp' | 'fp' | 'pending';

const alertActionStatusLabels: Record<AlertDetailActionResult['status'], string> = {
  recorded: '已记录',
  pending_approval: '待审批',
  approved_awaiting_executor: '已审批，等待执行器',
  blocked_external_executor: '外部执行器未配置',
  compensation_blocked_external_executor: '补偿执行器未配置',
  linked: '已关联',
  unlinked: '已解除关联',
  accepted: '已受理',
  running: '执行中',
  cancel_requested: '取消中',
  completed: '最终成功',
  partial: '部分成功',
  failed: '失败',
  cancelled: '已取消',
  compensating: '补偿中',
  compensated: '已补偿',
  compensation_failed: '补偿失败',
};

const feedbackReasonOptions = [
  { value: 'FALSE_ALARM', label: '规则/模型误报' },
  { value: 'BUSINESS_NORMAL', label: '正常业务行为' },
  { value: 'AUTHORIZED', label: '授权行为' },
  { value: 'TEST', label: '测试流量' },
  { value: 'WHITELIST', label: '已知白名单行为' },
  { value: 'TUNING_NEEDED', label: '需要调优' },
  { value: 'OTHER', label: '其他原因' },
];

function renderTextCell(value: unknown) {
  const text = String(value ?? '');
  return <span title={text}>{text}</span>;
}

function createAlertDetailAction(
  id: AlertDetailActionId,
  label: string,
  target: string,
  description?: string,
): AlertDetailBusinessAction {
  if (id === 'alert-label-update') {
    return {
      id,
      label,
      target,
      description: description ?? `编辑 ${target} 的告警标签；保存后写入告警版本和审计日志。`,
    };
  }
  if (id === 'alert-report-export') {
    return {
      id,
      label,
      target,
      description: description ?? `将为 ${target} 创建告警报告导出任务，并保留下载审计。`,
    };
  }
  if (id === 'alert-campaign-link') {
    return {
      id,
      label,
      target,
      description: description ?? '输入同租户战役 ID；系统将在一个事务内写入关系、历史、投影 outbox 与审计。',
    };
  }
  if (id === 'alert-response-request') {
    return {
      id,
      label,
      target,
      description: description ?? `将为 ${target} 创建“${label}”的受控响应请求，默认仅生成 dry-run 任务。`,
    };
  }
  if (id === 'alert-evidence-access') {
    return {
      id,
      label,
      target,
      description: description ?? `将登记 ${target} 的证据访问请求，并生成受控访问任务。`,
    };
  }
  return {
    id,
    label,
    target,
    description: description ?? `将把“${label}”记录为 ${target} 的研判操作，并生成审计任务。`,
  };
}

type EvidenceFocusActionProps = {
  alertId: string;
  title: string;
  target?: string;
  description?: string;
  className?: string;
  ariaLabel?: string;
  ariaPressed?: boolean;
  as?: 'button' | 'link';
  children: ReactNode;
};

function EvidenceFocusAction({
  alertId,
  title,
  target = title,
  description,
  className,
  ariaLabel,
  ariaPressed,
  as = 'button',
  children,
}: EvidenceFocusActionProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [result, setResult] = useState<AlertDetailActionResult>();
  const action = createAlertDetailAction('alert-evidence-access', title, target, description);
  const mutation = useMutation({
    mutationFn: submitAlertDetailAction,
    onSuccess: (submission) => {
      setResult(submission);
      message.success(`${submission.action}已持久化：${submission.jobId}`);
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '证据操作提交失败'),
  });
  const focusedEvidenceView = evidenceFocusView(title);
  const closeAction = () => {
    setOpen(false);
    setResult(undefined);
    mutation.reset();
  };
  const openAction = () => {
    if (focusedEvidenceView) {
      navigate(evidenceFocusRoute(alertId, location.search, focusedEvidenceView), { replace: true });
      return;
    }
    mutation.reset();
    setResult(undefined);
    setOpen(true);
  };
  const dialog = (
    <Modal
      className="taf-alert-detail-action-drawer"
      title={`${title}确认`}
      open={open}
      centered
      width={520}
      onCancel={closeAction}
      footer={(
        <Space>
          <Button size="small" onClick={closeAction}>取消</Button>
          <Button
            size="small"
            type="primary"
            loading={mutation.isPending}
            disabled={Boolean(result)}
            onClick={() => mutation.mutate({ alertId, actionId: action.id, target })}
          >
            {result ? '已生成任务' : '确认提交'}
          </Button>
        </Space>
      )}
    >
      <div className="taf-alert-detail-action-body">
        <p>{action.description}</p>
        <dl>
          <dt>告警对象</dt><dd>{alertId}</dd>
          <dt>操作目标</dt><dd>{target}</dd>
          <dt>执行状态</dt><dd>{result?.status === 'pending_approval' ? '已进入响应审批队列' : '已记录并保留审计事件'}</dd>
        </dl>
        {result && <Alert type="success" showIcon message={`任务 ${result.jobId} 已排队`} description={`${result.auditEvent}；${result.apiContract}`} />}
      </div>
    </Modal>
  );

  if (as === 'link') {
    return (
      <>
        <a
          href="#evidence-action"
          className={className}
          title={title}
          aria-label={ariaLabel}
          onClick={(event) => {
            event.preventDefault();
            openAction();
          }}
        >
          {children}
        </a>
        {dialog}
      </>
    );
  }

  return (
    <>
      <button type="button" className={className} title={title} aria-label={ariaLabel} aria-pressed={ariaPressed} onClick={openAction}>
        {children}
      </button>
      {dialog}
    </>
  );
}

export function AlertDetailPage({ route }: { route: NavRoute }) {
  const params = useParams();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const visualBreakdownMode = import.meta.env.DEV && isVisualBreakdownMode();
  const visualPageId = searchParams.get('__codex_page_id') || searchParams.get('pageId') || '';
  const evidenceView = searchParams.get('evidenceView') || searchParams.get('evidence') || '';
  const requestedReturnTo = searchParams.get('returnTo') || '';
  const returnTo = requestedReturnTo === '/alerts' || requestedReturnTo.startsWith('/alerts?')
    ? requestedReturnTo
    : '/alerts';
  const evidenceFilesFocusMode = visualPageId === 'alert-detail-evidence-files' || evidenceView === 'files';
  const evidencePcapFocusMode = visualPageId === 'alert-detail-evidence-pcap' || evidenceView === 'pcap';
  const evidenceSessionFocusMode =
    visualPageId === 'alert-detail-evidence-session' || evidenceView === 'session' || evidenceView === 'sessions';
  const evidenceLogsFocusMode = visualPageId === 'alert-detail-evidence-logs' || evidenceView === 'logs' || evidenceView === 'log';
  const evidenceGraphPathFocusMode =
    visualPageId === 'alert-detail-evidence-graph-path' || evidenceView === 'graph-path' || evidenceView === 'graph';
  const alertId = params.alertId ?? 'AL-20260620-000123';
  const [targetStatus, setTargetStatus] = useState<AlertStatusCode>();
  const [statusReason, setStatusReason] = useState('');
  const [feedbackResult, setFeedbackResult] = useState<FeedbackChoice>('tp');
  const [feedbackReason, setFeedbackReason] = useState('FALSE_ALARM');
  const [feedbackComment, setFeedbackComment] = useState('');
  const [feedbackAddToWhitelist, setFeedbackAddToWhitelist] = useState(false);
  const [lastWhitelistDraftUrl, setLastWhitelistDraftUrl] = useState('');
  const [businessAction, setBusinessAction] = useState<AlertDetailBusinessAction>();
  const [businessActionResult, setBusinessActionResult] = useState<AlertDetailActionResult>();
  const [businessActionTarget, setBusinessActionTarget] = useState('');
  const [businessActionReason, setBusinessActionReason] = useState('');
  const [evidencePage, setEvidencePage] = useState(1);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['alert-detail', alertId],
    queryFn: () => fetchAlertDetailSnapshot(alertId),
    refetchInterval: visualBreakdownMode ? false : 30_000,
    refetchIntervalInBackground: true,
  });
  const statusMutation = useMutation({
    mutationFn: () => {
      if (!targetStatus) throw new Error('请选择目标状态');
      return updateAlertStatus(alertId, targetStatus, statusReason, snapshot.stateVersion);
    },
    onSuccess: async (result) => {
      message.success(`告警状态已提交：${alertStatusLabel(result.newStatus)}`);
      setTargetStatus(undefined);
      setStatusReason('');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(mutationError instanceof Error ? mutationError.message : '状态变更提交失败');
    },
  });

  const snapshot = data ?? emptySnapshot(alertId);
  const loadedAlertId = data?.alertId;
  const loadedFeedbackResult = data?.feedback.defaultResult;
  const loadedFeedbackReason = data?.feedback.reason;
  const loadedWhitelistDraft = data?.feedback.whitelistDraft;
  useEffect(() => {
    if (!loadedFeedbackResult) return;
    setFeedbackResult(loadedFeedbackResult);
    setFeedbackReason(loadedFeedbackReason || 'FALSE_ALARM');
    setFeedbackAddToWhitelist(Boolean(loadedWhitelistDraft));
    setLastWhitelistDraftUrl('');
  }, [loadedAlertId, loadedFeedbackReason, loadedFeedbackResult, loadedWhitelistDraft]);
  const allowedNextStatuses = useMemo(() => alertAllowedNextStatuses(snapshot.status), [snapshot.status]);
  const canSubmitStatusChange = Boolean(
    targetStatus && canTransitionAlertStatus(snapshot.status, targetStatus) && statusReason.trim().length >= 4,
  );
  const canSubmitFeedback = feedbackResult !== 'pending' && (feedbackResult !== 'fp' || Boolean(feedbackReason));
  const whitelistPreview = snapshot.feedback.whitelistDraft || '按当前告警源 / 目的地址生成';
  const sourceAsset = snapshot.assets[0];
  const destinationAsset = snapshot.assets[1];
  const feedbackMutation = useMutation({
    mutationFn: () =>
      submitAlertFeedback(alertId, {
        label: feedbackResult === 'fp' ? 'FP' : 'TP',
        reasonCode: feedbackResult === 'fp' ? feedbackReason : undefined,
        comment: feedbackComment,
        addToWhitelist: feedbackResult === 'fp' && feedbackAddToWhitelist,
      }),
    onSuccess: async (result) => {
      const draftUrl = result.whitelistDraft?.url ?? '';
      setLastWhitelistDraftUrl(draftUrl);
      setFeedbackComment('');
      message.success(draftUrl ? '反馈已提交，白名单草案已生成' : '反馈已提交');
      await refetch();
    },
    onError: (mutationError) => {
      message.error(mutationError instanceof Error ? mutationError.message : '反馈提交失败');
    },
  });
  const campaignLinksQuery = useQuery({
    queryKey: ['alert-campaign-links', alertId],
    queryFn: () => fetchAlertCampaignLinks(alertId),
    enabled: Boolean(alertId),
    staleTime: 10_000,
  });
  const businessActionMutation = useMutation({
    mutationFn: (input: Parameters<typeof submitAlertDetailAction>[0]) => (
      input.actionId === 'alert-report-export'
        ? submitAlertReportWithSnapshotRetry(input, async () => {
          const refreshed = await refetch();
          const stateVersion = refreshed.data?.stateVersion;
          if (!Number.isSafeInteger(stateVersion) || Number(stateVersion) < 0) {
            throw new Error('刷新告警详情后未返回可用 revision');
          }
          return `alert:${alertId}:revision:${stateVersion}`;
        })
        : submitAlertDetailAction(input)
    ),
    onSuccess: async (result) => {
      setBusinessActionResult(result);
      message.success(`${result.action}已持久化：${result.jobId}`);
      await Promise.all([refetch(), campaignLinksQuery.refetch()]);
    },
    onError: (mutationError) => {
      message.error(mutationError instanceof Error ? mutationError.message : '业务动作提交失败');
    },
  });
  const unlinkCampaignMutation = useMutation({
    mutationFn: (link: AlertCampaignLink) => submitAlertDetailAction({
      alertId,
      actionId: 'alert-campaign-unlink',
      target: link.campaignId,
      reason: `告警详情确认解除与战役 ${link.campaignId} 的成员关系`,
      detail: {
        expectedRevision: link.revision,
        ...(link.currentCampaignRevision > 0
          ? { expectedCampaignRevision: link.currentCampaignRevision }
          : {}),
      },
    }),
    onSuccess: async (result) => {
      message.success(`战役关系已解除：${result.target}`);
      await Promise.all([refetch(), campaignLinksQuery.refetch()]);
    },
    onError: (mutationError) => {
      message.error(alertDetailActionErrorMessage(mutationError, '解除战役关系失败，请刷新revision后重试'));
    },
  });
  const reportJobQuery = useQuery({
    queryKey: ['alert-report-job', alertId, businessActionResult?.jobId],
    queryFn: () => fetchAlertReportJob(alertId, businessActionResult?.jobId ?? ''),
    enabled: businessActionResult?.actionId === 'alert-report-export' && Boolean(businessActionResult.jobId),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && ['completed', 'partial', 'failed', 'cancelled', 'compensated', 'compensation_failed'].includes(status) ? false : 1_500;
    },
  });
  const activeBusinessActionResult = reportJobQuery.data ?? businessActionResult;
  const reportCancelMutation = useMutation({
    mutationFn: () => {
      if (!activeBusinessActionResult?.jobId || !activeBusinessActionResult.revision) {
        throw new Error('报告任务 revision 暂不可用，请刷新任务状态后重试');
      }
      return cancelAlertReportWithRevisionRefresh(
        alertId,
        activeBusinessActionResult.jobId,
        activeBusinessActionResult.revision,
        `用户在告警详情取消报告导出：${businessActionReason.trim()}`,
      );
    },
    onSuccess: async (result) => {
      if (result.status === 'cancelled') message.success('报告任务已取消');
      else if (result.status === 'cancel_requested') message.success('取消请求已受理，正在清理临时对象');
      else message.warning(`报告任务已先进入${reportJobStatusLabel(result.status)}，当前取消未执行`);
      await reportJobQuery.refetch();
    },
    onError: (mutationError) => {
      message.error(alertDetailActionErrorMessage(mutationError, '报告取消失败，请刷新 revision 后重试'));
    },
  });
  const reportCompensationMutation = useMutation({
    mutationFn: () => {
      if (!activeBusinessActionResult?.jobId || !activeBusinessActionResult.revision) {
        throw new Error('报告任务 revision 暂不可用，请刷新任务状态后重试');
      }
      return compensateAlertReport(
        alertId,
        activeBusinessActionResult.jobId,
        activeBusinessActionResult.revision,
        `用户确认重试报告对象清理补偿：${businessActionReason.trim()}`,
      );
    },
    onSuccess: async () => {
      message.success('对象清理补偿已受理，正在等待最终回执');
      await reportJobQuery.refetch();
    },
    onError: (mutationError) => {
      message.error(alertDetailActionErrorMessage(mutationError, '对象清理补偿失败，请刷新 revision 后重试'));
    },
  });
  const evidenceDownloadMutation = useMutation({
    mutationFn: async (row: AlertDetailEvidenceRow) => {
      const evidenceId = row.evidenceId || row.文件记录;
      const result = await submitAlertDetailAction({
        alertId: snapshot.alertId,
        actionId: 'alert-evidence-access',
        target: evidenceId,
        reason: `下载告警证据：${row.文件记录}`,
        detail: {
          access_mode: 'download',
          evidence_id: evidenceId,
          requested_file_name: row.文件记录,
          evidence_kind: row.evidenceKind || row.证据类型,
          signed_url_requested: true,
          source: 'alert-detail-inline-evidence',
        },
      });
      if (!result.downloadUrl) throw new Error('证据下载地址未返回');
      await downloadAlertEvidenceFile(result.downloadUrl, result.fileName || row.文件记录);
      return { result, row };
    },
    onSuccess: ({ result, row }) => {
      message.success(`证据已下载：${result.fileName || row.文件记录}`);
    },
    onError: (mutationError) => {
      message.error(alertDetailActionErrorMessage(mutationError, '证据下载失败，请稍后重试'));
    },
  });
  const closeBusinessAction = () => {
    setBusinessAction(undefined);
    setBusinessActionResult(undefined);
    businessActionMutation.reset();
  };
  const openBusinessAction = (
    actionId: AlertDetailActionId,
    label: string,
    target = actionId === 'alert-campaign-link' ? '' : snapshot.alertId,
    description?: string,
  ) => {
    businessActionMutation.reset();
    setBusinessActionResult(undefined);
    const nextAction = createAlertDetailAction(actionId, label, target, description);
    setBusinessAction(nextAction);
    setBusinessActionTarget(nextAction.id === 'alert-label-update' ? snapshot.tags.join('，') : nextAction.target);
    setBusinessActionReason(nextAction.description);
  };
  const activeEvidenceTab: EvidenceFocusTab = evidenceFilesFocusMode
    ? 'files'
    : evidencePcapFocusMode
      ? 'pcap'
      : evidenceSessionFocusMode
        ? 'session'
        : evidenceLogsFocusMode
          ? 'logs'
          : evidenceGraphPathFocusMode
            ? 'graph'
            : 'all';
  const visibleEvidenceRows = useMemo(() => {
    if (activeEvidenceTab === 'pcap') return snapshot.evidenceRows.filter(isPcapEvidence);
    if (activeEvidenceTab === 'session') return snapshot.evidenceRows.filter(isSessionEvidence);
    if (activeEvidenceTab === 'logs') return snapshot.evidenceRows.filter(isLogEvidence);
    if (activeEvidenceTab === 'graph') return snapshot.evidenceRows.filter(isGraphPathEvidence);
    if (activeEvidenceTab === 'files') return snapshot.evidenceRows.filter(isFileEvidence);
    return snapshot.evidenceRows;
  }, [activeEvidenceTab, snapshot.evidenceRows]);
  useEffect(() => {
    setEvidencePage(1);
  }, [activeEvidenceTab]);
  useEffect(() => {
    const lastPage = Math.max(1, Math.ceil(visibleEvidenceRows.length / ALERT_DETAIL_EVIDENCE_PAGE_SIZE));
    setEvidencePage((current) => Math.min(current, lastPage));
  }, [visibleEvidenceRows.length]);
  const downloadEvidence = (row: AlertDetailEvidenceRow) => {
    evidenceDownloadMutation.mutate(row);
  };
  const viewEvidence = (row: AlertDetailEvidenceRow) => {
    navigate(evidenceViewRoute(row, snapshot.alertId));
  };
  const evidenceColumns = buildEvidenceColumns(
    downloadEvidence,
    viewEvidence,
    evidenceDownloadMutation.isPending,
  );

  return (
    <div className="taf-page taf-alert-detail-page is-visual-target" data-route-id={route.id}>
      <header className="taf-alert-detail-titlebar">
        <div className="taf-alert-detail-titlebar__context">
          <h1>告警详情</h1>
        </div>
        <Space size={visualBreakdownMode ? 12 : 8} wrap>
          <Button
            className="taf-alert-detail-back-button"
            size="small"
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate(returnTo)}
          >
            返回告警列表
          </Button>
          <Button size="small" icon={<CloudDownloadOutlined />} title="导出报告" data-action-id="alert-report-export" onClick={() => openBusinessAction('alert-report-export', '导出报告')}>导出报告</Button>
          <Button size="small" icon={<CheckCircleOutlined />} title="标记为战役" data-action-id="alert-campaign-link" onClick={() => openBusinessAction('alert-campaign-link', '标记为战役')}>标记为战役</Button>
          <Button
            size="small"
            icon={<SafetyCertificateOutlined />}
            title="加入白名单"
            onClick={() => {
              setFeedbackResult('fp');
              setFeedbackAddToWhitelist(true);
              setFeedbackReason((current) => current || 'FALSE_ALARM');
            }}
          >
            加入白名单
          </Button>
          <Button size="small" icon={<MoreOutlined />} title="更多操作" data-action-id="alert-investigation-note" onClick={() => openBusinessAction('alert-investigation-note', '更多操作')}>更多操作</Button>
          <Tooltip title="刷新告警详情">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
          </Tooltip>
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/alerts/{id}、/v1/alerts/{id}/evidence、APISIX 路由或 alert-service。'}
          action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
        />
      )}

      <div className="taf-alert-detail-grid">
        <main className="taf-alert-detail-main">
          <WorkPanel title="研判摘要" className="taf-alert-detail-summary-panel" extra={<Button type="link" size="small" data-action-id="alert-label-update" onClick={() => openBusinessAction('alert-label-update', '编辑标签')}>编辑标签</Button>}>
            <div className="taf-alert-detail-summary">
              <div className="taf-alert-detail-score" title={snapshot.score === undefined ? `置信评分暂不可用，${snapshot.severity}` : `置信评分 ${snapshot.score} / 100，${snapshot.severity}`}>
                {snapshot.score === undefined
                  ? <div className="taf-alert-detail-score-unavailable" role="status">评分暂不可用</div>
                  : <RiskScoreRingChart value={snapshot.score} size={116} ariaLabel={`告警风险评分 ${snapshot.score} 分 ECharts 圆环图`} />}
                <strong>{snapshot.severity}</strong>
              </div>
              <div className="taf-alert-detail-facts">
                <SummaryFact label="告警 ID" value={snapshot.alertId} span={2} />
                <SummaryFact label="告警名称" value={snapshot.title} span={2} />
                <SummaryFact label="规则 / 模型" value={snapshot.ruleModel} span={2} />
                <SummaryFact label="攻击阶段" value={snapshot.attackPhase} span={2} />
                <SummaryFact label="严重级别" value={snapshot.severity} status />
                <SummaryFact label="置信度" value={snapshot.confidence} />
                <SummaryFact label="当前状态" value={snapshot.status} status span={2} />
                <SummaryFact label="责任人" value={snapshot.assignee} span={2} />
                <SummaryFact label="首次发生" value={snapshot.firstSeen} span={2} />
                <SummaryFact label="影响资产" value="2 台主机" span={2} />
                <SummaryFact label="业务系统" value={snapshot.businessSystem} span={2} />
                <SummaryFact label="处置建议" value={snapshot.recommendation} span={3} />
                <div className="taf-alert-detail-tags is-summary-tags">
                  {snapshot.tags.map((tag) => <span key={tag}>{tag}</span>)}
                </div>
              </div>
            </div>
          </WorkPanel>

          <div className="taf-alert-detail-midgrid">
            <WorkPanel title="资产上下文" className="taf-alert-detail-assets-panel">
              <div className="taf-alert-detail-assets">
                {snapshot.assets.map((asset, index) => (
                  <div key={asset.title} className="taf-alert-detail-asset-card">
                    <header>
                      <span>{asset.title}</span>
                      <StatusTag value={asset.role} />
                    </header>
                    <div className="taf-alert-detail-asset-identity">
                      <span className="taf-alert-detail-asset-identity-icon">
                        {index === 0 ? <DesktopOutlined /> : <GlobalOutlined />}
                      </span>
                      <strong title={index === 0 ? asset.hostname : asset.ip}>
                        {index === 0 ? asset.hostname : asset.ip}
                      </strong>
                      {index === 0 && <StatusTag value="在线" />}
                    </div>
                    <dl>
                      {assetFacts(asset).map((fact) => (
                        <Fragment key={`${asset.title}-${fact.label}`}>
                          <dt>{fact.label}</dt>
                          <dd title={fact.value}>
                            {fact.label === '最近风险画像'
                              ? <span className="taf-alert-detail-asset-risk-tags">{fact.value.split(/\s*[/／]\s*/).filter(Boolean).map((tag) => <em key={tag}>{tag}</em>)}</span>
                              : fact.value}
                          </dd>
                        </Fragment>
                      ))}
                    </dl>
                  </div>
                ))}
              </div>
            </WorkPanel>

            <WorkPanel title="时间线" className="taf-alert-detail-timeline-panel" extra={<Button type="link" size="small" data-action-id="alert-investigation-note" onClick={() => openBusinessAction('alert-investigation-note', '查看完整时间线')}>查看完整时间线</Button>}>
              <div className="taf-alert-detail-timeline">
                {snapshot.timeline.map((item) => (
                  <div key={`${item.time}-${item.title}`} className={`taf-alert-detail-timeline-item is-${item.status}`}>
                    <i />
                    <span>{item.time}</span>
                    <strong>{item.title}</strong>
                    <em>{item.description}</em>
                  </div>
                ))}
              </div>
            </WorkPanel>
          </div>

          <WorkPanel
            title={`证据链（${snapshot.evidenceRows.length}）`}
            className="taf-alert-detail-evidence-panel"
            extra={<EvidenceTabsHeader snapshot={snapshot} routeAlertId={alertId} active={activeEvidenceTab} />}
          >
            {snapshot.evidenceApiError && (
              <Alert
                type="warning"
                showIcon
                message="证据接口暂不可用"
                description={`${snapshot.evidenceApiError}；当前空表不代表该告警没有证据。`}
              />
            )}
            {activeEvidenceTab === 'files' ? (
              <AlertEvidenceFilesFocusView snapshot={snapshot} isLoading={isLoading} />
            ) : activeEvidenceTab === 'pcap' ? (
              <AlertEvidencePcapFocusView snapshot={snapshot} isLoading={isLoading} />
            ) : activeEvidenceTab === 'session' ? (
              <AlertEvidenceSessionFocusView snapshot={snapshot} isLoading={isLoading} />
            ) : activeEvidenceTab === 'logs' ? (
              <AlertEvidenceLogsFocusView snapshot={snapshot} isLoading={isLoading} />
            ) : activeEvidenceTab === 'graph' ? (
              <AlertEvidenceGraphPathFocusView snapshot={snapshot} isLoading={isLoading} />
            ) : (
              <Table
                rowKey={(row) => `${row.证据类型}-${row.文件记录}`}
                size="small"
                loading={isLoading}
                pagination={{
                  current: evidencePage,
                  pageSize: ALERT_DETAIL_EVIDENCE_PAGE_SIZE,
                  total: visibleEvidenceRows.length,
                  showSizeChanger: false,
                  hideOnSinglePage: false,
                  onChange: setEvidencePage,
                }}
                scroll={{ x: 920, y: 190 }}
                columns={evidenceColumns}
                dataSource={visibleEvidenceRows}
                locale={{
                  emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无证据链数据" />,
                }}
              />
            )}
          </WorkPanel>
        </main>

        <aside className="taf-alert-detail-rail">
          <WorkPanel title="关联战役" extra={<Link to="/campaigns">查看战役</Link>}>
            {campaignLinksQuery.isError && (
              <Alert
                type="warning"
                showIcon
                message="战役关系暂不可用"
                description={alertDetailActionErrorMessage(campaignLinksQuery.error, '请检查关系API或数据库迁移')}
              />
            )}
            {campaignLinksQuery.data?.partial && (
              <Alert
                type="warning"
                showIcon
                message="战役关系为部分快照"
                description={`待补齐：${campaignLinksQuery.data.missingSections.join('、') || '关系水位对账'}`}
              />
            )}
            {Boolean(campaignLinksQuery.data?.links.length) && !campaignLinksQuery.data?.unlinkAvailable && (
              <Alert
                type="info"
                showIcon
                message="解除关系尚未开放"
                description="CAMPAIGN_AGGREGATE_V2 仍处于默认关闭阶段；当前只读展示既有关系。"
              />
            )}
            <Space direction="vertical" size={6} style={{ width: '100%' }}>
              {(campaignLinksQuery.data?.links ?? []).map((link) => (
                <Space key={link.relationId} style={{ width: '100%', justifyContent: 'space-between' }}>
                  <span title={`关系revision ${link.revision}；战役revision ${link.currentCampaignRevision || '兼容模式'}`}>
                    <LinkOutlined /> {link.campaignId}
                  </span>
                  <Popconfirm
                    title="解除战役关系"
                    description="该操作会追加unlink历史并递增关系与战役revision。"
                    okText="确认解除"
                    cancelText="取消"
                    onConfirm={() => unlinkCampaignMutation.mutate(link)}
                  >
                    <Button
                      size="small"
                      danger
                      data-action-id="alert-campaign-unlink"
                      disabled={!campaignLinksQuery.data?.unlinkAvailable}
                      loading={unlinkCampaignMutation.isPending && unlinkCampaignMutation.variables?.relationId === link.relationId}
                    >
                      解除
                    </Button>
                  </Popconfirm>
                </Space>
              ))}
              {!campaignLinksQuery.isLoading && !campaignLinksQuery.isError && !(campaignLinksQuery.data?.links.length) && (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前告警未关联战役" />
              )}
            </Space>
          </WorkPanel>
          <WorkPanel title="攻击阶段轨迹" extra={<Link to="/attack-chains">查看攻击链</Link>}>
            <div className="taf-alert-detail-stage">
              {snapshot.stageTrail.map((item, index) => (
                <div key={item.title} className={`taf-alert-detail-stage-node is-${item.status}`}>
                  <i>{stageIcon(index)}</i>
                  <strong>{item.title}</strong>
                  <span>{item.time}</span>
                </div>
              ))}
            </div>
          </WorkPanel>

          <WorkPanel title="影响范围" extra={<Link to="/assets">查看资产图谱</Link>}>
            <div className="taf-alert-detail-impact">
              <div><strong>影响主机</strong><span>2</span></div>
              <div><strong>关联账户</strong><span>1</span></div>
              <div><strong>业务系统</strong><span>1</span></div>
              <div><strong>脆弱资产</strong><span>0</span></div>
              <div className="taf-alert-detail-path">
                <span className="taf-alert-detail-path-node is-risk">
                  <span className="taf-alert-detail-path-icon"><StopOutlined /></span>
                  <strong>源端主机</strong>
                  <em>{sourceAsset?.ip ?? '暂不可用'}</em>
                </span>
                <i />
                <span className="taf-alert-detail-path-node">
                  <span className="taf-alert-detail-path-icon"><DatabaseOutlined /></span>
                  <strong>核心区</strong>
                  <em>{sourceAsset?.business ?? '办公区'}</em>
                </span>
                <i />
                <span className="taf-alert-detail-path-node is-ok">
                  <span className="taf-alert-detail-path-icon"><SafetyCertificateOutlined /></span>
                  <strong>目的端</strong>
                  <em>{destinationAsset?.ip ?? '185.22.14.9'}</em>
                </span>
              </div>
            </div>
          </WorkPanel>

          <WorkPanel title="处置与响应" extra={<Link to="/playbooks">查看 SOAR 剧本</Link>}>
            <div className="taf-alert-detail-response">
              {snapshot.responseActions.map((action, index) => (
                <button
                  key={action.label}
                  type="button"
                  className={`is-${action.status}`}
                  data-action-id="alert-response-request"
                  onClick={() => openBusinessAction('alert-response-request', action.label, snapshot.alertId, `将为 ${snapshot.alertId} 创建“${action.label}”响应请求。`)}
                >
                  {responseIcon(index)}
                  <span>{action.label}</span>
                  <em>{action.risk}</em>
                </button>
              ))}
              <p>执行前请确认影响范围，所有操作将记录审计日志。</p>
            </div>
          </WorkPanel>

          <WorkPanel title="状态流转">
            <div className="taf-alert-detail-action-body">
              <dl>
                <dt>当前状态</dt><dd>{alertStatusLabel(snapshot.status)}</dd>
                <dt>状态版本</dt><dd>{snapshot.stateVersion}</dd>
              </dl>
              <Select
                size="small"
                value={targetStatus}
                placeholder={allowedNextStatuses.length ? '选择目标状态' : '当前无可用状态流转'}
                options={allowedNextStatuses.map((status) => ({ value: status, label: alertStatusLabel(status) }))}
                disabled={!allowedNextStatuses.length || statusMutation.isPending}
                onChange={setTargetStatus}
              />
              <Input.TextArea
                rows={2}
                value={statusReason}
                placeholder="填写状态变更原因，至少 4 个字符"
                disabled={statusMutation.isPending}
                onChange={(event) => setStatusReason(event.target.value)}
              />
              <Button
                size="small"
                type="primary"
                loading={statusMutation.isPending}
                disabled={!canSubmitStatusChange}
                onClick={() => statusMutation.mutate()}
              >
                提交状态变更
              </Button>
            </div>
          </WorkPanel>

          <WorkPanel title="反馈与学习" className="taf-alert-detail-feedback-panel">
            <div className="taf-alert-detail-feedback">
              {snapshot.feedbackApiError && (
                <Alert
                  type="warning"
                  showIcon
                  message="历史反馈暂不可用"
                  description={`${snapshot.feedbackApiError}；可继续填写，本次提交仍会单独校验。`}
                />
              )}
              <label>
                <span>判定结果</span>
                <Radio.Group value={feedbackResult} size="small" onChange={(event) => setFeedbackResult(event.target.value as FeedbackChoice)}>
                  <Radio value="tp">TP（真实告警）</Radio>
                  <Radio value="fp">FP（误报）</Radio>
                  <Radio value="pending">待确认</Radio>
                </Radio.Group>
              </label>
              <label>
                <span>误报原因</span>
                <Select
                  size="small"
                  value={feedbackReason}
                  disabled={feedbackResult !== 'fp'}
                  options={feedbackReasonOptions}
                  onChange={setFeedbackReason}
                />
              </label>
              <label>
                <span>白名单策略</span>
                <div className="taf-alert-detail-feedback-inline">
                  <Input size="small" value={feedbackAddToWhitelist ? whitelistPreview : ''} placeholder="请输入 IP / 域名 / 进程 / Hash" readOnly />
                  <Checkbox
                    checked={feedbackAddToWhitelist}
                    disabled={feedbackResult !== 'fp'}
                    onChange={(event) => setFeedbackAddToWhitelist(event.target.checked)}
                  >
                    加入白名单
                  </Checkbox>
                </div>
              </label>
              <label className="taf-alert-detail-feedback-check">
                <span>样本回流</span>
                <Checkbox checked disabled>
                  {snapshot.feedback.sampleReturn}
                </Checkbox>
              </label>
              <label className="taf-alert-detail-feedback-comment">
                <span>备注</span>
                <Input.TextArea value={feedbackComment} placeholder="请输入分析备注..." rows={2} onChange={(event) => setFeedbackComment(event.target.value)} />
              </label>
              <div className="taf-alert-detail-feedback-actions">
                <Button
                  type="primary"
                  icon={<SendOutlined />}
                  disabled={!canSubmitFeedback}
                  loading={feedbackMutation.isPending}
                  onClick={() => feedbackMutation.mutate()}
                >
                  提交反馈
                </Button>
                {feedbackResult === 'pending' && (
                  <span className="taf-alert-detail-feedback-pending-hint">待确认是未决状态，请选择 TP 或 FP 后提交</span>
                )}
                {lastWhitelistDraftUrl && (
                  <Button size="small" icon={<LinkOutlined />} onClick={() => navigate(lastWhitelistDraftUrl)}>
                    查看草案
                  </Button>
                )}
              </div>
            </div>
          </WorkPanel>

        </aside>
      </div>
      <Modal
        className="taf-alert-detail-action-drawer"
        title={businessAction ? `${businessAction.label}确认` : '告警业务操作'}
        open={Boolean(businessAction)}
        centered
        width={520}
        onCancel={closeBusinessAction}
        footer={(
          <Space>
            <Button
              size="small"
              onClick={closeBusinessAction}
            >
              取消
            </Button>
            <Button
              size="small"
              type="primary"
              loading={businessActionMutation.isPending}
              disabled={
                Boolean(businessActionResult)
                || !businessActionTarget.trim()
                || businessActionReason.trim().length < 4
                || (businessAction?.id === 'alert-report-export'
                  && (!Number.isSafeInteger(snapshot.stateVersion) || Number(snapshot.stateVersion) < 1))
              }
              onClick={() => {
                if (businessAction) businessActionMutation.mutate({
                  alertId,
                  actionId: businessAction.id,
                  target: businessActionTarget,
                  reason: businessActionReason,
                  detail: {
                    previous_target: businessAction.target,
                    ...(Number.isSafeInteger(snapshot.stateVersion) && Number(snapshot.stateVersion) > 0
                      ? { snapshotId: `alert:${alertId}:revision:${snapshot.stateVersion}` }
                      : {}),
                    format: businessAction.id === 'alert-report-export' ? 'pdf' : undefined,
                    labels: businessAction.id === 'alert-label-update'
                      ? businessActionTarget.split(/[,，]/).map((item) => item.trim()).filter(Boolean)
                      : undefined,
                  },
                });
              }}
            >
              {businessActionResult ? '已生成任务' : '确认提交'}
            </Button>
          </Space>
        )}
      >
        <div className="taf-alert-detail-action-body">
          <p>{businessAction?.description}</p>
          <label>
            <span>{businessAction?.id === 'alert-label-update' ? '标签内容' : businessAction?.id === 'alert-campaign-link' ? '战役ID' : '操作目标'}</span>
            <Input
              value={businessActionTarget}
              placeholder={businessAction?.id === 'alert-label-update' ? '多个标签使用逗号分隔' : businessAction?.id === 'alert-campaign-link' ? '输入同租户战役ID，例如 CAM-20260730-001' : '输入本次操作目标'}
              onChange={(event) => setBusinessActionTarget(event.target.value)}
            />
          </label>
          <label>
            <span>操作原因</span>
            <Input.TextArea
              rows={3}
              value={businessActionReason}
              placeholder="填写操作原因，至少 4 个字符；提交后写入审计记录"
              onChange={(event) => setBusinessActionReason(event.target.value)}
            />
          </label>
          <dl>
            <dt>告警对象</dt><dd>{alertId}</dd>
            <dt>操作目标</dt><dd>{businessActionTarget || '-'}</dd>
            <dt>接口契约</dt><dd>已在 alert-detail 页面 API 计划中注册</dd>
            <dt>执行状态</dt><dd>{activeBusinessActionResult ? alertActionStatusLabels[activeBusinessActionResult.status] : '尚未提交'}</dd>
          </dl>
          {activeBusinessActionResult && (
            <Alert
              type={['failed', 'compensation_failed'].includes(activeBusinessActionResult.status) ? 'error' : activeBusinessActionResult.status === 'partial' ? 'warning' : 'success'}
              showIcon
              message={`任务 ${activeBusinessActionResult.jobId}：${alertActionStatusLabels[activeBusinessActionResult.status]}`}
              description={`${activeBusinessActionResult.auditEvent}；${activeBusinessActionResult.apiContract}`}
            />
          )}
          {activeBusinessActionResult?.status === 'completed' && activeBusinessActionResult.downloadUrl && (
            <Button
              size="small"
              type="primary"
              icon={<CloudDownloadOutlined />}
              onClick={() => void downloadAlertEvidenceFile(
                activeBusinessActionResult.downloadUrl ?? '',
                activeBusinessActionResult.fileName ?? `alert-${alertId}.pdf`,
              )}
            >
              下载并校验报告
            </Button>
          )}
          {activeBusinessActionResult?.actionId === 'alert-report-export'
            && ['accepted', 'running'].includes(activeBusinessActionResult.status) && (
            <Popconfirm
              title="取消报告导出"
              description="运行中的任务会先进入取消中；临时对象清理成功后才会显示已取消。"
              okText="确认取消"
              cancelText="继续执行"
              onConfirm={() => reportCancelMutation.mutate()}
            >
              <Button size="small" danger loading={reportCancelMutation.isPending} data-action-id="alert-report-cancel">
                取消报告任务
              </Button>
            </Popconfirm>
          )}
          {activeBusinessActionResult?.actionId === 'alert-report-export'
            && ['partial', 'compensation_failed'].includes(activeBusinessActionResult.status) && (
            <Popconfirm
              title="重试对象清理补偿"
              description="仅删除该任务 manifest 绑定的精确对象；收到删除回执后才会显示已补偿。"
              okText="确认补偿"
              cancelText="暂不处理"
              onConfirm={() => reportCompensationMutation.mutate()}
            >
              <Button size="small" danger loading={reportCompensationMutation.isPending} data-action-id="alert-report-compensate">
                重试清理补偿
              </Button>
            </Popconfirm>
          )}
        </div>
      </Modal>
    </div>
  );
}

type EvidenceFocusTab = 'all' | 'pcap' | 'session' | 'logs' | 'graph' | 'files';

function EvidenceTabsHeader({
  snapshot,
  routeAlertId,
  active,
}: {
  snapshot: AlertDetailSnapshot;
  routeAlertId: string;
  active: EvidenceFocusTab;
}) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const tabs: Array<{ id: EvidenceFocusTab; label: string; count: number; target: string }> = [
    { id: 'all', label: '全部', count: counts.all, target: '全部证据' },
    { id: 'pcap', label: 'PCAP', count: counts.pcap, target: 'PCAP 证据' },
    { id: 'session', label: 'Session', count: counts.session, target: 'Session 证据' },
    { id: 'logs', label: '日志', count: counts.log, target: '日志证据' },
    { id: 'graph', label: '图谱路径', count: counts.graph, target: '图谱路径证据' },
    { id: 'files', label: '文件', count: counts.files, target: '文件证据' },
  ];
  return (
    <div className="taf-alert-detail-evidence-tabs" role="tablist" aria-label="证据链分类">
      {tabs.map((tab) => (
        <EvidenceFocusAction
          key={tab.id}
          alertId={routeAlertId}
          title={`${tab.label} ${tab.count}`}
          target={tab.target}
          className={active === tab.id ? 'is-active' : undefined}
          ariaPressed={active === tab.id}
        >
          {tab.label} <strong>{tab.count}</strong>
        </EvidenceFocusAction>
      ))}
    </div>
  );
}

function AlertEvidenceFilesFocusView({ snapshot, isLoading }: { snapshot: AlertDetailSnapshot; isLoading: boolean }) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const fileRow = snapshot.evidenceRows.find((row) => isFileEvidence(row)) ?? snapshot.evidenceRows[snapshot.evidenceRows.length - 1];
  const filename = fileRow?.文件记录 || 'hash-1a2b3c4d5bef79a8h9i0j.txt';
  const hashValue = fileRow?.hashValue || 'SHA256: 1a2b3c4d5bef79a8h9i0j...';
  const signedUrl = fileRow?.signedUrl || `https://evidence.campus.local/signed/${snapshot.alertId}`;
  const generatedAt = compactDateTime(fileRow?.生成时间) || '06-20 03:43:04';
  const tags = fileRow?.fileTags?.length ? fileRow.fileTags : ['报告附件', '导出脚本', 'hash 校验', '下载审计 sec_analyst 03:45'];
  const copySignedUrl = async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard api unavailable');
      await navigator.clipboard.writeText(signedUrl);
      message.success('签名 URL 已复制');
    } catch {
      const textarea = document.createElement('textarea');
      textarea.value = signedUrl;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      const copied = document.execCommand('copy');
      textarea.remove();
      if (copied) message.success('签名 URL 已复制');
      else message.error('签名 URL 复制失败，请检查浏览器剪贴板权限');
    }
  };

  return (
    <section className="taf-alert-evidence-files-focus" data-page-id="alert-detail-evidence-files" aria-label="告警详情证据链文件">
      <div className="taf-alert-evidence-files-card">
        <div className="taf-alert-evidence-files-table" aria-busy={isLoading}>
          <div className="taf-alert-evidence-files-head" role="row">
            <span>证据类型</span>
            <span>文件名</span>
            <span>类型</span>
            <span>hash / 签名 URL</span>
            <span>大小</span>
            <span>生成时间</span>
            <span>校验状态</span>
            <span>操作</span>
          </div>

          <div className="taf-alert-evidence-files-row" role="row">
            <div className="taf-alert-evidence-files-type" title="文件">
              <FileTextOutlined />
              <strong>文件</strong>
            </div>
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-files-name" title={`查看证据文件：${filename}`} target={filename}>{filename}</EvidenceFocusAction>
            <span className="taf-alert-evidence-files-kind" title={fileRow?.evidenceKind || 'hash 清单 / 附件'}>{fileRow?.evidenceKind || 'hash 清单 / 附件'}</span>
            <div className="taf-alert-evidence-files-hash" title={`${hashValue}；signed-url 可用`}>
              <span><CodeOutlined />{hashValue}</span>
              <em><LinkOutlined />signed-url 可用</em>
            </div>
            <span title={fileRow?.大小 || '64 B'}>{fileRow?.大小 || '64 B'}</span>
            <span title={generatedAt}>{generatedAt}</span>
            <span className="taf-alert-evidence-files-status" title="已计算 / 可访问"><CheckCircleOutlined />已计算 / 可访问</span>
            <div className="taf-alert-evidence-files-actions" aria-label="文件操作">
              <EvidenceFocusAction alertId={snapshot.alertId} title="下载证据文件" target={filename} ariaLabel="下载证据文件"><CloudDownloadOutlined /></EvidenceFocusAction>
              <EvidenceFocusAction alertId={snapshot.alertId} title="查看证据文件" target={filename} ariaLabel="查看证据文件"><EyeOutlined /></EvidenceFocusAction>
            </div>
          </div>

          <div className="taf-alert-evidence-files-tags">
            <span>文件标签</span>
            {tags.map((tag: string, index: number) => (
              <EvidenceFocusAction key={`${tag}-${index}`} alertId={snapshot.alertId} title={`查看文件标签：${tag}`} target={filename}>
                {index === 0 ? <PaperClipOutlined /> : index === 1 ? <CodeOutlined /> : index === 2 ? <SafetyCertificateOutlined /> : <CloudDownloadOutlined />}
                {tag}
              </EvidenceFocusAction>
            ))}
            <button
              type="button"
              className="taf-alert-evidence-files-signed-url"
              title="复制签名 URL"
              aria-label="复制签名 URL"
              onClick={copySignedUrl}
            >
              <b>签名 URL 预览</b>
              <span>{signedUrl}</span>
              <CopyOutlined />
            </button>
          </div>

          <footer className="taf-alert-evidence-files-footer">
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" title={`查看全部 文件 ${counts.files} 项`} target="文件证据列表">查看全部 文件 {counts.files} 项 <ArrowLeftOutlined /></EvidenceFocusAction>
          </footer>
        </div>
      </div>
    </section>
  );
}

function AlertEvidencePcapFocusView({ snapshot, isLoading }: { snapshot: AlertDetailSnapshot; isLoading: boolean }) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const pcapRow = snapshot.evidenceRows.find((row) => isPcapEvidence(row)) ?? snapshot.evidenceRows[0];
  const pcap = pcapRow?.pcapEvidence ?? unavailablePcapEvidence();
  const generatedAt = compactDateTime(pcap.generatedAt || pcapRow?.生成时间) || '暂不可用';
  const statusLines = pcap.statusLines.length ? pcap.statusLines : ['暂不可用'];
  const summaryText = pcap.contentSummary || '暂不可用';
  const objectPath = pcap.objectPath || '暂不可用';

  return (
    <section className="taf-alert-evidence-pcap-focus" data-page-id="alert-detail-evidence-pcap" aria-label="告警详情证据链 PCAP">
      <div className="taf-alert-evidence-pcap-card">
        <div className="taf-alert-evidence-pcap-table" aria-busy={isLoading}>
          <div className="taf-alert-evidence-pcap-head" role="row">
            <span>证据类型</span>
            <span>文件 / 记录</span>
            <span>内容摘要</span>
            <span>大小</span>
            <span>生成时间</span>
            <span>校验状态</span>
            <span>下载审计</span>
            <span>操作</span>
          </div>

          <div className="taf-alert-evidence-pcap-row" role="row">
            <div className="taf-alert-evidence-pcap-type" title="PCAP">
              <span className="taf-alert-evidence-pcap-pulse" aria-hidden="true"><ApiOutlined /></span>
              <strong>PCAP</strong>
            </div>
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-pcap-file" title={`查看 PCAP：${pcap.fileName}`} target={pcap.fileName}>{pcap.fileName}</EvidenceFocusAction>
            <div className="taf-alert-evidence-pcap-summary" title={summaryText}>
              <span>{summaryText}</span>
            </div>
            <span title={pcap.size}>{pcap.size}</span>
            <span title={generatedAt}>{generatedAt}</span>
            <span className="taf-alert-evidence-pcap-status" title={statusLines.join(' ')}>
              <CheckCircleOutlined />
              <em>{statusLines[0] ?? '暂不可用'}</em>
              <b>{statusLines[1] ?? ''}</b>
            </span>
            <span className="taf-alert-evidence-pcap-audit" title={pcap.downloadAudit}>{pcap.downloadAudit}</span>
            <div className="taf-alert-evidence-pcap-actions" aria-label="PCAP 操作">
              <EvidenceFocusAction alertId={snapshot.alertId} title="下载 PCAP" target={pcap.fileName} ariaLabel="下载 PCAP"><CloudDownloadOutlined /></EvidenceFocusAction>
              <EvidenceFocusAction alertId={snapshot.alertId} title="查看 PCAP" target={pcap.fileName} ariaLabel="查看 PCAP"><EyeOutlined /></EvidenceFocusAction>
            </div>
          </div>

          <div className="taf-alert-evidence-pcap-detail">
            <label className="taf-alert-evidence-pcap-path-label" title="对象路径">
              <FileTextOutlined />
              <span>对象路径</span>
            </label>
            <div className="taf-alert-evidence-pcap-path" title={objectPath}>
              <span>{objectPath}</span>
              <CopyOutlined />
            </div>
            <label className="taf-alert-evidence-pcap-sha-label" title="SHA256">
              <b>#</b>
              <span>SHA256</span>
            </label>
            <div className="taf-alert-evidence-pcap-sha" title={pcap.sha256}>
              <span>{pcap.sha256}</span>
              <CopyOutlined />
            </div>
          </div>

          <footer className="taf-alert-evidence-pcap-footer">
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" title={`查看全部 PCAP ${counts.pcap} 项`} target="PCAP 证据列表">查看全部 PCAP {counts.pcap} 项 <ArrowLeftOutlined /></EvidenceFocusAction>
          </footer>
        </div>
      </div>
    </section>
  );
}

function AlertEvidenceSessionFocusView({ snapshot, isLoading }: { snapshot: AlertDetailSnapshot; isLoading: boolean }) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const sessionRows = snapshot.evidenceRows.filter((row) => isSessionEvidence(row));
  const sessions = sessionRows.map((row) => row.sessionEvidence ?? unavailableSessionEvidence());
  if (!sessions.length) sessions.push(unavailableSessionEvidence());
  const visibleSessions = sessions.slice(0, 2);
  const timeline = visibleSessions.find((session) => session.timeline.length)?.timeline ?? [];
  const linkedPcap = visibleSessions.find((session) => session.linkedPcap)?.linkedPcap;

  return (
    <section className="taf-alert-evidence-session-focus" data-page-id="alert-detail-evidence-session" aria-label="告警详情证据链 Session">
      <div className="taf-alert-evidence-session-card">
        <div className="taf-alert-evidence-session-table" aria-busy={isLoading}>
          <div className="taf-alert-evidence-session-head" role="row">
            <span>证据类型</span>
            <span>Session ID</span>
            <span>五元组</span>
            <span>请求/响应摘要</span>
            <span>字节数</span>
            <span>持续时间</span>
            <span>状态</span>
            <span>操作</span>
          </div>

          {visibleSessions.map((session, index) => (
            <div className="taf-alert-evidence-session-row" role="row" key={`${session.sessionId}-${index}`}>
              <div className="taf-alert-evidence-session-type" title="Session">
                <span className="taf-alert-evidence-session-shield" aria-hidden="true"><SafetyCertificateOutlined /></span>
                <strong>Session</strong>
              </div>
              <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-session-id" title={`查看 Session：${session.sessionId}`} target={session.sessionId}>{session.sessionId}</EvidenceFocusAction>
              <div className="taf-alert-evidence-session-tuple" title={session.tupleLines.join(' ')}>
                {session.tupleLines.map((line) => <span key={line}>{line}</span>)}
              </div>
              <div className="taf-alert-evidence-session-summary" title={session.summaryLines.join(' ')}>
                {session.summaryLines.map((line) => <span key={line}>{line}</span>)}
              </div>
              <span title={session.bytes}>{session.bytes}</span>
              <span title={session.duration}>{session.duration}</span>
              <span className="taf-alert-evidence-session-status" title={session.status}>{session.status}</span>
              <div className="taf-alert-evidence-session-actions" aria-label={`${session.sessionId} 操作`}>
                <EvidenceFocusAction alertId={snapshot.alertId} title={session.actionKind === 'file' ? '打开 Session 文件' : '重新关联 Session'} target={session.sessionId} ariaLabel={session.actionKind === 'file' ? '打开 Session 文件' : '重新关联 Session'}>
                  {session.actionKind === 'file' ? <FileTextOutlined /> : <ReloadOutlined />}
                </EvidenceFocusAction>
                <EvidenceFocusAction alertId={snapshot.alertId} title="查看 Session" target={session.sessionId} ariaLabel="查看 Session"><EyeOutlined /></EvidenceFocusAction>
              </div>
            </div>
          ))}

          <div className="taf-alert-evidence-session-flow" aria-label="Session 事件链">
            {timeline.map((event) => (
              <span key={`${event.time}-${event.label}`} className="taf-alert-evidence-session-event" title={`${event.time} ${event.label}`}>
                <i aria-hidden="true" />
                <b>{event.time}</b>
                <em>{event.label}</em>
              </span>
            ))}
            {!timeline.length && <span className="taf-alert-evidence-session-event"><em>Session 事件链暂不可用</em></span>}
            {linkedPcap
              ? (
                  <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-session-linked-pcap" title={`关联 PCAP: ${linkedPcap}`} target={linkedPcap}>
                    <LinkOutlined />
                    <span>关联 PCAP: </span>
                    <strong>{linkedPcap}</strong>
                  </EvidenceFocusAction>
                )
              : <span className="taf-alert-evidence-session-linked-pcap">关联 PCAP 暂不可用</span>}
          </div>

          <footer className="taf-alert-evidence-session-footer">
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" title={`查看全部 Session ${counts.session} 项`} target="Session 证据列表">查看全部 Session {counts.session} 项 <ArrowLeftOutlined /></EvidenceFocusAction>
          </footer>
        </div>
      </div>
    </section>
  );
}

function AlertEvidenceLogsFocusView({ snapshot, isLoading }: { snapshot: AlertDetailSnapshot; isLoading: boolean }) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const logRow = snapshot.evidenceRows.find((row) => isLogEvidence(row)) ?? snapshot.evidenceRows.find((row) => row.logEvidence);
  const log = logRow?.logEvidence ?? unavailableLogEvidence();
  const generatedAt = compactDateTime(log.generatedAt || logRow?.生成时间) || '06-20 03:43:05';
  const hitFieldText = log.hitFields.join('\n');

  return (
    <section className="taf-alert-evidence-logs-focus" data-page-id="alert-detail-evidence-logs" aria-label="告警详情证据链日志">
      <div className="taf-alert-evidence-logs-card">
        <div className="taf-alert-evidence-logs-table" aria-busy={isLoading}>
          <div className="taf-alert-evidence-logs-head" role="row">
            <span>证据类型</span>
            <span>日志文件</span>
            <span>来源</span>
            <span>命中字段</span>
            <span>内容摘要</span>
            <span>生成时间</span>
            <span>状态</span>
            <span>操作</span>
          </div>

          <div className="taf-alert-evidence-logs-row" role="row">
            <div className="taf-alert-evidence-logs-type" title="日志">
              <FileTextOutlined />
              <strong>日志</strong>
            </div>
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-logs-file" title={`查看日志：${log.logFile}`} target={log.logFile}>{log.logFile}</EvidenceFocusAction>
            <span className="taf-alert-evidence-logs-source" title={log.source}>{log.source}</span>
            <div className="taf-alert-evidence-logs-hit" title={hitFieldText}>
              {log.hitFields.map((field) => <span key={field}>{field}</span>)}
            </div>
            <div className="taf-alert-evidence-logs-summary" title={log.contentSummary}>
              {log.contentSummary.split('，').map((line) => <span key={line}>{line}</span>)}
            </div>
            <span title={generatedAt}>{generatedAt}</span>
            <span className="taf-alert-evidence-logs-status" title={log.status}>{log.status}</span>
            <div className="taf-alert-evidence-logs-actions" aria-label="日志操作">
              <EvidenceFocusAction alertId={snapshot.alertId} title="检索日志" target={log.logFile} ariaLabel="检索日志"><SearchOutlined /></EvidenceFocusAction>
              <EvidenceFocusAction alertId={snapshot.alertId} title="查看日志" target={log.logFile} ariaLabel="查看日志"><EyeOutlined /></EvidenceFocusAction>
            </div>
          </div>

          <div className="taf-alert-evidence-logs-detail">
            <section className="taf-alert-evidence-logs-fields" aria-label="关键字段高亮">
              <h2>关键字段（高亮）</h2>
              <div>
                {log.highlightedFields.map((field) => (
                  <span key={field.key} title={`${field.key}=${field.value}`}>
                    <b>{field.key}=</b><em>{field.value}</em>
                  </span>
                ))}
              </div>
            </section>
            <section className="taf-alert-evidence-logs-tags" aria-label="来源标签">
              <h2>来源标签</h2>
              <div>
                {log.sourceTags.map((tag) => (
                  <EvidenceFocusAction key={tag.label} alertId={snapshot.alertId} className={`is-${tag.kind}`} title={`查看来源标签：${tag.label}`} target={log.logFile}>
                    {logTagIcon(tag.kind)}
                    {tag.label}
                  </EvidenceFocusAction>
                ))}
              </div>
            </section>
          </div>

          <footer className="taf-alert-evidence-logs-footer">
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" title={`查看全部 日志 ${counts.log} 项`} target="日志证据列表">查看全部 日志 {counts.log} 项 <ArrowLeftOutlined /></EvidenceFocusAction>
          </footer>
        </div>
      </div>
    </section>
  );
}

function AlertEvidenceGraphPathFocusView({ snapshot, isLoading }: { snapshot: AlertDetailSnapshot; isLoading: boolean }) {
  const counts = evidenceBucketCounts(snapshot.evidenceRows);
  const graphRow = snapshot.evidenceRows.find((row) => isGraphPathEvidence(row)) ?? snapshot.evidenceRows.find((row) => row.graphPath);
  const graph = graphRow?.graphPath ?? unavailableGraphPathEvidence();
  const generatedAt = compactDateTime(graph.generatedAt || graphRow?.生成时间) || '暂不可用';
  const summaryLines = graph.pathSummary.split(/\n|；|;/).map((item) => item.trim()).filter(Boolean);
  const resources = graph.resources;

  return (
    <section className="taf-alert-evidence-graph-focus" data-page-id="alert-detail-evidence-graph-path" aria-label="告警详情证据链图谱路径">
      <div className="taf-alert-evidence-graph-card">
        <div className="taf-alert-evidence-graph-table" aria-busy={isLoading}>
          <div className="taf-alert-evidence-graph-head" role="row">
            <span>证据类型</span>
            <span>路径文件</span>
            <span>路径摘要</span>
            <span>边权重</span>
            <span>关联实体</span>
            <span>生成时间</span>
            <span>状态</span>
            <span>操作</span>
          </div>

          <div className="taf-alert-evidence-graph-row" role="row">
            <div className="taf-alert-evidence-graph-type" title="图谱路径">
              <NodeIndexOutlined />
              <strong>图谱路径</strong>
            </div>
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" className="taf-alert-evidence-graph-file" title={`查看图谱路径：${graph.pathFile}`} target={graph.pathFile}>{graph.pathFile}</EvidenceFocusAction>
            <div className="taf-alert-evidence-graph-summary" title={graph.pathSummary}>
              <span>{summaryLines[0] ?? '路径摘要暂不可用'}</span>
              <em>{summaryLines[1] ?? '路径关系暂不可用'}</em>
            </div>
            <span className="taf-alert-evidence-graph-weight" title={`${graph.edgeWeight} / ${graph.relationType}`}>
              {graph.edgeWeight} / {graph.relationType}
            </span>
            <div className="taf-alert-evidence-graph-entities" title={graph.relatedEntities.join('，')}>
              {graph.relatedEntities.map((entity) => <span key={entity}>{entity}</span>)}
            </div>
            <span title={generatedAt}>{generatedAt}</span>
            <span className="taf-alert-evidence-graph-status" title={graph.status}>{graph.status}</span>
            <div className="taf-alert-evidence-graph-actions" aria-label="图谱路径操作">
              <EvidenceFocusAction alertId={snapshot.alertId} title="打开路径图谱" target={graph.pathFile} ariaLabel="打开路径图谱"><NodeIndexOutlined /></EvidenceFocusAction>
              <EvidenceFocusAction alertId={snapshot.alertId} title="查看路径证据" target={graph.pathFile} ariaLabel="查看路径证据"><EyeOutlined /></EvidenceFocusAction>
            </div>
          </div>

          <div className="taf-alert-evidence-graph-detail">
            <section className="taf-alert-evidence-graph-map" aria-label="路径关系图">
              <h2>路径关系图 <InfoCircleOutlined /></h2>
              <div className="taf-alert-evidence-graph-map-canvas">
                <GraphPathEdges edges={graph.edges} />
                {graph.nodes.map((node, index) => (
                  <div key={node.id} className={`taf-alert-evidence-graph-node is-${node.kind}`} style={{ '--node-index': index } as CSSProperties} title={`${node.label} ${node.value}`}>
                    <i>{graphNodeIcon(node.kind)}</i>
                    <strong>{node.label}</strong>
                    <span>{node.value}</span>
                  </div>
                ))}
              </div>
            </section>
            <aside className="taf-alert-evidence-graph-stats" aria-label="路径统计">
              <h2>路径统计</h2>
              <dl>
                <dt>节点数：</dt><dd>{graph.nodes.length}</dd>
                <dt>边数：</dt><dd>{graph.edges.length}</dd>
                <dt>平均边权重：</dt><dd>{graph.edgeWeight}</dd>
                <dt>风险评分：</dt><dd className={graph.riskScore === undefined ? undefined : 'is-risk'}>{graph.riskScore === undefined ? '暂不可用' : `${graph.riskScore}（高风险）`}</dd>
              </dl>
            </aside>
          </div>

          <div className="taf-alert-evidence-graph-resources">
            <span>关联资源</span>
            {resources.map((resource) => (
              <EvidenceFocusAction key={resource} alertId={snapshot.alertId} title={`查看关联资源：${resource}`} target={resource}>
                {resource.startsWith('PCAP') ? <PaperClipOutlined /> : <LinkOutlined />}
                {resource}
              </EvidenceFocusAction>
            ))}
          </div>

          <footer className="taf-alert-evidence-graph-footer">
            <EvidenceFocusAction alertId={snapshot.alertId} as="link" title={`查看全部 图谱路径 ${counts.graph} 项`} target="图谱路径证据列表">查看全部 图谱路径 {counts.graph} 项 <ArrowLeftOutlined /></EvidenceFocusAction>
          </footer>
        </div>
      </div>
    </section>
  );
}

function GraphPathEdges({ edges }: { edges: NonNullable<AlertDetailEvidenceRow['graphPath']>['edges'] }) {
  const labels = edges.map((edge) => edge.label);
  return (
    <svg className="taf-alert-evidence-graph-edges" viewBox="0 0 1000 210" role="img" aria-label="图谱路径边关系">
      {[0, 1, 2].map((index) => (
        <g key={`edge-${index}`}>
          <line x1={220 + index * 250} y1="82" x2={386 + index * 250} y2="82" />
          <polygon points={`${386 + index * 250},72 ${408 + index * 250},82 ${386 + index * 250},92`} />
          <rect x={270 + index * 250} y="16" width="74" height="48" rx="4" />
          <text x={307 + index * 250} y="48">{labels[index] ?? ['通信', '登录', '访问'][index]}</text>
        </g>
      ))}
    </svg>
  );
}

function SummaryFact({
  label,
  value,
  status = false,
  span = 1,
}: {
  label: string;
  value: string;
  status?: boolean;
  span?: 1 | 2 | 3;
}) {
  return (
    <div className={`taf-alert-detail-summary-fact is-span-${span}`}>
      <span>{label}</span>
      {status ? <StatusTag value={value} /> : <strong title={value}>{value}</strong>}
    </div>
  );
}

function assetFacts(asset: AlertDetailSnapshot['assets'][number]) {
  return asset.facts?.length
    ? asset.facts
    : [
        { label: 'IP 地址', value: asset.ip },
        { label: '主机 / 组织', value: asset.hostname },
        { label: '服务', value: asset.service },
        { label: '业务系统', value: asset.business },
        { label: '最近风险画像', value: asset.risk },
      ];
}

function stageIcon(index: number) {
  const icons = [<SafetyCertificateOutlined key="shield" />, <CheckCircleOutlined key="check" />, <ClusterOutlined key="cluster" />, <StopOutlined key="stop" />, <UserSwitchOutlined key="user" />];
  return icons[index] ?? <NodeIndexOutlined />;
}

function responseIcon(index: number) {
  const icons = [<StopOutlined key="isolate" />, <BlockOutlined key="block" />, <UserSwitchOutlined key="user" />, <ApiOutlined key="script" />, <AuditOutlined key="ticket" />];
  return icons[index] ?? <LinkOutlined />;
}

function evidenceBucketCounts(rows: AlertDetailEvidenceRow[]) {
  const count = (predicate: (row: AlertDetailEvidenceRow) => boolean) => rows.filter(predicate).length;
  return {
    all: rows.length,
    pcap: count((row) => row.证据类型.toLowerCase().includes('pcap')),
    session: count((row) => row.证据类型.toLowerCase().includes('session')),
    log: count((row) => row.证据类型.includes('日志') || row.证据类型.toLowerCase().includes('log')),
    graph: count((row) => row.证据类型.includes('图谱') || row.证据类型.toLowerCase().includes('graph')),
    files: count(isFileEvidence),
  };
}

function isFileEvidence(row: AlertDetailEvidenceRow) {
  const text = `${row.证据类型} ${row.文件记录} ${row.evidenceKind ?? ''}`.toLowerCase();
  return text.includes('文件') || text.includes('hash') || text.includes('sign') || text.includes('url');
}

function isPcapEvidence(row: AlertDetailEvidenceRow) {
  const text = `${row.证据类型} ${row.文件记录} ${row.evidenceKind ?? ''}`.toLowerCase();
  return Boolean(row.pcapEvidence) || text.includes('pcap');
}

function isSessionEvidence(row: AlertDetailEvidenceRow) {
  const text = `${row.证据类型} ${row.文件记录} ${row.evidenceKind ?? ''}`.toLowerCase();
  return Boolean(row.sessionEvidence) || text.includes('session');
}

function isLogEvidence(row: AlertDetailEvidenceRow) {
  const text = `${row.证据类型} ${row.文件记录} ${row.evidenceKind ?? ''}`.toLowerCase();
  return Boolean(row.logEvidence) || row.证据类型.includes('日志') || text.includes('log');
}

function isGraphPathEvidence(row: AlertDetailEvidenceRow) {
  const text = `${row.证据类型} ${row.文件记录} ${row.evidenceKind ?? ''}`.toLowerCase();
  return Boolean(row.graphPath) || text.includes('图谱') || text.includes('graph') || text.includes('path');
}

function unavailablePcapEvidence(): NonNullable<AlertDetailEvidenceRow['pcapEvidence']> {
  return {
    fileName: '暂不可用',
    contentSummary: 'PCAP 证据暂不可用',
    size: '暂不可用',
    generatedAt: '暂不可用',
    statusLines: ['暂不可用'],
    downloadAudit: '暂不可用',
    objectPath: '',
    sha256: '',
  };
}

function unavailableSessionEvidence(): NonNullable<AlertDetailEvidenceRow['sessionEvidence']> {
  return {
    sessionId: '暂不可用',
    tupleLines: ['会话五元组暂不可用'],
    summaryLines: ['Session 证据暂不可用'],
    bytes: '暂不可用',
    duration: '暂不可用',
    status: '暂不可用',
    actionKind: 'file',
    timeline: [],
    linkedPcap: '',
  };
}

function unavailableLogEvidence(): NonNullable<AlertDetailEvidenceRow['logEvidence']> {
  return {
    logFile: '暂不可用',
    source: '暂不可用',
    hitFields: [],
    contentSummary: '日志证据暂不可用',
    generatedAt: '暂不可用',
    status: '暂不可用',
    highlightedFields: [],
    sourceTags: [],
  };
}

function unavailableGraphPathEvidence(): NonNullable<AlertDetailEvidenceRow['graphPath']> {
  return {
    pathFile: '暂不可用',
    pathSummary: '路径关系暂不可用',
    edgeWeight: '暂不可用',
    relationType: '暂不可用',
    relatedEntities: [],
    generatedAt: '暂不可用',
    status: '暂不可用',
    nodes: [],
    edges: [],
    resources: [],
  };
}

function logTagIcon(kind: NonNullable<AlertDetailEvidenceRow['logEvidence']>['sourceTags'][number]['kind']) {
  switch (kind) {
    case 'rule':
      return <NodeIndexOutlined />;
    case 'user':
      return <UserOutlined />;
    case 'device':
    default:
      return <SafetyCertificateOutlined />;
  }
}

function graphNodeIcon(kind: NonNullable<AlertDetailEvidenceRow['graphPath']>['nodes'][number]['kind']) {
  switch (kind) {
    case 'gateway':
      return <SafetyCertificateOutlined />;
    case 'server':
      return <DatabaseOutlined />;
    case 'account':
      return <UserOutlined />;
    case 'external':
    default:
      return <GlobalOutlined />;
  }
}

function compactDateTime(value: string | undefined) {
  if (!value) return '';
  const match = value.match(/(\d{2})-(\d{2})\s+(\d{2}:\d{2}:\d{2})$/);
  if (match) return `${match[1]}-${match[2]} ${match[3]}`;
  return value.replace(/^2026-/, '').replace(/^20\d{2}-/, '');
}

function reportJobStatusLabel(status: AlertDetailActionResult['status']) {
  const labels: Partial<Record<AlertDetailActionResult['status'], string>> = {
    completed: '已完成',
    partial: '部分完成',
    failed: '失败',
    compensated: '已补偿',
    compensation_failed: '补偿失败',
  };
  return labels[status] ?? status;
}

function emptySnapshot(alertId: string): AlertDetailSnapshot {
  return {
    alertId,
    title: '告警详情加载中',
    severity: '高危',
    score: 0,
    confidence: '-',
    status: '加载中',
    assignee: '-',
    ruleModel: '-',
    attackPhase: '-',
    firstSeen: '-',
    businessSystem: '-',
    recommendation: '-',
    tags: [],
    metrics: [],
    assets: [],
    stageTrail: [],
    timeline: [],
    evidenceRows: [],
    responseActions: [],
    feedback: { defaultResult: 'pending', reason: '', whitelistDraft: '', sampleReturn: '' },
    evidence: [],
    evidenceApiError: '',
    feedbackApiError: '',
  };
}
