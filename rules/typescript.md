# TypeScript / React 规范

基于 Airbnb React Style Guide、React Query 最佳实践。

## 1. 类型

```typescript
// 禁止 any, 必须显式类型
interface AlertQuery {
  tenantId: string;
  severity?: 'critical' | 'high' | 'medium' | 'low';
  page: number;
}

// API 响应泛型
interface ApiResponse<T> { code: number; data: T; message: string; }
```

## 2. 数据获取

```typescript
// React Query 统一管理
const { data, error, isLoading } = useQuery({
  queryKey: ['alerts', params],
  queryFn: () => alertApi.list(params),
  staleTime: 30_000,
});

// 禁止: 组件内直接 fetch, useEffect + fetch
```

## 3. 组件

```tsx
// 函数组件 + hooks, 优先 Ant Design
const AlertCard: React.FC<Props> = ({ alert }) => (
  <Card><Tag color={severityMap[alert.severity]}>{alert.title}</Tag></Card>
);
```

## 4. 状态管理

```typescript
// Zustand 全局, React Query 服务端, useState 局部
// 禁止: props drilling > 3 层, 所有状态放全局
```

## 5. 测试

```typescript
// Vitest + React Testing Library
test('renders alert list', async () => {
  render(<AlertList query={mockQuery} />);
  await waitFor(() => expect(screen.getByText('Critical Alert')).toBeInTheDocument());
});
```
