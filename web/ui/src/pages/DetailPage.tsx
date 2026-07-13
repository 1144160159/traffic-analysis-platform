import { ArrowLeftOutlined } from '@ant-design/icons';
import { Button, Tooltip } from 'antd';
import { useNavigate, useParams } from 'react-router-dom';
import { ProductPage } from '@/pages/ProductPage';
import { findRouteById, type NavRoute } from '@/routes/routeManifest';

export function DetailPage({ route }: { route: NavRoute }) {
  const params = useParams();
  const navigate = useNavigate();
  const id = params.alertId ?? params.campaignId ?? 'DETAIL';
  const parentRoute = route.activeNavId ? findRouteById(route.activeNavId) : undefined;
  const returnLabel = parentRoute ? `返回${parentRoute.title}` : '返回上一页';
  const returnToParent = () => {
    if (parentRoute) {
      navigate(parentRoute.path);
      return;
    }
    navigate(-1);
  };

  return (
    <div className="taf-detail-context">
      <header className="taf-detail-titlebar">
        <div className="taf-detail-titlebar__page-title">
          <h1 title={route.page.title}>{route.page.title}</h1>
          <span title={id}>对象 ID: {id}</span>
        </div>
        <Tooltip title={returnLabel}>
          <Button size="small" icon={<ArrowLeftOutlined />} aria-label={returnLabel} onClick={returnToParent} />
        </Tooltip>
      </header>
      <ProductPage route={route} hideHero />
    </div>
  );
}
