import {
  AlertOutlined,
  AreaChartOutlined,
  ArrowDownOutlined,
  ClockCircleOutlined,
  ControlOutlined,
  ExperimentOutlined,
  EyeOutlined,
  HistoryOutlined,
  ProfileOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SwapOutlined,
  ThunderboltOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Empty, Form, Input, InputNumber, Modal, Select, Space, Tabs, Tooltip, message } from 'antd';
import { useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import {
  type BaselineBoxplotDatum,
  type BaselineScatterDatum,
  type BaselineTrendDatum,
  BaselineRailGaugeChart,
} from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import { BaselineTypeWorkspace } from '@/pages/BaselineTypeWorkspace';
import type { BaselineEvidenceTarget } from '@/pages/BaselineTypeWorkspace';
import type { NavRoute } from '@/routes/routeManifest';
import { baselineTabSlug, resolveBaselineTab } from '@/routes/pageRouteState';
import {
  fetchBehaviorBaselines,
  fetchBehaviorBaselineOverview,
  fetchBehaviorBaselineActions,
  fetchBehaviorBaselineAnalytics,
  fetchBehaviorBaselineVersions,
  submitBehaviorBaselineAction,
  type BaselineTabType,
  type BehaviorBaseline,
  type BehaviorBaselineActionRequest,
  type BehaviorBaselineAction,
  type BehaviorBaselineAnalytics,
  type BehaviorBaselineOverview,
  type BehaviorBaselineVersion,
  type BehaviorMetric,
} from '@/services/baselineApi';

const tabTypes: Record<string, BaselineTabType> = {
  资产基线: 'asset',
  账号基线: 'account',
  端口基线: 'port',
  协议基线: 'protocol',
  时间段基线: 'time',
};

const baselineFilterCopy: Record<BaselineTabType, { object: string; allObjects: string; dimension: string; risk: string }> = {
  asset: { object: '资产组', allObjects: '全部资产组', dimension: '学习状态', risk: '漂移状态' },
  account: { object: '账号', allObjects: '全部账号', dimension: '事件基线状态', risk: '治理状态' },
  port: { object: '端口', allObjects: '全部端口', dimension: '端口基线状态', risk: '治理状态' },
  protocol: { object: '协议', allObjects: '全部协议', dimension: '协议基线状态', risk: '治理状态' },
  time: { object: '小时', allObjects: '全部小时', dimension: '时段基线状态', risk: '治理状态' },
};

type GovernanceAction = BehaviorBaselineActionRequest['action'];

type ActionDialog = {
  action: GovernanceAction;
  title: string;
  impact: string;
} | null;

const actionMeta: Record<GovernanceAction, { title: string; impact: string }> = {
  create_alert: { title: '创建告警请求', impact: '提交持久化处置请求；只有下游生成告警 ID 后才视为完成。' },
  adjust_threshold: { title: '阈值编辑', impact: '更新当前基线的告警倍数并形成审计记录。' },
  freeze: { title: '冻结基线', impact: '冻结当前治理配置；检测链路确认前保持排队状态。' },
  unfreeze: { title: '解除冻结', impact: '恢复基线治理配置；检测链路确认前保持排队状态。' },
  forensics: { title: '跳转取证', impact: '携带当前基线上下文打开取证页面。' },
  feedback_model: { title: '反馈模型', impact: '提交模型反馈请求；不会把排队状态显示为已完成。' },
  cold_start: { title: '冷启动', impact: '重置学习窗口并重新进入学习状态。' },
  drift_watch: { title: '漂移观察', impact: '将当前对象加入漂移观察治理队列。' },
  rebuild: { title: '重建基线', impact: '提交重建请求并重新开始样本学习。' },
  rollback: { title: '版本回滚', impact: '仅允许回滚到服务端已持久化的历史版本。' },
  audit_trace: { title: '审计留痕', impact: '跳转到当前基线的审计检索上下文。' },
};

export function BaselineWorkbenchPage({ route }: { route: NavRoute }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = resolveBaselineTab(searchParams.get('tab'), route.page.tabs);
  const baselineType = tabTypes[activeTab] ?? 'asset';
  const sourceAssetId = searchParams.get('assetId') ?? '';
  const [objectFilter, setObjectFilter] = useState(sourceAssetId || 'all');
  const [selectedId, setSelectedId] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [driftFilter, setDriftFilter] = useState('all');
  const [versionFilter, setVersionFilter] = useState('all');
  const [windowFilter, setWindowFilter] = useState('30d');
  const [typeFilter, setTypeFilter] = useState('all');
  const [scopeFilter, setScopeFilter] = useState('all');
  const [riskFilter, setRiskFilter] = useState('all');
  const [dialog, setDialog] = useState<ActionDialog>(null);
  const [form] = Form.useForm();

  const listQuery = useQuery({
    queryKey: ['behavior-baselines', baselineType, windowFilter],
    // Page-level KPI and distribution scope comes from /overview.  The object
    // chooser only needs a bounded real sample so high-cardinality port sets do
    // not block the independent page contract for tens of seconds.
    queryFn: () => fetchBehaviorBaselines(baselineType, Number(windowFilter.replace('d', '')), 500, 500),
  });
  const overviewQuery = useQuery({
    queryKey: ['behavior-baseline-overview', baselineType, windowFilter],
    queryFn: () => fetchBehaviorBaselineOverview(baselineType, Number(windowFilter.replace('d', ''))),
  });
  const baselines = useMemo(() => listQuery.data?.baselines ?? [], [listQuery.data?.baselines]);
  const filtered = useMemo(() => baselines.filter((item) => {
    if (objectFilter !== 'all' && item.entity_id !== objectFilter) return false;
    if (statusFilter !== 'all' && item.status !== statusFilter) return false;
    if (driftFilter === 'drift' && !item.drift_watch) return false;
    if (driftFilter === 'normal' && item.drift_watch) return false;
    if (versionFilter !== 'all' && item.version !== Number(versionFilter)) return false;
    return true;
  }), [baselines, driftFilter, objectFilter, statusFilter, versionFilter]);
  const selected = filtered.find((item) => item.baseline_id === selectedId)
    ?? preferredBaseline(baselineType, filtered)
    ?? filtered
      .filter((item) => item.status === 'active' && item.created_at < Date.now() - 6 * 60 * 60 * 1000)
      .sort((left, right) => baselineSampleEnvelope(right) - baselineSampleEnvelope(left))[0]
    ?? filtered[0];
  const versionsQuery = useQuery({
    queryKey: ['behavior-baseline-versions', selected?.baseline_id],
    queryFn: () => fetchBehaviorBaselineVersions(selected!.baseline_id),
    enabled: Boolean(selected?.baseline_id),
  });
  const actionsQuery = useQuery({
    queryKey: ['behavior-baseline-actions', selected?.baseline_id],
    queryFn: () => fetchBehaviorBaselineActions(selected!.baseline_id),
    enabled: Boolean(selected?.baseline_id),
  });
  const analyticsQuery = useQuery({
    queryKey: ['behavior-baseline-analytics', selected?.baseline_id, windowFilter],
    queryFn: () => fetchBehaviorBaselineAnalytics(selected!.baseline_id, Number(windowFilter.replace('d', ''))),
    enabled: Boolean(selected?.baseline_id),
  });

  const mutation = useMutation({
    mutationFn: ({ baselineId, payload }: { baselineId: string; payload: BehaviorBaselineActionRequest }) => submitBehaviorBaselineAction(baselineId, payload),
    onSuccess: async (result) => {
      setDialog(null);
      form.resetFields();
      await queryClient.invalidateQueries({ queryKey: ['behavior-baselines', baselineType] });
      await queryClient.invalidateQueries({ queryKey: ['behavior-baseline-versions', result.action.baseline_id] });
      await queryClient.invalidateQueries({ queryKey: ['behavior-baseline-actions', result.action.baseline_id] });
      await queryClient.invalidateQueries({ queryKey: ['behavior-baseline-analytics', result.action.baseline_id] });
      if (result.action.action === 'audit_trace') {
        navigate(`/audit?objectType=baseline&objectId=${encodeURIComponent(result.action.baseline_id)}`);
      }
      if (result.action.status === 'applied') {
        message.success(`${actionMeta[result.action.action as GovernanceAction]?.title ?? '操作'}已写入治理状态并完成审计`);
      } else {
        message.info(`请求已持久化，当前状态：${result.action.status}；尚未宣称下游完成`);
      }
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '行为基线操作失败'),
  });

  const openAction = (action: GovernanceAction, targetVersion?: number) => {
    if (!selected) {
      message.warning('请先选择一个真实基线对象');
      return;
    }
    if (action === 'forensics') {
      navigate(`/forensics?baselineId=${encodeURIComponent(selected.baseline_id)}`);
      return;
    }
    const meta = actionMeta[action];
    form.setFieldsValue({
      reason: '',
      warning_multiplier: selected.metrics[0]?.threshold_config.warning_multiplier ?? 2,
      alert_multiplier: selected.metrics[0]?.threshold_config.alert_multiplier ?? 3,
      target_version: targetVersion ?? versionsQuery.data?.versions.find((version) => version.version < selected.version)?.version,
    });
    setDialog({ action, ...meta });
  };

  const openEvidence = (target: BaselineEvidenceTarget) => {
    const context = selected?.baseline_id ? `baselineId=${encodeURIComponent(selected.baseline_id)}` : `baselineType=${baselineType}`;
    if (target === 'alerts') navigate(`/alerts?${context}`);
    else if (target === 'pcap') navigate(`/forensics?tab=PCAP%E8%AF%81%E6%8D%AE&${context}`);
    else if (target === 'sessions') navigate(`/forensics?tab=Session%E8%AE%B0%E5%BD%95&${context}`);
    else if (target === 'audit') navigate(`/audit?objectType=baseline&objectId=${encodeURIComponent(selected?.baseline_id ?? baselineType)}`);
    else document.querySelector('.taf-baseline-tertiary')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
  };

  const submitAction = async () => {
    if (!selected || !dialog) return;
    const values = await form.validateFields();
    mutation.mutate({ baselineId: selected.baseline_id, payload: { action: dialog.action, ...values } });
  };

  const counts = listQuery.data?.summary ?? summarizeBaselines(baselines);
  const stateCounts = listQuery.data?.summary ?? stateMachineCounts(baselines);
  const chartData = buildChartData(selected, analyticsQuery.data);
  const kpis = buildBaselineKpis(baselineType, counts, listQuery.data?.total ?? 0, overviewQuery.data);
  const filterCopy = baselineFilterCopy[baselineType];

  return (
    <div className="taf-page taf-baseline-workbench">
      <section className={`taf-baseline-left taf-baseline-left--${baselineType}`}>
        <header className="taf-baseline-title-tabs">
          <div className="taf-baseline-heading-row">
            <h1>{route.page.title}</h1>
            <Space size={6}>
              <Tooltip title="刷新真实基线数据"><Button size="small" icon={<ReloadOutlined />} loading={listQuery.isFetching} onClick={() => void listQuery.refetch()}>刷新</Button></Tooltip>
              <Button size="small" icon={<ControlOutlined />} onClick={() => openAction(selected?.frozen ? 'unfreeze' : 'freeze')}>管理基线</Button>
            </Space>
          </div>
          <Tabs
            activeKey={activeTab}
            onChange={(tab) => setSearchParams((current) => {
              setObjectFilter('all');
              setSelectedId('');
              const next = new URLSearchParams(current);
              next.set('tab', baselineTabSlug(tab));
              return next;
            })}
            items={route.page.tabs.map((tab) => ({ key: tab, label: tab }))}
          />
        </header>

        <div className="taf-baseline-filter" data-filter-count={baselineType === 'asset' ? 5 : 6}>
          <label><span>{specialistFilterCopy(baselineType).object ?? filterCopy.object}</span><Select size="small" showSearch value={objectFilter} onChange={setObjectFilter} options={[{ value: 'all', label: specialistFilterCopy(baselineType).allObjects ?? filterCopy.allObjects }, ...baselines.map((item) => ({ value: item.entity_id, label: item.entity_id }))]} /></label>
          <label><span>历史窗口</span><Select size="small" value={windowFilter} onChange={setWindowFilter} options={[{ value: '7d', label: '近 7 天' }, { value: '30d', label: '近 30 天' }, { value: '90d', label: '近 90 天' }]} /></label>
          {baselineType === 'asset' ? <>
            <label><span>{filterCopy.dimension}</span><Select size="small" value={statusFilter} onChange={setStatusFilter} options={[{ value: 'all', label: '全部' }, { value: 'learning', label: '学习中' }, { value: 'active', label: '稳定' }, { value: 'frozen', label: '已冻结' }]} /></label>
            <label><span>{filterCopy.risk}</span><Select size="small" value={driftFilter} onChange={setDriftFilter} options={[{ value: 'all', label: '全部' }, { value: 'drift', label: '漂移观察' }, { value: 'normal', label: '无漂移' }]} /></label>
          </> : <>
            <SpecialistSelect label={specialistFilterCopy(baselineType).typeLabel} value={typeFilter} onChange={setTypeFilter} options={specialistFilterCopy(baselineType).typeOptions} />
            <SpecialistSelect label={specialistFilterCopy(baselineType).scopeLabel} value={scopeFilter} onChange={setScopeFilter} options={specialistFilterCopy(baselineType).scopeOptions} />
            <SpecialistSelect label={specialistFilterCopy(baselineType).riskLabel} value={riskFilter} onChange={setRiskFilter} options={specialistFilterCopy(baselineType).riskOptions} />
          </>}
          <label><span>{baselineType === 'asset' ? '版本' : '基线版本'}</span><Select size="small" value={versionFilter} onChange={setVersionFilter} options={[{ value: 'all', label: '全部版本' }, ...uniqueVersions(baselines).map((value) => ({ value: String(value), label: `v${value}` }))]} /></label>
        </div>

        <div className="taf-baseline-kpis" data-kpi-count={kpis.length}>
          {kpis.map((item) => <BaselineKpi key={item.label} {...item} />)}
        </div>

        {listQuery.isError && <Alert showIcon type="error" message="真实行为基线加载失败" description={listQuery.error instanceof Error ? listQuery.error.message : '请检查 /v1/baselines、ClickHouse 与 APISIX 路由。'} action={<Button size="small" onClick={() => void listQuery.refetch()}>重试</Button>} />}
        {overviewQuery.isError && <Alert showIcon type="error" message="页面级真实聚合加载失败" description={overviewQuery.error instanceof Error ? overviewQuery.error.message : '请检查 /v1/baselines/overview 与 ClickHouse 聚合查询。'} action={<Button size="small" onClick={() => void overviewQuery.refetch()}>重试</Button>} />}

        <BaselineTypeWorkspace
          type={baselineType}
          baselines={filtered}
          selected={selected}
          analytics={analyticsQuery.data}
          overview={overviewQuery.data}
          chartData={chartData}
          stateMachine={<BaselineStateMachine type={baselineType} counts={stateCounts} overview={overviewQuery.data} />}
          versionGovernance={<VersionGovernance selected={selected} versions={versionsQuery.data?.versions ?? []} loading={versionsQuery.isLoading} error={versionsQuery.isError} onRetry={() => void versionsQuery.refetch()} onRollback={(version) => openAction('rollback', version)} />}
          windowLabel={windowFilter === '30d' ? '近30天聚合' : windowFilter === '7d' ? '近7天视图' : '近90天视图'}
          loading={listQuery.isLoading || analyticsQuery.isLoading || overviewQuery.isLoading}
          error={analyticsQuery.isError || overviewQuery.isError}
          onRetry={() => { void analyticsQuery.refetch(); void overviewQuery.refetch(); }}
          onSelect={setSelectedId}
          onEvidenceNavigate={openEvidence}
        />
      </section>

      <aside className={`taf-baseline-detail taf-baseline-detail--${baselineType}`}>
        <DeviationExplanation type={baselineType} selected={selected} overview={overviewQuery.data} onAction={openAction} />
        {baselineType !== 'account' && <GovernanceActions type={baselineType} selected={selected} versions={versionsQuery.data?.versions ?? []} persistedActions={actionsQuery.data?.actions ?? []} error={actionsQuery.isError} onRetry={() => void actionsQuery.refetch()} onAction={openAction} />}
      </aside>

      <Modal
        title={dialog?.title}
        open={Boolean(dialog)}
        width={520}
        style={{ top: 56 }}
        maskClosable={false}
        destroyOnClose
        confirmLoading={mutation.isPending}
        okText="确认提交"
        cancelText="取消"
        onCancel={() => setDialog(null)}
        onOk={() => void submitAction()}
        getContainer={false}
      >
        <Alert showIcon type="warning" message="影响范围" description={dialog?.impact} />
        <Form form={form} layout="vertical" className="taf-baseline-action-form">
          {dialog?.action === 'adjust_threshold' && (
            <div className="taf-baseline-threshold-fields">
              <Form.Item label="预警倍数" name="warning_multiplier" rules={[{ required: true }]}><InputNumber min={0.1} max={19} step={0.1} /></Form.Item>
              <Form.Item label="告警倍数" name="alert_multiplier" dependencies={['warning_multiplier']} rules={[{ required: true }, ({ getFieldValue }) => ({ validator: (_, value) => value > getFieldValue('warning_multiplier') ? Promise.resolve() : Promise.reject(new Error('告警倍数必须大于预警倍数')) })]}><InputNumber min={0.2} max={20} step={0.1} /></Form.Item>
            </div>
          )}
          {dialog?.action === 'rollback' && <Form.Item label="目标版本" name="target_version" rules={[{ required: true, message: '请选择服务端已持久化版本' }]}><Select options={(versionsQuery.data?.versions ?? []).filter((version) => version.version < (selected?.version ?? 1)).map((version) => ({ value: version.version, label: `v${version.version} · ${formatTimestamp(version.created_at)}` }))} /></Form.Item>}
          <Form.Item label="操作原因" name="reason" rules={[{ required: dialog?.action !== 'audit_trace', message: '治理操作必须填写原因' }, { max: 500 }]}><Input.TextArea rows={3} placeholder="说明操作依据、影响范围和预期结果" /></Form.Item>
        </Form>
        <div className="taf-baseline-audit-hint">权限：alert:write / admin · 请求与审计记录在同一事务写入 · 排队不等于下游完成</div>
      </Modal>
    </div>
  );
}

type SpecialistFilterCopy = {
  object: string;
  allObjects: string;
  typeLabel: string;
  typeOptions: Array<{ value: string; label: string }>;
  scopeLabel: string;
  scopeOptions: Array<{ value: string; label: string }>;
  riskLabel: string;
  riskOptions: Array<{ value: string; label: string }>;
};

function SpecialistSelect({ label, value, onChange, options }: { label: string; value: string; onChange: (value: string) => void; options: Array<{ value: string; label: string }> }) {
  return <label><span>{label}</span><Select size="small" value={value} onChange={onChange} options={options} /></label>;
}

function specialistFilterCopy(type: BaselineTabType): SpecialistFilterCopy {
  const all = { value: 'all', label: '全部' };
  const variants: Record<Exclude<BaselineTabType, 'asset'>, SpecialistFilterCopy> = {
    account: {
      object: '账号组', allObjects: '全部组', typeLabel: '账号类型', typeOptions: [all, { value: 'human', label: '人员账号' }, { value: 'service', label: '服务账号' }],
      scopeLabel: '地理范围', scopeOptions: [all, { value: 'campus', label: '园区内' }, { value: 'external', label: '园区外' }],
      riskLabel: '权限域', riskOptions: [all, { value: 'normal', label: '常规权限' }, { value: 'privileged', label: '高权限' }],
    },
    port: {
      object: '资产组', allObjects: '全部资产组', typeLabel: '端口范围', typeOptions: [all, { value: 'well-known', label: '知名端口' }, { value: 'dynamic', label: '动态端口' }],
      scopeLabel: '协议', scopeOptions: [all, { value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }],
      riskLabel: '暴露面', riskOptions: [all, { value: 'internal', label: '内网' }, { value: 'external', label: '外网暴露' }],
    },
    protocol: {
      object: '资产组', allObjects: '全部资产组', typeLabel: '协议族', typeOptions: [all, { value: 'transport', label: '传输层' }, { value: 'network', label: '网络层' }],
      scopeLabel: '方向', scopeOptions: [all, { value: 'inbound', label: '入站' }, { value: 'outbound', label: '出站' }],
      riskLabel: '风险等级', riskOptions: [all, { value: 'high', label: '高风险' }, { value: 'medium', label: '中风险' }, { value: 'low', label: '低风险' }],
    },
    time: {
      object: '资产组', allObjects: '全部资产组', typeLabel: '时段类型', typeOptions: [all, { value: 'workday', label: '工作日' }, { value: 'night', label: '夜间' }, { value: 'weekend', label: '周末' }],
      scopeLabel: '业务日历', scopeOptions: [all, { value: 'school', label: '校园业务日历' }, { value: 'natural', label: '自然日历' }],
      riskLabel: '异常等级', riskOptions: [all, { value: 'high', label: '高风险' }, { value: 'medium', label: '中风险' }, { value: 'low', label: '低风险' }],
    },
  };
  return type === 'asset' ? {
    object: '资产组', allObjects: '全部资产组', typeLabel: '学习状态', typeOptions: [all], scopeLabel: '漂移状态', scopeOptions: [all], riskLabel: '版本', riskOptions: [all],
  } : variants[type];
}

function BaselineKpi({ icon, label, value, detail, tone }: { icon: ReactNode; label: string; value: number; detail: string; tone: string }) {
  return <div className={`taf-baseline-kpi is-${tone}`}><i>{icon}</i><div><span>{label}</span><strong>{value.toLocaleString()}</strong><small>{detail}</small></div></div>;
}

function BaselineStateMachine({ type, counts, overview }: { type: BaselineTabType; counts: ReturnType<typeof stateMachineCounts>; overview?: BehaviorBaselineOverview }) {
  const observed = (key: string) => Math.round(overview?.kpis.find((item) => item.key === key)?.value ?? 0);
  const variants = {
    asset: [
      ['学习中', counts.learning, '自动采样', 'info'],
      ['稳定', counts.active, '阈值生效', 'ok'],
      ['漂移观察', counts.drift, '持续评估', 'warn'],
      ['待重建', counts.rebuild, '人工干预', 'risk'],
      ['已冻结', counts.frozen, '停止变更', 'purple'],
    ],
    account: [
      ['学习中', counts.learning, '自动采样', 'info'],
      ['稳定', counts.active, '阈值生效', 'ok'],
      ['漂移观察', counts.drift, '持续评估', 'warn'],
      ['待重建', counts.rebuild, '人工干预', 'risk'],
      ['已冻结', counts.frozen, '停止变更', 'purple'],
    ],
    port: [
      ['学习中', counts.learning, '自动采样', 'info'],
      ['稳定', counts.active, '阈值生效', 'ok'],
      ['新端口观察', counts.drift, '持续评估', 'warn'],
      ['扫描疑似', counts.rebuild, `未建连 SYN ${observed('failed_syn').toLocaleString('zh-CN')}`, 'risk'],
      ['服务变更', counts.rebuild, '人工确认', 'purple'],
      ['已冻结', counts.frozen, '停止变更', 'purple'],
    ],
    protocol: [
      ['学习中', counts.learning, '自动采样', 'info'],
      ['稳定', counts.active, '阈值生效', 'ok'],
      ['漂移观察', counts.drift, '持续评估', 'warn'],
      ['新协议确认', 0, '人工确认', 'purple'],
      ['异常协议', counts.rebuild, '基线重建', 'risk'],
      ['已冻结', counts.frozen, '停止变更', 'purple'],
    ],
    time: [
      ['学习中', counts.learning, '自动采样', 'info'],
      ['工作日稳定', counts.active, '阈值生效', 'ok'],
      ['夜间观察', counts.drift, '持续评估', 'warn'],
      ['周末偏离', 0, '人工确认', 'purple'],
      ['周期追踪', 0, '检测器待接入', 'risk'],
      ['已冻结', counts.frozen, '停止变更', 'purple'],
    ],
  } satisfies Record<BaselineTabType, Array<readonly [string, number, string, string]>>;
  const states = variants[type];
  const total = Math.max(1, states.reduce((sum, [, count]) => sum + count, 0));
  return <div className={`taf-baseline-state-machine is-${type} ${type === 'asset' ? '' : 'is-specialist'} ${states.length === 6 ? 'is-six-state' : 'is-five-state'}`}>{states.map(([label, count, note, tone], index) => (
    <div key={label} className={`is-${tone}`}>
      <i>{index < states.length - 1 && <span />}</i>
      <strong>{label}</strong>
      <em>对象 {count}</em>
      <small>{Math.round(count / total * 100)}%</small>
      <b>{note}</b>
      {type !== 'asset' && index < states.length - 1 && <span className="taf-baseline-state-transition"><ArrowDownOutlined /></span>}
    </div>
  ))}
    {type !== 'asset' && <aside className="taf-baseline-state-rails" aria-label="基线状态迁移路径">
      <span><SwapOutlined />自动迁移</span>
      <span><UserSwitchOutlined />人工干预</span>
      <span><HistoryOutlined />重建回流</span>
    </aside>}
    <footer><span>自动迁移</span><span>人工干预</span><span>重建回流</span></footer>
  </div>;
}

function VersionGovernance({ selected, versions, loading, error, onRetry, onRollback }: { selected?: BehaviorBaseline; versions: BehaviorBaselineVersion[]; loading: boolean; error: boolean; onRetry: () => void; onRollback: (version: number) => void }) {
  if (!selected) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择基线对象" />;
  if (loading) return <div className="taf-baseline-loading">正在读取持久化版本…</div>;
  if (error) return <Alert showIcon type="error" message="持久化版本读取失败" action={<Button size="small" onClick={onRetry}>重试</Button>} />;
  const persisted = versions.length ? versions : [{ baseline_id: selected.baseline_id, version: selected.version, snapshot: { warning_multiplier: selected.metrics[0]?.threshold_config.warning_multiplier, alert_multiplier: selected.metrics[0]?.threshold_config.alert_multiplier, frozen: selected.frozen, drift_watch: selected.drift_watch }, created_by: '', created_at: selected.updated_at }];
  const timeline = persisted.slice(0, 3).sort((left, right) => left.version - right.version);
  return <div className="taf-baseline-versions">
    <div className="taf-baseline-version-line">{timeline.map((version) => <span className={version.version === selected.version ? 'is-current' : ''} key={version.version}><b>v{version.version}</b><em>{version.version === selected.version ? '当前版本' : formatDate(version.created_at)}</em></span>)}</div>
    <div className="taf-baseline-version-head"><span>版本</span><span>状态</span><span>创建时间</span><span>阈值策略</span><span>创建人</span><span>操作</span></div>
    {persisted.slice(0, 4).map((version) => <div className="taf-baseline-version-row" key={version.version}><strong>v{version.version}</strong><StatusTag value={version.version === selected.version ? statusLabel(selected.status) : '历史'} /><span>{formatTimestamp(version.created_at)}</span><span>{version.snapshot.alert_multiplier ? `预警 ${version.snapshot.warning_multiplier} / 告警 ${version.snapshot.alert_multiplier}` : '快照缺失'}</span><span title={version.created_by}>{version.created_by ? shortId(version.created_by) : '-'}</span><Button size="small" disabled={version.version >= selected.version} onClick={() => onRollback(version.version)}>回滚</Button></div>)}
    {!versions.length && <div className="taf-baseline-version-empty">服务端尚无历史快照；当前版本用于展示，但不会被伪装成可回滚记录。</div>}
  </div>;
}

function DeviationExplanation({ type, selected, overview, onAction }: { type: BaselineTabType; selected?: BehaviorBaseline; overview?: BehaviorBaselineOverview; onAction: (action: GovernanceAction) => void }) {
  const metric = strongestMetric(selected);
  const score = metric?.deviation_score ?? 0;
  const warning = metric?.threshold_config.warning_multiplier ?? 2;
  const alert = metric?.threshold_config.alert_multiplier ?? 3;
  const confidence = Math.min(0.99, Math.max(0, score / Math.max(1, alert)));
  const protocolShare = type === 'protocol' ? overview?.shares.find((item) => item.key === selected?.entity_id) : undefined;
  const observed = (key: string) => overview?.kpis.find((item) => item.key === key)?.value ?? 0;
  const typeTitle = type === 'account' ? '账号行为偏离'
    : type === 'port' ? '端口会话偏离'
      : type === 'protocol' ? '协议占比偏离'
        : type === 'time' ? '访问时段偏离'
          : '资产行为偏离';
  const observation = protocolShare
    ? `当前窗口该协议占全部会话 ${(protocolShare.share * 100).toFixed(2)}%，共 ${protocolShare.sessions.toLocaleString('zh-CN')} 个会话；基线对象的 ${metricLabel(metric?.metric_name ?? '')} 偏离为 ${score.toFixed(2)}σ。`
    : metric
      ? `${metricLabel(metric.metric_name)}当前观察值为 ${formatMetric(metric.current_value, metric.unit)}，相对均值 ${formatMetric(metric.mean, metric.unit)} 的标准差偏离为 ${score.toFixed(2)}。`
      : '该对象暂无可解释的行为指标，系统不会生成推测性结论。';
  const suggestion = score >= alert ? '确认业务合理性，必要时创建告警并跳转取证。' : '继续观察偏离趋势，暂不宣称告警已生成。';
  const source = overview?.source ?? (type === 'account' ? 'ClickHouse traffic.user_events' : 'ClickHouse traffic.sessions');
  const specialistFacts: Record<Exclude<BaselineTabType, 'asset'>, Array<[string, string]>> = {
    account: [
      ['账号', selected?.entity_id ?? '-'],
      ['类型', selected?.entity_type ?? '-'],
      ['登录时间', metric?.metric_name === 'login_hour' ? formatMetric(metric.current_value, metric.unit) : metricLabel(metric?.metric_name ?? '')],
      ['地理位置', `${Math.round(observed('source_addresses')).toLocaleString('zh-CN')} 个来源地址`],
      ['访问资产', `${Math.round(observed('resources')).toLocaleString('zh-CN')} 个真实聚合对象`],
      ['权限变化', `${Math.round(observed('permission_changes')).toLocaleString('zh-CN')} 条`],
      ['证据来源', source],
      ['关联告警', `${Math.round(observed('denied_events')).toLocaleString('zh-CN')} 条拒绝事件`],
      ['置信度', confidence.toFixed(2)],
      ['基线版本', `v${selected?.version ?? '-'}`],
      ['解释说明', suggestion],
    ],
    port: [
      ['资产', '当前资产组聚合'],
      ['端口 / 协议', `${selected?.entity_id ?? '-'} / TCP`],
      ['服务', portService(selected?.entity_id ?? '')],
      ['基线状态', selected ? statusLabel(selected.status) : '-'],
      ['当前观测', formatMetric(metric?.current_value, metric?.unit)],
      ['偏离类型', `${metricLabel(metric?.metric_name ?? '')}偏离`],
      ['服务变化', `${Math.round(observed('ports')).toLocaleString('zh-CN')} 个端口对象`],
      ['证据来源', source],
      ['关联告警', `${Math.round(observed('failed_syn')).toLocaleString('zh-CN')} 个未建连 SYN`],
      ['置信度', confidence.toFixed(2)],
    ],
    protocol: [
      ['资产', '当前资产组聚合'],
      ['协议', protocolName(selected?.entity_id ?? '')],
      ['基线占比', metric ? formatMetric(metric.mean, metric.unit) : '-'],
      ['当前占比', protocolShare ? `${(protocolShare.share * 100).toFixed(2)}%` : formatMetric(metric?.current_value, metric?.unit)],
      ['偏离类型', '协议占比漂移'],
      ['方向', '会话双向聚合'],
      ['目的域', `${Math.round(observed('protocols')).toLocaleString('zh-CN')} 个协议对象`],
      ['证据来源', source],
      ['关联告警', `${Math.round(observed('unknown_protocols')).toLocaleString('zh-CN')} 个未知协议`],
      ['置信度', confidence.toFixed(2)],
    ],
    time: [
      ['资产', '当前资产组聚合'],
      ['时段', `${String(selected?.entity_id ?? '-').padStart(2, '0')}:00–${String((Number(selected?.entity_id ?? 0) + 1) % 24).padStart(2, '0')}:00`],
      ['基线行为', metricLabel(metric?.metric_name ?? '')],
      ['当前观测', formatMetric(metric?.current_value, metric?.unit)],
      ['偏离类型', Number(selected?.entity_id ?? 0) < 6 ? '异常夜间访问' : '访问时段偏离'],
      ['周期性', `${Math.round(observed('active_hours')).toLocaleString('zh-CN')} 个活跃小时`],
      ['目的地', `${Math.round(observed('sessions')).toLocaleString('zh-CN')} 个会话`],
      ['证据来源', source],
      ['关联告警', `${Math.round(observed('night_sessions')).toLocaleString('zh-CN')} 个夜间会话`],
      ['置信度', confidence.toFixed(2)],
    ],
  };
  const facts = type === 'asset' ? [
    ['偏离指标', metric ? metricLabel(metric.metric_name) : '-'],
    ['偏离强度', `${score.toFixed(2)}σ（相对告警阈值）`],
    ['证据来源', source],
    ['关联对象', selected?.entity_id ?? '-'],
    ['实体类型', selected?.entity_type ?? '-'],
    ['置信度', confidence.toFixed(2)],
    ['建议处置', suggestion],
  ] as Array<[string, string]> : specialistFacts[type];
  return <WorkPanel title="偏离解释" extra={<StatusTag value={score >= (metric?.threshold_config.alert_multiplier ?? 3) ? '高风险' : score > 0 ? '观察中' : '正常'} />}>
    {!selected ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择一个基线对象" /> : <div className="taf-baseline-explanation">
      <div className="taf-baseline-explain-title"><div><small>{selected.baseline_id}</small><strong>{type === 'asset' ? typeTitle : `${selected.entity_id} · ${typeTitle}`}</strong></div><span>v{selected.version}</span></div>
      <section><h4>为什么判定为异常</h4><p>{observation}</p></section>
      <div className="taf-baseline-comparison">
        <div><span>基线（当前窗口）</span><span>观察值（当前）</span></div>
        <div><span>基线均值</span><strong>{formatMetric(metric?.mean, metric?.unit)}</strong><span>当前观察</span><strong className="is-danger">{formatMetric(metric?.current_value, metric?.unit)}</strong></div>
        <div><span>正常区间</span><strong>{metric ? `${formatMetric(metric.normal_range[0], metric.unit)} ~ ${formatMetric(metric.normal_range[1], metric.unit)}` : '-'}</strong><span>阈值</span><strong>{warning}σ / {alert}σ</strong></div>
      </div>
      <dl className="taf-baseline-explain-facts">{facts.flatMap(([label, value]) => [<dt key={`${label}-term`}>{label}</dt>, <dd key={`${label}-value`} className={label.includes('偏离') ? 'is-danger' : ''} title={value}>{value}</dd>])}</dl>
      {type !== 'asset' && <BaselineRailGaugeChart items={baselineRailMetrics(type, score, alert, overview)} ariaLabel={`${typeTitle}摘要指标`} />}
      <div className="taf-baseline-explain-actions"><Button danger type="primary" icon={<AlertOutlined />} onClick={() => onAction('create_alert')}>创建告警</Button><Button icon={<ControlOutlined />} onClick={() => onAction('adjust_threshold')}>调整阈值</Button><Button icon={<SafetyCertificateOutlined />} onClick={() => onAction(selected.frozen ? 'unfreeze' : 'freeze')}>{selected.frozen ? '解除冻结' : '冻结基线'}</Button><Button icon={<EyeOutlined />} onClick={() => onAction('forensics')}>跳转取证</Button><Button icon={<ExperimentOutlined />} onClick={() => onAction('feedback_model')}>反馈模型</Button></div>
    </div>}
  </WorkPanel>;
}

function baselineRailMetrics(type: BaselineTabType, score: number, alert: number, overview?: BehaviorBaselineOverview) {
  const observed = (key: string) => Math.max(0, overview?.kpis.find((item) => item.key === key)?.value ?? 0);
  const facts = (kind: string) => overview?.facts.filter((item) => item.kind === kind).reduce((sum, item) => sum + Math.max(0, item.count), 0) ?? 0;
  const labels = type === 'account'
    ? [['登录偏离', score, Math.max(alert, score), `${score.toFixed(1)}σ`], ['拒绝事件', observed('denied_events'), Math.max(1, observed('events')), compactCount(observed('denied_events'))], ['权限漂移', observed('permission_changes'), Math.max(1, observed('events')), compactCount(observed('permission_changes'))], ['来源地址', observed('source_addresses'), Math.max(1, observed('accounts') * 4), compactCount(observed('source_addresses'))]]
    : type === 'port'
      ? [['偏离强度', score, Math.max(alert, score), `${score.toFixed(1)}σ`], ['未建连 SYN', observed('failed_syn'), Math.max(1, observed('sessions')), compactCount(observed('failed_syn'))], ['外部目标', observed('external_destinations'), Math.max(1, observed('ports')), compactCount(observed('external_destinations'))], ['暴露证据', facts('exposure'), Math.max(1, observed('external_destinations')), compactCount(facts('exposure'))]]
      : type === 'protocol'
        ? [['占比偏离', score, Math.max(alert, score), `${score.toFixed(1)}σ`], ['协议数量', observed('protocols'), Math.max(1, observed('protocols')), compactCount(observed('protocols'))], ['未知协议', observed('unknown_protocols'), Math.max(1, observed('protocols')), compactCount(observed('unknown_protocols'))], ['协议会话', observed('sessions'), Math.max(1, observed('sessions')), compactCount(observed('sessions'))]]
        : [['时段偏离', score, Math.max(alert, score), `${score.toFixed(1)}σ`], ['夜间会话', observed('night_sessions'), Math.max(1, observed('sessions')), compactCount(observed('night_sessions'))], ['周末会话', observed('weekend_sessions'), Math.max(1, observed('sessions')), compactCount(observed('weekend_sessions'))], ['活跃小时', observed('active_hours'), 24, compactCount(observed('active_hours'))]];
  const colors = ['#ff5d62', '#36d66b', '#ffb020', '#46c8ff'];
  return labels.map(([label, value, maximum, display], index) => ({ label: String(label), value: Number(value), maximum: Math.max(1, Number(maximum)), display: String(display), color: colors[index] }));
}

function compactCount(value: number) {
  return Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}

function GovernanceActions({ type, selected, versions, persistedActions, error, onRetry, onAction }: { type: BaselineTabType; selected?: BehaviorBaseline; versions: BehaviorBaselineVersion[]; persistedActions: BehaviorBaselineAction[]; error: boolean; onRetry: () => void; onAction: (action: GovernanceAction) => void }) {
  const actions: Array<[string, GovernanceAction, ReactNode]> = [
    ['冷启动', 'cold_start', <ThunderboltOutlined key="cold" />], ['漂移观察', 'drift_watch', <AreaChartOutlined key="drift" />], ['重建', 'rebuild', <ReloadOutlined key="rebuild" />], [selected?.frozen ? '解除冻结' : '冻结', selected?.frozen ? 'unfreeze' : 'freeze', <SafetyCertificateOutlined key="freeze" />], ['版本回滚', 'rollback', <HistoryOutlined key="rollback" />], ['审计留痕', 'audit_trace', <ProfileOutlined key="audit" />],
  ];
  if (type === 'port') {
    return <WorkPanel title="基线治理与版本">
      <BaselineRailVersionTimeline selected={selected} versions={versions} onAction={onAction} compact />
    </WorkPanel>;
  }
  return <WorkPanel title="治理与操作" extra={<span className="taf-baseline-permission">alert:write</span>}>
    <div className="taf-baseline-governance-actions">{actions.map(([label, action, icon]) => <Button key={action} icon={icon} disabled={!selected || (action === 'rollback' && selected.version <= 1)} onClick={() => onAction(action)}>{label}</Button>)}</div>
    <div className="taf-baseline-action-log"><h4><ClockCircleOutlined /> 最近操作</h4>{error ? <Alert showIcon type="error" message="治理操作读取失败" action={<Button size="small" onClick={onRetry}>重试</Button>} /> : persistedActions.length ? persistedActions.slice(0, 4).map((action) => <span key={action.action_id} title={action.downstream_error || action.reason}><b>{actionLabel(action.action)}</b><em>{formatTimestamp(action.created_at)}</em><StatusTag value={action.downstream_status === 'failed' ? '下游失败' : action.downstream_status === 'published' ? '已发布' : action.status === 'applied' ? '本地已生效' : action.status} /></span>) : <span><b>暂无持久化操作</b><em>治理动作提交后将在此显示</em></span>}<small>queued 仅表示等待下游消费；published/failed 直接来自持久化 outbox 状态。</small></div>
    {selected && <div className="taf-baseline-governance-version-rail"><h4><HistoryOutlined /> 基线与版本</h4><span><b>当前版本</b><em>v{selected.version}</em><StatusTag value={statusLabel(selected.status)} /></span><span><b>最近更新</b><em>{formatTimestamp(selected.updated_at)}</em><StatusTag value={selected.frozen ? '已冻结' : '本地已生效'} /></span><span><b>治理对象</b><em title={selected.entity_id}>{shortId(selected.entity_id)}</em><StatusTag value={selected.drift_watch ? '漂移观察' : '稳定'} /></span></div>}
  </WorkPanel>;
}

function BaselineRailVersionTimeline({ selected, versions, onAction, compact = false }: { selected?: BehaviorBaseline; versions: BehaviorBaselineVersion[]; onAction: (action: GovernanceAction) => void; compact?: boolean }) {
  if (!selected) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择基线对象" />;
  const persisted = versions.length ? versions.slice(0, 4) : [{
    baseline_id: selected.baseline_id,
    version: selected.version,
    snapshot: { frozen: selected.frozen, drift_watch: selected.drift_watch },
    created_by: '',
    created_at: selected.updated_at,
  }];
  return <section className={`taf-baseline-rail-version-timeline${compact ? ' is-compact' : ''}`}>
    <h4><HistoryOutlined /> 基线版本与治理</h4>
    <div>{persisted.map((version) => <article key={version.version} className={version.version === selected.version ? 'is-current' : ''}>
      <i />
      <strong>v{version.version}</strong>
      <StatusTag value={version.version === selected.version ? statusLabel(selected.status) : '历史'} />
      <time>{formatTimestamp(version.created_at)}</time>
      <em>{version.version === selected.version ? '当前版本' : '已持久化'}</em>
    </article>)}</div>
    <footer>
      <Button size="small" onClick={() => onAction(selected.frozen ? 'unfreeze' : 'freeze')}>{selected.frozen ? '解除冻结' : '冻结基线'}</Button>
      <Button size="small" disabled={selected.version <= 1} onClick={() => onAction('rollback')}>版本回滚</Button>
      <Button size="small" onClick={() => onAction('adjust_threshold')}>调整阈值</Button>
      <Button size="small" onClick={() => onAction('audit_trace')}>导出报告</Button>
    </footer>
  </section>;
}

function summarizeBaselines(items: BehaviorBaseline[]) {
  return {
    learning: items.filter((item) => item.status === 'learning').length,
    drift: items.filter((item) => item.drift_watch || item.status === 'drift').length,
    frozen: items.filter((item) => item.frozen || item.status === 'frozen').length,
    alerts: items.filter((item) => item.metrics.some((metric) => (metric.deviation_score ?? 0) >= metric.threshold_config.alert_multiplier)).length,
  };
}

function buildBaselineKpis(type: BaselineTabType, counts: ReturnType<typeof summarizeBaselines>, total: number, overview?: BehaviorBaselineOverview) {
  const observed = (key: string, fallback = 0) => overview?.kpis.find((item) => item.key === key)?.value ?? fallback;
  const dominantProtocol = overview?.shares.slice().sort((left, right) => right.sessions - left.sessions)[0];
  const stable = Math.max(0, total - counts.learning - counts.drift - counts.frozen);
  const base = {
    asset: [
      [<ProfileOutlined key="covered" />, '基线覆盖对象', observed('assets', total), 'ClickHouse src_ip 去重', 'info'],
      [<ThunderboltOutlined key="learning" />, '学习中对象', counts.learning, '全窗口治理汇总', 'cyan'],
      [<AreaChartOutlined key="drift" />, '漂移观察', counts.drift, '持久化治理状态', 'warning'],
      [<SafetyCertificateOutlined key="frozen" />, '冻结版本', counts.frozen, '持久化治理状态', 'purple'],
      [<AlertOutlined key="alerts" />, '阈值偏离', counts.alerts, '完整对象范围', 'danger'],
    ],
    account: [
      [<ProfileOutlined key="covered" />, '基线账号', observed('accounts', total), '覆盖率 · username 去重', 'info'],
      [<ClockCircleOutlined key="learning" />, '学习中', counts.learning, '完整账号范围', 'info'],
      [<SafetyCertificateOutlined key="stable" />, '稳定账号', stable, '阈值已生效', 'cyan'],
      [<AreaChartOutlined key="drift" />, '漂移账号', counts.drift, '持续观察', 'warning'],
      [<AlertOutlined key="denied" />, '异常登录', observed('denied_events'), 'result 非 success', 'danger'],
      [<ExperimentOutlined key="permission" />, '权限漂移', observed('permission_changes'), 'permission_change 事件', 'warning'],
      [<SafetyCertificateOutlined key="high" />, '高危账号', counts.alerts, '超过告警阈值', 'danger'],
      [<AlertOutlined key="related" />, '关联告警', counts.alerts, '24h 新增', 'purple'],
    ],
    port: [
      [<ProfileOutlined key="covered" />, '基线端口', observed('ports', total), 'dst_port 去重', 'info'],
      [<SafetyCertificateOutlined key="common" />, '常用端口', stable, '稳定端口对象', 'cyan'],
      [<ThunderboltOutlined key="new" />, '新端口', counts.drift, '首次出现或漂移', 'warning'],
      [<ExperimentOutlined key="scan" />, '扫描行为', observed('failed_syn'), '未建连 SYN', 'purple'],
      [<AreaChartOutlined key="service" />, '服务变更', counts.learning, '待确认服务对象', 'warning'],
      [<AlertOutlined key="external" />, '外网暴露', observed('external_destinations'), '公网目标地址去重', 'warning'],
      [<SafetyCertificateOutlined key="high" />, '高危端口', counts.alerts, '超过告警阈值', 'danger'],
      [<AlertOutlined key="related" />, '关联告警', counts.alerts, '24h 新增', 'purple'],
    ],
    protocol: [
      [<ProfileOutlined key="covered" />, '基准协议', observed('protocols', total), '全部协议', 'info'],
      [<SafetyCertificateOutlined key="dominant" />, '主协议', Number(dominantProtocol?.key ?? 0), `${protocolName(dominantProtocol?.key ?? '')} · 占比 ${Math.round((dominantProtocol?.share ?? 0) * 100)}%`, 'warning'],
      [<ExperimentOutlined key="new" />, '新协议', counts.drift, '首次观测', 'purple'],
      [<AlertOutlined key="abnormal" />, '异常协议', counts.alerts, '偏离阈值', 'danger'],
      [<AreaChartOutlined key="share" />, '占比漂移', Math.round((dominantProtocol?.share ?? 0) * 100), '主协议会话占比 %', 'cyan'],
      [<ProfileOutlined key="unknown" />, '未识别协议', observed('unknown_protocols'), 'IANA 编号映射', 'warning'],
      [<SafetyCertificateOutlined key="high" />, '高危协议', counts.alerts, '超过告警阈值', 'danger'],
      [<AlertOutlined key="related" />, '关联告警', counts.alerts, '24h 新增', 'warning'],
    ],
    time: [
      [<ClockCircleOutlined key="covered" />, '基准时段', total, '全部时段', 'info'],
      [<SafetyCertificateOutlined key="workday" />, '工作日稳定', stable, '工作日聚合', 'cyan'],
      [<ClockCircleOutlined key="night" />, '夜间异常', observed('night_sessions'), '00:00-06:00', 'purple'],
      [<AreaChartOutlined key="weekend" />, '周末访问', observed('weekend_sessions'), '周六 / 周日', 'warning'],
      [<ExperimentOutlined key="periodic" />, '周期连接', observed('active_hours'), '周期候选时段', 'info'],
      [<ThunderboltOutlined key="maintenance" />, '维护窗口', 0, '维护日历未接入', 'cyan'],
      [<AlertOutlined key="high" />, '高危时段', counts.alerts, '超过告警阈值', 'danger'],
      [<AlertOutlined key="related" />, '关联告警', counts.alerts, '24h 新增', 'warning'],
    ],
  } as Record<BaselineTabType, Array<[ReactNode, string, number, string, string]>>;
  return base[type].map(([icon, label, value, detail, tone]) => ({ icon, label, value, detail, tone }));
}

function stateMachineCounts(items: BehaviorBaseline[]) {
  return {
    learning: items.filter((item) => item.status === 'learning').length,
    active: items.filter((item) => item.status === 'active').length,
    drift: items.filter((item) => item.status === 'drift' || item.drift_watch).length,
    rebuild: 0,
    frozen: items.filter((item) => item.status === 'frozen' || item.frozen).length,
  };
}

function strongestMetric(item?: BehaviorBaseline) {
  return item?.metrics.reduce<BehaviorMetric | undefined>((best, metric) => !best || (metric.deviation_score ?? 0) > (best.deviation_score ?? 0) ? metric : best, undefined);
}

function baselineSampleEnvelope(item: BehaviorBaseline) {
  return item.metrics.reduce((largest, metric) => Math.max(largest, metric.normal_range?.[1] ?? 0), 0);
}

function buildChartData(selected?: BehaviorBaseline, analytics?: BehaviorBaselineAnalytics): { boxplots: BaselineBoxplotDatum[]; scatter: BaselineScatterDatum[]; trend: BaselineTrendDatum } {
  const boxplots = (analytics?.distributions ?? []).map((distribution) => ({ label: metricLabel(distribution.metric_name), values: distribution.values, unit: distribution.unit }));
  const primary = selected?.metrics.find((metric) => metric.metric_name === analytics?.metric_name) ?? selected?.metrics[0];
  const mean = primary?.mean ?? 0;
  const std = Math.max(primary?.std_dev ?? 0, 1);
  const scatter = (analytics?.series ?? []).flatMap((point) => point.samples.map((value, index) => {
    const z = Math.abs(value - mean) / std;
    const date = new Date(point.timestamp);
    return { hour: date.getHours() + Math.min(.95, index / Math.max(1, point.samples.length)), value, level: z >= (primary?.threshold_config.alert_multiplier ?? 3) ? 'danger' : z >= (primary?.threshold_config.warning_multiplier ?? 2) ? 'warning' : 'normal' } as BaselineScatterDatum;
  }));
  const series = analytics?.series ?? [];
  return { boxplots, scatter, trend: { labels: series.map((point) => new Date(point.timestamp).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit' })), mean: series.map((point) => point.mean), p50: series.map((point) => point.p50), p95: series.map((point) => point.p95), p99: series.map((point) => point.p99), upper: series.map((point) => point.upper), lower: series.map((point) => point.lower) } };
}

const uniqueVersions = (items: BehaviorBaseline[]) => [...new Set(items.map((item) => item.version))].sort((a, b) => b - a);
const metricLabel = (value: string) => ({ bytes_per_session: '会话流量', packets_per_session: '报文数', duration_ms: '会话时长', events_per_window: '事件频次', source_ip_count: '来源地址', resource_count: '访问资源', login_hour: '登录时段' }[value] ?? value);

function preferredBaseline(type: BaselineTabType, items: BehaviorBaseline[]) {
  const preferred = type === 'port' ? ['443', '80', '53', '22']
    : type === 'protocol' ? ['17', '6', '1']
      : type === 'time' ? ['2', '3', '0']
        : [];
  return preferred.map((entityId) => items.find((item) => item.entity_id === entityId)).find(Boolean);
}
const protocolName = (protocol: string) => ({ '0': 'HOPOPT', '1': 'ICMP', '2': 'IGMP', '6': 'TCP', '17': 'UDP', '41': 'IPv6', '47': 'GRE', '50': 'ESP', '58': 'ICMPv6' }[protocol] ?? `IP-${protocol || '未知'}`);
const portService = (port: string) => ({ '22': 'SSH', '53': 'DNS', '80': 'HTTP', '123': 'NTP', '443': 'HTTPS', '3306': 'MySQL', '5432': 'PostgreSQL', '6379': 'Redis', '9200': 'OpenSearch' }[port] ?? '未知服务');
const formatMetric = (value?: number, unit?: string) => value === undefined || !Number.isFinite(value) ? '-' : `${value >= 1000 ? value.toLocaleString(undefined, { maximumFractionDigits: 1 }) : value.toFixed(value < 10 ? 2 : 1)} ${unit ?? ''}`.trim();
const formatDate = (value: number) => value ? new Date(value).toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' }) : '-';
const statusLabel = (value: string) => ({ active: '稳定', learning: '学习中', frozen: '已冻结', drift: '漂移观察' }[value] ?? value);
const actionLabel = (value: string) => ({ create_alert: '创建告警', adjust_threshold: '调整阈值', freeze: '冻结', unfreeze: '解除冻结', feedback_model: '反馈模型', cold_start: '冷启动', drift_watch: '漂移观察', rebuild: '重建', rollback: '回滚', audit_trace: '审计留痕' }[value] ?? value);
const formatTimestamp = (value: number) => value ? new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-';
const shortId = (value: string) => value.length > 15 ? `${value.slice(0, 12)}…` : value;
