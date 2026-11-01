import api from "./client";

export type Schedule = {
  id: number;
  account_id: number;
  account_name: string;
  currency: string;
  amount_minor: number;
  category_id?: number;
  category_name: string;
  note: string;
  cadence: "weekly" | "monthly" | "yearly";
  first_due_date: string;
  end_date?: string;
  next_due_date: string;
  status: "active" | "paused" | "ended";
  revision: number;
};
export type ScheduleInput = {
  account_id: number;
  currency: string;
  amount_minor: number;
  category_id: number;
  note: string;
  cadence: Schedule["cadence"];
  first_due_date: string;
  end_date: string;
  revision?: number;
};
export async function listSchedules() {
  return (await api.get<Schedule[]>("/v1/schedules")).data;
}
export async function saveSchedule(
  body: ScheduleInput,
  key: string,
  id?: number,
) {
  return (
    await api.request<Schedule>({
      method: id ? "PUT" : "POST",
      url: id ? `/v1/schedules/${id}` : "/v1/schedules",
      data: body,
      headers: { "Idempotency-Key": key },
    })
  ).data;
}
export async function scheduleAction(
  schedule: Schedule,
  action: "pause" | "resume" | "end",
) {
  return (
    await api.post<Schedule>(`/v1/schedules/${schedule.id}/${action}`, {
      revision: schedule.revision,
    })
  ).data;
}
export async function syncSchedules() {
  return (await api.post("/v1/schedules/sync", {})).data;
}
export type CashflowDay = {
  date: string;
  in_minor: number;
  out_minor: number;
  projected_minor: number;
  count: number;
};
export type WalletCashflow = {
  account_id: number;
  name: string;
  currency: string;
  active: boolean;
  current_minor: number;
  in_minor: number;
  out_minor: number;
  projected_minor: number;
  lowest_minor: number;
  overdue_count: number;
  days: CashflowDay[];
};
export type Cashflow = {
  as_of: string;
  through_date: string;
  wallets: WalletCashflow[];
};
export async function getCashflow() {
  return (await api.get<Cashflow>("/v1/planning/cashflow")).data;
}
