import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Form, Input, InputNumber, Modal, Result, Select, Space, Table, Tabs, Tag, Typography } from 'antd';
import type { NavRoute } from '@/routes/routeManifest';
import {
  activateAnalysisSchedule, fetchAnalysisResources, fetchAnalysisSchedules, fetchAnalysisTasks, friendlyAnalysisError,
  pauseAnalysisSchedule, saveAnalysisSchedule,
  type AnalysisResourceViews, type AnalysisScheduleView, type AnalysisTaskView,
} from '@/services/analysisApi';

/** 报告状态/激活头状态的确定性文案(§18;未知枚举 fail-closed,不默认成功绿)。 */
const HEAD_STATE: Record<string, { label: string; color: string }> = {
  DRAFT: { label: '草稿', color: 'default' },
  ACTIVE: { label: '已激活', color: 'green' },
  PAUSED: { label: '已暂停', color: 'orange' },
  RETIRED: { label: '已退休', color: 'default' },
};

function headStateView(state: string) {
  const known = HEAD_STATE[state];
  if (!known) {
    return <Tag color="default">状态无法确认({state})</Tag>;
  }
  return <Tag color={known.color}>{known.label}</Tag>;
}

/**
 * 调度管理(§76.45.1):保存=DRAFT 不触发;激活后产生未来 Trigger;
 * 暂停只影响未来触发(If-Match expected authority revision)。
 * 新建表单只提交 exact plan binding,不重复提交 plan source。
 */
export function AnalysisSchedulesPage({ route }: { route: NavRoute }) {
  const [rows, setRows] = useState<AnalysisScheduleView[]>([]);
  const [tasks, setTasks] = useState<AnalysisTaskView[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setRows(await fetchAnalysisSchedules());
    } catch (e) {
      setError(friendlyAnalysisError(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    fetchAnalysisTasks().then(setTasks).catch(() => setTasks([]));
  }, [load]);

  const onActivate = async (row: AnalysisScheduleView) => {
    try {
      await activateAnalysisSchedule(row.ScheduleID, row.AuthorityRevision, 'operator');
      await load();
    } catch (e) {
      setError(friendlyAnalysisError(e));
    }
  };

  const onPause = async (row: AnalysisScheduleView) => {
    try {
      await pauseAnalysisSchedule(row.ScheduleID, row.AuthorityRevision, 'operator');
      await load();
    } catch (e) {
      setError(friendlyAnalysisError(e));
    }
  };

  const onSubmit = async () => {
    const values = await form.validateFields();
    try {
      await saveAnalysisSchedule({
        task_definition_id: values.task_definition_id,
        approved_plan_revision: values.approved_plan_revision,
        execution_spec_sha256: values.execution_spec_sha256,
        trigger_kind: values.trigger_kind,
        timezone: 'UTC',
        window_or_cron: { window_ms: values.window_ms },
        prepare_lead_time_ms: values.prepare_lead_time_ms,
        misfire_policy: values.misfire_policy,
        concurrency_policy: values.concurrency_policy,
        scheduling_class: values.scheduling_class,
        resource_restrictions: {},
        client_idempotency_key: `schedule-${crypto.randomUUID()}`,
      });
      setOpen(false);
      form.resetFields();
      await load();
    } catch (e) {
      setError(friendlyAnalysisError(e));
    }
  };

  const columns = [
    { title: '调度/修订', dataIndex: 'Revision', render: (_: unknown, r: AnalysisScheduleView) => `rev ${r.Revision}` },
    { title: '任务定义', dataIndex: 'TaskDefinitionID', ellipsis: true },
    { title: '绑定计划', dataIndex: 'ApprovedPlanRevision', render: (_: unknown, r: AnalysisScheduleView) => `plan@${r.ApprovedPlanRevision}` },
    { title: '触发', dataIndex: 'TriggerKind' },
    { title: '并发策略', dataIndex: 'ConcurrencyPolicy' },
    { title: '调度类别', dataIndex: 'SchedulingClass' },
    { title: '生命周期', dataIndex: 'HeadState', render: (v: string) => headStateView(v) },
    {
      title: '操作',
      key: 'actions',
      render: (_: unknown, r: AnalysisScheduleView) => (
        <Space>
          {r.HeadState === 'DRAFT' || r.HeadState === 'PAUSED' ? (
            <Button size="small" onClick={() => void onActivate(r)}>激活</Button>
          ) : null}
          {r.HeadState === 'ACTIVE' ? (
            <Button size="small" danger onClick={() => void onPause(r)}>暂停</Button>
          ) : null}
        </Space>
      ),
    },
  ];

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <Space style={{ marginBottom: 12 }}>
        <Button type="primary" onClick={() => setOpen(true)}>新建调度</Button>
        <Button onClick={() => void load()}>刷新</Button>
      </Space>
      {error ? <Alert type="error" message={error} style={{ marginBottom: 12 }} /> : null}
      <Table
        rowKey="ScheduleID"
        loading={loading}
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: '暂无调度;新建调度保存后为草稿,激活后才产生未来触发' }}
      />
      <Modal
        open={open}
        title="新建调度(保存=DRAFT 不触发;激活后生效)"
        onCancel={() => setOpen(false)}
        onOk={() => void onSubmit()}
        destroyOnClose
      >
        <Form form={form} layout="vertical" initialValues={{
          trigger_kind: 'CONTINUOUS_WINDOW', misfire_policy: 'MISFIRE_FAIL',
          concurrency_policy: 'FORBID_OVERLAP', scheduling_class: 'BASELINE',
          window_ms: 60000, prepare_lead_time_ms: 5000,
        }}>
          <Form.Item name="task_definition_id" label="任务定义(批准计划目录)" rules={[{ required: true }]}>
            <Select
              showSearch
              optionFilterProp="label"
              options={tasks.map((t) => ({ label: `${t.name} (${t.id.slice(0, 8)})`, value: t.id }))}
            />
          </Form.Item>
          <Form.Item name="approved_plan_revision" label="批准计划修订(精确绑定)" rules={[{ required: true }]}>
            <InputNumber min={1} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="execution_spec_sha256" label="执行规格哈希" rules={[{ required: true, len: 64 }]}>
            <Input placeholder="64 位 hex;与批准计划修订一致" />
          </Form.Item>
          <Form.Item name="trigger_kind" label="触发方式" rules={[{ required: true }]}>
            <Select options={[
              { label: '持续窗口', value: 'CONTINUOUS_WINDOW' },
              { label: 'Cron 窗口', value: 'CRON_WINDOW' },
              { label: '事件驱动', value: 'EVENT_DRIVEN' },
            ]} />
          </Form.Item>
          <Form.Item name="window_ms" label="窗口毫秒" rules={[{ required: true }]}>
            <InputNumber min={1000} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="prepare_lead_time_ms" label="前置准备毫秒">
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="misfire_policy" label="错过策略">
            <Select options={[
              { label: '失败', value: 'MISFIRE_FAIL' },
              { label: '延迟窗口', value: 'MISFIRE_DELAY' },
              { label: '有界回放(需证明输入)', value: 'MISFIRE_BOUNDED_REPLAY' },
            ]} />
          </Form.Item>
          <Form.Item name="concurrency_policy" label="并发策略">
            <Select options={[
              { label: '禁止重叠(命中即 SUPPRESSED)', value: 'FORBID_OVERLAP' },
              { label: '取消前次', value: 'CANCEL_PREVIOUS' },
              { label: '允许并发', value: 'ALLOW_CONCURRENT' },
            ]} />
          </Form.Item>
          <Form.Item name="scheduling_class" label="调度类别">
            <Select options={[
              { label: 'BASELINE', value: 'BASELINE' },
              { label: 'INTERACTIVE', value: 'INTERACTIVE' },
              { label: 'ACCEPTANCE', value: 'ACCEPTANCE' },
            ]} />
          </Form.Item>
          <Typography.Text type="secondary">
            保存仅创建 DRAFT 修订;激活按 expected authority revision 提交,暂停只停止未来触发(不影响已物化任务)。
          </Typography.Text>
        </Form>
      </Modal>
    </div>
  );
}

/**
 * 调度资源(§20):容量配额/队列/租约/执行器四 Tab,真实 API(GET /resources)。
 * 首屏回答:哪里阻塞、影响多少 Run、下一动作;数据全部来自调度权威 DB 投影。
 */
export function AnalysisResourcesPage({ route }: { route: NavRoute }) {
  const [views, setViews] = useState<AnalysisResourceViews | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setViews(await fetchAnalysisResources());
    } catch (e) {
      setError(friendlyAnalysisError(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
    const timer = setInterval(() => void load(), 10000);
    return () => clearInterval(timer);
  }, [load]);

  if (error) {
    return (
      <div className="taf-page">
        <h1>{route.title}</h1>
        <Result status="warning" title="调度资源读取失败" subTitle={error} extra={<Button onClick={() => void load()}>重试</Button>} />
      </div>
    );
  }

  const reservationColumns = [
    { title: 'Run', dataIndex: 'run_id', ellipsis: true },
    { title: '资源池', dataIndex: 'resource_pool' },
    { title: '状态', dataIndex: 'state', render: (v: string) => <Tag color={v === 'RESERVED' ? 'blue' : v === 'CONSUMED' ? 'green' : 'default'}>{v}</Tag> },
    { title: 'Epoch', dataIndex: 'epoch' },
    { title: '过期时间', dataIndex: 'expires_at' },
  ];
  const drrColumns = [
    { title: '租户', dataIndex: 'tenant_id' },
    { title: '调度类别', dataIndex: 'scheduling_class' },
    { title: 'Deficit', dataIndex: 'deficit' },
    { title: 'Quantum', dataIndex: 'quantum' },
    { title: '最后服务', dataIndex: 'last_served_at' },
    { title: 'Scheduler Epoch', dataIndex: 'scheduler_epoch' },
  ];
  const outboxColumns = [
    { title: 'Topic', dataIndex: 'topic', ellipsis: true },
    { title: '状态', dataIndex: 'state', render: (v: string) => <Tag color={v === 'PENDING' ? 'orange' : v === 'PUBLISHED' ? 'green' : 'red'}>{v}</Tag> },
    { title: '数量', dataIndex: 'count' },
    { title: '最旧 PENDING 年龄(s)', dataIndex: 'oldest_pending_age_seconds', render: (v: number) => (v > 0 ? v.toFixed(0) : '—') },
  ];
  const queueColumns = [
    { title: '调度类别', dataIndex: 'scheduling_class' },
    { title: 'Run 状态', dataIndex: 'state' },
    { title: '数量', dataIndex: 'count' },
  ];

  const activeRuns = (views?.queue ?? []).reduce((n, q) => n + q.count, 0);
  const pendingOutbox = (views?.outbox_ledger ?? []).filter((o) => o.state === 'PENDING').reduce((n, o) => n + o.count, 0);

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <Alert
        type={pendingOutbox > 0 ? 'warning' : 'info'}
        showIcon
        style={{ marginBottom: 16 }}
        message={`非终态 Run ${activeRuns} 个;Outbox 待投递 ${pendingOutbox} 条${pendingOutbox > 0 ? '(投递停滞,请查中继)' : ''}`}
      />
      <Tabs
        items={[
          {
            key: 'reservations',
            label: `容量配额(${views?.reservations.length ?? 0})`,
            children: (
              <Table
                rowKey="run_id"
                size="small"
                loading={loading}
                dataSource={views?.reservations ?? []}
                columns={reservationColumns}
                pagination={{ pageSize: 10 }}
              />
            ),
          },
          {
            key: 'queue',
            label: `队列(${activeRuns})`,
            children: (
              <Table rowKey={(r) => `${r.scheduling_class}-${r.state}`} size="small" loading={loading} dataSource={views?.queue ?? []} columns={queueColumns} pagination={false} />
            ),
          },
          {
            key: 'drr',
            label: `租约 DRR(${views?.drr.length ?? 0})`,
            children: (
              <Table rowKey={(r) => `${r.tenant_id}-${r.scheduling_class}`} size="small" loading={loading} dataSource={views?.drr ?? []} columns={drrColumns} pagination={false} />
            ),
          },
          {
            key: 'outbox',
            label: `执行器 Outbox(${views?.outbox_ledger.length ?? 0})`,
            children: (
              <Table rowKey={(r) => `${r.topic}-${r.state}`} size="small" loading={loading} dataSource={views?.outbox_ledger ?? []} columns={outboxColumns} pagination={{ pageSize: 10 }} />
            ),
          },
        ]}
      />
    </div>
  );
}

