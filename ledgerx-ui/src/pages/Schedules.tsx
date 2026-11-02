import { useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  formatMoney,
  listAccounts,
  listCategories,
  type Account,
  type Category,
} from "../api/endpoints";
import {
  listSchedules,
  saveSchedule,
  scheduleAction,
  type Schedule,
  type ScheduleInput,
} from "../api/schedules";
import { apiError } from "../lib/errors";
import { availableCategory, categoryLabel } from "../lib/categories";
import { parseAmount } from "../lib/money";
import { minorInput } from "../lib/imports";
import { refreshLedger } from "../lib/refresh-ledger";
import { nextWriteAttempt, type WriteAttempt } from "../lib/write-attempt";
import Dialog from "../components/Dialog";
import AmountInput from "../components/AmountInput";

export default function Schedules() {
  const qc = useQueryClient();
  const schedules = useQuery({
    queryKey: ["schedules"],
    queryFn: listSchedules,
  });
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const [editing, setEditing] = useState<Schedule | "new" | null>(null);
  const [choice, setChoice] = useState<{
    schedule: Schedule;
    action: "pause" | "resume" | "end";
  } | null>(null);
  const [showEnded, setShowEnded] = useState(false);
  const [notice, setNotice] = useState("");
  const active =
    accounts.data?.filter((a) => a.active && a.kind === "normal") ?? [];
  const action = useMutation({
    mutationFn: () => scheduleAction(choice!.schedule, choice!.action),
    onSuccess: async () => {
      setNotice(
        choice!.action === "pause"
          ? "Schedule paused. Future occurrences are on hold; already-due plans stay available."
          : choice!.action === "resume"
            ? "Schedule resumed from today. Previously skipped dates stay skipped."
            : "Schedule ended. Already-due plans stay available until you record or skip them.",
      );
      setChoice(null);
      await refreshLedger(qc);
    },
  });
  return (
    <div className="schedules-page">
      <div className="page-heading">
        <div>
          <h1>Recurring bills & income</h1>
          <p className="page-description">
            Prepare regular commitments once. Confirm each payment when it
            happens.
          </p>
        </div>
        <button
          className="btn btn-primary"
          disabled={
            accounts.isPending ||
            categories.isPending ||
            accounts.isError ||
            categories.isError ||
            !active.length
          }
          onClick={() => setEditing("new")}
        >
          New schedule
        </button>
      </div>
      <p>
        <Link className="text-link" to="/planned">
          View upcoming plans & cash flow
        </Link>
      </p>
      {notice && (
        <p role="status" className="success-notice">
          {notice}
        </p>
      )}
      {!accounts.isPending && !active.length && (
        <p className="input-hint">
          <Link to="/accounts">Create an active wallet</Link> before setting up
          a recurring commitment.
        </p>
      )}
      {(accounts.isError || categories.isError) && (
        <p role="alert" className="form-error">
          Wallets or categories could not be loaded.{" "}
          <button
            className="text-button"
            onClick={() => {
              void accounts.refetch();
              void categories.refetch();
            }}
          >
            Try again
          </button>
        </p>
      )}
      <p className="input-hint">
        We prepare occurrences 90 days ahead while the app runs, and refresh
        when you open Planned activity. Due dates use UTC. Nothing is recorded
        automatically.
      </p>
      <label className="show-ended">
        <input
          type="checkbox"
          checked={showEnded}
          onChange={(e) => setShowEnded(e.target.checked)}
        />{" "}
        Show ended schedules
      </label>
      {schedules.isPending ? (
        <p role="status">Loading schedules…</p>
      ) : schedules.isError ? (
        <p className="form-error" role="alert">
          {apiError(schedules.error)}{" "}
          <button className="text-button" onClick={() => schedules.refetch()}>
            Try again
          </button>
        </p>
      ) : !schedules.data.filter((s) => showEnded || s.status !== "ended")
          .length ? (
        <section className="import-panel">
          <h2>No recurring schedules</h2>
          <p>
            Start with rent, a subscription, or regular income. Your recorded
            balance changes only after confirmation.
          </p>
        </section>
      ) : (
        <section className="schedule-list" aria-label="Recurring schedules">
          {schedules.data
            .filter((s) => showEnded || s.status !== "ended")
            .map((s) => (
              <article className="schedule-card" key={s.id}>
                <div>
                  <p className="eyebrow">
                    {s.status} · {s.cadence}
                  </p>
                  <h2>
                    {s.note ||
                      (s.amount_minor < 0
                        ? "Recurring expense"
                        : "Recurring income")}
                  </h2>
                  <p className="muted">
                    {s.account_name} · {s.category_name}
                  </p>
                  <p className="input-hint">
                    First due {s.first_due_date}
                    {s.end_date ? ` · Ends ${s.end_date}` : ""}
                    {s.next_due_date
                      ? ` · Next pending ${s.next_due_date}`
                      : ""}
                  </p>
                </div>
                <strong>{formatMoney(s.amount_minor, s.currency)}</strong>
                {s.status !== "ended" && (
                  <div className="import-actions">
                    <button className="btn" onClick={() => setEditing(s)}>
                      Edit schedule
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        action.reset();
                        setChoice({
                          schedule: s,
                          action: s.status === "paused" ? "resume" : "pause",
                        });
                      }}
                    >
                      {s.status === "paused" ? "Resume" : "Pause"}
                    </button>
                    <button
                      className="text-button"
                      onClick={() => {
                        action.reset();
                        setChoice({ schedule: s, action: "end" });
                      }}
                    >
                      End schedule
                    </button>
                  </div>
                )}
              </article>
            ))}
        </section>
      )}
      {editing && (
        <ScheduleDialog
          key={editing === "new" ? "new" : editing.id}
          schedule={editing === "new" ? undefined : editing}
          accounts={accounts.data?.filter((a) => a.kind === "normal") ?? []}
          categories={categories.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null);
            setNotice(
              "Schedule saved. Its occurrences remain planned until you confirm them.",
            );
            await refreshLedger(qc);
          }}
        />
      )}
      {choice && (
        <Dialog
          title={
            choice.action === "pause"
              ? "Pause this schedule?"
              : choice.action === "resume"
                ? "Resume this schedule?"
                : "End this schedule?"
          }
          onClose={() => setChoice(null)}
          busy={action.isPending}
        >
          <div className="account-form">
            <p>
              {choice.schedule.note} ·{" "}
              {formatMoney(
                choice.schedule.amount_minor,
                choice.schedule.currency,
              )}
            </p>
            <p>
              {choice.action === "pause"
                ? "Future occurrences will be put on hold. Already-due plans are kept for you to record or skip."
                : choice.action === "resume"
                  ? "The schedule resumes from today. Dates during the pause stay skipped; previously skipped occurrences are kept skipped."
                  : "Future occurrences will be canceled. Already-due plans are kept. To restart an ended schedule, create a new one."}
            </p>
            {action.isError && (
              <p className="form-error" role="alert">
                {apiError(action.error)}{" "}
                <button
                  className="text-button"
                  onClick={() => {
                    setChoice(null);
                    void schedules.refetch();
                  }}
                >
                  Reload latest schedules
                </button>
              </p>
            )}
            <div className="dialog-actions">
              <button
                className="btn"
                disabled={action.isPending}
                onClick={() => setChoice(null)}
              >
                Go back
              </button>
              <button
                className="btn btn-primary"
                disabled={action.isPending}
                onClick={() => action.mutate()}
              >
                {action.isPending
                  ? "Saving…"
                  : choice.action === "pause"
                    ? "Pause schedule"
                    : choice.action === "resume"
                      ? "Resume schedule"
                      : "Confirm end"}
              </button>
            </div>
          </div>
        </Dialog>
      )}
    </div>
  );
}

function ScheduleDialog({
  schedule,
  accounts,
  categories,
  onClose,
  onSaved,
}: {
  schedule?: Schedule;
  accounts: Account[];
  categories: Category[];
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [accountID, setAccountID] = useState(
    schedule?.account_id ?? accounts.find((a) => a.active)?.id ?? 0,
  );
  const account = accounts.find((a) => a.id === accountID);
  const [kind, setKind] = useState<"expense" | "income">(
    (schedule?.amount_minor ?? -1) < 0 ? "expense" : "income",
  );
  const [amount, setAmount] = useState(
    schedule
      ? minorInput(Math.abs(schedule.amount_minor), schedule.currency)
      : "",
  );
  const [note, setNote] = useState(schedule?.note ?? "");
  const [category, setCategory] = useState(schedule?.category_id ?? 0);
  const [cadence, setCadence] = useState<Schedule["cadence"]>(
    schedule?.cadence ?? "monthly",
  );
  const [first, setFirst] = useState(
    schedule?.first_due_date ?? new Date().toISOString().slice(0, 10),
  );
  const [end, setEnd] = useState(schedule?.end_date ?? "");
  const attempt = useRef<WriteAttempt | null>(null);
  const minor = parseAmount(amount, account?.currency ?? "USD");
  const save = useMutation({
    mutationFn: () => {
      const body: ScheduleInput = {
        account_id: account!.id,
        currency: account!.currency,
        amount_minor: kind === "expense" ? -minor! : minor!,
        category_id: category,
        note: note.trim(),
        cadence,
        first_due_date: first,
        end_date: end,
        ...(schedule ? { revision: schedule.revision } : {}),
      };
      attempt.current = nextWriteAttempt(attempt.current, body);
      return saveSchedule(body, attempt.current.key, schedule?.id);
    },
    onSuccess: onSaved,
  });
  return (
    <Dialog
      title={schedule ? "Edit recurring schedule" : "Create recurring schedule"}
      onClose={onClose}
      busy={save.isPending}
    >
      <form
        className="account-form"
        onChange={() => {
          if (save.isError) save.reset();
        }}
        onSubmit={(e) => {
          e.preventDefault();
          if (minor && account?.active && !save.isPending) save.mutate();
        }}
      >
        <div>
          <label className="label" htmlFor="schedule-name">
            Name
          </label>
          <input
            id="schedule-name"
            className="input"
            required
            maxLength={1000}
            value={note}
            disabled={save.isPending}
            placeholder="e.g. Rent or monthly salary"
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
        <div>
          <label className="label" htmlFor="schedule-type">
            Type
          </label>
          <select
            id="schedule-type"
            className="input"
            value={kind}
            disabled={save.isPending}
            onChange={(e) => {
              setKind(e.target.value as "expense" | "income");
              setCategory(0);
            }}
          >
            <option value="expense">Expense</option>
            <option value="income">Income</option>
          </select>
        </div>
        <div>
          <label className="label" htmlFor="schedule-wallet">
            Wallet
          </label>
          <select
            id="schedule-wallet"
            className="input"
            value={accountID}
            disabled={save.isPending}
            onChange={(e) => {
              const next = accounts.find(
                (a) => a.id === Number(e.target.value),
              );
              if (next?.currency !== account?.currency) setAmount("");
              setAccountID(Number(e.target.value));
            }}
          >
            {accounts
              .filter((a) => a.active || a.id === schedule?.account_id)
              .map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} · {a.currency}
                  {a.active ? "" : " (archived)"}
                </option>
              ))}
          </select>
        </div>
        <AmountInput
          currency={account?.currency ?? "USD"}
          value={amount}
          onChange={setAmount}
          disabled={save.isPending}
        />
        <div>
          <label className="label" htmlFor="schedule-category">
            Category
          </label>
          <select
            id="schedule-category"
            className="input"
            value={category}
            disabled={save.isPending}
            onChange={(e) => setCategory(Number(e.target.value))}
          >
            <option value={0}>Uncategorized</option>
            {categories
              .filter(
                (c) =>
                  c.id === schedule?.category_id ||
                  availableCategory(c, categories, kind),
              )
              .map((c) => (
                <option key={c.id} value={c.id}>
                  {categoryLabel(c, categories)}
                  {c.archived ? " (archived)" : ""}
                </option>
              ))}
          </select>
        </div>
        <div className="schedule-date-fields">
          <div>
            <label className="label" htmlFor="schedule-frequency">
              Repeat
            </label>
            <select
              id="schedule-frequency"
              className="input"
              value={cadence}
              disabled={save.isPending}
              onChange={(e) =>
                setCadence(e.target.value as Schedule["cadence"])
              }
            >
              <option value="weekly">Weekly</option>
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly</option>
            </select>
          </div>
          <div>
            <label className="label" htmlFor="schedule-first">
              First due date · UTC
            </label>
            <input
              id="schedule-first"
              className="input"
              type="date"
              required
              min="0001-01-01"
              max="9999-12-31"
              value={first}
              disabled={save.isPending}
              onChange={(e) => setFirst(e.target.value)}
            />
          </div>
          <div>
            <label className="label" htmlFor="schedule-end">
              End date · optional
            </label>
            <input
              id="schedule-end"
              className="input"
              type="date"
              min={first || "0001-01-01"}
              max="9999-12-31"
              value={end}
              disabled={save.isPending}
              onChange={(e) => setEnd(e.target.value)}
            />
          </div>
        </div>
        <p className="input-hint">
          Monthly dates clamp to the last day of short months, then return to
          the original day. New schedules can include the past year. Pattern
          changes need a first due date from today onward.
        </p>
        {schedule && (
          <p className="input-hint">
            Edits replace pending occurrences from today. Earlier due plans,
            recorded payments and manually skipped dates are kept.
          </p>
        )}
        {save.isError && (
          <p className="form-error" role="alert">
            {apiError(save.error)} You can retry safely.
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            disabled={save.isPending}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={
              !minor ||
              !account?.active ||
              !note.trim() ||
              !first ||
              save.isPending
            }
          >
            {save.isPending ? "Saving…" : "Save schedule"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
