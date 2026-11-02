import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useSearchParams } from "react-router-dom";
import { requestExport } from "../api/endpoints";
import { apiError } from "../lib/errors";
import {
  reportRange,
  reportLabel,
  type ReportSelection,
} from "../lib/report-period";
import { rememberExport, type ExportJob } from "../lib/export-jobs";
import Icon from "../components/Icon";
import ExportJobCard from "../components/ExportJobCard";

export default function Exports() {
  const [params] = useSearchParams();
  const [ym, setYm] = useState(
    () => params.get("month") || new Date().toISOString().slice(0, 7),
  );
  const [type, setType] = useState<ReportSelection["type"]>(
    ["monthly", "weekly", "yearly", "custom", "all"].includes(
      params.get("type") ?? "",
    )
      ? (params.get("type") as ReportSelection["type"])
      : "monthly",
  );
  const [weekOf, setWeekOf] = useState(
    () => params.get("week_of") || new Date().toISOString().slice(0, 10),
  );
  const [year, setYear] = useState(
    () =>
      params.get("year") ||
      String(new Date().getUTCFullYear()).padStart(4, "0"),
  );
  const [start, setStart] = useState(
    () =>
      params.get("start_date") || new Date().toISOString().slice(0, 7) + "-01",
  );
  const [end, setEnd] = useState(
    () => params.get("end_date") || new Date().toISOString().slice(0, 10),
  );
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
          <h1>Export CSV</h1>
        </div>
      </div>
      <div className="export-create-grid">
        <section
          className="export-request-panel"
          aria-labelledby="new-export-title"
        >
          <h2 id="new-export-title">Choose a period</h2>
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
                    All recorded transactions across your accounts. Unconfirmed
                    plans are excluded.
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
              Export requested.
            </p>
          )}
        </section>
      </div>
      {jobs.data.length > 0 && (
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
                Your latest five requests. This list resets when you reload or
                log out.
              </p>
            </div>
            <Icon name="clock" width="20" height="20" />
          </div>
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
        </section>
      )}
      <details className="export-explanation">
        <summary>What’s included</summary>
        <p>
          All recorded account movements, including transfers and linked
          reversals. Spending reports apply different exclusions. CSV amounts
          include currency information; unconfirmed plans are excluded.
        </p>
      </details>
    </div>
  );
}
