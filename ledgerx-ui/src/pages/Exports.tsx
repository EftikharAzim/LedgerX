import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { requestExport } from "../api/endpoints";
import { apiError } from "../lib/errors";
import {
  reportRange,
  reportLabel,
  reportFilename,
  type ReportSelection,
} from "../lib/report-period";
import { rememberExport, type ExportJob } from "../lib/export-jobs";
import Icon from "../components/Icon";
import ExportJobCard from "../components/ExportJobCard";

export default function Exports() {
  const [ym, setYm] = useState(() => new Date().toISOString().slice(0, 7));
  const [type, setType] = useState<ReportSelection["type"]>("monthly");
  const [weekOf, setWeekOf] = useState(() =>
    new Date().toISOString().slice(0, 10),
  );
  const [year, setYear] = useState(() =>
    String(new Date().getUTCFullYear()).padStart(4, "0"),
  );
  const [start, setStart] = useState(
    () => new Date().toISOString().slice(0, 7) + "-01",
  );
  const [end, setEnd] = useState(() => new Date().toISOString().slice(0, 10));
  const selection: ReportSelection =
    type === "monthly"
      ? { type, month: ym }
      : type === "weekly"
        ? { type, week_of: weekOf }
        : type === "yearly"
          ? { type, year }
          : type === "custom"
            ? { type, start_date: start, end_date: end }
            : { type: "all" };
  const range = reportRange(selection);
  const qc = useQueryClient();
  const jobs = useQuery<ExportJob[]>({
    queryKey: ["export-jobs"],
    queryFn: () => [],
    initialData: [],
    staleTime: Infinity,
    gcTime: Infinity,
  });
  const request = useMutation({
    mutationFn: (period: ReportSelection) => requestExport(period),
    onSuccess: (data, period) => {
      qc.setQueryData<ExportJob[]>(["export-jobs"], (previous = []) =>
        rememberExport(previous, {
          id: data.id,
          selection: { ...period },
          requestedAt: new Date().toISOString(),
          initialStatus: data.status,
        }),
      );
    },
  });
  const generate = (period: ReportSelection) => {
    if (reportRange(period) && !request.isPending) request.mutate(period);
  };

  return (
    <div className="exports-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">THE BIG PICTURE. TO GO.</p>
          <h1>
            Your records.
            <br />
            <em>Ready to go.</em>
          </h1>
          <p className="page-description">
            Your details, neatly gathered in one CSV.
          </p>
        </div>
        <span className="export-format-label">
          <Icon name="export" width="17" height="17" />
          CSV EXPORT
        </span>
      </div>
      <div className="export-create-grid">
        <section
          className="export-request-panel"
          aria-labelledby="new-export-title"
        >
          <div>
            <span className="account-symbol tone-sage">
              <Icon name="export" width="24" height="24" />
            </span>
            <h2 id="new-export-title">Take your records with you.</h2>
            <p>
              Choose your report period. We'll gather the entries from all your
              accounts.
            </p>
          </div>
          <form
            className="export-request-form report-form"
            onSubmit={(e) => {
              e.preventDefault();
              generate(selection);
            }}
          >
            <fieldset className="report-types" disabled={request.isPending}>
              <legend className="label">Report period</legend>
              <div>
                {(
                  ["monthly", "weekly", "yearly", "custom", "all"] as const
                ).map((kind) => (
                  <label key={kind}>
                    <input
                      type="radio"
                      name="report-type"
                      checked={type === kind}
                      onChange={() => {
                        setType(kind);
                        request.reset();
                      }}
                    />
                    <span>
                      {kind === "all"
                        ? "All time"
                        : kind.charAt(0).toUpperCase() + kind.slice(1)}
                    </span>
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="report-fields">
              {type === "monthly" && (
                <div>
                  <label htmlFor="export-month" className="label">
                    Month to export
                  </label>
                  <input
                    id="export-month"
                    className="input"
                    type="month"
                    min="0001-01"
                    max="9999-12"
                    value={ym}
                    onChange={(e) => {
                      setYm(e.target.value);
                      request.reset();
                    }}
                    disabled={request.isPending}
                    required
                  />
                </div>
              )}
              {type === "weekly" && (
                <div>
                  <label htmlFor="export-week" className="label">
                    Week containing
                  </label>
                  <input
                    id="export-week"
                    className="input"
                    type="date"
                    min="0001-01-01"
                    max="9999-12-31"
                    value={weekOf}
                    onChange={(e) => {
                      setWeekOf(e.target.value);
                      request.reset();
                    }}
                    disabled={request.isPending}
                    required
                  />
                  <p className="input-hint">
                    Choose any day. Your report covers Monday through Sunday.
                  </p>
                </div>
              )}
              {type === "yearly" && (
                <div>
                  <label htmlFor="export-year" className="label">
                    Year to export
                  </label>
                  <input
                    id="export-year"
                    className="input"
                    type="text"
                    inputMode="numeric"
                    pattern="[0-9]{4}"
                    maxLength={4}
                    value={year}
                    onChange={(e) => {
                      setYear(e.target.value);
                      request.reset();
                    }}
                    placeholder="2026"
                    disabled={request.isPending}
                    required
                  />
                  <p className="input-hint">
                    January 1 through December 31 of this year.
                  </p>
                </div>
              )}
              {type === "custom" && (
                <div className="custom-report-dates">
                  <div>
                    <label htmlFor="export-from" className="label">
                      From date
                    </label>
                    <input
                      id="export-from"
                      className="input"
                      type="date"
                      min="0001-01-01"
                      max="9999-12-31"
                      value={start}
                      onChange={(e) => {
                        setStart(e.target.value);
                        request.reset();
                      }}
                      disabled={request.isPending}
                      required
                    />
                  </div>
                  <div>
                    <label htmlFor="export-to" className="label">
                      To date
                    </label>
                    <input
                      id="export-to"
                      className="input"
                      type="date"
                      min={start || "0001-01-01"}
                      max="9999-12-31"
                      value={end}
                      onChange={(e) => {
                        setEnd(e.target.value);
                        request.reset();
                      }}
                      disabled={request.isPending}
                      required
                    />
                  </div>
                </div>
              )}
              {type === "all" && (
                <div className="all-report-description">
                  <Icon name="receipt" width="22" height="22" />
                  <p>
                    Every recorded entry across all your accounts, including
                    past and future-dated entries.
                  </p>
                </div>
              )}
            </div>
            <p
              className={range ? "report-range-summary" : "field-error"}
              role="status"
            >
              {range ? (
                <>
                  <Icon name="calendar" width="15" height="15" />
                  {reportLabel(selection)}
                  {type !== "all" && " · UTC · End date included"}
                </>
              ) : type === "custom" ? (
                "Choose valid dates, with the end on or after the start."
              ) : (
                "Choose a valid report period."
              )}
            </p>
            <button
              className="btn btn-primary"
              disabled={!range || request.isPending}
            >
              {request.isPending ? "Requesting…" : "Generate CSV"}
              <Icon name="arrow" width="17" height="17" />
            </button>
          </form>
          {request.isError && (
            <p role="alert" className="form-error">
              <Icon name="info" width="17" height="17" />
              {apiError(request.error)} Please try generating the export again.
            </p>
          )}
          {request.isSuccess && (
            <p role="status" className="category-saved">
              <Icon name="check" width="15" height="15" />
              Request received. Follow its progress below.
            </p>
          )}
          <div className="export-included">
            <span>
              <Icon name="check" width="14" height="14" />
              All your accounts
            </span>
            <span>
              <Icon name="check" width="14" height="14" />
              Original entry details
            </span>
            <span>
              <Icon name="check" width="14" height="14" />
              Separate currency column
            </span>
          </div>
        </section>
        <aside className="export-preview">
          <div className="export-paper" aria-hidden="true">
            <span>
              <BrandFileMark />
              <span>LedgerX</span>
            </span>
            <div className="paper-title">
              Your records,
              <br />
              <em>in the details.</em>
            </div>
            <div className="paper-lines">
              <i />
              <i />
              <i />
              <i />
            </div>
            <span className="paper-csv">.CSV</span>
          </div>
          <p className="report-filename">
            {range
              ? reportFilename(selection)
              : "Choose a report period to get started"}
          </p>
          <span>Open in your favorite spreadsheet.</span>
        </aside>
      </div>
      <section
        className="recent-exports"
        aria-labelledby="recent-exports-title"
      >
        <div className="section-heading">
          <div>
            <h2 id="recent-exports-title">
              Requested this visit
              {jobs.data.length > 0 && (
                <span className="count-badge">{jobs.data.length}</span>
              )}
            </h2>
            <p>
              Your latest five requests. This list resets when you reload or log
              out.
            </p>
          </div>
          <Icon name="clock" width="20" height="20" />
        </div>
        {jobs.data.length ? (
          <ul className="export-jobs">
            {jobs.data.map((job) => (
              <ExportJobCard
                job={job}
                key={job.id}
                generating={request.isPending}
                onGenerateAgain={generate}
              />
            ))}
          </ul>
        ) : (
          <div className="export-empty">
            <span className="account-symbol tone-apricot">
              <Icon name="download" width="26" height="26" />
            </span>
            <div>
              <h3>Your next report, at a glance.</h3>
              <p>
                Generate your first CSV above. Its progress and download will
                appear here.
              </p>
            </div>
          </div>
        )}
      </section>
      <div className="export-footnote">
        <Icon name="info" width="17" height="17" />
        <p>
          CSV amounts use the currency's smallest unit: 100 cents is $1.00,
          while 100 yen is ¥100. Currency, dates, notes, account names, and
          category IDs are included.
        </p>
      </div>
      <p className="accounts-footnote">
        <Icon name="shield" width="15" height="15" />
        Only your workspace's records are included.
      </p>
    </div>
  );
}

function BrandFileMark() {
  return <span className="paper-mark" />;
}
