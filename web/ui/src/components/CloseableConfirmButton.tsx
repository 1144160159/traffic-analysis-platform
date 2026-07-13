import { CloseOutlined } from '@ant-design/icons';
import { Button, Modal } from 'antd';
import type { ButtonProps } from 'antd';
import type { ReactNode } from 'react';
import { useState } from 'react';

type CloseableConfirmButtonProps = {
  title: ReactNode;
  description: ReactNode;
  confirmText: string;
  cancelText?: string;
  danger?: boolean;
  buttonProps?: ButtonProps;
  className?: string;
  children: ReactNode;
  onConfirm?: () => void | Promise<void>;
};

const confirmModalWidth = 'min(420px, calc(var(--taf-window-inner-width, 100dvw) - 48px))';

export function CloseableConfirmButton({
  title,
  description,
  confirmText,
  cancelText = '取消',
  danger = false,
  buttonProps,
  className,
  children,
  onConfirm,
}: CloseableConfirmButtonProps) {
  const [open, setOpen] = useState(false);
  const { onClick, ...restButtonProps } = buttonProps ?? {};

  const close = () => setOpen(false);
  const confirm = async () => {
    await onConfirm?.();
    close();
  };

  return (
    <>
      <Button
        {...restButtonProps}
        onClick={(event) => {
          onClick?.(event);
          if (!event.defaultPrevented) setOpen(true);
        }}
      >
        {children}
      </Button>
      <Modal
        className={`taf-small-confirm-modal${className ? ` ${className}` : ''}`}
        title={title}
        open={open}
        closeIcon={<CloseOutlined title="关闭弹窗" />}
        onCancel={close}
        width={confirmModalWidth}
        footer={[
          <Button key="cancel" onClick={close}>{cancelText}</Button>,
          <Button key="ok" type="primary" danger={danger} onClick={() => void confirm()}>
            {confirmText}
          </Button>,
        ]}
      >
        <p className="taf-small-confirm-modal__description">{description}</p>
      </Modal>
    </>
  );
}
