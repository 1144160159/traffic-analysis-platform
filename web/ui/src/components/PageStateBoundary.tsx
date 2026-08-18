import { Alert, Button, Empty, Spin } from 'antd';
import type { ReactNode } from 'react';
import type { PageStateKind } from './pageState';

export type { PageStateKind } from './pageState';

type PageStateBoundaryProps = {
  state: PageStateKind;
  children?: ReactNode;
  title?: string;
  description?: string;
  missingSections?: string[];
  onRetry?: () => void;
  retrying?: boolean;
  labelledBy?: string;
  className?: string;
};

const defaultCopy: Record<Exclude<PageStateKind, 'final'>, { title: string; description: string }> = {
  loading: { title: '正在加载权威数据', description: '正在读取当前页面所需的真实接口与水位。' },
  empty: { title: '当前没有可显示的数据', description: '查询已完成，当前筛选条件下没有业务记录。' },
  partial: { title: '页面数据部分可用', description: '可用内容继续展示；缺失部分不会由示例数据补齐。' },
  unavailable: { title: '权威数据暂不可用', description: '页面不会退回伪数据，请检查服务状态后重试。' },
  conflict: { title: '页面状态已发生冲突', description: '服务端版本已变化，请刷新权威状态后重新提交。' },
};

export function PageStateBoundary({
  state,
  children,
  title,
  description,
  missingSections = [],
  onRetry,
  retrying = false,
  labelledBy,
  className = '',
}: PageStateBoundaryProps) {
  if (state === 'final') {
    return (
      <div className={`taf-page-state is-final ${className}`.trim()} data-page-state="final" aria-busy="false" aria-labelledby={labelledBy}>
        {children}
      </div>
    );
  }

  const copy = defaultCopy[state];
  const resolvedTitle = title || copy.title;
  const missing = missingSections.map((item) => item.trim()).filter(Boolean);
  const resolvedDescription = description || (state === 'partial' && missing.length
    ? `${copy.description} 待补齐：${missing.join('、')}`
    : copy.description);
  const role = state === 'unavailable' || state === 'conflict' ? 'alert' : 'status';
  const action = onRetry && (state === 'unavailable' || state === 'conflict')
    ? <Button size="small" danger={state === 'unavailable'} loading={retrying} onClick={onRetry}>刷新权威状态</Button>
    : undefined;

  return (
    <section
      className={`taf-page-state is-${state} ${className}`.trim()}
      data-page-state={state}
      role={role}
      aria-live={role === 'alert' ? 'assertive' : 'polite'}
      aria-busy={state === 'loading'}
      aria-labelledby={labelledBy}
    >
      {state === 'loading' ? (
        <div className="taf-page-state__loading">
          <Spin size="large" />
          <div><strong>{resolvedTitle}</strong><p>{resolvedDescription}</p></div>
        </div>
      ) : state === 'empty' ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={<span><strong>{resolvedTitle}</strong><small>{resolvedDescription}</small></span>} />
      ) : (
        <Alert
          type={state === 'partial' || state === 'conflict' ? 'warning' : 'error'}
          showIcon
          message={resolvedTitle}
          description={resolvedDescription}
          action={action}
        />
      )}
      {state === 'partial' ? children : null}
    </section>
  );
}
