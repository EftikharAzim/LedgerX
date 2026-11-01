import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type Account, formatMoney } from "../api/endpoints";
import { createBalanceCheck, listBalanceChecks } from "../api/imports";
import { parseStartingBalance } from "../lib/money";
import { nextWriteAttempt, type WriteAttempt } from "../lib/write-attempt";
import { apiError } from "../lib/errors";
import Dialog from "./Dialog";
import Icon from "./Icon";

export default function BalanceCheckDialog({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [actual, setActual] = useState("");
  const [note, setNote] = useState("");
  const attempt = useRef<WriteAttempt | null>(null);
  const minor = parseStartingBalance(actual, account.currency);
  const history = useQuery({
    queryKey: ["balance-checks", account.id],
    queryFn: () => listBalanceChecks(account.id),
  });
  const check = useMutation({
    mutationFn: () => {
      if (minor === null) throw new Error("Enter a valid posted balance.");
      const body = { actual_minor: minor, note: note.trim() };
      attempt.current = nextWriteAttempt(attempt.current, {
        account_id: account.id,
        ...body,
      });
      return createBalanceCheck(account.id, body, attempt.current.key);
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ["balance-checks", account.id] }),
  });
  const result = check.data;
  const changed =
    !!result &&
    !!history.data &&
    result.ledger_revision !== history.data.current_revision;
  return (
    <Dialog
      title="Check your balance"
      description={`${account.name} · ${account.currency}`}
      onClose={onClose}
      busy={check.isPending}
    >
      <p className="input-hint">
        Compare the posted or settled balance shown by your bank or wallet.
        Pending activity may differ. This saves a check; it does not adjust your
        records.
      </p>
      <div className="balance-check-recorded">
        <span>LedgerX recorded balance</span>
        <strong>
          {history.isPending
            ? "Loading…"
            : history.isError
              ? "Unavailable"
              : formatMoney(
                  history.data?.recorded_balance_minor ?? 0,
                  account.currency,
                )}
        </strong>
      </div>
      {history.isError && (
        <p role="alert" className="form-error">
          Couldn't load recent checks.{" "}
          <button className="text-button" onClick={() => history.refetch()}>
            Try again
          </button>
        </p>
      )}
      <form
        className="account-form"
        onChange={() => {
          if (check.isError || check.isSuccess) check.reset();
        }}
        onSubmit={(e) => {
          e.preventDefault();
          if (minor !== null && !check.isPending) check.mutate();
        }}
      >
        <div>
          <label className="label" htmlFor="check-actual">
            Bank or wallet balance · {account.currency}
          </label>
          <input
            id="check-actual"
            className="input"
            data-autofocus
            value={actual}
            onChange={(e) => setActual(e.target.value)}
            inputMode="decimal"
            placeholder="Enter the balance you see"
            disabled={check.isPending}
            required
            aria-invalid={!!actual && minor === null}
          />
          <p className="input-hint">
            Use a minus for money owed. Zero is a valid balance.
          </p>
        </div>
        <details>
          <summary className="balance-check-note">
            Add a note · optional
          </summary>
          <label className="sr-only" htmlFor="check-note">
            Check note
          </label>
          <input
            id="check-note"
            className="input"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            maxLength={200}
            placeholder="e.g. Compared with the bank app"
            disabled={check.isPending}
          />
        </details>
        {check.isError && (
          <p role="alert" className="form-error">
            {apiError(check.error)} You can retry this check safely.
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            onClick={onClose}
            disabled={check.isPending}
          >
            Done
          </button>
          <button
            className="btn btn-primary"
            disabled={minor === null || check.isPending}
          >
            {check.isPending ? "Checking…" : "Check balance"}
          </button>
        </div>
      </form>
      {result && (
        <section
          className={`balance-check-result ${result.difference_minor === 0 ? "matched" : "different"}`}
          aria-labelledby="check-result-title"
        >
          <h3 id="check-result-title">
            <Icon name={result.difference_minor === 0 ? "check" : "info"} />
            {result.difference_minor === 0
              ? "Balances matched at this check"
              : "A difference needs review"}
          </h3>
          <dl className="review-details">
            <div>
              <dt>Bank / wallet</dt>
              <dd>{formatMoney(result.actual_minor, account.currency)}</dd>
            </div>
            <div>
              <dt>Recorded in LedgerX</dt>
              <dd>{formatMoney(result.recorded_minor, account.currency)}</dd>
            </div>
            <div>
              <dt>Difference · actual minus recorded</dt>
              <dd>{formatMoney(result.difference_minor, account.currency)}</dd>
            </div>
          </dl>
          {result.difference_minor !== 0 && (
            <p>
              Your records are{" "}
              {formatMoney(Math.abs(result.difference_minor), account.currency)}{" "}
              {result.difference_minor > 0 ? "lower" : "higher"} than the
              bank/wallet balance. Review missing entries, duplicate records,
              refunds, and the starting balance.
            </p>
          )}
          {changed && (
            <p>
              New activity was recorded after this check. Check again for an
              up-to-date comparison.
            </p>
          )}
          <Link className="text-link" to={`/activity?account_id=${account.id}`}>
            Review account activity
            <Icon name="arrow" width="16" height="16" />
          </Link>
        </section>
      )}
      {!!history.data?.checks.length && (
        <details className="recent-balance-checks">
          <summary>Recent balance checks</summary>
          <ul>
            {history.data.checks.map((c) => (
              <li key={c.id}>
                <time dateTime={c.created_at}>
                  {new Date(c.created_at).toLocaleString()}
                </time>
                <span>
                  {c.difference_minor === 0
                    ? "Matched"
                    : `Difference ${formatMoney(c.difference_minor, account.currency)}`}
                  {c.ledger_revision !== history.data?.current_revision
                    ? " · New activity since"
                    : ""}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </Dialog>
  );
}
