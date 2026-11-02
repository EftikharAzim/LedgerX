import api from "./client.ts";
import { downloadBlob } from "../lib/download.ts";
export { formatMoney } from "../lib/money.ts";
import type { ReportSelection } from "../lib/report-period.ts";

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
  entry_type?: "standard" | "opening_balance" | "refund";
  import_id?: number;
  import_date_only?: boolean;
  exclude_from_reports?: boolean;
  correction_of?: number;
  corrected_by?: number;
};

export type ActivityEntry = Entry & {
  account_name: string;
  counterparty_account_id?: number;
  counterparty_account_name?: string;
  activity_type: string;
};

export type ActivityFilters = {
  transaction_id?: string;
  import_id?: string;
  search?: string;
  account_id?: string;
  category_id?: string;
  type?: string;
  currency?: string;
  start_date?: string;
  end_date?: string;
};

export type Spending = {
  currency: string;
  income_minor: number;
  spending_minor: number;
  net_minor: number;
  spending_count: number;
  categories: {
    category_id: number;
    name: string;
    spending_minor: number;
    count: number;
  }[];
};

export async function getSpending(
  selection: ReportSelection,
  currency: string,
) {
  const res = await api.get("/v1/spending", {
    params: { ...selection, currency },
  });
  return res.data as Spending;
}

export async function listActivity(filters: ActivityFilters, cursor?: string) {
  const res = await api.get("/v1/activity", {
    params: { ...filters, ...(cursor ? { cursor } : {}) },
  });
  return res.data as { entries: ActivityEntry[]; next_cursor: string };
}

export type Category = {
  id: number;
  user_id: number;
  name: string;
  parent_id?: number;
  icon?: string;
  kind?: string;
  archived?: boolean;
};

export type CategorySuggestionInput = {
  note: string;
  currency: string;
  kind: "expense" | "income" | "refund";
};
export type CategorySuggestion = {
  category_id: number;
  category_name: string;
  source: "history" | "keyword" | "ambiguous" | "none";
  reason: string;
  supporting_records: number;
  matching_records: number;
};

export async function suggestCategory(input: CategorySuggestionInput) {
  const res = await api.post<CategorySuggestion>(
    "/v1/categories/suggest",
    input,
  );
  return res.data;
}

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
  const accounts: Account[] = [];
  for (let offset = 0; ; offset += 100) {
    const res = await api.get<Account[]>("/v1/accounts", {
      params: { limit: 100, offset },
    });
    accounts.push(...res.data);
    if (res.data.length < 100) return accounts;
  }
}
export async function createAccount(
  name: string,
  currency = "USD",
  opening?: { opening_balance_minor: number; opening_balance_at?: string },
  idempotencyKey?: string,
) {
  const res = await api.post(
    "/v1/accounts",
    { name, currency, ...opening },
    { headers: idempotencyKey ? { "Idempotency-Key": idempotencyKey } : {} },
  );
  return res.data as Account;
}

export async function updateAccount(
  account: Account,
  change: { name?: string; active?: boolean },
) {
  const res = await api.patch<Account>(`/v1/accounts/${account.id}`, {
    ...change,
    expected_name: account.name,
    expected_active: account.active,
  });
  return res.data;
}

export async function createTransaction(
  body: {
    account_id: number;
    amount_minor: number;
    currency: string;
    occurred_at: string;
    note?: string;
    category_id?: number;
    entry_type?: "refund";
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
export async function createCategory(
  name: string,
  metadata: { parent_id?: number; icon?: string; kind?: string } = {},
) {
  const res = await api.post("/v1/categories", { name, ...metadata });
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

export async function requestExport(selection: ReportSelection | string) {
  const res = await api.post(`/v1/exports`, null, {
    params: typeof selection === "string" ? { month: selection } : selection,
  });
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
  downloadBlob(res.data as Blob, filename);
}
