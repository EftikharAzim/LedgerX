import { Link, useParams } from "react-router-dom";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import {
  confirmCategory, createCategory, createTransaction, createTransfer, formatMoney,
  getCurrentBalance, getMonthlySummary, listAccounts, listCategories, listEntries,
  reverseTransaction,
} from "../api/endpoints";
import { apiError } from "../lib/errors";

export default function AccountDetails() {
  const { id } = useParams();
  const accountId = Number(id);
  const qc = useQueryClient();
  const [ym, setYm] = useState(new Date().toISOString().slice(0, 7)); // "YYYY-MM"

  const { data: accounts } = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const account = accounts?.find((a) => a.id === accountId);
  const currency = account?.currency ?? "USD";

  const { data: sum } = useQuery({
    queryKey: ["summary", accountId, ym],
    queryFn: () => getMonthlySummary(accountId, ym),
  });
  const { data: bal } = useQuery({
    queryKey: ["balance", accountId],
    queryFn: () => getCurrentBalance(accountId),
  });
  const entries = useInfiniteQuery({
    queryKey: ["entries", accountId],
    queryFn: ({ pageParam }) => listEntries(accountId, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
  });

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["entries", accountId] });
    qc.invalidateQueries({ queryKey: ["balance", accountId] });
    qc.invalidateQueries({ queryKey: ["summary", accountId] });
  };

  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const create = useMutation({
    mutationFn: () => createTransaction({
      account_id: accountId,
      amount_minor: Math.round(Number(amount) * 100),
      currency, occurred_at: new Date().toISOString(), note,
    }),
    onSuccess: () => { setAmount(""); setNote(""); refresh(); },
  });

  const reverse = useMutation({
    mutationFn: (transactionId: number) => reverseTransaction(transactionId),
    onSuccess: refresh,
  });

  const { data: categories } = useQuery({ queryKey: ["categories"], queryFn: listCategories });
  const categoryName = (cid?: number) => categories?.find((c) => c.id === cid)?.name;
  const [newCategory, setNewCategory] = useState("");
  const addCategory = useMutation({
    mutationFn: () => createCategory(newCategory.trim()),
    onSuccess: () => { setNewCategory(""); qc.invalidateQueries({ queryKey: ["categories"] }); },
  });
  const accept = useMutation({
    mutationFn: ({ txId, categoryId }: { txId: number; categoryId: number }) => confirmCategory(txId, categoryId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["entries", accountId] }),
  });

  const [transferTo, setTransferTo] = useState("");
  const [transferAmount, setTransferAmount] = useState("");
  const transferTargets = accounts?.filter(
    (a) => a.id !== accountId && a.kind === "normal" && a.currency === currency,
  ) ?? [];
  const transfer = useMutation({
    mutationFn: () => createTransfer({
      from_account_id: accountId,
      to_account_id: Number(transferTo),
      amount_minor: Math.round(Number(transferAmount) * 100),
      currency, occurred_at: new Date().toISOString(),
    }),
    onSuccess: () => { setTransferAmount(""); refresh(); },
  });

  const allEntries = entries.data?.pages.flatMap((p) => p.entries) ?? [];

  return (
    <div className="space-y-6">
      <Link to="/accounts" className="text-sm text-indigo-600 hover:underline">← Accounts</Link>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-semibold">{account?.name ?? `Account ${accountId}`}</h1>
          <span className="badge">{currency}</span>
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-500">
          Month
          <input type="month" className="input w-auto" value={ym} onChange={(e) => setYm(e.target.value)} />
        </label>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label="Balance" value={formatMoney(bal?.balance_minor ?? 0, currency)} accent />
        <Stat label="Inflow" value={formatMoney(sum?.inflow ?? 0, currency)} />
        <Stat label="Outflow" value={formatMoney(sum?.outflow ?? 0, currency)} />
        <Stat label="Net" value={formatMoney(sum?.net ?? 0, currency)} />
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="card space-y-3">
          <h2 className="font-semibold">New transaction</h2>
          <p className="text-xs text-slate-400">Positive amounts are income, negative are expenses.</p>
          <input className="input" type="number" step="0.01" value={amount}
            onChange={(e) => setAmount(e.target.value)} placeholder="0.00" />
          <input className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Note (optional)" />
          <button className="btn btn-primary w-full" onClick={() => create.mutate()}
            disabled={!amount || create.isPending}>
            {create.isPending ? "Saving…" : "Add transaction"}
          </button>
          {create.isError && <p className="field-error">{apiError(create.error)}</p>}
        </div>

        <div className="card space-y-3">
          <h2 className="font-semibold">Transfer</h2>
          {transferTargets.length === 0 ? (
            <p className="text-sm text-slate-400">
              Create another {currency} account to transfer between accounts.
            </p>
          ) : (
            <>
              <select className="input" value={transferTo} onChange={(e) => setTransferTo(e.target.value)}>
                <option value="">Select destination…</option>
                {transferTargets.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
              </select>
              <input className="input" type="number" step="0.01" min="0.01" value={transferAmount}
                onChange={(e) => setTransferAmount(e.target.value)} placeholder="0.00" />
              <button className="btn btn-primary w-full" onClick={() => transfer.mutate()}
                disabled={!transferTo || !transferAmount || transfer.isPending}>
                {transfer.isPending ? "Transferring…" : "Transfer"}
              </button>
              {transfer.isError && <p className="field-error">{apiError(transfer.error)}</p>}
            </>
          )}
        </div>
      </div>

      <div className="card space-y-3">
        <h2 className="font-semibold">Categories</h2>
        <p className="text-xs text-slate-400">
          New income/expense transactions with a note are auto-suggested a category (✨); accept it from the history below.
        </p>
        <div className="flex flex-wrap gap-2">
          {categories?.length
            ? categories.map((c) => <span key={c.id} className="badge">{c.name}</span>)
            : <span className="text-sm text-slate-400">No categories yet.</span>}
        </div>
        <form className="flex gap-2" onSubmit={(ev) => { ev.preventDefault(); if (newCategory.trim()) addCategory.mutate(); }}>
          <input className="input" value={newCategory} onChange={(e) => setNewCategory(e.target.value)} placeholder="New category name" />
          <button className="btn" disabled={!newCategory.trim() || addCategory.isPending}>Add</button>
        </form>
        {addCategory.isError && <p className="field-error">{apiError(addCategory.error)}</p>}
      </div>

      <div className="card">
        <h2 className="mb-3 font-semibold">History</h2>
        {entries.isLoading && <p className="text-slate-500">Loading…</p>}
        {!entries.isLoading && allEntries.length === 0 && (
          <p className="text-slate-400">No transactions yet.</p>
        )}
        {reverse.isError && <p className="field-error mb-2">{apiError(reverse.error)}</p>}
        <ul className="divide-y divide-slate-100">
          {allEntries.map((e) => {
            const isReversal = e.note?.startsWith("reversal of");
            return (
              <li key={e.posting_id} className="flex items-center justify-between gap-3 py-3">
                <div>
                  <div className="text-sm font-medium">{new Date(e.occurred_at).toLocaleDateString()}</div>
                  {e.note && <div className="text-sm text-slate-500">{e.note}</div>}
                  <div className="mt-1">
                    {e.category_id ? (
                      <span className="badge bg-indigo-50 text-indigo-700">{categoryName(e.category_id) ?? "categorized"}</span>
                    ) : e.suggested_category_id ? (
                      <button
                        className="badge bg-amber-50 text-amber-700 hover:bg-amber-100"
                        title={`AI suggestion${e.suggestion_score ? ` (${Math.round(e.suggestion_score * 100)}%)` : ""} — click to confirm`}
                        disabled={accept.isPending}
                        onClick={() => accept.mutate({ txId: e.transaction_id, categoryId: e.suggested_category_id! })}
                      >
                        ✨ {categoryName(e.suggested_category_id) ?? "suggested"} — accept
                      </button>
                    ) : null}
                  </div>
                </div>
                <div className="flex items-center gap-3">
                  <span className={`tabular-nums font-medium ${e.amount_minor < 0 ? "text-red-600" : "text-green-700"}`}>
                    {formatMoney(e.amount_minor, e.currency)}
                  </span>
                  {!isReversal && (
                    <button
                      className="btn-ghost rounded-lg px-2 py-1 text-xs"
                      title="Create a reversing transaction"
                      disabled={reverse.isPending}
                      onClick={() => {
                        if (confirm("Reverse this transaction? A negating entry will be created.")) {
                          reverse.mutate(e.transaction_id);
                        }
                      }}
                    >
                      reverse
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
        {entries.hasNextPage && (
          <button className="btn mt-3" onClick={() => entries.fetchNextPage()} disabled={entries.isFetchingNextPage}>
            {entries.isFetchingNextPage ? "Loading…" : "Load more"}
          </button>
        )}
      </div>
    </div>
  );
}

function Stat({ label, value, accent }: { label: string; value: string; accent?: boolean }) {
  return (
    <div className={`card ${accent ? "border-indigo-200 bg-indigo-50" : ""}`}>
      <div className="text-sm text-slate-500">{label}</div>
      <div className="text-xl font-semibold tabular-nums">{value}</div>
    </div>
  );
}
