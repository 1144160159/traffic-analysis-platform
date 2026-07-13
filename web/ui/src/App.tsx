import { lazy, Suspense } from 'react';
import type { ReactNode } from 'react';
import { ConfigProvider, theme } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query';
import { Navigate, Route, BrowserRouter as Router, Routes } from 'react-router-dom';
import { AppShell } from '@/layouts/AppShell';
import { allRoutes, detailRoutes, navRoutes } from '@/routes/routeManifest';
import type { NavRoute } from '@/routes/routeManifest';
import { hasRouteAccess } from '@/routes/access';
import { fetchCurrentUser, localBypassUser } from '@/services/api';
import { appConfig } from '@/config/runtime';
import { consumeDesktopSmokeToken, getAuthToken } from '@/services/authStorage';
import { isVisualBreakdownMode } from '@/utils/visualBreakdownMode';
import { useDocumentWindowFrameCssVars } from '@/utils/windowFrameState';

const routerBasename = import.meta.env.BASE_URL === '/' ? undefined : import.meta.env.BASE_URL.replace(/\/$/, '');

const LoginPage = lazy(() => import('@/pages/LoginPage').then((module) => ({ default: module.LoginPage })));
const OidcCallbackPage = lazy(() => import('@/pages/OidcCallbackPage').then((module) => ({ default: module.OidcCallbackPage })));
const ProductPage = lazy(() => import('@/pages/ProductPage').then((module) => ({ default: module.ProductPage })));
const SituationalScreen = lazy(() => import('@/pages/SituationalScreen').then((module) => ({ default: module.SituationalScreen })));
const DashboardOperationsPage = lazy(() => import('@/pages/DashboardOperationsPage').then((module) => ({ default: module.DashboardOperationsPage })));
const TopicWorkbenchPage = lazy(() => import('@/pages/TopicWorkbenchPage').then((module) => ({ default: module.TopicWorkbenchPage })));
const ProbesManagementPage = lazy(() => import('@/pages/ProbesManagementPage').then((module) => ({ default: module.ProbesManagementPage })));
const DataQualityPage = lazy(() => import('@/pages/DataQualityPage').then((module) => ({ default: module.DataQualityPage })));
const AlertTriagePage = lazy(() => import('@/pages/AlertTriagePage').then((module) => ({ default: module.AlertTriagePage })));
const AssetInventoryPage = lazy(() => import('@/pages/AssetInventoryPage').then((module) => ({ default: module.AssetInventoryPage })));
const GraphEntityPage = lazy(() => import('@/pages/GraphEntityPage').then((module) => ({ default: module.GraphEntityPage })));
const FusionWorkbenchPage = lazy(() => import('@/pages/FusionWorkbenchPage').then((module) => ({ default: module.FusionWorkbenchPage })));
const BaselineWorkbenchPage = lazy(() => import('@/pages/BaselineWorkbenchPage').then((module) => ({ default: module.BaselineWorkbenchPage })));
const CampaignWorkbenchPage = lazy(() => import('@/pages/CampaignWorkbenchPage').then((module) => ({ default: module.CampaignWorkbenchPage })));
const AttackChainAnalysisPage = lazy(() => import('@/pages/AttackChainAnalysisPage').then((module) => ({ default: module.AttackChainAnalysisPage })));
const EncryptedTrafficPage = lazy(() => import('@/pages/EncryptedTrafficPage').then((module) => ({ default: module.EncryptedTrafficPage })));
const ForensicsWorkbenchPage = lazy(() => import('@/pages/ForensicsWorkbenchPage').then((module) => ({ default: module.ForensicsWorkbenchPage })));
const RuleManagementPage = lazy(() => import('@/pages/RuleManagementPage').then((module) => ({ default: module.RuleManagementPage })));
const DeploymentManagementPage = lazy(() => import('@/pages/DeploymentManagementPage').then((module) => ({ default: module.DeploymentManagementPage })));
const ModelManagementPage = lazy(() => import('@/pages/ModelManagementPage').then((module) => ({ default: module.ModelManagementPage })));
const MlopsOrchestrationPage = lazy(() => import('@/pages/MlopsOrchestrationPage').then((module) => ({ default: module.MlopsOrchestrationPage })));
const PlaybookAutomationPage = lazy(() => import('@/pages/PlaybookAutomationPage').then((module) => ({ default: module.PlaybookAutomationPage })));
const WhitelistGovernancePage = lazy(() => import('@/pages/WhitelistGovernancePage').then((module) => ({ default: module.WhitelistGovernancePage })));
const ComplianceAuditPage = lazy(() => import('@/pages/ComplianceAuditPage').then((module) => ({ default: module.ComplianceAuditPage })));
const AuditLogPage = lazy(() => import('@/pages/AuditLogPage').then((module) => ({ default: module.AuditLogPage })));
const NotificationConfigPage = lazy(() => import('@/pages/NotificationConfigPage').then((module) => ({ default: module.NotificationConfigPage })));
const SettingsGovernancePage = lazy(() => import('@/pages/SettingsGovernancePage').then((module) => ({ default: module.SettingsGovernancePage })));
const AlertDetailPage = lazy(() => import('@/pages/AlertDetailPage').then((module) => ({ default: module.AlertDetailPage })));
const CampaignDetailPage = lazy(() => import('@/pages/CampaignDetailPage').then((module) => ({ default: module.CampaignDetailPage })));
const DetailPage = lazy(() => import('@/pages/DetailPage').then((module) => ({ default: module.DetailPage })));
const NotFoundPage = lazy(() => import('@/pages/NotFoundPage').then((module) => ({ default: module.NotFoundPage })));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});

const hasLocalSession = () => {
  if (!appConfig.authEnabled) return true;
  consumeDesktopSmokeToken(appConfig.desktopSmokeTokenEnabled);
  return Boolean(getAuthToken());
};

const screenDemoUser = {
  username: 'screen_demo',
  role: '脱敏大屏观察员',
  roles: ['screen-viewer'],
  permissions: ['screen:view'],
};

const isScreenMaskedDemoRoute = (route?: NavRoute) => route?.id === 'screen' && appConfig.screenAccessMode === 'masked-demo';

function RouteLoading() {
  return (
    <div className="taf-route-loading" role="status" aria-live="polite">
      <span />
      <strong>页面加载中</strong>
    </div>
  );
}

function AccessDenied({ route }: { route?: NavRoute }) {
  return (
    <section className="taf-access-denied">
      <span>403</span>
      <h1>权限不足</h1>
      <p>{route ? `当前账号缺少访问「${route.title}」所需权限。` : '当前账号缺少访问该页面所需权限。'}</p>
      <div>
        {(route?.requiredScopes ?? []).map((scope) => (
          <code key={scope}>{scope}</code>
        ))}
      </div>
    </section>
  );
}

function ProtectedRoute({ children, route }: { children: ReactNode; route?: NavRoute }) {
  const visualBreakdownMode = isVisualBreakdownMode();
  const tokenPresent = visualBreakdownMode || hasLocalSession();
  const screenMaskedDemo = isScreenMaskedDemoRoute(route);
  const session = useQuery({
    queryKey: ['current-user'],
    queryFn: fetchCurrentUser,
    enabled: appConfig.authEnabled && tokenPresent && !screenMaskedDemo && !visualBreakdownMode,
    retry: false,
    staleTime: 60_000,
  });

  if (!tokenPresent && !screenMaskedDemo) return <Navigate to="/login" replace />;
  if (session.isLoading) return <RouteLoading />;
  if (session.isError) return <Navigate to="/login" replace />;

  const currentUser = screenMaskedDemo ? screenDemoUser : visualBreakdownMode || !appConfig.authEnabled ? localBypassUser : session.data;
  if (route && !hasRouteAccess(route, currentUser)) {
    return (
      <AppShell currentUser={currentUser}>
        <AccessDenied route={route} />
      </AppShell>
    );
  }

  return (
    <AppShell currentUser={currentUser}>
      <Suspense fallback={<RouteLoading />}>{children}</Suspense>
    </AppShell>
  );
}

const darkTheme = {
  algorithm: theme.darkAlgorithm,
  token: {
    colorPrimary: '#18a8ff',
    colorBgBase: '#03111c',
    colorBgContainer: 'rgba(6, 28, 43, 0.92)',
    colorBorder: 'rgba(56, 151, 201, 0.28)',
    borderRadius: 6,
    fontFamily:
      '"Microsoft YaHei", "PingFang SC", "Noto Sans CJK SC", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
  },
};

export default function App() {
  useDocumentWindowFrameCssVars();

  return (
    <ConfigProvider locale={zhCN} theme={darkTheme}>
      <QueryClientProvider client={queryClient}>
        <Router basename={routerBasename}>
          <Routes>
            <Route path="/" element={<Navigate to="/dashboard" replace />} />
            <Route path="/login" element={<Suspense fallback={<RouteLoading />}><LoginPage /></Suspense>} />
            <Route path="/oidc/callback" element={<Suspense fallback={<RouteLoading />}><OidcCallbackPage /></Suspense>} />
            {navRoutes.map((route) => (
              <Route
                key={route.id}
                path={route.path}
                element={
                  <ProtectedRoute route={route}>
                    {route.id === 'screen' ? (
                      <SituationalScreen route={route} maskedDemo={isScreenMaskedDemoRoute(route)} />
                    ) : route.id === 'dashboard' ? (
                      <DashboardOperationsPage route={route} />
                    ) : route.id === 'topics' ? (
                      <TopicWorkbenchPage route={route} />
                    ) : route.id === 'probes' ? (
                      <ProbesManagementPage route={route} />
                    ) : route.id === 'data-quality' ? (
                      <DataQualityPage route={route} />
                    ) : route.id === 'alerts' ? (
                      <AlertTriagePage route={route} />
                    ) : route.id === 'assets' ? (
                      <AssetInventoryPage route={route} />
                    ) : route.id === 'graph' ? (
                      <GraphEntityPage route={route} />
                    ) : route.id === 'fusion' ? (
                      <FusionWorkbenchPage route={route} />
                    ) : route.id === 'baselines' ? (
                      <BaselineWorkbenchPage route={route} />
                    ) : route.id === 'campaigns' ? (
                      <CampaignWorkbenchPage route={route} />
                    ) : route.id === 'attack-chains' ? (
                      <AttackChainAnalysisPage route={route} />
                    ) : route.id === 'encrypted-traffic' ? (
                      <EncryptedTrafficPage route={route} />
                    ) : route.id === 'forensics' ? (
                      <ForensicsWorkbenchPage route={route} />
                    ) : route.id === 'rules' ? (
                      <RuleManagementPage route={route} />
                    ) : route.id === 'deployments' ? (
                      <DeploymentManagementPage route={route} />
                    ) : route.id === 'models' ? (
                      <ModelManagementPage route={route} />
                    ) : route.id === 'mlops' ? (
                      <MlopsOrchestrationPage route={route} />
                    ) : route.id === 'playbooks' ? (
                      <PlaybookAutomationPage route={route} />
                    ) : route.id === 'whitelist' ? (
                      <WhitelistGovernancePage route={route} />
                    ) : route.id === 'compliance' ? (
                      <ComplianceAuditPage route={route} />
                    ) : route.id === 'audit-log' ? (
                      <AuditLogPage route={route} />
                    ) : route.id === 'notifications' ? (
                      <NotificationConfigPage route={route} />
                    ) : route.id === 'settings' ? (
                      <SettingsGovernancePage route={route} />
                    ) : (
                      <ProductPage route={route} />
                    )}
                  </ProtectedRoute>
                }
              />
            ))}
            <Route path="/topics/tunnel" element={<Navigate to="/topics?topic=tunnel" replace />} />
            <Route path="/topics/exfil" element={<Navigate to="/topics?topic=exfil" replace />} />
            <Route path="/topics/apt" element={<Navigate to="/topics?topic=apt" replace />} />
            {detailRoutes.map((route) => (
              <Route
                key={route.id}
                path={route.path}
                element={
                  <ProtectedRoute route={route}>
                    {route.id === 'alert-detail' ? (
                      <AlertDetailPage route={route} />
                    ) : route.id === 'campaign-detail' ? (
                      <CampaignDetailPage route={route} />
                    ) : (
                      <DetailPage route={route} />
                    )}
                  </ProtectedRoute>
                }
              />
            ))}
            <Route
              path="*"
              element={
                <ProtectedRoute>
                  <NotFoundPage knownRoutes={allRoutes} />
                </ProtectedRoute>
              }
            />
          </Routes>
        </Router>
      </QueryClientProvider>
    </ConfigProvider>
  );
}
