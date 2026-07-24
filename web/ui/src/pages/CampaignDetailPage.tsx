import {
  ApartmentOutlined,
  ArrowLeftOutlined,
  AuditOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloudDownloadOutlined,
  DatabaseOutlined,
  FileDoneOutlined,
  FileProtectOutlined,
  FlagOutlined,
  ForkOutlined,
  LinkOutlined,
  MoreOutlined,
  NodeIndexOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SafetyOutlined,
  SwapOutlined,
  TeamOutlined,
  UploadOutlined,
  UserSwitchOutlined,
  WifiOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Empty, Progress, Space, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { CSSProperties } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  fetchCampaignDetailSnapshot,
  type CampaignDetailAccountRow,
  type CampaignDetailAlertRow,
  type CampaignDetailAssetRow,
  type CampaignDetailBusinessSystemRow,
  type CampaignDetailCampusRow,
  type CampaignDetailDepartmentRow,
  type CampaignDetailEvidenceSummaryRow,
  type CampaignDetailImpactRiskRow,
  type CampaignDetailServiceRow,
  type CampaignDetailSnapshot,
} from '@/services/campaignDetailApi';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

const cellTitle = <T extends Record<string, string>>(key: keyof T) => (record: T) => ({
  title: record[key],
});

const alertColumns: ColumnsType<CampaignDetailAlertRow> = [
  { title: '告警时间', dataIndex: '告警时间', key: '告警时间', width: 116, onCell: cellTitle('告警时间') },
  { title: '告警 ID', dataIndex: '告警ID', key: '告警ID', width: 146, ellipsis: true, onCell: cellTitle('告警ID') },
  { title: '告警名称', dataIndex: '告警名称', key: '告警名称', ellipsis: true, onCell: cellTitle('告警名称') },
  { title: '攻击阶段', dataIndex: '攻击阶段', key: '攻击阶段', width: 94, onCell: cellTitle('攻击阶段') },
  { title: '影响资产', dataIndex: '影响资产', key: '影响资产', width: 82, onCell: cellTitle('影响资产') },
  { title: '风险', dataIndex: '风险', key: '风险', width: 74, render: (value) => <StatusTag value={value} /> },
  { title: '状态', dataIndex: '状态', key: '状态', width: 84, render: (value) => <StatusTag value={value} /> },
  {
    title: '操作',
    dataIndex: '操作',
    key: '操作',
    width: 68,
    render: (value, record) => <Link to={`/alerts/${encodeURIComponent(record.告警ID)}`}>{String(value)}</Link>,
  },
];

const assetColumns: ColumnsType<CampaignDetailAssetRow> = [
  { title: '资产', dataIndex: '资产', key: '资产', ellipsis: true, onCell: cellTitle('资产') },
  { title: '类型', dataIndex: '类型', key: '类型', width: 74, onCell: cellTitle('类型') },
  { title: '部门', dataIndex: '部门', key: '部门', width: 86, onCell: cellTitle('部门') },
  { title: '业务系统', dataIndex: '业务系统', key: '业务系统', width: 104, ellipsis: true, onCell: cellTitle('业务系统') },
  { title: '风险', dataIndex: '风险', key: '风险', width: 74, render: (value) => <StatusTag value={value} /> },
  { title: '证据', dataIndex: '证据', key: '证据', width: 112, ellipsis: true, onCell: cellTitle('证据') },
];

const evidenceColumns: ColumnsType<CampaignDetailEvidenceSummaryRow> = [
  { title: '证据类型', dataIndex: '证据类型', key: '证据类型', width: 86, onCell: cellTitle('证据类型') },
  { title: '文件记录', dataIndex: '文件记录', key: '文件记录', ellipsis: true, onCell: cellTitle('文件记录') },
  { title: '完整度', dataIndex: '完整度', key: '完整度', width: 76, onCell: cellTitle('完整度') },
  { title: '状态', dataIndex: '状态', key: '状态', width: 86, render: (value) => <StatusTag value={value} /> },
];

const campaignDetailOverlays: OverlayContract[] = [
  {
    id: 'modal-campaign-report-export',
    title: '战役报告导出',
    kind: 'Modal',
    actionLabel: '报告导出',
    description: '导出战役阶段、关联告警、影响资产、证据包和复盘结论。',
    impact: '生成可审计报告材料并绑定当前战役 ID。',
  },
];

export function CampaignDetailPage({ route }: { route: NavRoute }) {
  const params = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const visualBreakdownMode = isVisualBreakdownMode();
  const campaignId = params.campaignId ?? 'APT-20260619-001';
  const activeImpact = resolveCampaignImpact(searchParams.get('impact') || searchParams.get('tab'));
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['campaign-detail', campaignId],
    queryFn: () => fetchCampaignDetailSnapshot(campaignId),
    refetchInterval: visualBreakdownMode ? false : 30_000,
  });

  const snapshot = data ?? emptySnapshot(campaignId);
  const changeImpact = (nextImpact: string) => {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set('impact', nextImpact);
      return next;
    });
  };

  if (visualBreakdownMode && activeImpact === 'account') {
    return (
      <div className="taf-page taf-campaign-detail-page taf-campaign-impact-account-visual-page">
        <CampaignImpactAccountPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} focus />
      </div>
    );
  }

  if (visualBreakdownMode && activeImpact === 'business-system') {
    return (
      <div className="taf-page taf-campaign-detail-page taf-campaign-impact-account-visual-page">
        <CampaignImpactBusinessSystemPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} focus />
      </div>
    );
  }

  if (visualBreakdownMode && activeImpact === 'service') {
    return (
      <div className="taf-page taf-campaign-detail-page taf-campaign-impact-account-visual-page">
        <CampaignImpactServicePanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} focus />
      </div>
    );
  }

  if (visualBreakdownMode && activeImpact === 'campus') {
    return (
      <div className="taf-page taf-campaign-detail-page taf-campaign-impact-account-visual-page">
        <CampaignImpactCampusPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} focus />
      </div>
    );
  }

  if (visualBreakdownMode && activeImpact === 'department') {
    return (
      <div className="taf-page taf-campaign-detail-page taf-campaign-impact-account-visual-page">
        <CampaignImpactDepartmentPanel snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} focus />
      </div>
    );
  }

  return (
    <div className="taf-page taf-campaign-detail-page">
      <header className="taf-campaign-detail-titlebar">
        <div className="taf-campaign-detail-titlebar__page-title">
          <h1 title={route.page.title}>{route.page.title}</h1>
        </div>
        <Space size={8}>
          <Button size="small" icon={<CloudDownloadOutlined />}>导出战役包</Button>
          <Button size="small" type="primary" icon={<FileDoneOutlined />}>生成战役报告</Button>
          <Button size="small" icon={<AuditOutlined />}>写入审计</Button>
          <Button size="small" icon={<MoreOutlined />}>更多</Button>
          <OverlayContractHost overlays={campaignDetailOverlays} compact />
          <Tooltip title="返回战役列表">
            <Button size="small" icon={<ArrowLeftOutlined />} aria-label="返回战役列表" onClick={() => navigate('/campaigns')} />
          </Tooltip>
          <Tooltip title="刷新战役详情">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
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
            <span>{snapshot.campaignId}</span>
            <h2>{snapshot.title}</h2>
            <p>{snapshot.summary}</p>
            <div className="taf-campaign-detail-tags">
              {snapshot.tags.map((tag) => <b key={tag}>{tag}</b>)}
            </div>
          </div>
        </div>
        <div className="taf-campaign-detail-risk">
          <Progress type="circle" percent={snapshot.riskScore} size={82} strokeColor="#ff4d4f" format={() => snapshot.riskScore} />
          <strong>{snapshot.currentPhase}</strong>
        </div>
        <div className="taf-campaign-detail-profile-facts">
          {snapshot.profileFacts.map((item) => (
            <ProfileFact key={item.label} label={item.label} value={item.value} status={item.status} />
          ))}
        </div>
      </section>

      <div className="taf-campaign-detail-metrics">
        {snapshot.metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
      </div>

      <WorkPanel title="攻击时间轴（从发现到闭环）" className="taf-campaign-detail-phase-panel" extra={<Link to="/attack-chains">下钻攻击链</Link>}>
        <div className="taf-campaign-detail-phase-cards">
          {snapshot.phases.map((phase, index) => (
            <div key={phase.phase} className={`taf-campaign-detail-phase-card is-${phase.status}`}>
              <header>
                <i>{phaseIcon(index)}</i>
                <span>{phase.time}</span>
              </header>
              <strong>{phase.phase}</strong>
              <p>{phase.summary}</p>
              <footer>
                <b>{phase.alertCount} 告警</b>
                <b>{phase.evidenceCount} 证据</b>
              </footer>
            </div>
          ))}
        </div>
        <div className="taf-campaign-detail-phase-track">
          {snapshot.phases.map((phase) => <span key={phase.phase} className={`taf-campaign-detail-phase-dot is-${phase.status}`} />)}
        </div>
      </WorkPanel>

      <div className="taf-campaign-detail-grid">
        <main className="taf-campaign-detail-main">
          <WorkPanel title={`关联告警（${snapshot.alertCount}）`} className="taf-campaign-detail-alerts" extra={<Link to="/alerts">查看告警中心</Link>}>
            <div className="taf-campaign-detail-alert-filter">
              {['全部', '高危', '横向移动', 'C2通信', '数据外传'].map((label) => <button key={label} type="button">{label}</button>)}
            </div>
            <Table
              rowKey={(row) => row.告警ID}
              size="small"
              loading={isLoading}
              pagination={false}
              columns={alertColumns}
              dataSource={snapshot.alerts}
              locale={{
                emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无关联告警" />,
              }}
            />
          </WorkPanel>

          <div className="taf-campaign-detail-lower">
            <WorkPanel title="影响范围" className="taf-campaign-detail-impact-panel" extra={<Link to="/assets">资产台账</Link>}>
              <ImpactTabs snapshot={snapshot} activeImpact={activeImpact} onImpactChange={changeImpact} />
              {activeImpact === 'account' ? (
                <CampaignImpactAccountContent snapshot={snapshot} />
              ) : activeImpact === 'business-system' ? (
                <CampaignImpactBusinessSystemContent snapshot={snapshot} />
              ) : activeImpact === 'service' ? (
                <CampaignImpactServiceContent snapshot={snapshot} />
              ) : activeImpact === 'campus' ? (
                <CampaignImpactCampusContent snapshot={snapshot} />
              ) : activeImpact === 'department' ? (
                <CampaignImpactDepartmentContent snapshot={snapshot} />
              ) : (
                <Table
                  rowKey={(row) => row.资产}
                  size="small"
                  pagination={false}
                  columns={assetColumns}
                  dataSource={snapshot.topAssets}
                  rowClassName={() => 'taf-campaign-detail-top-asset'}
                  locale={{
                    emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无影响资产" />,
                  }}
                />
              )}
            </WorkPanel>

            <WorkPanel title="证据包" className="taf-campaign-detail-evidence-panel" extra={<Link to="/forensics">取证分析</Link>}>
              <div className="taf-campaign-detail-evidence-overview">
                <Progress type="circle" percent={snapshot.evidenceCompleteness} size={62} strokeColor="#36d66b" />
                <div>
                  {snapshot.evidenceChecks.map((item) => (
                    <span key={item.label} className={`taf-campaign-detail-evidence-check is-${item.status}`}>
                      <b>{item.label}</b>
                      <i><em style={{ width: item.label === '完整度' ? item.value : '88%' }} /></i>
                      <strong>{item.value}</strong>
                    </span>
                  ))}
                </div>
              </div>
              <Table
                rowKey={(row) => row.证据类型}
                size="small"
                pagination={false}
                columns={evidenceColumns}
                dataSource={snapshot.evidenceSummaryRows}
                locale={{
                  emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无证据包摘要" />,
                }}
              />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-campaign-detail-rail">
          <WorkPanel title="处置流程" extra={<Link to="/playbooks">SOAR 剧本</Link>}>
            <div className="taf-campaign-detail-response-flow">
              {snapshot.responseFlow.map((step) => (
                <div key={step.title} className={`taf-campaign-detail-response-step is-${step.status}`}>
                  <i><CheckCircleOutlined /></i>
                  <strong>{step.title}</strong>
                  <span>{step.time}</span>
                </div>
              ))}
            </div>
          </WorkPanel>

          <WorkPanel title="处置动作" className="taf-campaign-detail-actions-panel">
            <div className="taf-campaign-detail-action-list">
              {snapshot.responseActions.map((row) => (
                <div key={row.动作} className="taf-campaign-detail-action-row">
                  <strong>{row.动作}</strong>
                  <span>{row.目标}</span>
                  <em>{row.负责人}</em>
                  <StatusTag value={row.状态} />
                </div>
              ))}
            </div>
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
              <Link to="/graph">实体图谱</Link>
              <Link to="/forensics">证据包</Link>
              <Link to="/mlops">样本回流</Link>
            </div>
          </WorkPanel>
        </aside>
      </div>
    </div>
  );
}

function ProfileFact({ label, value, status = false }: { label: string; value: string; status?: boolean }) {
  return (
    <div className="taf-campaign-detail-profile-fact">
      <span>{label}</span>
      {status ? <StatusTag value={value} /> : <strong>{value}</strong>}
    </div>
  );
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
      <CampaignImpactRiskSummary total={total} unit={snapshot.impactAccount.unit} breakdown={breakdown} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键账号（Top 5）</h2>
        <AccountImpactTable rows={snapshot.impactAccount.rows} />
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
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键业务系统（Top 5）</h2>
        <BusinessSystemImpactTable rows={impact.rows} />
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
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键服务（Top 5）</h2>
        <ServiceImpactTable rows={impact.rows} />
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
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键校区（Top 5）</h2>
        <CampusImpactTable rows={impact.rows} />
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
      <CampaignImpactRiskSummary total={impact.total} unit={impact.unit} breakdown={impact.breakdown} />
      <div className="taf-campaign-impact-account-table-block">
        <h2>关键部门（Top 5）</h2>
        <DepartmentImpactTable rows={impact.rows} />
        <Link className="taf-campaign-impact-account-all-link" to="/assets?tab=department">查看全部部门 &gt;</Link>
      </div>
    </div>
  );
}

function CampaignImpactRiskSummary({
  total,
  unit,
  breakdown,
}: {
  total: number;
  unit: string;
  breakdown: CampaignDetailImpactRiskRow[];
}) {
  const highDeg = riskDegrees(breakdown[0], total);
  const mediumDeg = highDeg + riskDegrees(breakdown[1], total);
  const donutStyle = {
    '--taf-impact-high-deg': `${highDeg}deg`,
    '--taf-impact-medium-deg': `${mediumDeg}deg`,
  } as CSSProperties;
  return (
    <div className="taf-campaign-impact-account-summary">
      <div className="taf-campaign-impact-account-donut" style={donutStyle} aria-label={`${total} ${unit}`}>
        <div><strong>{total}</strong><span>{unit}</span></div>
      </div>
      <div className="taf-campaign-impact-account-risk-list">
        {breakdown.map((item) => <RiskBreakdownRow key={item.label} item={item} />)}
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

function AccountImpactTable({ rows }: { rows: CampaignDetailAccountRow[] }) {
  return (
    <div className="taf-campaign-impact-account-table" role="table" aria-label="关键账号 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">账号</span>
        <span role="columnheader">账号类型</span>
        <span role="columnheader">权限风险</span>
        <span role="columnheader">登录链路</span>
      </div>
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

function BusinessSystemImpactTable({ rows }: { rows: CampaignDetailBusinessSystemRow[] }) {
  return (
    <div className="taf-campaign-impact-account-table" role="table" aria-label="关键业务系统 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">业务系统</span>
        <span role="columnheader">关键服务</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">恢复优先级</span>
      </div>
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

function ServiceImpactTable({ rows }: { rows: CampaignDetailServiceRow[] }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-service-table" role="table" aria-label="关键服务 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">服务名称</span>
        <span role="columnheader">端口/协议</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">依赖关系</span>
      </div>
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

function CampusImpactTable({ rows }: { rows: CampaignDetailCampusRow[] }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-campus-table" role="table" aria-label="关键校区 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">校区/楼宇</span>
        <span role="columnheader">覆盖资产</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">链路</span>
      </div>
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

function DepartmentImpactTable({ rows }: { rows: CampaignDetailDepartmentRow[] }) {
  return (
    <div className="taf-campaign-impact-account-table taf-campaign-impact-department-table" role="table" aria-label="关键部门 Top 5">
      <div className="taf-campaign-impact-account-table__head" role="row">
        <span role="columnheader">部门名称</span>
        <span role="columnheader">责任人</span>
        <span role="columnheader">风险</span>
        <span role="columnheader">处置进度</span>
      </div>
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

function riskDegrees(item: CampaignDetailImpactRiskRow | undefined, total: number) {
  if (!item || total <= 0) return 0;
  return Math.max(0, Math.min(360, (item.count / total) * 360));
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
    evidenceCompleteness: 0,
    evidenceCompletenessAvailable: false,
    phaseDataBacked: false,
    evidenceChecks: [],
    evidenceSummaryRows: [],
    responseFlow: [],
    responseActions: [],
    reviewRows: [],
    evidence: [],
  };
}
