import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  listAccounts,
  listCategories,
  formatMoney,
  type Account,
  type Category,
} from "../api/endpoints";
import {
  getCorrectionContext,
  previewCorrection,
  saveCorrection,
  getCorrectionResult,
  type CorrectionContext,
  type CorrectionPlan,
  type CorrectionDraft,
} from "../api/workspace";
import {
  correctionDraft,
  correctionKind,
  sameCorrection,
  type EditKind,
} from "../lib/corrections";
import { minorInput } from "../lib/imports";
import {
  localDateTime,
  parseAmount,
  parseStartingBalance,
  currencyDigits,
} from "../lib/money";
import { apiError } from "../lib/errors";
import { nextWriteAttempt, type WriteAttempt } from "../lib/write-attempt";
import Dialog from "./Dialog";
import Icon from "./Icon";

export default function CorrectionDialog({
  id,
  onClose,
  onSaved,
}: {
  id: number;
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const context = useQuery({
    queryKey: ["edit-context", id],
    queryFn: () => getCorrectionContext(id),
    retry: false,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  if (context.isPending || accounts.isPending || categories.isPending)
    return (
      <Dialog title="Edit transaction" onClose={onClose}>
        <p role="status">Loading the record…</p>
      </Dialog>
    );
  if (context.isError || accounts.isError || categories.isError)
    return (
      <Dialog title="Edit transaction" onClose={onClose}>
        <p className="form-error" role="alert">
          {apiError(context.error || accounts.error || categories.error)}
        </p>
        <div className="dialog-actions">
          <button
            className="btn"
            onClick={() => {
              void context.refetch();
              void accounts.refetch();
              void categories.refetch();
            }}
          >
            Try again
          </button>
          <Link className="btn" to={`/activity?transaction_id=${id}`}>
            Review activity
          </Link>
        </div>
      </Dialog>
    );
  return (
    <CorrectionForm
      key={context.data.source_token}
      context={context.data}
      accounts={accounts.data ?? []}
      categories={categories.data ?? []}
      onClose={onClose}
      onSaved={onSaved}
      onReload={() => context.refetch()}
    />
  );
}

function CorrectionForm({
  context,
  accounts,
  categories,
  onClose,
  onSaved,
  onReload,
}: {
  context: CorrectionContext;
  accounts: Account[];
  categories: Category[];
  onClose: () => void;
  onSaved: (message: string) => void;
  onReload: () => unknown;
}) {
  const original = context.draft;
  const [kind, setKind] = useState<EditKind>(() => correctionKind(original));
  const [amount, setAmount] = useState(() =>
    minorInput(
      original.entry_type === "opening_balance"
        ? original.amount_minor
        : Math.abs(original.amount_minor),
      original.currency,
    ),
  );
  const [accountID, setAccountID] = useState(original.account_id);
  const [toAccountID, setToAccountID] = useState(original.to_account_id || 0);
  const [categoryID, setCategoryID] = useState(original.category_id);
  const [note, setNote] = useState(original.note);
  const [dateOnly, setDateOnly] = useState(original.date_only);
  const [date, setDate] = useState(original.occurred_at.slice(0, 10));
  const [localTime, setLocalTime] = useState(() =>
    localDateTime(new Date(original.occurred_at)),
  );
  const [timeEdited, setTimeEdited] = useState(false);
  const [plan, setPlan] = useState<CorrectionPlan | null>(null);
  const [reviewed, setReviewed] = useState(false);
  const attempt = useRef<WriteAttempt | null>(null);
  const amountRef = useRef<HTMLInputElement>(null),
    reviewRef = useRef<HTMLHeadingElement>(null);
  const draft = correctionDraft(original, {
    kind,
    amount,
    accountID,
    toAccountID,
    categoryID,
    note,
    dateOnly,
    date,
    localTime,
    timeEdited,
  });
  const amountMinor =
    kind === "opening_balance"
      ? parseStartingBalance(amount, original.currency)
      : parseAmount(amount, original.currency);
  const preview = useMutation({
    mutationFn: () =>
      previewCorrection(context.transaction_id, {
        draft: draft!,
        source_token: context.source_token,
      }),
    onSuccess: (value) => {
      setPlan(value);
      setReviewed(false);
    },
  });
  const save = useMutation({
    mutationFn: () => {
      if (!plan) throw new Error("Review the correction first.");
      const body = {
        draft: plan.new,
        source_token: plan.source_token,
        preview_token: plan.preview_token,
      };
      attempt.current = nextWriteAttempt(attempt.current, {
        id: context.transaction_id,
        ...body,
      });
      return saveCorrection(context.transaction_id, body, attempt.current.key);
    },
    onSuccess: () =>
      onSaved(
        "Transaction updated. Your original record and correction are linked in history.",
      ),
    onError: async () => {
      if (attempt.current) {
        try {
          const result = await getCorrectionResult(
            context.transaction_id,
            attempt.current.key,
          );
          if (result.original_id === context.transaction_id)
            onSaved(
              "Transaction updated. The saved correction was recovered safely.",
            );
        } catch {
          /* Keep the same review/key available for a safe retry. */
        }
      }
    },
  });
  useEffect(() => {
    if (plan) reviewRef.current?.focus();
    else amountRef.current?.focus();
  }, [plan]);
  const busy = preview.isPending || save.isPending;
  const normal = accounts.filter(
    (a) => a.kind === "normal" && a.currency === original.currency,
  );
  const accountName = (id: number) =>
    accounts.find((a) => a.id === id)?.name || `Account #${id}`;
  const categoryName = (id: number) =>
    categories.find((c) => c.id === id)?.name || "Uncategorized";
  const renderDate = (d: CorrectionDraft) =>
    d.date_only
      ? d.occurred_at.slice(0, 10) + " · calendar date"
      : new Date(d.occurred_at).toLocaleString();
  const differences = plan
    ? [
        {
          label: "Type",
          old: correctionKind(plan.old),
          new: correctionKind(plan.new),
        },
        {
          label: "Amount",
          old: formatMoney(plan.old.amount_minor, plan.old.currency),
          new: formatMoney(plan.new.amount_minor, plan.new.currency),
        },
        {
          label: "Account",
          old: accountName(plan.old.account_id),
          new: accountName(plan.new.account_id),
        },
        ...(plan.old.entry_type === "transfer"
          ? [
              {
                label: "To account",
                old: accountName(plan.old.to_account_id!),
                new: accountName(plan.new.to_account_id!),
              },
            ]
          : []),
        { label: "Date", old: renderDate(plan.old), new: renderDate(plan.new) },
        {
          label: "Note",
          old: plan.old.note || "No note",
          new: plan.new.note || "No note",
        },
        {
          label: "Category",
          old: categoryName(plan.old.category_id),
          new: categoryName(plan.new.category_id),
        },
      ].filter((d) => d.old !== d.new)
    : [];
  return (
    <Dialog
      title={plan ? "Review correction" : "Edit transaction"}
      description={`Transaction #${context.transaction_id} · ${original.currency}`}
      onClose={onClose}
      busy={busy}
    >
      {plan ? (
        <>
          <h3 className="correction-review-title" tabIndex={-1} ref={reviewRef}>
            Check the changes before saving.
          </h3>
          <dl className="correction-differences">
            {differences.map((d) => (
              <div key={d.label}>
                <dt>{d.label}</dt>
                <dd>
                  <span>Before: {d.old}</span>
                  <strong>After: {d.new}</strong>
                </dd>
              </div>
            ))}
          </dl>
          <h3 className="correction-review-title">Account balances</h3>
          <ul className="correction-balances">
            {plan.balances.map((b) => (
              <li key={b.account_id}>
                <strong>{b.name}</strong>
                <span>
                  {formatMoney(b.before_minor, b.currency)} →{" "}
                  {formatMoney(b.after_minor, b.currency)}
                </span>
                <small>Change: {formatMoney(b.change_minor, b.currency)}</small>
              </li>
            ))}
          </ul>
          <p className="input-hint">
            Saving keeps your original record and applies the correction
            together. Spending uses the updated record in its chosen period.
          </p>
          <label className="import-confirm">
            <input
              type="checkbox"
              checked={reviewed}
              disabled={busy}
              onChange={(e) => setReviewed(e.target.checked)}
            />
            <span>I reviewed this correction and its balance changes.</span>
          </label>
          {save.isError && (
            <p className="form-error" role="alert">
              {apiError(save.error)} Retrying this same correction is safe.
            </p>
          )}
          <div className="dialog-actions">
            <button
              className="btn"
              disabled={busy}
              onClick={() => {
                setPlan(null);
                save.reset();
                preview.reset();
              }}
            >
              Back to editing
            </button>
            <button
              className="btn btn-primary"
              disabled={!reviewed || busy}
              onClick={() => save.mutate()}
            >
              {save.isPending ? "Saving correction…" : "Save correction"}
            </button>
          </div>
          {save.isError && (
            <button
              className="text-button"
              onClick={() => {
                setPlan(null);
                onReload();
              }}
            >
              Reload record and review again
            </button>
          )}
        </>
      ) : (
        <form
          className="account-form"
          onChange={() => preview.reset()}
          onSubmit={(e) => {
            e.preventDefault();
            if (draft && !sameCorrection(draft, original)) preview.mutate();
          }}
        >
          {original.entry_type !== "transfer" &&
          original.entry_type !== "opening_balance" ? (
            <fieldset className="edit-kind" disabled={busy}>
              <legend className="label">Transaction type</legend>
              {(["expense", "income", "refund"] as const).map((value) => (
                <label key={value}>
                  <input
                    type="radio"
                    name="edit-kind"
                    checked={kind === value}
                    onChange={() => setKind(value)}
                  />
                  <span>{value[0].toUpperCase() + value.slice(1)}</span>
                </label>
              ))}
            </fieldset>
          ) : (
            <p className="input-hint">
              {original.entry_type === "transfer"
                ? "Edit this transfer between your own accounts."
                : "Correct the starting money or its date. Zero and money owed are allowed."}
            </p>
          )}
          <div>
            <label className="label" htmlFor="edit-amount">
              {kind === "opening_balance" ? "Starting balance" : "Amount"} ·{" "}
              {original.currency}
            </label>
            <input
              id="edit-amount"
              className="input"
              ref={amountRef}
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              inputMode="decimal"
              required
              disabled={busy}
              aria-invalid={!!amount && amountMinor === null}
              aria-describedby="edit-amount-help"
            />
            <p
              id="edit-amount-help"
              className={
                amount && amountMinor === null ? "field-error" : "input-hint"
              }
            >
              {amount && amountMinor === null
                ? `Enter ${kind === "opening_balance" ? "a valid" : "a positive"} amount with ${currencyDigits(original.currency) === 0 ? "whole units" : `up to ${currencyDigits(original.currency)} decimal places`}.`
                : kind === "opening_balance"
                  ? "Zero and negative starting balances are allowed."
                  : "Enter the amount before applying its income or expense direction."}
            </p>
          </div>
          <div>
            <label className="label" htmlFor="edit-account">
              {kind === "transfer" ? "From account" : "Account"}
            </label>
            <select
              id="edit-account"
              className="input"
              value={accountID}
              onChange={(e) => setAccountID(Number(e.target.value))}
              disabled={busy}
              required
            >
              {normal.map((a) => (
                <option key={a.id} value={a.id} disabled={!a.active}>
                  {a.name}
                  {!a.active ? " · inactive" : ""}
                </option>
              ))}
            </select>
            <p className="input-hint">
              Accounts use {original.currency}; no currency conversion is
              performed.
            </p>
          </div>
          {kind === "transfer" && (
            <div>
              <label className="label" htmlFor="edit-to-account">
                To account
              </label>
              <select
                id="edit-to-account"
                className="input"
                value={toAccountID}
                onChange={(e) => setToAccountID(Number(e.target.value))}
                disabled={busy}
                required
              >
                <option value="0">Choose destination</option>
                {normal
                  .filter((a) => a.id !== accountID)
                  .map((a) => (
                    <option key={a.id} value={a.id} disabled={!a.active}>
                      {a.name}
                    </option>
                  ))}
              </select>
            </div>
          )}
          <div>
            <label className="label" htmlFor="edit-note">
              Note · optional
            </label>
            <textarea
              id="edit-note"
              className="input"
              rows={2}
              maxLength={1000}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              disabled={busy}
            />
          </div>
          {kind !== "transfer" && kind !== "opening_balance" && (
            <div>
              <label className="label" htmlFor="edit-category">
                Category · optional
              </label>
              <select
                id="edit-category"
                className="input"
                value={categoryID}
                onChange={(e) => setCategoryID(Number(e.target.value))}
                disabled={busy}
              >
                <option value="0">Uncategorized</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
          )}
          <div>
            <label className="import-radio">
              <input
                type="checkbox"
                checked={dateOnly}
                onChange={(e) => setDateOnly(e.target.checked)}
                disabled={busy}
              />
              <span>Use a calendar date without a time</span>
            </label>
            {dateOnly ? (
              <>
                <label className="label" htmlFor="edit-date">
                  Calendar date · UTC
                </label>
                <input
                  id="edit-date"
                  type="date"
                  className="input"
                  value={date}
                  onChange={(e) => setDate(e.target.value)}
                  max={new Date().toISOString().slice(0, 10)}
                  disabled={busy}
                  required
                />
              </>
            ) : (
              <>
                <label className="label" htmlFor="edit-date-time">
                  Date and time · local
                </label>
                <input
                  id="edit-date-time"
                  type="datetime-local"
                  className="input"
                  value={localTime}
                  onChange={(e) => {
                    setLocalTime(e.target.value);
                    setTimeEdited(true);
                  }}
                  max={localDateTime()}
                  required
                  disabled={busy}
                />
              </>
            )}
          </div>
          {preview.isError && (
            <p className="form-error" role="alert">
              {apiError(preview.error)}{" "}
              <button
                className="text-button"
                type="button"
                onClick={() => onReload()}
              >
                Reload record
              </button>
            </p>
          )}
          <div className="dialog-actions">
            <button
              className="btn"
              type="button"
              onClick={onClose}
              disabled={busy}
            >
              Cancel
            </button>
            <button
              className="btn btn-primary"
              disabled={!draft || sameCorrection(draft, original) || busy}
            >
              {preview.isPending ? "Preparing review…" : "Review correction"}
              <Icon name="arrow" width="16" height="16" />
            </button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
