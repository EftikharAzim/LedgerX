import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { createAccount, listAccounts } from "../api/endpoints";
import { Link } from "react-router-dom";
import { useState } from "react";

export default function Accounts() {
  const qc = useQueryClient();
  const { data, isLoading } = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const [name, setName] = useState("");

  const create = useMutation({
    mutationFn: () => createAccount(name),
    onSuccess: () => { setName(""); qc.invalidateQueries({ queryKey: ["accounts"] }); }
  });

  if (isLoading) return <p className="p-6">Loading…</p>;

  return (
    <div className="max-w-3xl mx-auto p-6">
      <h1 className="text-2xl font-semibold mb-4">Accounts</h1>
      <div className="flex gap-2 mb-6">
        <input className="input flex-1" placeholder="New account name" value={name} onChange={e=>setName(e.target.value)} />
        <button className="btn" onClick={()=>create.mutate()} disabled={!name}>Create</button>
      </div>
      <ul className="space-y-2">
        {data?.map(a => (
          <li key={a.id} className="p-3 border rounded-xl flex justify-between">
            <span>{a.name}</span>
            <Link className="underline" to={`/accounts/${a.id}`}>Open</Link>
          </li>
        ))}
      </ul>
      <style>{`.input{padding:.6rem;border:1px solid #ddd;border-radius:.75rem}.btn{padding:.6rem .9rem;border:1px solid #222;border-radius:.75rem}`}</style>
    </div>
  );
}