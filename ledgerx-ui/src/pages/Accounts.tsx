import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useState } from "react";
import {
  type Account, createAccount, formatMoney, getCurrentBalance, listAccounts,
} from "../api/endpoints";
import { apiError } from "../lib/errors";

const CURRENCIES = ["USD", "EUR", "GBP", "JPY", "BDT", "INR"];

export default function Accounts() {
  const qc = useQueryClient();
  const { data, isLoading, isError, error } = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const [name, setName] = useState("");
  const [currency, setCurrency] = useState("USD");

  const create = useMutation({
    mutationFn: () => createAccount(name.trim(), currency),
    onSuccess: () => { setName(""); qc.invalidateQueries({ queryKey: ["accounts"] }); },
  });

  const accounts = data?.filter((a) => a.kind !== "external") ?? [];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Accounts</h1>
        <p className="text-sm text-slate-500">Each account tracks a running balance in its own currency.</p>
      </div>

      <div className="card">
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={(e) => { e.preventDefault(); if (name.trim()) create.mutate(); }}
        >
          <div className="flex-1 min-w-[12rem]">
            <label className="label">New account name</label>
            <input className="input" placeholder="e.g. Checking" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div>
            <label className="label">Currency</label>
            <select className="input" value={currency} onChange={(e) => setCurrency(e.target.value)}>
              {CURRENCIES.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </div>
          <button className="btn btn-primary" disabled={!name.trim() || create.isPending}>
            {create.isPending ? "Creating…" : "Create account"}
          </button>
        </form>
        {create.isError && <p className="field-error mt-2">{apiError(create.error)}</p>}
      </div>

      {isLoading && <p className="text-slate-500">Loading accounts…</p>}
      {isError && <p className="field-error">{apiError(error, "Could not load accounts")}</p>}

      {!isLoading && !isError && accounts.length === 0 && (
        <div className="card text-center text-slate-500">
          No accounts yet. Create your first one above to start recording transactions.
        </div>
      )}

      <ul className="space-y-2">
        {accounts.map((a) => <AccountRow key={a.id} account={a} />)}
      </ul>
    </div>
  );
}

function AccountRow({ account }: { account: Account }) {
  const { data: bal } = useQuery({
    queryKey: ["balance", account.id],
    queryFn: () => getCurrentBalance(account.id),
  });
  return (
    <li>
      <Link
        to={`/accounts/${account.id}`}
        className="card flex items-center justify-between transition hover:border-indigo-300 hover:shadow"
      >
        <div className="flex items-center gap-3">
          <span className="font-medium">{account.name}</span>
          <span className="badge">{account.currency}</span>
          {!account.active && <span className="badge bg-amber-100 text-amber-700">inactive</span>}
        </div>
        <div className="text-right">
          <div className="text-lg font-semibold tabular-nums">
            {bal ? formatMoney(bal.balance_minor, account.currency) : "—"}
          </div>
          <div className="text-xs text-slate-400">balance</div>
        </div>
      </Link>
    </li>
  );
}
