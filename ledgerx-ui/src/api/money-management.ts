import api from "./client";
import type { Category } from "./endpoints";
import type { ReportSelection } from "../lib/report-period";

export type CategoryEdit = {
  name?: string;
  parent_id?: number;
  icon?: string;
  kind?: string;
  archived?: boolean;
};
export type CategoryImpact = {
  transactions: number;
  plans: number;
  children: number;
  budgets: number;
  review_token: string;
};
export async function editCategory(id: number, body: CategoryEdit) {
  return (await api.patch<Category>(`/v1/categories/${id}`, body)).data;
}
export async function categoryImpact(id: number) {
  return (await api.get<CategoryImpact>(`/v1/categories/${id}/impact`)).data;
}
export async function deleteCategory(
  id: number,
  replacement_id: number,
  review_token: string,
) {
  return (
    await api.delete(`/v1/categories/${id}`, {
      data: { replacement_id, review_token },
    })
  ).data;
}
export type Budget = {
  id: number;
  month: string;
  currency: string;
  category_id: number;
  amount_minor: number;
  category_name: string;
  parent_id: number;
  archived: boolean;
  spent_minor: number;
  remaining_minor: number;
};
export async function listBudgets(month: string, currency: string) {
  return (
    await api.get<Budget[]>("/v1/budgets", { params: { month, currency } })
  ).data;
}
export async function saveBudget(body: {
  id?: number;
  month: string;
  currency: string;
  category_id: number;
  amount_minor: number;
}) {
  return (await api.put("/v1/budgets", body)).data;
}
export async function deleteBudget(id: number) {
  return api.delete(`/v1/budgets/${id}`);
}
export async function copyBudgets(
  from_month: string,
  to_month: string,
  currency: string,
) {
  return (
    await api.post<{ added: number }>("/v1/budgets/copy", {
      from_month,
      to_month,
      currency,
    })
  ).data;
}
export type ReportTotals = {
  income_minor: number;
  spending_minor: number;
  net_minor: number;
};
export type ReportCategory = {
  category_id: number;
  parent_id: number;
  name: string;
  income_minor: number;
  spending_minor: number;
  count: number;
};
export type FinancialReport = ReportTotals & {
  currency: string;
  start_date: string;
  end_date: string;
  opening_minor: number;
  ending_minor: number;
  other_movement_minor: number;
  categories: ReportCategory[];
  previous?: ReportTotals;
  change?: ReportTotals;
  previous_start?: string;
  previous_end?: string;
  trends: (ReportTotals & { month: string })[];
};
export async function getFinancialReport(
  selection: ReportSelection,
  currency: string,
) {
  return (
    await api.get<FinancialReport>("/v1/reports/summary", {
      params: { ...selection, currency },
    })
  ).data;
}
