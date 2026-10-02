import { useMemo } from 'react';
import { createPortal } from 'react-dom';

export interface ModalProps {
  open: boolean;
  title?: string;
  message: string;
  detail?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm?: () => void;
  onCancel?: () => void;
  variant?: 'info' | 'danger' | 'warn';
  confirmLoading?: boolean;
  children?: React.ReactNode;
}

const CUTE_VARIANTS = ['cute-fluffy', 'cute-petal', 'cute-spark'];

export default function Modal({ open, title, message, detail, confirmLabel, cancelLabel, onConfirm, onCancel, variant = 'info', children, confirmLoading }: ModalProps) {
  const cuteClass = useMemo(() => CUTE_VARIANTS[Math.floor(Math.random() * CUTE_VARIANTS.length)], [open]);

  if (!open) return null;

  const icon = variant === 'danger' ? '⚠️' : variant === 'warn' ? '🔔' : '🍃';
  const confirmClass = variant === 'danger' ? 'btn-danger' : variant === 'warn' ? 'btn-warning' : 'btn-primary';

  const modal = (
    <div className="modal-backdrop" onClick={onCancel}>
      <div className={`modal-content ${cuteClass}`} onClick={e => e.stopPropagation()}>
        <div className="modal-header">
          <span className="modal-icon">{icon}</span>
          {title && <h3 className="modal-title">{title}</h3>}
        </div>
        <div className="modal-body">
          <p className="modal-message">{message}</p>
          {detail && <p className="modal-detail">{detail}</p>}
          {children}
        </div>
        <div className="modal-actions">
          {onCancel && cancelLabel !== undefined && (
            <button className="btn btn-outline" onClick={onCancel}>{cancelLabel || '取消'}</button>
          )}
          {onConfirm && (
            <button className={`btn ${confirmClass}`} onClick={onConfirm} disabled={confirmLoading}>
              {confirmLoading ? <span className="btn-spinner" /> : null}
              {confirmLoading ? '处理中...' : (confirmLabel || '确定')}
            </button>
          )}
          {!onConfirm && !onCancel && (
            <button className="btn btn-outline" onClick={onCancel || (() => {})}>关闭</button>
          )}
        </div>
      </div>
    </div>
  );

  return createPortal(modal, document.body);
}
