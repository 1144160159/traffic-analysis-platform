import {
  BellOutlined,
  CheckCircleOutlined,
  ControlOutlined,
  DatabaseOutlined,
  DownOutlined,
  DotChartOutlined,
  ExpandOutlined,
  FileSearchOutlined,
  GlobalOutlined,
  HddOutlined,
  MenuOutlined,
  PoweroffOutlined,
  QuestionCircleOutlined,
  RadarChartOutlined,
  SafetyOutlined,
  SearchOutlined,
  SettingOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons';
import { Avatar, Badge, Button, Drawer, Dropdown, Select, Tooltip } from 'antd';
import type { MenuProps } from 'antd';
import { useEffect, useState } from 'react';
import dayjs from 'dayjs';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import screenTopbarShieldIcon from '@/assets/screenshot-icons/screen-topbar-shield-exact@3x.png';
import { appConfig } from '@/config/runtime';
import { findRouteByPath, navGroups } from '@/routes/routeManifest';
import { visibleNavGroups } from '@/routes/access';
import type { SessionPrincipal } from '@/routes/access';
import { logout } from '@/services/api';
import { useAuthorizedRealtime } from '@/services/realtime';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';
import { getWindowFrameCssVars, useWindowFrameState } from '@/utils/windowFrameState';

type AppShellProps = {
  children: React.ReactNode;
  currentUser?: SessionPrincipal;
};

const quickEntries = [
  { label: 'PCAP检索', icon: <FileSearchOutlined /> },
  { label: '资产检索', icon: <SearchOutlined /> },
  { label: '规则检索', icon: <ToolOutlined /> },
  { label: '脚本中心', icon: <DatabaseOutlined /> },
  { label: '帮助中心', icon: <QuestionCircleOutlined /> },
  { label: '更多应用', icon: <ExpandOutlined /> },
];

export function AppShell({ children, currentUser }: AppShellProps) {
  useEllipsizedTextTitles();
  const windowFrame = useWindowFrameState();
  const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => new Set());
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const visualBreakdownMode = isVisualBreakdownMode();
  const realtime = useAuthorizedRealtime(visualBreakdownMode ? undefined : currentUser);
  const allowedGroups = visibleNavGroups(navGroups, currentUser);
  const activeRoute = findRouteByPath(location.pathname) ?? navGroups[0].children[0];
  const encryptedTrafficRoute = location.pathname === '/encrypted-traffic';
  const activeNavId = activeRoute.activeNavId ?? activeRoute.id;
  const activeGroup = allowedGroups.find((group) => group.id === activeRoute.domain) ?? allowedGroups[0] ?? navGroups[0];
  const username = visualBreakdownMode ? 'sec_analyst' : currentUser?.username ?? 'sec_analyst';
  const role = visualBreakdownMode
    ? '安全分析师'
    : currentUser?.role || (currentUser?.roles?.includes('admin') ? '安全分析师' : currentUser?.roles?.join(' / ')) || '安全分析师';
  const shellClassName = [
    'taf-shell',
    encryptedTrafficRoute ? 'taf-shell--encrypted-traffic' : '',
    visualBreakdownMode ? 'taf-shell--visual-breakdown' : '',
    windowFrame.windowed ? 'taf-shell--os-windowed' : '',
    windowFrame.constrainedWidth ? 'taf-shell--os-constrained-width' : '',
    windowFrame.compactWidth ? 'taf-shell--os-compact-width' : '',
    windowFrame.compactHeight ? 'taf-shell--os-compact-height' : '',
    windowFrame.shortHeight ? 'taf-shell--os-short-height' : '',
    windowFrame.narrowWidth ? 'taf-shell--os-narrow-width' : '',
  ]
    .filter(Boolean)
    .join(' ');
  const shellStyle = getWindowFrameCssVars(windowFrame);

  const handleGroupClick = (groupId: string, firstPath: string) => {
    if (groupId !== activeGroup.id) {
      setCollapsedGroups((current) => {
        const next = new Set(current);
        next.delete(groupId);
        return next;
      });
      navigate(firstPath);
      return;
    }
    setCollapsedGroups((current) => {
      const next = new Set(current);
      if (next.has(groupId)) next.delete(groupId);
      else next.add(groupId);
      return next;
    });
  };

  const userMenu: MenuProps = {
    items: [
      { key: 'settings', label: '系统设置' },
      { key: 'audit-log', label: '审计日志' },
      { type: 'divider' },
      { key: 'logout', label: '退出登录', danger: true },
    ],
    onClick: ({ key }) => {
      if (key === 'logout') {
        void logout()
          .catch(() => undefined)
          .finally(() => {
            queryClient.removeQueries({ queryKey: ['current-user'] });
            navigate('/login', { replace: true });
          });
        return;
      }
      if (key === 'settings') navigate('/settings');
      if (key === 'audit-log') navigate('/audit-log');
    },
  };

  return (
    <div
      className={shellClassName}
      style={shellStyle}
      data-taf-window-inner={`${windowFrame.innerWidth}x${windowFrame.innerHeight}`}
      data-taf-window-outer={`${windowFrame.outerWidth}x${windowFrame.outerHeight}`}
      data-taf-window-frame={`${windowFrame.frameWidth}x${windowFrame.frameHeight}`}
      data-taf-screen-avail={`${windowFrame.screenAvailWidth}x${windowFrame.screenAvailHeight}`}
    >
      <header className="taf-topbar">
        <div className="taf-brand-wrap">
          <Tooltip title="移动端导航" open={visualBreakdownMode ? false : undefined}>
            <Button className="taf-mobile-nav-trigger" type="text" size="small" icon={<MenuOutlined />} aria-label="移动端导航" onClick={() => setMobileNavigationOpen(true)} />
          </Tooltip>
          <Link to="/dashboard" className="taf-brand" aria-label="返回仪表盘">
            <span className="taf-brand__shield" aria-hidden="true">
              <img
                className="taf-brand__shield-mark"
                src={screenTopbarShieldIcon}
                alt=""
                decoding="async"
                draggable={false}
                data-generated-icon="screenshot-screen-topbar-shield-exact"
              />
            </span>
            <span className="taf-brand__title">{appConfig.productName}</span>
          </Link>
        </div>
        <div className="taf-topbar__metrics">
          <div className="taf-topbar__block">
            <span>站点</span>
            <Select
              size="small"
              value="主园区"
              options={[{ value: '主园区' }, { value: '教学区' }, { value: '数据中心' }]}
            />
          </div>
          <TopMetric label="时间" value={visualBreakdownMode ? '2026-06-20 03:45:00' : dayjs().format('YYYY-MM-DD HH:mm:ss')} />
          <TopMetric label="风险态势" value="高风险 87/100" tone="danger" icon={<SafetyOutlined />} />
          <TopMetric label="告警总数" value="128 / 24h" tone="warn" icon={<BellOutlined />} />
          <TopMetric label="关键告警" value="9 / 24h" tone="danger" icon={<RadarChartOutlined />} />
          <TopMetric label="采集健康度" value="98.6% 在线探针 24/25" tone="ok" />
          <TopMetric label="数据质量" value="99.1% 合格率" tone="ok" />
        </div>
        <div className="taf-topbar__actions">
          {quickEntries.map((entry) => (
            <Tooltip key={entry.label} title={entry.label} open={visualBreakdownMode ? false : undefined}>
              <Button type="text" size="small" icon={entry.icon} aria-label={entry.label}>
                {entry.label}
              </Button>
            </Tooltip>
          ))}
        </div>
      </header>

      <Drawer
        className="taf-mobile-navigation-drawer"
        title="移动端导航"
        placement="left"
        width={360}
        open={mobileNavigationOpen}
        onClose={() => setMobileNavigationOpen(false)}
      >
        <div className="taf-mobile-navigation-drawer__groups">
          {allowedGroups.map((group) => (
            <section key={group.id}>
              <strong>{group.title}</strong>
              {group.children.map((route) => (
                <Link
                  key={route.id}
                  to={route.path}
                  className={route.id === activeNavId ? 'is-active' : undefined}
                  aria-current={route.id === activeNavId ? 'page' : undefined}
                  onClick={() => setMobileNavigationOpen(false)}
                >
                  <span>{route.icon}</span>
                  <em>{route.title}</em>
                  {route.badge && <Badge count={route.badge} overflowCount={999} />}
                </Link>
              ))}
            </section>
          ))}
        </div>
        <div className="taf-mobile-navigation-drawer__audit">
          <SafetyOutlined />
          <span>移动端导航仅承载路由跳转，权限仍由 requiredScopes 与审计 trace 控制。</span>
        </div>
      </Drawer>

      <aside className="taf-sidebar">
        <nav className="taf-sidebar__nav" aria-label="主导航">
          {allowedGroups.map((group) => {
            const isActive = group.id === activeGroup.id;
            const isExpanded = isActive && !collapsedGroups.has(group.id);
            return (
            <section key={group.id} className={isActive ? 'is-active' : undefined}>
              <button
                type="button"
                className={`taf-sidebar__group ${isActive ? 'is-active' : ''}`}
                aria-expanded={isExpanded}
                onClick={() => handleGroupClick(group.id, group.children[0].path)}
              >
                <span className="taf-sidebar__icon">{group.icon}</span>
                <span>{group.title}</span>
                <DownOutlined className="taf-sidebar__chevron" />
              </button>
              {isExpanded && (
                <div className="taf-sidebar__children">
                  {group.children.map((route) => (
                    <Link
                      key={route.id}
                      to={route.path}
                      className={`taf-sidebar__item ${route.id === activeNavId ? 'is-active' : ''}`}
                      aria-current={route.id === activeNavId ? 'page' : undefined}
                    >
                      <span className="taf-sidebar__icon">{route.icon}</span>
                      <span>{route.title}</span>
                      {route.badge && <Badge count={route.badge} overflowCount={999} />}
                    </Link>
                  ))}
                </div>
              )}
            </section>
            );
          })}
        </nav>
        <Dropdown menu={userMenu} placement="topLeft">
          <button className="taf-user" type="button">
            <Avatar size={34}>{username.slice(0, 1).toUpperCase()}</Avatar>
            <span>
              <strong>{username}</strong>
              <small>
                <em>{role}</em>
                <b>在线</b>
              </small>
            </span>
            <ExpandOutlined className="taf-user__screen" />
          </button>
        </Dropdown>
      </aside>

      <main className="taf-main">{children}</main>

      <footer className="taf-bottombar">
        <BottomMetric label="数据延迟" value="1.23 s" icon={<CheckCircleOutlined />} />
        <BottomMetric label="系统运行" value="23 天 14 小时" icon={<ThunderboltOutlined />} />
        <BottomMetric label="告警处理SLA" value="98.2%" icon={<SafetyOutlined />} />
        <BottomMetric label="数据质量合格率" value="99.1%" icon={<RadarChartOutlined />} />
        <BottomMetric label="存储使用" value="68.7 / 120 TB (57%)" icon={<HddOutlined />} />
        <BottomMetric label="带宽使用" value="42.7 / 100 Gbps (43%)" icon={<GlobalOutlined />} />
        <BottomMetric label="日志吞吐" value="12.6 K EPS" icon={<DotChartOutlined />} />
        {!visualBreakdownMode && appConfig.enableRealtime && (
          <span
            className={`taf-bottombar__realtime taf-tone-${
              realtime.status === 'connected'
                ? 'ok'
                : realtime.status === 'error' || realtime.status === 'closed'
                  ? 'danger'
                  : 'warn'
            }`}
          >
            <ThunderboltOutlined />
            <span>实时通道</span>
            <strong>{realtime.label}</strong>
          </span>
        )}
        <span className="taf-bottombar__icons">
          <Badge count={9}>
            <BellOutlined />
          </Badge>
          <SettingOutlined />
          <ControlOutlined />
          <PoweroffOutlined />
        </span>
      </footer>
    </div>
  );
}

function useEllipsizedTextTitles() {
  useEffect(() => {
    const root = document.querySelector<HTMLElement>('.taf-shell');
    if (!root) return undefined;

    let frame = 0;
    const syncTitles = () => {
      frame = 0;
      const candidates = root.querySelectorAll<HTMLElement>('button, a, span, strong, p, h1, h2, h3, td, th, em, b, code, label');
      candidates.forEach((element) => {
        const text = element.textContent?.replace(/\s+/g, ' ').trim() ?? '';
        const autoTitle = element.dataset.tafAutoTitle === '1';
        const clipped = text.length > 0 && (
          element.scrollWidth > element.clientWidth + 1 ||
          element.scrollHeight > element.clientHeight + 1
        );
        if (clipped && (!element.getAttribute('title') || autoTitle)) {
          element.setAttribute('title', text);
          element.dataset.tafAutoTitle = '1';
        } else if (!clipped && autoTitle) {
          element.removeAttribute('title');
          delete element.dataset.tafAutoTitle;
        }
      });
    };
    const schedule = () => {
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(syncTitles);
    };

    schedule();
    const resizeObserver = new ResizeObserver(schedule);
    resizeObserver.observe(root);
    const mutationObserver = new MutationObserver(schedule);
    mutationObserver.observe(root, { childList: true, subtree: true, characterData: true });
    window.addEventListener('resize', schedule);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      resizeObserver.disconnect();
      mutationObserver.disconnect();
      window.removeEventListener('resize', schedule);
    };
  }, []);
}

function TopMetric({
  label,
  value,
  tone = 'default',
  icon,
}: {
  label: string;
  value: string;
  tone?: 'default' | 'ok' | 'warn' | 'danger';
  icon?: React.ReactNode;
}) {
  return (
    <div className={`taf-topbar__block taf-tone-${tone}`}>
      <span>{label}</span>
      <strong>
        {icon}
        {value}
      </strong>
    </div>
  );
}

function BottomMetric({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) {
  return (
    <span className="taf-bottombar__metric">
      {icon}
      <span>{label}</span>
      <strong>{value}</strong>
    </span>
  );
}
