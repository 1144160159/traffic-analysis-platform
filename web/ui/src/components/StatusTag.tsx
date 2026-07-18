import { Tag } from 'antd';

export function StatusTag({ value }: { value: unknown }) {
  const text = String(value);
  const color = /高|失败|未处理|离线|错误|异常|阻断|危险|^FP$|拒绝|驳回/.test(text)
    ? 'red'
    : /中|警|处理中|待审批|待审核|待确认|未知/.test(text)
      ? 'gold'
      : /低|^TP$/.test(text)
        ? 'blue'
        : 'green';
  return <Tag color={color}>{text}</Tag>;
}
