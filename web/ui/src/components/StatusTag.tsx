import { Tag } from 'antd';

export function StatusTag({ value }: { value: unknown }) {
  const text = String(value);
  const color = text.includes('高') || text.includes('失败') || text.includes('未处理') ? 'red' : text.includes('中') || text.includes('警') || text.includes('处理中') ? 'gold' : text.includes('低') ? 'blue' : 'green';
  return <Tag color={color}>{text}</Tag>;
}
