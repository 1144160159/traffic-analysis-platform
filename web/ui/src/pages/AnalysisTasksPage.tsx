import { Table, Tag } from 'antd';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { analysisQueryKeys, fetchAnalysisTasks, type AnalysisTaskView } from '@/services/analysisApi';
import { PageStateBoundary } from '@/components/PageStateBoundary';
import { resolvePageState } from '@/components/pageState';
import type { NavRoute } from '@/routes/routeManifest';

/** 任务管理:七列列表;行名链接到详情五 Tab(/analysis/task-definitions/:id)。 */
export function AnalysisTasksPage({ route }: { route: NavRoute }) {
  const tasksQuery = useQuery({
    queryKey: analysisQueryKeys.tasks,
    queryFn: fetchAnalysisTasks,
    retry: false,
  });

  return (
    <div className="taf-page">
      <h1>{route.title}</h1>
      <PageStateBoundary state={resolvePageState({ isLoading: tasksQuery.isLoading, data: tasksQuery.data, error: tasksQuery.error })}>
        <Table<AnalysisTaskView>
          rowKey="id"
          dataSource={tasksQuery.data ?? []}
          pagination={{ pageSize: 20 }}
          columns={[
            {
              title: '定义/目标',
              dataIndex: 'name',
              render: (name: string, r: AnalysisTaskView) => (
                <Link to={`/analysis/task-definitions/${r.id}`}>{name}</Link>
              ),
            },
            { title: '当前方案', dataIndex: 'active_plan_revision', render: (v: number) => (v > 0 ? `plan@${v}` : '未激活') },
            { title: '调度', dataIndex: 'active_schedule_revision', render: (v: number) => (v > 0 ? `schedule@${v}` : '—') },
            { title: '状态', dataIndex: 'state', render: (s: string) => <Tag>{s}</Tag> },
            { title: 'owner', dataIndex: 'owner' },
            { title: 'revision', dataIndex: 'revision' },
            { title: '操作', key: 'ops', render: (_: unknown, r: AnalysisTaskView) => (
              <Link to={`/analysis/task-definitions/${r.id}`}>详情</Link>
            ) },
          ]}
        />
      </PageStateBoundary>
    </div>
  );
}
