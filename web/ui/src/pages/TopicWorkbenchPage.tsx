import {
  AlertOutlined,
  ApiOutlined,
  ArrowRightOutlined,
  AuditOutlined,
  BellOutlined,
  BranchesOutlined,
  CloudServerOutlined,
  DatabaseOutlined,
  DownOutlined,
  DownloadOutlined,
  EditOutlined,
  ExportOutlined,
  FileDoneOutlined,
  FileProtectOutlined,
  FileSearchOutlined,
  GlobalOutlined,
  KeyOutlined,
  LockOutlined,
  NodeIndexOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SearchOutlined,
  ShareAltOutlined,
  StarOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Checkbox, Dropdown, Input, Modal, Select, Table } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  DataQualityDonutChart,
  DataQualityTrendChart,
  ExfilBarChart,
  ExfilLineChart,
  ExfilPieChart,
  TopicTopologyGraph,
  type ExfilBarItem,
  type ExfilDistributionItem,
  type ExfilSankeyLink,
  type ExfilSankeyNode,
  type ExfilTrendPoint,
  type TopicTopologyLink,
  type TopicTopologyNode,
} from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { findRouteById } from '@/routes/routeManifest';
import type { NavRoute } from '@/routes/routeManifest';
import {
  createTopicSubscription,
  exportTopicArtifact,
  fetchPageSnapshot,
  fetchTopicScope,
  saveTopicView,
  submitTopicAction,
  updateTopicScope,
  updateTopicViewPreference,
} from '@/services/api';
import type { TopicExport } from '@/services/api';
import type { PageSnapshot, SnapshotRow, TopicVisuals } from '@/services/mockData';

type TopicId = 'topic-tunnel' | 'topic-exfil' | 'topic-apt';
type Tone = PageSnapshot['metrics'][number]['status'];
type SnapshotMetric = PageSnapshot['metrics'][number];

type ExfilTableRow = {
  region: string;
  asn: string;
  traffic: string;
  ratio: string;
};

type ExfilVisualModel = {
  sankeyNodes: ExfilSankeyNode[];
  sankeyLinks: ExfilSankeyLink[];
  destinationRows: ExfilTableRow[];
  distributionTitle: string;
  sensitiveTypes: ExfilDistributionItem[];
  protocols: ExfilDistributionItem[];
  trend: ExfilTrendPoint[];
  accounts: ExfilBarItem[];
  confidence: number;
  totalUploadGb: number;
  pathCount: number;
};

type AptCampaignNode = {
  name: string;
  fullName?: string;
  meta: string;
  events: number;
  tone: Tone;
};

type AptPhaseNode = {
  id: string;
  label: string;
  value: number;
  confidence: string;
  tone: Tone;
};

type AptEvidenceNode = {
  label: string;
  value: string;
  tone: Tone;
};

type AptTimelinePoint = {
  label: string;
  aptCn: number;
  tempHawk: number;
  unknown: number;
};

type AptIocRow = {
  ioc: string;
  type: string;
  hits: number;
  campaign: string;
  firstSeen: string;
  lastSeen: string;
};

type AptEvidenceEventRow = {
  id: string;
  phase: string;
  assetGroup: string;
  ioc: string;
  evidenceType: string;
  timeWindow: string;
  status: string;
  statusTone: Tone;
  actions: string[];
};

type AptVisualModel = {
  campaigns: AptCampaignNode[];
  phases: AptPhaseNode[];
  evidenceNodes: AptEvidenceNode[];
  assets: AptEvidenceNode[];
  timeline: AptTimelinePoint[];
  iocs: AptIocRow[];
  response: Array<{ label: string; value: number; tone: Tone }>;
  evidenceRows: PageSnapshot['evidence'];
  campaignDetails: NonNullable<TopicVisuals['aptCampaigns']>;
  lateralRelations: NonNullable<TopicVisuals['aptLateralPaths']>;
  evidenceAssociations: NonNullable<TopicVisuals['aptEvidenceAssociations']>;
  reportConfidence: number;
  closureRate: number;
  eventTotal: number;
};

type TopicConfig = {
  tone: 'tunnel' | 'exfil' | 'apt';
  topicCode: string;
  displayTopicId?: string;
  site: string;
  assetGroup: string;
  ipRange: string;
  protocol: string;
  timeRange: string;
  rule: string;
  model: string;
  canvasTitle: string;
  canvasMode: string;
  reportTitle: string;
  reportSubject: string;
  eventTotal: number;
  api: string;
  icon: ReactNode;
  focusModes: string[];
  signalTitle: string;
  signals: Array<{ label: string; value: string; detail: string; status: Tone; icon: ReactNode }>;
  laneTitle: string;
  lanes: Array<{ phase: string; target: string; evidence: string; status: Tone; icon: ReactNode }>;
  actionRows: Array<{ label: string; detail: string; status: Tone; icon: ReactNode }>;
  drillLinks: Array<{ label: string; to: string; icon: ReactNode }>;
  score: number;
};

type TopicActionButtonProps = {
  topic: string;
  title: string;
  target?: string;
  className?: string;
  ariaLabel?: string;
  overlayId?: string;
  reportBinding?: TopicReportSnapshotBinding;
  children: ReactNode;
};

type TopicReportSnapshotState = {
  exportId: string;
  snapshotSha256: string;
  reportModel: Record<string, unknown>;
  rawReport: Record<string, unknown>;
  sourceArtifact: TopicExport;
};

type TopicReportSnapshotBinding = {
  value?: TopicReportSnapshotState;
  update: (value?: TopicReportSnapshotState) => void;
};

function collectTopicViewState() {
  const shell = document.querySelector<HTMLElement>('.taf-topic-shell');
  const values = [...(shell?.querySelectorAll<HTMLInputElement>('input:not([type="hidden"])') ?? [])]
    .filter((input) => input.value.trim())
    .map((input) => ({ label: input.getAttribute('aria-label') || input.placeholder || input.name || 'input', value: input.value }));
  const selections = [...(shell?.querySelectorAll<HTMLElement>('.ant-select-selection-item') ?? [])]
    .map((item) => item.textContent?.trim())
    .filter((value): value is string => Boolean(value));
  const activeTabs = [...(shell?.querySelectorAll<HTMLElement>('[role="tab"][aria-selected="true"], .taf-topic-apt-tabs button.is-active') ?? [])]
    .map((item) => item.textContent?.trim())
    .filter((value): value is string => Boolean(value));
  const activePages = [...(shell?.querySelectorAll<HTMLElement>('.ant-pagination-item-active, .taf-topic-page-button.is-active') ?? [])]
    .map((item) => item.textContent?.trim())
    .filter((value): value is string => Boolean(value));
  return {
    route: `${window.location.pathname}${window.location.search}`,
    values,
    selections,
    active_tabs: activeTabs,
    active_pages: activePages,
    fullscreen: Boolean(document.fullscreenElement),
    captured_at: new Date().toISOString(),
  };
}

function collectTopicDataContext() {
  const shell = document.querySelector<HTMLElement>('.taf-topic-shell');
  return {
    data_mode: shell?.dataset.dataMode === 'simulated' ? 'simulated' as const : 'live' as const,
    simulation_id: shell?.dataset.simulationId || undefined,
    simulation_version: shell?.dataset.simulationVersion || undefined,
    view_state: collectTopicViewState(),
  };
}

function downloadTopicArtifact(result: { result?: Record<string, unknown> }) {
  const artifact = result.result ?? {};
  const encoded = typeof artifact.content_base64 === 'string' ? artifact.content_base64 : '';
  if (!encoded) return;
  const bytes = Uint8Array.from(window.atob(encoded), (character) => character.charCodeAt(0));
  const blob = new Blob([bytes], { type: typeof artifact.content_type === 'string' ? artifact.content_type : 'application/octet-stream' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = typeof artifact.filename === 'string' ? artifact.filename : 'topic-export.bin';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function decodeTopicReportExport(result: TopicExport): TopicReportSnapshotState {
  const artifact = result.result ?? {};
  const encoded = typeof artifact.content_base64 === 'string' ? artifact.content_base64 : '';
  if (!encoded) throw new Error('报告接口未返回可预览内容');
  const decoded = new TextDecoder().decode(Uint8Array.from(window.atob(encoded), (character) => character.charCodeAt(0)));
  const rawReport = JSON.parse(decoded) as Record<string, unknown>;
  const snapshotSha256 = typeof artifact.snapshot_sha256 === 'string' ? artifact.snapshot_sha256 : '';
  if (!snapshotSha256) throw new Error('报告接口未返回 snapshot_sha256');
  const reportModel = artifact.report_model && typeof artifact.report_model === 'object'
    ? artifact.report_model as Record<string, unknown>
    : {};
  return {
    exportId: result.export_id,
    snapshotSha256,
    reportModel,
    rawReport,
    sourceArtifact: result,
  };
}

function assertSameTopicReportSnapshot(source: TopicReportSnapshotState, derived: TopicExport) {
  const hash = typeof derived.result?.snapshot_sha256 === 'string' ? derived.result.snapshot_sha256 : '';
  if (!hash || hash !== source.snapshotSha256) {
    throw new Error(`报告快照校验失败：预览 ${source.snapshotSha256 || '缺失'}，下载 ${hash || '缺失'}`);
  }
}

function TopicActionButton({ topic, title, target = title, className, ariaLabel, overlayId, reportBinding, children }: TopicActionButtonProps) {
  const [actionSearchParams] = useSearchParams();
  const [open, setOpen] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [actionID, setActionID] = useState('');
  const [businessEffect, setBusinessEffect] = useState<{ state?: string; message?: string; next_route?: string; evidence_ref?: string }>();
  const [submitError, setSubmitError] = useState('');
  const [viewName, setViewName] = useState(`${title.includes('保存') ? '当前专题视图' : title}-${new Date().toISOString().slice(0, 10)}`);
  const [viewVisibility, setViewVisibility] = useState('private');
  const [favoriteView, setFavoriteView] = useState(false);
  const [scopeName, setScopeName] = useState('当前专题范围');
  const [includedAssets, setIncludedAssets] = useState(target);
  const [excludedAssets, setExcludedAssets] = useState('');
  const [riskLevels, setRiskLevels] = useState<string[]>(['high', 'medium']);
  const [timeWindow, setTimeWindow] = useState(title.includes('APT') ? '30d' : '24h');
  const [subscriptionChannel, setSubscriptionChannel] = useState('webhook');
  const [subscriptionThreshold, setSubscriptionThreshold] = useState('high');
  const [subscriptionSchedule, setSubscriptionSchedule] = useState('realtime');
  const [subscriptionRecipients, setSubscriptionRecipients] = useState('sec_analyst');
  const [exportFormat, setExportFormat] = useState('pdf');

  const actionKind =
    title.includes('编辑范围') ? 'scope'
      : title.includes('保存视图') ? 'view'
        : title.includes('证据包') ? 'evidence'
          : title.includes('报告') || title.includes('周报导出') ? 'report'
            : title === '订阅' || title.includes('订阅配置') ? 'subscription'
              : title === '分享' ? 'share'
                : title === '收藏' ? 'favorite'
                  : 'action';

  useEffect(() => {
    if (!overlayId || actionSearchParams.get('__codex_page_id') !== overlayId) return;
    if (actionKind === 'share' || actionKind === 'favorite') {
      setMenuOpen(true);
      return;
    }
    setOpen(true);
  }, [actionKind, actionSearchParams, overlayId]);

  const resetResult = () => {
    setSubmitted(false);
    setActionID('');
    setBusinessEffect(undefined);
    setSubmitError('');
  };

  const submit = async () => {
    setSubmitting(true);
    setSubmitError('');
    try {
      if (actionKind === 'scope') {
        const result = await updateTopicScope(topic, {
          scope_name: scopeName,
          included_assets: includedAssets.split(/[,，\n]/u).map((item) => item.trim()).filter(Boolean),
          excluded_assets: excludedAssets.split(/[,，\n]/u).map((item) => item.trim()).filter(Boolean),
          risk_levels: riskLevels,
          time_window: timeWindow,
          detail: { source: 'topic-workbench', target },
        });
        setActionID(`${result.topic}:${result.updated_at}`);
        window.dispatchEvent(new CustomEvent('taf:topic-scope-updated', { detail: { topic: result.topic } }));
      } else if (actionKind === 'view') {
        const result = await saveTopicView(topic, {
          name: viewName.trim() || '当前专题视图',
          visibility: viewVisibility,
          favorite: favoriteView,
          filters: { topic, target, source: 'topic-workbench', ...collectTopicViewState() },
        });
        setActionID(result.view_id);
      } else if (actionKind === 'report' || actionKind === 'evidence') {
        let result: TopicExport;
        if (actionKind === 'report') {
          let source = reportBinding?.value;
          if (!source) {
            source = decodeTopicReportExport(await exportTopicArtifact(topic, 'report', 'json', collectTopicDataContext()));
            reportBinding?.update(source);
          }
          result = await exportTopicArtifact(topic, 'report', exportFormat, collectTopicDataContext(), source.exportId);
          assertSameTopicReportSnapshot(source, result);
        } else {
          result = await exportTopicArtifact(topic, 'evidence_package', exportFormat, collectTopicDataContext());
        }
        downloadTopicArtifact(result);
        setActionID(result.export_id);
      } else if (actionKind === 'subscription') {
        const result = await createTopicSubscription(topic, {
          channel: subscriptionChannel,
          threshold: subscriptionThreshold,
          schedule: subscriptionSchedule,
          recipients: subscriptionRecipients.split(/[,，\n]/u).map((item) => item.trim()).filter(Boolean),
        });
        setActionID(result.subscription_id);
      } else {
        const result = await submitTopicAction(topic, title, target, collectTopicDataContext());
        setActionID(result.action_id);
        setBusinessEffect(result.business_effect);
      }
      setSubmitted(true);
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : '专题操作提交失败');
    } finally {
      setSubmitting(false);
    }
  };

  const hydrateScope = async () => {
    setSubmitting(true);
    setSubmitError('');
    try {
      const scope = await fetchTopicScope(topic);
      setScopeName(scope.scope_name || '默认专题范围');
      setIncludedAssets(scope.included_assets.join('\n'));
      setExcludedAssets(scope.excluded_assets.join('\n'));
      setRiskLevels(scope.risk_levels);
      setTimeWindow(scope.time_window || '24h');
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : '专题范围加载失败');
    } finally {
      setSubmitting(false);
    }
  };

  const submitPreference = async (preference: 'favorite' | 'shared') => {
    setSubmitting(true);
    setSubmitError('');
    try {
      const result = await updateTopicViewPreference(topic, preference);
      const resultID = preference === 'shared' && result.share_token ? result.share_token : result.view_id;
      setActionID(resultID);
      if (preference === 'shared' && result.share_token && navigator.clipboard) {
        void navigator.clipboard.writeText(result.share_token).catch(() => undefined);
      }
      setSubmitted(true);
    } catch (error) {
      setSubmitError(error instanceof Error ? error.message : '专题视图偏好更新失败');
    } finally {
      setSubmitting(false);
    }
  };

  if (actionKind === 'share' || actionKind === 'favorite') {
    return (
      <Dropdown
        trigger={['click']}
        open={menuOpen}
        onOpenChange={setMenuOpen}
        menu={{
          items: [
            { key: 'shared', label: '共享当前视图', icon: <ShareAltOutlined /> },
            { key: 'favorite', label: '收藏当前视图', icon: <StarOutlined /> },
          ],
          onClick: ({ key }) => void submitPreference(key === 'shared' ? 'shared' : 'favorite'),
        }}
      >
        <button
          type="button"
          className={className}
          title={title}
          aria-label={ariaLabel}
          aria-busy={submitting}
          onClick={resetResult}
        >
          {children}
          {submitted && <span className="taf-topic-inline-result" title={`视图 ${actionID}`}>已更新</span>}
          {submitError && <span className="taf-topic-inline-result is-error" title={submitError}>失败</span>}
        </button>
      </Dropdown>
    );
  }

  const governanceBody = (
    <div className="taf-topic-governance-form">
      {actionKind === 'scope' && (
        <>
          <label>范围名称<Input value={scopeName} onChange={(event) => setScopeName(event.target.value)} /></label>
          <label>纳入资产<Input.TextArea rows={2} value={includedAssets} onChange={(event) => setIncludedAssets(event.target.value)} placeholder="资产组、IP 段，以逗号分隔" /></label>
          <label>排除资产<Input.TextArea rows={2} value={excludedAssets} onChange={(event) => setExcludedAssets(event.target.value)} placeholder="可选，以逗号分隔" /></label>
          <label>风险等级<Select mode="multiple" value={riskLevels} onChange={setRiskLevels} options={[{ value: 'critical', label: '严重' }, { value: 'high', label: '高危' }, { value: 'medium', label: '中危' }, { value: 'low', label: '低危' }]} /></label>
          <label>时间窗口<Select value={timeWindow} onChange={setTimeWindow} options={[{ value: '24h', label: '近 24 小时' }, { value: '7d', label: '近 7 天' }, { value: '30d', label: '近 30 天' }]} /></label>
        </>
      )}
      {actionKind === 'view' && (
        <>
          <label>视图名称<Input value={viewName} onChange={(event) => setViewName(event.target.value)} /></label>
          <label>可见范围<Select value={viewVisibility} onChange={setViewVisibility} options={[{ value: 'private', label: '仅自己' }, { value: 'tenant', label: '当前租户' }, { value: 'role', label: '当前角色' }]} /></label>
          <Checkbox checked={favoriteView} onChange={(event) => setFavoriteView(event.target.checked)}>同时加入收藏</Checkbox>
        </>
      )}
      {(actionKind === 'report' || actionKind === 'evidence') && (
        <>
          <Alert
            type={actionKind === 'evidence' ? 'warning' : 'info'}
            showIcon
            message={actionKind === 'evidence' ? '导出证据包将记录下载范围与审计留痕' : '导出当前专题范围内的可审计报告'}
            description={`专题：${topic}；范围：${target}`}
          />
          <label>导出格式<Select value={exportFormat} onChange={setExportFormat} options={actionKind === 'evidence' ? [{ value: 'zip', label: 'ZIP' }, { value: 'json', label: 'JSON' }] : [{ value: 'pdf', label: 'PDF' }, { value: 'docx', label: 'DOCX' }, { value: 'json', label: 'JSON' }]} /></label>
        </>
      )}
      {actionKind === 'subscription' && (
        <>
          <label>通知渠道<Select value={subscriptionChannel} onChange={setSubscriptionChannel} options={[{ value: 'webhook', label: 'Webhook' }, { value: 'email', label: '邮件' }, { value: 'in_app', label: '站内通知' }]} /></label>
          <label>触发阈值<Select value={subscriptionThreshold} onChange={setSubscriptionThreshold} options={[{ value: 'critical', label: '严重' }, { value: 'high', label: '高危' }, { value: 'medium', label: '中危' }]} /></label>
          <label>推送周期<Select value={subscriptionSchedule} onChange={setSubscriptionSchedule} options={[{ value: 'realtime', label: '实时' }, { value: 'daily', label: '日报' }, { value: 'weekly', label: '周报' }]} /></label>
          <label>接收人<Input.TextArea rows={2} value={subscriptionRecipients} onChange={(event) => setSubscriptionRecipients(event.target.value)} placeholder="账号或角色，以逗号分隔" /></label>
        </>
      )}
      {actionKind === 'action' && (
        <>
          <p>将为专题“{topic}”创建“{title}”业务任务，并原子写入任务记录与专题审计上下文。</p>
          <dl>
            <dt>专题对象</dt><dd>{topic}</dd>
            <dt>操作目标</dt><dd>{target}</dd>
            <dt>执行接口</dt><dd>/v1/topics/{'{topic}'}/actions</dd>
          </dl>
        </>
      )}
      {submitError && <Alert type="error" showIcon message="专题业务操作提交失败" description={submitError} />}
      {submitted && (
        <Alert
          type="success"
          showIcon
          message={businessEffect?.message || '专题业务操作已执行并持久化'}
          description={[
            `记录：${actionID}`,
            `状态：${businessEffect?.state || 'completed'}`,
            businessEffect?.evidence_ref ? `证据：${businessEffect.evidence_ref}` : '',
            businessEffect?.next_route ? `后续页面：${businessEffect.next_route}` : '',
          ].filter(Boolean).join('；')}
        />
      )}
    </div>
  );

  const governanceTitle =
    actionKind === 'scope' ? '专题范围编辑'
      : actionKind === 'view' ? '专题保存视图'
        : actionKind === 'report' ? '专题报告导出'
          : actionKind === 'evidence' ? '专题证据包导出'
            : actionKind === 'subscription' ? '专题订阅配置'
              : `${title}确认`;

  const trigger = (
    <button
      type="button"
      className={className}
      title={title}
      aria-label={ariaLabel}
      onClick={() => {
        resetResult();
        if (actionKind === 'scope') {
          void hydrateScope().finally(() => setOpen(true));
        } else {
          setOpen(true);
        }
      }}
    >
      {children}
    </button>
  );

  if (actionKind !== 'action') {
    return (
      <>
        {trigger}
        <Modal
          className="taf-topic-governance-modal"
          title={governanceTitle}
          open={open}
          width="min(620px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
          onCancel={() => {
            setOpen(false);
            resetResult();
          }}
          okText={submitted ? '已完成' : '确认提交'}
          cancelText="取消"
          okButtonProps={{ loading: submitting, disabled: submitted }}
          onOk={() => void submit()}
        >
          {governanceBody}
        </Modal>
      </>
    );
  }

  return (
    <>
      {trigger}
      <Modal
        className="taf-topic-action-drawer"
        title={`${title}确认`}
        open={open}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onCancel={() => {
          setOpen(false);
          resetResult();
        }}
        okText={submitted ? '已完成' : '确认执行'}
        cancelText="取消"
        okButtonProps={{ loading: submitting, disabled: submitted }}
        onOk={() => void submit()}
      >
        {governanceBody}
      </Modal>
    </>
  );
}

function TopicReportPreviewButton({
  topic,
  config,
  visuals,
  reportBinding,
}: {
  topic: string;
  config: TopicConfig;
  visuals?: TopicVisuals;
  reportBinding: TopicReportSnapshotBinding;
}) {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [downloadingFormat, setDownloadingFormat] = useState('');
  const preview = async () => {
    setOpen(true);
    setError('');
    if (reportBinding.value) return;
    setLoading(true);
    try {
      const result = await exportTopicArtifact(topic, 'report', 'json', collectTopicDataContext());
      reportBinding.update(decodeTopicReportExport(result));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '报告预览生成失败');
    } finally {
      setLoading(false);
    }
  };
  const downloadFormat = async (format: 'pdf' | 'docx' | 'json') => {
    const source = reportBinding.value;
    if (!source) return;
    setError('');
    setDownloadingFormat(format);
    try {
      const result = await exportTopicArtifact(topic, 'report', format, collectTopicDataContext(), source.exportId);
      assertSameTopicReportSnapshot(source, result);
      downloadTopicArtifact(result);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '报告下载失败');
    } finally {
      setDownloadingFormat('');
    }
  };
  const report = reportBinding.value?.rawReport;
  const exportID = reportBinding.value?.exportId ?? '';
  const snapshot = report?.snapshot && typeof report.snapshot === 'object' ? report.snapshot as Record<string, unknown> : undefined;
  const model = reportBinding.value?.reportModel ?? {};
  const summary = model.summary && typeof model.summary === 'object'
    ? model.summary as Record<string, unknown>
    : snapshot?.summary && typeof snapshot.summary === 'object'
      ? snapshot.summary as Record<string, unknown>
      : {};
  const presentation = model.presentation && typeof model.presentation === 'object'
    ? model.presentation as Record<string, unknown>
    : snapshot?.presentation && typeof snapshot.presentation === 'object'
      ? snapshot.presentation as Record<string, unknown>
      : {};
  const summaryEntries = Object.entries(summary).filter(([, value]) => ['string', 'number', 'boolean'].includes(typeof value)).slice(0, 12);
  const evidenceSections = Object.entries(snapshot ?? {})
    .filter(([, value]) => Array.isArray(value) && value.length > 0)
    .map(([key, value]) => ({ key, count: (value as unknown[]).length }))
    .slice(0, 6);
  const reportConclusion = String(model.conclusion || presentation.report_conclusion || visuals?.presentation?.reportConclusion || '当前专题证据链可用于形成专题研判结论，未闭环风险需继续跟踪处置。');
  return (
    <>
      <Button
        type="primary"
        size="small"
        className="taf-topic-report-preview-trigger"
        loading={loading}
        onClick={() => void preview()}
      >
        预览报告
      </Button>
      <Modal
        className="taf-topic-report-preview-modal"
        title={String(presentation.report_title || visuals?.presentation?.reportTitle || config.reportTitle)}
        open={open}
        width="min(820px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        footer={(
          <>
            <Button loading={downloadingFormat === 'pdf'} onClick={() => void downloadFormat('pdf')}>下载 PDF</Button>
            <Button loading={downloadingFormat === 'docx'} onClick={() => void downloadFormat('docx')}>下载 DOCX</Button>
            <Button loading={downloadingFormat === 'json'} onClick={() => void downloadFormat('json')}>下载 JSON</Button>
            <Button onClick={() => setOpen(false)}>关闭</Button>
          </>
        )}
        onCancel={() => setOpen(false)}
      >
        <div className="taf-topic-report-modal" aria-busy={loading}>
          {loading && <Alert type="info" showIcon message="正在通过专题报告 API 生成预览" />}
          {!loading && report && (
            <div className="taf-topic-report-document" data-export-id={exportID}>
              <header>
                <FileDoneOutlined />
                <div>
                  <strong>{String(presentation.report_title || visuals?.presentation?.reportTitle || config.reportTitle)}</strong>
                  <span>{String(presentation.topic_id || config.displayTopicId || topic)} · {String(presentation.time_window_label || config.timeRange)}</span>
                </div>
                <em>专题分析报告</em>
              </header>
              <section className="taf-topic-report-executive-summary">
                <h3>执行摘要</h3>
                <p>{reportConclusion}</p>
                <dl>
                  <dt>分析范围</dt><dd>{String(presentation.report_scope || visuals?.presentation?.reportScope || config.reportSubject)}</dd>
                  <dt>生成时间</dt><dd>{String(report.generated_at || presentation.report_generated_at || '当前时间')}</dd>
                  <dt>数据来源</dt><dd>{snapshot?.data_mode === 'simulated' || visuals?.dataMode === 'simulated' ? `PostgreSQL 模拟数据 / ${String(snapshot?.simulation_version || visuals?.simulationVersion || '当前版本')}` : '实时专题聚合 API'}</dd>
                  <dt>制品编号</dt><dd>{exportID}</dd>
                </dl>
              </section>
              <section>
                <h3>关键指标</h3>
                <div className="taf-topic-report-metrics">
                  {summaryEntries.map(([label, value]) => {
                    const metric = formatTopicReportMetric(label, value);
                    return <span key={label}><b>{metric.label}</b><strong>{metric.value}</strong></span>;
                  })}
                </div>
              </section>
              <section className="taf-topic-report-findings">
                <h3>关键发现与证据范围</h3>
                <ol>
                  {summaryEntries.slice(0, 3).map(([label, value]) => {
                    const metric = formatTopicReportMetric(label, value);
                    return <li key={label}><b>{metric.label}</b><span>当前值 {metric.value}，已纳入本次专题判定与处置优先级。</span></li>;
                  })}
                </ol>
                <div>
                  {evidenceSections.map((item) => <span key={item.key}><b>{topicReportCollectionLabel(item.key)}</b><strong>{item.count} 条</strong></span>)}
                  {!evidenceSections.length && <span><b>专题快照</b><strong>已生成</strong></span>}
                </div>
              </section>
              <Alert
                type="success"
                showIcon
                message="报告预览已由后端生成并写入审计"
                description={`报告制品 ID：${exportID}；快照哈希：${reportBinding.value?.snapshotSha256}`}
              />
            </div>
          )}
          {error && <Alert type="error" showIcon message="报告预览生成失败" description={error} />}
        </div>
      </Modal>
    </>
  );
}

const topicReportMetricLabels: Record<string, { label: string; unit?: string }> = {
  protocol_count: { label: '识别协议族', unit: ' 类' },
  active_users: { label: '活跃用户', unit: ' 人' },
  high_risk_users: { label: '高危用户', unit: ' 人' },
  encrypted_traffic_gbps: { label: '加密会话流量', unit: ' Gbps' },
  endpoint_count: { label: '隧道端点', unit: ' 个' },
  session_count: { label: '异常会话', unit: ' 个' },
  suspicious_ratio: { label: '可疑隧道占比', unit: '%' },
  total_bytes: { label: '加密流量', unit: ' B' },
  total_events: { label: '关联事件', unit: ' 条' },
  evidence_completeness: { label: '证据完整度', unit: '%' },
  report_confidence: { label: '报告置信度', unit: '%' },
  source_count: { label: '可疑外传源', unit: ' 个' },
  path_count: { label: '外传路径', unit: ' 条' },
  destination_count: { label: '外传目的地', unit: ' 个' },
  risk_type_count: { label: '敏感数据类型', unit: ' 类' },
  peak_upload_gbps: { label: '异常上传峰值', unit: ' Gbps' },
  campaign_count: { label: '关联战役', unit: ' 个' },
  attack_phase_count: { label: '攻击阶段', unit: ' 个' },
  evidence_count: { label: '关联证据', unit: ' 条' },
  open_risk_count: { label: '未闭环风险', unit: ' 个' },
  pending_evidence_count: { label: '待补证据', unit: ' 条' },
  reportable_count: { label: '可生成报告', unit: ' 份' },
};

function formatTopicReportMetric(key: string, value: unknown) {
  const meta = topicReportMetricLabels[key] ?? { label: key.replace(/_/gu, ' ') };
  const normalized = typeof value === 'number' && !Number.isInteger(value) ? value.toFixed(2) : String(value);
  return { label: meta.label, value: `${normalized}${meta.unit ?? ''}` };
}

function topicReportCollectionLabel(key: string) {
  const labels: Record<string, string> = {
    protocols: '协议证据', users: '用户证据', sessions: '会话证据', fingerprints: '指纹证据',
    top_sources: '源资产证据', destinations: '目的地证据', paths: '外传路径', risk_types: '风险类型',
    campaigns: '战役实体', phase_distribution: '攻击阶段', evidence: '关联证据', actions: '处置记录',
  };
  return labels[key] ?? key.replace(/_/gu, ' ');
}

function TopicReportThumbnail({ title, topicId, completeness }: { title: string; topicId: string; completeness: number }) {
  return (
    <div className="taf-topic-report-sheet" aria-label={`${title} 首屏缩略图`}>
      <FileDoneOutlined />
      <strong>{title}</strong>
      <span>{topicId}</span>
      <small>证据完整度 {Math.max(0, Math.min(100, Math.round(completeness)))}%</small>
      <i style={{ '--report-progress': `${Math.max(0, Math.min(100, completeness))}%` } as CSSProperties} />
    </div>
  );
}

function TopicProgressDonut({
  value,
  ariaLabel,
  className,
  caption,
  detail,
}: {
  value: number;
  ariaLabel: string;
  className: string;
  caption: string;
  detail?: string;
}) {
  const normalized = Math.max(0, Math.min(100, value));
  return (
    <div className={`${className} taf-topic-progress-donut`} data-center-value={`${Math.round(normalized)}%`}>
      <div className="taf-topic-progress-donut__ring">
        <DataQualityDonutChart
          ariaLabel={ariaLabel}
          className="taf-topic-progress-donut__chart"
          rows={[
            { label: '已完成', value: normalized, color: normalized >= 80 ? '#83d75d' : normalized >= 60 ? '#ffb020' : '#ff6748' },
            { label: '待完成', value: 100 - normalized, color: 'rgba(56, 151, 201, 0.18)' },
          ]}
        />
        <strong className="taf-topic-progress-donut__value">{Math.round(normalized)}%</strong>
      </div>
      <span className="taf-topic-progress-donut__caption">{caption}</span>
      {detail ? <small className="taf-topic-progress-donut__detail">{detail}</small> : null}
    </div>
  );
}

const topicConfigs: Record<TopicId, TopicConfig> = {
  'topic-tunnel': {
    tone: 'tunnel',
    topicCode: 'tunnel',
    site: '主校区',
    assetGroup: '办公终端 / 服务群组',
    ipRange: '10.12.0.0/16',
    protocol: 'SSH / TLS / HTTPS / RDP / SOCKS',
    timeRange: '当前查询窗口',
    rule: '加密隧道识别规则集 v2.1',
    model: '加密隧道识别模型 v1.3',
    canvasTitle: '加密隧道局部影响面',
    canvasMode: '布局：径向',
    reportTitle: '加密隧道专题汇总周报',
    reportSubject: '办公终端 / 服务群组',
    eventTotal: 128,
    api: '/v1/topics/tunnel',
    icon: <LockOutlined />,
    focusModes: ['高危会话', '未知 SNI', 'DoH', '长连接'],
    signalTitle: '隧道信号雷达',
    signals: [
      { label: '协议族识别', value: 'TLS / QUIC / VPN', detail: '按协议、SNI、ALPN 和指纹聚合', status: 'info', icon: <CloudServerOutlined /> },
      { label: '高危用户', value: '风险 Top 20', detail: '源资产会话数、总流量、最近命中', status: 'risk', icon: <AlertOutlined /> },
      { label: '指纹证据', value: 'JA3 / JA3S', detail: '与加密流量页共享指纹证据', status: 'warn', icon: <KeyOutlined /> },
      { label: '取证窗口', value: 'PCAP / 会话', detail: '按隧道会话回收证据包', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: '隧道研判路径',
    lanes: [
      { phase: '识别', target: '协议族和未知加密通道', evidence: '专题接口', status: 'ok', icon: <ApiOutlined /> },
      { phase: '聚合', target: '源资产、目的对象和长连接', evidence: '用户/协议', status: 'info', icon: <NodeIndexOutlined /> },
      { phase: '解释', target: 'JA3、SNI、ALPN、证书链', evidence: '加密流量', status: 'warn', icon: <KeyOutlined /> },
      { phase: '取证', target: 'PCAP 时间窗和会话摘要', evidence: '取证分析', status: 'ok', icon: <FileSearchOutlined /> },
      { phase: '处置', target: '阻断、白名单复核、审计', evidence: '审计日志', status: 'risk', icon: <ThunderboltOutlined /> },
    ],
    actionRows: [
      { label: '提取隧道 PCAP', detail: '按源资产和目的对象生成取证任务', status: 'ok', icon: <DownloadOutlined /> },
      { label: '阻断高危通道', detail: '联动规则、SOAR 和边界策略', status: 'risk', icon: <LockOutlined /> },
      { label: '复核业务例外', detail: '对 CDN、VPN、备份流量做白名单复核', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '沉淀专题报告', detail: '输出隧道趋势、证据和处置复盘', status: 'info', icon: <AuditOutlined /> },
    ],
    drillLinks: [
      { label: '加密流量', to: '/encrypted-traffic', icon: <LockOutlined /> },
      { label: '取证分析', to: '/forensics', icon: <FileSearchOutlined /> },
      { label: '实体图谱', to: '/graph', icon: <NodeIndexOutlined /> },
      { label: '审计日志', to: '/audit-log', icon: <AuditOutlined /> },
    ],
    score: 92,
  },
  'topic-exfil': {
    tone: 'exfil',
    topicCode: 'exfil',
    site: '主校区',
    assetGroup: '科研文件服务 / 办公终端',
    ipRange: '10.14.0.0/16',
    protocol: 'HTTPS / S3 / WebDAV / DNS',
    timeRange: '当前查询窗口',
    rule: '数据外传识别模型 v3.2',
    model: '外传路径识别模型 v2.0',
    canvasTitle: '数据外传路径分析 (Sankey)',
    canvasMode: '风险路径 TOP',
    reportTitle: '数据外传专题汇总周报',
    reportSubject: '科研文件服务 / 办公终端',
    eventTotal: 128,
    api: '/v1/topics/exfil',
    icon: <DatabaseOutlined />,
    focusModes: ['高危源资产', '跨境目的地', '云存储', '异常上传'],
    signalTitle: '外传风险信号',
    signals: [
      { label: '源资产排行', value: '源资产 Top', detail: '上传量、会话数、目的地数量', status: 'risk', icon: <DatabaseOutlined /> },
      { label: '路径分叉', value: '外传路径', detail: '源 IP 到外联目的地的风险路径', status: 'warn', icon: <BranchesOutlined /> },
      { label: '风险类型', value: '类型聚合', detail: '云盘、境外、异常端口、未知协议', status: 'info', icon: <RadarChartOutlined /> },
      { label: '证据汇聚', value: 'PCAP / 会话', detail: '关联上传窗口、目的地和审计动作', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: '数据外传闭环路径',
    lanes: [
      { phase: '发现', target: '上传突增和异常目的地', evidence: '源资产排行', status: 'risk', icon: <AlertOutlined /> },
      { phase: '定位', target: '源资产、账号、业务系统', evidence: '资产图谱', status: 'warn', icon: <SearchOutlined /> },
      { phase: '分类', target: '云存储、跨境、敏感库', evidence: '风险类型', status: 'info', icon: <DatabaseOutlined /> },
      { phase: '阻断', target: '路径、账号和目的地策略', evidence: 'SOAR 剧本', status: 'risk', icon: <ThunderboltOutlined /> },
      { phase: '固化', target: '报告、审计和复验', evidence: '合规审计', status: 'ok', icon: <AuditOutlined /> },
    ],
    actionRows: [
      { label: '阻断外传路径', detail: '按目的地、端口和源资产生成策略', status: 'risk', icon: <LockOutlined /> },
      { label: '隔离源资产', detail: '对高危源资产触发 SOAR 审批', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '提取样本证据', detail: '生成上传窗口 PCAP 和 Session 包', status: 'ok', icon: <DownloadOutlined /> },
      { label: '复核白名单', detail: '业务备份与云服务例外进入白名单治理', status: 'info', icon: <FileProtectOutlined /> },
    ],
    drillLinks: [
      { label: '资产台账', to: '/assets', icon: <DatabaseOutlined /> },
      { label: '行为基准', to: '/baselines', icon: <RadarChartOutlined /> },
      { label: 'SOAR 剧本', to: '/playbooks', icon: <ThunderboltOutlined /> },
      { label: '合规审计', to: '/compliance', icon: <AuditOutlined /> },
    ],
    score: 88,
  },
  'topic-apt': {
    tone: 'apt',
    topicCode: 'apt',
    site: '主园区',
    assetGroup: '办公终端 / 数据中心',
    ipRange: '10.12.0.0/16',
    protocol: '初始访问 / 执行 / 横向移动 / 数据外传',
    timeRange: '当前查询窗口',
    rule: '战役关联规则 v2.4',
    model: '战役聚类模型 v1.8',
    canvasTitle: 'APT/战役攻击链画布',
    canvasMode: '布局：分层',
    reportTitle: 'APT/战役分析报告',
    reportSubject: '办公终端 / 数据中心',
    eventTotal: 156,
    api: '/v1/topics/apt',
    icon: <RadarChartOutlined />,
    focusModes: ['活跃战役', '横向移动', '高危实体', '阶段复盘'],
    signalTitle: '战役态势信号',
    signals: [
      { label: '阶段分布', value: 'ATT&CK 阶段', detail: '从初始访问到影响达成的阶段聚合', status: 'warn', icon: <BranchesOutlined /> },
      { label: '实体影响', value: '实体图谱', detail: '主机、账号、服务和目的地串联', status: 'risk', icon: <NodeIndexOutlined /> },
      { label: '关联告警', value: '战役聚类', detail: '多告警聚类成战役视角', status: 'info', icon: <AlertOutlined /> },
      { label: '复盘证据', value: '证据包', detail: '阶段、实体、PCAP、审计闭环', status: 'ok', icon: <FileProtectOutlined /> },
    ],
    laneTitle: 'APT 战役阶段线',
    lanes: [
      { phase: '初始访问', target: '漏洞利用、账号异常、恶意投递', evidence: '告警证据', status: 'warn', icon: <AlertOutlined /> },
      { phase: '执行活动', target: '脚本、进程、远控工具', evidence: '行为事件', status: 'risk', icon: <ThunderboltOutlined /> },
      { phase: '横向移动', target: '账号、SMB、RDP、服务跳转', evidence: '实体图谱', status: 'risk', icon: <NodeIndexOutlined /> },
      { phase: 'C2 通信', target: '加密隧道、DGA、异常外联', evidence: '加密流量', status: 'warn', icon: <GlobalOutlined /> },
      { phase: '影响达成', target: '数据外传、破坏、持久化', evidence: '战役复盘', status: 'ok', icon: <AuditOutlined /> },
    ],
    actionRows: [
      { label: '下钻攻击链', detail: '进入阶段画布复核关键节点', status: 'risk', icon: <BranchesOutlined /> },
      { label: '导出战役包', detail: '合并关联告警、实体和取证证据', status: 'ok', icon: <DownloadOutlined /> },
      { label: '关联检测规则', detail: '把复盘结论回写规则和模型治理', status: 'warn', icon: <SafetyCertificateOutlined /> },
      { label: '写入审计', detail: '固化战役处理、复盘结论和责任链', status: 'info', icon: <AuditOutlined /> },
    ],
    drillLinks: [
      { label: '战役列表', to: '/campaigns', icon: <RadarChartOutlined /> },
      { label: '攻击链分析', to: '/attack-chains', icon: <BranchesOutlined /> },
      { label: '实体图谱', to: '/graph', icon: <NodeIndexOutlined /> },
      { label: '规则管理', to: '/rules', icon: <SafetyCertificateOutlined /> },
    ],
    score: 90,
  },
};

const topicOptions: Array<{ id: TopicId; label: string; param: string }> = [
  { id: 'topic-tunnel', label: '加密隧道专题', param: 'tunnel' },
  { id: 'topic-exfil', label: '数据外传专题', param: 'exfil' },
  { id: 'topic-apt', label: 'APT/战役专题', param: 'apt' },
];

function TopicHeaderControls({ config }: { config: TopicConfig }) {
  return (
    <>
      <TopicActionButton topic={config.topicCode} title="编辑范围" target={config.assetGroup} className="ant-btn ant-btn-default ant-btn-sm" overlayId="drawer-topic-scope-edit">
        <EditOutlined />编辑范围
      </TopicActionButton>
      <TopicActionButton topic={config.topicCode} title="保存视图" target={config.topicCode} className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-save-view">
        <SaveOutlined />保存视图<DownOutlined className="taf-topic-save-view-chevron" />
      </TopicActionButton>
    </>
  );
}

function topicRailOverlayId(label: string) {
  if (label === '订阅') return 'drawer-topic-subscription';
  if (label === '分享') return 'dropdown-topic-share-favorite';
  return undefined;
}

type TunnelKpi = {
  label: string;
  icon: ReactNode;
};

const tunnelKpis: TunnelKpi[] = [
  { label: '隧道协议数', icon: <GlobalOutlined /> },
  { label: '高频隧道源', icon: <NodeIndexOutlined /> },
  { label: '加密会话流量', icon: <ThunderboltOutlined /> },
  { label: '异常隧道数', icon: <AlertOutlined /> },
  { label: '隧道端点数', icon: <RadarChartOutlined /> },
  { label: '可疑隧道占比', icon: <LockOutlined /> },
  { label: '证据完整度', icon: <SafetyCertificateOutlined /> },
  { label: '报告置信度', icon: <FileDoneOutlined /> },
  { label: '未闭环风险数', icon: <FileProtectOutlined /> },
];

const topicIdByParam: Record<string, TopicId> = {
  tunnel: 'topic-tunnel',
  exfil: 'topic-exfil',
  apt: 'topic-apt',
};

const resolveTopicId = (topicParam: string | null, tabParam: string | null): TopicId => {
  const param = topicParam ?? tabParam;
  return param && topicIdByParam[param] ? topicIdByParam[param] : 'topic-tunnel';
};

export function TopicWorkbenchPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedTopic = resolveTopicId(searchParams.get('topic'), searchParams.get('tab'));
  const selectedRoute = findRouteById(selectedTopic);
  const topicPage = selectedRoute?.page ?? route.page;
  const config = topicConfigs[selectedTopic];
  const [focusMode, setFocusMode] = useState(config.focusModes[0]);
  const [selectedSignal, setSelectedSignal] = useState(config.signals[0].label);
  const [scopeRevision, setScopeRevision] = useState(0);
  const [reportSnapshot, setReportSnapshot] = useState<TopicReportSnapshotState>();

  useEffect(() => {
    setFocusMode(config.focusModes[0]);
    setSelectedSignal(config.signals[0].label);
  }, [config]);

  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', selectedTopic, scopeRevision],
    queryFn: () => fetchPageSnapshot(selectedTopic),
  });

  useEffect(() => {
    const handleScopeUpdate = (event: Event) => {
      const updatedTopic = (event as CustomEvent<{ topic?: string }>).detail?.topic;
      if (!updatedTopic || updatedTopic === config.topicCode) setScopeRevision((value) => value + 1);
    };
    window.addEventListener('taf:topic-scope-updated', handleScopeUpdate);
    return () => window.removeEventListener('taf:topic-scope-updated', handleScopeUpdate);
  }, [config.topicCode]);

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const topicVisuals = data?.visuals?.topic;
  const reportValidityKey = [
    selectedTopic,
    scopeRevision,
    topicVisuals?.scope?.updatedAt ?? 0,
    topicVisuals?.simulationVersion ?? '',
    topicVisuals?.updatedAt ?? 0,
  ].join(':');
  useEffect(() => {
    setReportSnapshot(undefined);
  }, [reportValidityKey]);
  const reportBinding: TopicReportSnapshotBinding = {
    value: reportSnapshot,
    update: setReportSnapshot,
  };
  const runtimeTopicCode = topicVisuals?.topic ?? config.topicCode;
  const presentation = topicVisuals?.presentation;
  const runtimeTopicID = presentation?.topicId || runtimeTopicCode;
  const runtimeTimeRange = topicVisuals?.scope?.timeWindow
    ? ({ '24h': '近24小时', '7d': '近7天', '30d': '近30天' }[topicVisuals.scope.timeWindow] || topicVisuals.scope.timeWindow)
    : presentation?.timeWindowLabel || topicTimeRangeLabel(topicVisuals, config.timeRange);
  const runtimeAssetGroup = topicVisuals?.scope?.includedAssets.length
    ? topicVisuals.scope.includedAssets.join(' / ')
    : presentation?.assetGroup || config.assetGroup;
  const effectiveConfig = {
    ...config,
    topicCode: runtimeTopicCode,
    displayTopicId: runtimeTopicID,
    site: presentation?.site || config.site,
    assetGroup: runtimeAssetGroup,
    ipRange: presentation?.ipRange || config.ipRange,
    protocol: presentation?.protocols || config.protocol,
    timeRange: runtimeTimeRange,
    rule: presentation?.ruleVersion || config.rule,
    model: presentation?.modelVersion || config.model,
    reportTitle: presentation?.reportTitle || config.reportTitle,
    reportSubject: presentation?.reportScope || config.reportSubject,
    eventTotal: data?.total || config.eventTotal,
  };
  const metrics = topicPage.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const evidenceRows = data?.evidence.length ? data.evidence : topicPage.evidence.map((label) => ({ label, value: '待返回', status: 'info' as const }));
  const columns: ColumnsType<SnapshotRow> = topicPage.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    width: selectedTopic === 'topic-exfil' && column === '处置' ? 320 : undefined,
    ellipsis: true,
    render: (value, record) => renderTopicCell(config.topicCode, column, value, record),
  }));

  if (selectedTopic === 'topic-tunnel') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`} data-data-mode={topicVisuals?.dataMode || 'live'} data-simulation-id={topicVisuals?.simulationId || ''} data-simulation-version={topicVisuals?.simulationVersion || ''}>
          <div className="taf-topic-tunnel-layout">
            <div className="taf-topic-tunnel-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <TopicHeaderControls config={effectiveConfig} />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', runtimeTopicID],
                  ['站点', effectiveConfig.site],
                  ['资产组', runtimeAssetGroup],
                  ...(config.tone === 'apt'
                    ? [['攻击阶段', effectiveConfig.ipRange]]
                    : [['IP 段', effectiveConfig.ipRange], ['协议', effectiveConfig.protocol]]),
                  ['时间窗', runtimeTimeRange],
                  ['规则', effectiveConfig.rule],
                  ['模型', effectiveConfig.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <TunnelKpiStrip metrics={metrics} />

              <main className="taf-topic-main taf-topic-tunnel-main">
                <div className="taf-topic-boardline taf-topic-tunnel-boardline">
                  <TunnelCanvasPanel
                    config={effectiveConfig}
                    rows={rows}
                    visuals={topicVisuals}
                  />

                  <TunnelAnalysisPanel rows={rows} metrics={metrics} visuals={topicVisuals} />
                </div>

                <TunnelEvidenceSection
                  rows={rows}
                  isLoading={isLoading}
                  total={data?.total}
                  topic={config.topicCode}
                  displayTopicId={runtimeTopicID}
                />
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-tunnel-rail">
              <TunnelRightRail config={effectiveConfig} metrics={metrics} evidenceRows={evidenceRows} visuals={topicVisuals} reportBinding={reportBinding} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  if (selectedTopic === 'topic-exfil') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`} data-data-mode={topicVisuals?.dataMode || 'live'} data-simulation-id={topicVisuals?.simulationId || ''} data-simulation-version={topicVisuals?.simulationVersion || ''}>
          <div className="taf-topic-exfil-layout">
            <div className="taf-topic-exfil-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <TopicHeaderControls config={effectiveConfig} />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', runtimeTopicID],
                  ['站点', effectiveConfig.site],
                  ['资产组', runtimeAssetGroup],
                  ...(config.tone === 'apt'
                    ? [['攻击阶段', effectiveConfig.ipRange]]
                    : [['IP 段', effectiveConfig.ipRange], ['协议', effectiveConfig.protocol]]),
                  ['时间窗', runtimeTimeRange],
                  ['规则', effectiveConfig.rule],
                  ['模型', effectiveConfig.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <div className="taf-topic-kpis">
                {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
              </div>

              <main className="taf-topic-main taf-topic-exfil-main">
                <div className="taf-topic-boardline taf-topic-exfil-boardline">
                  <WorkPanel
                    title={config.canvasTitle}
                    className="taf-topic-canvas-panel taf-topic-exfil-canvas-panel"
                    extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
                  >
                    <ExfilCanvas rows={rows} metrics={metrics} visuals={topicVisuals} />
                  </WorkPanel>

                  <ExfilAnalysisDashboard rows={rows} metrics={metrics} focusMode={focusMode} visuals={topicVisuals} />
                </div>

                <ExfilEvidenceSection
                  title={topicPage.tableTitle}
                  rows={rows}
                  columns={columns}
                  rowKeyColumn={topicPage.tableColumns[0]}
                  isLoading={isLoading}
                />
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-exfil-rail">
              <ExfilRightRail config={effectiveConfig} metrics={metrics} evidenceRows={evidenceRows} visuals={topicVisuals} reportBinding={reportBinding} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  if (selectedTopic === 'topic-apt') {
    return (
      <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
        <section className={`taf-topic-shell bg-${topicPage.background}`} data-data-mode={topicVisuals?.dataMode || 'live'} data-simulation-id={topicVisuals?.simulationId || ''} data-simulation-version={topicVisuals?.simulationVersion || ''}>
          <div className="taf-topic-apt-layout">
            <div className="taf-topic-apt-left">
              <header className="taf-topic-titlebar">
                <div className="taf-topic-title-main">
                  <h1>{route.page.title}</h1>
                </div>
                <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
                  {topicOptions.map((option) => (
                    <button
                      key={option.id}
                      type="button"
                      role="tab"
                      aria-selected={option.id === selectedTopic}
                      className={option.id === selectedTopic ? 'is-active' : ''}
                      onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
                    >
                      {option.label}
                    </button>
                  ))}
                </div>
                <div className="taf-topic-controls">
                  <TopicHeaderControls config={effectiveConfig} />
                </div>
              </header>

              <div className="taf-topic-facts" aria-label="专题筛选条件">
                {[
                  ['专题ID', runtimeTopicID],
                  ['站点', effectiveConfig.site],
                  ['资产组', runtimeAssetGroup],
                  ...(config.tone === 'apt'
                    ? [['攻击阶段', effectiveConfig.ipRange]]
                    : [['IP 段', effectiveConfig.ipRange], ['协议', effectiveConfig.protocol]]),
                  ['时间窗', runtimeTimeRange],
                  ['规则', effectiveConfig.rule],
                  ['模型', effectiveConfig.model],
                ].map(([label, value]) => (
                  <span key={label} className={`is-${label === '时间窗' ? 'time' : label === '规则' ? 'rule' : label === '模型' ? 'model' : 'default'}`} title={`${label}: ${value}`}>
                    <b>{label}：</b>
                    <em>{value}</em>
                  </span>
                ))}
              </div>

              {isError && (
                <Alert
                  type="error"
                  showIcon
                  message="真实 API 数据加载失败"
                  description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
                  action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
                />
              )}

              <div className="taf-topic-kpis">
                {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
              </div>

              <main className="taf-topic-main taf-topic-apt-main">
                <div className="taf-topic-boardline taf-topic-apt-boardline">
                  <WorkPanel
                    title={config.canvasTitle}
                    className="taf-topic-canvas-panel taf-topic-apt-canvas-panel"
                    extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
                  >
                    <TopicCanvas topicId={selectedTopic} rows={rows} metrics={metrics} visuals={topicVisuals} />
                  </WorkPanel>

                  <AptAnalysisDashboard rows={rows} metrics={metrics} evidenceRows={evidenceRows} focusMode={focusMode} visuals={topicVisuals} />
                </div>

                <div className="taf-topic-apt-bottomline">
                  <WorkPanel title={`战役关联事件与证据 / ${runtimeTopicID}`} className="taf-topic-table-panel taf-topic-apt-table-panel">
                    <AptEvidenceTable rows={rows} isLoading={isLoading} topic={config.topicCode} />
                  </WorkPanel>

                  <AptResponsePanel rows={rows} metrics={metrics} visuals={topicVisuals} />
                </div>
              </main>
            </div>

            <aside className="taf-topic-rail taf-topic-apt-rail">
              <AptRightRail config={effectiveConfig} metrics={metrics} evidenceRows={evidenceRows} rows={rows} visuals={topicVisuals} reportBinding={reportBinding} />
            </aside>
          </div>
        </section>
      </div>
    );
  }

  return (
    <div className={`taf-page taf-topic-page taf-topic-${config.tone}`}>
      <section className={`taf-topic-shell bg-${topicPage.background}`} data-data-mode={topicVisuals?.dataMode || 'live'} data-simulation-id={topicVisuals?.simulationId || ''} data-simulation-version={topicVisuals?.simulationVersion || ''}>
        <header className="taf-topic-titlebar">
          <div className="taf-topic-title-main">
            <h1>{route.page.title}</h1>
          </div>
          <div className="taf-topic-tabs" role="tablist" aria-label="专题切换">
            {topicOptions.map((option) => (
              <button
                key={option.id}
                type="button"
                role="tab"
                aria-selected={option.id === selectedTopic}
                className={option.id === selectedTopic ? 'is-active' : ''}
                onClick={() => setSearchParams({ topic: option.param, tab: option.param })}
              >
                {option.label}
              </button>
            ))}
          </div>
          <div className="taf-topic-controls">
            <TopicHeaderControls config={effectiveConfig} />
          </div>
        </header>

        <div className="taf-topic-facts" aria-label="专题筛选条件">
          {[
            ['专题ID', runtimeTopicCode],
            ['站点', config.site],
            ['资产组', runtimeAssetGroup],
            ['IP 段', config.ipRange],
            ['协议', config.protocol],
            ['时间窗', runtimeTimeRange],
            ['规则', config.rule],
            ['模型', config.model],
          ].map(([label, value]) => (
            <span key={label}>
              <b>{label}：</b>
              <em>{value}</em>
            </span>
          ))}
        </div>

        {isError && (
          <Alert
            type="error"
            showIcon
            message="真实 API 数据加载失败"
            description={error instanceof Error ? error.message : `请检查 ${config.api}、APISIX 路由或 alert-service。`}
            action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
          />
        )}

        <div className="taf-topic-kpis">
          {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
        </div>

        <div className="taf-topic-grid">
          <main className="taf-topic-main">
            <div className="taf-topic-boardline">
              <WorkPanel
                title={config.canvasTitle}
                className="taf-topic-canvas-panel"
                extra={<span className="taf-topic-focus">{config.canvasMode}</span>}
              >
                <TopicCanvas topicId={selectedTopic} rows={rows} metrics={metrics} visuals={topicVisuals} />
                <div className="taf-topic-alert-strip">
                  {config.signals.slice(0, 3).map((signal) => (
                    <button
                      key={signal.label}
                      type="button"
                      className={`taf-topic-alert-chip is-${signal.status} ${selectedSignal === signal.label ? 'is-selected' : ''}`}
                      onClick={() => setSelectedSignal(signal.label)}
                    >
                      <span>{signal.icon}</span>
                      <strong>{signal.label}</strong>
                      <em>{signal.value}</em>
                    </button>
                  ))}
                </div>
              </WorkPanel>

              <AptAnalysisDashboard rows={rows} metrics={metrics} evidenceRows={evidenceRows} focusMode={focusMode} visuals={topicVisuals} />
            </div>

            <WorkPanel title={topicPage.tableTitle} className="taf-topic-table-panel" extra={<span>{topicPage.tabs[0]}</span>}>
              <Table
                rowKey={(record) => String(record[topicPage.tableColumns[0]] ?? JSON.stringify(record))}
                size="small"
                loading={isLoading}
                columns={columns}
                dataSource={rows}
                pagination={{ pageSize: 5, size: 'small' }}
                scroll={{ x: 980, y: 142 }}
              />
            </WorkPanel>
          </main>

          <aside className="taf-topic-rail">
            <AptRightRail config={config} metrics={metrics} evidenceRows={evidenceRows} rows={rows} visuals={topicVisuals} reportBinding={reportBinding} />
          </aside>
        </div>
      </section>
    </div>
  );
}

function TunnelKpiStrip({ metrics }: { metrics: SnapshotMetric[] }) {
  return (
    <div className="taf-topic-kpis taf-topic-tunnel-kpis" aria-label="加密隧道专题指标">
      {tunnelKpis.map((item) => {
        const metric = metrics.find((candidate) => candidate.label === item.label);
        const value = metric?.value ?? '0';
        const delta = metric?.delta || '实时接口';
        const status = metric?.status ?? 'info';
        return (
          <div key={item.label} className={`taf-topic-tunnel-kpi is-${status}`} title={`${item.label}: ${value}, ${delta}`}>
            <span>{item.icon}</span>
            <b>{item.label}</b>
            <strong>{value}</strong>
            <em>{delta}</em>
          </div>
        );
      })}
    </div>
  );
}

function TunnelCanvasPanel({
  config,
  rows,
  visuals,
}: {
  config: TopicConfig;
  rows: SnapshotRow[];
  visuals?: TopicVisuals;
}) {
  const [layout, setLayout] = useState<'radial' | 'layered'>('layered');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [controlError, setControlError] = useState('');
  const panelRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const handleFullscreenChange = () => setIsFullscreen(document.fullscreenElement === panelRef.current);
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange);
  }, []);
  const switchLayout = async () => {
    const next = layout === 'radial' ? 'layered' : 'radial';
    setControlError('');
    try {
      await submitTopicAction(config.topicCode, '切换拓扑布局', next, collectTopicDataContext());
      setLayout(next);
    } catch (error) {
      setControlError(error instanceof Error ? error.message : '布局切换审计失败');
    }
  };
  const toggleFullscreen = async () => {
    const target = panelRef.current;
    if (!target) return;
    setControlError('');
    try {
      if (document.fullscreenElement) await document.exitFullscreen();
      else await target.requestFullscreen();
      await submitTopicAction(config.topicCode, '切换拓扑全屏', config.displayTopicId ?? config.topicCode, collectTopicDataContext());
    } catch (error) {
      setControlError(error instanceof Error ? error.message : '全屏切换失败');
    }
  };
  const impactHighlights = visuals?.impactHighlights?.length
    ? visuals.impactHighlights
    : config.signals.slice(0, 3).map((signal) => ({
      label: signal.label,
      value: signal.value,
      detail: signal.detail,
      status: signal.status,
      targetSignal: signal.label,
    }));
  const highlightIcon = (status: Tone) => status === 'risk'
    ? <AlertOutlined />
    : status === 'warn'
      ? <ThunderboltOutlined />
      : <NodeIndexOutlined />;

  return (
    <div ref={panelRef} className="taf-topic-tunnel-fullscreen-host">
      <WorkPanel
        title={config.canvasTitle}
        className="taf-topic-canvas-panel taf-topic-tunnel-impact-panel"
        extra={(
          <span className="taf-topic-tunnel-panel-actions">
            {controlError && <em className="taf-topic-inline-result is-error" title={controlError}>失败</em>}
            <button type="button" onClick={() => void switchLayout()}>布局：{layout === 'layered' ? '径向' : '分层'}</button>
            <button type="button" onClick={() => void toggleFullscreen()}>{isFullscreen ? '退出全屏' : '全屏'}</button>
          </span>
        )}
      >
        <TunnelImpactMap rows={rows} layout={layout} visuals={visuals} />
        <div className="taf-topic-alert-strip taf-topic-tunnel-alert-strip">
          {impactHighlights.map((signal) => (
            <article
              key={signal.label}
              className={`taf-topic-alert-chip is-${signal.status}`}
              title={signal.detail}
              data-api-summary="true"
            >
              <span>{highlightIcon(signal.status)}</span>
              <strong>{signal.label}</strong>
              <em>{signal.value}</em>
            </article>
          ))}
        </div>
      </WorkPanel>
    </div>
  );
}

function TunnelImpactMap({ rows, visuals, layout = 'layered' }: { rows: SnapshotRow[]; visuals?: TopicVisuals; layout?: 'radial' | 'layered' }) {
  const [selectedNode, setSelectedNode] = useState('');
  const nodesByID = new Map<string, TopicTopologyNode>();
  const links: TopicTopologyLink[] = [];
  rows.slice(0, 8).forEach((row, index) => {
    const source = rowText(row, '隧道源') || rowText(row, '源资产');
    const protocol = rowText(row, '协议');
    const destination = rowText(row, '目的端点');
    const sourceID = `source:${source}`;
    const protocolID = `protocol:${protocol}`;
    const destinationID = `destination:${destination}`;
    const rowY = 14 + index * Math.min(11, 72 / Math.max(Math.min(rows.length, 8), 1));
    const angle = (Math.PI * 2 * index) / Math.max(Math.min(rows.length, 8), 1) - Math.PI / 2;
    const sourcePosition = layout === 'radial' ? { x: 50 + Math.cos(angle) * 40, y: 50 + Math.sin(angle) * 39 } : { x: 14, y: rowY };
    const protocolPosition = layout === 'radial' ? { x: 50 + Math.cos(angle) * 20, y: 50 + Math.sin(angle) * 20 } : { x: 50, y: rowY };
    const destinationPosition = layout === 'radial' ? { x: 50, y: 50 } : { x: 86, y: rowY };
    if (source) nodesByID.set(sourceID, { id: sourceID, label: source, detail: rowText(row, '风险状态') || '实时隧道源', tone: 'risk', ...sourcePosition });
    if (protocol) nodesByID.set(protocolID, { id: protocolID, label: protocolLabel(protocol), detail: `${rowNumber(row, '__session_count')} 会话`, tone: 'protocol', ...protocolPosition });
    if (destination && destination !== '-') nodesByID.set(destinationID, { id: destinationID, label: destination, detail: rowText(row, '时间窗') || '最近命中', tone: 'destination', ...destinationPosition });
    if (source && protocol) links.push({
      source: sourceID,
      target: protocolID,
      tone: 'risk',
      lineType: 'solid',
      value: rowNumber(row, '__session_count'),
      label: `${source} → ${protocolLabel(protocol)} / 已识别会话`,
    });
    if (protocol && destination && destination !== '-') links.push({
      source: protocolID,
      target: destinationID,
      tone: 'ok',
      lineType: 'dashed',
      value: rowNumber(row, '__session_count'),
      label: `${protocolLabel(protocol)} → ${destination} / 关联目的端`,
    });
  });
  const apiNodes: TopicTopologyNode[] = (visuals?.topologyNodes ?? []).map((node) => {
    const normalizedY = Math.max(0, Math.min(1, (node.y - 8) / 84));
    const angle = normalizedY * Math.PI * 2 - Math.PI / 2;
    const radius = 12 + Math.max(0, Math.min(1, node.x / 100)) * 34;
    const position = layout === 'radial'
      ? {
        x: 50 + Math.cos(angle) * radius,
        y: 50 + Math.sin(angle) * radius * 0.82,
      }
      : { x: node.x, y: node.y };
    return {
      id: node.id,
      label: node.label,
      detail: node.detail,
      ...position,
      tone: node.tone,
      size: node.symbol === 'circle'
        ? [Math.round(node.width * 1.15), Math.round(node.height * 1.15)]
        : [Math.round(node.width * 1.04), Math.round(node.height * 1.1)],
      symbol: node.symbol,
      icon: node.icon,
      labelPosition: node.labelPosition,
      selected: node.id === selectedNode,
    };
  });
  const apiLinks: TopicTopologyLink[] = (visuals?.topologyLinks ?? []).map((link) => ({
    source: link.source,
    target: link.target,
    value: link.value,
    tone: link.tone,
    lineType: link.lineType,
    label: link.label,
  }));
  const nodes = apiNodes.length ? apiNodes : [...nodesByID.values()].map((node) => ({ ...node, selected: node.id === selectedNode }));
  const graphLinks = apiLinks.length ? apiLinks : links;

  return (
    <div className="taf-topic-canvas taf-topic-tunnel-impact">
      <div className="taf-topic-canvas-legend taf-topic-tunnel-legend">
        {['主机 / 资产', '探针', '隧道协议', '代理 / 跳板', '外部端点', '告警', '战役'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      {nodes.length ? (
        <TopicTopologyGraph ariaLabel={`加密隧道实时关系图 / ${layout === 'radial' ? '径向' : '分层'}`} nodes={nodes} links={graphLinks} onNodeClick={setSelectedNode} />
      ) : (
        <div className="taf-topic-empty">当前时间窗没有可绘制的隧道关系</div>
      )}
    </div>
  );
}

function TunnelAnalysisPanel({
  rows,
  metrics,
  visuals,
}: {
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  visuals?: TopicVisuals;
}) {
  const topSources = buildTunnelTopSources(rows);
  const completeness = Math.round(metricValueNumber(metrics, '证据完整度'));
  const protocols = visuals?.tunnelProtocols ?? [];
  const users = visuals?.tunnelUsers ?? [];
  const protocolTotal = protocols.reduce((sum, item) => sum + item.count, 0);
  const protocolDistribution = protocols.map((item, index) => ({
    label: protocolLabel(item.protocol),
    value: protocolTotal ? Number((item.count / protocolTotal * 100).toFixed(1)) : 0,
    color: ['#58bfff', '#8bd85e', '#ff6b4a', '#ffb020', '#b685ff', '#7f8fb5'][index % 6],
  })).filter((item) => item.value > 0);
  const protocolRows = protocols.map((item) => ({
    label: protocolLabel(item.protocol),
    percent: percentOf(item.count, protocolTotal),
    traffic: bytesLabelCompact(item.totalBytes),
  }));
  const endpointGroups = new Map<string, number>();
  users.forEach((item) => {
    if (item.dstIp) endpointGroups.set(item.dstIp, (endpointGroups.get(item.dstIp) ?? 0) + item.count);
  });
  const endpointTop = [...endpointGroups.entries()]
    .map(([label, value]) => ({ label, value }))
    .sort((a, b) => b.value - a.value)
    .slice(0, 5);
  const endpointRows = (visuals?.destinationDistribution ?? []).length
    ? (visuals?.destinationDistribution ?? []).slice(0, 5).map((item) => [item.label, String(item.value), item.asn || '未归因', item.trafficGb.toFixed(1)])
    : endpointTop.map((item) => [item.label, String(item.value), '未归因', '-']);
  const tunnelSourceTop5 = users
    .slice()
    .sort((left, right) => right.totalBytes - left.totalBytes || right.count - left.count)
    .slice(0, 5);
  const tunnelSourceBars = tunnelSourceTop5.map((item) => ({
    label: item.ip,
    value: Number((item.totalBytes / (1024 ** 3)).toFixed(1)),
  }));
  const destinationDistributionBars = (visuals?.destinationDistribution ?? [])
    .slice()
    .sort((left, right) => right.value - left.value)
    .slice(0, 5)
    .map((item) => ({
      label: item.label,
      value: item.value,
    }));
  const trend = visuals?.tunnelTrend?.length
    ? visuals.tunnelTrend
    : users
      .filter((item) => item.lastSeen > 0)
      .sort((a, b) => a.lastSeen - b.lastSeen)
      .slice(-8)
      .map((item) => ({ label: formatTopicTime(item.lastSeen, 'MM-DD HH:mm'), value: Number((item.totalBytes / (1024 ** 3)).toFixed(2)) }));
  const reuseRows = visuals?.tunnelReusePaths?.length
    ? visuals.tunnelReusePaths
    : users.slice(0, 5).map((item) => [item.ip, protocolLabel(item.protocol), '代理待归因', item.dstIp || '未返回目的端点']);
  const sourceProtocolRows = tunnelSourceTop5.map((item) => [
    item.ip,
    protocolLabel(item.protocol),
    String(item.count),
    bytesLabelCompact(item.totalBytes),
  ]);
  const uniqueSourceDestinations = new Set(users.map((item) => item.dstIp).filter(Boolean)).size;
  const highRiskSourceCount = users.filter((item) => /高|high|critical/iu.test(item.risk)).length;
  const sourceTrafficBytes = users.reduce((sum, item) => sum + item.totalBytes, 0);
  const destinationTrafficBars = (visuals?.destinationDistribution ?? [])
    .slice()
    .sort((left, right) => right.trafficGb - left.trafficGb)
    .slice(0, 5)
    .map((item) => ({
      label: item.label,
      value: Number(item.trafficGb.toFixed(2)),
    }));
  const destinationEndpointTotal = (visuals?.destinationDistribution ?? []).reduce((sum, item) => sum + item.value, 0);
  const destinationTrafficTotal = (visuals?.destinationDistribution ?? []).reduce((sum, item) => sum + item.trafficGb, 0);
  const destinationConcentration = destinationEndpointTotal
    ? Number((((visuals?.destinationDistribution?.[0]?.value ?? 0) / destinationEndpointTotal) * 100).toFixed(1))
    : 0;
  const destinationAsnCount = new Set((visuals?.destinationDistribution ?? []).map((item) => item.asn).filter(Boolean)).size;
  const tabs = [
    { id: 'protocol', label: '协议分析' },
    { id: 'source', label: '隧道源' },
    { id: 'destination', label: '端点国家分布' },
  ] as const;
  const [activeTab, setActiveTab] = useState<(typeof tabs)[number]['id']>('protocol');
  return (
    <WorkPanel
      title="加密隧道分析"
      className="taf-topic-tunnel-analysis"
      extra={(
        <span className="taf-topic-tunnel-analysis-tabs">
          {tabs.map((item) => (
            <button key={item.id} type="button" role="tab" className={activeTab === item.id ? 'is-active' : ''} aria-selected={activeTab === item.id} data-tab-id={item.id} onClick={() => setActiveTab(item.id)}>{item.label}</button>
          ))}
        </span>
      )}
    >
      <div
        className="taf-topic-tunnel-analysis-grid"
        data-active-tab={activeTab}
        data-tab-geometry-contract="fixed-within-viewport"
      >
        {activeTab === 'protocol' && (
          <>
            <section className="taf-topic-tunnel-card is-protocol">
              <header><strong>协议</strong><span>{completeness ? `${completeness}% 证据完整` : '证据完整度待接口返回'}</span></header>
              {protocolDistribution.length ? (
                <div className="taf-topic-tunnel-protocol-body">
                  <ExfilPieChart items={protocolDistribution} ariaLabel="加密隧道协议占比" />
                  <div className="taf-topic-tunnel-protocol-table">
                    {protocolRows.map((row) => (
                      <span key={row.label} title={`${row.label} ${row.percent} ${row.traffic}`}>
                        <b>{row.label}</b><em>{row.percent}</em><strong>{row.traffic}</strong>
                      </span>
                    ))}
                  </div>
                </div>
              ) : <div className="taf-topic-empty">当前时间窗没有协议分布数据</div>}
            </section>
            <section className="taf-topic-tunnel-card is-source">
              <header><strong>高频隧道源 TOP5</strong><span>真实流量 (GB)</span></header>
              {topSources.length ? <ExfilBarChart items={topSources} ariaLabel="高频隧道源 TOP5" /> : <div className="taf-topic-empty">暂无源资产流量</div>}
            </section>
            <section className="taf-topic-tunnel-card is-asn">
              <header><strong>端点国家 / ASN TOP5</strong><span>流量 (GB)</span></header>
              <div className="taf-topic-tunnel-mini-table is-asn">
                <b>国家/地区</b><b>总端数</b><b>ASN</b><b>流量</b>
                {endpointRows.flatMap((row) => row.map((cell, index) => <span key={`${row[0]}-${index}`} title={cell}>{cell}</span>))}
              </div>
            </section>
            <section className="taf-topic-tunnel-card is-trend">
              <header>
                <strong>{visuals?.tunnelTrendUnit ? `最近命中流量 (${visuals.tunnelTrendUnit})` : '最近命中趋势'}</strong>
                <span>{visuals?.tunnelTrendUnit ? '接口趋势序列' : '单位待接口声明'}</span>
              </header>
              {trend.length ? <ExfilLineChart points={trend} ariaLabel="隧道源最近命中流量" /> : <div className="taf-topic-empty">暂无时间序列</div>}
            </section>
            <section className="taf-topic-tunnel-card is-ja3">
              <header><strong>JA3 / 证书异常疑点</strong><span>示例</span></header>
              {(visuals?.certificateAnomalies ?? []).length ? (
                <div className="taf-topic-tunnel-mini-table is-ja3">
                  <b>类型</b><b>数量</b><b>占比</b><b>示例</b>
                  {(visuals?.certificateAnomalies ?? []).flatMap((item) => [
                    item.label,
                    String(item.value),
                    `${(item.percent ?? 0).toFixed(1)}%`,
                    item.sample || '待补证',
                  ].map((cell, index) => <span key={`${item.label}-${index}`} title={cell}>{cell}</span>))}
                </div>
              ) : <div className="taf-topic-empty">当前接口没有返回 JA3 / 证书异常记录</div>}
            </section>
            <TunnelReuseCard rows={reuseRows} />
          </>
        )}
        {activeTab === 'source' && (
          <>
            <section className="taf-topic-tunnel-card is-source taf-topic-high-risk-users" data-business-view="source-traffic" data-api-source="tunnel_users.total_bytes">
              <header><strong>隧道源流量 TOP5</strong><span>按 API 流量排序</span></header>
              {tunnelSourceBars.length ? <ExfilBarChart items={tunnelSourceBars} ariaLabel="隧道源流量 TOP5" /> : <div className="taf-topic-empty">当前接口没有返回隧道源</div>}
            </section>
            <section className="taf-topic-tunnel-card is-reuse" data-business-view="source-evidence" data-api-source="tunnel_users">
              <header><strong>隧道源证据 TOP5</strong><span>源 / 风险 / 会话 / 目的端</span></header>
              {tunnelSourceTop5.length ? (
                <div className="taf-topic-tunnel-reuse">
                  {tunnelSourceTop5.map((item) => (
                    <span key={`${item.ip}-${item.dstIp}-${item.protocol}`} title={`${item.ip} ${item.risk} ${item.count} ${item.dstIp}`}>
                      <b>{item.ip}<i /></b><b>{item.risk || '高危'}<i /></b><b>{item.count} 会话<i /></b><b>{item.dstIp || '目的端待归因'}</b>
                    </span>
                  ))}
                </div>
              ) : <div className="taf-topic-empty">暂无隧道源证据</div>}
            </section>
            <section className="taf-topic-tunnel-card is-source-detail" data-business-view="source-protocol" data-api-source="tunnel_users.protocol">
              <header><strong>隧道源协议与会话</strong><span>源 / 协议 / 会话 / 流量</span></header>
              {sourceProtocolRows.length ? (
                <div className="taf-topic-tunnel-mini-table is-source-detail">
                  <b>隧道源</b><b>协议</b><b>会话</b><b>流量</b>
                  {sourceProtocolRows.flatMap((row) => row.map((cell, index) => <span key={`${row[0]}-source-${index}`} title={cell}>{cell}</span>))}
                </div>
              ) : <div className="taf-topic-empty">暂无隧道源协议明细</div>}
            </section>
            <section className="taf-topic-tunnel-card is-derived" data-business-view="source-summary" data-api-source="tunnel_users:derived">
              <header><strong>隧道源调查摘要</strong><span>API 派生</span></header>
              <div className="taf-topic-derived-metrics">
                <span><b>源资产</b><strong>{users.length}</strong><small>当前调查范围</small></span>
                <span><b>高风险源</b><strong>{highRiskSourceCount}</strong><small>风险等级归类</small></span>
                <span><b>关联目的端</b><strong>{uniqueSourceDestinations}</strong><small>去重目的地址</small></span>
                <span><b>累计流量</b><strong>{bytesLabelCompact(sourceTrafficBytes)}</strong><small>源侧流量汇总</small></span>
              </div>
            </section>
          </>
        )}
        {activeTab === 'destination' && (
          <>
            <section className="taf-topic-tunnel-card is-asn" data-business-view="destination-count" data-api-source="destination_distribution.value">
              <header><strong>端点国家分布 TOP5</strong><span>API 地域聚合</span></header>
              {destinationDistributionBars.length ? <ExfilBarChart items={destinationDistributionBars} ariaLabel="端点国家分布 TOP5" /> : <div className="taf-topic-empty">暂无端点国家分布</div>}
            </section>
            <section className="taf-topic-tunnel-card is-source" data-business-view="destination-traffic" data-api-source="destination_distribution.traffic_gb">
              <header><strong>国家端点流量 TOP5</strong><span>traffic_gb 聚合</span></header>
              {destinationTrafficBars.length ? <ExfilBarChart items={destinationTrafficBars} ariaLabel="国家端点流量 TOP5" /> : <div className="taf-topic-empty">暂无国家端点流量</div>}
            </section>
            <section className="taf-topic-tunnel-card is-asn" data-business-view="destination-asn" data-api-source="destination_distribution">
              <header><strong>端点国家 / ASN TOP5</strong><span>端点数 / ASN / 流量</span></header>
              <div className="taf-topic-tunnel-mini-table is-asn">
                <b>国家/地区</b><b>总端数</b><b>ASN</b><b>流量</b>
                {endpointRows.flatMap((row) => row.map((cell, index) => <span key={`${row[0]}-tab-${index}`} title={cell}>{cell}</span>))}
              </div>
            </section>
            <section className="taf-topic-tunnel-card is-derived" data-business-view="destination-summary" data-api-source="destination_distribution:derived">
              <header><strong>跨境端点集中度</strong><span>API 派生</span></header>
              <div className="taf-topic-derived-metrics">
                <span><b>端点总数</b><strong>{destinationEndpointTotal}</strong><small>地域端点汇总</small></span>
                <span><b>国家/地区</b><strong>{destinationDistributionBars.length}</strong><small>当前调查范围</small></span>
                <span><b>ASN 数</b><strong>{destinationAsnCount}</strong><small>去重网络归属</small></span>
                <span><b>TOP1 集中度</b><strong>{destinationConcentration}%</strong><small>首位地域占比</small></span>
                <span><b>聚合流量</b><strong>{destinationTrafficTotal.toFixed(1)} GB</strong><small>跨境流量汇总</small></span>
              </div>
            </section>
          </>
        )}
      </div>
    </WorkPanel>
  );
}

function TunnelReuseCard({ rows }: { rows: string[][] }) {
  return (
    <section className="taf-topic-tunnel-card is-reuse">
      <header><strong>隧道复用路径</strong><span>源主机 / 协议 / 目的端点</span></header>
      {rows.length ? (
        <div className="taf-topic-tunnel-reuse">
          {rows.map((row) => (
            <span key={row.join('-')} title={row.join(' -> ')}>
              {row.map((cell, index) => (
                <b key={`${cell}-${index}`}>
                  {cell}
                  {index < row.length - 1 ? <ArrowRightOutlined aria-hidden="true" /> : null}
                </b>
              ))}
            </span>
          ))}
        </div>
      ) : <div className="taf-topic-empty">暂无可复用路径</div>}
    </section>
  );
}

function TunnelTableToolbar({
  evidenceType,
  phase,
  risk,
  query,
  evidenceOptions,
  phaseOptions,
  riskOptions,
  onEvidenceTypeChange,
  onPhaseChange,
  onRiskChange,
  onQueryChange,
}: {
  evidenceType: string;
  phase: string;
  risk: string;
  query: string;
  evidenceOptions: string[];
  phaseOptions: string[];
  riskOptions: string[];
  onEvidenceTypeChange: (value: string) => void;
  onPhaseChange: (value: string) => void;
  onRiskChange: (value: string) => void;
  onQueryChange: (value: string) => void;
}) {
  return (
    <span className="taf-topic-tunnel-table-toolbar">
      <Select aria-label="证据类型筛选" size="small" value={evidenceType} onChange={onEvidenceTypeChange} options={['全部', ...evidenceOptions].map((value) => ({ value, label: `证据：${value}` }))} />
      <Select aria-label="阶段筛选" size="small" value={phase} onChange={onPhaseChange} options={['全部', ...phaseOptions].map((value) => ({ value, label: `阶段：${value}` }))} />
      <Select aria-label="风险等级筛选" size="small" value={risk} onChange={onRiskChange} options={['全部', ...riskOptions].map((value) => ({ value, label: `风险：${value}` }))} />
      <Input aria-label="搜索隧道证据" size="small" allowClear prefix={<SearchOutlined />} value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="搜索证据" />
    </span>
  );
}

function TunnelEvidenceSection({ rows, isLoading, total, topic, displayTopicId }: { rows: SnapshotRow[]; isLoading: boolean; total?: number; topic: string; displayTopicId: string }) {
  const [evidenceType, setEvidenceType] = useState('全部');
  const [phase, setPhase] = useState('全部');
  const [risk, setRisk] = useState('全部');
  const [query, setQuery] = useState('');
  const values = (column: string) => [...new Set(rows.map((row) => rowText(row, column)).filter(Boolean))];
  const filteredRows = rows.filter((row) => {
    if (evidenceType !== '全部' && !rowText(row, '证据类型').includes(evidenceType)) return false;
    if (phase !== '全部' && rowText(row, '阶段') !== phase) return false;
    if (risk !== '全部' && rowText(row, '风险状态') !== risk) return false;
    return !query || Object.values(row).some((value) => String(value).toLowerCase().includes(query.toLowerCase()));
  });
  const filtered = evidenceType !== '全部' || phase !== '全部' || risk !== '全部' || Boolean(query);
  return (
    <WorkPanel
      title={`加密隧道关联事件与证据 / topic: ${displayTopicId}`}
      className="taf-topic-table-panel taf-topic-tunnel-table-panel"
      extra={(
        <TunnelTableToolbar
          evidenceType={evidenceType}
          phase={phase}
          risk={risk}
          query={query}
          evidenceOptions={values('证据类型')}
          phaseOptions={values('阶段')}
          riskOptions={values('风险状态')}
          onEvidenceTypeChange={setEvidenceType}
          onPhaseChange={setPhase}
          onRiskChange={setRisk}
          onQueryChange={setQuery}
        />
      )}
    >
      <TunnelEvidenceTable rows={filteredRows} isLoading={isLoading} total={filtered ? filteredRows.length : total} topic={topic} />
    </WorkPanel>
  );
}

function TunnelEvidenceTable({ rows, isLoading, total, topic }: { rows: SnapshotRow[]; isLoading: boolean; total?: number; topic: string }) {
  const events = rows.map((row, index) => ({
    key: `${rowText(row, '事件ID') || 'tunnel'}-${index}`,
    id: rowText(row, '事件ID') || '-',
    source: rowText(row, '隧道源') || rowText(row, '源资产') || '-',
    protocol: protocolLabel(rowText(row, '协议') || rowText(row, '协议族')),
    destination: rowText(row, '目的端点') || rowText(row, '目标对象') || '-',
    evidenceType: rowText(row, '证据类型') || 'Session',
    timeRange: rowText(row, '时间窗') || '-',
    actions: ['PCAP', 'Session', '证书', '回溯路径', '审计日志'],
  }));

  const pageSize = 10;
  const pagedEvents = events;
  const [page, setPage] = useState(1);
  const pageCount = Math.max(1, Math.ceil(pagedEvents.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const visibleEvents = pagedEvents.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const pageTokens = compactPaginationTokens(pageCount, currentPage);

  return (
    <div className="taf-topic-tunnel-table" aria-busy={isLoading} data-column-separators="visible">
      <div className="taf-topic-tunnel-table-head">
        {['事件ID', '隧道源', '协议', '目的端点', '证据类型', '时间窗', '风险操作'].map((label) => <b key={label}>{label}</b>)}
      </div>
      <div className="taf-topic-tunnel-table-body">
        {!isLoading && !visibleEvents.length ? <div className="taf-topic-empty">当前时间窗没有真实隧道事件</div> : visibleEvents.map((row) => (
          <div key={row.key} className="taf-topic-tunnel-table-row">
            <span title={row.id}>{row.id}</span>
            <span title={row.source}>{row.source}</span>
            <span title={row.protocol}>{row.protocol}</span>
            <span title={row.destination}>{row.destination}</span>
            <span title={row.evidenceType}>{row.evidenceType}</span>
            <span title={row.timeRange}>{row.timeRange}</span>
            <span className="taf-topic-tunnel-evidence-tags">
              {row.actions.map((item) => (
                <TopicActionButton key={item} topic={topic} title={item} target={row.id}>{item}</TopicActionButton>
              ))}
            </span>
          </div>
        ))}
      </div>
      <div className="taf-topic-tunnel-table-footer">
        <span>共 {total ?? pagedEvents.length} 条</span>
        <button type="button" aria-label="隧道证据上一页" title="上一页" disabled={currentPage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>‹</button>
        {pageTokens.map((value) => typeof value === 'number' ? (
          <button key={value} type="button" className={currentPage === value ? 'is-active' : ''} aria-current={currentPage === value ? 'page' : undefined} title={`第 ${value} 页`} onClick={() => setPage(value)}>{value}</button>
        ) : <i key={value} aria-hidden="true">…</i>)}
        <button type="button" aria-label="隧道证据下一页" title="下一页" disabled={currentPage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>›</button>
        <span>{pageSize} 条/页</span>
      </div>
    </div>
  );
}

function TunnelRightRail({
  config,
  metrics,
  evidenceRows,
  visuals,
  reportBinding,
}: {
  config: TopicConfig;
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
  visuals?: TopicVisuals;
  reportBinding: TopicReportSnapshotBinding;
}) {
  const completeness = Math.round(metricValueNumber(metrics, '证据完整度'));
  const evidence = evidenceRows;
  const summary = [
    ['可生成报告', String(visuals?.summary?.reportable_count ?? (completeness > 0 ? 1 : 0)), completeness > 0 ? 'ok' : 'warn'],
    ['待补证据', String(visuals?.summary?.pending_evidence_count ?? evidence.filter((item) => item.status === 'warn').length), 'warn'],
    ['未闭环风险', String(visuals?.summary?.open_risk_count ?? metricValueNumber(metrics, '未闭环风险数')), 'risk'],
  ];
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出总报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['试点周报导出', <ExportOutlined key="export" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];

  return (
    <>
      <WorkPanel title={`专题交付摘要 / ${config.displayTopicId ?? config.topicCode}`} className="taf-topic-tunnel-delivery">
        <small className="taf-topic-tunnel-delivery-subtitle">(加密隧道专题)</small>
        <div className="taf-topic-tunnel-delivery-grid" data-responsive-summary-contract="ring-legend-values-container-proportional">
          <TopicProgressDonut
            value={completeness}
            ariaLabel="加密隧道报告就绪度"
            className="taf-topic-tunnel-ring"
            caption="报告就绪度"
            detail="较昨日 +8%"
          />
          <div className="taf-topic-tunnel-delivery-stats">
            {summary.map(([label, value, tone]) => (
              <span key={label} className={`is-${tone}`} title={`${label}: ${value}`}>
                <i />
                <b>{label}</b>
                <strong>{value}</strong>
              </span>
            ))}
          </div>
        </div>
        <div className="taf-topic-tunnel-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出报告" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-report-export" reportBinding={reportBinding}><DownloadOutlined />导出报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-evidence-package-export"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm" reportBinding={reportBinding}><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / 加密隧道专题" className="taf-topic-tunnel-evidence-panel">
        <div className="taf-topic-tunnel-evidence-list">
          {evidence.map((item) => (
            <span key={item.label} className={`is-${item.status}`} title={`${item.label}: ${item.value}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="报告预览 / 当前保存视图" className="taf-topic-tunnel-report-panel">
        <div className="taf-topic-report-preview taf-topic-tunnel-report-preview">
          <TopicReportThumbnail title={visuals?.presentation?.reportTitle || '加密隧道专题_试点周报'} topicId={config.displayTopicId ?? config.topicCode} completeness={completeness} />
          <div>
            <strong>报告类型：{visuals?.presentation?.reportTitle || '加密隧道专题_试点周报'}</strong>
            <span>时间窗：{visuals?.presentation?.reportTimeRange || config.timeRange}</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：{visuals?.presentation?.reportGeneratedAt || '提交导出任务时生成'}</span>
            <TopicReportPreviewButton topic={config.topicCode} config={config} visuals={visuals} reportBinding={reportBinding} />
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-tunnel-action-panel">
        <div className="taf-topic-exfil-action-grid taf-topic-tunnel-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)} target={config.displayTopicId ?? config.topicCode} overlayId={topicRailOverlayId(String(label))} reportBinding={reportBinding}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function buildTunnelTopSources(rows: SnapshotRow[]): ExfilBarItem[] {
  const totals = new Map<string, number>();
  rows.forEach((row) => {
    const label = rowText(row, '隧道源') || rowText(row, '源资产');
    const totalBytes = rowNumber(row, '__total_bytes');
    if (!/^\d{1,3}(?:\.\d{1,3}){3}$/.test(label) || totalBytes <= 0) return;
    totals.set(label, (totals.get(label) ?? 0) + totalBytes);
  });
  return [...totals.entries()]
    .map(([label, totalBytes]) => ({ label, value: totalBytes / (1024 ** 3) }))
    .sort((left, right) => right.value - left.value)
    .slice(0, 5);
}

function TopicCanvas({
  topicId,
  rows,
  metrics,
  visuals,
}: {
  topicId: TopicId;
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  visuals?: TopicVisuals;
}) {
  if (topicId === 'topic-exfil') return <ExfilCanvas rows={rows} metrics={metrics} visuals={visuals} />;
  if (topicId === 'topic-apt') return <AptCanvas rows={rows} metrics={metrics} visuals={visuals} />;
  return <TunnelImpactMap rows={rows} />;
}

function ExfilRightRail({
  config,
  metrics,
  evidenceRows,
  visuals,
  reportBinding,
}: {
  config: TopicConfig;
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
  visuals?: TopicVisuals;
  reportBinding: TopicReportSnapshotBinding;
}) {
  const completeness = Math.max(0, Math.min(100, Math.round(metricValueNumber(metrics, '证据完整度'))));
  const evidence = evidenceRows;
  const summaryStats: Array<[string, string]> = [
    ['可生成报告', String(visuals?.summary?.reportable_count ?? (metricValueNumber(metrics, '外传路径数') > 0 ? 1 : 0))],
    ['待补证据', String(visuals?.summary?.pending_evidence_count ?? evidence.filter((item) => item.status === 'warn').length)],
    ['未闭环风险', String(visuals?.summary?.open_risk_count ?? metricValueNumber(metrics, '外传预警量'))],
  ];
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出总报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['试点周报导出', <ExportOutlined key="export" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];

  return (
    <>
      <WorkPanel title={`专题交付摘要 / ${config.displayTopicId ?? config.topicCode}`} className="taf-topic-exfil-delivery">
        <div className="taf-topic-exfil-delivery-grid" data-responsive-summary-contract="ring-legend-values-container-proportional">
          <TopicProgressDonut
            value={completeness}
            ariaLabel="数据外传报告就绪度"
            className="taf-topic-exfil-delivery-ring"
            caption="实时证据计算"
          />
          <div className="taf-topic-exfil-delivery-stats">
            {summaryStats.map(([label, value]) => (
              <span key={label}>
                <i />
                <b>{label}</b>
                <strong>{value}</strong>
              </span>
            ))}
          </div>
        </div>
        <div className="taf-topic-exfil-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出总报告" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-report-export" reportBinding={reportBinding}><DownloadOutlined />导出总报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-evidence-package-export"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm" reportBinding={reportBinding}><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / 数据外传专题" className="taf-topic-exfil-evidence">
        <div className="taf-topic-exfil-evidence-list">
          {evidence.slice(0, 6).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="报告预览 / 当前保存视图" className="taf-topic-exfil-report">
        <div className="taf-topic-report-preview">
          <TopicReportThumbnail title={config.reportTitle} topicId={config.displayTopicId ?? config.topicCode} completeness={completeness} />
          <div>
            <strong>{config.reportTitle}</strong>
            <span>时间窗：{config.timeRange}</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：{visuals?.presentation?.reportGeneratedAt || '提交导出任务时生成'}</span>
            <TopicReportPreviewButton topic={config.topicCode} config={config} visuals={visuals} reportBinding={reportBinding} />
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-exfil-action-panel">
        <div className="taf-topic-exfil-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)} target={config.displayTopicId ?? config.topicCode} overlayId={topicRailOverlayId(String(label))} reportBinding={reportBinding}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function ExfilEvidenceSection({
  title,
  rows,
  columns,
  rowKeyColumn,
  isLoading,
}: {
  title: string;
  rows: SnapshotRow[];
  columns: ColumnsType<SnapshotRow>;
  rowKeyColumn: string;
  isLoading: boolean;
}) {
  const [risk, setRisk] = useState('全部');
  const [protocol, setProtocol] = useState('全部');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const tableHostRef = useRef<HTMLDivElement>(null);
  const [tableBodyHeight, setTableBodyHeight] = useState(280);
  const riskOptions = [...new Set(rows.map((row) => rowText(row, '风险等级')).filter(Boolean))];
  const protocolOptions = [...new Set(rows.map((row) => rowText(row, '协议')).filter(Boolean))];
  const filteredRows = rows.filter((row) => {
    if (risk !== '全部' && rowText(row, '风险等级') !== risk) return false;
    if (protocol !== '全部' && rowText(row, '协议') !== protocol) return false;
    return !query || Object.values(row).some((value) => String(value).toLowerCase().includes(query.toLowerCase()));
  });
  useEffect(() => setPage(1), [risk, protocol, query]);
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / 10));
  useEffect(() => setPage((current) => Math.min(current, pageCount)), [pageCount]);
  useEffect(() => {
    const host = tableHostRef.current;
    if (!host || typeof ResizeObserver === 'undefined') return undefined;
    let frame = 0;
    const updateHeight = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        // Header and fixed pagination remain outside the scroll owner. Ten
        // compact 28px rows fit when the panel has enough room; otherwise only
        // the tbody scrolls.
        setTableBodyHeight(Math.max(96, Math.min(280, Math.floor(host.clientHeight - 70))));
      });
    };
    const observer = new ResizeObserver(updateHeight);
    observer.observe(host);
    updateHeight();
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
    };
  }, []);
  return (
    <WorkPanel
      title={title}
      className="taf-topic-table-panel taf-topic-exfil-table-panel"
      extra={(
        <span className="taf-topic-tunnel-table-toolbar">
          <Select aria-label="外传风险筛选" size="small" value={risk} onChange={setRisk} options={['全部', ...riskOptions].map((value) => ({ value, label: `风险：${value}` }))} />
          <Select aria-label="外传协议筛选" size="small" value={protocol} onChange={setProtocol} options={['全部', ...protocolOptions].map((value) => ({ value, label: `协议：${value}` }))} />
          <Input aria-label="搜索外传事件" size="small" allowClear prefix={<SearchOutlined />} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索外传事件" />
        </span>
      )}
    >
      <div className="taf-topic-exfil-table-host" ref={tableHostRef} data-page-size="10" data-scroll-owner="tbody" data-column-separators="visible">
        <Table
          rowKey={(record) => String(record[rowKeyColumn] ?? JSON.stringify(record))}
          size="small"
          loading={isLoading}
          columns={columns}
          dataSource={filteredRows}
          pagination={{
            current: page,
            pageSize: 10,
            size: 'small',
            showSizeChanger: false,
            showTotal: (value) => `共 ${value} 条`,
            onChange: setPage,
          }}
          scroll={{ x: 1180, y: tableBodyHeight }}
        />
      </div>
    </WorkPanel>
  );
}

function ExfilCanvas({ rows, metrics, visuals }: { rows: SnapshotRow[]; metrics: SnapshotMetric[]; visuals?: TopicVisuals }) {
  const [selectedNode, setSelectedNode] = useState('');
  const model = buildExfilVisualModel(rows, metrics, visuals);
  const topologyNodes: TopicTopologyNode[] = (visuals?.topologyNodes ?? [])
    .map((node) => ({
      id: node.id, label: node.label, detail: node.detail, x: node.x, y: node.y,
      tone: node.tone, size: [node.width, node.height], symbol: node.symbol, icon: node.icon,
      labelPosition: node.labelPosition, selected: selectedNode === node.id,
    }));
  const topologyLinks: TopicTopologyLink[] = (visuals?.topologyLinks ?? [])
    .map((link) => ({
      source: link.source, target: link.target, value: link.value, tone: link.tone,
      lineType: link.lineType, label: link.label, width: link.width, curveness: link.curveness,
      selected: selectedNode !== '' && (link.source === selectedNode || link.target === selectedNode),
    }));
  const selectNode = (nodeID: string) => setSelectedNode((current) => current === nodeID ? '' : nodeID);
  return (
    <div className="taf-topic-canvas taf-topic-exfil-canvas">
      <div className="taf-topic-canvas-legend">
        {['内部源', '文件服务', '代理/中转', '外部目的地', '风险路径', '实线：确认', '虚线：推断'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <div className="taf-topic-sankey">
        <div className="taf-topic-exfil-stage-headings" aria-label="数据外传路径分层">
          {['内部源资产', '文件服务 / 共享', '代理 / 中转节点', '外部目的地（按国家）', '风险路径（TOP）']
            .map((item) => <span key={item}>{item}</span>)}
        </div>
        {topologyNodes.length && topologyLinks.length ? (
          <TopicTopologyGraph
            nodes={topologyNodes}
            links={topologyLinks}
            ariaLabel="数据外传路径动态关系图"
            onNodeClick={selectNode}
          />
        ) : (
          <Alert type="error" showIcon message="数据外传关系图缺少 API 拓扑数据" description="当前快照必须返回 topology_nodes 和 topology_links。" />
        )}
        <div className="taf-topic-sankey-summary">
          <span>总外传流量：{model.totalUploadGb.toFixed(1)} GB</span>
          <span>涉及路径：{model.pathCount}</span>
          <span>闭环可信度：{model.confidence}%</span>
        </div>
      </div>
    </div>
  );
}

function ExfilAnalysisDashboard({ rows, metrics, focusMode, visuals }: { rows: SnapshotRow[]; metrics: SnapshotMetric[]; focusMode: string; visuals?: TopicVisuals }) {
  const model = buildExfilVisualModel(rows, metrics, visuals);

  return (
    <WorkPanel
      title="数据外传分析"
      className="taf-topic-analysis-panel taf-topic-exfil-analysis-panel"
      extra={<span className="taf-topic-focus">多维研判</span>}
    >
      <div
        className="taf-topic-exfil-dashboard"
        aria-label="数据外传分析"
        data-analysis-module-contract="destination-sensitive-trend-protocol-confidence"
      >
        <div className="taf-topic-exfil-card is-table" data-layout-slot="destination">
          <header>
            <strong>目的地国家 / ASN TOP 5</strong>
            <span>{focusMode}</span>
          </header>
          <div className="taf-topic-exfil-table">
            <b>目的地址</b><b>归因</b><b>流量 GB</b><b>占比</b>
            {model.destinationRows.map((row) => [
              <span key={`${row.region}-region`}>{row.region}</span>,
              <span key={`${row.region}-asn`}>{row.asn}</span>,
              <span key={`${row.region}-traffic`}>{row.traffic}</span>,
              <span key={`${row.region}-ratio`}>{row.ratio}</span>,
            ])}
          </div>
        </div>

        <ExfilDistributionCard title={model.distributionTitle} items={model.sensitiveTypes} slot="sensitive" />

        <div className="taf-topic-exfil-card is-trend" data-layout-slot="trend">
          <header>
            <strong>异常上传峰值趋势</strong>
            <span>峰值 {Math.max(0, ...model.trend.map((point) => point.value)).toFixed(0)} 会话</span>
          </header>
          <ExfilLineChart points={model.trend} ariaLabel="异常上传峰值趋势" />
        </div>

        <ExfilDistributionCard title="外传协议占比" items={model.protocols} slot="protocol" />

        <div className="taf-topic-exfil-card is-score" data-layout-slot="confidence">
          <header>
            <strong>路径置信度评分</strong>
            <span>{model.pathCount} 条路径</span>
          </header>
          <TopicProgressDonut
            value={model.confidence}
            ariaLabel="数据外传路径置信度"
            className="taf-topic-exfil-score-ring"
            caption="路径置信度"
          />
        </div>

        <div className="taf-topic-exfil-card is-bars" data-layout-slot="account-service">
          <header>
            <strong>可疑账号 / 服务分布 TOP 5</strong>
            <span>命中 {model.accounts.reduce((sum, item) => sum + item.value, 0)}</span>
          </header>
          {model.accounts.length
            ? <ExfilBarChart items={model.accounts} ariaLabel="可疑账号和服务分布 TOP 5" />
            : <div className="taf-topic-empty">account_service_distribution 未返回</div>}
        </div>
      </div>
    </WorkPanel>
  );
}

function ExfilDistributionCard({
  title,
  items,
  slot,
}: {
  title: string;
  items: ExfilDistributionItem[];
  slot: 'sensitive' | 'protocol';
}) {
  const total = items.reduce((sum, item) => sum + item.value, 0);
  const primaryValue = total ? Math.round((items[0]?.value ?? 0) / total * 100) : 0;
  return (
    <div className="taf-topic-exfil-card is-donut" data-layout-slot={slot}>
      <header>
        <strong>{title}</strong>
        <span>{primaryValue}%</span>
      </header>
      <div className="taf-topic-exfil-donut-layout">
        <ExfilPieChart items={items} ariaLabel={title} center={['50%', '50%']} radius={['43%', '76%']} />
        <div className="taf-topic-exfil-legend">
          {items.map((item) => (
            <span key={item.label} style={{ '--color': item.color } as CSSProperties}>
              <b>{item.label}</b>
              <em>{item.value.toFixed(1)}%</em>
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

function buildExfilVisualModel(rows: SnapshotRow[], metrics: SnapshotMetric[], visuals?: TopicVisuals): ExfilVisualModel {
  const sourceRows = rows;
  const uploadByType = groupRows(sourceRows, '数据类型', '上传量');
  const uploadByDestination = groupRows(sourceRows, '目标区域', '上传量');
  const uploadByRisk = groupRows(sourceRows, '风险类型', '上传量');
  const sessionsBySource = groupRows(sourceRows, '源资产', '会话数');
  const totalUploadGb = visuals?.summary?.upload_bytes !== undefined
    ? visuals.summary.upload_bytes / (1024 ** 3)
    : sourceRows.reduce((sum, row) => sum + rowUploadGb(row), 0);
  const pathCount = visuals?.summary?.path_count ?? metricNumber(metrics, '外传路径数');
  const pathConfidence = visuals?.summary?.path_confidence;
  const confidence = Math.max(0, Math.min(100, Math.round(
    pathConfidence ?? metricNumber(metrics, '证据完整度'),
  )));
  const topSources = sessionsBySource.slice(0, 5);
  const classifiedTypes = uploadByType.filter((item) => !['-', '未知'].includes(item.label));
  const topTypes = classifiedTypes.slice(0, 5);
  const visualRiskTypes = (visuals?.exfilRiskTypes ?? []).map((item) => ({
    label: riskTypeLabel(item.type),
    value: item.totalBytes > 0 ? item.totalBytes / (1024 ** 3) : item.count,
  })).filter((item) => item.value > 0);
  const visualRiskLabels = new Set(visualRiskTypes.map((item) => item.label));
  const topRiskTypes = [
    ...visualRiskTypes,
    ...uploadByRisk.filter((item) => !visualRiskLabels.has(item.label)),
  ].slice(0, 5);
  const liveDistribution = topTypes.length ? topTypes : topRiskTypes;
  const distributionTitle = topTypes.length ? '敏感数据类型分布' : '风险类型分布';
  const hasClassifiedTypes = topTypes.length > 0;
  const riskDepth = hasClassifiedTypes ? 2 : 1;
  const destinationDepth = riskDepth + 1;
  const pathDepth = destinationDepth + 1;
  const actualDestinations = (visuals?.exfilDestinations ?? []).map((item) => ({
    // The Sankey must preserve destination identity. Grouping by region merged
    // multiple database destinations into one oversized node and obscured paths.
    label: item.dstIp || item.region,
    region: item.region || item.dstIp,
    value: item.uploadBytes / (1024 ** 3),
    asn: item.asn || '未归因',
  })).filter((item) => item.label && item.value > 0);
  const topDestinations = (actualDestinations.length ? actualDestinations : uploadByDestination)
    .slice()
    .sort((left, right) => right.value - left.value)
    .slice(0, 5);

  const sankeyNodes = [
    ...topSources.map((item) => ({ name: item.label, depth: 0 })),
    ...topTypes.map((item) => ({ name: `类型:${item.label}`, depth: 1 })),
    ...topRiskTypes.map((item) => ({ name: `风险:${item.label}`, depth: riskDepth })),
    ...topDestinations.map((item) => ({ name: item.label, depth: destinationDepth })),
    ...sourceRows.slice(0, 5).map((row, index) => ({ name: `路径-${String(index + 1).padStart(2, '0')}`, depth: pathDepth })),
  ];

  const sankeyLinks: ExfilSankeyLink[] = [];
  const representativeRows = topSources.flatMap((source) =>
    sourceRows.filter((row) => rowText(row, '源资产') === source.label).slice(0, 2));
  (representativeRows.length ? representativeRows : sourceRows.slice(0, 10)).forEach((row, index) => {
    const source = firstMatchingGroup(rowText(row, '源资产'), topSources) || topSources[index % Math.max(1, topSources.length)]?.label;
    const dataType = firstMatchingGroup(rowText(row, '数据类型'), topTypes) || topTypes[index % Math.max(1, topTypes.length)]?.label;
    const riskType = topRiskTypes[index % Math.max(1, topRiskTypes.length)]?.label
      || firstMatchingGroup(rowText(row, '风险类型'), topRiskTypes);
    const destination = topDestinations[index % Math.max(1, topDestinations.length)]?.label
      || firstMatchingGroup(rowText(row, '目标区域'), topDestinations);
    const riskPath = `路径-${String((index % 5) + 1).padStart(2, '0')}`;
    const value = Math.max(1, Math.log2(rowUploadGb(row) + 2) * 2);

    if (source && dataType) sankeyLinks.push({ source, target: `类型:${dataType}`, value });
    if (dataType && riskType) sankeyLinks.push({ source: `类型:${dataType}`, target: `风险:${riskType}`, value: value * 0.88 });
    if (source && !dataType && riskType) sankeyLinks.push({ source, target: `风险:${riskType}`, value });
    if (riskType && destination) sankeyLinks.push({ source: `风险:${riskType}`, target: destination, value: value * 0.78 });
    if (destination) sankeyLinks.push({ source: destination, target: riskPath, value: value * 0.68 });
  });

  const destinationRows = topDestinations.map((item) => ({
    region: 'region' in item && typeof item.region === 'string' ? item.region : item.label,
    asn: 'asn' in item && typeof item.asn === 'string' ? item.asn : '未归因',
    traffic: `${item.value.toFixed(1)}`,
    ratio: percentOf(item.value, totalUploadGb),
  }));

  return {
    sankeyNodes: uniqueSankeyNodes(sankeyNodes),
    sankeyLinks: mergeSankeyLinks(sankeyLinks),
    destinationRows,
    distributionTitle,
    sensitiveTypes: normalizeDistribution(liveDistribution, ['#65d86e', '#ffb020', '#ff8a3d', '#ff4d4f', '#b685ff']),
    protocols: buildProtocolDistribution(visuals?.exfilPaths ?? []),
    trend: buildExfilTrend(visuals?.exfilTrend ?? []),
    accounts: (visuals?.exfilAccountServices ?? [])
      .slice()
      .sort((left, right) => right.count - left.count)
      .slice(0, 5)
      .map((item) => ({ label: item.label, value: item.count })),
    confidence,
    totalUploadGb,
    pathCount,
  };
}

function groupRows(rows: SnapshotRow[], labelColumn: string, valueColumn: string) {
  const groups = new Map<string, number>();
  rows.forEach((row) => {
    const label = rowText(row, labelColumn) || '未知';
    const value = valueColumn === '上传量' ? rowUploadGb(row) : rowNumber(row, valueColumn);
    groups.set(label, (groups.get(label) ?? 0) + value);
  });
  return [...groups.entries()]
    .map(([label, value]) => ({ label, value }))
    .filter((item) => item.value > 0)
    .sort((a, b) => b.value - a.value);
}

function rowText(row: SnapshotRow, column: string) {
  const value = row[column];
  return typeof value === 'number' ? String(value) : String(value ?? '').trim();
}

function protocolLabel(value: string) {
  const labels: Record<string, string> = {
    DNS_HIGH_FREQUENCY: 'DNS 高频隧道',
    SSH_LONG_LIVED: 'SSH 长连接',
    QUIC_LONG_LIVED: 'QUIC 长连接',
    TLS_LARGE_LONG_LIVED: 'TLS 大流量长连接',
  };
  return labels[value] ?? (value || '未知协议');
}

function riskTypeLabel(value: string) {
  const labels: Record<string, string> = {
    large_encrypted_upload: '大流量加密上传',
    long_lived_encrypted_session: '长连接加密会话',
    non_standard_encrypted_port: '非标准加密端口',
  };
  return labels[value] ?? (value || '未知风险');
}

function bytesLabelCompact(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  if (value >= 1024 ** 3) return `${(value / (1024 ** 3)).toFixed(2)} GB`;
  if (value >= 1024 ** 2) return `${(value / (1024 ** 2)).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${Math.round(value)} B`;
}

function formatTopicTime(value: number, format: 'MM-DD HH:mm' | 'HH:mm' = 'MM-DD HH:mm') {
  if (!Number.isFinite(value) || value <= 0) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  const hour = String(date.getHours()).padStart(2, '0');
  const minute = String(date.getMinutes()).padStart(2, '0');
  return format === 'HH:mm' ? `${hour}:${minute}` : `${month}-${day} ${hour}:${minute}`;
}

function topicTimeRangeLabel(visuals: TopicVisuals | undefined, fallback: string) {
  const start = visuals?.timeRange.start ?? 0;
  const end = visuals?.timeRange.end ?? 0;
  if (!start || !end) return fallback;
  return `${formatTopicTime(start)} ~ ${formatTopicTime(end)}`;
}

function rowNumber(row: SnapshotRow, column: string) {
  const value = row[column];
  if (typeof value === 'number') return value;
  return parseNumericValue(String(value ?? ''));
}

function rowUploadGb(row: SnapshotRow) {
  const value = row['上传量'];
  if (typeof value === 'number') return value;
  const raw = String(value ?? '');
  const numeric = parseNumericValue(raw);
  if (/tb/i.test(raw)) return numeric * 1024;
  if (/mb/i.test(raw)) return numeric / 1024;
  if (/kb/i.test(raw)) return numeric / (1024 * 1024);
  return numeric;
}

function parseNumericValue(value: string) {
  const match = value.replace(/,/g, '').match(/-?\d+(?:\.\d+)?/);
  return match ? Number(match[0]) : 0;
}

function metricNumber(metrics: SnapshotMetric[], label: string) {
  const metric = metrics.find((item) => item.label === label);
  return metric ? parseNumericValue(metric.value) : 0;
}

function metricValueNumber(metrics: SnapshotMetric[], label: string) {
  return metricNumber(metrics, label);
}

function compactPaginationTokens(pageCount: number, currentPage: number): Array<number | string> {
  if (pageCount <= 7) return Array.from({ length: pageCount }, (_, index) => index + 1);
  const pages = new Set([1, pageCount, currentPage - 1, currentPage, currentPage + 1]);
  if (currentPage <= 3) [2, 3, 4].forEach((value) => pages.add(value));
  if (currentPage >= pageCount - 2) [pageCount - 3, pageCount - 2, pageCount - 1].forEach((value) => pages.add(value));
  const sorted = [...pages].filter((value) => value >= 1 && value <= pageCount).sort((a, b) => a - b);
  const tokens: Array<number | string> = [];
  sorted.forEach((value, index) => {
    if (index > 0 && value - sorted[index - 1] > 1) tokens.push(`ellipsis-${sorted[index - 1]}-${value}`);
    tokens.push(value);
  });
  return tokens;
}

function firstMatchingGroup(value: string, groups: Array<{ label: string }>) {
  if (!groups.length) return '';
  return groups.find((item) => item.label === value)?.label ?? groups[0].label;
}

function uniqueSankeyNodes(nodes: ExfilSankeyNode[]) {
  const seen = new Set<string>();
  return nodes.filter((node) => {
    if (seen.has(node.name)) return false;
    seen.add(node.name);
    return true;
  });
}

function mergeSankeyLinks(links: ExfilSankeyLink[]) {
  const merged = new Map<string, ExfilSankeyLink>();
  links.forEach((link) => {
    const key = `${link.source}->${link.target}`;
    const previous = merged.get(key);
    merged.set(key, previous ? { ...previous, value: previous.value + link.value } : { ...link });
  });
  return [...merged.values()].map((link) => ({ ...link, value: Number(link.value.toFixed(2)) }));
}

function normalizeDistribution(groups: Array<{ label: string; value: number }>, colors: string[]): ExfilDistributionItem[] {
  const total = groups.reduce((sum, item) => sum + item.value, 0) || 1;
  return groups
    .slice()
    .sort((left, right) => right.value - left.value)
    .slice(0, 5)
    .map((item, index) => ({
    label: item.label,
    value: Number((item.value / total * 100).toFixed(1)),
    color: colors[index % colors.length],
    }));
}

function buildProtocolDistribution(paths: NonNullable<TopicVisuals['exfilPaths']>): ExfilDistributionItem[] {
  const grouped = new Map<string, number>();
  paths.forEach((path) => {
    const label = path.protocol || (path.dstPort ? `端口 ${path.dstPort}` : '未知协议');
    grouped.set(label, (grouped.get(label) ?? 0) + path.uploadBytes);
  });
  return normalizeDistribution(
    [...grouped.entries()].map(([label, value]) => ({ label, value })).sort((a, b) => b.value - a.value),
    ['#1ea8ff', '#ffb020', '#65d86e', '#ff8a3d', '#b685ff'],
  );
}

function buildExfilTrend(trend: NonNullable<TopicVisuals['exfilTrend']>): ExfilTrendPoint[] {
  return trend
    .filter((item) => item.bucketStart > 0)
    .sort((a, b) => a.bucketStart - b.bucketStart)
    .map((item) => ({
      label: formatTopicTime(item.bucketStart, 'HH:mm'),
      value: item.largeUploadSessions + item.longLivedSessions + item.nonStandardPortSessions,
    }));
}

function percentOf(value: number, total: number) {
  if (!total) return '0.0%';
  return `${(value / total * 100).toFixed(1)}%`;
}

function AptCanvas({
  visuals,
}: {
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  visuals?: TopicVisuals;
}) {
  const [selectedNode, setSelectedNode] = useState('campaign-0');
  const nodes: TopicTopologyNode[] = (visuals?.topologyNodes ?? [])
    .map((node) => ({
      id: node.id, label: node.label, detail: node.detail, x: node.x, y: node.y,
      tone: node.tone, size: [node.width, node.height], symbol: node.symbol, icon: node.icon,
      labelPosition: node.labelPosition, selected: selectedNode === node.id,
    }));
  const links: TopicTopologyLink[] = (visuals?.topologyLinks ?? [])
    .map((link) => ({
      source: link.source, target: link.target, value: link.value, tone: link.tone,
      lineType: link.lineType, label: link.label, width: link.width, curveness: link.curveness,
      selected: selectedNode !== '' && (link.source === selectedNode || link.target === selectedNode),
    }));
  const selectNode = (nodeID: string) => setSelectedNode((current) => current === nodeID ? '' : nodeID);
  return (
    <div className="taf-topic-canvas taf-topic-apt-canvas">
      <div className="taf-topic-canvas-legend">
        {['战役簇', '攻击阶段', '资产/账号', '进程/服务', 'C2/外联', '横向移动', '数据外传', '证据节点'].map((item, index) => <span key={item} className={`tone-${index}`}>{item}</span>)}
      </div>
      <div className="taf-topic-attack-map taf-topic-apt-attack-map">
        <div className="taf-topic-apt-topology-svg">
          {nodes.length && links.length ? (
            <TopicTopologyGraph
              ariaLabel="APT 战役攻击关系图"
              nodes={nodes}
              links={links}
              onNodeClick={selectNode}
              visualProfile="apt-reference"
            />
          ) : (
            <Alert type="error" showIcon message="APT 战役画布缺少 API 拓扑数据" description="当前快照必须返回 topology_nodes 和 topology_links。" />
          )}
        </div>
      </div>
    </div>
  );
}

function AptAnalysisDashboard({
  rows,
  metrics,
  evidenceRows,
  focusMode,
  visuals,
}: {
  rows: SnapshotRow[];
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
  focusMode: string;
  visuals?: TopicVisuals;
}) {
  const model = buildAptVisualModel(rows, metrics, evidenceRows, visuals);
  const analysisTabs = ['ATT&CK阶段覆盖', '战役耗时线', '关键 IoC 命中', '横向移动关系', '处置动作状态', '证据关联强度'];
  const [activeTab, setActiveTab] = useState(analysisTabs[0]);
  const campaignDurationRows = model.campaignDetails.slice(0, 5).map((campaign) => {
    const durationHours = campaign.tsEnd > campaign.tsStart
      ? (campaign.tsEnd - campaign.tsStart) / 3_600_000
      : 0;
    return {
      ...campaign,
      duration: durationHours >= 24 ? `${(durationHours / 24).toFixed(1)} 天` : `${durationHours.toFixed(1)} 小时`,
      timeRange: `${formatTopicTime(campaign.tsStart)} ~ ${formatTopicTime(campaign.tsEnd)}`,
    };
  });
  const iocTypeCounts = [...model.iocs.reduce((groups, item) => {
    groups.set(item.type || '未知', (groups.get(item.type || '未知') ?? 0) + item.hits);
    return groups;
  }, new Map<string, number>()).entries()].map(([label, value]) => ({ label, value }));
  const responseTotal = model.response.reduce((sum, item) => sum + item.value, 0);
  const lateralSummaryCount = Math.round(metricValueNumber(metrics, '横向移动链路'));
  const evidenceGapRows = model.evidenceRows.filter((item) => item.status !== 'ok');
  const timelineEventCount = model.timeline.reduce(
    (sum, point) => sum + point.aptCn + point.tempHawk + point.unknown,
    0,
  );

  return (
    <WorkPanel
      title="战役分析"
      className="taf-topic-apt-analysis-panel"
      extra={<span className="taf-topic-focus">{focusMode}</span>}
    >
      <div className="taf-topic-apt-tabs" role="tablist" aria-label="APT 战役分析维度">
        {analysisTabs.map((item) => (
          <button
            key={item}
            type="button"
            role="tab"
            title={item}
            className={activeTab === item ? 'is-active' : ''}
            aria-selected={activeTab === item}
            onClick={() => setActiveTab(item)}
          >
            {item}
          </button>
        ))}
      </div>
      <div
        className="taf-topic-apt-analysis-grid"
        data-active-tab={activeTab}
        data-tab-geometry-contract="fixed-within-viewport"
      >
        {activeTab === 'ATT&CK阶段覆盖' && <div className="taf-topic-apt-matrix" aria-label="ATT&CK 阶段覆盖矩阵" data-business-view="attack-phase-matrix" data-api-source="campaigns.attack_phases,phase_distribution">
          <b />
          {model.phases.map((phase) => <b key={phase.id}>{phase.id}<small>{phase.label}</small></b>)}
          {model.campaigns.map((campaign) => [
            <strong key={`${campaign.name}-name`}>{campaign.name}</strong>,
            ...model.phases.map((phase) => {
              const campaignDetail = model.campaignDetails.find((item) => item.id === campaign.fullName);
              const hit = campaignDetail
                ? campaignDetail.attackPhases.some((item) => normalizeAptPhase(item) === phase.label)
                : rows.some((row) =>
                  rowText(row, '战役名称') === campaign.fullName
                  && normalizeAptPhase(rowText(row, '阶段')) === phase.label);
              return (
                <span
                  key={`${campaign.name}-${phase.id}`}
                  className={`is-${hit ? (phase.tone === 'risk' ? 'risk' : phase.tone === 'warn' ? 'warn' : 'ok') : 'info'}`}
                  title={`${campaign.name} / ${phase.label}: ${hit ? phase.value : 0}`}
                />
              );
            }),
          ])}
        </div>}

        {(activeTab === 'ATT&CK阶段覆盖' || activeTab === '战役耗时线') && <div
          className="taf-topic-apt-trend"
          aria-label="战役时间线事件数"
          data-business-view="campaign-event-trend"
          data-api-source="events.ts_start,events.campaign_id"
          data-timeline-point-count={model.timeline.length}
          data-visible-event-count={timelineEventCount}
          data-total-event-count={model.eventTotal}
        >
          <header>
            <strong>战役时间线（TOP3 事件数）</strong>
            <span>{timelineEventCount} / 全部 {model.eventTotal}</span>
          </header>
          <DataQualityTrendChart
            ariaLabel="APT 战役事件趋势"
            className="taf-topic-apt-trend-echart"
            categories={model.timeline.map((point) => point.label)}
            series={[
              { name: model.campaigns[0]?.name ?? '战役 1', color: '#ff5b3d', values: model.timeline.map((point) => point.aptCn), area: true },
              { name: model.campaigns[1]?.name ?? '战役 2', color: '#ffb020', values: model.timeline.map((point) => point.tempHawk) },
              { name: model.campaigns[2]?.name ?? '战役 3', color: '#65d86e', values: model.timeline.map((point) => point.unknown), dashed: true },
            ]}
          />
        </div>}

        {(activeTab === 'ATT&CK阶段覆盖' || activeTab === '关键 IoC 命中') && <div className="taf-topic-apt-ioc" aria-label="关键 IoC 命中 TOP5" data-business-view="ioc-top5" data-api-source="iocs">
          <header>
            <strong>关键 IoC 命中 TOP5</strong>
            <span>复盘证据</span>
          </header>
          <div>
            <b title="IoC">IoC</b><b title="类型">类型</b><b title="命中次数">命中次数</b><b title="首次命中">首次命中</b>
            {model.iocs.map((item) => [
              <span key={`${item.ioc}-ioc`} data-column="ioc" title={item.ioc}>{item.ioc}</span>,
              <span key={`${item.ioc}-type`} data-column="type" title={item.type}>{item.type}</span>,
              <span key={`${item.ioc}-hits`} data-column="hits" title={`${item.hits}`}>{item.hits}</span>,
              <span key={`${item.ioc}-time`} data-column="first-seen" title={item.firstSeen}>{item.firstSeen}</span>,
            ])}
          </div>
        </div>}

        {activeTab === '战役耗时线' && (
          <>
            <div className="taf-topic-apt-ioc" data-business-view="campaign-duration" data-api-source="campaigns.ts_start,ts_end">
              <header><strong>战役持续时间 TOP5</strong><span>真实首末活动时间</span></header>
              {campaignDurationRows.length ? (
                <div className="taf-topic-apt-business-table is-duration">
                  <b>战役</b><b>持续时间</b><b>状态</b><b>告警</b>
                  {campaignDurationRows.flatMap((item) => [
                    item.id,
                    item.duration,
                    item.status || item.activityStatus || 'unknown',
                    String(item.alertCount),
                  ].map((cell, index) => <span key={`${item.id}-duration-${index}`} title={cell}>{cell}</span>))}
                </div>
              ) : <div className="taf-topic-empty">campaigns 未返回首末活动时间</div>}
            </div>
            <div className="taf-topic-apt-trend" data-business-view="campaign-window" data-api-source="campaigns:derived">
              <header><strong>调查窗口与风险</strong><span>API 派生</span></header>
              {campaignDurationRows.length ? <div className="taf-topic-campaign-window-list">
                {campaignDurationRows.map((item) => (
                  <span key={`${item.id}-window`} title={item.timeRange}>
                    <b>{item.id}</b><em>{Math.round(item.score * 100)}%</em><small>{item.timeRange}</small>
                  </span>
                ))}
              </div> : <div className="taf-topic-empty">campaigns 未返回调查窗口</div>}
            </div>
          </>
        )}

        {activeTab === '关键 IoC 命中' && (
          <>
            <div className="taf-topic-apt-trend" data-business-view="ioc-types" data-api-source="iocs.type,hits">
              <header><strong>IoC 类型命中分布</strong><span>hits 求和</span></header>
              {iocTypeCounts.length ? <ExfilBarChart items={iocTypeCounts} ariaLabel="IoC 类型命中分布" /> : <div className="taf-topic-empty">iocs 未返回类型命中数据</div>}
            </div>
            <div className="taf-topic-apt-ioc" data-business-view="ioc-campaign" data-api-source="iocs.campaign">
              <header><strong>IoC 与战役关联</strong><span>真实关联字段</span></header>
              {model.iocs.length ? <div className="taf-topic-apt-business-table is-ioc-campaign">
                <b>IoC</b><b>类型</b><b>战役</b><b>命中</b>
                {model.iocs.flatMap((item) => [
                  item.ioc,
                  item.type,
                  item.campaign,
                  String(item.hits),
                ].map((cell, index) => <span
                  key={`${item.ioc}-campaign-${index}`}
                  data-column={['ioc', 'type', 'campaign', 'hits'][index]}
                  title={cell}
                >{cell}</span>))}
              </div> : <div className="taf-topic-empty">iocs 未返回战役关联</div>}
            </div>
          </>
        )}

        {activeTab === '横向移动关系' && (
          <>
            <div className="taf-topic-apt-ioc" data-business-view="lateral-relations" data-api-source="topology_links:derived">
              <header><strong>横向移动相关拓扑关系</strong><span>{model.lateralRelations.length} 条可绘制关系</span></header>
              {model.lateralRelations.length ? (
                <div className="taf-topic-apt-business-table is-lateral">
                  <b>源框</b><b>目标框</b><b>关系</b><b>可信</b>
                  {model.lateralRelations.flatMap((item) => [
                    item.sourceLabel,
                    item.targetLabel,
                    item.originalLabel,
                    item.lineType === 'solid' ? '确认' : '推断',
                  ].map((cell, index) => <span key={`${item.sourceId}-${item.targetId}-lateral-${index}`} title={cell}>{cell}</span>))}
                </div>
              ) : <div className="taf-topic-empty">topology_links 未返回可严格识别的横向移动关系</div>}
            </div>
            <div className="taf-topic-apt-trend" data-business-view="lateral-summary" data-api-source="summary.lateral_move_links,topology_links:derived">
              <header><strong>横向移动调查口径</strong><span>聚合值与可绘制关系分列</span></header>
              <div className="taf-topic-derived-metrics">
                <span><b>API 聚合链路</b><strong>{lateralSummaryCount}</strong><small>summary.lateral_move_links</small></span>
                <span><b>可绘制关系</b><strong>{model.lateralRelations.length}</strong><small>严格端点校验后</small></span>
                <span><b>确认关系</b><strong>{model.lateralRelations.filter((item) => item.lineType === 'solid').length}</strong><small>solid</small></span>
                <span><b>推断关系</b><strong>{model.lateralRelations.filter((item) => item.lineType === 'dashed').length}</strong><small>dashed</small></span>
              </div>
            </div>
            <div className="taf-topic-apt-ioc" data-business-view="lateral-campaigns" data-api-source="campaigns.attack_phases,entities">
              <header><strong>涉及战役与实体</strong><span>campaigns 原始字段</span></header>
              {model.campaignDetails.some((item) => item.attackPhases.some((phase) => normalizeAptPhase(phase) === '横向移动')) ? <div className="taf-topic-campaign-window-list">
                {model.campaignDetails.filter((item) => item.attackPhases.some((phase) => normalizeAptPhase(phase) === '横向移动')).map((item) => (
                  <span key={`${item.id}-lateral-campaign`}><b>{item.id}</b><em>{item.entities.length} 实体</em><small>{item.entities.join(' / ') || '无实体'}</small></span>
                ))}
              </div> : <div className="taf-topic-empty">campaigns 未返回横向移动阶段战役</div>}
            </div>
          </>
        )}
        {activeTab === '处置动作状态' && (
          <>
            <div className="taf-topic-apt-trend" data-business-view="response-donut" data-api-source="response">
              <header><strong>处置动作状态</strong><span>{responseTotal} 条动作</span></header>
              <DataQualityDonutChart
                ariaLabel="APT 处置动作状态"
                rows={model.response.map((item) => ({ label: item.label, value: item.value, color: item.tone === 'risk' ? '#ff5b3d' : item.tone === 'warn' ? '#ffb020' : '#65d86e' }))}
              />
            </div>
            <div className="taf-topic-apt-ioc" data-business-view="campaign-status" data-api-source="campaigns.status,activity_status">
              <header><strong>战役处置状态</strong><span>campaigns 原始状态</span></header>
              {model.campaignDetails.length ? <div className="taf-topic-apt-business-table is-campaign-status">
                <b>战役</b><b>状态</b><b>活动态</b><b>评分</b>
                {model.campaignDetails.slice(0, 5).flatMap((item) => [
                  item.id,
                  item.status || '-',
                  item.activityStatus || '-',
                  `${Math.round(item.score * 100)}%`,
                ].map((cell, index) => <span key={`${item.id}-status-${index}`} title={cell}>{cell}</span>))}
              </div> : <div className="taf-topic-empty">campaigns 未返回处置状态</div>}
            </div>
            <div className="taf-topic-apt-trend" data-business-view="response-summary" data-api-source="response,summary.closure_rate">
              <header><strong>闭环与待办</strong><span>实时处置口径</span></header>
              <div className="taf-topic-derived-metrics">
                <span><b>闭环率</b><strong>{Math.round(model.closureRate)}%</strong><small>summary.closure_rate</small></span>
                {model.response.map((item) => <span key={`${item.label}-summary`}><b>{item.label}</b><strong>{item.value}</strong><small>{Math.round(item.value / Math.max(responseTotal, 1) * 100)}%</small></span>)}
              </div>
            </div>
          </>
        )}
        {activeTab === '证据关联强度' && (
          <>
            <div className="taf-topic-apt-ioc" data-business-view="evidence-completeness" data-api-source="evidence_bundle">
              <header><strong>证据包完整度</strong><span>接口返回口径</span></header>
              {model.evidenceRows.length ? <div className="taf-topic-exfil-evidence-list">
                {model.evidenceRows.map((item) => <span key={item.label} className={`is-${item.status}`}><FileProtectOutlined /><b>{item.label}</b><em>{item.value}</em></span>)}
              </div> : <div className="taf-topic-empty">evidence_bundle 未返回证据完整度</div>}
            </div>
            <div className="taf-topic-apt-ioc" data-business-view="evidence-relations" data-api-source="topology_links:derived">
              <header><strong>证据关联拓扑</strong><span>{model.evidenceAssociations.length} 条严格关系</span></header>
              {model.evidenceAssociations.length ? (
                <div className="taf-topic-apt-business-table is-evidence">
                  <b>来源框</b><b>证据框</b><b>关系</b><b>数量</b>
                  {model.evidenceAssociations.flatMap((item) => [
                    item.sourceLabel,
                    item.targetLabel,
                    item.originalLabel,
                    String(item.value),
                  ].map((cell, index) => <span key={`${item.sourceId}-${item.targetId}-evidence-${index}`} title={cell}>{cell}</span>))}
                </div>
              ) : <div className="taf-topic-empty">topology_links 未返回 evidence-* 目标关系</div>}
            </div>
            <div className="taf-topic-apt-trend" data-business-view="evidence-gaps" data-api-source="evidence_bundle.status">
              <header><strong>缺证与补证队列</strong><span>{evidenceGapRows.length} 项待处理</span></header>
              <div className="taf-topic-campaign-window-list">
                {evidenceGapRows.length
                  ? evidenceGapRows.map((item) => <span key={`${item.label}-gap`}><b>{item.label}</b><em className={`is-${item.status}`}>{item.status}</em><small>{item.value}</small></span>)
                  : model.evidenceRows.length
                    ? <span><b>证据包</b><em>ok</em><small>当前无缺证项</small></span>
                    : <span><b>证据包</b><em>待返回</em><small>evidence_bundle 未返回</small></span>}
              </div>
            </div>
          </>
        )}
      </div>
    </WorkPanel>
  );
}

function AptResponsePanel({ rows, metrics, visuals }: { rows: SnapshotRow[]; metrics: SnapshotMetric[]; visuals?: TopicVisuals }) {
  const model = buildAptVisualModel(rows, metrics, [], visuals);
  const total = model.response.reduce((sum, item) => sum + item.value, 0);
  return (
    <WorkPanel title="处置动作状态（近30天）" className="taf-topic-apt-response-panel" extra={<span className="taf-topic-focus">总计 {total}</span>}>
      <div className="taf-topic-apt-response" aria-label="处置动作状态">
        <div className="taf-topic-apt-response-chart" data-responsive-chart-contract="container-proportional">
          <DataQualityDonutChart
            ariaLabel="APT 处置动作状态分布"
            className="taf-topic-apt-response-echart"
            rows={model.response.map((item) => ({
              label: item.label,
              value: item.value,
              color: item.tone === 'risk' ? '#ff5b3d' : item.tone === 'warn' ? '#ffb020' : '#65d86e',
            }))}
          />
          <div className="taf-topic-apt-response-center">
            <strong>{total}</strong>
            <span>总计</span>
          </div>
        </div>
        <div>
          {model.response.map((item) => (
            <span key={item.label} className={`is-${item.tone}`}>
              <b>{item.label}</b>
              <em>{item.value} ({Math.round(item.value / Math.max(total, 1) * 100)}%)</em>
            </span>
          ))}
        </div>
      </div>
    </WorkPanel>
  );
}

function AptRightRail({
  config,
  metrics,
  evidenceRows,
  rows,
  visuals,
  reportBinding,
}: {
  config: TopicConfig;
  metrics: SnapshotMetric[];
  evidenceRows: PageSnapshot['evidence'];
  rows: SnapshotRow[];
  visuals?: TopicVisuals;
  reportBinding: TopicReportSnapshotBinding;
}) {
  const model = buildAptVisualModel(rows, metrics, evidenceRows, visuals);
  const actions: Array<[string, ReactNode]> = [
    ['编辑范围', <EditOutlined key="edit" />],
    ['保存视图', <SaveOutlined key="save" />],
    ['导出战役报告', <FileDoneOutlined key="report" />],
    ['导出证据包', <DownloadOutlined key="download" />],
    ['订阅', <BellOutlined key="bell" />],
    ['静默', <SafetyCertificateOutlined key="mute" />],
    ['分享', <ShareAltOutlined key="share" />],
    ['收藏', <StarOutlined key="star" />],
  ];
  const riskOpen = model.response.find((item) => item.label === '待处置')?.value ?? 0;
  const reportableCount = visuals?.summary?.reportable_count ?? model.campaigns.length;
  const pendingEvidenceCount = visuals?.summary?.pending_evidence_count ?? model.evidenceRows.filter((item) => item.status === 'warn').length;
  const openRiskCount = visuals?.summary?.open_risk_count ?? riskOpen;

  return (
    <>
      <WorkPanel title={`战役交付摘要 / ${config.displayTopicId ?? config.topicCode}`} className="taf-topic-apt-delivery">
        <span className="taf-topic-apt-delivery-scope">(APT/战役专题)</span>
        <div className="taf-topic-exfil-delivery-grid" data-responsive-summary-contract="ring-legend-values-container-proportional">
          <TopicProgressDonut
            value={model.reportConfidence}
            ariaLabel="APT 战役报告就绪度"
            className="taf-topic-exfil-delivery-ring"
            caption="实时证据计算"
          />
          <div className="taf-topic-exfil-delivery-stats">
            <span><i /><b>可生成报告</b><strong>{reportableCount}</strong></span>
            <span><i /><b>待补证据</b><strong>{pendingEvidenceCount}</strong></span>
            <span><i /><b>未闭环风险</b><strong>{openRiskCount}</strong></span>
          </div>
        </div>
        <div className="taf-topic-exfil-delivery-actions">
          <TopicActionButton topic={config.topicCode} title="导出总报告" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-report-export" reportBinding={reportBinding}><DownloadOutlined />导出总报告</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="导出证据包" className="ant-btn ant-btn-default ant-btn-sm" overlayId="modal-topic-evidence-package-export"><FileProtectOutlined />导出证据包</TopicActionButton>
          <TopicActionButton topic={config.topicCode} title="试点周报导出" className="ant-btn ant-btn-default ant-btn-sm" reportBinding={reportBinding}><ExportOutlined />试点周报导出</TopicActionButton>
        </div>
      </WorkPanel>

      <WorkPanel title="证据包完整度 / APT/战役专题" className="taf-topic-apt-evidence">
        <div className="taf-topic-exfil-evidence-list">
          {model.evidenceRows.slice(0, 6).map((item) => (
            <span key={item.label} className={`is-${item.status}`}>
              <FileProtectOutlined />
              <b>{item.label}</b>
              <em>{item.value}</em>
            </span>
          ))}
        </div>
      </WorkPanel>

      <WorkPanel title="战役报告预览 / 当前保存视图" className="taf-topic-apt-report">
        <div className="taf-topic-report-preview">
          <TopicReportThumbnail title={config.reportTitle} topicId={config.displayTopicId ?? config.topicCode} completeness={model.reportConfidence} />
          <div>
            <strong>{config.reportTitle}</strong>
            <span>时间窗：{config.timeRange.split('(')[0].trim()}</span>
            <span>资产组：{config.reportSubject}</span>
            <span>生成时间：{visuals?.presentation?.reportGeneratedAt || '提交导出任务时生成'}</span>
            <TopicReportPreviewButton topic={config.topicCode} config={config} visuals={visuals} reportBinding={reportBinding} />
          </div>
        </div>
      </WorkPanel>

      <WorkPanel title="专题动作 / 仅作用于当前专题" className="taf-topic-apt-action-panel">
        <div className="taf-topic-exfil-action-grid">
          {actions.map(([label, icon]) => (
            <TopicActionButton key={String(label)} topic={config.topicCode} title={String(label)} target={config.displayTopicId ?? config.topicCode} overlayId={topicRailOverlayId(String(label))} reportBinding={reportBinding}>
              {icon}
              <span>{label}</span>
            </TopicActionButton>
          ))}
        </div>
      </WorkPanel>
    </>
  );
}

function AptEvidenceToolbar({
  phase,
  status,
  query,
  phases,
  statuses,
  onPhaseChange,
  onStatusChange,
  onQueryChange,
}: {
  phase: string;
  status: string;
  query: string;
  phases: string[];
  statuses: string[];
  onPhaseChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onQueryChange: (value: string) => void;
}) {
  return (
    <div className="taf-topic-apt-table-toolbar" aria-label="APT 证据表筛选">
      <Select aria-label="APT 阶段筛选" size="small" popupMatchSelectWidth={190} value={phase} onChange={onPhaseChange} options={['全部', ...phases].map((value) => ({ value, label: `阶段：${value}` }))} />
      <Select aria-label="APT 处置状态筛选" size="small" popupMatchSelectWidth={190} value={status} onChange={onStatusChange} options={['全部', ...statuses].map((value) => ({ value, label: `状态：${value}` }))} />
      <Input aria-label="搜索 APT 证据" size="small" allowClear prefix={<SearchOutlined />} value={query} onChange={(event) => onQueryChange(event.target.value)} placeholder="搜索战役 / IoC" />
    </div>
  );
}

function AptEvidenceTable({ rows, isLoading, topic }: { rows: SnapshotRow[]; isLoading: boolean; topic: string }) {
  const allTableRows = buildAptEvidenceEventRows(rows);
  const pageSize = 10;
  const [page, setPage] = useState(1);
  const [phase, setPhase] = useState('全部');
  const [status, setStatus] = useState('全部');
  const [query, setQuery] = useState('');
  const [selectedAction, setSelectedAction] = useState<{ action: string; row: AptEvidenceEventRow }>();
  const [submittedAction, setSubmittedAction] = useState(false);
  const [submittingAction, setSubmittingAction] = useState(false);
  const [actionID, setActionID] = useState('');
  const [actionResult, setActionResult] = useState('');
  const [actionError, setActionError] = useState('');
  const tableRows = allTableRows.filter((row) => {
    if (phase !== '全部' && row.phase !== phase) return false;
    if (status !== '全部' && row.status !== status) return false;
    return !query || Object.values(row).some((value) => String(value).toLowerCase().includes(query.toLowerCase()));
  });
  const pageCount = Math.max(1, Math.ceil(tableRows.length / pageSize));
  const currentPage = Math.min(page, pageCount);
  const visibleRows = tableRows.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const pageTokens = compactPaginationTokens(pageCount, currentPage);
  const columns: Array<[keyof AptEvidenceEventRow, string]> = [
    ['id', '事件ID'],
    ['phase', '阶段'],
    ['assetGroup', '资产组'],
    ['ioc', 'IoC'],
    ['evidenceType', '证据类型'],
    ['timeWindow', '时间窗'],
    ['status', '处置状态'],
  ];

  return (
    <>
      <AptEvidenceToolbar
        phase={phase}
        status={status}
        query={query}
        phases={[...new Set(allTableRows.map((row) => row.phase))]}
        statuses={[...new Set(allTableRows.map((row) => row.status))]}
        onPhaseChange={(value) => { setPhase(value); setPage(1); }}
        onStatusChange={(value) => { setStatus(value); setPage(1); }}
        onQueryChange={(value) => { setQuery(value); setPage(1); }}
      />
      <div
        className="taf-topic-apt-evidence-table"
        aria-busy={isLoading}
        aria-label="战役关联事件与证据"
        data-page-size={pageSize}
        data-current-page={currentPage}
        data-rendered-row-count={visibleRows.length}
      >
      {columns.map(([, label]) => <b key={label} title={label}>{label}</b>)}
      <b title="操作">操作</b>
      {isLoading ? (
        <span className="taf-topic-apt-table-loading">加载中...</span>
      ) : !visibleRows.length ? (
        <span className="taf-topic-apt-table-loading">当前时间窗没有真实战役证据</span>
      ) : visibleRows.map((row) => (
        <div key={row.id} className="taf-topic-apt-table-row">
          {columns.map(([key]) => (
            <span
              key={`${row.id}-${String(key)}`}
              className={key === 'status' ? `is-${row.statusTone}` : undefined}
              title={String(row[key])}
            >
              {String(row[key])}
            </span>
          ))}
          <span className="taf-topic-apt-table-actions" title={row.actions.join(' / ')}>
            {row.actions.map((action) => (
              <button
                key={`${row.id}-${action}`}
                type="button"
                title={action}
                onClick={() => {
                  setSubmittedAction(false);
                  setActionID('');
                  setActionResult('');
                  setActionError('');
                  setSelectedAction({ action, row });
                }}
              >
                {action}
              </button>
            ))}
          </span>
        </div>
      ))}
      {!isLoading && (
        <div className="taf-topic-apt-table-footer">
          <span>共 {tableRows.length} 条</span>
          <button type="button" title="上一页" aria-label="APT 证据上一页" disabled={currentPage <= 1} onClick={() => setPage((value) => Math.max(1, value - 1))}>‹</button>
          {pageTokens.map((value) => typeof value === 'number' ? (
            <button
              key={value}
              type="button"
              className={currentPage === value ? 'is-active' : ''}
              title={`第 ${value} 页`}
              aria-current={currentPage === value ? 'page' : undefined}
              onClick={() => setPage(value)}
            >
              {value}
            </button>
          ) : <i key={value} aria-hidden="true">…</i>)}
          <button type="button" title="下一页" aria-label="APT 证据下一页" disabled={currentPage >= pageCount} onClick={() => setPage((value) => Math.min(pageCount, value + 1))}>›</button>
          <span>{pageSize} 条/页</span>
        </div>
      )}
        <Modal
        className="taf-topic-action-drawer"
        title={selectedAction ? `${selectedAction.action}确认` : 'APT 证据操作'}
        open={Boolean(selectedAction)}
        width="min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))"
        onCancel={() => {
          setSelectedAction(undefined);
          setSubmittedAction(false);
          setActionID('');
          setActionResult('');
          setActionError('');
        }}
        okText={submittedAction ? '已完成' : '确认执行'}
        cancelText="取消"
        okButtonProps={{ loading: submittingAction, disabled: submittedAction || !selectedAction }}
        onOk={() => {
          if (!selectedAction) return;
          setSubmittingAction(true);
          setActionError('');
          void submitTopicAction(topic, selectedAction.action, selectedAction.row.id, collectTopicDataContext())
            .then((result) => {
              setActionID(result.action_id);
              setActionResult(result.business_effect?.message || 'APT 证据业务操作已执行');
              setSubmittedAction(true);
            })
            .catch((error: unknown) => setActionError(error instanceof Error ? error.message : 'APT 证据操作提交失败'))
            .finally(() => setSubmittingAction(false));
        }}
      >
        <div className="taf-alert-detail-action-body">
          <p>将为当前 APT 专题事件创建“{selectedAction?.action}”业务任务，并原子写入任务与审计上下文。</p>
          <dl>
            <dt>事件 ID</dt><dd>{selectedAction?.row.id}</dd>
            <dt>IoC</dt><dd>{selectedAction?.row.ioc}</dd>
            <dt>执行接口</dt><dd>/v1/topics/apt/actions</dd>
          </dl>
          {actionError && <Alert type="error" showIcon message="APT 证据操作提交失败" description={actionError} />}
          {submittedAction && <Alert type="success" showIcon message={actionResult} description={`任务 ${actionID}；事件 ${selectedAction?.row.id}；动作 ${selectedAction?.action}`} />}
        </div>
        </Modal>
      </div>
    </>
  );
}

function buildAptVisualModel(rows: SnapshotRow[], metrics: SnapshotMetric[], _evidenceRows: PageSnapshot['evidence'], visuals?: TopicVisuals): AptVisualModel {
  const sourceRows = rows;
  const apiCampaigns = visuals?.aptCampaigns ?? [];
  const campaignNames = apiCampaigns.length
    ? apiCampaigns.slice(0, 3).map((item) => item.id)
    : sourceRows.slice(0, 3).map((row) => rowText(row, '战役名称')).filter(Boolean);
  const alertTotal = sourceRows.reduce((sum, row) => sum + rowNumber(row, '关联告警'), 0);
  const eventTotal = visuals?.summary?.total_events ?? alertTotal;
  const campaigns = campaignNames.map((name, index) => ({
    name: compactCampaignName(name, index),
    fullName: name,
    meta: apiCampaigns[index]?.status || rowText(sourceRows[index] ?? {}, '风险等级') || '风险待评估',
    events: apiCampaigns[index]?.alertCount ?? rowNumber(sourceRows[index] ?? {}, '关联告警'),
    tone: apiCampaigns[index]
      ? (apiCampaigns[index].score >= 0.8 ? 'risk' : apiCampaigns[index].score >= 0.6 ? 'warn' : 'info')
      : aptRiskTone(rowText(sourceRows[index] ?? {}, '风险等级')),
  }));
  const tactics = [
    { id: 'TA0001', label: '初始访问' },
    { id: 'TA0002', label: '执行' },
    { id: 'TA0003', label: '持久化' },
    { id: 'TA0005', label: '防御规避' },
    { id: 'TA0006', label: '凭证访问' },
    { id: 'TA0007', label: '发现' },
    { id: 'TA0008', label: '横向移动' },
    { id: 'TA0011', label: '命令控制' },
    { id: 'TA0010', label: '数据外传' },
  ];
  const phaseCounts = new Map((visuals?.aptPhaseDistribution ?? []).map((item) => [normalizeAptPhase(item.phase), item.count]));
  const phases = tactics.map(({ id, label }) => {
    const value = phaseCounts.get(label) ?? phaseValue(label, sourceRows, metrics);
    return {
      id,
      label,
      value,
      confidence: value > 0 ? '真实命中' : '未命中',
      tone: (value > 0 ? (['横向移动', '数据外传', '命令控制'].includes(label) ? 'risk' : 'warn') : 'info') as Tone,
    };
  });
  const evidenceNodes: AptEvidenceNode[] = uniqueAptEntities(sourceRows).slice(0, 5).map((item) => ({
    label: aptEntityType(item.entity),
    value: `${item.entity} / ${item.hits} 告警`,
    tone: item.tone,
  }));
  if (!evidenceNodes.length) evidenceNodes.push({ label: '关联实体', value: '专题接口未返回', tone: 'info' });
  const assets = [
    { label: '资产/组', value: `命中 ${metricNumber(metrics, '关键资产命中')}`, tone: 'ok' as Tone },
    { label: '账号', value: '专题接口未返回', tone: 'info' as Tone },
    { label: '资产/后门', value: `命中 ${metricNumber(metrics, '持久化迹象数')}`, tone: 'ok' as Tone },
    { label: '关键凭据', value: '专题接口未返回', tone: 'info' as Tone },
  ];
  const reportConfidence = metricNumber(metrics, '报告置信度');
  const closureRate = metricNumber(metrics, '处置闭环率');
  const normalizedEvidenceRows = visuals?.evidenceBundle
    ? visuals.evidenceBundle.map((item) => ({
      label: item.label,
      value: `${item.complete} / ${item.total} (${item.total ? Math.round(item.complete / item.total * 100) : 0}%)`,
      status: item.status,
    }))
    : [];
  const response = visuals?.aptResponse
    ? [
      { label: '已完成', value: visuals.aptResponse.closed, tone: 'ok' as Tone },
      { label: '进行中', value: visuals.aptResponse.processing, tone: 'warn' as Tone },
      { label: '待处置', value: visuals.aptResponse.open, tone: 'risk' as Tone },
    ]
    : buildAptResponse(sourceRows);
  const visualIocs = visuals?.aptIocs ?? [];

  return {
    campaigns,
    phases,
    evidenceNodes,
    assets,
    timeline: buildAptTimeline(sourceRows, campaignNames.slice(0, 3)),
    iocs: visualIocs.length
      ? visualIocs.slice(0, 5).map((item) => ({
        ioc: item.value,
        type: item.type,
        hits: item.hits,
        campaign: item.campaign || '-',
        firstSeen: formatTopicTime(item.firstSeen ?? 0),
        lastSeen: formatTopicTime(item.lastSeen ?? 0),
      }))
      : uniqueAptEntities(sourceRows).slice(0, 5).map((item) => ({
        ioc: item.entity,
        type: aptEntityType(item.entity),
        hits: item.hits,
        campaign: '-',
        firstSeen: item.firstSeen,
        lastSeen: '-',
      })),
    response,
    evidenceRows: normalizedEvidenceRows,
    campaignDetails: visuals?.aptCampaigns ?? [],
    lateralRelations: visuals?.aptLateralPaths ?? [],
    evidenceAssociations: visuals?.aptEvidenceAssociations ?? [],
    reportConfidence,
    closureRate,
    eventTotal,
  };
}

function buildAptEvidenceEventRows(rows: SnapshotRow[]): AptEvidenceEventRow[] {
  const directRows = rows
    .map((row, index) => {
      const id = rowText(row, '事件ID') || rowText(row, '战役名称');
      if (!id) return null;
      const rawStatus = rowText(row, '处置状态') || 'unknown';
      const status = aptStatusLabel(rawStatus);
      const firstSeen = rowText(row, '首次发现');
      const lastSeen = rowText(row, '最近活动');
      const alertCount = rowNumber(row, '关联告警');
      return {
        id,
        phase: rowText(row, '阶段') || '初始访问',
        assetGroup: rowText(row, '资产组') || rowText(row, '关键实体') || '-',
        ioc: rowText(row, 'IoC') || rowText(row, '关键实体') || '-',
        evidenceType: rowText(row, '证据类型') || (alertCount > 0 ? `告警 ${alertCount}` : '战役记录'),
        timeWindow: rowText(row, '时间窗') || (firstSeen && lastSeen ? `${firstSeen} ~ ${lastSeen}` : lastSeen || firstSeen || `第 ${index + 1} 条战役记录`),
        status,
        statusTone: aptStatusTone(status),
        actions: ['全量详情', '溯源分析', '关联告警'],
      } satisfies AptEvidenceEventRow;
    })
    .filter((row): row is AptEvidenceEventRow => Boolean(row));

  return directRows;
}

function compactCampaignName(value: string, index: number) {
  const normalized = value.trim();
  if (!normalized) return `CAMPAIGN-${String(index + 1).padStart(2, '0')}`;
  if (normalized.length <= 18) return normalized;
  const segments = normalized.split('-').filter(Boolean);
  const suffix = segments[segments.length - 1] ?? '';
  if (suffix && suffix.length <= 10) return `${normalized.slice(0, 7)}…${suffix}`;
  return `${normalized.slice(0, 8)}…${normalized.slice(-7)}`;
}

function aptStatusTone(status: string): Tone {
  if (status.includes('完成') || status === 'closed' || status === 'resolved' || status === 'ended') return 'ok';
  if (status.includes('待') || status.includes('未') || status === 'unknown') return 'risk';
  return 'warn';
}

function aptStatusLabel(status: string) {
  const normalized = status.trim().toLowerCase();
  if (['closed', 'resolved', 'ended'].includes(normalized)) return '已完成';
  if (['active', 'in_progress', 'investigating', 'processing'].includes(normalized)) return '进行中';
  return status && normalized !== 'unknown' ? status : '待处置';
}

function aptRiskTone(risk: string): Tone {
  if (risk.includes('高') || risk.toLowerCase().includes('critical') || risk.toLowerCase().includes('high')) return 'risk';
  if (risk.includes('中') || risk.toLowerCase().includes('medium')) return 'warn';
  return 'info';
}

function normalizeAptPhase(phase: string) {
  const normalized = phase.trim().toLowerCase().replace(/[\s-]+/g, '_');
  const labels: Record<string, string> = {
    initial_access: '初始访问',
    execution: '执行',
    persistence: '持久化',
    defense_evasion: '防御规避',
    credential_access: '凭证访问',
    discovery: '发现',
    lateral_movement: '横向移动',
    command_control: '命令控制',
    command_and_control: '命令控制',
    exfiltration: '数据外传',
  };
  if (phase.trim() === '外传') return '数据外传';
  return labels[normalized] ?? phase;
}

function phaseValue(label: string, rows: SnapshotRow[], metrics: SnapshotMetric[]) {
  const byRow = rows.filter((row) => normalizeAptPhase(rowText(row, '阶段')).includes(label.slice(0, 2))).length;
  if (byRow) return byRow;
  if (label === '横向移动') return metricNumber(metrics, '横向移动链路');
  if (label === '持久化') return metricNumber(metrics, '持久化迹象数');
  if (label === '数据外传') return metricNumber(metrics, '外传关联证据');
  return 0;
}

function aptEntityType(entity: string) {
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(entity)) return 'IP';
  if (/^[a-f0-9]{32,64}$/i.test(entity)) return 'Hash';
  if (entity.includes('.')) return '域名/主机';
  return '实体';
}

function uniqueAptEntities(rows: SnapshotRow[]) {
  const entities = new Map<string, { entity: string; hits: number; firstSeen: string; tone: Tone }>();
  rows.forEach((row) => {
    const entity = rowText(row, '关键实体');
    if (!entity || entity === '-') return;
    const existing = entities.get(entity);
    const hits = rowNumber(row, '关联告警');
    entities.set(entity, {
      entity,
      hits: (existing?.hits ?? 0) + hits,
      firstSeen: existing?.firstSeen || rowText(row, '首次发现') || '-',
      tone: aptRiskTone(rowText(row, '风险等级')),
    });
  });
  return [...entities.values()].sort((a, b) => b.hits - a.hits);
}

function buildAptResponse(rows: SnapshotRow[]): AptVisualModel['response'] {
  const counts = { completed: 0, active: 0, pending: 0 };
  rows.forEach((row) => {
    const status = rowText(row, '处置状态').toLowerCase();
    if (['closed', 'resolved', 'ended'].includes(status)) counts.completed++;
    else if (['active', 'in_progress', 'investigating', 'processing'].includes(status)) counts.active++;
    else counts.pending++;
  });
  return [
    { label: '已完成', value: counts.completed, tone: 'ok' },
    { label: '进行中', value: counts.active, tone: 'warn' },
    { label: '待处置', value: counts.pending, tone: 'risk' },
  ];
}

function buildAptTimeline(rows: SnapshotRow[], campaignNames: string[]): AptTimelinePoint[] {
  const timestamped = rows
    .map((row) => {
      const rawTimestamp = rowNumber(row, '__ts_start');
      const timestamp = rawTimestamp > 0 && rawTimestamp < 10_000_000_000
        ? rawTimestamp * 1000
        : rawTimestamp;
      return {
        campaign: rowText(row, '__campaign_id') || rowText(row, '战役名称'),
        timestamp,
      };
    })
    .filter((item) => item.campaign && Number.isFinite(item.timestamp))
    .sort((left, right) => left.timestamp - right.timestamp);
  if (!timestamped.length) return [];

  const minTimestamp = timestamped[0].timestamp;
  const maxTimestamp = timestamped[timestamped.length - 1].timestamp;
  const bucketCount = Math.min(10, Math.max(1, timestamped.length));
  const bucketWidth = Math.max(1, Math.ceil((maxTimestamp - minTimestamp + 1) / bucketCount));
  const counts = Array.from({ length: bucketCount }, () => [0, 0, 0]);

  timestamped.forEach((item) => {
    const campaignIndex = campaignNames.findIndex((name) => name === item.campaign);
    if (campaignIndex < 0 || campaignIndex > 2) return;
    const bucketIndex = Math.min(bucketCount - 1, Math.floor((item.timestamp - minTimestamp) / bucketWidth));
    counts[bucketIndex][campaignIndex] += 1;
  });

  return counts.map((values, index) => ({
    label: formatTopicTime(minTimestamp + (index * bucketWidth)).slice(0, 5),
    aptCn: values[0],
    tempHawk: values[1],
    unknown: values[2],
  }));
}

function renderTopicCell(topic: string, column: string, value: unknown, record: SnapshotRow) {
  if (column.includes('风险')) return <StatusTag value={value} />;
  if (column === '处置') {
    const target = rowText(record, '事件ID') || rowText(record, '外传路径') || rowText(record, '源资产') || topic;
    if (topic === 'exfil') {
      return (
        <div className="taf-topic-exfil-row-actions" aria-label={`外传证据操作 ${target}`}>
          {['PCAP', 'Session', '文件摘要', '回溯路径', '审计日志'].map((label) => (
            <TopicActionButton
              key={label}
              topic={topic}
              title={label}
              target={target}
              className="ant-btn ant-btn-default ant-btn-sm"
            >
              {label}
            </TopicActionButton>
          ))}
        </div>
      );
    }
    return <TopicActionButton topic={topic} title={String(value || '下钻')} target={target} className="ant-btn ant-btn-link ant-btn-sm">{String(value || '下钻')}</TopicActionButton>;
  }
  if (column.includes('流量') || column.includes('上传量') || column.includes('告警')) {
    return <strong className="taf-topic-strong-cell">{String(value ?? '-')}</strong>;
  }
  if (column.includes('源') || column.includes('实体') || column.includes('会话')) {
    return <span className="taf-topic-entity-cell"><GlobalOutlined />{String(value ?? '-')}</span>;
  }
  return String(value ?? '-');
}

function fallbackMetric(label: string): PageSnapshot['metrics'][number] {
  return { label, value: label.includes('完整') ? '0.0%' : '0', delta: '等待 API', status: 'info' };
}
