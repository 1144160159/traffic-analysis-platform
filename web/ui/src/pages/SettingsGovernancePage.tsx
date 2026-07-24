import {
  ApiOutlined,
  AuditOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloudServerOutlined,
  ClusterOutlined,
  ControlOutlined,
  CopyOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  ExperimentOutlined,
  EyeOutlined,
  FileProtectOutlined,
  KeyOutlined,
  LockOutlined,
  NodeIndexOutlined,
  PartitionOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SecurityScanOutlined,
  SettingOutlined,
  TeamOutlined,
  UserSwitchOutlined,
} from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import {
  Alert,
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useEffect, useMemo, useState } from 'react';
import { MetricTile } from '@/components/MetricTile';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import { getAuthToken } from '@/services/authStorage';
import type { PageSnapshot, SnapshotRow } from '@/services/mockData';
import {
  createSettingsToken,
  fetchSettingsTokenScopes,
  fetchSystemSettingsImpact,
  fetchSystemSettingsWorkbench,
  regenerateSettingsToken,
  revokeSettingsToken,
  runSystemSettingsAction,
  saveSystemSettings,
  updateSettingsTokenScopes,
  type IntegrationSetting,
  type RetentionPolicy,
  type SystemSettingsAction,
  type SystemSettingsActionResult,
  type SystemSettingsImpact,
  type SystemSite,
  type SystemSettingsWorkbench,
  type TenantSystemSettings,
} from '@/services/settingsGovernanceApi';

const permissionColumns = [
  ['告警查看', 'alert:read'],
  ['PCAP访问', 'pcap:read'],
  ['规则发布', 'rule:write'],
  ['模型激活', 'deploy:activate'],
  ['脚本执行', 'playbook:execute'],
  ['证据导出', 'evidence:export'],
  ['系统配置', 'admin:write'],
] as const;

const integrationIcons: Record<string, React.ReactNode> = {
  keycloak: <UserSwitchOutlined />,
  apisix: <ApiOutlined />,
  kafka: <PartitionOutlined />,
  minio: <CloudServerOutlined />,
  opensearch: <ExperimentOutlined />,
  nebula: <NodeIndexOutlined />,
  webhook: <ClusterOutlined />,
};

const isolationLabels: Record<string, string> = {
  isolated: '已隔离',
  partial: '部分隔离',
  unisolated: '未隔离',
};

type DrawerState =
  | { kind: 'site'; site: SystemSite }
  | { kind: 'integration'; integration: IntegrationSetting }
  | { kind: 'retention' }
  | { kind: 'token-scopes' }
  | { kind: 'impact'; impact: SystemSettingsImpact }
  | { kind: 'action'; result: SystemSettingsActionResult; plainToken?: string }
  | null;

type CreateTokenForm = {
  name: string;
  description?: string;
  scopes: string[];
  expires_days: number;
};

export function SettingsGovernancePage({ route }: { route: NavRoute }) {
  const [messageApi, messageContext] = message.useMessage();
  const [selectedKey, setSelectedKey] = useState<string>();
  const [draft, setDraft] = useState<TenantSystemSettings>();
  const [drawer, setDrawer] = useState<DrawerState>(null);
  const [createTokenOpen, setCreateTokenOpen] = useState(false);
  const [busyAction, setBusyAction] = useState('');
  const [createTokenForm] = Form.useForm<CreateTokenForm>();
  const [scopeForm] = Form.useForm<{ scopes: string[] }>();
  const permissions = useMemo(() => readTokenPermissions(getAuthToken()), []);
  const canAdminWrite = hasSettingsScope(permissions, 'admin:write');
  const canTokenWrite = hasSettingsScope(permissions, 'token:write');

  const snapshotQuery = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });
  const workbenchQuery = useQuery({
    queryKey: ['system-settings-workbench'],
    queryFn: fetchSystemSettingsWorkbench,
  });
  const scopesQuery = useQuery({
    queryKey: ['system-settings-token-scopes'],
    queryFn: fetchSettingsTokenScopes,
  });

  const { data, error, isError, isLoading, refetch } = snapshotQuery;
  const workbench = workbenchQuery.data;
  useEffect(() => {
    if (workbench?.settings) setDraft(structuredClone(workbench.settings));
  }, [workbench]);

  const rows = useMemo(() => data?.rows ?? [], [data?.rows]);
  const selected = useMemo(() => rows.find((row) => settingsRowKey(row) === selectedKey) ?? rows.find(tokenActionable) ?? rows[0], [rows, selectedKey]);
  const metrics = route.page.kpis.map((label) => data?.metrics.find((item) => item.label === label) ?? fallbackMetric(label));
  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: column !== '操作',
    width: column === '操作' ? 148 : undefined,
    render: (value, record) => column === '操作'
      ? <TokenRowActions record={record} busyAction={busyAction} canWrite={canTokenWrite} onRotate={() => void rotateToken(record)} onRevoke={() => void revokeToken(record)} onScopes={() => openTokenScopes(record)} />
      : renderSettingsCell(column, value),
  }));

  const refreshAll = async () => {
    await Promise.all([refetch(), workbenchQuery.refetch(), scopesQuery.refetch()]);
  };

  const reportFailure = (action: string, reason: unknown) => {
    const detail = reason instanceof Error ? reason.message : '请求失败，请检查权限、版本或后端状态。';
    messageApi.error(`${action}失败：${detail}`);
  };

  const runAction = async (action: SystemSettingsAction, targetId?: string) => {
    if (!workbench) return;
    setBusyAction(action + (targetId ?? ''));
    try {
      const result = await runSystemSettingsAction(action, workbench.revision, targetId);
      setDrawer({ kind: 'action', result });
      messageApi.success(result.message);
      await refreshAll();
    } catch (reason) {
      reportFailure('系统设置动作', reason);
    } finally {
      setBusyAction('');
    }
  };

  const saveDraft = async () => {
    if (!workbench || !draft) return;
    setBusyAction('save');
    try {
      const saved = await saveSystemSettings(workbench.revision, draft);
      setDraft(structuredClone(saved.settings));
      messageApi.success(`配置已保存并写入审计，revision ${saved.revision}`);
      await refreshAll();
    } catch (reason) {
      reportFailure('保存配置', reason);
    } finally {
      setBusyAction('');
    }
  };

  const showImpact = async () => {
    setBusyAction('impact');
    try {
      const impact = await fetchSystemSettingsImpact();
      setDrawer({ kind: 'impact', impact });
    } catch (reason) {
      reportFailure('影响范围评估', reason);
    } finally {
      setBusyAction('');
    }
  };

  const createToken = async () => {
    try {
      const values = await createTokenForm.validateFields();
      setBusyAction('create-token');
      const created = await createSettingsToken({
        name: values.name.trim(),
        description: values.description?.trim(),
        scopes: values.scopes,
        expires_in_sec: values.expires_days * 86_400,
      });
      setCreateTokenOpen(false);
      createTokenForm.resetFields();
      setDrawer({
        kind: 'action',
        plainToken: created.token,
        result: {
          action: 'scope-review', status: 'success', revision: workbench?.revision ?? 0,
          updated_at: created.created_at, message: `令牌 ${created.name} 已创建，明文仅展示一次。`,
        },
      });
      await refreshAll();
    } catch (reason) {
      if (reason && typeof reason === 'object' && 'errorFields' in reason) return;
      reportFailure('创建令牌', reason);
    } finally {
      setBusyAction('');
    }
  };

  const rotateToken = async (record = selected) => {
    const tokenID = tokenIDFrom(record);
    if (!tokenID) {
      messageApi.warning('请先选择可轮换的真实令牌。');
      return;
    }
    setBusyAction(`rotate-${tokenID}`);
    try {
      const regenerated = await regenerateSettingsToken(tokenID);
      setDrawer({
        kind: 'action', plainToken: regenerated.token,
        result: {
          action: 'scope-review', status: 'success', revision: workbench?.revision ?? 0,
          updated_at: regenerated.created_at, message: `令牌已轮换，新令牌 ${regenerated.name} 的明文仅展示一次。`,
        },
      });
      await refreshAll();
    } catch (reason) {
      reportFailure('轮换令牌', reason);
    } finally {
      setBusyAction('');
    }
  };

  const revokeToken = async (record = selected) => {
    const tokenID = tokenIDFrom(record);
    if (!tokenID) {
      messageApi.warning('请先选择可吊销的真实令牌。');
      return;
    }
    setBusyAction(`revoke-${tokenID}`);
    try {
      const result = await revokeSettingsToken(tokenID);
      messageApi.success(result.message || '令牌已吊销并写入审计');
      await refreshAll();
    } catch (reason) {
      reportFailure('吊销令牌', reason);
    } finally {
      setBusyAction('');
    }
  };

  const openTokenScopes = (record = selected) => {
    if (!tokenIDFrom(record)) {
      messageApi.warning('请先选择真实令牌。');
      return;
    }
    setSelectedKey(settingsRowKey(record!));
    scopeForm.setFieldsValue({ scopes: scopesFrom(record) });
    setDrawer({ kind: 'token-scopes' });
  };

  const saveTokenScopes = async () => {
    const tokenID = tokenIDFrom(selected);
    if (!tokenID) return;
    try {
      const values = await scopeForm.validateFields();
      setBusyAction(`scopes-${tokenID}`);
      await updateSettingsTokenScopes(tokenID, values.scopes);
      messageApi.success('令牌权限范围已更新并写入审计');
      setDrawer(null);
      await refreshAll();
    } catch (reason) {
      if (reason && typeof reason === 'object' && 'errorFields' in reason) return;
      reportFailure('更新令牌权限', reason);
    } finally {
      setBusyAction('');
    }
  };

  const updateSite = (next: SystemSite) => {
    setDraft((current) => current ? { ...current, sites: current.sites.map((item) => item.id === next.id ? next : item) } : current);
    setDrawer({ kind: 'site', site: next });
  };

  const updateIntegration = (next: IntegrationSetting) => {
    setDraft((current) => current ? { ...current, integrations: current.integrations.map((item) => item.id === next.id ? next : item) } : current);
    setDrawer({ kind: 'integration', integration: next });
  };

  const updateRetention = (index: number, next: RetentionPolicy) => {
    setDraft((current) => current ? { ...current, retention_policies: current.retention_policies.map((item, itemIndex) => itemIndex === index ? next : item) } : current);
  };

  const loading = isLoading || workbenchQuery.isLoading;
  const combinedError = error ?? workbenchQuery.error;

  return (
    <div className="taf-page taf-settings-page">
      {messageContext}
      <section className="taf-settings-shell">
        <main className="taf-settings-main">
          <header className="taf-settings-titlebar">
            <div>
              <h1>{route.page.title}</h1>
              <small>{workbench ? `${workbench.tenant_name} · revision ${workbench.revision}` : '正在读取租户配置'}</small>
            </div>
            <Space size={8} wrap>
              <Button data-settings-action="save" size="small" type="primary" icon={<SaveOutlined />} loading={busyAction === 'save'} disabled={!canAdminWrite || !workbench || !draft || loading || Boolean(busyAction)} onClick={() => void saveDraft()}>保存配置</Button>
              <Button data-settings-action="connection-test" size="small" icon={<ApiOutlined />} loading={busyAction.startsWith('connection-test')} disabled={!canAdminWrite || !workbench || loading || Boolean(busyAction)} onClick={() => void runAction('connection-test')}>连接测试</Button>
              <Button data-settings-action="create-token" size="small" icon={<KeyOutlined />} disabled={!canTokenWrite || !workbench || loading || Boolean(busyAction)} onClick={() => setCreateTokenOpen(true)}>创建令牌</Button>
              <Popconfirm title="确认轮换当前令牌？" description="旧令牌与新令牌将以同一事务切换，明文仅展示一次。" okText="确认轮换" cancelText="取消" onConfirm={() => void rotateToken()}>
                <Button data-settings-action="rotate-token" size="small" icon={<ControlOutlined />} loading={busyAction.startsWith('rotate-')} disabled={!canTokenWrite || !workbench || loading || Boolean(busyAction) || !tokenActionable(selected)}>轮换令牌</Button>
              </Popconfirm>
              <Button data-settings-action="security-audit" size="small" icon={<AuditOutlined />} loading={busyAction.startsWith('security-audit')} disabled={!canAdminWrite || !workbench || loading || Boolean(busyAction)} onClick={() => void runAction('security-audit')}>触发安全审计</Button>
              <Button data-settings-action="impact" size="small" icon={<EyeOutlined />} loading={busyAction === 'impact'} disabled={!workbench || loading || Boolean(busyAction)} onClick={() => void showImpact()}>查看影响范围</Button>
              <Tooltip title="刷新系统设置">
                <Button aria-label="刷新系统设置" size="small" icon={<ReloadOutlined />} loading={snapshotQuery.isFetching || workbenchQuery.isFetching} onClick={() => void refreshAll()} />
              </Tooltip>
            </Space>
          </header>

          {(isError || workbenchQuery.isError) && (
            <Alert
              type="error"
              showIcon
              message="真实 API 数据加载失败"
              description={combinedError instanceof Error ? combinedError.message : '请检查 /v1/auth/system-settings、/v1/tokens 或 auth-service。'}
              action={<Button size="small" danger onClick={() => void refreshAll()}>重试</Button>}
            />
          )}

          <div className="taf-settings-kpis">
            {metrics.map((metric) => <MetricTile key={metric.label} metric={metric} />)}
          </div>

          <div className="taf-settings-workbench" aria-busy={loading}>
            <WorkPanel title="A. 租户与站点" className="taf-settings-tenant-panel" extra={<TeamOutlined />}>
              <TenantTree sites={draft?.sites ?? []} selectedId={drawer?.kind === 'site' ? drawer.site.id : ''} onSelect={(site) => setDrawer({ kind: 'site', site })} />
            </WorkPanel>

            <WorkPanel title="B. RBAC 权限矩阵" className="taf-settings-rbac-panel" extra={<SecurityScanOutlined />}>
              <RbacMatrix workbench={workbench} />
            </WorkPanel>

            <WorkPanel title="C. API 令牌" className="taf-settings-token-panel" extra={<KeyOutlined />}>
              <Table
                rowKey={settingsRowKey}
                size="small"
                loading={loading}
                pagination={false}
                columns={columns}
                dataSource={rows.slice(0, 5)}
                rowSelection={{ selectedRowKeys: selected ? [settingsRowKey(selected)] : [], onChange: (keys) => setSelectedKey(String(keys[0] ?? '')) }}
                onRow={(record) => ({ onClick: () => setSelectedKey(settingsRowKey(record)) })}
              />
              <div className="taf-settings-token-footer">
                <span>共 {workbench?.tokens.active ?? metricValue(data, '有效令牌', '0')} 个有效令牌</span>
                <span>{selected?.令牌名称 ?? '未选择'} 已关联审计</span>
                <Select size="small" value="10 条/页" options={[{ value: '10 条/页' }]} />
              </div>
            </WorkPanel>

            <WorkPanel title="D. 数据留存策略" className="taf-settings-retention-panel" extra={<DatabaseOutlined />}>
              <RetentionPolicyView policies={draft?.retention_policies ?? []} />
            </WorkPanel>

            <WorkPanel title="E. 集成配置健康" className="taf-settings-integration-panel" extra={<ApiOutlined />}>
              <IntegrationHealth integrations={draft?.integrations ?? []} onConfigure={(integration) => setDrawer({ kind: 'integration', integration })} />
            </WorkPanel>

            <WorkPanel title="F. 安全策略与系统参数" className="taf-settings-security-panel" extra={<SettingOutlined />}>
              <SecurityParams settings={draft} />
            </WorkPanel>

            <WorkPanel title="G. 闭环动作入口" className="taf-settings-loop-panel" extra={<AuditOutlined />}>
              <LoopActions
                busyAction={busyAction}
                canUseToken={tokenActionable(selected)}
                canAdminWrite={canAdminWrite}
                canTokenWrite={canTokenWrite}
                ready={Boolean(workbench && draft) && !loading}
                onSync={() => void runAction('scope-review')}
                onSave={() => void saveDraft()}
                onCreate={() => setCreateTokenOpen(true)}
                onRotate={() => void rotateToken()}
                onRevoke={() => void revokeToken()}
                onRetention={() => setDrawer({ kind: 'retention' })}
                onConnection={() => void runAction('connection-test')}
                onAudit={() => void runAction('security-audit')}
                onImpact={() => void showImpact()}
              />
            </WorkPanel>
          </div>
        </main>
      </section>

      <Modal
        title="创建 API 令牌"
        width={560}
        open={createTokenOpen}
        okText="创建并显示一次"
        cancelText="取消"
        confirmLoading={busyAction === 'create-token'}
        onOk={() => void createToken()}
        onCancel={() => setCreateTokenOpen(false)}
      >
        <Alert type="warning" showIcon message="令牌明文只在创建成功后展示一次；创建、范围和过期时间都会写入审计。" />
        <Form form={createTokenForm} layout="vertical" initialValues={{ scopes: ['alert:read', 'token:read'], expires_days: 30 }}>
          <Form.Item name="name" label="令牌名称" rules={[{ required: true, whitespace: true, message: '请输入令牌名称' }]}><Input maxLength={80} /></Form.Item>
          <Form.Item name="description" label="用途说明"><Input maxLength={200} /></Form.Item>
          <Form.Item name="scopes" label="权限范围" rules={[{ required: true, type: 'array', min: 1, message: '至少选择一个权限范围' }]}>
            <Select mode="multiple" showSearch optionFilterProp="label" options={scopeOptions(scopesQuery.data)} />
          </Form.Item>
          <Form.Item name="expires_days" label="有效天数" rules={[{ required: true }]}><InputNumber min={1} max={365} precision={0} /></Form.Item>
        </Form>
      </Modal>

      <Drawer
        rootClassName="taf-settings-detail-drawer"
        title={drawerTitle(drawer)}
        width={560}
        open={Boolean(drawer)}
        getContainer={false}
        rootStyle={{ position: 'absolute' }}
        destroyOnClose
        onClose={() => setDrawer(null)}
        extra={<Tag color="blue">当前区域内操作</Tag>}
      >
        {drawer?.kind === 'site' && draft && <SiteEditor site={drawer.site} canWrite={canAdminWrite} onChange={updateSite} />}
        {drawer?.kind === 'integration' && draft && (
          <IntegrationEditor
            integration={drawer.integration}
            canWrite={canAdminWrite}
            busy={busyAction === `test-integration${drawer.integration.id}`}
            onChange={updateIntegration}
            onTest={() => void runAction('test-integration', drawer.integration.id)}
          />
        )}
        {drawer?.kind === 'retention' && draft && (
          <RetentionEditor policies={draft.retention_policies} canWrite={canAdminWrite} onChange={updateRetention} onReview={() => void runAction('lifecycle-review')} />
        )}
        {drawer?.kind === 'token-scopes' && selected && (
          <Form form={scopeForm} layout="vertical">
            <Alert type="info" showIcon message={`正在配置：${String(selected.令牌名称)}`} description="提交后直接更新 api_tokens.scopes，并写入 update_token_scopes 审计。" />
            <Form.Item name="scopes" label="权限范围" rules={[{ required: true, type: 'array', min: 1, message: '至少选择一个权限范围' }]}>
              <Select mode="multiple" showSearch optionFilterProp="label" options={scopeOptions(scopesQuery.data)} />
            </Form.Item>
            <Popconfirm title="确认更新令牌权限？" description="新权限不得超过当前账号权限，并将与审计记录同一事务提交。" okText="确认更新" cancelText="取消" onConfirm={() => void saveTokenScopes()}>
              <Button type="primary" icon={<SaveOutlined />} loading={busyAction.startsWith('scopes-')} disabled={!canTokenWrite || !workbench || loading || Boolean(busyAction)}>保存令牌权限</Button>
            </Popconfirm>
          </Form>
        )}
        {drawer?.kind === 'impact' && <ImpactView impact={drawer.impact} />}
        {drawer?.kind === 'action' && <ActionResultView result={drawer.result} plainToken={drawer.plainToken} onCopy={() => drawer.plainToken && void navigator.clipboard.writeText(drawer.plainToken).then(() => messageApi.success('令牌已复制'))} />}
      </Drawer>
    </div>
  );
}

function TenantTree({ sites, selectedId, onSelect }: { sites: SystemSite[]; selectedId: string; onSelect: (site: SystemSite) => void }) {
  const levels = siteLevels(sites);
  return (
    <div className="taf-settings-tenant-tree">
      <header><span>名称</span><span>隔离范围 / 状态</span></header>
      {sites.map((site) => (
        <button key={site.id} type="button" aria-pressed={selectedId === site.id} className={`${site.isolation_status === 'unisolated' ? 'is-risk' : site.isolation_status === 'partial' ? 'is-warn' : ''} ${selectedId === site.id ? 'is-selected' : ''}`} style={{ '--indent': String(levels.get(site.id) ?? 0) } as React.CSSProperties} onClick={() => onSelect(site)}>
          <span><i>{site.kind === 'site' ? <ClusterOutlined /> : <DatabaseOutlined />}</i>{site.name}</span>
          <span>{site.cidr || ''}</span>
          <em>{isolationLabels[site.isolation_status] ?? site.isolation_status}</em>
        </button>
      ))}
    </div>
  );
}

function RbacMatrix({ workbench }: { workbench?: SystemSettingsWorkbench }) {
  const roles = workbench?.roles ?? [];
  return (
    <div className="taf-settings-rbac">
      <div className="taf-settings-rbac-head"><span>角色</span>{permissionColumns.map(([label]) => <span key={label}>{label}</span>)}</div>
      {roles.map((role) => (
        <div key={role.id} className="taf-settings-rbac-row">
          <Tooltip title={role.description}><b>{roleName(role.name)}</b></Tooltip>
          {permissionColumns.map(([label, permission]) => {
            const state = permissionState(role.permissions, permission);
            return <Tooltip key={`${role.id}-${label}`} title={`${roleName(role.name)} · ${label} · ${state === 'ok' ? '允许' : state === 'pending' ? '需审批' : '未授权'}`}><span className={`is-${state}`}>{state === 'ok' ? <CheckCircleOutlined /> : state === 'pending' ? <ClockCircleOutlined /> : <LockOutlined />}</span></Tooltip>;
          })}
        </div>
      ))}
      {!roles.length && <Alert type="warning" showIcon message="当前租户尚未配置角色策略" />}
    </div>
  );
}

function RetentionPolicyView({ policies }: { policies: RetentionPolicy[] }) {
  return (
    <div className="taf-settings-retention">
      <header><span>数据类型</span><span>保留周期</span><span>生命周期状态</span><span>下一步动作</span></header>
      {policies.map((policy) => (
        <div key={policy.data_type} className={policy.status === 'expiring' ? 'is-warn' : ''}>
          <span>{policy.data_type}</span><span>{formatRetention(policy.retention_days)}</span><span>{policy.status === 'expiring' ? '即将到期' : '正常'}</span><span>{policy.next_action}</span>
        </div>
      ))}
    </div>
  );
}

function IntegrationHealth({ integrations, onConfigure }: { integrations: IntegrationSetting[]; onConfigure: (integration: IntegrationSetting) => void }) {
  return (
    <div className="taf-settings-integrations">
      <header><span>集成组件</span><span>连接状态</span><span>最近测试</span><span>配置入口</span></header>
      {integrations.map((integration) => (
        <button key={integration.id} type="button" onClick={() => onConfigure(integration)}>
          <span><i>{integrationIcons[integration.id] ?? <ApiOutlined />}</i>{integration.name}</span>
          <b className={integration.status !== 'healthy' ? 'is-warn' : ''}>{integration.status === 'healthy' ? '健康' : integration.status === 'disabled' ? '已停用' : '降级'}</b>
          <span>{formatTestTime(integration.last_tested_at)}</span>
          <em>配置</em>
        </button>
      ))}
    </div>
  );
}

function SecurityParams({ settings }: { settings?: TenantSystemSettings }) {
  const security = settings?.security;
  const rows: Array<[string, string, React.ReactNode]> = [
    ['登录策略', security?.login_policy ?? '-', <SafetyCertificateOutlined key="login" />],
    ['密码策略', security?.password_policy ?? '-', <LockOutlined key="password" />],
    ['MFA', security?.mfa_enabled ? '已启用' : '未启用', <KeyOutlined key="mfa" />],
    ['IP 访问控制', `已配置 ${security?.ip_access_rules ?? 0} 条`, <SecurityScanOutlined key="ip" />],
    ['脱敏策略', security?.masking_policy ?? '-', <ControlOutlined key="mask" />],
    ['时间窗默认值', security?.default_time_range === 'last_24h' ? '最近 24 小时' : security?.default_time_range ?? '-', <ClockCircleOutlined key="time" />],
    ['告警阈值', security?.alert_threshold ?? '-', <SettingOutlined key="threshold" />],
    ['页面刷新频率', `${security?.refresh_interval_sec ?? 0} 秒`, <ReloadOutlined key="refresh" />],
    ['大屏脱敏', security?.screen_masking ? '已启用' : '未启用', <EyeOutlined key="screen" />],
    ['功能开关', `${security?.feature_flags.length ?? 0} 项已启用`, <DeploymentUnitOutlined key="feature" />],
  ];
  return <div className="taf-settings-security">{rows.map(([label, value, icon]) => <div key={label}><i>{icon}</i><span>{label}</span><b>{value}</b></div>)}</div>;
}

function LoopActions(props: {
  busyAction: string;
  ready: boolean;
  canUseToken: boolean;
  canAdminWrite: boolean;
  canTokenWrite: boolean;
  onSync: () => void;
  onSave: () => void;
  onCreate: () => void;
  onRotate: () => void;
  onRevoke: () => void;
  onRetention: () => void;
  onConnection: () => void;
  onAudit: () => void;
  onImpact: () => void;
}) {
  const actions = [
    ['复核资产和权限范围', <ReloadOutlined key="sync" />, props.onSync, false, props.canAdminWrite],
    ['保存并写审计', <SaveOutlined key="save" />, props.onSave, false, props.canAdminWrite],
    ['创建令牌', <KeyOutlined key="create" />, props.onCreate, false, props.canTokenWrite],
    ['轮换令牌', <ControlOutlined key="rotate" />, props.onRotate, false, props.canTokenWrite && props.canUseToken],
    ['吊销令牌', <LockOutlined key="revoke" />, props.onRevoke, true, props.canTokenWrite && props.canUseToken],
    ['更新生命周期策略', <DatabaseOutlined key="retention" />, props.onRetention, false, props.canAdminWrite],
    ['连接测试', <ApiOutlined key="test" />, props.onConnection, false, props.canAdminWrite],
    ['触发安全审计', <AuditOutlined key="audit" />, props.onAudit, false, props.canAdminWrite],
    ['提示配置影响范围', <FileProtectOutlined key="impact" />, props.onImpact, false, true],
  ] as const;
  return (
    <div className="taf-settings-loop-actions">
      {actions.map(([label, icon, action, danger, enabled], index) => {
        const needsConfirm = danger || label === '轮换令牌';
        const button = <button key={label} type="button" disabled={!props.ready || !enabled || Boolean(props.busyAction)} className={danger ? 'is-risk' : index >= 7 ? 'is-warn' : ''} onClick={needsConfirm ? undefined : action}><i>{icon}</i><span>{label}</span></button>;
        return needsConfirm ? <Popconfirm key={label} title={danger ? '确认吊销当前令牌？' : '确认轮换当前令牌？'} description={danger ? '调用将立即失效，并写入审计日志。' : '旧令牌与新令牌将以同一事务切换，明文仅展示一次。'} okText={danger ? '确认吊销' : '确认轮换'} cancelText="取消" onConfirm={action}>{button}</Popconfirm> : button;
      })}
    </div>
  );
}

function TokenRowActions({ record, busyAction, canWrite, onRotate, onRevoke, onScopes }: { record: SnapshotRow; busyAction: string; canWrite: boolean; onRotate: () => void; onRevoke: () => void; onScopes: () => void }) {
  const actionable = tokenActionable(record);
  const tokenID = tokenIDFrom(record);
  return (
    <Space size={4} className="taf-settings-token-actions">
      <Popconfirm title="确认轮换该令牌？" description="旧令牌与新令牌将原子切换，明文仅展示一次。" okText="确认轮换" cancelText="取消" onConfirm={onRotate}>
        <Button type="link" size="small" disabled={!canWrite || !actionable || Boolean(busyAction)} loading={busyAction === `rotate-${tokenID}`} onClick={(event) => event.stopPropagation()}>轮换</Button>
      </Popconfirm>
      <Button type="link" size="small" disabled={!canWrite || !actionable || Boolean(busyAction)} onClick={(event) => { event.stopPropagation(); onScopes(); }}>权限</Button>
      <Popconfirm title="确认吊销该令牌？" description="吊销后不能恢复，并会写入审计。" okText="确认吊销" cancelText="取消" onConfirm={onRevoke}>
        <Button danger type="link" size="small" disabled={!canWrite || !actionable || Boolean(busyAction)} loading={busyAction === `revoke-${tokenID}`} onClick={(event) => event.stopPropagation()}>吊销</Button>
      </Popconfirm>
    </Space>
  );
}

function SiteEditor({ site, canWrite, onChange }: { site: SystemSite; canWrite: boolean; onChange: (site: SystemSite) => void }) {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type={site.isolation_status === 'unisolated' ? 'error' : site.isolation_status === 'partial' ? 'warning' : 'success'} showIcon message={`${site.name} · ${isolationLabels[site.isolation_status] ?? site.isolation_status}`} description="修改保存在当前草稿中，点击页面“保存配置”后持久化并写入审计。" />
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="范围 ID">{site.id}</Descriptions.Item>
        <Descriptions.Item label="上级范围">{site.parent_id || '租户根节点'}</Descriptions.Item>
        <Descriptions.Item label="CIDR">{site.cidr || '未限定'}</Descriptions.Item>
        <Descriptions.Item label="类型">{site.kind}</Descriptions.Item>
      </Descriptions>
      <label>隔离状态</label>
      <Select disabled={!canWrite} value={site.isolation_status} style={{ width: '100%' }} options={[{ value: 'isolated', label: '已隔离' }, { value: 'partial', label: '部分隔离' }, { value: 'unisolated', label: '未隔离' }]} onChange={(value) => onChange({ ...site, isolation_status: value })} />
    </Space>
  );
}

function IntegrationEditor({ integration, busy, canWrite, onChange, onTest }: { integration: IntegrationSetting; busy: boolean; canWrite: boolean; onChange: (value: IntegrationSetting) => void; onTest: () => void }) {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type="info" showIcon message={`${integration.name} 配置`} description="敏感信息只允许填写 secret:// 引用，页面和接口均不返回明文。" />
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="状态">{integration.status}</Descriptions.Item>
        <Descriptions.Item label="最近测试">{formatTestTime(integration.last_tested_at)}</Descriptions.Item>
      </Descriptions>
      <Space><span>启用集成</span><Switch disabled={!canWrite} checked={integration.enabled} onChange={(enabled) => onChange({ ...integration, enabled, status: enabled ? integration.status : 'disabled' })} /></Space>
      <label>Secret 引用</label>
      <Input disabled={!canWrite} value={integration.secret_ref} placeholder="secret://namespace/name/key" onChange={(event) => onChange({ ...integration, secret_ref: event.target.value })} />
      <label>Endpoint 提示（不含凭证）</label>
      <Input disabled={!canWrite} value={integration.endpoint_hint} placeholder="内部服务地址或配置说明" onChange={(event) => onChange({ ...integration, endpoint_hint: event.target.value })} />
      <Button data-settings-action="test-integration" type="primary" icon={<ApiOutlined />} loading={busy} disabled={!canWrite} onClick={onTest}>测试此连接</Button>
    </Space>
  );
}

function RetentionEditor({ policies, canWrite, onChange, onReview }: { policies: RetentionPolicy[]; canWrite: boolean; onChange: (index: number, policy: RetentionPolicy) => void; onReview: () => void }) {
  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Alert type="warning" showIcon message="生命周期变更会影响归档与删除窗口" description="调整后先执行复核，再点击页面保存配置；变更及复核结果均写入审计。" />
      {policies.map((policy, index) => (
        <div className="taf-settings-retention-editor" key={policy.data_type}>
          <strong>{policy.data_type}</strong>
          <InputNumber disabled={!canWrite} min={1} max={3650} value={policy.retention_days} addonAfter="天" onChange={(value) => onChange(index, { ...policy, retention_days: Number(value ?? policy.retention_days) })} />
          <Select disabled={!canWrite} value={policy.action} options={[{ value: 'delete', label: '到期删除' }, { value: 'archive', label: '到期归档' }]} onChange={(action) => onChange(index, { ...policy, action, next_action: action === 'archive' ? '到期归档' : '到期删除' })} />
        </div>
      ))}
      <Button icon={<AuditOutlined />} disabled={!canWrite} onClick={onReview}>复核生命周期影响</Button>
    </Space>
  );
}

function ImpactView({ impact }: { impact: SystemSettingsImpact }) {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type="warning" showIcon message={`风险级别：${impact.risk}`} description={impact.summary} />
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="租户">{impact.tenant_id}</Descriptions.Item>
        <Descriptions.Item label="配置版本">revision {impact.revision}</Descriptions.Item>
        <Descriptions.Item label="所需权限">{impact.approval}</Descriptions.Item>
        <Descriptions.Item label="审计动作">{impact.audit_action}</Descriptions.Item>
      </Descriptions>
      <div><Typography.Title level={5}>影响范围</Typography.Title>{impact.affected_scopes.map((scope) => <Tag key={scope} color="blue">{scope}</Tag>)}</div>
    </Space>
  );
}

function ActionResultView({ result, plainToken, onCopy }: { result: SystemSettingsActionResult; plainToken?: string; onCopy: () => void }) {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Alert type={result.status === 'warning' ? 'warning' : 'success'} showIcon message={result.message} description={`状态 ${result.status} · revision ${result.revision} · ${formatTestTime(result.updated_at)}`} />
      {plainToken && <div className="taf-settings-one-time-token"><Typography.Text strong>一次性令牌明文</Typography.Text><Input.Password readOnly value={plainToken} visibilityToggle /><Button icon={<CopyOutlined />} onClick={onCopy}>复制</Button><Typography.Text type="warning">关闭抽屉后页面不会再次展示该值。</Typography.Text></div>}
      {result.findings?.length ? <div><Typography.Title level={5}>审查结果</Typography.Title>{result.findings.map((finding) => <Alert key={finding} type="warning" showIcon message={finding} />)}</div> : null}
      {result.tokens && <Descriptions column={2} bordered size="small"><Descriptions.Item label="令牌总数">{result.tokens.total}</Descriptions.Item><Descriptions.Item label="有效令牌">{result.tokens.active}</Descriptions.Item><Descriptions.Item label="即将过期">{result.tokens.expiring_soon}</Descriptions.Item><Descriptions.Item label="已吊销">{result.tokens.revoked}</Descriptions.Item></Descriptions>}
    </Space>
  );
}

const renderSettingsCell = (column: string, value: unknown) => {
  if (column === '轮换状态') return <StatusTag value={value} />;
  if (column === '令牌指纹') return <span className="taf-settings-token-fingerprint">{String(value)}</span>;
  return String(value);
};

const settingsRowKey = (row: SnapshotRow) => String(row.token_id || row.令牌名称 || JSON.stringify(row));
const tokenIDFrom = (row?: SnapshotRow) => String(row?.token_id ?? '').trim();
const tokenActionable = (row?: SnapshotRow) => Boolean(tokenIDFrom(row)) && !['revoked', 'expired'].includes(String(row?.token_status ?? '').toLowerCase());
const scopesFrom = (row?: SnapshotRow) => String(row?.scopes ?? '').split(',').map((value) => value.trim()).filter(Boolean);

const fallbackMetric = (label: string): PageSnapshot['metrics'][number] => ({ label, value: '0', delta: 'API', status: 'info' });
const metricValue = (data: PageSnapshot | undefined, label: string, fallback: string) => data?.metrics.find((metric) => metric.label === label)?.value ?? fallback;

const scopeOptions = (scopes?: Array<{ name: string; description?: string }>) => (scopes ?? []).map((scope) => ({ value: scope.name, label: scope.description ? `${scope.name} · ${scope.description}` : scope.name }));

const roleName = (name: string) => ({ admin: '管理员', analyst: '研判员', viewer: '只读大屏账号', operator: '安全值班员', auditor: '审计员' }[name] ?? name);

const permissionState = (permissions: string[], target: string): 'ok' | 'pending' | 'locked' => {
  if (permissions.includes('*') || permissions.includes('admin:*') || permissions.includes(target)) return 'ok';
  const domain = target.split(':')[0];
  if (permissions.some((permission) => permission === `${domain}:*`)) return 'ok';
  if (target === 'playbook:execute' && permissions.some((permission) => permission.startsWith('script:'))) return 'pending';
  if (target === 'deploy:activate' && permissions.some((permission) => permission.startsWith('model:'))) return 'pending';
  return 'locked';
};

const siteLevels = (sites: SystemSite[]) => {
  const byID = new Map(sites.map((site) => [site.id, site]));
  const levels = new Map<string, number>();
  for (const site of sites) {
    let level = 0;
    let parentID = site.parent_id;
    const seen = new Set<string>();
    while (parentID && byID.has(parentID) && !seen.has(parentID) && level < 3) {
      seen.add(parentID);
      level += 1;
      parentID = byID.get(parentID)?.parent_id;
    }
    levels.set(site.id, level);
  }
  return levels;
};

const formatRetention = (days: number) => days % 365 === 0 ? `${days / 365} 年` : `${days} 天`;
const formatTestTime = (value?: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '尚未测试';
const readTokenPermissions = (token: string | null): string[] => {
  if (!token) return [];
  try {
    const base64 = token.split('.')[1]?.replace(/-/g, '+').replace(/_/g, '/');
    if (!base64) return [];
    const payload = JSON.parse(atob(base64.padEnd(Math.ceil(base64.length / 4) * 4, '='))) as { permissions?: unknown };
    return Array.isArray(payload.permissions) ? payload.permissions.filter((value): value is string => typeof value === 'string') : [];
  } catch {
    return [];
  }
};
const hasSettingsScope = (permissions: string[], required: string) => permissions.some((permission) => permission === '*' || permission === required || (permission.endsWith(':*') && required.startsWith(permission.slice(0, -1))));
const drawerTitle = (drawer: DrawerState) => {
  if (!drawer) return '';
  if (drawer.kind === 'site') return `租户范围 / ${drawer.site.name}`;
  if (drawer.kind === 'integration') return `集成配置 / ${drawer.integration.name}`;
  if (drawer.kind === 'retention') return '数据生命周期策略';
  if (drawer.kind === 'token-scopes') return 'API 令牌权限配置';
  if (drawer.kind === 'impact') return '配置影响范围';
  return '操作结果与审计上下文';
};
