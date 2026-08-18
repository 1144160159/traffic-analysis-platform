import { useCallback, useState } from 'react';
import { Button, Descriptions, Form, InputNumber, Modal, Select, Space, Table, Tabs, Tag, Typography, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useParams } from 'react-router-dom';
import {
  activateTaskDefinition,
  fetchDefinitionAllowedActions,
  fetchTaskDefinitionDetail,
  saveTaskDefinitionReportPolicy,
  suspendTaskDefinition,
  type AnalysisAuditRecordView,
  type AnalysisPlanRevisionView,
  type AnalysisReportPolicyView,
  type AnalysisScheduleView,
  type AnalysisTaskDefinitionDetail,
} from '@/services/analysisApi';
import { PageStateBoundary } from '@/components/PageStateBoundary';
import { resolvePageState } from '@/components/pageState';
import type { NavRoute } from '@/routes/routeManifest';

const DEFINITION_STATE: Record<string, { label: string; color: string }> = {
  DRAFT: { label: '草稿', color: 'default' },
  VALIDATED: { label: '已验证', color: 'blue' },
  ACTIVE: { label: '已激活', color: 'green' },
  SUSPENDED: { label: '已挂起', color: 'orange' },
  RETIRED: { label: '已退休', color: 'default' },
};

const POLICY_MODE: Record<string, { label: string }> = {
  DISABLED: { label: '关闭' },
  ON_DEMAND: { label: '按需' },
  AUTO_ASYNC: { label: '自动异步' },
};

function stateTag(state: string) {
  const known = DEFINITION_STATE[state];
  return <Tag color={known?.color ?? 'default'}>{known?.label ?? `状态无法确认(${state})`}</Tag>;
}

/**
 * 任务定义详情五 Tab(§20):基本信息/方案版本/调度计划/报告策略/审计记录。
 * 按钮(激活/挂起)由 GET allowed-actions 服务端驱动;If-Match expected revision。
 */
export function AnalysisTaskDefinitionDetailPage({ route }: { route: NavRoute }) {
  const { taskDefinitionId = '' } = useParams();
  const queryClient = useQueryClient();

  const detailQuery = useQuery({
    queryKey: ['analysis', 'task-definition', taskDefinitionId],
    queryFn: () => fetchTaskDefinitionDetail(taskDefinitionId),
    enabled: Boolean(taskDefinitionId),
    retry: false,
  });

  const actionsQuery = useQuery({
    queryKey: ['analysis', 'task-definition-actions', taskDefinitionId],
    queryFn: () => fetchDefinitionAllowedActions(taskDefinitionId),
    enabled: Boolean(taskDefinitionId),
    retry: false,
  });

  const [policyOpen, setPolicyOpen] = useState(false);
  const [form] = Form.useForm();

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['analysis', 'task-definition', taskDefinitionId] });
    void queryClient.invalidateQueries({ queryKey: ['analysis', 'task-definition-actions', taskDefinitionId] });
  }, [queryClient, taskDefinitionId]);

  const activateMutation = useMutation({
    mutationFn: () => activateTaskDefinition(taskDefinitionId, detailQuery.data?.revision ?? 0, 'operator'),
    onSuccess: () => { message.success('已激活'); invalidate(); },
    onError: (error: Error) => message.error(error.message),
  });

  const suspendMutation = useMutation({
    mutationFn: () => suspendTaskDefinition(taskDefinitionId, detailQuery.data?.revision ?? 0, 'operator'),
    onSuccess: () => { message.success('已挂起'); invalidate(); },
    onError: (error: Error) => message.error(error.message),
  });

  const policyMutation = useMutation({
    mutationFn: (values: { mode: string; template_revision: string; locale: string; retention_days: number }) =>
      saveTaskDefinitionReportPolicy(taskDefinitionId, {
        ...values,
        client_idempotency_key: `policy-${crypto.randomUUID()}`,
      }),
    onSuccess: () => { message.success('报告策略修订已保存'); setPolicyOpen(false); form.resetFields(); invalidate(); },
    onError: (error: Error) => message.error(error.message),
  });

  const detail = detailQuery.data;
  const actions = actionsQuery.data;

  const planColumns = [
    { title: '修订', dataIndex: 'plan_revision' },
    { title: '来源', dataIndex: 'plan_source', render: (v: string) => <Tag color={v === 'AUTO_DEFAULT' ? 'blue' : 'purple'}>{v}</Tag> },
    { title: '源类型', dataIndex: 'source_kind' },
    { title: '执行规格 sha', dataIndex: 'execution_spec_sha256', ellipsis: true, render: (v: string) => (v ? v.slice(0, 16) + '…' : '—') },
    { title: '治理状态', dataIndex: 'governance_state', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : 'default'}>{v}</Tag> },
    { title: '创建时间', dataIndex: 'created_at' },
  ];

  const scheduleColumns = [
    { title: '修订', dataIndex: 'Revision' },
    { title: '绑定计划', dataIndex: 'ApprovedPlanRevision', render: (_: unknown, r: AnalysisScheduleView) => `plan@${r.ApprovedPlanRevision}` },
    { title: '触发', dataIndex: 'TriggerKind' },
    { title: '并发策略', dataIndex: 'ConcurrencyPolicy' },
    { title: '调度类别', dataIndex: 'SchedulingClass' },
    { title: '生命周期', dataIndex: 'HeadState', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : v === 'PAUSED' ? 'orange' : 'default'}>{v}</Tag> },
  ];

  const policyColumns = [
    { title: '修订', dataIndex: 'revision' },
    { title: '模式', dataIndex: 'mode', render: (v: string) => POLICY_MODE[v]?.label ?? v },
    { title: '模板', dataIndex: 'template_revision' },
    { title: '语言', dataIndex: 'locale' },
    { title: '保留天数', dataIndex: 'retention_days' },
    { title: '策略 sha', dataIndex: 'policy_sha256', ellipsis: true, render: (v: string) => (v ? v.slice(0, 16) + '…' : '—') },
  ];

  const auditColumns = [
    { title: '动作', dataIndex: 'action', width: 160 },
    { title: '操作者', dataIndex: 'actor', width: 140 },
    { title: '明细', dataIndex: 'detail', ellipsis: true },
    { title: '时间', dataIndex: 'created_at', width: 220 },
  ];

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <PageStateBoundary state={resolvePageState({ isLoading: detailQuery.isLoading, data: detailQuery.data, error: detailQuery.error })}>
        {detail && (
          <>
            <Space style={{ marginBottom: 16 }}>
              {actions?.activate && (
                <Button type="primary" loading={activateMutation.isPending} onClick={() => activateMutation.mutate()}>
                  激活(If-Match rev {detail.revision})
                </Button>
              )}
              {actions?.suspend && (
                <Button danger loading={suspendMutation.isPending} onClick={() => suspendMutation.mutate()}>
                  挂起(If-Match rev {detail.revision})
                </Button>
              )}
            </Space>
            <Tabs
              items={[
                {
                  key: 'basic',
                  label: '基本信息',
                  children: (
                    <Descriptions bordered size="small" column={2}>
                      <Descriptions.Item label="ID">{detail.id}</Descriptions.Item>
                      <Descriptions.Item label="名称">{detail.name}</Descriptions.Item>
                      <Descriptions.Item label="状态">{stateTag(detail.state)}</Descriptions.Item>
                      <Descriptions.Item label="负责人">{detail.owner}</Descriptions.Item>
                      <Descriptions.Item label="默认调度类别">{detail.default_scheduling_class}</Descriptions.Item>
                      <Descriptions.Item label="权威修订">{detail.revision}</Descriptions.Item>
                      <Descriptions.Item label="激活计划修订">{detail.active_plan_revision ?? '—'}</Descriptions.Item>
                      <Descriptions.Item label="激活调度修订">{detail.active_schedule_revision ?? '—'}</Descriptions.Item>
                    </Descriptions>
                  ),
                },
                {
                  key: 'plans',
                  label: `方案版本(${detail.plans?.length ?? 0})`,
                  children: (
                    <Table<AnalysisPlanRevisionView>
                      rowKey={(r) => `${r.plan_id}-${r.plan_revision}`}
                      size="small"
                      dataSource={detail.plans ?? []}
                      columns={planColumns}
                      pagination={{ pageSize: 10 }}
                    />
                  ),
                },
                {
                  key: 'schedules',
                  label: `调度计划(${detail.schedules?.length ?? 0})`,
                  children: (
                    <Table<AnalysisScheduleView>
                      rowKey={(r) => `${r.ScheduleID}-${r.Revision}`}
                      size="small"
                      dataSource={detail.schedules ?? []}
                      columns={scheduleColumns}
                      pagination={{ pageSize: 10 }}
                    />
                  ),
                },
                {
                  key: 'policies',
                  label: `报告策略(${detail.report_policies?.length ?? 0})`,
                  children: (
                    <>
                      <Space style={{ marginBottom: 12 }}>
                        <Button type="primary" onClick={() => setPolicyOpen(true)}>保存策略修订</Button>
                        <Typography.Text type="secondary">独立冻结,不进执行计划哈希;自动分配修订号。</Typography.Text>
                      </Space>
                      <Table<AnalysisReportPolicyView>
                        rowKey={(r) => `${r.policy_id}-${r.revision}`}
                        size="small"
                        dataSource={detail.report_policies ?? []}
                        columns={policyColumns}
                        pagination={{ pageSize: 10 }}
                      />
                    </>
                  ),
                },
                {
                  key: 'audit',
                  label: `审计记录(${detail.audit_records?.length ?? 0})`,
                  children: (
                    <Table<AnalysisAuditRecordView>
                      rowKey={(r) => `${r.created_at}-${r.action}-${r.actor}`}
                      size="small"
                      dataSource={detail.audit_records ?? []}
                      columns={auditColumns}
                      pagination={{ pageSize: 10 }}
                    />
                  ),
                },
              ]}
            />
          </>
        )}
      </PageStateBoundary>

      <Modal
        title="保存报告策略修订"
        open={policyOpen}
        onCancel={() => setPolicyOpen(false)}
        onOk={() => form.validateFields().then((v) => policyMutation.mutate(v)).catch(() => undefined)}
        confirmLoading={policyMutation.isPending}
      >
        <Form form={form} layout="vertical" initialValues={{ mode: 'ON_DEMAND', template_revision: 'default-v1', locale: 'zh-CN', retention_days: 30 }}>
          <Form.Item name="mode" label="模式" rules={[{ required: true }]}>
            <Select options={Object.entries(POLICY_MODE).map(([value, { label }]) => ({ value, label }))} />
          </Form.Item>
          <Form.Item name="template_revision" label="模板修订" rules={[{ required: true }]}>
            <Select options={['default-v1'].map((v) => ({ value: v, label: v }))} />
          </Form.Item>
          <Form.Item name="locale" label="语言" rules={[{ required: true }]}>
            <Select options={['zh-CN', 'en-US'].map((v) => ({ value: v, label: v }))} />
          </Form.Item>
          <Form.Item name="retention_days" label="保留天数" rules={[{ required: true }]}>
            <InputNumber min={1} max={3650} style={{ width: '100%' }} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
