import { Alert, Button, Empty } from 'antd';
import {
  AlertOutlined,
  AuditOutlined,
  CalendarOutlined,
  ClockCircleOutlined,
  DatabaseOutlined,
  FileSearchOutlined,
  HistoryOutlined,
  SafetyCertificateOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import type { ReactNode } from 'react';
import {
  BaselineBoxplotChart,
  BaselineCalendarChart,
  BaselineDeviationScatterChart,
  BaselineDonutChart,
  BaselineHeatmapChart,
  BaselineIntervalTrendChart,
  BaselineMultiSeriesChart,
  BaselineNetworkChart,
  BaselinePeriodicityChart,
  BaselinePortScanChart,
  type BaselineBoxplotDatum,
  type BaselineScatterDatum,
  type BaselineTrendDatum,
} from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { baselineAccountNetwork, baselineOverviewHeatmap, baselineOverviewSeries } from '@/pages/baselineOverviewAdapters';
import type { BaselineTabType, BehaviorBaseline, BehaviorBaselineAnalytics, BehaviorBaselineOverview, BehaviorMetric } from '@/services/baselineApi';

type BaselineChartData = {
  boxplots: BaselineBoxplotDatum[];
  scatter: BaselineScatterDatum[];
  trend: BaselineTrendDatum;
};

type BaselineTypeWorkspaceProps = {
  type: BaselineTabType;
  baselines: BehaviorBaseline[];
  selected?: BehaviorBaseline;
  analytics?: BehaviorBaselineAnalytics;
  overview?: BehaviorBaselineOverview;
  chartData: BaselineChartData;
  stateMachine: ReactNode;
  versionGovernance: ReactNode;
  windowLabel: string;
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  onSelect: (baselineId: string) => void;
  onEvidenceNavigate: (target: BaselineEvidenceTarget) => void;
};

export type BaselineEvidenceTarget = 'alerts' | 'pcap' | 'sessions' | 'audit' | 'versions';

export function BaselineTypeWorkspace({
  type,
  baselines,
  selected,
  analytics,
  overview,
  chartData,
  stateMachine,
  versionGovernance,
  windowLabel,
  loading,
  error,
  onRetry,
  onSelect,
  onEvidenceNavigate,
}: BaselineTypeWorkspaceProps) {
  if (type === 'asset') {
    return <>
      <div className="taf-baseline-upper taf-baseline-upper--asset" data-baseline-board="asset">
        <WorkPanel title="资产基线状态机">{stateMachine}</WorkPanel>
        <WorkPanel title={`行为分布分析${selected ? `（${selected.entity_id}）` : ''}`} extra={<span className="taf-baseline-data-note">{windowLabel}</span>}>
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-baseline-chart-grid"><div className="taf-baseline-chart-box"><BaselineBoxplotChart data={chartData.boxplots} /></div><div className="taf-baseline-chart-scatter"><BaselineDeviationScatterChart data={chartData.scatter} /></div><div className="taf-baseline-chart-trend"><BaselineIntervalTrendChart data={chartData.trend} /></div></div>
          </BaselineChartState>
        </WorkPanel>
      </div>
      <div className="taf-baseline-lower taf-baseline-lower--asset">
        <WorkPanel title={`偏离列表（共 ${deviationCount(baselines)} 条）`}><BaselineDenseTable type="asset" baselines={baselines} selected={selected} onSelect={onSelect} /></WorkPanel>
        <WorkPanel title="基线版本管理">{versionGovernance}</WorkPanel>
      </div>
    </>;
  }
  if (type === 'account') {
    return <>
      <div className="taf-baseline-upper taf-baseline-upper--account" data-baseline-board="account">
        <WorkPanel title="账号基线状态机">{stateMachine}</WorkPanel>
        <WorkPanel title="账号登录时间轴图（按账号组 · 小时分布 / 会话时长）" extra={<span className="taf-baseline-data-note">{windowLabel}</span>}>
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-account-login-shell">
              <SpecialistChartToolbar
                items={[['登录小时', 'active'], ['会话时长（分钟）', 'muted']]}
                signals={[`夜间异常 ${overviewCount(overview, 'denied_events')}`, `权限漂移 ${overviewCount(overview, 'permission_changes')}`]}
              />
              <div className="taf-specialist-chart-stage"><BaselineBoxplotChart data={buildAccountLoginBoxplots(overview, analytics)} ariaLabel="账号登录小时箱线分布图" /></div>
            </div>
          </BaselineChartState>
        </WorkPanel>
        <WorkPanel title="访问资产基线（账号 → 资产 · 边粗代表频率）">
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-account-network-shell">
              <SpecialistChartToolbar items={[['高危', 'risk'], ['中危', 'warning'], ['低危', 'ok']]} />
              <div className="taf-specialist-chart-stage"><BaselineNetworkChart data={baselineAccountNetwork(overview, selected?.entity_id ?? '')} /></div>
            </div>
          </BaselineChartState>
        </WorkPanel>
      </div>
      <div className="taf-baseline-lower taf-baseline-lower--account">
        <WorkPanel title="异常账号列表（Top 5）"><BaselineDenseTable type="account" baselines={baselines} selected={selected} onSelect={onSelect} /></WorkPanel>
        <WorkPanel title="异常地理位置"><BaselineInsightPanel type="account" overview={overview} selected={selected} /></WorkPanel>
        <WorkPanel title="权限漂移矩阵"><BaselinePortraitPanel type="account" baselines={baselines} selected={selected} overview={overview} /></WorkPanel>
      </div>
      <div className="taf-baseline-tertiary taf-baseline-tertiary--account" data-baseline-tertiary="account">
        <WorkPanel title="账号行为分表（证据摘要）"><BaselineWideDetails type="account" baselines={baselines} /></WorkPanel>
        <WorkPanel title={`告警入口与证据（24h 新增 ${deviationCount(baselines)} 条）`}><BaselineEvidenceList type="account" baselines={baselines} /></WorkPanel>
        <WorkPanel title="基线版本与治理">{versionGovernance}</WorkPanel>
      </div>
      <BaselineEvidenceShortcutBar type="account" overview={overview} onNavigate={onEvidenceNavigate} />
    </>;
  }
  if (type === 'port') {
    return <>
      <div className="taf-baseline-upper taf-baseline-upper--port" data-baseline-board="port">
        <WorkPanel title="端口基线状态机">{stateMachine}</WorkPanel>
        <WorkPanel title="端口热力图（近 30 天访问频度）">
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-port-heatmap-shell">
              <div className="taf-specialist-chart-stage"><BaselineHeatmapChart data={baselineOverviewHeatmap(overview?.heatmap)} ariaLabel="资产端口基线热力图" scale="quantile" /></div>
              <SpecialistChartToolbar items={[['正常', 'ok'], ['新端口', 'warning'], ['高危', 'risk'], ['扫描', 'purple'], ['关闭', 'muted']]} compact />
            </div>
          </BaselineChartState>
        </WorkPanel>
        <WorkPanel title="服务变化趋势（近 30 天）" extra={<span className="taf-baseline-data-note">{windowLabel}</span>}>
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-port-trend-shell">
              <SpecialistChartToolbar items={[['新增端口', 'info'], ['关闭端口', 'muted'], ['服务版本变化', 'ok'], ['外网暴露端口', 'risk'], ['扫描命中次数', 'purple']]} />
              <div className="taf-specialist-event-flags"><span>维护窗口</span><span>服务发布</span><span className="is-risk">异常日峰</span></div>
              <div className="taf-specialist-chart-stage"><BaselineMultiSeriesChart data={baselineOverviewSeries(overview, portService)} ariaLabel="服务变化趋势图" /></div>
            </div>
          </BaselineChartState>
        </WorkPanel>
      </div>
      <div className="taf-baseline-lower taf-baseline-lower--port"><WorkPanel title="端口偏离表（Top 5）"><BaselineDenseTable type="port" baselines={baselines} selected={selected} onSelect={onSelect} /></WorkPanel><WorkPanel title="端口扫描特征"><BaselineInsightPanel type="port" overview={overview} selected={selected} /></WorkPanel><WorkPanel title="常用端口画像"><BaselinePortraitPanel type="port" baselines={baselines} selected={selected} overview={overview} /></WorkPanel></div>
      <div className="taf-baseline-tertiary taf-baseline-tertiary--port" data-baseline-tertiary="port"><WorkPanel title="服务变更明细（近 30 天）"><BaselineWideDetails type="port" baselines={baselines} /></WorkPanel><WorkPanel title="告警入口与证据（近 24 小时）"><BaselineEvidenceList type="port" baselines={baselines} /></WorkPanel></div>
      <BaselineEvidenceShortcutBar type="port" overview={overview} onNavigate={onEvidenceNavigate} />
    </>;
  }
  if (type === 'protocol') {
    return <>
      <div className="taf-baseline-upper taf-baseline-upper--protocol" data-baseline-board="protocol">
        <WorkPanel title="协议基线状态机">{stateMachine}</WorkPanel>
        <WorkPanel title="协议分布环图">
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-protocol-donut-shell">
              <div className="taf-specialist-chart-stage"><BaselineDonutChart data={buildProtocolDonut(overview)} centerLabel="主协议占比" /></div>
              <SpecialistChartToolbar items={(overview?.shares ?? []).slice(0, 6).map((item, index) => [protocolName(item.key), index === 0 ? 'info' : index === 1 ? 'ok' : index === 2 ? 'warning' : 'purple'])} compact />
            </div>
          </BaselineChartState>
        </WorkPanel>
        <WorkPanel title="协议占比漂移趋势（近 30 天）" extra={<span className="taf-baseline-data-note">{windowLabel}</span>}>
          <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
            <div className="taf-specialist-chart-shell taf-protocol-trend-shell">
              <SpecialistChartToolbar items={(overview?.shares ?? []).slice(0, 6).map((item, index) => [protocolName(item.key), index === 0 ? 'info' : index === 1 ? 'ok' : index === 2 ? 'warning' : 'purple'])} />
              <div className="taf-specialist-chart-stage"><BaselineMultiSeriesChart data={baselineOverviewSeries(overview, protocolName, true)} ariaLabel="协议占比漂移趋势图" /></div>
            </div>
          </BaselineChartState>
        </WorkPanel>
      </div>
      <div className="taf-baseline-lower taf-baseline-lower--protocol"><WorkPanel title={`协议偏离列表（共 ${deviationCount(baselines)} 条）`}><BaselineDenseTable type="protocol" baselines={baselines} selected={selected} onSelect={onSelect} /></WorkPanel><WorkPanel title="新协议发现（近 30 天）"><BaselineInsightPanel type="protocol" overview={overview} selected={selected} /></WorkPanel><WorkPanel title={`异常协议画像（共 ${Math.min(9, baselines.length)} 个）`}><BaselinePortraitPanel type="protocol" baselines={baselines} selected={selected} overview={overview} /></WorkPanel></div>
      <div className="taf-baseline-tertiary taf-baseline-tertiary--protocol" data-baseline-tertiary="protocol"><WorkPanel title={`协议基线明细（共 ${baselines.length} 条）`}><BaselineWideDetails type="protocol" baselines={baselines} /></WorkPanel><WorkPanel title="告警入口与证据（近 30 天）"><BaselineEvidenceList type="protocol" baselines={baselines} /></WorkPanel><WorkPanel title="基线治理与版本">{versionGovernance}</WorkPanel></div>
      <BaselineEvidenceShortcutBar type="protocol" overview={overview} onNavigate={onEvidenceNavigate} />
    </>;
  }
  return <>
    <div className="taf-baseline-upper taf-baseline-upper--time" data-baseline-board="time">
      <WorkPanel title="时间段基线状态机">{stateMachine}</WorkPanel>
      <WorkPanel title="时间热力图">
        <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
          <div className="taf-specialist-chart-shell taf-time-heatmap-shell">
            <div className="taf-time-window-markers"><span>夜间窗口 00:00–06:00</span><span>P95 上限</span><span>维护窗口 02:00–04:00</span></div>
            <div className="taf-specialist-chart-stage"><BaselineHeatmapChart data={baselineOverviewHeatmap(overview?.heatmap)} ariaLabel="时间段基线热力图" normalizeByRow /></div>
            <SpecialistChartToolbar items={[['工作日正常', 'info'], ['夜间异常', 'risk'], ['周末偏高', 'warning'], ['维护窗口', 'ok'], ['周期连接', 'purple']]} compact />
          </div>
        </BaselineChartState>
      </WorkPanel>
      <WorkPanel title={`日历视图 ${overview?.window_days ? `· 近 ${overview.window_days} 天` : ''}`}>
        <BaselineChartState loading={loading} error={error} selected={selected} onRetry={onRetry}>
          <div className="taf-specialist-chart-shell taf-time-calendar-shell">
            <div className="taf-specialist-chart-stage"><BaselineCalendarChart data={baselineOverviewHeatmap(overview?.calendar)} ariaLabel="时间段基线日历视图" /></div>
            <SpecialistChartToolbar items={[['工作日', 'info'], ['周末', 'warning'], ['节假日', 'purple'], ['维护', 'ok'], ['异常', 'risk']]} compact />
          </div>
        </BaselineChartState>
      </WorkPanel>
    </div>
    <div className="taf-baseline-lower taf-baseline-lower--time"><WorkPanel title={`异常时段列表（共 ${deviationCount(baselines)} 条）`}><BaselineDenseTable type="time" baselines={baselines} selected={selected} onSelect={onSelect} /></WorkPanel><WorkPanel title="周期性连接"><BaselineInsightPanel type="time" overview={overview} selected={selected} /></WorkPanel><WorkPanel title="工作日 / 夜间 / 周末画像"><BaselinePortraitPanel type="time" baselines={baselines} selected={selected} overview={overview} /></WorkPanel></div>
    <div className="taf-baseline-tertiary taf-baseline-tertiary--time" data-baseline-tertiary="time"><WorkPanel title={`时间段基线明细（共 ${baselines.length} 条）`}><BaselineWideDetails type="time" baselines={baselines} /></WorkPanel><WorkPanel title="告警入口与证据（近 30 天）"><BaselineEvidenceList type="time" baselines={baselines} /></WorkPanel><WorkPanel title="基线治理与版本">{versionGovernance}</WorkPanel></div>
    <BaselineEvidenceShortcutBar type="time" overview={overview} onNavigate={onEvidenceNavigate} />
  </>;
}

function BaselineEvidenceShortcutBar({ type, overview, onNavigate }: { type: Exclude<BaselineTabType, 'asset'>; overview?: BehaviorBaselineOverview; onNavigate: (target: BaselineEvidenceTarget) => void }) {
  const observed = (key: string) => Math.round(overview?.kpis.find((item) => item.key === key)?.value ?? 0);
  const rows: Record<Exclude<BaselineTabType, 'asset'>, Array<[BaselineEvidenceTarget, ReactNode, string, string]>> = {
    account: [
      ['alerts', <AlertOutlined key="alerts" />, '关联告警', `${Math.max(1, observed('denied_events')).toLocaleString('zh-CN')} 条`],
      ['audit', <AuditOutlined key="login" />, '登录日志', `${Math.max(1, observed('events')).toLocaleString('zh-CN')} 条`],
      ['sessions', <DatabaseOutlined key="sessions" />, 'Session记录', `${Math.max(1, observed('events')).toLocaleString('zh-CN')} 条`],
      ['pcap', <FileSearchOutlined key="pcap" />, 'PCAP证据', `${Math.max(1, observed('source_addresses')).toLocaleString('zh-CN')} 个来源`],
      ['audit', <SafetyCertificateOutlined key="permission" />, '权限审计', `${Math.max(1, observed('permission_changes')).toLocaleString('zh-CN')} 条`],
      ['versions', <HistoryOutlined key="versions" />, '基线版本', '查看治理版本'],
    ],
    port: [
      ['alerts', <AlertOutlined key="alerts" />, '关联告警', `${Math.max(1, observed('failed_syn')).toLocaleString('zh-CN')} 条`],
      ['pcap', <FileSearchOutlined key="pcap" />, 'PCAP证据', `${Math.max(1, observed('external_destinations')).toLocaleString('zh-CN')} 个目标`],
      ['sessions', <DatabaseOutlined key="sessions" />, 'Session记录', `${Math.max(1, observed('sessions')).toLocaleString('zh-CN')} 条`],
      ['audit', <AuditOutlined key="port" />, '端口告警', `${Math.max(1, observed('ports')).toLocaleString('zh-CN')} 个端口`],
      ['versions', <HistoryOutlined key="versions" />, '基线版本', '查看治理版本'],
    ],
    protocol: [
      ['alerts', <AlertOutlined key="alerts" />, '关联告警', `${Math.max(1, observed('unknown_protocols')).toLocaleString('zh-CN')} 条`],
      ['pcap', <FileSearchOutlined key="pcap" />, 'PCAP证据', `${Math.max(1, observed('protocols')).toLocaleString('zh-CN')} 个协议`],
      ['sessions', <DatabaseOutlined key="sessions" />, 'Session记录', `${Math.max(1, observed('sessions')).toLocaleString('zh-CN')} 条`],
      ['audit', <AuditOutlined key="dns" />, 'DNS日志', '查看协议日志'],
      ['alerts', <SafetyCertificateOutlined key="protocol" />, '协议告警', `${Math.max(1, observed('unknown_protocols')).toLocaleString('zh-CN')} 条`],
      ['versions', <HistoryOutlined key="versions" />, '协议版本', '查看治理版本'],
    ],
    time: [
      ['alerts', <AlertOutlined key="alerts" />, '关联告警', `${Math.max(1, observed('night_sessions')).toLocaleString('zh-CN')} 条`],
      ['pcap', <FileSearchOutlined key="pcap" />, 'PCAP证据', `${Math.max(1, observed('active_hours')).toLocaleString('zh-CN')} 个时段`],
      ['sessions', <DatabaseOutlined key="sessions" />, 'Session记录', `${Math.max(1, observed('sessions')).toLocaleString('zh-CN')} 条`],
      ['audit', <AuditOutlined key="dns" />, 'DNS日志', '查看时间日志'],
      ['audit', <SafetyCertificateOutlined key="time" />, '时间基线', `${Math.max(1, observed('active_hours')).toLocaleString('zh-CN')} 个`],
      ['versions', <HistoryOutlined key="versions" />, '基线版本', '查看治理版本'],
    ],
  };
  return <nav className={`taf-baseline-evidence-shortcuts taf-baseline-evidence-shortcuts--${type}`} aria-label={`${type}基线证据与入口快捷栏`}>
    <strong>证据与入口快捷栏</strong>
    {rows[type].map(([target, icon, label, value], index) => <button type="button" key={`${target}-${label}-${index}`} onClick={() => onNavigate(target)}><i>{icon}</i><span>{label}<small>{value}</small></span><b>›</b></button>)}
  </nav>;
}

function SpecialistChartToolbar({
  items,
  signals = [],
  compact = false,
}: {
  items: ReadonlyArray<ReadonlyArray<string>>;
  signals?: string[];
  compact?: boolean;
}) {
  return <div className={`taf-specialist-chart-toolbar${compact ? ' is-compact' : ''}`}>
    <div>{items.map(([label, tone], index) => <span className={`is-${tone || 'info'}`} key={`${label}-${index}`}><i />{label}</span>)}</div>
    {signals.length > 0 && <aside>{signals.map((signal) => <em key={signal}>{signal}</em>)}</aside>}
  </div>;
}

function overviewCount(overview: BehaviorBaselineOverview | undefined, key: string) {
  return Math.round(overview?.kpis.find((item) => item.key === key)?.value ?? 0).toLocaleString('zh-CN');
}

function BaselineChartState({ loading, error, selected, onRetry, children }: { loading: boolean; error: boolean; selected?: BehaviorBaseline; onRetry: () => void; children: ReactNode }) {
  if (loading) return <div className="taf-baseline-loading">正在读取真实分布与时间桶…</div>;
  if (error) return <Alert showIcon type="error" message="真实行为分布加载失败" action={<Button size="small" onClick={onRetry}>重试</Button>} />;
  if (!selected) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前筛选范围没有真实基线数据" />;
  return children;
}

function BaselineWideDetails({ type, baselines }: { type: BaselineTabType; baselines: BehaviorBaseline[] }) {
  const rows = baselines.slice(0, 7);
  return <div className="taf-baseline-wide-details">
    <div><span>{type === 'account' ? '账号' : type === 'port' ? '端口 / 服务' : type === 'protocol' ? '协议' : '时间段'}</span><span>基线值</span><span>当前值</span><span>偏离</span><span>版本</span><span>状态</span></div>
    {rows.map((item) => {
      const metric = strongestMetric(item);
      return <button type="button" key={item.baseline_id}>
        <strong>{type === 'port' ? `${item.entity_id} / ${portService(item.entity_id)}` : type === 'protocol' ? protocolName(item.entity_id) : item.entity_id}</strong>
        <span>{formatMetric(metric?.mean, metric?.unit)}</span>
        <span>{formatMetric(metric?.current_value, metric?.unit)}</span>
        <em className={scoreOf(item) > 2 ? 'is-risk' : ''}>{scoreOf(item).toFixed(2)}σ</em>
        <span>v{item.version}</span>
        <StatusTag value={statusLabel(item)} />
      </button>;
    })}
  </div>;
}

function BaselineEvidenceList({ type, baselines }: { type: BaselineTabType; baselines: BehaviorBaseline[] }) {
  return <div className="taf-baseline-evidence-list">{baselines.slice(0, 6).map((item, index) => {
    const metric = strongestMetric(item);
    const evidence = type === 'account' ? `${Math.round(metricValue(item, 'source_ip_count'))} 个来源 · ${Math.round(metricValue(item, 'resource_count'))} 个资源`
      : type === 'port' ? `${portService(item.entity_id)} · 当前 ${formatMetric(metric?.current_value, metric?.unit)}`
        : type === 'protocol' ? `${protocolName(item.entity_id)} · ${formatMetric(metric?.current_value, metric?.unit)}`
          : `${String(item.entity_id).padStart(2, '0')}:00–${String((Number(item.entity_id) + 1) % 24).padStart(2, '0')}:00 · ${formatMetric(metric?.current_value, metric?.unit)}`;
    return <article key={item.baseline_id}><i className={scoreOf(item) > 2 ? 'is-risk' : ''}>{index + 1}</i><div><strong>{item.entity_id}</strong><span>{evidence}</span></div><StatusTag value={statusLabel(item)} /></article>;
  })}</div>;
}

function BaselineDenseTable({ type, baselines, selected, onSelect }: { type: BaselineTabType; baselines: BehaviorBaseline[]; selected?: BehaviorBaseline; onSelect: (baselineId: string) => void }) {
  if (!baselines.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无真实偏离数据" />;
  const rows = baselines.slice().sort((left, right) => scoreOf(right) - scoreOf(left)).slice(0, 5);
  const columns = tableColumns(type);
  return <div className="taf-baseline-dense-table" data-baseline-table={type}>
    <div className="taf-baseline-dense-head">{columns.map((column) => <span key={column}>{column}</span>)}</div>
    {rows.map((item) => {
      const metric = strongestMetric(item);
      const score = scoreOf(item);
      return <button type="button" key={item.baseline_id} className={selected?.baseline_id === item.baseline_id ? 'is-selected' : ''} onClick={() => onSelect(item.baseline_id)}>
        <strong title={item.entity_id}>{type === 'protocol' ? protocolName(item.entity_id) : item.entity_id}</strong>
        <span>{typeSpecificValue(type, item, metric, 0)}</span>
        <span>{typeSpecificValue(type, item, metric, 1)}</span>
        <span>{formatMetric(metric?.current_value, metric?.unit)}</span>
        <span className={score >= (metric?.threshold_config.alert_multiplier ?? 3) ? 'is-danger' : score > 0 ? 'is-warning' : ''}>{score.toFixed(2)}σ</span>
        <StatusTag value={statusLabel(item)} />
      </button>;
    })}
  </div>;
}

function BaselineInsightPanel({ type, overview, selected }: { type: BaselineTabType; overview?: BehaviorBaselineOverview; selected?: BehaviorBaseline }) {
  if (type === 'account') {
    const sources = overview?.facts.filter((item) => item.kind === 'source_ip').slice(0, 5) ?? [];
    const accountId = selected?.entity_id ?? sources[0]?.entity_id ?? '当前账号';
    const sourceNodes = sources.map((item, index) => ({
      id: `source-${index}`,
      name: item.related_id || item.entity_id,
      value: Math.max(1, item.count),
      category: item.denied ? 1 : 2,
    }));
    return <div className="taf-baseline-account-geo">
      <div className="taf-baseline-account-geo__map" aria-label="账号来源地址分布背景图">
        {sourceNodes.length ? <BaselineNetworkChart data={{
          nodes: [{ id: 'account', name: accountId, value: Math.max(1, sourceNodes.reduce((sum, item) => sum + item.value, 0)), category: 0 }, ...sourceNodes],
          links: sourceNodes.map((item) => ({ source: item.id, target: 'account', value: item.value })),
        }} ariaLabel="账号来源地址真实关系图" /> : <span>来源地址关系数据未接入</span>}
      </div>
      <div className="taf-baseline-account-geo__facts">
        {sources.slice(0, 4).map((item) => <InsightRow key={`${item.entity_id}-${item.related_id}`} label={item.entity_id} value={`${item.related_id} · ${item.count.toLocaleString('zh-CN')} 次${item.denied ? ` · 拒绝 ${item.denied}` : ''}`} tone={item.denied ? 'warning' : 'ok'} />)}
      </div>
      <small>{overview?.availability.geolocation ?? '当前数据源未提供经验证的地理维度；关系图仅呈现来源地址证据，不推断地理位置。'}</small>
    </div>;
  }
  if (type === 'port') {
    const syn = overview?.facts.find((item) => item.kind === 'scan_signal')?.count ?? 0;
    const exposure = overview?.facts.find((item) => item.kind === 'exposure')?.count ?? 0;
    const sourceFacts = overview?.facts.filter((item) => item.kind === 'scan_signal' || item.kind === 'source_ip').slice(0, 5) ?? [];
    const targetFacts = overview?.facts.filter((item) => item.kind === 'exposure' || item.kind === 'port').slice(0, 8) ?? [];
    const timestamps = [...new Set((overview?.series ?? []).map((item) => item.timestamp))].sort((left, right) => left - right);
    const trendValues = timestamps.map((timestamp) => (overview?.series ?? []).filter((item) => item.timestamp === timestamp).reduce((sum, item) => sum + item.value, 0));
    const distribution = (overview?.shares ?? []).slice(0, 6).map((item) => ({ name: portService(item.key), value: item.sessions }));
    return <div className="taf-baseline-scan-summary taf-baseline-scan-summary--chart">
      <BaselinePortScanChart data={{
        sources: sourceFacts.map((item) => ({ name: item.entity_id, value: item.count })),
        targets: targetFacts.map((item) => ({ name: item.related_id || item.entity_id, value: item.count })),
        distribution: distribution.length ? distribution : targetFacts.slice(0, 6).map((item) => ({ name: item.related_id || item.entity_id, value: item.count })),
        trendLabels: timestamps.map((timestamp) => new Date(timestamp).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })),
        trendValues,
      }} />
      <div className="taf-baseline-scan-summary__metrics"><MiniStat label="未建连 SYN" value={syn.toLocaleString('zh-CN')} tone="risk" /><MiniStat label="外部目标" value={exposure.toLocaleString('zh-CN')} tone="warning" /><MiniStat label="结论" value="仅观测" tone="purple" /></div>
      <small>{overview?.availability.scan_classification ?? '未输出未经验证的扫描分类'}</small>
    </div>;
  }
  if (type === 'protocol') {
    return <div className="taf-baseline-discovery-cards">{(overview?.shares ?? []).slice(0, 5).map((item) => <article key={item.key}><strong>{protocolName(item.key)}</strong><span>首次观测 {formatDateTime(item.first_seen)}</span><em>{(item.share * 100).toFixed(2)}%</em><StatusTag value="已观测" /></article>)}</div>;
  }
  const heatmap = baselineOverviewHeatmap(overview?.heatmap);
  const active = heatmap.x.map((label, index) => ({ label, value: heatmap.values.filter((item) => item[0] === index).reduce((sum, item) => sum + Number(item[2] || 0), 0) })).sort((left, right) => right.value - left.value).slice(0, 3);
  return <div className="taf-baseline-periodic">
    <BaselinePeriodicityChart data={heatmap} />
    <div className="taf-baseline-periodic__signals"><strong>周期候选（真实聚合）</strong>{active.map((item, index) => <span key={item.label}><i>{index + 1}</i><b>{item.label}</b><em>{item.value.toLocaleString('zh-CN')} 会话</em></span>)}</div>
    <div className="taf-baseline-periodic__evidence"><strong>候选时段记录</strong><span><b>时段</b><b>聚合会话</b><b>判定</b></span>{active.map((item) => <span key={`evidence-${item.label}`}><em>{item.label}</em><em>{item.value.toLocaleString('zh-CN')}</em><StatusTag value="待验证" /></span>)}</div>
    <small>{overview?.availability.periodicity ?? '当前数据契约未接入周期检测器；候选仅按小时聚合强度排序，不宣称已确认周期。'}</small>
  </div>;
}

function BaselinePortraitPanel({ type, baselines, selected, overview }: { type: BaselineTabType; baselines: BehaviorBaseline[]; selected?: BehaviorBaseline; overview?: BehaviorBaselineOverview }) {
  if (type === 'account') {
    const facts = overview?.facts.filter((item) => item.kind === 'permission') ?? [];
    const actions = [...new Set(facts.map((item) => item.related_id).filter(Boolean))].slice(0, 3) as string[];
    const accounts = [...new Set(facts.map((item) => item.entity_id))].slice(0, 4);
    return <div className="taf-baseline-matrix"><div className="taf-baseline-matrix-head"><span>账号</span>{actions.map((action) => <span key={action}>{action}</span>)}</div>{accounts.map((account) => <div key={account}><strong>{account}</strong>{actions.map((action) => { const fact = facts.find((item) => item.entity_id === account && item.related_id === action); return <span key={action} className={fact?.denied ? 'is-risk' : ''}>{fact ? `${fact.count}${fact.denied ? `/${fact.denied}` : ''}` : '—'}</span>; })}</div>)}</div>;
  }
  if (type === 'port') {
    return <div className="taf-baseline-portrait-grid">{baselines.slice(0, 4).map((item, index) => <article key={item.baseline_id}><strong>{item.entity_id} / {portService(item.entity_id)}</strong><StatusTag value={index === 0 ? '异常' : '常用'} /><span>会话 {Math.round(metricValue(item, 'packets_per_session')).toLocaleString('zh-CN')}</span><span>P95 {scoreOf(item).toFixed(1)}σ</span></article>)}</div>;
  }
  if (type === 'protocol') {
    return <div className="taf-baseline-protocol-mirrors">{(overview?.shares ?? []).slice(0, 4).map((item) => <article key={item.key}><i>{protocolName(item.key).slice(0, 3).toUpperCase()}</i><div><strong>{protocolName(item.key)}</strong><span>{item.sessions.toLocaleString('zh-CN')} 会话 · {formatBytes(item.bytes)}</span><b style={{ width: `${Math.max(2, item.share * 100)}%` }} /></div><em>{(item.share * 100).toFixed(1)}%</em></article>)}</div>;
  }
  const total = overview?.kpis.find((item) => item.key === 'sessions')?.value ?? 0;
  const profiles = [
    [<ClockCircleOutlined key="night" />, '夜间 00:00–06:00', overview?.kpis.find((item) => item.key === 'night_sessions')?.value ?? 0, '夜间'],
    [<ClockCircleOutlined key="day" />, '日间 06:00–24:00', Math.max(0, total - (overview?.kpis.find((item) => item.key === 'night_sessions')?.value ?? 0)), '工作日'],
    [<CalendarOutlined key="weekend" />, '周末（周六 / 周日）', overview?.kpis.find((item) => item.key === 'weekend_sessions')?.value ?? 0, '周末'],
    [<CalendarOutlined key="weekday" />, '工作日（周一 / 周五）', Math.max(0, total - (overview?.kpis.find((item) => item.key === 'weekend_sessions')?.value ?? 0)), '工作日'],
    [<ToolOutlined key="window" />, '全窗口会话', total, '完整窗口'],
  ] as const;
  return <div className="taf-baseline-time-profiles">{profiles.map(([icon, label, value, scope]) => <article key={label}><i>{icon}</i><div><strong>{label}</strong><span>{scope} · 置信度 {total ? Math.min(.99, value / total + .35).toFixed(2) : '0.00'}</span><b><u style={{ width: `${total ? Math.max(2, value / total * 100) : 0}%` }} /></b></div><StatusTag value="真实聚合" /><em>{value.toLocaleString('zh-CN')}</em></article>)}{selected && <small>当前小时：{selected.entity_id}:00</small>}</div>;
}

function InsightRow({ label, value, tone }: { label: string; value: string; tone: 'ok' | 'warning' | 'risk' | 'purple' }) {
  return <div className={`taf-baseline-insight-row is-${tone}`}><i /><strong>{label}</strong><span>{value}</span></div>;
}

function MiniStat({ label, value, tone }: { label: string; value: string; tone: string }) {
  return <div className={`taf-baseline-mini-stat is-${tone}`}><span>{label}</span><strong>{value}</strong></div>;
}

function buildAccountLoginBoxplots(overview?: BehaviorBaselineOverview, analytics?: BehaviorBaselineAnalytics): BaselineBoxplotDatum[] {
  const real = (overview?.boxplots ?? []).slice(0, 6).map((item) => ({ label: item.entity_id, values: item.values, unit: 'hour' }));
  if (real.length) return real;
  const distribution = analytics?.distributions.find((item) => item.metric_name === 'login_hour');
  return distribution ? [{ label: '当前账号', values: distribution.values, unit: distribution.unit }] : [];
}

function buildProtocolDonut(overview?: BehaviorBaselineOverview) {
  return (overview?.shares ?? []).slice(0, 8).map((item) => ({ name: protocolName(item.key), value: item.sessions, secondaryValue: item.bytes }));
}

function tableColumns(type: BaselineTabType) {
  const variants: Record<BaselineTabType, string[]> = {
    asset: ['对象', '偏离类型', '窗口', '观察值', '偏离', '状态'],
    account: ['账号', '观察指标', '证据源', '当前观察', '偏离', '状态'],
    port: ['端口', '标准服务', '数据范围', '当前观察', '偏离', '状态'],
    protocol: ['协议', '协议编号', '数据范围', '当前观察', '偏离', '状态'],
    time: ['时间段', '时段分类', '数据范围', '当前观察', '偏离', '状态'],
  };
  return variants[type];
}

function typeSpecificValue(type: BaselineTabType, item: BehaviorBaseline, metric: BehaviorMetric | undefined, index: number) {
  if (type === 'account') return index === 0 ? metricLabel(metric?.metric_name) : 'user_events';
  if (type === 'port') return index === 0 ? portService(item.entity_id) : 'sessions 会话聚合';
  if (type === 'protocol') return index === 0 ? `IP-${item.entity_id}` : 'sessions 会话聚合';
  if (type === 'time') return index === 0 ? (Number(item.entity_id) < 6 ? '夜间' : Number(item.entity_id) >= 18 ? '晚间' : '日间') : 'sessions 小时聚合';
  return index === 0 ? metricLabel(metric?.metric_name) : '近 30 天';
}

function portService(port: string) {
  return ({ '22': 'SSH', '53': 'DNS', '80': 'HTTP', '123': 'NTP', '443': 'HTTPS', '3306': 'MySQL', '5432': 'PostgreSQL', '6379': 'Redis', '9200': 'OpenSearch' } as Record<string, string>)[port] ?? '未知服务';
}

function protocolName(protocol: string) {
  return ({ '0': 'HOPOPT', '1': 'ICMP', '2': 'IGMP', '6': 'TCP', '17': 'UDP', '41': 'IPv6', '47': 'GRE', '50': 'ESP', '58': 'ICMPv6' } as Record<string, string>)[protocol] ?? `IP-${protocol || '未知'}`;
}

function metricValue(item: BehaviorBaseline, name: string) {
  return item.metrics.find((metric) => metric.metric_name === name)?.current_value ?? 0;
}

function strongestMetric(item?: BehaviorBaseline) {
  return item?.metrics.reduce<BehaviorMetric | undefined>((best, metric) => !best || (metric.deviation_score ?? 0) > (best.deviation_score ?? 0) ? metric : best, undefined);
}

function scoreOf(item?: BehaviorBaseline) {
  return strongestMetric(item)?.deviation_score ?? 0;
}

function deviationCount(items: BehaviorBaseline[]) {
  return items.filter((item) => scoreOf(item) >= Math.max(1, strongestMetric(item)?.threshold_config.warning_multiplier ?? 2)).length;
}

function metricLabel(value?: string) {
  return ({ bytes_per_session: '异常流量', packets_per_session: '报文数偏离', duration_ms: '会话时长', events_per_window: '登录频次', source_ip_count: '来源地址', resource_count: '访问资源', login_hour: '登录小时' } as Record<string, string>)[value ?? ''] ?? value ?? '-';
}

function statusLabel(item: BehaviorBaseline) {
  if (item.frozen || item.status === 'frozen') return '已冻结';
  if (item.drift_watch || item.status === 'drift') return '漂移观察';
  if (item.status === 'learning') return '学习中';
  return scoreOf(item) >= 3 ? '待解释' : '稳定';
}

function formatMetric(value?: number, unit?: string) {
  if (value === undefined || !Number.isFinite(value)) return '-';
  const compact = Math.abs(value) >= 1000 ? value.toLocaleString('zh-CN', { maximumFractionDigits: 1 }) : value.toFixed(Math.abs(value) < 10 ? 2 : 1);
  return `${compact} ${unit ?? ''}`.trim();
}

function formatDateTime(value?: number) {
  return value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-';
}

function formatBytes(value: number) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let current = Math.max(0, value);
  let index = 0;
  while (current >= 1024 && index < units.length - 1) { current /= 1024; index += 1; }
  return `${current.toFixed(current >= 100 ? 0 : 1)} ${units[index]}`;
}
