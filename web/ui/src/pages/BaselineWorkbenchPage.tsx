import {
  AlertOutlined,
  AreaChartOutlined,
  ClockCircleOutlined,
  ControlOutlined,
  ExperimentOutlined,
  EyeOutlined,
  HistoryOutlined,
  ProfileOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Empty, Progress, Select, Space, Tabs, Tooltip } from 'antd';
import { useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { OverlayContractHost, type OverlayContract } from '@/components/OverlayContractHost';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { baselineTabSlug, resolveBaselineTab } from '@/routes/pageRouteState';
import { fetchPageSnapshot } from '@/services/api';
import type { SnapshotRow } from '@/services/mockData';

const states = [
  ['学习中', '128', '3.8%', 'info'],
  ['稳定', '3,021', '90.6%', 'ok'],
  ['漂移观察', '42', '1.3%', 'warn'],
  ['待重建', '15', '0.4%', 'risk'],
  ['已冻结', '217', '6.5%', 'info'],
];

const deviationRows = [
  ['BD-20260625-041', '实验楼-SRV-12', '新的目的地偏离', 'v3.3', '3个', '0个', '24h', '待解释'],
  ['BD-20260625-037', '图书馆-NAS-03', '新端口扫描偏离', 'v3.2', '896个/小时', '120个/小时', '24h', '已解释'],
  ['BD-20260625-036', '教学区-PC-0421', '出站流量偏离', 'v3.3', '6.12GB/天', '1.38GB/天', '24h', '待解释'],
  ['BD-20260625-029', '汇聚交换机-07', '协议分布偏离', 'v3.1', 'TCP 92%', 'TCP 68%', '24h', '已解释'],
];

const baselineOverlays: OverlayContract[] = [
  {
    id: 'modal-baseline-threshold',
    title: '基线阈值编辑',
    kind: 'Modal',
    actionLabel: '阈值编辑',
    description: '调整行为基线阈值、漂移观察窗口、冻结策略和版本回滚条件。',
    impact: '影响偏离识别、异常告警和模型反馈样本。',
    audit: '记录阈值变更、版本、审批人和生效范围。',
  },
];

export function BaselineWorkbenchPage({ route }: { route: NavRoute }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const sourceAssetId = searchParams.get('assetId') ?? '';
  const activeTab = resolveBaselineTab(searchParams.get('tab'), route.page.tabs);
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selectedRow = useMemo(() => rows[0], [rows]);
  const isEmpty = !isLoading && !isError && rows.length === 0;

  return (
    <div className="taf-page taf-baseline-workbench">
      <header className="taf-baseline-titlebar">
        <div>
          <h1>{route.page.title}</h1>
        </div>
        <Space>
          <Button size="small" icon={<ControlOutlined />}>管理基线</Button>
          <Tooltip title="刷新基线">
            <Button size="small" icon={<ReloadOutlined />} onClick={() => void refetch()} />
          </Tooltip>
          <OverlayContractHost overlays={baselineOverlays} compact />
        </Space>
      </header>

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 /v1/baselines、APISIX 路由、ClickHouse sessions 或后端服务。'}
          action={
            <Button size="small" danger onClick={() => void refetch()}>
              重试
            </Button>
          }
        />
      )}

      {isLoading && (
        <Alert
          type="info"
          showIcon
          message="基线数据加载中"
          description="正在读取 /v1/baselines、行为分布与偏离解释数据。"
        />
      )}

      {sourceAssetId && <Alert showIcon type="info" message="已限定资产台账上下文" description={`当前基线范围资产 ID：${sourceAssetId}`} />}

      <Tabs
        className="taf-baseline-tabs"
        activeKey={activeTab}
        onChange={(tab) => setSearchParams((current) => {
          const next = new URLSearchParams(current);
          next.set('tab', baselineTabSlug(tab));
          return next;
        })}
        items={route.page.tabs.map((tab) => ({ key: tab, label: tab }))}
      />

      <div className="taf-baseline-filter">
        <label>
          <span>资产组</span>
          <Select size="small" value={sourceAssetId || '全部资产组'} options={[{ value: sourceAssetId || '全部资产组', label: sourceAssetId || '全部资产组' }]} />
        </label>
        <label>
          <span>历史窗口</span>
          <Select size="small" value="近 30 天" options={[{ value: '近 7 天' }, { value: '近 30 天' }, { value: '近 90 天' }]} />
        </label>
        <label>
          <span>学习状态</span>
          <Select size="small" value="全部" options={[{ value: '全部' }, { value: '学习中' }, { value: '稳定' }, { value: '冻结' }]} />
        </label>
        <label>
          <span>漂移状态</span>
          <Select size="small" value="全部" options={[{ value: '全部' }, { value: '待解释' }, { value: '已解释' }]} />
        </label>
        <label>
          <span>版本</span>
          <Select size="small" value="全部版本" options={[{ value: '全部版本' }, { value: 'v3.3' }, { value: 'v3.2' }]} />
        </label>
      </div>

      <div className="taf-baseline-kpis">
        {(data?.metrics ?? []).map((metric) => (
          <MetricTile key={metric.label} metric={metric} />
        ))}
      </div>

      <div className="taf-baseline-grid">
        <main className="taf-baseline-main">
          <div className="taf-baseline-upper">
            <WorkPanel title="基线状态机">
              <BaselineStateMachine />
            </WorkPanel>
            <WorkPanel title={`行为分布分析（${text(selectedRow, '对象', '实验楼-SRV-12')}）`}>
              {isEmpty ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无基线对象数据" /> : <DistributionPanel />}
            </WorkPanel>
          </div>

          <div className="taf-baseline-lower">
            <WorkPanel title="偏离列表（共 42 条）">
              <DeviationList />
            </WorkPanel>
            <WorkPanel title="基线版本管理">
              <VersionGovernance />
            </WorkPanel>
          </div>
        </main>

        <aside className="taf-baseline-detail">
          <DeviationExplanation selectedRow={selectedRow} />
          <GovernanceActions />
        </aside>
      </div>
    </div>
  );
}

function BaselineStateMachine() {
  return (
    <div className="taf-baseline-state-machine">
      {states.map(([label, count, ratio, tone], index) => (
        <div key={label} className={`is-${tone}`}>
          <i>{index < states.length - 1 && <span />}</i>
          <strong>{label}</strong>
          <em>对象 {count}</em>
          <small>占比 {ratio}</small>
        </div>
      ))}
    </div>
  );
}

function DistributionPanel() {
  return (
    <div className="taf-baseline-distribution">
      <div className="taf-baseline-boxplots">
        {['会话长度（秒）', '目的端口数（个/小时）', '时间段 vs 流量偏离'].map((title, index) => (
          <div key={title}>
            <span>{title}</span>
            <i style={{ height: `${48 + index * 12}px` }} />
            <b />
            <em />
          </div>
        ))}
      </div>
      <div className="taf-baseline-scatter">
        {Array.from({ length: 84 }, (_, index) => (
          <i key={index} className={index % 17 === 0 ? 'is-risk' : index % 11 === 0 ? 'is-warn' : 'is-ok'} style={{ left: `${(index * 13) % 96}%`, bottom: `${10 + ((index * 29) % 78)}%` }} />
        ))}
      </div>
      <TrendBands />
    </div>
  );
}

function TrendBands() {
  const bands = ['均值', 'P50', 'P95', 'P99', '阈值上限', '阈值下限'];
  return (
    <div className="taf-baseline-trend">
      {bands.map((band, index) => (
        <span key={band} className={`band-${index}`}>
          {band}
        </span>
      ))}
      {Array.from({ length: 36 }, (_, index) => (
        <i key={index} style={{ height: `${18 + ((index * 7) % 32)}px` }} />
      ))}
    </div>
  );
}

function DeviationList() {
  return (
    <div className="taf-baseline-deviation-list">
      <div className="taf-baseline-deviation-head">
        <span>偏离ID</span>
        <span>对象</span>
        <span>偏离类型</span>
        <span>基线版本</span>
        <span>状态</span>
      </div>
      {deviationRows.map(([id, object, type, version, current, baseline, sample, status]) => (
        <button key={id} type="button">
          <strong>{id}</strong>
          <span>{object}</span>
          <span>{type}</span>
          <span>{version}</span>
          <StatusTag value={status} />
          <small>{current} / {baseline} / {sample}</small>
        </button>
      ))}
    </div>
  );
}

function VersionGovernance() {
  const versions = [
    ['v3.3', '稳定（当前）', '2026-05-11 ~ 2026-06-10', '1,248,932', '126', 'P95 * 1.5'],
    ['v3.2', '已冻结', '2026-04-11 ~ 2026-05-10', '1,102,548', '124', 'P95 * 1.6'],
    ['v3.1', '已冻结', '2026-03-11 ~ 2026-04-10', '987,321', '118', 'P95 * 1.6'],
  ];
  return (
    <div className="taf-baseline-versions">
      <div className="taf-baseline-version-line">
        {versions.map(([version, status]) => (
          <span key={version}>
            <b>{version}</b>
            <em>{status}</em>
          </span>
        ))}
      </div>
      {versions.map(([version, status, window, samples, features, threshold]) => (
        <div key={version} className="taf-baseline-version-row">
          <strong>{version}</strong>
          <StatusTag value={status} />
          <span>{window}</span>
          <span>{samples}</span>
          <span>{features}</span>
          <span>{threshold}</span>
        </div>
      ))}
    </div>
  );
}

function DeviationExplanation({ selectedRow }: { selectedRow?: SnapshotRow }) {
  const score = Number(text(selectedRow, '偏离值', '7.5').replace(/[^\d.]/g, '')) || 7.5;
  return (
    <WorkPanel title="偏离解释" extra={<Button size="small" type="text">关闭</Button>}>
      <div className="taf-baseline-explain-title">
        <strong>{text(selectedRow, '解释', '新的目的地偏离')}</strong>
        <StatusTag value={score >= 6 ? '高风险' : '中风险'} />
      </div>
      <p>目的 IP 203.0.113.45 为基线外的新增目的地，近 30 天出现 7 次，累计出站 2.83 GB。</p>
      <dl className="taf-baseline-explain-facts">
        <dt>基线窗口</dt>
        <dd>近 30 天</dd>
        <dt>观察值</dt>
        <dd>2026-06-25</dd>
        <dt>偏离强度</dt>
        <dd>{score.toFixed(1)}x（相对 P95）</dd>
        <dt>证据来源</dt>
        <dd>Flow / DNS / TLS / PCAP</dd>
        <dt>关联资产</dt>
        <dd>{text(selectedRow, '对象', '实验楼-SRV-12')}</dd>
        <dt>关联告警</dt>
        <dd>AL-20260625-000123</dd>
      </dl>
      <div className="taf-baseline-score">
        <Progress type="circle" size={72} percent={Math.min(99, Math.round(score * 10))} format={() => score.toFixed(1)} strokeColor="#ff4d4f" />
        <span>置信度 0.86</span>
        <span>阈值：P95 * 1.5</span>
      </div>
      <div className="taf-baseline-explain-actions">
        <Button danger type="primary" icon={<AlertOutlined />}>创建告警</Button>
        <Button icon={<ControlOutlined />}>调整阈值</Button>
        <Button icon={<SafetyCertificateOutlined />}>冻结基线</Button>
        <Button icon={<EyeOutlined />}>跳转取证</Button>
        <Button icon={<ExperimentOutlined />}>反馈模型</Button>
      </div>
    </WorkPanel>
  );
}

function GovernanceActions() {
  const actions = [
    ['冷启动', <ThunderboltOutlined key="cold" />],
    ['漂移', <AreaChartOutlined key="drift" />],
    ['重建', <ReloadOutlined key="rebuild" />],
    ['冻结', <SafetyCertificateOutlined key="freeze" />],
    ['版本回滚', <HistoryOutlined key="rollback" />],
    ['审计留痕', <ProfileOutlined key="audit" />],
  ];
  return (
    <WorkPanel title="治理与操作">
      <div className="taf-baseline-governance-actions">
        {actions.map(([label, icon]) => (
          <Button key={String(label)} icon={icon}>{label}</Button>
        ))}
      </div>
      <div className="taf-baseline-action-log">
        {['2026-06-25 11:22 冻结基线 实验楼-SRV-12 v3.3', '2026-06-25 10:14 漂移观察 图书馆-NAS-03 v3.2', '2026-06-24 18:09 创建基线 教学区-PC-0421 v3.1'].map((item) => (
          <span key={item}><ClockCircleOutlined />{item}</span>
        ))}
      </div>
    </WorkPanel>
  );
}

const text = (row: SnapshotRow | undefined, key: string, fallback: string) => {
  const value = row?.[key];
  return value === undefined || value === null || value === '' ? fallback : String(value);
};
