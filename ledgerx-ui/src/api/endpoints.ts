import api from "./client";

export type Account = {
  id: number;
  user_id: number;
  name: string;
  currency: string;
  active: boolean;
  kind: "normal" | "external";
};

export type Entry = {
  posting_id: number;
  transaction_id: number;
  account_id: number;
  amount_minor: number;
  currency: string;
  occurred_at: string;
  note?: string;
};

export async function register(email: string, password: string) {
  const res = await api.post("/auth/register", { email, password });
  return res.data as { token: string };
}
export async function login(email: string, password: string) {
  const res = await api.post("/auth/login", { email, password });
  return res.data as { token: string };
}

export async function listAccounts() {
  const res = await api.get("/v1/accounts");
  return res.data as Account[];
}
export async function createAccount(name: string, currency = "USD") {
  const res = await api.post("/v1/accounts", { name, currency });
  return res.data as Account;
}

export async function createTransaction(body: {
  account_id: number; amount_minor: number;
  currency: string; occurred_at: string; note?: string;
}) {
  const res = await api.post("/v1/transactions", body, {
    // retry-safe: the server replays the cached response for a repeated key
    headers: { "Idempotency-Key": crypto.randomUUID() },
  });
  return res.data;
}

export async function createTransfer(body: {
  from_account_id: number; to_account_id: number; amount_minor: number;
  currency: string; occurred_at: string; note?: string;
}) {
  const res = await api.post("/v1/transfers", body, {
    headers: { "Idempotency-Key": crypto.randomUUID() },
  });
  return res.data;
}

export async function reverseTransaction(transactionId: number) {
  const res = await api.post(`/v1/transactions/${transactionId}/reverse`, null, {
    headers: { "Idempotency-Key": crypto.randomUUID() },
  });
  return res.data;
}

export async function listEntries(accountId: number, cursor?: string) {
  const res = await api.get(`/v1/accounts/${accountId}/transactions`, {
    params: { limit: 20, ...(cursor ? { cursor } : {}) },
  });
  return res.data as { entries: Entry[]; next_cursor: string };
}

export async function getMonthlySummary(accountId: number, ym: string) {
  const res = await api.get(`/v1/accounts/${accountId}/summary`, { params: { month: ym } });
  return res.data as { inflow: number; outflow: number; net: number };
}

export async function getCurrentBalance(accountId: number) {
  const res = await api.get(`/v1/accounts/${accountId}/balance`);
  return res.data as { account_id: number; as_of: string; balance_minor: number };
}

export async function requestExport(month: string) {
  const res = await api.post(`/v1/exports`, null, { params: { month } });
  return res.data as { id: number; status: string };
}
export async function getExportStatus(id: number) {
  const res = await api.get(`/v1/exports/${id}/status`);
  return res.data as { id: number; status: string; file_path?: string };
}

// Downloads via XHR so the Authorization header is sent (the endpoint is
// authenticated now), then hands the blob to the browser.
export async function downloadExport(id: number) {
  const res = await api.get(`/v1/exports/${id}/download`, { responseType: "blob" });
  const url = URL.createObjectURL(res.data as Blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `export_${id}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

export function formatMoney(minor: number, currency: string) {
  return new Intl.NumberFormat(undefined, { style: "currency", currency }).format(minor / 100);
}
