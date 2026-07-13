import { EyeOutlined, FileDoneOutlined, ReloadOutlined, SearchOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, DatePicker, Empty, Input, Select, Space, Table, Tabs, Timeline, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { Link } from 'react-router-dom';
import { MetricTile } from '@/components/MetricTile';
import { RingChart, TrendChart } from '@/components/charts';
import { StatusTag } from '@/components/StatusTag';
import { WorkPanel } from '@/components/WorkPanel';
import type { NavRoute } from '@/routes/routeManifest';
import { fetchPageSnapshot } from '@/services/api';
import type { SnapshotRow } from '@/services/mockData';

export function ProductPage({ route, hideHero = false }: { route: NavRoute; hideHero?: boolean }) {
  const { data, error, isError, isLoading, refetch } = useQuery({
    queryKey: ['page-snapshot', route.id],
    queryFn: () => fetchPageSnapshot(route.id),
  });

  const columns: ColumnsType<SnapshotRow> = route.page.tableColumns.map((column) => ({
    title: column,
    dataIndex: column,
    key: column,
    ellipsis: true,
    render: (value) => (isStatusColumn(column) ? <StatusTag value={value} /> : value),
  }));

  return (
    <div className={`taf-page taf-page-${route.page.variant}`}>
      {!hideHero && <PageHero route={route} onRefresh={() => void refetch()} />}

      {isError && (
        <Alert
          type="error"
          showIcon
          message="真实 API 数据加载失败"
          description={error instanceof Error ? error.message : '请检查 APISIX 路由、后端服务、鉴权或网络连通性。'}
          action={
            <Button size="small" danger onClick={() => void refetch()}>
              重试
            </Button>
          }
        />
      )}

      <div className="taf-kpi-grid">
        {(data?.metrics ?? []).map((metric) => (
          <MetricTile key={metric.label} metric={metric} />
        ))}
      </div>

      <WorkPanel
        title="筛选检索"
        extra={
          <Tooltip title="刷新当前工作台数据">
            <Button icon={<ReloadOutlined />} size="small" onClick={() => void refetch()} />
          </Tooltip>
        }
      >
        <div className="taf-filterbar">
          <DatePicker.RangePicker size="small" showTime />
          <Select
            size="small"
            value="近24小时"
            options={[{ value: '近24小时' }, { value: '近7天' }, { value: '自定义视图' }]}
          />
          <Input size="small" prefix={<SearchOutlined />} placeholder="资产、IP、告警、证据 ID" />
          <Button size="small" type="primary">查询</Button>
          <Button size="small">保存视图</Button>
        </div>
      </WorkPanel>

      <Tabs
        className="taf-tabs"
        defaultActiveKey={route.page.tabs[0]}
        items={route.page.tabs.map((tab) => ({ key: tab, label: tab }))}
      />

      <div className="taf-workgrid">
        <WorkPanel title={route.page.tableTitle} className="taf-workgrid__table">
          <Table
            rowKey={(record) => String(record[route.page.tableColumns[0]])}
            size="small"
            loading={isLoading}
            columns={columns}
            dataSource={data?.rows ?? []}
            pagination={{ pageSize: 8, size: 'small' }}
          />
        </WorkPanel>

        <WorkPanel title="趋势与分布" className="taf-workgrid__chart">
          <TrendChart title={`${route.title} 近 24 小时`} />
        </WorkPanel>

        <WorkPanel title="状态时间线" className="taf-workgrid__timeline">
          {data ? (
            <Timeline
              items={data.timeline.map((item) => ({
                color: item.status === 'risk' ? 'red' : item.status === 'warn' ? 'gold' : 'blue',
                children: (
                  <span>
                    <strong>{item.title}</strong>
                    <small>{item.description}</small>
                  </span>
                ),
              }))}
            />
          ) : (
            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} />
          )}
        </WorkPanel>

        <aside className="taf-right-rail">
          <WorkPanel title={route.page.rightRailTitle}>
            <RingChart value={route.id === 'alerts' ? 92 : 68} />
            <div className="taf-rail-actions">
              {route.page.actions.map((action) => (
                <Button key={action} size="small" type={action === route.page.actions[0] ? 'primary' : 'default'}>
                  {action}
                </Button>
              ))}
            </div>
          </WorkPanel>
          <WorkPanel title="证据与审计">
            <div className="taf-evidence-list">
              {(data?.evidence ?? []).map((item) => (
                <span key={item.label} className={`taf-evidence-item is-${item.status}`}>
                  <FileDoneOutlined />
                  <strong>{item.label}</strong>
                  <em>{item.value}</em>
                </span>
              ))}
            </div>
            <Space className="taf-page-links">
              <Link to="/audit-log">
                <EyeOutlined /> 查看审计
              </Link>
              <Link to="/compliance">生成验收证据</Link>
            </Space>
          </WorkPanel>
        </aside>
      </div>
    </div>
  );
}

function PageHero({ route, onRefresh }: { route: NavRoute; onRefresh: () => void }) {
  return (
    <header className={`taf-page-hero is-${route.domain} is-${route.page.variant} bg-${route.page.background}`}>
      <span className="taf-page-hero__mesh" aria-hidden="true">
        {Array.from({ length: 9 }, (_, index) => <i key={index} />)}
      </span>
      <div>
        <span className="taf-page-hero__domain">{route.title}</span>
        <h1>{route.page.title}</h1>
        <p>{route.page.subtitle}</p>
      </div>
      <Space>
        <Button icon={<ReloadOutlined />} onClick={onRefresh}>刷新</Button>
        <Button type="primary">创建闭环任务</Button>
      </Space>
    </header>
  );
}

const isStatusColumn = (column: string) =>
  column.includes('状态') || column.includes('风险') || column.includes('级别') || column.includes('结果');
