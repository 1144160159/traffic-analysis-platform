import {
  AuditOutlined,
  BellOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  CopyOutlined,
  DashboardOutlined,
  FundProjectionScreenOutlined,
  GatewayOutlined,
  GlobalOutlined,
  LeftOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs from 'dayjs';
import { useMemo, useRef, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { NavRoute } from '@/routes/routeManifest';
import { hasRouteAccess } from '@/routes/access';
import { localBypassUser, type CurrentUser } from '@/services/api';
import { recordNavigationMiss, requestNavigationSupport } from '@/services/notFoundApi';

type Entry = {
  id: string;
  label: string;
  detail: string;
  to: string;
  icon: ReactNode;
  tone?: 'info' | 'warn' | 'ok';
};

const statusIcons: Record<string, ReactNode> = {
  gateway: <GatewayOutlined />, auth: <SafetyCertificateOutlined />, 'frontend-route': <GlobalOutlined />, 'audit-write': <AuditOutlined />,
};

function routePath(knownRoutes: NavRoute[], id: string, fallback: string) {
  const route = knownRoutes.find((item) => item.id === id);
  return route?.path ?? fallback;
}

function createOpaqueNavigationEventId() {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, (value) => value.toString(16).padStart(2, '0')).join('');
  return `nav-${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

async function writeSafeClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch {
      // HTTP deployments may expose the Clipboard API but deny writes.
    }
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  textarea.addEventListener('copy', (event) => {
    event.clipboardData?.setData('text/plain', value);
    event.preventDefault();
  }, { once: true });
  const copied = document.execCommand('copy');
  textarea.remove();
  if (!copied) throw new Error('clipboard unavailable');
}

export function NotFoundPage({ knownRoutes }: { knownRoutes: NavRoute[] }) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const eventId = useRef(createOpaqueNavigationEventId()).current;
  const [contactOpen, setContactOpen] = useState(false);
  const [localFeedback, setLocalFeedback] = useState('');
  const context = useQuery({
    queryKey: ['navigation-miss', eventId],
    queryFn: () => recordNavigationMiss(eventId),
    retry: 1,
    retryDelay: 0,
    staleTime: Number.POSITIVE_INFINITY,
  });
  const currentUser = queryClient.getQueryData<CurrentUser>(['current-user']) ?? localBypassUser;
  const entries = useMemo(() => ([
    {
      id: 'dashboard',
      label: '仪表盘',
      detail: '全局概览与关键指标',
      to: routePath(knownRoutes, 'dashboard', '/dashboard'),
      icon: <DashboardOutlined />,
      tone: 'info',
    },
    {
      id: 'screen',
      label: '态势大屏',
      detail: '安全态势可视化大屏',
      to: routePath(knownRoutes, 'screen', '/screen'),
      icon: <FundProjectionScreenOutlined />,
      tone: 'info',
    },
    {
      id: 'alerts',
      label: '告警中心',
      detail: '告警检索与处置中心',
      to: routePath(knownRoutes, 'alerts', '/alerts'),
      icon: <BellOutlined />,
      tone: 'warn',
    },
    {
      id: 'audit-log',
      label: '审计日志',
      detail: '操作审计与日志检索',
      to: routePath(knownRoutes, 'audit-log', '/audit-log'),
      icon: <AuditOutlined />,
      tone: 'info',
    },
  ] as Entry[]).filter((entry) => {
    const route = knownRoutes.find((item) => item.id === entry.id);
    return route ? hasRouteAccess(route, currentUser) : false;
  }), [currentUser, knownRoutes]);
  const support = useMutation({ mutationFn: () => requestNavigationSupport(eventId) });
  const returnEntries = entries.filter((entry) => entry.id !== 'audit-log');
  const auditEntry = entries.find((entry) => entry.id === 'audit-log');

  const traceId = context.data?.trace_id ?? '';
  const contextFailed = context.isError;
  const factRows = [
    { label: '追踪 ID', value: traceId || (contextFailed ? '追踪记录失败' : '正在生成安全追踪标识'), icon: <CopyOutlined /> },
    { label: '时间', value: context.data?.occurred_at ? dayjs(context.data.occurred_at).format('YYYY-MM-DD HH:mm:ss') : contextFailed ? '记录失败' : '正在记录', icon: <ClockCircleOutlined /> },
    { label: '当前租户 / 站点', value: context.data ? `${context.data.tenant_name} / ${context.data.site_name}` : contextFailed ? '上下文不可用' : '正在读取租户上下文', icon: <SettingOutlined /> },
    { label: '访问来源', value: context.data?.access_source ?? (contextFailed ? '确认失败' : '正在确认'), icon: <UserOutlined /> },
  ];
  const fallbackStatus = contextFailed ? { state: 'unavailable', value: '不可用' } : { state: 'pending', value: '确认中' };
  const statusRows = context.data?.statuses ?? [
    { id: 'gateway', label: '网关服务', ...fallbackStatus },
    { id: 'auth', label: '鉴权服务', ...fallbackStatus },
    { id: 'frontend-route', label: '前端路由', ...fallbackStatus },
    { id: 'audit-write', label: '审计写入', ...fallbackStatus },
  ];

  const copyTraceId = async () => {
    if (!traceId) {
      setLocalFeedback('追踪 ID 尚未生成，请先重试追踪记录');
      return;
    }
    try {
      await writeSafeClipboard(traceId);
      setLocalFeedback('追踪 ID 已复制');
    } catch {
      setLocalFeedback('浏览器未授权复制，请手动记录追踪 ID');
    }
  };

  const returnToPrevious = () => {
    const routerIndex = Number(window.history.state?.idx ?? 0);
    if (routerIndex > 0) navigate(-1);
    else navigate(entries[0]?.to ?? '/login');
  };

  const contactAdministrator = () => {
    setContactOpen(true);
    if (context.data && !support.data && !support.isPending) support.mutate();
  };

  return (
    <main
      className="taf-notfound"
      aria-labelledby="taf-notfound-title"
    >
      <section className="taf-notfound__main">
        <div className="taf-notfound__hero">
          <div className="taf-notfound__visual" aria-hidden="true">
            <img src="/ui-assets/backgrounds/not-found.png" alt="" data-screenshot-background="not-found" />
            <strong>404</strong>
            <span>页面不存在</span>
          </div>

          <div className="taf-notfound__summary">
            <h1 id="taf-notfound-title">404 页面不存在</h1>
            <p>请求的页面不可用或已被移除。</p>
            <div className="taf-notfound__facts">
              {factRows.map((item) => (
                <div key={item.label} className="taf-notfound__fact">
                  <span>{item.label}</span>
                  <strong data-notfound-fact={item.label === '追踪 ID' ? 'trace-id' : undefined}>
                    {item.value}
                    {item.icon}
                  </strong>
                </div>
              ))}
            </div>
          </div>
        </div>

        {context.isError && (
          <div className="taf-notfound__context-state is-error" role="alert" data-notfound-state="context-error">
            <SafetyCertificateOutlined />
            <span><strong>追踪记录暂不可用</strong><em>页面导航仍可使用；请重试生成审计追踪信息。</em></span>
            <button type="button" data-notfound-action="retry-context" onClick={() => void context.refetch()}>
              <ReloadOutlined />
              重试
            </button>
          </div>
        )}

        <section className="taf-notfound__section" aria-labelledby="taf-notfound-return">
          <h2 id="taf-notfound-return">返回入口</h2>
          <div className="taf-notfound__return-grid">
            {returnEntries.map((entry) => (
              <Link key={entry.id} to={entry.to} className={entry.tone === 'warn' ? 'is-warn' : undefined} data-notfound-action={`return-${entry.id}`}>
                {entry.icon}
                <span>返回{entry.label}</span>
              </Link>
            ))}
            <button type="button" data-notfound-action="previous" onClick={returnToPrevious}>
              <LeftOutlined />
              <span>返回上一页</span>
            </button>
          </div>
        </section>

        <div className="taf-notfound__notice">
          <SafetyCertificateOutlined />
          <div>
            <strong>安全提示</strong>
            <p>不展示内部路径、堆栈、凭据或接口细节。如反复出现此页面，请联系管理员并提供追踪 ID 以便快速定位问题。</p>
          </div>
        </div>

        <section className="taf-notfound__section" aria-labelledby="taf-notfound-actions">
          <h2 id="taf-notfound-actions">辅助操作</h2>
          <div className="taf-notfound__aux">
            {auditEntry && (
              <Link to={auditEntry.to} data-notfound-action="audit-log">
                <AuditOutlined />
                <span>查看审计日志</span>
              </Link>
            )}
            <button type="button" data-notfound-action="contact-admin" onClick={contactAdministrator}>
              <UserOutlined />
              <span>联系管理员</span>
            </button>
            <button type="button" data-notfound-action="copy-trace" onClick={() => void copyTraceId()}>
              <CopyOutlined />
              <span>复制追踪 ID</span>
            </button>
          </div>
          {localFeedback && <div className="taf-notfound__feedback" role="status" data-notfound-feedback>{localFeedback}</div>}
        </section>

        {contactOpen && (
          <aside className="taf-notfound__contact" role="dialog" aria-modal="false" aria-labelledby="taf-notfound-contact-title" data-notfound-contact-panel>
            <header>
              <span><UserOutlined /><strong id="taf-notfound-contact-title">管理员联系请求</strong></span>
              <button type="button" aria-label="关闭管理员联系界面" data-notfound-action="close-contact" onClick={() => setContactOpen(false)}>×</button>
            </header>
            {support.isPending && <div className="taf-notfound__contact-state is-pending" role="status">正在提交至平台值班管理员队列…</div>}
            {support.isError && (
              <div className="taf-notfound__contact-state is-error" role="alert">
                <strong>联系请求提交失败</strong>
                <span>未产生虚假成功记录，可在此业务区内重试。</span>
                <button type="button" data-notfound-action="retry-contact" onClick={() => support.mutate()}><ReloadOutlined />重新提交</button>
              </div>
            )}
            {support.data && (
              <div className="taf-notfound__contact-state is-success" data-notfound-support-request={support.data.support_request_id}>
                <CheckCircleOutlined />
                <strong>已提交至{support.data.queue}</strong>
                <dl>
                  <div><dt>请求编号</dt><dd>{support.data.support_request_id}</dd></div>
                  <div><dt>联系追踪 ID</dt><dd>{support.data.trace_id}</dd></div>
                  <div><dt>状态</dt><dd>已排队</dd></div>
                </dl>
                {auditEntry && <Link to={auditEntry.to} data-notfound-action="contact-audit-log"><AuditOutlined />在审计日志中查看</Link>}
              </div>
            )}
            {!context.data && !context.isLoading && (
              <div className="taf-notfound__contact-state is-error" role="alert">请先重试追踪记录，再提交管理员联系请求。</div>
            )}
          </aside>
        )}
      </section>

      <aside className="taf-notfound__rail">
        <section className="taf-notfound__rail-card" aria-labelledby="taf-notfound-recent">
          <h2 id="taf-notfound-recent">最近可用入口</h2>
          <div className="taf-notfound__entries">
            {entries.map((entry) => (
              <Link key={entry.id} to={entry.to} data-notfound-entry={entry.id} className={`taf-notfound__entry is-${entry.tone ?? 'info'}`}>
                {entry.icon}
                <span>
                  <strong>{entry.label}</strong>
                  <em>{entry.detail}</em>
                </span>
                <LeftOutlined />
              </Link>
            ))}
          </div>
        </section>

        <section className="taf-notfound__rail-card" aria-labelledby="taf-notfound-status">
          <h2 id="taf-notfound-status">相关系统状态</h2>
          <div className="taf-notfound__status">
            {statusRows.map((item) => (
              <div key={item.id} className={`taf-notfound__status-row is-${item.state}`}>
                <span>
                  {statusIcons[item.id] ?? <GlobalOutlined />}
                  {item.label}
                </span>
                <strong>
                  {item.state === 'healthy' ? <CheckCircleOutlined /> : <ClockCircleOutlined />}
                  {item.value}
                </strong>
              </div>
            ))}
          </div>
        </section>
      </aside>
    </main>
  );
}
