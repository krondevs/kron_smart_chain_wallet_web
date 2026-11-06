import type { ReactNode } from 'react';

export function Modal({
  title,
  subtitle,
  children,
  onClose,
  actions,
}: {
  title: string;
  subtitle?: string;
  children?: ReactNode;
  onClose?: () => void;
  actions?: ReactNode;
}) {
  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={(event) => {
      if (event.target === event.currentTarget) onClose?.();
    }}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={title}>
        <div className="spread mb">
          <div>
            <h3>{title}</h3>
            {subtitle ? <p style={{ margin: '4px 0 0' }}>{subtitle}</p> : null}
          </div>
          {onClose ? (
            <button className="btn ghost small" onClick={onClose} aria-label="Cerrar">
              ✕
            </button>
          ) : null}
        </div>
        {children}
        {actions ? <div className="btn-group mt">{actions}</div> : null}
      </div>
    </div>
  );
}
