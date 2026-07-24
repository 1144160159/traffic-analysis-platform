import {
  AuditOutlined,
  BellOutlined,
  CalendarOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  ControlOutlined,
  FileTextOutlined,
  MailOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SendOutlined,
  SettingOutlined,
  SlackOutlined,
  TeamOutlined,
  ToolOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Select, Space, Switch, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import axios from 'axios';
import type { ChangeEvent, ReactNode } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  createNotificationEscalationPolicy,
  createNotificationSilenceRule,
  createNotificationTemplate,
  fetchNotificationAudits,
  fetchNotificationWorkbench,
  patchNotificationEscalationPolicy,
  patchNotificationRule,
  patchNotificationSilenceRule,
  patchNotificationTemplate,
  retryNotificationDelivery,
  testNotificationChannel,
  testNotificationTemplate,
  updateNotificationSettings,
  type NotificationAuditEvent,
  type NotificationChannelKey,
  type NotificationDelivery,
  type NotificationEscalationPolicy,
  type NotificationRule,
  type NotificationSilenceRule,
  type NotificationTemplate,
  type NotificationWorkbench,
} from '@/services/notificationGovernanceApi';
import type { PageSnapshot } from '@/services/mockData';

const channelDefinitions: Array<{ key: NotificationChannelKey; name: string; icon: ReactNode }> = [
  { key: 'email', name: '邮件', icon: <MailOutlined /> },
  { key: 'webhook', name: 'Webhook', icon: <ControlOutlined /> },
  { key: 'wechat', name: '企业微信', icon: <BellOutlined /> },
  { key: 'dingtalk', name: '钉钉', icon: <SendOutlined /> },
  { key: 'slack', name: 'Slack', icon: <SlackOutlined /> },
  { key: 'feishu', name: '飞书', icon: <TeamOutlined /> },
];

const channelOptions = channelDefinitions;

const channelLabels = Object.fromEntries(channelOptions.map((channel) => [channel.key, channel.name])) as Record<string, string>;

type DrawerMode =
  | 'channel-create'
  | 'channel-test'
  | 'rule-detail'
  | 'escalation-create'
  | 'escalation-edit'
  | 'template-create'
  | 'template-edit'
  | 'template-preview'
  | 'delivery-detail'
  | 'silence-create'
  | 'silence-edit'
  | 'audit';

type DrawerState = {
  mode: DrawerMode;
  title: string;
  values: Record<string, unknown>;
};

type MutationTask = { label: string; run: () => Promise<unknown>; closeOnSuccess?: boolean };

export function NotificationConfigPage({ route }: { route: NavRoute }) {
  const queryClient = useQueryClient();
  const calendarInputRef = useRef<HTMLInputElement>(null);
  const [selectedRuleID, setSelectedRuleID] = useState('');
  const [listPage, setListPage] = useState(1);
  const [ruleConditions, setRuleConditions] = useState<Record<string, unknown>>({});
  const [ruleChannels, setRuleChannels] = useState<NotificationChannelKey[]>([]);
  const [drawer, setDrawer] = useState<DrawerState>();
  const [actionResult, setActionResult] = useState<string>();

  const workbenchQuery = useQuery({ queryKey: ['notification-workbench'], queryFn: fetchNotificationWorkbench });
  const auditQuery = useQuery({ queryKey: ['notification-audits'], queryFn: fetchNotificationAudits, enabled: drawer?.mode === 'audit' });
  const workbench = workbenchQuery.data;
  const selectedRule = useMemo(
    () => workbench?.rules.find((rule) => rule.rule_id === selectedRuleID) ?? workbench?.rules[0],
    [selectedRuleID, workbench?.rules],
  );

  useEffect(() => {
    if (!selectedRuleID && workbench?.rules[0]) setSelectedRuleID(workbench.rules[0].rule_id);
  }, [selectedRuleID, workbench?.rules]);

  useEffect(() => {
    setRuleConditions(selectedRule?.conditions ?? {});
    setRuleChannels(selectedRule?.channels ?? []);
  }, [selectedRule?.rule_id, selectedRule?.updated_at]);

  const mutation = useMutation({
    mutationFn: async (task: MutationTask) => task.run(),
    onMutate: (task) => setActionResult(`${task.label}处理中…`),
    onSuccess: async (_data, task) => {
      await queryClient.invalidateQueries({ queryKey: ['notification-workbench'] });
      await queryClient.invalidateQueries({ queryKey: ['notification-audits'] });
      setActionResult(`${task.label}成功，服务端数据与审计记录已刷新。`);
      if (task.closeOnSuccess !== false) setDrawer(undefined);
    },
    onError: async (error: Error, task) => {
      await queryClient.invalidateQueries({ queryKey: ['notification-workbench'] });
      await queryClient.invalidateQueries({ queryKey: ['notification-audits'] });
      const responseMessage = axios.isAxiosError(error) && typeof error.response?.data?.message === 'string'
        ? error.response.data.message
        : error.message;
      setActionResult(`${task.label}失败：${responseMessage}`);
    },
  });

  const run = (task: MutationTask) => mutation.mutate(task);
  const rules = workbench?.rules ?? [];
  const pageSize = 6;
  const pageCount = Math.max(1, Math.ceil(rules.length / pageSize));
  const visibleRules = rules.slice((listPage - 1) * pageSize, listPage * pageSize);
  const metrics = buildNotificationMetrics(route.page.kpis, workbench);
  const ruleColumns = buildRuleColumns({
    selectedRuleID: selectedRule?.rule_id,
    onSelect: (rule) => setSelectedRuleID(rule.rule_id),
    onToggle: (rule) => run({ label: `${rule.enabled ? '停用' : '启用'}订阅规则`, run: () => patchNotificationRule(rule.rule_id, { enabled: !rule.enabled }) }),
    onDetail: (rule) => setDrawer({ mode: 'rule-detail', title: '订阅规则详情', values: { rule } }),
  });

  const saveSelectedRule = () => {
    if (!selectedRule) {
      setActionResult('没有可保存的订阅规则。');
      return;
    }
    run({
      label: '保存订阅策略',
      run: () => patchNotificationRule(selectedRule.rule_id, { conditions: ruleConditions, channels: ruleChannels }),
    });
  };

  const updateChannel = (channel: NotificationChannelKey, enabled: boolean) => {
    if (!workbench) return;
    run({
      label: `${enabled ? '启用' : '停用'}${channelLabels[channel]}`,
      run: () => updateNotificationSettings({ channels: { ...workbench.settings.channels, [channel]: enabled } }),
    });
  };

  const submitDrawer = () => {
    if (!drawer) return;
    const value = (key: string) => String(drawer.values[key] ?? '').trim();
    const enabled = drawer.values.enabled !== false;
    if (drawer.mode === 'channel-create') {
      const channel = value('channel') as NotificationChannelKey;
      if (!channel || !workbench) return setActionResult('请选择需要启用的通知渠道。');
      run({ label: `启用${channelLabels[channel]}`, run: () => updateNotificationSettings({ channels: { ...workbench.settings.channels, [channel]: true } }) });
      return;
    }
    if (drawer.mode === 'channel-test') {
      const channel = (value('channel') || 'email') as NotificationChannelKey;
      run({ label: `测试${channelLabels[channel]}`, run: () => testNotificationChannel(channel, value('target') || '安全值班组', value('alert_type') || 'scan') });
      return;
    }
    if (drawer.mode === 'escalation-create') {
      const name = value('name');
      if (!name) return setActionResult('升级策略名称不能为空。');
      const stages = parseEscalationStages(value('stages_json'));
      if (!stages) return setActionResult('升级阶段 JSON 必须是包含 after_minutes 与 target_role 的数组。');
      run({ label: '新建升级策略', run: () => createNotificationEscalationPolicy({ name, stages, enabled }) });
      return;
    }
    if (drawer.mode === 'escalation-edit') {
      const policy = drawer.values.policy as NotificationEscalationPolicy;
      const stages = parseEscalationStages(value('stages_json'));
      if (!stages) return setActionResult('升级阶段 JSON 必须是包含 after_minutes 与 target_role 的数组。');
      run({ label: '更新升级策略', run: () => patchNotificationEscalationPolicy(policy.policy_id, { name: value('name') || policy.name, stages, enabled }) });
      return;
    }
    if (drawer.mode === 'template-create') {
      if (!value('name') || !value('body')) return setActionResult('模板名称和正文不能为空。');
      run({ label: '新建通知模板', run: () => createNotificationTemplate({ template_type: value('template_type') || '告警模板', name: value('name'), subject: value('subject'), body: value('body'), variable_schema: {}, enabled }) });
      return;
    }
    if (drawer.mode === 'template-edit') {
      const template = drawer.values.template as NotificationTemplate;
      run({ label: '更新通知模板', run: () => patchNotificationTemplate(template.template_id, { template_type: value('template_type') || template.template_type, name: value('name') || template.name, subject: value('subject'), body: value('body'), enabled }) });
      return;
    }
    if (drawer.mode === 'silence-create') {
      const startsAt = value('starts_at');
      const endsAt = value('ends_at');
      if (!value('name') || !startsAt || !endsAt) return setActionResult('静默窗口名称、开始和结束时间不能为空。');
      run({ label: '新建静默窗口', run: () => createNotificationSilenceRule({ name: value('name'), scope: value('scope') || '主园区', starts_at: new Date(startsAt).toISOString(), ends_at: new Date(endsAt).toISOString(), affected_targets: value('affected_targets').split('/').map((item) => item.trim()).filter(Boolean), policy: value('policy') || '全部策略', reason: value('reason') || '计划维护', enabled }) });
      return;
    }
    if (drawer.mode === 'silence-edit') {
      const silence = drawer.values.silence as NotificationSilenceRule;
      const startsAt = value('starts_at');
      const endsAt = value('ends_at');
      if (!value('name') || !startsAt || !endsAt) return setActionResult('静默窗口名称、开始和结束时间不能为空。');
      run({ label: '更新静默窗口', run: () => patchNotificationSilenceRule(silence.rule_id, { name: value('name'), scope: value('scope'), starts_at: new Date(startsAt).toISOString(), ends_at: new Date(endsAt).toISOString(), affected_targets: value('affected_targets').split('/').map((item) => item.trim()).filter(Boolean), policy: value('policy'), reason: value('reason'), enabled }) });
    }
  };

  const onCalendarFile = async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file) return;
    try {
      const calendar = parseCalendarEvent(await file.text());
      run({ label: '导入维护日历', run: () => createNotificationSilenceRule(calendar) });
    } catch (error) {
      setActionResult(`导入维护日历失败：${error instanceof Error ? error.message : String(error)}`);
    }
  };

  return (
    <div className="taf-page taf-notifications" data-testid="notification-governance-page">
      <section className="taf-notifications-shell">
        <main className="taf-notifications-main">
          <header className="taf-notifications-titlebar">
            <h1>{route.page.title}</h1>
            <Space size={8} wrap>
              <Button size="small" type="primary" icon={<PlusOutlined />} onClick={() => setDrawer({ mode: 'channel-create', title: '新增通知渠道', values: { channel: 'email' } })}>新增渠道</Button>
              <Button size="small" icon={<SendOutlined />} onClick={() => setDrawer({ mode: 'channel-test', title: '测试发送', values: { channel: 'email', target: '安全值班组', alert_type: 'scan' } })}>测试发送</Button>
              <Button size="small" icon={<CheckCircleOutlined />} disabled={!selectedRule || mutation.isPending} onClick={saveSelectedRule}>保存订阅策略</Button>
              <Button size="small" icon={<ClockCircleOutlined />} onClick={() => setDrawer({ mode: 'escalation-create', title: '新建升级策略', values: { name: '', stages_json: JSON.stringify(defaultEscalationStages, null, 2), enabled: true } })}>新建升级策略</Button>
              <Button size="small" icon={<CalendarOutlined />} onClick={() => setDrawer(newSilenceDrawer())}>静默窗口</Button>
              <Button size="small" icon={<AuditOutlined />} onClick={() => setDrawer({ mode: 'audit', title: '通知审计记录', values: {} })}>查看审计</Button>
            </Space>
          </header>

          {workbenchQuery.isError && <Alert type="error" showIcon message="真实通知工作台加载失败" description={workbenchQuery.error.message} action={<Button size="small" danger onClick={() => void workbenchQuery.refetch()}>重试</Button>} />}
          {actionResult && <Alert className="taf-notifications-action-result" type={actionResult.includes('失败') ? 'error' : actionResult.includes('处理中') ? 'info' : 'success'} showIcon closable message={actionResult} onClose={() => setActionResult(undefined)} />}

          <div className="taf-notifications-kpis">{metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}</div>

          <div className="taf-notifications-workbench">
            <WorkPanel title="A. 通知渠道健康" className="taf-notifications-channel-panel" extra={<MailOutlined />}>
              <ChannelHealth workbench={workbench} pending={mutation.isPending} onToggle={updateChannel} onTest={(channel) => setDrawer({ mode: 'channel-test', title: `测试${channelLabels[channel]}`, values: { channel, target: '安全值班组', alert_type: 'scan' } })} />
            </WorkPanel>

            <WorkPanel title="B. 订阅规则" className="taf-notifications-rules-panel" extra={<SettingOutlined />}>
              <Table rowKey="rule_id" size="small" loading={workbenchQuery.isLoading} pagination={false} scroll={{ x: 1040, y: 180 }} columns={ruleColumns} dataSource={visibleRules} rowSelection={{ selectedRowKeys: selectedRule ? [selectedRule.rule_id] : [], onChange: (keys) => setSelectedRuleID(String(keys[0] ?? '')) }} onRow={(record) => ({ onClick: () => setSelectedRuleID(record.rule_id) })} />
              <div className="taf-notifications-pagination"><span>共 {rules.length} 条</span><button type="button" aria-label="订阅规则上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{Array.from({ length: pageCount }, (_, index) => index + 1).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-label={`订阅规则第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="订阅规则下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
            </WorkPanel>

            <WorkPanel title="C. 条件构造器" className="taf-notifications-builder-panel" extra={<ToolOutlined />}>
              <ConditionBuilder selected={selectedRule} conditions={ruleConditions} channels={ruleChannels} onChange={setRuleConditions} onChannelsChange={setRuleChannels} onSave={saveSelectedRule} pending={mutation.isPending} />
            </WorkPanel>

            <WorkPanel title="D. 升级策略流程" className="taf-notifications-escalation-panel" extra={<WarningOutlined />}>
              <EscalationFlow policy={workbench?.escalation_policies[0]} pending={mutation.isPending} onToggle={(policy, enabled) => run({ label: `${enabled ? '启用' : '停用'}升级策略`, run: () => patchNotificationEscalationPolicy(policy.policy_id, { enabled }) })} onEdit={(policy) => setDrawer({ mode: 'escalation-edit', title: '编辑升级策略', values: { policy, name: policy.name, stages_json: JSON.stringify(policy.stages, null, 2), enabled: policy.enabled } })} />
            </WorkPanel>

            <WorkPanel title="E. 模板管理" className="taf-notifications-templates-panel" extra={<Space size={6}><Button size="small" type="primary" onClick={() => setDrawer({ mode: 'template-create', title: '新建通知模板', values: { template_type: '告警模板', enabled: true } })}>新建模板</Button><Button size="small" aria-label="刷新通知配置" icon={<ReloadOutlined />} loading={workbenchQuery.isFetching} onClick={() => void workbenchQuery.refetch()} /></Space>}>
              <TemplateManager templates={workbench?.templates ?? []} pending={mutation.isPending} onEdit={(template) => setDrawer(templateDrawer('template-edit', '编辑通知模板', template))} onPreview={(template) => setDrawer(templateDrawer('template-preview', '预览通知模板', template))} onTest={(template) => run({ label: `测试模板 ${template.name}`, run: () => testNotificationTemplate(template.template_id) })} />
            </WorkPanel>

            <WorkPanel title="F. 发送历史" className="taf-notifications-history-panel">
              <SendHistory deliveries={workbench?.deliveries ?? []} pending={mutation.isPending} onDetail={(delivery) => setDrawer({ mode: 'delivery-detail', title: '通知投递详情', values: { delivery } })} onRetry={(delivery) => run({ label: `重试通知 ${delivery.notification_id}`, run: () => retryNotificationDelivery(delivery.notification_id) })} />
            </WorkPanel>

            <WorkPanel title="G. 抑制与静默" className="taf-notifications-silence-panel" extra={<Space size={6}><Button size="small" type="primary" onClick={() => setDrawer(newSilenceDrawer())}>新建维护窗口</Button><Button size="small" onClick={() => calendarInputRef.current?.click()}>导入日历</Button><input ref={calendarInputRef} hidden type="file" accept=".ics,text/calendar" onChange={(event) => void onCalendarFile(event)} /></Space>}>
              <SilenceWindows silences={workbench?.silence_rules ?? []} pending={mutation.isPending} onEdit={(silence) => setDrawer(silenceDrawer(silence))} onToggle={(silence) => run({ label: `${silence.enabled ? '停用' : '启用'}静默窗口`, run: () => patchNotificationSilenceRule(silence.rule_id, { enabled: !silence.enabled }) })} />
            </WorkPanel>
          </div>
        </main>
      </section>
      <NotificationActionDrawer drawer={drawer} pending={mutation.isPending} auditEvents={auditQuery.data ?? []} auditLoading={auditQuery.isLoading} onChange={(key, value) => setDrawer((current) => current ? { ...current, values: { ...current.values, [key]: value } } : current)} onSubmit={submitDrawer} onClose={() => setDrawer(undefined)} />
    </div>
  );
}

function ChannelHealth({ workbench, pending, onToggle, onTest }: { workbench?: NotificationWorkbench; pending: boolean; onToggle: (channel: NotificationChannelKey, enabled: boolean) => void; onTest: (channel: NotificationChannelKey) => void }) {
  return <div className="taf-notifications-channels">{channelDefinitions.map((channel) => {
    const stats = deliveryStats(workbench?.deliveries ?? [], channel.key);
    const enabled = Boolean(workbench?.settings.channels[channel.key]);
    const rateTone = stats.total === 0 ? 'muted' : stats.rate < 95 ? 'danger' : stats.rate < 99 ? 'warning' : 'success';
    return <div key={channel.key} className="taf-notifications-channel-card" data-channel={channel.key}><header><i>{channel.icon}</i><b>{channel.name}</b><Switch aria-label={`${channel.name}渠道开关`} size="small" checked={enabled} loading={pending} onChange={(checked) => onToggle(channel.key, checked)} /></header><span>成功率 <strong data-tone={rateTone}>{stats.rate.toFixed(2)}%</strong></span><span>投递数 <strong data-tone="info">{stats.total}</strong></span><span>失败数 <strong data-tone={stats.failed > 0 ? 'danger' : 'success'}>{stats.failed}</strong></span><div className="taf-notifications-channel-echart"><DataQualityKpiSparklineChart ariaLabel={`${channel.name}送达趋势`} tone={stats.failed ? 'warn' : 'ok'} values={enabled ? stats.trend : [0]} /></div><footer><button type="button" disabled={pending || !enabled} onClick={() => onTest(channel.key)}>测试发送</button></footer></div>;
  })}</div>;
}

function ConditionBuilder({ selected, conditions, channels, onChange, onChannelsChange, onSave, pending }: { selected?: NotificationRule; conditions: Record<string, unknown>; channels: NotificationChannelKey[]; onChange: (next: Record<string, unknown>) => void; onChannelsChange: (next: NotificationChannelKey[]) => void; onSave: () => void; pending: boolean }) {
  const update = (key: string, value: unknown) => onChange({ ...conditions, [key]: value });
  return <div className="taf-notifications-builder"><label><span>严重级别</span><Select aria-label="订阅规则严重级别" size="small" value={String(conditions.severity ?? 'high')} options={['critical', 'high', 'medium', 'low'].map((value) => ({ value, label: severityLabel(value) }))} onChange={(value) => update('severity', value)} /></label><label><span>告警类型</span><Select aria-label="订阅规则告警类型" size="small" value={String(conditions.alert_type ?? '攻击告警')} options={['攻击告警', '数据泄露', '异常登录', '任务失败'].map((value) => ({ value }))} onChange={(value) => update('alert_type', value)} /></label><label><span>资产组</span><Input aria-label="订阅规则资产组" size="small" value={String(conditions.asset_scope ?? '')} onChange={(event) => update('asset_scope', event.target.value)} /></label><label><span>园区</span><Input aria-label="订阅规则园区" size="small" value={String(conditions.campus ?? '')} onChange={(event) => update('campus', event.target.value)} /></label><label><span>时间窗</span><div className="taf-notifications-time"><Input aria-label="订阅规则开始时间" size="small" value={String(conditions.window_start ?? '00:00')} onChange={(event) => update('window_start', event.target.value)} /><Input aria-label="订阅规则结束时间" size="small" value={String(conditions.window_end ?? '23:59')} onChange={(event) => update('window_end', event.target.value)} /></div></label><label><span>接收渠道</span><Select aria-label="订阅规则渠道" mode="multiple" size="small" value={channels} options={channelDefinitions.map((channel) => ({ value: channel.key, label: channel.name }))} onChange={(values) => onChannelsChange(values as NotificationChannelKey[])} /></label><footer><span>{selected?.name ?? '请选择订阅规则'}</span><Button size="small" type="primary" disabled={!selected || channels.length === 0} loading={pending} onClick={onSave}>保存条件</Button></footer></div>;
}

function EscalationFlow({ policy, pending, onToggle, onEdit }: { policy?: NotificationEscalationPolicy; pending: boolean; onToggle: (policy: NotificationEscalationPolicy, enabled: boolean) => void; onEdit: (policy: NotificationEscalationPolicy) => void }) {
  const stages = policy?.stages.length ? policy.stages : defaultEscalationStages;
  return <div className="taf-notifications-escalation"><div className="taf-notifications-policy"><span>策略：{policy?.name ?? '未配置'}</span>{policy && <Switch aria-label="升级策略开关" size="small" checked={policy.enabled} loading={pending} onChange={(checked) => onToggle(policy, checked)} />}</div><div className="taf-notifications-steps">{stages.slice(0, 5).map((stage, index) => <span key={`${stage.condition}-${index}`}><i>{escalationIcon(index)}</i><b>{stage.condition ?? `阶段 ${index + 1}`}</b><em>{stage.after_minutes ?? 0} 分钟</em><small>升级至 {stage.target_role ?? '安全值班组'}</small></span>)}</div><footer><span>阶段与接收角色来自 PostgreSQL 升级策略。</span><button type="button" disabled={!policy || pending} onClick={() => policy && onEdit(policy)}>编辑策略</button></footer></div>;
}

function TemplateManager({ templates, pending, onEdit, onPreview, onTest }: { templates: NotificationTemplate[]; pending: boolean; onEdit: (template: NotificationTemplate) => void; onPreview: (template: NotificationTemplate) => void; onTest: (template: NotificationTemplate) => void }) {
  return <div className="taf-notifications-templates"><div><span>模板类型</span><span>模板名称</span><span>版本</span><span>变量校验</span><span>最近修改</span><span>状态</span><span>操作</span></div>{templates.slice(0, 4).map((template) => <div className="taf-notifications-data-row" key={template.template_id}><span>{template.template_type}</span><span>{template.name}</span><span>v{template.version}</span><span className={template.validation_status === 'passed' ? 'is-ok' : 'is-warn'}>{template.validation_status === 'passed' ? '通过' : '告警(1)'}</span><span>{formatTime(template.updated_at)}</span><span className={template.enabled ? 'is-ok' : 'is-warn'}>{template.enabled ? '启用' : '停用'}</span><em><button type="button" disabled={pending} onClick={() => onEdit(template)}>编辑</button><button type="button" onClick={() => onPreview(template)}>预览</button><button type="button" disabled={pending} onClick={() => onTest(template)}>测试</button></em></div>)}<footer><span>共 {templates.length} 条</span><span>真实模板版本</span></footer></div>;
}

function SendHistory({ deliveries, pending, onDetail, onRetry }: { deliveries: NotificationDelivery[]; pending: boolean; onDetail: (delivery: NotificationDelivery) => void; onRetry: (delivery: NotificationDelivery) => void }) {
  return <div className="taf-notifications-history"><div><span>时间</span><span>通知对象</span><span>渠道</span><span>告警类型</span><span>状态</span><span>失败原因</span><span>重试次数</span><span>通知ID</span><span>操作</span></div>{deliveries.slice(0, 5).map((delivery) => <div className="taf-notifications-data-row" key={delivery.notification_id}><span>{formatTime(delivery.created_at)}</span><span>{delivery.target_name || delivery.alert_id}</span><span>{channelLabels[delivery.channel] ?? delivery.channel}</span><span>{delivery.alert_type || '-'}</span><span className={delivery.status === 'failed' ? 'is-risk' : 'is-ok'}>{delivery.status === 'failed' ? '失败' : '成功'}</span><span>{delivery.error_message || '-'}</span><span>{delivery.retry_count}</span><span>{delivery.notification_id}</span><em><button type="button" onClick={() => onDetail(delivery)}>详情</button><button type="button" disabled={pending || delivery.status !== 'failed'} onClick={() => onRetry(delivery)}>重试</button></em></div>)}</div>;
}

function SilenceWindows({ silences, pending, onEdit, onToggle }: { silences: NotificationSilenceRule[]; pending: boolean; onEdit: (silence: NotificationSilenceRule) => void; onToggle: (silence: NotificationSilenceRule) => void }) {
  return <div className="taf-notifications-silence"><div><span>窗口名称</span><span>园区/范围</span><span>开始时间</span><span>结束时间</span><span>影响范围</span><span>关联策略</span><span>状态</span><span>操作</span></div>{silences.slice(0, 3).map((silence) => <div className="taf-notifications-data-row" key={silence.rule_id}><span>{silence.name}</span><span>{silence.scope}</span><span>{formatTime(silence.starts_at)}</span><span>{formatTime(silence.ends_at)}</span><span>{silence.affected_targets.join(' / ') || '-'}</span><span>{silence.policy}</span><span className={silence.enabled ? 'is-ok' : 'is-warn'}>{silence.enabled ? '启用' : '停用'}</span><em><button type="button" onClick={() => onEdit(silence)}>编辑</button><button type="button" disabled={pending} onClick={() => onToggle(silence)}>{silence.enabled ? '禁用' : '启用'}</button></em></div>)}<footer><span>共 {silences.length} 条</span><span>已写入 audit_logs</span></footer></div>;
}

function NotificationActionDrawer({ drawer, pending, auditEvents, auditLoading, onChange, onSubmit, onClose }: { drawer?: DrawerState; pending: boolean; auditEvents: NotificationAuditEvent[]; auditLoading: boolean; onChange: (key: string, value: unknown) => void; onSubmit: () => void; onClose: () => void }) {
  const readOnly = drawer?.mode === 'rule-detail' || drawer?.mode === 'template-preview' || drawer?.mode === 'delivery-detail' || drawer?.mode === 'audit';
  return <Drawer className="taf-notifications-action-drawer" title={drawer?.title ?? '通知操作'} open={Boolean(drawer)} width="min(560px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={onClose} extra={!readOnly && <Button size="small" type="primary" loading={pending} onClick={onSubmit}>确认提交</Button>}><div className="taf-alert-detail-action-body">{drawer && <DrawerBody drawer={drawer} auditEvents={auditEvents} auditLoading={auditLoading} onChange={onChange} />}</div></Drawer>;
}

function DrawerBody({ drawer, auditEvents, auditLoading, onChange }: { drawer: DrawerState; auditEvents: NotificationAuditEvent[]; auditLoading: boolean; onChange: (key: string, value: unknown) => void }) {
  if (drawer.mode === 'audit') return auditLoading ? <p>正在读取通知审计记录…</p> : <div className="taf-notification-audit-list">{auditEvents.length ? auditEvents.map((event) => <div key={event.log_id}><b>{event.action}</b><span>{event.object_type} / {event.object_id}</span><small>{formatTime(event.timestamp)}</small></div>) : <Alert type="info" showIcon message="当前窗口没有通知审计事件" />}</div>;
  if (drawer.mode === 'rule-detail') return <RecordDetails value={drawer.values.rule} />;
  if (drawer.mode === 'delivery-detail') return <RecordDetails value={drawer.values.delivery} />;
  if (drawer.mode === 'template-preview') { const template = drawer.values.template as NotificationTemplate; return <><dl><dt>模板</dt><dd>{template.name} / v{template.version}</dd><dt>主题</dt><dd>{template.subject}</dd><dt>正文</dt><dd>{template.body}</dd><dt>变量契约</dt><dd><pre>{JSON.stringify(template.variable_schema, null, 2)}</pre></dd></dl></>; }
  if (drawer.mode === 'channel-create' || drawer.mode === 'channel-test') return <><label>渠道<Select aria-label="通知渠道" value={String(drawer.values.channel ?? 'email')} options={(drawer.mode === 'channel-create' ? channelOptions : channelDefinitions).map((channel) => ({ value: channel.key, label: channel.name }))} onChange={(value) => onChange('channel', value)} /></label>{drawer.mode === 'channel-test' && <><label>通知对象<Input aria-label="测试通知对象" value={String(drawer.values.target ?? '')} onChange={(event) => onChange('target', event.target.value)} /></label><label>告警类型<Select aria-label="测试告警类型" value={String(drawer.values.alert_type ?? 'scan')} options={['scan', 'data_quality', 'task_failure', 'compliance_gap'].map((value) => ({ value }))} onChange={(value) => onChange('alert_type', value)} /></label></>}</>;
  if (drawer.mode.startsWith('template-')) return <><label>模板类型<Input aria-label="模板类型" value={String(drawer.values.template_type ?? '')} onChange={(event) => onChange('template_type', event.target.value)} /></label><label>模板名称<Input aria-label="模板名称" value={String(drawer.values.name ?? '')} onChange={(event) => onChange('name', event.target.value)} /></label><label>主题<Input aria-label="模板主题" value={String(drawer.values.subject ?? '')} onChange={(event) => onChange('subject', event.target.value)} /></label><label>正文<Input.TextArea aria-label="模板正文" rows={6} value={String(drawer.values.body ?? '')} onChange={(event) => onChange('body', event.target.value)} /></label><label>启用<Switch checked={drawer.values.enabled !== false} onChange={(checked) => onChange('enabled', checked)} /></label></>;
  if (drawer.mode.startsWith('escalation-')) return <><label>策略名称<Input aria-label="升级策略名称" value={String(drawer.values.name ?? '')} onChange={(event) => onChange('name', event.target.value)} /></label><label>升级阶段 JSON<Input.TextArea aria-label="升级策略阶段" rows={12} value={String(drawer.values.stages_json ?? '')} onChange={(event) => onChange('stages_json', event.target.value)} /></label><label>启用<Switch checked={drawer.values.enabled !== false} onChange={(checked) => onChange('enabled', checked)} /></label><Alert type="info" showIcon message="每个阶段必须包含 after_minutes 与 target_role，可选 condition。" /></>;
  if (drawer.mode.startsWith('silence-')) return <><label>窗口名称<Input aria-label="静默窗口名称" value={String(drawer.values.name ?? '')} onChange={(event) => onChange('name', event.target.value)} /></label><label>园区/范围<Input aria-label="静默窗口范围" value={String(drawer.values.scope ?? '')} onChange={(event) => onChange('scope', event.target.value)} /></label><label>开始时间<Input aria-label="静默窗口开始时间" type="datetime-local" value={String(drawer.values.starts_at ?? '')} onChange={(event) => onChange('starts_at', event.target.value)} /></label><label>结束时间<Input aria-label="静默窗口结束时间" type="datetime-local" value={String(drawer.values.ends_at ?? '')} onChange={(event) => onChange('ends_at', event.target.value)} /></label><label>影响范围<Input aria-label="静默窗口影响范围" value={String(drawer.values.affected_targets ?? '')} onChange={(event) => onChange('affected_targets', event.target.value)} /></label><label>关联策略<Input aria-label="静默窗口关联策略" value={String(drawer.values.policy ?? '')} onChange={(event) => onChange('policy', event.target.value)} /></label><label>原因<Input aria-label="静默窗口原因" value={String(drawer.values.reason ?? '')} onChange={(event) => onChange('reason', event.target.value)} /></label><label>启用<Switch checked={drawer.values.enabled !== false} onChange={(checked) => onChange('enabled', checked)} /></label></>;
  return null;
}

const buildRuleColumns = ({ selectedRuleID, onSelect, onToggle, onDetail }: { selectedRuleID?: string; onSelect: (rule: NotificationRule) => void; onToggle: (rule: NotificationRule) => void; onDetail: (rule: NotificationRule) => void }): ColumnsType<NotificationRule> => [
  { title: '规则', dataIndex: 'name', key: 'name', width: 170, ellipsis: true },
  { title: '严重级别', key: 'severity', width: 86, render: (_, rule) => <StatusTag value={severityLabel(String(rule.conditions.severity ?? 'medium'))} /> },
  { title: '告警类型', key: 'alert_type', width: 95, ellipsis: true, render: (_, rule) => String(rule.conditions.alert_type ?? '-') },
  { title: '资产组/园区', key: 'scope', width: 130, ellipsis: true, render: (_, rule) => `${String(rule.conditions.asset_scope ?? '-')} / ${String(rule.conditions.campus ?? '-')}` },
  { title: '时间窗', key: 'window', width: 108, render: (_, rule) => `${String(rule.conditions.window_start ?? '00:00')}-${String(rule.conditions.window_end ?? '23:59')}` },
  { title: '渠道', key: 'channels', width: 110, ellipsis: true, render: (_, rule) => rule.channels.map((channel) => channelLabels[channel] ?? channel).join(' / ') },
  { title: '升级策略', key: 'policy', width: 120, ellipsis: true, render: (_, rule) => String(rule.conditions.escalation_policy ?? '-') },
  { title: '静默', key: 'silence', width: 90, ellipsis: true, render: (_, rule) => String(rule.conditions.silence_mode ?? '-') },
  { title: '状态', key: 'enabled', width: 58, render: (_, rule) => <StatusTag value={rule.enabled ? '启用' : '停用'} /> },
  { title: '操作', key: 'actions', fixed: 'right', width: 126, render: (_, rule) => <span className="taf-notifications-row-actions"><button type="button" aria-pressed={rule.rule_id === selectedRuleID} onClick={(event) => { event.stopPropagation(); onSelect(rule); }}>编辑</button><button type="button" onClick={(event) => { event.stopPropagation(); onToggle(rule); }}>{rule.enabled ? '停用' : '启用'}</button><button type="button" onClick={(event) => { event.stopPropagation(); onDetail(rule); }}>详情</button></span> },
];

const buildNotificationMetrics = (labels: string[], workbench?: NotificationWorkbench): PageSnapshot['metrics'] => {
  const deliveries = workbench?.deliveries ?? [];
  const enabledChannels = channelDefinitions.filter((channel) => workbench?.settings.channels[channel.key]).length;
  const failed = deliveries.filter((delivery) => delivery.status === 'failed').length;
  const pending = deliveries.filter((delivery) => ['pending', 'queued', 'accepted'].includes(delivery.status)).length;
  const values = [enabledChannels, workbench?.rules.length ?? 0, pending, failed, workbench?.escalation_policies.length ?? 0, workbench?.silence_rules.length ?? 0];
  return labels.map((label, index) => ({ label, value: `${values[index] ?? 0}${index === 0 || index === 5 ? ' 个' : ' 条'}`, delta: workbench ? 'PostgreSQL' : '加载中', status: index === 3 && failed ? 'risk' : index === 2 && pending ? 'warn' : 'ok' }));
};

const deliveryStats = (deliveries: NotificationDelivery[], channel: NotificationChannelKey) => {
  const matched = deliveries.filter((delivery) => delivery.channel === channel).slice().reverse();
  let successful = 0;
  const trend = matched.map((delivery, index) => {
    if (delivery.status === 'sent') successful += 1;
    return (successful / (index + 1)) * 100;
  });
  const failed = matched.filter((delivery) => delivery.status === 'failed').length;
  const rate = matched.length ? (successful / matched.length) * 100 : 100;
  return { total: matched.length, failed, rate, trend: trend.length ? trend : [0] };
};
const severityLabel = (value: string) => ({ critical: '严重', high: '高危', medium: '中危', low: '低危' }[value] ?? value);
const formatTime = (value: string | number) => { const time = typeof value === 'number' && value < 10_000_000_000 ? value * 1000 : value; const date = new Date(time); return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString('zh-CN', { hour12: false }).replace(/\//g, '-'); };
const escalationIcon = (index: number) => [<ClockCircleOutlined key="sla" />, <BellOutlined key="unack" />, <SafetyCertificateOutlined key="fail" />, <WarningOutlined key="repeat" />, <FileTextOutlined key="gap" />][index] ?? <BellOutlined />;

const defaultEscalationStages = [
  { after_minutes: 15, condition: 'SLA 超时', target_role: '安全值班组' },
  { after_minutes: 30, condition: '未确认', target_role: '安全值班组' },
  { after_minutes: 30, condition: '处置失败', target_role: '安全管理组' },
  { after_minutes: 30, condition: '重复告警', target_role: '运维管理组' },
  { after_minutes: 1440, condition: '验收缺口', target_role: '审计管理组' },
];

const templateDrawer = (mode: 'template-edit' | 'template-preview', title: string, template: NotificationTemplate): DrawerState => ({ mode, title, values: { template, template_type: template.template_type, name: template.name, subject: template.subject, body: template.body, enabled: template.enabled } });
const newSilenceDrawer = (): DrawerState => { const start = new Date(Date.now() + 86_400_000); const end = new Date(start.getTime() + 4 * 3_600_000); const local = (date: Date) => new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16); return { mode: 'silence-create', title: '新建维护窗口', values: { name: '', scope: '主园区', starts_at: local(start), ends_at: local(end), affected_targets: '核心交换机 / 网络设备', policy: '夜间升级策略', reason: '计划维护', enabled: true } }; };
const silenceDrawer = (silence: NotificationSilenceRule): DrawerState => { const local = (value: string) => { const date = new Date(value); return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16); }; return { mode: 'silence-edit', title: '编辑静默窗口', values: { silence, name: silence.name, scope: silence.scope, starts_at: local(silence.starts_at), ends_at: local(silence.ends_at), affected_targets: silence.affected_targets.join(' / '), policy: silence.policy, reason: silence.reason, enabled: silence.enabled } }; };

const parseEscalationStages = (value: string): NotificationEscalationPolicy['stages'] | undefined => {
  try {
    const parsed = JSON.parse(value) as NotificationEscalationPolicy['stages'];
    if (!Array.isArray(parsed) || parsed.length === 0 || parsed.some((stage) => typeof stage.after_minutes !== 'number' || !String(stage.target_role ?? '').trim())) return undefined;
    return parsed;
  } catch {
    return undefined;
  }
};

const parseCalendarEvent = (content: string): Pick<NotificationSilenceRule, 'name' | 'scope' | 'starts_at' | 'ends_at' | 'affected_targets' | 'policy' | 'reason'> => {
  const property = (name: string) => content.match(new RegExp(`^${name}(?:;[^:]*)?:(.+)$`, 'mi'))?.[1]?.trim() ?? '';
  const parseDate = (value: string) => { const match = value.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z?$/); if (!match) throw new Error(`${value || '空值'} 不是支持的 ICS 时间`); const [, year, month, day, hour, minute, second] = match; return new Date(`${year}-${month}-${day}T${hour}:${minute}:${second}Z`).toISOString(); };
  const name = property('SUMMARY');
  if (!name) throw new Error('ICS 缺少 SUMMARY');
  return { name, scope: property('LOCATION') || '主园区', starts_at: parseDate(property('DTSTART')), ends_at: parseDate(property('DTEND')), affected_targets: ['日历导入范围'], policy: '全部策略', reason: property('DESCRIPTION') || '维护日历导入' };
};

function RecordDetails({ value }: { value: unknown }) { return <dl className="taf-notification-record-details">{Object.entries((value ?? {}) as Record<string, unknown>).map(([key, item]) => <div key={key}><dt>{key}</dt><dd>{typeof item === 'object' ? <pre>{JSON.stringify(item, null, 2)}</pre> : String(item ?? '-')}</dd></div>)}</dl>; }
