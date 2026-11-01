import api from "./client";

export type PlannedEntry = {
  id: number;
  schedule_id?: number;
  account_id: number;
  account_name: string;
  category_name: string;
  currency: string;
  amount_minor: number;
  occurred_at: string;
  date_only: boolean;
  note: string;
  entry_type: string;
  exclude_from_reports: boolean;
};

export async function listPlans(offset = 0, view = "all", currency = "") {
  return (
    await api.get<{ entries: PlannedEntry[]; next_offset: number }>(
      "/v1/plans",
      { params: { offset, view, currency } },
    )
  ).data;
}

export async function createPlan(
  body: {
    account_id: number;
    amount_minor: number;
    currency: string;
    occurred_at: string;
    note: string;
    category_id?: number;
    entry_type?: "refund";
  },
  key: string,
) {
  return (
    await api.post("/v1/plans", body, { headers: { "Idempotency-Key": key } })
  ).data;
}

export async function resolvePlan(
  id: number,
  action: "post" | "cancel",
  date?: string,
  amount_minor?: number,
) {
  return (
    await api.post(
      `/v1/plans/${id}/${action}`,
      action === "post" ? { date, amount_minor } : {},
    )
  ).data;
}
