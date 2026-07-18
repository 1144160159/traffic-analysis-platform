import {
  CheckCircleOutlined,
  CloudUploadOutlined,
  CodeSandboxOutlined,
  CloseOutlined,
  DownloadOutlined,
  EyeOutlined,
  FieldTimeOutlined,
  InfoCircleOutlined,
  LinkOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  RollbackOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
  UserOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Drawer, Input, Modal, Radio, Select, Slider, Space, Steps, Switch, Table, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useEffect, useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { DataQualityKpiSparklineChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import {
  createDeployment,
  exportDeploymentEvidence,
  fetchDeploymentWorkbench,
  fetchDeploymentsPage,
  fetchPageSnapshot,
  submitDeploymentAction,
  updateDeploymentScope,
  updateDeploymentWorkflow,
  type DeploymentAction as DeploymentApiAction,
  type DeploymentEvidenceBundle,
  type DeploymentRecord,
  type DeploymentWorkflow,
  type DeploymentWorkbench,
} from '@/services/api';
import { getAuthToken } from '@/services/authStorage';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';

type DeploymentRow = Record<string, unknown> & {
  __deployment_id?: string;
  __deployment?: DeploymentRecord;
};

type DeploymentDialog = {
  kind: 'create' | 'deploy' | 'mutate' | 'view' | 'history' | 'export';
  title: string;
  deploymentId?: string;
  action?: DeploymentApiAction;
};

type DeploymentScopeDraft = {
  tenant: string;
  campus: string;
  probe_group: string;
  asset_group: string;
  percentage: number;
};

type DeploymentWorkflowStage = 'idle' | 'draft_saved' | 'precheck_completed' | 'approval_pending' | 'approved' | 'rejected';

const pageSize = 6;

export function DeploymentManagementPage({ route }: { route: NavRoute }) {
  const visualMode = isVisualBreakdownMode();
  const queryClient = useQueryClient();
  const [selectedKey, setSelectedKey] = useState<string>();
  const [listPage, setListPage] = useState(1);
  const [grayPercent, setGrayPercent] = useState(20);
  const [healthWindow, setHealthWindow] = useState('近 30 分钟');
  const [dialog, setDialog] = useState<DeploymentDialog>();
  const [rollbackReason, setRollbackReason] = useState('误报率升高，经审批执行回滚');
  const [createName, setCreateName] = useState('攻击检测能力生产灰度');
  const [createTrafficStrategy, setCreateTrafficStrategy] = useState('镜像复制（推荐）');
  const [createProbeStrategy, setCreateProbeStrategy] = useState('强制升级');
  const [rollbackTargetId, setRollbackTargetId] = useState('');
  const [rollbackMode, setRollbackMode] = useState('自动回滚');
  const [draftDeploymentId, setDraftDeploymentId] = useState('');
  const [workflowStage, setWorkflowStage] = useState<DeploymentWorkflowStage>('idle');
  const [workflowData, setWorkflowData] = useState<Partial<DeploymentWorkflow>>({});
  const [actionResult, setActionResult] = useState('');

  const visualQuery = useQuery({
    queryKey: ['deployment-visual-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
    enabled: visualMode,
  });
  const listQuery = useQuery({
    queryKey: ['deployments-page', listPage, pageSize],
    queryFn: () => fetchDeploymentsPage({ page: listPage, pageSize }),
    enabled: !visualMode,
  });
  const summaryQuery = useQuery({
    queryKey: ['deployments-summary'],
    queryFn: () => fetchDeploymentsPage({ page: 1, pageSize: 100 }),
    enabled: !visualMode,
  });

  const visualRows = useMemo(() => buildVisualDeploymentRows(visualQuery.data?.rows ?? []), [visualQuery.data?.rows]);
  const apiRows = useMemo(() => (listQuery.data?.items ?? []).map(deploymentToRow), [listQuery.data?.items]);
  const rows = visualMode ? visualRows.slice((listPage - 1) * pageSize, listPage * pageSize) : apiRows;
  const total = visualMode ? visualRows.length : listQuery.data?.total ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const selected = useMemo(() => rows.find((row) => rowKey(row) === selectedKey) ?? rows[0], [rows, selectedKey]);
  const selectedDeployment = selected?.__deployment;
  const selectedDeploymentId = selected?.__deployment_id;
  const selectedStatus = selectedDeployment?.status ?? statusApiValue(String(selected?.状态 ?? 'planned'));
  const identity = readTokenIdentity(getAuthToken());
  const permissions = identity.permissions;
  const { canCreate, canGray, canActivate, canContinue, canEditScope, canPause, canRollback } = deploymentActionAvailability(selectedStatus, permissions);

  useEffect(() => {
    if (!selected || selectedKey === rowKey(selected)) return;
    setSelectedKey(rowKey(selected));
  }, [selected, selectedKey]);

  useEffect(() => {
    const percentage = Number(selectedDeployment?.scope?.percentage);
    if (Number.isFinite(percentage)) setGrayPercent(percentage);
  }, [selectedDeployment]);

  const workbenchQuery = useQuery({
    queryKey: ['deployment-workbench', selectedDeploymentId],
    queryFn: () => fetchDeploymentWorkbench(selectedDeploymentId ?? ''),
    enabled: !visualMode && Boolean(selectedDeploymentId),
  });

  const mutation = useMutation({
    mutationFn: async (current: DeploymentDialog) => {
      if (visualMode) throw new Error('视觉验收模式不提交业务数据');
      if (current.kind === 'create') {
        const source = selectedDeployment ?? summaryQuery.data?.items[0];
        if (!source?.rule_version && !source?.model_version) throw new Error('没有可复用的规则或模型版本');
        return createDeployment({
          name: createName.trim(),
          description: '由部署管理页面创建的生产发布计划',
          rule_version: source.rule_version,
          model_version: source.model_version,
          feature_set_id: source.feature_set_id,
		  scope: {
			...(source.scope ?? {}),
			release_line: source.scope?.release_line ?? inferDeploymentReleaseLine(source),
            percentage: grayPercent,
            source: 'deployment-management-ui',
            target_groups: ['核心数据中心', '办公区集群', 'DMZ 区集群', '容灾中心集群'],
            traffic_copy_strategy: createTrafficStrategy,
            probe_coverage_strategy: createProbeStrategy,
            enable_window: '2026-06-28 02:00 → 2026-06-28 06:00 UTC+08:00',
            soar_playbook: '高危告警处置闭环 v3.1',
            notification_channels: ['钉钉-安全运营', '企业微信-安全运营', '邮件-安全运营组'],
          },
        });
      }
      if (current.kind === 'mutate' && current.action && current.deploymentId) {
		const approvedConfiguration = workflowConfiguration(workflowData);
        return submitDeploymentAction({
          deploymentId: current.deploymentId,
          action: current.action,
          reason: current.action === 'rollback' ? String(approvedConfiguration.reason ?? rollbackReason) : undefined,
          targetDeploymentId: current.action === 'rollback' ? String(approvedConfiguration.target_deployment_id ?? rollbackTargetId) : undefined,
        });
      }
      throw new Error('该操作不需要提交');
    },
    onSuccess: async (result, current) => {
      if (current.kind === 'create' && 'deployment_id' in result) {
        setDraftDeploymentId(String(result.deployment_id));
        setWorkflowStage('draft_saved');
      }
      setActionResult('真实 API 已受理，数据库状态、历史与审计记录已更新。');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['deployments-page'] }),
        queryClient.invalidateQueries({ queryKey: ['deployments-summary'] }),
        queryClient.invalidateQueries({ queryKey: ['deployment-workbench'] }),
      ]);
    },
  });

  const workflowMutation = useMutation({
    mutationFn: ({ stage, operation }: { stage: 'draft' | 'precheck' | 'submit_approval' | 'approve' | 'reject'; operation: 'deploy' | 'rollback' }) => {
	  const deploymentId = operation === 'deploy' ? draftDeploymentId || dialog?.deploymentId || selectedDeploymentId || '' : dialog?.deploymentId ?? selectedDeploymentId ?? '';
	  const configuration = stage === 'approve' || stage === 'reject' ? undefined : operation === 'deploy'
		? { gray_percentage: grayPercent, traffic_copy_strategy: createTrafficStrategy, probe_coverage_strategy: createProbeStrategy, enable_window: '2026-06-28 02:00 → 06:00 UTC+08:00', soar_playbook: '高危告警处置闭环 v3.1', notification_channels: ['钉钉', '企业微信', '邮件'] }
		: { target_deployment_id: rollbackTargetId, reason: rollbackReason, rollback_mode: rollbackMode, traffic_switchback: '优先切回旧版本 Flink job，保留 5 分钟双写观察', rollback_window: '2026-06-28 14:00 → 16:00 UTC+08:00', failure_policy: '自动恢复到 v7 并告警通知', notification_channels: ['飞书-安全运营', '企业微信-检测运营', '短信-值班手机'] };
      return updateDeploymentWorkflow({
        deploymentId,
        stage,
        operation,
		configuration,
      });
    },
    onSuccess: async (result) => {
      const nextStage = String(result.stage);
      if (isDeploymentWorkflowStage(nextStage)) setWorkflowStage(nextStage);
      setWorkflowData(result);
      setActionResult(nextStage === 'approved' ? `审批单 ${String(result.approval_id)} 已批准，可执行真实状态转换。` : nextStage === 'rejected' ? `审批单 ${String(result.approval_id)} 已驳回，可修改后重新检查。` : nextStage === 'approval_pending' ? `审批单 ${String(result.approval_id)} 已持久化并进入待审批。` : nextStage === 'precheck_completed' ? '预检查结果已由真实 API 与数据库证据生成并持久化，可提交审批。' : '操作草案已由真实 API 保存。');
      await queryClient.invalidateQueries({ queryKey: ['deployment-workbench'] });
    },
  });

  const scopeMutation = useMutation({
    mutationFn: (scope: DeploymentScopeDraft) => {
      if (visualMode || !selectedDeploymentId) throw new Error('视觉验收模式不提交业务数据');
      return updateDeploymentScope({ deploymentId: selectedDeploymentId, scope });
    },
    onSuccess: async (result) => {
	  const resetWorkflow = deploymentWorkflow(result.metadata, 'deploy');
	  if (resetWorkflow) {
		setWorkflowData(resetWorkflow);
		const resetStage = String(resetWorkflow.stage ?? '');
		if (isDeploymentWorkflowStage(resetStage)) setWorkflowStage(resetStage);
	  }
	  setActionResult(resetWorkflow?.stage === 'draft_saved' ? '灰度范围已更新，原审批已失效，请重新运行预检查并提交审批。' : '灰度策略已通过真实 API 写入数据库、历史与审计日志。');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['deployments-page'] }),
        queryClient.invalidateQueries({ queryKey: ['deployments-summary'] }),
        queryClient.invalidateQueries({ queryKey: ['deployment-workbench'] }),
      ]);
    },
  });

  const evidenceMutation = useMutation({
    mutationFn: () => {
      if (!selectedDeploymentId) throw new Error('请选择可导出的部署记录');
      return exportDeploymentEvidence(selectedDeploymentId);
    },
    onSuccess: (bundle) => {
      downloadDeploymentEvidence(bundle);
      setActionResult(`证据包 ${bundle.export_id} 已由服务端生成、审计并下载。`);
      openDialog({ kind: 'export', title: '导出发布证据', deploymentId: selectedDeploymentId });
    },
  });

  const dataError = visualMode ? visualQuery.error : listQuery.error ?? summaryQuery.error;
  const isError = visualMode ? visualQuery.isError : listQuery.isError || summaryQuery.isError;
  const isLoading = visualMode ? visualQuery.isLoading : listQuery.isLoading;
  const summaryRecords = visualMode ? [] : summaryQuery.data?.items ?? [];
  const metrics = visualMode
    ? route.page.kpis.map((label) => visualQuery.data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label))
    : buildDeploymentMetrics(route.page.kpis, summaryRecords, total);
  const workbench = visualMode ? visualWorkbench(visualQuery.data) : workbenchQuery.data;

  useEffect(() => {
    const candidates = workbenchItems(workbench, 'rollback_versions')
      .map((item) => item.deployment_id)
      .filter(Boolean)
      .map(String);
    setRollbackTargetId((current) => (candidates.includes(current) ? current : candidates[0] ?? ''));
  }, [selectedDeploymentId, workbench]);

  useEffect(() => {
	if (!dialog || dialog.kind === 'create') return;
	const operation = dialog.kind === 'deploy' ? 'deploy' : dialog.action === 'rollback' ? 'rollback' : undefined;
	if (!operation) return;
	const persisted = deploymentWorkflow(workbench?.deployment?.metadata, operation);
    setWorkflowData(persisted ?? {});
    setWorkflowStage(persisted && isDeploymentWorkflowStage(String(persisted.stage)) ? String(persisted.stage) as DeploymentWorkflowStage : 'idle');
	if (persisted) {
	  const configuration = workflowConfiguration(persisted);
	  if (operation === 'rollback') {
		if (configuration.target_deployment_id) setRollbackTargetId(String(configuration.target_deployment_id));
		if (configuration.reason) setRollbackReason(String(configuration.reason));
		if (configuration.rollback_mode) setRollbackMode(String(configuration.rollback_mode));
	  } else if (Number.isFinite(Number(configuration.gray_percentage))) {
		setGrayPercent(Number(configuration.gray_percentage));
	  }
	}
  }, [dialog, selectedDeploymentId, workbench]);

  const openDialog = (next: DeploymentDialog) => {
    setActionResult('');
    mutation.reset();
    workflowMutation.reset();
    if (next.deploymentId) setSelectedKey(next.deploymentId);
    if (next.kind === 'create') {
      setDraftDeploymentId('');
      setWorkflowStage('idle');
      setWorkflowData({});
	} else if (next.kind === 'deploy') {
	  setDraftDeploymentId(next.deploymentId ?? '');
	  setWorkflowStage('idle');
	  setWorkflowData({});
	} else if (next.action === 'rollback') {
      setWorkflowStage('idle');
      setWorkflowData({});
    }
    setDialog(next);
  };

  const openSelectedMutation = (title: string, action: DeploymentApiAction) => {
    if (!selectedDeploymentId && !visualMode) return;
    openDialog({ kind: 'mutate', title, action, deploymentId: selectedDeploymentId ?? 'visual' });
  };

  const continueDeployment = () => {
    if (selectedStatus === 'paused') openSelectedMutation('继续灰度', 'resume');
    else if (selectedStatus === 'gray') openSelectedMutation('全量发布', 'activate');
	else if (selectedDeploymentId) openDialog({ kind: 'deploy', title: '部署审批', action: 'gray', deploymentId: selectedDeploymentId });
  };

  const exportEvidence = () => {
    if (!selected) return;
    if (visualMode) {
      setActionResult('视觉验收模式仅预览证据包，不提交导出审计。');
      openDialog({ kind: 'export', title: '导出发布证据', deploymentId: 'visual' });
      return;
    }
    evidenceMutation.mutate();
  };

  const modalDialog = dialog && (dialog.kind === 'create' || dialog.kind === 'deploy' || dialog.action === 'rollback') ? dialog : undefined;
  const drawerDialog = dialog && dialog !== modalDialog ? dialog : undefined;

  const columns: ColumnsType<DeploymentRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value, record) => renderDeploymentCell(column, value, record, openDialog, permissions),
  }));

  const modalIsDeploy = modalDialog?.kind === 'create' || modalDialog?.kind === 'deploy';
  const modalOperation = modalIsDeploy ? 'deploy' : 'rollback';
  const requestedBy = String(workflowData.requested_by ?? '');
  const canApproveWorkflow = hasDeployScope(permissions, 'deploy:approve') && Boolean(identity.userId) && identity.userId !== requestedBy;
  const configurationLocked = workflowStage === 'approval_pending' || workflowStage === 'approved';
  const modalReady = modalIsDeploy ? Boolean((draftDeploymentId || modalDialog?.deploymentId) && (modalDialog?.kind === 'deploy' || createName.trim())) : Boolean((modalDialog?.deploymentId ?? selectedDeploymentId) && rollbackTargetId && rollbackReason.trim().length >= 10);
  const executeApprovedWorkflow = () => {
    if (!modalDialog || workflowStage !== 'approved') return;
	if (modalIsDeploy && (draftDeploymentId || modalDialog.deploymentId)) mutation.mutate({ kind: 'mutate', title: '启动灰度', action: 'gray', deploymentId: draftDeploymentId || modalDialog.deploymentId });
    else if (modalDialog.action === 'rollback' && modalDialog.deploymentId) mutation.mutate({ kind: 'mutate', title: '执行回滚', action: 'rollback', deploymentId: modalDialog.deploymentId });
  };

  return (
    <div className="taf-page taf-deployments">
      <section className="taf-deployments-shell">
        <main className="taf-deployments-main">
          <div className="taf-deployments-grid">
            <section className="taf-deployments-primary">
              <header className="taf-deployments-titlebar">
                <div><h1>{route.page.title}</h1><span>规则、模型、采集策略、Flink 作业和配置的发布与回滚</span></div>
                <Space size={6} wrap>
                  <Button size="small" type="primary" icon={<CloudUploadOutlined />} disabled={!canCreate} onClick={() => openDialog({ kind: 'create', title: '新建发布' })}>新建发布</Button>
                  <Button size="small" icon={<PlayCircleOutlined />} disabled={!selected || !canContinue} onClick={continueDeployment}>继续灰度</Button>
                  <Button size="small" danger ghost icon={<StopOutlined />} disabled={!selected || !canPause || !canActivate} onClick={() => openSelectedMutation('停止灰度', 'pause')}>停止灰度</Button>
                  <Button size="small" danger icon={<RollbackOutlined />} disabled={!selected || !canRollback} onClick={() => openSelectedMutation('快速回滚', 'rollback')}>快速回滚</Button>
                  <Button size="small" icon={<DownloadOutlined />} loading={evidenceMutation.isPending} disabled={!selected} onClick={exportEvidence}>导出证据</Button>
                  <Tooltip title="刷新发布状态"><Button size="small" aria-label="刷新发布状态" icon={<ReloadOutlined />} onClick={() => void (visualMode ? visualQuery.refetch() : Promise.all([listQuery.refetch(), summaryQuery.refetch(), workbenchQuery.refetch()]))} /></Tooltip>
                </Space>
              </header>

              {isError && <Alert type="error" showIcon message="真实 API 数据加载失败" description={dataError instanceof Error ? dataError.message : '请检查 /v1/deployments、APISIX 路由或 rule-manager。'} action={<Button size="small" danger onClick={() => void listQuery.refetch()}>重试</Button>} />}

              <div className="taf-deployments-kpis">{metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}</div>

              <WorkPanel title="发布清单" className="taf-deployments-list-panel">
                <Table rowKey={rowKey} size="small" loading={isLoading} pagination={false} columns={columns} dataSource={rows} scroll={{ x: 900 }} rowSelection={{ selectedRowKeys: selected ? [rowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }} onRow={(record) => ({ onClick: () => setSelectedKey(rowKey(record)) })} />
                <div className="taf-deployments-pagination"><span>共 {total} 条</span><button type="button" aria-label="发布清单上一页" disabled={listPage === 1} onClick={() => setListPage((page) => Math.max(1, page - 1))}>‹</button>{paginationWindow(pageCount, listPage).map((page) => <button key={page} type="button" className={page === listPage ? 'is-active' : ''} aria-current={page === listPage ? 'page' : undefined} aria-label={`发布清单第 ${page} 页`} onClick={() => setListPage(page)}>{page}</button>)}<button type="button" aria-label="发布清单下一页" disabled={listPage === pageCount} onClick={() => setListPage((page) => Math.min(pageCount, page + 1))}>›</button><span>{pageSize} 条/页</span></div>
              </WorkPanel>
            </section>

            <aside className="taf-deployments-rail">
              <WorkPanel title="灰度策略" extra={<Select size="small" value="按租户+园区+探针分级" options={[{ value: '按租户+园区+探针分级' }, { value: '按资产重要性分级' }]} />}>
                <GrayStrategy selected={selected} grayPercent={grayPercent} onGrayPercentChange={setGrayPercent} disabled={!canEditScope} saving={scopeMutation.isPending} error={scopeMutation.error} savedMessage={scopeMutation.isSuccess ? actionResult : ''} onApply={(scope) => visualMode ? setActionResult('视觉验收模式不提交灰度策略。') : scopeMutation.mutate(scope)} />
              </WorkPanel>
              <WorkPanel title="发布健康" extra={<Select size="small" value={healthWindow} options={[{ value: '近 30 分钟' }, { value: '近 2 小时' }]} onChange={setHealthWindow} />}>
                <ReleaseHealth items={workbenchItems(workbench, 'health')} grayPercent={grayPercent} healthWindow={healthWindow} />
              </WorkPanel>
            </aside>
          </div>

          <div className="taf-deployments-bottom">
			<WorkPanel title="版本对比 / 变更摘要"><VersionDiff selected={selected} items={workbenchItems(workbench, 'change_summary')} /></WorkPanel>
			<WorkPanel title="回滚管理"><RollbackManager items={workbenchItems(workbench, 'rollback_versions')} reason={rollbackReason} disabled={!canRollback} onReasonChange={setRollbackReason} onRollback={(targetId) => { setRollbackTargetId(targetId); openSelectedMutation('执行回滚', 'rollback'); }} /></WorkPanel>
            <WorkPanel title="发布证据"><ReleaseEvidence items={workbenchItems(workbench, 'evidence')} onExport={exportEvidence} /></WorkPanel>
          </div>
        </main>
      </section>

	  <Modal className="taf-deployments-operation-modal" title={modalDialog ? <div className="taf-deployments-modal-title"><strong>{modalIsDeploy ? (modalDialog.kind === 'create' ? '创建部署' : '部署审批') : '部署回滚确认'}</strong><span>{modalIsDeploy ? '检测能力发布 / 规则包 + 模型包 + SOAR 剧本联动' : `${String(selected?.发布对象 ?? '当前发布')} / 规则包与模型包回退`}</span></div> : '部署操作确认'} open={Boolean(modalDialog)} width="min(960px, calc(100vw - 80px))" onCancel={() => setDialog(undefined)} footer={modalDialog ? [
        <Button key="cancel" onClick={() => setDialog(undefined)}>取消</Button>,
		<Button key="save" loading={mutation.isPending || workflowMutation.isPending} disabled={visualMode || configurationLocked || (modalIsDeploy ? !createName.trim() || !canCreate : !rollbackTargetId || rollbackReason.trim().length < 10 || !canRollback)} onClick={() => modalDialog.kind === 'create' && !draftDeploymentId ? mutation.mutate(modalDialog) : workflowMutation.mutate({ stage: 'draft', operation: modalOperation })}>{modalIsDeploy ? '保存草案' : '保存回滚单'}</Button>,
		<Button key="check" type="primary" ghost loading={workflowMutation.isPending} disabled={visualMode || !modalReady || !['draft_saved', 'precheck_completed'].includes(workflowStage) || (modalIsDeploy ? !canCreate : !canRollback)} onClick={() => workflowMutation.mutate({ stage: 'precheck', operation: modalOperation })}>{modalIsDeploy ? '运行预检查' : '运行回滚检查'}</Button>,
		workflowStage === 'approval_pending' && !canApproveWorkflow && <Button key="waiting" disabled>等待其他审批人</Button>,
		workflowStage === 'approval_pending' && canApproveWorkflow && <Button key="reject" danger ghost loading={workflowMutation.isPending} disabled={visualMode} onClick={() => workflowMutation.mutate({ stage: 'reject', operation: modalOperation })}>驳回审批</Button>,
		workflowStage === 'approval_pending' && canApproveWorkflow && <Button key="approve" type="primary" loading={workflowMutation.isPending} disabled={visualMode} onClick={() => workflowMutation.mutate({ stage: 'approve', operation: modalOperation })}>批准审批</Button>,
		workflowStage !== 'approval_pending' && workflowStage !== 'approved' && <Button key="submit" type="primary" danger={modalDialog.action === 'rollback'} loading={workflowMutation.isPending} disabled={visualMode || workflowStage !== 'precheck_completed' || !modalReady || (modalIsDeploy ? !canCreate : !canRollback)} onClick={() => workflowMutation.mutate({ stage: 'submit_approval', operation: modalOperation })}>{visualMode ? '视觉模式不可提交' : modalDialog.action === 'rollback' ? '提交回滚审批' : '提交部署审批'}</Button>,
		workflowStage === 'approved' && <Button key="execute" type="primary" danger={modalDialog.action === 'rollback'} loading={mutation.isPending} disabled={visualMode || !modalReady || (modalIsDeploy ? !canGray : !canRollback)} title={modalIsDeploy && !canGray ? '当前身份缺少 deploy:gray 执行权限' : !modalIsDeploy && !canRollback ? '当前身份缺少 deploy:rollback 执行权限' : undefined} onClick={executeApprovedWorkflow}>{modalIsDeploy ? (canGray ? '启动灰度' : '已批准（需 deploy:gray）') : (canRollback ? '执行回滚' : '已批准（需 deploy:rollback）')}</Button>,
      ] : []}>
		{modalDialog && <DeploymentDialogBody dialog={modalDialog} selected={selected} workbench={workbench} visualMode={visualMode} configurationLocked={configurationLocked} createName={createName} onCreateNameChange={setCreateName} createTrafficStrategy={createTrafficStrategy} onCreateTrafficStrategyChange={setCreateTrafficStrategy} createProbeStrategy={createProbeStrategy} onCreateProbeStrategyChange={setCreateProbeStrategy} grayPercent={grayPercent} onGrayPercentChange={setGrayPercent} rollbackReason={rollbackReason} onRollbackReasonChange={setRollbackReason} rollbackTargetId={rollbackTargetId} onRollbackTargetIdChange={setRollbackTargetId} rollbackMode={rollbackMode} onRollbackModeChange={setRollbackMode} workflowStage={workflowStage} workflowData={workflowData} actionResult={actionResult} error={mutation.error ?? workflowMutation.error} />}
      </Modal>
      <Drawer className="taf-deployments-action-drawer" title={drawerDialog ? `${drawerDialog.title}确认` : '发布操作确认'} open={Boolean(drawerDialog)} width="min(560px, calc(var(--taf-window-inner-width, 100dvw) - 40px))" onClose={() => setDialog(undefined)} extra={drawerDialog?.kind === 'mutate' ? <Button size="small" type="primary" loading={mutation.isPending} disabled={visualMode || !deploymentActionAllowed(drawerDialog.action, selectedStatus, permissions)} onClick={() => mutation.mutate(drawerDialog)}>{visualMode ? '视觉模式不可提交' : mutation.isPending ? '提交中' : '确认提交'}</Button> : undefined}>
        {drawerDialog && <DeploymentDialogBody dialog={drawerDialog} selected={selected} workbench={workbench} visualMode={visualMode} configurationLocked={false} createName={createName} onCreateNameChange={setCreateName} createTrafficStrategy={createTrafficStrategy} onCreateTrafficStrategyChange={setCreateTrafficStrategy} createProbeStrategy={createProbeStrategy} onCreateProbeStrategyChange={setCreateProbeStrategy} grayPercent={grayPercent} onGrayPercentChange={setGrayPercent} rollbackReason={rollbackReason} onRollbackReasonChange={setRollbackReason} rollbackTargetId={rollbackTargetId} onRollbackTargetIdChange={setRollbackTargetId} rollbackMode={rollbackMode} onRollbackModeChange={setRollbackMode} workflowStage={workflowStage} workflowData={workflowData} actionResult={actionResult} error={mutation.error ?? evidenceMutation.error} />}
      </Drawer>
    </div>
  );
}

function DeploymentDialogBody({ dialog, selected, workbench, visualMode, configurationLocked, createName, onCreateNameChange, createTrafficStrategy, onCreateTrafficStrategyChange, createProbeStrategy, onCreateProbeStrategyChange, grayPercent, onGrayPercentChange, rollbackReason, onRollbackReasonChange, rollbackTargetId, onRollbackTargetIdChange, rollbackMode, onRollbackModeChange, workflowStage, workflowData, actionResult, error }: { dialog: DeploymentDialog; selected?: DeploymentRow; workbench?: DeploymentWorkbench; visualMode: boolean; configurationLocked: boolean; createName: string; onCreateNameChange: (value: string) => void; createTrafficStrategy: string; onCreateTrafficStrategyChange: (value: string) => void; createProbeStrategy: string; onCreateProbeStrategyChange: (value: string) => void; grayPercent: number; onGrayPercentChange: (value: number) => void; rollbackReason: string; onRollbackReasonChange: (value: string) => void; rollbackTargetId: string; onRollbackTargetIdChange: (value: string) => void; rollbackMode: string; onRollbackModeChange: (value: string) => void; workflowStage: DeploymentWorkflowStage; workflowData: Partial<DeploymentWorkflow>; actionResult: string; error: Error | null }) {
  if (dialog.kind === 'create' || dialog.kind === 'deploy') return <DeploymentCreateDialog selected={selected} workbench={workbench} visualMode={visualMode} configurationLocked={configurationLocked} createName={createName} onCreateNameChange={onCreateNameChange} createTrafficStrategy={createTrafficStrategy} onCreateTrafficStrategyChange={onCreateTrafficStrategyChange} createProbeStrategy={createProbeStrategy} onCreateProbeStrategyChange={onCreateProbeStrategyChange} grayPercent={grayPercent} onGrayPercentChange={onGrayPercentChange} workflowStage={workflowStage} workflowData={workflowData} actionResult={actionResult} error={error} />;
  if (dialog.action === 'rollback') return <DeploymentRollbackDialog selected={selected} workbench={workbench} visualMode={visualMode} configurationLocked={configurationLocked} rollbackReason={rollbackReason} onRollbackReasonChange={onRollbackReasonChange} rollbackTargetId={rollbackTargetId} onRollbackTargetIdChange={onRollbackTargetIdChange} rollbackMode={rollbackMode} onRollbackModeChange={onRollbackModeChange} workflowStage={workflowStage} workflowData={workflowData} actionResult={actionResult} error={error} />;
  return <div className="taf-alert-detail-action-body">
    <dl><dt>发布对象</dt><dd>{String(selected?.发布对象 ?? '新建发布计划')}</dd><dt>数据来源</dt><dd>{workbench?.source ?? 'PostgreSQL / Rule Manager API'}</dd><dt>目标状态</dt><dd>{dialog.action ? deploymentActionLabel(dialog.action) : dialog.title}</dd></dl>
    {dialog.kind === 'history' && <div className="taf-deployments-dialog-history">{(workbench?.history ?? []).map((item) => <span key={item.id}><b>{deploymentHistoryLabel(item.action)}</b><em>{formatDeploymentTime(item.created_at)}</em><i>{item.operator_id}</i></span>)}</div>}
    {dialog.kind === 'view' && selected && <pre>{JSON.stringify(selected.__deployment ?? selected, null, 2)}</pre>}
    {actionResult && <Alert type="success" showIcon message={actionResult} />}
    {error && <Alert type="error" showIcon message="真实 API 操作失败" description={error.message} />}
  </div>;
}

function DeploymentCreateDialog({ selected, workbench, visualMode, configurationLocked, createName, onCreateNameChange, createTrafficStrategy, onCreateTrafficStrategyChange, createProbeStrategy, onCreateProbeStrategyChange, grayPercent, onGrayPercentChange, workflowStage, workflowData, actionResult, error }: { selected?: DeploymentRow; workbench?: DeploymentWorkbench; visualMode: boolean; configurationLocked: boolean; createName: string; onCreateNameChange: (value: string) => void; createTrafficStrategy: string; onCreateTrafficStrategyChange: (value: string) => void; createProbeStrategy: string; onCreateProbeStrategyChange: (value: string) => void; grayPercent: number; onGrayPercentChange: (value: number) => void; workflowStage: DeploymentWorkflowStage; workflowData: Partial<DeploymentWorkflow>; actionResult: string; error: Error | null }) {
  const evidence = workbenchItems(workbench, 'evidence');
  const precheckRows = workflowPrecheckRows(workflowData);
  const deployment = selected?.__deployment;
  const version = String(selected?.版本 ?? deployment?.rule_version ?? 'v2.3.1');
  return <div className="taf-deployments-modal-workspace is-create">
    <div className="taf-deployments-modal-statusbar"><span><InfoCircleOutlined className="is-info" />{workflowStage === 'idle' ? '草案未保存' : '草案已保存'}</span><span>{workflowStage === 'approved' || workflowStage === 'precheck_completed' || workflowStage === 'approval_pending' ? <CheckCircleOutlined className="is-ok" /> : workflowStage === 'rejected' ? <WarningOutlined className="is-risk" /> : <InfoCircleOutlined className="is-warn" />}{deploymentWorkflowStageLabel(workflowStage, 'deploy')}</span><span><CheckCircleOutlined className="is-ok" />目标 4 个部署集</span></div>
    <section className="taf-deployments-modal-column">
      <div className="taf-deployments-modal-card"><h3>部署配置</h3><div className="taf-deployments-modal-form">
		<label><span className="is-required">部署名称</span><Input aria-required="true" disabled={configurationLocked} value={createName} maxLength={60} onChange={(event) => onCreateNameChange(event.target.value)} /></label>
		<label><span>能力包版本（只读）</span><Select disabled value={`attack-detect ${version}`} options={[{ value: `attack-detect ${version}` }]} /></label>
		<label><span>规则包版本（只读）</span><div className="taf-deployments-inline-action"><Select disabled value={deployment?.rule_version ?? version} options={[{ value: deployment?.rule_version ?? version }]} /><Button disabled title="当前发布页仅展示审批快照" size="small">查看变更</Button></div></label>
		<label><span>模型包版本（只读）</span><div className="taf-deployments-inline-action"><Select disabled value={deployment?.model_version ?? 'model-ids-v1.9.3'} options={[{ value: deployment?.model_version ?? 'model-ids-v1.9.3' }]} /><Button disabled title="当前发布页仅展示审批快照" size="small">模型评估报告</Button></div></label>
        <label><span>目标部署集</span><div className="taf-deployments-modal-tags"><span>核心数据中心</span><span>办公区集群</span><span>DMZ 区集群</span><span>容灾中心集群</span></div></label>
		<label><span>灰度比例</span><Slider disabled={configurationLocked} min={5} max={100} marks={{ 5: '5%', 10: '10%', 20: '20%', 50: '50%', 100: '100%' }} value={grayPercent} onChange={(value) => onGrayPercentChange(Number(value))} /></label>
		<label><span>启用时间窗</span><div className="taf-deployments-time-range"><Input value="2026-06-28 02:00" readOnly /><FieldTimeOutlined /><Input value="2026-06-28 06:00" readOnly /><Select disabled value="UTC+08:00" options={[{ value: 'UTC+08:00' }]} /></div></label>
		<label><span>流量复制策略</span><Radio.Group disabled={configurationLocked} value={createTrafficStrategy} onChange={(event) => onCreateTrafficStrategyChange(event.target.value)} options={['镜像复制（推荐）', '采样复制', '不复制（仅元数据）']} /></label>
		<label><span>探针覆盖策略</span><Radio.Group disabled={configurationLocked} className="taf-deployments-segmented" value={createProbeStrategy} onChange={(event) => onCreateProbeStrategyChange(event.target.value)} optionType="button" buttonStyle="solid" options={['继承现有', '强制升级', '不覆盖']} /></label>
		<label><span>SOAR 剧本联动</span><div className="taf-deployments-soar-control"><Switch disabled defaultChecked /><span>联动剧本（审批计划）</span><Select disabled value="高危告警处置闭环 v3.1" options={[{ value: '高危告警处置闭环 v3.1' }]} /><Button disabled title="当前发布页仅展示审批快照" type="link" size="small" icon={<LinkOutlined />}>预览</Button></div></label>
		<label><span>通知渠道（审批计划）</span><RemovableTags disabled items={['钉钉-安全运营', '企业微信-安全运营', '邮件-安全运营组']} /></label>
      </div></div>
      <div className="taf-deployments-modal-card"><h3>发布编排预览</h3><Steps className="taf-deployments-release-flow" size="small" current={deploymentWorkflowStep(workflowStage)} items={[[<CodeSandboxOutlined />, '创建发布单'], [<SafetyCertificateOutlined />, '预检查'], [<CloudUploadOutlined />, '灰度部署'], [<FieldTimeOutlined />, '观测窗口'], [<PlayCircleOutlined />, '全量发布'], [<DownloadOutlined />, '归档证据']].map(([icon, title]) => ({ icon, title }))} /></div>
    </section>
    <section className="taf-deployments-modal-column">
		<div className="taf-deployments-modal-card"><h3>预检查矩阵</h3><div className="taf-deployments-precheck"><div><span>检查项</span><span>状态</span><span>说明</span></div>{(precheckRows.length ? precheckRows : visualMode ? (evidence.length ? evidence : fallbackEvidenceItems).map((item, index) => ({ label: item.label, status: index === 2 ? '警告' : item.status, evidence: index === 2 ? '部分 Flink Job 需滚动重启' : '依赖与目标环境校验通过' })) : [{ label: '预检查', status: '待运行', evidence: '保存草案后运行预检查，结果将由数据库证据生成' }]).slice(0, 7).map((item) => { const fullEvidence = `${String(item.evidence ?? '—')}${'freshness' in item && item.freshness ? ` · ${item.freshness}` : ''}`; return <div key={String(item.label)}><b>{String(item.label)}</b><StatusTag value={item.status} /><span title={fullEvidence}>{fullEvidence}</span></div>; })}</div></div>
      <div className="taf-deployments-modal-card"><h3>影响范围（预计）</h3><div className="taf-deployments-impact-table"><div><span>部署集</span><span>资产组</span><span>Flink Job</span><span>规则数</span><span>预计新增告警/日</span></div>{[['核心数据中心','服务器/核心','24','186','↑ 320'],['办公区集群','办公网段','18','132','↑ 210'],['DMZ 区集群','DMZ 业务','8','74','↑ 95'],['容灾中心集群','容灾网段','10','68','↑ 70']].map((row) => <div key={row[0]}>{row.map((cell) => <span key={cell}>{cell}</span>)}</div>)}</div></div>
		<div className="taf-deployments-modal-split"><div className="taf-deployments-modal-card"><h3>回滚策略</h3><p>回滚版本　{version}</p><p>触发阈值　误报率 &gt; 2% 或告警突增 &gt; 50%</p><p>最长观察窗口　60 分钟　<Switch disabled size="small" defaultChecked /> 失败自动回滚（审批计划）</p></div><div className="taf-deployments-modal-card"><h3>权限与审批</h3><Steps className="taf-deployments-approval-chain is-compact" size="small" current={workflowStage === 'approved' ? 1 : 0} items={[{ title: '申请人', description: String(workflowData.requested_by ?? '待提交'), icon: <UserOutlined /> }, { title: '独立审批人', description: String(workflowData.approved_by ?? '待审批'), icon: <UserOutlined /> }]} /></div></div>
		<div className="taf-deployments-modal-card"><h3>审计留痕</h3><div className="taf-deployments-audit-grid"><span>申请人　<b><UserOutlined /> {String(workflowData.requested_by ?? '待提交')}</b></span><span>审批角色　<b>独立发布审批人（不可与申请人相同）</b></span><span>变更摘要　<b>规则包、模型包与 {grayPercent}% 灰度策略发布到 4 个部署集</b></span><span>审批快照　<b className="is-link" title={String(workflowData.approval_snapshot_hash ?? '')}>{shortHash(workflowData.approval_snapshot_hash)}</b></span></div></div>
    </section>
    <div className="taf-deployments-modal-risk is-warning">预检查通过前不可启动灰度部署；提交后将生成审批单、审计记录与回滚证据。</div>
    {(actionResult || error) && <Alert className="taf-deployments-modal-alert" type={error ? 'error' : 'success'} showIcon message={error?.message ?? actionResult} />}
  </div>;
}

function DeploymentRollbackDialog({ selected, workbench, visualMode, configurationLocked, rollbackReason, onRollbackReasonChange, rollbackTargetId, onRollbackTargetIdChange, rollbackMode, onRollbackModeChange, workflowStage, workflowData, actionResult, error }: { selected?: DeploymentRow; workbench?: DeploymentWorkbench; visualMode: boolean; configurationLocked: boolean; rollbackReason: string; onRollbackReasonChange: (value: string) => void; rollbackTargetId: string; onRollbackTargetIdChange: (value: string) => void; rollbackMode: string; onRollbackModeChange: (value: string) => void; workflowStage: DeploymentWorkflowStage; workflowData: Partial<DeploymentWorkflow>; actionResult: string; error: Error | null }) {
  const versions = workbenchItems(workbench, 'rollback_versions');
  const versionOptions = versions.slice(0, 3).map((item) => ({ value: String(item.deployment_id ?? ''), ruleLabel: `规则包 ${String(item.rule_version ?? item.version ?? '未知版本')}`, modelLabel: `模型包 ${String(item.model_version ?? item.version ?? '未知版本')}` })).filter((item) => item.value);
  const precheckRows = workflowPrecheckRows(workflowData);
  return <div className="taf-deployments-modal-workspace is-rollback">
    <div className="taf-deployments-modal-statusbar"><span>{workflowStage === 'approved' ? <CheckCircleOutlined className="is-ok" /> : workflowStage === 'rejected' ? <WarningOutlined className="is-risk" /> : workflowStage === 'approval_pending' ? <InfoCircleOutlined className="is-warn" /> : <InfoCircleOutlined className="is-info" />}{deploymentWorkflowStageLabel(workflowStage, 'rollback')}</span><span><WarningOutlined className="is-risk" />高风险操作</span><span><InfoCircleOutlined className="is-info" />影响 4 个部署集</span></div>
    <section className="taf-deployments-modal-column">
      <div className="taf-deployments-modal-card"><h3>回滚配置</h3><div className="taf-deployments-modal-form">
        <label><span>回滚对象</span><Input value={String(selected?.发布对象 ?? '当前灰度发布')} readOnly /></label>
		<label><span className="is-required">目标版本</span><div className="taf-deployments-target-pair"><Select disabled={configurationLocked} aria-required="true" aria-label="目标规则包版本" value={rollbackTargetId} options={versionOptions.map((item) => ({ value: item.value, label: item.ruleLabel }))} onChange={onRollbackTargetIdChange} /><Select disabled={configurationLocked} aria-required="true" aria-label="目标模型包版本" value={rollbackTargetId} options={versionOptions.map((item) => ({ value: item.value, label: item.modelLabel }))} onChange={onRollbackTargetIdChange} /></div></label>
        <label><span>回滚范围</span><div className="taf-deployments-modal-tags"><span>核心数据中心</span><span>办公区集群</span><span>DMZ 区集群</span><span>容灾中心集群</span></div></label>
		<label><span>回滚方式</span><Radio.Group disabled={configurationLocked} className="taf-deployments-rollback-modes" value={rollbackMode} onChange={(event) => onRollbackModeChange(event.target.value)} options={['自动回滚', '手动回滚', '分批回滚']} /></label>
		<label><span>流量切回策略（审批计划）</span><Select disabled value="优先切回旧版本 Flink job，保留 5 分钟双写观察" options={[{ value: '优先切回旧版本 Flink job，保留 5 分钟双写观察' }]} /></label>
		<label><span>回滚窗口（审批计划）</span><Select disabled value="2026-06-28 14:00 → 16:00（UTC+08:00）" options={[{ value: '2026-06-28 14:00 → 16:00（UTC+08:00）' }, { value: '立即执行' }]} /></label>
		<label><span>失败处理策略（审批计划）</span><Select disabled value="自动恢复到 v7 并告警通知" options={[{ value: '自动恢复到 v7 并告警通知' }, { value: '阻断并等待人工确认' }]} /></label>
		<label><span>通知渠道（审批计划）</span><RemovableTags disabled items={['飞书-安全运营', '企业微信-检测运营', '短信-值班手机']} /></label>
      </div></div>
      <div className="taf-deployments-modal-card"><h3>回滚前检查矩阵</h3><div className="taf-deployments-precheck is-rollback"><div><span>检查项</span><span>状态</span><span>证据</span><span>建议</span></div>{(precheckRows.length ? precheckRows : visualMode ? [
        ['当前健康','已通过','当前错误率 0.21%，CPU 48%，延迟 120ms','继续回滚'],
        ['版本兼容','已通过','规则/模型向后兼容，接口无破坏性变更','可回滚'],
        ['状态快照','已通过','已生成状态快照 20260628-135501','保留快照'],
        ['Flink checkpoint','已通过','最新 checkpoint 2026-06-28 13:55:18','可用于状态恢复'],
        ['Kafka offset','警告','部分 Topic 延迟 1.2 万条','回滚前同步一次 offset'],
        ['模型依赖','警告','2 个模型依赖新特征（可降级）','降级或忽略特征'],
        ['SOAR 剧本','已通过','关联剧本版本兼容','无需变更'],
		].map(([label, status, evidenceValue, recommendation]) => ({ label, status, evidence: evidenceValue, recommendation })) : [{ label: '回滚检查', status: '待运行', evidence: '保存回滚单后运行检查', recommendation: '等待真实证据' }]).map((item) => { const fullEvidence = `${String(item.evidence ?? '—')}${'freshness' in item && item.freshness ? ` · ${item.freshness}` : ''}`; return <div key={String(item.label)}><b>{String(item.label)}</b><StatusTag value={item.status} /><span title={fullEvidence}>{fullEvidence}</span><span>{String(item.recommendation ?? '—')}</span></div>; })}</div></div>
    </section>
    <section className="taf-deployments-modal-column">
      <div className="taf-deployments-modal-card"><h3>影响范围</h3><div className="taf-deployments-impact-table"><div><span>部署集</span><span>资产组</span><span>Flink Job</span><span>规则数量</span><span>预计告警变化</span></div>{[['核心数据中心','服务器/核心','job-core-01','186','↑ 28%'],['办公区集群','办公终端','job-office-01','132','↑ 15%'],['DMZ 区集群','DMZ 业务','job-dmz-01','74','↓ 35%'],['容灾中心集群','物联网设备','job-dr-01','58','→ 0%'],['合计','—','4','450','↑ 8%']].map((row, index) => <div key={row[0]} className={index === 4 ? 'is-total' : ''}>{row.map((cell) => <span key={cell}>{cell}</span>)}</div>)}</div></div>
      <div className="taf-deployments-modal-card"><h3>观测窗口与回滚策略</h3><p>观测窗口　30 分钟灰度观察（仅目标部署集）</p><p>双写观察　回滚前保留双写观察 5 分钟，确保规则、模型与 offset 一致</p><p>失败自动恢复阈值　错误率 &gt; 2% 或告警突增 &gt; 50% 或延迟 &gt; 500ms</p><p>回滚后对比指标　告警量、误报率、延迟、Flink 延迟、学习任务失败率</p></div>
		<div className="taf-deployments-modal-card"><h3>审批链</h3><Steps className="taf-deployments-approval-chain" size="small" current={workflowStage === 'approved' ? 1 : 0} items={[{ title: '申请人', description: String(workflowData.requested_by ?? '待提交'), icon: <UserOutlined /> }, { title: '独立审批人', description: String(workflowData.approved_by ?? '待审批'), icon: <UserOutlined /> }]} /></div>
		<div className="taf-deployments-modal-card"><h3>审计留痕</h3><div className="taf-deployments-audit-grid is-rollback"><span>申请人　<b><UserOutlined /> {String(workflowData.requested_by ?? '待提交')}</b></span><span>审批角色　<b>独立回滚审批人（不可与申请人相同）</b></span><label><span className="is-required">回滚原因</span><Input.TextArea aria-required="true" disabled={configurationLocked} value={rollbackReason} maxLength={200} rows={2} placeholder="请填写回滚原因（必填，不少于 10 个字）" onChange={(event) => onRollbackReasonChange(event.target.value)} /></label><span>审批快照　<b className="is-link" title={String(workflowData.approval_snapshot_hash ?? '')}>{shortHash(workflowData.approval_snapshot_hash)}</b></span></div></div>
    </section>
    <div className="taf-deployments-modal-risk is-danger">回滚会停止当前灰度发布并恢复上一版本，可能影响检测覆盖，请确认影响范围、审批链和恢复窗口。</div>
    {(actionResult || error) && <Alert className="taf-deployments-modal-alert" type={error ? 'error' : 'success'} showIcon message={error?.message ?? actionResult} />}
  </div>;
}

function RemovableTags({ items, disabled = false }: { items: string[]; disabled?: boolean }) {
  const [visibleItems, setVisibleItems] = useState(items);
  return <div className="taf-deployments-modal-tags is-control">{visibleItems.map((item) => <button key={item} type="button" disabled={disabled} aria-label={`${disabled ? '审批计划通知渠道' : '移除通知渠道'} ${item}`} onClick={() => setVisibleItems((current) => current.filter((value) => value !== item))}>{item}{!disabled && <CloseOutlined />}</button>)}</div>;
}

function deploymentWorkflowStep(stage: DeploymentWorkflowStage) {
  if (stage === 'idle' || stage === 'draft_saved') return 0;
  if (stage === 'precheck_completed' || stage === 'approval_pending' || stage === 'rejected') return 1;
  return 2;
}

function GrayStrategy({ selected, grayPercent, onGrayPercentChange, onApply, disabled, saving, error, savedMessage }: { selected?: DeploymentRow; grayPercent: number; onGrayPercentChange: (value: number) => void; onApply: (scope: DeploymentScopeDraft) => void; disabled: boolean; saving: boolean; error: Error | null; savedMessage: string }) {
  const scope = selected?.__deployment?.scope ?? {};
  const [tenant, setTenant] = useState(String(scope.tenant ?? '租户A'));
  const [campus, setCampus] = useState(String(scope.campus ?? '华东园区'));
  const [probeGroup, setProbeGroup] = useState(String(scope.probe_group ?? '办公区探针组 (12)'));
  const [assetGroup, setAssetGroup] = useState(String(scope.asset_group ?? '核心业务资产组'));
  useEffect(() => {
    setTenant(String(scope.tenant ?? '租户A'));
    setCampus(String(scope.campus ?? '华东园区'));
    setProbeGroup(String(scope.probe_group ?? '办公区探针组 (12)'));
    setAssetGroup(String(scope.asset_group ?? '核心业务资产组'));
  }, [scope.asset_group, scope.campus, scope.probe_group, scope.tenant]);
  const apply = () => onApply({ tenant, campus, probe_group: probeGroup, asset_group: assetGroup, percentage: grayPercent });
  return <div className="taf-deployments-gray">
    <div className="taf-deployments-gray-form">
      <label><span>租户</span><Select size="small" value={tenant} options={[{ value: '租户A' }, { value: '租户B' }]} onChange={setTenant} /></label>
      <label><span>园区</span><Select size="small" value={campus} options={[{ value: '华东园区' }, { value: '华南园区' }]} onChange={setCampus} /></label>
      <label><span>探针组</span><Select size="small" value={probeGroup} options={[{ value: '办公区探针组 (12)' }, { value: '核心区探针组 (8)' }]} onChange={setProbeGroup} /></label>
      <label><span>资产组</span><Select size="small" value={assetGroup} options={[{ value: '核心业务资产组' }, { value: '办公终端资产组' }]} onChange={setAssetGroup} /></label>
    </div>
    <div className="taf-deployments-slider"><span>流量百分比</span><Slider disabled={disabled} min={0} max={100} value={grayPercent} marks={{ 5: '5%', 20: '20%', 50: '50%', 100: '100%' }} onChange={(value) => onGrayPercentChange(Number(value))} /></div>
    <div className="taf-deployments-gray-meta"><span>当前阶段<b>{String(selected?.状态 ?? '待发布')}</b></span><span>生效时间<b>{String(selected?.发布时间 ?? '—')}</b></span><span>预计观察时长<b>30 分钟</b></span><span>自动推进<b>未开启</b></span><Button size="small" type="primary" disabled={disabled} loading={saving} onClick={apply}>编辑策略</Button></div>
    {(error || savedMessage) && <div className={`taf-deployments-gray-feedback${error ? ' is-error' : ''}`}>{error?.message ?? savedMessage}</div>}
  </div>;
}

function ReleaseHealth({ items, grayPercent, healthWindow }: { items: Array<Record<string, unknown>>; grayPercent: number; healthWindow: string }) {
  const source = items.length ? items : fallbackHealthItems;
  return <div className="taf-deployments-health">{source.slice(0, 6).map((item, index) => {
    const tone = String(item.tone ?? 'ok');
    const values = Array.isArray(item.values) ? item.values.map(Number).filter(Number.isFinite) : buildHealthTrend(index, grayPercent);
    return <div key={String(item.label)}><span>{String(item.label)}</span><strong className={`is-${tone}`}>{String(item.value)}</strong><i className={`is-${tone}`} /><div className="taf-deployments-health-echart"><DataQualityKpiSparklineChart ariaLabel={`${String(item.label)}${healthWindow}趋势`} tone={tone === 'risk' ? 'risk' : tone === 'warn' ? 'warn' : 'ok'} values={values} /></div></div>;
  })}</div>;
}

function VersionDiff({ selected, items }: { selected?: DeploymentRow; items: Array<Record<string, unknown>> }) {
	const changes = items;
  return <div className="taf-deployments-diff"><div className="taf-deployments-version-pair"><span>当前版本<b>v2.2.7</b><em>已发布</em></span><strong>→</strong><span>目标版本<b>{String(selected?.版本 ?? 'v2.3.1')}</b><em>{String(selected?.状态 ?? '灰度中')}</em></span></div><div className="taf-deployments-change-list">{changes.slice(0, 5).map((item) => <span key={String(item.label)}><b>{String(item.label)}</b><em>{String(item.from)}</em><strong>→</strong><em>{String(item.to)}</em><i>{String(item.delta)}</i></span>)}</div><p>影响评估：中，建议先在 20% 流量灰度验证误报率。</p></div>;
}

function RollbackManager({ items, reason, disabled, onReasonChange, onRollback }: { items: Array<Record<string, unknown>>; reason: string; disabled: boolean; onReasonChange: (value: string) => void; onRollback: (targetDeploymentId: string) => void }) {
	const rows = items;
	const firstTargetId = String(rows[0]?.deployment_id ?? '');
	return <div className="taf-deployments-rollback"><div><span>可回滚版本</span><span>发布时间</span><span>影响范围</span><span>发布人</span><span>操作</span></div>{rows.slice(0, 3).map((item) => { const targetId = String(item.deployment_id ?? ''); return <button key={targetId || String(item.version)} type="button" disabled={disabled || !targetId} onClick={() => onRollback(targetId)}><b>{String(item.version)}</b><span>{String(item.released_at)}</span><span>{String(item.scope)}</span><span>{String(item.owner)}</span><em>{disabled ? '不可用' : '回滚'}</em></button>; })}<label><span>回滚原因（必填）</span><Input aria-required="true" disabled={disabled} value={reason} placeholder="请输入回滚原因" onChange={(event) => onReasonChange(event.target.value)} /></label><div className="taf-deployments-rollback-actions"><Button size="small" disabled={disabled} onClick={() => onReasonChange('误报率升高，经审批执行回滚')}>误报升高</Button><Button size="small" disabled={disabled} onClick={() => onReasonChange('性能指标下降，经审批执行回滚')}>性能下降</Button><Button size="small" disabled={disabled} onClick={() => onReasonChange('策略变更异常，经审批执行回滚')}>策略变更</Button><Button size="small" danger disabled={disabled || !reason.trim() || !firstTargetId} onClick={() => onRollback(firstTargetId)}>执行回滚</Button></div></div>;
}

function ReleaseEvidence({ items, onExport }: { items: Array<Record<string, unknown>>; onExport: () => void }) {
	const rows = items;
  return <div className="taf-deployments-evidence"><div><span>证据项</span><span>校验状态</span><span>哈希 / 编号</span><span>操作</span></div>{rows.slice(0, 6).map((item) => <button key={String(item.label)} type="button" onClick={onExport}><b>{String(item.label)}</b><StatusTag value={item.status} /><span>{String(item.checksum)}</span><DownloadOutlined /></button>)}<Button className="taf-deployments-evidence-export" size="small" type="primary" onClick={onExport}>导出合规证据包</Button></div>;
}

function renderDeploymentCell(column: string, value: unknown, record: DeploymentRow, openDialog: (dialog: DeploymentDialog) => void, permissions: string[]) {
  if (column === '发布对象') return <span className="taf-deployments-object"><CodeSandboxOutlined />{String(value)}</span>;
  if (column === '状态') return <StatusTag value={value} />;
  if (column !== '操作') return String(value ?? '—');
  const deploymentId = record.__deployment_id;
	const status = record.__deployment?.status ?? statusApiValue(String(record.状态));
	const deployWorkflow = deploymentWorkflow(record.__deployment?.metadata, 'deploy');
	const rollbackWorkflow = deploymentWorkflow(record.__deployment?.metadata, 'rollback');
	const canOpenDeployWorkflow = status === 'planned' && (deploymentActionAllowed('gray', status, permissions) || (deployWorkflow?.stage === 'approval_pending' && hasDeployScope(permissions, 'deploy:approve')));
	const canOpenRollbackWorkflow = deploymentActionAllowed('rollback', status, permissions) || (rollbackWorkflow?.stage === 'approval_pending' && hasDeployScope(permissions, 'deploy:approve'));
  const action = (title: string, kind: DeploymentDialog['kind'], apiAction?: DeploymentApiAction) => (event: React.MouseEvent) => { event.stopPropagation(); openDialog({ title, kind, action: apiAction, deploymentId }); };
  return <span className="taf-deployments-row-actions">
    <Tooltip title="查看发布详情"><Button size="small" type="text" aria-label="查看发布详情" icon={<EyeOutlined />} onClick={action('查看发布详情', 'view')} /></Tooltip>
		<Tooltip title={deployWorkflow?.stage === 'approval_pending' ? '处理部署审批' : '启动灰度'}><Button size="small" type="text" aria-label={deployWorkflow?.stage === 'approval_pending' ? '处理部署审批' : '启动灰度'} disabled={!visualModeActionAllowed(record, ['planned']) || !canOpenDeployWorkflow} icon={<CloudUploadOutlined />} onClick={action('部署审批', 'deploy', 'gray')} /></Tooltip>
    <Tooltip title="暂停发布"><Button size="small" type="text" aria-label="暂停发布" disabled={!visualModeActionAllowed(record, ['gray', 'active']) || !deploymentActionAllowed('pause', status, permissions)} icon={<PauseCircleOutlined />} onClick={action('暂停发布', 'mutate', 'pause')} /></Tooltip>
    <Tooltip title="查看发布时间线"><Button size="small" type="text" aria-label="查看发布时间线" icon={<FieldTimeOutlined />} onClick={action('查看发布时间线', 'history')} /></Tooltip>
		<Tooltip title={rollbackWorkflow?.stage === 'approval_pending' ? '处理回滚审批' : '回滚发布'}><Button size="small" type="text" aria-label={rollbackWorkflow?.stage === 'approval_pending' ? '处理回滚审批' : '回滚发布'} disabled={!canOpenRollbackWorkflow} icon={<RollbackOutlined />} onClick={action('回滚发布', 'mutate', 'rollback')} /></Tooltip>
  </span>;
}

function deploymentToRow(deployment: DeploymentRecord): DeploymentRow {
  const scope = deployment.scope ?? {};
  return {
    __deployment_id: deployment.deployment_id,
    __deployment: deployment,
    发布对象: deployment.name || deployment.deployment_id,
    版本: String(scope.version ?? deployment.rule_version ?? deployment.model_version ?? '—'),
    环境: String(scope.environment ?? 'prod'),
    状态: deploymentStatusLabel(deployment.status, Number(scope.percentage)),
    负责人: String(scope.owner ?? deployment.created_by ?? 'system'),
    发布时间: formatDeploymentTime(deployment.updated_at || deployment.created_at),
    影响范围: `${String(scope.campus ?? '默认园区')} / ${String(scope.tenant ?? '租户A')} / ${String(scope.impact ?? '全量')}`,
    操作: '',
  };
}

function inferDeploymentReleaseLine(deployment: DeploymentRecord) {
	const bound = [deployment.rule_version, deployment.model_version, deployment.feature_set_id].filter(Boolean);
	if (bound.length > 1) return 'detection-bundle';
	if (deployment.rule_version) return 'ruleset';
	if (deployment.model_version) return 'model';
	if (deployment.feature_set_id) return 'feature-set';
	return 'deployment';
}

export function deploymentStatusLabel(status: string, percentage = 0) {
  const normalized = status.trim().toLowerCase();
  if (normalized === 'planned') return '待发布';
  if (normalized === 'gray') return `灰度中 ${percentage || 20}%`;
  if (normalized === 'active') return '已发布';
  if (normalized === 'paused') return '已暂停';
  if (normalized === 'rolled_back') return '已回滚';
  if (normalized === 'failed') return '阻断';
  if (normalized === 'cancelled') return '已取消';
  if (normalized === 'superseded') return '已替代';
  return status || '未知';
}

function statusApiValue(label: string) {
  if (label.includes('灰度')) return 'gray';
  if (label.includes('暂停')) return 'paused';
  if (label.includes('回滚')) return 'rolled_back';
  if (label.includes('发布') && !label.includes('待')) return 'active';
  if (label.includes('阻断') || label.includes('失败')) return 'failed';
  return 'planned';
}

function isDeploymentWorkflowStage(value: string): value is DeploymentWorkflowStage {
  return ['idle', 'draft_saved', 'precheck_completed', 'approval_pending', 'approved', 'rejected'].includes(value);
}

function deploymentWorkflow(metadata: Record<string, unknown> | undefined, operation: 'deploy' | 'rollback') {
  const value = metadata?.workflow;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const workflow = value as Record<string, unknown>;
	return workflow.operation === operation ? workflow as Partial<DeploymentWorkflow> : undefined;
}

function workflowPrecheckRows(workflow: Partial<DeploymentWorkflow>) {
  const rows = Array.isArray(workflow.precheck_results) ? workflow.precheck_results : [];
  return rows.flatMap((row) => {
    if (!row || typeof row !== 'object' || Array.isArray(row)) return [];
    const item = row as Record<string, unknown>;
    const status = String(item.status ?? 'unknown');
    return [{
      label: String(item.label ?? '检查项'),
      status: status === 'passed' ? '已通过' : status === 'warning' ? '警告' : status === 'failed' ? '失败' : status,
      evidence: String(item.evidence ?? '—'),
		recommendation: String(item.recommendation ?? '—'),
		freshness: formatEvidenceFreshness(item.source_observed_at),
    }];
  });
}

function deploymentWorkflowStageLabel(stage: DeploymentWorkflowStage, operation: 'deploy' | 'rollback') {
  if (stage === 'approved') return '审批已通过';
  if (stage === 'rejected') return '审批已驳回';
  if (stage === 'approval_pending') return '审批待处理';
  if (stage === 'precheck_completed') return operation === 'rollback' ? '回滚检查已完成' : '预检查已完成';
  if (stage === 'draft_saved') return operation === 'rollback' ? '回滚单已保存' : '预检查待运行';
  return operation === 'rollback' ? '回滚单草案' : '预检查待运行';
}

function visualModeActionAllowed(record: DeploymentRow, statuses: string[]) {
  return statuses.includes(record.__deployment?.status ?? statusApiValue(String(record.状态)));
}

function rowKey(row: DeploymentRow) {
  return String(row.__deployment_id ?? row.发布对象 ?? row.批次 ?? JSON.stringify(row));
}

function buildVisualDeploymentRows(rows: SnapshotRow[]): DeploymentRow[] {
  const source = rows.length ? rows : [{ 发布对象: '规则包-APT检测增强', 版本: 'v2.3.1', 环境: 'prod', 状态: '灰度中 20%', 负责人: '安全运营组', 发布时间: '2026-06-20 03:45', 影响范围: '华东园区 / 租户A / 12个探针', 操作: '' }];
  return Array.from({ length: 48 }, (_, index) => {
    const base = source[index % source.length];
    return { ...base, 发布对象: index < source.length ? base.发布对象 : `${['规则包-横向移动检测', '模型-加密流量识别', '采集策略-边界区', 'Flink作业-会话聚合'][index % 4]}-${String(index + 1).padStart(2, '0')}`, 版本: String(base.版本 ?? `v2.${index % 5}.${index % 8}`) };
  });
}

function buildDeploymentMetrics(labels: string[], records: DeploymentRecord[], total: number): PageSnapshot['metrics'] {
  const statusCount = (status: string) => records.filter((item) => item.status === status).length;
  const metadata = records[0]?.metadata ?? {};
  const values: Record<string, { value: string; delta: string; status: PageSnapshot['metrics'][number]['status'] }> = {
    待发布对象: { value: String(statusCount('planned')), delta: `共 ${total} 条`, status: 'info' },
    灰度中: { value: String(statusCount('gray')), delta: '实时 API', status: 'warn' },
    '失败/阻断': { value: String(statusCount('failed')), delta: '需处置', status: 'risk' },
    可回滚版本: { value: String(metadata.rollback_version_count ?? 23), delta: '数据库快照', status: 'ok' },
    发布成功率: { value: `${Number(metadata.release_success_rate ?? 98.2).toFixed(1)}%`, delta: '近 7 日', status: 'ok' },
    平均生效延迟: { value: `${Number(metadata.avg_activation_latency_seconds ?? 58)}s`, delta: '近 7 日', status: 'info' },
  };
  return labels.map((label) => ({ label, ...(values[label] ?? { value: '0', delta: 'API', status: 'info' }) }));
}

function visualWorkbench(snapshot?: PageSnapshot): DeploymentWorkbench | undefined {
  if (!snapshot) return undefined;
  return { deployment: {} as DeploymentRecord, history: [], items: { health: fallbackHealthItems, evidence: fallbackEvidenceItems, change_summary: fallbackChangeItems, rollback_versions: fallbackRollbackItems }, source: 'visual-breakdown' };
}

function workbenchItems(workbench: DeploymentWorkbench | undefined, category: string) {
  return workbench?.items?.[category] ?? [];
}

function downloadDeploymentEvidence(bundle: DeploymentEvidenceBundle) {
  const blob = new Blob([bundle.download_content], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `${bundle.export_id}.json`;
  document.body.append(anchor);
  anchor.click();
  window.setTimeout(() => {
    anchor.remove();
    URL.revokeObjectURL(url);
  }, 60_000);
}

function readTokenIdentity(token: string | null): { userId: string; permissions: string[] } {
	if (!token) return { userId: '', permissions: [] };
  try {
    const base64 = token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');
	const payload = JSON.parse(atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, '='))) as { permissions?: unknown; user_id?: unknown; sub?: unknown };
	return {
	  userId: String(payload.user_id ?? payload.sub ?? ''),
	  permissions: Array.isArray(payload.permissions) ? payload.permissions.filter((item): item is string => typeof item === 'string') : [],
	};
	} catch { return { userId: '', permissions: [] }; }
}

function workflowConfiguration(workflow: Partial<DeploymentWorkflow>): Record<string, unknown> {
	const snapshot = workflow.approval_snapshot;
	if (snapshot && typeof snapshot === 'object' && !Array.isArray(snapshot)) {
	  const configuration = snapshot.configuration;
	  if (configuration && typeof configuration === 'object' && !Array.isArray(configuration)) return configuration as Record<string, unknown>;
	}
	return workflow.configuration ?? {};
}

function shortHash(value: unknown) {
	const hash = String(value ?? '');
	if (!hash) return '待生成';
	return hash.length > 20 ? `${hash.slice(0, 15)}…${hash.slice(-4)}` : hash;
}

function formatEvidenceFreshness(value: unknown) {
	const timestamp = Date.parse(String(value ?? ''));
	if (!Number.isFinite(timestamp)) return '';
	const minutes = Math.max(0, Math.round((Date.now() - timestamp) / 60_000));
	return minutes < 1 ? '刚刚' : `${minutes}m`;
}

function hasDeployScope(permissions: string[], required: string) {
  return permissions.some((permission) => permission === '*' || permission === 'admin:*' || permission === 'deploy:*' || permission === required);
}

export function deploymentActionAvailability(status: string, permissions: string[]) {
  const canCreate = hasDeployScope(permissions, 'deploy:create');
  const canGray = hasDeployScope(permissions, 'deploy:gray');
  const canActivate = hasDeployScope(permissions, 'deploy:activate');
  const canRollbackPermission = hasDeployScope(permissions, 'deploy:rollback');
  return {
    canCreate,
    canGray,
    canActivate,
    canContinue: ['planned', 'gray', 'paused'].includes(status) && (status === 'planned' ? canGray : canActivate),
		canEditScope: status === 'planned' && canGray,
    canPause: ['gray', 'active'].includes(status) && canActivate,
    canRollback: ['gray', 'active', 'paused', 'failed'].includes(status) && canRollbackPermission,
  };
}

function deploymentActionAllowed(action: DeploymentApiAction | undefined, status: string, permissions: string[]) {
  if (!action) return false;
  const availability = deploymentActionAvailability(status, permissions);
  if (action === 'gray') return status === 'planned' && availability.canGray;
  if (action === 'activate') return status === 'gray' && availability.canActivate;
  if (action === 'pause') return availability.canPause;
  if (action === 'resume') return status === 'paused' && availability.canActivate;
  if (action === 'rollback') return availability.canRollback;
  return false;
}

function paginationWindow(count: number, current: number) {
  if (count <= 6) return Array.from({ length: count }, (_, index) => index + 1);
  const start = Math.max(1, Math.min(current - 2, count - 4));
  return Array.from({ length: 5 }, (_, index) => start + index);
}

function formatDeploymentTime(value: string) {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value || '—';
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(timestamp).replace(/\//g, '-');
}

function deploymentActionLabel(action: DeploymentApiAction) {
  return ({ gray: '灰度中', activate: '已发布', pause: '已暂停', resume: '已发布', rollback: '已回滚' } as const)[action];
}

function deploymentHistoryLabel(action: string) {
  return ({ created: '创建发布单', gray_started: '启动灰度', activated: '全量发布', paused: '暂停发布', resumed: '恢复发布', rolled_back: '执行回滚' } as Record<string, string>)[action] ?? action;
}

const fallbackHealthItems: Array<Record<string, unknown>> = [
  { label: 'Flink Checkpoint 成功率', value: '98.8%', tone: 'ok' }, { label: 'Kafka 消费延迟 (P95)', value: '320 ms', tone: 'warn' }, { label: '告警数量变化', value: '-18.5%', tone: 'ok' }, { label: '误报率变化', value: '+2.1%', tone: 'risk' }, { label: '端到端延迟 (P95)', value: '1.28 s', tone: 'ok' }, { label: '采集丢包率', value: '0.03%', tone: 'ok' },
];
const fallbackEvidenceItems: Array<Record<string, unknown>> = [
  { label: 'manifest', status: '已通过', checksum: 'a1b2c3d4...e5f6' }, { label: '镜像', status: '已通过', checksum: 'sha256:7e8d...9e0f' }, { label: 'DDL', status: '已通过', checksum: 'ddl_20260620_001' }, { label: 'topic', status: '已通过', checksum: 'topic_20260620_001' }, { label: '规则版本', status: '已通过', checksum: 'rules_v2.3.1' }, { label: '模型版本', status: '已通过', checksum: 'model_v1.8.0' },
];
const fallbackChangeItems: Array<Record<string, unknown>> = [
  { label: '规则变更数', from: '32 条', to: '57 条', delta: '+25' }, { label: '模型版本', from: 'v1.7.3', to: 'v1.8.0', delta: '升级' }, { label: 'DDL 变更', from: '2 处', to: '3 处', delta: '+1' }, { label: 'Topic 变更', from: '1 个', to: '2 个', delta: '+1' }, { label: '风险等级', from: '低风险', to: '中风险', delta: '升高' },
];
const fallbackRollbackItems: Array<Record<string, unknown>> = [
	{ deployment_id: 'visual-v2.2.7', version: 'v2.2.7', released_at: '2026-06-19 16:10', scope: '租户A / 全量', owner: '安全运营组' }, { deployment_id: 'visual-v2.2.3', version: 'v2.2.3', released_at: '2026-06-18 11:05', scope: '租户A / 全量', owner: '安全运营组' }, { deployment_id: 'visual-v2.1.9', version: 'v2.1.9', released_at: '2026-06-17 09:40', scope: '租户A / 全量', owner: '安全运营组' },
];

function buildHealthTrend(index: number, grayPercent: number) {
  const baseline = 76 + (index * 7 + grayPercent) % 18;
  return [baseline - 3, baseline + 1, baseline - 1, baseline + 3, baseline, baseline + 2, baseline + 1];
}

function fallbackMetric(label: string): PageSnapshot['metrics'][number] {
  return { label, value: label.includes('率') ? '98.2%' : label.includes('延迟') ? '58s' : '0', delta: 'API', status: 'info' };
}
