import { useMutation, useQuery } from "@tanstack/react-query";
import { downloadExport, getExportStatus } from "../api/endpoints";
import {
  exportPresentation,
  shouldPollExport,
  type ExportJob,
} from "../lib/export-jobs";
import { monthLabel } from "../lib/month";
import { apiError } from "../lib/errors";
import Icon from "./Icon";

export default function ExportJobCard({
  job,
  onGenerateAgain,
  generating,
}: {
  job: ExportJob;
  onGenerateAgain: (month: string) => void;
  generating: boolean;
}) {
  const status = useQuery({
    queryKey: ["export", job.id],
    queryFn: () => getExportStatus(job.id),
    initialData: { id: job.id, status: job.initialStatus },
    staleTime: 0,
    refetchInterval: (query) =>
      shouldPollExport(query.state.data?.status, query.state.status === "error")
        ? 1500
        : false,
  });
  const download = useMutation({
    mutationFn: () => downloadExport(job.id, `ledgerx-${job.month}.csv`),
  });
  const unavailable = status.isError;
  const presentation = exportPresentation(
    unavailable ? "unavailable" : status.data?.status,
  );
  return (
    <li className="export-job-card" data-export-id={job.id}>
      <div className="export-job-main">
        <span className={`export-job-symbol ${presentation.tone}`}>
          <Icon
            name={
              presentation.tone === "ready"
                ? "check"
                : presentation.tone === "failed"
                  ? "info"
                  : "clock"
            }
            width="23"
            height="23"
          />
        </span>
        <div className="export-job-name">
          <h3>{monthLabel(job.month)}</h3>
          <span>CSV · All accounts · Export #{job.id}</span>
        </div>
        <span className={`export-status ${presentation.tone}`} role="status">
          {presentation.label}
        </span>
      </div>
      <div className="export-job-details">
        <div>
          <p>{presentation.description}</p>
          <time dateTime={job.requestedAt}>
            Requested{" "}
            {new Date(job.requestedAt).toLocaleTimeString(undefined, {
              hour: "numeric",
              minute: "2-digit",
            })}
          </time>
        </div>
        {unavailable ||
        !["pending", "processing", "done", "error"].includes(
          status.data?.status ?? "",
        ) ? (
          <button
            className="btn"
            disabled={status.isFetching}
            onClick={() => status.refetch()}
          >
            {status.isFetching ? "Checking…" : "Check again"}
          </button>
        ) : status.data?.status === "done" ? (
          <button
            className="btn btn-primary"
            disabled={download.isPending}
            onClick={() => download.mutate()}
          >
            <Icon name="download" width="16" height="16" />
            {download.isPending ? "Downloading…" : "Download CSV"}
          </button>
        ) : status.data?.status === "error" ? (
          <button
            className="btn"
            disabled={generating}
            onClick={() => onGenerateAgain(job.month)}
          >
            Generate again
            <Icon name="arrow" width="16" height="16" />
          </button>
        ) : (
          <span className="export-in-progress">
            <span className="status-pulse" />
            We'll keep checking for you.
          </span>
        )}
      </div>
      {unavailable && (
        <p role="alert" className="export-card-error">
          Couldn't check the status. Your request may still be processing.
        </p>
      )}
      {download.isError && (
        <p role="alert" className="export-card-error">
          Download couldn't start. {apiError(download.error)} Try Download CSV
          again.
        </p>
      )}
      {download.isSuccess && (
        <p role="status" className="category-saved">
          <Icon name="check" width="14" height="14" />
          Download started.
        </p>
      )}
    </li>
  );
}
