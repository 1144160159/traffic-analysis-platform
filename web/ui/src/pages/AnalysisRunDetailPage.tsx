import { Button, Descriptions, Space, Table, Tag, message } from 'antd';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useNavigate, useParams } from 'react-router-dom';
import {
  analysisQueryKeys,
  cancelAnalysisRun,
  fetchAnalysisRun,
  fetchRunAllowedActions,
  requestAnalysisReport,
  retryAnalysisStage,
  retryAnalysisTask,
  type AnalysisStageView,
} from '@/services/analysisApi';
import { PageStateBoundary } from '@/components/PageStateBoundary';
import { resolvePageState } from '@/components/pageState';
import type { NavRoute } from '@/routes/routeManifest';

/** 运行详情:五轴正交渲染(ENG-UI-004);报告 FAILED 不回退 Run;未知枚举 fail-closed。
 *  §20/§21 allowedActions 服务端驱动:按钮授权来自 GET /runs/{id}/allowed-actions,
 *  前端不自行推断(被拒动作不隐藏,由服务端 403/409 保留审计)。 */
export function AnalysisRunDetailPage({ route }: { route: NavRoute }) {
  const { runId = '' } = useParams();
  const navigate = useNavigate();
  const runQuery = useQuery({
    queryKey: analysisQueryKeys.run(runId),
    queryFn: () => fetchAnalysisRun(runId),
    enabled: Boolean(runId),
    retry: false,
    // 先查 receipt 语义:轮询直到终态,后台不刷
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      const terminal = ['SUCCEEDED', 'PARTIALLY_SUCCEEDED', 'FAILED', 'CANCELLED'].includes(state ?? '');
      return terminal ? false : 5000;
    },
  });

  // 服务端动作授权(与 run 状态同轮询频率)
  const actionsQuery = useQuery({
    queryKey: ['analysis', 'run-actions', runId],
    queryFn: () => fetchRunAllowedActions(runId),
    enabled: Boolean(runId),
    retry: false,
    refetchInterval: (query) => {
      const terminal = ['SUCCEEDED', 'PARTIALLY_SUCCEEDED', 'FAILED', 'CANCELLED'].includes(query.state.data?.state ?? '');
      return terminal ? 15000 : 5000;
    },
  });

  const reportMutation = useMutation({
    mutationFn: () => requestAnalysisReport(runId),
    onSuccess: () => message.success('报告已进入生成队列(独立于运行终态)'),
    onError: (error: Error) => message.error(error.message),
  });

  const cancelMutation = useMutation({
    mutationFn: () => cancelAnalysisRun(runId, `ui-${crypto.randomUUID()}`),
    onSuccess: () => message.success('取消已受理(异步推进)'),
    onError: (error: Error) => message.error(error.message),
  });

  const retryTaskMutation = useMutation({
    mutationFn: () => retryAnalysisTask(runId, `ui-${crypto.randomUUID()}`, 'operator'),
    onSuccess: (data) => {
      message.success('已创建同任务新运行');
      navigate(`/analysis/runs/${data.run_id}`, { replace: true });
    },
    onError: (error: Error) => message.error(error.message),
  });

  // 节点级重试(§76.47.3):仅 actions.retry_stage(服务端判定)+ 该节点 FAILED 时暴露;
  // SHARED_STREAM 节点服务端返回 STAGE_RETRY_UNSUPPORTED(引导整 Run 重试)。
  const retryStageMutation = useMutation({
    mutationFn: (executionNodeId: string) => retryAnalysisStage(runId, executionNodeId, 'operator'),
    onSuccess: (data) => message.success(`节点重试已受理:attempt ${data.Attempt}`),
    onError: (error: Error) => message.error(error.message),
  });

  const run = runQuery.data;
  const actions = actionsQuery.data;

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <PageStateBoundary state={resolvePageState({ isLoading: runQuery.isLoading, data: runQuery.data, error: runQuery.error })}>
        {run && (
          <>
            <Descriptions bordered size="small" column={2}>
              <Descriptions.Item label="Run ID">{run.run_id}</Descriptions.Item>
              <Descriptions.Item label="Task ID">{run.task_id}</Descriptions.Item>
              <Descriptions.Item label="状态(RunState)">
                <Tag>{run.state || '结果待确认'}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="完整性(Completeness)">{run.completeness || '结果待确认'}</Descriptions.Item>
              <Descriptions.Item label="证据完整性(IntegrityState)">{run.integrity_state || '结果待确认'}</Descriptions.Item>
              <Descriptions.Item label="机器结论(FindingConclusion)">{run.finding_conclusion || '结果待确认'}</Descriptions.Item>
              <Descriptions.Item label="风险(RiskSeverity)">
                {run.finding_conclusion === 'THREAT_FOUND' ? (run.risk_severity || '—') : '—'}
              </Descriptions.Item>
              <Descriptions.Item label="报告状态(ReportState)">
                <Tag color={run.report_state === 'AVAILABLE' ? 'green' : run.report_state === 'FAILED' ? 'red' : 'default'}>
                  {run.report_state || 'NOT_REQUESTED'}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="执行规格">{run.execution_spec_sha256.slice(0, 16)}…</Descriptions.Item>
            </Descriptions>

            <Space style={{ marginTop: 16 }}>
              {actions?.cancel && (
                <Button danger loading={cancelMutation.isPending} onClick={() => cancelMutation.mutate()}>
                  取消 Run
                </Button>
              )}
              {actions?.retry_task && (
                <Button loading={retryTaskMutation.isPending} onClick={() => retryTaskMutation.mutate()}>
                  整 Run 重试(同任务新运行)
                </Button>
              )}
              {actions?.request_report && (
                <Button type="primary" loading={reportMutation.isPending} onClick={() => reportMutation.mutate()}>
                  生成人读报告
                </Button>
              )}
            </Space>

            <h2 style={{ marginTop: 24 }}>执行阶段(业务五段投影)</h2>
            <Table<AnalysisStageView>
              rowKey={(r) => `${r.execution_node_id}-${r.attempt}`}
              size="small"
              pagination={false}
              dataSource={run.stages ?? []}
              columns={[
                { title: '阶段', dataIndex: 'business_phase_id', width: 90 },
                { title: '执行节点', dataIndex: 'execution_node_id' },
                { title: 'attempt', dataIndex: 'attempt', width: 90 },
                { title: '状态', dataIndex: 'state', render: (s: string) => <Tag>{s}</Tag> },
                { title: 'provider', dataIndex: 'provider_mode' },
                { title: 'activation', dataIndex: 'activation_mode' },
                { title: 'skip 原因', dataIndex: 'skip_reason', render: (v: string) => v || '—' },
                {
                  title: '操作',
                  key: 'ops',
                  width: 120,
                  render: (_: unknown, st: AnalysisStageView) =>
                    actions?.retry_stage && st.state === 'FAILED' ? (
                      <Button
                        size="small"
                        loading={retryStageMutation.isPending && retryStageMutation.variables === st.execution_node_id}
                        onClick={() => retryStageMutation.mutate(st.execution_node_id)}
                      >
                        重试
                      </Button>
                    ) : '—',
                },
              ]}
            />
          </>
        )}
      </PageStateBoundary>
    </div>
  );
}
