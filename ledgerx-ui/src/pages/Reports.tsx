import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { formatMoney, listAccounts, listCategories } from "../api/endpoints";
import { getFinancialReport } from "../api/money-management";
import { defaultReportSelection } from "../lib/spending";
import { groupReportCategories, shiftMonth } from "../lib/financial-reports";
import {
  reportRange,
  reportLabel,
  type ReportSelection,
} from "../lib/report-period";
import { monthLabel } from "../lib/month";
import { apiError } from "../lib/errors";
import PeriodPicker from "../components/PeriodPicker";

export default function Reports() {
  const [params] = useSearchParams();
  const [selection, setSelection] = useState<ReportSelection>(() =>
    params.get("month")
      ? { type: "monthly", month: params.get("month")! }
      : defaultReportSelection("monthly"),
  );
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const [choice, setChoice] = useState(params.get("currency") ?? "");
  const currencies = [
    ...new Set(
      accounts.data?.filter((a) => a.kind === "normal").map((a) => a.currency),
    ),
  ].sort();
  const code = choice || currencies[0] || "USD";
  const range = reportRange(selection);
  const report = useQuery({
    queryKey: ["financial-report", selection, code],
    queryFn: () => getFinancialReport(selection, code),
    enabled: !!range,
  });
  const [breakdown, setBreakdown] = useState<"income" | "spending">("spending");
  const data = report.data;
  const groups = data
    ? groupReportCategories(data.categories, categories.data ?? [])
        .filter((g) =>
          breakdown === "spending"
            ? g.children.some((c) => c.count > 0)
            : g.income !== 0,
        )
        .sort((a, b) => b[breakdown] - a[breakdown])
    : [];
  const activity = (category?: number) =>
    `/activity?${new URLSearchParams({ currency: code, ...(data?.start_date ? { start_date: data.start_date, end_date: data.end_date } : {}), ...(category !== undefined ? { category_id: String(category) } : {}) })}`;
  const maximum = data
    ? Math.max(
        1,
        ...data.trends.flatMap((t) => [
          Math.abs(t.income_minor),
          Math.abs(t.spending_minor),
        ]),
      )
    : 1;
  return (
    <div className="financial-reports-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">UNDERSTAND YOUR MONEY</p>
          <h1>See the story behind your spending.</h1>
          <p className="page-description">
            Explore income, spending and balances for any period, one currency
            at a time.
          </p>
        </div>
        <Link className="btn" to={`/exports?${new URLSearchParams(selection)}`}>
          Download CSV
        </Link>
      </div>
      <div className="money-controls">
        <PeriodPicker value={selection} onChange={setSelection} />
        <div className="money-control">
          <label htmlFor="report-currency">Currency</label>
          <select
            id="report-currency"
            className="input"
            value={code}
            onChange={(e) => setChoice(e.target.value)}
          >
            {[...new Set([...currencies, code])].map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </div>
        {selection.type === "monthly" && (
          <div className="import-actions">
            <button
              className="btn"
              aria-label="Previous report month"
              disabled={!shiftMonth(selection.month, -1)}
              onClick={() =>
                setSelection({
                  type: "monthly",
                  month: shiftMonth(selection.month, -1)!,
                })
              }
            >
              ←
            </button>
            <button
              className="btn"
              aria-label="Next report month"
              disabled={!shiftMonth(selection.month, 1)}
              onClick={() =>
                setSelection({
                  type: "monthly",
                  month: shiftMonth(selection.month, 1)!,
                })
              }
            >
              →
            </button>
          </div>
        )}
      </div>
      <p className="input-hint">
        {reportLabel(selection)} · All wallets in {code} · UTC calendar dates.{" "}
        <Link to="/planned">Future plans</Link> stay separate until confirmed.
      </p>
      {!range ? (
        <p role="alert">Choose a valid reporting period.</p>
      ) : report.isPending ? (
        <p role="status">Preparing your report…</p>
      ) : report.isError ? (
        <p role="alert" className="form-error">
          {apiError(report.error)}{" "}
          <button className="text-button" onClick={() => report.refetch()}>
            Try again
          </button>
        </p>
      ) : (
        data && (
          <>
            <section className="financial-stat-grid" aria-label="Period totals">
              {[
                ["Opening balance", data.opening_minor],
                ["Income", data.income_minor],
                ["Spending", data.spending_minor],
                ["Net income", data.net_minor],
                ["Ending balance", data.ending_minor],
              ].map(([label, value]) => (
                <article className="financial-stat" key={label}>
                  <span>{label}</span>
                  <strong>{formatMoney(Number(value), code)}</strong>
                </article>
              ))}
            </section>
            {!!data.other_movement_minor && (
              <details className="report-explanation">
                <summary>
                  Other balance changes:{" "}
                  {formatMoney(data.other_movement_minor, code)}
                </summary>
                <p>
                  Opening balance + net income + other balance changes = ending
                  balance. Other changes include starting money, report-excluded
                  activity, and corrections or reversals crossing period
                  boundaries. Income and spending use the current corrected
                  view.
                </p>
                <Link className="text-link" to={activity()}>
                  Review all period activity
                </Link>
              </details>
            )}
            {data.previous && (
              <section
                className="report-comparison"
                aria-label="Previous period comparison"
              >
                <h2>Compared with the previous period</h2>
                <p className="input-hint">
                  {data.previous_start} to {data.previous_end}. Full calendar
                  periods are compared; the current period may still be in
                  progress.
                </p>
                <dl>
                  {[
                    [
                      "Income",
                      data.change?.income_minor ?? 0,
                      data.previous.income_minor,
                    ],
                    [
                      "Spending",
                      data.change?.spending_minor ?? 0,
                      data.previous.spending_minor,
                    ],
                    [
                      "Net income",
                      data.change?.net_minor ?? 0,
                      data.previous.net_minor,
                    ],
                  ].map(([name, now, before]) => (
                    <div key={name}>
                      <dt>{name}</dt>
                      <dd>
                        {formatMoney(Number(now), code)} change{" "}
                        <span className="muted">
                          · previously {formatMoney(Number(before), code)}
                        </span>
                      </dd>
                    </div>
                  ))}
                </dl>
              </section>
            )}
            <section
              className="report-trends"
              aria-labelledby="report-trends-title"
            >
              <h2 id="report-trends-title">Six-month trends</h2>
              <p className="input-hint">
                Select a month to explore it. Income and spending values are
                shown for every month.
              </p>
              <div className="trend-bars">
                {data.trends.map((t) => (
                  <button
                    key={t.month}
                    className="trend-month"
                    title={`View ${monthLabel(t.month)} report`}
                    aria-pressed={
                      selection.type === "monthly" &&
                      selection.month === t.month
                    }
                    onClick={() =>
                      setSelection({ type: "monthly", month: t.month })
                    }
                  >
                    <span className="trend-plot" aria-hidden="true">
                      <span
                        className="trend-income"
                        style={{
                          height: `${Math.max(1, (Math.abs(t.income_minor) / maximum) * 100)}%`,
                        }}
                      />
                      <span
                        className="trend-spending"
                        style={{
                          height: `${Math.max(1, (Math.abs(t.spending_minor) / maximum) * 100)}%`,
                        }}
                      />
                    </span>
                    <strong>{monthLabel(t.month)}</strong>
                    <span>In {formatMoney(t.income_minor, code)}</span>
                    <span>Spent {formatMoney(t.spending_minor, code)}</span>
                  </button>
                ))}
              </div>
            </section>
            <section aria-labelledby="report-categories-title">
              <div className="section-heading">
                <h2 id="report-categories-title">By category</h2>
                <div className="money-control">
                  <label htmlFor="report-breakdown">Breakdown</label>
                  <select
                    id="report-breakdown"
                    className="input"
                    value={breakdown}
                    onChange={(e) =>
                      setBreakdown(e.target.value as "income" | "spending")
                    }
                  >
                    <option value="spending">Spending</option>
                    <option value="income">Income</option>
                  </select>
                </div>
              </div>
              <p className="input-hint">
                Parent totals include direct entries and subcategories once.
                Refunds reduce spending.
              </p>
              {!groups.length ? (
                <p>No {breakdown} recorded for this period.</p>
              ) : (
                <div className="report-category-list">
                  {groups.map((g) => (
                    <details key={g.id} className="report-category">
                      <summary>
                        <span>{g.name}</span>
                        <strong>{formatMoney(g[breakdown], code)}</strong>
                      </summary>
                      <ul>
                        {g.children.map((c) => (
                          <li key={c.category_id}>
                            <Link to={activity(c.category_id)}>
                              {c.parent_id ? c.name : "Direct entries"}
                            </Link>
                            <span>
                              {formatMoney(
                                breakdown === "spending"
                                  ? c.spending_minor
                                  : c.income_minor,
                                code,
                              )}
                            </span>
                          </li>
                        ))}
                      </ul>
                      <Link className="text-link" to={activity(g.id)}>
                        View category activity
                      </Link>
                    </details>
                  ))}
                </div>
              )}
            </section>
            <p className="input-hint">
              Transfers and opening money affect wallet movement; they are
              excluded from income and spending. Report-excluded entries and
              unconfirmed plans are excluded from these spending totals.{" "}
              <Link to="/budgets">Turn this into a monthly budget</Link>.
            </p>
          </>
        )
      )}
    </div>
  );
}
