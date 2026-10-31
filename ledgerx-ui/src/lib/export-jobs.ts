import type { ReportSelection } from "./report-period.ts";

export type ExportJob = {
  id: number;
  month?: string;
  selection?: ReportSelection;
  requestedAt: string;
  initialStatus: string;
};

export function jobSelection(job: ExportJob): ReportSelection {
  return job.selection ?? { type: "monthly", month: job.month ?? "" };
}

export function rememberExport(
  previous: ExportJob[],
  job: ExportJob,
): ExportJob[] {
  return [job, ...previous.filter((item) => item.id !== job.id)].slice(0, 5);
}

export function shouldPollExport(
  status: string | undefined,
  failed = false,
): boolean {
  return (
    !failed &&
    (status === undefined || status === "pending" || status === "processing")
  );
}

export function exportPresentation(status: string | undefined) {
  switch (status) {
    case "pending":
      return {
        label: "Queued",
        description: "Your request is in the queue.",
        tone: "queued",
      };
    case "processing":
      return {
        label: "Preparing",
        description: "Putting your records together.",
        tone: "queued",
      };
    case "done":
      return {
        label: "Ready",
        description: "Your CSV is ready to take with you.",
        tone: "ready",
      };
    case "error":
      return {
        label: "Couldn't prepare",
        description: "Try generating this report again.",
        tone: "failed",
      };
    case undefined:
      return {
        label: "Checking",
        description: "Checking on your request…",
        tone: "queued",
      };
    default:
      return {
        label: "Status unavailable",
        description: "Check again for an update.",
        tone: "failed",
      };
  }
}
