import {
  AuditOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  DownloadOutlined,
  EditOutlined,
  HistoryOutlined,
  LockOutlined,
  PlayCircleOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SaveOutlined,
  SearchOutlined,
  SettingOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { isPlaybookRollbackEvidence } from '../utils/playbookAudit';
import { Alert, Button, Empty, Form, Input, InputNumber, Modal, Select, Space, Table, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { ReactNode } from 'react';
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { PlaybookFlowConnectionsChart, type PlaybookFlowGeometry } from '@/components/charts';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  downloadPlaybookEvidence,
  drillPlaybook,
  fetchPlaybookCatalog,
  fetchPlaybookWorkbench,
  newPlaybookDraft,
  rollbackPlaybookDrill,
  savePlaybookDraft,
  setPlaybookEnabled,
  transitionPlaybook,
  type PlaybookAction,
  type PlaybookAuditRecord,
  type PlaybookDefinition,
  type PlaybookDefinitionRecord,
  type PlaybookExecutionRecord,
} from '@/services/playbookAutomationApi';
import type { PageSnapshot } from '@/services/mockData';

type EditorValues = {
  name: string;
  displayName: string;
  description: string;
  alertType: string;
  severity: string;
  score: number;
  maxRuns: number;
  cooldownMinutes: number;
  actionTypes: string[];
};

type FlowNode = { id: string; title: string; caption: string; icon: ReactNode; tone: string };

export function PlaybookAutomationPage({ route }: { route: NavRoute }) {
  const queryClient = useQueryClient();
  const [form] = Form.useForm<EditorValues>();
  const [messageApi, messageContext] = message.useMessage();
  const [selectedName, setSelectedName] = useState('');
  const [search, setSearch] = useState('');
  const [alertFilter, setAlertFilter] = useState('all');
  const [riskFilter, setRiskFilter] = useState('all');
  const [editorOpen, setEditorOpen] = useState(false);
  const [editing, setEditing] = useState<PlaybookDefinitionRecord>();
  const [rollbackExecution, setRollbackExecution] = useState<PlaybookExecutionRecord>();
  const [rollbackReason, setRollbackReason] = useState('演练验证完成，记录回滚证据');

  const catalogQuery = useQuery({ queryKey: ['playbook-catalog'], queryFn: fetchPlaybookCatalog });
  const catalog = useMemo(() => catalogQuery.data ?? [], [catalogQuery.data]);
  useEffect(() => {
    if (!selectedName && catalog[0]?.name) setSelectedName(catalog[0].name);
    if (selectedName && catalog.length > 0 && !catalog.some((item) => item.name === selectedName)) setSelectedName(catalog[0].name);
  }, [catalog, selectedName]);
  const selected = catalog.find((item) => item.name === selectedName) ?? catalog[0];
  const workbenchQuery = useQuery({
    queryKey: ['playbook-workbench', selected?.name],
    queryFn: () => fetchPlaybookWorkbench(selected!.name),
    enabled: Boolean(selected?.name),
  });
  const workbench = workbenchQuery.data;
  const filtered = catalog.filter((item) => {
    const needle = search.trim().toLowerCase();
    const matchesSearch = !needle || `${item.display_name} ${item.name} ${item.description}`.toLowerCase().includes(needle);
    const matchesAlert = alertFilter === 'all' || item.definition.trigger.alert_type === alertFilter;
    const matchesRisk = riskFilter === 'all' || item.risk_level === riskFilter;
    return matchesSearch && matchesAlert && matchesRisk;
  });

  const refresh = async (name?: string) => {
    await queryClient.invalidateQueries({ queryKey: ['playbook-catalog'] });
    await queryClient.invalidateQueries({ queryKey: ['playbook-workbench', name ?? selected?.name] });
  };
  const lifecycleMutation = useMutation({
    mutationFn: async (input: { kind: 'submit' | 'approve' | 'reject' | 'drill' | 'enable' | 'disable'; reason?: string }) => {
      if (!selected) throw new Error('请先选择剧本');
      if (input.kind === 'drill') return drillPlaybook(selected.name, selected.version);
	  if (input.kind === 'enable' || input.kind === 'disable') return setPlaybookEnabled(selected.name, input.kind === 'enable', selected.version);
      const action = input.kind === 'submit' ? 'submit-approval' : input.kind;
      return transitionPlaybook(selected.name, action, selected.version, input.reason);
    },
    onSuccess: async (_, input) => {
	  const successMessage = input.kind === 'drill'
	    ? '演练已完成：所有动作均为模拟，未施加外部影响'
	    : input.kind === 'enable' ? '剧本已启用并写入审计'
	      : input.kind === 'disable' ? '剧本已停用并写入审计' : '剧本状态已更新并写入审计';
	  messageApi.success(successMessage);
      await refresh();
    },
    onError: (error) => messageApi.error(errorText(error)),
  });
  const saveMutation = useMutation({
    mutationFn: (values: EditorValues) => {
      const base = editing?.definition ?? newPlaybookDraft();
      const definition = editorDefinition(values, base);
      return savePlaybookDraft({
        name: values.name,
        expectedVersion: editing?.version ?? 0,
        displayName: values.displayName,
        description: values.description,
        definition,
        create: !editing,
      });
    },
    onSuccess: async (record) => {
      setEditorOpen(false);
      setSelectedName(record.name);
      messageApi.success(`草稿 v${record.version} 已保存并写入审计`);
      await refresh(record.name);
    },
    onError: (error) => messageApi.error(errorText(error)),
  });
  const rollbackMutation = useMutation({
    mutationFn: async () => {
      if (!rollbackExecution) throw new Error('没有可回滚的演练记录');
      return rollbackPlaybookDrill(rollbackExecution.execution_id, rollbackReason);
    },
    onSuccess: async () => {
      setRollbackExecution(undefined);
      messageApi.success('演练回滚已记录；该操作未施加外部网络或终端变更');
      await refresh();
    },
    onError: (error) => messageApi.error(errorText(error)),
  });

  const openEditor = (record?: PlaybookDefinitionRecord) => {
    const definition = record?.definition ?? newPlaybookDraft();
    setEditing(record);
    form.setFieldsValue({
      name: definition.name,
      displayName: record?.display_name ?? '新建安全响应剧本',
      description: record?.description ?? definition.description,
      alertType: definition.trigger.alert_type,
      severity: definition.trigger.severity_min,
      score: definition.trigger.score_min,
      maxRuns: definition.max_runs,
      cooldownMinutes: Math.max(1, Math.round(definition.cooldown / 60_000_000_000)),
      actionTypes: definition.actions.map((action) => action.type),
    });
    setEditorOpen(true);
  };
  const latestRollbackCandidate = workbench?.executions.find((item) => item.mode === 'drill' && item.status === 'succeeded' && !item.rolled_back_at);
  const exportEvidence = async () => {
    try {
      const { blob, filename } = await downloadPlaybookEvidence();
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = filename;
      anchor.click();
      URL.revokeObjectURL(url);
      messageApi.success('租户剧本证据包已导出');
    } catch (error) {
      messageApi.error(errorText(error));
    }
  };
  const metrics = buildMetrics(route.page.kpis, catalog, workbench?.executions ?? []);
  const columns: ColumnsType<PlaybookDefinitionRecord> = [
    { title: '剧本名称', dataIndex: 'display_name', ellipsis: true, render: (value) => <span className="taf-playbooks-name"><ThunderboltOutlined />{String(value)}</span> },
    { title: '适用告警', render: (_, record) => record.definition.trigger.alert_type },
    { title: '动作类型', render: (_, record) => <span className="taf-playbooks-action-tags">{record.definition.actions.slice(0, 2).map((item) => <em key={item.type}>{actionLabel(item.type)}</em>)}</span> },
    { title: '风险级别', dataIndex: 'risk_level', render: (value) => <StatusTag value={riskLabel(String(value))} /> },
    { title: '启用状态', render: (_, record) => <StatusTag value={record.stage === 'approval_pending' ? '待审批' : record.enabled ? '已启用' : record.stage === 'rejected' ? '已驳回' : '草稿'} /> },
    { title: '最近执行', render: (_, record) => latestExecutionTime(workbench?.executions, record.name) },
    { title: '操作', width: 60, render: (_, record) => <Button type="text" size="small" icon={<EditOutlined />} onClick={(event) => { event.stopPropagation(); openEditor(record); }} /> },
  ];

  const error = catalogQuery.error ?? workbenchQuery.error;
  return (
    <div className="taf-page taf-playbooks">
      {messageContext}
      <section className="taf-playbooks-shell">
        <main className="taf-playbooks-main">
          <header className="taf-playbooks-titlebar">
            <div><h1>{route.page.title}</h1><span>PostgreSQL 租户剧本 · 两人审批 · 仅演练执行 · 全链路审计</span></div>
            <Space size={6} wrap>
              <Button size="small" type="primary" icon={<ThunderboltOutlined />} onClick={() => openEditor()}>新建剧本</Button>
              <Button size="small" icon={<SaveOutlined />} disabled={!selected || selected.stage === 'approval_pending'} onClick={() => selected && openEditor(selected)}>保存草稿</Button>
              <Button size="small" icon={<AuditOutlined />} disabled={!selected || !['draft', 'rejected'].includes(selected.stage)} loading={lifecycleMutation.isPending} onClick={() => lifecycleMutation.mutate({ kind: 'submit' })}>提交审批</Button>
              {selected?.stage === 'approval_pending' && <Button size="small" icon={<CheckCircleOutlined />} onClick={() => lifecycleMutation.mutate({ kind: 'approve' })}>独立审批</Button>}
              {selected?.stage === 'approval_pending' && <Button size="small" danger icon={<CloseCircleOutlined />} onClick={() => lifecycleMutation.mutate({ kind: 'reject', reason: '审批证据不完整，需要补充后重新提交' })}>驳回</Button>}
			  {selected?.stage === 'approved' && <Button size="small" icon={<PoweroffOutlined />} loading={lifecycleMutation.isPending} onClick={() => lifecycleMutation.mutate({ kind: selected.enabled ? 'disable' : 'enable' })}>{selected.enabled ? '停用' : '启用'}</Button>}
              <Tooltip title="验证动作计划并持久化 simulated 结果，不调用网络、终端或通知提供方"><Button size="small" icon={<PlayCircleOutlined />} disabled={!selected} onClick={() => lifecycleMutation.mutate({ kind: 'drill' })}>执行演练</Button></Tooltip>
              <Button size="small" danger ghost icon={<RollbackOutlined />} disabled={!latestRollbackCandidate} onClick={() => { setRollbackExecution(latestRollbackCandidate); setRollbackReason('演练验证完成，记录回滚证据'); }}>回滚演练</Button>
              <Button size="small" icon={<DownloadOutlined />} onClick={() => void exportEvidence()}>导出审计</Button>
              <Button size="small" icon={<ReloadOutlined />} onClick={() => void refresh()} />
            </Space>
          </header>

          {(catalogQuery.isError || workbenchQuery.isError) && <Alert type="error" showIcon message="真实 API 数据加载失败" description={errorText(error)} action={<Button size="small" danger onClick={() => void refresh()}>重试</Button>} />}
          <div className="taf-playbooks-kpis">{metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}</div>

          <div className="taf-playbooks-workbench">
            <section className="taf-playbooks-left">
              <WorkPanel title="A. 剧本列表" extra={<span>{filtered.length} / {catalog.length} 条</span>}>
                <div className="taf-playbooks-filterbar">
                  <Input size="small" prefix={<SearchOutlined />} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索剧本名称" />
                  <Select size="small" value={alertFilter} onChange={setAlertFilter} options={[{ value: 'all', label: '适用告警' }, ...unique(catalog.map((item) => item.definition.trigger.alert_type)).map((value) => ({ value, label: value }))]} />
                  <Select size="small" value={riskFilter} onChange={setRiskFilter} options={[{ value: 'all', label: '风险级别' }, ...['critical', 'high', 'medium', 'low'].map((value) => ({ value, label: riskLabel(value) }))]} />
                  <Select size="small" value="all" options={[{ value: 'all', label: '全部阶段' }]} />
                  <Button size="small" icon={<SettingOutlined />} />
                </div>
                <Table rowKey="name" size="small" loading={catalogQuery.isLoading} pagination={false} columns={columns} dataSource={filtered} scroll={{ y: 270 }} onRow={(record) => ({ onClick: () => setSelectedName(record.name) })} rowSelection={{ type: 'radio', selectedRowKeys: selected ? [selected.name] : [], onChange: (keys) => setSelectedName(String(keys[0] ?? '')) }} />
              </WorkPanel>
              <WorkPanel title="E. 执行历史"><ExecutionHistory rows={workbench?.executions ?? []} /></WorkPanel>
            </section>

            <section className="taf-playbooks-center">
              <WorkPanel title={`B. 剧本编排：${selected?.display_name ?? '-'}（v${selected?.version ?? '-'} / ${stageLabel(selected?.stage)}）`} extra={<span className="taf-playbooks-canvas-tools"><SearchOutlined />检查节点<SettingOutlined />演练模式</span>}>
                <PlaybookFlow definition={selected?.definition} />
              </WorkPanel>
              <WorkPanel title={`F. 演练效果（${selected?.display_name ?? '-'}）`} extra={<span>外部影响：未施加</span>}><EffectComparison execution={workbench?.executions[0]} /></WorkPanel>
            </section>

            <aside className="taf-playbooks-right">
              <WorkPanel title="C. 节点配置 / 触发策略"><TriggerPolicy definition={selected?.definition} /></WorkPanel>
              <WorkPanel title="D. 风险控制"><RiskControl record={selected} /></WorkPanel>
              <WorkPanel title="G. 审计与证据"><AuditEvidence rows={workbench?.audits ?? []} executionCount={workbench?.executions.length ?? 0} /></WorkPanel>
            </aside>
          </div>
        </main>
      </section>

      <Modal title={editing ? `编辑剧本 · v${editing.version}` : '新建剧本草稿'} open={editorOpen} width={820} onCancel={() => setEditorOpen(false)} onOk={() => void form.validateFields().then((values) => saveMutation.mutate(values))} confirmLoading={saveMutation.isPending} destroyOnClose>
        <Alert type="info" showIcon message="保存会生成新的草稿版本；高风险动作会强制两人审批与可回滚策略。" />
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Space align="start" wrap>
            <Form.Item name="name" label="唯一名称" rules={[{ required: true }, { pattern: /^[a-z0-9]+(?:-[a-z0-9]+)*$/, message: '仅允许小写字母、数字和内部连字符' }]}><Input disabled={Boolean(editing)} style={{ width: 240 }} /></Form.Item>
            <Form.Item name="displayName" label="显示名称" rules={[{ required: true, min: 2, max: 80 }]}><Input style={{ width: 240 }} /></Form.Item>
            <Form.Item name="alertType" label="告警类型" rules={[{ required: true }]}><Select style={{ width: 180 }} options={['scan', 'c2', 'brute_force', 'data_exfil', 'lateral_movement', 'dns_tunnel'].map((value) => ({ value }))} /></Form.Item>
          </Space>
          <Form.Item name="description" label="说明" rules={[{ required: true }]}><Input /></Form.Item>
          <Space align="start" wrap>
            <Form.Item name="severity" label="最低严重级别"><Select style={{ width: 150 }} options={['medium', 'high', 'critical'].map((value) => ({ value }))} /></Form.Item>
            <Form.Item name="score" label="最低置信度"><InputNumber min={0} max={1} step={0.05} style={{ width: 150 }} /></Form.Item>
            <Form.Item name="maxRuns" label="实行动作上限（provider 接入后）"><InputNumber min={0} max={1000} style={{ width: 150 }} /></Form.Item>
            <Form.Item name="cooldownMinutes" label="实行动作冷却（provider 接入后）"><InputNumber min={1} max={10080} style={{ width: 170 }} /></Form.Item>
          </Space>
          <Form.Item name="actionTypes" label="动作节点（演练时全部 simulated）" rules={[{ required: true }]}><Select mode="multiple" options={['block_ip', 'block_domain', 'quarantine', 'capture_pcap', 'rate_limit', 'tag', 'enrich', 'escalate', 'notify'].map((value) => ({ value, label: actionLabel(value) }))} /></Form.Item>
        </Form>
      </Modal>

      <Modal title="记录演练回滚" open={Boolean(rollbackExecution)} onCancel={() => setRollbackExecution(undefined)} onOk={() => rollbackMutation.mutate()} confirmLoading={rollbackMutation.isPending} okButtonProps={{ danger: true }}>
        <Alert type="warning" showIcon message="此操作只回滚演练记录状态，不声称恢复任何外部网络或终端配置。" />
        <Input.TextArea value={rollbackReason} onChange={(event) => setRollbackReason(event.target.value)} rows={3} style={{ marginTop: 12 }} />
      </Modal>
    </div>
  );
}

function PlaybookFlow({ definition }: { definition?: PlaybookDefinition }) {
  if (!definition) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="请选择剧本" />;
  return <PlaybookFlowCanvas definition={definition} />;
}

function PlaybookFlowCanvas({ definition }: { definition: PlaybookDefinition }) {
  const flowRef = useRef<HTMLDivElement>(null);
  const [geometry, setGeometry] = useState<PlaybookFlowGeometry>();
  const actionNodes = definition.actions.slice(0, 4);
  const nodes: FlowNode[] = [
    { id: 'start', title: '开始', caption: definition.trigger.alert_type, icon: <PlayCircleOutlined />, tone: 'ok' },
    { id: 'condition', title: '触发条件', caption: `${definition.trigger.severity_min} / ${definition.trigger.score_min}`, icon: <BranchesOutlined />, tone: 'ok' },
    { id: 'confirm', title: '独立审批', caption: definition.approval_policy.two_person_rule ? '两人规则' : '单人规则', icon: <LockOutlined />, tone: 'warn' },
    ...actionNodes.map((action, index): FlowNode => ({ id: ['isolate', 'block', 'rollback', 'script'][index], title: actionLabel(action.type), caption: '模拟验证', icon: <ThunderboltOutlined />, tone: actionRisk(action.type) ? 'risk' : 'info' })),
    { id: 'end', title: '结束', caption: '写入审计', icon: <CheckCircleOutlined />, tone: 'ok' },
  ];
  const nodeSignature = nodes.map((node) => `${node.id}:${node.tone}`).join('|');

  useLayoutEffect(() => {
    const root = flowRef.current;
    if (!root) return undefined;
    let frame = 0;
    const update = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const rootRect = root.getBoundingClientRect();
        const elements = Array.from(root.querySelectorAll<HTMLElement>('[data-flow-node-id]'));
        const boxes = new Map(elements.map((element) => {
          const rect = element.getBoundingClientRect();
          return [element.dataset.flowNodeId ?? '', {
            left: rect.left - rootRect.left,
            right: rect.right - rootRect.left,
            top: rect.top - rootRect.top,
            bottom: rect.bottom - rootRect.top,
            centerX: rect.left - rootRect.left + rect.width / 2,
            centerY: rect.top - rootRect.top + rect.height / 2,
          }] as const;
        }));
        const links = nodes.slice(0, -1).flatMap((node, index) => {
          const next = nodes[index + 1];
          const source = boxes.get(node.id);
          const target = boxes.get(next.id);
          if (!source || !target) return [];
          const deltaX = target.centerX - source.centerX;
          const deltaY = target.centerY - source.centerY;
          const horizontal = Math.abs(deltaX) >= Math.abs(deltaY);
          const start: [number, number] = horizontal
            ? [deltaX >= 0 ? source.right : source.left, source.centerY]
            : [source.centerX, deltaY >= 0 ? source.bottom : source.top];
          const end: [number, number] = horizontal
            ? [deltaX >= 0 ? target.left : target.right, target.centerY]
            : [target.centerX, deltaY >= 0 ? target.top : target.bottom];
          return [{ start, end, tone: next.tone as PlaybookFlowGeometry['links'][number]['tone'] }];
        });
        setGeometry({ width: rootRect.width, height: rootRect.height, links });
      });
    };
    update();
    const observer = new ResizeObserver(update);
    observer.observe(root);
    window.addEventListener('resize', update);
    return () => {
      cancelAnimationFrame(frame);
      observer.disconnect();
      window.removeEventListener('resize', update);
    };
  }, [nodeSignature]);

  return <div ref={flowRef} className={`taf-playbooks-flow has-${actionNodes.length}-actions`}><PlaybookFlowConnectionsChart geometry={geometry} />{nodes.map((node) => <button key={node.id} type="button" data-flow-node-id={node.id} title={`${node.title} · ${node.caption}`} aria-label={`${node.title}：${node.caption}`} className={`taf-playbooks-flow-node is-${node.tone} is-${node.id}`}><span>{node.icon}</span><b>{node.title}</b><em>{node.caption}</em><small>{node.id === 'confirm' ? '审批门禁' : 'simulated'}</small></button>)}<div className="taf-playbooks-flow-legend"><i className="is-ok" />已定义<i className="is-warn" />审批门禁<i className="is-risk" />高风险模拟<i className="is-info" />普通模拟</div></div>;
}

function TriggerPolicy({ definition }: { definition?: PlaybookDefinition }) {
  if (!definition) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  const rows = [
    ['告警类型', definition.trigger.alert_type], ['最低严重级别', definition.trigger.severity_min], ['最低置信度', definition.trigger.score_min.toFixed(2)],
    ['条件数量', String(definition.conditions?.length ?? 0)], ['实行动作上限', `${definition.max_runs}（待 provider）`], ['实行动作冷却', `${Math.round(definition.cooldown / 60_000_000_000)} 分钟（待 provider）`],
    ['审批角色', definition.approval_policy.minimum_role || '-'], ['确认方式', definition.approval_policy.two_person_rule ? '两人独立审批' : '单人审批'], ['执行模式', '仅演练 / simulated'],
  ];
  return <div className="taf-playbooks-policy">{rows.map(([label, value]) => <label key={label}><span>{label}</span><Input size="small" value={value} readOnly /></label>)}</div>;
}

function RiskControl({ record }: { record?: PlaybookDefinitionRecord }) {
  if (!record) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  const definition = record.definition;
  const rows = [
    ['高危动作二次确认', definition.approval_policy.two_person_rule ? '已启用' : '未启用'], ['授权边界', definition.approval_policy.minimum_role || '-'],
    ['当前阶段', stageLabel(record.stage)], ['风险级别', riskLabel(record.risk_level)], ['审批人', record.approved_by || '待独立审批'],
    ['可回滚', definition.rollback_policy.supported ? (definition.rollback_policy.automatic ? '支持自动记录' : '支持手动记录') : '不支持'], ['外部动作提供方', '未接入（演练安全门）'],
  ];
  return <div className="taf-playbooks-risk">{rows.map(([label, value]) => <span key={label}><em>{label}</em><b>{value}</b></span>)}</div>;
}

function ExecutionHistory({ rows }: { rows: PlaybookExecutionRecord[] }) {
  if (!rows.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无演练执行记录" />;
  return <div className="taf-playbooks-history"><div><span>执行时间</span><span>执行对象</span><span>步骤状态</span><span>耗时</span><span>模式</span><span>操作者</span><span>关联告警</span></div>{rows.slice(0, 5).map((row) => <button key={row.execution_id} type="button">{[formatTime(row.created_at), String(row.request_payload.source_ip ?? '-'), row.status, `${row.duration_ms}ms`, row.mode, row.requested_by || '-', row.alert_id].map((cell, index) => <span key={`${cell}-${index}`} className={index === 2 ? statusClass(cell) : ''}>{cell}</span>)}</button>)}<footer><HistoryOutlined /> 共 {rows.length} 条租户执行记录</footer></div>;
}

function EffectComparison({ execution }: { execution?: PlaybookExecutionRecord }) {
  if (!execution) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="执行演练后生成效果证据" />;
  const effect = execution.effect ?? {};
  const before = numberValue(effect.alerts_before);
  const after = numberValue(effect.alerts_after);
  const rows: Array<[string, number, number, string]> = [
    ['告警数量', before, after, `${after - before}`], ['模拟阻断连接', 0, numberValue(effect.blocked_connections), `+${numberValue(effect.blocked_connections)}`],
    ['模拟隔离主机', 0, numberValue(effect.isolated_hosts), `+${numberValue(effect.isolated_hosts)}`], ['外部影响', 0, effect.external_effect_applied === true ? 1 : 0, effect.external_effect_applied === true ? '已施加' : '未施加'],
  ];
  return <div className="taf-playbooks-effect">{rows.map(([label, start, end, delta]) => <span key={label}><em>{label}</em><b className={end === 0 ? 'is-ok' : 'is-warn'}>{delta}</b><i><strong style={{ height: `${Math.max(14, start * 4)}px` }} /><strong style={{ height: `${Math.max(14, end * 4)}px` }} /></i><small>演练前 {start} / 演练后 {end}</small></span>)}<footer>数据来源：演练输入与 simulated 动作结果 <a>执行 ID：{execution.execution_id}</a></footer></div>;
}

function AuditEvidence({ rows, executionCount }: { rows: PlaybookAuditRecord[]; executionCount: number }) {
  if (!rows.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无剧本审计记录" />;
  return <div className="taf-playbooks-audit"><div><span>事件编号</span><span>对象</span><span>动作</span><span>版本</span><span>审计时间</span><span>证据</span></div>{rows.slice(0, 4).map((row) => <button key={row.event_id} type="button"><span>{row.event_id}</span><span>{row.object_id}</span><span>{auditLabel(row.action)}</span><span>{String(row.detail.version ?? '-')}</span><span>{formatTime(row.created_at)}</span><AuditOutlined /></button>)}<footer><span><AuditOutlined />审计事件<b>{rows.length}</b></span><span><PlayCircleOutlined />演练记录<b>{executionCount}</b></span><span><LockOutlined />审批证据<b>{rows.filter((row) => row.action.includes('APPROV')).length}</b></span><span data-testid="playbook-rollback-evidence"><RollbackOutlined />回滚证据<b>{rows.filter((row) => isPlaybookRollbackEvidence(row.action)).length}</b></span></footer></div>;
}

const editorDefinition = (values: EditorValues, base: PlaybookDefinition): PlaybookDefinition => {
  const previous = new Map(base.actions.map((action) => [action.type, action]));
  const actions = values.actionTypes.map((type): PlaybookAction => previous.get(type) ?? { type, parameters: defaultParameters(type), timeout: 30_000_000_000 });
  const highRisk = actions.some((action) => actionRisk(action.type));
  return {
    ...base, name: values.name, description: values.description, enabled: false,
    trigger: { ...base.trigger, alert_type: values.alertType, severity_min: values.severity, score_min: values.score },
    actions, cooldown: values.cooldownMinutes * 60_000_000_000, max_runs: values.maxRuns,
    approval_policy: { required: highRisk || base.approval_policy.required, minimum_role: base.approval_policy.minimum_role || '安全运营组（L2）', two_person_rule: highRisk || base.approval_policy.two_person_rule },
    rollback_policy: { supported: highRisk || base.rollback_policy.supported, automatic: base.rollback_policy.automatic },
  };
};

const buildMetrics = (labels: string[], definitions: PlaybookDefinitionRecord[], executions: PlaybookExecutionRecord[]): PageSnapshot['metrics'] => {
  const today = new Date().toISOString().slice(0, 10);
  const values = [definitions.filter((item) => item.enabled).length, definitions.filter((item) => item.stage === 'approval_pending').length, executions.filter((item) => item.created_at.startsWith(today)).length, executions.reduce((sum, item) => sum + item.failed_actions, 0), definitions.filter((item) => ['high', 'critical'].includes(item.risk_level) && item.stage === 'approval_pending').length, executions.length ? `${Math.round(executions.reduce((sum, item) => sum + item.duration_ms, 0) / executions.length)}ms` : '0ms'];
  return labels.map((label, index) => ({ label, value: String(values[index] ?? 0), delta: 'PostgreSQL', status: index === 3 && Number(values[index]) > 0 ? 'warn' : 'info' }));
};

const latestExecutionTime = (rows: PlaybookExecutionRecord[] | undefined, name: string) => formatTime(rows?.find((item) => item.playbook_name === name)?.created_at);
const unique = (values: string[]) => [...new Set(values.filter(Boolean))];
const numberValue = (value: unknown) => typeof value === 'number' && Number.isFinite(value) ? value : Number(value) || 0;
const formatTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false, month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '-';
const errorText = (error: unknown) => error instanceof Error ? error.message : '请检查 /v1/playbooks/*、APISIX 路由或 alert-service。';
const stageLabel = (stage?: string) => ({ draft: '草稿', approval_pending: '待审批', approved: '已审批', rejected: '已驳回' }[stage ?? ''] ?? '-');
const riskLabel = (risk: string) => ({ critical: '高危', high: '高危', medium: '中危', low: '低危' }[risk] ?? risk);
const actionLabel = (action: string) => ({ block_ip: '封禁 IP', block_domain: '阻断域名', quarantine: '隔离主机', capture_pcap: '采集 PCAP', rate_limit: '限速', tag: '资产标记', enrich: '情报富化', escalate: '升级处置', notify: '通知' }[action] ?? action);
const auditLabel = (action: string) => ({ PLAYBOOK_DRAFT_SAVED: '保存草稿', PLAYBOOK_APPROVAL_SUBMITTED: '提交审批', PLAYBOOK_APPROVED: '审批通过', PLAYBOOK_REJECTED: '审批驳回', PLAYBOOK_DRILL_COMPLETED: '完成演练', PLAYBOOK_DRILL_ROLLED_BACK: '记录回滚' }[action] ?? action);
const actionRisk = (action: string) => ['block_ip', 'block_domain', 'quarantine', 'rate_limit', 'escalate'].includes(action);
const defaultParameters = (action: string): Record<string, unknown> => action === 'capture_pcap' ? { duration: '300s' } : action === 'notify' ? { channel: 'security-operations' } : {};
const statusClass = (value: string) => value.includes('fail') || value.includes('失败') ? 'is-risk' : value.includes('rollback') || value.includes('回滚') ? 'is-warn' : 'is-ok';
