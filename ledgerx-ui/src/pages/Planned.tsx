import { useState } from "react";
import { Link } from "react-router-dom";
import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { listPlans, resolvePlan, type PlannedEntry } from "../api/plans";
import { formatMoney } from "../api/endpoints";
import { apiError } from "../lib/errors";
import { localDateTime } from "../lib/money";
import { refreshLedger } from "../lib/refresh-ledger";
import Dialog from "../components/Dialog";

export default function Planned() {
  const qc = useQueryClient();
  const plans = useInfiniteQuery({
    queryKey: ["plans"],
    queryFn: ({ pageParam }) => listPlans(pageParam),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_offset || undefined,
  });
  const [choice, setChoice] = useState<{
    entry: PlannedEntry;
    action: "post" | "cancel";
  } | null>(null);
  const [date, setDate] = useState(() => localDateTime().slice(0, 10));
  const [notice, setNotice] = useState("");
  const save = useMutation({
    mutationFn: () => resolvePlan(choice!.entry.id, choice!.action, date),
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
          <p className="eyebrow">LOOK AHEAD</p>
          <h1>Planned activity</h1>
          <p className="page-description">
            Upcoming money in and out. Your balance changes only when you
            confirm it happened.
          </p>
        </div>
        <Link className="btn" to="/activity">
          Back to activity
        </Link>
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
          <h2>Nothing planned yet</h2>
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
                    setDate(localDateTime().slice(0, 10));
                    setChoice({ entry: e, action: "post" });
                  }}
                >
                  Record now
                </button>
                <button
                  className="text-button"
                  onClick={() => {
                    save.reset();
                    setChoice({ entry: e, action: "cancel" });
                  }}
                >
                  Cancel plan
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
              : "Cancel this plan?"
          }
          onClose={() => setChoice(null)}
          busy={save.isPending}
        >
          <form
            className="account-form"
            onSubmit={(e) => {
              e.preventDefault();
              if (!save.isPending) save.mutate();
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
                <label className="label" htmlFor="plan-date">
                  Actual date
                </label>
                <input
                  className="input"
                  id="plan-date"
                  type="date"
                  required
                  value={date}
                  max={localDateTime().slice(0, 10)}
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
              <p>The plan will leave this list. No money will be recorded.</p>
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
                disabled={save.isPending || (choice.action === "post" && !date)}
              >
                {save.isPending
                  ? "Saving…"
                  : choice.action === "post"
                    ? "Confirm & record"
                    : "Cancel plan"}
              </button>
            </div>
          </form>
        </Dialog>
      )}
    </div>
  );
}
