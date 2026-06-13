import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { downloadExport, getExportStatus, requestExport } from "../api/endpoints";
import { apiError } from "../lib/errors";

export default function Exports() {
  const [ym, setYm] = useState(new Date().toISOString().slice(0, 7));
  const [id, setId] = useState<number | null>(null);

  const request = useMutation({
    mutationFn: () => requestExport(ym),
    onSuccess: (res) => setId(res.id),
  });

  // Poll status every 1.5s until the worker finishes; then stop.
  const status = useQuery({
    queryKey: ["export", id],
    queryFn: () => getExportStatus(id as number),
    enabled: id != null,
    refetchInterval: (q) => {
      const s = q.state.data?.status;
      return s === "done" || s === "error" ? false : 1500;
    },
  });

  const s = status.data?.status;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Exports</h1>
        <p className="text-sm text-slate-500">Generate a CSV of a month's transactions across all your accounts.</p>
      </div>

      <div className="card space-y-3">
        <div className="flex flex-wrap items-end gap-3">
          <div>
            <label className="label">Month</label>
            <input type="month" className="input w-auto" value={ym} onChange={(e) => setYm(e.target.value)} />
          </div>
          <button className="btn btn-primary" onClick={() => request.mutate()} disabled={request.isPending}>
            {request.isPending ? "Requesting…" : "Request export"}
          </button>
        </div>
        {request.isError && <p className="field-error">{apiError(request.error)}</p>}
      </div>

      {id != null && (
        <div className="card flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="font-medium">Export #{id}</span>
            <StatusPill status={s} />
          </div>
          {s === "done" && (
            <button className="btn btn-primary" onClick={() => downloadExport(id)}>Download CSV</button>
          )}
          {s === "error" && <span className="field-error">Generation failed — try again.</span>}
        </div>
      )}
    </div>
  );
}

function StatusPill({ status }: { status?: string }) {
  const map: Record<string, string> = {
    pending: "bg-amber-100 text-amber-700",
    processing: "bg-amber-100 text-amber-700",
    done: "bg-green-100 text-green-700",
    error: "bg-red-100 text-red-700",
  };
  const cls = (status && map[status]) ?? "bg-slate-100 text-slate-600";
  return <span className={`badge ${cls}`}>{status ?? "…"}</span>;
}
