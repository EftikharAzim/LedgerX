import { useState } from "react";
import { requestExport, getExportStatus } from "../api/endpoints";

export default function Exports() {
  const [ym, setYm] = useState(new Date().toISOString().slice(0,7));
  const [id, setId] = useState<number | null>(null);
  const [status, setStatus] = useState<string>("");

  async function kick() {
    const res = await requestExport(ym);
    setId(res.id); setStatus(res.status);
  }
  async function poll() {
    if (!id) return;
    const res = await getExportStatus(id);
    setStatus(res.status);
  }

  return (
    <div className="max-w-lg mx-auto p-6 space-y-3">
      <h1 className="text-2xl font-semibold">CSV Exports</h1>
      <input type="month" className="input" value={ym} onChange={e=>setYm(e.target.value)} />
      <div className="flex gap-2">
        <button className="btn" onClick={kick}>Request export</button>
        <button className="btn" onClick={poll} disabled={!id}>Check status</button>
      </div>
      {id && <p>Export #{id} → <b>{status}</b></p>}
      <style>{`.input{padding:.6rem;border:1px solid #ddd;border-radius:.75rem}.btn{padding:.6rem;border:1px solid #222;border-radius:.75rem}`}</style>
    </div>
  );
}