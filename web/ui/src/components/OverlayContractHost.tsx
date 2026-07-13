import {
  AuditOutlined,
  CheckCircleOutlined,
  CloseOutlined,
  ControlOutlined,
  FileProtectOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { Button, Descriptions, Drawer, Dropdown, Modal, Space, Tag } from 'antd';
import type { MenuProps } from 'antd';
import { useMemo, useState } from 'react';

export type OverlayContractKind = 'Modal' | 'Drawer' | 'Dropdown/Menu' | 'Popconfirm';

export type OverlayContract = {
  id: string;
  title: string;
  kind: OverlayContractKind;
  description: string;
  actionLabel?: string;
  impact?: string;
  audit?: string;
  danger?: boolean;
  fields?: Array<[string, string]>;
};

type OverlayContractHostProps = {
  overlays: OverlayContract[];
  compact?: boolean;
};

const overlayModalWidth = 'min(620px, calc(var(--taf-window-inner-width, 100dvw) - 64px))';
const overlayDrawerWidth = 'min(520px, calc(var(--taf-window-inner-width, 100dvw) - 40px))';

export function OverlayContractHost({ overlays, compact = false }: OverlayContractHostProps) {
  const [activeId, setActiveId] = useState<string>();
  const activeOverlay = useMemo(() => overlays.find((overlay) => overlay.id === activeId), [activeId, overlays]);
  const close = () => setActiveId(undefined);
  const modalOpen = activeOverlay?.kind === 'Modal' || activeOverlay?.kind === 'Popconfirm';

  return (
    <div className={`taf-overlay-host${compact ? ' is-compact' : ''}`}>
      {overlays.map((overlay) => (
        <OverlayTrigger key={overlay.id} overlay={overlay} onOpen={() => setActiveId(overlay.id)} />
      ))}
      <Modal
        className="taf-overlay-modal"
        title={activeOverlay?.title}
        open={modalOpen}
        closeIcon={<CloseOutlined title="关闭弹窗" />}
        onCancel={close}
        width={overlayModalWidth}
        footer={[
          <Button key="cancel" onClick={close}>取消</Button>,
          <Button key="ok" type="primary" danger={activeOverlay?.danger} onClick={close}>
            {activeOverlay?.kind === 'Popconfirm' ? '确认执行' : '确认写入审计'}
          </Button>,
        ]}
      >
        {activeOverlay && <OverlayBody overlay={activeOverlay} />}
      </Modal>
      <Drawer
        className="taf-overlay-drawer"
        title={activeOverlay?.title}
        open={activeOverlay?.kind === 'Drawer'}
        closeIcon={<CloseOutlined title="关闭弹窗" />}
        onClose={close}
        width={overlayDrawerWidth}
        extra={<Button size="small" type="primary" onClick={close}>完成</Button>}
      >
        {activeOverlay && <OverlayBody overlay={activeOverlay} />}
      </Drawer>
    </div>
  );
}

function OverlayTrigger({ overlay, onOpen }: { overlay: OverlayContract; onOpen: () => void }) {
  const label = overlay.actionLabel ?? overlay.title;
  const labelNode = <span title={label}>{label}</span>;

  if (overlay.kind === 'Dropdown/Menu') {
    const menu: MenuProps = {
      items: [
        { key: 'open', label: overlay.title, icon: <ControlOutlined /> },
        { key: 'audit', label: overlay.audit ?? '写入审计 trace', icon: <AuditOutlined /> },
        { key: 'impact', label: overlay.impact ?? '查看影响范围', icon: <SafetyCertificateOutlined /> },
      ],
      onClick: ({ key }) => {
        if (key === 'open') onOpen();
      },
    };
    return (
      <Dropdown menu={menu} placement="bottomRight">
        <Button size="small" title={label} icon={<ControlOutlined />}>{labelNode}</Button>
      </Dropdown>
    );
  }

  if (overlay.kind === 'Popconfirm') {
    return (
      <Button size="small" title={label} danger={overlay.danger} icon={<SafetyCertificateOutlined />} onClick={onOpen}>
        {labelNode}
      </Button>
    );
  }

  return (
    <Button size="small" title={label} danger={overlay.danger} icon={overlay.kind === 'Drawer' ? <FileProtectOutlined /> : <ControlOutlined />} onClick={onOpen}>
      {labelNode}
    </Button>
  );
}

function OverlayBody({ overlay }: { overlay: OverlayContract }) {
  const fields = overlay.fields?.length
    ? overlay.fields
    : [
        ['契约 ID', overlay.id],
        ['影响范围', overlay.impact ?? '当前租户、当前筛选条件与所选对象'],
        ['审计 trace', overlay.audit ?? '记录操作者、对象、时间、来源页面与结果'],
      ];

  return (
    <div className="taf-overlay-body">
      <Space size={6} wrap>
        <Tag color="blue">{overlay.kind}</Tag>
        <Tag color={overlay.danger ? 'red' : 'green'}>{overlay.danger ? '高影响操作' : '可回退操作'}</Tag>
        <Tag color="purple">RBAC</Tag>
      </Space>
      <p>{overlay.description}</p>
      <Descriptions size="small" column={1} bordered>
        {fields.map(([label, value]) => (
          <Descriptions.Item key={label} label={label}>{value}</Descriptions.Item>
        ))}
      </Descriptions>
      <div className="taf-overlay-body__guard">
        <CheckCircleOutlined />
        <span>执行前校验权限、租户边界和对象状态；提交后同步审计日志与业务事件。</span>
      </div>
    </div>
  );
}
