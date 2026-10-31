import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { requestExport } from "../api/endpoints";
import { apiError } from "../lib/errors";
import { isValidMonth } from "../lib/month";
import { rememberExport, type ExportJob } from "../lib/export-jobs";
import Icon from "../components/Icon";
import ExportJobCard from "../components/ExportJobCard";

export default function Exports() {
  const [ym, setYm] = useState(() => new Date().toISOString().slice(0, 7));
  const qc = useQueryClient();
  const jobs = useQuery<ExportJob[]>({
    queryKey: ["export-jobs"],
    queryFn: () => [],
    initialData: [],
    staleTime: Infinity,
    gcTime: Infinity,
  });
  const request = useMutation({
    mutationFn: (month: string) => requestExport(month),
    onSuccess: (data, month) => {
      qc.setQueryData<ExportJob[]>(["export-jobs"], (previous = []) =>
        rememberExport(previous, {
          id: data.id,
          month,
          requestedAt: new Date().toISOString(),
          initialStatus: data.status,
        }),
      );
    },
  });
  const generate = (month: string) => {
    if (isValidMonth(month) && !request.isPending) request.mutate(month);
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
            A month of details, neatly gathered in one CSV.
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
            <h2 id="new-export-title">Take a month with you.</h2>
            <p>
              Choose a month. We'll gather the entries from all your accounts.
            </p>
          </div>
          <form
            className="export-request-form"
            onSubmit={(e) => {
              e.preventDefault();
              generate(ym);
            }}
          >
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
                aria-describedby="export-month-help"
              />
              <p id="export-month-help" className="input-hint">
                Calendar months are based on UTC.
              </p>
            </div>
            <button
              className="btn btn-primary"
              disabled={!isValidMonth(ym) || request.isPending}
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
              Your month,
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
          <p>
            {isValidMonth(ym)
              ? `ledgerx-${ym}.csv`
              : "Choose a month to get started"}
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
              <h3>Your next month in review.</h3>
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
