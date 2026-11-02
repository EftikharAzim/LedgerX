import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { listPlans, resolvePlan, type PlannedEntry } from "../api/plans";
import { syncSchedules } from "../api/schedules";
import PlanningCashflow from "../components/PlanningCashflow";
import { formatMoney, listAccounts } from "../api/endpoints";
import { apiError } from "../lib/errors";
import { parseAmount } from "../lib/money";
import { minorInput } from "../lib/imports";
import AmountInput from "../components/AmountInput";
import { refreshLedger } from "../lib/refresh-ledger";
import Dialog from "../components/Dialog";

export default function Planned() {
  const qc = useQueryClient();
  const [view, setView] = useState("all");
  const [currency, setCurrency] = useState("");
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const {
    mutate: refreshSchedules,
    isPending: refreshing,
    isError: refreshFailed,
    error: refreshError,
  } = useMutation({
    mutationFn: syncSchedules,
    onSuccess: () => refreshLedger(qc),
  });
  useEffect(() => {
    refreshSchedules();
  }, [refreshSchedules]);
  const today = new Date().toISOString().slice(0, 10);
  const plans = useInfiniteQuery({
    queryKey: ["plans", view, currency],
    queryFn: ({ pageParam }) => listPlans(pageParam, view, currency),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_offset || undefined,
  });
  const [choice, setChoice] = useState<{
    entry: PlannedEntry;
    action: "post" | "cancel";
  } | null>(null);
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [notice, setNotice] = useState("");
  const [actual, setActual] = useState("");
  const actualMinor = choice
    ? parseAmount(actual, choice.entry.currency)
    : null;
  const save = useMutation({
    mutationFn: () =>
      resolvePlan(
        choice!.entry.id,
        choice!.action,
        date,
        choice!.action === "post" && actualMinor
          ? choice!.entry.amount_minor < 0
            ? -actualMinor
            : actualMinor
          : undefined,
      ),
    onSuccess: async () => {
      setNotice(
        choice!.action === "post"
          ? "Recorded once. Your balance and reports are updated."
          : "Plan canceled. Your balance is unchanged.",
      );
      setChoice(null);
      await refreshLedger(qc);
    },
  });
  const entries = plans.data?.pages.flatMap((p) => p.entries) ?? [];
  return (
    <div className="planned-page">
      <div className="page-heading">
        <div>
          <h1>Planned activity</h1>
          <p className="page-description">
            Upcoming money in and out. Your balance changes only when you
            confirm it happened.
          </p>
        </div>
        <div className="data-tool-links">
          <Link className="btn btn-primary" to="/schedules">
            Recurring bills & income
          </Link>
          <Link className="text-link" to="/activity">
            Back to activity
          </Link>
        </div>
      </div>
      {notice && (
        <p className="success-notice" role="status">
          {notice}
        </p>
      )}
      <p className="input-hint">
        To add a plan, choose a future date when adding an expense or income.
        Due plans stay here until you record or cancel them.
      </p>
      <PlanningCashflow />
      <div className="planning-controls">
        <div>
          <label className="label" htmlFor="planned-window">
            Show plans
          </label>
          <select
            className="input"
            id="planned-window"
            value={view}
            onChange={(e) => setView(e.target.value)}
          >
            <option value="all">All pending</option>
            <option value="overdue">Overdue</option>
            <option value="soon">Next 30 days</option>
          </select>
        </div>
        <div>
          <label className="label" htmlFor="planned-currency">
            Currency
          </label>
          <select
            className="input"
            id="planned-currency"
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
          >
            <option value="">All currencies</option>
            {[
              ...new Set(
                accounts.data
                  ?.filter((a) => a.kind === "normal")
                  .map((a) => a.currency),
              ),
            ]
              .sort()
              .map((c) => (
                <option key={c}>{c}</option>
              ))}
          </select>
        </div>
        <button
          className="btn"
          disabled={refreshing}
          onClick={() => refreshSchedules()}
        >
          {refreshing ? "Refreshing…" : "Refresh plans"}
        </button>
      </div>
      {refreshFailed && (
        <p className="form-error" role="alert">
          {apiError(refreshError)} Existing plans are still available.{" "}
          <button className="text-button" onClick={() => refreshSchedules()}>
            Retry refresh
          </button>
        </p>
      )}
      <p className="input-hint">
        Due status and the 30-day window use UTC calendar dates.
      </p>
      {plans.isPending ? (
        <p role="status">Loading plans…</p>
      ) : plans.isError ? (
        <div role="alert">
          <p>{apiError(plans.error)}</p>
          <button className="btn" onClick={() => plans.refetch()}>
            Try again
          </button>
        </div>
      ) : !entries.length ? (
        <section className="import-panel">
          <h2>
            {view === "overdue"
              ? "No overdue plans"
              : view === "soon" || currency
                ? "No matching plans"
                : "Nothing planned yet"}
          </h2>
          <p>
            Plan a bill, a larger purchase, or expected income without changing
            today's money.
          </p>
          <Link className="btn" to="/activity">
            Add a transaction
          </Link>
        </section>
      ) : (
        <section aria-label="Pending plans" className="planned-list">
          {entries.map((e) => (
            <article className="planned-card" key={e.id}>
              <div>
                <p className="eyebrow">
                  {new Date(e.occurred_at).toLocaleDateString(undefined, {
                    timeZone: e.date_only ? "UTC" : undefined,
                    dateStyle: "medium",
                  })}
                </p>
                <p className="plan-due-status">
                  {e.occurred_at.slice(0, 10) < today
                    ? "Overdue"
                    : e.occurred_at.slice(0, 10) === today
                      ? "Due today"
                      : "Upcoming"}
                  {e.schedule_id ? " · Recurring" : ""}
                </p>
                <h2>
                  {e.note ||
                    (e.amount_minor < 0 ? "Planned expense" : "Planned income")}
                </h2>
                <p className="muted">
                  {e.account_name}
                  {e.category_name ? ` · ${e.category_name}` : ""}
                  {e.exclude_from_reports ? " · Excluded from reports" : ""}
                </p>
              </div>
              <strong className="planned-amount">
                {formatMoney(e.amount_minor, e.currency)}
              </strong>
              <div className="import-actions">
                <button
                  className="btn btn-primary"
                  onClick={() => {
                    save.reset();
                    setDate(new Date().toISOString().slice(0, 10));
                    setActual(minorInput(Math.abs(e.amount_minor), e.currency));
                    setChoice({ entry: e, action: "post" });
                  }}
                >
                  {e.schedule_id
                    ? e.amount_minor < 0
                      ? "Mark paid"
                      : "Mark received"
                    : "Record now"}
                </button>
                <button
                  className="text-button"
                  onClick={() => {
                    save.reset();
                    setChoice({ entry: e, action: "cancel" });
                  }}
                >
                  {e.schedule_id ? "Skip occurrence" : "Cancel plan"}
                </button>
              </div>
            </article>
          ))}
        </section>
      )}
      {plans.hasNextPage && (
        <button
          className="btn"
          disabled={plans.isFetchingNextPage}
          onClick={() => plans.fetchNextPage()}
        >
          {plans.isFetchingNextPage ? "Loading…" : "Load more plans"}
        </button>
      )}
      {choice && (
        <Dialog
          title={
            choice.action === "post"
              ? "Record planned activity"
              : choice.entry.schedule_id
                ? "Skip this occurrence?"
                : "Cancel this plan?"
          }
          onClose={() => setChoice(null)}
          busy={save.isPending}
        >
          <form
            className="account-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (
                !save.isPending &&
                (choice.action === "cancel" || actualMinor)
              )
                save.mutate();
            }}
          >
            <p>
              {choice.entry.note || "Planned activity"} ·{" "}
              {formatMoney(choice.entry.amount_minor, choice.entry.currency)}
            </p>
            {choice.action === "post" ? (
              <>
                <p>
                  This records the money as received or spent and updates your
                  balance.
                </p>
                <AmountInput
                  currency={choice.entry.currency}
                  value={actual}
                  onChange={(value) => {
                    setActual(value);
                    save.reset();
                  }}
                  disabled={save.isPending}
                />
                <p className="input-hint">
                  Confirming completes this occurrence. Use the actual amount
                  paid or received; the planned estimate is kept.
                </p>
                <label className="label" htmlFor="plan-date">
                  Actual date
                </label>
                <input
                  className="input"
                  id="plan-date"
                  type="date"
                  required
                  value={date}
                  max={new Date().toISOString().slice(0, 10)}
                  onChange={(e) => {
                    setDate(e.target.value);
                    save.reset();
                  }}
                  disabled={save.isPending}
                />
                <p className="input-hint">
                  Recorded as a calendar date in UTC, consistent with imported
                  history.
                </p>
              </>
            ) : (
              <p>
                {choice.entry.schedule_id
                  ? "This skips only this occurrence. The recurring schedule stays available, and no money is recorded."
                  : "The plan will leave this list. No money will be recorded."}
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
                onClick={() => setChoice(null)}
              >
                Go back
              </button>
              <button
                className="btn btn-primary"
                disabled={
                  save.isPending ||
                  (choice.action === "post" && (!date || !actualMinor))
                }
              >
                {save.isPending
                  ? "Saving…"
                  : choice.action === "post"
                    ? "Confirm & record"
                    : choice.entry.schedule_id
                      ? "Skip occurrence"
                      : "Cancel plan"}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
