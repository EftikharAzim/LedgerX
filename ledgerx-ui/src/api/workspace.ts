import api from "./client.ts";
import { AxiosError } from "axios";
import type { AccountAppearance } from "../lib/account-appearance.ts";

export type CorrectionDraft = {
  account_id: number;
  to_account_id?: number;
  amount_minor: number;
  currency: string;
  entry_type: "standard" | "refund" | "transfer" | "opening_balance";
  occurred_at: string;
  date_only: boolean;
  note: string;
  category_id: number;
};
export type CorrectionContext = {
  transaction_id: number;
  draft: CorrectionDraft;
  source_token: string;
};
export type BalanceChange = {
  account_id: number;
  name: string;
  currency: string;
  before_minor: number;
  change_minor: number;
  after_minor: number;
};
export type CorrectionPlan = {
  original_id: number;
  old: CorrectionDraft;
  new: CorrectionDraft;
  balances: BalanceChange[];
  source_token: string;
  preview_token: string;
};
export type CorrectionInput = {
  draft: CorrectionDraft;
  source_token: string;
  preview_token?: string;
};
export type CorrectionReceipt = {
  original_id: number;
  reversal_id: number;
  replacement_id?: number;
  old: CorrectionDraft;
  new: CorrectionDraft;
  balances: BalanceChange[];
  completed_at: string;
};

export async function getCorrectionContext(id: number) {
  return (
    await api.get<CorrectionContext>(`/v1/transactions/${id}/edit-context`)
  ).data;
}
export async function previewCorrection(id: number, body: CorrectionInput) {
  return (
    await api.post<CorrectionPlan>(
      `/v1/transactions/${id}/correction-preview`,
      body,
    )
  ).data;
}
export async function saveCorrection(
  id: number,
  body: CorrectionInput,
  key: string,
) {
  return (
    await api.post<CorrectionReceipt>(
      `/v1/transactions/${id}/corrections`,
      body,
      { headers: { "Idempotency-Key": key } },
    )
  ).data;
}
export async function getCorrectionResult(id: number, key: string) {
  return (
    await api.get<CorrectionReceipt>(
      `/v1/transactions/${id}/corrections/result`,
      { headers: { "Idempotency-Key": key } },
    )
  ).data;
}

export type WorkspaceSummary = {
  exported_at: string;
  accounts: {
    source_id: number;
    name: string;
    currency: string;
    active: boolean;
    balance_minor: number;
  }[];
  category_count: number;
  transaction_count: number;
  planned_count?: number;
  posting_count: number;
  import_count: number;
  correction_count: number;
  check_count: number;
};
export type RestoredAccount = {
  source_id: number;
  id: number;
  name: string;
  currency: string;
  appearance?: AccountAppearance;
};
export type RestoreReceipt = {
  id: number;
  filename: string;
  checksum: string;
  accounts: RestoredAccount[];
  summary: WorkspaceSummary;
  completed_at: string;
};
export type RestoreView = {
  id: number;
  filename: string;
  checksum: string;
  status: "preview" | "completed";
  summary: WorkspaceSummary;
  created_at: string;
  expires_at: string;
  receipt?: RestoreReceipt;
};
export type WorkspaceState = {
  empty: boolean;
  accounts: {
    id: number;
    name: string;
    currency: string;
    active: boolean;
    kind: "normal";
  }[];
  restores: {
    id: number;
    filename: string;
    status: "preview" | "completed";
    summary: WorkspaceSummary;
    receipt?: RestoreReceipt;
    created_at: string;
    expires_at: string;
  }[];
};
export async function getWorkspaceState() {
  return (await api.get<WorkspaceState>("/v1/workspace")).data;
}
export async function downloadWorkspace(
  appearance: Record<number, AccountAppearance>,
) {
  try {
    return (
      await api.post<Blob>(
        "/v1/workspace/backup",
        { appearance },
        { responseType: "blob", timeout: 60000 },
      )
    ).data;
  } catch (error) {
    if (error instanceof AxiosError && error.response?.data instanceof Blob) {
      const message = await error.response.data.text();
      throw new Error(message.trim() || "Couldn't create the backup.");
    }
    throw error;
  }
}
export async function previewRestore(filename: string, raw: string) {
  return (
    await api.post<RestoreView>("/v1/workspace/restores/preview", raw, {
      params: { filename },
      headers: { "Content-Type": "application/json" },
      timeout: 60000,
    })
  ).data;
}
export async function getRestore(id: number) {
  return (await api.get<RestoreView>(`/v1/workspace/restores/${id}`)).data;
}
export async function commitRestore(id: number, checksum: string) {
  return (
    await api.post<RestoreReceipt>(
      `/v1/workspace/restores/${id}/commit`,
      { checksum },
      { timeout: 120000 },
    )
  ).data;
}
