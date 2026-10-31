import Icon from "./Icon";

export default function SuccessNotice({
  message,
  onDismiss,
}: {
  message: string;
  onDismiss: () => void;
}) {
  if (!message) return null;
  return (
    <div className="success-notice quiet-success">
      <span role="status">
        <Icon name="check" width="17" height="17" />
        {message}
      </span>
      <button
        type="button"
        className="icon-button"
        aria-label="Dismiss confirmation"
        onClick={onDismiss}
      >
        <Icon name="close" width="16" height="16" />
      </button>
    </div>
  );
}
