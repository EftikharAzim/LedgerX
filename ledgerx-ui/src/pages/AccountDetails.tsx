import { useParams } from "react-router-dom";
import { useQuery, useMutation } from "@tanstack/react-query";
import { getMonthlySummary, createTransaction } from "../api/endpoints";
import { useState } from "react";

export default function AccountDetails() {
  const { id } = useParams(); const accountId = Number(id);
  const [ym, setYm] = useState(new Date().toISOString().slice(0,7)); // "YYYY-MM"
  const { data: sum } = useQuery({
    queryKey: ["summary", accountId, ym],
    queryFn: () => getMonthlySummary(accountId, ym)
  });

  const [amount, setAmount] = useState(0);
  const [note, setNote] = useState("");
  const create = useMutation({
    mutationFn: () => createTransaction({
      user_id: 1, account_id: accountId, amount_minor: amount,
      currency: "USD", occurred_at: new Date().toISOString(), note
    })
  });

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-6">
      <div className="flex items-center gap-3">
        <h1 className="text-2xl font-semibold">Account {accountId}</h1>
        <input type="month" className="input" value={ym} onChange={e=>setYm(e.target.value)} />
      </div>

      <div className="grid grid-cols-3 gap-3">
        <Stat label="Inflow"  value={sum?.inflow ?? 0} />
        <Stat label="Outflow" value={sum?.outflow ?? 0} />
        <Stat label="Net"     value={sum?.net ?? 0} />
      </div>

      <div className="p-4 border rounded-xl space-y-3">
        <h2 className="font-semibold">New transaction</h2>
        <input className="input" type="number" value={amount} onChange={e=>setAmount(Number(e.target.value))} placeholder="amount_minor (+ income, - expense)"/>
        <input className="input" value={note} onChange={e=>setNote(e.target.value)} placeholder="note (optional)"/>
        <button className="btn" onClick={()=>create.mutate()}>Create</button>
      </div>

      <style>{`.input{padding:.6rem;border:1px solid #ddd;border-radius:.75rem}.btn{padding:.6rem;border:1px solid #222;border-radius:.75rem}`}</style>
    </div>
  );
}
function Stat({label, value}:{label:string; value:number}) {
  return <div className="p-4 border rounded-xl"><div className="text-sm opacity-70">{label}</div><div className="text-xl font-semibold">{value}</div></div>;
}