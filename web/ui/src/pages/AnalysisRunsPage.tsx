import { Button, Form, Input, Modal, Select, Steps, Table, Tag, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  analysisQueryKeys,
  approveAnalysisPlan,
  cancelAnalysisRun,
  fetchAnalysisRuns,
  saveAnalysisCustomPlan,
  submitAnalysisTrigger,
  type AnalysisRunView,
  type AnalysisTriggerOverrides,
  type SubmitTriggerInput,
  fetchAnalysisTasks,
  type AnalysisTaskView,
} from '@/services/analysisApi';
import { PageStateBoundary } from '@/components/PageStateBoundary';
import { resolvePageState } from '@/components/pageState';
import type { NavRoute } from '@/routes/routeManifest';

const stateColor: Record<string, string> = {
  SUCCEEDED: 'success',
  PARTIALLY_SUCCEEDED: 'warning',
  FAILED: 'error',
  CANCELLED: 'default',
  CANCELLED_REQUESTED: 'warning',
  RUNNING: 'processing',
  QUEUED: 'default',
  PREPARING: 'default',
  ACCEPTED: 'default',
  FINALIZING: 'processing',
};

export function AnalysisRunsPage({ route }: { route: NavRoute }) {
  const queryClient = useQueryClient();
  const [stateFilter, setStateFilter] = useState<string>('');
  const [wizardOpen, setWizardOpen] = useState(false);

  const runsQuery = useQuery({
    queryKey: analysisQueryKeys.runs(stateFilter),
    queryFn: () => fetchAnalysisRuns(stateFilter || undefined),
    retry: false,
  });

  const cancelMutation = useMutation({
    mutationFn: ({ runId, key }: { runId: string; key: string }) => cancelAnalysisRun(runId, key),
    onSuccess: () => {
      message.success('取消请求已受理(终态以运行详情为准)');
      void queryClient.invalidateQueries({ queryKey: ['analysis', 'runs'] });
    },
    onError: (error: Error) => message.error(error.message),
  });

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <div style={{ display: 'flex', gap: 12, marginBottom: 16, justifyContent: 'space-between' }}>
        <Select
          allowClear
          placeholder="按状态筛选"
          style={{ width: 220 }}
          value={stateFilter || undefined}
          onChange={(v) => setStateFilter(v ?? '')}
          options={Object.keys(stateColor).map((s) => ({ value: s, label: s }))}
        />
        <Button type="primary" onClick={() => setWizardOpen(true)}>即时分析</Button>
      </div>
      <PageStateBoundary state={resolvePageState({ isLoading: runsQuery.isLoading, data: runsQuery.data, error: runsQuery.error })}>
        <Table<AnalysisRunView>
          rowKey="run_id"
          dataSource={runsQuery.data ?? []}
          pagination={{ pageSize: 20 }}
          columns={[
            { title: '运行 ID', dataIndex: 'run_id', render: (id: string) => <Link to={`/analysis/runs/${id}`}>{id.slice(0, 12)}</Link> },
            { title: '任务', dataIndex: 'task_id', render: (id: string) => id.slice(0, 12) },
            { title: '状态', dataIndex: 'state', render: (s: string) => <Tag color={stateColor[s]}>{s}</Tag> },
            { title: '机器结论', dataIndex: 'finding_conclusion', render: (v: string) => v || '—' },
            { title: '完整性', dataIndex: 'completeness', render: (v: string) => v || '—' },
            { title: '证据完整性', dataIndex: 'integrity_state', render: (v: string) => v || '—' },
            { title: '风险', dataIndex: 'risk_severity', render: (v: string) => v || '—' },
            { title: '报告状态', dataIndex: 'report_state', render: (v: string) => <Tag color={v === 'AVAILABLE' ? 'green' : v === 'FAILED' ? 'red' : 'default'}>{v || 'NOT_REQUESTED'}</Tag> },
            { title: '操作', key: 'ops', render: (_, row) =>
                ['RUNNING', 'QUEUED', 'PREPARING', 'ACCEPTED', 'FINALIZING'].includes(row.state) ? (
                  <Button
                    size="small"
                    danger
                    onClick={() => cancelMutation.mutate({
                      runId: row.run_id,
                      key: `cancel-${row.run_id}-${Date.now()}`,
                    })}
                  >
                    取消
                  </Button>
                ) : (
                  <span>—</span>
                ),
            },
          ]}
        />
      </PageStateBoundary>
      <TriggerWizard open={wizardOpen} onClose={() => setWizardOpen(false)} />
    </div>
  );
}

/** 三步即时分析向导(默认/自定义;自定义才展开覆盖字段;ON_DEMAND 固定)。 */
function TriggerWizard({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [step, setStep] = useState(0);
  const [taskDefinitionId, setTaskDefinitionId] = useState('');
  const [taskCatalog, setTaskCatalog] = useState<AnalysisTaskView[]>([]);
  // 稳定幂等键:向导会话内生成一次,提交超时后以原键恢复(§5.2 操作规则)。
  const [idempotencyKey] = useState(() => `ui-${crypto.randomUUID()}`);
  const [planSource, setPlanSource] = useState<'AUTO_DEFAULT' | 'MANUAL_CUSTOM'>('AUTO_DEFAULT');
  const [overridesJson, setOverridesJson] = useState('');
  const [checker, setChecker] = useState('');
  const [maker, setMaker] = useState('');

  const submitMutation = useMutation({
    mutationFn: async (input: {
      taskDefinitionId: string;
      planSource: 'AUTO_DEFAULT' | 'MANUAL_CUSTOM';
      overrides?: AnalysisTriggerOverrides;
      idempotencyKey: string;
      maker: string;
      checker: string;
    }) => {
      // P2 人工选择列车:custom 分支先 保存草稿→审批激活,再触发(与 AUTO 同一物化链)。
      if (input.planSource === 'MANUAL_CUSTOM') {
        const draft = await saveAnalysisCustomPlan(input.taskDefinitionId, input.overrides ?? {}, input.idempotencyKey);
        await approveAnalysisPlan(draft.plan_id, input.maker, input.checker);
      }
      return submitAnalysisTrigger({
        task_definition_id: input.taskDefinitionId,
        plan_source: input.planSource,
        custom_overrides: input.planSource === 'MANUAL_CUSTOM' ? input.overrides : undefined,
        client_idempotency_key: input.idempotencyKey,
      });
    },
    onSuccess: (result) => {
      message.success(`已受理:run ${result.run_id.slice(0, 12)}`);
      onClose();
    },
    onError: (error: Error) => message.error(error.message),
  });

  const canNext = step === 0 ? taskDefinitionId.trim() !== '' : true;

  // 打开向导时加载批准任务定义目录(向导不得用空白自由输入代替权威定义)。
  useEffect(() => {
    if (open) {
      fetchAnalysisTasks().then(setTaskCatalog).catch(() => setTaskCatalog([]));
    }
  }, [open]);

  return (
    <Modal
      title="即时分析"
      open={open}
      onCancel={onClose}
      width={520}
      footer={[
        step > 0 && <Button key="back" onClick={() => setStep(step - 1)}>上一步</Button>,
        step < 2 && (
          <Button key="next" type="primary" disabled={!canNext} onClick={() => setStep(step + 1)}>下一步</Button>
        ),
        step === 2 && (
          <Button
            key="submit"
            type="primary"
            loading={submitMutation.isPending}
            onClick={() => {
              let overrides: AnalysisTriggerOverrides | undefined;
              if (planSource === 'MANUAL_CUSTOM') {
                try {
                  overrides = JSON.parse(overridesJson || '{}') as AnalysisTriggerOverrides;
                } catch {
                  message.error('自定义覆盖项必须是合法 JSON');
                  return;
                }
                if (maker.trim() === '' || checker.trim() === '') {
                  message.error('自定义方案需要填写 maker 与 checker(审批人,须不同)');
                  return;
                }
                if (maker.trim() === checker.trim()) {
                  message.error('maker 与 checker 不能是同一人');
                  return;
                }
              }
              submitMutation.mutate({
                taskDefinitionId,
                planSource,
                overrides,
                maker: maker.trim(),
                checker: checker.trim(),
                // 幂等键一次生成,重试复用;断线后以同 key 查 receipt
                idempotencyKey,
              });
            }}
          >
            校验并提交
          </Button>
        ),
      ]}
    >
      <Steps size="small" current={step} items={[
        { title: '任务与范围' }, { title: '方案' }, { title: '校验提交' },
      ]} />
      <div style={{ marginTop: 24, minHeight: 140 }}>
        {step === 0 && (
          <Form layout="vertical">
            <Form.Item label="任务定义" required>
              <Select
                showSearch
                optionFilterProp="label"
                placeholder="从已批准任务定义目录选择"
                value={taskDefinitionId || undefined}
                onChange={(v) => setTaskDefinitionId(v)}
                options={taskCatalog.map((t) => ({ label: `${t.name} (${t.id.slice(0, 8)})`, value: t.id }))}
                notFoundContent="暂无可发起的任务定义"
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item label="说明">触发方式固定为按需(ON_DEMAND);持续/定时/事件触发在调度管理创建。</Form.Item>
          </Form>
        )}
        {step === 1 && (
          <Form layout="vertical">
            <Form.Item label="方案来源" required>
              <Select value={planSource} onChange={(v) => setPlanSource(v)} options={[
                { value: 'AUTO_DEFAULT', label: '默认方案(AUTO_DEFAULT)' },
                { value: 'MANUAL_CUSTOM', label: '自定义方案(MANUAL_CUSTOM,主业务链执行环节)' },
              ]} />
            </Form.Item>
            {planSource === 'MANUAL_CUSTOM' && (
              <Form.Item label="覆盖项(探针/特征/识别模型/检测模型/规则/阈值,JSON)">
                <Input.TextArea
                  rows={4}
                  value={overridesJson}
                  onChange={(e) => setOverridesJson(e.target.value)}
                  placeholder={'{"selected_feature_ids":["pktlen_mean"],"threat_detector_refs":["rule-scan-detect@v3"]}'}
                />
              </Form.Item>
            )}
            {planSource === 'AUTO_DEFAULT' && <Form.Item label="说明">默认方案不展示逐项 feature/model/rule;使用已批准模板。</Form.Item>}
          </Form>
        )}
        {step === 2 && (
          <Form layout="vertical">
            {planSource === 'MANUAL_CUSTOM' && (
              <>
                <Form.Item label="maker(提交人)" required>
                  <Input value={maker} onChange={(e) => setMaker(e.target.value)} placeholder="operator id" />
                </Form.Item>
                <Form.Item label="checker(审批人,须与 maker 不同)" required>
                  <Input value={checker} onChange={(e) => setChecker(e.target.value)} placeholder="approver id" />
                </Form.Item>
              </>
            )}
            <Form.Item label="提交确认">
              {planSource === 'MANUAL_CUSTOM'
                ? '自定义方案:保存草稿→maker/checker 审批激活→触发物化(与默认方案同一物化链)。'
                : '服务端将做 preflight 后物化任务与首个 Run;202 仅在同一物化事务提交后返回。'}
            </Form.Item>
          </Form>
        )}
      </div>
    </Modal>
  );
}
