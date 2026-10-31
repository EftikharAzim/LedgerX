import api from "./client.ts";
export { formatMoney } from "../lib/money.ts";

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
  category_id?: number;
  suggested_category_id?: number;
  suggestion_score?: number;
  reversal_of?: number;
  reversed_by?: number;
  is_transfer: boolean;
};

export type Category = { id: number; user_id: number; name: string };

export type Tokens = {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
};

export async function register(
  email: string,
  password: string,
  signal?: AbortSignal,
) {
  const res = await api.post("/auth/register", { email, password }, { signal });
  return res.data as Tokens;
}
export async function login(
  email: string,
  password: string,
  signal?: AbortSignal,
) {
  const res = await api.post("/auth/login", { email, password }, { signal });
  return res.data as Tokens;
}
export async function logout(refreshToken: string) {
  await api.post("/auth/logout", { refresh_token: refreshToken });
}

export async function listAccounts() {
  const res = await api.get("/v1/accounts");
  return res.data as Account[];
}
export async function createAccount(name: string, currency = "USD") {
  const res = await api.post("/v1/accounts", { name, currency });
  return res.data as Account;
}

export async function createTransaction(
  body: {
    account_id: number;
    amount_minor: number;
    currency: string;
    occurred_at: string;
    note?: string;
  },
  idempotencyKey: string = crypto.randomUUID(),
) {
  const res = await api.post("/v1/transactions", body, {
    // retry-safe: the server replays the cached response for a repeated key
    headers: { "Idempotency-Key": idempotencyKey },
  });
  return res.data;
}

export async function createTransfer(
  body: {
    from_account_id: number;
    to_account_id: number;
    amount_minor: number;
    currency: string;
    occurred_at: string;
    note?: string;
  },
  idempotencyKey: string = crypto.randomUUID(),
) {
  const res = await api.post("/v1/transfers", body, {
    headers: { "Idempotency-Key": idempotencyKey },
  });
  return res.data;
}

export async function listCategories() {
  const res = await api.get("/v1/categories");
  return res.data as Category[];
}
export async function createCategory(name: string) {
  const res = await api.post("/v1/categories", { name });
  return res.data as Category;
}
export async function confirmCategory(
  transactionId: number,
  categoryId: number,
) {
  const res = await api.post(`/v1/transactions/${transactionId}/category`, {
    category_id: categoryId,
  });
  return res.data;
}

export async function reverseTransaction(
  transactionId: number,
  idempotencyKey: string = crypto.randomUUID(),
) {
  const res = await api.post(
    `/v1/transactions/${transactionId}/reverse`,
    null,
    {
      headers: { "Idempotency-Key": idempotencyKey },
    },
  );
  return res.data;
}

export async function listEntries(
  accountId: number,
  cursor?: string,
  limit = 20,
) {
  const res = await api.get(`/v1/accounts/${accountId}/transactions`, {
    params: { limit, ...(cursor ? { cursor } : {}) },
  });
  return res.data as { entries: Entry[]; next_cursor: string };
}

export async function getMonthlySummary(accountId: number, ym: string) {
  const res = await api.get(`/v1/accounts/${accountId}/summary`, {
    params: { month: ym },
  });
  return res.data as { inflow: number; outflow: number; net: number };
}

export async function getCurrentBalance(accountId: number) {
  const res = await api.get(`/v1/accounts/${accountId}/balance`);
  return res.data as {
    account_id: number;
    as_of: string;
    balance_minor: number;
  };
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
export async function downloadExport(
  id: number,
  filename = `export_${id}.csv`,
) {
  const res = await api.get(`/v1/exports/${id}/download`, {
    responseType: "blob",
  });
  const url = URL.createObjectURL(res.data as Blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Give the browser time to begin the download before releasing its URL.
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
