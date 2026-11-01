import api from "./client.ts";

export type CSVInspection = {
  headers: string[];
  delimiter: string;
  row_count: number;
  samples: string[][];
};
export type ImportMapping = {
  date?: number;
  amount?: number;
  debit?: number;
  credit?: number;
  note?: number;
  category?: number;
  currency?: number;
  type?: number;
  reference?: number;
  amount_mode: "signed" | "debit_credit";
  date_format: "" | "iso" | "day_first" | "month_first";
  number_style: "dot" | "comma";
  sign_convention: "income_positive" | "expense_positive";
};
export type ImportContext = {
  account_id: number;
  name: string;
  currency: string;
  active: boolean;
  balance_minor: number;
  revision: number;
  entry_count: number;
  opening_id: number;
  opening_minor: number;
  opening_at?: string;
  opening_count: number;
  tracking_start_at?: string;
  token: string;
};
export type StartingPlan = {
  mode: "existing" | "set";
  amount_minor: number;
  date: string;
};
export type ImportDuplicate = {
  transaction_id?: number;
  row_number?: number;
  occurred_at?: string;
  amount_minor?: number;
  note?: string;
  match_type: "reference" | "exact" | "date_amount" | "file";
};
export type ImportRow = {
  number: number;
  occurred_at: string;
  date_only: boolean;
  amount_minor: number;
  currency: string;
  entry_type: string;
  note: string;
  category: string;
  source_ref: string;
  fingerprint: string;
  errors: string[];
  duplicates: ImportDuplicate[];
  new_category: boolean;
};
export type ImportPreview = {
  context: ImportContext;
  rows: ImportRow[];
  starting_balance: StartingPlan;
  mapping: ImportMapping;
  new_categories: string[];
  plan_errors: string[];
  projected_balance_minor: number;
  ready_count: number;
  duplicate_count: number;
  invalid_count: number;
};
export type ImportReceipt = {
  id: number;
  account_id: number;
  account_name: string;
  currency: string;
  filename: string;
  imported_count: number;
  skipped_count: number;
  invalid_count: number;
  duplicate_skipped: number;
  balance_before_minor: number;
  balance_after_minor: number;
  imported_minor: number;
  starting_before_minor: number;
  starting_after_minor: number;
  new_categories: number;
  transaction_ids: number[];
  completed_at: string;
};
export type ImportView = {
  id: number;
  account_id: number;
  filename: string;
  status: "preview" | "completed";
  created_at: string;
  expires_at: string;
  preview?: ImportPreview;
  receipt?: ImportReceipt;
};
export type ImportHistory = {
  id: number;
  account_id: number;
  filename: string;
  status: "preview" | "completed";
  created_at: string;
  receipt?: ImportReceipt;
};
export type ImportInput = {
  account_id: number;
  filename: string;
  csv: string;
  delimiter: string;
  mapping: ImportMapping;
  starting_balance: StartingPlan;
};
export type BalanceCheck = {
  id: number;
  account_id: number;
  actual_minor: number;
  recorded_minor: number;
  difference_minor: number;
  ledger_revision: number;
  note: string;
  created_at: string;
};
export type BalanceCheckHistory = {
  checks: BalanceCheck[];
  current_revision: number;
  recorded_balance_minor: number;
  currency: string;
};

export async function inspectCSV(csv: string, delimiter: string) {
  return (
    await api.post<CSVInspection>("/v1/imports/inspect", { csv, delimiter })
  ).data;
}
export async function getImportContext(id: number) {
  return (await api.get<ImportContext>(`/v1/accounts/${id}/import-context`))
    .data;
}
export async function previewImport(body: ImportInput) {
  return (await api.post<ImportView>("/v1/imports/preview", body)).data;
}
export async function getImport(id: number) {
  return (await api.get<ImportView>(`/v1/imports/${id}`)).data;
}
export async function listImports() {
  return (await api.get<ImportHistory[]>("/v1/imports")).data;
}
export async function commitImport(
  id: number,
  rows: number[],
  duplicateApprovals: number[],
) {
  return (
    await api.post<ImportReceipt>(`/v1/imports/${id}/commit`, {
      rows,
      duplicate_approvals: duplicateApprovals,
    })
  ).data;
}
export async function listBalanceChecks(id: number) {
  return (
    await api.get<BalanceCheckHistory>(`/v1/accounts/${id}/balance-checks`)
  ).data;
}
export async function createBalanceCheck(
  id: number,
  body: { actual_minor: number; note: string },
  key: string,
) {
  return (
    await api.post<BalanceCheck>(`/v1/accounts/${id}/balance-checks`, body, {
      headers: { "Idempotency-Key": key },
    })
  ).data;
}
