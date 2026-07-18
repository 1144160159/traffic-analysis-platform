import {
  ApartmentOutlined,
  AppstoreOutlined,
  DesktopOutlined,
  HddOutlined,
  QuestionCircleOutlined,
} from '@ant-design/icons';

const normalizedKind = (kind?: string) => String(kind ?? '').trim().toLowerCase();

export function AssetTypeIcon({ kind }: { kind?: string }) {
  const value = normalizedKind(kind);
  if (value.includes('server') || value.includes('服务器')) return <HddOutlined />;
  if (value.includes('network') || value.includes('switch') || value.includes('router') || value.includes('网络')) return <ApartmentOutlined />;
  if (value.includes('business') || value.includes('system') || value.includes('业务')) return <AppstoreOutlined />;
  if (value.includes('endpoint') || value.includes('terminal') || value.includes('终端')) return <DesktopOutlined />;
  return <QuestionCircleOutlined />;
}
