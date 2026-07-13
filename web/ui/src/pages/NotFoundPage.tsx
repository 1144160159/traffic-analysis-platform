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
  SafetyCertificateOutlined,
  SettingOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { message } from 'antd';
import type { ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import type { NavRoute } from '@/routes/routeManifest';

const traceId = 'trace-20260621-8F3A';

type Entry = {
  id: string;
  label: string;
  detail: string;
  to: string;
  icon: ReactNode;
  tone?: 'info' | 'warn' | 'ok';
};

const statusRows = [
  { label: '网关服务', value: '正常', icon: <GatewayOutlined /> },
  { label: '鉴权服务', value: '正常', icon: <SafetyCertificateOutlined /> },
  { label: '前端路由', value: '正常', icon: <GlobalOutlined /> },
  { label: '审计写入', value: '正常', icon: <AuditOutlined /> },
];

const factRows = [
  { label: '追踪 ID', value: traceId, icon: <CopyOutlined /> },
  { label: '时间', value: '2026-06-21 14:30:22', icon: <ClockCircleOutlined /> },
  { label: '当前租户 / 站点', value: '园区A', icon: <SettingOutlined /> },
  { label: '访问来源', value: '内网访问', icon: <UserOutlined /> },
];

function routePath(knownRoutes: NavRoute[], id: string, fallback: string) {
  const route = knownRoutes.find((item) => item.id === id);
  return route?.path ?? fallback;
}

export function NotFoundPage({ knownRoutes }: { knownRoutes: NavRoute[] }) {
  const navigate = useNavigate();
  const entries: Entry[] = [
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
  ];

  const copyTraceId = async () => {
    try {
      if (!navigator.clipboard) throw new Error('clipboard unavailable');
      await navigator.clipboard.writeText(traceId);
      message.success('追踪 ID 已复制');
    } catch {
      message.warning('浏览器未授权复制，请记录追踪 ID');
    }
  };

  return (
    <main
      className="taf-notfound"
      aria-labelledby="taf-notfound-title"
    >
      <span className="taf-notfound__mesh" aria-hidden="true">
        {Array.from({ length: 10 }, (_, index) => <i key={index} />)}
      </span>
      <section className="taf-notfound__main">
        <div className="taf-notfound__hero">
          <div className="taf-notfound__visual" aria-hidden="true">
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
                  <strong>
                    {item.value}
                    {item.icon}
                  </strong>
                </div>
              ))}
            </div>
          </div>
        </div>

        <section className="taf-notfound__section" aria-labelledby="taf-notfound-return">
          <h2 id="taf-notfound-return">返回入口</h2>
          <div className="taf-notfound__return-grid">
            <Link to={entries[0].to}>
              <DashboardOutlined />
              <span>返回仪表盘</span>
            </Link>
            <Link to={entries[1].to}>
              <FundProjectionScreenOutlined />
              <span>返回态势大屏</span>
            </Link>
            <Link to={entries[2].to} className="is-warn">
              <BellOutlined />
              <span>返回告警中心</span>
            </Link>
            <button type="button" onClick={() => navigate(-1)}>
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
            <Link to={entries[3].to}>
              <AuditOutlined />
              <span>查看审计日志</span>
            </Link>
            <Link to={routePath(knownRoutes, 'settings', '/settings')}>
              <UserOutlined />
              <span>联系管理员</span>
            </Link>
            <button type="button" onClick={() => void copyTraceId()}>
              <CopyOutlined />
              <span>复制追踪 ID</span>
            </button>
          </div>
        </section>
      </section>

      <aside className="taf-notfound__rail">
        <section className="taf-notfound__rail-card" aria-labelledby="taf-notfound-recent">
          <h2 id="taf-notfound-recent">最近可用入口</h2>
          <div className="taf-notfound__entries">
            {entries.map((entry) => (
              <Link key={entry.id} to={entry.to} className={`taf-notfound__entry is-${entry.tone ?? 'info'}`}>
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
              <div key={item.label} className="taf-notfound__status-row">
                <span>
                  {item.icon}
                  {item.label}
                </span>
                <strong>
                  <CheckCircleOutlined />
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
