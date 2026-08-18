import { useCallback, useEffect, useState } from 'react';
import { Alert, Button, Descriptions, Form, Input, Modal, Select, Space, Table, Tag, Typography, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  approveAnalysisPlan,
  fetchAnalysisTasks,
  fetchTaskDefinitionDetail,
  preflightAnalysisPlan,
  saveAnalysisPlan,
  type AnalysisPlanRevisionView,
  type AnalysisTaskView,
} from '@/services/analysisApi';
import { PageStateBoundary } from '@/components/PageStateBoundary';
import { resolvePageState } from '@/components/pageState';
import type { NavRoute } from '@/routes/routeManifest';

const PLAN_SOURCE_LABEL: Record<string, { label: string; color: string }> = {
  AUTO_DEFAULT: { label: '默认计划', color: 'blue' },
  MANUAL_CUSTOM: { label: '人工定制', color: 'purple' },
};

/**
 * 任务编排(§20):预检(不物化)/保存默认计划/保存定制计划/审批激活。
 * 只读五段 + PlanReady/Reconcile 技术闸门说明;计划修订列表来自任务定义详情。
 */
export function AnalysisOrchestrationPage({ route }: { route: NavRoute }) {
  const queryClient = useQueryClient();
  const [defId, setDefId] = useState<string>('');
  const [planSource, setPlanSource] = useState<'AUTO_DEFAULT' | 'MANUAL_CUSTOM'>('AUTO_DEFAULT');
  const [overridesText, setOverridesText] = useState('');
  const [saveOpen, setSaveOpen] = useState(false);
  const [preflightResult, setPreflightResult] = useState<{ execution_spec_sha256: string; source_kind: string; compatible: boolean } | null>(null);
  const [form] = Form.useForm();

  const tasksQuery = useQuery({
    queryKey: ['analysis', 'tasks'],
    queryFn: fetchAnalysisTasks,
    retry: false,
  });

  const detailQuery = useQuery({
    queryKey: ['analysis', 'task-definition', defId],
    queryFn: () => fetchTaskDefinitionDetail(defId),
    enabled: Boolean(defId),
    retry: false,
  });

  const invalidate = useCallback(() => {
    if (defId) {
      void queryClient.invalidateQueries({ queryKey: ['analysis', 'task-definition', defId] });
    }
  }, [queryClient, defId]);

  useEffect(() => {
    if (!defId && tasksQuery.data && tasksQuery.data.length > 0) {
      setDefId(tasksQuery.data[0].id);
    }
  }, [tasksQuery.data, defId]);

  const parseOverrides = (): Record<string, unknown> | undefined => {
    const text = overridesText.trim();
    if (!text) {
      return undefined;
    }
    try {
      return JSON.parse(text) as Record<string, unknown>;
    } catch {
      message.error('custom_overrides 不是合法 JSON');
      return undefined;
    }
  };

  const buildInput = () => ({
    task_definition_id: defId,
    plan_source: planSource,
    custom_overrides: planSource === 'MANUAL_CUSTOM' ? parseOverrides() : undefined,
    client_idempotency_key: `plan-${crypto.randomUUID()}`,
  });

  const preflightMutation = useMutation({
    mutationFn: () => preflightAnalysisPlan(buildInput()),
    onSuccess: (data) => setPreflightResult(data),
    onError: (error: Error) => message.error(error.message),
  });

  const saveMutation = useMutation({
    mutationFn: () => saveAnalysisPlan(buildInput()),
    onSuccess: (data) => {
      message.success(`计划修订已保存:rev ${data.plan_revision}(DRAFT,需审批激活)`);
      setSaveOpen(false);
      invalidate();
    },
    onError: (error: Error) => message.error(error.message),
  });

  const approveMutation = useMutation({
    mutationFn: (planId: string) => approveAnalysisPlan(planId, 'operator', 'checker-ops'),
    onSuccess: () => { message.success('已审批激活'); invalidate(); },
    onError: (error: Error) => message.error(error.message),
  });

  const plans = detailQuery.data?.plans ?? [];

  const planColumns = [
    { title: '修订', dataIndex: 'plan_revision', width: 80 },
    {
      title: '来源',
      dataIndex: 'plan_source',
      render: (v: string) => {
        const known = PLAN_SOURCE_LABEL[v];
        return <Tag color={known?.color ?? 'default'}>{known?.label ?? v}</Tag>;
      },
    },
    { title: '源类型', dataIndex: 'source_kind' },
    { title: '执行规格 sha', dataIndex: 'execution_spec_sha256', ellipsis: true, render: (v: string) => (v ? v.slice(0, 16) + '…' : '—') },
    { title: '治理状态', dataIndex: 'governance_state', render: (v: string) => <Tag color={v === 'ACTIVE' ? 'green' : 'default'}>{v}</Tag> },
    {
      title: '操作',
      key: 'ops',
      width: 120,
      render: (_: unknown, p: AnalysisPlanRevisionView) =>
        p.governance_state === 'DRAFT' ? (
          <Button size="small" loading={approveMutation.isPending} onClick={() => approveMutation.mutate(p.plan_id)}>
            审批激活
          </Button>
        ) : '—',
    },
  ];

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <Space direction="vertical" style={{ width: '100%' }} size="middle">
        <Alert
          type="info"
          showIcon
          message="PlanReady 与 Reconcile 是不可删除的技术闸门"
          description="PlanReady 在阶段 1 前校验 required consumer exact-set;Reconcile 在阶段 4/5 之间对账 count/watermark;二者不占用业务五段(S1 采集→S2 特征→S3 识别→S4 检测→S5 机器摘要)。"
        />
        <Space wrap>
          <Select
            style={{ minWidth: 280 }}
            placeholder="选择任务定义"
            value={defId || undefined}
            onChange={setDefId}
            options={(tasksQuery.data ?? []).map((t: AnalysisTaskView) => ({ value: t.id, label: `${t.name}(${t.state})` }))}
          />
          <Select
            style={{ minWidth: 140 }}
            value={planSource}
            onChange={setPlanSource}
            options={[
              { value: 'AUTO_DEFAULT', label: '默认计划(AUTO)' },
              { value: 'MANUAL_CUSTOM', label: '人工定制(MANUAL)' },
            ]}
          />
          <Button loading={preflightMutation.isPending} onClick={() => preflightMutation.mutate()}>
            预检(不物化)
          </Button>
          <Button type="primary" onClick={() => setSaveOpen(true)}>
            保存计划修订
          </Button>
        </Space>

        {preflightResult && (
          <Descriptions bordered size="small" column={2}>
            <Descriptions.Item label="冻结执行规格 sha">
              {preflightResult.execution_spec_sha256.slice(0, 24)}…
            </Descriptions.Item>
            <Descriptions.Item label="源类型">{preflightResult.source_kind}</Descriptions.Item>
            <Descriptions.Item label="兼容性">
              <Tag color={preflightResult.compatible ? 'green' : 'red'}>{preflightResult.compatible ? '兼容' : '不兼容'}</Tag>
            </Descriptions.Item>
          </Descriptions>
        )}

        <PageStateBoundary state={resolvePageState({ isLoading: detailQuery.isLoading, data: detailQuery.data, error: detailQuery.error })}>
          {detailQuery.data && (
            <Table<AnalysisPlanRevisionView>
              rowKey={(r) => `${r.plan_id}-${r.plan_revision}`}
              size="small"
              dataSource={plans}
              columns={planColumns}
              pagination={{ pageSize: 10 }}
            />
          )}
        </PageStateBoundary>
      </Space>

      <Modal
        title={`保存计划修订:${planSource === 'AUTO_DEFAULT' ? '默认计划' : '人工定制'}`}
        open={saveOpen}
        onCancel={() => setSaveOpen(false)}
        onOk={() => {
          if (planSource === 'MANUAL_CUSTOM') {
            if (!overridesText.trim()) {
              message.error('MANUAL_CUSTOM 需要 custom_overrides(至少一项覆盖)');
              return;
            }
            if (parseOverrides() === undefined) {
              return;
            }
          }
          saveMutation.mutate();
        }}
        confirmLoading={saveMutation.isPending}
      >
        <Form form={form} layout="vertical">
          <Typography.Text type="secondary">
            保存=AUTO_DEFAULT 以模板为全部输入;MANUAL_CUSTOM 以模板为基座+覆盖项。保存仅产生 DRAFT 修订,审批(maker/checker)后 ACTIVE。
          </Typography.Text>
          {planSource === 'MANUAL_CUSTOM' && (
            <Form.Item label="custom_overrides(JSON)" style={{ marginTop: 12 }}>
              <Input.TextArea
                rows={6}
                value={overridesText}
                onChange={(e) => setOverridesText(e.target.value)}
                placeholder={'{"source_kind":"PCAP_REPLAY","rule_refs":["rule@v1"]}'}
              />
            </Form.Item>
          )}
        </Form>
      </Modal>
    </div>
  );
}
