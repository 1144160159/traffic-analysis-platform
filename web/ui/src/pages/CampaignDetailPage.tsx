import {
  ApartmentOutlined,
  ArrowRightOutlined,
  ArrowLeftOutlined,
  AuditOutlined,
  BranchesOutlined,
  CloudDownloadOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  FileDoneOutlined,
  FileTextOutlined,
  FileProtectOutlined,
  FlagOutlined,
  ForkOutlined,
  LinkOutlined,
  MoreOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  SearchOutlined,
  SyncOutlined,
  SwapOutlined,
  TeamOutlined,
  UploadOutlined,
  UserSwitchOutlined,
  WarningOutlined,
  WifiOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Checkbox, Empty, message, Modal, Select, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties, ReactNode } from 'react';
import { Fragment, useEffect, useMemo, useRef, useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { DataQualityDonutChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  classifyCampaignActionStatus,
  downloadCampaignReport,
  saveCampaignReportArtifact,
  submitCampaignAction,
  waitForCampaignReport,
  type CampaignActionStatus,
  type CampaignReportStatus,
} from '@/services/campaignActionApi';
import {
  fetchCampaignDetailSnapshot,
  type CampaignDetailAccountRow,
  type CampaignDetailAlertRow,
  type CampaignDetailAssetRow,
  type CampaignDetailBusinessSystemRow,
  type CampaignDetailCampusRow,
  type CampaignDetailDepartmentRow,
  type CampaignDetailImpactRiskRow,
  type CampaignDetailServiceRow,
  type CampaignDetailSnapshot,
} from '@/services/campaignDetailApi';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const cellTitle = <T extends Record<string, string>>(key: keyof T) => (record: T) => ({
  title: record[key],
});

const showCampaignActionNotice = (status: CampaignActionStatus, jobId: string) => {
  const statusClass = classifyCampaignActionStatus(status);
  if (statusClass === 'in_progress') {
    message.info(`战役操作已受理，尚未最终完成：${jobId}`);
  } else if (statusClass === 'succeeded') {
    message.success(`战役操作已完成并写入审计：${jobId}`);
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

const alertColumns: ColumnsType<CampaignDetailAlertRow> = [
  {
    title: '时间',
    dataIndex: '告警时间',
    key: '告警时间',
    width: 80,
    ellipsis: true,
    onCell: cellTitle('告警时间'),
    render: (value) => compactTimestamp(String(value)),
  },
  {
    title: '告警名称',
    dataIndex: '告警名称',
    key: '告警名称',
    width: 102,
    ellipsis: true,
    onCell: cellTitle('告警名称'),
    render: (value, record) => <Link to={`/alerts/${encodeURIComponent(record.告警ID)}`}>{String(value)}</Link>,
  },
  { title: '攻击阶段', dataIndex: '攻击阶段', key: '攻击阶段', width: 70, ellipsis: true, onCell: cellTitle('攻击阶段') },
  { title: '资产', dataIndex: '影响资产', key: '影响资产', width: 46, ellipsis: true, onCell: cellTitle('影响资产') },
  { title: '风险', dataIndex: '风险', key: '风险', width: 56, render: (value) => <StatusTag value={value} /> },
  { title: '状态', dataIndex: '状态', key: '状态', width: 66, render: (value) => <StatusTag value={value} /> },
];

const assetColumns: ColumnsType<CampaignDetailAssetRow> = [
  { title: '类型', dataIndex: '类型', key: '类型', width: 74, onCell: cellTitle('类型') },
  { title: '部门', dataIndex: '部门', key: '部门', width: 86, onCell: cellTitle('部门') },
  { title: '业务系统', dataIndex: '业务系统', key: '业务系统', ellipsis: true, onCell: cellTitle('业务系统') },
  { title: '风险', dataIndex: '风险', key: '风险', width: 74, render: (value) => <StatusTag value={value} /> },
];

export function CampaignDetailPage({ route }: { route: NavRoute }) {
  const params = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const visualPageId = searchParams.get('__codex_page_id') ?? '';
  const [reportOpen, setReportOpen] = useState(visualPageId === 'modal-campaign-report-export');
  const [reportStatus, setReportStatus] = useState<CampaignReportStatus>();
  const reportAbortRef = useRef<AbortController>();
  const visualBreakdownMode = import.meta.env.DEV && isVisualBreakdownMode();
  const campaignId = params.campaignId ?? 'APT-20260619-001';
  const requestedSnapshotId = searchParams.get('snapshot_id') ?? '';
  const activeImpact = resolveCampaignImpact(searchParams.get('impact'));
  const alertRiskFilter = resolveAlertRisk(searchParams.get('alertRisk'));
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['campaign-detail', campaignId, requestedSnapshotId],
    queryFn: () => fetchCampaignDetailSnapshot(campaignId, requestedSnapshotId || undefined),
    refetchInterval: visualBreakdownMode ? false : 30_000,
  });
  const snapshot = data ?? emptySnapshot(campaignId);
  const releasePinnedSnapshot = () => {
    if (!requestedSnapshotId) return;
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.delete('snapshot_id');
      return next;
    }, { replace: true });
  };
  const actionMutation = useMutation({
    mutationFn: submitCampaignAction,
    onSuccess: (result) => {
      showCampaignActionNotice(result.jobStatus, result.jobId);
      if (result.mode === 'server-persisted-mutation') releasePinnedSnapshot();
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '战役操作提交失败'),
  });
  const reportMutation = useMutation({
    mutationFn: async (payload: Record<string, unknown>) => {
      reportAbortRef.current?.abort();
      const controller = new AbortController();
      reportAbortRef.current = controller;
      setReportStatus(undefined);
      try {
        const receipt = await submitCampaignAction({
          actionId: 'campaign-report-generate',
          campaignId,
          target: '生成战役报告',
          metadata: {
            ...payload,
            ...(snapshot.snapshotId ? { snapshot_id: snapshot.snapshotId } : {}),
          },
          expectedRevision: snapshot.stateVersion,
          reason: '冻结当前战役快照并生成可校验的复盘报告',
        });
        const reportId = typeof receipt.result.report_id === 'string' ? receipt.result.report_id.trim() : '';
        if (!reportId) throw new Error('战役报告受理响应缺少稳定 report_id');
        message.info(`战役报告已受理，正在等待执行器终态：${receipt.jobId}`);
        const completed = await waitForCampaignReport(campaignId, reportId, {
          signal: controller.signal,
          onStatus: setReportStatus,
        });
        const artifact = await downloadCampaignReport(campaignId, completed);
        saveCampaignReportArtifact(artifact);
        return { receipt, completed, artifact };
      } finally {
        if (reportAbortRef.current === controller) reportAbortRef.current = undefined;
      }
    },
    onSuccess: async ({ completed, artifact }) => {
      releasePinnedSnapshot();
      if (!requestedSnapshotId) await refetch();
      message.success(`战役报告已完成并校验下载：${artifact.filename}（${completed.artifactSHA256}）`);
    },
    onError: (mutationError) => message.error(mutationError instanceof Error ? mutationError.message : '战役报告执行失败'),
  });
  const filteredAlerts = useMemo(
    () => alertRiskFilter === '全部'
      ? snapshot.alerts
      : snapshot.alerts.filter((row) => row.风险.includes(alertRiskFilter.replace('危', ''))),
    [alertRiskFilter, snapshot.alerts],
  );
  const runAction = (
    actionId: Parameters<typeof submitCampaignAction>[0]['actionId'],
    target: string,
    metadata?: Record<string, unknown>,
  ) => actionMutation.mutateAsync({
    actionId,
    campaignId,
    target,
    metadata: {
      ...(metadata ?? {}),
      ...(snapshot.snapshotId ? { snapshot_id: snapshot.snapshotId } : {}),
    },
    expectedRevision: snapshot.stateVersion,
    reason: `战役详情操作：${target}`,
  });
  const openCampaignReport = () => {
    setReportStatus(undefined);
    setReportOpen(true);
  };
  const closeCampaignReport = () => {
    if (reportMutation.isPending) {
      reportAbortRef.current?.abort();
      message.info('已停止浏览器等待；服务端已受理的报告任务仍会继续执行。');
    }
    setReportOpen(false);
  };
  const exportCampaignPackage = () => {
    reportMutation.mutate({
      format: 'json',
      sections: ['profile', 'attack_phases', 'alerts', 'impact', 'evidence', 'response', 'review'],
    });
  };
  const changeImpact = (nextImpact: string) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set('impact', nextImpact);
      return next;
    });
  };
  const changeAlertRisk = (nextRisk: string) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (nextRisk === '全部') next.delete('alertRisk');
      else next.set('alertRisk', nextRisk);
      return next;
    });
  };

  return (
    <div className="taf-page taf-campaign-detail-page is-redesigned" data-page-id="campaign-detail">
      <header className="taf-campaign-detail-titlebar">
        <div className="taf-campaign-detail-titlebar__page-title">
          <h1 title={route.page.title}>{route.page.title}</h1>
        </div>
        <Space size={8}>
          <Button size="small" icon={<ReloadOutlined />} loading={isLoading} onClick={() => void refetch()}>刷新</Button>
          <Button size="small" icon={<CloudDownloadOutlined />} loading={reportMutation.isPending} onClick={exportCampaignPackage}>导出战役包</Button>
          <Button size="small" type="primary" icon={<FileDoneOutlined />} onClick={openCampaignReport}>生成战役报告</Button>
          <Button size="small" icon={<AuditOutlined />} loading={actionMutation.isPending} onClick={() => void runAction('campaign-context-action', '写入审计', { event: 'CAMPAIGN_DETAIL_AUDIT_REQUESTED' })}>写入审计</Button>
          <Button size="small" icon={<MoreOutlined />}>更多</Button>
          <Tooltip title="返回战役列表">
            <Button size="small" icon={<ArrowLeftOutlined />} aria-label="返回战役列表" onClick={() => navigate('/campaigns')} />
          </Tooltip>
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/campaigns/{id}、APISIX 路由、ClickHouse campaigns 表或 alert-service。'}
          action={<Button size="small" danger onClick={() => void refetch()}>重试</Button>}
        />
      )}

      <section className="taf-campaign-detail-profile">
        <div className="taf-campaign-detail-profile-main">
          <div className="taf-campaign-detail-profile-icon">
            <FlagOutlined />
          </div>
          <div>
            <h2>{snapshot.campaignId} <StatusTag value={snapshot.status} /></h2>
            <p>{snapshot.title}</p>
            <div className="taf-campaign-detail-tags">
              {snapshot.tags.map((tag) => <b key={tag}>{tag}</b>)}
            </div>
          </div>
        </div>
        <div className="taf-campaign-detail-risk">
          <DataQualityDonutChart
            ariaLabel={`战役风险评分 ${snapshot.riskScore}`}
            rows={[
              { label: snapshot.currentPhase, value: snapshot.riskScore, color: '#ff4d4f' },
              { label: '剩余分值', value: Math.max(0, 100 - snapshot.riskScore), color: 'rgba(71, 117, 145, 0.22)' },
            ]}
          />
          <span className="taf-campaign-detail-risk__label">
            <b>{snapshot.riskScore}</b>
            <strong>{snapshot.currentPhase}</strong>
          </span>
        </div>
        <div className="taf-campaign-detail-profile-facts">
          {snapshot.profileFacts.filter((item) => !['战役 ID', '风险评分'].includes(item.label)).map((item) => (
            <ProfileFact key={item.label} label={item.label} value={item.value} status={item.status} />
          ))}
        </div>
        <Button size="small" className="taf-campaign-detail-profile-edit" onClick={() => void runAction('campaign-context-action', '编辑战役信息', { intent: 'edit_profile' })}>编辑信息</Button>
      </section>

      <div className="taf-campaign-detail-grid">
        <main className="taf-campaign-detail-main">
          <WorkPanel title="攻击时间轴（基于 ATT&CK 阶段）" className="taf-campaign-detail-phase-panel" extra={<Link to={`/attack-chains?campaign=${encodeURIComponent(snapshot.campaignId)}`}>查看完整时间线</Link>}>
            <div className="taf-campaign-detail-phase-cards">
              {snapshot.phases.map((phase, index) => (
                <div key={phase.phase} className={`taf-campaign-detail-phase-card is-${phase.status}`}>
                  <header><i>{phaseIcon(index)}</i><strong>{phase.phase}</strong></header>
                  <span>{phase.time}</span>
                  <footer><b>{phase.alertCount} 告警</b><b>{phase.evidenceCount} 证据</b></footer>
                </div>
              ))}
            </div>
            <div className="taf-campaign-detail-phase-track">
              {snapshot.phases.map((phase) => <span key={phase.phase} className={`taf-campaign-detail-phase-dot is-${phase.status}`} />)}
            </div>
            <div className="taf-campaign-detail-phase-legend">
              <span className="is-risk">告警事件</span><span className="is-info">证据生成</span>
              <span className="is-ok">处置动作</span><span className="is-warn">关键节点</span>
            </div>
          </WorkPanel>

          <div className="taf-campaign-detail-bottom-grid">
          <WorkPanel title={`关联告警（${snapshot.alertCount}）`} className="taf-campaign-detail-alerts" extra={<Link to="/alerts">查看全部告警</Link>}>
            <div className="taf-campaign-detail-alert-filter">
              {['全部', '高危', '中危', '低危'].map((label) => (
                <button
                  key={label}
                  type="button"
                  className={alertRiskFilter === label ? 'is-active' : ''}
                  aria-pressed={alertRiskFilter === label}
                  onClick={() => changeAlertRisk(label)}
                >
                  {label}
                </button>
              ))}
            </div>
            <Table
              rowKey={(row) => row.告警ID}
              size="small"
              loading={isLoading}
              pagination={false}
              tableLayout="fixed"
              columns={alertColumns}
              dataSource={filteredAlerts.slice(0, 5)}
              scroll={{ x: 420 }}
              locale={{
                emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={`暂无${alertRiskFilter === '全部' ? '' : alertRiskFilter}关联告警`} />,
              }}
            />
          </WorkPanel>

            <WorkPanel
              title="影响范围"
              className="taf-campaign-detail-impact-panel"
              extra={<Link to={impactDestination(activeImpact).href}>{impactDestination(activeImpact).label}</Link>}
            >
              <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} />
              <CampaignImpactInlineContent snapshot={snapshot} activeImpact={activeImpact} />
            </WorkPanel>

            <div className="taf-campaign-detail-evidence-stack">
              <WorkPanel title="证据包完整度" className="taf-campaign-detail-evidence-panel" extra={<Link to="/forensics">查看证据中心</Link>}>
                <div className="taf-campaign-detail-evidence-overview">
                  <div className="taf-campaign-detail-evidence-donut">
                    <DataQualityDonutChart
                      ariaLabel="战役证据包完整度"
                      rows={snapshot.evidenceCompletenessAvailable || visualBreakdownMode
                        ? [
                            { label: '完整', value: snapshot.evidenceCompleteness, color: '#42bfff' },
                            { label: '待补齐', value: Math.max(0, 100 - snapshot.evidenceCompleteness), color: 'rgba(56,151,201,0.18)' },
                          ]
                        : [{ label: '数据未知', value: 1, color: 'rgba(56,151,201,0.28)' }]}
                    />
                    <strong>{snapshot.evidenceCompletenessAvailable || visualBreakdownMode ? `${snapshot.evidenceCompleteness}%` : '--'}</strong>
                  </div>
                  <div>
                    {snapshot.evidenceChecks.map((item) => (
                      <span key={item.label} className={`taf-campaign-detail-evidence-check is-${item.status}`}>
                        <b>{item.label}</b>
                        <i><em style={{ width: `${item.percent}%` }} /></i>
                        <strong>{item.value}</strong>
                      </span>
                    ))}
                    {!snapshot.evidenceChecks.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无完整度明细" />}
                  </div>
                </div>
              </WorkPanel>
              <WorkPanel title="证据摘要" className="taf-campaign-detail-evidence-summary-panel">
                <div className="taf-campaign-detail-evidence-digest">
                  {snapshot.evidenceDigest.map((item) => (
                    <div key={item.label} className="taf-campaign-detail-evidence-digest__row">
                      <span>{item.label}</span>
                      <strong title={item.value}>{item.value || '--'}</strong>
                    </div>
                  ))}
                </div>
              </WorkPanel>
            </div>
          </div>
        </main>

        <aside className="taf-campaign-detail-rail">
          <WorkPanel title="处置流程" className="taf-campaign-detail-response-panel">
            <div className="taf-campaign-detail-response-flow">
              {snapshot.responseFlow.map((step, index) => (
                <Fragment key={step.title}>
                  <div className={`taf-campaign-detail-response-step is-${step.status} is-step-${index}`}>
                    <i>{responseStepIcon(index)}</i>
                    <strong>{step.title}</strong>
                    <span title={step.time}>{step.time}</span>
                  </div>
                  {index < snapshot.responseFlow.length - 1 && (
                    <span className="taf-campaign-detail-response-connector" aria-hidden="true">
                      <ArrowRightOutlined />
                    </span>
                  )}
                </Fragment>
              ))}
            </div>
            <div className="taf-campaign-detail-response-actions-title">处置动作（{snapshot.responseActions.length}）</div>
            <div className="taf-campaign-detail-action-list">
              {snapshot.responseActions.map((row) => (
                <div key={row.动作} className="taf-campaign-detail-action-row">
                  <strong>{row.动作}</strong>
                  <span>{row.目标}</span>
                  <em>{row.负责人}</em>
                  <StatusTag value={row.状态} />
                </div>
              ))}
              {!snapshot.responseActions.length && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无处置动作" />}
            </div>
            <Link className="taf-campaign-detail-panel-more" to={`/playbooks?campaign=${encodeURIComponent(snapshot.campaignId)}`}>查看全部处置记录 &gt;</Link>
          </WorkPanel>

          <WorkPanel title="复盘结论" className="taf-campaign-detail-review-panel">
            <div className="taf-campaign-detail-review-list">
              {snapshot.reviewRows.map((row) => (
                <div key={row.维度} className="taf-campaign-detail-review-row">
                  <strong>{row.维度}</strong>
                  <span>{row.结论}</span>
                  <StatusTag value={row.状态} />
                </div>
              ))}
            </div>
            <div className="taf-campaign-detail-review-links">
              <Link to={`/campaigns/${encodeURIComponent(snapshot.campaignId)}?view=review`}>查看复盘报告 &gt;</Link>
            </div>
          </WorkPanel>
        </aside>
      </div>
      <CampaignReportModal
        open={reportOpen}
        snapshot={snapshot}
        pending={reportMutation.isPending}
        reportStatus={reportStatus}
        onCancel={closeCampaignReport}
        onSubmit={async (payload) => {
          await reportMutation.mutateAsync(payload);
          setReportOpen(false);
        }}
      />
    </div>
  );
}

function CampaignImpactInlineContent({
  snapshot,
  activeImpact,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
}) {
  if (activeImpact === 'account') return <CampaignImpactAccountContent snapshot={snapshot} />;
  if (activeImpact === 'business-system') return <CampaignImpactBusinessSystemContent snapshot={snapshot} />;
  if (activeImpact === 'service') return <CampaignImpactServiceContent snapshot={snapshot} />;
  if (activeImpact === 'campus') return <CampaignImpactCampusContent snapshot={snapshot} />;
  if (activeImpact === 'department') return <CampaignImpactDepartmentContent snapshot={snapshot} />;
  return <CampaignImpactAssetCompact snapshot={snapshot} />;
}

export function CampaignImpactModalContent({
  snapshot,
  activeImpact,
  onImpactChange,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
}) {
  if (activeImpact === 'account') {
    return <CampaignImpactAccountPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} focus />;
  }
  if (activeImpact === 'business-system') {
    return <CampaignImpactBusinessSystemPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} focus />;
  }
  if (activeImpact === 'service') {
    return <CampaignImpactServicePanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} focus />;
  }
  if (activeImpact === 'campus') {
    return <CampaignImpactCampusPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} focus />;
  }
  if (activeImpact === 'department') {
    return <CampaignImpactDepartmentPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} focus />;
  }
  return (
    <section className="taf-campaign-impact-account-focus taf-campaign-impact-asset-focus" data-page-id="campaign-detail-impact-asset">
      <header><h1>影响范围</h1></header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactAssetCompact snapshot={snapshot} />
    </section>
  );
}

function CampaignImpactAssetCompact({ snapshot }: { snapshot: CampaignDetailSnapshot }) {
  const breakdown = snapshot.impactAsset.breakdown;
  const dataBacked = snapshot.impactDataBacked.asset;
  return (
    <div className="taf-campaign-detail-impact-compact">
      <div className="taf-campaign-detail-impact-chart">
        <DataQualityDonutChart
          ariaLabel="战役影响资产风险分布"
          rows={dataBacked
            ? breakdown.map((item, index) => ({
                label: item.label,
                value: item.count,
                color: ['#ff4d4f', '#faad14', '#75c743'][index] ?? '#42bfff',
              }))
            : [{ label: '数据不可用', value: 1, color: 'rgba(56,151,201,0.28)' }]}
        />
        <strong>{dataBacked ? snapshot.impactAsset.total : '--'}</strong>
        <span>受影响资产</span>
      </div>
      <div className="taf-campaign-detail-impact-risk-list">
        {breakdown.map((item, index) => (
          <span key={item.label}>
            <i style={{ backgroundColor: ['#ff4d4f', '#faad14', '#75c743'][index] ?? '#42bfff' }} />
            <b>{item.label}</b><strong>{dataBacked ? item.count : '--'}</strong><em>{dataBacked ? item.percent : '--'}</em>
          </span>
        ))}
      </div>
      <Table
        rowKey={(row) => row.资产}
        size="small"
        pagination={false}
        columns={assetColumns}
        dataSource={snapshot.impactAsset.rows}
        rowClassName={() => 'taf-campaign-detail-top-asset'}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={dataBacked ? '暂无影响资产' : '影响资产数据不可用'} /> }}
      />
    </div>
  );
}

function CampaignReportModal({
  open,
  snapshot,
  pending,
  reportStatus,
  onCancel,
  onSubmit,
}: {
  open: boolean;
  snapshot: CampaignDetailSnapshot;
  pending: boolean;
  reportStatus?: CampaignReportStatus;
  onCancel: () => void;
  onSubmit: (payload: Record<string, unknown>) => Promise<void>;
}) {
  const [selectedPhases, setSelectedPhases] = useState<string[] | null>(null);
  const [rootCause, setRootCause] = useState<string>();
  const [blocker, setBlocker] = useState<string>();
  const [residualRisk, setResidualRisk] = useState('中风险');
  const [recommendation, setRecommendation] = useState<string>();
  const [format, setFormat] = useState('PDF');
  const [includeAttachments, setIncludeAttachments] = useState(true);
  const phases = useMemo(() => snapshot.phases.map((phase) => phase.phase), [snapshot.phases]);
  const effectiveSelectedPhases = selectedPhases ?? phases;
  const readyEvidence = snapshot.evidenceSummaryRows.filter((row) => row.状态.includes('完整') || row.状态.includes('就绪')).length;
  useEffect(() => {
    if (!open) return;
    setSelectedPhases(phases);
    setRootCause(undefined);
    setBlocker(undefined);
    setResidualRisk('中风险');
    setRecommendation(undefined);
    setFormat('PDF');
    setIncludeAttachments(true);
  }, [open, phases, snapshot.campaignId]);
  return (
    <Modal
      className="taf-campaign-report-modal"
      width="min(1200px, calc(100dvw - 32px))"
      open={open}
      centered
      title={null}
      footer={null}
      onCancel={onCancel}
      destroyOnHidden={false}
    >
      <header className="taf-campaign-report-modal__header">
        <h2>生成战役报告</h2>
        <p>战役详情 / {snapshot.campaignId} / <StatusTag value={snapshot.status} /></p>
        <span>Trace ID：{snapshot.campaignId.replace(/[^A-Za-z0-9]/g, '').slice(-16) || '--'}</span>
      </header>
      <div className="taf-campaign-report-modal__metrics">
        <ReportMetric icon={<NodeIndexOutlined />} label="战役阶段" value={`${snapshot.phases.length} 个阶段`} tone="info" />
        <ReportMetric icon={<SafetyOutlined />} label="关联告警" value={`${snapshot.alertCount} 条`} tone="risk" />
        <ReportMetric icon={<DatabaseOutlined />} label="影响资产" value={`${snapshot.assetCount} 个`} tone="warn" />
        <ReportMetric icon={<SafetyCertificateOutlined />} label="证据完整度" value={snapshot.evidenceCompletenessAvailable ? `${snapshot.evidenceCompleteness}%` : '--'} tone="ok" />
        <ReportMetric icon={<UserSwitchOutlined />} label="执行状态" value={campaignReportStatusLabel(reportStatus?.status)} tone={reportStatus?.status === 'completed' ? 'ok' : reportStatus?.status === 'failed' ? 'risk' : 'warn'} />
      </div>
      <div className="taf-campaign-report-modal__body">
        <section className="taf-campaign-report-scope">
          <h3>报告范围</h3>
          <div className="taf-campaign-report-scope__title"><b>攻击阶段（已选 {effectiveSelectedPhases.length}/{phases.length}）</b><button type="button" onClick={() => setSelectedPhases(phases)}>全选</button></div>
          <Checkbox.Group value={effectiveSelectedPhases} onChange={(values) => setSelectedPhases(values.map(String))}>
            {phases.map((phase) => <Checkbox key={phase} value={phase}>{phase}</Checkbox>)}
          </Checkbox.Group>
          <label><span>时间窗口</span><strong>{snapshot.firstSeen} ~ {snapshot.lastUpdated}</strong></label>
          <label><span>资产范围</span><strong>所有受影响资产（{snapshot.assetCount}）</strong></label>
          <label><span>地域范围</span><strong>{snapshot.impactCampus.total ? `${snapshot.impactCampus.total} 个校区` : '未提供'}</strong></label>
          <p>将包含选定阶段内的关键事件、证据与处置记录。</p>
        </section>
        <section className="taf-campaign-report-evidence">
          <h3>证据包 <span>总计 {snapshot.evidenceSummaryRows.length} 类</span></h3>
          <Table
            size="small"
            pagination={false}
            rowKey="证据类型"
            dataSource={snapshot.evidenceSummaryRows}
            columns={[
              { title: '类型', dataIndex: '证据类型' },
              { title: '数量', dataIndex: '文件记录' },
              { title: '完整度', dataIndex: '完整度' },
              { title: 'Hash 校验', render: (_, row) => <span className={row.状态.includes('完整') || row.状态.includes('就绪') ? 'is-ok' : ''}>{row.状态.includes('完整') || row.状态.includes('就绪') ? '已登记' : '未提供'}</span> },
              { title: '状态', dataIndex: '状态', render: (value) => <StatusTag value={value} /> },
            ]}
            locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无真实证据包数据" /> }}
          />
          <Alert type="warning" showIcon message={`证据包已就绪 ${readyEvidence}/${snapshot.evidenceSummaryRows.length} 类，导出结果受权限与审批控制。`} />
        </section>
        <section className="taf-campaign-report-review">
          <h3>复盘结论</h3>
          <ReportSelect label="根因分析" value={rootCause} onChange={setRootCause} options={['钓鱼邮件', '凭证泄露', '弱口令', '供应链风险']} />
          <ReportSelect label="关键阻断点" value={blocker} onChange={setBlocker} options={['入口封禁', '账号吊销', '网络隔离', '域名阻断']} />
          <ReportSelect label="遗留风险" value={residualRisk} onChange={setResidualRisk} options={['高风险', '中风险', '低风险']} />
          <ReportSelect label="整改建议" value={recommendation} onChange={setRecommendation} options={['加强终端检测', '完善账号基线', '收紧出口访问', '补齐证据采集']} />
          <label className="taf-campaign-report-format"><span>报告模板</span><Select value="战役复盘报告（标准版）" options={[{ value: '战役复盘报告（标准版）' }]} /></label>
          <div className="taf-campaign-report-format-buttons">
            {['PDF', 'Word'].map((item) => <button type="button" key={item} className={format === item ? 'is-active' : ''} onClick={() => setFormat(item)}>{item}</button>)}
          </div>
          <Checkbox checked={includeAttachments} onChange={(event) => setIncludeAttachments(event.target.checked)}>包含附件（原始证据清单）</Checkbox>
        </section>
      </div>
      <footer className="taf-campaign-report-modal__footer">
        <Alert
          type={reportStatus?.status === 'completed' ? 'success' : reportStatus?.status === 'failed' ? 'error' : 'info'}
          showIcon
          message={reportStatus ? `报告状态：${campaignReportStatusLabel(reportStatus.status)}` : '提交后将冻结快照并等待服务端最终结果'}
          description={reportStatus
            ? `report_id=${reportStatus.reportId}，尝试次数=${reportStatus.attempts}${reportStatus.errorMessage ? `，最近错误=${reportStatus.errorMessage}` : ''}`
            : 'HTTP 202 仅代表已受理；完成后浏览器将校验对象大小与摘要，再触发下载并写入审计。'}
        />
        <Space>
          <Button onClick={onCancel}>取消</Button>
          <Button onClick={() => message.info('报告预览将在生成任务完成后开放')}>预览报告</Button>
          <Button
            type="primary"
            loading={pending}
            disabled={!effectiveSelectedPhases.length}
            onClick={() => void onSubmit({
              format: format.toLowerCase(),
              phases: effectiveSelectedPhases,
              sections: effectiveSelectedPhases,
              evidence_count: snapshot.evidenceSummaryRows.length,
              root_cause: rootCause,
              key_blocker: blocker,
              residual_risk: residualRisk,
              recommendation,
              include_attachments: includeAttachments,
            }).catch(() => {})}
          >
            生成、校验并下载
          </Button>
        </Space>
      </footer>
    </Modal>
  );
}

function campaignReportStatusLabel(status?: CampaignReportStatus['status']) {
  switch (status) {
    case 'accepted': return '已受理';
    case 'running': return '执行中';
    case 'completed': return '最终成功';
    case 'partial': return '部分完成';
    case 'failed': return '失败';
    case 'cancelled': return '已取消';
    case 'compensated': return '已补偿';
    default: return '尚未提交';
  }
}

function ReportMetric({ icon, label, value, tone }: { icon: ReactNode; label: string; value: string; tone: string }) {
  return <span className={`is-${tone}`}><i>{icon}</i><b>{label}</b><strong>{value}</strong></span>;
}

function ReportSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value?: string;
  onChange: (value: string) => void;
  options: string[];
}) {
  return <label><span>{label}</span><Select placeholder="未选择" value={value} onChange={onChange} options={options.map((item) => ({ value: item }))} /></label>;
}

function ProfileFact({ label, value, status = false }: { label: string; value: string; status?: boolean }) {
  return (
    <div className="taf-campaign-detail-profile-fact">
      <span>{label}</span>
      {status ? <StatusTag value={value} /> : <strong>{value}</strong>}
    </div>
  );
}

function compactTimestamp(value: string) {
  return value.replace(/^\d{4}-/, '').replace('T', ' ').slice(0, 14);
}

function ImpactTabs({
  snapshot,
  activeImpact,
  onImpactChange,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
}) {
  return (
    <div className="taf-campaign-detail-impact-tabs">
      {snapshot.impactTabs.map((item) => {
        const id = impactId(item.label);
        return (
          <button
            key={item.label}
            type="button"
            className={`taf-campaign-detail-impact-tab is-${item.status}${id === activeImpact ? ' is-active' : ''}`}
            onClick={() => onImpactChange(id)}
          >
            {impactIcon(item.label)}
            <span>{item.label}</span>
            <strong>{item.value}</strong>
          </button>
        );
      })}
    </div>
  );
}

function CampaignImpactAccountPanel({
  snapshot,
  activeImpact,
  onImpactChange,
  focus = false,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
  focus?: boolean;
}) {
  return (
    <section className={focus ? 'taf-campaign-impact-account-focus' : 'taf-campaign-impact-account-panel'} data-page-id="campaign-detail-impact-account">
      <header>
        <h1>影响范围</h1>
      </header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactAccountContent snapshot={snapshot} focus={focus} />
    </section>
  );
}

function CampaignImpactAccountContent({ snapshot, focus = false }: { snapshot: CampaignDetailSnapshot; focus?: boolean }) {
  const breakdown = snapshot.impactAccount.breakdown;
  const total = snapshot.impactAccount.total;
  return (
    <div className={focus ? 'taf-campaign-impact-account-content is-focus' : 'taf-campaign-impact-account-content'}>
      <CampaignImpactRiskSummary
        total={total}
        unit={snapshot.impactAccount.unit}
        breakdown={breakdown}
        dataBacked={snapshot.impactDataBacked.account}
      />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键账号（Top 5）</h2>
        <AccountImpactTable rows={snapshot.impactAccount.rows} dataBacked={snapshot.impactDataBacked.account} />
        <Link className="taf-campaign-impact-account-all-link" to="/baselines?tab=account">查看全部账号 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactBusinessSystemPanel({
  snapshot,
  activeImpact,
  onImpactChange,
  focus = false,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
  focus?: boolean;
}) {
  const className = focus
    ? 'taf-campaign-impact-account-focus taf-campaign-impact-entity-focus taf-campaign-impact-business-system-focus'
    : 'taf-campaign-impact-account-panel taf-campaign-impact-business-system-panel';
  return (
    <section className={className} data-page-id="campaign-detail-impact-business-system">
      <header><h1>影响范围</h1></header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactBusinessSystemContent snapshot={snapshot} focus={focus} />
    </section>
  );
}

function CampaignImpactBusinessSystemContent({ snapshot, focus = false }: { snapshot: CampaignDetailSnapshot; focus?: boolean }) {
  const impact = snapshot.impactBusinessSystem;
  return (
    <div className={`${focus ? 'taf-campaign-impact-account-content is-focus taf-campaign-impact-entity-content' : 'taf-campaign-impact-account-content'} taf-campaign-impact-business-system-content`}>
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} dataBacked={snapshot.impactDataBacked['business-system']} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键业务系统（Top 5）</h2>
        <BusinessSystemImpactTable rows={impact.rows} dataBacked={snapshot.impactDataBacked['business-system']} />
        <Link className="taf-campaign-impact-account-all-link" to="/assets?tab=business-system">查看全部业务系统 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactServicePanel({
  snapshot,
  activeImpact,
  onImpactChange,
  focus = false,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
  focus?: boolean;
}) {
  const className = focus
    ? 'taf-campaign-impact-account-focus taf-campaign-impact-entity-focus taf-campaign-impact-service-focus'
    : 'taf-campaign-impact-account-panel taf-campaign-impact-service-panel';
  return (
    <section className={className} data-page-id="campaign-detail-impact-service">
      <header><h1>影响范围</h1></header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactServiceContent snapshot={snapshot} focus={focus} />
    </section>
  );
}

function CampaignImpactServiceContent({ snapshot, focus = false }: { snapshot: CampaignDetailSnapshot; focus?: boolean }) {
  const impact = snapshot.impactService;
  return (
    <div className={`${focus ? 'taf-campaign-impact-account-content is-focus taf-campaign-impact-entity-content' : 'taf-campaign-impact-account-content'} taf-campaign-impact-service-content`}>
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} dataBacked={snapshot.impactDataBacked.service} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键服务（Top 5）</h2>
        <ServiceImpactTable rows={impact.rows} dataBacked={snapshot.impactDataBacked.service} />
        <Link className="taf-campaign-impact-account-all-link" to="/assets?tab=service">查看全部服务 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactCampusPanel({
  snapshot,
  activeImpact,
  onImpactChange,
  focus = false,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
  focus?: boolean;
}) {
  const className = focus
    ? 'taf-campaign-impact-account-focus taf-campaign-impact-entity-focus taf-campaign-impact-campus-focus'
    : 'taf-campaign-impact-account-panel taf-campaign-impact-campus-panel';
  return (
    <section className={className} data-page-id="campaign-detail-impact-campus">
      <header><h1>影响范围</h1></header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactCampusContent snapshot={snapshot} focus={focus} />
    </section>
  );
}

function CampaignImpactCampusContent({ snapshot, focus = false }: { snapshot: CampaignDetailSnapshot; focus?: boolean }) {
  const impact = snapshot.impactCampus;
  return (
    <div className={`${focus ? 'taf-campaign-impact-account-content is-focus taf-campaign-impact-entity-content' : 'taf-campaign-impact-account-content'} taf-campaign-impact-campus-content`}>
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} dataBacked={snapshot.impactDataBacked.campus} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键校区（Top 5）</h2>
        <CampusImpactTable rows={impact.rows} dataBacked={snapshot.impactDataBacked.campus} />
        <Link className="taf-campaign-impact-account-all-link" to="/assets?tab=campus">查看全部校区 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactDepartmentPanel({
  snapshot,
  activeImpact,
  onImpactChange,
  focus = false,
}: {
  snapshot: CampaignDetailSnapshot;
  activeImpact: string;
  onImpactChange: (impact: string) => void;
  focus?: boolean;
}) {
  const className = focus
    ? 'taf-campaign-impact-account-focus taf-campaign-impact-entity-focus taf-campaign-impact-department-focus'
    : 'taf-campaign-impact-account-panel taf-campaign-impact-department-panel';
  return (
    <section className={className} data-page-id="campaign-detail-impact-department">
      <header><h1>影响范围</h1></header>
      <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={onImpactChange} />
      <CampaignImpactDepartmentContent snapshot={snapshot} focus={focus} />
    </section>
  );
}

function CampaignImpactDepartmentContent({ snapshot, focus = false }: { snapshot: CampaignDetailSnapshot; focus?: boolean }) {
  const impact = snapshot.impactDepartment;
  return (
    <div className={`${focus ? 'taf-campaign-impact-account-content is-focus taf-campaign-impact-entity-content' : 'taf-campaign-impact-account-content'} taf-campaign-impact-department-content`}>
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} dataBacked={snapshot.impactDataBacked.department} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键部门（Top 5）</h2>
        <DepartmentImpactTable rows={impact.rows} dataBacked={snapshot.impactDataBacked.department} />
        <Link className="taf-campaign-impact-account-all-link" to="/assets?tab=department">查看全部部门 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactRiskSummary({
  total,
  unit,
  breakdown,
  dataBacked = true,
}: {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
  dataBacked?: boolean;
}) {
  const palette = ['#ff3b3f', '#ffad18', '#75c743'];
  return (
    <div className="taf-campaign-impact-account-summary">
      <div className="taf-campaign-impact-account-donut" aria-label={`${total} ${unit}`}>
        <DataQualityDonutChart
          ariaLabel={`影响范围风险分布，共 ${total} ${unit}`}
          rows={dataBacked
            ? breakdown.map((item, index) => ({
                label: item.label,
                value: item.count,
                color: palette[index] ?? '#42bfff',
              }))
            : [{ label: '数据不可用', value: 1, color: 'rgba(56,151,201,0.28)' }]}
        />
        <div className="taf-campaign-impact-account-donut__label"><strong>{dataBacked ? total : '--'}</strong><span>{unit}</span></div>
      </div>
      <div className="taf-campaign-impact-account-risk-list">
        {dataBacked
          ? breakdown.map((item) => <RiskBreakdownRow key={item.label} item={item} />)
          : <div className="taf-campaign-impact-empty">数据不可用</div>}
      </div>
    </div>
  );
}

function RiskBreakdownRow({ item }: { item: CampaignDetailImpactRiskRow }) {
  return (
    <div className={`taf-campaign-impact-account-risk-row is-${item.status}`}>
      <i />
      <span>{item.label}</span>
      <strong>{item.count}</strong>
      <em>{item.percent}</em>
    </div>
  );
}

function ImpactTableEmpty({ dataBacked }: { dataBacked: boolean }) {
  return <div className="taf-campaign-impact-table-empty">{dataBacked ? '暂无影响记录' : '数据不可用'}</div>;
}

function AccountImpactTable({ rows, dataBacked }: { rows: CampaignDetailAccountRow[]; dataBacked: boolean }) {
  return (
    <div className="taf-campaign-impact-account-table" role="table" aria-label="关键账号 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">账号</span>
        <span role="columnheader">账号类型</span>
        <span role="columnheader">权限风险</span>
        <span role="columnheader">登录链路</span>
      </div>
      {!rows.length && <ImpactTableEmpty dataBacked={dataBacked} />}
      {rows.map((row) => (
        <div key={row.账号} className="taf-campaign-impact-account-table__row" role="row">
          <strong role="cell" title={row.账号}>{row.账号}</strong>
          <span role="cell" title={row.账号类型}>{row.账号类型}</span>
          <span role="cell"><b className={row.权限风险.includes('高') ? 'is-risk' : 'is-warn'}>{row.权限风险}</b></span>
          <em role="cell" title={row.登录链路}>{row.登录链路}</em>
        </div>
      ))}
    </div>
  );
}

function BusinessSystemImpactTable({ rows, dataBacked }: { rows: CampaignDetailBusinessSystemRow[]; dataBacked: boolean }) {
  return (
    <div className="taf-campaign-impact-account-table" role="table" aria-label="关键业务系统 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">业务系统</span>
        <span role="columnheader">关键服务</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">恢复优先级</span>
      </div>
      {!rows.length && <ImpactTableEmpty dataBacked={dataBacked} />}
      {rows.map((row) => (
        <div key={row.业务系统} className="taf-campaign-impact-account-table__row" role="row">
          <strong role="cell" title={row.业务系统}>{row.业务系统}</strong>
          <span role="cell" title={row.关键服务}>{row.关键服务}</span>
          <span role="cell"><b className={row.风险.includes('高') ? 'is-risk' : 'is-warn'}>{row.风险}</b></span>
          <em role="cell"><b className={priorityClass(row.恢复优先级)}>{row.恢复优先级}</b></em>
        </div>
      ))}
    </div>
  );
}

function ServiceImpactTable({ rows, dataBacked }: { rows: CampaignDetailServiceRow[]; dataBacked: boolean }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-service-table" role="table" aria-label="关键服务 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">服务名称</span>
        <span role="columnheader">端口/协议</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">依赖关系</span>
      </div>
      {!rows.length && <ImpactTableEmpty dataBacked={dataBacked} />}
      {rows.map((row) => (
        <div key={`${row.服务名称}-${row.端口协议}`} className="taf-campaign-impact-account-table__row" role="row">
          <strong role="cell" title={row.服务名称}>{row.服务名称}</strong>
          <span role="cell" title={row.端口协议}>{row.端口协议}</span>
          <span role="cell"><b className={riskClass(row.风险)}>{row.风险}</b></span>
          <em role="cell" title={row.依赖关系}>{row.依赖关系}</em>
        </div>
      ))}
    </div>
  );
}

function CampusImpactTable({ rows, dataBacked }: { rows: CampaignDetailCampusRow[]; dataBacked: boolean }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-campus-table" role="table" aria-label="关键校区 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">校区/楼宇</span>
        <span role="columnheader">覆盖资产</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">链路</span>
      </div>
      {!rows.length && <ImpactTableEmpty dataBacked={dataBacked} />}
      {rows.map((row) => (
        <div key={row.校区楼宇} className="taf-campaign-impact-account-table__row" role="row">
          <strong role="cell" title={row.校区楼宇}>{row.校区楼宇}</strong>
          <span role="cell">{row.覆盖资产}</span>
          <span role="cell"><b className={riskClass(row.风险)}>{row.风险}</b></span>
          <em role="cell">
            <b className={`taf-campaign-impact-campus-link ${riskClass(row.风险)}`}>
              {campusPathIcon(row.链路)}
              <span>{row.链路}</span>
            </b>
          </em>
        </div>
      ))}
    </div>
  );
}

function DepartmentImpactTable({ rows, dataBacked }: { rows: CampaignDetailDepartmentRow[]; dataBacked: boolean }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-department-table" role="table" aria-label="关键部门 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">部门名称</span>
        <span role="columnheader">责任人</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">处置进度</span>
      </div>
      {!rows.length && <ImpactTableEmpty dataBacked={dataBacked} />}
      {rows.map((row) => (
        <div key={row.部门名称} className="taf-campaign-impact-account-table__row" role="row">
          <strong role="cell" title={row.部门名称}>{row.部门名称}</strong>
          <span role="cell" title={row.责任人}>{row.责任人}</span>
          <span role="cell"><b className={riskClass(row.风险)}>{row.风险}</b></span>
          <em role="cell">
            <span
              className={`taf-campaign-impact-department-progress ${riskClass(row.风险)}`}
              style={{ '--taf-impact-progress': `${row.处置进度}%` } as CSSProperties}
            >
              <i />
              <b>{row.处置进度}%</b>
            </span>
          </em>
        </div>
      ))}
    </div>
  );
}

function campusPathIcon(path: string) {
  if (path.includes('核心')) return <LinkOutlined />;
  if (path.includes('东西')) return <SwapOutlined />;
  if (path.includes('VPN')) return <SafetyOutlined className="taf-campaign-impact-campus-shield-plus" />;
  if (path.includes('无线')) return <WifiOutlined />;
  return <UploadOutlined />;
}

function phaseIcon(index: number) {
  const icons = [
    <SafetyCertificateOutlined key="shield" />,
    <FileProtectOutlined key="file" />,
    <DatabaseOutlined key="database" />,
    <ForkOutlined key="fork" />,
    <NodeIndexOutlined key="node" />,
    <CloudDownloadOutlined key="download" />,
    <AuditOutlined key="audit" />,
  ];
  return icons[index] ?? <FlagOutlined />;
}

function responseStepIcon(index: number) {
  const icons = [
    <SafetyCertificateOutlined key="discover" />,
    <SearchOutlined key="triage" />,
    <WarningOutlined key="contain" />,
    <DeleteOutlined key="eradicate" />,
    <SyncOutlined key="recover" />,
    <FileTextOutlined key="review" />,
  ];
  return icons[index] ?? <SafetyCertificateOutlined />;
}

function impactIcon(label: string) {
  if (label === '资产') return <ApartmentOutlined />;
  if (label === '账号') return <TeamOutlined />;
  if (label === '服务') return <SafetyCertificateOutlined />;
  if (label === '部门') return <UserSwitchOutlined />;
  if (label === '园区' || label === '校区') return <NodeIndexOutlined />;
  return <BranchesOutlined />;
}

function impactId(label: string) {
  if (label === '账号') return 'account';
  if (label === '服务') return 'service';
  if (label === '部门') return 'department';
  if (label === '园区' || label === '校区') return 'campus';
  if (label === '业务系统') return 'business-system';
  return 'asset';
}

function resolveCampaignImpact(value: string | null) {
  const normalized = (value || '').toLowerCase();
  if (['account', 'accounts', '账号'].includes(normalized)) return 'account';
  if (['service', 'services', '服务'].includes(normalized)) return 'service';
  if (['department', 'dept', '部门'].includes(normalized)) return 'department';
  if (['campus', '园区', '校区'].includes(normalized)) return 'campus';
  if (['business-system', 'business', 'system', '业务系统'].includes(normalized)) return 'business-system';
  return 'asset';
}

function resolveAlertRisk(value: string | null) {
  return ['高危', '中危', '低危'].includes(value || '') ? String(value) : '全部';
}

function impactDestination(impact: string) {
  if (impact === 'account') return { href: '/baselines?tab=account', label: '查看全部账号' };
  if (impact === 'service') return { href: '/assets?tab=service', label: '查看全部服务' };
  if (impact === 'department') return { href: '/assets?tab=department', label: '查看全部部门' };
  if (impact === 'campus') return { href: '/assets?tab=campus', label: '查看全部校区' };
  if (impact === 'business-system') return { href: '/assets?tab=business-system', label: '查看全部业务系统' };
  return { href: '/assets', label: '查看全部资产' };
}

function priorityClass(priority: string) {
  if (priority === 'P0') return 'is-risk';
  if (priority === 'P2') return 'is-ok';
  return 'is-warn';
}

function riskClass(risk: string) {
  if (risk.includes('高')) return 'is-risk';
  if (risk.includes('低')) return 'is-ok';
  return 'is-warn';
}

function emptySnapshot(campaignId: string): CampaignDetailSnapshot {
  return {
    campaignId,
    stateVersion: 0,
    snapshotId: '',
    snapshotSHA256: '',
    partial: true,
    missingSections: ['campaign_snapshot'],
    sourceWatermarks: {},
    reports: [],
    campaignType: '未分类',
    title: '战役详情加载中',
    riskScore: 0,
    currentPhase: '-',
    duration: '-',
    firstSeen: '-',
    lastUpdated: '-',
    status: '加载中',
    activityStatus: '加载中',
    workflowStatus: '加载中',
    assignee: '-',
    alertCount: 0,
    assetCount: 0,
    tags: [],
    summary: '-',
    profileFacts: [],
    metrics: [],
    phases: [],
    alerts: [],
    impactTabs: [],
    topAssets: [],
    impactAsset: {
      total: 0,
      unit: '受影响资产',
      breakdown: [],
      rows: [],
    },
    impactAccount: {
      total: 0,
      unit: '受影响账号',
      breakdown: [],
      rows: [],
    },
    impactBusinessSystem: {
      total: 0,
      unit: '受影响系统',
      breakdown: [],
      rows: [],
    },
    impactService: {
      total: 0,
      unit: '受影响服务',
      breakdown: [],
      rows: [],
    },
    impactDepartment: {
      total: 0,
      unit: '受影响部门',
      breakdown: [],
      rows: [],
    },
    impactCampus: {
      total: 0,
      unit: '受影响校区',
      breakdown: [],
      rows: [],
    },
    impactDataBacked: {
      asset: false,
      account: false,
      service: false,
      department: false,
      campus: false,
      'business-system': false,
    },
    evidenceCompleteness: 0,
    evidenceCompletenessAvailable: false,
    phaseDataBacked: false,
    evidenceRail: [],
    statusTransitions: [],
    evidenceChecks: [],
    evidenceSummaryRows: [],
    evidenceDigest: [
      { label: '首个可疑文件', value: '--' },
      { label: 'SHA256', value: '--' },
      { label: '首次外联域名', value: '--' },
      { label: '解析 IP', value: '--' },
      { label: '首次外联时间', value: '--' },
    ],
    responseFlow: [],
    responseActions: [],
    reviewRows: [],
    evidence: [],
  };
}
