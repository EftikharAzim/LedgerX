import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { type Account, createTransfer, formatMoney } from "../api/endpoints";
import { localDateTime, parseAmount, toOccurredAt } from "../lib/money";
import { nextWriteAttempt, type WriteAttempt } from "../lib/write-attempt";
import { apiError } from "../lib/errors";
import Dialog from "./Dialog";
import AmountInput from "./AmountInput";
import Icon from "./Icon";

type TransferBody = Parameters<typeof createTransfer>[0];

export default function TransferDialog({
  account,
  accounts,
  onClose,
  onSaved,
}: {
  account: Account;
  accounts: Account[];
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const targets = accounts.filter(
    (a) =>
      a.id !== account.id &&
      a.kind === "normal" &&
      a.active &&
      a.currency === account.currency,
  );
  const [target, setTarget] = useState("");
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [date, setDate] = useState(() => localDateTime());
  const [review, setReview] = useState<TransferBody | null>(null);
  const attempt = useRef<WriteAttempt | null>(null);
  const minor = parseAmount(amount, account.currency);
  const occurredAt = toOccurredAt(date);
  const destination = targets.find((a) => a.id === Number(target));
  const save = useMutation({
    mutationFn: () => {
      if (!review) throw new Error("Review the transfer before confirming.");
      attempt.current = nextWriteAttempt(attempt.current, review);
      return createTransfer(review, attempt.current.key);
    },
    onSuccess: () =>
      onSaved(
        `${formatMoney(review!.amount_minor, account.currency)} transferred to ${destination!.name}.`,
      ),
  });
  return (
    <Dialog
      title={review ? "One last look." : "Move money, simply."}
      description={
        review
          ? "Check the details before confirming your transfer."
          : "Between your accounts. In the same currency."
      }
      onClose={onClose}
      busy={save.isPending}
    >
      {!targets.length ? (
        <div className="transfer-unavailable">
          <span className="movement-icon income">
            <Icon name="transfer" width="25" height="25" />
          </span>
          <h3>Make room for a second account.</h3>
          <p>
            You'll need another active {account.currency} account to transfer
            money. Transfers stay in one currency.
          </p>
          <Link to="/accounts" onClick={onClose} className="btn btn-primary">
            Go to accounts
            <Icon name="arrow" width="17" height="17" />
          </Link>
        </div>
      ) : review ? (
        <div className="account-form">
          <div className="transfer-review-amount">
            <span>YOU'RE MOVING</span>
            <strong>
              {formatMoney(review.amount_minor, account.currency)}
            </strong>
            <span>
              {account.currency} → {account.currency}
            </span>
          </div>
          <dl className="review-details">
            <div>
              <dt>From</dt>
              <dd>{account.name}</dd>
            </div>
            <div>
              <dt>To</dt>
              <dd>{destination?.name}</dd>
            </div>
            <div>
              <dt>Date & time</dt>
              <dd>{new Date(review.occurred_at).toLocaleString()}</dd>
            </div>
            {review.note && (
              <div>
                <dt>Note</dt>
                <dd>{review.note}</dd>
              </div>
            )}
          </dl>
          <p className="transfer-explanation">
            <Icon name="info" width="16" height="16" />
            This moves money between your accounts. Your combined{" "}
            {account.currency} balance stays the same.
          </p>
          {save.isError && (
            <p role="alert" className="form-error">
              <Icon name="info" />
              {apiError(save.error)} Retry with the same details to avoid a
              duplicate.
            </p>
          )}
          <div className="dialog-actions">
            <button
              type="button"
              className="btn"
              onClick={() => {
                setReview(null);
                save.reset();
              }}
              disabled={save.isPending}
            >
              Back to edit
            </button>
            <button
              className="btn btn-primary"
              onClick={() => save.mutate()}
              disabled={save.isPending}
            >
              {save.isPending ? "Transferring…" : "Confirm transfer"}
              <Icon name="arrow" width="17" height="17" />
            </button>
          </div>
        </div>
      ) : (
        <form
          className="account-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (minor && occurredAt && destination)
              setReview({
                from_account_id: account.id,
                to_account_id: destination.id,
                amount_minor: minor,
                currency: account.currency,
                occurred_at: occurredAt,
                note: note.trim(),
              });
          }}
        >
          <div className="transfer-route">
            <div>
              <span>FROM</span>
              <strong>{account.name}</strong>
              <small>{account.currency}</small>
            </div>
            <span className="transfer-route-arrow">
              <Icon name="arrow" />
            </span>
            <div>
              <label htmlFor="transfer-to">TO</label>
              <select
                id="transfer-to"
                data-autofocus
                required
                value={target}
                onChange={(e) => setTarget(e.target.value)}
              >
                <option value="">Choose an account</option>
                {targets.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
              <small>{account.currency}</small>
            </div>
          </div>
          <AmountInput
            value={amount}
            onChange={setAmount}
            currency={account.currency}
          />
          <div>
            <label className="label" htmlFor="transfer-date">
              Date & time
            </label>
            <input
              id="transfer-date"
              type="datetime-local"
              className="input"
              value={date}
              required
              onChange={(e) => setDate(e.target.value)}
            />
            <p className="input-hint">In your local time zone.</p>
          </div>
          <div>
            <label className="label" htmlFor="transfer-note">
              Note <span className="optional-label">optional</span>
            </label>
            <input
              id="transfer-note"
              className="input"
              placeholder="e.g. Putting a little aside"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <div className="dialog-actions">
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            <button
              className="btn btn-primary"
              disabled={!minor || !occurredAt || !destination}
            >
              Review transfer
              <Icon name="arrow" width="17" height="17" />
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
