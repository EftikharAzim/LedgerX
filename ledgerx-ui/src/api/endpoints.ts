import api from "./client";

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
  return res.data as Array<{ id: number; name: string; balance_minor?: number }>;
}
export async function createAccount(name: string, currency = "USD") {
  const res = await api.post("/v1/accounts", { name, currency });
  return res.data;
}

export async function createTransaction(body: {
  user_id: number; account_id: number; amount_minor: number;
  currency: string; occurred_at: string; note?: string;
}) {
  const res = await api.post("/v1/transactions", body, {
    // show idempotency concept in UI by letting users retry safely (optional)
    headers: { "Idempotency-Key": crypto.randomUUID() },
  });
  return res.data;
}

export async function getMonthlySummary(accountId: number, ym: string) {
  const res = await api.get(`/v1/accounts/${accountId}/summary`, { params: { month: ym } });
  return res.data as { inflow: number; outflow: number; net: number };
}

export async function requestExport(month: string) {
  const res = await api.post(`/exports`, null, { params: { month } });
  return res.data as { id: number; status: string };
}
export async function getExportStatus(id: number) {
  const res = await api.get(`/exports/${id}/status`);
  return res.data as { id: number; status: string; file_path?: string };
}