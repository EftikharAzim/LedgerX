import type { ReactNode } from "react";
import { useModalDialog } from "../lib/use-modal-dialog";
import Icon from "./Icon";

export default function Dialog({
  title,
  description,
  children,
  onClose,
  busy = false,
  drawer = false,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  busy?: boolean;
  drawer?: boolean;
}) {
  const ref = useModalDialog();
  return (
    <dialog
      ref={ref}
      className={`account-dialog money-dialog ${drawer ? "detail-drawer" : ""}`}
      aria-labelledby="money-dialog-title"
      aria-describedby={description ? "money-dialog-description" : undefined}
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
    >
      <header className="money-dialog-header">
        <div>
          <p className="eyebrow">THE DETAILS MATTER</p>
          <h2 id="money-dialog-title">{title}</h2>
        </div>
        <button
          className="icon-button"
          aria-label="Close dialog"
          disabled={busy}
          onClick={onClose}
        >
          <Icon name="close" />
        </button>
      </header>
      {description && (
        <p id="money-dialog-description" className="muted">
          {description}
        </p>
      )}
      {children}
    </dialog>
  );
}
