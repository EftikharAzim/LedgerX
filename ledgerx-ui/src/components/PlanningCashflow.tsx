import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getCashflow } from "../api/schedules";
import { formatMoney } from "../api/endpoints";
import { apiError } from "../lib/errors";

export default function PlanningCashflow() {
  const [open, setOpen] = useState(false);
  const cashflow = useQuery({
    queryKey: ["cashflow"],
    queryFn: getCashflow,
    enabled: open,
  });
  return (
    <details
      className="planning-cashflow"
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>Projected cash flow · next 30 days</summary>
      <p className="input-hint">
        An estimate based on recorded balances and pending plans. Overdue plans
        are assumed to happen today; daily totals do not predict payment order.
        Recording a payment updates your actual balance.
      </p>
      {open &&
        (cashflow.isPending ? (
          <p role="status">Preparing projected balances…</p>
        ) : cashflow.isError ? (
          <p role="alert" className="form-error">
            {apiError(cashflow.error)}{" "}
            <button className="text-button" onClick={() => cashflow.refetch()}>
              Try again
            </button>
          </p>
        ) : (
          cashflow.data && (
            <>
              <p className="input-hint">
                {cashflow.data.as_of} to {cashflow.data.through_date} · UTC
                calendar dates · wallets and currencies kept separate.
              </p>
              {!cashflow.data.wallets.length ? (
                <p>Create a wallet to start planning.</p>
              ) : (
                <div className="cashflow-grid">
                  {cashflow.data.wallets.map((w) => (
                    <article className="cashflow-wallet" key={w.account_id}>
                      <h2>
                        {w.name} <span className="muted">{w.currency}</span>
                      </h2>
                      <dl>
                        <div>
                          <dt>Recorded balance</dt>
                          <dd>{formatMoney(w.current_minor, w.currency)}</dd>
                        </div>
                        <div>
                          <dt>Planned money in</dt>
                          <dd>{formatMoney(w.in_minor, w.currency)}</dd>
                        </div>
                        <div>
                          <dt>Planned money out</dt>
                          <dd>{formatMoney(w.out_minor, w.currency)}</dd>
                        </div>
                        <div>
                          <dt>Projected balance</dt>
                          <dd>{formatMoney(w.projected_minor, w.currency)}</dd>
                        </div>
                      </dl>
                      {w.lowest_minor < 0 && (
                        <p className="cashflow-low">
                          A projected daily balance falls below zero:{" "}
                          {formatMoney(w.lowest_minor, w.currency)}.
                        </p>
                      )}
                      {!!w.overdue_count && (
                        <p className="input-hint">
                          Includes {w.overdue_count} overdue{" "}
                          {w.overdue_count === 1 ? "plan" : "plans"} assumed to
                          happen today.
                        </p>
                      )}
                      {!w.active && (
                        <p className="input-hint">
                          This wallet is archived. It must be active to record
                          payments.
                        </p>
                      )}
                      {!!w.days.length && (
                        <details>
                          <summary>Daily projection</summary>
                          <ul className="cashflow-days">
                            {w.days.map((d) => (
                              <li key={d.date}>
                                <span>
                                  {d.date}
                                  <small>
                                    {d.count} plans · in{" "}
                                    {formatMoney(d.in_minor, w.currency)} · out{" "}
                                    {formatMoney(d.out_minor, w.currency)}
                                  </small>
                                </span>
                                <strong>
                                  {formatMoney(d.projected_minor, w.currency)}
                                </strong>
                              </li>
                            ))}
                          </ul>
                        </details>
                      )}
                    </article>
                  ))}
                </div>
              )}
            </>
          )
        ))}
    </details>
  );
}
